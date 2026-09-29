package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// A password hash is a credential. It must never appear in a response body.
//
// This exists because making the hash survive a restart — which it has to, or
// every account is locked out on the first deploy — meant removing the `json:"-"`
// tag that had been keeping it out of responses. domain.User is the storage type
// and the response type, so the hash went out in the body of the session,
// sign-in and profile endpoints, and into the browser's network tab on every
// sign-in. Three green suites did not catch it; the e2e suite did not look.
func TestNoUserIsReturnedWithItsPasswordHash(t *testing.T) {
	server, tokens, data := newTestServerWithSessions(t)
	organizer := tokenFor(t, tokens, data, "organizer")

	for _, probe := range []struct {
		name   string
		method string
		path   string
		token  string
	}{
		{"the session", http.MethodGet, "/v1/me", organizer},
		{"the profile", http.MethodGet, "/v1/account/profile", organizer},
	} {
		got := request(t, server, probe.method, probe.path, probe.token, nil)
		if got.Code != http.StatusOK {
			t.Errorf("%s: status = %d, body = %s", probe.name, got.Code, got.Body.String())
			continue
		}
		body := got.Body.String()
		if strings.Contains(body, "password_hash") {
			t.Errorf("%s: the response carries a password hash: %s", probe.name, body)
		}
		if strings.Contains(body, "$2a$") || strings.Contains(body, "$2b$") {
			t.Errorf("%s: the response carries what looks like a bcrypt digest: %s", probe.name, body)
		}
	}
}

// Sign-in is the response most likely to be inspected, and the one a user
// shares when they paste a network trace into an issue.
func TestSignInDoesNotReturnThePasswordHash(t *testing.T) {
	server, _, _ := newTestServer(t)

	got := request(t, server, http.MethodPost, "/v1/auth/login", "", map[string]any{
		"email": "organizer@example.org", "password": testPassword,
	})
	if got.Code != http.StatusOK {
		t.Fatalf("sign-in status = %d, body = %s", got.Code, got.Body.String())
	}
	body := got.Body.String()
	if strings.Contains(body, "password_hash") || strings.Contains(body, "$2a$") {
		t.Errorf("the sign-in response carries a password hash: %s", body)
	}
	// And the hash must still be there server-side, or the redaction has
	// deleted the credential rather than hiding it.
	if !strings.Contains(body, `"email"`) || !strings.Contains(body, `"role"`) {
		t.Errorf("the sign-in response lost the fields the app needs: %s", body)
	}
}
