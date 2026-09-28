package httpapi

import (
	"net/http"
	"strings"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/domain"
)

func (s *Server) adminListUsers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": s.store.ListUsers()})
}

func (s *Server) adminUpdateUserState(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	userID := r.PathValue("userID")
	if userID == principal.UserID {
		writeError(w, http.StatusConflict, "self_change_forbidden", "administrators cannot change their own account state")
		return
	}
	var request struct {
		State  domain.AccountState `json:"state"`
		Reason string              `json:"reason"`
	}
	if err := decodeJSON(r, &request); err != nil || !request.State.Valid() || strings.TrimSpace(request.Reason) == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "a valid state and reason are required")
		return
	}
	if err := s.store.SetUserState(userID, request.State); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	s.audit(r, principal.UserID, "account.state_changed", "user", userID, "", request.Reason, map[string]any{"state": request.State})
	_ = s.store.CreateNotification(domain.Notification{ID: domain.NewID("ntf"), UserID: userID, Kind: domain.NotificationAccountSecurity, Title: "Account state changed", Body: "An administrator changed your account state.", CreatedAt: s.now().UTC()})
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

func (s *Server) adminUpdateUserRole(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	userID := r.PathValue("userID")
	if userID == principal.UserID {
		writeError(w, http.StatusConflict, "self_change_forbidden", "administrators cannot change their own global role")
		return
	}
	var request struct {
		Role   domain.Role `json:"role"`
		Reason string      `json:"reason"`
	}
	if err := decodeJSON(r, &request); err != nil || !request.Role.Valid() || strings.TrimSpace(request.Reason) == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "a valid role and reason are required")
		return
	}
	if err := s.store.SetUserRole(userID, request.Role); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	s.audit(r, principal.UserID, "account.role_changed", "user", userID, "", request.Reason, map[string]any{"role": request.Role})
	_ = s.store.CreateNotification(domain.Notification{ID: domain.NewID("ntf"), UserID: userID, Kind: domain.NotificationAccountSecurity, Title: "Account role changed", Body: "An administrator changed your account permissions.", CreatedAt: s.now().UTC()})
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

func (s *Server) adminRevokeUserSessions(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	userID := r.PathValue("userID")
	if _, err := s.store.UserByID(userID); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	var request struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &request); err != nil || strings.TrimSpace(request.Reason) == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "a revocation reason is required")
		return
	}
	count := s.store.RevokeUserSessions(userID, "", request.Reason)
	s.audit(r, principal.UserID, "account.sessions_revoked", "user", userID, "", request.Reason, map[string]any{"count": count})
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"status": "revoked", "count": count}})
}

func (s *Server) adminAudit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": s.store.ListAudit(r.URL.Query().Get("event_id"))})
}
