package store

import (
	"fmt"
	"sort"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

// RecordComparison stores a head-to-head verdict.
//
// A judge comparing the same two projects again updates the existing row rather
// than adding a second one. That is a deliberate choice: a judge who changes
// their mind has answered the question once, and the later answer is the one that
// counts. Keeping both would let a single judge contribute two verdicts to the
// same match, which the estimator would weight as if it were two judges.
func (s *Store) RecordComparison(comparison domain.Comparison) (domain.Comparison, error) {
	if err := s.validateComparison(comparison); err != nil {
		return domain.Comparison{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	comparison.UpdatedAt = now
	if existing, ok := s.findComparisonLocked(comparison); ok {
		// The pair is stored as written, so a judge who swaps the two sides is
		// updating the row they already answered rather than starting a new one.
		// Keeping the new orientation is what makes a change of mind visible
		// rather than a silently different row.
		comparison.ID = existing.ID
		comparison.CreatedAt = existing.CreatedAt
	} else {
		comparison.ID = domain.NewID("cmp")
		comparison.CreatedAt = now
	}
	s.comparisons[comparison.ID] = comparison
	return comparison, nil
}

// findComparisonLocked locates an existing verdict for the same matchup.
//
// The scan is deliberate. A second index keyed by the unordered pair would have to
// be kept consistent with the id map on every write and every restore, and the
// number of comparisons one judge has in one event is small enough that the scan
// is not the thing worth optimising.
func (s *Store) findComparisonLocked(comparison domain.Comparison) (domain.Comparison, bool) {
	for _, existing := range s.comparisons {
		if existing.JudgeID != comparison.JudgeID || existing.EventID != comparison.EventID {
			continue
		}
		samePair := (existing.Left == comparison.Left && existing.Right == comparison.Right) ||
			(existing.Left == comparison.Right && existing.Right == comparison.Left)
		if samePair {
			return existing, true
		}
	}
	return domain.Comparison{}, false
}

// validationError wraps a message so callers can classify it with errors.Is
// while the text still names the specific problem. Returning a bare error here
// would reach an HTTP handler as a 500, and a bad verdict is the caller's
// mistake, not the server's.
func validationError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", domain.ErrValidation, fmt.Sprintf(format, args...))
}

func (s *Store) validateComparison(comparison domain.Comparison) error {
	if comparison.JudgeID == "" {
		return validationError("a comparison needs a judge")
	}
	if comparison.Left == "" || comparison.Right == "" {
		return validationError("a comparison needs two projects")
	}
	if comparison.Left == comparison.Right {
		// A project compared with itself carries no information, and admitting it
		// would put a half-win tie in the tally for no reason.
		return validationError("a project cannot be compared with itself")
	}
	if !comparison.Verdict.Valid() {
		return validationError("verdict must be left, right or tie")
	}
	if _, err := s.EventByID(comparison.EventID); err != nil {
		return domain.ErrNotFound
	}
	left, err := s.SubmissionByID(comparison.Left)
	if err != nil {
		return domain.ErrNotFound
	}
	if left.EventID != comparison.EventID {
		return validationError("a project from another event cannot be compared here")
	}
	right, err := s.SubmissionByID(comparison.Right)
	if err != nil {
		return domain.ErrNotFound
	}
	if right.EventID != comparison.EventID {
		return validationError("a project from another event cannot be compared here")
	}
	return nil
}

// ComparisonsForEvent returns every recorded verdict for an event, in a stable
// order so that two identical requests produce identical output.
func (s *Store) ComparisonsForEvent(eventID string) []domain.Comparison {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Comparison, 0, len(s.comparisons))
	for _, comparison := range s.comparisons {
		if comparison.EventID == eventID {
			out = append(out, comparison)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].JudgeID != out[j].JudgeID {
			return out[i].JudgeID < out[j].JudgeID
		}
		if out[i].Left != out[j].Left {
			return out[i].Left < out[j].Left
		}
		return out[i].Right < out[j].Right
	})
	return out
}

// ComparisonsForJudge is one judge's own verdicts, which is all a judge may see.
func (s *Store) ComparisonsForJudge(eventID, judgeID string) []domain.Comparison {
	all := s.ComparisonsForEvent(eventID)
	out := make([]domain.Comparison, 0, len(all))
	for _, comparison := range all {
		if comparison.JudgeID == judgeID {
			out = append(out, comparison)
		}
	}
	return out
}

// ComparisonByID returns one verdict.
func (s *Store) ComparisonByID(id string) (domain.Comparison, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	comparison, ok := s.comparisons[id]
	if !ok {
		return domain.Comparison{}, domain.ErrNotFound
	}
	return comparison, nil
}

// DeleteComparison removes a verdict.
//
// Recorded comparisons are reversible, and a judge who recorded one by mistake
// should be able to take it back. The audit trail is what makes that safe: the
// record of the mistake survives even though the verdict does not.
func (s *Store) DeleteComparison(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.comparisons[id]; !ok {
		return domain.ErrNotFound
	}
	delete(s.comparisons, id)
	return nil
}
