package httpapi

import (
	"encoding/csv"
	"errors"
	"net/http"
	"strings"
	"strconv"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/authz"
	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

// The action audit is the record of who tried to do what, and whether they were
// allowed. It is queryable, exportable, and independently verifiable, because a
// log nobody can read is a log that answers no question.

// actionAuditQuery lists audit entries.
func (s *Server) actionAuditQuery(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	filter := store.ParseActionFilter(r.URL.Query())
	filter.Since = parseSince(r.URL.Query().Get("since"))
	filter.Until = parseSince(r.URL.Query().Get("until"))

	entries := s.store.ListActionAudit(filter)
	writeJSON(w, http.StatusOK, map[string]any{
		"data":    entries,
		"count":   len(entries),
		"total":   s.store.ActionAuditCount(),
		"head":    s.store.ActionAuditHead(),
		"dropped": s.store.VerifyActionAudit().Dropped,
		"meta": map[string]any{
			"actor_id": filter.ActorID, "event_id": filter.EventID,
			"action": filter.Action, "limit": filter.Limit,
			"before_seq": filter.BeforeSeq,
			"note":       "entries are newest first; page back with before_seq",
		},
	})
	// A read is not audited, deliberately: a browse of the audit log would
	// otherwise write to it, and the signal is in the ratio of refusals.
	_ = principal
}

// actionAuditExport streams the audit trail as CSV.
func (s *Server) actionAuditExport(w http.ResponseWriter, r *http.Request) {
	filter := store.ParseActionFilter(r.URL.Query())
	filter.Since = parseSince(r.URL.Query().Get("since"))
	filter.Until = parseSince(r.URL.Query().Get("until"))
	filter.Limit = 0 // the export is bounded by ParseActionFilter's own cap

	entries := s.store.ExportActionAudit(filter)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="codeceremony-audit.csv"`)
	writer := csv.NewWriter(w)
	defer writer.Flush()
	// prev_hash is exported alongside hash, and it has to be. A verifier needs
	// the link to the preceding entry, and on a trimmed chain the first row's
	// predecessor is not the row above it, so it cannot be inferred from the
	// file. Omitting it would make the export readable but not checkable, which
	// is the one thing it is exported for.
	_ = writer.Write([]string{
		"seq", "created_at", "actor_id", "actor_role", "action", "event_id",
		"target_type", "target_id", "allowed", "source", "reason",
		"method", "path", "status", "request_id", "remote_addr",
		"prev_hash", "hash",
	})
	for _, entry := range entries {
		_ = writer.Write([]string{
			strconv.FormatInt(entry.Seq, 10),
			entry.CreatedAt.UTC().Format(time.RFC3339),
			entry.ActorID, entry.ActorRole, entry.Action, entry.EventID,
			entry.TargetType, entry.TargetID,
			strconv.FormatBool(entry.Allowed), entry.Source, entry.Reason,
			entry.Method, entry.Path, strconv.Itoa(entry.Status), entry.RequestID,
			entry.RemoteAddr, entry.PrevHash, entry.Hash,
		})
	}
}

// actionAuditVerify reconciles the hash chain and reports the first break.
//
// This is the endpoint that makes the trail worth having. An organizer can hand
// over a copy of the export, and a third party can check the chain links: every
// row's prev_hash has to equal the row above, and the sequence numbers have to be
// contiguous, so nothing was removed or reordered.
//
// What a third party cannot do without the key is recompute the hashes
// themselves, because the chain is keyed with HMAC and the key is the operator's.
// Recomputing is what the server does here. This is the honest limit of the
// design: the export proves the log is internally consistent, and the key holder
// proves its contents are unedited. Making the latter public would mean signing
// with a key the server publishes, which detects server tampering but not
// compromise of the server itself.
func (s *Server) actionAuditVerify(w http.ResponseWriter, r *http.Request) {
	result := s.store.VerifyActionAudit()
	status := http.StatusOK
	if !result.Valid {
		// A broken chain is a genuine integrity failure, not a soft warning.
		status = http.StatusConflict
	}
	writeJSON(w, status, map[string]any{
		"data": result,
		// Naming the configuration variable rather than the value is the point:
		// a verifier needs to know which key to ask for, and the key itself must
		// not travel in a response body.
		"key_derivation": "HMAC-SHA256 over a key derived from AUDIT_SECRET (falling back to SESSION_SECRET when it is unset)",
		"note":           "a non-zero dropped count means the chain is a retained suffix, not the whole history",
	})
}

// ---- grants ---------------------------------------------------------------

// grantList returns explicit grants, for an operator reviewing who has what.
func (s *Server) grantList(w http.ResponseWriter, r *http.Request) {
	filter := store.GrantFilter{
		UserID:         r.URL.Query().Get("user_id"),
		EventID:        r.URL.Query().Get("event_id"),
		Action:         authz.Action(r.URL.Query().Get("action")),
		IncludeExpired: r.URL.Query().Get("include_expired") == "true",
	}
	grants := s.store.ListGrants(filter)
	writeJSON(w, http.StatusOK, map[string]any{
		"data": grants, "count": len(grants),
		"meta": map[string]any{
			"note": "a deny is evaluated before any allow, so revoking is immediate",
		},
	})
}

// grantCreate records an explicit allow or deny.
func (s *Server) grantCreate(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	var request struct {
		UserID    string `json:"user_id"`
		Action    string `json:"action"`
		EventID   string `json:"event_id"`
		ObjectID  string `json:"object_id"`
		Allow     bool   `json:"allow"`
		Reason    string `json:"reason"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	action, err := authz.ParseAction(request.Action)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	// The shape of the request is checked before the caller is judged on it. A
	// grant with no reason is a malformed request whatever the caller's role,
	// and answering it 422 rather than 403 keeps the two failures
	// distinguishable: a 403 would tell an unauthorized caller that the
	// permission they asked for parses.
	if strings.TrimSpace(request.Reason) == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error",
			"a grant needs a reason: a permission nobody can explain is a permission nobody can defend")
		return
	}
	var expiresAt *time.Time
	if request.ExpiresAt != "" {
		parsed, err := time.Parse(time.RFC3339, request.ExpiresAt)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "validation_error",
				"expires_at must be an RFC3339 timestamp")
			return
		}
		expiresAt = &parsed
	}

	// Resolving a grant needs the event it is scoped to, so a grant for an
	// event the caller does not staff is refused before it is written rather
	// than after.
	//
	// A grant with no event is not "a grant for no event": it is a
	// platform-wide grant, because that is the target an unscoped decision is
	// evaluated against. Gating it on ActionGrantManage, which every organizer
	// holds by role, let any organizer mint a self-applied platform-wide allow
	// for any action in the vocabulary — account.set_role among them — and then
	// use it. A grant is the one record in this system that can manufacture
	// authority, so minting a platform-wide one is a platform permission and
	// nothing less will do.
	if request.EventID != "" {
		event, err := s.store.EventByID(request.EventID)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "event not found")
			return
		}
		if _, ok := s.authorize(w, r, authz.ActionGrantManage, authz.Target{EventID: event.ID}); !ok {
			return
		}
	} else if _, ok := s.authorize(w, r, authz.ActionPlatformManage, authz.Target{}); !ok {
		return
	}

	// Holding a permission is not the same as being entitled to hand it out.
	// Without this, an organizer with no platform role could grant themselves
	// an event-scoped allow for an action they do not hold, and the resolver
	// would then honour it on the next request.
	if request.UserID == principal.UserID {
		if _, ok := s.authorize(w, r, action, authz.Target{EventID: request.EventID, ObjectID: request.ObjectID}); !ok {
			return
		}
	} else if request.Allow {
		if _, ok := s.authorize(w, r, action, authz.Target{EventID: request.EventID, ObjectID: request.ObjectID}); !ok {
			return
		}
	}

	grant := authz.Grant{
		UserID:    request.UserID,
		Action:    action,
		EventID:   request.EventID,
		ObjectID:  request.ObjectID,
		Allow:     request.Allow,
		Reason:    request.Reason,
		GrantedBy: principal.UserID,
		CreatedAt: s.now().UTC(),
	}
	grant.ExpiresAt = expiresAt
	created, err := s.store.CreateGrant(grant)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "the user or event does not exist")
			return
		}
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	s.audit(r, principal.UserID, "grant."+grantEffect(grant.Allow), "grant", created.ID,
		created.EventID, created.Reason, map[string]any{
			"user_id": created.UserID, "action": string(created.Action), "allow": created.Allow,
		})
	status := http.StatusCreated
	writeJSON(w, status, map[string]any{"data": created})
}

// grantRevoke removes a grant.
func (s *Server) grantRevoke(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	grant, err := s.store.GrantByID(r.PathValue("grantID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "grant not found")
		return
	}
	target := authz.Target{}
	if grant.EventID != "" {
		target.EventID = grant.EventID
	}
	if _, ok := s.authorize(w, r, authz.ActionGrantManage, target); !ok {
		return
	}
	if err := s.store.RevokeGrant(grant.ID); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "grant not found")
		return
	}
	s.audit(r, principal.UserID, "grant.revoked", "grant", grant.ID, grant.EventID,
		"revoked "+string(grant.Action)+" for "+grant.UserID, nil)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": grant.ID, "revoked": true}})
}

func grantEffect(allow bool) string {
	if allow {
		return "allowed"
	}
	return "denied"
}
