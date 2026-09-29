package store

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// IdempotencyRecord is the remembered outcome of a request that carried a key.
//
// The body is kept rather than a reference to the resource it created, because
// the point of the record is to answer a retry with exactly what the first
// attempt answered. Storing the row and re-reading it would not do that: the
// resource may have changed since, and a retry that got back different content
// would be indistinguishable, to the caller, from a second write.
type IdempotencyRecord struct {
	Key         string
	Fingerprint string
	Status      int
	Body        []byte
	ContentType string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

// InFlight reports whether a request with this key is still being handled, which
// is the window where a retry is concurrent rather than sequential.
type IdempotencyOutcome int

const (
	// IdempotencyNew means no record exists and none was claimed.
	IdempotencyNew IdempotencyOutcome = iota
	// IdempotencyReplay means a completed record was found and can be served.
	IdempotencyReplay
	// IdempotencyInFlight means another attempt with this key is running.
	IdempotencyInFlight
	// IdempotencyConflict means the key was used for a different request.
	IdempotencyConflict
)

type idempotencyEntry struct {
	record   IdempotencyRecord
	inFlight bool
	// done is closed when the in-flight attempt finishes, so a concurrent retry
	// can wait for the answer instead of being told to come back later.
	done chan struct{}
}

type idempotency struct {
	mu      sync.Mutex
	entries map[string]*idempotencyEntry
	ttl     time.Duration
}

const defaultIdempotencyTTL = 24 * time.Hour

func (s *Store) initIdempotency() {
	s.idempotency = idempotency{entries: make(map[string]*idempotencyEntry), ttl: defaultIdempotencyTTL}
}

// idempotencyScope is the per-actor namespace a key lives in.
//
// Keys are scoped by actor rather than global. A key is chosen by the client, so
// a globally shared namespace would let one caller read another's cached
// response, and a collision between two unrelated clients would turn into a
// spurious conflict.
func idempotencyScope(actorID, key string) string {
	sum := sha256.Sum256([]byte(actorID + "\x00" + key))
	return hex.EncodeToString(sum[:])
}

// FingerprintRequest identifies the request a key was used for.
//
// Method, path and body are all included. Without the body, reusing a key
// against a different payload would silently return the first payload's
// response, which is the worst possible outcome: the caller believes their
// second, different request succeeded.
func FingerprintRequest(method, path string, body []byte) string {
	// A hash.Hash rather than a [32]byte, because the parts are written
	// incrementally. The NUL separators keep the parts unambiguous: without them
	// ("POST", "/ab") and ("POSTA", "/b") would fingerprint identically.
	digest := sha256.New()
	_, _ = digest.Write([]byte(method))
	_, _ = digest.Write([]byte("\x00"))
	_, _ = digest.Write([]byte(path))
	_, _ = digest.Write([]byte("\x00"))
	_, _ = digest.Write(body)
	return hex.EncodeToString(digest.Sum(nil))
}

// BeginIdempotent claims a key, or reports why it cannot.
//
// The three failure modes are kept distinct on purpose. Replay is the good case.
// In-flight is genuinely different from a replay, because the caller can wait for
// the answer. Conflict is a client bug and must be reported as one, rather than
// being smoothed over by returning the first response.
func (s *Store) BeginIdempotent(actorID, key, fingerprint string) (IdempotencyOutcome, IdempotencyRecord, <-chan struct{}) {
	if key == "" {
		return IdempotencyNew, IdempotencyRecord{}, nil
	}
	scope := idempotencyScope(actorID, key)
	now := time.Now().UTC()

	s.idempotency.mu.Lock()
	defer s.idempotency.mu.Unlock()
	s.expireIdempotencyLocked(now)

	entry, ok := s.idempotency.entries[scope]
	if !ok {
		s.idempotency.entries[scope] = &idempotencyEntry{
			record:   IdempotencyRecord{Key: key, Fingerprint: fingerprint, CreatedAt: now},
			inFlight: true,
			done:     make(chan struct{}),
		}
		return IdempotencyNew, IdempotencyRecord{}, nil
	}
	if entry.record.Fingerprint != fingerprint {
		return IdempotencyConflict, entry.record, nil
	}
	if entry.inFlight {
		return IdempotencyInFlight, entry.record, entry.done
	}
	return IdempotencyReplay, entry.record, nil
}

// CompleteIdempotent records the outcome and releases any waiting retries.
func (s *Store) CompleteIdempotent(actorID, key, fingerprint string, status int, body []byte, contentType string) {
	if key == "" {
		return
	}
	scope := idempotencyScope(actorID, key)
	now := time.Now().UTC()

	s.idempotency.mu.Lock()
	defer s.idempotency.mu.Unlock()
	entry, ok := s.idempotency.entries[scope]
	if !ok {
		entry = &idempotencyEntry{done: make(chan struct{})}
		s.idempotency.entries[scope] = entry
	}
	entry.record = IdempotencyRecord{
		Key: key, Fingerprint: fingerprint, Status: status,
		Body: append([]byte(nil), body...), ContentType: contentType,
		CreatedAt: now, ExpiresAt: now.Add(s.idempotency.ttl),
	}
	entry.inFlight = false
	if entry.done != nil {
		close(entry.done)
		entry.done = nil
	}
}

// AbandonIdempotent releases a key whose request failed before producing a
// response, so a corrected retry is not permanently blocked by a claim that will
// never complete.
//
// A 5xx is deliberately not remembered: it may have partially applied, and
// replaying a recorded failure would hide that from the caller. Releasing the
// claim lets the client retry and find out.
func (s *Store) AbandonIdempotent(actorID, key string) {
	if key == "" {
		return
	}
	scope := idempotencyScope(actorID, key)

	s.idempotency.mu.Lock()
	defer s.idempotency.mu.Unlock()
	entry, ok := s.idempotency.entries[scope]
	if !ok || !entry.inFlight {
		return
	}
	if entry.done != nil {
		close(entry.done)
		entry.done = nil
	}
	delete(s.idempotency.entries, scope)
}

func (s *Store) expireIdempotencyLocked(now time.Time) {
	for scope, entry := range s.idempotency.entries {
		if entry.inFlight {
			continue
		}
		if entry.record.ExpiresAt.IsZero() || now.After(entry.record.ExpiresAt) {
			delete(s.idempotency.entries, scope)
		}
	}
}

// IdempotencyStatus is a summary for a handler that wants to report on the
// mechanism itself.
type IdempotencyStatus struct {
	Tracked int
	Pending int
}

// IdempotencyStats reports the size of the table, so an unbounded map is visible
// rather than inferred.
func (s *Store) IdempotencyStats() IdempotencyStatus {
	s.idempotency.mu.Lock()
	defer s.idempotency.mu.Unlock()
	s.expireIdempotencyLocked(time.Now().UTC())
	status := IdempotencyStatus{}
	for _, entry := range s.idempotency.entries {
		if entry.inFlight {
			status.Pending++
		} else {
			status.Tracked++
		}
	}
	return status
}
