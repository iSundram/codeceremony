package httpapi

import (
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/config"
	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/mailer"
	"github.com/iSundram/codeceremony/backend/internal/ratelimit"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

// RouteEntry is a route captured at registration time.
type RouteEntry struct {
	Method  string
	Pattern string
}

// routeMux wraps http.ServeMux so every registered route is recorded, which
// keeps the endpoint catalog and OpenAPI document in step with the router.
type routeMux struct {
	*http.ServeMux
	entries []RouteEntry
}

func newRouteMux() *routeMux {
	return &routeMux{ServeMux: http.NewServeMux()}
}

func (m *routeMux) Handle(pattern string, handler http.Handler) {
	if method, rest, found := strings.Cut(pattern, " "); found {
		m.entries = append(m.entries, RouteEntry{Method: method, Pattern: rest})
	} else {
		m.entries = append(m.entries, RouteEntry{Method: "ANY", Pattern: pattern})
	}
	m.ServeMux.Handle(pattern, handler)
}

func (m *routeMux) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	if method, rest, found := strings.Cut(pattern, " "); found {
		m.entries = append(m.entries, RouteEntry{Method: method, Pattern: rest})
	} else {
		m.entries = append(m.entries, RouteEntry{Method: "ANY", Pattern: pattern})
	}
	m.ServeMux.HandleFunc(pattern, handler)
}

func (m *routeMux) recorded() []RouteEntry {
	out := make([]RouteEntry, len(m.entries))
	copy(out, m.entries)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Pattern == out[j].Pattern {
			return out[i].Method < out[j].Method
		}
		return out[i].Pattern < out[j].Pattern
	})
	return out
}

type Server struct {
	cfg         config.Config
	store       *store.Store
	tokens      auth.TokenIssuer
	mailService *mailer.Service
	webhooks    *mailer.Webhooks
	limiter     *ratelimit.Limiter

	// pagesMu guards the lazily compiled per-page template cache.
	pagesMu sync.Mutex
	pages   map[string]*template.Template
	routes  []RouteEntry
	now     func() time.Time
	logger  *slog.Logger
}

func New(cfg config.Config, data *store.Store, tokens auth.TokenIssuer, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	registry := mailer.NewRegistry()
	dispatcher := mailer.NewDispatcher(data, registry, mailer.NewSender(cfg, logger), mailer.DispatcherOptions{
		FromAddress: cfg.SMTPFrom,
		Logger:      logger,
	})
	mailService := mailer.NewService(data, dispatcher, registry, mailer.Options{
		AppURL:      cfg.AppBaseURL,
		FromAddress: cfg.SMTPFrom,
		Logger:      logger,
	})
	return &Server{
		cfg:         cfg,
		store:       data,
		tokens:      tokens,
		mailService: mailService,
		webhooks:    mailer.NewWebhooks(data, mailer.NewHTTPHookTransport(), logger, cfg.MailMaxAttempts, time.Duration(cfg.MailInterval)*time.Second),
		now:         time.Now,
		logger:      logger,
		limiter:     ratelimit.New(),
		pages:       make(map[string]*template.Template),
	}
}

// Webhooks exposes the outbound webhook dispatcher for the process entry point.
func (s *Server) Webhooks() *mailer.Webhooks {
	return s.webhooks
}

// MailDispatcher exposes the delivery worker so the process entry point can run it.
func (s *Server) MailDispatcher() *mailer.Dispatcher {
	return s.mailService.Dispatcher()
}

// MailService exposes the mail service for background jobs and tests.
func (s *Server) MailService() *mailer.Service {
	return s.mailService
}

func (s *Server) Handler() http.Handler {
	mux := newRouteMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /readyz", s.ready)
	mux.Handle("POST /v1/auth/login", s.guard(ratelimit.Login, http.HandlerFunc(s.login)))
	mux.Handle("POST /v1/auth/logout", s.guard(ratelimit.Account, http.HandlerFunc(s.logout)))
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
	mux.HandleFunc("GET /v1/directory", s.eventDirectory)
	mux.HandleFunc("GET /v1/permissions", s.permissionMatrix)
	mux.Handle("GET /v1/activity", s.requirePermission("", s.activityFeed))
	mux.Handle("GET /v1/events/{slug}/activity", s.optionalAuth(s.activityFeed))
	mux.Handle("GET /v1/events/{slug}/staff", s.optionalAuth(s.listEventStaff))
	mux.Handle("POST /v1/events/{slug}/staff", s.requirePermission(domain.PermissionManageRoles, s.addEventStaff))
	mux.Handle("DELETE /v1/events/{slug}/staff/{userID}", s.requirePermission(domain.PermissionManageRoles, s.removeEventStaff))
	mux.Handle("GET /v1/email/preferences", s.requirePermission(domain.PermissionViewOwnNotifications, s.mailPreferences))
	mux.Handle("PATCH /v1/email/preferences", s.requirePermission(domain.PermissionManageSelf, s.updateMailPreferences))
	mux.Handle("POST /v1/email/verify", s.guard(ratelimit.Account, s.requirePermission(domain.PermissionManageSelf, s.requestVerificationMail)))
	mux.Handle("POST /v1/email/password-reset", s.guard(ratelimit.Account, s.requirePermission(domain.PermissionManageSelf, s.requestPasswordResetMail)))
	mux.HandleFunc("GET /v1/unsubscribe/{token}", s.unsubscribe)
	mux.HandleFunc("GET /v1/unsubscribe", s.unsubscribe)
	mux.Handle("POST /v1/organizer/events/{slug}/announcements", s.requirePermission(domain.PermissionManageEvent, s.sendAnnouncement))
	mux.Handle("POST /v1/organizer/events/{slug}/review-reminders", s.requirePermission(domain.PermissionManageEvent, s.sendReviewReminders))
	mux.Handle("POST /v1/organizer/mail/flush", s.requirePermission(domain.PermissionManagePlatform, s.flushMailQueue))
	mux.Handle("POST /v1/organizer/mail/weekly-digest", s.requirePermission(domain.PermissionManagePlatform, s.sendWeeklyDigests))
	mux.Handle("GET /v1/organizer/mail/outbox", s.requirePermission(domain.PermissionViewAudit, s.mailOutbox))
	mux.Handle("GET /v1/organizer/mail/templates", s.requirePermission(domain.PermissionViewAudit, s.mailTemplates))
	mux.Handle("POST /v1/events", s.requirePermission(domain.PermissionManageEvent, s.createEvent))
	mux.HandleFunc("GET /v1/events/{slug}", s.hackathon)
	mux.Handle("PATCH /v1/events/{slug}", s.requirePermission(domain.PermissionManageEvent, s.updateHackathon))
	mux.HandleFunc("GET /v1/events/{slug}/milestones", s.listMilestones)
	mux.Handle("POST /v1/events/{slug}/milestones", s.requirePermission(domain.PermissionManageEvent, s.createMilestone))
	mux.HandleFunc("GET /v1/events/{slug}/hosts", s.listHosts)
	mux.Handle("POST /v1/events/{slug}/hosts", s.requirePermission(domain.PermissionManageEvent, s.createHost))
	mux.HandleFunc("GET /v1/events/{slug}/questions", s.listQuestions)
	mux.Handle("POST /v1/events/{slug}/questions", s.requirePermission(domain.PermissionManageEvent, s.createQuestion))
	mux.Handle("PUT /v1/organizer/questions/{questionID}", s.requirePermission(domain.PermissionManageEvent, s.updateQuestion))
	mux.Handle("DELETE /v1/organizer/questions/{questionID}", s.requirePermission(domain.PermissionManageEvent, s.deleteQuestion))
	mux.Handle("GET /v1/events/{slug}/comments", s.optionalAuth(func(w http.ResponseWriter, r *http.Request) { s.listComments(w, r) }))
	mux.Handle("POST /v1/events/{slug}/projects/{projectID}/comments", s.guard(ratelimit.Comment, s.requirePermission("", s.postComment)))
	mux.Handle("DELETE /v1/comments/{commentID}", s.requirePermission("", s.deleteComment))
	mux.Handle("PUT /v1/organizer/comments/{commentID}/moderate", s.requirePermission(domain.PermissionManageSubmission, s.moderateComment))
	mux.Handle("POST /v1/comments/{commentID}/report", s.guard(ratelimit.Comment, s.requirePermission("", s.reportComment)))
	mux.Handle("GET /v1/organizer/reports", s.requirePermission(domain.PermissionManageSubmission, s.listReports))
	mux.Handle("PUT /v1/organizer/reports/{reportID}", s.requirePermission(domain.PermissionManageSubmission, s.resolveReport))
	mux.HandleFunc("GET /v1/events/{slug}/vote-campaigns", s.listCampaigns)
	mux.Handle("GET /v1/events/{slug}/vote/ballot", s.guard(ratelimit.Ballot, s.optionalAuth(s.ballotOptions)))
	mux.Handle("POST /v1/organizer/events/{slug}/vote-campaigns", s.requirePermission(domain.PermissionManageEvent, s.createCampaign))
	mux.Handle("PUT /v1/organizer/vote-campaigns/{campaignID}/{status}", s.requirePermission(domain.PermissionManageEvent, s.setCampaignStatus))
	mux.Handle("POST /v1/events/{slug}/vote", s.guard(ratelimit.Ballot, s.requirePermission("", s.castVotes)))
	mux.Handle("GET /v1/events/{slug}/vote", s.optionalAuth(s.voteResults))
	mux.Handle("GET /v1/vote/mine", s.requirePermission("", s.myVotes))
	mux.Handle("GET /v1/events/{slug}/webhooks", s.requirePermission(domain.PermissionManageIntegrations, s.listWebhooks))
	mux.Handle("POST /v1/organizer/events/{slug}/webhooks", s.requirePermission(domain.PermissionManageIntegrations, s.createWebhook))
	mux.Handle("DELETE /v1/organizer/webhooks/{webhookID}", s.requirePermission(domain.PermissionManageIntegrations, s.deleteWebhook))
	mux.Handle("POST /v1/organizer/webhooks/{webhookID}/test", s.requirePermission(domain.PermissionManageIntegrations, s.testWebhook))
	mux.Handle("GET /v1/organizer/webhooks/deliveries", s.requirePermission(domain.PermissionViewAudit, s.listDeliveries))
	mux.Handle("POST /v1/organizer/webhooks/flush", s.requirePermission(domain.PermissionManageIntegrations, s.flushWebhooks))
	mux.Handle("GET /v1/events/{slug}/judges", s.optionalAuth(s.listJudges))
	mux.Handle("POST /v1/events/{slug}/judges", s.requirePermission(domain.PermissionManageEvent, s.addJudge))
	mux.Handle("DELETE /v1/events/{slug}/judges/{judgeID}", s.requirePermission(domain.PermissionManageEvent, s.removeJudge))
	mux.Handle("GET /v1/events/{slug}/leaderboard", s.optionalAuth(s.leaderboard))
	mux.Handle("POST /v1/organizer/events/{slug}/publish-results", s.requirePermission(domain.PermissionManageEvent, s.publishResults))
	mux.Handle("POST /v1/organizer/events/{slug}/unpublish-results", s.requirePermission(domain.PermissionManageEvent, s.unpublishResults))
	mux.Handle("GET /v1/profiles/{userID}", s.requirePermission("", s.profileView))
	mux.Handle("PATCH /v1/profile", s.requirePermission(domain.PermissionManageSelf, s.updateProfileSettings))
	mux.Handle("GET /v1/discover", s.requirePermission(domain.PermissionManageSelf, s.discover))
	mux.Handle("GET /v1/discover/teams", s.requirePermission("", s.teamOpportunities))
	mux.Handle("POST /v1/teams/{teamID}/invites", s.requirePermission(domain.PermissionManageTeam, s.createInvite))
	mux.Handle("DELETE /v1/teams/{teamID}/invites/{inviteID}", s.requirePermission(domain.PermissionManageTeam, s.revokeInvite))
	mux.Handle("GET /v1/invites", s.requirePermission(domain.PermissionManageSelf, s.listInvites))
	mux.Handle("POST /v1/invites/{inviteID}", s.requirePermission(domain.PermissionManageSelf, s.respondToInvite))
	mux.Handle("GET /v1/users/{userID}/appearances", s.requirePermission("", s.appearances))
	mux.HandleFunc("GET /v1/events/{slug}/projects", s.projects)
	mux.HandleFunc("GET /v1/events/{slug}/tracks", s.listTracks)
	mux.Handle("POST /v1/events/{slug}/tracks", s.requirePermission(domain.PermissionManageEvent, s.createTrack))
	mux.HandleFunc("GET /v1/events/{slug}/prizes", s.listPrizes)
	mux.Handle("POST /v1/events/{slug}/prizes", s.requirePermission(domain.PermissionManageEvent, s.createPrize))
	mux.HandleFunc("GET /v1/events/{slug}/teams", s.listTeams)
	mux.Handle("POST /v1/events/{slug}/teams", s.requirePermission(domain.PermissionManageTeam, s.createTeam))
	mux.Handle("GET /v1/teams/{teamID}/members", s.requirePermission(domain.PermissionManageTeam, s.listTeamMembers))
	mux.Handle("POST /v1/teams/{teamID}/members/{userID}/promote", s.requirePermission(domain.PermissionManageTeam, s.promoteMember))
	mux.Handle("POST /v1/teams/{teamID}/members/{userID}/demote", s.requirePermission(domain.PermissionManageTeam, s.demoteMember))
	mux.Handle("POST /v1/teams/{teamID}/transfer", s.requirePermission(domain.PermissionManageTeam, s.transferCaptaincy))
	mux.Handle("DELETE /v1/teams/{teamID}/members/{userID}", s.requirePermission(domain.PermissionManageTeam, s.removeMember))
	mux.Handle("PATCH /v1/teams/{teamID}", s.requirePermission(domain.PermissionManageTeam, s.updateTeam))
	mux.Handle("DELETE /v1/teams/{teamID}", s.requirePermission(domain.PermissionManageTeam, s.deleteTeam))
	mux.Handle("POST /v1/events/{slug}/submissions", s.requirePermission(domain.PermissionSubmitProject, s.createSubmission))
	mux.Handle("GET /v1/judge/scores", s.requirePermission(domain.PermissionViewOwnScores, s.judgeScores))
	mux.Handle("PUT /v1/judge/projects/{projectID}/review", s.requirePermission(domain.PermissionReviewProject, s.saveReview))
	mux.Handle("GET /v1/submissions/{projectID}", s.requirePermission("", s.submissionDetail))
	mux.Handle("PATCH /v1/submissions/{projectID}", s.requirePermission(domain.PermissionSubmitProject, s.editSubmission))
	mux.Handle("POST /v1/submissions/{projectID}/submit", s.requirePermission(domain.PermissionSubmitProject, s.submitRevision))
	mux.Handle("POST /v1/submissions/{projectID}/withdraw", s.requirePermission(domain.PermissionSubmitProject, s.withdrawSubmission))
	mux.Handle("PUT /v1/organizer/submissions/{projectID}/eligibility", s.requirePermission(domain.PermissionManageSubmission, s.setEligibility))
	mux.Handle("PUT /v1/organizer/submissions/{projectID}/status", s.requirePermission(domain.PermissionManageSubmission, s.setSubmissionStatus))
	mux.Handle("GET /v1/organizer/duplicates", s.requirePermission(domain.PermissionManageSubmission, s.listDuplicates))
	mux.Handle("POST /v1/organizer/duplicates/scan", s.requirePermission(domain.PermissionManageSubmission, s.scanDuplicates))
	mux.Handle("PUT /v1/organizer/duplicates/{duplicateID}", s.requirePermission(domain.PermissionManageSubmission, s.resolveDuplicate))
	mux.Handle("GET /v1/organizer/assignments", s.requirePermission(domain.PermissionViewAssignments, s.organizerAssignments))
	mux.Handle("POST /v1/organizer/assignments", s.requirePermission(domain.PermissionManageAssignments, s.createAssignments))
	mux.Handle("DELETE /v1/organizer/assignments/{assignmentID}", s.requirePermission(domain.PermissionManageAssignments, s.revokeAssignment))
	mux.Handle("GET /v1/judge/assignments", s.requirePermission(domain.PermissionViewAssignments, s.judgeAssignments))
	mux.Handle("POST /v1/judge/assignments/{assignmentID}/conflict", s.requirePermission(domain.PermissionDeclareConflict, s.declareConflict))
	mux.Handle("GET /v1/organizer/rubrics", s.requirePermission(domain.PermissionManageEvent, s.listRubrics))
	mux.Handle("POST /v1/organizer/rubrics", s.requirePermission(domain.PermissionManageEvent, s.createRubric))
	mux.Handle("PUT /v1/organizer/rubrics/{rubricID}", s.requirePermission(domain.PermissionManageEvent, s.updateRubric))
	mux.Handle("POST /v1/organizer/rubrics/{rubricID}/publish", s.requirePermission(domain.PermissionManageEvent, s.publishRubric))
	mux.Handle("POST /v1/organizer/rubrics/{rubricID}/archive", s.requirePermission(domain.PermissionManageEvent, s.archiveRubric))
	mux.HandleFunc("GET /v1/judging/rubric", s.activeRubric)
	mux.Handle("GET /v1/organizer/progress", s.requirePermission(domain.PermissionManageEvent, s.progress))
	mux.Handle("GET /v1/organizer/reviews", s.requirePermission(domain.PermissionViewPeerScores, s.organizerReviews))
	mux.Handle("GET /v1/organizer/results", s.requirePermission(domain.PermissionViewPeerScores, s.results))
	mux.Handle("GET /v1/organizer/pairwise", s.requirePermission(domain.PermissionViewPeerScores, s.pairwise))
	mux.Handle("GET /v1/organizer/export.csv", s.requirePermission(domain.PermissionExportData, s.exportCSV))
	mux.Handle("GET /v1/organizer/export", s.requirePermission(domain.PermissionExportData, s.exportEvent))
	mux.Handle("POST /v1/organizer/import", s.requirePermission(domain.PermissionManageEvent, s.importEvent))
	mux.HandleFunc("GET /v1/openapi.json", s.openapi)
	mux.HandleFunc("GET /v1/endpoints", s.routeCatalog)

	// The HTML frontend is mounted after the API so that the explicit /v1
	// namespace can never be shadowed by a page pattern.
	s.registerWeb(mux)

	s.routes = mux.recorded()
	return s.middleware(mux)
}

// routePatterns returns the routes captured during handler construction.
func (s *Server) routePatterns(r *http.Request) []RouteEntry {
	if len(s.routes) == 0 {
		return nil
	}
	return s.routes
}

func (s *Server) SetClock(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

// optionalAuth attaches a principal when a valid session token is present and
// otherwise leaves the request anonymous, for endpoints that are public but
// behave differently for signed-in staff.
func (s *Server) optionalAuth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := auth.TokenFromRequest(r)
		if token == "" {
			next(w, r)
			return
		}
		claims, err := s.tokens.Parse(token)
		if err != nil {
			next(w, r)
			return
		}
		user, err := s.store.UserByID(claims.Subject)
		if err != nil {
			next(w, r)
			return
		}
		principal := auth.Principal{UserID: user.ID, Email: user.Email, Role: user.Role, SessionID: claims.SessionID}
		next(w, r.WithContext(auth.WithPrincipal(r.Context(), principal)))
	})
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

// guard applies a rate-limit budget to a route.
//
// The key is the authenticated account when there is one and the client address
// otherwise. Keying authenticated routes by account rather than by address
// matters in practice: a hackathon venue, a university, or a corporate network
// puts an entire panel behind one egress address, and a per-address budget
// there would let the busiest attendee lock everyone else out of commenting.
func (s *Server) guard(policy ratelimit.Policy, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := ""
		if principal, ok := auth.PrincipalFromContext(r.Context()); ok && principal.UserID != "" {
			key = principal.UserID
		} else {
			key = r.RemoteAddr
			if index := strings.LastIndex(key, ":"); index > 0 {
				key = key[:index]
			}
		}
		if !s.limiter.Guard(w, policy, key) {
			s.logger.Warn("rate limited a request",
				"policy", policy.Name, "method", r.Method, "path", r.URL.Path)
			return
		}
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

// logout clears the cookie and revokes the session behind it. Clearing the
// cookie alone is not logging out: the login response also hands back the token
// as a bearer credential, so a client that stored that token would keep working
// until the session expired on its own.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if token := auth.TokenFromRequest(r); token != "" {
		if claims, err := s.tokens.Parse(token); err == nil && claims.SessionID != "" {
			if err := s.tokens.Revoke(claims.SessionID, "logout"); err != nil && !errors.Is(err, auth.ErrSessionNotFound) {
				s.logger.Warn("could not revoke session on logout", "error", err)
			}
		}
	}
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
		Story         string            `json:"story"`
		RepositoryURL string            `json:"repo_url"`
		LiveURL       string            `json:"live_url"`
		VideoURL      string            `json:"video_url"`
		ThumbnailURL  string            `json:"thumbnail_url"`
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
	if err := s.store.ValidateAnswers(event.ID, domain.AudienceSubmission, request.CustomAnswers); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	if err := validateProjectLinks(request.RepositoryURL, request.LiveURL, request.VideoURL); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
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
		Story:         strings.TrimSpace(request.Story),
		ThumbnailURL:  strings.TrimSpace(request.ThumbnailURL),
		RepositoryURL: strings.TrimSpace(request.RepositoryURL),
		LiveURL:       strings.TrimSpace(request.LiveURL),
		VideoURL:      strings.TrimSpace(request.VideoURL),
		Tags:          request.Tags,
		CustomAnswers: request.CustomAnswers,
		Status:        domain.SubmissionSubmitted,
		Eligibility:   domain.EligibilityPending,
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
	s.recordActivity(r, principal.UserID, domain.ActivitySubmission, "submission.created", "submission", submission.ID, event.ID,
		team.Name+" submitted "+submission.Title, domain.ActivityPublic, map[string]any{"track_id": submission.TrackID, "version": submission.Version})
	s.webhooks.Emit(event.ID, "submission.created", event.Slug, map[string]any{
		"submission_id": submission.ID, "team_id": team.ID, "title": submission.Title, "track_id": submission.TrackID,
	})
	if _, err := s.mailService.SendSubmissionReceived(principal.UserID, event.ID, submission.ID, submission.Title, submission.Version); err != nil {
		s.logger.Warn("submission mail not queued", "submission_id", submission.ID, "error", err)
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
	if len(request.Criteria) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "at least one criterion score is required")
		return
	}
	project, err := s.store.SubmissionByID(projectID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "project not found")
		return
	}
	// The event a review belongs to is the event the project belongs to. A body
	// field cannot be allowed to choose it: taking event_id from the request
	// would let a judge file a review against an event they were assigned in
	// while the project actually lives somewhere else, and the row would then
	// vanish from the leaderboard that should have counted it.
	if request.EventID != "" && request.EventID != project.EventID {
		writeError(w, http.StatusUnprocessableEntity, "validation_error",
			"event_id does not match the event this project belongs to")
		return
	}
	rubric, err := s.store.ActiveRubric(project.EventID, project.TrackID)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "no published rubric is active for this project")
		return
	}
	if err := rubric.ValidateScores(request.Criteria); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	if !s.store.IsAssigned(project.EventID, principal.UserID, projectID) {
		writeError(w, http.StatusForbidden, "forbidden", "this project is not assigned to the current judge")
		return
	}
	now := s.now().UTC()
	review := domain.Review{ID: domain.NewID("rev"), EventID: project.EventID, JudgeID: principal.UserID, ProjectID: projectID, Criteria: request.Criteria, RubricID: rubric.ID, RubricVersion: rubric.Version, Comment: request.Comment, Submitted: request.Submitted, UpdatedAt: now}
	if request.Submitted {
		review.SubmittedAt = &now
	}
	saved, err := s.store.SaveReview(review)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrConflict):
			writeError(w, http.StatusConflict, "review_locked", "a submitted review cannot be changed")
		case errors.Is(err, domain.ErrValidation):
			writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		default:
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "review could not be saved")
		}
		return
	}
	if saved.Submitted {
		s.recordActivity(r, principal.UserID, domain.ActivityJudging, "review.submitted", "submission", saved.ProjectID, saved.EventID,
			"a judge completed a review", domain.ActivityParticipants, map[string]any{"review_id": saved.ID, "rubric_version": saved.RubricVersion})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": saved, "rubric": rubric})
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
