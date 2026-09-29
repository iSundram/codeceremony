package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

// errRedirectRefused is reported instead of following a redirect. The signed
// body and its signature header are re-attached to every hop, so a redirect to
// a host the organizer does not control would hand the event data to that host
// with a valid-looking signature.
var errRedirectRefused = errors.New("webhook endpoint redirected; delivery refused")

type HookTransport interface {
	Do(request *http.Request) (statusCode int, body string, err error)
}

type HTTPHookTransport struct {
	client *http.Client
}

func NewHTTPHookTransport() *HTTPHookTransport {
	return &HTTPHookTransport{client: &http.Client{Timeout: 10 * time.Second, CheckRedirect: refuseRedirect}}
}

func refuseRedirect(*http.Request, []*http.Request) error {
	return errRedirectRefused
}

func (t *HTTPHookTransport) Do(request *http.Request) (int, string, error) {
	response, err := t.client.Do(request)
	if err != nil {
		return 0, "", err
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 4096))
	if readErr != nil {
		return response.StatusCode, "", readErr
	}
	return response.StatusCode, string(body), nil
}

const (
	defaultDeliveryTimeout  = 10 * time.Second
	defaultDeliveryParallel = 4
)

// Webhooks fans events out to organizer endpoints. Like the mail dispatcher,
// deliveries run with a bounded concurrency and a per-delivery deadline so one
// hanging endpoint cannot stall the queue.
type Webhooks struct {
	store       *store.Store
	transport   HookTransport
	logger      *slog.Logger
	now         func() time.Time
	maxTries    int
	baseBackoff time.Duration
	timeout     time.Duration
	parallel    int
}

func NewWebhooks(data *store.Store, transport HookTransport, logger *slog.Logger, maxTries int, baseBackoff time.Duration) *Webhooks {
	if logger == nil {
		logger = slog.Default()
	}
	if transport == nil {
		transport = NewHTTPHookTransport()
	}
	if maxTries <= 0 {
		maxTries = 4
	}
	if baseBackoff <= 0 {
		baseBackoff = 15 * time.Second
	}
	return &Webhooks{
		store:       data,
		transport:   transport,
		logger:      logger,
		now:         time.Now,
		maxTries:    maxTries,
		baseBackoff: baseBackoff,
		timeout:     defaultDeliveryTimeout,
		parallel:    defaultDeliveryParallel,
	}
}

// Emit fans an event out to every active webhook subscribed to it. The
// eventID argument scopes the fan-out; an empty eventID means the platform scope.
func (w *Webhooks) Emit(eventID, event string, hackathonSlug string, data map[string]any) []domain.WebhookDelivery {
	envelopes := make([]domain.WebhookDelivery, 0)
	for _, webhook := range w.store.Webhooks(eventID) {
		if !webhook.Active || !webhook.SubscribedTo(event) {
			continue
		}
		envelope := domain.Envelope{
			ID:        domain.NewID("evt"),
			Event:     event,
			Hackathon: hackathonSlug,
			CreatedAt: w.now().UTC(),
			Data:      data,
		}
		payload, err := json.Marshal(envelope)
		if err != nil {
			w.logger.Error("webhook payload could not be encoded", "webhook_id", webhook.ID, "error", err)
			continue
		}
		delivery, err := w.store.EnqueueDelivery(domain.WebhookDelivery{
			WebhookID: webhook.ID,
			EventID:   eventID,
			Event:     event,
			Payload:   string(payload),
		})
		if err != nil {
			w.logger.Error("webhook delivery could not be queued", "webhook_id", webhook.ID, "error", err)
			continue
		}
		envelopes = append(envelopes, delivery)
	}
	return envelopes
}

func (w *Webhooks) Backoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := w.baseBackoff * time.Duration(1<<uint(attempts-1))
	if delay > time.Hour {
		delay = time.Hour
	}
	return delay
}

// DeliverOnce claims due deliveries and posts them. Sends overlap under a
// bounded concurrency; the retry accounting per delivery is unchanged.
func (w *Webhooks) DeliverOnce(limit int) (int, error) {
	now := w.now().UTC()
	claimed := w.store.ClaimDeliveries(now, limit)
	results := make([]bool, len(claimed))
	failures := make([]error, len(claimed))
	slots := make(chan struct{}, w.parallel)
	wait := &sync.WaitGroup{}
	for index, delivery := range claimed {
		wait.Add(1)
		go func(index int, delivery domain.WebhookDelivery) {
			defer wait.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			results[index], failures[index] = w.deliverOne(now, delivery)
		}(index, delivery)
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

func (w *Webhooks) deliverOne(now time.Time, delivery domain.WebhookDelivery) (bool, error) {
	updated, err := w.deliverWithin(context.Background(), delivery)
	delivery = updated
	if err != nil {
		giveUp := delivery.Attempts >= w.maxTries
		delivery.Status = domain.DeliveryFailed
		delivery.LastError = err.Error()
		if !giveUp {
			delivery.Status = domain.DeliveryPending
			delivery.NextAttempt = now.Add(w.Backoff(delivery.Attempts))
		}
		if markErr := w.store.MarkDelivery(delivery.ID, delivery); markErr != nil {
			return false, markErr
		}
		return false, nil
	}
	return true, nil
}

// deliverWithin posts one delivery under its own deadline and turns a panic in
// the transport into an ordinary failure, so neither a bad endpoint nor a buggy
// call site can stop the delivery loop.
func (w *Webhooks) deliverWithin(ctx context.Context, delivery domain.WebhookDelivery) (updated domain.WebhookDelivery, err error) {
	if w.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, w.timeout)
		defer cancel()
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			updated, err = delivery, fmt.Errorf("webhook delivery panicked: %v", recovered)
		}
	}()
	return w.deliver(ctx, delivery)
}

func (w *Webhooks) deliver(ctx context.Context, delivery domain.WebhookDelivery) (domain.WebhookDelivery, error) {
	webhook, err := w.store.WebhookByID(delivery.WebhookID)
	if err != nil {
		return delivery, fmt.Errorf("webhook no longer exists")
	}
	if !webhook.Active {
		delivery.Status = domain.DeliveryDelivered
		return delivery, w.store.MarkDelivery(delivery.ID, delivery)
	}
	// The secret is not persisted, so a webhook restored from a snapshot comes
	// back without one. Signing with an empty key would publish a payload that
	// no signature can prove, so the delivery fails loudly and keeps failing
	// visibly until the endpoint is re-registered.
	if strings.TrimSpace(webhook.Secret) == "" {
		w.logger.Error("webhook has no signing secret; refusing to deliver", "webhook_id", webhook.ID, "delivery_id", delivery.ID)
		return delivery, fmt.Errorf("webhook %s has no signing secret", webhook.ID)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook.URL, bytes.NewReader([]byte(delivery.Payload)))
	if err != nil {
		return delivery, fmt.Errorf("webhook request could not be built: %w", err)
	}
	timestamp := w.now().UTC()
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "CodeCeremony-Webhooks/1.0")
	request.Header.Set("X-CodeCeremony-Event", delivery.Event)
	request.Header.Set("X-CodeCeremony-Delivery", delivery.ID)
	request.Header.Set("X-CodeCeremony-Timestamp", fmt.Sprint(timestamp.Unix()))
	request.Header.Set("X-CodeCeremony-Signature", domain.Sign(webhook.Secret, timestamp, []byte(delivery.Payload)))
	statusCode, body, err := w.transport.Do(request)
	delivery.ResponseCode = statusCode
	delivery.ResponseBody = body
	if err != nil {
		return delivery, err
	}
	if statusCode < 200 || statusCode >= 300 {
		delivery.Status = domain.DeliveryFailed
		delivery.LastError = fmt.Sprintf("endpoint returned %d", statusCode)
		return delivery, fmt.Errorf("endpoint returned %d", statusCode)
	}
	now := w.now().UTC()
	delivery.Status = domain.DeliveryDelivered
	delivery.DeliveredAt = &now
	delivery.LastError = ""
	return delivery, w.store.MarkDelivery(delivery.ID, delivery)
}

func (w *Webhooks) Run(ctx context.Context, interval time.Duration, batch int) {
	if interval <= 0 {
		interval = 15 * time.Second
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
			w.cycle(batch)
		}
	}
}

// cycle runs one delivery pass and recovers, because the entry point starts
// this loop bare: an unrecovered panic would end webhooks for the process.
func (w *Webhooks) cycle(batch int) {
	defer func() {
		if recovered := recover(); recovered != nil {
			w.logger.Error("webhook dispatch cycle panicked", "panic", recovered)
		}
	}()
	if _, err := w.DeliverOnce(batch); err != nil {
		w.logger.Error("webhook dispatch cycle failed", "error", err)
	}
}

// SetTransportForTest replaces the delivery transport. It exists so tests can
// assert on signed requests without reaching the network.
func (w *Webhooks) SetTransportForTest(transport HookTransport) {
	if transport != nil {
		w.transport = transport
	}
}

// SetRetryPolicyForTest overrides the attempt budget and backoff.
func (w *Webhooks) SetRetryPolicyForTest(maxAttempts int, baseBackoff time.Duration) {
	if maxAttempts > 0 {
		w.maxTries = maxAttempts
	}
	if baseBackoff > 0 {
		w.baseBackoff = baseBackoff
	}
}
