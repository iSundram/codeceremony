package httpapi

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/judging"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

// minInt bounds a slice of a possibly short body in a failure message.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// comparisonFixture gives a judge two assigned, unreviewed projects to compare.
func comparisonFixture(t *testing.T, data *store.Store, judgeID string) {
	t.Helper()
	for _, projectID := range []string{"prj_02", "prj_04"} {
		if _, err := data.CreateAssignment(domain.Assignment{
			ID: "asg_cmp_" + judgeID + "_" + projectID, EventID: "evt_01",
			JudgeID: judgeID, ProjectID: projectID, AssignedBy: "organizer",
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("CreateAssignment() error = %v", err)
		}
	}
}

func TestAJudgeCanRecordAComparison(t *testing.T) {
	server, tokens, data := newTestServer(t)
	judge := tokenFor(t, tokens, data, "judge_a")
	comparisonFixture(t, data, "judge_a")

	got := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/comparisons", judge,
		map[string]any{"left": "prj_02", "right": "prj_04", "verdict": "left", "comment": "clearer problem statement"})
	if got.Code != http.StatusCreated {
		t.Fatalf("record status = %d, want 201, body = %s", got.Code, got.Body.String())
	}
	body := decode(t, got.Body.Bytes())["data"].(map[string]any)
	if body["verdict"] != "left" {
		t.Errorf("verdict = %v, want left", body["verdict"])
	}
	if body["judge_id"] != "judge_a" {
		t.Errorf("judge_id = %v, want judge_a", body["judge_id"])
	}
	if got.Header().Get("ETag") == "" {
		t.Error("a recorded comparison served no ETag")
	}
}

func TestAComparisonRequiresAnAssignment(t *testing.T) {
	server, tokens, data := newTestServer(t)
	judge := tokenFor(t, tokens, data, "judge_b")
	// judge_b is not assigned to either project here.
	got := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/comparisons", judge,
		map[string]any{"left": "prj_02", "right": "prj_04", "verdict": "left"})
	if got.Code != http.StatusForbidden {
		t.Fatalf("comparing unassigned projects status = %d, want 403, body = %s", got.Code, got.Body.String())
	}
}

func TestAComparisonAgainstItselfIsRefused(t *testing.T) {
	server, tokens, data := newTestServer(t)
	judge := tokenFor(t, tokens, data, "judge_a")
	comparisonFixture(t, data, "judge_a")

	got := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/comparisons", judge,
		map[string]any{"left": "prj_02", "right": "prj_02", "verdict": "left"})
	if got.Code != http.StatusUnprocessableEntity {
		t.Errorf("self comparison status = %d, want 422, body = %s", got.Code, got.Body.String())
	}
}

func TestAnUnknownVerdictIsRefused(t *testing.T) {
	server, tokens, data := newTestServer(t)
	judge := tokenFor(t, tokens, data, "judge_a")
	comparisonFixture(t, data, "judge_a")

	for _, verdict := range []string{"", "better", "maybe", "LEFT"} {
		got := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/comparisons", judge,
			map[string]any{"left": "prj_02", "right": "prj_04", "verdict": verdict})
		if got.Code != http.StatusUnprocessableEntity {
			t.Errorf("verdict %q status = %d, want 422, body = %s", verdict, got.Code, got.Body.String())
		}
	}
}

// assign gives a judge a project, for tests that need the assignment to exist.
func assign(t *testing.T, data *store.Store, judgeID, eventID, projectID string) {
	t.Helper()
	if _, err := data.CreateAssignment(domain.Assignment{
		ID: "asg_x_" + judgeID + "_" + projectID, EventID: eventID, JudgeID: judgeID,
		ProjectID: projectID, AssignedBy: "organizer", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateAssignment(%s, %s) error = %v", eventID, projectID, err)
	}
}

// A judge who changes their mind has answered once, and the later answer counts.
// Recording a second verdict for the same pair must update, not accumulate.
func TestRerecordingAComparisonUpdatesIt(t *testing.T) {
	server, tokens, data := newTestServer(t)
	judge := tokenFor(t, tokens, data, "judge_a")
	comparisonFixture(t, data, "judge_a")

	first := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/comparisons", judge,
		map[string]any{"left": "prj_02", "right": "prj_04", "verdict": "left"})
	if first.Code != http.StatusCreated {
		t.Fatalf("first record status = %d, body = %s", first.Code, first.Body.String())
	}
	firstID := decode(t, first.Body.Bytes())["data"].(map[string]any)["id"].(string)

	// The same pair, the other way round, with the opposite answer.
	second := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/comparisons", judge,
		map[string]any{"left": "prj_04", "right": "prj_02", "verdict": "left"})
	if second.Code != http.StatusCreated {
		t.Fatalf("second record status = %d, body = %s", second.Code, second.Body.String())
	}
	secondData := decode(t, second.Body.Bytes())["data"].(map[string]any)
	if secondData["id"] != firstID {
		t.Errorf("a repeated comparison created a second row: %v then %v", firstID, secondData["id"])
	}

	listed := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/comparisons", judge, nil)
	rows, _ := decode(t, listed.Body.Bytes())["data"].([]any)
	if len(rows) != 1 {
		t.Fatalf("the judge has %d comparisons, want 1", len(rows))
	}
	// Reversing the sides and answering "left" is a change of mind about prj_03.
	row := rows[0].(map[string]any)
	if row["left"] != "prj_04" || row["verdict"] != "left" {
		t.Errorf("the stored verdict is %v/%v, want the reversed one", row["left"], row["verdict"])
	}
}

// A judge may withdraw their own answer, and the record of the withdrawal
// survives so the mistake is still traceable.
func TestAComparisonCanBeWithdrawn(t *testing.T) {
	server, tokens, data := newTestServer(t)
	judge := tokenFor(t, tokens, data, "judge_a")
	comparisonFixture(t, data, "judge_a")

	created := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/comparisons", judge,
		map[string]any{"left": "prj_02", "right": "prj_04", "verdict": "left"})
	if created.Code != http.StatusCreated {
		t.Fatalf("record status = %d, body = %s", created.Code, created.Body.String())
	}
	id := decode(t, created.Body.Bytes())["data"].(map[string]any)["id"].(string)

	organizer := tokenFor(t, tokens, data, "organizer")
	withdrawn := request(t, server, http.MethodDelete, "/v1/organizer/comparisons/"+id, organizer, nil)
	if withdrawn.Code != http.StatusOK {
		t.Fatalf("withdraw status = %d, body = %s", withdrawn.Code, withdrawn.Body.String())
	}

	listed := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/comparisons", judge, nil)
	rows, _ := decode(t, listed.Body.Bytes())["data"].([]any)
	if len(rows) != 0 {
		t.Errorf("the withdrawn comparison is still listed: %v", rows)
	}

	// The audit keeps the withdrawal, with the verdict it withdrew, so the change
	// is visible even though the row is gone.
	audit := request(t, server, http.MethodGet, "/v1/audit/actions?action=comparison.withdrawn&limit=50", organizer, nil)
	body := string(audit.Body.Bytes())
	if !strings.Contains(body, "comparison.withdrawn") {
		t.Errorf("the withdrawal is not in the audit: %s", body[:minInt(400, len(body))])
	}
}

// A judge sees their own verdicts; the organizer sees the panel's.
func TestComparisonVisibilityFollowsTheResultsPermission(t *testing.T) {
	server, tokens, data := newTestServer(t)
	judge := tokenFor(t, tokens, data, "judge_a")
	other := tokenFor(t, tokens, data, "judge_b")
	comparisonFixture(t, data, "judge_a")
	comparisonFixture(t, data, "judge_b")

	request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/comparisons", judge,
		map[string]any{"left": "prj_02", "right": "prj_04", "verdict": "left"})
	request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/comparisons", other,
		map[string]any{"left": "prj_02", "right": "prj_04", "verdict": "right"})

	mine := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/comparisons", judge, nil)
	rows, _ := decode(t, mine.Body.Bytes())["data"].([]any)
	if len(rows) != 1 {
		t.Errorf("a judge sees %d comparisons, want only their own 1", len(rows))
	}
	if rows[0].(map[string]any)["judge_id"] != "judge_a" {
		t.Errorf("a judge sees another judge's verdict: %v", rows[0])
	}

	organizer := tokenFor(t, tokens, data, "organizer")
	all := request(t, server, http.MethodGet, "/v1/organizer/events/sample-hack-2026/comparisons", organizer, nil)
	rows, _ = decode(t, all.Body.Bytes())["data"].([]any)
	if len(rows) != 2 {
		t.Errorf("an organizer sees %d comparisons, want the panel's 2", len(rows))
	}
}

// The point of recording verdicts: when the panel has answered enough of them,
// the ranking is built from those answers and says so.
func TestThePairwiseViewPrefersRecordedVerdicts(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	judge := tokenFor(t, tokens, data, "judge_a")
	comparisonFixture(t, data, "judge_a")

	before := request(t, server, http.MethodGet, "/v1/organizer/pairwise?event_id=evt_01", organizer, nil)
	if before.Code != http.StatusOK {
		t.Fatalf("pairwise status = %d, body = %s", before.Code, before.Body.String())
	}
	source := decode(t, before.Body.Bytes())["data"].(map[string]any)["source"]
	if source != "derived" {
		t.Errorf("with no recorded verdicts the source is %v, want derived", source)
	}

	// Record a complete set for the two projects the seed reviews cover, so the
	// recorded set can stand on its own.
	recorded := 0
	// The derived fit only ranks projects the panel reviewed, and the seed has
	// judge_a reviewing prj_01 alone. Recording verdicts for three projects while
	// the derived fit knows about one would not be a comparison of the two, so the
	// other two need reviews first. Both are comparable: comparisonFixture assigned
	// them, and the seed assigns prj_01.
	for _, projectID := range []string{"prj_02", "prj_04"} {
		saved := request(t, server, http.MethodPut, "/v1/judge/projects/"+projectID+"/review", judge,
			map[string]any{"event_id": "evt_01", "criteria": map[string]any{"functionality": 3, "quality": 3, "innovation": 3}})
		if saved.Code != http.StatusOK {
			t.Fatalf("seed review for %s status = %d, body = %s", projectID, saved.Code, saved.Body.String())
		}
	}

	// A fully decisive round-robin has no maximum likelihood estimate at all, so
	// the model refuses to rank it and reports unbounded instead. These verdicts
	// keep every project both winning and losing, which is what makes a finite
	// estimate exist: judge_a ranks prj_01 above prj_02 above prj_04, and judge_b
	// disagrees about prj_01 against prj_04.
	for _, pair := range [][2]string{{"prj_01", "prj_02"}, {"prj_01", "prj_04"}, {"prj_02", "prj_04"}} {
		got := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/comparisons", judge,
			map[string]any{"left": pair[0], "right": pair[1], "verdict": "left"})
		if got.Code != http.StatusCreated {
			t.Fatalf("record %v status = %d, body = %s", pair, got.Code, got.Body.String())
		}
		recorded++
	}
	// The disagreement has to come from a different judge: one judge has one answer
	// per pair, and re-answering the same pair updates the row rather than adding
	// to it.
	comparisonFixture(t, data, "judge_b")
	disagreement := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/comparisons",
		tokenFor(t, tokens, data, "judge_b"),
		map[string]any{"left": "prj_04", "right": "prj_01", "verdict": "left"})
	if disagreement.Code != http.StatusCreated {
		t.Fatalf("the second judge's verdict status = %d, body = %s", disagreement.Code, disagreement.Body.String())
	}
	recorded++

	after := request(t, server, http.MethodGet, "/v1/organizer/pairwise?event_id=evt_01", organizer, nil)
	result := decode(t, after.Body.Bytes())["data"].(map[string]any)
	if result["source"] != "recorded" {
		t.Errorf("with %d recorded verdicts the source is %v, want recorded", recorded, result["source"])
	}
	if count, _ := result["recorded_comparisons"].(float64); int(count) != recorded {
		t.Errorf("recorded_comparisons = %v, want %d", result["recorded_comparisons"], recorded)
	}
	if count, _ := result["derived_comparisons"].(float64); count != 0 {
		t.Errorf("derived_comparisons = %v, want 0 when the fit used recorded verdicts", result["derived_comparisons"])
	}
	entries, _ := result["entries"].([]any)
	if len(entries) == 0 {
		t.Fatal("a recorded fit produced no entries")
	}
	// The verdicts rank prj_01 above prj_02 above prj_04, and a recorded verdict is
	// the judge answering directly, so the ranking must follow them. Whether the
	// strengths themselves are well determined is the estimator's business and is
	// tested in the judging package, where a panel large enough to judge it can be
	// built.
	want := []string{"prj_01", "prj_02", "prj_04"}
	for i, id := range want {
		entry := entries[i].(map[string]any)
		if entry["project_id"] != id {
			got := []string{}
			for _, e := range entries {
				got = append(got, e.(map[string]any)["project_id"].(string))
			}
			t.Fatalf("ranking is %v, want %v", got, want)
		}
	}
}

// A partial recorded set must not be blended in: it would drop projects the
// derived fit scored and quietly change the question.
func TestAPartialRecordedSetFallsBackToDerived(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	judge := tokenFor(t, tokens, data, "judge_a")
	comparisonFixture(t, data, "judge_a")

	// One comparison is not enough to rank the panel.
	request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/comparisons", judge,
		map[string]any{"left": "prj_02", "right": "prj_04", "verdict": "left"})

	got := request(t, server, http.MethodGet, "/v1/organizer/pairwise?event_id=evt_01", organizer, nil)
	result := decode(t, got.Body.Bytes())["data"].(map[string]any)
	if result["source"] != "derived" {
		t.Errorf("a partial recorded set produced source %v, want derived", result["source"])
	}
	if count, _ := result["recorded_comparisons"].(float64); count != 0 {
		t.Errorf("recorded_comparisons = %v, want 0 on the derived path", result["recorded_comparisons"])
	}
}

// The estimator itself, independent of HTTP.
func TestRecordedTallyMatchesHandCountedVerdicts(t *testing.T) {
	reviews := []domain.Review{
		{ID: "r1", EventID: "e", JudgeID: "j1", ProjectID: "a", Criteria: map[string]int{"q": 5}},
		{ID: "r2", EventID: "e", JudgeID: "j1", ProjectID: "b", Criteria: map[string]int{"q": 1}},
		{ID: "r3", EventID: "e", JudgeID: "j2", ProjectID: "a", Criteria: map[string]int{"q": 4}},
		{ID: "r4", EventID: "e", JudgeID: "j2", ProjectID: "b", Criteria: map[string]int{"q": 2}},
	}
	weights := judging.Weights{"q": 1}

	// A recorded verdict that contradicts the reviews: a says b is better.
	comparisons := []domain.Comparison{
		{EventID: "e", JudgeID: "j1", Left: "a", Right: "b", Verdict: domain.ComparisonRightWins},
		{EventID: "e", JudgeID: "j2", Left: "a", Right: "b", Verdict: domain.ComparisonLeftWins},
	}

	result, err := judging.PairwiseRecorded(reviews, comparisons, weights)
	if err != nil {
		t.Fatalf("PairwiseRecorded() error = %v", err)
	}
	if result.Source != judging.SourceRecorded {
		t.Fatalf("source = %v, want recorded", result.Source)
	}
	byProject := map[string]judging.PairwiseEntry{}
	for _, entry := range result.Entries {
		byProject[entry.ProjectID] = entry
	}
	a, b := byProject["a"], byProject["b"]
	// The panel split: one judge preferred b, the other preferred a. Each project
	// has one win and one loss, so the rates must be even and the strengths equal.
	if a.WinRate == nil || b.WinRate == nil {
		t.Fatalf("a recorded fit reported no win rate: %+v %+v", a, b)
	}
	if *a.WinRate != 0.5 || *b.WinRate != 0.5 {
		t.Errorf("win rates are %v and %v, want 0.5 and 0.5 for a split panel", *a.WinRate, *b.WinRate)
	}
	if a.Strength != b.Strength {
		t.Errorf("a split panel produced unequal strengths %v and %v", a.Strength, b.Strength)
	}
	if a.Ties != 0 || b.Ties != 0 {
		t.Errorf("ties are %d and %d, want none", a.Ties, b.Ties)
	}
	if result.TotalComparisons != 2 {
		t.Errorf("total comparisons = %d, want 2", result.TotalComparisons)
	}
}

func TestATieCountsHalfToEachSide(t *testing.T) {
	reviews := []domain.Review{
		{ID: "r1", EventID: "e", JudgeID: "j1", ProjectID: "a", Criteria: map[string]int{"q": 5}},
		{ID: "r2", EventID: "e", JudgeID: "j1", ProjectID: "b", Criteria: map[string]int{"q": 1}},
	}
	comparisons := []domain.Comparison{
		{EventID: "e", JudgeID: "j1", Left: "a", Right: "b", Verdict: domain.ComparisonTied},
	}
	result, err := judging.PairwiseRecorded(reviews, comparisons, judging.Weights{"q": 1})
	if err != nil {
		t.Fatalf("PairwiseRecorded() error = %v", err)
	}
	for _, entry := range result.Entries {
		if entry.Ties != 1 {
			t.Errorf("%s recorded %d ties, want 1", entry.ProjectID, entry.Ties)
		}
		if entry.Wins != judging.DefaultPairwiseConfig().WeightTie {
			t.Errorf("%s has %v wins, want the half-tie weight %v", entry.ProjectID, entry.Wins, judging.DefaultPairwiseConfig().WeightTie)
		}
		// A pure tie has no decided comparison, so a win rate would be a division
		// by zero dressed up as a number.
		if entry.WinRate != nil {
			t.Errorf("%s reported a win rate of %v for an all-ties record", entry.ProjectID, *entry.WinRate)
		}
	}
}
