package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/authz"
	"github.com/iSundram/codeceremony/backend/internal/domain"
)

// Comparisons are the first-class form of the pairwise question.
//
// The organizer-facing pairwise view can already be built from rubric scores, but
// a derived comparison rests on an assumption: that a judge's 4 out of 5 on one
// project and 3 out of 5 on another means they preferred the first. A recorded
// verdict is the judge answering directly, and it is attributed, timestamped and
// reversible, which an inferred one is not.

// comparisonEvent resolves the event a comparison request is about, from the
// route's {slug} or the event_id query parameter.
func (s *Server) comparisonEvent(w http.ResponseWriter, r *http.Request) (domain.Event, bool) {
	if slug := r.PathValue("slug"); slug != "" {
		event, err := s.store.EventBySlug(slug)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "event not found")
			return domain.Event{}, false
		}
		return event, true
	}
	eventID := strings.TrimSpace(r.URL.Query().Get("event_id"))
	if eventID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "event_id or event_slug is required")
		return domain.Event{}, false
	}
	event, err := s.store.EventByID(eventID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return domain.Event{}, false
	}
	return event, true
}

// listComparisons returns the event's recorded verdicts.
//
// A judge sees their own and organizers see the event's, which falls out of the
// route being declared twice with different actions rather than from a branch in
// the handler.
func (s *Server) listComparisons(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	event, ok := s.comparisonEvent(w, r)
	if !ok {
		return
	}

	comparisons := s.store.ComparisonsForEvent(event.ID)
	if !s.maySeeAllComparisons(principal, event.ID, r) {
		own := make([]domain.Comparison, 0, len(comparisons))
		for _, comparison := range comparisons {
			if comparison.JudgeID == principal.UserID {
				own = append(own, comparison)
			}
		}
		comparisons = own
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": comparisons, "count": len(comparisons), "event_id": event.ID,
	})
}

// maySeeAllComparisons reports whether the caller may see the whole panel's
// verdicts rather than only their own. Reading results is the right test: it is
// the permission that already distinguishes an organizer from a judge, and
// inventing a second notion of "may see the panel" would be one more thing to
// keep in step.
func (s *Server) maySeeAllComparisons(principal auth.Principal, eventID string, r *http.Request) bool {
	decision := s.resolve(principal, authz.ActionResultsRead, authz.Target{EventID: eventID, Resource: "event"}, r)
	return decision.Allowed
}

// recordComparison stores one head-to-head verdict.
func (s *Server) recordComparison(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	event, ok := s.comparisonEvent(w, r)
	if !ok {
		return
	}

	var request struct {
		Left    string `json:"left"`
		Right   string `json:"right"`
		Verdict string `json:"verdict"`
		Comment string `json:"comment"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if strings.TrimSpace(request.Left) == "" || strings.TrimSpace(request.Right) == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "left and right are both required")
		return
	}
	if request.Left == request.Right {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "a project cannot be compared with itself")
		return
	}

	// A judge may only compare projects they were assigned. Without this, any
	// judge could rank the whole field, which would be a much larger judgement
	// than the one they were asked to make and would be indistinguishable from
	// the organizer's own.
	for _, projectID := range []string{request.Left, request.Right} {
		if !s.store.IsAssigned(event.ID, principal.UserID, projectID) {
			writeError(w, http.StatusForbidden, "forbidden",
				"both projects must be assigned to you before you can compare them")
			return
		}
	}

	created, err := s.store.RecordComparison(domain.Comparison{
		EventID: event.ID, JudgeID: principal.UserID,
		Left: request.Left, Right: request.Right,
		Verdict: domain.ComparisonVerdict(strings.TrimSpace(request.Verdict)),
		Comment: strings.TrimSpace(request.Comment),
	})
	if err != nil {
		s.writeComparisonError(w, err)
		return
	}
	s.audit(r, principal.UserID, "comparison.recorded", "comparison", created.ID, event.ID,
		"judge compared "+created.Left+" with "+created.Right, map[string]any{
			"left": created.Left, "right": created.Right, "verdict": created.Verdict,
		})
	// The ETag identifies the recorded pair rather than a version counter, since
	// re-answering the same pair updates the row in place.
	setETag(w, contentVersion(created.ID, created.Left, created.Right))
	writeJSON(w, http.StatusCreated, map[string]any{"data": created})
}

func (s *Server) deleteComparison(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	id := r.PathValue("comparisonID")
	comparison, err := s.store.ComparisonByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "comparison not found")
		return
	}
	// A judge may withdraw their own answer. An organizer may withdraw anyone's,
	// which is the escape hatch for a mistaken verdict that the judge has not
	// noticed.
	decision := s.resolve(principal, authz.ActionResultsRead, authz.Target{
		EventID: comparison.EventID, Resource: "comparison", ObjectID: comparison.ID,
	}, r.WithContext(r.Context()))
	if !decision.Allowed && comparison.JudgeID != principal.UserID {
		writeError(w, http.StatusForbidden, "forbidden", "this comparison is not yours to withdraw")
		return
	}
	if err := s.store.DeleteComparison(id); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "comparison not found")
		return
	}
	s.audit(r, principal.UserID, "comparison.withdrawn", "comparison", comparison.ID, comparison.EventID,
		"comparison withdrawn", map[string]any{
			"left": comparison.Left, "right": comparison.Right, "verdict": comparison.Verdict,
			"owner": comparison.JudgeID,
		})
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": id, "withdrawn": true}})
}

func (s *Server) writeComparisonError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "the judge, event or project does not exist")
	case errors.Is(err, domain.ErrValidation):
		// The store's prefix is dropped so the client is told what is wrong rather
		// than how the store words it.
		writeError(w, http.StatusUnprocessableEntity, "validation_error",
			strings.TrimSpace(strings.TrimPrefix(err.Error(), domain.ErrValidation.Error())))
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "the comparison could not be recorded")
	}
}
