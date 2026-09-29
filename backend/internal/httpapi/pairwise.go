package httpapi

import (
	"net/http"

	"github.com/iSundram/codeceremony/backend/internal/judging"
)

// pairwise is the organizer-facing pairwise view.
//
// It is a second estimator, not a second opinion on the same numbers: the
// rubric pipeline uses absolute per-criterion scores, while this one uses only
// head-to-head verdicts. Verdicts judges recorded directly are used when the
// panel answered enough of them to stand alone, and comparisons derived from the
// reviews fill in otherwise. The response reports which, because a ranking built
// from recorded answers and one built from inference are different claims.
// Where this and the rubric pipeline disagree, that is a finding about the panel
// rather than an error in either, and JUDGING.md explains what it means.
func (s *Server) pairwise(w http.ResponseWriter, r *http.Request) {
	eventID := r.URL.Query().Get("event_id")
	if eventID == "" {
		eventID = "evt_01"
	}
	event, err := s.store.EventByID(eventID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	// Recorded verdicts are preferred where they cover the panel; otherwise the
	// fit falls back to comparisons derived from the reviews and says so in the
	// response, rather than leaving the reader to assume which it is looking at.
	result, err := judging.PairwiseRecorded(
		s.store.AllReviews(event.ID), s.store.ComparisonsForEvent(event.ID), s.eventWeights(event.ID))
	if err != nil {
		// An empty or unusable panel is reported, not hidden behind a 500: an
		// organizer asking for the pairwise view before any reviews exist
		// should be told why it is empty.
		writeJSON(w, http.StatusOK, map[string]any{"data": result, "event_id": event.ID, "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":     result,
		"event_id": event.ID,
		"event":    map[string]any{"slug": event.Slug, "name": event.Name},
	})
}
