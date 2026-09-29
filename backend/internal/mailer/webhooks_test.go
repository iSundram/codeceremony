package mailer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/seed"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

// recordingHookTransport captures what a delivery would have sent, so the
// guards can be asserted without a network.
type recordingHookTransport struct {
	mu       sync.Mutex
	requests []*http.Request
	status   int
	err      error
}

func (r *recordingHookTransport) Do(request *http.Request) (int, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return 0, "", r.err
	}
	r.requests = append(r.requests, request)
	return r.status, "ok", nil
}

func (r *recordingHookTransport) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

// newTestWebhooks builds a store holding exactly the supplied webhooks. It goes
// through Restore rather than CreateWebhook so a webhook can be in the state a
// restart leaves it in, which CreateWebhook's validation would otherwise hide.
func newTestWebhooks(t *testing.T, transport HookTransport, webhooks ...domain.Webhook) (*store.Store, *Webhooks) {
	t.Helper()
	hash, err := auth.HashPassword("mailer-webhook-password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	data := store.New(seed.Default(hash))
	snapshot := data.Snapshot()
	snapshot.Webhooks = webhooks
	if err := data.Restore(snapshot); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	hooks := NewWebhooks(data, transport, slog.New(slog.NewTextHandler(discard{}, nil)), 3, time.Millisecond)
	return data, hooks
}

func testWebhook(url, secret string) domain.Webhook {
	return domain.Webhook{
		ID:        "whk_01",
		EventID:   "evt_01",
		URL:       url,
		Secret:    secret,
		Events:    []string{"results.published"},
		Active:    true,
		CreatedAt: time.Date(2026, time.March, 2, 8, 0, 0, 0, time.UTC),
	}
}

func TestHTTPHookTransportRefusesToFollowARedirect(t *testing.T) {
	// The redirect target stands in for a host the organizer does not control.
	// The signed body and signature header are re-attached to every hop, so
	// following one would hand the event data to that host with a signature that
	// verifies against the organizer's own secret.
	var mu sync.Mutex
	forwarded := make([]*http.Request, 0)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		forwarded = append(forwarded, r.Clone(context.Background()))
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/stolen", http.StatusFound)
	}))
	defer origin.Close()

	request, err := http.NewRequest(http.MethodPost, origin.URL+"/hook", strings.NewReader(`{"event":"results.published"}`))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	request.Header.Set("X-CodeCeremony-Signature", "t=1,v1=deadbeef")

	statusCode, _, err := NewHTTPHookTransport().Do(request)
	if !errors.Is(err, errRedirectRefused) {
		t.Fatalf("error = %v, want the redirect to be refused", err)
	}
	if statusCode == http.StatusNoContent {
		t.Fatalf("a refused redirect must not read as a successful delivery")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(forwarded) != 0 {
		t.Fatalf("the redirect target received %d requests, want none", len(forwarded))
	}
}

func TestWebhookDeliveryRefusesAnEmptySigningSecret(t *testing.T) {
	transport := &recordingHookTransport{status: http.StatusNoContent}
	// domain.Webhook.Secret is not serialised, so a webhook that comes back from
	// a restart has no secret. Signing with an empty key would publish a payload
	// that no signature can prove, so delivery has to fail loudly instead.
	data, hooks := newTestWebhooks(t, transport, testWebhook("https://hooks.example.org/endpoint", ""))

	envelopes := hooks.Emit("evt_01", "results.published", "sample-hack-2026", map[string]any{"winner": "prj_01"})
	if len(envelopes) != 1 {
		t.Fatalf("envelopes = %d, want 1", len(envelopes))
	}
	delivered, err := hooks.DeliverOnce(10)
	if err != nil {
		t.Fatalf("DeliverOnce() error = %v", err)
	}
	if delivered != 0 {
		t.Fatalf("delivered = %d, want the unsigned delivery refused", delivered)
	}
	if transport.count() != 0 {
		t.Fatalf("the endpoint was contacted %d times without a signing secret", transport.count())
	}

	stored := data.Deliveries("whk_01", 10)
	if len(stored) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(stored))
	}
	if stored[0].Status == domain.DeliveryDelivered {
		t.Fatalf("the delivery was reported as delivered despite having no signature")
	}
	if stored[0].LastError == "" {
		t.Fatalf("expected the missing secret to be recorded on the delivery")
	}
}

func TestWebhookDeliverySignsWithTheRegisteredSecret(t *testing.T) {
	transport := &recordingHookTransport{status: http.StatusNoContent}
	secret := "a-signing-secret-of-sufficient-length"
	data, hooks := newTestWebhooks(t, transport, testWebhook("https://hooks.example.org/endpoint", secret))

	envelopes := hooks.Emit("evt_01", "results.published", "sample-hack-2026", map[string]any{"winner": "prj_01"})
	if len(envelopes) != 1 {
		t.Fatalf("envelopes = %d, want 1", len(envelopes))
	}
	delivered, err := hooks.DeliverOnce(10)
	if err != nil {
		t.Fatalf("DeliverOnce() error = %v", err)
	}
	if delivered != 1 {
		t.Fatalf("delivered = %d, want the signed delivery to succeed", delivered)
	}

	transport.mu.Lock()
	sent := transport.requests[0]
	transport.mu.Unlock()
	body, err := io.ReadAll(sent.Body)
	if err != nil {
		t.Fatalf("reading the sent body: %v", err)
	}
	if err := domain.VerifySignature(secret, sent.Header.Get("X-CodeCeremony-Signature"), body, time.Minute); err != nil {
		t.Fatalf("the delivered body did not verify against the registered secret: %v", err)
	}

	stored := data.Deliveries("whk_01", 10)
	if stored[0].Status != domain.DeliveryDelivered {
		t.Fatalf("status = %q, want delivered", stored[0].Status)
	}
}
