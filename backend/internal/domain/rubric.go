package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type RubricStatus string

const (
	RubricDraft     RubricStatus = "draft"
	RubricPublished RubricStatus = "published"
	RubricArchived  RubricStatus = "archived"
)

type RubricCriterion struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	MinScore    int    `json:"min_score"`
	MaxScore    int    `json:"max_score"`
	Weight      int    `json:"weight"`
	Required    bool   `json:"required"`
}

type Rubric struct {
	ID           string            `json:"id"`
	EventID      string            `json:"event_id"`
	TrackID      string            `json:"track_id,omitempty"`
	Name         string            `json:"name"`
	Version      int               `json:"version"`
	Status       RubricStatus      `json:"status"`
	Criteria     []RubricCriterion `json:"criteria"`
	CreatedBy    string            `json:"created_by"`
	CreatedAt    time.Time         `json:"created_at"`
	PublishedAt  *time.Time        `json:"published_at,omitempty"`
	ArchivedAt   *time.Time        `json:"archived_at,omitempty"`
	Instructions string            `json:"instructions,omitempty"`
}

func (r Rubric) Clone() Rubric {
	copyRubric := r
	copyRubric.Criteria = append([]RubricCriterion(nil), r.Criteria...)
	return copyRubric
}

func (r Rubric) TotalWeight() int {
	total := 0
	for _, criterion := range r.Criteria {
		total += criterion.Weight
	}
	return total
}

func (r Rubric) Criterion(key string) (RubricCriterion, bool) {
	for _, criterion := range r.Criteria {
		if criterion.Key == key {
			return criterion, true
		}
	}
	return RubricCriterion{}, false
}

func (r Rubric) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("%w: rubric name is required", ErrValidation)
	}
	if len(r.Criteria) == 0 {
		return fmt.Errorf("%w: at least one criterion is required", ErrValidation)
	}
	seen := map[string]struct{}{}
	total := 0
	for index, criterion := range r.Criteria {
		key := strings.TrimSpace(criterion.Key)
		if key == "" {
			return fmt.Errorf("%w: criterion %d is missing a key", ErrValidation, index+1)
		}
		if _, ok := seen[key]; ok {
			return fmt.Errorf("%w: duplicate criterion key %s", ErrValidation, key)
		}
		seen[key] = struct{}{}
		if strings.TrimSpace(criterion.Label) == "" {
			return fmt.Errorf("%w: criterion %s is missing a label", ErrValidation, key)
		}
		if criterion.MinScore < 0 || criterion.MaxScore <= criterion.MinScore {
			return fmt.Errorf("%w: criterion %s has an invalid score range", ErrValidation, key)
		}
		if criterion.Weight <= 0 {
			return fmt.Errorf("%w: criterion %s must have a positive weight", ErrValidation, key)
		}
		total += criterion.Weight
	}
	if total != 100 {
		return fmt.Errorf("%w: criterion weights must total 100, got %d", ErrValidation, total)
	}
	return nil
}

func (r Rubric) ValidateScores(scores map[string]int) error {
	if len(scores) == 0 {
		return fmt.Errorf("%w: at least one criterion score is required", ErrValidation)
	}
	for key, score := range scores {
		criterion, ok := r.Criterion(key)
		if !ok {
			return fmt.Errorf("%w: %s is not part of rubric %s", ErrValidation, key, r.Name)
		}
		if score < criterion.MinScore || score > criterion.MaxScore {
			return fmt.Errorf("%w: score for %s must be between %d and %d", ErrValidation, key, criterion.MinScore, criterion.MaxScore)
		}
	}
	for _, criterion := range r.Criteria {
		if !criterion.Required {
			continue
		}
		if _, ok := scores[criterion.Key]; !ok {
			return fmt.Errorf("%w: %s is required", ErrValidation, criterion.Key)
		}
	}
	return nil
}

func (r Rubric) NormalizeScores(scores map[string]int) (map[string]int, error) {
	if err := r.ValidateScores(scores); err != nil {
		return nil, err
	}
	normalized := make(map[string]int, len(r.Criteria))
	for key, score := range scores {
		criterion, _ := r.Criterion(key)
		span := criterion.MaxScore - criterion.MinScore
		if span <= 0 {
			normalized[key] = score
			continue
		}
		scaled := 100 * (score - criterion.MinScore) / span
		normalized[key] = scaled
	}
	return normalized, nil
}

func SortRubrics(rubrics []Rubric) {
	sort.SliceStable(rubrics, func(i, j int) bool {
		if rubrics[i].TrackID == rubrics[j].TrackID {
			return rubrics[i].Version > rubrics[j].Version
		}
		return rubrics[i].TrackID < rubrics[j].TrackID
	})
}

// MinScale is the lowest score any criterion accepts, and MaxScale the highest.
//
// The rubric does not store a global scale: each criterion carries its own
// range, and a rubric with mixed ranges is legitimate. These helpers report the
// widest range the rubric permits so a renderer can size a control once.
func (r Rubric) MinScale() int {
	if len(r.Criteria) == 0 {
		return 1
	}
	lowest := r.Criteria[0].MinScore
	for _, criterion := range r.Criteria {
		if criterion.MinScore < lowest {
			lowest = criterion.MinScore
		}
	}
	return lowest
}

func (r Rubric) MaxScale() int {
	if len(r.Criteria) == 0 {
		return 5
	}
	highest := r.Criteria[0].MaxScore
	for _, criterion := range r.Criteria {
		if criterion.MaxScore > highest {
			highest = criterion.MaxScore
		}
	}
	return highest
}
