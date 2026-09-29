package httpapi

import (
	"context"
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
	"github.com/iSundram/codeceremony/backend/internal/authz"
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
	authz       *authz.Resolver

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
		authz:       authz.NewResolver(data),
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
	mux.Handle("GET /v1/me", s.requireAction(authz.ActionAccountReadSelf, s.me))
	mux.Handle("GET /v1/account/profile", s.requireAction(authz.ActionAccountReadSelf, s.profile))
	mux.Handle("PATCH /v1/account/profile", s.requireAction(authz.ActionAccountUpdateSelf, s.updateProfile))
	mux.Handle("POST /v1/account/password", s.requireAction(authz.ActionAccountUpdateSelf, s.changePassword))
	mux.Handle("GET /v1/account/export", s.requireAction(authz.ActionAccountExportSelf, s.exportAccount))
	mux.Handle("POST /v1/account/deletion", s.requireAction(authz.ActionAccountDeleteSelf, s.requestDeletion))
	mux.Handle("POST /v1/account/deletion/cancel", s.requireAction(authz.ActionAccountDeleteSelf, s.cancelDeletion))
	mux.Handle("GET /v1/account/sessions", s.requireAction(authz.ActionSessionReadOwn, s.listSessions))
	mux.Handle("DELETE /v1/account/sessions/{sessionID}", s.requireAction(authz.ActionSessionRevokeOwn, s.revokeSession))
	mux.Handle("POST /v1/account/sessions/revoke-others", s.requireAction(authz.ActionSessionRevokeOwn, s.revokeOtherSessions))
	mux.Handle("GET /v1/notifications", s.requireAction(authz.ActionNotificationReadOwn, s.listNotifications))
	mux.Handle("POST /v1/notifications/{notificationID}/read", s.requireAction(authz.ActionNotificationReadOwn, s.markNotificationRead))
	mux.Handle("GET /v1/admin/users", s.requireAction(authz.ActionAccountReadAny, s.adminListUsers))
	mux.Handle("PATCH /v1/admin/users/{userID}/state", s.requireAction(authz.ActionAccountSuspend, s.adminUpdateUserState))
	mux.Handle("PUT /v1/admin/users/{userID}/role", s.requireAction(authz.ActionAccountSetRole, s.adminUpdateUserRole))
	mux.Handle("POST /v1/admin/users/{userID}/sessions/revoke", s.requireAction(authz.ActionAccountRevokeSession, s.adminRevokeUserSessions))
	mux.Handle("GET /v1/admin/audit", s.requireAction(authz.ActionAuditRead, s.adminAudit))
	mux.Handle("GET /v1/events", s.optionalAuth(s.listEvents))
	mux.Handle("GET /v1/directory", s.optionalAuth(s.eventDirectory))
	mux.HandleFunc("GET /v1/permissions", s.permissionMatrix)
	mux.Handle("GET /v1/activity", s.requireAction(authz.ActionEventRead, s.activityFeed))
	mux.Handle("GET /v1/events/{slug}/activity", s.optionalAuth(s.activityFeed))
	mux.Handle("GET /v1/events/{slug}/staff", s.optionalAuth(s.listEventStaff))
	mux.Handle("POST /v1/events/{slug}/staff", s.requireAction(authz.ActionEventStaffManage, s.addEventStaff))
	mux.Handle("DELETE /v1/events/{slug}/staff/{userID}", s.requireAction(authz.ActionEventStaffManage, s.removeEventStaff))
	mux.Handle("GET /v1/email/preferences", s.requireAction(authz.ActionAccountReadSelf, s.mailPreferences))
	mux.Handle("PATCH /v1/email/preferences", s.requireAction(authz.ActionAccountUpdateSelf, s.updateMailPreferences))
	mux.Handle("POST /v1/email/verify", s.guard(ratelimit.Account, s.requireAction(authz.ActionAccountUpdateSelf, s.requestVerificationMail)))
	mux.Handle("POST /v1/email/password-reset", s.guard(ratelimit.Account, s.requireAction(authz.ActionAccountUpdateSelf, s.requestPasswordResetMail)))
	mux.HandleFunc("GET /v1/unsubscribe/{token}", s.unsubscribe)
	mux.HandleFunc("GET /v1/unsubscribe", s.unsubscribe)
	mux.Handle("POST /v1/organizer/events/{slug}/announcements", s.requireAction(authz.ActionMailSend, s.sendAnnouncement))
	mux.Handle("POST /v1/organizer/events/{slug}/review-reminders", s.requireAction(authz.ActionMailSend, s.sendReviewReminders))
	mux.Handle("POST /v1/organizer/mail/flush", s.requireAction(authz.ActionMailFlush, s.flushMailQueue))
	mux.Handle("POST /v1/organizer/mail/weekly-digest", s.requireAction(authz.ActionMailSend, s.sendWeeklyDigests))
	mux.Handle("GET /v1/organizer/mail/outbox", s.requireAction(authz.ActionMailRead, s.mailOutbox))
	mux.Handle("GET /v1/organizer/mail/templates", s.requireAction(authz.ActionMailRead, s.mailTemplates))
	mux.Handle("POST /v1/events", s.requireAction(authz.ActionEventCreate, s.createEvent))
	mux.HandleFunc("GET /v1/events/{slug}", s.hackathon)
	mux.Handle("PATCH /v1/events/{slug}", s.requireAction(authz.ActionEventUpdate, s.updateHackathon))
	mux.HandleFunc("GET /v1/events/{slug}/milestones", s.listMilestones)
	mux.Handle("POST /v1/events/{slug}/milestones", s.requireAction(authz.ActionEventConfigure, s.createMilestone))
	mux.HandleFunc("GET /v1/events/{slug}/hosts", s.listHosts)
	mux.Handle("POST /v1/events/{slug}/hosts", s.requireAction(authz.ActionEventConfigure, s.createHost))
	mux.HandleFunc("GET /v1/events/{slug}/questions", s.listQuestions)
	mux.Handle("POST /v1/events/{slug}/questions", s.requireAction(authz.ActionEventConfigure, s.createQuestion))
	mux.Handle("PUT /v1/organizer/questions/{questionID}", s.requireAction(authz.ActionEventConfigure, s.updateQuestion))
	mux.Handle("DELETE /v1/organizer/questions/{questionID}", s.requireAction(authz.ActionEventConfigure, s.deleteQuestion))
	mux.Handle("GET /v1/events/{slug}/comments", s.optionalAuth(s.listComments))
	mux.Handle("POST /v1/events/{slug}/projects/{projectID}/comments", s.guard(ratelimit.Comment, s.requireAction(authz.ActionCommentCreate, s.postComment)))
	mux.Handle("DELETE /v1/comments/{commentID}", s.requireAction(authz.ActionCommentDeleteOwn, s.deleteComment))
	mux.Handle("PUT /v1/organizer/comments/{commentID}/moderate", s.requireAction(authz.ActionCommentModerate, s.moderateComment))
	mux.Handle("POST /v1/comments/{commentID}/report", s.guard(ratelimit.Comment, s.requireAction(authz.ActionCommentReport, s.reportComment)))
	mux.Handle("GET /v1/organizer/reports", s.requireAction(authz.ActionCommentModerate, s.listReports))
	mux.Handle("PUT /v1/organizer/reports/{reportID}", s.requireAction(authz.ActionCommentModerate, s.resolveReport))
	mux.HandleFunc("GET /v1/events/{slug}/vote-campaigns", s.listCampaigns)
	mux.Handle("GET /v1/events/{slug}/vote/ballot", s.guard(ratelimit.Ballot, s.optionalAuth(s.ballotOptions)))
	mux.Handle("POST /v1/organizer/events/{slug}/vote-campaigns", s.requireAction(authz.ActionVoteCampaignManage, s.createCampaign))
	mux.Handle("PUT /v1/organizer/vote-campaigns/{campaignID}/{status}", s.requireAction(authz.ActionVoteCampaignManage, s.setCampaignStatus))
	mux.Handle("POST /v1/events/{slug}/vote", s.guard(ratelimit.Ballot, s.requireAction(authz.ActionVoteCast, s.castVotes)))
	mux.Handle("GET /v1/events/{slug}/vote", s.optionalAuth(s.voteResults))
	mux.Handle("GET /v1/vote/mine", s.requireAction(authz.ActionVoteCast, s.myVotes))
	mux.Handle("GET /v1/events/{slug}/webhooks", s.requireAction(authz.ActionIntegrationManage, s.listWebhooks))
	mux.Handle("POST /v1/organizer/events/{slug}/webhooks", s.requireAction(authz.ActionIntegrationManage, s.createWebhook))
	mux.Handle("DELETE /v1/organizer/webhooks/{webhookID}", s.requireAction(authz.ActionIntegrationManage, s.deleteWebhook))
	mux.Handle("POST /v1/organizer/webhooks/{webhookID}/test", s.requireAction(authz.ActionIntegrationManage, s.testWebhook))
	mux.Handle("GET /v1/organizer/webhooks/deliveries", s.requireAction(authz.ActionIntegrationManage, s.listDeliveries))
	mux.Handle("POST /v1/organizer/webhooks/flush", s.requireAction(authz.ActionIntegrationManage, s.flushWebhooks))
	mux.Handle("GET /v1/events/{slug}/judges", s.optionalAuth(s.listJudges))
	mux.Handle("POST /v1/events/{slug}/judges", s.requireAction(authz.ActionPanelManage, s.addJudge))
	mux.Handle("DELETE /v1/events/{slug}/judges/{judgeID}", s.requireAction(authz.ActionPanelManage, s.removeJudge))
	mux.Handle("GET /v1/events/{slug}/leaderboard", s.optionalAuth(s.leaderboard))
	mux.Handle("POST /v1/organizer/events/{slug}/publish-results", s.requireAction(authz.ActionResultsPublish, s.publishResults))
	mux.Handle("POST /v1/organizer/events/{slug}/unpublish-results", s.requireAction(authz.ActionResultsPublish, s.unpublishResults))
	mux.Handle("GET /v1/profiles/{userID}", s.requireAction(authz.ActionDirectoryRead, s.profileView))
	mux.Handle("PATCH /v1/profile", s.requireAction(authz.ActionAccountUpdateSelf, s.updateProfileSettings))
	mux.Handle("GET /v1/discover", s.requireAction(authz.ActionAccountReadSelf, s.discover))
	mux.Handle("GET /v1/discover/teams", s.requireAction(authz.ActionDirectoryRead, s.teamOpportunities))
	mux.Handle("POST /v1/teams/{teamID}/invites", s.requireAction(authz.ActionTeamInvite, s.createInvite))
	mux.Handle("DELETE /v1/teams/{teamID}/invites/{inviteID}", s.requireAction(authz.ActionTeamInvite, s.revokeInvite))
	mux.Handle("GET /v1/invites", s.requireAction(authz.ActionAccountReadSelf, s.listInvites))
	mux.Handle("POST /v1/invites/{inviteID}", s.requireAction(authz.ActionTeamUpdate, s.respondToInvite))
	mux.Handle("GET /v1/users/{userID}/appearances", s.requireAction(authz.ActionDirectoryRead, s.appearances))
	mux.HandleFunc("GET /v1/events/{slug}/projects", s.projects)
	mux.HandleFunc("GET /v1/events/{slug}/tracks", s.listTracks)
	mux.Handle("POST /v1/events/{slug}/tracks", s.requireAction(authz.ActionEventConfigure, s.createTrack))
	mux.HandleFunc("GET /v1/events/{slug}/prizes", s.listPrizes)
	mux.Handle("POST /v1/events/{slug}/prizes", s.requireAction(authz.ActionEventConfigure, s.createPrize))
	mux.HandleFunc("GET /v1/events/{slug}/teams", s.listTeams)
	mux.Handle("POST /v1/events/{slug}/teams", s.requireAction(authz.ActionTeamCreate, s.createTeam))
	mux.Handle("GET /v1/teams/{teamID}/members", s.requireAction(authz.ActionTeamUpdate, s.listTeamMembers))
	mux.Handle("POST /v1/teams/{teamID}/members/{userID}/promote", s.requireAction(authz.ActionTeamManageUser, s.promoteMember))
	mux.Handle("POST /v1/teams/{teamID}/members/{userID}/demote", s.requireAction(authz.ActionTeamManageUser, s.demoteMember))
	mux.Handle("POST /v1/teams/{teamID}/transfer", s.requireAction(authz.ActionTeamManageUser, s.transferCaptaincy))
	mux.Handle("DELETE /v1/teams/{teamID}/members/{userID}", s.requireAction(authz.ActionTeamManageUser, s.removeMember))
	mux.Handle("PATCH /v1/teams/{teamID}", s.requireAction(authz.ActionTeamUpdate, s.updateTeam))
	mux.Handle("DELETE /v1/teams/{teamID}", s.requireAction(authz.ActionTeamDelete, s.deleteTeam))
	mux.Handle("POST /v1/events/{slug}/submissions", s.requireAction(authz.ActionSubmissionCreate, s.createSubmission))
	mux.Handle("GET /v1/judge/scores", s.requireAction(authz.ActionReviewReadOwn, s.judgeScores))
	mux.Handle("PUT /v1/judge/projects/{projectID}/review", s.requireAction(authz.ActionReviewWriteOwn, s.saveReview))
	mux.Handle("GET /v1/submissions/{projectID}", s.requireAction(authz.ActionSubmissionRead, s.submissionDetail))
	mux.Handle("PATCH /v1/submissions/{projectID}", s.requireAction(authz.ActionSubmissionUpdateOwn, s.editSubmission))
	mux.Handle("POST /v1/submissions/{projectID}/submit", s.requireAction(authz.ActionSubmissionSubmit, s.submitRevision))
	mux.Handle("POST /v1/submissions/{projectID}/withdraw", s.requireAction(authz.ActionSubmissionWithdraw, s.withdrawSubmission))
	mux.Handle("PUT /v1/organizer/submissions/{projectID}/eligibility", s.requireAction(authz.ActionSubmissionSetEligiblity, s.setEligibility))
	mux.Handle("PUT /v1/organizer/submissions/{projectID}/status", s.requireAction(authz.ActionSubmissionUpdateAny, s.setSubmissionStatus))
	mux.Handle("GET /v1/organizer/duplicates", s.requireAction(authz.ActionDuplicateScan, s.listDuplicates))
	mux.Handle("POST /v1/organizer/duplicates/scan", s.requireAction(authz.ActionDuplicateScan, s.scanDuplicates))
	mux.Handle("PUT /v1/organizer/duplicates/{duplicateID}", s.requireAction(authz.ActionDuplicateResolve, s.resolveDuplicate))
	mux.Handle("GET /v1/organizer/assignments", s.requireAction(authz.ActionAssignmentRead, s.organizerAssignments))
	mux.Handle("POST /v1/organizer/assignments", s.requireAction(authz.ActionAssignmentBulk, s.createAssignments))
	mux.Handle("DELETE /v1/organizer/assignments/{assignmentID}", s.requireAction(authz.ActionAssignmentRevoke, s.revokeAssignment))
	mux.Handle("GET /v1/judge/assignments", s.requireAction(authz.ActionAssignmentRead, s.judgeAssignments))
	mux.Handle("POST /v1/judge/assignments/{assignmentID}/conflict", s.requireAction(authz.ActionJudgeConflict, s.declareConflict))
	mux.Handle("GET /v1/organizer/rubrics", s.requireAction(authz.ActionRubricCreate, s.listRubrics))
	mux.Handle("POST /v1/organizer/rubrics", s.requireAction(authz.ActionRubricCreate, s.createRubric))
	mux.Handle("PUT /v1/organizer/rubrics/{rubricID}", s.requireAction(authz.ActionRubricUpdate, s.updateRubric))
	mux.Handle("POST /v1/organizer/rubrics/{rubricID}/publish", s.requireAction(authz.ActionRubricPublish, s.publishRubric))
	mux.Handle("POST /v1/organizer/rubrics/{rubricID}/archive", s.requireAction(authz.ActionRubricArchive, s.archiveRubric))
	mux.HandleFunc("GET /v1/judging/rubric", s.activeRubric)
	mux.Handle("GET /v1/organizer/progress", s.requireAction(authz.ActionProgressRead, s.progress))
	mux.Handle("GET /v1/organizer/reviews", s.requireAction(authz.ActionReviewReadPeer, s.organizerReviews))
	mux.Handle("GET /v1/organizer/results", s.requireAction(authz.ActionResultsRead, s.results))
	mux.Handle("GET /v1/organizer/pairwise", s.requireAction(authz.ActionResultsRead, s.pairwise))
	mux.Handle("GET /v1/organizer/export.csv", s.requireAction(authz.ActionExportCSV, s.exportCSV))
	mux.Handle("GET /v1/organizer/export", s.requireAction(authz.ActionExportFull, s.exportEvent))
	mux.Handle("POST /v1/organizer/import", s.requireAction(authz.ActionImportApply, s.importEvent))
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

// requireAction authenticates the caller and resolves one authorization
// question through the single authz resolver.
//
// This is the only place a request is authenticated, and the only place a
// role-level decision is made. Everything finer-grained — is this judge assigned
// to this project, is this review already submitted, does this organizer hold an
// event role on this event — is expressed by building a richer Target and
// asking the resolver again, or by an object check in the handler that needs the
// object.
//
// Two properties are worth stating because they are easy to lose:
//
//   - The role is re-read from the store on every request, never taken from the
//     token, so a role change takes effect on the next request and a stale token
//     cannot escalate.
//   - A denial is recorded, not just returned. Refusals are the only signal that
//     someone is probing, and before action auditing they vanished entirely.
func (s *Server) requireAction(action authz.Action, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := s.authenticate(w, r)
		if !ok {
			return
		}
		target, ok := s.routeTarget(r)
		if !ok {
			writeError(w, http.StatusNotFound, "not_found", "event not found")
			return
		}
		decision := s.resolve(principal, action, target, r)
		if !decision.Allowed {
			s.denied(r, principal, action, decision, w)
			return
		}
		// The resolved event is put on the context so a handler acting on a
		// different event than the path named is detectable rather than silent.
		ctx := auth.WithPrincipal(r.Context(), principal)
		if target.EventID != "" {
			ctx = context.WithValue(ctx, routeEventKey{}, target.EventID)
		}
		next(w, r.WithContext(ctx))
	})
}

// routeEventKey carries the event the route path resolved to.
type routeEventKey struct{}

// routeEvent returns the event id the request path resolved to, or "" for a
// route that is not event-scoped.
func routeEvent(r *http.Request) string {
	id, _ := r.Context().Value(routeEventKey{}).(string)
	return id
}

// routeTarget builds the authorization target for a request, resolving the event
// from the {slug} path value when the route has one.
//
// This is what makes the gate per-event rather than per-role. A route addressed
// by event slug is checked against that event's staff, so a co-organizer of one
// event is refused on another instead of being allowed by a global role and left
// for each handler to fence. Handlers still re-check with the object they load;
// this establishes the event fence and makes the mismatch detectable.
func (s *Server) routeTarget(r *http.Request) (authz.Target, bool) {
	slug := r.PathValue("slug")
	if slug == "" {
		return authz.Target{}, true
	}
	event, err := s.store.EventBySlug(slug)
	if err != nil {
		return authz.Target{}, false
	}
	return authz.Target{EventID: event.ID, Resource: "event"}, true
}

// requireRouteEvent refuses when a handler is about to act on an event other
// than the one its path named.
//
// Without this, a route like /events/{slug}/submissions could authorize against
// {slug} and then write to whatever event the body asked for. The event a
// request acts on must be the event its path identified.
func (s *Server) requireRouteEvent(w http.ResponseWriter, r *http.Request, eventID string) bool {
	route := routeEvent(r)
	if route == "" || eventID == "" || route == eventID {
		return true
	}
	writeError(w, http.StatusUnprocessableEntity, "event_mismatch",
		"this request path names a different event than the one it acts on")
	return false
}

// authenticate resolves the caller from the request token.
//
// It deliberately does not apply the account-state gate. requireAction gets that
// from the resolver, which is where it belongs, because the resolver has to know
// which action is being attempted in order to allow the one case a pending
// deletion is permitted. A caller that authenticates but resolves no action must
// still be gated, and accountUsable below is what gates it.
func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) (auth.Principal, bool) {
	token := auth.TokenFromRequest(r)
	if token == "" {
		s.unauthenticated(r, "no token presented")
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return auth.Principal{}, false
	}
	claims, err := s.tokens.Parse(token)
	if err != nil {
		s.unauthenticated(r, "token did not parse: "+err.Error())
		writeError(w, http.StatusUnauthorized, "unauthorized", "the session is invalid or expired")
		return auth.Principal{}, false
	}
	user, err := s.store.UserByID(claims.Subject)
	if err != nil {
		s.unauthenticated(r, "session subject no longer exists")
		writeError(w, http.StatusUnauthorized, "unauthorized", "the session user no longer exists")
		return auth.Principal{}, false
	}
	return auth.Principal{
		UserID: user.ID, Email: user.Email, Role: user.Role,
		SessionID: claims.SessionID, State: user.State,
	}, true
}

// accountUsable reports whether an account in this state may act at all.
//
// A suspended or deletion-pending account is refused everything except
// cancelling its own deletion request, which is why the check takes the action
// rather than living inside authenticate: without the action it could not tell
// the one permitted case from the rest.
func accountUsable(principal auth.Principal, action authz.Action) bool {
	if principal.State == "" || principal.State == domain.AccountActive {
		return true
	}
	return principal.State == domain.AccountDeletionPending && action == authz.ActionAccountDeleteSelf
}

// resolve asks the authz resolver on behalf of an authenticated principal,
// filling in the per-event role from the store so handlers do not have to.
//
// The event id comes from the target the handler supplies, which is built from
// the loaded object. It never comes from the request, because a caller who can
// choose the event can choose the fence.
func (s *Server) resolve(principal auth.Principal, action authz.Action, target authz.Target, r *http.Request) authz.Decision {
	authzPrincipal := authz.Principal{
		UserID: principal.UserID, Email: principal.Email, Role: principal.Role,
		AccountState: principal.State,
	}
	if target.EventID != "" {
		if role, ok := s.store.EventRoleFor(target.EventID, principal.UserID); ok {
			// Both the role and the event it was resolved for travel together.
			// The resolver checks they match, so a role cannot be presented
			// against an event it was not granted on.
			authzPrincipal.EventRole = role
			authzPrincipal.EventRoleFor = target.EventID
		}
	}
	return s.authz.Resolve(authzPrincipal, action, target)
}

// authorize is the per-handler form. A handler calls it with the action and the
// object it has actually loaded, and gets back the decision.
//
// It exists so that the object-level rules live in one place rather than being
// re-derived in each handler and drifting. A handler that forgets to call it
// still gets the route-level check from requireAction; calling it is how a
// handler tightens that check to the object.
func (s *Server) authorize(w http.ResponseWriter, r *http.Request, action authz.Action, target authz.Target) (auth.Principal, bool) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return principal, false
	}
	decision := s.resolve(principal, action, target, r)
	if !decision.Allowed {
		s.denied(r, principal, action, decision, w)
		return principal, false
	}
	return principal, true
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
	// The assignment is the object-level half of the judging authorization, and
	// it is checked here because only the handler holds the project. The
	// route-level gate asked the same question with an unknown target and
	// deferred; this resolves it definitively.
	assignment := authz.AssignmentNotHeld
	if s.store.IsAssigned(project.EventID, principal.UserID, projectID) {
		assignment = authz.AssignmentHeld
	}
	if _, ok := s.authorize(w, r, authz.ActionReviewWriteOwn, authz.Target{
		EventID:    project.EventID,
		ObjectID:   projectID,
		Resource:   "submission",
		Assignment: assignment,
	}); !ok {
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
