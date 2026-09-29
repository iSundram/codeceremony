package authz

import (
	"sort"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

// Principal is the authenticated caller, as the resolver sees them.
type Principal struct {
	UserID string
	Email  string
	Role   domain.Role
	// AccountState is checked before anything else. A suspended or
	// deletion-pending account is refused every action except cancelling its own
	// deletion, regardless of what its role or grants would otherwise permit.
	AccountState domain.AccountState
	// EventRole is the caller's per-event role, or empty when they hold none.
	EventRole domain.EventRole
	// EventRoleFor is the event that EventRole was resolved for.
	//
	// It is separate from EventRole on purpose. Carrying only the role would
	// mean the resolver has to trust that whoever built the Principal paired it
	// with the right event, and a mistake there is a cross-tenant read. Naming
	// the event makes the pair checkable, so a role for event 1 cannot be
	// presented as a role for event 2.
	EventRoleFor string
}

// GrantSource supplies the explicit grants and denies for a principal. The store
// implements it; the resolver depends on the interface so the whole model is
// testable without any storage.
type GrantSource interface {
	GrantsFor(userID string) []Grant
}

// Resolver answers authorization questions.
//
// It is stateless apart from its clock and its grant source, so it is safe to
// share across goroutines and cheap to call on every request.
type Resolver struct {
	grants GrantSource
	now    func() time.Time
}

// NewResolver builds a resolver. A nil grant source means no explicit grants
// exist, which is the correct behaviour for a portal that has never had one.
func NewResolver(grants GrantSource) *Resolver {
	return &Resolver{grants: grants, now: time.Now}
}

// SetClock replaces the clock. Tests use it to drive grant expiry.
func (r *Resolver) SetClock(now func() time.Time) { r.now = now }

// Resolve decides whether a principal may perform an action on a target.
//
// The order below is the whole policy, and each step is written down because the
// order is the policy:
//
//  1. Account state. A suspended account does nothing. This is first so that no
//     combination of role and grant can resurrect a suspended user.
//  2. Target constraints. A submitted review is immutable for everyone,
//     including the judge who wrote it and the organizer; and a judging write
//     requires an assignment. Both are properties of the record rather than
//     permissions, so they are checked before any role or grant is consulted.
//  3. Explicit denies. A deny is evaluated before any grant, and a deny wins
//     unconditionally. This is what makes a revoke actually revoke.
//  4. Explicit allows. Narrow, expiring, and auditable.
//  5. Per-event role, for a target that names an event. A union with the global
//     role, scoped strictly to the event the caller holds the role on.
//  6. Global role default.
//  7. Ownership, for the actions that are about your own object.
//
// A step that denies returns immediately. A step that cannot decide continues.
func (r *Resolver) Resolve(p Principal, action Action, target Target) Decision {
	if !action.Valid() {
		return deny(action, SourceNone, "the action is not part of the declared vocabulary")
	}
	if p.UserID == "" {
		return deny(action, SourceNone, "no principal")
	}
	now := r.now()

	// 1. Account state.
	if p.AccountState != "" && p.AccountState != domain.AccountActive {
		// A pending deletion may still cancel it, or the account would have no
		// way back at all.
		if p.AccountState == domain.AccountDeletionPending && action == ActionAccountDeleteSelf {
			return allow(action, SourceAccountState, "account is pending deletion and may withdraw the request")
		}
		return deny(action, SourceAccountState, "account state "+string(p.AccountState)+" does not permit any action")
	}

	// 2. Constraints imposed by the target itself. These are properties of the
	// record, not permissions, so they are checked before any role or grant: a
	// submitted review is immutable, and a judging write needs an assignment.
	// Placing them here rather than at the end is the point. Checked last, a
	// judge's role default would already have permitted the write and the
	// constraint would never be consulted.
	if target.Submitted {
		switch action {
		case ActionReviewWriteOwn, ActionReviewSubmit, ActionReviewReopen:
			return deny(action, SourceLockedObject,
				"this review has been submitted and is immutable; a submitted score cannot be moved after the standings are visible")
		}
	}
	switch action {
	case ActionReviewWriteOwn, ActionReviewSubmit:
		// Only a known negative is refused here. A target the caller has not
		// yet checked against the assignment table is deferred to the handler,
		// which can check it; refusing on unknown would make the route-level
		// gate unusable, and permitting on unknown would be a hole, so the
		// handler is required to resolve it and does.
		if target.Assignment == AssignmentNotHeld {
			return deny(action, SourceUnassigned,
				"this project is not assigned to the caller, so no review may be written for it")
		}
	}

	grants := r.grantsFor(p.UserID)

	// 3. Explicit denies win over everything.
	if grant, ok := matchingGrant(grants, action, target, now, false); ok {
		return deny(action, SourceDeniedByGrant, grant.Reason+" (explicit deny by "+grant.GrantedBy+")")
	}

	// 4. Explicit allows.
	if grant, ok := matchingGrant(grants, action, target, now, true); ok {
		decision := allow(action, SourceGrant, grant.Reason+" (explicit grant by "+grant.GrantedBy+")")
		decision.ExpiresAt = grant.ExpiresAt
		return decision
	}

	// 5. Per-event role.
	//
	// Consulted only for a target that names an event, and only when the caller
	// holds that role on that event, so a co-organizer of one event inherits
	// nothing on any other.
	//
	// It is a union with the global role, not an intersection with it.
	// Intersection is the intuitive reading and it is wrong in practice: an
	// organizer who owns an event is plainly authorized to manage it, and
	// intersecting would refuse them because the bare global role does not
	// enumerate every event-level action. The property worth protecting is that
	// an event role cannot reach outside its own event, and the event scoping
	// above already guarantees that. Also requiring the global role would add a
	// second way to lock an owner out of their own event.
	hasEventRole := p.EventRoleFor != "" && p.EventRoleFor == target.EventID && target.EventID != ""
	if hasEventRole {
		if EventRoleAllows(p.EventRole, action) {
			return allow(action, SourceEventRole, "granted by event role "+string(p.EventRole)+" on this event")
		}
		// Not in the event role, which is not a refusal: the event role is an
		// additional grant rather than a restriction layered on the global role,
		// so the global role still gets its say below.
	}

	// 6. Global role default.
	if RoleAllows(p.Role, action) {
		return allow(action, SourceRole, "granted by the default rights of role "+string(p.Role))
	}

	// 7. Ownership.
	if target.OwnerID != "" && target.OwnerID == p.UserID {
		switch action {
		case ActionSubmissionUpdateOwn, ActionTeamUpdate, ActionTeamInvite, ActionTeamManageUser:
			return allow(action, SourceOwnership, "the caller owns this object")
		}
	}

	return deny(action, SourceNone, "no role, event role, grant or ownership rule permits "+string(action))
}

// grantsFor reads the grant source, tolerating a nil one.
func (r *Resolver) grantsFor(userID string) []Grant {
	if r.grants == nil {
		return nil
	}
	return r.grants.GrantsFor(userID)
}

// matchingGrant finds the narrowest applicable grant with the requested effect.
// Narrowest first matters: a deny scoped to one object must be found before a
// broad allow, and a deny on a specific event before a platform-wide allow.
//
// Within equal scope, the most recently created wins, so revoking and re-granting
// behaves the way an operator expects rather than depending on map order.
func matchingGrant(grants []Grant, action Action, target Target, now time.Time, wantAllow bool) (Grant, bool) {
	type candidate struct {
		grant   Grant
		scope   int
		created time.Time
	}
	var best *candidate
	for _, grant := range grants {
		if grant.Allow != wantAllow || grant.Action != action {
			continue
		}
		if !grant.Applies(target, now) {
			continue
		}
		scope := 0
		if grant.EventID != "" {
			scope += 2
		}
		if grant.ObjectID != "" {
			scope++
		}
		if best == nil || scope > best.scope ||
			(scope == best.scope && grant.CreatedAt.After(best.created)) {
			found := candidate{grant: grant, scope: scope, created: grant.CreatedAt}
			best = &found
		}
	}
	if best == nil {
		return Grant{}, false
	}
	return best.grant, true
}

// Matrix is the published view of who may do what, for the permission endpoint
// and the documentation.
type Matrix map[string][]string

// BuildMatrix renders the static role defaults. It is deliberately derived from
// the same tables the resolver reads, so the published matrix cannot drift from
// what is enforced.
func BuildMatrix() Matrix {
	matrix := make(Matrix)
	for role := range RoleDefaults {
		actions := ActionsForRole(role)
		names := make([]string, 0, len(actions))
		for _, action := range actions {
			names = append(names, string(action))
		}
		matrix[string(role)] = names
	}
	return matrix
}

// BuildEventMatrix renders the per-event role grants.
func BuildEventMatrix() Matrix {
	matrix := make(Matrix)
	for role := range EventRoleGrants {
		actions := ActionsForEventRole(role)
		names := make([]string, 0, len(actions))
		for _, action := range actions {
			names = append(names, string(action))
		}
		matrix[string(role)] = names
	}
	return matrix
}

// Resources lists every distinct resource in the vocabulary, sorted.
func Resources() []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, 32)
	for _, action := range allActions {
		resource := action.Resource()
		if _, ok := seen[resource]; ok {
			continue
		}
		seen[resource] = struct{}{}
		out = append(out, resource)
	}
	sort.Strings(out)
	return out
}

// ResolutionOrder describes the order the resolver consults its sources, for the
// published matrix endpoint.
//
// Publishing it matters: an authorization model nobody can describe is one
// nobody can reason about during an incident, and the order is the part that is
// genuinely surprising.
func ResolutionOrder() []string {
	return []string{
		"account_state: a suspended account does nothing",
		"target_constraints: a submitted review is immutable; a judging write needs an assignment",
		"explicit_deny: a deny beats every allow, and beats the role",
		"explicit_grant: narrow, expiring, auditable",
		"event_role: only for a target that names the event, and only for the event the role was granted on",
		"role_default: the coarse global defaults",
		"ownership: your own object",
	}
}
