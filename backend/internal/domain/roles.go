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
	PermissionViewOwnScores    Permission = "view_own_scores"
	PermissionViewPeerScores   Permission = "view_peer_scores"
	PermissionSubmitProject    Permission = "submit_project"
	PermissionReviewProject    Permission = "review_project"
	PermissionManageEvent      Permission = "manage_event"
	PermissionExportData       Permission = "export_data"
	PermissionViewAudit        Permission = "view_audit"
	PermissionManagePlatform   Permission = "manage_platform"
	PermissionVote             Permission = "vote"
	PermissionComment          Permission = "comment"
	PermissionManageSubmission Permission = "manage_submission"
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
		PermissionSubmitProject: {},
		PermissionVote:          {},
		PermissionComment:       {},
	},
	RoleJudge: {
		PermissionViewOwnScores: {},
		PermissionReviewProject: {},
	},
	RoleOrganizer: {
		PermissionViewOwnScores:    {},
		PermissionViewPeerScores:   {},
		PermissionManageEvent:      {},
		PermissionExportData:       {},
		PermissionViewAudit:        {},
		PermissionManageSubmission: {},
		PermissionVote:             {},
		PermissionComment:          {},
	},
	RoleAdmin: {
		PermissionViewOwnScores:    {},
		PermissionViewPeerScores:   {},
		PermissionManageEvent:      {},
		PermissionExportData:       {},
		PermissionViewAudit:        {},
		PermissionManagePlatform:   {},
		PermissionManageSubmission: {},
		PermissionVote:             {},
		PermissionComment:          {},
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
