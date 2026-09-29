package judging

import (
	"testing"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

func verdict(judge, left, right string, v domain.ComparisonVerdict) domain.Comparison {
	return domain.Comparison{ID: judge + ":" + left + ":" + right, JudgeID: judge, Left: left, Right: right, Verdict: v}
}

func strengthOf(t *testing.T, r PairwiseResult) map[string]float64 {
	t.Helper()
	out := make(map[string]float64, len(r.Entries))
	for _, entry := range r.Entries {
		out[entry.ProjectID] = entry.Strength
	}
	return out
}

// scored builds one submitted review per project for each judge.
func scored(judges []string, marks map[string][3]int) []domain.Review {
	var out []domain.Review
	for _, j := range judges {
		for project, m := range marks {
			out = append(out, pairReview(j, project, m[0], m[1], m[2]))
		}
	}
	return out
}

// The recorded path was reached by no test, so nothing observed what it does.
//
// It is reached through PairwiseRecorded, which falls back to the derived fit
// unless the recorded comparisons cover exactly the projects the reviews cover.
// A test that omits a review, or omits a project, is therefore silently
// exercising the fallback and calling the result "recorded" — which is how a
// completely inverted fit reached 100% green. The panel below makes the scores
// and the verdicts disagree on purpose, so which path ran is visible in the
// answer, and the ordering assertion cannot be satisfied by the fallback.
func TestPairwiseRecordedUsesTheVerdictsWhenTheyCoverTheScoredProjects(t *testing.T) {
	// The scores rank d first. The verdicts say a beat b beat c beat d.
	reviews := scored([]string{"j1", "j2"}, map[string][3]int{
		"a": {1, 1, 1}, "b": {2, 2, 2}, "c": {3, 3, 3}, "d": {5, 5, 5},
	})
	comparisons := []domain.Comparison{
		verdict("j1", "a", "b", domain.ComparisonLeftWins),
		verdict("j1", "b", "c", domain.ComparisonLeftWins),
		verdict("j1", "c", "d", domain.ComparisonLeftWins),
		verdict("j1", "a", "c", domain.ComparisonLeftWins),
		verdict("j1", "a", "d", domain.ComparisonLeftWins),
		verdict("j1", "b", "d", domain.ComparisonLeftWins),
		verdict("j2", "a", "b", domain.ComparisonLeftWins),
		verdict("j2", "b", "c", domain.ComparisonLeftWins),
		verdict("j2", "c", "d", domain.ComparisonLeftWins),
		verdict("j2", "a", "c", domain.ComparisonLeftWins),
		verdict("j2", "a", "d", domain.ComparisonLeftWins),
		// j2 prefers d over b, which stops the panel being decisive.
		verdict("j2", "d", "b", domain.ComparisonLeftWins),
	}

	result, err := PairwiseRecorded(reviews, comparisons, pairWeights())
	if err != nil {
		t.Fatalf("PairwiseRecorded() error = %v", err)
	}
	if result.Source != "recorded" {
		t.Fatalf("Source = %q, want \"recorded\"; the recorded path was skipped", result.Source)
	}
	if result.Iterations < 2 {
		t.Errorf("Iterations = %d, want more than 1: the fitter never moved, so it published its seed strengths", result.Iterations)
	}

	strength := strengthOf(t, result)
	if len(strength) != 4 {
		t.Fatalf("got %d entries, want 4: %+v", len(strength), result.Entries)
	}
	// Following the verdicts is the entire point of a recorded comparison. With
	// the scores saying the opposite, following them here would mean the
	// recorded path silently did nothing.
	for i := 0; i < 3; i++ {
		better, worse := "abcd"[i:i+1], "abcd"[i+1:i+2]
		if strength[better] <= strength[worse] {
			t.Errorf("strength[%s] = %v, want > strength[%s] = %v: the fit is inverted or flat",
				better, strength[better], worse, strength[worse])
		}
	}
}

// The published converged flag has to mean something on the recorded path too.
// A well-posed panel — every project both wins and loses, so the maximum
// likelihood estimate is finite and well conditioned — must converge inside the
// iteration budget rather than reporting a failed fit to a judge.
func TestPairwiseRecordedConvergesOnAWellPosedPanel(t *testing.T) {
	reviews := scored([]string{"j1", "j2", "j3"}, map[string][3]int{
		"a": {4, 4, 4}, "b": {3, 3, 3}, "c": {2, 2, 2}, "d": {1, 1, 1},
	})
	// A cycle plus a few extra results, so nobody is undefeated or unbeaten.
	comparisons := []domain.Comparison{
		verdict("j1", "a", "b", domain.ComparisonLeftWins),
		verdict("j1", "b", "c", domain.ComparisonLeftWins),
		verdict("j1", "c", "d", domain.ComparisonLeftWins),
		verdict("j1", "d", "a", domain.ComparisonLeftWins),
		verdict("j1", "a", "c", domain.ComparisonLeftWins),
		verdict("j1", "b", "d", domain.ComparisonLeftWins),
		verdict("j2", "a", "b", domain.ComparisonLeftWins),
		verdict("j2", "b", "c", domain.ComparisonLeftWins),
		verdict("j2", "c", "d", domain.ComparisonLeftWins),
		verdict("j2", "d", "a", domain.ComparisonLeftWins),
		verdict("j2", "b", "d", domain.ComparisonLeftWins),
		verdict("j3", "a", "c", domain.ComparisonLeftWins),
		verdict("j3", "c", "b", domain.ComparisonLeftWins),
		verdict("j3", "d", "a", domain.ComparisonLeftWins),
		verdict("j3", "b", "d", domain.ComparisonLeftWins),
	}

	result, err := PairwiseRecorded(reviews, comparisons, pairWeights())
	if err != nil {
		t.Fatalf("PairwiseRecorded() error = %v", err)
	}
	if result.Source != "recorded" {
		t.Fatalf("Source = %q, want \"recorded\"", result.Source)
	}
	if !result.Converged {
		t.Errorf("Converged = false after %d iterations on a well-posed panel; the published flag misreports a healthy fit", result.Iterations)
	}
	if result.Unbounded {
		t.Errorf("Unbounded = true on a panel where every project both won and lost")
	}
}

// A pair judged by two judges is one matchup carrying weight two, not two
// matchups. The recorded path appended the pair once per verdict, which put it
// in the fitter's opponent list twice and counted that matchup at double
// strength; the derived path has always de-duplicated.
func TestPairwiseRecordedCountsAMatchupOnceHoweverManyJudgesSawIt(t *testing.T) {
	build := func(judges ...string) ([]domain.Review, []domain.Comparison) {
		marks := map[string][3]int{"a": {1, 1, 1}, "b": {2, 2, 2}, "c": {3, 3, 3}}
		var comparisons []domain.Comparison
		for _, j := range judges {
			comparisons = append(comparisons,
				verdict(j, "a", "b", domain.ComparisonLeftWins),
				verdict(j, "b", "c", domain.ComparisonLeftWins),
				verdict(j, "a", "c", domain.ComparisonLeftWins))
		}
		return scored(judges, marks), comparisons
	}

	oneReviews, oneComparisons := build("j1")
	twoReviews, twoComparisons := build("j1", "j2")

	one, err := PairwiseRecorded(oneReviews, oneComparisons, pairWeights())
	if err != nil {
		t.Fatalf("PairwiseRecorded(one) error = %v", err)
	}
	if one.Source != "recorded" {
		t.Fatalf("one judge: Source = %q, want \"recorded\"", one.Source)
	}
	if one.TotalComparisons != 3 {
		t.Errorf("one judge: TotalComparisons = %d, want 3 distinct matchups", one.TotalComparisons)
	}

	two, err := PairwiseRecorded(twoReviews, twoComparisons, pairWeights())
	if err != nil {
		t.Fatalf("PairwiseRecorded(two) error = %v", err)
	}
	// The count is of verdicts, not matchups: six answers, three pairings.
	if two.TotalComparisons != 6 {
		t.Errorf("two judges: TotalComparisons = %d, want 6 verdicts", two.TotalComparisons)
	}

	// A second judge who agrees on everything changes the weight, not the
	// shape, so the ordering is identical and only the scale moves.
	oneStrength, twoStrength := strengthOf(t, one), strengthOf(t, two)
	for i := 0; i < 2; i++ {
		better, worse := "abc"[i:i+1], "abc"[i+1:i+2]
		if oneStrength[better] <= oneStrength[worse] {
			t.Errorf("one judge: strength[%s] = %v, want > strength[%s] = %v",
				better, oneStrength[better], worse, oneStrength[worse])
		}
		if twoStrength[better] <= twoStrength[worse] {
			t.Errorf("two judges: strength[%s] = %v, want > strength[%s] = %v",
				better, twoStrength[better], worse, twoStrength[worse])
		}
	}
}

// The decisive-panel note names an arbitrary project. A value that reshuffles
// between two identical requests cannot be diagnosed by anyone reading it, and
// the previous implementation took the first match out of a randomized map.
func TestPairwiseReportedNoteIsStableAcrossIdenticalRequests(t *testing.T) {
	reviews := scored([]string{"j1", "j2"}, map[string][3]int{
		"a": {1, 1, 1}, "b": {2, 2, 2}, "c": {3, 3, 3}, "d": {4, 4, 4},
	})
	comparisons := []domain.Comparison{
		verdict("j1", "a", "b", domain.ComparisonLeftWins),
		verdict("j1", "a", "c", domain.ComparisonLeftWins),
		verdict("j1", "a", "d", domain.ComparisonLeftWins),
		verdict("j2", "c", "b", domain.ComparisonLeftWins),
		verdict("j2", "c", "d", domain.ComparisonLeftWins),
	}
	first, err := PairwiseRecorded(reviews, comparisons, pairWeights())
	if err != nil {
		t.Fatalf("PairwiseRecorded() error = %v", err)
	}
	if first.Note == "" {
		t.Skip("this panel produced no note, so there is nothing to compare")
	}
	for i := 0; i < 100; i++ {
		again, err := PairwiseRecorded(reviews, comparisons, pairWeights())
		if err != nil {
			t.Fatalf("PairwiseRecorded() error = %v", err)
		}
		if again.Note != first.Note {
			t.Fatalf("Note changed between identical requests: %q then %q", first.Note, again.Note)
		}
	}
}
