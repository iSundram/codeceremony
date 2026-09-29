package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/seed"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

// fixturePasswordHash is a well-formed but fictional bcrypt digest. Tests that
// round-trip a snapshot through Restore need the fixture to look like a real
// hash, because the restore path rejects a password field that is not one —
// which is how a plaintext password in a data file is caught. It is not the hash
// of any password.
const fixturePasswordHash = "$2a$10$4M0PJg2VnNWu3T3vB2n1ueOY0Q7Zt3YbM2Xh1Kq1L9pG4Q6r0zC"

func TestSessionManagerIssuesAndRevokesSession(t *testing.T) {
	data := store.New(seed.Default(fixturePasswordHash))
	manager := NewSessionManager(time.Hour, data)
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	user, err := data.UserByID("participant")
	if err != nil {
		t.Fatalf("UserByID() error = %v", err)
	}
	token, err := manager.Issue(user)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	claims, err := manager.Parse(token)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if claims.Subject != user.ID || claims.SessionID == "" {
		t.Fatalf("claims = %+v, want subject and session id", claims)
	}
	if len(manager.Sessions(user.ID)) != 1 {
		t.Fatalf("session count = %d, want 1", len(manager.Sessions(user.ID)))
	}
	if err := manager.Revoke(claims.SessionID, "test"); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if _, err := manager.Parse(token); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("revoked Parse() error = %v, want ErrExpiredToken", err)
	}
}

func TestSessionManagerRejectsExpiredSession(t *testing.T) {
	data := store.New(seed.Default(fixturePasswordHash))
	manager := NewSessionManager(time.Hour, data)
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	user, _ := data.UserByID("participant")
	token, err := manager.Issue(user)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	manager.now = func() time.Time { return now.Add(2 * time.Hour) }
	if _, err := manager.Parse(token); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("expired Parse() error = %v, want ErrExpiredToken", err)
	}
}
