package store

import (
	"net/http"
	"testing"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/seed"
)

// The store's own invariants, tested directly so a failure points at the
// mechanism rather than at whichever handler happened to exercise it.
func TestIdempotencyStoreOutcomes(t *testing.T) {
	portal := New(seed.Data{})
	actor := "usr_participant"
	const key = "store-key"
	print := FingerprintRequest("POST", "/v1/x", nil, "")

	if outcome, _, _ := portal.BeginIdempotent(actor, key, print); outcome != IdempotencyNew {
		t.Fatalf("a fresh key reported outcome %d, want IdempotencyNew", outcome)
	}
	// A second claim while the first is in flight is not a replay.
	if outcome, _, _ := portal.BeginIdempotent(actor, key, print); outcome != IdempotencyInFlight {
		t.Errorf("a concurrent claim reported outcome %d, want IdempotencyInFlight", outcome)
	}
	// A different request under the same key is a conflict, not a replay.
	if outcome, _, _ := portal.BeginIdempotent(actor, key, FingerprintRequest("POST", "/v1/y", nil, "")); outcome != IdempotencyConflict {
		t.Errorf("a changed request reported outcome %d, want IdempotencyConflict", outcome)
	}

	portal.CompleteIdempotent(actor, key, print, http.StatusCreated, []byte(`{"ok":true}`), "application/json")
	if outcome, record, _ := portal.BeginIdempotent(actor, key, print); outcome != IdempotencyReplay {
		t.Errorf("a completed key reported outcome %d, want IdempotencyReplay", outcome)
	} else if record.Status != http.StatusCreated || string(record.Body) != `{"ok":true}` {
		t.Errorf("the replayed record is %d %q, want the original response", record.Status, record.Body)
	}

	// Abandoning a claim must not leave the key permanently unusable.
	portal.AbandonIdempotent(actor, "abandoned-key")
	portal.BeginIdempotent(actor, "abandoned-key", print)
	portal.AbandonIdempotent(actor, "abandoned-key")
	if outcome, _, _ := portal.BeginIdempotent(actor, "abandoned-key", print); outcome != IdempotencyNew {
		t.Errorf("an abandoned key reported outcome %d, want IdempotencyNew", outcome)
	}
}

// Records have to expire, or the table grows without bound and a key is honoured
// forever.
func TestIdempotencyRecordsExpire(t *testing.T) {
	portal := New(seed.Data{})
	actor := "usr_participant"
	print := FingerprintRequest("POST", "/v1/x", nil, "")

	portal.CompleteIdempotent(actor, "aging", print, http.StatusOK, []byte("{}"), "application/json")
	if outcome, _, _ := portal.BeginIdempotent(actor, "aging", print); outcome != IdempotencyReplay {
		t.Fatalf("a fresh record reported outcome %d, want IdempotencyReplay", outcome)
	}
	if stats := portal.IdempotencyStats(); stats.Tracked != 1 {
		t.Errorf("tracked = %d, want 1", stats.Tracked)
	}

	// Age the record past its window rather than sleeping through it.
	portal.idempotency.mu.Lock()
	for _, entry := range portal.idempotency.entries {
		entry.record.ExpiresAt = time.Now().UTC().Add(-time.Minute)
	}
	portal.idempotency.mu.Unlock()

	if outcome, _, _ := portal.BeginIdempotent(actor, "aging", print); outcome != IdempotencyNew {
		t.Errorf("an expired key reported outcome %d, want IdempotencyNew", outcome)
	}
	if stats := portal.IdempotencyStats(); stats.Tracked != 0 {
		t.Errorf("tracked = %d after expiry, want 0", stats.Tracked)
	}
}
