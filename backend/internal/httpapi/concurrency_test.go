package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

// openSubmissions reopens the fixture event, which the seed ships closed.
//
// The closed state is correct for the demo it was written for, but it would make
// every write in this file a 422, and a test that cannot reach the write it is
// testing is not testing anything.
func openSubmissions(t *testing.T, data *store.Store) {
	t.Helper()
	_, err := data.UpdateEvent("evt_01", func(event domain.Event) (domain.Event, error) {
		event.SubmissionsOpen = true
		event.SubmissionsClose = time.Now().UTC().Add(24 * time.Hour)
		return event, nil
	})
	if err != nil {
		t.Fatalf("UpdateEvent() error = %v", err)
	}
}

// assignJudge gives a judge a fresh, unreviewed project.
//
// Only prj_01 is assigned in the seed, and judge_a's review of it is already
// submitted, so writing to it again is refused as locked. A review that cannot be
// written cannot carry an ETag.
func assignJudge(t *testing.T, data *store.Store, judgeID, projectID string) {
	t.Helper()
	_, err := data.CreateAssignment(domain.Assignment{
		ID: "asg_" + judgeID + "_" + projectID, EventID: "evt_01",
		JudgeID: judgeID, ProjectID: projectID, AssignedBy: "organizer",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateAssignment() error = %v", err)
	}
}

// editableFixture reopens submissions and assigns the judge an unreviewed
// project, returning the project id to write against.
func editableFixture(t *testing.T, data *store.Store, judgeID string) string {
	t.Helper()
	openSubmissions(t, data)
	const project = "prj_02"
	assignJudge(t, data, judgeID, project)
	return project
}

func TestETagIsServedOnASubmission(t *testing.T) {
	server, tokens, data := newTestServer(t)
	captain := tokenFor(t, tokens, data, "participant")
	openSubmissions(t, data)

	got := request(t, server, http.MethodGet, "/v1/submissions/prj_02", captain, nil)
	if got.Code != http.StatusOK {
		t.Fatalf("read submission status = %d, body = %s", got.Code, got.Body.String())
	}
	if etag := got.Header().Get("ETag"); etag == "" {
		t.Error("a submission read served no ETag, so a client cannot write a precondition")
	}
}

func TestIfMatchRefusesAStaleSubmissionEdit(t *testing.T) {
	server, tokens, data := newTestServer(t)
	captain := tokenFor(t, tokens, data, "participant")
	openSubmissions(t, data)

	read := request(t, server, http.MethodGet, "/v1/submissions/prj_02", captain, nil)
	stale := read.Header().Get("ETag")
	if stale == "" {
		t.Fatal("no ETag to test against")
	}

	// Someone else advances the submission.
	first := request(t, server, http.MethodPatch, "/v1/submissions/prj_02", captain,
		map[string]any{"title": "A better title", "reason": "clarity"})
	if first.Code != http.StatusOK {
		t.Fatalf("first edit status = %d, body = %s", first.Code, first.Body.String())
	}
	fresh := first.Header().Get("ETag")
	if fresh == "" || fresh == stale {
		t.Fatalf("the ETag did not change after a write: %q then %q", stale, fresh)
	}

	// The stale tab now tries to write what it read. It must be refused.
	conflict := requestWithHeader(t, server, http.MethodPatch, "/v1/submissions/prj_02", captain,
		map[string]any{"title": "A title from a stale tab", "reason": "conflicting"}, "If-Match", stale)
	if conflict.Code != http.StatusPreconditionFailed {
		t.Fatalf("a stale edit status = %d, want 412, body = %s", conflict.Code, conflict.Body.String())
	}

	// The refusal has to tell the client what to merge against, or it has to
	// spend a second round trip finding that out.
	if got := conflict.Header().Get("ETag"); got != fresh {
		t.Errorf("the 412 carried ETag %q, want the current %q", got, fresh)
	}
	if code := decode(t, conflict.Body.Bytes())["error"]; code == nil {
		t.Error("the 412 carried no error body")
	}

	// The stale write must not have landed.
	after := request(t, server, http.MethodGet, "/v1/submissions/prj_02", captain, nil)
	if body := string(after.Body.Bytes()); strings.Contains(body, "A title from a stale tab") {
		t.Error("the refused stale edit was applied anyway")
	}
}

// A client that read the current version must be able to write. Otherwise the
// check is not a check, it is an outage.
func TestIfMatchAcceptsTheCurrentVersion(t *testing.T) {
	server, tokens, data := newTestServer(t)
	captain := tokenFor(t, tokens, data, "participant")
	openSubmissions(t, data)

	read := request(t, server, http.MethodGet, "/v1/submissions/prj_02", captain, nil)
	current := read.Header().Get("ETag")

	ok := requestWithHeader(t, server, http.MethodPatch, "/v1/submissions/prj_02", captain,
		map[string]any{"title": "A current edit", "reason": "clarity"}, "If-Match", current)
	if ok.Code != http.StatusOK {
		t.Fatalf("an edit at the current version status = %d, want 200, body = %s", ok.Code, ok.Body.String())
	}
	if body := string(ok.Body.Bytes()); !strings.Contains(body, "A current edit") {
		t.Error("the accepted edit did not apply")
	}
}

// No header means no precondition, because requiring one would break every
// client that does not track versions.
func TestAnUnconditionalEditIsStillAllowed(t *testing.T) {
	server, tokens, data := newTestServer(t)
	captain := tokenFor(t, tokens, data, "participant")
	openSubmissions(t, data)

	got := request(t, server, http.MethodPatch, "/v1/submissions/prj_02", captain,
		map[string]any{"title": "No precondition", "reason": "clarity"})
	if got.Code != http.StatusOK {
		t.Fatalf("an unconditional edit status = %d, want 200, body = %s", got.Code, got.Body.String())
	}
}

func TestIfMatchAcceptsWildcardAndLists(t *testing.T) {
	server, tokens, data := newTestServer(t)
	captain := tokenFor(t, tokens, data, "participant")
	openSubmissions(t, data)

	for _, form := range []string{"*", "current", "list", "weak"} {
		// Re-read each time: the previous iteration's write moved the version,
		// so one read cannot supply a fresh precondition for all of them.
		read := request(t, server, http.MethodGet, "/v1/submissions/prj_02", captain, nil)
		current := read.Header().Get("ETag")
		header := map[string]string{"*": "*", "current": current, "list": `"v999", ` + current, "weak": `W/` + current}[form]
		got := requestWithHeader(t, server, http.MethodPatch, "/v1/submissions/prj_02", captain,
			map[string]any{"title": "Wildcard edit " + form, "reason": "clarity"}, "If-Match", header)
		if got.Code != http.StatusOK {
			t.Errorf("If-Match %s (%q) status = %d, want 200, body = %s", form, header, got.Code, got.Body.String())
		}
	}
}

// A submitted review freezes the work. A judge whose tab is behind must not be
// able to submit a revision they never saw.
func TestIfMatchRefusesAStaleReviewSave(t *testing.T) {
	server, tokens, data := newTestServer(t)
	judge := tokenFor(t, tokens, data, "judge_a")
	project := editableFixture(t, data, "judge_a")
	reviewPath := "/v1/judge/projects/" + project + "/review"
	scores := map[string]any{"functionality": 4, "quality": 3, "innovation": 5}

	first := request(t, server, http.MethodPut, reviewPath, judge,
		map[string]any{"event_id": "evt_01", "criteria": scores, "comment": "first pass"})
	if first.Code != http.StatusOK {
		t.Fatalf("first save status = %d, body = %s", first.Code, first.Body.String())
	}
	stale := first.Header().Get("ETag")
	if stale == "" {
		t.Fatal("a review save served no ETag")
	}

	// The judge keeps working in another tab.
	second := request(t, server, http.MethodPut, reviewPath, judge,
		map[string]any{"event_id": "evt_01", "criteria": map[string]any{"functionality": 5, "quality": 5, "innovation": 5}, "comment": "second pass"})
	if second.Code != http.StatusOK {
		t.Fatalf("second save status = %d, body = %s", second.Code, second.Body.String())
	}
	if fresh := second.Header().Get("ETag"); fresh == stale {
		t.Fatalf("the review ETag did not change between two different saves: %q", fresh)
	}

	// The first tab submits on top of what it read.
	conflict := requestWithHeader(t, server, http.MethodPut, reviewPath, judge,
		map[string]any{"event_id": "evt_01", "criteria": scores, "comment": "stale submit", "submitted": true},
		"If-Match", stale)
	if conflict.Code != http.StatusPreconditionFailed {
		t.Fatalf("a stale review save status = %d, want 412, body = %s", conflict.Code, conflict.Body.String())
	}
	// The submitted flag is the part that must not have taken effect. Scoped to
	// the project under test: the judge also has a seeded, already-submitted
	// review in this event, which would mask the result either way.
	after := request(t, server, http.MethodGet, "/v1/judge/scores?event_id=evt_01", judge, nil)
	if reviewSubmittedFor(string(after.Body.Bytes()), project) {
		t.Error("the refused stale review was submitted anyway")
	}
	if !strings.Contains(string(after.Body.Bytes()), "second pass") {
		t.Error("the accepted second save did not land, so the test proved nothing")
	}
}

// reviewSubmittedFor reports whether the scores listing has a submitted review
// for one project. The listing is a JSON array of review objects.
func reviewSubmittedFor(body, projectID string) bool {
	var reviews []struct {
		ProjectID string `json:"project_id"`
		Submitted bool   `json:"submitted"`
	}
	if err := json.Unmarshal([]byte(body), &reviews); err != nil {
		return false
	}
	for _, review := range reviews {
		if review.ProjectID == projectID {
			return review.Submitted
		}
	}
	return false
}

// The ETag has to be stable for unchanged content. An ETag that varies between
// two identical reads would reject a client's own retry, which is the opposite of
// what it is for.
func TestETagIsStableAcrossUnchangedReads(t *testing.T) {
	server, tokens, data := newTestServer(t)
	judge := tokenFor(t, tokens, data, "judge_a")
	reviewPath := "/v1/judge/projects/" + editableFixture(t, data, "judge_a") + "/review"

	saved := request(t, server, http.MethodPut, reviewPath, judge,
		map[string]any{"event_id": "evt_01", "criteria": map[string]any{"functionality": 4, "quality": 3, "innovation": 5}})
	if saved.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", saved.Code, saved.Body.String())
	}
	want := saved.Header().Get("ETag")

	// Several reads, and a save of identical scores, must all agree.
	for i := 0; i < 5; i++ {
		got := request(t, server, http.MethodGet, reviewPath, judge, nil)
		if etag := got.Header().Get("ETag"); etag != "" && etag != want {
			t.Fatalf("read %d served ETag %q, want the stable %q", i, etag, want)
		}
	}
	again := request(t, server, http.MethodPut, reviewPath, judge,
		map[string]any{"event_id": "evt_01", "criteria": map[string]any{"functionality": 4, "quality": 3, "innovation": 5}})
	if etag := again.Header().Get("ETag"); etag != want {
		t.Errorf("saving identical scores changed the ETag: %q then %q", want, etag)
	}
	// And a client holding that ETag can still write.
	ok := requestWithHeader(t, server, http.MethodPut, reviewPath, judge,
		map[string]any{"event_id": "evt_01", "criteria": map[string]any{"functionality": 4, "quality": 4, "innovation": 5}, "comment": "adjusted"},
		"If-Match", want)
	if ok.Code != http.StatusOK {
		t.Errorf("a write at the unchanged ETag status = %d, want 200, body = %s", ok.Code, ok.Body.String())
	}
}

func TestIfNoneMatchOnARead(t *testing.T) {
	server, tokens, data := newTestServer(t)
	captain := tokenFor(t, tokens, data, "participant")
	openSubmissions(t, data)

	read := request(t, server, http.MethodGet, "/v1/submissions/prj_02", captain, nil)
	current := read.Header().Get("ETag")
	if current == "" {
		t.Fatal("no ETag to test against")
	}

	unchanged := requestWithHeader(t, server, http.MethodGet, "/v1/submissions/prj_02", captain, nil,
		"If-None-Match", current)
	if unchanged.Code != http.StatusNotModified {
		t.Errorf("If-None-Match on unchanged content status = %d, want 304", unchanged.Code)
	}

	changed := requestWithHeader(t, server, http.MethodGet, "/v1/submissions/prj_02", captain, nil,
		"If-None-Match", `"v999"`)
	if changed.Code != http.StatusOK {
		t.Errorf("If-None-Match on changed content status = %d, want 200", changed.Code)
	}
}

// The header parsing itself, including the weak-tag forms a proxy may add.
func TestETagMatching(t *testing.T) {
	current := versionedVersion(7)
	if got := current.etag(); got != `"v7"` {
		t.Errorf("etag() = %q, want %q", got, `"v7"`)
	}
	cases := []struct {
		header string
		want   bool
	}{
		{`"v7"`, true},
		{`"v8"`, false},
		{`"v8", "v7"`, true},
		{`"v8", "v9"`, false},
		{"*", true},
		{`W/"v7"`, true},
		{`  "v7"  `, true},
		{`"V7"`, false},
		{"", false},
	}
	for _, c := range cases {
		if got := etagMatches(c.header, current); got != c.want {
			t.Errorf("etagMatches(%q) = %v, want %v", c.header, got, c.want)
		}
	}
}
