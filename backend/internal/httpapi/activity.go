package httpapi

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/authz"
	"github.com/iSundram/codeceremony/backend/internal/domain"
)

func (s *Server) activityFeed(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	filter := domain.ActivityFilter{
		EventID:    strings.TrimSpace(r.URL.Query().Get("event_id")),
		Category:   strings.TrimSpace(r.URL.Query().Get("category")),
		Action:     strings.TrimSpace(r.URL.Query().Get("action")),
		ActorID:    strings.TrimSpace(r.URL.Query().Get("actor_id")),
		TargetType: strings.TrimSpace(r.URL.Query().Get("target_type")),
		Visibility: strings.TrimSpace(r.URL.Query().Get("visibility")),
	}
	filter.Limit = parseQueryInt(r, "limit", 25)
	filter.Offset = parseQueryInt(r, "offset", 0)
	filter.RollupCount = r.URL.Query().Get("counts_only") == "true"

	page, err := s.store.ListActivity(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "the activity log could not be read")
		return
	}
	isStaff, isJudge, isParticipant := s.viewerContext(principal, filter.EventID)
	visible := make([]domain.ActivityEntry, 0, len(page.Entries))
	for _, entry := range page.Entries {
		if entry.VisibleTo(principal.Role, isStaff, isJudge, isParticipant) {
			visible = append(visible, entry)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":          visible,
		"count":         len(visible),
		"total":         page.Total,
		"limit":         page.Limit,
		"offset":        page.Offset,
		"by_category":   page.ByCategory,
		"by_visibility": page.ByVisibility,
	})
}

func (s *Server) viewerContext(principal auth.Principal, eventID string) (bool, bool, bool) {
	if principal.Role == domain.RoleOrganizer || principal.Role == domain.RoleAdmin {
		return true, true, true
	}
	if eventID == "" {
		return false, principal.Role == domain.RoleJudge, principal.Role == domain.RoleParticipant
	}
	if principal.Role == domain.RoleJudge && s.store.JudgeOnRoster(eventID, principal.UserID) {
		return false, true, true
	}
	for _, membership := range s.store.TeamMembershipsForUser(principal.UserID) {
		if membership.EventID == eventID {
			return false, principal.Role == domain.RoleJudge, true
		}
	}
	return false, principal.Role == domain.RoleJudge, false
}

// listEventStaff is public, because knowing who runs an event is public. The
// addresses behind those names are not, so they are added only for staff.
func (s *Server) listEventStaff(w http.ResponseWriter, r *http.Request) {
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	staff := s.isStaff(r)
	members := s.store.EventStaffMembers(event.ID)
	views := make([]map[string]any, 0, len(members))
	for _, member := range members {
		// can_manage_event reads the action table rather than the old
		// permission map, so the value shown is the value that is enforced.
		view := map[string]any{
			"staff":            member,
			"can_manage_event": authz.EventRoleAllows(member.Role, authz.ActionEventUpdate),
		}
		if user, err := s.store.UserByID(member.UserID); err == nil {
			view["display_name"] = user.DisplayName
			if staff {
				view["email"] = user.Email
			}
		}
		views = append(views, view)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": views, "count": len(views), "roles": domain.EventRoleNames()})
}

func (s *Server) addEventStaff(w http.ResponseWriter, r *http.Request) {
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
		UserID string `json:"user_id"`
		Role   string `json:"role"`
		Title  string `json:"title"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	role := domain.EventRole(strings.TrimSpace(request.Role))
	if role == "" {
		role = domain.EventRoleCoOrganizer
	}
	member := domain.EventStaff{EventID: event.ID, UserID: strings.TrimSpace(request.UserID), Role: role, Title: strings.TrimSpace(request.Title), AddedBy: principal.UserID}
	created, err := s.store.AddEventStaff(member)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrAlreadyExists):
			writeError(w, http.StatusConflict, "conflict", "this user already has an equal or higher event role")
		case errors.Is(err, domain.ErrValidation):
			writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		default:
			writeError(w, http.StatusNotFound, "not_found", "hackathon or user not found")
		}
		return
	}
	// Who can administer an event is the most consequential thing an organizer
	// changes, so it is in the audit trail and not only on the activity feed.
	// The two are different records: activity is public-facing and filtered,
	// audit is the accountability record.
	s.allowed(r, principal, authz.ActionEventStaffManage, event.ID, "event_staff", created.UserID)
	s.audit(r, principal.UserID, "event.staff_added", "event_staff", created.UserID, event.ID,
		principal.UserID+" added "+created.UserID+" as "+string(created.Role), map[string]any{"role": created.Role})
	s.recordActivity(r, principal.UserID, domain.ActivityEvent, "event.staff_added", "event_staff", created.UserID, event.ID, principal.UserID+" added "+created.UserID+" as "+string(created.Role), domain.ActivityOrganizers, map[string]any{"role": created.Role})
	writeJSON(w, http.StatusCreated, map[string]any{"data": created})
}

func (s *Server) removeEventStaff(w http.ResponseWriter, r *http.Request) {
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
	userID := r.PathValue("userID")
	if err := s.store.RemoveEventStaff(event.ID, userID); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event staff member not found")
		return
	}
	s.allowed(r, principal, authz.ActionEventStaffManage, event.ID, "event_staff", userID)
	s.audit(r, principal.UserID, "event.staff_removed", "event_staff", userID, event.ID,
		principal.UserID+" removed "+userID+" from the organizer team", nil)
	s.recordActivity(r, principal.UserID, domain.ActivityEvent, "event.staff_removed", "event_staff", userID, event.ID, principal.UserID+" removed "+userID+" from the organizer team", domain.ActivityOrganizers, nil)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"user_id": userID, "removed": true}})
}

// permissionMatrix publishes what the resolver actually enforces.
//
// It is generated from the authz tables rather than restated, because the old
// version enumerated permissions separately and had already drifted from the
// code: the endpoint advertised a matrix that was not the one being applied,
// which is worse than publishing none.
func (s *Server) permissionMatrix(w http.ResponseWriter, r *http.Request) {
	platform := authz.BuildMatrix()
	event := authz.BuildEventMatrix()

	roles := make([]string, 0, len(platform))
	for role := range platform {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	eventRoles := make([]string, 0, len(event))
	for role := range event {
		eventRoles = append(eventRoles, role)
	}
	sort.Strings(eventRoles)

	actions := make([]string, 0, len(authz.All()))
	for _, action := range authz.All() {
		actions = append(actions, string(action))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"platform_roles":   platform,
			"event_roles":      event,
			"actions":          actions,
			"resources":        authz.Resources(),
			"resolution_order": authz.ResolutionOrder(),
		},
		"platform_roles": roles,
		"event_roles":    eventRoles,
	})
}

func (s *Server) eventDirectory(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.URL.Query().Get("slug"))
	if slug != "" {
		event, err := s.store.EventBySlug(slug)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": s.directoryEntry(event)})
		return
	}
	state := domain.HackathonState(strings.TrimSpace(r.URL.Query().Get("state")))
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	events := s.store.ListEvents()
	entries := make([]domain.EventDirectoryEntry, 0, len(events))
	for _, event := range events {
		if state != "" && event.State != state {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(event.Name+" "+event.Slug+" "+event.Description), query) {
			continue
		}
		entries = append(entries, s.directoryEntry(event))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":  entries,
		"count": len(entries),
		"meta": map[string]any{
			"state":    state,
			"q":        query,
			"now":      s.now().UTC(),
			"catalogs": s.directoryCatalogs(),
		},
	})
}

func (s *Server) directoryEntry(event domain.Event) domain.EventDirectoryEntry {
	entry := domain.EventDirectoryEntry{
		Event:         event,
		Tracks:        len(s.store.ListTracks(event.ID)),
		Milestones:    len(s.store.Milestones(event.ID)),
		Questions:     len(s.store.Questions(event.ID, "")),
		Judges:        len(s.store.JudgeRoster(event.ID)),
		Participation: len(s.store.Participations("", event.ID)),
		State:         event.State,
	}
	for _, team := range s.store.ListTeams(event.ID) {
		if team.Status == domain.TeamStatusActive {
			entry.Teams++
		}
	}
	for _, project := range s.store.ListSubmissions(event.ID, "", "") {
		if project.Status == domain.SubmissionSubmitted || project.Status == domain.SubmissionLocked {
			entry.Submissions++
		}
	}
	for _, team := range s.store.ListTeams(event.ID) {
		for _, invite := range s.store.InvitesForTeam(team.ID) {
			if invite.Status == domain.InvitePending {
				entry.OpenInvites++
			}
		}
	}
	now := s.now().UTC()
	if event.SubmissionsClose.After(now) {
		entry.EndsInDays = int(event.SubmissionsClose.Sub(now).Hours() / 24)
	}
	if event.SubmissionsClose.Before(now) && event.JudgingClose.After(now) {
		entry.StartsInDays = -int(event.JudgingClose.Sub(now).Hours() / 24)
	}
	return entry
}

func (s *Server) directoryCatalogs() map[string]any {
	states := map[domain.HackathonState]int{}
	activity := map[string]int{}
	for _, event := range s.store.ListEvents() {
		states[event.State]++
		counts := s.store.ActivityCategories(event.ID)
		for category, count := range counts {
			activity[category] += count
		}
	}
	return map[string]any{
		"states":               states,
		"activity_by_category": activity,
		"action_count":         len(authz.All()),
		"event_roles":          domain.EventRoleNames(),
	}
}

func (s *Server) recordActivity(r *http.Request, actorID string, category domain.ActivityCategory, action, targetType, targetID, eventID, summary string, visibility domain.ActivityVisibility, metadata map[string]any) {
	entry, err := s.store.RecordActivity(domain.ActivityEntry{
		EventID:    eventID,
		ActorID:    actorID,
		Category:   category,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Summary:    summary,
		Visibility: visibility,
		Metadata:   metadata,
		RequestID:  r.Header.Get("X-Request-ID"),
	})
	if err != nil {
		s.logger.Warn("activity entry rejected", "action", action, "error", err)
		return
	}
	_ = entry
}

// parseQueryInt reads a count from the query string, bounded to [1, max].
//
// The bounds are the whole point. The parsed value used to be returned as-is and
// then used as a make() capacity, so ?limit=99999999999 made the process attempt
// a hundred-gigabyte allocation and die with an unrecoverable out-of-memory —
// not a 500, because there is no recovering from that, the runtime does not come
// back. One request, from any caller who can reach the route, took the portal
// down. A negative value panicked in make for the same reason.
//
// A caller asking for more than the cap gets the cap. Refusing the request
// would be more honest, but the cap is a safety property and silently serving
// fewer rows than asked for is the conventional reading of a limit.
func parseQueryInt(r *http.Request, key string, fallback int) int {
	const max = 500
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	if parsed < 1 {
		return 1
	}
	if parsed > max {
		return max
	}
	return parsed
}
