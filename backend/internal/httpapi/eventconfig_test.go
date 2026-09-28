package httpapi

import (
	"net/http"
	"testing"
)

func TestOrganizerConfiguresTracksAndPrizes(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	created := request(t, server, http.MethodPost, "/v1/events", organizer, map[string]any{
		"slug":              "config-hack-2026",
		"name":              "Config Hack 2026",
		"registration_open": true,
		"submissions_open":  true,
		"submissions_close": "2026-04-01T18:00:00Z",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", created.Code, created.Body.String())
	}
	track := request(t, server, http.MethodPost, "/v1/events/config-hack-2026/tracks", organizer, map[string]any{"name": "Climate Data"})
	if track.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", track.Code, track.Body.String())
	}
	trackID := decodeAssignmentsBody(t, track)["data"].(map[string]any)["id"].(string)

	duplicate := request(t, server, http.MethodPost, "/v1/events/config-hack-2026/tracks", organizer, map[string]any{"name": "Climate Data"})
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", duplicate.Code, duplicate.Body.String())
	}

	prize := request(t, server, http.MethodPost, "/v1/events/config-hack-2026/prizes", organizer, map[string]any{
		"name":     "Climate track prize",
		"track_id": trackID,
		"rank":     1,
	})
	if prize.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", prize.Code, prize.Body.String())
	}

	tracks := request(t, server, http.MethodGet, "/v1/events/config-hack-2026/tracks", "", nil)
	if tracks.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", tracks.Code, tracks.Body.String())
	}
	if !containsProjectTitle(tracks.Body.String(), "Climate Data") {
		t.Fatalf("expected the public track list to include the new track: %s", tracks.Body.String())
	}
	prizes := request(t, server, http.MethodGet, "/v1/events/config-hack-2026/prizes", "", nil)
	if !containsProjectTitle(prizes.Body.String(), "Climate track prize") {
		t.Fatalf("expected the public prize list to include the new prize: %s", prizes.Body.String())
	}
}

func TestParticipantCannotConfigureEvent(t *testing.T) {
	server, tokens, data := newTestServer(t)
	participant := tokenFor(t, tokens, data, "participant")
	track := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/tracks", participant, map[string]any{"name": "Not allowed"})
	if track.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", track.Code, track.Body.String())
	}
	prize := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/prizes", participant, map[string]any{"name": "Not allowed"})
	if prize.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", prize.Code, prize.Body.String())
	}
}

func TestPrizeRequiresTrackFromSameEvent(t *testing.T) {
	server, tokens, data := newTestServer(t)
	response := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/prizes", tokenFor(t, tokens, data, "organizer"), map[string]any{
		"name":     "Bad track",
		"track_id": "trk_missing",
	})
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", response.Code, response.Body.String())
	}
}
