package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/domain"
)

func (s *Server) createEvent(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Slug             string `json:"slug"`
		Name             string `json:"name"`
		Description      string `json:"description"`
		Timezone         string `json:"timezone"`
		RegistrationOpen bool   `json:"registration_open"`
		SubmissionsOpen  bool   `json:"submissions_open"`
		SubmissionsClose string `json:"submissions_close"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if strings.TrimSpace(request.Slug) == "" || strings.TrimSpace(request.Name) == "" || strings.TrimSpace(request.SubmissionsClose) == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "slug, name, and submissions_close are required")
		return
	}
	submissionsClose, err := time.Parse(time.RFC3339, request.SubmissionsClose)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "submissions_close must be an RFC3339 timestamp")
		return
	}
	if strings.TrimSpace(request.Timezone) == "" {
		request.Timezone = "UTC"
	}
	event := domain.Event{
		ID:               domain.NewID("evt"),
		Slug:             strings.TrimSpace(request.Slug),
		Name:             strings.TrimSpace(request.Name),
		Description:      request.Description,
		Timezone:         request.Timezone,
		RegistrationOpen: request.RegistrationOpen,
		SubmissionsOpen:  request.SubmissionsOpen,
		SubmissionsClose: submissionsClose.UTC(),
		CreatedAt:        s.now().UTC(),
	}
	if err := s.store.CreateEvent(event); err != nil {
		if err == domain.ErrAlreadyExists {
			writeError(w, http.StatusConflict, "conflict", "event slug already exists")
			return
		}
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "event could not be created")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": event})
}

func (s *Server) listTeams(w http.ResponseWriter, r *http.Request) {
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": s.store.ListTeams(event.ID)})
}

func (s *Server) createTeam(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	if !event.RegistrationOpen {
		writeError(w, http.StatusUnprocessableEntity, "registration_closed", "this event is not accepting new teams")
		return
	}
	var request struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if strings.TrimSpace(request.Name) == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "team name is required")
		return
	}
	team := domain.Team{ID: domain.NewID("tm"), EventID: event.ID, Name: strings.TrimSpace(request.Name), Description: request.Description, CaptainID: principal.UserID, CreatedAt: s.now().UTC()}
	if err := s.store.CreateTeam(team); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "team could not be created")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": team})
}
