package mailer

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/seed"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

// probeSender records how many sends were in flight at the same time and can
// hold one recipient open until its deadline, which is how the batch behaviour
// is observed without a network.
type probeSender struct {
	mu       sync.Mutex
	inFlight int
	peak     int
	sent     []string
	// hangFor is the recipient whose send only ends when its context is done,
	// standing in for an unreachable relay with a long dial timeout.
	hangFor string
	// hold is how long every other send occupies a worker slot.
	hold time.Duration
	// panicOnce makes the first send panic, to model a buggy or hostile relay.
	panicOnce bool
	failures  int
}

func (p *probeSender) Name() string { return "probe" }

func (p *probeSender) Send(ctx context.Context, message Message) error {
	p.mu.Lock()
	p.inFlight++
	if p.inFlight > p.peak {
		p.peak = p.inFlight
	}
	panics := p.panicOnce
	p.panicOnce = false
	hangs := p.hangFor != "" && message.To == p.hangFor
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.inFlight--
		p.mu.Unlock()
	}()

	if panics {
		panic("relay exploded")
	}
	if hangs {
		<-ctx.Done()
		p.recordFailure()
		return ctx.Err()
	}
	if p.hold > 0 {
		select {
		case <-time.After(p.hold):
		case <-ctx.Done():
			p.recordFailure()
			return ctx.Err()
		}
	}
	p.mu.Lock()
	p.sent = append(p.sent, message.To)
	p.mu.Unlock()
	return nil
}

func (p *probeSender) recordFailure() {
	p.mu.Lock()
	p.failures++
	p.mu.Unlock()
}

func (p *probeSender) snapshot() (peak int, sent []string, failures int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.peak, append([]string(nil), p.sent...), p.failures
}

func newTestDispatcher(t *testing.T, sender Sender, options DispatcherOptions) (*store.Store, *Dispatcher) {
	t.Helper()
	hash, err := auth.HashPassword("mailer-dispatch-password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if options.Logger == nil {
		options.Logger = slog.New(slog.NewTextHandler(discard{}, nil))
	}
	if options.BaseBackoff <= 0 {
		options.BaseBackoff = time.Millisecond
	}
	data := store.New(seed.Default(hash))
	return data, NewDispatcher(data, NewRegistry(), sender, options)
}

// queueBatch fills the outbox with messages to distinct recipients. UserID is
// left empty so the queue does not consult per-recipient preferences and the
// test only exercises delivery.
func queueBatch(t *testing.T, dispatcher *Dispatcher, count int) []domain.MailMessage {
	t.Helper()
	queued := make([]domain.MailMessage, 0, count)
	for index := range count {
		message, err := dispatcher.Queue(domain.MailMessage{
			Email:    fmt.Sprintf("recipient-%02d@example.org", index),
			Template: "welcome",
		}, Context{
			"DisplayName":    "Pia Participant",
			"AppURL":         "https://app.example.org",
			"PreferencesURL": "https://app.example.org/settings/notifications",
		})
		if err != nil {
			t.Fatalf("Queue() error = %v", err)
		}
		queued = append(queued, message)
	}
	return queued
}

func TestDispatchOnceOverlapsSendsAndHonoursTheParallelismLimit(t *testing.T) {
	// A batch of six 60ms sends costs 360ms in sequence. The pool exists so an
	// unreachable endpoint costs its own messages one timeout, not the batch's.
	sender := &probeSender{hold: 60 * time.Millisecond}
	_, dispatcher := newTestDispatcher(t, sender, DispatcherOptions{Parallelism: 3, SendTimeout: 5 * time.Second})
	queueBatch(t, dispatcher, 6)

	started := time.Now()
	delivered, err := dispatcher.DispatchOnce(10)
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if delivered != 6 {
		t.Fatalf("delivered = %d, want 6", delivered)
	}
	peak, sent, failures := sender.snapshot()
	if len(sent) != 6 || failures != 0 {
		t.Fatalf("sent = %d, failures = %d, want 6 and 0", len(sent), failures)
	}
	if peak < 2 {
		t.Fatalf("the batch was sent one at a time, peak in flight = %d", peak)
	}
	if peak > 3 {
		t.Fatalf("the bounded pool was exceeded, peak in flight = %d", peak)
	}
	if elapsed >= 300*time.Millisecond {
		t.Fatalf("the batch took %s, which is the sequential cost rather than the pooled one", elapsed)
	}
}

func TestDispatchOnceGivesEachSendItsOwnDeadline(t *testing.T) {
	sender := &probeSender{hangFor: "recipient-00@example.org"}
	data, dispatcher := newTestDispatcher(t, sender, DispatcherOptions{Parallelism: 4, SendTimeout: 50 * time.Millisecond})
	queued := queueBatch(t, dispatcher, 4)

	started := time.Now()
	delivered, err := dispatcher.DispatchOnce(10)
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if delivered != 3 {
		t.Fatalf("delivered = %d, want the three reachable recipients", delivered)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("the batch took %s, so the hung send was not bounded by its own deadline", elapsed)
	}

	// The hung message is accounted for exactly as any other delivery failure:
	// one attempt consumed, requeued, and scheduled from that same count.
	hang, err := data.MailByID(queued[0].ID)
	if err != nil {
		t.Fatalf("MailByID() error = %v", err)
	}
	if hang.Attempts != 1 {
		t.Fatalf("attempts = %d, want exactly one attempt consumed", hang.Attempts)
	}
	if hang.Status != domain.MailQueued {
		t.Fatalf("status = %q, want the message requeued for a retry", hang.Status)
	}
	if hang.LastError == "" {
		t.Fatalf("expected the deadline to be recorded as the failure reason")
	}
	if !hang.ScheduledAt.After(started) {
		t.Fatalf("scheduled at %s, want a backoff after the failed attempt", hang.ScheduledAt)
	}
	for _, message := range queued[1:] {
		sent, err := data.MailByID(message.ID)
		if err != nil {
			t.Fatalf("MailByID() error = %v", err)
		}
		if sent.Status != domain.MailSent {
			t.Fatalf("recipient %s: status = %q, want sent despite the hung neighbour", message.Email, sent.Status)
		}
	}
}

func TestDispatchCycleRecoversFromAPanicAndKeepsWorking(t *testing.T) {
	sender := &probeSender{}
	_, dispatcher := newTestDispatcher(t, sender, DispatcherOptions{Parallelism: 2, SendTimeout: time.Second})

	// A negative limit makes the store's claim allocate a slice with a negative
	// capacity, which panics inside DispatchOnce. The loop is started bare from
	// the process entry point, so the guard has to be here rather than in the
	// caller.
	dispatcher.cycle(-1)

	// The worker is still usable afterwards, which is the point of recovering.
	queueBatch(t, dispatcher, 1)
	delivered, err := dispatcher.DispatchOnce(5)
	if err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if delivered != 1 {
		t.Fatalf("delivered = %d, want the queue to still drain after the recovered panic", delivered)
	}
}

func TestRunSurvivesAPanickingSendAndKeepsTicking(t *testing.T) {
	sender := &probeSender{panicOnce: true}
	_, dispatcher := newTestDispatcher(t, sender, DispatcherOptions{
		Parallelism: 2,
		SendTimeout: 2 * time.Second,
		MaxAttempts: 5,
	})
	queueBatch(t, dispatcher, 2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		dispatcher.Run(ctx, 2*time.Millisecond, 10)
	}()

	// Both messages land: the panicking one is retried on a later tick, which
	// is only possible if the panic neither unwound the loop nor stalled it.
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, sent, _ := sender.snapshot()
		if len(sent) >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the run loop stopped after the panic, only delivered %v", sent)
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatalf("Run did not return after the context was cancelled")
	}
}
