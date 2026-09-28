package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/domain"
)

func (s *Server) profileView(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("userID")
	if userID == "" {
		userID = "me"
	}
	if userID == "me" {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
			return
		}
		userID = principal.UserID
	}
	user, err := s.store.UserByID(userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	profile, err := s.store.ProfileOrDefault(userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	payload := map[string]any{
		"user":        user,
		"profile":     profile,
		"links":       profile.LinkList(),
		"teams":       s.store.TeamMembershipsForUser(userID),
		"appearances": s.appearanceViews(s.store.Participations(userID, "")),
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) updateProfileSettings(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	profile, err := s.store.ProfileOrDefault(principal.UserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "profile not found")
		return
	}
	var request struct {
		Headline       *string            `json:"headline"`
		Bio            *string            `json:"bio"`
		Location       *string            `json:"location"`
		Skills         *[]string          `json:"skills"`
		Links          *map[string]string `json:"links"`
		Availability   *string            `json:"availability"`
		OpenToInvites  *bool              `json:"open_to_invites"`
		SeekingTeam    *bool              `json:"seeking_team"`
		SeekingRole    *string            `json:"seeking_role"`
		SeekingEventID *string            `json:"seeking_event_id"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if request.Headline != nil {
		profile.Headline = strings.TrimSpace(*request.Headline)
	}
	if request.Bio != nil {
		profile.Bio = strings.TrimSpace(*request.Bio)
	}
	if request.Location != nil {
		profile.Location = strings.TrimSpace(*request.Location)
	}
	if request.Skills != nil {
		profile.Skills = *request.Skills
	}
	if request.Links != nil {
		profile.Links = *request.Links
	}
	if request.Availability != nil {
		profile.Availability = domain.UserAvailability(strings.TrimSpace(*request.Availability))
	}
	if request.OpenToInvites != nil {
		profile.OpenToInvites = *request.OpenToInvites
	}
	if request.SeekingTeam != nil {
		profile.SeekingTeam = *request.SeekingTeam
	}
	if request.SeekingRole != nil {
		profile.SeekingRole = strings.TrimSpace(*request.SeekingRole)
	}
	if request.SeekingEventID != nil {
		profile.SeekingEventID = strings.TrimSpace(*request.SeekingEventID)
	}
	// availability is the source of truth when it is set explicitly; seeking_team
	// is derived from it so the two fields can never disagree.
	if request.Availability != nil {
		profile.SeekingTeam = profile.Availability == domain.UserAvailabilityLookingForTeam
		if profile.Availability == domain.UserAvailabilityLookingForTeam {
			profile.OpenToInvites = true
		}
	} else if profile.SeekingTeam {
		profile.Availability = domain.UserAvailabilityLookingForTeam
		profile.OpenToInvites = true
	}
	if profile.Availability == domain.UserAvailabilityTeamed {
		profile.SeekingTeam = false
		profile.OpenToInvites = false
	}
	saved, err := s.store.SaveProfile(profile)
	if err != nil {
		if errors.Is(err, domain.ErrValidation) {
			writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "the profile could not be saved")
		return
	}
	if _, err := s.store.UpdateUserProfile(principal.UserID, "", "", saved.Bio, "", "", ""); err != nil {
		s.logger.Warn("profile bio mirror failed", "user_id", principal.UserID, "error", err)
	}
	s.audit(r, principal.UserID, "profile.updated", "user", principal.UserID, profile.SeekingEventID, "profile updated", nil)
	writeJSON(w, http.StatusOK, map[string]any{"data": saved, "links": saved.LinkList()})
}

func (s *Server) discover(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	eventID := strings.TrimSpace(r.URL.Query().Get("event_id"))
	if eventID != "" {
		if _, err := s.store.EventByID(eventID); err != nil {
			writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
			return
		}
	}
	seeking := r.URL.Query().Get("seeking") != "false"
	profiles := s.store.SearchProfiles(eventID, seeking)
	views := make([]map[string]any, 0, len(profiles))
	for _, profile := range profiles {
		if profile.UserID == principal.UserID {
			continue
		}
		view := map[string]any{"profile": profile, "links": profile.LinkList()}
		if user, err := s.store.UserByID(profile.UserID); err == nil {
			view["display_name"] = user.DisplayName
			view["avatar_url"] = user.AvatarURL
			view["organization"] = user.Organization
		}
		views = append(views, view)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": views, "count": len(views), "event_id": eventID})
}

func (s *Server) teamOpportunities(w http.ResponseWriter, r *http.Request) {
	eventID := strings.TrimSpace(r.URL.Query().Get("event_id"))
	if eventID != "" {
		if _, err := s.store.EventByID(eventID); err != nil {
			writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
			return
		}
	}
	opportunities := s.store.TeamOpportunities(eventID)
	writeJSON(w, http.StatusOK, map[string]any{"data": opportunities, "count": len(opportunities), "event_id": eventID})
}

func (s *Server) createInvite(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	teamID := r.PathValue("teamID")
	var request struct {
		InviteeID      string `json:"invitee_id"`
		InviteeMail    string `json:"invitee_email"`
		Message        string `json:"message"`
		ExpiresInHours int    `json:"expires_in_hours"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	membership, err := s.store.TeamMembership(teamID, principal.UserID)
	if err != nil {
		writeError(w, http.StatusForbidden, "forbidden", "only team members can send invites")
		return
	}
	if membership.Role != domain.TeamRoleCaptain && membership.Role != domain.TeamRoleLeader {
		writeError(w, http.StatusForbidden, "forbidden", "only a captain or leader can send invites")
		return
	}
	team, err := s.store.TeamByID(teamID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "team not found")
		return
	}
	if team.Availability == domain.TeamClosed || team.Availability == domain.TeamFull {
		writeError(w, http.StatusConflict, "team_closed", "this team is not accepting invites")
		return
	}
	if team.MaxSize > 0 && len(s.store.TeamMembers(teamID)) >= team.MaxSize {
		writeError(w, http.StatusConflict, "team_full", "this team is already full")
		return
	}
	inviteeID := strings.TrimSpace(request.InviteeID)
	inviteeMail := strings.TrimSpace(request.InviteeMail)
	if inviteeID != "" {
		if profile, err := s.store.Profile(inviteeID); err == nil && !profile.OpenToInvites && !profile.SeekingTeam {
			writeError(w, http.StatusConflict, "invites_closed", "this participant is not accepting team invites")
			return
		}
		if member, err := s.store.TeamMembership(teamID, inviteeID); err == nil && member.Status == "active" {
			writeError(w, http.StatusConflict, "already_member", "this participant is already on the team")
			return
		}
	}
	invite, err := s.store.CreateInvite(domain.TeamInvite{
		TeamID:      teamID,
		EventID:     team.EventID,
		InviterID:   principal.UserID,
		InviteeID:   inviteeID,
		InviteeMail: inviteeMail,
		Message:     strings.TrimSpace(request.Message),
		Role:        domain.TeamRoleMember,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrAlreadyExists):
			writeError(w, http.StatusConflict, "already_member", "this participant is already on the team")
		case errors.Is(err, domain.ErrForbidden):
			writeError(w, http.StatusForbidden, "forbidden", "only team members can send invites")
		case errors.Is(err, domain.ErrValidation):
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "invitee_id or invitee_email is required")
		case errors.Is(err, domain.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "team or invitee not found")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "the invite could not be created")
		}
		return
	}
	s.audit(r, principal.UserID, "team.invite_sent", "team", teamID, team.EventID, "team invite sent", map[string]any{"invite_id": invite.ID})
	writeJSON(w, http.StatusCreated, map[string]any{"data": invite})
}

func (s *Server) listInvites(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	incoming := s.inviteViews(s.store.InvitesForUser(principal.UserID), principal)
	outgoing := make([]map[string]any, 0)
	for _, membership := range s.store.TeamMembershipsForUser(principal.UserID) {
		if membership.Role != domain.TeamRoleCaptain && membership.Role != domain.TeamRoleLeader {
			continue
		}
		outgoing = append(outgoing, s.inviteViews(s.store.InvitesForTeam(membership.TeamID), principal)...)
	}
	writeJSON(w, http.StatusOK, map[string]any{"incoming": incoming, "outgoing": outgoing})
}

func (s *Server) inviteViews(invites []domain.TeamInvite, principal auth.Principal) []map[string]any {
	views := make([]map[string]any, 0, len(invites))
	for _, invite := range invites {
		view := map[string]any{"invite": invite, "direction": "incoming"}
		if invite.InviterID == principal.UserID {
			view["direction"] = "outgoing"
		}
		if team, err := s.store.TeamByID(invite.TeamID); err == nil {
			view["team"] = team
		}
		if inviter, err := s.store.UserByID(invite.InviterID); err == nil {
			view["inviter"] = map[string]string{"user_id": inviter.ID, "display_name": inviter.DisplayName}
		}
		views = append(views, view)
	}
	return views
}

func (s *Server) respondToInvite(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	user, err := s.store.UserByID(principal.UserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	inviteID := r.PathValue("inviteID")
	invite, err := s.store.InviteByID(inviteID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "invite not found")
		return
	}
	if !s.store.InviteTargetsUser(invite, principal.UserID, user.Email) {
		writeError(w, http.StatusForbidden, "forbidden", "this invite was not addressed to you")
		return
	}
	var request struct {
		Decision string `json:"decision"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	decision := domain.InviteStatus(strings.TrimSpace(request.Decision))
	updated, err := s.store.RespondToInvite(inviteID, principal.UserID, decision)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrConflict):
			writeError(w, http.StatusConflict, "invite_closed", "this invite was already answered or has expired")
		case errors.Is(err, domain.ErrValidation):
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "decision must be accepted or declined")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "the invite could not be updated")
		}
		return
	}
	if updated.Status == domain.InviteAccepted {
		if team, err := s.store.TeamByID(updated.TeamID); err == nil {
			if err := s.store.AddMembershipRecord(principal.UserID, team.ID, team.EventID, domain.ParticipationMember); err == nil {
				_ = team
			}
			if err := s.store.CreateNotification(domain.Notification{
				UserID:    principal.UserID,
				EventID:   team.EventID,
				Kind:      domain.NotificationTeamActivity,
				Title:     "You joined a team",
				Body:      "You are now on " + team.Name + ".",
				ActionURL: "/teams/" + team.ID,
			}); err == nil {
				_ = err
			}
		}
	}
	s.audit(r, principal.UserID, "team.invite_"+string(updated.Status), "team", updated.TeamID, updated.EventID, "invite "+string(updated.Status), map[string]any{"invite_id": inviteID})
	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

func (s *Server) revokeInvite(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	updated, err := s.store.RevokeInvite(r.PathValue("inviteID"), principal.UserID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrForbidden):
			writeError(w, http.StatusForbidden, "forbidden", "only the sender can revoke an invite")
		case errors.Is(err, domain.ErrConflict):
			writeError(w, http.StatusConflict, "invite_closed", "this invite is no longer pending")
		default:
			writeError(w, http.StatusNotFound, "not_found", "invite not found")
		}
		return
	}
	s.audit(r, principal.UserID, "team.invite_revoked", "team", updated.TeamID, updated.EventID, "invite revoked", nil)
	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

func (s *Server) appearances(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("userID")
	if userID == "" || userID == "me" {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
			return
		}
		userID = principal.UserID
	}
	if _, err := s.store.UserByID(userID); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	eventID := strings.TrimSpace(r.URL.Query().Get("event_id"))
	participations := s.store.Participations(userID, eventID)
	views := s.appearanceViews(participations)
	writeJSON(w, http.StatusOK, map[string]any{"data": views, "count": len(views), "user_id": userID})
}

func (s *Server) appearanceViews(participations []domain.Participation) []map[string]any {
	views := make([]map[string]any, 0, len(participations))
	for _, participation := range participations {
		view := map[string]any{
			"participation": participation,
			"role":          participation.Role,
		}
		if event, err := s.store.EventByID(participation.EventID); err == nil {
			view["hackathon"] = map[string]any{
				"id":    event.ID,
				"slug":  event.Slug,
				"name":  event.Name,
				"state": event.State,
			}
		}
		if participation.TeamID != "" {
			if team, err := s.store.TeamByID(participation.TeamID); err == nil {
				view["team"] = map[string]any{"id": team.ID, "name": team.Name, "status": team.Status}
			}
		}
		views = append(views, view)
	}
	return views
}
