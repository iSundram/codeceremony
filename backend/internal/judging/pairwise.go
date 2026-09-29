package judging

// Bradley-Terry pairwise comparison.
//
// # Why a second estimator
//
// The rubric pipeline in judging.go asks a judge to score each project on each
// criterion independently. That is convenient to use and convenient to reason
// about, but it is a demanding thing to ask: producing three absolute judgements
// per project, per criterion, and holding them consistent across 40 projects is
// cognitively expensive, and judges are famously inconsistent at it.
//
// A pairwise question is much easier to answer honestly: "which of these two do
// you prefer?" It requires no scale, no calibration between criteria, and no
// memory of what you said about the last project. It is also what people
// actually do when they have to rank things.
//
// So the portal offers a second view built entirely on pairwise data, with no
// rubric involved.
//
// # Deriving comparisons
//
// The fixture file contains rubric scores rather than explicit head-to-head
// verdicts, so comparisons are derived: for each judge, take every pair of
// projects that judge reviewed, and let the higher weighted rubric score win.
// A judge who scored A at 4 and B at 3 has, on their own scale, said A beats B.
// Deriving this way means every comparison is attributable to a specific judge
// and traceable back to the reviews behind it, which an organiser can audit.
//
// # The model
//
// P(i beats j) = pi_i / (pi_i + pi_j)
//
// pi is a positive strength. The model has an identifiability problem — scaling
// every pi by the same constant leaves every probability unchanged — so the
// geometric mean of pi is normalised to 1.
//
// # Fitting
//
// Minimised by the MM (minorisation-maximisation) algorithm of Hunter (2004),
// which is the standard estimator for this model. It monotonically decreases the
// negative log-likelihood and cannot diverge the way plain gradient ascent on a
// log-ratio parameterisation can, because each update is a proper maximum over a
// concave minorant. Ties are handled as half a win to each side.

import (
	"fmt"
	"math"
	"sort"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

// PairwiseConfig tunes the estimator.
type PairwiseConfig struct {
	// MaxIterations bounds the MM loop. Convergence is usually reached in well
	// under this; the bound exists so a pathological input cannot spin.
	MaxIterations int
	// Tolerance is the convergence threshold on the largest change in log pi.
	Tolerance float64
	// Damping mixes each update with the previous one. MM does not need damping
	// for correctness, but it makes the reported iteration count reproducible
	// across runs with different orderings of the input map.
	Damping float64
	// WeightTie is the share of a win each side receives from a tied comparison.
	WeightTie float64
}

// DefaultPairwiseConfig returns the shipped estimator's parameters.
func DefaultPairwiseConfig() PairwiseConfig {
	// The MM iteration converges linearly and can be slow. 1e-6 on log pi is a
	// strength accurate to one part in a million, which is far beyond anything a
	// ranking needs, and 200 iterations is a generous budget for the panels this
	// sees. When it is not enough the result reports converged=false rather than
	// presenting an unfinished fit as a finished one.
	return PairwiseConfig{
		MaxIterations: 200,
		Tolerance:     1e-6,
		Damping:       0.5,
		WeightTie:     0.5,
	}
}

func (c PairwiseConfig) withDefaults() PairwiseConfig {
	defaults := DefaultPairwiseConfig()
	if c.MaxIterations <= 0 {
		c.MaxIterations = defaults.MaxIterations
	}
	if c.Tolerance <= 0 {
		c.Tolerance = defaults.Tolerance
	}
	if c.Damping <= 0 || c.Damping > 1 {
		c.Damping = defaults.Damping
	}
	if c.WeightTie <= 0 || c.WeightTie >= 1 {
		c.WeightTie = defaults.WeightTie
	}
	return c
}

// PairwiseEntry is one project's strength under the pairwise model.
type PairwiseEntry struct {
	ProjectID   string  `json:"project_id"`
	Strength    float64 `json:"strength"`
	Wins        float64 `json:"wins"`
	Losses      float64 `json:"losses"`
	Ties        int     `json:"ties"`
	Comparisons int     `json:"comparisons"`
	// WinRate is wins / comparisons, ignoring ties. Null when a project was
	// never compared, which is different from having a rate of zero.
	WinRate *float64 `json:"win_rate"`
	// Rank is 1-based, most strong first, with deterministic tiebreaks.
	Rank int `json:"rank"`
}

// JudgeBreakdown attributes comparisons to the judges who made them, so an
// organiser can see whether one judge is driving an ordering.
type JudgeBreakdown struct {
	JudgeID     string  `json:"judge_id"`
	Comparisons int     `json:"comparisons"`
	WinRate     float64 `json:"win_rate"`
}

// PairwiseResult is the whole pairwise view.
type PairwiseResult struct {
	Method        string           `json:"method"`
	MethodVersion string           `json:"method_version"`
	Entries       []PairwiseEntry  `json:"entries"`
	Judges        []JudgeBreakdown `json:"judges"`
	// TotalComparisons is the number of head-to-head verdicts the fit consumed.
	TotalComparisons int `json:"total_comparisons"`
	// Iterations is how many MM updates ran before convergence or the bound.
	Iterations int `json:"iterations"`
	// Converged is false if the iteration budget ran out first.
	Converged bool `json:"converged"`
	// Unbounded is true when the panel is decisive enough that the maximum
	// likelihood estimate does not exist: a project that beat every project it
	// met has no finite strength. Reporting that plainly is the only honest
	// option, because a large finite number in that position is an artefact of
	// where the iteration happened to stop.
	Unbounded bool   `json:"unbounded"`
	Note      string `json:"note,omitempty"`
	// ProjectsWithNoComparisons is reported rather than silently dropped: a
	// project nobody compared has no strength estimate, and a strength of zero
	// would be a fabrication.
	ProjectsWithNoComparisons []string `json:"projects_with_no_comparisons"`
	// Source records whether the fit came from verdicts judges answered directly
	// or from comparisons inferred from rubric scores. A reader should not have
	// to consult the documentation to know which kind of evidence they are
	// looking at.
	Source ComparisonSource `json:"source"`
	// RecordedComparisons and DerivedComparisons break the total down, so a
	// panel that answers some pairs directly and leaves the rest to the rubric
	// says so in the response rather than in a footnote.
	RecordedComparisons int `json:"recorded_comparisons"`
	DerivedComparisons  int `json:"derived_comparisons"`
}

// ComparisonSource says what a fit was built from.
type ComparisonSource string

const (
	// SourceDerived means every comparison was inferred from rubric scores.
	SourceDerived ComparisonSource = "derived"
	// SourceRecorded means the fit used verdicts judges answered directly.
	SourceRecorded ComparisonSource = "recorded"
)

// PairwiseMethod is the identifier reported in the API and in JUDGING.md.
const PairwiseMethod = "bradley-terry-mm"

// Pairwise fits a Bradley-Terry model to comparisons derived from the reviews.
func Pairwise(reviews []domain.Review, weights Weights) (PairwiseResult, error) {
	return PairwiseConfigured(reviews, weights, DefaultPairwiseConfig())
}

// PairwiseRecorded fits the model to verdicts judges actually recorded.
//
// Recorded verdicts are used when they cover the same set of projects the
// derived fit did. A partial recorded set is not blended in: dropping the
// derived comparisons it does not cover would change the question being asked,
// and the resulting ordering would no longer be comparable with the rubric
// pipeline. Either way the response says which source was used, so a reader is
// never left inferring it from the documentation.
func PairwiseRecorded(reviews []domain.Review, comparisons []domain.Comparison, weights Weights) (PairwiseResult, error) {
	derived, err := PairwiseConfigured(reviews, weights, DefaultPairwiseConfig())
	if err != nil {
		return derived, err
	}
	record, complete := tallyFromComparisons(comparisons, DefaultPairwiseConfig().WeightTie)
	if !complete || !coversSameProjects(record, derived) {
		return derived, nil
	}
	result := fitFromTally(record, DefaultPairwiseConfig())
	result.Source = SourceRecorded
	result.RecordedComparisons = result.TotalComparisons
	result.DerivedComparisons = 0
	return result, nil
}

// coversSameProjects reports whether a recorded tally ranks exactly the projects
// the derived fit did. A set that is missing one would silently drop it.
func coversSameProjects(record tally, result PairwiseResult) bool {
	recorded := make(map[string]bool, len(record.played))
	for id := range record.played {
		recorded[id] = true
	}
	if len(recorded) != len(result.Entries) {
		return false
	}
	for _, entry := range result.Entries {
		if !recorded[entry.ProjectID] {
			return false
		}
	}
	return true
}

// tallyFromComparisons builds a head-to-head record from recorded verdicts.
//
// complete is false when the recorded set could not stand on its own: a project
// nobody compared, or a verdict that is not one of the three defined answers. The
// caller falls back to the derived fit rather than reporting a ranking built from
// a partial picture, which would look complete and not be.
func tallyFromComparisons(comparisons []domain.Comparison, tieWeight float64) (tally, bool) {
	record := tally{
		wins:     make(map[string]float64),
		losses:   make(map[string]float64),
		ties:     make(map[string]int),
		meetings: make(map[pairKey]int),
		played:   make(map[string]int),
		reviewed: make(map[string]bool),
		byJudge:  make(map[string]judgeComparison),
	}
	complete := true
	for _, comparison := range comparisons {
		left, right := comparison.Left, comparison.Right
		if left == right || !comparison.Verdict.Valid() {
			complete = false
			continue
		}
		record.played[left]++
		record.played[right]++
		record.reviewed[left] = true
		record.reviewed[right] = true

		judgeRecord := record.byJudge[comparison.JudgeID]
		judgeRecord.comparisons++
		switch comparison.Verdict {
		case domain.ComparisonLeftWins:
			record.wins[left]++
			record.losses[right]++
			judgeRecord.wins++
		case domain.ComparisonRightWins:
			record.wins[right]++
			record.losses[left]++
			judgeRecord.losses++
		case domain.ComparisonTied:
			record.ties[left]++
			record.ties[right]++
			// A draw counts as half a win to each side, which is the standard
			// handling and the reason the configuration carries a tie weight.
			record.wins[left] += tieWeight
			record.wins[right] += tieWeight
			judgeRecord.ties++
		}
		record.byJudge[comparison.JudgeID] = judgeRecord
		record.order = append(record.order, newPairKey(left, right))
	}
	for id := range record.reviewed {
		if record.played[id] == 0 {
			complete = false
		}
	}
	return record, complete
}

func PairwiseConfigured(reviews []domain.Review, weights Weights, config PairwiseConfig) (PairwiseResult, error) {
	config = config.withDefaults()
	if _, _, err := validateWeights(weights); err != nil {
		return PairwiseResult{}, err
	}

	record := deriveTally(reviews, weights, config.WeightTie)

	if len(record.order) == 0 {
		// Reported rather than hidden behind a 500: an organizer asking for the
		// pairwise view before any reviews exist should be told why it is empty.
		return PairwiseResult{
			Method: PairwiseMethod, MethodVersion: DefaultMethodVersion, Source: SourceDerived,
		}, fmt.Errorf("no pairwise comparisons could be derived: no judge reviewed two projects on the same criteria")
	}
	result := fitFromTally(record, config)
	result.Source = SourceDerived
	result.DerivedComparisons = result.TotalComparisons
	return result, nil
}

// fitFromTally turns a head-to-head record into the reported result.
//
// Both sources share this, which is the point: a derived comparison and a
// recorded verdict are different evidence, but once they are in the tally they
// are the same kind of datum, and two code paths would eventually disagree about
// how a strength is computed.
func fitFromTally(record tally, config PairwiseConfig) PairwiseResult {
	result := PairwiseResult{
		Method:           PairwiseMethod,
		MethodVersion:    DefaultMethodVersion,
		TotalComparisons: len(record.order),
	}
	// Every project that took part needs a strength, including one that only
	// ever lost, or it would be invisible in the output. Projects nobody
	// compared are included too and reported separately, because a strength of
	// zero would be a fabrication.
	seen := make(map[string]bool, len(record.reviewed))
	for id := range record.reviewed {
		seen[id] = true
	}
	participating := make([]string, 0, len(seen))
	uncompared := make([]string, 0)
	for id := range seen {
		if record.played[id] == 0 {
			uncompared = append(uncompared, id)
			continue
		}
		participating = append(participating, id)
	}
	sort.Strings(participating)
	sort.Strings(uncompared)
	result.ProjectsWithNoComparisons = uncompared

	strength, iterations, converged := fitBradleyTerry(participating, record, config)

	entries := make([]PairwiseEntry, 0, len(participating))
	for _, id := range participating {
		entry := PairwiseEntry{
			ProjectID:   id,
			Strength:    round(strength[id], 6),
			Wins:        round(record.wins[id], 4),
			Losses:      round(record.losses[id], 4),
			Ties:        record.ties[id],
			Comparisons: record.played[id],
		}
		// A tie contributes WeightTie to Wins, so computing the rate from the
		// weighted totals would report an all-ties record as a 100% win rate.
		// Subtracting the tie contribution leaves the decided comparisons, which
		// is what a win rate is a proportion of. A record with no decided
		// comparison leaves the rate null rather than dividing by zero.
		decidedWins := entry.Wins - float64(entry.Ties)*config.WeightTie
		decided := decidedWins + entry.Losses
		if decided > 0 {
			rate := decidedWins / decided
			entry.WinRate = &rate
		}
		entries = append(entries, entry)
	}
	sortPairwise(entries)
	for i := range entries {
		entries[i].Rank = i + 1
	}

	judges := make([]JudgeBreakdown, 0, len(record.byJudge))
	for judgeID, judgeRecord := range record.byJudge {
		judges = append(judges, JudgeBreakdown{
			JudgeID:     judgeID,
			Comparisons: judgeRecord.comparisons,
			WinRate:     round(judgeRecord.winRate(), 4),
		})
	}
	sort.Slice(judges, func(i, j int) bool { return judges[i].JudgeID < judges[j].JudgeID })

	result.Entries = entries
	result.Judges = judges
	result.Iterations = iterations
	result.Converged = converged

	// A panel where somebody never lost, and somebody never won, has no finite
	// maximum likelihood estimate: the strength ratio between them grows without
	// limit, and any number reported for it is decided by the iteration budget
	// rather than by the data.
	//
	// This is detected from the tally rather than from the size of the fitted
	// spread. A spread threshold is a proxy that fails in both directions: it does
	// not fire for a genuinely decisive panel whose truncated fit happens to stay
	// within the threshold, and it would fire for a well-determined panel whose fit
	// ran long. The structural condition is exact, and it is what the note below
	// describes.
	if neverLost, neverWon := decisiveExtremes(record); neverLost != "" && neverWon != "" {
		result.Unbounded = true
		result.Note = "this panel is decisive: " + neverLost + " never lost a comparison and " + neverWon +
			" never won one, so the maximum likelihood strength has no finite value. The strengths below are a " +
			"truncated fit, not estimates. Collect more comparisons, or use the rubric pipeline, which is " +
			"bounded by construction."
	}
	return result
}

func sortPairwise(entries []PairwiseEntry) {
	sort.Slice(entries, func(i, j int) bool {
		left, right := entries[i], entries[j]
		if left.Strength != right.Strength {
			return left.Strength > right.Strength
		}
		// A project nobody compared sorts last rather than being given an
		// arbitrary strength; an estimated strength outranks an absent one.
		leftUncompared := left.WinRate == nil
		rightUncompared := right.WinRate == nil
		if leftUncompared != rightUncompared {
			return rightUncompared
		}
		return left.ProjectID < right.ProjectID
	})
}

// pairKey identifies a meeting between two projects, order-independent so that
// a pair is recorded once regardless of which judge or which iteration saw it
// first.
type pairKey struct {
	left  string
	right string
}

func newPairKey(a, b string) pairKey {
	if a < b {
		return pairKey{left: a, right: b}
	}
	return pairKey{left: b, right: a}
}

// tally is the head-to-head record derived from the reviews.
type tally struct {
	// wins and losses are fractional so a tie can contribute a half to each.
	wins   map[string]float64
	losses map[string]float64
	ties   map[string]int
	// meetings counts, for each unordered pair, how many judges compared them.
	meetings map[pairKey]int
	// played is the total number of comparisons each project took part in.
	played map[string]int
	// reviewed is every project that appears in any review, including one that
	// never met another project. Without it such a project is invisible: it has
	// no tally entry, so it cannot be reported as uncompared either.
	reviewed map[string]bool
	// byJudge attributes comparisons to the judge who made them.
	byJudge map[string]judgeComparison
	// order is the deterministic iteration order of pair keys, so two runs over
	// identical input do the same arithmetic in the same sequence.
	order []pairKey
}

type judgeComparison struct {
	comparisons int
	wins        int
	losses      int
	ties        int
}

// deriveTally builds the head-to-head record from per-judge reviews.
//
// For each judge, every pair of projects that judge reviewed becomes one
// comparison, won by the higher weighted rubric score. A review missing a
// weighted criterion is excluded from head-to-head comparison rather than
// compared on the criteria it happens to carry, which would be a different
// question from the one being asked.
func deriveTally(reviews []domain.Review, weights Weights, tieWeight float64) tally {
	result := tally{
		wins:     make(map[string]float64),
		losses:   make(map[string]float64),
		ties:     make(map[string]int),
		meetings: make(map[pairKey]int),
		played:   make(map[string]int),
		reviewed: make(map[string]bool),
		byJudge:  make(map[string]judgeComparison),
	}

	byJudge := make(map[string][]domain.Review)
	for _, review := range reviews {
		byJudge[review.JudgeID] = append(byJudge[review.JudgeID], review)
		result.reviewed[review.ProjectID] = true
	}
	judgeIDs := make([]string, 0, len(byJudge))
	for judgeID := range byJudge {
		judgeIDs = append(judgeIDs, judgeID)
	}
	sort.Strings(judgeIDs)

	for _, judgeID := range judgeIDs {
		panel := byJudge[judgeID]
		sort.Slice(panel, func(i, j int) bool { return panel[i].ProjectID < panel[j].ProjectID })
		record := judgeComparison{}
		for i := 0; i < len(panel); i++ {
			left, leftOK := completeWeightedScore(panel[i], weights)
			for j := i + 1; j < len(panel); j++ {
				right, rightOK := completeWeightedScore(panel[j], weights)
				if !leftOK || !rightOK {
					continue
				}
				key := newPairKey(panel[i].ProjectID, panel[j].ProjectID)
				if result.meetings[key] == 0 {
					result.order = append(result.order, key)
				}
				result.meetings[key]++
				result.played[panel[i].ProjectID]++
				result.played[panel[j].ProjectID]++
				record.comparisons++
				switch {
				case left > right:
					result.wins[panel[i].ProjectID]++
					result.losses[panel[j].ProjectID]++
					record.wins++
				case right > left:
					result.wins[panel[j].ProjectID]++
					result.losses[panel[i].ProjectID]++
					record.losses++
				default:
					// A tie is half a win to each side, which is what keeps the
					// estimator's total weight equal to the number of meetings.
					result.ties[panel[i].ProjectID]++
					result.ties[panel[j].ProjectID]++
					result.wins[panel[i].ProjectID] += tieWeight
					result.wins[panel[j].ProjectID] += tieWeight
					result.losses[panel[i].ProjectID] += tieWeight
					result.losses[panel[j].ProjectID] += tieWeight
					record.ties++
				}
			}
		}
		if record.comparisons > 0 {
			result.byJudge[judgeID] = record
		}
	}
	sort.Slice(result.order, func(i, j int) bool {
		if result.order[i].left != result.order[j].left {
			return result.order[i].left < result.order[j].left
		}
		return result.order[i].right < result.order[j].right
	})
	return result
}

// completeWeightedScore returns a review's weighted score, and false if any
// weighted criterion is missing.
func completeWeightedScore(review domain.Review, weights Weights) (float64, bool) {
	keys := make([]string, 0, len(weights))
	for key := range weights {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	total, weightSum := 0.0, 0.0
	for _, key := range keys {
		score, ok := review.Criteria[key]
		if !ok {
			return 0, false
		}
		total += float64(score) * weights[key]
		weightSum += weights[key]
	}
	if weightSum == 0 {
		return 0, false
	}
	return total / weightSum, true
}

// fitBradleyTerry runs Hunter's MM algorithm over the tally.
//
// Each update is
//
//	pi_new[i] = credit[i] / sum over j of meetings(i,j) / (pi_i + pi_j)
//
// which is the maximiser of a concave minorant of the negative log-likelihood,
// so the sequence decreases the objective monotonically. Working in logs keeps
// pi positive without an explicit constraint.
func fitBradleyTerry(ids []string, record tally, config PairwiseConfig) (map[string]float64, int, bool) {
	// credit[i] is i's total weight won: wins plus half of each tie. Using the
	// same quantity for the numerator and for the pair totals is what makes the
	// estimator consistent under ties.
	credit := make(map[string]float64, len(ids))
	pi := make(map[string]float64, len(ids))
	for _, id := range ids {
		credit[id] = record.wins[id] + float64(record.ties[id])*config.WeightTie
		pi[id] = 1
	}
	// A project that was only ever tied has no wins, which makes its first
	// update divide by zero. Seed it from its comparison count so the estimator
	// has a starting point.
	for _, id := range ids {
		if credit[id] <= 0 {
			pi[id] = math.Sqrt(float64(record.played[id]) + 1)
		}
	}

	// Precompute each project's opponent list, so the inner loop is a sum over
	// pairs rather than a scan over every id.
	type opponent struct {
		id       string
		meetings float64
	}
	against := make(map[string][]opponent, len(ids))
	for _, key := range record.order {
		count := float64(record.meetings[key])
		against[key.left] = append(against[key.left], opponent{id: key.right, meetings: count})
		against[key.right] = append(against[key.right], opponent{id: key.left, meetings: count})
	}

	iterations, converged := 0, false
	for iterations < config.MaxIterations {
		iterations++
		maxChange := 0.0
		next := make(map[string]float64, len(ids))
		for _, id := range ids {
			denominator := 0.0
			for _, other := range against[id] {
				denominator += other.meetings / (pi[id] + pi[other.id])
			}
			updated := pi[id]
			if denominator > 0 {
				updated = credit[id] / denominator
			}
			if updated <= 0 {
				updated = pi[id]
			}
			// Damping keeps the sequence monotone under rounding, at the cost
			// of a few more iterations.
			damped := config.Damping*updated + (1-config.Damping)*pi[id]
			next[id] = damped
			// Convergence is judged only over projects the model can estimate.
			// A project that won nothing has a boundary maximum likelihood
			// estimate of zero: it is pinned by construction, so including it
			// would make the iteration count meaningless and hide a genuine
			// failure to converge on everything else.
			if credit[id] > 0 {
				if change := math.Abs(math.Log(damped) - math.Log(pi[id])); change > maxChange {
					maxChange = change
				}
			}
		}
		pi = next
		if maxChange < config.Tolerance {
			converged = true
			break
		}
	}
	return normalizeStrength(pi, ids), iterations, converged
}

// normalizeStrength fixes the scale indeterminacy by forcing the geometric mean
// of the fitted strengths to 1, and makes the strengths interpretable as
// "times as strong as the average project".
// UnboundedRatio is the fitted strength spread above which a panel is treated
// as decisive rather than measurable. Past this point the numbers are being
// decided by the iteration budget rather than by the data.
const UnboundedRatio = 1e4

// MinStrength floors a fitted strength.
//
// Bradley-Terry has a genuine boundary case: a project that wins no comparison
// has a maximum likelihood estimate of zero, and the MM iteration drives its
// strength toward zero without ever reaching it. Left alone that produces an
// unbounded strength ratio against the rest of the field and a division by zero
// downstream.
const MinStrength = 1e-6

// decisiveExtremes reports the projects that never lost and never won, if any.
//
// A tie counts as neither: a project whose only comparisons were draws has not
// beaten anyone, and treating it as unbeaten would flag a panel that is in fact
// entirely undecided.
func decisiveExtremes(record tally) (neverLost, neverWon string) {
	for id, played := range record.played {
		if played == 0 {
			continue
		}
		wins := record.wins[id] - float64(record.ties[id])*defaultTieWeight
		if record.losses[id] == 0 && wins > 0 && neverLost == "" {
			neverLost = id
		}
		if wins == 0 && record.losses[id] > 0 && neverWon == "" {
			neverWon = id
		}
	}
	return neverLost, neverWon
}

// defaultTieWeight mirrors DefaultPairwiseConfig so the win count can be
// recovered from the tally, which stores ties as half a win.
const defaultTieWeight = 0.5

// normalizeStrength fixes the scale indeterminacy by forcing the geometric mean
// of the fitted strengths to 1, which makes them interpretable as "times as
// strong as the average project", and applies MinStrength.
func normalizeStrength(pi map[string]float64, ids []string) map[string]float64 {
	if len(ids) == 0 {
		return pi
	}
	floored := make(map[string]float64, len(ids))
	for _, id := range ids {
		value := pi[id]
		// NaN and Inf both mean the iteration diverged for this project.
		if math.IsNaN(value) || math.IsInf(value, 0) || value < MinStrength {
			value = MinStrength
		}
		floored[id] = value
	}
	logSum := 0.0
	for _, id := range ids {
		logSum += math.Log(floored[id])
	}
	offset := logSum / float64(len(ids))
	out := make(map[string]float64, len(ids))
	for _, id := range ids {
		out[id] = round(math.Exp(math.Log(floored[id])-offset), 6)
	}
	return out
}

// winRate is a judge's share of decided comparisons, ignoring ties.
func (j judgeComparison) winRate() float64 {
	decided := j.wins + j.losses
	if decided == 0 {
		return 0
	}
	return float64(j.wins) / float64(decided)
}
