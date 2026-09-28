package httpapi

import (
	"errors"
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
	teams := s.store.ListTeams(event.ID)
	activeTeams := make([]domain.Team, 0, len(teams))
	for _, team := range teams {
		if team.Status == domain.TeamStatusActive {
			activeTeams = append(activeTeams, team)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": activeTeams})
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
		Name         string   `json:"name"`
		Description  string   `json:"description"`
		Scope        string   `json:"scope"`
		Availability string   `json:"availability"`
		MaxSize      int      `json:"max_size"`
		OpenRoles    []string `json:"open_roles"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if strings.TrimSpace(request.Name) == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "team name is required")
		return
	}
	team := domain.Team{
		ID:           domain.NewID("tm"),
		EventID:      event.ID,
		Scope:        domain.TeamScope(strings.TrimSpace(request.Scope)),
		Name:         strings.TrimSpace(request.Name),
		Description:  request.Description,
		CaptainID:    principal.UserID,
		Availability: domain.TeamAvailability(strings.TrimSpace(request.Availability)),
		MaxSize:      request.MaxSize,
		OpenRoles:    request.OpenRoles,
		CreatedAt:    s.now().UTC(),
	}
	if err := s.store.CreateTeam(team); err != nil {
		switch {
		case errors.Is(err, domain.ErrValidation):
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "team settings are not valid; a global team requires the hackathon to allow global teams, and an open team must list open roles")
		case errors.Is(err, domain.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "hackathon or captain not found")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "the team could not be created")
		}
		return
	}
	s.audit(r, principal.UserID, "team.created", "team", team.ID, event.ID, "team created", map[string]any{"scope": team.Scope, "availability": team.Availability})
	s.recordActivity(r, principal.UserID, domain.ActivityTeam, "team.created", "team", team.ID, event.ID,
		principal.UserID+" created the team "+team.Name, domain.ActivityPublic, map[string]any{"scope": team.Scope, "availability": team.Availability})
	writeJSON(w, http.StatusCreated, map[string]any{"data": team})
}

func (s *Server) listTracks(w http.ResponseWriter, r *http.Request) {
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	tracks := s.store.ListTracks(event.ID)
	writeJSON(w, http.StatusOK, map[string]any{"data": tracks, "count": len(tracks)})
}

func (s *Server) createTrack(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	var request struct {
		Name    string `json:"name"`
		Slug    string `json:"slug"`
		Summary string `json:"summary"`
		Order   int    `json:"order"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	track, err := s.store.CreateTrack(domain.Track{
		Event:   event.ID,
		Name:    strings.TrimSpace(request.Name),
		Slug:    strings.TrimSpace(request.Slug),
		Summary: strings.TrimSpace(request.Summary),
		Order:   request.Order,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrAlreadyExists):
			writeError(w, http.StatusConflict, "conflict", "a track with this slug already exists for the event")
		case errors.Is(err, domain.ErrValidation):
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "track name is required")
		case errors.Is(err, domain.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "event not found")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "the track could not be created")
		}
		return
	}
	s.audit(r, principal.UserID, "event.track_created", "track", track.ID, event.ID, "track created", nil)
	writeJSON(w, http.StatusCreated, map[string]any{"data": track})
}

func (s *Server) listPrizes(w http.ResponseWriter, r *http.Request) {
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	prizes := s.store.ListPrizes(event.ID)
	writeJSON(w, http.StatusOK, map[string]any{"data": prizes, "count": len(prizes)})
}

func (s *Server) createPrize(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	var request struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		TrackID     string `json:"track_id"`
		Rank        int    `json:"rank"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if request.Rank <= 0 {
		request.Rank = 1
	}
	prize, err := s.store.CreatePrize(domain.Prize{
		EventID:     event.ID,
		TrackID:     strings.TrimSpace(request.TrackID),
		Name:        strings.TrimSpace(request.Name),
		Description: strings.TrimSpace(request.Description),
		Rank:        request.Rank,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrValidation):
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "prize name is required and the track must belong to this event")
		case errors.Is(err, domain.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "event not found")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "the prize could not be created")
		}
		return
	}
	s.audit(r, principal.UserID, "event.prize_created", "prize", prize.ID, event.ID, "prize created", nil)
	writeJSON(w, http.StatusCreated, map[string]any{"data": prize})
}

func (s *Server) updateTeam(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	var request struct {
		Name         *string   `json:"name"`
		Description  *string   `json:"description"`
		Availability *string   `json:"availability"`
		MaxSize      *int      `json:"max_size"`
		OpenRoles    *[]string `json:"open_roles"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	teamID := r.PathValue("teamID")
	team, err := s.store.UpdateTeam(teamID, func(current domain.Team) (domain.Team, error) {
		next := current
		if request.Name != nil {
			if strings.TrimSpace(*request.Name) == "" {
				return current, domain.ErrValidation
			}
			next.Name = strings.TrimSpace(*request.Name)
		}
		if request.Description != nil {
			next.Description = *request.Description
		}
		if request.Availability != nil {
			next.Availability = domain.TeamAvailability(strings.TrimSpace(*request.Availability))
		}
		if request.MaxSize != nil {
			if *request.MaxSize < 1 {
				return current, domain.ErrValidation
			}
			next.MaxSize = *request.MaxSize
		}
		if request.OpenRoles != nil {
			next.OpenRoles = *request.OpenRoles
		}
		return next, nil
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrValidation):
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "an open team must list open roles and max_size cannot be below the current member count")
		case errors.Is(err, domain.ErrConflict):
			writeError(w, http.StatusConflict, "team_archived", "this team is archived and cannot be updated")
		default:
			writeError(w, http.StatusNotFound, "not_found", "team not found")
		}
		return
	}
	s.audit(r, principal.UserID, "team.updated", "team", team.ID, team.EventID, "team updated", map[string]any{"availability": team.Availability})
	writeJSON(w, http.StatusOK, map[string]any{"data": team})
}
