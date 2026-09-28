package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/domain"
)

type submissionEditRequest struct {
	Title         string            `json:"title"`
	Summary       string            `json:"summary"`
	Description   string            `json:"description"`
	TrackID       string            `json:"track_id"`
	RepositoryURL string            `json:"repo_url"`
	LiveURL       string            `json:"live_url"`
	VideoURL      string            `json:"video_url"`
	Tags          []string          `json:"tags"`
	CustomAnswers map[string]string `json:"custom_answers"`
	Reason        string            `json:"reason"`
}

func (s *Server) submissionDetail(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	projectID := r.PathValue("projectID")
	project, err := s.store.SubmissionByID(projectID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "submission not found")
		return
	}
	if !s.canViewSubmission(principal, project) {
		writeError(w, http.StatusForbidden, "forbidden", "you cannot view this submission")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":     project,
		"team":     s.teamSummary(project.TeamID),
		"versions": s.store.SubmissionVersions(projectID),
		"flags":    s.duplicateFlagsFor(projectID),
	})
}

func (s *Server) canViewSubmission(principal auth.Principal, project domain.Submission) bool {
	switch {
	case principal.Role == domain.RoleOrganizer, principal.Role == domain.RoleAdmin:
		return true
	case principal.Role == domain.RoleJudge:
		return s.store.IsAssigned(project.EventID, principal.UserID, project.ID)
	default:
		_, err := s.store.TeamMembership(project.TeamID, principal.UserID)
		return err == nil
	}
}

func (s *Server) teamSummary(teamID string) map[string]any {
	team, err := s.store.TeamByID(teamID)
	if err != nil {
		return map[string]any{}
	}
	members := s.store.TeamMembers(teamID)
	names := make([]map[string]string, 0, len(members))
	for _, member := range members {
		entry := map[string]string{"user_id": member.UserID, "role": string(member.Role)}
		if user, err := s.store.UserByID(member.UserID); err == nil {
			entry["display_name"] = user.DisplayName
		}
		names = append(names, entry)
	}
	return map[string]any{"id": team.ID, "name": team.Name, "status": team.Status, "members": names}
}

func (s *Server) duplicateFlagsFor(projectID string) []domain.DuplicateFlag {
	flags := make([]domain.DuplicateFlag, 0)
	for _, flag := range s.store.ListDuplicates("", "") {
		if flag.ProjectID == projectID || flag.DuplicateOfProjectID == projectID {
			flags = append(flags, flag)
		}
	}
	return flags
}

func (s *Server) editSubmission(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	projectID := r.PathValue("projectID")
	project, err := s.store.SubmissionByID(projectID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "submission not found")
		return
	}
	if !s.store.IsTeamCaptain(principal.UserID, project.TeamID) {
		writeError(w, http.StatusForbidden, "forbidden", "only a team captain can edit this submission")
		return
	}
	event, err := s.store.EventByID(project.EventID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	if !event.SubmissionsOpen || !s.now().UTC().Before(event.SubmissionsClose) {
		writeError(w, http.StatusUnprocessableEntity, "submissions_closed", "this event is closed for submissions")
		return
	}
	if strings.TrimSpace(project.Title) == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "title and summary are required")
		return
	}
	var request submissionEditRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	updated, version, err := s.store.ReviseSubmission(projectID, principal.UserID, request.Reason, func(current domain.Submission) (domain.Submission, error) {
		next := current
		next.Status = domain.SubmissionDraft
		if strings.TrimSpace(request.Title) != "" {
			next.Title = strings.TrimSpace(request.Title)
		}
		if strings.TrimSpace(request.Summary) != "" {
			next.Summary = strings.TrimSpace(request.Summary)
		}
		if strings.TrimSpace(request.Description) != "" {
			next.Description = request.Description
		}
		if strings.TrimSpace(request.TrackID) != "" {
			track, err := s.store.TrackByID(strings.TrimSpace(request.TrackID))
			if err != nil || track.Event != current.EventID {
				return current, domain.ErrValidation
			}
			next.TrackID = track.ID
		}
		if strings.TrimSpace(request.RepositoryURL) != "" {
			next.RepositoryURL = strings.TrimSpace(request.RepositoryURL)
		}
		if strings.TrimSpace(request.LiveURL) != "" {
			next.LiveURL = strings.TrimSpace(request.LiveURL)
		}
		if strings.TrimSpace(request.VideoURL) != "" {
			next.VideoURL = strings.TrimSpace(request.VideoURL)
		}
		if len(request.Tags) > 0 {
			next.Tags = request.Tags
		}
		if len(request.CustomAnswers) > 0 {
			next.CustomAnswers = request.CustomAnswers
		}
		return next, nil
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "submission not found")
		case errors.Is(err, domain.ErrConflict):
			writeError(w, http.StatusConflict, "submission_locked", "this submission is locked and can no longer be edited")
		case errors.Is(err, domain.ErrValidation):
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "a valid track is required")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "the submission could not be saved")
		}
		return
	}
	s.audit(r, principal.UserID, "submission.revised", "submission", updated.ID, updated.EventID, request.Reason, map[string]any{"version": version.Version})
	writeJSON(w, http.StatusOK, map[string]any{"data": updated, "version": version})
}

func (s *Server) submitRevision(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	projectID := r.PathValue("projectID")
	project, err := s.store.SubmissionByID(projectID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "submission not found")
		return
	}
	if !s.store.IsTeamCaptain(principal.UserID, project.TeamID) {
		writeError(w, http.StatusForbidden, "forbidden", "only a team captain can submit this project")
		return
	}
	event, err := s.store.EventByID(project.EventID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	if !event.SubmissionsOpen || !s.now().UTC().Before(event.SubmissionsClose) {
		writeError(w, http.StatusUnprocessableEntity, "submissions_closed", "this event is closed for submissions")
		return
	}
	updated, err := s.store.SetSubmissionStatus(projectID, domain.SubmissionSubmitted, "", principal.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			writeError(w, http.StatusConflict, "invalid_state", "this submission cannot be submitted from its current state")
			return
		}
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "the submission could not be submitted")
		return
	}
	if _, err := s.store.RecordSubmissionVersion(projectID, principal.UserID, "submitted"); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "the version history could not be recorded")
		return
	}
	s.audit(r, principal.UserID, "submission.submitted", "submission", updated.ID, updated.EventID, "captain submitted", map[string]any{"version": updated.Version})
	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

func (s *Server) withdrawSubmission(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	projectID := r.PathValue("projectID")
	project, err := s.store.SubmissionByID(projectID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "submission not found")
		return
	}
	if !s.store.IsTeamCaptain(principal.UserID, project.TeamID) {
		writeError(w, http.StatusForbidden, "forbidden", "only a team captain can withdraw this submission")
		return
	}
	var request struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	updated, err := s.store.SetSubmissionStatus(projectID, domain.SubmissionWithdrawn, request.Reason, principal.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			writeError(w, http.StatusConflict, "invalid_state", "this submission cannot be withdrawn from its current state")
			return
		}
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "the submission could not be withdrawn")
		return
	}
	s.audit(r, principal.UserID, "submission.withdrawn", "submission", updated.ID, updated.EventID, request.Reason, nil)
	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

func (s *Server) setEligibility(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	var request struct {
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	projectID := r.PathValue("projectID")
	updated, err := s.store.SetSubmissionEligibility(projectID, domain.EligibilityDecision(strings.TrimSpace(request.Decision)), request.Note, principal.UserID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "submission not found")
		case errors.Is(err, domain.ErrValidation):
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "decision must be eligible, ineligible, or pending and ineligibility requires a note")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "the decision could not be recorded")
		}
		return
	}
	s.audit(r, principal.UserID, "submission.eligibility_set", "submission", updated.ID, updated.EventID, request.Note, map[string]any{"decision": request.Decision})
	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

func (s *Server) setSubmissionStatus(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	var request struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	projectID := r.PathValue("projectID")
	updated, err := s.store.SetSubmissionStatus(projectID, domain.SubmissionStatus(strings.TrimSpace(request.Status)), request.Note, principal.UserID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "submission not found")
		case errors.Is(err, domain.ErrConflict):
			writeError(w, http.StatusConflict, "invalid_state", "this submission cannot move to the requested state")
		case errors.Is(err, domain.ErrValidation):
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "the requested status is not valid")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "the status could not be changed")
		}
		return
	}
	s.audit(r, principal.UserID, "submission.status_changed", "submission", updated.ID, updated.EventID, request.Note, map[string]any{"status": request.Status})
	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

func (s *Server) scanDuplicates(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	eventID := r.URL.Query().Get("event_id")
	if eventID == "" {
		eventID = "evt_01"
	}
	flags, err := s.store.ScanForDuplicates(eventID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	s.audit(r, principal.UserID, "duplicates.scanned", "event", eventID, eventID, "duplicate scan", map[string]any{"new_flags": len(flags)})
	writeJSON(w, http.StatusOK, map[string]any{"data": flags, "created": len(flags), "open": len(s.store.ListDuplicates(eventID, domain.DuplicateOpen))})
}

func (s *Server) listDuplicates(w http.ResponseWriter, r *http.Request) {
	eventID := r.URL.Query().Get("event_id")
	if eventID == "" {
		eventID = "evt_01"
	}
	flags := s.store.ListDuplicates(eventID, domain.DuplicateStatus(strings.TrimSpace(r.URL.Query().Get("status"))))
	writeJSON(w, http.StatusOK, map[string]any{"data": flags, "count": len(flags)})
}

func (s *Server) resolveDuplicate(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	var request struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	flag, err := s.store.ResolveDuplicate(r.PathValue("duplicateID"), domain.DuplicateStatus(strings.TrimSpace(request.Status)), request.Note, principal.UserID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "duplicate flag not found")
		case errors.Is(err, domain.ErrValidation):
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "status must be confirmed or dismissed and a note is required")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "the flag could not be resolved")
		}
		return
	}
	s.audit(r, principal.UserID, "duplicate.resolved", "submission", flag.ProjectID, flag.EventID, request.Note, map[string]any{"status": request.Status})
	writeJSON(w, http.StatusOK, map[string]any{"data": flag})
}

func validateProjectLinks(repositoryURL, liveURL, videoURL string) error {
	for name, value := range map[string]string{"repo_url": repositoryURL, "live_url": liveURL, "video_url": videoURL} {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		lowered := strings.ToLower(trimmed)
		if !strings.HasPrefix(lowered, "https://") && !strings.HasPrefix(lowered, "http://") {
			return errors.New(name + " must be a valid http or https url")
		}
	}
	if trimmed := strings.TrimSpace(repositoryURL); trimmed != "" {
		withoutScheme := trimmed
		if index := strings.Index(withoutScheme, "://"); index >= 0 {
			withoutScheme = withoutScheme[index+3:]
		}
		segments := strings.Split(strings.Trim(withoutScheme, "/"), "/")
		if len(segments) < 3 {
			return errors.New("repo_url must include a host and a repository path, for example https://github.com/org/repo")
		}
	}
	return nil
}
