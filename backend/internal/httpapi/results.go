package httpapi

import (
	"net/http"

	"github.com/iSundram/codeceremony/backend/internal/judging"
)

func (s *Server) results(w http.ResponseWriter, r *http.Request) {
	eventID := r.URL.Query().Get("event_id")
	if eventID == "" {
		eventID = "evt_01"
	}
	if _, err := s.store.EventByID(eventID); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	weights := judging.Weights{
		"functionality": 40,
		"quality":       35,
		"innovation":    25,
	}
	summary, err := judging.Normalize(s.store.AllReviews(eventID), weights)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "normalization_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": summary})
}
