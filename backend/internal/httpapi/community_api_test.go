package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/mailer"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

func TestCommentThreadAndModeration(t *testing.T) {
	server, tokens, data := newTestServer(t)
	participant := tokenFor(t, tokens, data, "participant")
	other := tokenFor(t, tokens, data, "participant_other")

	created := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/projects/prj_01/comments", participant, map[string]any{
		"body": "This is a thoughtful build. The streaming demo is unusually good.",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", created.Code, created.Body.String())
	}
	commentID := decodeAssignmentsBody(t, created)["data"].(map[string]any)["id"].(string)

	reply := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/projects/prj_01/comments", other, map[string]any{
		"body":      "Agreed, the docs are unusually clear.",
		"parent_id": commentID,
	})
	if reply.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", reply.Code, reply.Body.String())
	}

	empty := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/comments/x", participant, map[string]any{"body": "   "})
	_ = empty
	badBody := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/projects/prj_01/comments", participant, map[string]any{"body": ""})
	if badBody.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", badBody.Code, badBody.Body.String())
	}

	anonymous := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/projects/prj_01/comments", "", map[string]any{"body": "hi"})
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", anonymous.Code, anonymous.Body.String())
	}

	public := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/comments?project_id=prj_01", "", nil)
	if public.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", public.Code, public.Body.String())
	}
	if !containsProjectTitle(public.Body.String(), "streaming demo") {
		t.Fatalf("expected the public comment: %s", public.Body.String())
	}

	organizer := tokenFor(t, tokens, data, "organizer")
	hidden := request(t, server, http.MethodPut, "/v1/organizer/comments/"+commentID+"/moderate", organizer, map[string]any{
		"status": "hidden",
		"note":   "contains a personal attack",
	})
	if hidden.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", hidden.Code, hidden.Body.String())
	}
	noNote := request(t, server, http.MethodPut, "/v1/organizer/comments/"+commentID+"/moderate", organizer, map[string]any{"status": "hidden"})
	if noNote.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", noNote.Code, noNote.Body.String())
	}

	publicAfter := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/comments?project_id=prj_01", "", nil)
	if containsProjectTitle(publicAfter.Body.String(), "unusually good") {
		t.Fatalf("hidden comments must not be public: %s", publicAfter.Body.String())
	}
	organizerView := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/comments?project_id=prj_01", organizer, nil)
	if !containsProjectTitle(organizerView.Body.String(), "unusually good") {
		t.Fatalf("organizers should still see hidden comments: %s", organizerView.Body.String())
	}
	participantAttempt := request(t, server, http.MethodPut, "/v1/organizer/comments/"+commentID+"/moderate", participant, map[string]any{"status": "visible", "note": "undo"})
	if participantAttempt.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", participantAttempt.Code, participantAttempt.Body.String())
	}
}

func TestCommentLimitPerAuthor(t *testing.T) {
	server, tokens, data := newTestServer(t)
	participant := tokenFor(t, tokens, data, "participant")
	for index := 0; index < 5; index++ {
		response := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/projects/prj_01/comments", participant, map[string]any{"body": "comment " + string(rune('a'+index))})
		if response.Code != http.StatusCreated {
			t.Fatalf("comment %d status = %d, want 201, body = %s", index, response.Code, response.Body.String())
		}
	}
	overflow := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/projects/prj_01/comments", participant, map[string]any{"body": "one too many"})
	if overflow.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", overflow.Code, overflow.Body.String())
	}
}

func TestCommentReportAndResolution(t *testing.T) {
	server, tokens, data := newTestServer(t)
	participant := tokenFor(t, tokens, data, "participant")
	other := tokenFor(t, tokens, data, "participant_other")
	organizer := tokenFor(t, tokens, data, "organizer")
	created := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/projects/prj_02/comments", participant, map[string]any{"body": "Check this out"})
	commentID := decodeAssignmentsBody(t, created)["data"].(map[string]any)["id"].(string)

	reported := request(t, server, http.MethodPost, "/v1/comments/"+commentID+"/report", other, map[string]any{"reason": "spam"})
	if reported.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", reported.Code, reported.Body.String())
	}
	duplicate := request(t, server, http.MethodPost, "/v1/comments/"+commentID+"/report", other, map[string]any{"reason": "spam again"})
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", duplicate.Code, duplicate.Body.String())
	}
	noReason := request(t, server, http.MethodPost, "/v1/comments/"+commentID+"/report", tokenFor(t, tokens, data, "judge_a"), map[string]any{"reason": ""})
	if noReason.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", noReason.Code, noReason.Body.String())
	}

	queue := request(t, server, http.MethodGet, "/v1/organizer/reports?event_id=evt_01", organizer, nil)
	if !containsProjectTitle(queue.Body.String(), "spam") {
		t.Fatalf("expected the report in the queue: %s", queue.Body.String())
	}
	reportID := decodeAssignmentsBody(t, queue)["data"].([]any)[0].(map[string]any)["id"].(string)
	resolved := request(t, server, http.MethodPut, "/v1/organizer/reports/"+reportID, organizer, map[string]any{
		"status": "dismissed",
		"note":   "not spam",
	})
	if resolved.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", resolved.Code, resolved.Body.String())
	}
	badStatus := request(t, server, http.MethodPut, "/v1/organizer/reports/"+reportID, organizer, map[string]any{"status": "open", "note": "undo"})
	if badStatus.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", badStatus.Code, badStatus.Body.String())
	}
}

func TestAuthorCanDeleteOwnComment(t *testing.T) {
	server, tokens, data := newTestServer(t)
	participant := tokenFor(t, tokens, data, "participant")
	other := tokenFor(t, tokens, data, "participant_other")
	created := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/projects/prj_03/comments", participant, map[string]any{"body": "my own words"})
	commentID := decodeAssignmentsBody(t, created)["data"].(map[string]any)["id"].(string)
	forbidden := request(t, server, http.MethodDelete, "/v1/comments/"+commentID, other, nil)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", forbidden.Code, forbidden.Body.String())
	}
	deleted := request(t, server, http.MethodDelete, "/v1/comments/"+commentID, participant, nil)
	if deleted.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", deleted.Code, deleted.Body.String())
	}
}

func TestCommunityVotingLifecycle(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	created := request(t, server, http.MethodPost, "/v1/organizer/events/sample-hack-2026/vote-campaigns", organizer, map[string]any{
		"name":                 "Community choice",
		"max_choices_per_user": 2,
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", created.Code, created.Body.String())
	}
	campaignID := decodeAssignmentsBody(t, created)["data"].(map[string]any)["id"].(string)

	beforeOpen := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/vote", tokenFor(t, tokens, data, "participant"), map[string]any{
		"campaign_id": campaignID, "project_ids": []string{"prj_01"},
	})
	if beforeOpen.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", beforeOpen.Code, beforeOpen.Body.String())
	}

	hidden := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/vote?campaign_id="+campaignID, "", nil)
	if hidden.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", hidden.Code, hidden.Body.String())
	}

	if opened := request(t, server, http.MethodPut, "/v1/organizer/vote-campaigns/"+campaignID+"/open", organizer, nil); opened.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", opened.Code, opened.Body.String())
	}
	if reopen := request(t, server, http.MethodPut, "/v1/organizer/vote-campaigns/"+campaignID+"/open", organizer, nil); reopen.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", reopen.Code, reopen.Body.String())
	}

	tooMany := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/vote", tokenFor(t, tokens, data, "judge_a"), map[string]any{
		"campaign_id": campaignID, "project_ids": []string{"prj_01", "prj_02", "prj_03"},
	})
	if tooMany.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", tooMany.Code, tooMany.Body.String())
	}

	first := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/vote", tokenFor(t, tokens, data, "participant"), map[string]any{
		"campaign_id": campaignID, "project_ids": []string{"prj_01"},
	})
	if first.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", first.Code, first.Body.String())
	}
	second := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/vote", tokenFor(t, tokens, data, "participant_other"), map[string]any{
		"campaign_id": campaignID, "project_ids": []string{"prj_01"},
	})
	if second.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", second.Code, second.Body.String())
	}
	secondChoice := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/vote", tokenFor(t, tokens, data, "participant"), map[string]any{
		"campaign_id": campaignID, "project_ids": []string{"prj_02"},
	})
	if secondChoice.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", secondChoice.Code, secondChoice.Body.String())
	}
	overBudget := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/vote", tokenFor(t, tokens, data, "participant"), map[string]any{
		"campaign_id": campaignID, "project_ids": []string{"prj_04"},
	})
	if overBudget.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", overBudget.Code, overBudget.Body.String())
	}
	duplicateChoice := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/vote", tokenFor(t, tokens, data, "judge_a"), map[string]any{
		"campaign_id": campaignID, "project_ids": []string{"prj_03", "prj_03"},
	})
	if duplicateChoice.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", duplicateChoice.Code, duplicateChoice.Body.String())
	}
	foreign := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/vote", tokenFor(t, tokens, data, "judge_a"), map[string]any{
		"campaign_id": campaignID, "project_ids": []string{"prj_missing"},
	})
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", foreign.Code, foreign.Body.String())
	}

	if closed := request(t, server, http.MethodPut, "/v1/organizer/vote-campaigns/"+campaignID+"/closed", organizer, nil); closed.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", closed.Code, closed.Body.String())
	}
	results := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/vote?campaign_id="+campaignID, "", nil)
	if results.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", results.Code, results.Body.String())
	}
	payload := decodeAssignmentsBody(t, results)["data"].(map[string]any)
	entries := payload["results"].([]any)
	if len(entries) != 2 {
		t.Fatalf("expected two projects with votes, body = %s", results.Body.String())
	}
	winner := entries[0].(map[string]any)
	if winner["project_id"] != "prj_01" || winner["votes"].(float64) != 2 || winner["rank"].(float64) != 1 {
		t.Fatalf("unexpected tally: %v", winner)
	}
	runnerUp := entries[1].(map[string]any)
	if runnerUp["project_id"] != "prj_02" || runnerUp["rank"].(float64) != 2 {
		t.Fatalf("unexpected second place: %v", runnerUp)
	}
	if payload["voters"].(float64) != 2 {
		t.Fatalf("expected two voters: %s", results.Body.String())
	}
	if !containsProjectTitle(results.Body.String(), "tiebreaker") {
		t.Fatalf("expected the tiebreaker note: %s", results.Body.String())
	}
}

func TestExportBundleAndImport(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	exported := request(t, server, http.MethodGet, "/v1/organizer/export?event_id=evt_01", organizer, nil)
	if exported.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", exported.Code, exported.Body.String())
	}
	var payload struct {
		Data ExportBundle `json:"data"`
	}
	if err := json.Unmarshal(exported.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	bundle := payload.Data
	if bundle.Event.Slug != "sample-hack-2026" || len(bundle.Submissions) != 4 || len(bundle.Tracks) != 3 {
		t.Fatalf("unexpected bundle: event=%s submissions=%d tracks=%d", bundle.Event.Slug, len(bundle.Submissions), len(bundle.Tracks))
	}
	if strings.Contains(exported.Body.String(), "codeceremony-dev") {
		t.Fatalf("export must not include credentials")
	}
	if !containsProjectTitle(exported.Body.String(), "password hashes, session tokens") {
		t.Fatalf("expected the export notes: %s", exported.Body.String())
	}

	bundle.Event.Name = "Imported Hack 2026"
	bundle.Event.Slug = "imported-hack-2026"
	dryRun := request(t, server, http.MethodPost, "/v1/organizer/import", organizer, map[string]any{
		"dry_run": true,
		"bundle":  bundle,
	})
	if dryRun.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", dryRun.Code, dryRun.Body.String())
	}
	plan := decodeAssignmentsBody(t, dryRun)["plan"].(map[string]any)
	if plan["dry_run"] != true || plan["submissions"].(float64) != 4 {
		t.Fatalf("unexpected dry run plan: %v", plan)
	}
	if _, err := data.EventBySlug("imported-hack-2026"); err == nil {
		t.Fatalf("a dry run must not create the hackathon")
	}

	applied := request(t, server, http.MethodPost, "/v1/organizer/import", organizer, map[string]any{"bundle": bundle})
	if applied.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", applied.Code, applied.Body.String())
	}
	appliedPlan := decodeAssignmentsBody(t, applied)["plan"].(map[string]any)
	if appliedPlan["created"] != true || appliedPlan["teams"] == nil {
		t.Fatalf("unexpected apply plan: %v", appliedPlan)
	}
	imported, err := data.EventBySlug("imported-hack-2026")
	if err != nil {
		t.Fatalf("EventBySlug() error = %v", err)
	}
	if imported.ResultsPublished {
		t.Fatalf("an imported hackathon must not start with published results")
	}
	importedProjects := data.ListSubmissions(imported.ID, "", "")
	if len(importedProjects) != 4 {
		t.Fatalf("expected four imported submissions, got %d", len(importedProjects))
	}
	importedEvent, _ := data.EventByID(imported.ID)
	importedRubrics := data.RubricsForEvent(imported.ID)
	if len(importedRubrics) == 0 {
		t.Fatalf("expected the rubric to be imported as a draft")
	}
	for _, rubric := range importedRubrics {
		if rubric.Status != domain.RubricDraft {
			t.Fatalf("imported rubrics must start as drafts, got %s", rubric.Status)
		}
	}
	if _, err := data.ActiveRubric(imported.ID, ""); err == nil {
		t.Fatalf("a freshly imported hackathon must not have an active rubric")
	}
	_ = importedEvent
}

func TestImportRejectsBrokenBundle(t *testing.T) {
	server, tokens, data := newTestServer(t)
	response := request(t, server, http.MethodPost, "/v1/organizer/import", tokenFor(t, tokens, data, "organizer"), map[string]any{
		"bundle": map[string]any{"event": map[string]any{"name": ""}},
	})
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", response.Code, response.Body.String())
	}
	plan := decodeAssignmentsBody(t, response)["plan"].(map[string]any)
	if len(plan["errors"].([]any)) == 0 {
		t.Fatalf("expected validation errors: %s", response.Body.String())
	}
	forbidden := request(t, server, http.MethodPost, "/v1/organizer/import", tokenFor(t, tokens, data, "participant"), map[string]any{
		"bundle": map[string]any{"event": map[string]any{"name": "x", "slug": "x"}},
	})
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", forbidden.Code, forbidden.Body.String())
	}
	noExport := request(t, server, http.MethodGet, "/v1/organizer/export?event_id=evt_01", tokenFor(t, tokens, data, "participant"), nil)
	if noExport.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", noExport.Code, noExport.Body.String())
	}
}

func TestRouteCatalogAndOpenAPI(t *testing.T) {
	server, _, _ := newTestServer(t)
	catalog := request(t, server, http.MethodGet, "/v1/endpoints", "", nil)
	if catalog.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", catalog.Code, catalog.Body.String())
	}
	payload := decodeAssignmentsBody(t, catalog)
	routes := payload["data"].([]any)
	if len(routes) < 60 {
		t.Fatalf("expected a comprehensive route catalog, got %d", len(routes))
	}
	groups := payload["by_group"].(map[string]any)
	for _, expected := range []string{"account", "events", "organizer", "judge", "community", "mail", "activity"} {
		if groups[expected] == nil {
			t.Fatalf("expected a %q group in the catalog: %s", expected, catalog.Body.String())
		}
	}
	found := false
	for _, item := range routes {
		entry := item.(map[string]any)
		if entry["pattern"] == "/v1/events/{slug}/submissions" && entry["method"] == "POST" {
			found = true
			if entry["auth"] == "none" {
				t.Fatalf("authenticated routes must not be marked public: %v", entry)
			}
		}
	}
	if !found {
		t.Fatalf("expected the submission route in the catalog")
	}

	spec := request(t, server, http.MethodGet, "/v1/openapi.json", "", nil)
	if spec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", spec.Code, spec.Body.String())
	}
	var document map[string]any
	if err := json.Unmarshal(spec.Body.Bytes(), &document); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if document["openapi"] != "3.1.0" {
		t.Fatalf("unexpected openapi version %v", document["openapi"])
	}
	paths := document["paths"].(map[string]any)
	if len(paths) < 50 {
		t.Fatalf("expected the spec to describe the api, got %d paths", len(paths))
	}
	submissions, ok := paths["/v1/events/{slug}/submissions"].(map[string]any)
	if !ok {
		t.Fatalf("expected submissions path in the spec")
	}
	post := submissions["post"].(map[string]any)
	if post["x-auth"] == "none" {
		t.Fatalf("expected the submissions POST to require a session")
	}
	if len(post["parameters"].([]any)) != 1 {
		t.Fatalf("expected the slug path parameter, got %v", post["parameters"])
	}
	components := document["components"].(map[string]any)
	if components["securitySchemes"] == nil || components["schemas"] == nil {
		t.Fatalf("expected components in the spec")
	}
}

type recordingTransport struct {
	mu       sync.Mutex
	requests []*http.Request
	bodies   []string
	status   int
}

func (t *recordingTransport) Do(request *http.Request) (int, string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	body, _ := io.ReadAll(request.Body)
	t.requests = append(t.requests, request.Clone(request.Context()))
	t.bodies = append(t.bodies, string(body))
	if t.status == 0 {
		return 200, "ok", nil
	}
	return t.status, "rejected", nil
}

func TestWebhookCreationValidationAndDelivery(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")

	invalidScheme := request(t, server, http.MethodPost, "/v1/organizer/events/sample-hack-2026/webhooks", organizer, map[string]any{
		"url":    "http://example.org/hook",
		"secret": "a-long-enough-secret",
		"events": []string{"submission.created"},
	})
	if invalidScheme.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", invalidScheme.Code, invalidScheme.Body.String())
	}
	shortSecret := request(t, server, http.MethodPost, "/v1/organizer/events/sample-hack-2026/webhooks", organizer, map[string]any{
		"url":    "https://example.org/hook",
		"secret": "short",
		"events": []string{"submission.created"},
	})
	if shortSecret.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", shortSecret.Code, shortSecret.Body.String())
	}
	unknownEvent := request(t, server, http.MethodPost, "/v1/organizer/events/sample-hack-2026/webhooks", organizer, map[string]any{
		"url":    "https://example.org/hook",
		"secret": "a-long-enough-secret",
		"events": []string{"something.else"},
	})
	if unknownEvent.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", unknownEvent.Code, unknownEvent.Body.String())
	}

	created := request(t, server, http.MethodPost, "/v1/organizer/events/sample-hack-2026/webhooks", organizer, map[string]any{
		"url":    "https://example.org/hook",
		"secret": "a-long-enough-secret",
		"events": []string{"submission.created", "results.published"},
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", created.Code, created.Body.String())
	}
	webhookID := decodeAssignmentsBody(t, created)["data"].(map[string]any)["id"].(string)
	if containsProjectTitle(created.Body.String(), "a-long-enough-secret") {
		t.Fatalf("the signing secret must never be returned: %s", created.Body.String())
	}

	// Webhook destination URLs and delivery counts describe the organizer's
	// infrastructure, so the list is not public. An anonymous caller is
	// refused rather than shown a redacted view.
	anonymous := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/webhooks", "", nil)
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for an anonymous webhook listing, body = %s", anonymous.Code, anonymous.Body.String())
	}
	listed := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/webhooks", organizer, nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", listed.Code, listed.Body.String())
	}
	if !containsProjectTitle(listed.Body.String(), "secret_set") {
		t.Fatalf("expected the organizer webhook view: %s", listed.Body.String())
	}
	if !containsProjectTitle(listed.Body.String(), "results.published") {
		t.Fatalf("expected the subscribed event list: %s", listed.Body.String())
	}

	participant := tokenFor(t, tokens, data, "participant")
	forbidden := request(t, server, http.MethodPost, "/v1/organizer/events/sample-hack-2026/webhooks", participant, map[string]any{
		"url":    "https://example.org/hook",
		"secret": "a-long-enough-secret",
		"events": []string{"submission.created"},
	})
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", forbidden.Code, forbidden.Body.String())
	}
	webhook, err := data.WebhookByID(webhookID)
	if err != nil {
		t.Fatalf("WebhookByID() error = %v", err)
	}
	if webhook.Secret != "a-long-enough-secret" {
		t.Fatalf("the signing secret should be stored for signing")
	}
	if !webhook.Active {
		t.Fatalf("expected a new webhook to be active")
	}
	if removed := request(t, server, http.MethodDelete, "/v1/organizer/webhooks/"+webhookID, organizer, nil); removed.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", removed.Code, removed.Body.String())
	}
	deactivated, err := data.WebhookByID(webhookID)
	if err != nil {
		t.Fatalf("WebhookByID() error = %v", err)
	}
	if deactivated.Active {
		t.Fatalf("expected the webhook to be deactivated")
	}
}

func TestWebhookEmissionAndSignature(t *testing.T) {
	server, tokens, data := newTestServer(t)
	transport := &recordingTransport{}
	server.Webhooks().SetTransportForTest(transport)
	organizer := tokenFor(t, tokens, data, "organizer")
	open := openHackathon(t, server, tokens, data)
	created := request(t, server, http.MethodPost, "/v1/organizer/events/"+open.slug+"/webhooks", organizer, map[string]any{
		"url":    "https://example.org/hook",
		"secret": "a-very-long-signing-secret",
		"events": []string{"submission.created", "results.published"},
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", created.Code, created.Body.String())
	}
	webhookID := decodeAssignmentsBody(t, created)["data"].(map[string]any)["id"].(string)
	submitFor(t, server, open, data)
	delivered, err := server.Webhooks().DeliverOnce(10)
	if err != nil {
		t.Fatalf("DeliverOnce() error = %v", err)
	}
	if delivered != 1 {
		t.Fatalf("delivered = %d, want 1", delivered)
	}
	if len(transport.requests) != 1 {
		t.Fatalf("expected one request, got %d", len(transport.requests))
	}
	sent := transport.requests[0]
	if sent.Header.Get("X-CodeCeremony-Event") != "submission.created" {
		t.Fatalf("unexpected event header %q", sent.Header.Get("X-CodeCeremony-Event"))
	}
	if sent.Header.Get("X-CodeCeremony-Delivery") == "" || sent.Header.Get("X-CodeCeremony-Timestamp") == "" {
		t.Fatalf("expected delivery and timestamp headers")
	}
	body := transport.bodies[0]
	if err := domain.VerifySignature("a-very-long-signing-secret", sent.Header.Get("X-CodeCeremony-Signature"), []byte(body), time.Minute); err != nil {
		t.Fatalf("VerifySignature() error = %v", err)
	}
	if err := domain.VerifySignature("wrong-secret-value", sent.Header.Get("X-CodeCeremony-Signature"), []byte(body), time.Minute); err == nil {
		t.Fatalf("expected verification to fail with the wrong secret")
	}
	if !containsProjectTitle(body, "submission_id") {
		t.Fatalf("expected the payload to carry the submission: %s", body)
	}
	_ = webhookID

	history := request(t, server, http.MethodGet, "/v1/organizer/webhooks/deliveries?webhook_id="+webhookID, tokenFor(t, tokens, data, "admin"), nil)
	if history.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", history.Code, history.Body.String())
	}
	if !containsProjectTitle(history.Body.String(), "delivered") {
		t.Fatalf("expected a delivered record: %s", history.Body.String())
	}
	_ = open
}

// openHackathon creates a hackathon with submissions open, a track, and a team
// so a test can submit afterwards.
func openHackathon(t *testing.T, server *Server, tokens *auth.Manager, data *store.Store) openEvent {
	t.Helper()
	openSubmissionWindow(t, server)
	organizer := tokenFor(t, tokens, data, "organizer")
	created := request(t, server, http.MethodPost, "/v1/events", organizer, map[string]any{
		"slug":              "webhook-hack-2026",
		"name":              "Webhook Hack 2026",
		"registration_open": true,
		"submissions_open":  true,
		"submissions_close": "2026-08-01T18:00:00Z",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("event create status = %d, body = %s", created.Code, created.Body.String())
	}
	track := request(t, server, http.MethodPost, "/v1/events/webhook-hack-2026/tracks", organizer, map[string]any{"name": "General"})
	trackID := decodeAssignmentsBody(t, track)["data"].(map[string]any)["id"].(string)
	captain := tokenFor(t, tokens, data, "participant")
	team := request(t, server, http.MethodPost, "/v1/events/webhook-hack-2026/teams", captain, map[string]any{"name": "Hook Crew"})
	teamID := decodeAssignmentsBody(t, team)["data"].(map[string]any)["id"].(string)
	event := decodeAssignmentsBody(t, created)["data"].(map[string]any)
	return openEvent{
		slug:      "webhook-hack-2026",
		eventID:   event["id"].(string),
		trackID:   trackID,
		teamID:    teamID,
		organizer: organizer,
		captain:   captain,
	}
}

func submitFor(t *testing.T, server *Server, open openEvent, data *store.Store) {
	t.Helper()
	response := request(t, server, http.MethodPost, "/v1/events/"+open.slug+"/submissions", open.captain, map[string]any{
		"team_id":  open.teamID,
		"track_id": open.trackID,
		"title":    "Hooked",
		"summary":  "A project created to exercise webhooks.",
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("submission status = %d, body = %s", response.Code, response.Body.String())
	}
	open.projectID = decodeAssignmentsBody(t, response)["data"].(map[string]any)["id"].(string)
	_ = data
}

func TestWebhookFailedDeliveryIsRetried(t *testing.T) {
	server, tokens, data := newTestServer(t)
	transport := &recordingTransport{status: 500}
	server.Webhooks().SetTransportForTest(transport)
	server.Webhooks().SetRetryPolicyForTest(2, time.Millisecond)
	organizer := tokenFor(t, tokens, data, "organizer")
	open := openHackathon(t, server, tokens, data)
	if created := request(t, server, http.MethodPost, "/v1/organizer/events/"+open.slug+"/webhooks", organizer, map[string]any{
		"url":    "https://example.org/hook",
		"secret": "a-very-long-signing-secret",
		"events": []string{"submission.created"},
	}); created.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", created.Code, created.Body.String())
	}
	submitFor(t, server, open, data)

	for attempt := 0; attempt < 2; attempt++ {
		time.Sleep(4 * time.Millisecond)
		if _, err := server.Webhooks().DeliverOnce(10); err != nil {
			t.Fatalf("DeliverOnce() error = %v", err)
		}
	}
	deliveries := data.Deliveries("", 0)
	if len(deliveries) == 0 {
		t.Fatalf("expected a delivery record")
	}
	final := deliveries[0]
	if final.Status != domain.DeliveryFailed || final.LastError == "" {
		t.Fatalf("expected the delivery to be marked failed, got %+v", final)
	}
	if final.ResponseCode != 500 {
		t.Fatalf("response code = %d, want 500", final.ResponseCode)
	}
	webhook, err := data.WebhookByID(final.WebhookID)
	if err != nil {
		t.Fatalf("WebhookByID() error = %v", err)
	}
	if webhook.FailureCount == 0 {
		t.Fatalf("expected the webhook failure count to increase")
	}
}

func TestWebhookManualTestAndFlush(t *testing.T) {
	server, tokens, data := newTestServer(t)
	transport := &recordingTransport{}
	server.Webhooks().SetTransportForTest(transport)
	organizer := tokenFor(t, tokens, data, "organizer")
	created := request(t, server, http.MethodPost, "/v1/organizer/events/sample-hack-2026/webhooks", organizer, map[string]any{
		"url":    "https://example.org/hook",
		"secret": "a-very-long-signing-secret",
		"events": []string{"submission.created"},
	})
	webhookID := decodeAssignmentsBody(t, created)["data"].(map[string]any)["id"].(string)
	tested := request(t, server, http.MethodPost, "/v1/organizer/webhooks/"+webhookID+"/test", organizer, nil)
	if tested.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", tested.Code, tested.Body.String())
	}
	if decodeAssignmentsBody(t, tested)["delivered"].(float64) != 1 {
		t.Fatalf("expected the test delivery to succeed: %s", tested.Body.String())
	}
	flushed := request(t, server, http.MethodPost, "/v1/organizer/webhooks/flush", organizer, nil)
	if flushed.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", flushed.Code, flushed.Body.String())
	}
	deactivated := request(t, server, http.MethodDelete, "/v1/organizer/webhooks/"+webhookID, organizer, nil)
	if deactivated.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", deactivated.Code, deactivated.Body.String())
	}
	after := request(t, server, http.MethodPost, "/v1/organizer/webhooks/"+webhookID+"/test", organizer, nil)
	if decodeAssignmentsBody(t, after)["queued"].(float64) != 0 {
		t.Fatalf("a deactivated webhook must not queue deliveries: %s", after.Body.String())
	}
}

func TestWebhookSignatureRejectsStaleAndMalformed(t *testing.T) {
	body := []byte(`{"event":"submission.created"}`)
	signature := domain.Sign("secret-secret-secret", time.Now(), body)
	if err := domain.VerifySignature("secret-secret-secret", signature, body, time.Minute); err != nil {
		t.Fatalf("VerifySignature() error = %v", err)
	}
	stale := domain.Sign("secret-secret-secret", time.Now().Add(-2*time.Hour), body)
	if err := domain.VerifySignature("secret-secret-secret", stale, body, time.Minute); err == nil {
		t.Fatalf("expected a stale signature to be rejected")
	}
	if err := domain.VerifySignature("secret-secret-secret", "nonsense", body, time.Minute); err == nil {
		t.Fatalf("expected a malformed header to be rejected")
	}
	if err := domain.VerifySignature("secret-secret-secret", "t=abc,v1=def", body, time.Minute); err == nil {
		t.Fatalf("expected a malformed timestamp to be rejected")
	}
}

var _ = mailer.NewHTTPHookTransport
var _ = httptest.NewServer
