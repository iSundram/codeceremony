package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/config"
	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/seed"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

func newSessionTestServer(t *testing.T) (*Server, *auth.SessionManager, *store.Store) {
	t.Helper()
	hash, err := auth.HashPassword(testPassword)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	cfg := config.Config{Environment: "test", HTTPAddr: ":8080", SessionSecret: "test-secret", AllowedOrigin: "http://localhost:3000", SeedDemoData: true, SessionTTLHours: 1}
	data := store.New(seed.Default(hash))
	tokens := auth.NewSessionManager(time.Hour, data)
	server := New(cfg, data, tokens, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server.SetClock(func() time.Time { return time.Date(2026, time.March, 2, 12, 0, 0, 0, time.UTC) })
	return server, tokens, data
}

func loginForTest(t *testing.T, server *Server, email string) string {
	t.Helper()
	response := request(t, server, http.MethodPost, "/v1/auth/login", "", map[string]any{"email": email, "password": testPassword})
	if response.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	return payload.Data.AccessToken
}

func TestAuthMethodsKeepLocalAvailable(t *testing.T) {
	server, _, _ := newSessionTestServer(t)
	response := request(t, server, http.MethodGet, "/v1/auth/methods", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("methods status = %d, want 200", response.Code)
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(`"local"`)) {
		t.Fatalf("methods body = %s", response.Body.String())
	}
}

func TestSessionCanBeListedAndRevoked(t *testing.T) {
	server, _, _ := newSessionTestServer(t)
	token := loginForTest(t, server, "participant@example.org")
	response := request(t, server, http.MethodGet, "/v1/account/sessions", token, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("list sessions status = %d, want 200", response.Code)
	}
	var payload struct {
		Data []domain.Session `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode sessions: %v", err)
	}
	if len(payload.Data) != 1 || payload.Data[0].ID == "" {
		t.Fatalf("sessions = %+v, want one session", payload.Data)
	}
	revoke := request(t, server, http.MethodDelete, "/v1/account/sessions/"+payload.Data[0].ID, token, nil)
	if revoke.Code != http.StatusOK {
		t.Fatalf("revoke status = %d, want 200", revoke.Code)
	}
	after := request(t, server, http.MethodGet, "/v1/account/sessions", token, nil)
	if after.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token status = %d, want 401", after.Code)
	}
}

func TestCaptainCanPromoteDemoteAndTransfer(t *testing.T) {
	server, _, _ := newSessionTestServer(t)
	captain := loginForTest(t, server, "participant@example.org")
	promote := request(t, server, http.MethodPost, "/v1/teams/tm_01/members/participant_other/promote", captain, nil)
	if promote.Code != http.StatusOK {
		t.Fatalf("promote status = %d, want 200: %s", promote.Code, promote.Body.String())
	}
	demote := request(t, server, http.MethodPost, "/v1/teams/tm_01/members/participant_other/demote", captain, nil)
	if demote.Code != http.StatusOK {
		t.Fatalf("demote status = %d, want 200: %s", demote.Code, demote.Body.String())
	}
	transfer := request(t, server, http.MethodPost, "/v1/teams/tm_01/transfer", captain, map[string]any{"next_captain_id": "participant_other"})
	if transfer.Code != http.StatusOK {
		t.Fatalf("transfer status = %d, want 200: %s", transfer.Code, transfer.Body.String())
	}
}

func TestNotificationCanBeReadByOwner(t *testing.T) {
	server, _, data := newSessionTestServer(t)
	if err := data.CreateNotification(domain.Notification{UserID: "participant", Kind: domain.NotificationEventActivity, Title: "Event update", Body: "A new announcement is available."}); err != nil {
		t.Fatalf("CreateNotification() error = %v", err)
	}
	token := loginForTest(t, server, "participant@example.org")
	list := request(t, server, http.MethodGet, "/v1/notifications", token, nil)
	if list.Code != http.StatusOK || !bytes.Contains(list.Body.Bytes(), []byte("Event update")) {
		t.Fatalf("notification list = %d %s", list.Code, list.Body.String())
	}
	var payload struct {
		Data []domain.Notification `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &payload); err != nil || len(payload.Data) != 1 {
		t.Fatalf("decode notifications: %v payload=%+v", err, payload)
	}
	read := request(t, server, http.MethodPost, "/v1/notifications/"+payload.Data[0].ID+"/read", token, nil)
	if read.Code != http.StatusOK {
		t.Fatalf("mark read status = %d, want 200", read.Code)
	}
}

func TestProfileUpdateAndPasswordChange(t *testing.T) {
	server, _, _ := newSessionTestServer(t)
	token := loginForTest(t, server, "participant@example.org")
	profile := request(t, server, http.MethodPatch, "/v1/account/profile", token, map[string]any{"display_name": "Pia Updated", "bio": "Building CodeCeremony.", "timezone": "UTC", "locale": "en"})
	if profile.Code != http.StatusOK {
		t.Fatalf("profile status = %d, want 200: %s", profile.Code, profile.Body.String())
	}
	password := request(t, server, http.MethodPost, "/v1/account/password", token, map[string]any{"current_password": testPassword, "new_password": "new-local-password"})
	if password.Code != http.StatusOK {
		t.Fatalf("password status = %d, want 200: %s", password.Code, password.Body.String())
	}
	oldPassword := request(t, server, http.MethodPost, "/v1/auth/login", "", map[string]any{"email": "participant@example.org", "password": testPassword})
	if oldPassword.Code != http.StatusUnauthorized {
		t.Fatalf("old password status = %d, want 401", oldPassword.Code)
	}
	newPassword := request(t, server, http.MethodPost, "/v1/auth/login", "", map[string]any{"email": "participant@example.org", "password": "new-local-password"})
	if newPassword.Code != http.StatusOK {
		t.Fatalf("new password status = %d, want 200", newPassword.Code)
	}
}

func TestAdminCanManageAccountStateButParticipantCannot(t *testing.T) {
	server, _, _ := newSessionTestServer(t)
	participant := loginForTest(t, server, "participant@example.org")
	denied := request(t, server, http.MethodGet, "/v1/admin/users", participant, nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("participant admin list status = %d, want 403", denied.Code)
	}
	admin := loginForTest(t, server, "admin@example.org")
	listed := request(t, server, http.MethodGet, "/v1/admin/users", admin, nil)
	if listed.Code != http.StatusOK || !bytes.Contains(listed.Body.Bytes(), []byte("participant@example.org")) {
		t.Fatalf("admin list = %d %s", listed.Code, listed.Body.String())
	}
	changed := request(t, server, http.MethodPatch, "/v1/admin/users/participant/state", admin, map[string]any{"state": "suspended", "reason": "test"})
	if changed.Code != http.StatusOK {
		t.Fatalf("state change status = %d, want 200: %s", changed.Code, changed.Body.String())
	}
	blocked := request(t, server, http.MethodGet, "/v1/me", participant, nil)
	if blocked.Code != http.StatusUnauthorized {
		t.Fatalf("suspended account status = %d, want 401", blocked.Code)
	}
	restored := request(t, server, http.MethodPatch, "/v1/admin/users/participant/state", admin, map[string]any{"state": "active", "reason": "test restore"})
	if restored.Code != http.StatusOK {
		t.Fatalf("restore status = %d, want 200", restored.Code)
	}
}
