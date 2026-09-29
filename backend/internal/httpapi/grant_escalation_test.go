package httpapi

import (
	"net/http"
	"testing"
)

// A grant is the one record in the system that can manufacture authority, so
// the two questions "may this caller mint a grant" and "may this caller hand out
// this permission" have to be answered separately.
//
// They were the same question. ActionGrantManage is held by every organizer by
// role, and a grant with no event_id is evaluated against an unscoped target —
// which is to say platform-wide. So any organizer could POST a grant with no
// event for any action in the vocabulary, naming themselves as the beneficiary,
// and the resolver would honour it on the very next request. The demo ends with
// the organizer promoting a participant to admin through the ordinary admin
// role route.
func TestAnOrganizerCannotMintAPlatformWideGrantForThemselves(t *testing.T) {
	server, tokens, data := newTestServerWithSessions(t)
	organizer := tokenFor(t, tokens, data, "organizer")

	// The escalation itself: no event, so platform-wide, and aimed at the
	// caller. account.set_role is the action that turns it into an admin.
	escalation := map[string]any{
		"user_id": "organizer", "action": "account.set_role",
		"allow": true, "reason": "self promotion",
	}
	got := request(t, server, http.MethodPost, "/v1/grants", organizer, escalation)
	if got.Code == http.StatusCreated {
		t.Fatalf("an organizer minted a platform-wide self-grant: %d, body = %s", got.Code, got.Body.String())
	}
	if got.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403; the refusal must be an authorization one, not a validation error", got.Code)
	}

	// And the grant must not have been written, whatever the status said.
	listed := request(t, server, http.MethodGet, "/v1/grants", organizer, nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("listing grants: %d, body = %s", listed.Code, listed.Body.String())
	}
	if data := decode(t, listed.Body.Bytes())["data"].([]any); len(data) != 0 {
		t.Errorf("the refused grant was persisted anyway: %+v", data)
	}
}

// The end-to-end consequence, asserted so the fix cannot pass by refusing the
// wrong request: after the refused grant, the organizer still cannot promote
// anyone.
func TestTheRefusedSelfGrantGrantsNoPromotion(t *testing.T) {
	server, tokens, data := newTestServerWithSessions(t)
	organizer := tokenFor(t, tokens, data, "organizer")

	escalation := map[string]any{
		"user_id": "organizer", "action": "account.set_role",
		"allow": true, "reason": "self promotion",
	}
	request(t, server, http.MethodPost, "/v1/grants", organizer, escalation)

	promote := request(t, server, http.MethodPut, "/v1/admin/users/organizer/role", organizer,
		map[string]any{"role": "admin"})
	if promote.Code == http.StatusOK {
		t.Fatalf("the organizer promoted themselves to admin: %d, body = %s", promote.Code, promote.Body.String())
	}
	if user, err := data.UserByID("organizer"); err == nil && user.Role == "admin" {
		t.Errorf("the organizer's role is now %q", user.Role)
	}
}

// Holding a permission is not the same as being entitled to delegate it, so an
// event-scoped grant for an action the granter does not hold is refused too.
func TestAGranterCannotDelegateAPermissionTheyDoNotHold(t *testing.T) {
	server, tokens, data := newTestServerWithSessions(t)
	organizer := tokenFor(t, tokens, data, "organizer")

	body := map[string]any{
		"user_id": "organizer", "action": "account.set_role",
		"event_id": "evt_01", "allow": true, "reason": "for later",
	}
	got := request(t, server, http.MethodPost, "/v1/grants", organizer, body)
	if got.Code == http.StatusCreated {
		t.Fatalf("an organizer delegated a permission it does not hold: %d, body = %s", got.Code, got.Body.String())
	}
}

// A refusal has to be a refusal for the right reason. The original grant test
// asserted 422 for a missing reason, and that must survive the reordering.
func TestAGrantWithNoReasonIsStillAValidationError(t *testing.T) {
	server, tokens, data := newTestServerWithSessions(t)
	organizer := tokenFor(t, tokens, data, "organizer")

	got := request(t, server, http.MethodPost, "/v1/grants", organizer, map[string]any{
		"user_id": "judge_a", "action": "results.read", "event_id": "evt_01", "allow": true,
	})
	if got.Code != http.StatusUnprocessableEntity {
		t.Errorf("a grant with no reason: status = %d, want 422; body = %s", got.Code, got.Body.String())
	}
}
