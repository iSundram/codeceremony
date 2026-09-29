package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// These tests cover the isolation rules the DOGFOOD acceptance checker probes,
// plus the neighbouring leaks that a curl would find on a real portal. They are
// backend tests on purpose: the point of the requirement is that a check living
// in a template is not a check, so asserting on the handler's own response is
// the only assertion worth making.

func TestJudgeCannotReadPeerScoresThroughAnyRoute(t *testing.T) {
	server, tokens, data := newTestServer(t)
	judgeA := tokenFor(t, tokens, data, "judge_a")
	judgeB := tokenFor(t, tokens, data, "judge_b")

	// Every route that could plausibly surface another judge's scores.
	attempts := []struct {
		name   string
		method string
		path   string
		token  string
	}{
		{"explicit judge param", http.MethodGet, "/v1/judge/scores?judge=judge_a&event_id=evt_01", judgeB},
		{"explicit judge param on assignments", http.MethodGet, "/v1/organizer/assignments?judge_id=judge_a&event_id=evt_01", judgeB},
		{"peer reviews", http.MethodGet, "/v1/organizer/reviews?event_id=evt_01", judgeB},
		{"results", http.MethodGet, "/v1/organizer/results?event_id=evt_01", judgeB},
		{"csv export", http.MethodGet, "/v1/organizer/export.csv?event_id=evt_01", judgeB},
		{"json export", http.MethodGet, "/v1/organizer/export?event_id=evt_01", judgeB},
	}
	for _, attempt := range attempts {
		t.Run(attempt.name, func(t *testing.T) {
			response := request(t, server, attempt.method, attempt.path, attempt.token, nil)
			if response.Code != http.StatusForbidden && response.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s as judge_b = %d, want 401 or 403, body = %s",
					attempt.method, attempt.path, response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "Runs clean") {
				t.Fatalf("peer review text leaked through %s: %s", attempt.path, response.Body.String())
			}
		})
	}

	// The flip side: a judge must still be able to read their own work, or the
	// isolation is just a broken console.
	own := request(t, server, http.MethodGet, "/v1/judge/scores?event_id=evt_01", judgeA, nil)
	if own.Code != http.StatusOK {
		t.Fatalf("judge reading own scores = %d, want 200, body = %s", own.Code, own.Body.String())
	}
	ownExplicit := request(t, server, http.MethodGet, "/v1/judge/scores?judge=judge_a&event_id=evt_01", judgeA, nil)
	if ownExplicit.Code != http.StatusOK {
		t.Fatalf("judge reading own scores by name = %d, want 200, body = %s", ownExplicit.Code, ownExplicit.Body.String())
	}
}

func TestParticipantAndVisitorAreNotJudges(t *testing.T) {
	server, tokens, data := newTestServer(t)
	participant := tokenFor(t, tokens, data, "participant")
	for _, path := range []string{
		"/v1/judge/scores?event_id=evt_01",
		"/v1/judge/assignments?event_id=evt_01",
		"/v1/organizer/reviews?event_id=evt_01",
		"/v1/organizer/results?event_id=evt_01",
		"/v1/organizer/export.csv?event_id=evt_01",
		"/v1/admin/users",
		"/v1/admin/audit",
	} {
		response := request(t, server, http.MethodGet, path, participant, nil)
		if response.Code != http.StatusForbidden && response.Code != http.StatusUnauthorized {
			t.Errorf("participant GET %s = %d, want 401 or 403", path, response.Code)
		}
	}
	anonymous := request(t, server, http.MethodGet, "/v1/judge/scores?event_id=evt_01", "", nil)
	if anonymous.Code != http.StatusUnauthorized {
		t.Errorf("anonymous GET /v1/judge/scores = %d, want 401", anonymous.Code)
	}
}

// A review must be filed against the event its project actually belongs to. If
// the event id can be supplied by the caller, a review disappears from the
// leaderboard that should have counted it.
func TestReviewCannotBeFiledAgainstTheWrongEvent(t *testing.T) {
	server, tokens, data := newTestServer(t)
	judge := tokenFor(t, tokens, data, "judge_a")

	mismatched := request(t, server, http.MethodPut, "/v1/judge/projects/prj_01/review", judge, map[string]any{
		"event_id": "some_other_event",
		"criteria": map[string]int{"functionality": 4, "quality": 4, "innovation": 4},
	})
	if mismatched.Code != http.StatusUnprocessableEntity && mismatched.Code != http.StatusForbidden {
		t.Fatalf("review with a foreign event_id = %d, want 422 or 403, body = %s",
			mismatched.Code, mismatched.Body.String())
	}
}

// Anonymous callers get the public shape of the panel, not the addresses behind
// it and not the organizer's reviewer pool.
func TestPublicPanelDoesNotLeakAddressesOrInternals(t *testing.T) {
	server, _, data := newTestServer(t)
	_ = data
	organizerEmail := "judge-a@example.org"

	judges := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/judges", "", nil)
	if judges.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", judges.Code)
	}
	if strings.Contains(judges.Body.String(), organizerEmail) {
		t.Errorf("anonymous panel listing leaked a judge email: %s", judges.Body.String())
	}
	if strings.Contains(judges.Body.String(), `"global"`) {
		t.Errorf("anonymous panel listing leaked the reviewer pool: %s", judges.Body.String())
	}

	staff := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/staff", "", nil)
	if staff.Code != http.StatusOK {
		t.Fatalf("staff listing status = %d, want 200", staff.Code)
	}
	if strings.Contains(staff.Body.String(), "organizer@example.org") {
		t.Errorf("anonymous staff listing leaked an organizer email: %s", staff.Body.String())
	}
}

// Logging out has to invalidate the session, not just clear the cookie. The
// login response hands the token back as a bearer credential, so a client that
// stored it would otherwise stay authenticated.
func TestLogoutRevokesTheSessionServerSide(t *testing.T) {
	// This has to run against the store-backed session manager, not the
	// stateless HMAC issuer the other tests use: a stateless token cannot be
	// revoked, so asserting logout behaviour on one would assert nothing.
	server, tokens, data := newTestServerWithSessions(t)
	organizer := tokenFor(t, tokens, data, "organizer")

	if before := request(t, server, http.MethodGet, "/v1/me", organizer, nil); before.Code != http.StatusOK {
		t.Fatalf("before logout = %d, want 200", before.Code)
	}
	if out := request(t, server, http.MethodPost, "/v1/auth/logout", organizer, nil); out.Code != http.StatusOK {
		t.Fatalf("logout = %d, want 200, body = %s", out.Code, out.Body.String())
	}
	after := request(t, server, http.MethodGet, "/v1/me", organizer, nil)
	if after.Code != http.StatusUnauthorized {
		t.Fatalf("after logout = %d, want 401, the token should be dead, body = %s", after.Code, after.Body.String())
	}
}

func TestLoginIsRateLimited(t *testing.T) {
	server, _, data := newTestServer(t)
	_ = data
	var last *httptest.ResponseRecorder
	refused := 0
	for attempt := 0; attempt < 40; attempt++ {
		last = request(t, server, http.MethodPost, "/v1/auth/login", "", map[string]any{
			"email": "organizer@example.org", "password": "wrong-password",
		})
		if last.Code == http.StatusTooManyRequests {
			refused++
			break
		}
	}
	if refused == 0 {
		t.Fatalf("40 failed logins were never rate limited, last = %d: %s", last.Code, last.Body.String())
	}
	if last.Header().Get("Retry-After") == "" {
		t.Error("a 429 response must carry a Retry-After header")
	}
	var body map[string]any
	if err := json.Unmarshal(last.Body.Bytes(), &body); err != nil {
		t.Fatalf("429 body is not JSON: %s", last.Body.String())
	}
	if _, ok := body["error"]; !ok {
		t.Errorf("429 body should use the standard error envelope: %s", last.Body.String())
	}
}

// A judge must not be able to learn a peer's address, or which projects a peer
// holds, or whether that peer has finished reviewing.
func TestJudgeCannotEnumeratePeersThroughAssignments(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	judgeA := tokenFor(t, tokens, data, "judge_a")
	judgeB := tokenFor(t, tokens, data, "judge_b")

	// The organizer seeds a second batch so there is something to leak.
	created := request(t, server, http.MethodPost, "/v1/organizer/assignments", organizer, map[string]any{
		"event_slug": "sample-hack-2026", "judge_ids": []string{"judge_b"},
		"project_ids": []string{"prj_01"}, "strategy": "manual",
	})
	if created.Code != http.StatusOK && created.Code != http.StatusCreated {
		t.Fatalf("seeding an assignment = %d, body = %s", created.Code, created.Body.String())
	}

	peers := request(t, server, http.MethodGet, "/v1/organizer/assignments?judge_id=judge_b&event_id=evt_01", judgeA, nil)
	if peers.Code != http.StatusForbidden {
		t.Fatalf("judge reading a peer's assignments = %d, want 403, body = %s", peers.Code, peers.Body.String())
	}

	own := request(t, server, http.MethodGet, "/v1/organizer/assignments?event_id=evt_01", judgeB, nil)
	if own.Code != http.StatusOK {
		t.Fatalf("judge reading own assignments = %d, want 200, body = %s", own.Code, own.Body.String())
	}
	if strings.Contains(own.Body.String(), "judge-a@example.org") {
		t.Errorf("judge saw a peer address in their own assignment view: %s", own.Body.String())
	}
	for _, view := range decodeAssignmentsBody(t, own)["data"].([]any) {
		entry := view.(map[string]any)
		if entry["judge_id"] != "judge_b" {
			t.Errorf("judge saw an assignment belonging to %v", entry["judge_id"])
		}
	}
}
