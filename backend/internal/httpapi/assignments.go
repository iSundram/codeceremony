package httpapi

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/domain"
)

type assignmentView struct {
	domain.Assignment
	JudgeDisplayName string `json:"judge_display_name"`
	JudgeEmail       string `json:"judge_email,omitempty"`
	ProjectTitle     string `json:"project_title"`
	TrackID          string `json:"track_id"`
	ReviewSubmitted  bool   `json:"review_submitted"`
}

func (s *Server) assignmentViews(eventID, judgeID string) []assignmentView {
	views := make([]assignmentView, 0)
	for _, assignment := range s.store.ListAssignments(eventID, judgeID) {
		view := assignmentView{Assignment: assignment}
		if judge, err := s.store.UserByID(assignment.JudgeID); err == nil {
			view.JudgeDisplayName = judge.DisplayName
			if assignment.JudgeID != judgeID {
				view.JudgeEmail = judge.Email
			}
		}
		if project, err := s.store.SubmissionByID(assignment.ProjectID); err == nil {
			view.ProjectTitle = project.Title
			view.TrackID = project.TrackID
		}
		if review, err := s.store.ReviewForJudgeProject(assignment.JudgeID, assignment.ProjectID); err == nil {
			view.ReviewSubmitted = review.Submitted
		}
		views = append(views, view)
	}
	return views
}

func (s *Server) organizerAssignments(w http.ResponseWriter, r *http.Request) {
	eventID := r.URL.Query().Get("event_id")
	if eventID == "" {
		eventID = "evt_01"
	}
	if _, err := s.store.EventByID(eventID); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	views := s.assignmentViews(eventID, r.URL.Query().Get("judge_id"))
	active := make([]assignmentView, 0, len(views))
	for _, view := range views {
		if view.RevokedAt == nil {
			active = append(active, view)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": active, "count": len(active)})
}

func (s *Server) judgeAssignments(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	eventID := r.URL.Query().Get("event_id")
	if eventID == "" {
		eventID = "evt_01"
	}
	if _, err := s.store.EventByID(eventID); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	views := s.assignmentViews(eventID, principal.UserID)
	pending := 0
	active := make([]assignmentView, 0, len(views))
	for _, view := range views {
		if view.RevokedAt == nil {
			active = append(active, view)
			if !view.ReviewSubmitted {
				pending++
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": active, "count": len(active), "pending": pending})
}

func (s *Server) createAssignments(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	var request struct {
		EventSlug         string   `json:"event_slug"`
		ProjectIDs        []string `json:"project_ids"`
		JudgeIDs          []string `json:"judge_ids"`
		Strategy          string   `json:"strategy"`
		ReviewsPerProject int      `json:"reviews_per_project"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	slug := strings.TrimSpace(request.EventSlug)
	if slug == "" {
		slug = "sample-hack-2026"
	}
	event, err := s.store.EventBySlug(slug)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	strategy := domain.AssignmentStrategy(strings.TrimSpace(request.Strategy))
	switch strategy {
	case "":
		strategy = domain.AssignmentBalanced
	case domain.AssignmentManual, domain.AssignmentBalanced, domain.AssignmentBatch:
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", "strategy must be manual, balanced, or batch")
		return
	}
	reviewsPerProject := request.ReviewsPerProject
	if reviewsPerProject <= 0 {
		reviewsPerProject = event.ReviewsPerProject
	}
	if reviewsPerProject <= 0 {
		reviewsPerProject = 1
	}
	if event.JudgingMode == domain.JudgingManual && request.ReviewsPerProject <= 0 {
		reviewsPerProject = 1
	}
	if reviewsPerProject > 10 {
		writeError(w, http.StatusBadRequest, "invalid_request", "reviews_per_project must be between 1 and 10")
		return
	}

	projects := s.selectedProjects(event.ID, request.ProjectIDs)
	if len(projects) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "no eligible projects were selected")
		return
	}
	judges := s.selectedJudges(event, request.JudgeIDs)
	if len(judges) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "no active judges from this hackathon roster were selected")
		return
	}
	if strategy == domain.AssignmentManual && len(request.JudgeIDs) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "manual assignment requires an explicit judge list")
		return
	}

	created := make([]domain.Assignment, 0)
	skipped := make([]map[string]string, 0)
	load := map[string]int{}
	for _, assignment := range s.store.ListAssignments(event.ID, "") {
		if assignment.RevokedAt == nil {
			load[assignment.JudgeID]++
		}
	}

	ordered := make([]domain.JudgeProfile, len(judges))
	copy(ordered, judges)

	for _, project := range projects {
		sort.SliceStable(ordered, func(i, j int) bool {
			if load[ordered[i].UserID] == load[ordered[j].UserID] {
				return ordered[i].UserID < ordered[j].UserID
			}
			return load[ordered[i].UserID] < load[ordered[j].UserID]
		})
		assigned := 0
		for _, judge := range ordered {
			if assigned >= reviewsPerProject {
				break
			}
			assignment, skippedItem, ok := s.assign(event, project, judge, principal.UserID, strategy, load, &assigned)
			if ok {
				created = append(created, assignment)
				continue
			}
			skipped = append(skipped, skippedItem)
		}
	}

	s.audit(r, principal.UserID, "assignments.bulk_created", "event", event.ID, event.ID, "assignment builder", map[string]any{
		"strategy":            string(strategy),
		"projects":            len(projects),
		"judges":              len(judges),
		"created":             len(created),
		"skipped":             len(skipped),
		"reviews_per_project": reviewsPerProject,
	})
	writeJSON(w, http.StatusCreated, map[string]any{"created": created, "skipped": skipped})
}

func (s *Server) assign(event domain.Event, project domain.Submission, judge domain.JudgeProfile, actorID string, strategy domain.AssignmentStrategy, load map[string]int, assigned *int) (domain.Assignment, map[string]string, bool) {
	assignment, err := s.store.CreateAssignment(domain.Assignment{
		EventID:    event.ID,
		JudgeID:    judge.UserID,
		ProjectID:  project.ID,
		AssignedBy: actorID,
		Strategy:   strategy,
	})
	if err != nil {
		return domain.Assignment{}, map[string]string{
			"judge_id":   judge.UserID,
			"project_id": project.ID,
			"reason":     assignmentFailureReason(err),
		}, false
	}
	load[judge.UserID]++
	*assigned++
	return assignment, map[string]string{}, true
}

func assignmentFailureReason(err error) string {
	switch {
	case errors.Is(err, domain.ErrForbidden):
		return "judge_track_scope_mismatch"
	case errors.Is(err, domain.ErrConflict):
		return "capacity_or_conflict"
	case errors.Is(err, domain.ErrAlreadyExists):
		return "already_assigned"
	case errors.Is(err, domain.ErrNotFound):
		return "not_found"
	default:
		return "invalid_request"
	}
}

func (s *Server) selectedProjects(eventID string, projectIDs []string) []domain.Submission {
	all := s.store.ListSubmissions(eventID, "", "")
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	if len(projectIDs) == 0 {
		return all
	}
	wanted := make(map[string]struct{}, len(projectIDs))
	for _, id := range projectIDs {
		wanted[strings.TrimSpace(id)] = struct{}{}
	}
	selected := make([]domain.Submission, 0, len(projectIDs))
	for _, project := range all {
		if _, ok := wanted[project.ID]; ok {
			selected = append(selected, project)
		}
	}
	return selected
}

// selectedJudges limits assignment to judges on the hackathon roster, plus the
// global reviewer pool, so an organizer cannot assign a judge who is not staff
// for this event.
func (s *Server) selectedJudges(event domain.Event, judgeIDs []string) []domain.JudgeProfile {
	wanted := map[string]struct{}{}
	for _, id := range judgeIDs {
		wanted[strings.TrimSpace(id)] = struct{}{}
	}
	eligible := map[string]struct{}{}
	for _, entry := range s.store.JudgeRoster(event.ID) {
		eligible[entry.JudgeID] = struct{}{}
	}
	for _, entry := range s.store.GlobalJudges() {
		eligible[entry.JudgeID] = struct{}{}
	}
	selected := make([]domain.JudgeProfile, 0, len(eligible))
	for userID := range eligible {
		if len(wanted) > 0 {
			if _, ok := wanted[userID]; !ok {
				continue
			}
		}
		profile, err := s.store.JudgeProfile(userID)
		if err != nil {
			profile = domain.JudgeProfile{UserID: userID, Active: true}
		}
		if !profile.Active {
			continue
		}
		selected = append(selected, profile)
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].UserID < selected[j].UserID })
	return selected
}

func (s *Server) revokeAssignment(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	assignmentID := r.PathValue("assignmentID")
	assignment, err := s.store.AssignmentByID(assignmentID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "assignment not found")
		return
	}
	reason := r.URL.Query().Get("reason")
	if strings.TrimSpace(reason) == "" {
		reason = "revoked_by_organizer"
	}
	if err := s.store.RevokeAssignment(assignmentID, reason); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "assignment not found")
		return
	}
	s.audit(r, principal.UserID, "assignment.revoked", "assignment", assignmentID, assignment.EventID, reason, nil)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": assignmentID, "revoked": true}})
}

func (s *Server) declareConflict(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	assignment, err := s.store.AssignmentByID(r.PathValue("assignmentID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "assignment not found")
		return
	}
	if assignment.JudgeID != principal.UserID {
		writeError(w, http.StatusForbidden, "forbidden", "judges can only declare conflicts for their own assignments")
		return
	}
	var request struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	reason := strings.TrimSpace(request.Reason)
	if reason == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "reason is required")
		return
	}
	declaration, err := s.store.DeclareConflict(domain.ConflictDeclaration{
		EventID:   assignment.EventID,
		JudgeID:   principal.UserID,
		ProjectID: assignment.ProjectID,
		Reason:    reason,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrConflict):
			writeError(w, http.StatusConflict, "already_exists", "a conflict was already declared for this assignment")
		case errors.Is(err, domain.ErrForbidden):
			writeError(w, http.StatusForbidden, "forbidden", "the assignment is no longer active")
		default:
			writeError(w, http.StatusBadRequest, "invalid_request", "the conflict could not be recorded")
		}
		return
	}
	s.audit(r, principal.UserID, "judge.conflict_declared", "project", assignment.ProjectID, assignment.EventID, reason, map[string]any{
		"assignment_id": assignment.ID,
		"conflict_id":   declaration.ID,
	})
	writeJSON(w, http.StatusCreated, map[string]any{"data": declaration, "assignment_revoked": true})
}
