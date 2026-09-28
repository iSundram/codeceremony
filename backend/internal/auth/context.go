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
