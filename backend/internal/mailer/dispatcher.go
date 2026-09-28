package mailer

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

type Dispatcher struct {
	store     *store.Store
	registry  *Registry
	sender    Sender
	logger    *slog.Logger
	now       func() time.Time
	maxTries  int
	baseDelay time.Duration
	from      string
}

type DispatcherOptions struct {
	MaxAttempts int
	BaseBackoff time.Duration
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
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	return &Dispatcher{
		store:     data,
		registry:  registry,
		sender:    sender,
		logger:    options.Logger,
		now:       options.Clock,
		maxTries:  options.MaxAttempts,
		baseDelay: options.BaseBackoff,
		from:      options.FromAddress,
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

// DispatchOnce claims a batch of due messages and attempts delivery.
func (d *Dispatcher) DispatchOnce(limit int) (int, error) {
	now := d.now().UTC()
	claimed := d.store.ClaimMail(now, limit)
	delivered := 0
	for _, message := range claimed {
		if err := d.deliver(message); err != nil {
			giveUp := message.Attempts >= d.maxTries
			retryAt := now.Add(d.Backoff(message.Attempts))
			if markErr := d.store.MarkMailFailed(message.ID, err.Error(), retryAt, giveUp); markErr != nil {
				return delivered, markErr
			}
			if giveUp {
				d.logger.Error("mail gave up after retries", "mail_id", message.ID, "to", message.Email, "error", err)
			} else {
				d.logger.Warn("mail delivery failed", "mail_id", message.ID, "to", message.Email, "attempt", message.Attempts, "retry_at", retryAt, "error", err)
			}
			continue
		}
		delivered++
	}
	return delivered, nil
}

func (d *Dispatcher) deliver(message domain.MailMessage) error {
	from := d.from
	if from == "" {
		from = "no-reply@codeceremony.local"
	}
	if err := d.sender.Send(Message{
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
			if _, err := d.DispatchOnce(batch); err != nil {
				d.logger.Error("mail dispatch cycle failed", "error", err)
			}
		}
	}
}

func (d *Dispatcher) SenderName() string {
	return d.sender.Name()
}
