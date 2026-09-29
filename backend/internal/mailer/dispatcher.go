package mailer

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

// A send that hangs would otherwise hold the whole batch: the SMTP dial alone
// waits 15 seconds, and the run loop is the only mail worker in the process.
const (
	defaultSendTimeout   = 20 * time.Second
	defaultBatchParallel = 4
)

type Dispatcher struct {
	store      *store.Store
	registry   *Registry
	sender     Sender
	logger     *slog.Logger
	now        func() time.Time
	maxTries   int
	baseDelay  time.Duration
	from       string
	sendWindow time.Duration
	parallel   int
}

type DispatcherOptions struct {
	MaxAttempts int
	BaseBackoff time.Duration
	// SendTimeout bounds a single delivery, including its SMTP dial. Zero means
	// defaultSendTimeout.
	SendTimeout time.Duration
	// Parallelism is the number of sends DispatchOnce runs at once. Zero means
	// defaultBatchParallel.
	Parallelism int
	SenderName  string
	FromAddress string
	Clock       func() time.Time
	Logger      *slog.Logger
}

func NewDispatcher(data *store.Store, registry *Registry, sender Sender, options DispatcherOptions) *Dispatcher {
	if options.MaxAttempts <= 0 {
		options.MaxAttempts = 4
	}
	if options.BaseBackoff <= 0 {
		options.BaseBackoff = 30 * time.Second
	}
	if options.SendTimeout <= 0 {
		options.SendTimeout = defaultSendTimeout
	}
	if options.Parallelism <= 0 {
		options.Parallelism = defaultBatchParallel
	}
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	return &Dispatcher{
		store:      data,
		registry:   registry,
		sender:     sender,
		logger:     options.Logger,
		now:        options.Clock,
		maxTries:   options.MaxAttempts,
		baseDelay:  options.BaseBackoff,
		from:       options.FromAddress,
		sendWindow: options.SendTimeout,
		parallel:   options.Parallelism,
	}
}

// Backoff returns the retry delay for an attempt using exponential backoff.
func (d *Dispatcher) Backoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := time.Duration(float64(d.baseDelay) * math.Pow(2, float64(attempts-1)))
	if delay > 30*time.Minute {
		delay = 30 * time.Minute
	}
	return delay
}

// Queue renders a template and places the message in the outbox, honouring the
// recipient preferences. A skipped message is still recorded for the audit trail.
func (d *Dispatcher) Queue(message domain.MailMessage, values Context) (domain.MailMessage, error) {
	tmpl, ok := d.registry.Get(message.Template)
	if !ok {
		return domain.MailMessage{}, fmt.Errorf("unknown mail template %q", message.Template)
	}
	if message.Kind == "" {
		message.Kind = tmpl.Kind
	}
	if message.Topic == "" {
		message.Topic = tmpl.Topic
	}
	rendered, err := d.registry.Render(message.Template, values)
	if err != nil {
		return domain.MailMessage{}, err
	}
	message.Subject = rendered.Subject
	message.BodyText = rendered.TextBody
	message.BodyHTML = rendered.HTMLBody
	if err := message.Validate(); err != nil {
		return domain.MailMessage{}, err
	}
	if message.UserID != "" {
		preferences := d.store.MailPreferences(message.UserID)
		if !preferences.Allows(message.Topic) {
			message.Status = domain.MailSkipped
			message.LastError = "recipient preferences do not allow topic " + string(message.Topic)
			queued, queueErr := d.store.QueueMail(message)
			if queueErr != nil {
				return domain.MailMessage{}, queueErr
			}
			if err := d.store.MarkMailSkipped(queued.ID, message.LastError); err != nil {
				return domain.MailMessage{}, err
			}
			return d.store.MailByID(queued.ID)
		}
	}
	return d.store.QueueMail(message)
}

// DispatchOnce claims a batch of due messages and attempts delivery. The sends
// run with a small bounded concurrency under per-send deadlines, so an
// unreachable relay delays only its own messages instead of the whole batch.
func (d *Dispatcher) DispatchOnce(limit int) (int, error) {
	now := d.now().UTC()
	claimed := d.store.ClaimMail(now, limit)
	// The store call is the only shared mutation from a worker, so its result
	// is the only thing that needs collecting after the wait.
	results := make([]bool, len(claimed))
	failures := make([]error, len(claimed))
	slots := make(chan struct{}, d.parallel)
	wait := &sync.WaitGroup{}
	for index, message := range claimed {
		wait.Add(1)
		go func(index int, message domain.MailMessage) {
			defer wait.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			results[index], failures[index] = d.dispatchOne(now, message)
		}(index, message)
	}
	wait.Wait()
	delivered := 0
	var firstErr error
	for index := range claimed {
		if results[index] {
			delivered++
		}
		if failures[index] != nil && firstErr == nil {
			firstErr = failures[index]
		}
	}
	return delivered, firstErr
}

// dispatchOne sends a single claimed message and applies the retry accounting
// for it. The accounting is deliberately identical to the sequential form:
// attempts were already incremented by the claim, and the next attempt is
// scheduled from that same count.
func (d *Dispatcher) dispatchOne(now time.Time, message domain.MailMessage) (bool, error) {
	if err := d.deliver(context.Background(), message); err != nil {
		giveUp := message.Attempts >= d.maxTries
		retryAt := now.Add(d.Backoff(message.Attempts))
		if markErr := d.store.MarkMailFailed(message.ID, err.Error(), retryAt, giveUp); markErr != nil {
			return false, markErr
		}
		if giveUp {
			d.logger.Error("mail gave up after retries", "mail_id", message.ID, "to", message.Email, "error", err)
		} else {
			d.logger.Warn("mail delivery failed", "mail_id", message.ID, "to", message.Email, "attempt", message.Attempts, "retry_at", retryAt, "error", err)
		}
		return false, nil
	}
	return true, nil
}

// deliver sends one message under its own deadline. A panic inside a send is
// converted into an ordinary delivery failure so the retry accounting still
// applies and the worker survives.
func (d *Dispatcher) deliver(ctx context.Context, message domain.MailMessage) (err error) {
	from := d.from
	if from == "" {
		from = "no-reply@codeceremony.local"
	}
	if d.sendWindow > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d.sendWindow)
		defer cancel()
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("mail delivery panicked: %v", recovered)
		}
	}()
	if err := d.sender.Send(ctx, Message{
		From:    from,
		To:      message.Email,
		Subject: message.Subject,
		Text:    message.BodyText,
		HTML:    message.BodyHTML,
	}); err != nil {
		return err
	}
	return d.store.MarkMailSent(message.ID, d.now().UTC())
}

// Run delivers queued mail until the context is cancelled.
func (d *Dispatcher) Run(ctx context.Context, interval time.Duration, batch int) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	if batch <= 0 {
		batch = 20
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.cycle(batch)
		}
	}
}

// cycle runs one dispatch pass. It recovers because this loop is started bare
// from the process entry point: an unrecovered panic would end the goroutine and
// silently stop all mail for the life of the process.
func (d *Dispatcher) cycle(batch int) {
	defer func() {
		if recovered := recover(); recovered != nil {
			d.logger.Error("mail dispatch cycle panicked", "panic", recovered)
		}
	}()
	if _, err := d.DispatchOnce(batch); err != nil {
		d.logger.Error("mail dispatch cycle failed", "error", err)
	}
}

func (d *Dispatcher) SenderName() string {
	return d.sender.Name()
}
