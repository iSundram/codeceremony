package store

import (
	"sort"
	"strings"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/authz"
	"github.com/iSundram/codeceremony/backend/internal/domain"
)

// Grants are the explicit allow and deny rules layered on top of roles.
//
// They exist because roles cannot express the three cases that actually come up
// in an event: one judge also covering as a liaison, one organizer suspended
// from a single event, one account with a time-boxed export permission. An
// explicit deny is the important half. Roles are granted in code and changed in
// deploys; a deny must be revocable in seconds by whoever is on duty.

// GrantsFor implements authz.GrantSource.
func (s *Store) GrantsFor(userID string) []authz.Grant {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.grantsForLocked(userID, time.Now().UTC())
}

func (s *Store) grantsForLocked(userID string, now time.Time) []authz.Grant {
	out := make([]authz.Grant, 0, len(s.grants))
	for _, grant := range s.grants {
		if grant.UserID != userID {
			continue
		}
		// Expired grants are filtered on read as well as on write, so a stale
		// grant cannot be revived by restoring an old snapshot.
		if grant.ExpiresAt != nil && !now.Before(*grant.ExpiresAt) {
			continue
		}
		out = append(out, grant)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// GrantFilter narrows a grant listing.
type GrantFilter struct {
	UserID  string
	EventID string
	Action  authz.Action
	// IncludeExpired keeps grants whose expiry has passed. They are excluded by
	// default because a listing is usually an operator asking "what is in force".
	IncludeExpired bool
}

// ListGrants returns grants matching a filter, newest first.
func (s *Store) ListGrants(filter GrantFilter) []authz.Grant {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now().UTC()
	out := make([]authz.Grant, 0, len(s.grants))
	for _, grant := range s.grants {
		if filter.UserID != "" && grant.UserID != filter.UserID {
			continue
		}
		if filter.EventID != "" && grant.EventID != filter.EventID {
			continue
		}
		if filter.Action != "" && grant.Action != filter.Action {
			continue
		}
		if !filter.IncludeExpired && grant.ExpiresAt != nil && !now.Before(*grant.ExpiresAt) {
			continue
		}
		out = append(out, grant)
	}
	// Newest first, so an operator reviewing a list sees the most recent change
	// at the top rather than hunting for it.
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// CreateGrant records a grant after validating it and rejecting overlaps.
//
// A deny that already covers the same scope is not duplicated: replacing it
// silently would lose the reason the operator gave. An allow is replaced, since
// re-granting the same scope is a normal revocation-then-grant flow.
func (s *Store) CreateGrant(grant authz.Grant) (authz.Grant, error) {
	if err := grant.Validate(); err != nil {
		return authz.Grant{}, err
	}
	if grant.ID == "" {
		grant.ID = domain.NewID("grt")
	}
	if grant.CreatedAt.IsZero() {
		grant.CreatedAt = time.Now().UTC()
	}
	// A grant for an event that does not exist would never fire and would look
	// like it was working.
	if grant.EventID != "" {
		if _, err := s.EventByID(grant.EventID); err != nil {
			return authz.Grant{}, domain.ErrNotFound
		}
	}
	if _, err := s.UserByID(grant.UserID); err != nil {
		return authz.Grant{}, domain.ErrNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for id, existing := range s.grants {
		if !existing.Allow || !grant.Allow {
			continue
		}
		if existing.UserID == grant.UserID && existing.Action == grant.Action &&
			existing.EventID == grant.EventID && existing.ObjectID == grant.ObjectID {
			delete(s.grants, id)
		}
	}
	s.grants[grant.ID] = grant
	return grant, nil
}

// RevokeGrant removes a grant by id.
func (s *Store) RevokeGrant(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.grants[id]; !ok {
		return domain.ErrNotFound
	}
	delete(s.grants, id)
	return nil
}

// GrantByID returns one grant.
func (s *Store) GrantByID(id string) (authz.Grant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	grant, ok := s.grants[id]
	if !ok {
		return authz.Grant{}, domain.ErrNotFound
	}
	return grant, nil
}

// ExpiredGrants returns grants past their expiry, so a purge job can clear them
// rather than letting the table grow forever.
func (s *Store) ExpiredGrants(now time.Time) []authz.Grant {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]authz.Grant, 0)
	for _, grant := range s.grants {
		if grant.ExpiresAt != nil && !now.Before(*grant.ExpiresAt) {
			out = append(out, grant)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// EventRoleFor returns a user's per-event role, and whether they hold one.
//
// This is what makes two organizers of different events two separate people. The
// caller passes it into the authz.Principal, and the resolver intersects the
// event role with the global role rather than replacing it.
func (s *Store) EventRoleFor(eventID, userID string) (domain.EventRole, bool) {
	if eventID == "" || userID == "" {
		return "", false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	member, ok := s.staff[staffKey(eventID, userID)]
	if !ok || !member.Role.Valid() {
		return "", false
	}
	return member.Role, true
}

// IsEventStaff reports whether a user holds any role on an event.
func (s *Store) IsEventStaff(eventID, userID string) bool {
	_, ok := s.EventRoleFor(eventID, userID)
	return ok
}

// EventOwners returns the user ids holding the owner role on an event, used for
// dual control on destructive actions such as deleting or transferring an event.
func (s *Store) EventOwners(eventID string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	owners := make([]string, 0, 2)
	for _, member := range s.staff {
		if member.EventID == eventID && member.Role == domain.EventRoleOwner {
			owners = append(owners, member.UserID)
		}
	}
	sort.Strings(owners)
	return owners
}

// NormalizeGrantEvent is a small helper for handlers that accept a slug or an id
// in a body field. An empty value is allowed and means a platform-wide grant.
func NormalizeGrantEvent(value string) string { return strings.TrimSpace(value) }
