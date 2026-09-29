package httpapi

import (
	"net/http"

	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/judging"
)

var fallbackWeights = judging.Weights{
	"functionality": 40,
	"quality":       35,
	"innovation":    25,
}

func (s *Server) results(w http.ResponseWriter, r *http.Request) {
	eventID, ok := s.queryEvent(w, r)
	if !ok {
		return
	}
	reviews := s.store.AllReviews(eventID)
	summary, err := judging.Normalize(reviews, s.eventWeights(eventID))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "normalization_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":   summary,
		"rubric": s.eventRubricSummary(eventID, reviews),
	})
}

func (s *Server) eventWeights(eventID string) judging.Weights {
	rubric, err := s.store.ActiveRubric(eventID, "")
	if err != nil {
		return fallbackWeights
	}
	return rubricWeights(rubric)
}

func (s *Server) eventRubricSummary(eventID string, reviews []domain.Review) map[string]any {
	rubric, err := s.store.ActiveRubric(eventID, "")
	if err != nil {
		return map[string]any{"id": nil, "version": 0, "name": "default", "weight_source": "fallback"}
	}
	versions := map[string]int{rubric.ID: rubric.Version}
	for _, review := range reviews {
		if review.RubricID != "" {
			versions[review.RubricID] = review.RubricVersion
		}
	}
	return map[string]any{
		"id":            rubric.ID,
		"version":       rubric.Version,
		"name":          rubric.Name,
		"versions_used": versions,
		"weight_source": "published_rubric",
	}
}
