package store

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

// The action audit log is a hash-chained append-only list. It has its own mutex
// rather than sharing the store's, because sealing an entry reads the previous
// entry's hash, and holding the main store lock across a read-modify-write for
// every request would serialise all traffic behind the audit path.
type actionAudit struct {
	mu      sync.Mutex
	entries []domain.ActionAuditEntry
	head    string
	seq     int64
	key     []byte
	// maxEntries bounds the log. An unbounded audit trail on a laptop is a disk
	// exhaustion bug waiting for the event that nobody prunes for.
	maxEntries int
	dropped    int
}

// auditConfig configures the action log.
type auditConfig struct {
	key        []byte
	maxEntries int
}

const defaultAuditMaxEntries = 200_000

// appendActionAudit seals an entry onto the chain and stores it.
func (s *Store) appendActionAudit(entry domain.ActionAuditEntry) domain.ActionAuditEntry {
	s.actionAudit.mu.Lock()
	defer s.actionAudit.mu.Unlock()

	s.actionAudit.seq++
	entry.Seq = s.actionAudit.seq
	entry.Seal(s.actionAudit.head, s.actionAudit.key)
	s.actionAudit.head = entry.Hash
	s.actionAudit.entries = append(s.actionAudit.entries, entry)

	if len(s.actionAudit.entries) > s.actionAudit.maxEntries {
		// Drop the oldest. The chain deliberately does not renumber: seq keeps
		// counting so a gap at the head is visible as a retention boundary
		// rather than being silently renumbered away.
		overflow := len(s.actionAudit.entries) - s.actionAudit.maxEntries
		s.actionAudit.entries = s.actionAudit.entries[overflow:]
		s.actionAudit.dropped += overflow
	}
	return entry
}

// ActionAuditFilter narrows an audit listing.
type ActionAuditFilter struct {
	ActorID   string
	EventID   string
	Action    string
	TargetID  string
	Allowed   *bool
	RequestID string
	Since     time.Time
	Until     time.Time
	Limit     int
	// BeforeSeq pages backwards from a sequence number, which is the only stable
	// cursor for an append-only log: an offset would shift under concurrent
	// writes.
	BeforeSeq int64
}

// ListActionAudit returns entries newest first, with an optional cursor.
func (s *Store) ListActionAudit(filter ActionAuditFilter) []domain.ActionAuditEntry {
	s.actionAudit.mu.Lock()
	defer s.actionAudit.mu.Unlock()

	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	out := make([]domain.ActionAuditEntry, 0, limit)
	// Walk backwards so the newest entries are found first and a full log does
	// not have to be materialised to answer a small page.
	for i := len(s.actionAudit.entries) - 1; i >= 0 && len(out) < limit; i-- {
		entry := s.actionAudit.entries[i]
		if filter.BeforeSeq > 0 && entry.Seq >= filter.BeforeSeq {
			continue
		}
		if !matchActionAudit(entry, filter) {
			continue
		}
		out = append(out, entry)
	}
	// The caller asked for newest first.
	return out
}

func matchActionAudit(entry domain.ActionAuditEntry, filter ActionAuditFilter) bool {
	if filter.ActorID != "" && entry.ActorID != filter.ActorID {
		return false
	}
	if filter.EventID != "" && entry.EventID != filter.EventID {
		return false
	}
	if filter.Action != "" && entry.Action != filter.Action {
		return false
	}
	if filter.TargetID != "" && entry.TargetID != filter.TargetID {
		return false
	}
	if filter.RequestID != "" && entry.RequestID != filter.RequestID {
		return false
	}
	if filter.Allowed != nil && entry.Allowed != *filter.Allowed {
		return false
	}
	if !filter.Since.IsZero() && entry.CreatedAt.Before(filter.Since) {
		return false
	}
	if !filter.Until.IsZero() && entry.CreatedAt.After(filter.Until) {
		return false
	}
	return true
}

// VerifyActionAudit walks the whole chain and reports the first break.
func (s *Store) VerifyActionAudit() domain.AuditChainResult {
	s.actionAudit.mu.Lock()
	defer s.actionAudit.mu.Unlock()

	entries := make([]domain.ActionAuditEntry, len(s.actionAudit.entries))
	copy(entries, s.actionAudit.entries)
	// After retention trimming the oldest retained entry no longer chains to
	// genesis, so verifying the whole thing against the genesis hash would fail
	// on correct data. The suffix verifier checks the retained window against
	// itself and confirms the operator's dropped count, which is the strongest
	// claim the retained data can support.
	return domain.VerifyAuditSuffix(entries, s.actionAudit.key, s.actionAudit.dropped)
}

// ActionAuditHead is the hash of the newest entry.
func (s *Store) ActionAuditHead() string {
	s.actionAudit.mu.Lock()
	defer s.actionAudit.mu.Unlock()
	return s.actionAudit.head
}

// ActionAuditCount is how many entries are retained.
func (s *Store) ActionAuditCount() int {
	s.actionAudit.mu.Lock()
	defer s.actionAudit.mu.Unlock()
	return len(s.actionAudit.entries)
}

// RecordAction writes an entry describing an attempted action.
func (s *Store) RecordAction(entry domain.ActionAuditEntry) domain.ActionAuditEntry {
	return s.appendActionAudit(entry)
}

// ExportActionAudit returns entries in chain order for export and verification
// by a third party.
func (s *Store) ExportActionAudit(filter ActionAuditFilter) []domain.ActionAuditEntry {
	entries := s.ListActionAudit(ActionAuditFilter{
		ActorID: filter.ActorID, EventID: filter.EventID, Action: filter.Action,
		TargetID: filter.TargetID, Allowed: filter.Allowed, RequestID: filter.RequestID,
		Since: filter.Since, Until: filter.Until, Limit: 500,
	})
	// ListActionAudit returns newest first; an export is more useful oldest
	// first, because that is the order a verifier can walk the chain in.
	sort.Slice(entries, func(i, j int) bool { return entries[i].Seq < entries[j].Seq })
	return entries
}

// parsePositiveInt parses a positive integer, returning 0 for anything else so
// a bad query value degrades to the default rather than erroring a read.
func parsePositiveInt(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 1 {
		return 0
	}
	return parsed
}

// ParseActionFilter builds a filter from query values, ignoring blanks so an
// empty query string means "everything" rather than "nothing".
func ParseActionFilter(values map[string][]string) ActionAuditFilter {
	pick := func(key string) string {
		if list, ok := values[key]; ok && len(list) > 0 {
			return strings.TrimSpace(list[0])
		}
		return ""
	}
	filter := ActionAuditFilter{
		ActorID:   pick("actor_id"),
		EventID:   pick("event_id"),
		Action:    pick("action"),
		TargetID:  pick("target_id"),
		RequestID: pick("request_id"),
		Limit:     100,
	}
	if raw := pick("allowed"); raw != "" {
		switch strings.ToLower(raw) {
		case "true", "1", "yes":
			value := true
			filter.Allowed = &value
		case "false", "0", "no":
			value := false
			filter.Allowed = &value
		}
	}
	if raw := pick("limit"); raw != "" {
		if parsed := parsePositiveInt(raw); parsed > 0 {
			filter.Limit = parsed
		}
	}
	if raw := pick("before_seq"); raw != "" {
		if parsed := parsePositiveInt(raw); parsed > 0 {
			filter.BeforeSeq = int64(parsed)
		}
	}
	return filter
}
