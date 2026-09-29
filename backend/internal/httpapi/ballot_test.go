package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
)

// ballotOrder fetches the randomized ballot for a voter and returns the
// project ids in the order that voter saw.
func ballotOrder(t *testing.T, server *Server, voter, path string) []string {
	t.Helper()
	response := request(t, server, http.MethodGet, path, voter, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200, body = %s", path, response.Code, response.Body.String())
	}
	var payload struct {
		Data []struct {
			ProjectID string `json:"project_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("ballot body is not JSON: %s", response.Body.String())
	}
	ids := make([]string, 0, len(payload.Data))
	for _, option := range payload.Data {
		ids = append(ids, option.ProjectID)
	}
	return ids
}

// Every voter seeing the same order is the failure mode this randomization
// exists to prevent: a fixed list lets a campaign be gamed by telling its
// audience to tick a visible pattern, and lets collusion be coordinated around
// "everything in the top five".
func TestBallotOrderDiffersBetweenVoters(t *testing.T) {
	server, tokens, data := newTestServer(t)
	first := tokenFor(t, tokens, data, "organizer")
	second := tokenFor(t, tokens, data, "participant")

	// The ballot needs an open campaign, so create one.
	created := request(t, server, http.MethodPost, "/v1/organizer/events/sample-hack-2026/vote-campaigns", first, map[string]any{
		"name": "Community choice", "max_choices_per_user": 3, "require_eligible": true,
	})
	if created.Code != http.StatusCreated && created.Code != http.StatusOK {
		t.Fatalf("creating a campaign = %d, body = %s", created.Code, created.Body.String())
	}
	opened := request(t, server, http.MethodPut, "/v1/organizer/vote-campaigns/"+
		decodeAssignmentsBody(t, created)["data"].(map[string]any)["id"].(string)+"/open", first, nil)
	if opened.Code != http.StatusOK && opened.Code != http.StatusNoContent {
		t.Fatalf("opening a campaign = %d, body = %s", opened.Code, opened.Body.String())
	}

	path := "/v1/events/sample-hack-2026/vote/ballot"
	organizerOrder := ballotOrder(t, server, first, path)
	participantOrder := ballotOrder(t, server, second, path)
	if len(organizerOrder) < 4 {
		t.Fatalf("ballot has only %d options, the seeded event is too small to detect a fixed order", len(organizerOrder))
	}
	same := true
	for i := range organizerOrder {
		if organizerOrder[i] != participantOrder[i] {
			same = false
			break
		}
	}
	if same {
		t.Errorf("two voters were shown the same order: %v", organizerOrder)
	}
}

// Refreshing must not reshuffle the list. A voter who re-rolls until they get
// an order they like reintroduces the very bias the shuffle removes, and a
// list that moves under the cursor is unusable.
func TestBallotOrderIsStableForOneVoter(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	created := request(t, server, http.MethodPost, "/v1/organizer/events/sample-hack-2026/vote-campaigns", organizer, map[string]any{
		"name": "Community choice", "max_choices_per_user": 3,
	})
	if created.Code != http.StatusCreated && created.Code != http.StatusOK {
		t.Fatalf("creating a campaign = %d, body = %s", created.Code, created.Body.String())
	}
	request(t, server, http.MethodPut, "/v1/organizer/vote-campaigns/"+
		decodeAssignmentsBody(t, created)["data"].(map[string]any)["id"].(string)+"/open", organizer, nil)

	path := "/v1/events/sample-hack-2026/vote/ballot"
	first := ballotOrder(t, server, organizer, path)
	for attempt := 0; attempt < 4; attempt++ {
		again := ballotOrder(t, server, organizer, path)
		if len(again) != len(first) {
			t.Fatalf("ballot length changed between requests: %d then %d", len(first), len(again))
		}
		for i := range first {
			if first[i] != again[i] {
				t.Fatalf("ballot reshuffled on refresh at %d: %v then %v", i, first, again)
			}
		}
	}
}

// The ballot must not be readable anonymously: a campaign that only admits
// eligible submissions should not leak the existence of the ineligible ones.
func TestBallotRequiresAuthentication(t *testing.T) {
	server, _, _ := newTestServer(t)
	response := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/vote/ballot", "", nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous ballot = %d, want 401", response.Code)
	}
}

func TestVoterKeyIsStableAndDistinct(t *testing.T) {
	salt := "cmp_demo"
	first := voterKey(salt, "prj_01")
	if first != voterKey(salt, "prj_01") {
		t.Error("voterKey is not stable for the same input")
	}
	if first == voterKey(salt, "prj_02") {
		t.Error("voterKey collided for two different projects")
	}
	if first == voterKey("cmp_other", "prj_01") {
		t.Error("voterKey did not depend on the campaign salt")
	}
}
