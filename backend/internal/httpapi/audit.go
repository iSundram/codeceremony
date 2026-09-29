package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/authz"
	"github.com/iSundram/codeceremony/backend/internal/domain"
)

// Every authorization outcome is written to the action audit, allowed or refused.
//
// This is the part that was missing and that the spec's "audit trail an organizer
// can actually read" points at. A trail that records only successes is a list of
// things that were permitted, and it is completely blind to the case that
// actually matters: somebody finding out what they can reach. A run of refused
// attempts against the peer-score route is the clearest possible signal that
// something is wrong, and it is exactly the thing an allow-only log discards.

// auditSink buffers an audit entry for a request that is about to be answered.
func (s *Server) auditEntry(r *http.Request, principal auth.Principal, action authz.Action) domain.ActionAuditEntry {
	requestID := r.Header.Get("X-Request-ID")
	entry := domain.ActionAuditEntry{
		CreatedAt:  s.now().UTC(),
		ActorID:    principal.UserID,
		ActorRole:  string(principal.Role),
		ActorEmail: principal.Email,
		Action:     string(action),
		Method:     r.Method,
		Path:       r.URL.Path,
		RequestID:  requestID,
		RemoteAddr: clientAddress(r),
		UserAgent:  truncate(r.UserAgent(), 256),
	}
	if entry.ActorID == "" {
		entry.ActorRole = ""
		entry.ActorEmail = ""
	}
	return entry
}

// denied records a refused action and answers the request.
//
// The status is 401 when there was no usable principal and 403 when there was
// one that was refused, which is the distinction a client needs to tell "log in"
// from "you may not".
func (s *Server) denied(r *http.Request, principal auth.Principal, action authz.Action, decision authz.Decision, w http.ResponseWriter) {
	status := http.StatusForbidden
	if principal.UserID == "" {
		status = http.StatusUnauthorized
	}
	if decision.Source == authz.SourceAccountState {
		// The token parsed and the user exists, but the account is not in a
		// state that may act. That is an authentication-class failure, not an
		// authorization one: the client's session is not usable, and the right
		// response is to stop using it rather than to try a different route.
		status = http.StatusUnauthorized
	}
	entry := s.auditEntry(r, principal, action)
	entry.Allowed = false
	entry.Reason = decision.Reason
	entry.Source = string(decision.Source)
	entry.Status = status
	if principal.State == "" {
		entry.Assurance = strconv.FormatBool(principal.Assured)
	}
	s.store.RecordAction(entry)

	// The reason is attached to the response for a caller who is debugging their
	// own access, because "forbidden" with no explanation is the single most
	// common complaint an API produces. It names the rule, not the policy
	// internals.
	writeError(w, status, "forbidden", decision.Reason)
}

// allowed records a permitted action.
//
// Handlers that mutate call this on success, so the trail records the action as
// well as the refusal. Reads are deliberately not recorded: a gallery browse
// would otherwise bury the refusals in a flood of noise, and the value of the
// trail is in the ratio of refusals to permissions.
func (s *Server) allowed(r *http.Request, principal auth.Principal, action authz.Action, eventID, targetType, targetID string) {
	entry := s.auditEntry(r, principal, action)
	entry.Allowed = true
	entry.Reason = "permitted"
	entry.Status = http.StatusOK
	entry.EventID = eventID
	entry.TargetType = targetType
	entry.TargetID = targetID
	entry.Assurance = strconv.FormatBool(principal.Assured)
	s.store.RecordAction(entry)
}

// finalizeAudit fills in an entry's target and outcome.
//
// It is a function rather than a method on the domain type because a method
// cannot be declared on a type from another package, and because keeping the
// construction of an audit record in one file is worth more than the tidiness of
// a method. A caller building an entry by hand cannot accidentally produce a
// record with a target on one field and an outcome on another, which is the kind
// of inconsistency an auditor notices.
func finalizeAudit(entry domain.ActionAuditEntry, targetType, targetID, eventID, reason string, allowed bool) domain.ActionAuditEntry {
	entry.Allowed = allowed
	entry.Reason = reason
	entry.TargetType = targetType
	entry.TargetID = targetID
	entry.EventID = eventID
	entry.Status = http.StatusOK
	if !allowed {
		entry.Status = http.StatusForbidden
	}
	return entry
}

// unauthenticated records an attempt that carried no usable credential.
func (s *Server) unauthenticated(r *http.Request, reason string) {
	entry := s.auditEntry(r, auth.Principal{}, authz.Action("auth.identify"))
	entry.Allowed = false
	entry.Reason = reason
	entry.Source = string(authz.SourceNone)
	entry.Status = http.StatusUnauthorized
	s.store.RecordAction(entry)
}

// clientAddress is the peer address without the port.
func clientAddress(r *http.Request) string {
	host := r.RemoteAddr
	if index := strings.LastIndex(host, ":"); index > 0 {
		host = host[:index]
	}
	if host == "" {
		return "unknown"
	}
	return host
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

// parseSince parses an optional RFC3339 lower bound for an audit query.
func parseSince(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}
