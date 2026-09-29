package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type EventRole string

const (
	EventRoleOwner        EventRole = "owner"
	EventRoleCoOrganizer  EventRole = "co_organizer"
	EventRoleJudgeLiaison EventRole = "judge_liaison"
	EventRoleModerator    EventRole = "moderator"
	EventRoleViewer       EventRole = "viewer"
)

var eventRolePermissions = map[EventRole]map[Permission]struct{}{
	EventRoleOwner: {
		PermissionManageEvent:         {},
		PermissionManageSubmission:    {},
		PermissionViewPeerScores:      {},
		PermissionExportData:          {},
		PermissionViewAudit:           {},
		PermissionManageAssignments:   {},
		PermissionViewAssignments:     {},
		PermissionManageRoles:         {},
		PermissionManageNotifications: {},
	},
	EventRoleCoOrganizer: {
		PermissionManageEvent:         {},
		PermissionManageSubmission:    {},
		PermissionViewAssignments:     {},
		PermissionManageAssignments:   {},
		PermissionViewAudit:           {},
		PermissionManageNotifications: {},
	},
	EventRoleJudgeLiaison: {
		PermissionViewAssignments:   {},
		PermissionManageAssignments: {},
		PermissionManageEvent:       {},
		PermissionViewAudit:         {},
	},
	EventRoleModerator: {
		PermissionManageSubmission: {},
		PermissionComment:          {},
		PermissionViewAssignments:  {},
	},
	EventRoleViewer: {
		PermissionViewAssignments: {},
	},
}

func (r EventRole) Valid() bool {
	_, ok := eventRolePermissions[r]
	return ok
}

func (r EventRole) Can(permission Permission) bool {
	permissions, ok := eventRolePermissions[r]
	if !ok {
		return false
	}
	_, allowed := permissions[permission]
	return allowed
}

func (r EventRole) Rank() int {
	switch r {
	case EventRoleOwner:
		return 3
	case EventRoleCoOrganizer:
		return 2
	case EventRoleModerator:
		return 1
	case EventRoleJudgeLiaison:
		return 1
	case EventRoleViewer:
		return 0
	default:
		return -1
	}
}

func EventRoleNames() []string {
	return []string{string(EventRoleOwner), string(EventRoleCoOrganizer), string(EventRoleJudgeLiaison), string(EventRoleViewer)}
}

type EventStaff struct {
	EventID   string    `json:"event_id"`
	UserID    string    `json:"user_id"`
	Role      EventRole `json:"role"`
	Title     string    `json:"title,omitempty"`
	AddedBy   string    `json:"added_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (s EventStaff) Validate() error {
	if strings.TrimSpace(s.UserID) == "" || strings.TrimSpace(s.EventID) == "" {
		return fmt.Errorf("%w: event staff needs an event and a user", ErrValidation)
	}
	if !s.Role.Valid() {
		return fmt.Errorf("%w: unknown event role %q", ErrValidation, s.Role)
	}
	return nil
}

func (s EventStaff) CanManage(actor EventStaff) bool {
	return actor.EventID == s.EventID && actor.Role.Rank() > s.Role.Rank()
}

type EventDirectoryEntry struct {
	Event         Event          `json:"event"`
	Teams         int            `json:"teams"`
	Submissions   int            `json:"submissions"`
	Judges        int            `json:"judges"`
	Tracks        int            `json:"tracks"`
	Milestones    int            `json:"milestones"`
	Questions     int            `json:"questions"`
	OpenInvites   int            `json:"open_invites"`
	Participation int            `json:"participation"`
	State         HackathonState `json:"state"`
	StartsInDays  int            `json:"starts_in_days,omitempty"`
	EndsInDays    int            `json:"ends_in_days,omitempty"`
}

type PermissionDescriptor struct {
	Key         Permission  `json:"key"`
	Description string      `json:"description"`
	Roles       []Role      `json:"roles"`
	EventRoles  []EventRole `json:"event_roles"`
	Category    string      `json:"category"`
}

func PermissionMatrix(platform map[Role][]Permission, event map[EventRole][]Permission) []PermissionDescriptor {
	categories := map[Permission]string{
		PermissionManageEvent:          "event",
		PermissionManageSubmission:     "submission",
		PermissionViewAssignments:      "judging",
		PermissionManageAssignments:    "judging",
		PermissionViewPeerScores:       "judging",
		PermissionViewOwnScores:        "judging",
		PermissionReviewProject:        "judging",
		PermissionExportData:           "operations",
		PermissionViewAudit:            "operations",
		PermissionManageRoles:          "accounts",
		PermissionManagePlatform:       "accounts",
		PermissionViewAnySessions:      "accounts",
		PermissionManageSelf:           "accounts",
		PermissionViewOwnSessions:      "accounts",
		PermissionRevokeOwnSession:     "accounts",
		PermissionSubmitProject:        "participation",
		PermissionManageTeam:           "participation",
		PermissionVote:                 "participation",
		PermissionComment:              "participation",
		PermissionManageNotifications:  "notifications",
		PermissionViewOwnNotifications: "notifications",
		PermissionManageIntegrations:   "integrations",
		PermissionCancelDeletion:       "accounts",
		PermissionDeclareConflict:      "judging",
	}
	descriptions := map[Permission]string{
		PermissionManageEvent:          "create and configure hackathons, questions, and rosters",
		PermissionManageSubmission:     "review, lock, and disqualify submissions",
		PermissionViewAssignments:      "read judge assignments for a hackathon",
		PermissionManageAssignments:    "build and revoke judge assignments",
		PermissionViewPeerScores:       "read other judges reviews and published results",
		PermissionViewOwnScores:        "read only the current judge own reviews",
		PermissionReviewProject:        "score assigned projects",
		PermissionExportData:           "export hackathon data",
		PermissionViewAudit:            "read the audit and activity log",
		PermissionManageRoles:          "change platform and event roles",
		PermissionManagePlatform:       "administer accounts, sessions, and audit",
		PermissionViewAnySessions:      "read sessions for any account",
		PermissionManageSelf:           "update own profile, password, and export data",
		PermissionViewOwnSessions:      "list own sessions",
		PermissionRevokeOwnSession:     "revoke own sessions",
		PermissionSubmitProject:        "create and revise submissions",
		PermissionManageTeam:           "manage team membership and invites",
		PermissionVote:                 "vote in community polls",
		PermissionComment:              "comment on projects",
		PermissionManageNotifications:  "manage notification settings for others",
		PermissionViewOwnNotifications: "read own notifications",
		PermissionManageIntegrations:   "manage webhooks and outbound integrations",
		PermissionCancelDeletion:       "cancel an account deletion request",
		PermissionDeclareConflict:      "declare a judging conflict of interest",
	}
	descriptors := make([]PermissionDescriptor, 0, len(categories))
	for permission, category := range categories {
		descriptor := PermissionDescriptor{Key: permission, Category: category, Description: descriptions[permission]}
		for _, role := range []Role{RoleParticipant, RoleJudge, RoleOrganizer, RoleAdmin} {
			if role.Can(permission) {
				descriptor.Roles = append(descriptor.Roles, role)
			}
		}
		for _, eventRole := range []EventRole{EventRoleOwner, EventRoleCoOrganizer, EventRoleJudgeLiaison, EventRoleViewer} {
			if eventRole.Can(permission) {
				descriptor.EventRoles = append(descriptor.EventRoles, eventRole)
			}
		}
		sort.Slice(descriptor.Roles, func(i, j int) bool { return descriptor.Roles[i] < descriptor.Roles[j] })
		descriptors = append(descriptors, descriptor)
	}
	sort.Slice(descriptors, func(i, j int) bool {
		if descriptors[i].Category == descriptors[j].Category {
			return descriptors[i].Key < descriptors[j].Key
		}
		return descriptors[i].Category < descriptors[j].Category
	})
	return descriptors
}

// ParseEventRole validates a caller-supplied event role string.
func ParseEventRole(value string) (EventRole, error) {
	role := EventRole(value)
	if !role.Valid() {
		return "", fmt.Errorf("unknown event role %q", value)
	}
	return role, nil
}
