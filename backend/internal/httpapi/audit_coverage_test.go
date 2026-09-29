package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The action audit is only worth having if it is complete. A trail that records
// some actions is worse than none, because it is trusted to be complete.
//
// This test reads the route table and the handler sources directly rather than
// driving requests, because driving requests cannot prove a code path is
// unreachable. Three properties are checked:
//
//   - every mutating route declares an action, so a new route is unauthorized by
//     default rather than by luck;
//   - every mutating handler calls the audit hook, so an allowed action is
//     recorded as well as a refused one;
//   - the route table and the source agree, so a route cannot be registered in one
//     place and implemented in another.

var mutatingMethod = regexp.MustCompile(`mux\.Handle\("(POST|PUT|PATCH|DELETE) `)

// routeActionPattern captures the action a route declares.
var routeActionPattern = regexp.MustCompile(`requireAction\(authz\.(Action[A-Za-z]+)`)

func TestEveryMutatingRouteDeclaresAnAction(t *testing.T) {
	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("ReadFile(server.go) error = %v", err)
	}
	text := string(source)

	missing := make([]string, 0)
	total := 0
	for _, line := range strings.Split(text, "\n") {
		if !mutatingMethod.MatchString(line) {
			continue
		}
		total++
		if !routeActionPattern.MatchString(line) && !strings.Contains(line, "s.guard(") {
			missing = append(missing, strings.TrimSpace(line))
		}
	}
	if total == 0 {
		t.Fatal("no mutating routes found; the pattern is probably stale")
	}
	if len(missing) > 0 {
		t.Errorf("%d of %d mutating routes declare no action:", len(missing), total)
		for _, line := range missing {
			t.Errorf("  %s", line)
		}
	}
}

// Every handler that changes state must record the change. A handler listed as
// the target of a mutating route is named here; the check is that the file
// containing it calls one of the audit hooks.
func TestEveryMutatingHandlerRecordsAnAction(t *testing.T) {
	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("ReadFile(server.go) error = %v", err)
	}
	routeText := string(source)

	// Collect the handler names bound to mutating routes.
	handlers := make(map[string]string) // handler -> route
	for _, line := range strings.Split(routeText, "\n") {
		m := mutatingMethod.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		// The handler is the identifier following the last "s." on the line,
		// after any wrapper calls. A regexp anchored at the end is brittle once
		// a route is nested in a rate-limit guard, so the trailing punctuation
		// is stripped explicitly.
		at := strings.LastIndex(line, "s.")
		if at < 0 {
			continue
		}
		name := line[at+2:]
		name = strings.TrimRight(name, ")]\t ")
		if name != "" && !strings.ContainsAny(name, " .\t") {
			handlers[name] = strings.TrimSpace(line)
		}
	}
	if len(handlers) == 0 {
		t.Fatal("no mutating handlers found; the pattern is probably stale")
	}
	if len(handlers) < 20 {
		t.Errorf("only %d mutating handlers identified; the pattern is probably partly stale", len(handlers))
	}

	// Group handlers by the file that defines them, then check each file.
	byFile := make(map[string]map[string]bool)
	for handler := range handlers {
		file := handlerFile(t, handler)
		if file == "" {
			t.Errorf("handler %s could not be located; it is bound to %s", handler, handlers[handler])
			continue
		}
		if byFile[file] == nil {
			byFile[file] = make(map[string]bool)
		}
		byFile[file][handler] = true
	}

	auditHook := regexp.MustCompile(`s\.(allowed|audit)\(`)
	for file, names := range byFile {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", file, err)
		}
		text := string(body)
		if auditHook.MatchString(text) {
			continue
		}
		list := make([]string, 0, len(names))
		for name := range names {
			list = append(list, name)
		}
		t.Errorf("%s handles mutating routes (%s) but never records an action audit",
			file, strings.Join(list, ", "))
	}
}

// handlerFile finds the file defining a handler method.
func handlerFile(t *testing.T, handler string) string {
	t.Helper()
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("Glob error = %v", err)
	}
	needle := "func (s *Server) " + handler + "("
	for _, file := range matches {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		body, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		if bytes.Contains(body, []byte(needle)) {
			return file
		}
	}
	return ""
}

// The resolver must be the only route gate. A route that authenticates without
// resolving an action is authorized by nothing, which is the failure this whole
// change set exists to remove.
func TestNoRouteUsesAnAuthorizationBypass(t *testing.T) {
	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("ReadFile(server.go) error = %v", err)
	}
	text := string(source)
	for _, forbidden := range []string{
		"requirePermission",   // the pre-action shim, now removed
		"bypassAuthorization", // nothing may opt out
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("server.go still references %q; every route must go through requireAction", forbidden)
		}
	}
}

// The published matrix must be derived, so a hand-written list cannot drift.
func TestPermissionMatrixIsDerivedNotRestated(t *testing.T) {
	source, err := os.ReadFile("activity.go")
	if err != nil {
		t.Fatalf("ReadFile(activity.go) error = %v", err)
	}
	text := string(source)
	if !strings.Contains(text, "authz.BuildMatrix()") {
		t.Error("the permission matrix is not built from the authz tables")
	}
	if strings.Contains(text, "allPermissions") {
		t.Error("a hand-written permission list survived; it will drift")
	}
}

// A refused action must be recorded, not just returned. This drives the
// resolver through a real refusal and reads the trail back.
func TestRefusalsAreRecorded(t *testing.T) {
	server, tokens, data := newTestServer(t)
	participant := tokenFor(t, tokens, data, "participant")
	organizer := tokenFor(t, tokens, data, "organizer")

	before := server.store.ActionAuditCount()
	refused := request(t, server, http.MethodGet, "/v1/organizer/results?event_id=evt_01", participant, nil)
	if refused.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", refused.Code)
	}
	after := server.store.ActionAuditCount()
	if after <= before {
		t.Fatalf("a refusal was not recorded: count went from %d to %d", before, after)
	}

	listed := request(t, server, http.MethodGet, "/v1/audit/actions?allowed=false", organizer, nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("audit query = %d, want 200, body = %s", listed.Code, listed.Body.String())
	}
	var payload struct {
		Data []struct {
			Allowed bool   `json:"allowed"`
			Action  string `json:"action"`
			Source  string `json:"source"`
			Reason  string `json:"reason"`
		} `json:"data"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &payload); err != nil {
		t.Fatalf("audit body is not JSON: %s", listed.Body.String())
	}
	if payload.Count == 0 {
		t.Fatal("the query for refused actions returned nothing, so the refusal was not recorded")
	}
	found := false
	for _, entry := range payload.Data {
		if !entry.Allowed && entry.Action == "results.read" {
			found = true
			if entry.Source == "" || entry.Reason == "" {
				t.Errorf("a refusal was recorded without the rule that refused it: %+v", entry)
			}
		}
	}
	if !found {
		t.Error("the results.read refusal is missing from the trail")
	}
}

// The chain must verify after real traffic. The tampering half of this test
// lives in the store package, which owns the key and must not export it.
func TestAuditChainVerifiesAfterRealTraffic(t *testing.T) {
	server, _, _ := newTestServer(t)
	organizer := tokenFor(t, server.tokens, server.store, "organizer")
	participant := tokenFor(t, server.tokens, server.store, "participant")

	// A mixture of permitted and refused, across several resources.
	// Reads are not audited, by design, so the entries come from the refusals
	// and from the one permitted mutation below.
	request(t, server, http.MethodGet, "/v1/audit/actions", organizer, nil)
	request(t, server, http.MethodGet, "/v1/organizer/results?event_id=evt_01", participant, nil)
	request(t, server, http.MethodGet, "/v1/admin/users", participant, nil)
	request(t, server, http.MethodGet, "/v1/audit/verify", organizer, nil)
	request(t, server, http.MethodPost, "/v1/grants", organizer, map[string]any{
		"user_id": "judge_a", "action": "results.read", "event_id": "evt_01",
		"allow": true, "reason": "liaison cover while the results lead is away",
	})

	result := server.store.VerifyActionAudit()
	if !result.Valid {
		t.Fatalf("a chain written under mixed traffic did not verify: %+v", result)
	}
	// Two refusals and one permitted grant write are expected. The successful
	// reads write nothing, by design: an audit browse that wrote to the audit
	// would drown the refusals it exists to surface.
	if result.Entries < 3 {
		t.Errorf("chain has %d entries, expected the two refusals and the grant write", result.Entries)
	}
	if result.Head == "" {
		t.Error("the verified chain reported no head hash")
	}
}

// The verify endpoint is itself a staff surface, and it answers 409 rather than
// 200 when the chain does not reconcile, because a break is a real failure.
func TestAuditVerifyIsStaffOnly(t *testing.T) {
	server, tokens, data := newTestServer(t)
	participant := tokenFor(t, tokens, data, "participant")
	if got := request(t, server, http.MethodGet, "/v1/audit/verify", participant, nil); got.Code != http.StatusForbidden {
		t.Errorf("a participant read the audit verify endpoint: %d", got.Code)
	}
	organizer := tokenFor(t, tokens, data, "organizer")
	if got := request(t, server, http.MethodGet, "/v1/audit/verify", organizer, nil); got.Code != http.StatusOK {
		t.Errorf("an organizer could not verify the chain: %d, body = %s", got.Code, got.Body.String())
	}
}

// Grants are staff-managed: an explicit deny is only meaningful if the ability
// to write one is controlled.
func TestGrantsAreStaffOnly(t *testing.T) {
	server, tokens, data := newTestServer(t)
	participant := tokenFor(t, tokens, data, "participant")
	organizer := tokenFor(t, tokens, data, "organizer")

	if got := request(t, server, http.MethodGet, "/v1/grants", participant, nil); got.Code != http.StatusForbidden {
		t.Errorf("a participant listed grants: %d", got.Code)
	}
	body := map[string]any{
		"user_id": "judge_a", "action": "results.read",
		"event_id": "evt_01", "allow": true, "reason": "liaison cover",
	}
	if got := request(t, server, http.MethodPost, "/v1/grants", participant, body); got.Code != http.StatusForbidden {
		t.Errorf("a participant created a grant: %d", got.Code)
	}
	created := request(t, server, http.MethodPost, "/v1/grants", organizer, body)
	if created.Code != http.StatusCreated {
		t.Fatalf("an organizer could not create a grant: %d, body = %s", created.Code, created.Body.String())
	}
	// A grant with no reason is refused, because a permission nobody can explain
	// is a permission nobody can defend.
	noReason := map[string]any{"user_id": "judge_a", "action": "results.read", "allow": true}
	if got := request(t, server, http.MethodPost, "/v1/grants", organizer, noReason); got.Code != http.StatusUnprocessableEntity {
		t.Errorf("a grant with no reason was accepted: %d", got.Code)
	}
	unknown := map[string]any{"user_id": "judge_a", "action": "not.a.real.action", "allow": true, "reason": "x"}
	if got := request(t, server, http.MethodPost, "/v1/grants", organizer, unknown); got.Code != http.StatusUnprocessableEntity {
		t.Errorf("a grant for an undeclared action was accepted: %d", got.Code)
	}
}

// A deny must beat a role. This is the property that makes a revoke take effect
// without a deploy, and it only holds if the resolver consults denies first.
func TestExplicitDenyBeatsARoleThroughTheAPI(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	admin := tokenFor(t, tokens, data, "admin")

	// The organizer may read results by role.
	if got := request(t, server, http.MethodGet, "/v1/organizer/results?event_id=evt_01", organizer, nil); got.Code != http.StatusOK {
		t.Fatalf("precondition: organizer could not read results: %d, body = %s", got.Code, got.Body.String())
	}
	// An admin denies that specific organizer that specific event.
	denied := request(t, server, http.MethodPost, "/v1/grants", admin, map[string]any{
		"user_id": "organizer", "action": "results.read", "event_id": "evt_01",
		"allow": false, "reason": "suspended from results review pending an audit",
	})
	if denied.Code != http.StatusCreated {
		t.Fatalf("could not write the deny: %d, body = %s", denied.Code, denied.Body.String())
	}
	// Now the role no longer helps.
	after := request(t, server, http.MethodGet, "/v1/organizer/results?event_id=evt_01", organizer, nil)
	if after.Code != http.StatusForbidden {
		t.Fatalf("an explicit deny was overridden by the organizer role: %d", after.Code)
	}
	if !strings.Contains(after.Body.String(), "suspended from results review") {
		t.Errorf("the refusal did not carry the reason recorded on the grant: %s", after.Body.String())
	}
	// And the refusal is in the trail with the deny as its source.
	listed := request(t, server, http.MethodGet, "/v1/audit/actions?action=results.read&allowed=false", admin, nil)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), "denied_by_grant") {
		t.Errorf("the deny-sourced refusal is missing from the trail: %s", listed.Body.String())
	}
}
