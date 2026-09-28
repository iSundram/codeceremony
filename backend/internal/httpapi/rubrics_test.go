package httpapi

import (
	"net/http"
	"testing"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

func rubricPayload(name string, criteria []domain.RubricCriterion) map[string]any {
	return map[string]any{"event_id": "evt_01", "name": name, "criteria": criteria}
}

func standardCriteria() []domain.RubricCriterion {
	return []domain.RubricCriterion{
		{Key: "functionality", Label: "Functionality", MinScore: 1, MaxScore: 5, Weight: 50, Required: true},
		{Key: "impact", Label: "Impact", MinScore: 1, MaxScore: 10, Weight: 50, Required: true},
	}
}

func TestRubricRequiresWeightsTotallingOneHundred(t *testing.T) {
	server, tokens, data := newTestServer(t)
	criteria := standardCriteria()
	criteria[1].Weight = 40
	response := request(t, server, http.MethodPost, "/v1/organizer/rubrics", tokenFor(t, tokens, data, "organizer"), rubricPayload("Broken weights", criteria))
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", response.Code, response.Body.String())
	}
}

func TestRubricVersionSupersedesActiveRubric(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	created := request(t, server, http.MethodPost, "/v1/organizer/rubrics", organizer, rubricPayload("Second edition", standardCriteria()))
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", created.Code, created.Body.String())
	}
	rubricID := decodeAssignmentsBody(t, created)["data"].(map[string]any)["id"].(string)

	beforePublish := request(t, server, http.MethodGet, "/v1/judging/rubric?event_id=evt_01", "", nil)
	payload := decodeAssignmentsBody(t, beforePublish)
	if payload["data"].(map[string]any)["id"] != "rub_01" {
		t.Fatalf("expected the seeded rubric to stay active, body = %s", beforePublish.Body.String())
	}

	published := request(t, server, http.MethodPost, "/v1/organizer/rubrics/"+rubricID+"/publish", organizer, nil)
	if published.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", published.Code, published.Body.String())
	}
	afterPublish := request(t, server, http.MethodGet, "/v1/judging/rubric?event_id=evt_01", "", nil)
	active := decodeAssignmentsBody(t, afterPublish)["data"].(map[string]any)
	if active["id"] != rubricID {
		t.Fatalf("expected the newer published rubric to be active, body = %s", afterPublish.Body.String())
	}
	if active["version"].(float64) != 2 {
		t.Fatalf("version = %v, want 2", active["version"])
	}
}

func TestPublishedRubricCannotBeEdited(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	response := request(t, server, http.MethodPut, "/v1/organizer/rubrics/rub_01", organizer, rubricPayload("Renamed", standardCriteria()))
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", response.Code, response.Body.String())
	}
}

func TestTrackScopedRubricBeatsEventRubric(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	criteria := standardCriteria()
	criteria[0].Weight = 60
	criteria[1].Weight = 40
	created := request(t, server, http.MethodPost, "/v1/organizer/rubrics", organizer, map[string]any{
		"event_id": "evt_01",
		"track_id": "trk_02",
		"name":     "Track two rubric",
		"criteria": criteria,
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", created.Code, created.Body.String())
	}
	rubricID := decodeAssignmentsBody(t, created)["data"].(map[string]any)["id"].(string)
	if published := request(t, server, http.MethodPost, "/v1/organizer/rubrics/"+rubricID+"/publish", organizer, nil); published.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", published.Code, published.Body.String())
	}

	scoped := request(t, server, http.MethodGet, "/v1/judging/rubric?event_id=evt_01&track_id=trk_02", "", nil)
	if scoped.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", scoped.Code, scoped.Body.String())
	}
	if decodeAssignmentsBody(t, scoped)["data"].(map[string]any)["id"] != rubricID {
		t.Fatalf("expected the track rubric to be used, body = %s", scoped.Body.String())
	}

	fallback := request(t, server, http.MethodGet, "/v1/judging/rubric?event_id=evt_01&track_id=trk_01", "", nil)
	if decodeAssignmentsBody(t, fallback)["data"].(map[string]any)["id"] != "rub_01" {
		t.Fatalf("expected the event rubric to remain active for trk_01, body = %s", fallback.Body.String())
	}
}

func TestReviewValidatesAgainstActiveRubricRange(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	created := request(t, server, http.MethodPost, "/v1/organizer/rubrics", organizer, rubricPayload("Ten point rubric", standardCriteria()))
	rubricID := decodeAssignmentsBody(t, created)["data"].(map[string]any)["id"].(string)
	if published := request(t, server, http.MethodPost, "/v1/organizer/rubrics/"+rubricID+"/publish", organizer, nil); published.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", published.Code, published.Body.String())
	}
	assignments := request(t, server, http.MethodPost, "/v1/organizer/assignments", organizer, map[string]any{
		"event_slug":          "sample-hack-2026",
		"reviews_per_project": 1,
	})
	created0 := decodeAssignmentsBody(t, assignments)["created"].([]any)
	projectID := created0[0].(map[string]any)["project_id"].(string)
	judgeID := created0[0].(map[string]any)["judge_id"].(string)

	rejected := request(t, server, http.MethodPut, "/v1/judge/projects/"+projectID+"/review", tokenFor(t, tokens, data, judgeID), map[string]any{
		"event_id": "evt_01",
		"criteria": map[string]int{"functionality": 4, "impact": 42},
	})
	if rejected.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", rejected.Code, rejected.Body.String())
	}

	unknown := request(t, server, http.MethodPut, "/v1/judge/projects/"+projectID+"/review", tokenFor(t, tokens, data, judgeID), map[string]any{
		"event_id": "evt_01",
		"criteria": map[string]int{"functionality": 4, "impact": 8, "innovation": 5},
	})
	if unknown.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", unknown.Code, unknown.Body.String())
	}

	accepted := request(t, server, http.MethodPut, "/v1/judge/projects/"+projectID+"/review", tokenFor(t, tokens, data, judgeID), map[string]any{
		"event_id":  "evt_01",
		"criteria":  map[string]int{"functionality": 4, "impact": 8},
		"submitted": true,
	})
	if accepted.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", accepted.Code, accepted.Body.String())
	}
	review := decodeAssignmentsBody(t, accepted)["data"].(map[string]any)
	if review["rubric_id"] != rubricID {
		t.Fatalf("expected the review to record the rubric, body = %s", accepted.Body.String())
	}
	normalized, ok := review["normalized"].(map[string]any)
	if !ok {
		t.Fatalf("expected normalized scores, body = %s", accepted.Body.String())
	}
	if normalized["functionality"].(float64) != 75 {
		t.Fatalf("normalized functionality = %v, want 75", normalized["functionality"])
	}
	if normalized["impact"].(float64) != 77 {
		t.Fatalf("normalized impact = %v, want 77", normalized["impact"])
	}
}

func TestSubmittedReviewIsLocked(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	assignments := request(t, server, http.MethodPost, "/v1/organizer/assignments", organizer, map[string]any{
		"event_slug":          "sample-hack-2026",
		"reviews_per_project": 1,
	})
	first := decodeAssignmentsBody(t, assignments)["created"].([]any)[0].(map[string]any)
	projectID := first["project_id"].(string)
	judgeID := first["judge_id"].(string)
	judge := tokenFor(t, tokens, data, judgeID)

	submitted := request(t, server, http.MethodPut, "/v1/judge/projects/"+projectID+"/review", judge, map[string]any{
		"event_id":  "evt_01",
		"criteria":  map[string]int{"functionality": 4, "quality": 3, "innovation": 5},
		"submitted": true,
	})
	if submitted.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", submitted.Code, submitted.Body.String())
	}

	changed := request(t, server, http.MethodPut, "/v1/judge/projects/"+projectID+"/review", judge, map[string]any{
		"event_id":  "evt_01",
		"criteria":  map[string]int{"functionality": 1, "quality": 1, "innovation": 1},
		"submitted": true,
	})
	if changed.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", changed.Code, changed.Body.String())
	}
	unchanged := request(t, server, http.MethodPut, "/v1/judge/projects/"+projectID+"/review", judge, map[string]any{
		"event_id":  "evt_01",
		"criteria":  map[string]int{"functionality": 4, "quality": 3, "innovation": 5},
		"submitted": true,
	})
	if unchanged.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for an idempotent resubmit, body = %s", unchanged.Code, unchanged.Body.String())
	}
}

func TestResultsUsePublishedRubricWeights(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	criteria := standardCriteria()
	criteria[0].Weight = 100
	criteria[1].Weight = 0 + 1
	criteria[1].Weight = 1
	created := request(t, server, http.MethodPost, "/v1/organizer/rubrics", organizer, map[string]any{
		"event_id": "evt_01",
		"name":     "Functionality only",
		"criteria": []domain.RubricCriterion{{Key: "functionality", Label: "Functionality", MinScore: 1, MaxScore: 5, Weight: 100, Required: true}},
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", created.Code, created.Body.String())
	}
	rubricID := decodeAssignmentsBody(t, created)["data"].(map[string]any)["id"].(string)
	if published := request(t, server, http.MethodPost, "/v1/organizer/rubrics/"+rubricID+"/publish", organizer, nil); published.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", published.Code, published.Body.String())
	}
	response := request(t, server, http.MethodGet, "/v1/organizer/results?event_id=evt_01", organizer, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", response.Code, response.Body.String())
	}
	payload := decodeAssignmentsBody(t, response)
	rubric := payload["rubric"].(map[string]any)
	if rubric["id"] != rubricID || rubric["weight_source"] != "published_rubric" {
		t.Fatalf("expected results to report the published rubric, body = %s", response.Body.String())
	}
}

func TestArchivedRubricIsNotActive(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	if archived := request(t, server, http.MethodPost, "/v1/organizer/rubrics/rub_01/archive", organizer, nil); archived.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", archived.Code, archived.Body.String())
	}
	response := request(t, server, http.MethodGet, "/v1/judging/rubric?event_id=evt_01", "", nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", response.Code, response.Body.String())
	}
}

func TestParticipantCannotManageRubrics(t *testing.T) {
	server, tokens, data := newTestServer(t)
	response := request(t, server, http.MethodPost, "/v1/organizer/rubrics", tokenFor(t, tokens, data, "participant"), rubricPayload("Not allowed", standardCriteria()))
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", response.Code, response.Body.String())
	}
}
