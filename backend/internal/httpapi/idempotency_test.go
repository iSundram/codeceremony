package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/iSundram/codeceremony/backend/internal/store"
)

// keyedRequest issues a request carrying an idempotency key and returns the
// response plus whether the server replayed a remembered one.
type keyedResponse struct {
	*httptest.ResponseRecorder
	replayed bool
}

func postKeyed(t *testing.T, server *Server, token, path, key string, body any) keyedResponse {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal() error = %v", err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Cookie", "session="+token)
	}
	if key != "" {
		req.Header.Set(IdempotencyHeader, key)
	}
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	return keyedResponse{ResponseRecorder: rec, replayed: rec.Header().Get("Idempotency-Replayed") == "true"}
}

func decode(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode() error = %v, body = %s", err, body)
	}
	return out
}

// openCampaign creates and opens a vote campaign and returns its id. The
// idempotency tests need a real target to write to: a request refused by the
// ballot rules before reaching the store would not test anything.
func openCampaign(t *testing.T, server *Server, data *store.Store) string {
	t.Helper()
	organizer := tokenFor(t, server.tokens, data, "organizer")
	created := postKeyed(t, server, organizer, "/v1/organizer/events/sample-hack-2026/vote-campaigns", "",
		map[string]any{"name": "Community choice", "max_choices_per_user": 3, "require_eligible": true})
	if created.Code >= 400 {
		t.Fatalf("create campaign status = %d, body = %s", created.Code, created.Body.String())
	}
	id, _ := decode(t, created.Body.Bytes())["data"].(map[string]any)["id"].(string)
	opened := request(t, server, http.MethodPut, "/v1/organizer/vote-campaigns/"+id+"/open",
		organizer, nil)
	if opened.Code >= 400 {
		t.Fatalf("open campaign status = %d, body = %s", opened.Code, opened.Body.String())
	}
	return id
}

// ballotOptions returns the project ids a voter may choose from, in the order
// the route offers them.
func ballotOptions(t *testing.T, server *Server, token, campaign string) []string {
	t.Helper()
	listed := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/vote/ballot?campaign_id="+campaign, token, nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("list ballot status = %d, body = %s", listed.Code, listed.Body.String())
	}
	options, _ := decode(t, listed.Body.Bytes())["data"].([]any)
	ids := make([]string, 0, len(options))
	for _, option := range options {
		ids = append(ids, option.(map[string]any)["project_id"].(string))
	}
	if len(ids) < 4 {
		t.Fatalf("the campaign offers %d options, need at least 4 to build two distinct ballots", len(ids))
	}
	return ids
}

// ballotBody builds a ballot for a campaign from the given option positions.
func ballotBody(t *testing.T, server *Server, token, campaign string, options []string, positions ...int) map[string]any {
	t.Helper()
	ids := make([]string, 0, len(positions))
	for _, at := range positions {
		ids = append(ids, options[at])
	}
	return map[string]any{"campaign_id": campaign, "project_ids": ids}
}

// A retried ballot must not count twice. This is the failure the mechanism
// exists for: a dropped response, a client that cannot tell, a retry.
func TestARepeatedKeyedBallotIsCastOnce(t *testing.T) {
	server, tokens, data := newTestServer(t)
	participant := tokenFor(t, tokens, data, "participant")
	campaign := openCampaign(t, server, data)
	options := ballotOptions(t, server, participant, campaign)
	ballot := ballotBody(t, server, participant, campaign, options, 0, 1)

	first := postKeyed(t, server, participant, "/v1/events/sample-hack-2026/vote", "ballot-key-1", ballot)
	if first.Code != http.StatusOK && first.Code != http.StatusCreated {
		t.Fatalf("first ballot status = %d, body = %s", first.Code, first.Body.String())
	}
	if first.replayed {
		t.Error("a first attempt was reported as a replay")
	}

	second := postKeyed(t, server, participant, "/v1/events/sample-hack-2026/vote", "ballot-key-1", ballot)
	if !second.replayed {
		t.Error("a repeated request with the same key was not reported as a replay")
	}
	if second.Body.String() != first.Body.String() {
		t.Errorf("a replay answered differently:\nfirst:  %s\nsecond: %s", first.Body.String(), second.Body.String())
	}

	// The caller's own recorded ballot is the thing a double-cast would corrupt,
	// so that is what is checked rather than an aggregate count.
	mine := request(t, server, http.MethodGet, "/v1/vote/mine?campaign_id="+campaign, participant, nil)
	if mine.Code != http.StatusOK {
		t.Fatalf("/v1/vote/mine status = %d, body = %s", mine.Code, mine.Body.String())
	}
	recorded, _ := decode(t, mine.Body.Bytes())["data"].([]any)
	if len(recorded) != 2 {
		t.Errorf("the voter has %d recorded choices, want 2; a replayed ballot would add more", len(recorded))
	}
	seen := map[string]bool{}
	for _, choice := range recorded {
		id, _ := choice.(map[string]any)["project_id"].(string)
		if seen[id] {
			t.Errorf("project %s is recorded twice, so the ballot was counted twice", id)
		}
		seen[id] = true
	}
}

// The same key against a different body is a client bug and must not be
// smoothed over by returning the first response, which would be a lie about the
// second request.
func TestReusingAKeyForADifferentRequestIsRefused(t *testing.T) {
	server, tokens, data := newTestServer(t)
	participant := tokenFor(t, tokens, data, "participant")
	campaign := openCampaign(t, server, data)
	options := ballotOptions(t, server, participant, campaign)
	one := ballotBody(t, server, participant, campaign, options, 0, 1)
	// The second request has to be genuinely different, or the key is being
	// reused correctly and there is no conflict to report.
	two := ballotBody(t, server, participant, campaign, options, 2, 3)

	first := postKeyed(t, server, participant, "/v1/events/sample-hack-2026/vote", "shared-key", one)
	if first.Code >= 400 {
		t.Fatalf("first ballot status = %d, body = %s", first.Code, first.Body.String())
	}

	conflict := postKeyed(t, server, participant, "/v1/events/sample-hack-2026/vote", "shared-key", two)
	if conflict.Code != http.StatusUnprocessableEntity {
		t.Fatalf("reusing a key for a different body status = %d, want 422, body = %s", conflict.Code, conflict.Body.String())
	}
	if conflict.replayed {
		t.Error("a conflicting request was reported as a replay")
	}
	if got := decode(t, conflict.Body.Bytes()); got["error"] == nil {
		t.Error("the conflict response carried no error body")
	}
}

// A key is the caller's namespace. Two users picking the same key must not see
// each other's responses, which a globally scoped key would hand them.
func TestIdempotencyKeysAreScopedPerCaller(t *testing.T) {
	server, tokens, data := newTestServer(t)
	participant := tokenFor(t, tokens, data, "participant")
	other_voter := tokenFor(t, tokens, data, "participant_other")
	campaign := openCampaign(t, server, data)
	options := ballotOptions(t, server, participant, campaign)
	one := ballotBody(t, server, participant, campaign, options, 0, 1)
	other := ballotBody(t, server, other_voter, campaign, options, 0, 1)

	first := postKeyed(t, server, participant, "/v1/events/sample-hack-2026/vote", "same-key", one)
	if first.Code >= 400 {
		t.Fatalf("first ballot status = %d, body = %s", first.Code, first.Body.String())
	}
	second := postKeyed(t, server, other_voter, "/v1/events/sample-hack-2026/vote", "same-key", other)
	if second.replayed {
		t.Error("a second caller was served the first caller's replayed response")
	}
	if second.Code >= 400 && second.Code != http.StatusConflict {
		t.Errorf("the second caller got %d, want a success or an explicit conflict", second.Code)
	}
}

// A request without a key must behave exactly as it did, or adding the mechanism
// would break every existing client.
func TestAnUnkeyedRequestIsUnaffected(t *testing.T) {
	server, tokens, data := newTestServer(t)
	participant := tokenFor(t, tokens, data, "participant")
	campaign := openCampaign(t, server, data)
	options := ballotOptions(t, server, participant, campaign)
	ballot := ballotBody(t, server, participant, campaign, options, 0, 1)

	first := postKeyed(t, server, participant, "/v1/events/sample-hack-2026/vote", "", ballot)
	if first.replayed {
		t.Error("an unkeyed request was reported as a replay")
	}
	if got := first.Header().Get(IdempotencyHeader); got != "" {
		t.Errorf("an unkeyed request was answered with key %q", got)
	}
	// Repeating it without a key is the caller's problem, and the server must
	// not invent protection they did not ask for.
	second := postKeyed(t, server, participant, "/v1/events/sample-hack-2026/vote", "", ballot)
	if second.replayed {
		t.Error("an unkeyed repeat was reported as a replay")
	}
}

// Reads are already safe to repeat, so tracking them would only fill the table.
func TestReadsAreNotTracked(t *testing.T) {
	server, tokens, data := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/audit/actions", nil)
	req.Header.Set("Cookie", "session="+tokenFor(t, tokens, data, "organizer"))
	req.Header.Set(IdempotencyHeader, "read-key")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if got := rec.Header().Get(IdempotencyHeader); got != "" {
		t.Errorf("a GET was answered with an idempotency key %q", got)
	}
}

// A refused request must not be remembered as a success, or a client that fixed
// the problem and retried the same key would be told it had already succeeded.
func TestARefusedRequestDoesNotOccupyTheKey(t *testing.T) {
	server, tokens, data := newTestServer(t)
	participant := tokenFor(t, tokens, data, "participant")

	refused := postKeyed(t, server, participant, "/v1/events/sample-hack-2026/staff",
		"reused-key", map[string]any{"email": "someone@example.org", "role": "co_organizer"})
	if refused.Code != http.StatusForbidden && refused.Code != http.StatusUnauthorized {
		t.Fatalf("expected the staff add to be refused, got %d: %s", refused.Code, refused.Body.String())
	}
	if refused.replayed {
		t.Error("a refused request was reported as a replay")
	}
}

// Concurrent retries of the same keyed request must produce one write. This is
// the interleaving a sequential test cannot reach, and the one that produces a
// double ballot in production.
func TestConcurrentRetriesWithOneKeyApplyOnce(t *testing.T) {
	server, tokens, data := newTestServer(t)
	participant := tokenFor(t, tokens, data, "participant")
	campaign := openCampaign(t, server, data)
	options := ballotOptions(t, server, participant, campaign)
	ballot := ballotBody(t, server, participant, campaign, options, 0, 1, 2)

	const attempts = 8
	var wg sync.WaitGroup
	results := make([]keyedResponse, attempts)
	start := make(chan struct{})
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = postKeyed(t, server, participant, "/v1/events/sample-hack-2026/vote", "race-key", ballot)
		}(i)
	}
	close(start)
	wg.Wait()

	succeeded, replayed := 0, 0
	for _, r := range results {
		if r.Code < 400 {
			succeeded++
		}
		if r.replayed {
			replayed++
		}
	}
	if succeeded != attempts {
		t.Errorf("%d of %d concurrent retries succeeded, want all %d answered", succeeded, attempts, attempts)
	}
	// Exactly one attempt did the work; every other was answered from the record
	// or waited for the winner.
	if replayed != attempts-1 {
		t.Errorf("%d of %d were replays, want %d", replayed, attempts, attempts-1)
	}
}
