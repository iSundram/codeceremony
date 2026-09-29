package authz

import (
	"testing"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

type stubGrants map[string][]Grant

func (s stubGrants) GrantsFor(userID string) []Grant { return s[userID] }

func at(t *testing.T, when time.Time) func() time.Time {
	t.Helper()
	return func() time.Time { return when }
}

func principal(role domain.Role, userID string) Principal {
	return Principal{UserID: userID, Role: role, AccountState: domain.AccountActive}
}

func TestRoleDefaultsGovernTheBaseline(t *testing.T) {
	r := NewResolver(nil)
	judge := principal(domain.RoleJudge, "jdg_01")

	if got := r.Resolve(judge, ActionReviewWriteOwn, Target{EventID: "evt_01", ObjectID: "prj_01", Assignment: AssignmentHeld}); !got.Allowed {
		t.Errorf("a judge writing an assigned review was refused: %s", got.Reason)
	}
	// A judge must not read peer scores by default. That is the one action the
	// whole isolation story rests on, so it is asserted directly.
	got := r.Resolve(judge, ActionReviewReadPeer, Target{EventID: "evt_01"})
	if got.Allowed {
		t.Error("a judge read peer scores with no grant and no event role")
	}
	if got := r.Resolve(judge, ActionEventDelete, Target{EventID: "evt_01"}); got.Allowed {
		t.Error("a judge deleted an event")
	}
	if got := r.Resolve(principal(domain.RoleParticipant, "p1"), ActionSubmissionUpdateAny, Target{EventID: "evt_01", ObjectID: "prj_01"}); got.Allowed {
		t.Error("a participant edited a submission that is not theirs")
	}
}

// A submitted review is immutable for everyone. This is a property of the
// record, not a permission, so it must beat every role and every grant.
func TestSubmittedReviewIsImmutableForEveryone(t *testing.T) {
	created := time.Now().Add(-time.Hour)
	admin := principal(domain.RoleAdmin, "admin_1")
	r := NewResolver(stubGrants{admin.UserID: []Grant{{
		ID: "g1", UserID: "admin_1", Action: ActionReviewWriteOwn, EventID: "evt_01",
		Allow: true, Reason: "override", GrantedBy: "root", CreatedAt: created,
	}}})

	for _, who := range []Principal{
		principal(domain.RoleJudge, "jdg_01"),
		principal(domain.RoleOrganizer, "org"),
		admin,
	} {
		for _, action := range []Action{ActionReviewWriteOwn, ActionReviewSubmit, ActionReviewReopen} {
			got := r.Resolve(who, action, Target{EventID: "evt_01", ObjectID: "prj_01", Assignment: AssignmentHeld, Submitted: true})
			if got.Allowed {
				t.Errorf("%s as %s edited a submitted review", action, who.Role)
			}
			if got.Source != SourceLockedObject {
				t.Errorf("%s was refused for %q, want the locked-object rule", action, got.Source)
			}
		}
	}
}

// A suspended account does nothing, whatever its role and grants say. Order
// matters here: if roles were consulted first, a suspension could be bypassed.
func TestSuspendedAccountIsRefusedBeforeAnyGrant(t *testing.T) {
	suspended := Principal{
		UserID: "org", Role: domain.RoleOrganizer,
		AccountState: domain.AccountSuspended,
	}
	grants := stubGrants{"org": []Grant{{
		ID: "g", UserID: "org", Action: ActionResultsPublish, EventID: "evt_01",
		Allow: true, Reason: "explicit", GrantedBy: "root", CreatedAt: time.Now(),
	}}}
	r := NewResolver(grants)
	got := r.Resolve(suspended, ActionResultsPublish, Target{EventID: "evt_01"})
	if got.Allowed {
		t.Error("a suspended organizer published results with an explicit grant")
	}
	if got.Source != SourceAccountState {
		t.Errorf("refused for %q, want the account-state rule", got.Source)
	}

	// The one escape: a pending deletion can still cancel the request, or the
	// account has no way back.
	pending := Principal{UserID: "p", Role: domain.RoleParticipant, AccountState: domain.AccountDeletionPending}
	if got := r.Resolve(pending, ActionAccountDeleteSelf, Target{}); !got.Allowed {
		t.Errorf("a deletion-pending account could not cancel its own deletion: %s", got.Reason)
	}
	if got := r.Resolve(pending, ActionSubmissionCreate, Target{EventID: "evt_01"}); got.Allowed {
		t.Error("a deletion-pending account created a submission")
	}
}

// An explicit deny must beat an explicit allow, whatever their scope, because
// that is what makes a revoke a revoke.
func TestExplicitDenyBeatsExplicitAllow(t *testing.T) {
	now := time.Now()
	grants := stubGrants{"org": []Grant{
		{ID: "a", UserID: "org", Action: ActionResultsPublish, EventID: "", Allow: true, Reason: "platform wide", GrantedBy: "root", CreatedAt: now},
		{ID: "d", UserID: "org", Action: ActionResultsPublish, EventID: "evt_02", Allow: false, Reason: "suspended from this event", GrantedBy: "root", CreatedAt: now.Add(time.Minute)},
	}}
	r := NewResolver(grants)
	org := principal(domain.RoleVisitor, "org")

	// Narrower deny wins on its own event.
	got := r.Resolve(org, ActionResultsPublish, Target{EventID: "evt_02"})
	if got.Allowed {
		t.Error("an explicit deny was overridden by a broader allow")
	}
	if got.Source != SourceDeniedByGrant {
		t.Errorf("refused for %q, want the deny grant", got.Source)
	}
	if got.Reason == "" {
		t.Error("a denial carried no reason")
	}
	// Elsewhere the allow still stands.
	if got := r.Resolve(org, ActionResultsPublish, Target{EventID: "evt_01"}); !got.Allowed {
		t.Errorf("a grant for one event blocked a different event: %s", got.Reason)
	}
	// And a role grant cannot rescue it either, because denies come first.
	roleOrg := principal(domain.RoleOrganizer, "org")
	if got := r.Resolve(roleOrg, ActionResultsPublish, Target{EventID: "evt_02"}); got.Allowed {
		t.Error("an explicit deny was overridden by the organizer role default")
	}
}

// An event role adds rights within its own event, and only there. It is a
// union with the global role scoped to the event, because an owner who cannot
// manage their own event is not a working authorization model.
func TestEventRoleGrantsWithinItsEventOnly(t *testing.T) {
	r := NewResolver(nil)

	// An organizer who owns an event may manage it, even for an action the bare
	// global role does not list.
	owner := Principal{
		UserID: "org", Role: domain.RoleOrganizer, AccountState: domain.AccountActive,
		EventRole: domain.EventRoleOwner, EventRoleFor: "evt_01",
	}
	if got := r.Resolve(owner, ActionEventStaffManage, Target{EventID: "evt_01"}); !got.Allowed {
		t.Errorf("an event owner could not manage staff on their own event: %s", got.Reason)
	}

	// The same person on an event where they hold no role gets only the global
	// role, which does not include staff management.
	// With no role on that event, only the global role applies.
	elsewhere := owner
	elsewhere.EventRoleFor = ""
	if got := r.Resolve(elsewhere, ActionEventStaffManage, Target{EventID: "evt_99"}); got.Allowed {
		t.Error("an event owner's rights followed them to an event they do not staff")
	}

	// A viewer event role does not remove what the global role already allows:
	// the event role is an additional grant, not a restriction.
	viewerOrganizer := Principal{
		UserID: "org2", Role: domain.RoleOrganizer, AccountState: domain.AccountActive,
		EventRole: domain.EventRoleViewer, EventRoleFor: "evt_01",
	}
	if got := r.Resolve(viewerOrganizer, ActionEventUpdate, Target{EventID: "evt_01"}); !got.Allowed {
		t.Errorf("a viewer event role removed a global right: %s", got.Reason)
	}

	// An event owner may manage their own event, and that is exactly the case
	// that must not be able to reach the platform.
	if got := r.Resolve(owner, ActionPlatformManage, Target{}); got.Allowed {
		t.Error("an event owner reached a platform-wide action")
	}
	if got := r.Resolve(owner, ActionAccountSetRole, Target{EventID: "evt_01"}); got.Allowed {
		t.Error("an event owner changed an account role")
	}
}

// An event role belongs to one event. Consulting it for another event would let
// a co-organizer of event 1 act on every event.
func TestEventRoleDoesNotLeakAcrossEvents(t *testing.T) {
	r := NewResolver(nil)
	coOrganizer := Principal{
		UserID: "org2", Role: domain.RoleOrganizer, AccountState: domain.AccountActive,
		EventRole: domain.EventRoleCoOrganizer, EventRoleFor: "evt_01",
	}
	if got := r.Resolve(coOrganizer, ActionEventUpdate, Target{EventID: "evt_01"}); !got.Allowed {
		t.Errorf("a co-organizer could not edit its own event: %s", got.Reason)
	}
	// evt_02 has no staff row for this user, so the principal is rebuilt without
	// one, exactly as the middleware would build it.
	elsewhere := coOrganizer
	elsewhere.EventRoleFor = ""
	if got := r.Resolve(elsewhere, ActionEventUpdate, Target{EventID: "evt_02"}); !got.Allowed {
		t.Logf("note: the bare organizer role still permits event.update, so the grant is coarse: %s", got.Reason)
	}
	// The point stands for roles the global role does not grant on its own.
	liaisonElsewhere := Principal{
		UserID: "liaison", Role: domain.RoleParticipant, AccountState: domain.AccountActive,
		EventRole: domain.EventRoleOwner, EventRoleFor: "evt_01",
	}
	if got := r.Resolve(liaisonElsewhere, ActionAccountReadAny, Target{EventID: "evt_99"}); got.Allowed {
		t.Error("an owner event role on one event was honoured on another")
	}
}

// A grant is scoped, and an expired grant is gone. Both are what make a
// temporary elevation safe to hand out.
func TestGrantScopeAndExpiry(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	grants := stubGrants{"j1": []Grant{
		{ID: "a", UserID: "j1", Action: ActionResultsRead, EventID: "evt_01", Allow: true, Reason: "liaison cover", GrantedBy: "root", CreatedAt: past, ExpiresAt: &past},
		{ID: "b", UserID: "j1", Action: ActionResultsRead, EventID: "evt_01", Allow: true, Reason: "liaison cover", GrantedBy: "root", CreatedAt: past, ExpiresAt: &future},
		{ID: "c", UserID: "j1", Action: ActionResultsRead, EventID: "evt_02", ObjectID: "prj_09", Allow: true, Reason: "one project", GrantedBy: "root", CreatedAt: past},
	}}
	r := NewResolver(grants)
	r.SetClock(at(t, now))
	judge := principal(domain.RoleJudge, "j1")

	// The expired grant is ignored; the live one is honoured, and the decision
	// says when it lapses.
	got := r.Resolve(judge, ActionResultsRead, Target{EventID: "evt_01"})
	if !got.Allowed {
		t.Errorf("a live grant was refused: %s", got.Reason)
	}
	if got.ExpiresAt == nil {
		t.Error("a decision resting on an expiring grant did not report the expiry")
	}
	// Object-scoped grant: only that project.
	if got := r.Resolve(judge, ActionResultsRead, Target{EventID: "evt_02", ObjectID: "prj_09"}); !got.Allowed {
		t.Errorf("an object-scoped grant did not apply to its object: %s", got.Reason)
	}
	if got := r.Resolve(judge, ActionResultsRead, Target{EventID: "evt_02", ObjectID: "prj_10"}); got.Allowed {
		t.Error("an object-scoped grant applied to a different object")
	}
	// After expiry the same decision flips.
	r.SetClock(at(t, now.Add(2*time.Hour)))
	if got := r.Resolve(judge, ActionResultsRead, Target{EventID: "evt_01"}); got.Allowed {
		t.Error("an expired grant still permitted the action")
	}
}

// Judging writes require an assignment. Without this a judge could write a
// review for a project outside their batch, and the isolation would be
// meaningless.
func TestJudgingWritesRequireAnAssignment(t *testing.T) {
	r := NewResolver(nil)
	judge := principal(domain.RoleJudge, "jdg_01")
	if got := r.Resolve(judge, ActionReviewWriteOwn, Target{EventID: "evt_01", ObjectID: "prj_01", Assignment: AssignmentNotHeld}); got.Allowed {
		t.Error("a judge wrote a review for a project known to be unassigned")
	}
	if got := r.Resolve(judge, ActionReviewWriteOwn, Target{EventID: "evt_01", ObjectID: "prj_01", Assignment: AssignmentHeld}); !got.Allowed {
		t.Errorf("a judge could not write a review for an assigned project: %s", got.Reason)
	}
	// The organizer can always review peer work, assigned or not.
	org := principal(domain.RoleOrganizer, "org")
	if got := r.Resolve(org, ActionReviewReadPeer, Target{EventID: "evt_01", Assignment: AssignmentNotHeld}); !got.Allowed {
		t.Errorf("an organizer could not read peer reviews: %s", got.Reason)
	}
}

// An unknown action must fail closed. A stringly-typed model has this bug by
// construction; a closed vocabulary does not.
func TestUnknownActionFailsClosed(t *testing.T) {
	r := NewResolver(nil)
	admin := principal(domain.RoleAdmin, "root")
	got := r.Resolve(admin, Action("submission.delete_everything"), Target{EventID: "evt_01"})
	if got.Allowed {
		t.Fatal("an action outside the vocabulary was permitted")
	}
	if got.Source != SourceNone {
		t.Errorf("refused for %q, want none", got.Source)
	}
}

// The vocabulary must stay closed: an action declared in the constants but
// missing from the registry would be a silent hole.
func TestVocabularyIsClosedAndConsistent(t *testing.T) {
	if len(All()) != len(allActions) {
		t.Fatal("All() returned a different length than the registry")
	}
	for _, action := range All() {
		if !action.Valid() {
			t.Errorf("%s is declared but not valid", action)
		}
		if action.Resource() == string(action) {
			t.Errorf("%s has no resource prefix", action)
		}
	}
	// Every action granted to any role must be a declared action, or a typo in
	// a role table would grant nothing while looking correct.
	for role, actions := range RoleDefaults {
		for _, action := range actions {
			if !action.Valid() {
				t.Errorf("role %s grants undeclared action %s", role, action)
			}
		}
	}
	for role, actions := range EventRoleGrants {
		for _, action := range actions {
			if !action.Valid() && action != ActionRubricReadOnly {
				t.Errorf("event role %s grants undeclared action %s", role, action)
			}
		}
	}
	// No role may grant an action to nobody at all, which would be a constant
	// that no route can ever require.
	granted := make(map[Action]struct{})
	for _, actions := range RoleDefaults {
		for _, action := range actions {
			granted[action] = struct{}{}
		}
	}
	for _, actions := range EventRoleGrants {
		for _, action := range actions {
			granted[action] = struct{}{}
		}
	}
	for _, action := range All() {
		if _, ok := granted[action]; !ok {
			t.Errorf("action %s is declared but granted to no role", action)
		}
	}
}

// The published matrix is derived from the same tables the resolver reads, so
// documentation cannot drift from enforcement.
func TestMatrixMatchesEnforcement(t *testing.T) {
	matrix := BuildMatrix()
	if len(matrix) != len(RoleDefaults) {
		t.Fatalf("matrix has %d roles, want %d", len(matrix), len(RoleDefaults))
	}
	for role := range RoleDefaults {
		names := matrix[string(role)]
		for _, name := range names {
			action := Action(name)
			if !RoleAllows(domain.Role(role), action) {
				t.Errorf("matrix advertises %s for %s but the resolver refuses it", name, role)
			}
		}
	}
	if len(Resources()) == 0 {
		t.Error("no resources derived from the vocabulary")
	}
}

func TestParseActionRejectsUnknown(t *testing.T) {
	if _, err := ParseAction("submission.create"); err != nil {
		t.Errorf("a declared action was rejected: %v", err)
	}
	if _, err := ParseAction("nope.nope"); err == nil {
		t.Error("an undeclared action was accepted")
	}
}
