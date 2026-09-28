package store

import (
	"sort"
	"strings"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

func (s *Store) RecordActivity(entry domain.ActivityEntry) (domain.ActivityEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry.ID == "" {
		entry.ID = domain.NewID("act")
	}
	if entry.ActorID != "" {
		if actor, ok := s.users[entry.ActorID]; ok {
			entry.ActorName = actor.DisplayName
			entry.ActorRole = actor.Role
		}
	}
	if err := entry.Validate(); err != nil {
		return domain.ActivityEntry{}, err
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	s.activity[entry.ID] = entry
	return entry, nil
}

func (s *Store) ListActivity(filter domain.ActivityFilter) (domain.ActivityPage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	byCategory := map[string]int{}
	byVisibility := map[string]int{}
	matched := make([]domain.ActivityEntry, 0, len(s.activity))
	for _, entry := range s.activity {
		if filter.EventID != "" && entry.EventID != filter.EventID {
			continue
		}
		if filter.Category != "" && string(entry.Category) != filter.Category {
			continue
		}
		if filter.Action != "" && entry.Action != filter.Action {
			continue
		}
		if filter.ActorID != "" && entry.ActorID != filter.ActorID {
			continue
		}
		if filter.TargetType != "" && entry.TargetType != filter.TargetType {
			continue
		}
		if filter.Visibility != "" && string(entry.Visibility) != filter.Visibility {
			continue
		}
		byCategory[string(entry.Category)]++
		byVisibility[string(entry.Visibility)]++
		matched = append(matched, entry)
	}
	domain.SortActivity(matched)
	total := len(matched)
	limit := filter.Limit
	if limit <= 0 {
		limit = 25
	}
	if limit > 200 {
		limit = 200
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := domain.ActivityPage{
		Entries:      matched[offset:end],
		Count:        end - offset,
		Total:        total,
		Limit:        limit,
		Offset:       offset,
		ByCategory:   byCategory,
		ByVisibility: byVisibility,
	}
	if filter.RollupCount {
		page.Entries = nil
		page.Count = 0
	}
	return page, nil
}

func (s *Store) ActivityCategories(eventID string) map[string]int {
	page, _ := s.ListActivity(domain.ActivityFilter{EventID: eventID})
	return page.ByCategory
}

func (s *Store) AddEventStaff(staff domain.EventStaff) (domain.EventStaff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := staff.Validate(); err != nil {
		return domain.EventStaff{}, err
	}
	if _, ok := s.events[staff.EventID]; !ok {
		return domain.EventStaff{}, domain.ErrNotFound
	}
	user, ok := s.users[staff.UserID]
	if !ok {
		return domain.EventStaff{}, domain.ErrNotFound
	}
	if user.Role != domain.RoleOrganizer && user.Role != domain.RoleAdmin && staff.Role != domain.EventRoleOwner {
		return domain.EventStaff{}, domain.ErrValidation
	}
	if existing, ok := s.staff[staffKey(staff.EventID, staff.UserID)]; ok && existing.Role.Rank() >= staff.Role.Rank() {
		return domain.EventStaff{}, domain.ErrAlreadyExists
	}
	if existing, ok := s.staff[staffKey(staff.EventID, staff.UserID)]; ok && staff.Title != "" {
		staff.CreatedAt = existing.CreatedAt
	}
	staff.CreatedAt = time.Now().UTC()
	s.staff[staffKey(staff.EventID, staff.UserID)] = staff
	return staff, nil
}

func (s *Store) RemoveEventStaff(eventID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.staff[staffKey(eventID, userID)]; !ok {
		return domain.ErrNotFound
	}
	delete(s.staff, staffKey(eventID, userID))
	return nil
}

func (s *Store) EventStaffMembers(eventID string) []domain.EventStaff {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.eventStaffLocked(eventID)
}

func (s *Store) EventStaffRole(eventID, userID string) (domain.EventRole, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	staff, ok := s.staff[staffKey(eventID, userID)]
	if !ok {
		return "", domain.ErrNotFound
	}
	return staff.Role, nil
}

func (s *Store) eventStaffLocked(eventID string) []domain.EventStaff {
	staff := make([]domain.EventStaff, 0)
	for _, entry := range s.staff {
		if entry.EventID == eventID {
			staff = append(staff, entry)
		}
	}
	sort.Slice(staff, func(i, j int) bool {
		if staff[i].Role.Rank() == staff[j].Role.Rank() {
			return staff[i].UserID < staff[j].UserID
		}
		return staff[i].Role.Rank() > staff[j].Role.Rank()
	})
	return staff
}

func staffKey(eventID, userID string) string {
	return eventID + ":" + userID
}

func (s *Store) MailPreferences(userID string) domain.MailPreferences {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if preferences, ok := s.mailPreferences[userID]; ok {
		return preferences
	}
	return domain.DefaultMailPreferences(userID)
}

func (s *Store) SaveMailPreferences(preferences domain.MailPreferences) (domain.MailPreferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[preferences.UserID]; !ok {
		return domain.MailPreferences{}, domain.ErrNotFound
	}
	if !preferences.Transactional && (preferences.AccountSecurity || preferences.AccountLifecycle) {
		return domain.MailPreferences{}, domain.ErrValidation
	}
	preferences.UpdatedAt = time.Now().UTC()
	s.mailPreferences[preferences.UserID] = preferences
	return preferences, nil
}

func (s *Store) UnsubscribeAll(userID string) (domain.MailPreferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	preferences, ok := s.mailPreferences[userID]
	if !ok {
		preferences = domain.DefaultMailPreferences(userID)
	}
	now := time.Now().UTC()
	preferences.UnsubscribedAll = true
	preferences.UnsubscribedAt = &now
	preferences.Marketing = false
	preferences.WeeklyDigest = false
	preferences.UpdatedAt = now
	s.mailPreferences[userID] = preferences
	return preferences, nil
}

func (s *Store) QueueMail(message domain.MailMessage) (domain.MailMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := message.Validate(); err != nil {
		return domain.MailMessage{}, err
	}
	if message.DedupeKey != "" {
		if existing, ok := s.mailByDedupe[message.DedupeKey]; ok {
			return existing, nil
		}
	}
	now := time.Now().UTC()
	if message.ID == "" {
		message.ID = domain.NewID("mail")
	}
	if message.Status == "" {
		message.Status = domain.MailQueued
	}
	if message.ScheduledAt.IsZero() {
		message.ScheduledAt = now
	}
	message.CreatedAt = now
	message.UpdatedAt = now
	s.mail[message.ID] = message
	if message.DedupeKey != "" {
		s.mailByDedupe[message.DedupeKey] = message
	}
	return message, nil
}

func (s *Store) ClaimMail(now time.Time, limit int) []domain.MailMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	claimed := make([]domain.MailMessage, 0, limit)
	for id, message := range s.mail {
		if len(claimed) >= limit {
			break
		}
		if message.Status != domain.MailQueued || message.ScheduledAt.After(now) {
			continue
		}
		message.Status = domain.MailSending
		message.Attempts++
		message.UpdatedAt = now
		s.mail[id] = message
		claimed = append(claimed, message)
	}
	sort.Slice(claimed, func(i, j int) bool { return claimed[i].ScheduledAt.Before(claimed[j].ScheduledAt) })
	return claimed
}

func (s *Store) MarkMailSent(id string, sentAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	message, ok := s.mail[id]
	if !ok {
		return domain.ErrNotFound
	}
	message.Status = domain.MailSent
	message.SentAt = &sentAt
	message.LastError = ""
	message.UpdatedAt = sentAt
	s.mail[id] = message
	return nil
}

func (s *Store) MarkMailFailed(id string, reason string, retryAt time.Time, giveUp bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	message, ok := s.mail[id]
	if !ok {
		return domain.ErrNotFound
	}
	message.LastError = strings.TrimSpace(reason)
	message.UpdatedAt = time.Now().UTC()
	if giveUp {
		message.Status = domain.MailFailed
	} else {
		message.Status = domain.MailQueued
		message.ScheduledAt = retryAt
	}
	s.mail[id] = message
	return nil
}

func (s *Store) MarkMailSkipped(id, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	message, ok := s.mail[id]
	if !ok {
		return domain.ErrNotFound
	}
	message.Status = domain.MailSkipped
	message.LastError = strings.TrimSpace(reason)
	message.UpdatedAt = time.Now().UTC()
	s.mail[id] = message
	return nil
}

func (s *Store) MailByID(id string) (domain.MailMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	message, ok := s.mail[id]
	if !ok {
		return domain.MailMessage{}, domain.ErrNotFound
	}
	return message, nil
}

func (s *Store) ListMail(status domain.MailStatus, limit int) []domain.MailMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.MailMessage, 0, len(s.mail))
	for _, message := range s.mail {
		if status != "" && message.Status != status {
			continue
		}
		result = append(result, message)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result
}

func (s *Store) MailStats() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stats := map[string]int{}
	for _, message := range s.mail {
		stats[string(message.Status)]++
		stats["total"]++
	}
	return stats
}

func (s *Store) SaveUnsubscribeToken(token domain.UnsubscribeToken) (domain.UnsubscribeToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(token.Token) == "" || strings.TrimSpace(token.UserID) == "" {
		return domain.UnsubscribeToken{}, domain.ErrValidation
	}
	now := time.Now().UTC()
	if token.CreatedAt.IsZero() {
		token.CreatedAt = now
	}
	if token.ExpiresAt.IsZero() {
		token.ExpiresAt = now.Add(90 * 24 * time.Hour)
	}
	s.unsubscribe[token.Token] = token
	return token, nil
}

func (s *Store) UnsubscribeTokenByValue(value string) (domain.UnsubscribeToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	token, ok := s.unsubscribe[value]
	if !ok {
		return domain.UnsubscribeToken{}, domain.ErrNotFound
	}
	return token, nil
}

func (s *Store) DeleteUnsubscribeToken(value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.unsubscribe[value]; !ok {
		return domain.ErrNotFound
	}
	delete(s.unsubscribe, value)
	return nil
}
