package httpapi

import (
	"net/http"
	"testing"

	"github.com/iSundram/codeceremony/backend/internal/authz"
	"github.com/iSundram/codeceremony/backend/internal/domain"
)

func TestActivityLogIsRecordedAndFiltered(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	open := createOpenEvent(t, server, tokens, data)

	captainEdit := request(t, server, http.MethodPatch, "/v1/submissions/"+open.projectID, open.captain, map[string]any{"title": "Lantern revised"})
	if captainEdit.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", captainEdit.Code, captainEdit.Body.String())
	}
	resubmitted := request(t, server, http.MethodPost, "/v1/submissions/"+open.projectID+"/submit", open.captain, nil)
	if resubmitted.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", resubmitted.Code, resubmitted.Body.String())
	}

	public := request(t, server, http.MethodGet, "/v1/events/"+open.slug+"/activity?visibility=public", "", nil)
	if public.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", public.Code, public.Body.String())
	}
	if !containsProjectTitle(public.Body.String(), "is submitted and awaiting review") {
		t.Fatalf("expected the submission activity to be public: %s", public.Body.String())
	}

	all := request(t, server, http.MethodGet, "/v1/activity?event_id="+open.eventID, organizer, nil)
	if all.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", all.Code, all.Body.String())
	}
	payload := decodeAssignmentsBody(t, all)
	if payload["total"].(float64) < 2 {
		t.Fatalf("expected several activity entries, body = %s", all.Body.String())
	}
	categories, ok := payload["by_category"].(map[string]any)
	if !ok || categories["submission"] == nil {
		t.Fatalf("expected a category rollup, body = %s", all.Body.String())
	}

	filtered := request(t, server, http.MethodGet, "/v1/activity?event_id="+open.eventID+"&category=submission&limit=1", organizer, nil)
	if filtered.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", filtered.Code, filtered.Body.String())
	}
	filteredPayload := decodeAssignmentsBody(t, filtered)
	if len(filteredPayload["data"].([]any)) != 1 {
		t.Fatalf("expected the limit to apply: %s", filtered.Body.String())
	}

	counts := request(t, server, http.MethodGet, "/v1/activity?event_id="+open.eventID+"&counts_only=true", organizer, nil)
	if len(decodeAssignmentsBody(t, counts)["data"].([]any)) != 0 {
		t.Fatalf("expected counts_only to return no entries: %s", counts.Body.String())
	}
}

func TestActivityVisibilityIsEnforcedPerRole(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	open := createOpenEvent(t, server, tokens, data)
	if locked := request(t, server, http.MethodPut, "/v1/organizer/submissions/"+open.projectID+"/eligibility", organizer, map[string]any{
		"decision": "ineligible",
		"note":     "internal review note",
	}); locked.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", locked.Code, locked.Body.String())
	}

	public := request(t, server, http.MethodGet, "/v1/activity?event_id="+open.eventID, tokenFor(t, tokens, data, "participant_other"), nil)
	if public.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", public.Code, public.Body.String())
	}
	if containsProjectTitle(public.Body.String(), "internal review note") {
		t.Fatalf("organizer-only activity must not leak to participants: %s", public.Body.String())
	}
	staff := request(t, server, http.MethodGet, "/v1/activity?event_id="+open.eventID, organizer, nil)
	if !containsProjectTitle(staff.Body.String(), "internal review note") {
		t.Fatalf("organizers should see organizer-only activity: %s", staff.Body.String())
	}
	_ = open
}

func TestEventStaffLifecycleAndPermissions(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	listed := request(t, server, http.MethodGet, "/v1/events/sample-hack-2026/staff", "", nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", listed.Code, listed.Body.String())
	}
	if !containsProjectTitle(listed.Body.String(), "owner") {
		t.Fatalf("expected the seeded owner, body = %s", listed.Body.String())
	}

	// The seeded admin already holds a co_organizer role, so an equal or lower
	// role must be refused.
	equal := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/staff", organizer, map[string]any{
		"user_id": "admin",
		"role":    "co_organizer",
	})
	if equal.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", equal.Code, equal.Body.String())
	}
	lower := request(t, server, http.MethodPost, "/v1/events/sample-hack-2026/staff", organizer, map[string]any{
		"user_id": "admin",
		"role":    "viewer",
	})
	if lower.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", lower.Code, lower.Body.String())
	}

	// A fresh hackathon has no staff yet, so the same user can be added.
	created := request(t, server, http.MethodPost, "/v1/events", organizer, map[string]any{
		"slug":              "staffed-hack-2026",
		"name":              "Staffed Hack 2026",
		"registration_open": true,
		"submissions_open":  true,
		"submissions_close": "2026-07-01T18:00:00Z",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", created.Code, created.Body.String())
	}
	added := request(t, server, http.MethodPost, "/v1/events/staffed-hack-2026/staff", organizer, map[string]any{
		"user_id": "admin",
		"role":    "judge_liaison",
		"title":   "Judging liaison",
	})
	if added.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", added.Code, added.Body.String())
	}
	promoted := request(t, server, http.MethodPost, "/v1/events/staffed-hack-2026/staff", organizer, map[string]any{
		"user_id": "admin",
		"role":    "co_organizer",
	})
	if promoted.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", promoted.Code, promoted.Body.String())
	}
	notStaff := request(t, server, http.MethodPost, "/v1/events/staffed-hack-2026/staff", organizer, map[string]any{
		"user_id": "participant",
		"role":    "co_organizer",
	})
	if notStaff.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", notStaff.Code, notStaff.Body.String())
	}
	badRole := request(t, server, http.MethodPost, "/v1/events/staffed-hack-2026/staff", organizer, map[string]any{
		"user_id": "participant_other",
		"role":    "wizard",
	})
	if badRole.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", badRole.Code, badRole.Body.String())
	}
	participantAttempt := request(t, server, http.MethodPost, "/v1/events/staffed-hack-2026/staff", tokenFor(t, tokens, data, "participant"), map[string]any{
		"user_id": "participant_other",
		"role":    "co_organizer",
	})
	if participantAttempt.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", participantAttempt.Code, participantAttempt.Body.String())
	}
	removed := request(t, server, http.MethodDelete, "/v1/events/staffed-hack-2026/staff/admin", organizer, nil)
	if removed.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", removed.Code, removed.Body.String())
	}
	afterRemoval := request(t, server, http.MethodGet, "/v1/events/staffed-hack-2026/staff", "", nil)
	if containsProjectTitle(afterRemoval.Body.String(), "admin@example.org") {
		t.Fatalf("expected the removed staff member to disappear: %s", afterRemoval.Body.String())
	}
}

// The published matrix must describe every action, and must be derived from the
// table the resolver reads. The previous version of this endpoint enumerated
// permissions separately from the code that enforced them and had already
// drifted, so the test now checks the two are the same thing rather than
// checking a hand-written list is self-consistent.
func TestPermissionMatrixDescribesEveryAction(t *testing.T) {
	server, _, _ := newTestServer(t)
	response := request(t, server, http.MethodGet, "/v1/permissions", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", response.Code, response.Body.String())
	}
	payload := decodeAssignmentsBody(t, response)
	data := payload["data"].(map[string]any)

	actions := data["actions"].([]any)
	if len(actions) != len(authz.All()) {
		t.Fatalf("matrix lists %d actions, want %d", len(actions), len(authz.All()))
	}
	seen := make(map[string]bool, len(actions))
	for _, raw := range actions {
		action := raw.(string)
		seen[action] = true
		if !authz.Action(action).Valid() {
			t.Errorf("the matrix advertises %q, which is not a declared action", action)
		}
	}
	for _, action := range authz.All() {
		if !seen[string(action)] {
			t.Errorf("action %q is missing from the published matrix", action)
		}
	}

	platform := data["platform_roles"].(map[string]any)
	for role, raw := range platform {
		for _, name := range raw.([]any) {
			action := authz.Action(name.(string))
			if !authz.RoleAllows(domain.Role(role), action) {
				t.Errorf("matrix says role %s may %s but the resolver refuses it", role, name)
			}
		}
	}
	event := data["event_roles"].(map[string]any)
	for role, raw := range event {
		parsed, err := domain.ParseEventRole(role)
		if err != nil {
			t.Errorf("matrix advertises an unknown event role %q", role)
			continue
		}
		for _, name := range raw.([]any) {
			action := authz.Action(name.(string))
			if action == authz.ActionRubricReadOnly {
				continue
			}
			if !authz.EventRoleAllows(parsed, action) {
				t.Errorf("matrix says event role %s may %s but the resolver refuses it", role, name)
			}
		}
	}

	order := data["resolution_order"].([]any)
	if len(order) < 5 {
		t.Errorf("resolution order has %d steps, want the full policy", len(order))
	}
	resources := data["resources"].([]any)
	if len(resources) == 0 {
		t.Error("no resources published")
	}
}

func TestEventDirectoryCountsAndFilters(t *testing.T) {
	server, _, _ := newTestServer(t)
	response := request(t, server, http.MethodGet, "/v1/directory", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", response.Code, response.Body.String())
	}
	entries := decodeAssignmentsBody(t, response)["data"].([]any)
	if len(entries) == 0 {
		t.Fatalf("expected at least the seeded hackathon")
	}
	first := entries[0].(map[string]any)
	entry := first["event"].(map[string]any)
	if entry["slug"] != "sample-hack-2026" {
		t.Fatalf("unexpected event in directory: %v", entry)
	}
	if first["tracks"].(float64) != 3 || first["submissions"].(float64) != 4 {
		t.Fatalf("unexpected directory counts: %v", first)
	}
	if first["judges"].(float64) != 2 || first["questions"].(float64) == 0 {
		t.Fatalf("expected judge and question counts: %v", first)
	}

	filtered := request(t, server, http.MethodGet, "/v1/directory?q=sample+hack", "", nil)
	if filtered.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", filtered.Code, filtered.Body.String())
	}
	if decodeAssignmentsBody(t, filtered)["count"].(float64) != 1 {
		t.Fatalf("expected the search filter to match one hackathon: %s", filtered.Body.String())
	}
	byState := request(t, server, http.MethodGet, "/v1/directory?state=results_published", "", nil)
	if decodeAssignmentsBody(t, byState)["count"].(float64) != 0 {
		t.Fatalf("expected no published hackathons: %s", byState.Body.String())
	}
	single := request(t, server, http.MethodGet, "/v1/directory?slug=sample-hack-2026", "", nil)
	if decodeAssignmentsBody(t, single)["data"].(map[string]any) == nil {
		t.Fatalf("expected a single directory entry: %s", single.Body.String())
	}
	if missing := request(t, server, http.MethodGet, "/v1/directory?slug=nope", "", nil); missing.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", missing.Code, missing.Body.String())
	}
}

func TestMailPreferencesUpdate(t *testing.T) {
	server, tokens, data := newTestServer(t)
	participant := tokenFor(t, tokens, data, "participant")
	read := request(t, server, http.MethodGet, "/v1/email/preferences", participant, nil)
	if read.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", read.Code, read.Body.String())
	}
	if !containsProjectTitle(read.Body.String(), "preferences_url") {
		t.Fatalf("expected the preferences url: %s", read.Body.String())
	}

	updated := request(t, server, http.MethodPatch, "/v1/email/preferences", participant, map[string]any{
		"marketing":        false,
		"team_activity":    false,
		"account_security": true,
	})
	if updated.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", updated.Code, updated.Body.String())
	}
	preferences := decodeAssignmentsBody(t, updated)["data"].(map[string]any)
	if preferences["marketing"] != false || preferences["team_activity"] != false {
		t.Fatalf("preferences not saved: %v", preferences)
	}
	if preferences["transactional"] != true {
		t.Fatalf("transactional mail should stay enabled: %v", preferences)
	}
	invalid := request(t, server, http.MethodPatch, "/v1/email/preferences", participant, map[string]any{
		"transactional":    false,
		"account_security": true,
	})
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", invalid.Code, invalid.Body.String())
	}
}

func TestVerificationAndResetMailEndpoints(t *testing.T) {
	server, tokens, data := newTestServer(t)
	participant := tokenFor(t, tokens, data, "participant")
	verify := request(t, server, http.MethodPost, "/v1/email/verify", participant, nil)
	if verify.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202, body = %s", verify.Code, verify.Body.String())
	}
	payload := decodeAssignmentsBody(t, verify)
	if payload["queued"] != true {
		t.Fatalf("expected the verification mail to be queued: %s", verify.Body.String())
	}
	reset := request(t, server, http.MethodPost, "/v1/email/password-reset", participant, nil)
	if reset.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202, body = %s", reset.Code, reset.Body.String())
	}
	if anonymous := request(t, server, http.MethodPost, "/v1/email/verify", "", nil); anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", anonymous.Code, anonymous.Body.String())
	}
}

func TestUnsubscribeEndpoint(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	participant := tokenFor(t, tokens, data, "participant")
	queued := request(t, server, http.MethodPost, "/v1/email/verify", participant, nil)
	if queued.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202, body = %s", queued.Code, queued.Body.String())
	}
	outbox := request(t, server, http.MethodGet, "/v1/organizer/mail/outbox?status=queued", organizer, nil)
	if outbox.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", outbox.Code, outbox.Body.String())
	}
	if !containsProjectTitle(outbox.Body.String(), "verify_email") {
		t.Fatalf("expected the verification mail in the outbox: %s", outbox.Body.String())
	}

	unsubscribed := request(t, server, http.MethodGet, "/v1/unsubscribe/does-not-exist", "", nil)
	if unsubscribed.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", unsubscribed.Code, unsubscribed.Body.String())
	}
	missing := request(t, server, http.MethodGet, "/v1/unsubscribe", "", nil)
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", missing.Code, missing.Body.String())
	}
}

func TestAnnouncementRespectsMarketingConsent(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	announcement := request(t, server, http.MethodPost, "/v1/organizer/events/sample-hack-2026/announcements", organizer, map[string]any{
		"headline": "Results are live",
		"body":     "Check the leaderboard now.",
		"audience": "all",
	})
	if announcement.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202, body = %s", announcement.Code, announcement.Body.String())
	}
	payload := decodeAssignmentsBody(t, announcement)
	if payload["audience"] != "all" {
		t.Fatalf("expected the audience to be echoed, body = %s", announcement.Body.String())
	}
	queued := payload["queued"].([]any)
	if len(queued) != 1 {
		t.Fatalf("expected only the consenting participant to be queued: %s", announcement.Body.String())
	}
	excluded := payload["excluded"].([]any)
	if len(excluded) == 0 {
		t.Fatalf("expected exclusion reasons for non-consenting recipients: %s", announcement.Body.String())
	}

	invalid := request(t, server, http.MethodPost, "/v1/organizer/events/sample-hack-2026/announcements", organizer, map[string]any{
		"headline": "Hi",
		"body":     "There",
		"audience": "everyone on earth",
	})
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", invalid.Code, invalid.Body.String())
	}
	empty := request(t, server, http.MethodPost, "/v1/organizer/events/sample-hack-2026/announcements", organizer, map[string]any{"audience": "all"})
	if empty.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", empty.Code, empty.Body.String())
	}
	forbidden := request(t, server, http.MethodPost, "/v1/organizer/events/sample-hack-2026/announcements", tokenFor(t, tokens, data, "participant"), map[string]any{
		"headline": "Hi", "body": "There", "audience": "all",
	})
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", forbidden.Code, forbidden.Body.String())
	}
}

func TestReviewRemindersOnlyForPendingJudges(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	created := request(t, server, http.MethodPost, "/v1/organizer/assignments", organizer, map[string]any{
		"event_slug":          "sample-hack-2026",
		"reviews_per_project": 1,
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", created.Code, created.Body.String())
	}
	reminders := request(t, server, http.MethodPost, "/v1/organizer/events/sample-hack-2026/review-reminders", organizer, nil)
	if reminders.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202, body = %s", reminders.Code, reminders.Body.String())
	}
	sent := decodeAssignmentsBody(t, reminders)["sent"].([]any)
	if len(sent) == 0 {
		t.Fatalf("expected reminders for judges with pending work: %s", reminders.Body.String())
	}
}

func TestMailTemplatesAndOutboxAdmin(t *testing.T) {
	server, tokens, data := newTestServer(t)
	admin := tokenFor(t, tokens, data, "admin")
	templates := request(t, server, http.MethodGet, "/v1/organizer/mail/templates", admin, nil)
	if templates.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", templates.Code, templates.Body.String())
	}
	if !containsProjectTitle(templates.Body.String(), "hackathon_announcement") {
		t.Fatalf("expected the announcement template: %s", templates.Body.String())
	}
	participantView := request(t, server, http.MethodGet, "/v1/organizer/mail/templates", tokenFor(t, tokens, data, "participant"), nil)
	if participantView.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", participantView.Code, participantView.Body.String())
	}
	flushed := request(t, server, http.MethodPost, "/v1/organizer/mail/flush", admin, nil)
	if flushed.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", flushed.Code, flushed.Body.String())
	}
	digest := request(t, server, http.MethodPost, "/v1/organizer/mail/weekly-digest", admin, nil)
	if digest.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202, body = %s", digest.Code, digest.Body.String())
	}
}

func TestSubmissionAndTeamActivityAreRecorded(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	open := createOpenEvent(t, server, tokens, data)
	activity := request(t, server, http.MethodGet, "/v1/activity?event_id="+open.eventID+"&category=team", organizer, nil)
	if activity.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", activity.Code, activity.Body.String())
	}
	if len(decodeAssignmentsBody(t, activity)["data"].([]any)) == 0 {
		t.Fatalf("expected a team activity entry from team creation: %s", activity.Body.String())
	}
}
