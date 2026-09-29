package judging

import (
	"testing"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

func testReview(id, judge, project string, functionality, quality, innovation int) domain.Review {
	return domain.Review{
		ID:        id,
		JudgeID:   judge,
		ProjectID: project,
		Criteria:  map[string]int{"functionality": functionality, "quality": quality, "innovation": innovation},
		Submitted: true,
		UpdatedAt: time.Now(),
	}
}

func TestNormalizeProducesRankedProjects(t *testing.T) {
	reviews := []domain.Review{
		testReview("r1", "judge_a", "project_a", 2, 3, 2),
		testReview("r2", "judge_a", "project_b", 5, 5, 5),
		testReview("r3", "judge_a", "project_c", 3, 3, 3),
		testReview("r4", "judge_b", "project_a", 4, 4, 4),
		testReview("r5", "judge_b", "project_b", 5, 4, 5),
		testReview("r6", "judge_b", "project_c", 3, 3, 3),
	}
	weights := Weights{"functionality": 40, "quality": 35, "innovation": 25}

	summary, err := Normalize(reviews, weights)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if len(summary.Projects) != 3 {
		t.Fatalf("project count = %d, want 3", len(summary.Projects))
	}
	if summary.Projects[0].ProjectID != "project_b" {
		t.Fatalf("first project = %s, want project_b", summary.Projects[0].ProjectID)
	}
	if summary.Projects[0].Rank != 1 {
		t.Fatalf("first rank = %d, want 1", summary.Projects[0].Rank)
	}
	if len(summary.LowInformationJudges) != 0 {
		t.Fatalf("low information judges = %v, want none", summary.LowInformationJudges)
	}
}

func TestNormalizeFlagsConstantJudgeWithoutDroppingReview(t *testing.T) {
	reviews := []domain.Review{
		testReview("r1", "constant_judge", "project_a", 4, 4, 4),
		testReview("r2", "constant_judge", "project_b", 4, 4, 4),
		testReview("r3", "constant_judge", "project_c", 4, 4, 4),
	}
	weights := Weights{"functionality": 1, "quality": 1, "innovation": 1}

	summary, err := Normalize(reviews, weights)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if len(summary.Projects) != 3 {
		t.Fatalf("project count = %d, want 3", len(summary.Projects))
	}
	if len(summary.LowInformationJudges) != 1 || summary.LowInformationJudges[0] != "constant_judge" {
		t.Fatalf("low information judges = %v, want [constant_judge]", summary.LowInformationJudges)
	}
	for _, project := range summary.Projects {
		if project.LowInformationReviews != 1 {
			t.Fatalf("project %s low information reviews = %d, want 1", project.ProjectID, project.LowInformationReviews)
		}
	}
}

// A judge who skipped a criterion expressed no opinion on it. Rejecting the
// whole review would make the results endpoint fail closed the moment one
// criterion went unscored, so the missing criterion is imputed at the judge's
// own average, which is exactly neutral, and the review is flagged partial.
func TestNormalizeImputesMissingCriterionNeutrally(t *testing.T) {
	reviews := []domain.Review{{
		ID: "r1", JudgeID: "judge_a", ProjectID: "project_a",
		Criteria: map[string]int{"functionality": 4}, Submitted: true,
	}}
	summary, err := Normalize(reviews, Weights{"functionality": 1, "quality": 1})
	if err != nil {
		t.Fatalf("Normalize() error = %v, want a partial review to be tolerated", err)
	}
	if len(summary.Reviews) != 1 {
		t.Fatalf("review count = %d, want 1", len(summary.Reviews))
	}
	review := summary.Reviews[0]
	if !review.Partial {
		t.Error("review is not flagged partial despite a missing criterion")
	}
	if len(review.MissingCriteria) != 1 || review.MissingCriteria[0] != "quality" {
		t.Errorf("missing criteria = %v, want [quality]", review.MissingCriteria)
	}
	if got := review.Criteria["quality"]; got != neutralScore {
		t.Errorf("imputed quality = %v, want the neutral %v", got, neutralScore)
	}
}

// A judge who rated every project identically on a criterion has no spread to
// rescale against, so that criterion must land on neutral rather than divide by
// an almost-zero standard deviation and produce an enormous z.
func TestNormalizeDegenerateJudgeIsNeutralNotExplosive(t *testing.T) {
	reviews := []domain.Review{
		{ID: "r1", JudgeID: "flat", ProjectID: "project_a", Criteria: map[string]int{"functionality": 4}, Submitted: true},
		{ID: "r2", JudgeID: "flat", ProjectID: "project_b", Criteria: map[string]int{"functionality": 4}, Submitted: true},
	}
	summary, err := Normalize(reviews, Weights{"functionality": 1})
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	for _, project := range summary.Projects {
		if project.NormalizedMean > neutralScore+1e-9 || project.NormalizedMean < neutralScore-1e-9 {
			t.Errorf("project %s normalized mean = %v, want exactly neutral", project.ProjectID, project.NormalizedMean)
		}
	}
	if len(summary.LowInformationJudges) != 1 || summary.LowInformationJudges[0] != "flat" {
		t.Errorf("low information judges = %v, want [flat]", summary.LowInformationJudges)
	}
}

// A judge with one review cannot establish their own scale, so their z-scores
// are shrunk to almost nothing and they barely move the result.
func TestNormalizeShrinksSingleReviewJudge(t *testing.T) {
	reviews := []domain.Review{
		{ID: "r1", JudgeID: "thorough", ProjectID: "project_a", Criteria: map[string]int{"functionality": 5}, Submitted: true},
		{ID: "r2", JudgeID: "thorough", ProjectID: "project_b", Criteria: map[string]int{"functionality": 1}, Submitted: true},
		{ID: "r3", JudgeID: "thorough", ProjectID: "project_c", Criteria: map[string]int{"functionality": 3}, Submitted: true},
		{ID: "r4", JudgeID: "one_shot", ProjectID: "project_a", Criteria: map[string]int{"functionality": 5}, Submitted: true},
	}
	summary, err := Normalize(reviews, Weights{"functionality": 1})
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	diagnostics := map[string]JudgeDiagnostic{}
	for _, diagnostic := range summary.Judges {
		diagnostics[diagnostic.JudgeID] = diagnostic
	}
	if got := diagnostics["one_shot"].Reliability; got >= 0.5 {
		t.Errorf("single-review judge reliability = %v, want below 0.5", got)
	}
	if got := diagnostics["thorough"].Reliability; got <= 0.5 {
		t.Errorf("three-review judge reliability = %v, want above 0.5", got)
	}
	if !diagnostics["one_shot"].LowInformation {
		t.Error("single-review judge is not flagged low information")
	}
}

// The bootstrap must be reproducible, otherwise an archived result set cannot
// be recomputed and the reported intervals are not auditable.
func TestNormalizeUncertaintyIsDeterministic(t *testing.T) {
	reviews := make([]domain.Review, 0, 40)
	for i := 0; i < 40; i++ {
		review := domain.Review{
			ID:        "r" + string(rune('a'+i%26)) + string(rune('a'+i/26)),
			JudgeID:   "judge_" + string(rune('a'+i%7)),
			Criteria:  map[string]int{"functionality": 1 + i%5, "quality": 1 + (i*3)%5, "innovation": 1 + (i*7)%5},
			Submitted: true,
		}
		review.ProjectID = "project_" + string(rune('a'+i%11))
		reviews = append(reviews, review)
	}
	weights := Weights{"functionality": 40, "quality": 35, "innovation": 25}
	first, err := Normalize(reviews, weights)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	second, err := Normalize(reviews, weights)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	for i := range first.Projects {
		if first.Projects[i] != second.Projects[i] {
			t.Fatalf("project %d differs between runs: %+v vs %+v", i, first.Projects[i], second.Projects[i])
		}
	}
}

// Adjacent projects whose intervals overlap must not be reported as separated.
func TestAnnotateSeparabilityMarksOverlappingIntervals(t *testing.T) {
	projects := []ProjectResult{
		{ProjectID: "a", NormalizedMean: 80, Low: 72, High: 88},
		{ProjectID: "b", NormalizedMean: 79, Low: 70, High: 86},
		{ProjectID: "c", NormalizedMean: 40, Low: 20, High: 60},
	}
	annotateSeparability(projects)
	if !projects[0].Separable {
		t.Error("top project should be separable")
	}
	if projects[1].Separable {
		t.Error("project b overlaps a and should not be separable")
	}
	if !projects[2].Separable {
		t.Error("project c is far below b and should be separable")
	}
	if projects[1].TieGroup != projects[0].TieGroup {
		t.Errorf("tie groups = %d,%d want overlapping projects in the same group", projects[0].TieGroup, projects[1].TieGroup)
	}
	if projects[2].TieGroup == projects[1].TieGroup {
		t.Error("a separable project should start a new tie group")
	}
}
