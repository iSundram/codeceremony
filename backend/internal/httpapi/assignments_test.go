package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

func decodeAssignmentsBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, body = %s", err, recorder.Body.String())
	}
	return payload
}

func bodyList(t *testing.T, payload map[string]any, key string) []any {
	t.Helper()
	items, ok := payload[key].([]any)
	if !ok {
		t.Fatalf("expected %q to be a list, payload = %#v", key, payload)
	}
	return items
}

func TestOrganizerBulkAssignsBalancedAssignments(t *testing.T) {
	server, tokens, data := newTestServer(t)
	response := request(t, server, http.MethodPost, "/v1/organizer/assignments", tokenFor(t, tokens, data, "organizer"), map[string]any{
		"event_slug":          "sample-hack-2026",
		"strategy":            "balanced",
		"reviews_per_project": 2,
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", response.Code, response.Body.String())
	}
	created := bodyList(t, decodeAssignmentsBody(t, response), "created")
	if len(created) == 0 {
		t.Fatalf("expected created assignments, body = %s", response.Body.String())
	}
	byJudge := map[string]int{}
	for _, item := range created {
		entry := item.(map[string]any)
		byJudge[entry["judge_id"].(string)]++
	}
	lowest, highest := created[0].(map[string]any), created[0].(map[string]any)
	for _, item := range created {
		entry := item.(map[string]any)
		if byJudge[entry["judge_id"].(string)] < byJudge[lowest["judge_id"].(string)] {
			lowest = entry
		}
		if byJudge[entry["judge_id"].(string)] > byJudge[highest["judge_id"].(string)] {
			highest = entry
		}
	}
	spread := byJudge[highest["judge_id"].(string)] - byJudge[lowest["judge_id"].(string)]
	if spread > 1 {
		t.Fatalf("expected a balanced spread of at most 1, got %#v", byJudge)
	}
	if len(byJudge) < 2 {
		t.Fatalf("expected more than one judge to be used, got %#v", byJudge)
	}
	for _, item := range created {
		entry := item.(map[string]any)
		if entry["strategy"] != "balanced" {
			t.Fatalf("expected strategy to be recorded, got %#v", entry)
		}
	}
}

func TestBulkAssignmentRespectsTrackScope(t *testing.T) {
	server, tokens, data := newTestServer(t)
	projects := data.ListSubmissions("evt_01", "", "")
	if len(projects) == 0 {
		t.Fatalf("expected seeded submissions")
	}
	project := projects[0]
	otherTrack := "trk_02"
	if project.TrackID == otherTrack {
		otherTrack = "trk_03"
	}
	profile, err := data.JudgeProfile("judge_a")
	if err != nil {
		t.Fatalf("JudgeProfile() error = %v", err)
	}
	profile.Tracks = []string{otherTrack}
	if err := data.SetJudgeProfile(profile); err != nil {
		t.Fatalf("SetJudgeProfile() error = %v", err)
	}
	response := request(t, server, http.MethodPost, "/v1/organizer/assignments", tokenFor(t, tokens, data, "organizer"), map[string]any{
		"event_slug":          "sample-hack-2026",
		"judge_ids":           []string{"judge_a"},
		"project_ids":         []string{project.ID},
		"reviews_per_project": 1,
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", response.Code, response.Body.String())
	}
	payload := decodeAssignmentsBody(t, response)
	skipped := bodyList(t, payload, "skipped")
	if len(skipped) != 1 {
		t.Fatalf("expected one skipped assignment, body = %s", response.Body.String())
	}
	if reason := skipped[0].(map[string]any)["reason"]; reason != "judge_track_scope_mismatch" {
		t.Fatalf("reason = %v, want judge_track_scope_mismatch", reason)
	}
}

func TestOrganizerAssignmentListHidesRevoked(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	created := request(t, server, http.MethodPost, "/v1/organizer/assignments", organizer, map[string]any{
		"event_slug":          "sample-hack-2026",
		"reviews_per_project": 1,
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", created.Code, created.Body.String())
	}
	assignmentID := bodyList(t, decodeAssignmentsBody(t, created), "created")[0].(map[string]any)["id"].(string)

	revoked := request(t, server, http.MethodDelete, "/v1/organizer/assignments/"+assignmentID+"?reason=conflict", organizer, nil)
	if revoked.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", revoked.Code, revoked.Body.String())
	}
	listed := request(t, server, http.MethodGet, "/v1/organizer/assignments?event_id=evt_01", organizer, nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", listed.Code, listed.Body.String())
	}
	for _, item := range bodyList(t, decodeAssignmentsBody(t, listed), "data") {
		if item.(map[string]any)["id"] == assignmentID {
			t.Fatalf("revoked assignment should not be listed: %s", listed.Body.String())
		}
	}
}

func TestJudgeSeesOnlyOwnAssignments(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	created := request(t, server, http.MethodPost, "/v1/organizer/assignments", organizer, map[string]any{
		"event_slug":          "sample-hack-2026",
		"reviews_per_project": 1,
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", created.Code, created.Body.String())
	}
	response := request(t, server, http.MethodGet, "/v1/judge/assignments?event_id=evt_01", tokenFor(t, tokens, data, "judge_a"), nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", response.Code, response.Body.String())
	}
	payload := decodeAssignmentsBody(t, response)
	for _, item := range bodyList(t, payload, "data") {
		entry := item.(map[string]any)
		if entry["judge_id"] != "judge_a" {
			t.Fatalf("judge received another judge's assignment: %s", response.Body.String())
		}
		if _, ok := entry["judge_email"]; ok {
			t.Fatalf("judge list must not expose judge emails: %s", response.Body.String())
		}
	}
	if _, ok := payload["pending"]; !ok {
		t.Fatalf("expected a pending count: %s", response.Body.String())
	}
}

func TestJudgeConflictDeclarationRevokesAssignment(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	created := request(t, server, http.MethodPost, "/v1/organizer/assignments", organizer, map[string]any{
		"event_slug":          "sample-hack-2026",
		"judge_ids":           []string{"judge_a"},
		"reviews_per_project": 1,
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", created.Code, created.Body.String())
	}
	assignmentID := bodyList(t, decodeAssignmentsBody(t, created), "created")[0].(map[string]any)["id"].(string)
	assignment, err := data.AssignmentByID(assignmentID)
	if err != nil {
		t.Fatalf("AssignmentByID() error = %v", err)
	}

	declaration := request(t, server, http.MethodPost, "/v1/judge/assignments/"+assignmentID+"/conflict", tokenFor(t, tokens, data, "judge_a"), map[string]any{
		"reason": "I built this project.",
	})
	if declaration.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", declaration.Code, declaration.Body.String())
	}
	revoked, err := data.AssignmentByID(assignmentID)
	if err != nil {
		t.Fatalf("AssignmentByID() error = %v", err)
	}
	if revoked.RevokedAt == nil {
		t.Fatalf("expected the assignment to be revoked after a conflict")
	}
	if !data.HasConflict(assignment.EventID, "judge_a", assignment.ProjectID) {
		t.Fatalf("expected the conflict to be recorded")
	}

	reassigned := request(t, server, http.MethodPost, "/v1/organizer/assignments", organizer, map[string]any{
		"event_slug":          "sample-hack-2026",
		"judge_ids":           []string{"judge_a"},
		"project_ids":         []string{assignment.ProjectID},
		"reviews_per_project": 1,
	})
	if reassigned.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", reassigned.Code, reassigned.Body.String())
	}
	skipped := bodyList(t, decodeAssignmentsBody(t, reassigned), "skipped")
	if len(skipped) != 1 || skipped[0].(map[string]any)["reason"] != "capacity_or_conflict" {
		t.Fatalf("expected a conflict skip, body = %s", reassigned.Body.String())
	}
}

func TestJudgeCannotDeclareConflictForAnotherJudge(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	created := request(t, server, http.MethodPost, "/v1/organizer/assignments", organizer, map[string]any{
		"event_slug":          "sample-hack-2026",
		"judge_ids":           []string{"judge_b"},
		"reviews_per_project": 1,
	})
	assignmentID := bodyList(t, decodeAssignmentsBody(t, created), "created")[0].(map[string]any)["id"].(string)
	response := request(t, server, http.MethodPost, "/v1/judge/assignments/"+assignmentID+"/conflict", tokenFor(t, tokens, data, "judge_a"), map[string]any{"reason": "nope"})
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", response.Code, response.Body.String())
	}
}

func TestParticipantCannotListAssignments(t *testing.T) {
	server, tokens, data := newTestServer(t)
	response := request(t, server, http.MethodGet, "/v1/organizer/assignments?event_id=evt_01", tokenFor(t, tokens, data, "participant"), nil)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", response.Code, response.Body.String())
	}
}

func TestAssignmentRejectsUnknownProject(t *testing.T) {
	server, tokens, data := newTestServer(t)
	response := request(t, server, http.MethodPost, "/v1/organizer/assignments", tokenFor(t, tokens, data, "organizer"), map[string]any{
		"event_slug":          "sample-hack-2026",
		"project_ids":         []string{"prj_missing"},
		"reviews_per_project": 1,
	})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", response.Code, response.Body.String())
	}
}

func TestJudgeProfileRequiresJudgeRole(t *testing.T) {
	_, _, data := newTestServer(t)
	err := data.SetJudgeProfile(domain.JudgeProfile{UserID: "participant", Active: true})
	if err == nil {
		t.Fatalf("expected a non-judge profile to be rejected")
	}
}
