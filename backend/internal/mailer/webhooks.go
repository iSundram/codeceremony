package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

type HookTransport interface {
	Do(request *http.Request) (statusCode int, body string, err error)
}

type HTTPHookTransport struct {
	client *http.Client
}

func NewHTTPHookTransport() *HTTPHookTransport {
	return &HTTPHookTransport{client: &http.Client{Timeout: 10 * time.Second}}
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

type Webhooks struct {
	store       *store.Store
	transport   HookTransport
	logger      *slog.Logger
	now         func() time.Time
	maxTries    int
	baseBackoff time.Duration
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
	return &Webhooks{store: data, transport: transport, logger: logger, now: time.Now, maxTries: maxTries, baseBackoff: baseBackoff}
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

func (w *Webhooks) DeliverOnce(limit int) (int, error) {
	now := w.now().UTC()
	claimed := w.store.ClaimDeliveries(now, limit)
	delivered := 0
	for _, delivery := range claimed {
		updated, err := w.deliver(delivery)
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
				return delivered, markErr
			}
			continue
		}
		delivered++
	}
	return delivered, nil
}

func (w *Webhooks) deliver(delivery domain.WebhookDelivery) (domain.WebhookDelivery, error) {
	webhook, err := w.store.WebhookByID(delivery.WebhookID)
	if err != nil {
		return delivery, fmt.Errorf("webhook no longer exists")
	}
	if !webhook.Active {
		delivery.Status = domain.DeliveryDelivered
		return delivery, w.store.MarkDelivery(delivery.ID, delivery)
	}
	request, err := http.NewRequest(http.MethodPost, webhook.URL, bytes.NewReader([]byte(delivery.Payload)))
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
			if _, err := w.DeliverOnce(batch); err != nil {
				w.logger.Error("webhook dispatch cycle failed", "error", err)
			}
		}
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
