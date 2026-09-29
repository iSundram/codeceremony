package httpapi

import (
	"net/http"

	"github.com/iSundram/codeceremony/backend/internal/judging"
)

// pairwise is the organizer-facing pairwise view.
//
// It is a second estimator, not a second opinion on the same numbers: the
// rubric pipeline uses absolute per-criterion scores, while this one uses only
// head-to-head verdicts derived from them. Where the two disagree, that is a
// finding about the panel rather than an error in either, and JUDGING.md
// explains what a disagreement means.
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
	result, err := judging.Pairwise(s.store.AllReviews(event.ID), s.eventWeights(event.ID))
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
