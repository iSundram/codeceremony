package domain

import "fmt"

type Role string

const (
	RoleVisitor     Role = "visitor"
	RoleParticipant Role = "participant"
	RoleJudge       Role = "judge"
	RoleOrganizer   Role = "organizer"
	RoleAdmin       Role = "admin"
)

type Permission string

const (
	PermissionViewOwnScores        Permission = "view_own_scores"
	PermissionViewPeerScores       Permission = "view_peer_scores"
	PermissionSubmitProject        Permission = "submit_project"
	PermissionReviewProject        Permission = "review_project"
	PermissionManageEvent          Permission = "manage_event"
	PermissionExportData           Permission = "export_data"
	PermissionViewAudit            Permission = "view_audit"
	PermissionManagePlatform       Permission = "manage_platform"
	PermissionVote                 Permission = "vote"
	PermissionComment              Permission = "comment"
	PermissionManageSubmission     Permission = "manage_submission"
	PermissionManageTeam           Permission = "manage_team"
	PermissionViewOwnSessions      Permission = "view_own_sessions"
	PermissionRevokeOwnSession     Permission = "revoke_own_session"
	PermissionViewAnySessions      Permission = "view_any_sessions"
	PermissionManageRoles          Permission = "manage_roles"
	PermissionViewOwnNotifications Permission = "view_own_notifications"
	PermissionManageNotifications  Permission = "manage_notifications"
	PermissionManageIntegrations   Permission = "manage_integrations"
	PermissionManageSelf           Permission = "manage_self"
	PermissionCancelDeletion       Permission = "cancel_deletion"
	PermissionViewAssignments      Permission = "view_assignments"
	PermissionManageAssignments    Permission = "manage_assignments"
	PermissionDeclareConflict      Permission = "declare_conflict"
)

var validRoles = map[Role]struct{}{
	RoleVisitor:     {},
	RoleParticipant: {},
	RoleJudge:       {},
	RoleOrganizer:   {},
	RoleAdmin:       {},
}

var rolePermissions = map[Role]map[Permission]struct{}{
	RoleVisitor: {},
	RoleParticipant: {
		PermissionSubmitProject:        {},
		PermissionManageTeam:           {},
		PermissionVote:                 {},
		PermissionComment:              {},
		PermissionManageSelf:           {},
		PermissionCancelDeletion:       {},
		PermissionViewOwnSessions:      {},
		PermissionRevokeOwnSession:     {},
		PermissionViewOwnNotifications: {},
	},
	RoleJudge: {
		PermissionViewOwnScores:        {},
		PermissionReviewProject:        {},
		PermissionViewAssignments:      {},
		PermissionDeclareConflict:      {},
		PermissionManageSelf:           {},
		PermissionCancelDeletion:       {},
		PermissionViewOwnSessions:      {},
		PermissionRevokeOwnSession:     {},
		PermissionViewOwnNotifications: {},
	},
	RoleOrganizer: {
		PermissionViewOwnScores:        {},
		PermissionViewPeerScores:       {},
		PermissionManageEvent:          {},
		PermissionManageIntegrations:   {},
		PermissionViewAssignments:      {},
		PermissionManageAssignments:    {},
		PermissionExportData:           {},
		PermissionViewAudit:            {},
		PermissionManageSubmission:     {},
		PermissionManageTeam:           {},
		PermissionManageSelf:           {},
		PermissionCancelDeletion:       {},
		PermissionViewOwnSessions:      {},
		PermissionRevokeOwnSession:     {},
		PermissionViewAnySessions:      {},
		PermissionManageRoles:          {},
		PermissionViewOwnNotifications: {},
		PermissionManageNotifications:  {},
		PermissionVote:                 {},
		PermissionComment:              {},
	},
	RoleAdmin: {
		PermissionViewOwnScores:        {},
		PermissionViewPeerScores:       {},
		PermissionManageEvent:          {},
		PermissionViewAssignments:      {},
		PermissionManageAssignments:    {},
		PermissionExportData:           {},
		PermissionViewAudit:            {},
		PermissionManagePlatform:       {},
		PermissionManageSubmission:     {},
		PermissionManageTeam:           {},
		PermissionManageSelf:           {},
		PermissionCancelDeletion:       {},
		PermissionViewOwnSessions:      {},
		PermissionRevokeOwnSession:     {},
		PermissionViewAnySessions:      {},
		PermissionManageRoles:          {},
		PermissionViewOwnNotifications: {},
		PermissionManageNotifications:  {},
		PermissionManageIntegrations:   {},
		PermissionVote:                 {},
		PermissionComment:              {},
	},
}

func (r Role) Valid() bool {
	_, ok := validRoles[r]
	return ok
}

func (r Role) Can(permission Permission) bool {
	permissions, ok := rolePermissions[r]
	if !ok {
		return false
	}
	_, ok = permissions[permission]
	return ok
}

func (r Role) IsStaff() bool {
	return r == RoleOrganizer || r == RoleAdmin
}

func (r Role) String() string {
	return string(r)
}

func ParseRole(value string) (Role, error) {
	role := Role(value)
	if !role.Valid() {
		return "", fmt.Errorf("unknown role %q", value)
	}
	return role, nil
}
