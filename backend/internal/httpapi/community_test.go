package httpapi

import (
	"net/http"
	"testing"
)

func TestProfileLinksAndAvailabilityValidation(t *testing.T) {
	server, tokens, data := newTestServer(t)
	participant := tokenFor(t, tokens, data, "participant")
	updated := request(t, server, http.MethodPatch, "/v1/profile", participant, map[string]any{
		"headline":         "Systems engineer",
		"bio":              "I build boring reliable things.",
		"location":         "Berlin, Germany",
		"skills":           []string{"go", "kubernetes"},
		"availability":     "looking_for_team",
		"seeking_role":     "backend",
		"seeking_event_id": "evt_01",
		"links": map[string]string{
			"github":    "https://github.com/participant",
			"linkedin":  "https://www.linkedin.com/in/participant",
			"portfolio": "https://participant.example.org",
			"website":   "https://participant.example.org/blog",
		},
	})
	if updated.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", updated.Code, updated.Body.String())
	}
	profile := decodeAssignmentsBody(t, updated)["data"].(map[string]any)
	if profile["availability"] != "looking_for_team" || profile["seeking_team"] != true {
		t.Fatalf("availability not stored: %v", profile)
	}

	badLink := request(t, server, http.MethodPatch, "/v1/profile", participant, map[string]any{
		"links": map[string]string{"github": "https://evil.example.org/user"},
	})
	if badLink.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", badLink.Code, badLink.Body.String())
	}

	unsupported := request(t, server, http.MethodPatch, "/v1/profile", participant, map[string]any{
		"links": map[string]string{"myspace": "https://myspace.example.org/user"},
	})
	if unsupported.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", unsupported.Code, unsupported.Body.String())
	}
}

func TestTeamedProfileClearsSeekingFlag(t *testing.T) {
	server, tokens, data := newTestServer(t)
	other := tokenFor(t, tokens, data, "participant_other")
	response := request(t, server, http.MethodPatch, "/v1/profile", other, map[string]any{
		"availability": "teamed",
		"seeking_team": true,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", response.Code, response.Body.String())
	}
	profile := decodeAssignmentsBody(t, response)["data"].(map[string]any)
	if profile["seeking_team"] != false || profile["availability"] != "teamed" {
		t.Fatalf("expected a teamed profile to stop seeking, got %v", profile)
	}
}

func TestProfileViewIsVisibleToOtherUsersButNotAnonymous(t *testing.T) {
	server, tokens, data := newTestServer(t)
	if anonymous := request(t, server, http.MethodGet, "/v1/profiles/participant", "", nil); anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", anonymous.Code, anonymous.Body.String())
	}
	view := request(t, server, http.MethodGet, "/v1/profiles/participant", tokenFor(t, tokens, data, "judge_a"), nil)
	if view.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", view.Code, view.Body.String())
	}
	if !containsProjectTitle(view.Body.String(), "https://github.com/participant") {
		t.Fatalf("expected profile links in the view: %s", view.Body.String())
	}
	if !containsProjectTitle(view.Body.String(), "appearances") {
		t.Fatalf("expected hackathon appearances in the view: %s", view.Body.String())
	}
}

func TestDiscoverListsSeekingParticipants(t *testing.T) {
	server, tokens, data := newTestServer(t)
	response := request(t, server, http.MethodGet, "/v1/discover?event_id=evt_01", tokenFor(t, tokens, data, "participant_other"), nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", response.Code, response.Body.String())
	}
	entries := decodeAssignmentsBody(t, response)["data"].([]any)
	if len(entries) != 1 {
		t.Fatalf("expected only the seeking participant, body = %s", response.Body.String())
	}
	if entries[0].(map[string]any)["profile"].(map[string]any)["user_id"] != "participant" {
		t.Fatalf("expected the seeking participant to be discoverable: %s", response.Body.String())
	}
}

func TestTeamInviteLifecycle(t *testing.T) {
	server, tokens, data := newTestServer(t)
	captain := tokenFor(t, tokens, data, "participant")
	teamResponse := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/teams", captain, map[string]any{"name": "Merge Request"})
	if teamResponse.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", teamResponse.Code, teamResponse.Body.String())
	}
	teamID := decodeAssignmentsBody(t, teamResponse)["data"].(map[string]any)["id"].(string)
	created := request(t, server, http.MethodPost, "/v1/teams/"+teamID+"/invites", captain, map[string]any{
		"invitee_email": "participant-other@example.org",
		"message":       "We would love your frontend skills.",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", created.Code, created.Body.String())
	}
	inviteID := decodeAssignmentsBody(t, created)["data"].(map[string]any)["id"].(string)

	listed := request(t, server, http.MethodGet, "/v1/invites", captain, nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", listed.Code, listed.Body.String())
	}
	if !containsProjectTitle(listed.Body.String(), "outgoing") {
		t.Fatalf("expected the invite in the outgoing list: %s", listed.Body.String())
	}

	wrongUser := request(t, server, http.MethodPost, "/v1/invites/"+inviteID, tokenFor(t, tokens, data, "judge_a"), map[string]any{"decision": "accepted"})
	if wrongUser.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", wrongUser.Code, wrongUser.Body.String())
	}

	accepted := request(t, server, http.MethodPost, "/v1/invites/"+inviteID, tokenFor(t, tokens, data, "participant_other"), map[string]any{"decision": "accepted"})
	if accepted.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", accepted.Code, accepted.Body.String())
	}
	if decodeAssignmentsBody(t, accepted)["data"].(map[string]any)["status"] != "accepted" {
		t.Fatalf("expected the invite to be accepted: %s", accepted.Body.String())
	}
	if _, err := data.TeamMembership(teamID, "participant_other"); err != nil {
		t.Fatalf("expected the invitee to join the team, error = %v", err)
	}

	again := request(t, server, http.MethodPost, "/v1/invites/"+inviteID, tokenFor(t, tokens, data, "participant_other"), map[string]any{"decision": "accepted"})
	if again.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", again.Code, again.Body.String())
	}
}

func TestInviteRespectsAvailabilityAndMembership(t *testing.T) {
	server, tokens, data := newTestServer(t)
	captain := tokenFor(t, tokens, data, "participant")
	existing := request(t, server, http.MethodPost, "/v1/teams/tm_01/invites", captain, map[string]any{"invitee_id": "participant_other"})
	if existing.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", existing.Code, existing.Body.String())
	}
	closed := request(t, server, http.MethodPost, "/v1/teams/tm_01/invites", captain, map[string]any{"invitee_id": "judge_b"})
	if closed.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 for a participant who is not open to invites, body = %s", closed.Code, closed.Body.String())
	}
	byEmail := request(t, server, http.MethodPost, "/v1/teams/tm_01/invites", captain, map[string]any{"invitee_email": "judge-b@example.org"})
	if byEmail.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", byEmail.Code, byEmail.Body.String())
	}
	otherToken := tokenFor(t, tokens, data, "judge_b")
	inbox := request(t, server, http.MethodGet, "/v1/invites", otherToken, nil)
	if !containsProjectTitle(inbox.Body.String(), "incoming") {
		t.Fatalf("expected an email-addressed invite in the inbox: %s", inbox.Body.String())
	}
}

func TestInviteRevocation(t *testing.T) {
	server, tokens, data := newTestServer(t)
	captain := tokenFor(t, tokens, data, "participant")
	created := request(t, server, http.MethodPost, "/v1/teams/tm_01/invites", captain, map[string]any{"invitee_email": "someone@example.org"})
	inviteID := decodeAssignmentsBody(t, created)["data"].(map[string]any)["id"].(string)
	revoked := request(t, server, http.MethodDelete, "/v1/teams/tm_01/invites/"+inviteID, captain, nil)
	if revoked.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", revoked.Code, revoked.Body.String())
	}
	notSender := request(t, server, http.MethodDelete, "/v1/teams/tm_01/invites/"+inviteID, tokenFor(t, tokens, data, "participant_other"), nil)
	if notSender.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", notSender.Code, notSender.Body.String())
	}
}

func TestOpenTeamsAppearInDiscovery(t *testing.T) {
	server, tokens, data := newTestServer(t)
	response := request(t, server, http.MethodGet, "/v1/discover/teams?event_id=evt_01", tokenFor(t, tokens, data, "participant_other"), nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", response.Code, response.Body.String())
	}
	if !containsProjectTitle(response.Body.String(), "NorthKiln") {
		t.Fatalf("expected the open team in the opportunity list: %s", response.Body.String())
	}
	if containsProjectTitle(response.Body.String(), "Quiet Harbor") {
		t.Fatalf("invite-only teams must stay out of the public opportunity list: %s", response.Body.String())
	}
}

func TestTeamAvailabilityUpdateRules(t *testing.T) {
	server, tokens, data := newTestServer(t)
	captain := tokenFor(t, tokens, data, "participant")
	opened := request(t, server, http.MethodPatch, "/v1/teams/tm_01", captain, map[string]any{"availability": "open", "open_roles": []string{"data"}})
	if opened.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", opened.Code, opened.Body.String())
	}
	badOpen := request(t, server, http.MethodPatch, "/v1/teams/tm_01", captain, map[string]any{"availability": "open", "open_roles": []string{}})
	if badOpen.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", badOpen.Code, badOpen.Body.String())
	}
	tooSmall := request(t, server, http.MethodPatch, "/v1/teams/tm_01", captain, map[string]any{"max_size": 1})
	if tooSmall.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", tooSmall.Code, tooSmall.Body.String())
	}
}

func TestGlobalTeamRequiresHackathonOptIn(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	captain := tokenFor(t, tokens, data, "participant")
	created := request(t, server, http.MethodPost, "/v1/events", organizer, map[string]any{
		"slug":              "local-teams-hack",
		"name":              "Local Teams Hack",
		"registration_open": true,
		"submissions_open":  true,
		"submissions_close": "2026-06-01T18:00:00Z",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", created.Code, created.Body.String())
	}
	globalTeam := request(t, server, http.MethodPost, "/v1/events/local-teams-hack/teams", captain, map[string]any{"name": "Global Crew", "scope": "global"})
	if globalTeam.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", globalTeam.Code, globalTeam.Body.String())
	}
	if opted := request(t, server, http.MethodPatch, "/v1/events/local-teams-hack", organizer, map[string]any{"allow_global_teams": true}); opted.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", opted.Code, opted.Body.String())
	}
	allowed := request(t, server, http.MethodPost, "/v1/events/local-teams-hack/teams", captain, map[string]any{"name": "Global Crew", "scope": "global"})
	if allowed.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", allowed.Code, allowed.Body.String())
	}
}

func TestLeaderboardVisibilityAndPublish(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	if hidden := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/leaderboard", "", nil); hidden.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", hidden.Code, hidden.Body.String())
	}
	staff := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/leaderboard", organizer, nil)
	if staff.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", staff.Code, staff.Body.String())
	}

	published := request(t, server, http.MethodPost, "/v1/organizer/events/sample-hack-2026/publish-results", organizer, map[string]any{
		"public": true,
		"note":   "Congratulations to every team.",
	})
	if published.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", published.Code, published.Body.String())
	}
	payload := decodeAssignmentsBody(t, published)
	if payload["data"].(map[string]any)["state"] != "results_published" {
		t.Fatalf("expected the hackathon to move to results_published: %s", published.Body.String())
	}
	if payload["notified"].(float64) < 1 {
		t.Fatalf("expected team notifications: %s", published.Body.String())
	}

	public := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/leaderboard", "", nil)
	if public.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", public.Code, public.Body.String())
	}
	entries := decodeAssignmentsBody(t, public)["data"].([]any)
	if len(entries) == 0 {
		t.Fatalf("expected leaderboard entries: %s", public.Body.String())
	}
	if entries[0].(map[string]any)["rank"].(float64) != 1 {
		t.Fatalf("expected the first entry to be rank 1: %s", public.Body.String())
	}

	notifications := request(t, server, http.MethodGet, "/v1/notifications", tokenFor(t, tokens, data, "participant"), nil)
	if !containsProjectTitle(notifications.Body.String(), "Results published") {
		t.Fatalf("expected a results notification for the team: %s", notifications.Body.String())
	}

	if noReason := request(t, server, http.MethodPost, "/v1/organizer/events/sample-hack-2026/unpublish-results", organizer, map[string]any{}); noReason.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", noReason.Code, noReason.Body.String())
	}
	unpublished := request(t, server, http.MethodPost, "/v1/organizer/events/sample-hack-2026/unpublish-results", organizer, map[string]any{"reason": "scoring bug"})
	if unpublished.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", unpublished.Code, unpublished.Body.String())
	}
	if closed := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/leaderboard", "", nil); closed.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", closed.Code, closed.Body.String())
	}
}

func TestAppearancesListHackathons(t *testing.T) {
	server, tokens, data := newTestServer(t)
	response := request(t, server, http.MethodGet, "/v1/users/participant/appearances", tokenFor(t, tokens, data, "participant"), nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", response.Code, response.Body.String())
	}
	if !containsProjectTitle(response.Body.String(), "Sample Hack 2026") {
		t.Fatalf("expected the hackathon in appearances: %s", response.Body.String())
	}
	if !containsProjectTitle(response.Body.String(), "NorthKiln") {
		t.Fatalf("expected the team in appearances: %s", response.Body.String())
	}
	mine := request(t, server, http.MethodGet, "/v1/users/me/appearances", tokenFor(t, tokens, data, "participant"), nil)
	if mine.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", mine.Code, mine.Body.String())
	}
}

func TestParticipantCannotPublishResults(t *testing.T) {
	server, tokens, data := newTestServer(t)
	response := request(t, server, http.MethodPost, "/v1/organizer/events/sample-hack-2026/publish-results", tokenFor(t, tokens, data, "participant"), map[string]any{"public": true})
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", response.Code, response.Body.String())
	}
}
