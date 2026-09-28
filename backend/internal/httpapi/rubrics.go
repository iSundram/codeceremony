package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/judging"
)

func (s *Server) listRubrics(w http.ResponseWriter, r *http.Request) {
	eventID := r.URL.Query().Get("event_id")
	if eventID == "" {
		eventID = "evt_01"
	}
	if _, err := s.store.EventByID(eventID); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	rubrics := s.store.RubricsForEvent(eventID)
	if trackID := strings.TrimSpace(r.URL.Query().Get("track_id")); trackID != "" {
		filtered := make([]domain.Rubric, 0, len(rubrics))
		for _, rubric := range rubrics {
			if rubric.TrackID == trackID {
				filtered = append(filtered, rubric)
			}
		}
		rubrics = filtered
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rubrics, "count": len(rubrics)})
}

func (s *Server) activeRubric(w http.ResponseWriter, r *http.Request) {
	eventID := strings.TrimSpace(r.URL.Query().Get("event_id"))
	if eventID == "" {
		eventID = "evt_01"
	}
	if _, err := s.store.EventByID(eventID); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	trackID := strings.TrimSpace(r.URL.Query().Get("track_id"))
	rubric, err := s.store.ActiveRubric(eventID, trackID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "no published rubric is active")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rubric})
}

type rubricRequest struct {
	EventID      string                   `json:"event_id"`
	TrackID      string                   `json:"track_id"`
	Name         string                   `json:"name"`
	Instructions string                   `json:"instructions"`
	Criteria     []domain.RubricCriterion `json:"criteria"`
}

func (request rubricRequest) rubric(actorID string) domain.Rubric {
	return domain.Rubric{
		EventID:      request.EventID,
		TrackID:      strings.TrimSpace(request.TrackID),
		Name:         strings.TrimSpace(request.Name),
		Instructions: strings.TrimSpace(request.Instructions),
		Criteria:     request.Criteria,
		CreatedBy:    actorID,
		Status:       domain.RubricDraft,
	}
}

func (s *Server) createRubric(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	var request rubricRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if request.EventID == "" {
		request.EventID = "evt_01"
	}
	rubric, err := s.store.CreateRubric(request.rubric(principal.UserID))
	if err != nil {
		s.writeRubricError(w, err)
		return
	}
	s.audit(r, principal.UserID, "rubric.created", "rubric", rubric.ID, rubric.EventID, "rubric draft created", map[string]any{"version": rubric.Version})
	writeJSON(w, http.StatusCreated, map[string]any{"data": rubric})
}

func (s *Server) updateRubric(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	var request rubricRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	rubricID := r.PathValue("rubricID")
	rubric, err := s.store.UpdateRubric(rubricID, func(current domain.Rubric) (domain.Rubric, error) {
		next := current
		if strings.TrimSpace(request.Name) != "" {
			next.Name = strings.TrimSpace(request.Name)
		}
		if strings.TrimSpace(request.Instructions) != "" {
			next.Instructions = strings.TrimSpace(request.Instructions)
		}
		if len(request.Criteria) > 0 {
			next.Criteria = request.Criteria
		}
		return next, nil
	})
	if err != nil {
		s.writeRubricError(w, err)
		return
	}
	s.audit(r, principal.UserID, "rubric.updated", "rubric", rubric.ID, rubric.EventID, "rubric draft updated", nil)
	writeJSON(w, http.StatusOK, map[string]any{"data": rubric})
}

func (s *Server) publishRubric(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	rubric, err := s.store.PublishRubric(r.PathValue("rubricID"))
	if err != nil {
		s.writeRubricError(w, err)
		return
	}
	s.audit(r, principal.UserID, "rubric.published", "rubric", rubric.ID, rubric.EventID, "rubric published", map[string]any{"version": rubric.Version})
	writeJSON(w, http.StatusOK, map[string]any{"data": rubric})
}

func (s *Server) archiveRubric(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	rubric, err := s.store.ArchiveRubric(r.PathValue("rubricID"))
	if err != nil {
		s.writeRubricError(w, err)
		return
	}
	s.audit(r, principal.UserID, "rubric.archived", "rubric", rubric.ID, rubric.EventID, "rubric archived", map[string]any{"version": rubric.Version})
	writeJSON(w, http.StatusOK, map[string]any{"data": rubric})
}

func rubricWeights(rubric domain.Rubric) judging.Weights {
	weights := judging.Weights{}
	for _, criterion := range rubric.Criteria {
		weights[criterion.Key] = float64(criterion.Weight)
	}
	return weights
}

func (s *Server) writeRubricError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "the event, track, or rubric was not found")
	case errors.Is(err, domain.ErrConflict):
		writeError(w, http.StatusConflict, "invalid_state", "only draft rubrics can be edited, and published rubrics cannot be republished")
	case errors.Is(err, domain.ErrValidation):
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "the rubric could not be saved")
	}
}
