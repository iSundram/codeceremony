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

func TestNormalizeRejectsMissingCriterion(t *testing.T) {
	reviews := []domain.Review{{ID: "r1", JudgeID: "judge_a", ProjectID: "project_a", Criteria: map[string]int{"functionality": 4}, Submitted: true}}
	_, err := Normalize(reviews, Weights{"functionality": 1, "quality": 1})
	if err == nil {
		t.Fatal("Normalize() error = nil, want missing criterion error")
	}
}
