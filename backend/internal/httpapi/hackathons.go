package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/judging"
)

func (s *Server) hackathon(w http.ResponseWriter, r *http.Request) {
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	isStaff := principal.Role == domain.RoleOrganizer || principal.Role == domain.RoleAdmin
	payload := map[string]any{
		"data":        event,
		"tracks":      s.store.ListTracks(event.ID),
		"prizes":      s.store.ListPrizes(event.ID),
		"hosts":       s.store.Hosts(event.ID),
		"milestones":  s.store.Milestones(event.ID),
		"questions":   s.store.Questions(event.ID, ""),
		"judge_count": len(s.store.JudgeRoster(event.ID)),
		"team_policy": map[string]any{
			"scope":              event.ScopeOrDefault(),
			"min_team_size":      event.MinTeamSize,
			"max_team_size":      event.MaxTeamSize,
			"allow_global_teams": event.AllowGlobalTeams,
		},
	}
	if isStaff {
		payload["judges"] = s.store.JudgeRoster(event.ID)
	}
	if rubric, err := s.store.ActiveRubric(event.ID, ""); err == nil {
		payload["rubric"] = rubric
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) updateHackathon(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	var request struct {
		Name              *string `json:"name"`
		Summary           *string `json:"summary"`
		Description       *string `json:"description"`
		Timezone          *string `json:"timezone"`
		State             *string `json:"state"`
		JudgingMode       *string `json:"judging_mode"`
		RegistrationOpen  *bool   `json:"registration_open"`
		SubmissionsOpen   *bool   `json:"submissions_open"`
		SubmissionsClose  *string `json:"submissions_close"`
		JudgingClose      *string `json:"judging_close"`
		MinTeamSize       *int    `json:"min_team_size"`
		MaxTeamSize       *int    `json:"max_team_size"`
		AllowGlobalTeams  *bool   `json:"allow_global_teams"`
		ReviewsPerProject *int    `json:"reviews_per_project"`
		LeaderboardPublic *bool   `json:"leaderboard_public"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	updated, err := s.store.UpdateEvent(event.ID, func(current domain.Event) (domain.Event, error) {
		next := current
		if request.Name != nil {
			if strings.TrimSpace(*request.Name) == "" {
				return current, domain.ErrValidation
			}
			next.Name = strings.TrimSpace(*request.Name)
		}
		if request.Summary != nil {
			next.Summary = strings.TrimSpace(*request.Summary)
		}
		if request.Description != nil {
			next.Description = *request.Description
		}
		if request.Timezone != nil {
			next.Timezone = strings.TrimSpace(*request.Timezone)
		}
		if request.State != nil {
			state := domain.HackathonState(strings.TrimSpace(*request.State))
			if !validHackathonState(state) {
				return current, domain.ErrValidation
			}
			next.State = state
		}
		if request.JudgingMode != nil {
			mode := domain.JudgingMode(strings.TrimSpace(*request.JudgingMode))
			if mode != domain.JudgingAutomatic && mode != domain.JudgingManual {
				return current, domain.ErrValidation
			}
			next.JudgingMode = mode
		}
		if request.RegistrationOpen != nil {
			next.RegistrationOpen = *request.RegistrationOpen
		}
		if request.SubmissionsOpen != nil {
			next.SubmissionsOpen = *request.SubmissionsOpen
		}
		if request.SubmissionsClose != nil {
			parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*request.SubmissionsClose))
			if err != nil {
				return current, domain.ErrValidation
			}
			next.SubmissionsClose = parsed.UTC()
		}
		if request.JudgingClose != nil {
			parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*request.JudgingClose))
			if err != nil {
				return current, domain.ErrValidation
			}
			next.JudgingClose = parsed.UTC()
		}
		if request.MinTeamSize != nil {
			if *request.MinTeamSize < 1 {
				return current, domain.ErrValidation
			}
			next.MinTeamSize = *request.MinTeamSize
		}
		if request.MaxTeamSize != nil {
			if *request.MaxTeamSize < 1 {
				return current, domain.ErrValidation
			}
			next.MaxTeamSize = *request.MaxTeamSize
		}
		if next.MaxTeamSize > 0 && next.MinTeamSize > next.MaxTeamSize {
			return current, domain.ErrValidation
		}
		if request.AllowGlobalTeams != nil {
			next.AllowGlobalTeams = *request.AllowGlobalTeams
		}
		if request.ReviewsPerProject != nil {
			if *request.ReviewsPerProject < 1 || *request.ReviewsPerProject > 10 {
				return current, domain.ErrValidation
			}
			next.ReviewsPerProject = *request.ReviewsPerProject
		}
		if request.LeaderboardPublic != nil {
			next.LeaderboardPublic = *request.LeaderboardPublic
		}
		return next, nil
	})
	if err != nil {
		if errors.Is(err, domain.ErrValidation) {
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "the hackathon settings are not valid")
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	s.audit(r, principal.UserID, "hackathon.updated", "event", updated.ID, updated.ID, "hackathon settings updated", nil)
	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

func validHackathonState(state domain.HackathonState) bool {
	switch state {
	case domain.HackathonDraft, domain.HackathonRegistrationOpen, domain.HackathonSubmissionsOpen, domain.HackathonSubmissionsClosed, domain.HackathonJudging, domain.HackathonResultsPublished, domain.HackathonArchived:
		return true
	default:
		return false
	}
}

func (s *Server) listQuestions(w http.ResponseWriter, r *http.Request) {
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	questions := s.store.Questions(event.ID, domain.QuestionAudience(strings.TrimSpace(r.URL.Query().Get("audience"))))
	writeJSON(w, http.StatusOK, map[string]any{"data": questions, "count": len(questions)})
}

func (s *Server) createQuestion(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	var request struct {
		Prompt   string   `json:"prompt"`
		HelpText string   `json:"help_text"`
		Type     string   `json:"type"`
		Audience string   `json:"audience"`
		Required bool     `json:"required"`
		Options  []string `json:"options"`
		Position int      `json:"position"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	question, err := s.store.CreateQuestion(domain.HackathonQuestion{
		EventID:  event.ID,
		Prompt:   strings.TrimSpace(request.Prompt),
		HelpText: strings.TrimSpace(request.HelpText),
		Type:     domain.QuestionType(strings.TrimSpace(request.Type)),
		Audience: domain.QuestionAudience(strings.TrimSpace(request.Audience)),
		Required: request.Required,
		Options:  request.Options,
		Position: request.Position,
	})
	if err != nil {
		s.writeQuestionError(w, err)
		return
	}
	s.audit(r, principal.UserID, "hackathon.question_created", "question", question.ID, event.ID, "submission question added", nil)
	writeJSON(w, http.StatusCreated, map[string]any{"data": question})
}

func (s *Server) updateQuestion(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	var request struct {
		Prompt   *string   `json:"prompt"`
		HelpText *string   `json:"help_text"`
		Required *bool     `json:"required"`
		Options  *[]string `json:"options"`
		Position *int      `json:"position"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	question, err := s.store.UpdateQuestion(r.PathValue("questionID"), func(current domain.HackathonQuestion) (domain.HackathonQuestion, error) {
		next := current
		if request.Prompt != nil {
			next.Prompt = strings.TrimSpace(*request.Prompt)
		}
		if request.HelpText != nil {
			next.HelpText = strings.TrimSpace(*request.HelpText)
		}
		if request.Required != nil {
			next.Required = *request.Required
		}
		if request.Options != nil {
			next.Options = *request.Options
		}
		if request.Position != nil {
			next.Position = *request.Position
		}
		return next, nil
	})
	if err != nil {
		s.writeQuestionError(w, err)
		return
	}
	s.audit(r, principal.UserID, "hackathon.question_updated", "question", question.ID, question.EventID, "submission question updated", nil)
	writeJSON(w, http.StatusOK, map[string]any{"data": question})
}

func (s *Server) deleteQuestion(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	questionID := r.PathValue("questionID")
	if err := s.store.DeleteQuestion(questionID); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "question not found")
		return
	}
	s.audit(r, principal.UserID, "hackathon.question_deleted", "question", questionID, "", "submission question removed", nil)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": questionID, "deleted": true}})
}

func (s *Server) writeQuestionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrAlreadyExists):
		writeError(w, http.StatusConflict, "conflict", "an equivalent question already exists for this audience")
	case errors.Is(err, domain.ErrValidation):
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "hackathon or question not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "the question could not be saved")
	}
}

func (s *Server) listMilestones(w http.ResponseWriter, r *http.Request) {
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	milestones := s.store.Milestones(event.ID)
	writeJSON(w, http.StatusOK, map[string]any{"data": milestones, "count": len(milestones)})
}

func (s *Server) createMilestone(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	var request struct {
		Title    string `json:"title"`
		Detail   string `json:"detail"`
		DueAt    string `json:"due_at"`
		Position int    `json:"position"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	dueAt := time.Time{}
	if strings.TrimSpace(request.DueAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(request.DueAt))
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "due_at must be an RFC3339 timestamp")
			return
		}
		dueAt = parsed.UTC()
	}
	milestone, err := s.store.CreateMilestone(domain.HackathonMilestone{
		EventID:  event.ID,
		Title:    strings.TrimSpace(request.Title),
		Detail:   strings.TrimSpace(request.Detail),
		DueAt:    dueAt,
		Position: request.Position,
	})
	if err != nil {
		if errors.Is(err, domain.ErrValidation) {
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "milestone title is required")
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	s.audit(r, principal.UserID, "hackathon.milestone_created", "milestone", milestone.ID, event.ID, "milestone added", nil)
	writeJSON(w, http.StatusCreated, map[string]any{"data": milestone})
}

func (s *Server) listHosts(w http.ResponseWriter, r *http.Request) {
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	hosts := s.store.Hosts(event.ID)
	writeJSON(w, http.StatusOK, map[string]any{"data": hosts, "count": len(hosts)})
}

func (s *Server) createHost(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	var request struct {
		Name    string `json:"name"`
		URL     string `json:"url"`
		LogoURL string `json:"logo_url"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	host, err := s.store.AddHost(domain.HackathonHost{
		EventID: event.ID,
		Name:    strings.TrimSpace(request.Name),
		URL:     strings.TrimSpace(request.URL),
		LogoURL: strings.TrimSpace(request.LogoURL),
	})
	if err != nil {
		if errors.Is(err, domain.ErrValidation) {
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "host name is required")
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	s.audit(r, principal.UserID, "hackathon.host_added", "host", host.ID, event.ID, "host added", nil)
	writeJSON(w, http.StatusCreated, map[string]any{"data": host})
}

func (s *Server) listJudges(w http.ResponseWriter, r *http.Request) {
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	roster := s.store.JudgeRoster(event.ID)
	views := make([]map[string]any, 0, len(roster))
	for _, entry := range roster {
		view := map[string]any{"entry": entry}
		if user, err := s.store.UserByID(entry.JudgeID); err == nil {
			view["display_name"] = user.DisplayName
			view["email"] = user.Email
		}
		if profile, err := s.store.JudgeProfile(entry.JudgeID); err == nil {
			view["capacity"] = profile.Capacity
			view["tracks"] = profile.Tracks
			view["bio"] = profile.Bio
		}
		views = append(views, view)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": views, "count": len(views), "global": s.store.GlobalJudges()})
}

func (s *Server) addJudge(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	var request struct {
		JudgeID   string   `json:"judge_id"`
		Scope     string   `json:"scope"`
		Headline  string   `json:"headline"`
		Expertise []string `json:"expertise"`
		Tracks    []string `json:"tracks"`
		Capacity  int      `json:"capacity"`
		Bio       string   `json:"bio"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	judgeID := strings.TrimSpace(request.JudgeID)
	if judgeID == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "judge_id is required")
		return
	}
	if len(request.Tracks) > 0 || request.Capacity > 0 || strings.TrimSpace(request.Bio) != "" {
		profile, err := s.store.JudgeProfile(judgeID)
		if err == nil {
			profile.Bio = strings.TrimSpace(request.Bio)
			profile.Tracks = request.Tracks
			profile.Capacity = request.Capacity
			profile.Active = true
			if err := s.store.SetJudgeProfile(profile); err != nil {
				writeError(w, http.StatusUnprocessableEntity, "validation_error", "judge profile could not be updated")
				return
			}
		}
	}
	entry, err := s.store.AddJudgeToRoster(domain.JudgeRosterEntry{
		EventID:   event.ID,
		JudgeID:   judgeID,
		Scope:     domain.JudgeScope(strings.TrimSpace(request.Scope)),
		Headline:  strings.TrimSpace(request.Headline),
		Expertise: request.Expertise,
		AddedBy:   principal.UserID,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "hackathon or judge not found")
		case errors.Is(err, domain.ErrValidation):
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "only judges and admins can join a hackathon roster")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "the judge could not be added")
		}
		return
	}
	s.audit(r, principal.UserID, "hackathon.judge_added", "user", judgeID, event.ID, "judge added to roster", map[string]any{"scope": entry.Scope})
	writeJSON(w, http.StatusCreated, map[string]any{"data": entry})
}

func (s *Server) removeJudge(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	judgeID := r.PathValue("judgeID")
	if err := s.store.RemoveJudgeFromRoster(event.ID, judgeID); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "judge not found on this roster")
		return
	}
	s.audit(r, principal.UserID, "hackathon.judge_removed", "user", judgeID, event.ID, "judge removed from roster", nil)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"judge_id": judgeID, "active": false}})
}

func (s *Server) leaderboard(w http.ResponseWriter, r *http.Request) {
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	isStaff := principal.Role == domain.RoleOrganizer || principal.Role == domain.RoleAdmin
	if !isStaff && !(event.ResultsPublished && event.LeaderboardPublic) {
		writeError(w, http.StatusForbidden, "forbidden", "the leaderboard is not public yet")
		return
	}
	weights := s.eventWeights(event.ID)
	summary, err := judging.Normalize(s.store.AllReviews(event.ID), weights)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "normalization_error", err.Error())
		return
	}
	entries := make([]map[string]any, 0)
	for _, project := range summary.Projects {
		submission, err := s.store.SubmissionByID(project.ProjectID)
		if err != nil {
			continue
		}
		if !isStaff && submission.Eligibility == domain.EligibilityIneligible {
			continue
		}
		entry := map[string]any{
			"rank":            project.Rank,
			"project_id":      submission.ID,
			"title":           submission.Title,
			"team_id":         submission.TeamID,
			"track_id":        submission.TrackID,
			"summary":         submission.Summary,
			"raw_mean":        project.RawMean,
			"normalized_mean": project.NormalizedMean,
			"review_count":    project.ReviewCount,
		}
		if team, err := s.store.TeamByID(submission.TeamID); err == nil {
			entry["team_name"] = team.Name
		}
		entries = append(entries, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":        entries,
		"count":       len(entries),
		"published":   event.ResultsPublished,
		"is_staff":    isStaff,
		"event_state": event.State,
	})
}

func (s *Server) publishResults(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	var request struct {
		Public bool   `json:"public"`
		Note   string `json:"note"`
	}
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
	}
	reviews := s.store.AllReviews(event.ID)
	pending := 0
	for _, assignment := range s.store.ListAssignments(event.ID, "") {
		if assignment.RevokedAt != nil {
			continue
		}
		if review, err := s.store.ReviewForJudgeProject(assignment.JudgeID, assignment.ProjectID); err != nil || !review.Submitted {
			pending++
		}
	}
	publishedAt := s.now().UTC()
	updated, err := s.store.UpdateEvent(event.ID, func(current domain.Event) (domain.Event, error) {
		next := current
		next.ResultsPublished = true
		next.ResultsPublishedAt = &publishedAt
		next.State = domain.HackathonResultsPublished
		next.LeaderboardPublic = request.Public
		return next, nil
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	notified := s.notifyResultsPublished(event, updated, request.Note, pending, len(reviews))
	s.audit(r, principal.UserID, "hackathon.results_published", "event", event.ID, event.ID, strings.TrimSpace(request.Note), map[string]any{
		"notified":        notified,
		"pending_reviews": pending,
		"public":          request.Public,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"data":            updated,
		"notified":        notified,
		"pending_reviews": pending,
	})
}

func (s *Server) unpublishResults(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	var request struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if strings.TrimSpace(request.Reason) == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "a reason is required to unpublish results")
		return
	}
	updated, err := s.store.UpdateEvent(event.ID, func(current domain.Event) (domain.Event, error) {
		next := current
		next.ResultsPublished = false
		next.ResultsPublishedAt = nil
		next.LeaderboardPublic = false
		next.State = domain.HackathonJudging
		return next, nil
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	s.audit(r, principal.UserID, "hackathon.results_unpublished", "event", event.ID, event.ID, request.Reason, nil)
	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

func (s *Server) notifyResultsPublished(event, updated domain.Event, note string, pending, totalReviews int) int {
	notified := 0
	rankByTeam := map[string]string{}
	if summary, err := judging.Normalize(s.store.AllReviews(event.ID), s.eventWeights(event.ID)); err == nil {
		for _, project := range summary.Projects {
			submission, err := s.store.SubmissionByID(project.ProjectID)
			if err != nil {
				continue
			}
			rankByTeam[submission.TeamID] = strconv.Itoa(project.Rank)
		}
	}
	seen := map[string]struct{}{}
	for _, team := range s.store.ListTeams(event.ID) {
		if team.Status != domain.TeamStatusActive {
			continue
		}
		for _, membership := range s.store.TeamMembers(team.ID) {
			if _, done := seen[membership.UserID]; done {
				continue
			}
			seen[membership.UserID] = struct{}{}
			rank := rankByTeam[team.ID]
			message := "Results are published for " + updated.Name + "."
			if rank != "" {
				message = "Results are published for " + updated.Name + ". Your team placed #" + rank + "."
			}
			if err := s.store.CreateNotification(domain.Notification{
				UserID:    membership.UserID,
				EventID:   updated.ID,
				Kind:      domain.NotificationEventActivity,
				Title:     "Results published",
				Body:      message,
				ActionURL: "/hackathons/" + updated.Slug + "/leaderboard",
			}); err == nil {
				notified++
			}
		}
	}
	for _, entry := range s.store.JudgeRoster(event.ID) {
		if err := s.store.CreateNotification(domain.Notification{
			UserID:    entry.JudgeID,
			EventID:   updated.ID,
			Kind:      domain.NotificationJudging,
			Title:     "Results published",
			Body:      strings.TrimSpace(note),
			ActionURL: "/hackathons/" + updated.Slug + "/results",
		}); err == nil {
			notified++
		}
	}
	_ = totalReviews
	_ = pending
	_ = event
	return notified
}
