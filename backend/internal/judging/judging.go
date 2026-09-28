package judging

import (
	"fmt"
	"math"
	"sort"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

const (
	minimumReviewsForNormalization = 3
	minimumStandardDeviation       = 1e-9
)

type Weights map[string]float64

type ReviewScore struct {
	ReviewID        string             `json:"review_id"`
	ProjectID       string             `json:"project_id"`
	JudgeID         string             `json:"judge_id"`
	RawScore        float64            `json:"raw_score"`
	NormalizedScore float64            `json:"normalized_score"`
	LowInformation  bool               `json:"low_information"`
	Criteria        map[string]float64 `json:"criteria"`
}

type ProjectResult struct {
	ProjectID             string  `json:"project_id"`
	RawMean               float64 `json:"raw_mean"`
	NormalizedMean        float64 `json:"normalized_mean"`
	ReviewCount           int     `json:"review_count"`
	LowInformationReviews int     `json:"low_information_reviews"`
	Rank                  int     `json:"rank"`
}

type Summary struct {
	Reviews              []ReviewScore   `json:"reviews"`
	Projects             []ProjectResult `json:"projects"`
	LowInformationJudges []string        `json:"low_information_judges"`
}

func Normalize(reviews []domain.Review, weights Weights) (Summary, error) {
	if len(weights) == 0 {
		return Summary{}, fmt.Errorf("at least one criterion weight is required")
	}
	weightTotal := 0.0
	for criterion, weight := range weights {
		if criterion == "" {
			return Summary{}, fmt.Errorf("criterion name must not be empty")
		}
		if weight <= 0 {
			return Summary{}, fmt.Errorf("weight for %s must be positive", criterion)
		}
		weightTotal += weight
	}
	if weightTotal <= 0 {
		return Summary{}, fmt.Errorf("criterion weights must have a positive total")
	}

	valuesByJudge := make(map[string]map[string][]float64)
	for _, review := range reviews {
		if review.JudgeID == "" || review.ProjectID == "" {
			return Summary{}, fmt.Errorf("review judge and project are required")
		}
		if _, ok := valuesByJudge[review.JudgeID]; !ok {
			valuesByJudge[review.JudgeID] = make(map[string][]float64)
		}
		for criterion := range weights {
			score, ok := review.Criteria[criterion]
			if !ok {
				return Summary{}, fmt.Errorf("review %s is missing criterion %s", review.ID, criterion)
			}
			if score < 1 || score > 5 {
				return Summary{}, fmt.Errorf("review %s has invalid score for %s", review.ID, criterion)
			}
			valuesByJudge[review.JudgeID][criterion] = append(valuesByJudge[review.JudgeID][criterion], float64(score))
		}
	}

	lowInformationJudges := make(map[string]struct{})
	summary := Summary{Reviews: make([]ReviewScore, 0, len(reviews))}
	for _, review := range reviews {
		rawScore := 0.0
		normalizedScore := 0.0
		criteria := make(map[string]float64, len(weights))
		lowInformation := false
		for criterion, weight := range weights {
			score := float64(review.Criteria[criterion])
			rawScore += score * weight
			values := valuesByJudge[review.JudgeID][criterion]
			normalized, judgeLowInformation := normalizeCriterion(score, values)
			if judgeLowInformation {
				lowInformation = true
				lowInformationJudges[review.JudgeID] = struct{}{}
			}
			criteria[criterion] = normalized
			normalizedScore += normalized * weight
		}
		summary.Reviews = append(summary.Reviews, ReviewScore{
			ReviewID:        review.ID,
			ProjectID:       review.ProjectID,
			JudgeID:         review.JudgeID,
			RawScore:        rawScore / weightTotal,
			NormalizedScore: normalizedScore / weightTotal,
			LowInformation:  lowInformation,
			Criteria:        criteria,
		})
	}

	projects := make(map[string]*ProjectResult)
	for _, review := range summary.Reviews {
		project, ok := projects[review.ProjectID]
		if !ok {
			project = &ProjectResult{ProjectID: review.ProjectID}
			projects[review.ProjectID] = project
		}
		project.RawMean += review.RawScore
		project.NormalizedMean += review.NormalizedScore
		project.ReviewCount++
		if review.LowInformation {
			project.LowInformationReviews++
		}
	}
	summary.Projects = make([]ProjectResult, 0, len(projects))
	for _, project := range projects {
		project.RawMean /= float64(project.ReviewCount)
		project.NormalizedMean /= float64(project.ReviewCount)
		summary.Projects = append(summary.Projects, *project)
	}
	sort.Slice(summary.Projects, func(i, j int) bool {
		left, right := summary.Projects[i], summary.Projects[j]
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
	for index := range summary.Projects {
		summary.Projects[index].Rank = index + 1
	}
	for judge := range lowInformationJudges {
		summary.LowInformationJudges = append(summary.LowInformationJudges, judge)
	}
	sort.Strings(summary.LowInformationJudges)
	return summary, nil
}

func normalizeCriterion(score float64, values []float64) (float64, bool) {
	if len(values) < minimumReviewsForNormalization {
		return 50, true
	}
	mean := 0.0
	for _, value := range values {
		mean += value
	}
	mean /= float64(len(values))
	variance := 0.0
	for _, value := range values {
		difference := value - mean
		variance += difference * difference
	}
	standardDeviation := math.Sqrt(variance / float64(len(values)))
	if standardDeviation < minimumStandardDeviation {
		return 50, true
	}
	z := (score - mean) / standardDeviation
	return 100 / (1 + math.Exp(-z)), false
}
