package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

const SessionCookieName = "session"

type Principal struct {
	UserID    string
	Email     string
	Role      domain.Role
	SessionID string
	// State is the account's lifecycle state. It is carried on the principal so
	// the authorization resolver can refuse a suspended account as its first
	// step, rather than each handler remembering to check.
	State domain.AccountState
	// Assured records whether this session has passed a second factor. Routes
	// that need a fresh authentication check it rather than the session's age.
	Assured bool
}

type contextKey struct{}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(contextKey{}).(Principal)
	return principal, ok
}

func TokenFromRequest(request *http.Request) string {
	if header := strings.TrimSpace(request.Header.Get("Authorization")); header != "" {
		parts := strings.SplitN(header, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
	}
	cookie, err := request.Cookie(SessionCookieName)
	if err == nil {
		return strings.TrimSpace(cookie.Value)
	}
	return ""
}
