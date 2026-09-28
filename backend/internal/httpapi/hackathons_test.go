package httpapi

import (
	"net/http"
	"testing"
)

func TestOrganizerConfiguresHackathon(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	created := request(t, server, http.MethodPost, "/v1/events", organizer, map[string]any{
		"slug":              "hosted-hack-2026",
		"name":              "Hosted Hack 2026",
		"registration_open": true,
		"submissions_open":  true,
		"submissions_close": "2026-05-01T18:00:00Z",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", created.Code, created.Body.String())
	}
	updated := request(t, server, http.MethodPatch, "/v1/events/hosted-hack-2026", organizer, map[string]any{
		"summary":             "A hosted event for the community.",
		"state":               "judging",
		"judging_mode":        "manual",
		"max_team_size":       5,
		"min_team_size":       2,
		"allow_global_teams":  true,
		"reviews_per_project": 2,
		"judging_close":       "2026-05-20T18:00:00Z",
	})
	if updated.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", updated.Code, updated.Body.String())
	}
	payload := decodeAssignmentsBody(t, updated)["data"].(map[string]any)
	if payload["state"] != "judging" || payload["judging_mode"] != "manual" {
		t.Fatalf("unexpected hackathon state: %v", payload)
	}
	if payload["max_team_size"].(float64) != 5 || payload["allow_global_teams"] != true {
		t.Fatalf("team policy not stored: %v", payload)
	}

	invalid := request(t, server, http.MethodPatch, "/v1/events/hosted-hack-2026", organizer, map[string]any{"judging_mode": "vibes"})
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", invalid.Code, invalid.Body.String())
	}
}

func TestParticipantCannotUpdateHackathon(t *testing.T) {
	server, tokens, data := newTestServer(t)
	response := request(t, server, http.MethodPatch, "/v1/events/sample-hack-2026", tokenFor(t, tokens, data, "participant"), map[string]any{"summary": "nope"})
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", response.Code, response.Body.String())
	}
}

func TestCustomQuestionLifecycle(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	created := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/questions", organizer, map[string]any{
		"prompt":   "What is your deployment plan?",
		"type":     "long_text",
		"audience": "submission",
		"required": true,
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", created.Code, created.Body.String())
	}
	questionID := decodeAssignmentsBody(t, created)["data"].(map[string]any)["id"].(string)

	duplicate := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/questions", organizer, map[string]any{
		"prompt":   "What is your deployment plan?",
		"type":     "long_text",
		"audience": "submission",
	})
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", duplicate.Code, duplicate.Body.String())
	}

	badSelect := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/questions", organizer, map[string]any{
		"prompt":   "Pick a stage",
		"type":     "select",
		"audience": "submission",
	})
	if badSelect.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", badSelect.Code, badSelect.Body.String())
	}

	updated := request(t, server, http.MethodPut, "/v1/organizer/questions/"+questionID, organizer, map[string]any{"required": false})
	if updated.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", updated.Code, updated.Body.String())
	}

	listed := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/questions?audience=submission", "", nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", listed.Code, listed.Body.String())
	}
	if !containsProjectTitle(listed.Body.String(), "deployment plan") {
		t.Fatalf("expected the public question list to include the question: %s", listed.Body.String())
	}

	deleted := request(t, server, http.MethodDelete, "/v1/organizer/questions/"+questionID, organizer, nil)
	if deleted.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", deleted.Code, deleted.Body.String())
	}
}

func TestSubmissionAnswersMustMatchQuestions(t *testing.T) {
	server, tokens, data := newTestServer(t)
	open := createOpenEvent(t, server, tokens, data)
	organizer := open.organizer
	question := request(t, server, http.MethodPost, "/v1/events/"+open.slug+"/questions", organizer, map[string]any{
		"prompt":   "Where can judges try it?",
		"type":     "url",
		"audience": "submission",
		"required": true,
	})
	if question.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", question.Code, question.Body.String())
	}
	key := decodeAssignmentsBody(t, question)["data"].(map[string]any)["key"].(string)

	team, err := data.TeamByID(open.teamID)
	if err != nil {
		t.Fatalf("TeamByID() error = %v", err)
	}
	missing := request(t, server, http.MethodPost, "/v1/events/"+open.slug+"/submissions", open.captain, map[string]any{
		"team_id":  team.ID,
		"track_id": open.trackID,
		"title":    "Needs answers",
		"summary":  "Missing a required custom answer.",
	})
	if missing.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", missing.Code, missing.Body.String())
	}

	invalidURL := request(t, server, http.MethodPost, "/v1/events/"+open.slug+"/submissions", open.captain, map[string]any{
		"team_id":        team.ID,
		"track_id":       open.trackID,
		"title":          "Bad url answer",
		"summary":        "The url answer is not a url.",
		"custom_answers": map[string]string{key: "not-a-url"},
	})
	if invalidURL.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", invalidURL.Code, invalidURL.Body.String())
	}

	accepted := request(t, server, http.MethodPost, "/v1/events/"+open.slug+"/submissions", open.captain, map[string]any{
		"team_id":        team.ID,
		"track_id":       open.trackID,
		"title":          "Complete answers",
		"summary":        "All custom questions answered.",
		"story":          "We built this in a weekend and learned a lot about streaming data.",
		"repo_url":       "https://github.com/example/complete-answers",
		"live_url":       "https://example.org/demo",
		"custom_answers": map[string]string{key: "https://example.org/demo"},
	})
	if accepted.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", accepted.Code, accepted.Body.String())
	}
	submission := decodeAssignmentsBody(t, accepted)["data"].(map[string]any)
	if submission["story"] == nil || submission["story"] == "" {
		t.Fatalf("expected the project story to be stored: %s", accepted.Body.String())
	}
}

func TestRepositoryURLMustLookLikeARepository(t *testing.T) {
	server, tokens, data := newTestServer(t)
	open := createOpenEvent(t, server, tokens, data)
	team, err := data.TeamByID(open.teamID)
	if err != nil {
		t.Fatalf("TeamByID() error = %v", err)
	}
	response := request(t, server, http.MethodPost, "/v1/events/"+open.slug+"/submissions", open.captain, map[string]any{
		"team_id":  team.ID,
		"track_id": open.trackID,
		"title":    "Bad repo",
		"summary":  "The repo url has no repository path.",
		"repo_url": "https://github.com",
	})
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", response.Code, response.Body.String())
	}
}

func TestMilestonesAndHosts(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	milestone := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/milestones", organizer, map[string]any{
		"title":    "Demo day",
		"due_at":   "2026-03-10T18:00:00Z",
		"detail":   "Teams present their work.",
		"position": 4,
	})
	if milestone.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", milestone.Code, milestone.Body.String())
	}
	if bad := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/milestones", organizer, map[string]any{"title": "No date", "due_at": "soon"}); bad.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", bad.Code, bad.Body.String())
	}
	host := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/hosts", organizer, map[string]any{"name": "Acme", "url": "https://acme.example.org"})
	if host.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", host.Code, host.Body.String())
	}
	detail := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026", "", nil)
	if detail.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", detail.Code, detail.Body.String())
	}
	for _, expected := range []string{"Demo day", "Acme", "Standard judging rubric", "Northwind Labs"} {
		if !containsProjectTitle(detail.Body.String(), expected) {
			t.Fatalf("expected %q in hackathon detail: %s", expected, detail.Body.String())
		}
	}
}

func TestJudgeRosterLifecycle(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	added := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/judges", organizer, map[string]any{
		"judge_id": "judge_a",
		"headline": "Infrastructure",
		"tracks":   []string{"trk_01", "trk_02"},
		"capacity": 8,
	})
	if added.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", added.Code, added.Body.String())
	}
	listed := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/judges", "", nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", listed.Code, listed.Body.String())
	}
	if !containsProjectTitle(listed.Body.String(), "Infrastructure") {
		t.Fatalf("expected roster entry with headline: %s", listed.Body.String())
	}
	if !containsProjectTitle(listed.Body.String(), "judge-a@example.org") {
		t.Fatalf("expected roster to include judge email for organizers: %s", listed.Body.String())
	}

	notAJudge := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/judges", organizer, map[string]any{"judge_id": "participant"})
	if notAJudge.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", notAJudge.Code, notAJudge.Body.String())
	}

	removed := request(t, server, http.MethodDelete, "/v1/events/sample-hack-2026/judges/judge_a", organizer, nil)
	if removed.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", removed.Code, removed.Body.String())
	}
	afterRemoval := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/judges", "", nil)
	if containsProjectTitle(afterRemoval.Body.String(), "judge-a@example.org") {
		t.Fatalf("expected the removed judge to leave the roster: %s", afterRemoval.Body.String())
	}
}

func TestAssignmentOnlyUsesHackathonRoster(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	if removed := request(t, server, http.MethodDelete, "/v1/events/sample-hack-2026/judges/judge_a", organizer, nil); removed.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", removed.Code, removed.Body.String())
	}
	projects := data.ListSubmissions("evt_01", "", "")
	project := projects[0]
	created := request(t, server, http.MethodPost, "/v1/organizer/assignments", organizer, map[string]any{
		"event_slug":          "sample-hack-2026",
		"judge_ids":           []string{"judge_a", "judge_b"},
		"project_ids":         []string{project.ID},
		"reviews_per_project": 1,
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", created.Code, created.Body.String())
	}
	assignments := decodeAssignmentsBody(t, created)["created"].([]any)
	if len(assignments) != 1 {
		t.Fatalf("expected one assignment for the remaining judge: %s", created.Body.String())
	}
	if assignments[0].(map[string]any)["judge_id"] != "judge_b" {
		t.Fatalf("expected only the roster judge to be assignable: %s", created.Body.String())
	}

	onlyRemoved := request(t, server, http.MethodPost, "/v1/organizer/assignments", organizer, map[string]any{
		"event_slug":          "sample-hack-2026",
		"judge_ids":           []string{"judge_a"},
		"project_ids":         []string{project.ID},
		"reviews_per_project": 1,
	})
	if onlyRemoved.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", onlyRemoved.Code, onlyRemoved.Body.String())
	}
}
