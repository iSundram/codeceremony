// Package judging turns raw review scores into a defensible ranking.
//
// # Why normalization is needed
//
// Judges do not share a scale. One hands out 5s for work they admire, another
// hands out 3s for the same work. Averaging raw scores therefore measures the
// judges' temperaments at least as much as it measures the projects, and it
// rewards a panel that happens to be uniformly generous.
//
// The method here removes the judge effect before aggregating, and then reports
// how much the ranking can actually be trusted. The second half matters as much
// as the first: on a typical hackathon panel the top ten projects are not
// statistically separable, and a platform that prints a confident ordered
// leaderboard is asserting more than its data supports.
//
// # The method, step by step
//
//  1. Calibration. For each judge j and criterion c, take that judge's own
//     scores across every project they reviewed, and compute the mean mu and
//     population standard deviation sigma. A project's score is then expressed
//     as z = (s - mu) / sigma: how far this project sits from where this judge
//     normally lands on this criterion.
//
//  2. Shrinkage. A judge with one review, or whose scores barely vary, produces
//     a z-score that is mostly noise. Each z is multiplied by a reliability
//     factor lam = n / (n + k), where n is that judge's review count on that
//     criterion and k is a prior strength. Few reviews means lam approaches 0
//     and the judge contributes close to neutral; many reviews means lam
//     approaches 1 and the judge is taken at full value. This is a smooth
//     alternative to the usual hard cutoff at some minimum review count, which
//     throws away a judge's entire contribution at a threshold and produces a
//     cliff in the ranking.
//
//  3. Squash. The shrunken z is mapped through the logistic function to a
//     0-100 scale: 100 / (1 + exp(-z)). The logistic is strictly increasing,
//     so it never reorders anything within a single judge, but it is bounded,
//     so one judge who uses a 1-5 range with a spread of 0.2 cannot dominate
//     the panel through sheer z magnitude. It also puts every project on a
//     comparable 0-100 scale, where 50 is exactly this judge's own average.
//
//  4. Aggregation. Per review, the criterion values are combined with the
//     organizer's rubric weights. Per project, the reviews are averaged with
//     equal weight per judge: every judge is one voice, regardless of how many
//     projects they happened to be assigned.
//
//  5. Uncertainty. Judges, not reviews, are the independent unit. Reviews
//     written by one judge are correlated by construction, which is precisely
//     what step 1 removes, so resampling individual reviews would report
//     intervals that are far too narrow. The interval is therefore a bootstrap
//     over judges: resample the panel with replacement, rerun steps 1-4, and
//     take percentiles of each project's score.
//
// # What the method does not claim
//
// Normalization equalizes judges' scales. It does not correct for genuine
// disagreement about what a good project is, and it cannot manufacture signal
// where a panel did not provide any. A project ranked first with a confidence
// interval that overlaps the project ranked third has not been separated from
// third place, and the API reports that rather than hiding it behind a rank
// number. Separable is the field to read.
package judging

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

// DefaultMethod names the normalization pipeline, and DefaultMethodVersion
// identifies the exact arithmetic within it. Both are reported in every results
// response so an archived result set can be recomputed under the method that
// produced it. The version is bumped whenever the arithmetic changes; the name
// changes only when the shape of the method does.
const (
	DefaultMethod        = "judge-zscore-shrink-logistic"
	DefaultMethodVersion = "1"
)

const (
	// defaultShrinkageK is the prior strength for review-count shrinkage. A
	// judge with k reviews sits at exactly half reliability. Two is chosen so
	// that the judges a small panel actually has (often 1-3 reviews each) are
	// meaningfully discounted without being discarded.
	defaultShrinkageK = 2.0
	// defaultMinStdDev floors sigma so that dividing by a spread that is
	// numerically present but practically zero cannot manufacture a huge z.
	defaultMinStdDev = 1e-6
	// defaultResamples is the bootstrap draw count. 2000 is enough for stable
	// 5th and 95th percentiles without making a results request slow.
	defaultResamples = 2000
	// defaultConfidence is the two-sided coverage of the reported interval.
	defaultConfidence = 0.90
)

// Config tunes the pipeline. The zero value is not usable; start from
// DefaultConfig so that a future field addition is not silently zero.
type Config struct {
	// Method is the pipeline identifier reported to callers.
	Method string
	// ShrinkageK is the prior strength in lam = n / (n + k).
	ShrinkageK float64
	// MinStdDev is the floor applied to per-judge sigma.
	MinStdDev float64
	// Resamples is the bootstrap draw count over judges.
	Resamples int
	// Confidence is the two-sided coverage of the reported interval, in (0, 1).
	Confidence float64
	// Seed makes the bootstrap deterministic, which matters because a results
	// endpoint that reshuffles its confidence intervals on every request is
	// unauditable.
	Seed int64
}

// DefaultConfig returns the shipped method's parameters.
func DefaultConfig() Config {
	return Config{
		Method:     DefaultMethod,
		ShrinkageK: defaultShrinkageK,
		MinStdDev:  defaultMinStdDev,
		Resamples:  defaultResamples,
		Confidence: defaultConfidence,
		Seed:       0x5EED_C0DE,
	}
}

func (c Config) withDefaults() Config {
	defaults := DefaultConfig()
	if c.Method == "" {
		c.Method = defaults.Method
	}
	if c.ShrinkageK <= 0 {
		c.ShrinkageK = defaults.ShrinkageK
	}
	if c.MinStdDev <= 0 {
		c.MinStdDev = defaults.MinStdDev
	}
	if c.Resamples <= 0 {
		c.Resamples = defaults.Resamples
	}
	if c.Confidence <= 0 || c.Confidence >= 1 {
		c.Confidence = defaults.Confidence
	}
	return c
}

// Weights are the organizer's rubric weights, keyed by criterion.
type Weights map[string]float64

// ReviewScore is one review after normalization.
type ReviewScore struct {
	ReviewID        string             `json:"review_id"`
	ProjectID       string             `json:"project_id"`
	JudgeID         string             `json:"judge_id"`
	RawScore        float64            `json:"raw_score"`
	NormalizedScore float64            `json:"normalized_score"`
	LowInformation  bool               `json:"low_information"`
	Partial         bool               `json:"partial"`
	MissingCriteria []string           `json:"missing_criteria,omitempty"`
	Criteria        map[string]float64 `json:"criteria"`
}

// JudgeDiagnostic explains how much this judge was trusted and why. It is what
// makes the normalization auditable rather than a black box.
type JudgeDiagnostic struct {
	JudgeID         string             `json:"judge_id"`
	Reviews         int                `json:"reviews"`
	Reliability     float64            `json:"reliability"`
	LowInformation  bool               `json:"low_information"`
	Reasons         []string           `json:"reasons,omitempty"`
	CriteriaMean    map[string]float64 `json:"criteria_mean,omitempty"`
	CriteriaStdDev  map[string]float64 `json:"criteria_stddev,omitempty"`
	CriteriaSamples map[string]int     `json:"criteria_samples,omitempty"`
}

// ProjectResult is one project's outcome, including how confidently it can be
// placed.
type ProjectResult struct {
	ProjectID             string  `json:"project_id"`
	RawMean               float64 `json:"raw_mean"`
	NormalizedMean        float64 `json:"normalized_mean"`
	Low                   float64 `json:"low"`
	High                  float64 `json:"high"`
	ReviewCount           int     `json:"review_count"`
	LowInformationReviews int     `json:"low_information_reviews"`
	Rank                  int     `json:"rank"`
	// Separable is false when this project's confidence interval overlaps the
	// project ranked immediately above it.
	//
	// This is deliberately a statement about one adjacent pair and not about a
	// group. A run of projects whose neighbours overlap is a chain, and every
	// member of a long chain shares the same TieGroup even though the top of
	// the chain and the bottom of it are plainly not tied. Reading TieGroup as
	// "these are all equivalent" would overstate what the data supports, so the
	// API publishes the pairwise flag and the rank the project failed to beat,
	// and the console words it that way.
	Separable bool `json:"separable"`
	// TieGroup numbers consecutive non-separable projects from 1. It is a
	// display grouping, not a claim of equivalence.
	TieGroup int `json:"tie_group"`
	// PreviousRank is the rank of the project directly above, or 0 for the top
	// project. Combined with Separable it says exactly what was and was not
	// established.
	PreviousRank int `json:"previous_rank"`
}

// Summary is the full, reproducible result of a normalization run.
type Summary struct {
	Method               string            `json:"method"`
	MethodVersion        string            `json:"method_version"`
	Confidence           float64           `json:"confidence"`
	Resamples            int               `json:"resamples"`
	Reviews              []ReviewScore     `json:"reviews"`
	Projects             []ProjectResult   `json:"projects"`
	LowInformationJudges []string          `json:"low_information_judges"`
	Judges               []JudgeDiagnostic `json:"judges"`
	// CriterionStats records the observed range per criterion, so a caller can
	// see that judges used the whole scale or squashed into the middle.
	CriterionStats map[string]CriterionStat `json:"criterion_stats"`
}

// IntervalWidth is the width of the confidence interval in normalized points.
// It exists so a renderer can size a bar without repeating the arithmetic.
func (p ProjectResult) IntervalWidth() float64 {
	width := p.High - p.Low
	if width < 0 {
		return 0
	}
	return width
}

// MeanText summarises a judge's per-criterion means in one string, so a table
// can show "what this judge normally awards" without a nested loop.
func (d JudgeDiagnostic) MeanText() string { return summarize(d.CriteriaMean) }

// StdDevText summarises a judge's per-criterion spreads in one string.
func (d JudgeDiagnostic) StdDevText() string { return summarize(d.CriteriaStdDev) }

func summarize(values map[string]float64) string {
	if len(values) == 0 {
		return "—"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s %.2f", key, values[key]))
	}
	return strings.Join(parts, ", ")
}

// CriterionStat is the observed distribution of one criterion.
type CriterionStat struct {
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Mean   float64 `json:"mean"`
	StdDev float64 `json:"stddev"`
	Count  int     `json:"count"`
}

// calibration is one (judge, criterion) cell of the calibration table.
type calibration struct {
	mean       float64
	stdDev     float64
	samples    int
	degenerate bool
}

// Normalize runs the default pipeline. See the package comment for the method.
func Normalize(reviews []domain.Review, weights Weights) (Summary, error) {
	return NormalizeConfigured(reviews, weights, DefaultConfig())
}

// NormalizeConfigured runs the pipeline with explicit parameters.
func NormalizeConfigured(reviews []domain.Review, weights Weights, config Config) (Summary, error) {
	config = config.withDefaults()

	normalizedWeights, total, err := validateWeights(weights)
	if err != nil {
		return Summary{}, err
	}
	clean, err := usableReviews(reviews)
	if err != nil {
		return Summary{}, err
	}
	if len(clean) == 0 {
		return Summary{
			Method: config.Method, MethodVersion: DefaultMethodVersion, Confidence: config.Confidence,
			Resamples: config.Resamples, Reviews: []ReviewScore{}, Projects: []ProjectResult{},
			LowInformationJudges: []string{}, Judges: []JudgeDiagnostic{}, CriterionStats: map[string]CriterionStat{},
		}, nil
	}

	table, judgeIDs := buildCalibration(clean, normalizedWeights)
	scored := scoreReviews(clean, table, normalizedWeights, total, config)
	projects := aggregate(scored, total)

	attachUncertainty(projects, clean, normalizedWeights, total, config)
	annotateSeparability(projects)

	lowInfo := make([]string, 0, len(judgeIDs))
	diagnostics := make([]JudgeDiagnostic, 0, len(judgeIDs))
	for _, judgeID := range judgeIDs {
		diagnostic := diagnose(table, judgeID, normalizedWeights, config)
		diagnostics = append(diagnostics, diagnostic)
		if diagnostic.LowInformation {
			lowInfo = append(lowInfo, judgeID)
		}
	}
	sort.Strings(lowInfo)
	sort.Slice(diagnostics, func(i, j int) bool { return diagnostics[i].JudgeID < diagnostics[j].JudgeID })

	return Summary{
		Method:               config.Method,
		MethodVersion:        DefaultMethodVersion,
		Confidence:           config.Confidence,
		Resamples:            config.Resamples,
		Reviews:              scored,
		Projects:             projects,
		LowInformationJudges: lowInfo,
		Judges:               diagnostics,
		CriterionStats:       criterionStats(clean, normalizedWeights),
	}, nil
}

func validateWeights(weights Weights) (Weights, float64, error) {
	if len(weights) == 0 {
		return nil, 0, fmt.Errorf("at least one criterion weight is required")
	}
	total := 0.0
	for criterion, weight := range weights {
		if criterion == "" {
			return nil, 0, fmt.Errorf("criterion name must not be empty")
		}
		if weight <= 0 {
			return nil, 0, fmt.Errorf("weight for %s must be positive", criterion)
		}
		total += weight
	}
	if total <= 0 {
		return nil, 0, fmt.Errorf("criterion weights must have a positive total")
	}
	return weights, total, nil
}

// usableReviews drops rows that cannot be attributed to a judge and a project,
// and blank criterion maps. It deliberately does not reject a review for
// missing a criterion: partial reviews are the normal state of a live panel and
// the pipeline imputes them neutrally. Rejecting them instead would make the
// results endpoint fail closed the moment one judge skipped a criterion.
func usableReviews(reviews []domain.Review) ([]domain.Review, error) {
	clean := make([]domain.Review, 0, len(reviews))
	for _, review := range reviews {
		if review.JudgeID == "" || review.ProjectID == "" {
			return nil, fmt.Errorf("review %s is missing a judge or project", review.ID)
		}
		if len(review.Criteria) == 0 {
			continue
		}
		clean = append(clean, review)
	}
	return clean, nil
}

func buildCalibration(reviews []domain.Review, weights Weights) (map[string]map[string]calibration, []string) {
	table := make(map[string]map[string]calibration)
	for _, review := range reviews {
		if _, ok := table[review.JudgeID]; !ok {
			table[review.JudgeID] = make(map[string]calibration, len(weights))
		}
		for criterion := range weights {
			score, ok := review.Criteria[criterion]
			if !ok {
				continue
			}
			cell := table[review.JudgeID][criterion]
			if cell.samples == 0 {
				cell = calibration{mean: float64(score), samples: 1}
			} else {
				cell.mean += float64(score)
				cell.samples++
			}
			table[review.JudgeID][criterion] = cell
		}
	}
	judgeIDs := make([]string, 0, len(table))
	for judgeID, cells := range table {
		for criterion, cell := range cells {
			cell.mean /= float64(cell.samples)
			cells[criterion] = cell
		}
		judgeIDs = append(judgeIDs, judgeID)
	}
	// Second pass for sigma now that every mean is final.
	for _, review := range reviews {
		cells := table[review.JudgeID]
		for criterion := range weights {
			score, ok := review.Criteria[criterion]
			if !ok {
				continue
			}
			cell := cells[criterion]
			difference := float64(score) - cell.mean
			cell.stdDev += difference * difference
			cells[criterion] = cell
		}
	}
	for judgeID, cells := range table {
		for criterion, cell := range cells {
			cell.stdDev = math.Sqrt(cell.stdDev / float64(cell.samples))
			cell.degenerate = cell.stdDev < defaultMinStdDev
			cells[criterion] = cell
		}
		table[judgeID] = cells
	}
	sort.Strings(judgeIDs)
	return table, judgeIDs
}

func scoreReviews(reviews []domain.Review, table map[string]map[string]calibration, weights Weights, total float64, config Config) []ReviewScore {
	// Iterate criteria in a fixed order. Floating-point addition is not
	// associative, so summing in Go's randomized map order would make the
	// published score differ in the last bits between identical runs and
	// quietly break the reproducibility this package promises.
	criteria := make([]string, 0, len(weights))
	for criterion := range weights {
		criteria = append(criteria, criterion)
	}
	sort.Strings(criteria)

	scored := make([]ReviewScore, 0, len(reviews))
	for _, review := range reviews {
		cells := table[review.JudgeID]
		raw := 0.0
		normalized := 0.0
		values := make(map[string]float64, len(weights))
		var missing []string
		lowInformation := false
		for _, criterion := range criteria {
			weight := weights[criterion]
			cell, present := cells[criterion]
			if present {
				if cell.degenerate {
					// This judge gave the same score for this criterion to
					// every project, so there is no signal to rescale.
					lowInformation = true
				}
				raw += float64(review.Criteria[criterion]) * weight
				values[criterion] = rescale(review.Criteria[criterion], cell, config)
			} else {
				// The judge expressed no opinion on this criterion. Impute
				// their own average, which is a z of 0 and therefore exactly
				// neutral, rather than dropping the review and silently
				// changing who is voting.
				values[criterion] = neutralScore
				missing = append(missing, criterion)
			}
			normalized += values[criterion] * weight
		}
		sort.Strings(missing)
		scored = append(scored, ReviewScore{
			ReviewID:        review.ID,
			ProjectID:       review.ProjectID,
			JudgeID:         review.JudgeID,
			RawScore:        raw / total,
			NormalizedScore: normalized / total,
			LowInformation:  lowInformation,
			Partial:         len(missing) > 0,
			MissingCriteria: missing,
			Criteria:        values,
		})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].ProjectID != scored[j].ProjectID {
			return scored[i].ProjectID < scored[j].ProjectID
		}
		return scored[i].ReviewID < scored[j].ReviewID
	})
	return scored
}

func aggregate(scored []ReviewScore, total float64) []ProjectResult {
	byProject := make(map[string]*ProjectResult)
	for _, review := range scored {
		project, ok := byProject[review.ProjectID]
		if !ok {
			project = &ProjectResult{ProjectID: review.ProjectID}
			byProject[review.ProjectID] = project
		}
		project.RawMean += review.RawScore
		project.NormalizedMean += review.NormalizedScore
		project.ReviewCount++
		if review.LowInformation {
			project.LowInformationReviews++
		}
	}
	projects := make([]ProjectResult, 0, len(byProject))
	for _, project := range byProject {
		project.RawMean /= float64(project.ReviewCount)
		project.NormalizedMean /= float64(project.ReviewCount)
		projects = append(projects, *project)
	}
	sortProjects(projects)
	for i := range projects {
		projects[i].Rank = i + 1
	}
	return projects
}

// sortProjects orders by normalized mean, then raw mean, then more reviews, then
// id. Every tiebreak is deterministic so two organizers looking at the same
// data see the same order.
func sortProjects(projects []ProjectResult) {
	sort.Slice(projects, func(i, j int) bool {
		left, right := projects[i], projects[j]
		if left.NormalizedMean != right.NormalizedMean {
			return left.NormalizedMean > right.NormalizedMean
		}
		if left.RawMean != right.RawMean {
			return left.RawMean > right.RawMean
		}
		if left.ReviewCount != right.ReviewCount {
			return left.ReviewCount > right.ReviewCount
		}
		return left.ProjectID < right.ProjectID
	})
}

// attachUncertainty resamples the panel with replacement to put an interval on
// every project's normalized mean. Judges are the resampling unit; see the
// package comment for why individual reviews are not.
func attachUncertainty(projects []ProjectResult, reviews []domain.Review, weights Weights, total float64, config Config) {
	byJudge := make(map[string][]domain.Review)
	for _, review := range reviews {
		byJudge[review.JudgeID] = append(byJudge[review.JudgeID], review)
	}
	judgeIDs := make([]string, 0, len(byJudge))
	for judgeID := range byJudge {
		judgeIDs = append(judgeIDs, judgeID)
	}
	sort.Strings(judgeIDs)

	rng := newRand(config.Seed)
	draws := make(map[string][]float64, len(projects))
	for draw := 0; draw < config.Resamples; draw++ {
		sample := make([]domain.Review, 0, len(reviews))
		for i := 0; i < len(judgeIDs); i++ {
			// A judge drawn k times contributes k copies of their whole block
			// of reviews. Repeating the block rather than the row keeps the
			// judge effect inside the resample, which is the correlation the
			// interval is supposed to account for.
			pick := judgeIDs[rng.intn(len(judgeIDs))]
			for _, review := range byJudge[pick] {
				sample = append(sample, review)
			}
		}
		table, _ := buildCalibration(sample, weights)
		for _, review := range scoreReviews(sample, table, weights, total, config) {
			draws[review.ProjectID] = append(draws[review.ProjectID], review.NormalizedScore)
		}
	}

	alpha := (1 - config.Confidence) / 2
	for i := range projects {
		values := draws[projects[i].ProjectID]
		if len(values) == 0 {
			projects[i].Low = projects[i].NormalizedMean
			projects[i].High = projects[i].NormalizedMean
			continue
		}
		sort.Float64s(values)
		projects[i].Low = percentile(values, alpha)
		projects[i].High = percentile(values, 1-alpha)
		if projects[i].Low > projects[i].NormalizedMean {
			projects[i].Low = projects[i].NormalizedMean
		}
		if projects[i].High < projects[i].NormalizedMean {
			projects[i].High = projects[i].NormalizedMean
		}
	}
}

// annotateSeparability marks a project as not separated from the one above it
// when their intervals overlap, and numbers the resulting tie groups. Reading
// Separable is how a caller tells "ranked 4th" from "in a group of six that all
// scored the same".
func annotateSeparability(projects []ProjectResult) {
	group := 0
	for i := range projects {
		projects[i].Separable = true
		projects[i].TieGroup = 0
	}
	for i := 1; i < len(projects); i++ {
		// Mutate in place. Assigning to a copy here is how separability
		// silently stops being recorded. The test is a genuine two-sided
		// interval overlap, so a project far below the one above it is
		// separable even though its own interval is wide.
		if projects[i].Low <= projects[i-1].High && projects[i-1].Low <= projects[i].High {
			projects[i].Separable = false
		}
	}
	for i := range projects {
		if projects[i].Separable {
			group++
			projects[i].TieGroup = group
			continue
		}
		projects[i].TieGroup = group
	}
}

func diagnose(table map[string]map[string]calibration, judgeID string, weights Weights, config Config) JudgeDiagnostic {
	cells := table[judgeID]
	diagnostic := JudgeDiagnostic{
		JudgeID:         judgeID,
		CriteriaMean:    make(map[string]float64, len(weights)),
		CriteriaStdDev:  make(map[string]float64, len(weights)),
		CriteriaSamples: make(map[string]int, len(weights)),
	}
	total := 0
	for criterion := range weights {
		cell, ok := cells[criterion]
		if !ok {
			continue
		}
		diagnostic.CriteriaMean[criterion] = round(cell.mean, 4)
		diagnostic.CriteriaStdDev[criterion] = round(cell.stdDev, 4)
		diagnostic.CriteriaSamples[criterion] = cell.samples
		if cell.samples > total {
			total = cell.samples
		}
		if cell.degenerate {
			diagnostic.LowInformation = true
			diagnostic.Reasons = append(diagnostic.Reasons,
				fmt.Sprintf("%s: every reviewed project received the same score", criterion))
		}
	}
	diagnostic.Reviews = total
	diagnostic.Reliability = round(reliability(total, config.ShrinkageK), 4)
	if total > 0 && total <= 1 {
		diagnostic.LowInformation = true
		diagnostic.Reasons = append(diagnostic.Reasons, "a single review cannot establish this judge's scale")
	}
	return diagnostic
}

func criterionStats(reviews []domain.Review, weights Weights) map[string]CriterionStat {
	stats := make(map[string]CriterionStat, len(weights))
	for criterion := range weights {
		values := make([]float64, 0, len(reviews))
		for _, review := range reviews {
			if score, ok := review.Criteria[criterion]; ok {
				values = append(values, float64(score))
			}
		}
		if len(values) == 0 {
			continue
		}
		sum := 0.0
		min, max := values[0], values[0]
		for _, value := range values {
			sum += value
			if value < min {
				min = value
			}
			if value > max {
				max = value
			}
		}
		mean := sum / float64(len(values))
		variance := 0.0
		for _, value := range values {
			variance += (value - mean) * (value - mean)
		}
		stats[criterion] = CriterionStat{
			Min: min, Max: max, Mean: round(mean, 4),
			StdDev: round(math.Sqrt(variance/float64(len(values))), 4),
			Count:  len(values),
		}
	}
	return stats
}

// rescale maps one raw criterion score to the 0-100 normalized scale.
func rescale(score int, cell calibration, config Config) float64 {
	if cell.samples == 0 || cell.degenerate {
		return neutralScore
	}
	z := (float64(score) - cell.mean) / math.Max(cell.stdDev, config.MinStdDev)
	z *= reliability(cell.samples, config.ShrinkageK)
	return 100 / (1 + math.Exp(-z))
}

// reliability is the shrinkage factor lam = n / (n + k).
func reliability(samples int, k float64) float64 {
	if samples <= 0 || k <= 0 {
		return 0
	}
	return float64(samples) / (float64(samples) + k)
}

// neutralScore is the normalized value of "exactly this judge's average", and
// of any contribution that carries no information.
const neutralScore = 50.0

// percentile returns the linear-interpolated quantile of a sorted slice.
func percentile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	if q <= 0 {
		return sorted[0]
	}
	if q >= 1 {
		return sorted[len(sorted)-1]
	}
	position := q * float64(len(sorted)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return sorted[lower]
	}
	fraction := position - float64(lower)
	return sorted[lower] + (sorted[upper]-sorted[lower])*fraction
}

func round(value float64, places int) float64 {
	shift := math.Pow(10, float64(places))
	return math.Round(value*shift) / shift
}
