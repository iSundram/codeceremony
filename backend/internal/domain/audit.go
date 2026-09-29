package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// ActionAuditEntry is one record of an attempted action.
//
// The important design decision is that entries are written for attempts, not
// just successes. A trail that only records what succeeded is a list of things
// that were allowed, and it is blind to the case that actually matters: someone
// probing what they can reach. Every refusal is the cheapest detection signal the
// platform produces, so a refusal is recorded with the same care as an approval.
//
// Each entry is chained to the previous one by hash. Without that, an attacker
// with write access to the data file can remove a line from the history and
// nothing downstream can tell. With it, any edit breaks the chain from that point
// on, and VerifyAuditChain says where.
type ActionAuditEntry struct {
	ID        string    `json:"id"`
	Seq       int64     `json:"seq"`
	CreatedAt time.Time `json:"created_at"`

	// Who acted. ActorID is empty for an unauthenticated attempt, which is
	// itself a meaningful record.
	ActorID    string `json:"actor_id,omitempty"`
	ActorRole  string `json:"actor_role,omitempty"`
	ActorEmail string `json:"actor_email,omitempty"`

	// What was attempted.
	Action     string `json:"action"`
	EventID    string `json:"event_id,omitempty"`
	TargetType string `json:"target_type,omitempty"`
	TargetID   string `json:"target_id,omitempty"`
	Method     string `json:"method,omitempty"`
	Path       string `json:"path,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
	RemoteAddr string `json:"remote_addr,omitempty"`
	UserAgent  string `json:"user_agent,omitempty"`

	// The outcome, and why.
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
	Source  string `json:"source,omitempty"`
	Status  int    `json:"status,omitempty"`
	// Assurance records whether the session had passed a second factor.
	Assurance string `json:"assurance,omitempty"`

	// PrevHash and Hash make the chain verifiable.
	PrevHash string `json:"prev_hash"`
	Hash     string `json:"hash"`
}

// AuditChainHead is the hash of the most recent entry, which the next entry
// chains to. The empty string is the chain's own genesis value.
const AuditChainHead = ""

// canonicalAudit is the byte representation that gets hashed.
//
// It is built by hand, field by field, with explicit separators. Marshalling the
// struct would be shorter and wrong: Go's JSON encoder does not promise a stable
// key order across versions, and a hash over an unstable encoding produces a
// chain that fails verification after a toolchain bump.
func canonicalAudit(e ActionAuditEntry) string {
	var b strings.Builder
	add := func(parts ...string) {
		for i, part := range parts {
			if i > 0 {
				b.WriteByte('\x1f')
			}
			b.WriteString(part)
		}
		b.WriteByte('\x1e')
	}
	add(e.ID, strconv.FormatInt(e.Seq, 10), e.CreatedAt.UTC().Format(time.RFC3339Nano))
	add(e.ActorID, e.ActorRole, e.ActorEmail)
	add(e.Action, e.EventID, e.TargetType, e.TargetID)
	add(e.Method, e.Path, e.RequestID, e.RemoteAddr, e.UserAgent)
	add(strconv.FormatBool(e.Allowed), e.Reason, e.Source, strconv.Itoa(e.Status))
	add(e.PrevHash)
	return b.String()
}

// Seal computes the entry's hash and fills it in, chaining to prevHash.
func (e *ActionAuditEntry) Seal(prevHash string, key []byte) {
	e.PrevHash = prevHash
	e.Hash = ""
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(canonicalAudit(*e)))
	e.Hash = hex.EncodeToString(mac.Sum(nil))
}

// Verify recomputes the hash and compares it. It is constant time, because a
// timing-variable comparison on a hash is a comparison an attacker can measure.
func (e ActionAuditEntry) Verify(key []byte) bool {
	copy := e
	copy.Hash = ""
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(canonicalAudit(copy)))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(e.Hash))
}

// AuditChainResult reports whether a chain reconciles, and where it does not.
type AuditChainResult struct {
	Entries int  `json:"entries"`
	Valid   bool `json:"valid"`
	// BrokenAtSeq is the sequence number of the first entry that failed
	// verification or failed to link to its predecessor.
	BrokenAtSeq int64  `json:"broken_at_seq,omitempty"`
	Detail      string `json:"detail,omitempty"`
	Head        string `json:"head"`
	// Dropped counts entries removed by retention, so a verifier knows the
	// chain it holds is a suffix rather than the whole history.
	Dropped int `json:"dropped,omitempty"`
}

// VerifyAuditChain walks a sequence of entries and reports the first break.
//
// Three things are checked, and all three matter: that each entry's own hash
// recomputes, that it chains to the previous entry's hash, and that the sequence
// numbers are contiguous. A gap in the sequence is as much a sign of tampering as
// a changed hash, and is the cheaper tamper to perform.
func VerifyAuditChain(entries []ActionAuditEntry, key []byte) AuditChainResult {
	previous := AuditChainHead
	var lastSeq int64
	for index, entry := range entries {
		if entry.Seq != lastSeq+1 {
			return AuditChainResult{
				Entries:     len(entries),
				Valid:       false,
				BrokenAtSeq: entry.Seq,
				Detail:      "sequence number is not contiguous with the previous entry",
				Head:        previous,
			}
		}
		if entry.PrevHash != previous {
			return AuditChainResult{
				Entries:     len(entries),
				Valid:       false,
				BrokenAtSeq: entry.Seq,
				Detail:      "entry does not chain to the previous entry's hash",
				Head:        previous,
			}
		}
		if !entry.Verify(key) {
			return AuditChainResult{
				Entries:     len(entries),
				Valid:       false,
				BrokenAtSeq: entry.Seq,
				Detail:      "entry hash does not match its contents",
				Head:        previous,
			}
		}
		previous = entry.Hash
		lastSeq = entry.Seq
		_ = index
	}
	return AuditChainResult{Entries: len(entries), Valid: true, Head: previous}
}
