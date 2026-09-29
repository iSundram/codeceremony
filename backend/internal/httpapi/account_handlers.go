package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/authz"
	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

func (s *Server) authMethods(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": []map[string]any{
		{"id": "local", "enabled": true, "required": true},
	}})
}

// profile returns the caller's own account record.
//
// It is scoped by the session and not by anything in the request, so there is no
// path by which a caller reads another account's record. The teams and
// participations ride along because they are the same identity read: an account
// page that had to guess a second endpoint for "the teams I am in" would either
// fetch a platform-wide list and filter in the browser, which ships every team's
// membership to the client, or add an endpoint that does not exist.
func (s *Server) profile(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	user, err := s.store.UserByID(principal.UserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"user":           user.Redacted(),
		"profile":        mustProfile(s.store, principal.UserID),
		"memberships":    s.store.TeamMembershipsForUser(principal.UserID),
		"participations": s.store.Participations(principal.UserID, ""),
	}})
}

// mustProfile returns the profile or the zero value. A missing profile is a
// legitimate state for an account that has never edited one, so it is not an
// error worth failing a read over.
func mustProfile(portal *store.Store, userID string) domain.UserProfile {
	profile, err := portal.ProfileOrDefault(userID)
	if err != nil {
		return domain.UserProfile{UserID: userID}
	}
	return profile
}

func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	var request struct {
		DisplayName  string `json:"display_name"`
		AvatarURL    string `json:"avatar_url"`
		Bio          string `json:"bio"`
		Organization string `json:"organization"`
		Timezone     string `json:"timezone"`
		Locale       string `json:"locale"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	user, err := s.store.UpdateUserProfile(principal.UserID, request.DisplayName, request.AvatarURL, request.Bio, request.Organization, request.Timezone, request.Locale)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "profile could not be updated")
		return
	}
	s.audit(r, principal.UserID, "account.profile_updated", "user", principal.UserID, "", "", nil)
	writeJSON(w, http.StatusOK, map[string]any{"data": user.Redacted()})
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	var request struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	user, err := s.store.UserByID(principal.UserID)
	if err != nil || !auth.VerifyPassword(user.PasswordHash, request.CurrentPassword) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "current password is incorrect")
		return
	}
	hash, err := auth.HashPassword(request.NewPassword)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "new password must be at least 8 characters")
		return
	}
	if err := s.store.UpdatePasswordHash(principal.UserID, hash); err != nil {
		writeError(w, http.StatusInternalServerError, "password_error", "password could not be updated")
		return
	}
	s.store.RevokeUserSessions(principal.UserID, principal.SessionID, "password_changed")
	s.audit(r, principal.UserID, "account.password_changed", "user", principal.UserID, "", "", nil)
	_ = s.store.CreateNotification(domain.Notification{ID: domain.NewID("ntf"), UserID: principal.UserID, Kind: domain.NotificationAccountSecurity, Title: "Password changed", Body: "Your password was changed and other sessions were signed out.", CreatedAt: s.now().UTC()})
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

func (s *Server) exportAccount(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	user, err := s.store.UserByID(principal.UserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	s.audit(r, principal.UserID, "account.exported", "user", principal.UserID, "", "", nil)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"user":          user.Redacted(),
		"memberships":   s.store.TeamMembershipsForUser(principal.UserID),
		"sessions":      s.store.ListSessions(principal.UserID),
		"notifications": s.store.ListNotifications(principal.UserID),
	}})
}

func (s *Server) requestDeletion(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	if err := s.store.SetUserState(principal.UserID, domain.AccountDeletionPending); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "deletion_error", "account deletion could not be requested")
		return
	}
	s.store.RevokeUserSessions(principal.UserID, "", "deletion_requested")
	s.audit(r, principal.UserID, "account.deletion_requested", "user", principal.UserID, "", "user_requested", nil)
	scheduledFor := s.now().UTC().Add(30 * 24 * time.Hour)
	if _, err := s.mailService.SendDeletionScheduled(principal.UserID, scheduledFor); err != nil {
		s.logger.Warn("deletion notice not queued", "user_id", principal.UserID, "error", err)
	}
	s.recordActivity(r, principal.UserID, domain.ActivityAccount, "account.deletion_requested", "user", principal.UserID, "",
		"an account deletion was requested", domain.ActivityOrganizers, map[string]any{"scheduled_for": scheduledFor})
	writeJSON(w, http.StatusAccepted, map[string]any{"data": map[string]string{
		"status":        "deletion_pending",
		"scheduled_for": scheduledFor.Format(time.RFC3339),
		"cancel_within": "30 days",
	}})
}

func (s *Server) cancelDeletion(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	user, err := s.store.UserByID(principal.UserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	if user.State != domain.AccountDeletionPending {
		writeError(w, http.StatusConflict, "deletion_not_pending", "account deletion is not pending")
		return
	}
	if err := s.store.SetUserState(principal.UserID, domain.AccountActive); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "deletion_error", "account deletion could not be canceled")
		return
	}
	s.audit(r, principal.UserID, "account.deletion_canceled", "user", principal.UserID, "", "user_canceled", nil)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "active"}})
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"data": s.store.ListSessions(principal.UserID)})
}

func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	session, err := s.store.SessionByID(r.PathValue("sessionID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}
	if session.UserID != principal.UserID && !principal.Role.IsStaff() {
		writeError(w, http.StatusForbidden, "forbidden", "you cannot revoke another user's session")
		return
	}
	if err := s.store.RevokeSession(session.ID, "revoked_by_user"); err != nil {
		writeError(w, http.StatusInternalServerError, "session_error", "session could not be revoked")
		return
	}
	s.audit(r, principal.UserID, "session.revoked", "session", session.ID, "", "revoked_by_user", nil)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "revoked"}})
}

func (s *Server) revokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	count := s.store.RevokeUserSessions(principal.UserID, principal.SessionID, "revoked_by_user")
	s.audit(r, principal.UserID, "session.revoked_others", "user", principal.UserID, "", "revoked_by_user", map[string]any{"count": count})
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"status": "revoked", "count": count}})
}

func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"data": s.store.ListNotifications(principal.UserID)})
}

func (s *Server) markNotificationRead(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	if err := s.store.MarkNotificationRead(principal.UserID, r.PathValue("notificationID")); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "notification not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "read"}})
}

func (s *Server) listTeamMembers(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	team, err := s.store.TeamByID(r.PathValue("teamID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "team not found")
		return
	}
	if !principal.Role.IsStaff() {
		if _, err := s.store.TeamMembership(team.ID, principal.UserID); err != nil {
			writeError(w, http.StatusForbidden, "forbidden", "only team members can view this team roster")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": s.store.TeamMembers(team.ID)})
}

func (s *Server) promoteMember(w http.ResponseWriter, r *http.Request) {
	s.changeTeamRole(w, r, domain.TeamRoleLeader, "team.member.promoted")
}

func (s *Server) demoteMember(w http.ResponseWriter, r *http.Request) {
	s.changeTeamRole(w, r, domain.TeamRoleMember, "team.member.demoted")
}

func (s *Server) changeTeamRole(w http.ResponseWriter, r *http.Request, role domain.TeamRole, action string) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	team, target, ok := s.teamTarget(w, r, principal)
	if !ok {
		return
	}
	if target.Role == domain.TeamRoleCaptain {
		writeError(w, http.StatusConflict, "captain_required", "the captain cannot be demoted")
		return
	}
	if err := s.store.SetTeamMemberRole(team.ID, target.UserID, role); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "membership_error", "team role could not be changed")
		return
	}
	s.audit(r, principal.UserID, action, "team_member", team.ID+":"+target.UserID, team.EventID, "", map[string]any{"role": role})
	_ = s.store.CreateNotification(domain.Notification{ID: domain.NewID("ntf"), UserID: target.UserID, EventID: team.EventID, Kind: domain.NotificationTeamActivity, Title: "Your team role changed", Body: "Your team role was updated by an authorized organizer or captain.", CreatedAt: s.now().UTC()})
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

func (s *Server) transferCaptaincy(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	team, err := s.store.TeamByID(r.PathValue("teamID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "team not found")
		return
	}
	if !principal.Role.IsStaff() && team.CaptainID != principal.UserID {
		writeError(w, http.StatusForbidden, "forbidden", "only the captain can transfer captaincy")
		return
	}
	var request struct {
		NextCaptainID string `json:"next_captain_id"`
	}
	if err := decodeJSON(r, &request); err != nil || strings.TrimSpace(request.NextCaptainID) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "next_captain_id is required")
		return
	}
	if err := s.store.TransferTeamCaptaincy(team.ID, team.CaptainID, request.NextCaptainID); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "captaincy_error", "captaincy could not be transferred")
		return
	}
	s.audit(r, principal.UserID, "team.captaincy.transferred", "team", team.ID, team.EventID, "", map[string]any{"next_captain_id": request.NextCaptainID})
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "transferred"}})
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	team, target, ok := s.teamTarget(w, r, principal)
	if !ok {
		return
	}
	if target.Role == domain.TeamRoleCaptain {
		writeError(w, http.StatusConflict, "captain_required", "the captain cannot be removed")
		return
	}
	if err := s.store.RemoveTeamMember(team.ID, target.UserID); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "membership_error", "member could not be removed")
		return
	}
	s.audit(r, principal.UserID, "team.member.removed", "team_member", team.ID+":"+target.UserID, team.EventID, "", nil)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "removed"}})
}

func (s *Server) deleteTeam(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	team, err := s.store.TeamByID(r.PathValue("teamID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "team not found")
		return
	}
	if !principal.Role.IsStaff() && team.CaptainID != principal.UserID {
		writeError(w, http.StatusForbidden, "forbidden", "only the captain or an organizer can archive this team")
		return
	}
	if err := s.store.ArchiveTeam(team.ID); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "team_error", "team could not be archived")
		return
	}
	s.audit(r, principal.UserID, "team.archived", "team", team.ID, team.EventID, "", nil)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "archived"}})
}

func (s *Server) teamTarget(w http.ResponseWriter, r *http.Request, principal auth.Principal) (domain.Team, domain.TeamMembership, bool) {
	team, err := s.store.TeamByID(r.PathValue("teamID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "team not found")
		return domain.Team{}, domain.TeamMembership{}, false
	}
	if !principal.Role.IsStaff() && team.CaptainID != principal.UserID {
		writeError(w, http.StatusForbidden, "forbidden", "only the captain or an organizer can change team roles")
		return domain.Team{}, domain.TeamMembership{}, false
	}
	target, err := s.store.TeamMembership(team.ID, r.PathValue("userID"))
	if err != nil || target.Status != "active" {
		writeError(w, http.StatusNotFound, "not_found", "active team member not found")
		return domain.Team{}, domain.TeamMembership{}, false
	}
	return team, target, true
}

// audit records an action that succeeded.
//
// It writes two records on purpose, and the duplication is deliberate rather
// than an oversight:
//
//   - an AuditEvent, which is what GET /v1/admin/audit has always read. It is
//     mutable and unchained, which is fine for an operational view.
//   - an ActionAuditEntry, which is hash-chained, queryable by action, and the
//     record an organizer can hand to a third party to verify.
//
// Routing every existing call site through here is what gives the chain coverage
// across the whole surface without touching sixty call sites, and it means a
// handler that audits cannot accidentally skip the chain.
func (s *Server) audit(r *http.Request, actorID, action, targetType, targetID, eventID, reason string, metadata map[string]any) {
	_ = s.store.RecordAudit(domain.AuditEvent{ID: domain.NewID("aud"), EventID: eventID, ActorID: actorID, Action: action, TargetType: targetType, TargetID: targetID, Reason: reason, Metadata: metadata, RequestID: r.Header.Get("X-Request-ID"), CreatedAt: s.now().UTC()})

	principal := auth.Principal{UserID: actorID}
	if user, err := s.store.UserByID(actorID); err == nil {
		principal.Role = user.Role
		principal.Email = user.Email
		principal.State = user.State
	}
	entry := s.auditEntry(r, principal, authz.Action(action))
	entry = finalizeAudit(entry, targetType, targetID, eventID, reason, true)
	s.store.RecordAction(entry)
}
