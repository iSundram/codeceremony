package judging

import (
	"math"
	"testing"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

func pairReview(judge, project string, functionality, quality, innovation int) domain.Review {
	return domain.Review{
		ID: judge + ":" + project, JudgeID: judge, ProjectID: project,
		Criteria:  map[string]int{"functionality": functionality, "quality": quality, "innovation": innovation},
		Submitted: true, UpdatedAt: time.Now(),
	}
}

func pairWeights() Weights { return Weights{"functionality": 40, "quality": 35, "innovation": 25} }

// A transitive ordering must come out in the right order, with the strongest
// project's strength above the geometric mean and the weakest below it.
func TestPairwiseRecoversATransitiveOrdering(t *testing.T) {
	// A > B > C, with one upset so the panel is not perfectly decisive. A panel
	// where somebody never lost and somebody never won has no finite maximum
	// likelihood estimate at all, which is covered by its own test below.
	reviews := []domain.Review{
		pairReview("j1", "a", 5, 5, 5), pairReview("j1", "b", 3, 3, 3), pairReview("j1", "c", 1, 1, 1),
		pairReview("j2", "a", 5, 4, 5), pairReview("j2", "b", 3, 3, 2), pairReview("j2", "c", 2, 1, 1),
		pairReview("j3", "a", 4, 5, 4), pairReview("j3", "b", 3, 2, 3), pairReview("j3", "c", 2, 2, 2),
		pairReview("j4", "a", 5, 5, 5), pairReview("j4", "b", 3, 3, 3), pairReview("j4", "c", 2, 1, 1),
		// j5 is the upset: this judge preferred c over both a and b, so a is
		// not undefeated and the maximum likelihood estimate is finite.
		pairReview("j5", "a", 3, 3, 3), pairReview("j5", "b", 3, 2, 3), pairReview("j5", "c", 5, 5, 5),
	}
	result, err := Pairwise(reviews, pairWeights())
	if err != nil {
		t.Fatalf("Pairwise() error = %v", err)
	}
	if len(result.Entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(result.Entries))
	}
	order := []string{result.Entries[0].ProjectID, result.Entries[1].ProjectID, result.Entries[2].ProjectID}
	if order[0] != "a" || order[1] != "b" || order[2] != "c" {
		t.Errorf("order = %v, want [a b c]", order)
	}
	for i, entry := range result.Entries {
		if entry.Rank != i+1 {
			t.Errorf("entry %s rank = %d, want %d", entry.ProjectID, entry.Rank, i+1)
		}
	}
	if result.Entries[0].Strength <= 1 {
		t.Errorf("top strength = %v, want above the normalised mean of 1", result.Entries[0].Strength)
	}
	if result.Entries[2].Strength >= 1 {
		t.Errorf("bottom strength = %v, want below the normalised mean of 1", result.Entries[2].Strength)
	}
	if !result.Converged {
		t.Error("estimator did not converge on a well-conditioned problem")
	}
}

// The model is only identified up to a common scale, so the implementation
// normalises the geometric mean to 1. A project that beats half the field
// should land near 1.
func TestPairwiseNormalisesScale(t *testing.T) {
	// a and b never meet each other. Each beats c exactly once, so their
	// records are identical and the model must give them identical strengths.
	reviews := []domain.Review{
		pairReview("j1", "a", 5, 5, 5), pairReview("j1", "c", 3, 3, 3),
		pairReview("j2", "b", 5, 5, 5), pairReview("j2", "c", 3, 3, 3),
	}
	result, err := Pairwise(reviews, pairWeights())
	if err != nil {
		t.Fatalf("Pairwise() error = %v", err)
	}
	product := 1.0
	for _, entry := range result.Entries {
		product *= entry.Strength
	}
	geometricMean := math.Pow(product, 1/float64(len(result.Entries)))
	if math.Abs(geometricMean-1) > 1e-3 {
		t.Errorf("geometric mean of strengths = %v, want 1", geometricMean)
	}
	strengths := map[string]float64{}
	for _, entry := range result.Entries {
		strengths[entry.ProjectID] = entry.Strength
	}
	if math.Abs(strengths["a"]-strengths["b"]) > 1e-6 {
		t.Errorf("a and b have identical records but strengths differ: %v vs %v", strengths["a"], strengths["b"])
	}
	if strengths["c"] >= strengths["a"] {
		t.Error("c lost to both a and b and must be the weakest project")
	}
}

// A tie must contribute half a win to each side, or the two projects would get
// systematically different treatment from an undecided comparison.
func TestPairwiseCountsTiesAsHalfAWin(t *testing.T) {
	reviews := []domain.Review{
		pairReview("j1", "a", 3, 3, 3), pairReview("j1", "b", 3, 3, 3),
		pairReview("j2", "a", 4, 4, 4), pairReview("j2", "b", 3, 3, 3),
	}
	result, err := Pairwise(reviews, pairWeights())
	if err != nil {
		t.Fatalf("Pairwise() error = %v", err)
	}
	wins := map[string]float64{}
	for _, entry := range result.Entries {
		wins[entry.ProjectID] = entry.Wins
		if entry.Ties != 1 {
			t.Errorf("project %s ties = %d, want 1", entry.ProjectID, entry.Ties)
		}
	}
	// j1 tied them, j2 scored a above b, so a has one win plus half a tie and
	// b has half a tie and nothing else.
	if math.Abs(wins["a"]-1.5) > 1e-9 {
		t.Errorf("project a wins = %v, want 1.5 (one win plus half a tie)", wins["a"])
	}
	if math.Abs(wins["b"]-0.5) > 1e-9 {
		t.Errorf("project b wins = %v, want 0.5 (half a tie)", wins["b"])
	}
	if result.Entries[0].Strength <= result.Entries[1].Strength {
		t.Error("a won its non-tied comparison and should be the stronger project")
	}
}

// A project nobody compared has no strength estimate. Reporting it as zero
// would be a fabricated number, so it is listed separately and left unranked
// behind the estimated ones.
func TestPairwiseReportsUncomparedProjectsRatherThanScoringThemZero(t *testing.T) {
	reviews := []domain.Review{
		pairReview("j1", "a", 5, 5, 5), pairReview("j1", "b", 3, 3, 3),
		pairReview("j2", "a", 5, 5, 5), pairReview("j2", "b", 3, 3, 3),
		// j3 reviewed only this project, so it never met anyone.
		pairReview("j3", "lonely", 4, 4, 4),
	}
	result, err := Pairwise(reviews, pairWeights())
	if err != nil {
		t.Fatalf("Pairwise() error = %v", err)
	}
	if len(result.ProjectsWithNoComparisons) != 1 || result.ProjectsWithNoComparisons[0] != "lonely" {
		t.Fatalf("uncompared = %v, want [lonely]", result.ProjectsWithNoComparisons)
	}
	for _, entry := range result.Entries {
		if entry.ProjectID == "lonely" {
			t.Fatal("an uncompared project must not appear in the ranked entries")
		}
	}
}

// A review missing a criterion cannot be compared head to head on a
// one-criterion basis, because that is a different question.
func TestPairwiseSkipsIncompleteReviews(t *testing.T) {
	reviews := []domain.Review{
		pairReview("j1", "a", 5, 5, 5),
		{ID: "j1:b", JudgeID: "j1", ProjectID: "b", Criteria: map[string]int{"functionality": 1}, Submitted: true},
		pairReview("j2", "a", 5, 5, 5), pairReview("j2", "b", 2, 2, 2),
	}
	result, err := Pairwise(reviews, pairWeights())
	if err != nil {
		t.Fatalf("Pairwise() error = %v", err)
	}
	// Only the j2 comparison of a and b is usable.
	if result.TotalComparisons != 1 {
		t.Errorf("total comparisons = %d, want 1", result.TotalComparisons)
	}
}

// Nothing to compare is an error, not a table of zeroes.
func TestPairwiseRefusesWhenNoComparisonsExist(t *testing.T) {
	reviews := []domain.Review{
		pairReview("j1", "a", 5, 5, 5),
		pairReview("j2", "b", 3, 3, 3),
	}
	if _, err := Pairwise(reviews, pairWeights()); err == nil {
		t.Fatal("Pairwise() error = nil, want a failure when no judge reviewed two projects")
	}
}

// The estimator must be reproducible, or a published pairwise table could not be
// recomputed.
func TestPairwiseIsDeterministic(t *testing.T) {
	reviews := make([]domain.Review, 0, 60)
	for judge := 0; judge < 6; judge++ {
		for project := 0; project < 10; project++ {
			reviews = append(reviews, pairReview(
				"j"+string(rune('a'+judge)),
				"p"+string(rune('a'+project)),
				1+project, 1+((project*3)%5), 1+((project*7)%5)))
		}
	}
	first, err := Pairwise(reviews, pairWeights())
	if err != nil {
		t.Fatalf("Pairwise() error = %v", err)
	}
	second, err := Pairwise(reviews, pairWeights())
	if err != nil {
		t.Fatalf("Pairwise() error = %v", err)
	}
	if len(first.Entries) != len(second.Entries) {
		t.Fatalf("entry counts differ: %d vs %d", len(first.Entries), len(second.Entries))
	}
	for i := range first.Entries {
		left, right := first.Entries[i], second.Entries[i]
		if left.ProjectID != right.ProjectID || left.Strength != right.Strength ||
			left.Wins != right.Wins || left.Losses != right.Losses ||
			left.Ties != right.Ties || left.Comparisons != right.Comparisons || left.Rank != right.Rank {
			t.Fatalf("entry %d differs between runs: %+v vs %+v", i, left, right)
		}
		if (left.WinRate == nil) != (right.WinRate == nil) {
			t.Fatalf("entry %d win rate presence differs between runs", i)
		}
		if left.WinRate != nil && *left.WinRate != *right.WinRate {
			t.Fatalf("entry %d win rate differs between runs: %v vs %v", i, *left.WinRate, *right.WinRate)
		}
	}
}

// A perfectly decisive panel has no finite maximum likelihood estimate: the
// project that won everything has unbounded strength and the one that lost
// everything has zero. The fit must say so rather than report a large finite
// number, because such a number is decided by the iteration budget and not by
// the data.
func TestPairwiseReportsADecisivePanelAsUnbounded(t *testing.T) {
	reviews := []domain.Review{
		pairReview("j1", "a", 5, 5, 5), pairReview("j1", "b", 3, 3, 3), pairReview("j1", "c", 1, 1, 1),
		pairReview("j2", "a", 5, 4, 5), pairReview("j2", "b", 3, 3, 2), pairReview("j2", "c", 2, 1, 1),
		pairReview("j3", "a", 4, 5, 4), pairReview("j3", "b", 3, 2, 3), pairReview("j3", "c", 1, 2, 1),
	}
	result, err := Pairwise(reviews, pairWeights())
	if err != nil {
		t.Fatalf("Pairwise() error = %v", err)
	}
	if !result.Unbounded {
		t.Error("a panel where one project never lost and another never won must be reported as unbounded")
	}
	if result.Note == "" {
		t.Error("an unbounded result must carry a note explaining what to do instead")
	}
}

// A panel that is not decisive must not be reported as decisive.
//
// The minimum-sentinal bug this covers was quiet: the search for the weakest
// strength started at zero, and a Bradley-Terry strength is always positive, so
// it never updated and the strongest/weakest ratio was infinite for every panel.
// The warning fired unconditionally, which is worse than not having it, because
// a reader learns to ignore it.
func TestANonDecisivePanelIsNotReportedAsUnbounded(t *testing.T) {
	reviews := []domain.Review{
		// Two judges rank a > b > c. The third reverses the whole order, which is what
		// puts cycles in the comparison graph: every project both wins and loses, so no
		// strength ratio diverges and the estimate is finite.
		{ID: "r1", EventID: "e", JudgeID: "j1", ProjectID: "a", Criteria: map[string]int{"q": 5}},
		{ID: "r2", EventID: "e", JudgeID: "j1", ProjectID: "b", Criteria: map[string]int{"q": 3}},
		{ID: "r3", EventID: "e", JudgeID: "j1", ProjectID: "c", Criteria: map[string]int{"q": 1}},
		{ID: "r4", EventID: "e", JudgeID: "j2", ProjectID: "a", Criteria: map[string]int{"q": 5}},
		{ID: "r5", EventID: "e", JudgeID: "j2", ProjectID: "b", Criteria: map[string]int{"q": 3}},
		{ID: "r6", EventID: "e", JudgeID: "j2", ProjectID: "c", Criteria: map[string]int{"q": 1}},
		{ID: "r7", EventID: "e", JudgeID: "j3", ProjectID: "a", Criteria: map[string]int{"q": 1}},
		{ID: "r8", EventID: "e", JudgeID: "j3", ProjectID: "b", Criteria: map[string]int{"q": 3}},
		{ID: "r9", EventID: "e", JudgeID: "j3", ProjectID: "c", Criteria: map[string]int{"q": 5}},
	}
	// A cycle is what keeps the maximum likelihood finite: every project both wins
	// and loses, so no strength ratio diverges.
	result, err := PairwiseConfigured(reviews, Weights{"q": 1}, DefaultPairwiseConfig())
	if err != nil {
		t.Fatalf("PairwiseConfigured() error = %v", err)
	}
	if result.Unbounded {
		t.Errorf("a panel with a cycle was reported as decisive: %s", result.Note)
	}
	// And the sentinel itself: the spread of the reported strengths has to be
	// within the threshold for the panel to be considered measurable.
	var strongest, weakest = 0.0, 0.0
	for i, entry := range result.Entries {
		if i == 0 || entry.Strength > strongest {
			strongest = entry.Strength
		}
		if i == 0 || entry.Strength < weakest {
			weakest = entry.Strength
		}
	}
	if weakest <= 0 {
		t.Fatalf("a fitted strength is %v; the ratio test cannot be trusted", weakest)
	}
	if strongest/weakest > UnboundedRatio {
		t.Errorf("strength spread %v exceeds the threshold %v, so the panel is genuinely decisive",
			strongest/weakest, UnboundedRatio)
	}
}
