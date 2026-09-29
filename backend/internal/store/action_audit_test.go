package store

import (
	"testing"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/authz"
	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/seed"
)

func auditedStore(t *testing.T, secret string) *Store {
	t.Helper()
	return New(seed.Data{AuditSecret: secret})
}

// grantedStore includes accounts, because a grant is validated against a real
// user at creation: granting an action to someone who does not exist is a typo
// that would otherwise sit in the table until it mattered.
func grantedStore(t *testing.T) (*Store, time.Time) {
	t.Helper()
	created := time.Now().UTC().Add(-48 * time.Hour)
	users := make([]domain.User, 0, 2)
	for _, id := range []string{"org", "admin"} {
		users = append(users, domain.User{
			ID: id, Email: id + "@example.test", DisplayName: id,
			Role: domain.RoleOrganizer, State: domain.AccountActive,
			PasswordHash: "x", CreatedAt: created,
		})
	}
	users[1].Role = domain.RoleAdmin
	// A grant names both a user and an event, and both are checked on creation.
	event := domain.Event{
		ID: "evt_01", Slug: "spring-hack", Name: "Spring Hack",
		Description: "an event", Timezone: "UTC", State: domain.HackathonRegistrationOpen,
		CreatedAt: created,
	}
	return New(seed.Data{AuditSecret: "secret", Users: users, Events: []domain.Event{event}}), created
}

// auditKey exposes the chain key to a test in the same package so that a chain
// can be verified independently of the store that wrote it. That independence is
// the property worth testing: a verifier that asks the store for the answer
// proves nothing.
func auditKey(portal *Store) []byte {
	return portal.actionAudit.key
}

func recordAt(portal *Store, id, action, actor string, allowed bool, minute int) {
	portal.RecordAction(domain.ActionAuditEntry{
		ID:        id,
		CreatedAt: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC).Add(time.Duration(minute) * time.Minute),
		ActorID:   actor,
		ActorRole: "organizer",
		Action:    action,
		EventID:   "evt_01",
		Method:    "GET",
		Path:      "/v1/organizer/results",
		Allowed:   allowed,
		Reason:    "a reason",
		Source:    "role",
		Status:    200,
	})
}

func TestActionAuditChainVerifies(t *testing.T) {
	portal := auditedStore(t, "secret")
	for i := 0; i < 5; i++ {
		recordAt(portal, string(rune('a'+i)), "results.read", "org", i%2 == 0, i)
	}

	result := portal.VerifyActionAudit()
	if !result.Valid {
		t.Fatalf("a freshly written chain did not verify: %+v", result)
	}
	if result.Entries != 5 {
		t.Errorf("chain has %d entries, want 5", result.Entries)
	}
	if result.Head == "" || result.Head != portal.ActionAuditHead() {
		t.Errorf("head %q does not match the stored head %q", result.Head, portal.ActionAuditHead())
	}
	if result.Dropped != 0 {
		t.Errorf("dropped = %d on a fresh chain, want 0", result.Dropped)
	}
	if portal.ActionAuditCount() != 5 {
		t.Errorf("count = %d, want 5", portal.ActionAuditCount())
	}
}

// The exported chain has to verify on its own, or publishing it is pointless.
func TestExportedActionAuditVerifiesIndependently(t *testing.T) {
	portal := auditedStore(t, "secret")
	for i := 0; i < 5; i++ {
		recordAt(portal, string(rune('a'+i)), "results.read", "org", true, i)
	}

	entries := portal.ExportActionAudit(ActionAuditFilter{Limit: 100})
	if len(entries) != 5 {
		t.Fatalf("export returned %d entries, want 5", len(entries))
	}
	// A verifier walks the chain forwards, so the export must be in chain order.
	if entries[0].Seq != 1 || entries[len(entries)-1].Seq != 5 {
		t.Fatalf("export is not in chain order: first %d, last %d", entries[0].Seq, entries[len(entries)-1].Seq)
	}
	if check := domain.VerifyAuditChain(entries, auditKey(portal)); !check.Valid {
		t.Errorf("an exported chain did not verify independently: %+v", check)
	}
}

func TestActionAuditDetectsAnEditedEntry(t *testing.T) {
	portal := auditedStore(t, "secret")
	for i := 0; i < 5; i++ {
		recordAt(portal, string(rune('a'+i)), "results.read", "org", true, i)
	}
	entries := portal.ExportActionAudit(ActionAuditFilter{Limit: 100})

	tampered := append([]domain.ActionAuditEntry(nil), entries...)
	tampered[2].Reason = "edited to look legitimate"

	result := domain.VerifyAuditChain(tampered, auditKey(portal))
	if result.Valid {
		t.Fatal("a chain with an edited entry verified; the content is not protected")
	}
	if result.BrokenAtSeq != tampered[2].Seq {
		t.Errorf("break reported at seq %d, want %d", result.BrokenAtSeq, tampered[2].Seq)
	}
	if result.Detail == "" {
		t.Error("a tamper was reported without an explanation")
	}
}

// Removing an entry is the cheaper tamper and has to fail too.
func TestActionAuditDetectsARemovedEntry(t *testing.T) {
	portal := auditedStore(t, "secret")
	for i := 0; i < 5; i++ {
		recordAt(portal, string(rune('a'+i)), "results.read", "org", true, i)
	}
	entries := portal.ExportActionAudit(ActionAuditFilter{Limit: 100})

	withGap := append(append([]domain.ActionAuditEntry{}, entries[:2]...), entries[3:]...)
	if result := domain.VerifyAuditChain(withGap, auditKey(portal)); result.Valid {
		t.Fatal("a chain with a removed entry verified")
	}
}

// Rewriting an entry and resealing it still fails, because the next entry chains
// to the old hash.
func TestActionAuditDetectsAResealedEntry(t *testing.T) {
	portal := auditedStore(t, "secret")
	for i := 0; i < 5; i++ {
		recordAt(portal, string(rune('a'+i)), "results.read", "org", true, i)
	}
	entries := portal.ExportActionAudit(ActionAuditFilter{Limit: 100})

	forged := append([]domain.ActionAuditEntry(nil), entries...)
	forged[2].Allowed = false
	forged[2].Seal(forged[2].PrevHash, auditKey(portal))

	result := domain.VerifyAuditChain(forged, auditKey(portal))
	if result.Valid {
		t.Fatal("a resealed entry verified; the chain is not actually binding")
	}
	if result.BrokenAtSeq != 4 {
		t.Errorf("break reported at seq %d, want 4 (the entry after the forgery)", result.BrokenAtSeq)
	}
}

func TestActionAuditHashBindsSequenceAndKey(t *testing.T) {
	base := domain.ActionAuditEntry{
		CreatedAt: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
		Action:    "results.read",
		Allowed:   true,
		Reason:    "same",
		Status:    200,
	}
	first, second := base, base
	first.Seq, second.Seq = 1, 2
	key := []byte("test-key")
	first.Seal("", key)
	second.Seal(first.Hash, key)
	if first.Hash == second.Hash {
		t.Fatal("two entries with identical content and sequence numbers hash the same")
	}
	if !first.Verify(key) || !second.Verify(key) {
		t.Error("a correctly sealed entry failed its own verification")
	}
	if first.Verify([]byte("wrong-key")) {
		t.Error("an entry verified under a different key")
	}
}

// Retention trims the log. The retained window must still verify, and the
// dropped count has to be reported rather than the sequence silently renumbered.
func TestActionAuditRetentionKeepsTheSuffixVerifiable(t *testing.T) {
	portal := auditedStore(t, "secret")
	portal.actionAudit.maxEntries = 5
	for i := 0; i < 20; i++ {
		recordAt(portal, domain.NewID("aud"), "results.read", "org", true, i)
	}

	result := portal.VerifyActionAudit()
	if !result.Valid {
		t.Fatalf("a trimmed chain did not verify: %+v", result)
	}
	if result.Entries != 5 {
		t.Errorf("retained %d entries, want 5", result.Entries)
	}
	if result.Dropped != 15 {
		t.Errorf("dropped = %d, want 15", result.Dropped)
	}
	// The retained entries keep their original numbers rather than restarting.
	entries := portal.ExportActionAudit(ActionAuditFilter{Limit: 100})
	if entries[0].Seq != 16 {
		t.Errorf("first retained seq = %d, want 16; renumbering would hide the gap", entries[0].Seq)
	}
}

// An operator must not be able to pass off a truncated log as a complete one.
func TestActionAuditSuffixRejectsAnInflatedDroppedCount(t *testing.T) {
	portal := auditedStore(t, "secret")
	portal.actionAudit.maxEntries = 5
	for i := 0; i < 20; i++ {
		recordAt(portal, domain.NewID("aud"), "results.read", "org", true, i)
	}
	entries := portal.ExportActionAudit(ActionAuditFilter{Limit: 100})

	if result := domain.VerifyAuditSuffix(entries, auditKey(portal), 15); !result.Valid {
		t.Errorf("a genuine suffix failed to verify: %+v", result)
	}
	if result := domain.VerifyAuditSuffix(entries, auditKey(portal), 3); result.Valid {
		t.Error("a suffix verified against a dropped count that contradicts its first sequence number")
	}
}

func TestActionAuditFiltersNarrowTheListing(t *testing.T) {
	portal := auditedStore(t, "secret")
	record := func(actor, action, event string, allowed bool, minute int) {
		portal.RecordAction(domain.ActionAuditEntry{
			ID:        domain.NewID("aud"),
			CreatedAt: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC).Add(time.Duration(minute) * time.Minute),
			ActorID:   actor,
			Action:    action,
			EventID:   event,
			Allowed:   allowed,
			Reason:    "r",
			Status:    200,
		})
	}
	for i := 0; i < 6; i++ {
		record("org", "results.read", "evt_01", true, i)
		record("judge", "review.read_peer", "evt_01", false, i)
	}
	record("admin", "platform.manage", "", true, 1)

	if got := portal.ListActionAudit(ActionAuditFilter{ActorID: "judge"}); len(got) != 6 {
		t.Errorf("actor filter returned %d, want 6", len(got))
	}
	if got := portal.ListActionAudit(ActionAuditFilter{Action: "review.read_peer"}); len(got) != 6 {
		t.Errorf("action filter returned %d, want 6", len(got))
	}
	refused := false
	if got := portal.ListActionAudit(ActionAuditFilter{Allowed: &refused}); len(got) != 6 {
		t.Errorf("denied filter returned %d, want 6", len(got))
	}
	if got := portal.ListActionAudit(ActionAuditFilter{EventID: "evt_01"}); len(got) != 12 {
		t.Errorf("event filter returned %d, want 12", len(got))
	}
	// A platform-wide entry has no event and must not leak into an event's view.
	if got := portal.ListActionAudit(ActionAuditFilter{EventID: "evt_01", Action: "platform.manage"}); len(got) != 0 {
		t.Errorf("a platform-wide entry appeared under an event filter (%d rows)", len(got))
	}
}

func TestActionAuditPagesBackwardsBySequence(t *testing.T) {
	portal := auditedStore(t, "secret")
	for i := 0; i < 12; i++ {
		recordAt(portal, domain.NewID("aud"), "results.read", "org", true, i)
	}

	page := portal.ListActionAudit(ActionAuditFilter{Limit: 5})
	if len(page) != 5 {
		t.Fatalf("page size %d, want 5", len(page))
	}
	for i := 1; i < len(page); i++ {
		if page[i-1].Seq <= page[i].Seq {
			t.Fatalf("listing is not newest first: %d then %d", page[i-1].Seq, page[i].Seq)
		}
	}
	older := portal.ListActionAudit(ActionAuditFilter{Limit: 5, BeforeSeq: page[len(page)-1].Seq})
	if len(older) != 5 {
		t.Fatalf("second page size %d, want 5", len(older))
	}
	for _, entry := range older {
		if entry.Seq >= page[len(page)-1].Seq {
			t.Errorf("paging returned seq %d, want below %d", entry.Seq, page[len(page)-1].Seq)
		}
	}
}

func TestGrantLifecycleAndExpiry(t *testing.T) {
	portal, created := grantedStore(t)
	now := time.Now().UTC()
	// Created two days ago so that both a lapsed and a current expiry sit
	// after it; the store rejects an expiry that precedes creation.
	past, future := now.Add(-time.Hour), now.Add(time.Hour)

	newGrant := func(id, action string, allow bool, expires *time.Time) authz.Grant {
		return authz.Grant{
			ID: id, UserID: "org", Action: authz.Action(action),
			EventID: "evt_01", Allow: allow, Reason: "cover", ExpiresAt: expires,
			GrantedBy: "admin", CreatedAt: created,
		}
	}
	if _, err := portal.CreateGrant(newGrant("g1", "results.read", false, &past)); err != nil {
		t.Fatalf("CreateGrant() error = %v", err)
	}
	if _, err := portal.CreateGrant(newGrant("g2", "results.read", true, &future)); err != nil {
		t.Fatalf("CreateGrant() error = %v", err)
	}

	// An expired grant must not be in force, or an old grant silently outlives
	// the reason it was made.
	live := portal.GrantsFor("org")
	if len(live) != 1 || live[0].ID != "g2" {
		t.Errorf("GrantsFor returned %d grants, want only the live one: %+v", len(live), live)
	}
	if got := portal.ListGrants(GrantFilter{UserID: "org"}); len(got) != 1 {
		t.Errorf("ListGrants returned %d, want only the live grant", len(got))
	}
	if got := portal.ListGrants(GrantFilter{UserID: "org", IncludeExpired: true}); len(got) != 2 {
		t.Errorf("ListGrants with IncludeExpired returned %d, want 2", len(got))
	}
	if got := portal.ExpiredGrants(now); len(got) != 1 || got[0].ID != "g1" {
		t.Errorf("ExpiredGrants returned %+v, want just g1", got)
	}

	if err := portal.RevokeGrant("g2"); err != nil {
		t.Fatalf("RevokeGrant() error = %v", err)
	}
	if got := portal.GrantsFor("org"); len(got) != 0 {
		t.Errorf("a revoked grant is still in force: %+v", got)
	}
	if _, err := portal.GrantByID("g2"); err == nil {
		t.Error("a revoked grant is still readable by id")
	}
}

// The key belongs to the composition root, not to the seed. If seeding could
// blank it, the chain would quietly start signing with the empty string and
// every existing entry would stop verifying.
func TestSeedingNeverBlanksAConfiguredAuditKey(t *testing.T) {
	portal, _ := grantedStore(t)
	before := string(auditKey(portal))
	if before == "" {
		t.Fatal("a configured store has an empty audit key")
	}
	portal.RecordAction(domain.ActionAuditEntry{ID: domain.NewID("aud"), Action: "results.read", Allowed: true, Reason: "ok", Status: 200})
	if result := portal.VerifyActionAudit(); !result.Valid {
		t.Fatalf("chain did not verify before seeding: %+v", result)
	}

	// A seed set carrying no secret, which is what every seed path produced until
	// the composition root started setting the field.
	portal.SeedFrom(seed.Data{AuditSecret: ""})

	if after := string(auditKey(portal)); after != before {
		t.Errorf("audit key changed across seeding: %q then %q", before, after)
	}
	if result := portal.VerifyActionAudit(); !result.Valid {
		t.Errorf("entries written before seeding stopped verifying: %+v", result)
	}
}
