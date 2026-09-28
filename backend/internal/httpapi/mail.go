package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/mailer"
)

func (s *Server) mailPreferences(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	preferences := s.store.MailPreferences(principal.UserID)
	writeJSON(w, http.StatusOK, map[string]any{
		"data":              preferences,
		"preferences_url":   s.mailService.PreferencesURL(),
		"unsubscribe_scope": []string{"marketing", "all"},
	})
}

func (s *Server) updateMailPreferences(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	preferences := s.store.MailPreferences(principal.UserID)
	var request struct {
		Transactional    *bool `json:"transactional"`
		AccountSecurity  *bool `json:"account_security"`
		AccountLifecycle *bool `json:"account_lifecycle"`
		TeamActivity     *bool `json:"team_activity"`
		EventActivity    *bool `json:"event_activity"`
		Judging          *bool `json:"judging"`
		Results          *bool `json:"results"`
		Marketing        *bool `json:"marketing"`
		DigestOnly       *bool `json:"digest_only"`
		WeeklyDigest     *bool `json:"weekly_digest"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	applyBool(&preferences.Transactional, request.Transactional)
	applyBool(&preferences.AccountSecurity, request.AccountSecurity)
	applyBool(&preferences.AccountLifecycle, request.AccountLifecycle)
	applyBool(&preferences.TeamActivity, request.TeamActivity)
	applyBool(&preferences.EventActivity, request.EventActivity)
	applyBool(&preferences.Judging, request.Judging)
	applyBool(&preferences.Results, request.Results)
	applyBool(&preferences.Marketing, request.Marketing)
	applyBool(&preferences.DigestOnly, request.DigestOnly)
	applyBool(&preferences.WeeklyDigest, request.WeeklyDigest)
	if preferences.Marketing {
		preferences.UnsubscribedAll = false
		preferences.UnsubscribedAt = nil
	}
	saved, err := s.store.SaveMailPreferences(preferences)
	if err != nil {
		if errors.Is(err, domain.ErrValidation) {
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "account security and lifecycle mail cannot be disabled while transactional mail is on")
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	s.recordActivity(r, principal.UserID, domain.ActivityAccount, "account.mail_preferences_updated", "user", principal.UserID, "", principal.UserID+" updated email preferences", domain.ActivityOrganizers, map[string]any{"marketing": saved.Marketing})
	writeJSON(w, http.StatusOK, map[string]any{"data": saved})
}

func applyBool(target *bool, value *bool) {
	if value != nil {
		*target = *value
	}
}

func (s *Server) unsubscribe(w http.ResponseWriter, r *http.Request) {
	tokenValue := strings.TrimSpace(r.PathValue("token"))
	if tokenValue == "" {
		tokenValue = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	if tokenValue == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "an unsubscribe token is required")
		return
	}
	token, err := s.store.UnsubscribeTokenByValue(tokenValue)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "this unsubscribe link is not valid")
		return
	}
	if token.Expired(s.now().UTC()) {
		if err := s.store.DeleteUnsubscribeToken(tokenValue); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "the unsubscribe link could not be processed")
			return
		}
		writeError(w, http.StatusGone, "link_expired", "this unsubscribe link has expired")
		return
	}
	preferences, err := s.store.UnsubscribeAll(token.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "the preferences could not be updated")
		return
	}
	if err := s.store.DeleteUnsubscribeToken(tokenValue); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "the unsubscribe link could not be consumed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":         preferences,
		"unsubscribed": true,
		"scope":        token.Scope,
	})
}

func (s *Server) requestVerificationMail(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	token, err := mailer.RandomToken(24)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "the token could not be generated")
		return
	}
	message, err := s.mailService.SendVerification(principal.UserID, token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "the verification email could not be queued")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"queued":         message.Status != domain.MailSkipped,
		"status":         message.Status,
		"skipped_reason": message.LastError,
	})
}

func (s *Server) requestPasswordResetMail(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	token, err := mailer.RandomToken(24)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "the token could not be generated")
		return
	}
	message, err := s.mailService.SendPasswordReset(principal.UserID, token, 2)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "the reset email could not be queued")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"queued":         message.Status != domain.MailSkipped,
		"status":         message.Status,
		"skipped_reason": message.LastError,
	})
}

func (s *Server) sendAnnouncement(w http.ResponseWriter, r *http.Request) {
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
		Headline string `json:"headline"`
		Body     string `json:"body"`
		Audience string `json:"audience"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	headline, body, audience := mailer.NormalizeAnnouncement(mailer.AnnouncementRequest{
		Headline: request.Headline,
		Body:     request.Body,
		SendTo:   request.Audience,
	}, s.now().UTC())
	if strings.TrimSpace(headline) == "" || strings.TrimSpace(body) == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "headline and body are required")
		return
	}
	switch audience {
	case "all", "seeking", "team_captains", "judges":
	default:
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "audience must be all, seeking, team_captains, or judges")
		return
	}
	result, err := s.mailService.SendAnnouncement(event, headline, body, audience)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "the announcement could not be queued")
		return
	}
	result["headline"] = headline
	s.recordActivity(r, principal.UserID, domain.ActivityCommunication, "event.announcement_sent", "event", event.ID, event.ID, principal.UserID+" announced \""+headline+"\" to "+audience, domain.ActivityOrganizers, map[string]any{"queued": len(result["queued"].([]string))})
	s.audit(r, principal.UserID, "event.announcement_sent", "event", event.ID, event.ID, headline, map[string]any{"audience": audience})
	writeJSON(w, http.StatusAccepted, result)
}

func (s *Server) mailOutbox(w http.ResponseWriter, r *http.Request) {
	status := domain.MailStatus(strings.TrimSpace(r.URL.Query().Get("status")))
	messages := s.store.ListMail(status, parseQueryInt(r, "limit", 50))
	views := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		views = append(views, map[string]any{
			"id":           message.ID,
			"user_id":      message.UserID,
			"template":     message.Template,
			"topic":        message.Topic,
			"kind":         message.Kind,
			"status":       message.Status,
			"attempts":     message.Attempts,
			"last_error":   message.LastError,
			"scheduled_at": message.ScheduledAt,
			"sent_at":      message.SentAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":   views,
		"count":  len(views),
		"stats":  s.store.MailStats(),
		"sender": s.mailService.Dispatcher().SenderName(),
	})
}

func (s *Server) mailTemplates(w http.ResponseWriter, r *http.Request) {
	templates := s.mailService.Registry().List()
	views := make([]map[string]any, 0, len(templates))
	for _, tmpl := range templates {
		views = append(views, map[string]any{
			"name":        tmpl.Name,
			"kind":        tmpl.Kind,
			"topic":       tmpl.Topic,
			"subject":     tmpl.Subject,
			"description": tmpl.Description,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": views, "count": len(views), "configured": s.cfg.SMTPConfigured()})
}

func (s *Server) flushMailQueue(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	delivered, err := s.mailService.Dispatcher().DispatchOnce(parseQueryInt(r, "limit", 25))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "the mail queue could not be flushed")
		return
	}
	s.audit(r, principal.UserID, "mail.queue_flushed", "mail_queue", "", "", "manual flush", map[string]any{"delivered": delivered})
	writeJSON(w, http.StatusOK, map[string]any{"delivered": delivered, "stats": s.store.MailStats()})
}

func (s *Server) sendReviewReminders(w http.ResponseWriter, r *http.Request) {
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
	sent := make([]string, 0)
	skipped := make([]string, 0)
	for _, entry := range s.store.JudgeRoster(event.ID) {
		pending := 0
		for _, assignment := range s.store.ListAssignments(event.ID, entry.JudgeID) {
			if assignment.RevokedAt != nil {
				continue
			}
			if review, err := s.store.ReviewForJudgeProject(entry.JudgeID, assignment.ProjectID); err != nil || !review.Submitted {
				pending++
			}
		}
		if pending == 0 {
			continue
		}
		message, err := s.mailService.SendReviewReminder(entry.JudgeID, event.ID, pending)
		if err != nil {
			skipped = append(skipped, entry.JudgeID)
			continue
		}
		if message.Status == domain.MailSkipped {
			skipped = append(skipped, entry.JudgeID)
			continue
		}
		sent = append(sent, entry.JudgeID)
	}
	s.recordActivity(r, principal.UserID, domain.ActivityJudging, "judging.reminders_sent", "event", event.ID, event.ID, principal.UserID+" sent judging reminders to "+strconv.Itoa(len(sent))+" judges", domain.ActivityJudges, map[string]any{"sent": len(sent), "skipped": len(skipped)})
	writeJSON(w, http.StatusAccepted, map[string]any{"sent": sent, "skipped": skipped})
}

func (s *Server) sendWeeklyDigests(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	sent := make([]string, 0)
	skipped := make([]string, 0)
	for _, user := range s.store.ListUsers() {
		preferences := s.store.MailPreferences(user.ID)
		if !preferences.WeeklyDigest || !preferences.Marketing {
			continue
		}
		pendingInvites := 0
		for _, invite := range s.store.InvitesForUser(user.ID) {
			if invite.Status == domain.InvitePending {
				pendingInvites++
			}
		}
		pendingAssignments := 0
		for _, participation := range s.store.Participations(user.ID, "") {
			for _, assignment := range s.store.ListAssignments(participation.EventID, user.ID) {
				if assignment.RevokedAt != nil {
					continue
				}
				if review, err := s.store.ReviewForJudgeProject(user.ID, assignment.ProjectID); err != nil || !review.Submitted {
					pendingAssignments++
				}
			}
		}
		hackathons := len(s.store.Participations(user.ID, ""))
		message, err := s.mailService.SendWeeklyDigest(user.ID, hackathons, pendingInvites, pendingAssignments)
		if err != nil || message.Status == domain.MailSkipped {
			skipped = append(skipped, user.ID)
			continue
		}
		sent = append(sent, user.ID)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"sent": sent, "skipped": skipped, "requested_by": principal.UserID})
}
