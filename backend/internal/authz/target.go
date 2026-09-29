package authz

import (
	"fmt"
	"strings"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

// Target describes the thing an action is attempted against.
//
// The single most important field is EventID, and the single most important rule
// is that it comes from the loaded object rather than the request. An earlier
// design took the event from a query parameter, which let a caller stamp a
// review against an event its project did not belong to. Making the event part of
// the target, and requiring handlers to build the target from what they loaded,
// turns that from a bug each handler has to remember into a shape the resolver
// can rely on.
type Target struct {
	// EventID scopes the action to one event. Empty means platform-wide.
	EventID string
	// ObjectID is the specific record, when the action targets one.
	ObjectID string
	// Resource names the kind of object, for grants scoped to a type.
	Resource string
	// OwnerID is the principal that owns the object, used by the ownership
	// rules: a team captain editing their team, a judge editing their review.
	OwnerID string
	// Assignment is the caller's assignment state for this object, for judging
	// actions that require one.
	//
	// It is three-valued on purpose. The route-level check runs before the
	// handler has loaded the project, so it genuinely does not know, and a
	// boolean would have it either refuse every judge or permit an unassigned
	// one. The resolver refuses on a known negative and defers on unknown,
	// which is the honest behaviour: the handler, holding the object, resolves
	// the question definitively.
	Assignment Assignment
	// Submitted reports whether the target object is already in a locked state.
	// A submitted review is immutable regardless of who is asking.
	Submitted bool
}

// Assignment is a caller's assignment state for an object.
type Assignment string

const (
	// AssignmentUnknown means nobody has checked yet.
	AssignmentUnknown Assignment = ""
	// AssignmentHeld means the caller is assigned to the object.
	AssignmentHeld Assignment = "assigned"
	// AssignmentNotHeld is a known negative, and is refused.
	AssignmentNotHeld Assignment = "not_assigned"
)

// Assigned is a convenience for a handler that has checked and found the
// assignment present.
func (t Target) Assigned() bool { return t.Assignment == AssignmentHeld }

// Grant is an explicit allow or deny, optionally scoped to an event or an
// object, optionally expiring.
//
// Grants exist because roles cannot express "this one judge may also see results"
// or "this organizer is suspended from event 7 until further notice". An explicit
// deny is the important half: it is how a compromised or mistaken account is
// fenced without editing code.
type Grant struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	Action    Action     `json:"action"`
	EventID   string     `json:"event_id,omitempty"`
	ObjectID  string     `json:"object_id,omitempty"`
	Allow     bool       `json:"allow"`
	Reason    string     `json:"reason"`
	GrantedBy string     `json:"granted_by"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// Applies reports whether the grant is in force for this target at this moment.
//
// Scope narrows as it widens: a grant with no event applies anywhere, one with an
// event applies only there, one with an object applies only to that object. A
// grant for a different event never applies, which is what makes a per-event
// grant safe to hand out.
func (g Grant) Applies(target Target, now time.Time) bool {
	if g.UserID == "" || g.Action == "" {
		return false
	}
	if g.ExpiresAt != nil && !now.Before(*g.ExpiresAt) {
		return false
	}
	if g.EventID != "" && g.EventID != target.EventID {
		return false
	}
	if g.ObjectID != "" && g.ObjectID != target.ObjectID {
		return false
	}
	return true
}

// Source names which layer of the resolution decided. It goes into the audit
// record so that a permission can be explained after the fact rather than
// guessed at.
type Source string

const (
	SourceNone           Source = "none"
	SourceRole           Source = "role"
	SourceEventRole      Source = "event_role"
	SourceGrant          Source = "grant"
	SourceDeniedByGrant  Source = "denied_by_grant"
	SourceOwnership      Source = "ownership"
	SourceAccountState   Source = "account_state"
	SourceLockedObject   Source = "locked_object"
	SourceUnassigned     Source = "not_assigned"
	SourceExpiredSession Source = "expired_session"
	SourceNoEventRole    Source = "no_event_role"
)

// Decision is the result of a resolution, carrying the reason.
//
// The reason is not decoration. "Why may this judge read these scores?" is the
// first question anyone asks during a disputed result, and a boolean cannot
// answer it.
type Decision struct {
	Allowed bool
	Action  Action
	Source  Source
	Reason  string
	// ExpiresAt is set when the decision rests on an expiring grant, so a
	// caller can be told when the access goes away.
	ExpiresAt *time.Time
}

// Allow builds an allowing decision.
func allow(action Action, source Source, reason string) Decision {
	return Decision{Allowed: true, Action: action, Source: source, Reason: reason}
}

// Deny builds a refusing decision.
func deny(action Action, source Source, reason string) Decision {
	return Decision{Allowed: false, Action: action, Source: source, Reason: reason}
}

// Validate checks a grant before it is stored, so a malformed rule is rejected
// at the point an operator creates it rather than silently never firing.
func (g Grant) Validate() error {
	if strings.TrimSpace(g.UserID) == "" {
		return fmt.Errorf("%w: a grant needs a user", domain.ErrValidation)
	}
	if !g.Action.Valid() {
		return fmt.Errorf("%w: %q is not a declared action", domain.ErrValidation, g.Action)
	}
	if strings.TrimSpace(g.Reason) == "" {
		// A grant without a reason is unexplainable during a dispute, which is
		// the one moment the reason matters most.
		return fmt.Errorf("%w: a grant needs a reason", domain.ErrValidation)
	}
	if strings.TrimSpace(g.GrantedBy) == "" {
		return fmt.Errorf("%w: a grant needs to record who made it", domain.ErrValidation)
	}
	if g.ExpiresAt != nil && !g.ExpiresAt.After(g.CreatedAt) {
		return fmt.Errorf("%w: a grant cannot expire before it was made", domain.ErrValidation)
	}
	return nil
}
