package httpapi

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

func openSubmissionWindow(t *testing.T, server *Server) {
	t.Helper()
	server.SetClock(func() time.Time { return time.Date(2026, time.February, 28, 12, 0, 0, 0, time.UTC) })
}

type openEvent struct {
	slug      string
	eventID   string
	trackID   string
	teamID    string
	projectID string
	organizer string
	captain   string
}

func createOpenEvent(t *testing.T, server *Server, tokens *auth.Manager, data *store.Store) openEvent {
	t.Helper()
	openSubmissionWindow(t, server)
	organizer := tokenFor(t, tokens, data, "organizer")
	captain := tokenFor(t, tokens, data, "participant")
	created := request(t, server, http.MethodPost, "/v1/events", organizer, map[string]any{
		"slug":              "open-hack-2026",
		"name":              "Open Hack 2026",
		"registration_open": true,
		"submissions_open":  true,
		"submissions_close": "2026-03-01T18:00:00Z",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("event create status = %d, body = %s", created.Code, created.Body.String())
	}
	event := decodeAssignmentsBody(t, created)["data"].(map[string]any)
	trackResponse := request(t, server, http.MethodPost, "/v1/events/open-hack-2026/tracks", organizer, map[string]any{"name": "General"})
	if trackResponse.Code != http.StatusCreated {
		t.Fatalf("track create status = %d, body = %s", trackResponse.Code, trackResponse.Body.String())
	}
	trackID := decodeAssignmentsBody(t, trackResponse)["data"].(map[string]any)["id"].(string)
	teamResponse := request(t, server, http.MethodPost, "/v1/events/open-hack-2026/teams", captain, map[string]any{"name": "Bright Forge"})
	if teamResponse.Code != http.StatusCreated {
		t.Fatalf("team create status = %d, body = %s", teamResponse.Code, teamResponse.Body.String())
	}
	teamID := decodeAssignmentsBody(t, teamResponse)["data"].(map[string]any)["id"].(string)
	submission := request(t, server, http.MethodPost, "/v1/events/open-hack-2026/submissions", captain, map[string]any{
		"team_id":  teamID,
		"track_id": trackID,
		"title":    "Lantern",
		"summary":  "A project submitted inside the open window.",
		"repo_url": "https://example.org/repo/lantern",
	})
	if submission.Code != http.StatusCreated {
		t.Fatalf("submission create status = %d, body = %s", submission.Code, submission.Body.String())
	}
	project := decodeAssignmentsBody(t, submission)["data"].(map[string]any)
	return openEvent{
		slug:      "open-hack-2026",
		eventID:   event["id"].(string),
		trackID:   trackID,
		teamID:    teamID,
		projectID: project["id"].(string),
		organizer: organizer,
		captain:   captain,
	}
}

func TestCaptainEditCreatesVersionHistory(t *testing.T) {
	server, tokens, data := newTestServer(t)
	open := createOpenEvent(t, server, tokens, data)
	captain := open.captain
	response := request(t, server, http.MethodPatch, "/v1/submissions/"+open.projectID, captain, map[string]any{
		"title":  "Glass Signal v2",
		"reason": "clarified the summary",
	})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", response.Code, response.Body.String())
	}
	payload := decodeAssignmentsBody(t, response)
	submission := payload["data"].(map[string]any)
	if submission["status"] != string(domain.SubmissionDraft) {
		t.Fatalf("status = %v, want draft after an edit", submission["status"])
	}
	if submission["version"].(float64) != 2 {
		t.Fatalf("version = %v, want 2", submission["version"])
	}

	detail := request(t, server, http.MethodGet, "/v1/submissions/"+open.projectID, captain, nil)
	if detail.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", detail.Code, detail.Body.String())
	}
	versions := decodeAssignmentsBody(t, detail)["versions"].([]any)
	if len(versions) != 1 {
		t.Fatalf("expected one recorded version, body = %s", detail.Body.String())
	}
	if versions[0].(map[string]any)["version"].(float64) != 2 {
		t.Fatalf("version snapshot = %s", detail.Body.String())
	}
}

func TestNonCaptainCannotEditSubmission(t *testing.T) {
	server, tokens, data := newTestServer(t)
	response := request(t, server, http.MethodPatch, "/v1/submissions/prj_01", tokenFor(t, tokens, data, "participant_other"), map[string]any{"title": "Hijacked"})
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", response.Code, response.Body.String())
	}
}

func TestEditedSubmissionMustBeResubmitted(t *testing.T) {
	server, tokens, data := newTestServer(t)
	open := createOpenEvent(t, server, tokens, data)
	edited := request(t, server, http.MethodPatch, "/v1/submissions/"+open.projectID, open.captain, map[string]any{"title": "Draft again"})
	if edited.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", edited.Code, edited.Body.String())
	}
	gallery := request(t, server, http.MethodGet, "/v1/events/"+open.slug+"/projects", "", nil)
	if containsProjectTitle(gallery.Body.String(), "Draft again") {
		t.Fatalf("drafts must not appear in the public gallery: %s", gallery.Body.String())
	}
	progress := request(t, server, http.MethodGet, "/v1/organizer/progress?event_id="+open.eventID, open.organizer, nil)
	payload := decodeAssignmentsBody(t, progress)["data"].(map[string]any)
	if payload["submissions"].(float64) != 0 {
		t.Fatalf("a draft must not count as a submission, body = %s", progress.Body.String())
	}
	resubmitted := request(t, server, http.MethodPost, "/v1/submissions/"+open.projectID+"/submit", open.captain, nil)
	if resubmitted.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", resubmitted.Code, resubmitted.Body.String())
	}
	gallery = request(t, server, http.MethodGet, "/v1/events/"+open.slug+"/projects", "", nil)
	if !containsProjectTitle(gallery.Body.String(), "Draft again") {
		t.Fatalf("expected the resubmitted project in the gallery: %s", gallery.Body.String())
	}
}

func TestIneligibleSubmissionLeavesGallery(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	response := request(t, server, http.MethodPut, "/v1/organizer/submissions/prj_02/eligibility", organizer, map[string]any{
		"decision": "ineligible",
		"note":     "repository is empty",
	})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", response.Code, response.Body.String())
	}
	gallery := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/projects", "", nil)
	if containsProjectTitle(gallery.Body.String(), "Signal Garden") {
		t.Fatalf("an ineligible submission must not appear in the gallery: %s", gallery.Body.String())
	}
}

func TestIneligibleDecisionRequiresNote(t *testing.T) {
	server, tokens, data := newTestServer(t)
	response := request(t, server, http.MethodPut, "/v1/organizer/submissions/prj_02/eligibility", tokenFor(t, tokens, data, "organizer"), map[string]any{"decision": "ineligible"})
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", response.Code, response.Body.String())
	}
}

func TestOrganizerStatusFlowEndsAtLock(t *testing.T) {
	server, tokens, data := newTestServer(t)
	open := createOpenEvent(t, server, tokens, data)
	needsChange := request(t, server, http.MethodPut, "/v1/organizer/submissions/"+open.projectID+"/status", open.organizer, map[string]any{
		"status": "needs_changes",
		"note":   "add a demo video",
	})
	if needsChange.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", needsChange.Code, needsChange.Body.String())
	}
	locked := request(t, server, http.MethodPut, "/v1/organizer/submissions/"+open.projectID+"/status", open.organizer, map[string]any{"status": "locked"})
	if locked.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", locked.Code, locked.Body.String())
	}
	captainEdit := request(t, server, http.MethodPatch, "/v1/submissions/"+open.projectID, open.captain, map[string]any{"title": "Too late"})
	if captainEdit.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", captainEdit.Code, captainEdit.Body.String())
	}
	illegal := request(t, server, http.MethodPut, "/v1/organizer/submissions/"+open.projectID+"/status", open.organizer, map[string]any{"status": "submitted"})
	if illegal.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", illegal.Code, illegal.Body.String())
	}
}

func TestWithdrawnSubmissionLeavesGallery(t *testing.T) {
	server, tokens, data := newTestServer(t)
	open := createOpenEvent(t, server, tokens, data)
	withdrawn := request(t, server, http.MethodPost, "/v1/submissions/"+open.projectID+"/withdraw", open.captain, map[string]any{"reason": "we are pivoting"})
	if withdrawn.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", withdrawn.Code, withdrawn.Body.String())
	}
	gallery := request(t, server, http.MethodGet, "/v1/events/"+open.slug+"/projects", "", nil)
	if containsProjectTitle(gallery.Body.String(), "Lantern") {
		t.Fatalf("a withdrawn submission must leave the gallery: %s", gallery.Body.String())
	}
}

func TestJudgeCannotReadUnassignedSubmission(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	created := request(t, server, http.MethodPost, "/v1/organizer/assignments", organizer, map[string]any{
		"event_slug":          "sample-hack-2026",
		"reviews_per_project": 1,
	})
	assignedToJudgeA := map[string]bool{}
	for _, item := range decodeAssignmentsBody(t, created)["created"].([]any) {
		entry := item.(map[string]any)
		if entry["judge_id"] == "judge_a" {
			assignedToJudgeA[entry["project_id"].(string)] = true
		}
	}
	other := ""
	for _, project := range data.ListSubmissions("evt_01", "", "") {
		if !assignedToJudgeA[project.ID] {
			other = project.ID
			break
		}
	}
	if other == "" {
		t.Fatalf("expected at least one unassigned submission")
	}
	response := request(t, server, http.MethodGet, "/v1/submissions/"+other, tokenFor(t, tokens, data, "judge_a"), nil)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", response.Code, response.Body.String())
	}
}

func TestDuplicateScanFlagsSharedRepository(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	if _, err := data.UpdateSubmission("prj_02", func(submission domain.Submission) (domain.Submission, error) {
		submission.RepositoryURL = "https://example.org/repo/01"
		return submission, nil
	}); err != nil {
		t.Fatalf("UpdateSubmission() error = %v", err)
	}
	scanned := request(t, server, http.MethodPost, "/v1/organizer/duplicates/scan?event_id=evt_01", organizer, nil)
	if scanned.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", scanned.Code, scanned.Body.String())
	}
	payload := decodeAssignmentsBody(t, scanned)
	if payload["created"].(float64) < 1 {
		t.Fatalf("expected at least one duplicate flag, body = %s", scanned.Body.String())
	}
	flag := payload["data"].([]any)[0].(map[string]any)
	if flag["signal"] != "repository" {
		t.Fatalf("signal = %v, want repository", flag["signal"])
	}

	resolved := request(t, server, http.MethodPut, "/v1/organizer/duplicates/"+flag["id"].(string), organizer, map[string]any{
		"status": "dismissed",
		"note":   "teams forked the same starter repo",
	})
	if resolved.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", resolved.Code, resolved.Body.String())
	}
	if resolvedStatus := decodeAssignmentsBody(t, resolved)["data"].(map[string]any)["status"]; resolvedStatus != "dismissed" {
		t.Fatalf("status = %v, want dismissed", resolvedStatus)
	}

	rescan := request(t, server, http.MethodPost, "/v1/organizer/duplicates/scan?event_id=evt_01", organizer, nil)
	if decodeAssignmentsBody(t, rescan)["created"].(float64) != 0 {
		t.Fatalf("expected the scan to be idempotent, body = %s", rescan.Body.String())
	}
}

func TestParticipantCannotScanDuplicates(t *testing.T) {
	server, tokens, data := newTestServer(t)
	response := request(t, server, http.MethodPost, "/v1/organizer/duplicates/scan", tokenFor(t, tokens, data, "participant"), nil)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", response.Code, response.Body.String())
	}
}

func containsProjectTitle(body, title string) bool {
	return strings.Contains(body, title)
}
