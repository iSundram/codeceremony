package store

import (
	"crypto/sha256"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

// New builds a store with an action-audit chain keyed by auditSecret.
//
// The key is derived rather than the secret being used directly, so the audit
// signing key is a separate value from the session secret: a leaked session
// secret must not also let an attacker forge history.
//
// The genesis value is the empty string, so the first entry's PrevHash is empty.
// That is the single place a chain can be checked for having had its beginning
// cut off, which is why retention drops entries from the front without
// renumbering the survivors.
func (s *Store) initActionAudit(auditSecret string) {
	key := sha256.Sum256([]byte("codeceremony.action-audit.v1\x00" + auditSecret))
	s.actionAudit = actionAudit{
		entries:    make([]domain.ActionAuditEntry, 0, 256),
		key:        key[:],
		maxEntries: defaultAuditMaxEntries,
	}
}
