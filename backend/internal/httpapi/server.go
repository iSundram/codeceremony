package httpapi

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/config"
	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

type Server struct {
	cfg    config.Config
	store  *store.Store
	tokens auth.TokenIssuer
	now    func() time.Time
	logger *slog.Logger
}

func New(cfg config.Config, data *store.Store, tokens auth.TokenIssuer, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		cfg:    cfg,
		store:  data,
		tokens: tokens,
		now:    time.Now,
		logger: logger,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /readyz", s.ready)
	mux.HandleFunc("POST /v1/auth/login", s.login)
	mux.HandleFunc("POST /v1/auth/logout", s.logout)
	mux.HandleFunc("GET /v1/auth/methods", s.authMethods)
	mux.Handle("GET /v1/me", s.requirePermission("", s.me))
	mux.Handle("GET /v1/account/profile", s.requirePermission(domain.PermissionManageSelf, s.profile))
	mux.Handle("PATCH /v1/account/profile", s.requirePermission(domain.PermissionManageSelf, s.updateProfile))
	mux.Handle("POST /v1/account/password", s.requirePermission(domain.PermissionManageSelf, s.changePassword))
	mux.Handle("GET /v1/account/export", s.requirePermission(domain.PermissionManageSelf, s.exportAccount))
	mux.Handle("POST /v1/account/deletion", s.requirePermission(domain.PermissionManageSelf, s.requestDeletion))
	mux.Handle("POST /v1/account/deletion/cancel", s.requirePermission(domain.PermissionCancelDeletion, s.cancelDeletion))
	mux.Handle("GET /v1/account/sessions", s.requirePermission(domain.PermissionViewOwnSessions, s.listSessions))
	mux.Handle("DELETE /v1/account/sessions/{sessionID}", s.requirePermission(domain.PermissionRevokeOwnSession, s.revokeSession))
	mux.Handle("POST /v1/account/sessions/revoke-others", s.requirePermission(domain.PermissionRevokeOwnSession, s.revokeOtherSessions))
	mux.Handle("GET /v1/notifications", s.requirePermission(domain.PermissionViewOwnNotifications, s.listNotifications))
	mux.Handle("POST /v1/notifications/{notificationID}/read", s.requirePermission(domain.PermissionViewOwnNotifications, s.markNotificationRead))
	mux.Handle("GET /v1/admin/users", s.requirePermission(domain.PermissionManagePlatform, s.adminListUsers))
	mux.Handle("PATCH /v1/admin/users/{userID}/state", s.requirePermission(domain.PermissionManagePlatform, s.adminUpdateUserState))
	mux.Handle("PUT /v1/admin/users/{userID}/role", s.requirePermission(domain.PermissionManagePlatform, s.adminUpdateUserRole))
	mux.Handle("POST /v1/admin/users/{userID}/sessions/revoke", s.requirePermission(domain.PermissionManagePlatform, s.adminRevokeUserSessions))
	mux.Handle("GET /v1/admin/audit", s.requirePermission(domain.PermissionManagePlatform, s.adminAudit))
	mux.HandleFunc("GET /v1/events", s.listEvents)
	mux.Handle("POST /v1/events", s.requirePermission(domain.PermissionManageEvent, s.createEvent))
	mux.HandleFunc("GET /v1/events/{slug}", s.event)
	mux.HandleFunc("GET /v1/events/{slug}/projects", s.projects)
	mux.HandleFunc("GET /v1/events/{slug}/teams", s.listTeams)
	mux.Handle("POST /v1/events/{slug}/teams", s.requirePermission(domain.PermissionManageTeam, s.createTeam))
	mux.Handle("GET /v1/teams/{teamID}/members", s.requirePermission(domain.PermissionManageTeam, s.listTeamMembers))
	mux.Handle("POST /v1/teams/{teamID}/members/{userID}/promote", s.requirePermission(domain.PermissionManageTeam, s.promoteMember))
	mux.Handle("POST /v1/teams/{teamID}/members/{userID}/demote", s.requirePermission(domain.PermissionManageTeam, s.demoteMember))
	mux.Handle("POST /v1/teams/{teamID}/transfer", s.requirePermission(domain.PermissionManageTeam, s.transferCaptaincy))
	mux.Handle("DELETE /v1/teams/{teamID}/members/{userID}", s.requirePermission(domain.PermissionManageTeam, s.removeMember))
	mux.Handle("DELETE /v1/teams/{teamID}", s.requirePermission(domain.PermissionManageTeam, s.deleteTeam))
	mux.Handle("POST /v1/events/{slug}/submissions", s.requirePermission(domain.PermissionSubmitProject, s.createSubmission))
	mux.Handle("GET /v1/judge/scores", s.requirePermission(domain.PermissionViewOwnScores, s.judgeScores))
	mux.Handle("PUT /v1/judge/projects/{projectID}/review", s.requirePermission(domain.PermissionReviewProject, s.saveReview))
	mux.Handle("GET /v1/organizer/progress", s.requirePermission(domain.PermissionManageEvent, s.progress))
	mux.Handle("GET /v1/organizer/reviews", s.requirePermission(domain.PermissionViewPeerScores, s.organizerReviews))
	mux.Handle("GET /v1/organizer/results", s.requirePermission(domain.PermissionViewPeerScores, s.results))
	mux.Handle("GET /v1/organizer/export.csv", s.requirePermission(domain.PermissionExportData, s.exportCSV))
	return s.middleware(mux)
}

func (s *Server) SetClock(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID = domain.NewID("req")
		}
		w.Header().Set("X-Request-ID", requestID)
		if origin := r.Header.Get("Origin"); origin != "" && origin == s.cfg.AllowedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("request panic", "request_id", requestID, "panic", recovered)
				writeError(w, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requirePermission(permission domain.Permission, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := auth.TokenFromRequest(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
			return
		}
		claims, err := s.tokens.Parse(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "the session is invalid or expired")
			return
		}
		user, err := s.store.UserByID(claims.Subject)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "the session user no longer exists")
			return
		}
		if user.State != "" && user.State != domain.AccountActive {
			if !(user.State == domain.AccountDeletionPending && permission == domain.PermissionCancelDeletion) {
				writeError(w, http.StatusUnauthorized, "account_unavailable", "the account is not active")
				return
			}
		}
		if permission != "" && !user.Role.Can(permission) {
			writeError(w, http.StatusForbidden, "forbidden", "the current role cannot perform this action")
			return
		}
		principal := auth.Principal{UserID: user.ID, Email: user.Email, Role: user.Role, SessionID: claims.SessionID}
		ctx := auth.WithPrincipal(r.Context(), principal)
		next(w, r.WithContext(ctx))
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "codeceremony-api"})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready", "service": "codeceremony-api"})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	user, err := s.store.UserByID(principal.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "the session user no longer exists")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": user})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	user, err := s.store.UserByEmail(request.Email)
	if err != nil || !auth.VerifyPassword(user.PasswordHash, request.Password) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect")
		return
	}
	if user.State != "" && user.State != domain.AccountActive && user.State != domain.AccountDeletionPending {
		writeError(w, http.StatusUnauthorized, "account_unavailable", "the account is not active")
		return
	}
	token, err := s.tokens.Issue(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_error", "could not create a session")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.IsProduction(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   s.cfg.SessionTTLHours * 3600,
	})
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"access_token": token, "user": user}})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.IsProduction(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "logged_out"}})
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": s.store.ListEvents()})
}

func (s *Server) event(w http.ResponseWriter, r *http.Request) {
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": event})
}

func (s *Server) projects(w http.ResponseWriter, r *http.Request) {
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	projects := s.store.ListSubmissions(event.ID, r.URL.Query().Get("q"), r.URL.Query().Get("track"))
	page, pageSize := pagination(r)
	start := (page - 1) * pageSize
	if start > len(projects) {
		start = len(projects)
	}
	end := start + pageSize
	if end > len(projects) {
		end = len(projects)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": projects[start:end],
		"meta": map[string]int{"page": page, "page_size": pageSize, "total": len(projects)},
	})
}

func (s *Server) createSubmission(w http.ResponseWriter, r *http.Request) {
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	if !event.SubmissionsOpen || !s.now().UTC().Before(event.SubmissionsClose) {
		writeError(w, http.StatusUnprocessableEntity, "submissions_closed", "this event is closed for submissions")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	var request struct {
		TeamID        string            `json:"team_id"`
		TrackID       string            `json:"track_id"`
		Title         string            `json:"title"`
		Summary       string            `json:"summary"`
		Description   string            `json:"description"`
		RepositoryURL string            `json:"repo_url"`
		LiveURL       string            `json:"live_url"`
		VideoURL      string            `json:"video_url"`
		Tags          []string          `json:"tags"`
		CustomAnswers map[string]string `json:"custom_answers"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if strings.TrimSpace(request.Title) == "" || strings.TrimSpace(request.Summary) == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "title and summary are required")
		return
	}
	team, err := s.store.TeamByID(request.TeamID)
	if err != nil || team.EventID != event.ID || team.Status != domain.TeamStatusActive {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "a valid active team for this event is required")
		return
	}
	if !s.store.IsTeamCaptain(principal.UserID, team.ID) {
		writeError(w, http.StatusForbidden, "forbidden", "only a team captain can submit for this team")
		return
	}
	if _, err := s.store.TrackByID(request.TrackID); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "a valid track is required")
		return
	}
	now := s.now().UTC()
	submission := domain.Submission{
		ID:            domain.NewID("sub"),
		EventID:       event.ID,
		TeamID:        team.ID,
		TrackID:       request.TrackID,
		Title:         strings.TrimSpace(request.Title),
		Summary:       strings.TrimSpace(request.Summary),
		Description:   request.Description,
		RepositoryURL: request.RepositoryURL,
		LiveURL:       request.LiveURL,
		VideoURL:      request.VideoURL,
		Tags:          request.Tags,
		CustomAnswers: request.CustomAnswers,
		Status:        domain.SubmissionSubmitted,
		SubmittedAt:   &now,
		UpdatedAt:     now,
		Version:       1,
	}
	if err := s.store.CreateSubmission(submission); err != nil {
		if errors.Is(err, domain.ErrAlreadyExists) {
			writeError(w, http.StatusConflict, "conflict", "submission already exists")
			return
		}
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "submission could not be created")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": submission})
}

func (s *Server) judgeScores(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	if requestedJudge := strings.TrimSpace(r.URL.Query().Get("judge")); requestedJudge != "" && requestedJudge != principal.UserID {
		writeError(w, http.StatusForbidden, "forbidden", "judges cannot read another judge's scores")
		return
	}
	eventID := r.URL.Query().Get("event_id")
	if eventID == "" {
		eventID = "evt_01"
	}
	reviews := s.store.ReviewsForJudge(eventID, principal.UserID)
	writeJSON(w, http.StatusOK, map[string]any{"data": reviews})
}

func (s *Server) saveReview(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	projectID := r.PathValue("projectID")
	var request struct {
		EventID   string         `json:"event_id"`
		Criteria  map[string]int `json:"criteria"`
		Comment   string         `json:"comment"`
		Submitted bool           `json:"submitted"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if request.EventID == "" {
		request.EventID = "evt_01"
	}
	if len(request.Criteria) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "at least one criterion score is required")
		return
	}
	for criterion, score := range request.Criteria {
		if score < 1 || score > 5 {
			writeError(w, http.StatusUnprocessableEntity, "validation_error", fmt.Sprintf("score for %s must be between 1 and 5", criterion))
			return
		}
	}
	if !s.store.IsAssigned(request.EventID, principal.UserID, projectID) {
		writeError(w, http.StatusForbidden, "forbidden", "this project is not assigned to the current judge")
		return
	}
	now := s.now().UTC()
	review := domain.Review{ID: domain.NewID("rev"), EventID: request.EventID, JudgeID: principal.UserID, ProjectID: projectID, Criteria: request.Criteria, Comment: request.Comment, Submitted: request.Submitted, UpdatedAt: now}
	if request.Submitted {
		review.SubmittedAt = &now
	}
	if err := s.store.SaveReview(review); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "review could not be saved")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": review})
}

func (s *Server) progress(w http.ResponseWriter, r *http.Request) {
	eventID := r.URL.Query().Get("event_id")
	if eventID == "" {
		eventID = "evt_01"
	}
	if _, err := s.store.EventByID(eventID); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": s.store.Progress(eventID)})
}

func (s *Server) organizerReviews(w http.ResponseWriter, r *http.Request) {
	eventID := r.URL.Query().Get("event_id")
	if eventID == "" {
		eventID = "evt_01"
	}
	if _, err := s.store.EventByID(eventID); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": s.store.AllReviews(eventID)})
}

func pagination(r *http.Request) (int, int) {
	page := 1
	pageSize := 24
	if value := r.URL.Query().Get("page"); value != "" {
		if parsed, err := parsePositiveInt(value); err == nil {
			page = parsed
		}
	}
	if value := r.URL.Query().Get("page_size"); value != "" {
		if parsed, err := parsePositiveInt(value); err == nil && parsed <= 100 {
			pageSize = parsed
		}
	}
	return page, pageSize
}

func parsePositiveInt(value string) (int, error) {
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil || parsed < 1 {
		return 0, domain.ErrValidation
	}
	return parsed, nil
}
