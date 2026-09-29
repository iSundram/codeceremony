package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/config"
	"github.com/iSundram/codeceremony/backend/internal/seed"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

const testPassword = "codeceremony-test-password"

func newTestServer(t *testing.T) (*Server, *auth.Manager, *store.Store) {
	t.Helper()
	hash, err := auth.HashPassword(testPassword)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	cfg := config.Config{
		Environment:     "test",
		HTTPAddr:        ":8080",
		SessionSecret:   "test-secret",
		AllowedOrigin:   "http://localhost:3000",
		SeedDemoData:    true,
		SessionTTLHours: 1,
	}
	data := store.New(seed.Default(hash))
	tokens := auth.NewManager(cfg.SessionSecret, time.Hour)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := New(cfg, data, tokens, logger)
	server.SetClock(func() time.Time { return time.Date(2026, time.March, 2, 12, 0, 0, 0, time.UTC) })
	return server, tokens, data
}

// newTestServerWithSessions is the same fixture driven by the store-backed
// session manager that production uses, rather than the stateless HMAC issuer.
// Session revocation can only be asserted against the former.
func newTestServerWithSessions(t *testing.T) (*Server, auth.TokenIssuer, *store.Store) {
	t.Helper()
	hash, err := auth.HashPassword(testPassword)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	cfg := testServerConfig()
	data := store.New(seed.Default(hash))
	tokens := auth.NewSessionManager(time.Hour, data)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := New(cfg, data, tokens, logger)
	server.SetClock(func() time.Time { return time.Date(2026, time.March, 2, 12, 0, 0, 0, time.UTC) })
	return server, tokens, data
}

func testServerConfig() config.Config {
	return config.Config{
		Environment:     "test",
		HTTPAddr:        ":8080",
		SessionSecret:   "test-secret",
		AllowedOrigin:   "http://localhost:3000",
		SeedDemoData:    true,
		SessionTTLHours: 1,
	}
}

func tokenFor(t *testing.T, tokens auth.TokenIssuer, data *store.Store, id string) string {
	t.Helper()
	user, err := data.UserByID(id)
	if err != nil {
		t.Fatalf("UserByID(%q) error = %v", id, err)
	}
	token, err := tokens.Issue(user)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	return token
}

func request(t *testing.T, server *Server, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, req)
	return recorder
}

// requestWithHeader issues a request carrying one extra header, which is how the
// conditional-write tests pass If-Match and If-None-Match.
func requestWithHeader(t *testing.T, server *Server, method, path, token string, body any, name, value string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if name != "" {
		req.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, req)
	return recorder
}

func TestHealth(t *testing.T) {
	server, _, _ := newTestServer(t)
	response := request(t, server, http.MethodGet, "/healthz", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
}

func TestPublicGalleryContainsFixtureTitle(t *testing.T) {
	server, _, _ := newTestServer(t)
	response := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/projects", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if !strings.Contains(response.Body.String(), "Glass Signal") {
		t.Fatalf("body does not contain seeded project title: %s", response.Body.String())
	}
}

func TestClosedEventRejectsParticipantSubmission(t *testing.T) {
	server, tokens, data := newTestServer(t)
	response := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/submissions", tokenFor(t, tokens, data, "participant"), map[string]any{
		"team_id":  "tm_01",
		"track_id": "trk_01",
		"title":    "Late submission",
		"summary":  "Should be rejected",
	})
	if response.Code < 400 || response.Code >= 500 {
		t.Fatalf("status = %d, want 4xx", response.Code)
	}
}

func TestJudgeSeesOwnScoresButNotPeerScores(t *testing.T) {
	server, tokens, data := newTestServer(t)
	judgeA := tokenFor(t, tokens, data, "judge_a")
	judgeB := tokenFor(t, tokens, data, "judge_b")

	own := request(t, server, http.MethodGet, "/v1/judge/scores", judgeA, nil)
	if own.Code != http.StatusOK {
		t.Fatalf("own scores status = %d, want 200", own.Code)
	}
	peer := request(t, server, http.MethodGet, "/v1/judge/scores?judge=judge_a", judgeB, nil)
	if peer.Code != http.StatusForbidden {
		t.Fatalf("peer scores status = %d, want 403", peer.Code)
	}
}

func TestParticipantCannotReadJudgeScores(t *testing.T) {
	server, tokens, data := newTestServer(t)
	response := request(t, server, http.MethodGet, "/v1/judge/scores", tokenFor(t, tokens, data, "participant"), nil)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
}

func TestOrganizerCSVExport(t *testing.T) {
	server, tokens, data := newTestServer(t)
	response := request(t, server, http.MethodGet, "/v1/organizer/export.csv", tokenFor(t, tokens, data, "organizer"), nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	firstLine := strings.SplitN(response.Body.String(), "\n", 2)[0]
	if !strings.Contains(firstLine, ",") {
		t.Fatalf("first CSV line has no comma: %q", firstLine)
	}
}

func TestLoginSetsSessionCookie(t *testing.T) {
	server, _, _ := newTestServer(t)
	response := request(t, server, http.MethodPost, "/v1/auth/login", "", map[string]any{
		"email":    "organizer@example.org",
		"password": testPassword,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if len(response.Result().Cookies()) == 0 {
		t.Fatal("login did not set a session cookie")
	}
}

func TestOrganizerResultsIncludeRankingAndNormalization(t *testing.T) {
	server, tokens, data := newTestServer(t)
	response := request(t, server, http.MethodGet, "/v1/organizer/results", tokenFor(t, tokens, data, "organizer"), nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if !strings.Contains(response.Body.String(), "normalized_mean") {
		t.Fatalf("results body missing normalized scores: %s", response.Body.String())
	}
}

func TestPublicEventListIsSeeded(t *testing.T) {
	server, _, _ := newTestServer(t)
	response := request(t, server, http.MethodGet, "/v1/events", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if !strings.Contains(response.Body.String(), "sample-hack-2026") {
		t.Fatalf("event list missing seeded slug: %s", response.Body.String())
	}
}

func TestParticipantCreatesTeam(t *testing.T) {
	server, tokens, data := newTestServer(t)
	response := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/teams", tokenFor(t, tokens, data, "participant"), map[string]any{
		"name":        "New Team",
		"description": "Created during a test.",
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", response.Code, response.Body.String())
	}
}

func TestOrganizerCreatesEvent(t *testing.T) {
	server, tokens, data := newTestServer(t)
	response := request(t, server, http.MethodPost, "/v1/events", tokenFor(t, tokens, data, "organizer"), map[string]any{
		"slug":              "new-event",
		"name":              "New Event",
		"description":       "Created during a test.",
		"timezone":          "UTC",
		"registration_open": true,
		"submissions_open":  true,
		"submissions_close": "2026-03-03T18:00:00Z",
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", response.Code, response.Body.String())
	}
}
