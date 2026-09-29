package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

var ErrSessionNotFound = errors.New("session not found")

type TokenIssuer interface {
	Issue(user domain.User) (string, error)
	Parse(token string) (Claims, error)
	// Revoke invalidates a session server-side. Stateless issuers accept a
	// session id they do not track and return nil, so callers do not need a
	// type assertion to ask for a logout to be honoured.
	Revoke(sessionID, reason string) error
}

type SessionRepository interface {
	CreateSession(session domain.Session) error
	SessionByID(id string) (domain.Session, error)
	SessionByTokenHash(tokenHash string) (domain.Session, error)
	ListSessions(userID string) []domain.Session
	TouchSession(id string, seenAt time.Time) error
	RevokeSession(id, reason string) error
	RevokeUserSessions(userID, exceptID, reason string) int
}

type SessionManager struct {
	ttl      time.Duration
	now      func() time.Time
	sessions SessionRepository
}

func NewSessionManager(ttl time.Duration, sessions SessionRepository) *SessionManager {
	return &SessionManager{ttl: ttl, now: time.Now, sessions: sessions}
}

// SeededSessionTTL is the lifetime of a seeded identity session. Seeded
// identities exist so an acceptance checker and a stranger can attach a fixed
// header on a cold container without logging in, so they must not expire
// between booting the portal and running the checks. They are only ever created
// when seeding is enabled, which the configuration layer refuses to do in
// production.
const SeededSessionTTL = 100 * 365 * 24 * time.Hour

// SessionMethodSeeded marks a session created from a fixed, well-known token.
const SessionMethodSeeded = "seeded"

func (m *SessionManager) Issue(user domain.User) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return m.issue(user, base64.RawURLEncoding.EncodeToString(raw), "local", m.ttl)
}

// IssueWithToken issues a session for a caller-supplied token. It exists so that
// seeded identities can be given stable, documented tokens. A token that is
// already in use is rejected, so this can never silently shadow a real login.
func (m *SessionManager) IssueWithToken(user domain.User, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrInvalidToken
	}
	if _, err := m.Parse(token); err == nil {
		return errors.New("seeded token is already in use")
	}
	if _, err := m.issue(user, token, SessionMethodSeeded, SeededSessionTTL); err != nil {
		return err
	}
	return nil
}

func (m *SessionManager) issue(user domain.User, token, method string, ttl time.Duration) (string, error) {
	if user.ID == "" || !user.Role.Valid() {
		return "", ErrInvalidToken
	}
	if m.sessions == nil {
		return "", errors.New("session repository is required")
	}
	now := m.now().UTC()
	session := domain.Session{
		ID:         domain.NewID("ses"),
		UserID:     user.ID,
		TokenHash:  hashToken(token),
		Method:     method,
		CreatedAt:  now,
		LastSeenAt: now,
		ExpiresAt:  now.Add(ttl),
	}
	if err := m.sessions.CreateSession(session); err != nil {
		return "", err
	}
	return token, nil
}

func (m *SessionManager) Parse(token string) (Claims, error) {
	if m.sessions == nil {
		return Claims{}, ErrInvalidToken
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return Claims{}, ErrInvalidToken
	}
	session, err := m.sessions.SessionByTokenHash(hashToken(token))
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	now := m.now().UTC()
	if session.RevokedAt != nil || !now.Before(session.ExpiresAt) {
		return Claims{}, ErrExpiredToken
	}
	_ = m.sessions.TouchSession(session.ID, now)
	return Claims{
		Subject:   session.UserID,
		Role:      domain.RoleVisitor,
		IssuedAt:  session.CreatedAt.Unix(),
		ExpiresAt: session.ExpiresAt.Unix(),
		Version:   1,
		SessionID: session.ID,
	}, nil
}

func (m *SessionManager) Sessions(userID string) []domain.Session {
	if m.sessions == nil {
		return nil
	}
	return m.sessions.ListSessions(userID)
}

func (m *SessionManager) Revoke(sessionID, reason string) error {
	if m.sessions == nil {
		return ErrSessionNotFound
	}
	return m.sessions.RevokeSession(sessionID, reason)
}

func (m *SessionManager) RevokeOthers(userID, exceptID, reason string) int {
	if m.sessions == nil {
		return 0
	}
	return m.sessions.RevokeUserSessions(userID, exceptID, reason)
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
