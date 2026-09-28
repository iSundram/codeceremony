package domain

import "time"

type AccountState string

const (
	AccountPending         AccountState = "pending"
	AccountActive          AccountState = "active"
	AccountSuspended       AccountState = "suspended"
	AccountDeactivated     AccountState = "deactivated"
	AccountLocked          AccountState = "locked"
	AccountDeletionPending AccountState = "deletion_pending"
	AccountDeleted         AccountState = "deleted"
	AccountMerged          AccountState = "merged"
)

func (s AccountState) Valid() bool {
	switch s {
	case AccountPending, AccountActive, AccountSuspended, AccountDeactivated, AccountLocked, AccountDeletionPending, AccountDeleted, AccountMerged:
		return true
	default:
		return false
	}
}

type TeamRole string

const (
	TeamRoleMember  TeamRole = "member"
	TeamRoleLeader  TeamRole = "leader"
	TeamRoleCaptain TeamRole = "captain"
)

func (r TeamRole) Valid() bool {
	return r == TeamRoleMember || r == TeamRoleLeader || r == TeamRoleCaptain
}

type TeamStatus string

const (
	TeamStatusActive          TeamStatus = "active"
	TeamStatusDeletionPending TeamStatus = "deletion_pending"
	TeamStatusArchived        TeamStatus = "archived"
)

type TeamMembership struct {
	ID        string     `json:"id"`
	EventID   string     `json:"event_id"`
	TeamID    string     `json:"team_id"`
	UserID    string     `json:"user_id"`
	Role      TeamRole   `json:"role"`
	Status    string     `json:"status"`
	JoinedAt  time.Time  `json:"joined_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	LeftAt    *time.Time `json:"left_at,omitempty"`
}

type Session struct {
	ID               string     `json:"id"`
	UserID           string     `json:"user_id"`
	TokenHash        string     `json:"-"`
	Method           string     `json:"method"`
	DeviceLabel      string     `json:"device_label"`
	IPAddress        string     `json:"ip_address,omitempty"`
	UserAgent        string     `json:"user_agent,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	LastSeenAt       time.Time  `json:"last_seen_at"`
	ExpiresAt        time.Time  `json:"expires_at"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	RevocationReason string     `json:"revocation_reason,omitempty"`
}

type NotificationKind string

const (
	NotificationAccountSecurity NotificationKind = "account_security"
	NotificationTeamActivity    NotificationKind = "team_activity"
	NotificationEventActivity   NotificationKind = "event_activity"
	NotificationJudging         NotificationKind = "judging"
	NotificationPublicActivity  NotificationKind = "public_activity"
	NotificationOperations      NotificationKind = "operations"
)

type Notification struct {
	ID        string            `json:"id"`
	UserID    string            `json:"user_id"`
	EventID   string            `json:"event_id,omitempty"`
	Kind      NotificationKind  `json:"kind"`
	Title     string            `json:"title"`
	Body      string            `json:"body"`
	ActionURL string            `json:"action_url,omitempty"`
	Payload   map[string]string `json:"payload,omitempty"`
	ReadAt    *time.Time        `json:"read_at,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

type AuditEvent struct {
	ID         string         `json:"id"`
	EventID    string         `json:"event_id,omitempty"`
	ActorID    string         `json:"actor_id,omitempty"`
	Action     string         `json:"action"`
	TargetType string         `json:"target_type"`
	TargetID   string         `json:"target_id,omitempty"`
	Reason     string         `json:"reason,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	RequestID  string         `json:"request_id,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}
