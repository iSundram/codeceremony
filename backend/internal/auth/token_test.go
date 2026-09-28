package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if !VerifyPassword(hash, "correct horse battery staple") {
		t.Fatal("VerifyPassword() = false for correct password")
	}
	if VerifyPassword(hash, "wrong password") {
		t.Fatal("VerifyPassword() = true for wrong password")
	}
}

func TestTokenRoundTrip(t *testing.T) {
	manager := NewManager("test-secret", time.Hour)
	manager.now = func() time.Time { return time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC) }
	user := domain.User{ID: "judge_a", Role: domain.RoleJudge}

	token, err := manager.Issue(user)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	claims, err := manager.Parse(token)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if claims.Subject != user.ID || claims.Role != user.Role {
		t.Fatalf("claims = %+v, want subject %q role %q", claims, user.ID, user.Role)
	}
}

func TestTokenRejectsTamperingAndExpiry(t *testing.T) {
	manager := NewManager("test-secret", time.Hour)
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	user := domain.User{ID: "judge_a", Role: domain.RoleJudge}
	token, err := manager.Issue(user)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if _, err := manager.Parse(token + "x"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("tampered Parse() error = %v, want ErrInvalidToken", err)
	}
	manager.now = func() time.Time { return now.Add(2 * time.Hour) }
	if _, err := manager.Parse(token); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("expired Parse() error = %v, want ErrExpiredToken", err)
	}
}
