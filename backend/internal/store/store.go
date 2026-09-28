package store

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/seed"
)

type Store struct {
	mu            sync.RWMutex
	users         map[string]domain.User
	usersByMail   map[string]string
	events        map[string]domain.Event
	tracks        map[string]domain.Track
	teams         map[string]domain.Team
	memberships   map[string]domain.TeamMembership
	submissions   map[string]domain.Submission
	assignments   map[string]domain.Assignment
	reviews       map[string]domain.Review
	sessions      map[string]domain.Session
	notifications map[string]domain.Notification
	auditEvents   map[string]domain.AuditEvent
}

func New(data seed.Data) *Store {
	store := &Store{
		users:         make(map[string]domain.User, len(data.Users)),
		usersByMail:   make(map[string]string, len(data.Users)),
		events:        make(map[string]domain.Event, len(data.Events)),
		tracks:        make(map[string]domain.Track, len(data.Tracks)),
		teams:         make(map[string]domain.Team, len(data.Teams)),
		memberships:   make(map[string]domain.TeamMembership, len(data.TeamMemberships)),
		submissions:   make(map[string]domain.Submission, len(data.Submissions)),
		assignments:   make(map[string]domain.Assignment, len(data.Assignments)),
		reviews:       make(map[string]domain.Review, len(data.Reviews)),
		sessions:      make(map[string]domain.Session),
		notifications: make(map[string]domain.Notification, len(data.Notifications)),
		auditEvents:   make(map[string]domain.AuditEvent, len(data.AuditEvents)),
	}
	for _, user := range data.Users {
		store.users[user.ID] = user
		store.usersByMail[strings.ToLower(user.Email)] = user.ID
	}
	for _, event := range data.Events {
		store.events[event.ID] = event
	}
	for _, track := range data.Tracks {
		store.tracks[track.ID] = track
	}
	for _, team := range data.Teams {
		store.teams[team.ID] = team
	}
	for _, membership := range data.TeamMemberships {
		store.memberships[membershipKey(membership.TeamID, membership.UserID)] = membership
	}
	for _, submission := range data.Submissions {
		store.submissions[submission.ID] = submission
	}
	for _, assignment := range data.Assignments {
		store.assignments[assignment.ID] = assignment
	}
	for _, review := range data.Reviews {
		store.reviews[reviewKey(review.JudgeID, review.ProjectID)] = review
	}
	for _, notification := range data.Notifications {
		store.notifications[notification.ID] = notification
	}
	for _, event := range data.AuditEvents {
		store.auditEvents[event.ID] = event
	}
	return store
}

func (s *Store) UserByEmail(email string) (domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.usersByMail[strings.ToLower(strings.TrimSpace(email))]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return s.users[id], nil
}

func (s *Store) UserByID(id string) (domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.users[id]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return user, nil
}

func (s *Store) ListUsers() []domain.User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.User, 0, len(s.users))
	for _, user := range s.users {
		result = append(result, user)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Email < result[j].Email })
	return result
}

func (s *Store) SetUserState(id string, state domain.AccountState) error {
	if !state.Valid() {
		return domain.ErrValidation
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[id]
	if !ok {
		return domain.ErrNotFound
	}
	user.State = state
	s.users[id] = user
	return nil
}

func (s *Store) SetUserRole(id string, role domain.Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[id]
	if !ok {
		return domain.ErrNotFound
	}
	if !role.Valid() {
		return domain.ErrValidation
	}
	user.Role = role
	s.users[id] = user
	return nil
}

func (s *Store) UpdateUserProfile(id string, displayName, avatarURL, bio, organization, timezone, locale string) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[id]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	if strings.TrimSpace(displayName) == "" {
		return domain.User{}, domain.ErrValidation
	}
	user.DisplayName = strings.TrimSpace(displayName)
	user.AvatarURL = avatarURL
	user.Bio = bio
	user.Organization = organization
	user.Timezone = timezone
	user.Locale = locale
	s.users[id] = user
	return user, nil
}

func (s *Store) UpdatePasswordHash(id, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[id]; !ok {
		return domain.ErrNotFound
	}
	if hash == "" {
		return domain.ErrValidation
	}
	user := s.users[id]
	user.PasswordHash = hash
	s.users[id] = user
	return nil
}

func (s *Store) EventBySlug(slug string) (domain.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, event := range s.events {
		if event.Slug == slug {
			return event, nil
		}
	}
	return domain.Event{}, domain.ErrNotFound
}

func (s *Store) EventByID(id string) (domain.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	event, ok := s.events[id]
	if !ok {
		return domain.Event{}, domain.ErrNotFound
	}
	return event, nil
}

func (s *Store) ListEvents() []domain.Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.Event, 0, len(s.events))
	for _, event := range s.events {
		result = append(result, event)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result
}

func (s *Store) CreateEvent(event domain.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.ID == "" {
		event.ID = domain.NewID("evt")
	}
	if event.Slug == "" || event.Name == "" || event.SubmissionsClose.IsZero() {
		return domain.ErrValidation
	}
	for _, existing := range s.events {
		if existing.Slug == event.Slug {
			return domain.ErrAlreadyExists
		}
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	s.events[event.ID] = event
	return nil
}

func (s *Store) TrackByID(id string) (domain.Track, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	track, ok := s.tracks[id]
	if !ok {
		return domain.Track{}, domain.ErrNotFound
	}
	return track, nil
}

func (s *Store) TeamByID(id string) (domain.Team, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	team, ok := s.teams[id]
	if !ok {
		return domain.Team{}, domain.ErrNotFound
	}
	return team, nil
}

func (s *Store) IsTeamCaptain(userID, teamID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	team, ok := s.teams[teamID]
	return ok && team.CaptainID == userID
}

func (s *Store) CreateTeam(team domain.Team) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if team.ID == "" {
		team.ID = domain.NewID("tm")
	}
	if team.EventID == "" || team.Name == "" || team.CaptainID == "" {
		return domain.ErrValidation
	}
	if _, ok := s.events[team.EventID]; !ok {
		return domain.ErrNotFound
	}
	if _, ok := s.users[team.CaptainID]; !ok {
		return domain.ErrNotFound
	}
	if team.CreatedAt.IsZero() {
		team.CreatedAt = time.Now().UTC()
	}
	if team.Status == "" {
		team.Status = domain.TeamStatusActive
	}
	s.teams[team.ID] = team
	s.memberships[membershipKey(team.ID, team.CaptainID)] = domain.TeamMembership{ID: domain.NewID("tmem"), EventID: team.EventID, TeamID: team.ID, UserID: team.CaptainID, Role: domain.TeamRoleCaptain, Status: "active", JoinedAt: team.CreatedAt, UpdatedAt: team.CreatedAt}
	return nil
}

func (s *Store) ListTeams(eventID string) []domain.Team {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.Team, 0)
	for _, team := range s.teams {
		if team.EventID == eventID {
			result = append(result, team)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].ID < result[j].ID
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func (s *Store) SubmissionByID(id string) (domain.Submission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	submission, ok := s.submissions[id]
	if !ok {
		return domain.Submission{}, domain.ErrNotFound
	}
	return cloneSubmission(submission), nil
}

func (s *Store) ListSubmissions(eventID, query, trackID string) []domain.Submission {
	s.mu.RLock()
	defer s.mu.RUnlock()
	query = strings.ToLower(strings.TrimSpace(query))
	result := make([]domain.Submission, 0)
	for _, submission := range s.submissions {
		if submission.EventID != eventID || !isPublicSubmission(submission.Status) {
			continue
		}
		if trackID != "" && submission.TrackID != trackID {
			continue
		}
		if query != "" && !submissionMatches(submission, query) {
			continue
		}
		result = append(result, cloneSubmission(submission))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Title == result[j].Title {
			return result[i].ID < result[j].ID
		}
		return result[i].Title < result[j].Title
	})
	return result
}

func (s *Store) CreateSubmission(submission domain.Submission) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if submission.ID == "" {
		submission.ID = domain.NewID("sub")
	}
	if _, exists := s.submissions[submission.ID]; exists {
		return domain.ErrAlreadyExists
	}
	if _, exists := s.events[submission.EventID]; !exists {
		return domain.ErrNotFound
	}
	if _, exists := s.teams[submission.TeamID]; !exists {
		return domain.ErrNotFound
	}
	if submission.Version == 0 {
		submission.Version = 1
	}
	if submission.Status == "" {
		submission.Status = domain.SubmissionDraft
	}
	if submission.UpdatedAt.IsZero() {
		submission.UpdatedAt = time.Now().UTC()
	}
	s.submissions[submission.ID] = cloneSubmission(submission)
	return nil
}

func (s *Store) UpdateSubmission(id string, update func(domain.Submission) (domain.Submission, error)) (domain.Submission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	submission, ok := s.submissions[id]
	if !ok {
		return domain.Submission{}, domain.ErrNotFound
	}
	updated, err := update(cloneSubmission(submission))
	if err != nil {
		return domain.Submission{}, err
	}
	updated.ID = id
	updated.Version = submission.Version + 1
	updated.UpdatedAt = time.Now().UTC()
	s.submissions[id] = cloneSubmission(updated)
	return cloneSubmission(updated), nil
}

func (s *Store) IsAssigned(eventID, judgeID, projectID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, assignment := range s.assignments {
		if assignment.EventID == eventID && assignment.JudgeID == judgeID && assignment.ProjectID == projectID {
			return true
		}
	}
	return false
}

func (s *Store) ReviewsForJudge(eventID, judgeID string) []domain.Review {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.Review, 0)
	for _, review := range s.reviews {
		if review.EventID == eventID && review.JudgeID == judgeID {
			result = append(result, cloneReview(review))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ProjectID < result[j].ProjectID })
	return result
}

func (s *Store) ReviewForJudgeProject(judgeID, projectID string) (domain.Review, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	review, ok := s.reviews[reviewKey(judgeID, projectID)]
	if !ok {
		return domain.Review{}, domain.ErrNotFound
	}
	return cloneReview(review), nil
}

func (s *Store) SaveReview(review domain.Review) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if review.ID == "" {
		review.ID = domain.NewID("rev")
	}
	if _, exists := s.events[review.EventID]; !exists {
		return domain.ErrNotFound
	}
	if _, exists := s.submissions[review.ProjectID]; !exists {
		return domain.ErrNotFound
	}
	review.UpdatedAt = time.Now().UTC()
	s.reviews[reviewKey(review.JudgeID, review.ProjectID)] = cloneReview(review)
	return nil
}

func (s *Store) Progress(eventID string) domain.Progress {
	s.mu.RLock()
	defer s.mu.RUnlock()
	progress := domain.Progress{EventID: eventID}
	for _, submission := range s.submissions {
		if submission.EventID != eventID || !isPublicSubmission(submission.Status) {
			continue
		}
		progress.Submissions++
		progress.EligibleSubmissions++
	}
	for _, assignment := range s.assignments {
		if assignment.EventID == eventID {
			progress.Assignments++
		}
	}
	for _, review := range s.reviews {
		if review.EventID != eventID {
			continue
		}
		if review.Submitted {
			progress.ReviewsCompleted++
		} else {
			progress.ReviewsStarted++
		}
	}
	progress.ReviewsPending = progress.Assignments - progress.ReviewsCompleted
	if progress.ReviewsPending < 0 {
		progress.ReviewsPending = 0
	}
	return progress
}

func (s *Store) AllReviews(eventID string) []domain.Review {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.Review, 0)
	for _, review := range s.reviews {
		if review.EventID == eventID {
			result = append(result, cloneReview(review))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ProjectID == result[j].ProjectID {
			return result[i].JudgeID < result[j].JudgeID
		}
		return result[i].ProjectID < result[j].ProjectID
	})
	return result
}

func (s *Store) CreateSession(session domain.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session.ID == "" || session.UserID == "" || session.TokenHash == "" || session.ExpiresAt.IsZero() {
		return domain.ErrValidation
	}
	if _, exists := s.sessions[session.ID]; exists {
		return domain.ErrAlreadyExists
	}
	if _, exists := s.users[session.UserID]; !exists {
		return domain.ErrNotFound
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now().UTC()
	}
	if session.LastSeenAt.IsZero() {
		session.LastSeenAt = session.CreatedAt
	}
	s.sessions[session.ID] = session
	return nil
}

func (s *Store) SessionByID(id string) (domain.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[id]
	if !ok {
		return domain.Session{}, domain.ErrNotFound
	}
	return session, nil
}

func (s *Store) SessionByTokenHash(tokenHash string) (domain.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, session := range s.sessions {
		if session.TokenHash == tokenHash {
			return session, nil
		}
	}
	return domain.Session{}, domain.ErrNotFound
}

func (s *Store) ListSessions(userID string) []domain.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.Session, 0)
	for _, session := range s.sessions {
		if session.UserID == userID {
			result = append(result, session)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].LastSeenAt.After(result[j].LastSeenAt) })
	return result
}

func (s *Store) TouchSession(id string, seenAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return domain.ErrNotFound
	}
	session.LastSeenAt = seenAt.UTC()
	s.sessions[id] = session
	return nil
}

func (s *Store) RevokeSession(id, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return domain.ErrNotFound
	}
	if session.RevokedAt == nil {
		now := time.Now().UTC()
		session.RevokedAt = &now
		session.RevocationReason = reason
		s.sessions[id] = session
	}
	return nil
}

func (s *Store) RevokeUserSessions(userID, exceptID, reason string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	count := 0
	for id, session := range s.sessions {
		if session.UserID != userID || id == exceptID || session.RevokedAt != nil {
			continue
		}
		session.RevokedAt = &now
		session.RevocationReason = reason
		s.sessions[id] = session
		count++
	}
	return count
}

func (s *Store) TeamMembership(teamID, userID string) (domain.TeamMembership, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	membership, ok := s.memberships[membershipKey(teamID, userID)]
	if !ok {
		return domain.TeamMembership{}, domain.ErrNotFound
	}
	return membership, nil
}

func (s *Store) TeamMembers(teamID string) []domain.TeamMembership {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.TeamMembership, 0)
	for _, membership := range s.memberships {
		if membership.TeamID == teamID && membership.Status == "active" {
			result = append(result, membership)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UserID < result[j].UserID })
	return result
}

func (s *Store) TeamMembershipsForUser(userID string) []domain.TeamMembership {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.TeamMembership, 0)
	for _, membership := range s.memberships {
		if membership.UserID == userID {
			result = append(result, membership)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].TeamID < result[j].TeamID })
	return result
}

func (s *Store) UpsertTeamMember(membership domain.TeamMembership) error {
	if !membership.Role.Valid() {
		return domain.ErrValidation
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if membership.TeamID == "" || membership.UserID == "" || membership.EventID == "" {
		return domain.ErrValidation
	}
	if _, exists := s.teams[membership.TeamID]; !exists {
		return domain.ErrNotFound
	}
	if _, exists := s.users[membership.UserID]; !exists {
		return domain.ErrNotFound
	}
	if membership.ID == "" {
		membership.ID = domain.NewID("tmem")
	}
	if membership.JoinedAt.IsZero() {
		membership.JoinedAt = time.Now().UTC()
	}
	membership.UpdatedAt = time.Now().UTC()
	s.memberships[membershipKey(membership.TeamID, membership.UserID)] = membership
	return nil
}

func (s *Store) SetTeamMemberRole(teamID, userID string, role domain.TeamRole) error {
	if !role.Valid() {
		return domain.ErrValidation
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	membership, ok := s.memberships[membershipKey(teamID, userID)]
	if !ok || membership.Status != "active" {
		return domain.ErrNotFound
	}
	membership.Role = role
	membership.UpdatedAt = time.Now().UTC()
	s.memberships[membershipKey(teamID, userID)] = membership
	return nil
}

func (s *Store) TransferTeamCaptaincy(teamID, currentCaptainID, nextCaptainID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	team, ok := s.teams[teamID]
	if !ok || team.Status != domain.TeamStatusActive || team.CaptainID != currentCaptainID {
		return domain.ErrForbidden
	}
	membership, ok := s.memberships[membershipKey(teamID, nextCaptainID)]
	if !ok || membership.Status != "active" {
		return domain.ErrNotFound
	}
	team.CaptainID = nextCaptainID
	s.teams[teamID] = team
	membership.Role = domain.TeamRoleCaptain
	membership.UpdatedAt = time.Now().UTC()
	s.memberships[membershipKey(teamID, nextCaptainID)] = membership
	old, ok := s.memberships[membershipKey(teamID, currentCaptainID)]
	if ok {
		old.Role = domain.TeamRoleLeader
		old.UpdatedAt = time.Now().UTC()
		s.memberships[membershipKey(teamID, currentCaptainID)] = old
	}
	return nil
}

func (s *Store) RemoveTeamMember(teamID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	team, ok := s.teams[teamID]
	if !ok {
		return domain.ErrNotFound
	}
	if team.CaptainID == userID {
		return domain.ErrConflict
	}
	membership, ok := s.memberships[membershipKey(teamID, userID)]
	if !ok {
		return domain.ErrNotFound
	}
	now := time.Now().UTC()
	membership.Status = "removed"
	membership.LeftAt = &now
	membership.UpdatedAt = now
	s.memberships[membershipKey(teamID, userID)] = membership
	return nil
}

func (s *Store) ArchiveTeam(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	team, ok := s.teams[id]
	if !ok {
		return domain.ErrNotFound
	}
	now := time.Now().UTC()
	team.Status = domain.TeamStatusArchived
	team.DeletedAt = &now
	s.teams[id] = team
	for key, membership := range s.memberships {
		if membership.TeamID != id || membership.Status != "active" {
			continue
		}
		membership.Status = "removed"
		membership.LeftAt = &now
		membership.UpdatedAt = now
		s.memberships[key] = membership
	}
	return nil
}

func (s *Store) CreateNotification(notification domain.Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if notification.ID == "" {
		notification.ID = domain.NewID("ntf")
	}
	if notification.UserID == "" || notification.Title == "" {
		return domain.ErrValidation
	}
	if _, exists := s.users[notification.UserID]; !exists {
		return domain.ErrNotFound
	}
	if notification.CreatedAt.IsZero() {
		notification.CreatedAt = time.Now().UTC()
	}
	s.notifications[notification.ID] = notification
	return nil
}

func (s *Store) ListNotifications(userID string) []domain.Notification {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.Notification, 0)
	for _, notification := range s.notifications {
		if notification.UserID == userID {
			result = append(result, notification)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result
}

func (s *Store) MarkNotificationRead(userID, notificationID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	notification, ok := s.notifications[notificationID]
	if !ok || notification.UserID != userID {
		return domain.ErrNotFound
	}
	now := time.Now().UTC()
	notification.ReadAt = &now
	s.notifications[notificationID] = notification
	return nil
}

func (s *Store) RecordAudit(event domain.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.ID == "" {
		event.ID = domain.NewID("aud")
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if event.Action == "" {
		return domain.ErrValidation
	}
	s.auditEvents[event.ID] = event
	return nil
}

func (s *Store) ListAudit(eventID string) []domain.AuditEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.AuditEvent, 0)
	for _, event := range s.auditEvents {
		if eventID == "" || event.EventID == eventID {
			result = append(result, event)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result
}

func isPublicSubmission(status domain.SubmissionStatus) bool {
	return status == domain.SubmissionSubmitted || status == domain.SubmissionLocked
}

func submissionMatches(submission domain.Submission, query string) bool {
	values := []string{submission.Title, submission.Summary, strings.Join(submission.Tags, " ")}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

func reviewKey(judgeID, projectID string) string {
	return judgeID + ":" + projectID
}

func membershipKey(teamID, userID string) string {
	return teamID + ":" + userID
}

func cloneSubmission(submission domain.Submission) domain.Submission {
	copySubmission := submission
	copySubmission.Tags = append([]string(nil), submission.Tags...)
	if submission.CustomAnswers != nil {
		copySubmission.CustomAnswers = make(map[string]string, len(submission.CustomAnswers))
		for key, value := range submission.CustomAnswers {
			copySubmission.CustomAnswers[key] = value
		}
	}
	return copySubmission
}

func cloneReview(review domain.Review) domain.Review {
	copyReview := review
	copyReview.Criteria = make(map[string]int, len(review.Criteria))
	for key, value := range review.Criteria {
		copyReview.Criteria[key] = value
	}
	return copyReview
}
