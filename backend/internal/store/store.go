package store

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/seed"
)

type Store struct {
	mu                 sync.RWMutex
	users              map[string]domain.User
	usersByMail        map[string]string
	judgeProfiles      map[string]domain.JudgeProfile
	conflicts          map[string]domain.ConflictDeclaration
	rubrics            map[string]domain.Rubric
	profiles           map[string]domain.UserProfile
	questions          map[string]domain.HackathonQuestion
	milestones         map[string]domain.HackathonMilestone
	hosts              map[string]domain.HackathonHost
	roster             map[string]domain.JudgeRosterEntry
	invites            map[string]domain.TeamInvite
	participations     map[string]domain.Participation
	activity           map[string]domain.ActivityEntry
	staff              map[string]domain.EventStaff
	mail               map[string]domain.MailMessage
	mailByDedupe       map[string]domain.MailMessage
	mailPreferences    map[string]domain.MailPreferences
	unsubscribe        map[string]domain.UnsubscribeToken
	events             map[string]domain.Event
	tracks             map[string]domain.Track
	prizes             map[string]domain.Prize
	teams              map[string]domain.Team
	memberships        map[string]domain.TeamMembership
	submissions        map[string]domain.Submission
	submissionVersions map[string]domain.SubmissionVersion
	duplicates         map[string]domain.DuplicateFlag
	assignments        map[string]domain.Assignment
	reviews            map[string]domain.Review
	sessions           map[string]domain.Session
	notifications      map[string]domain.Notification
	auditEvents        map[string]domain.AuditEvent
}

func New(data seed.Data) *Store {
	store := &Store{
		users:              make(map[string]domain.User, len(data.Users)),
		usersByMail:        make(map[string]string, len(data.Users)),
		judgeProfiles:      make(map[string]domain.JudgeProfile, len(data.JudgeProfiles)),
		conflicts:          make(map[string]domain.ConflictDeclaration),
		rubrics:            make(map[string]domain.Rubric, len(data.Rubrics)),
		profiles:           make(map[string]domain.UserProfile, len(data.Profiles)),
		questions:          make(map[string]domain.HackathonQuestion, len(data.Questions)),
		milestones:         make(map[string]domain.HackathonMilestone, len(data.Milestones)),
		hosts:              make(map[string]domain.HackathonHost, len(data.Hosts)),
		roster:             make(map[string]domain.JudgeRosterEntry, len(data.JudgeRoster)),
		invites:            make(map[string]domain.TeamInvite, len(data.Invites)),
		participations:     make(map[string]domain.Participation, len(data.Participations)),
		activity:           make(map[string]domain.ActivityEntry, len(data.Activity)),
		staff:              make(map[string]domain.EventStaff, len(data.EventStaff)),
		mail:               make(map[string]domain.MailMessage),
		mailByDedupe:       make(map[string]domain.MailMessage),
		mailPreferences:    make(map[string]domain.MailPreferences, len(data.MailPreferences)),
		unsubscribe:        make(map[string]domain.UnsubscribeToken),
		events:             make(map[string]domain.Event, len(data.Events)),
		tracks:             make(map[string]domain.Track, len(data.Tracks)),
		prizes:             make(map[string]domain.Prize, len(data.Prizes)),
		teams:              make(map[string]domain.Team, len(data.Teams)),
		memberships:        make(map[string]domain.TeamMembership, len(data.TeamMemberships)),
		submissions:        make(map[string]domain.Submission, len(data.Submissions)),
		submissionVersions: make(map[string]domain.SubmissionVersion),
		duplicates:         make(map[string]domain.DuplicateFlag),
		assignments:        make(map[string]domain.Assignment, len(data.Assignments)),
		reviews:            make(map[string]domain.Review, len(data.Reviews)),
		sessions:           make(map[string]domain.Session),
		notifications:      make(map[string]domain.Notification, len(data.Notifications)),
		auditEvents:        make(map[string]domain.AuditEvent, len(data.AuditEvents)),
	}
	for _, user := range data.Users {
		store.users[user.ID] = user
		store.usersByMail[strings.ToLower(user.Email)] = user.ID
	}
	for _, profile := range data.JudgeProfiles {
		store.judgeProfiles[profile.UserID] = profile
	}
	for _, rubric := range data.Rubrics {
		store.rubrics[rubric.ID] = rubric.Clone()
	}
	for _, event := range data.Events {
		store.events[event.ID] = event
	}
	for _, track := range data.Tracks {
		store.tracks[track.ID] = track
	}
	for _, prize := range data.Prizes {
		store.prizes[prize.ID] = prize
	}
	for _, profile := range data.Profiles {
		store.profiles[profile.UserID] = profile.Clone()
	}
	for _, question := range data.Questions {
		store.questions[question.ID] = question
	}
	for _, milestone := range data.Milestones {
		store.milestones[milestone.ID] = milestone
	}
	for _, host := range data.Hosts {
		store.hosts[host.ID] = host
	}
	for _, entry := range data.JudgeRoster {
		store.roster[rosterKey(entry.EventID, entry.JudgeID)] = entry
	}
	for _, invite := range data.Invites {
		store.invites[invite.ID] = invite.Clone()
	}
	for _, participation := range data.Participations {
		store.participations[participationKey(participation.UserID, participation.EventID)] = participation
	}
	for _, entry := range data.Activity {
		store.activity[entry.ID] = entry
	}
	for _, member := range data.EventStaff {
		store.staff[staffKey(member.EventID, member.UserID)] = member
	}
	for _, preferences := range data.MailPreferences {
		store.mailPreferences[preferences.UserID] = preferences
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

func (s *Store) CreateTrack(track domain.Track) (domain.Track, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if track.ID == "" {
		track.ID = domain.NewID("trk")
	}
	if _, ok := s.events[track.Event]; !ok {
		return domain.Track{}, domain.ErrNotFound
	}
	if strings.TrimSpace(track.Name) == "" {
		return domain.Track{}, domain.ErrValidation
	}
	if strings.TrimSpace(track.Slug) == "" {
		track.Slug = slugify(track.Name)
	}
	for _, existing := range s.tracks {
		if existing.Event == track.Event && existing.Slug == track.Slug {
			return domain.Track{}, domain.ErrAlreadyExists
		}
	}
	if track.Order <= 0 {
		for _, existing := range s.tracks {
			if existing.Event == track.Event && existing.Order >= track.Order {
				track.Order = existing.Order + 1
			}
		}
		if track.Order <= 0 {
			track.Order = 1
		}
	}
	s.tracks[track.ID] = track
	return track, nil
}

func (s *Store) ListTracks(eventID string) []domain.Track {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tracks := make([]domain.Track, 0)
	for _, track := range s.tracks {
		if track.Event == eventID {
			tracks = append(tracks, track)
		}
	}
	sort.Slice(tracks, func(i, j int) bool {
		if tracks[i].Order == tracks[j].Order {
			return tracks[i].ID < tracks[j].ID
		}
		return tracks[i].Order < tracks[j].Order
	})
	return tracks
}

func (s *Store) CreatePrize(prize domain.Prize) (domain.Prize, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if prize.ID == "" {
		prize.ID = domain.NewID("prz")
	}
	if _, ok := s.events[prize.EventID]; !ok {
		return domain.Prize{}, domain.ErrNotFound
	}
	if strings.TrimSpace(prize.Name) == "" {
		return domain.Prize{}, domain.ErrValidation
	}
	if prize.TrackID != "" {
		track, ok := s.tracks[prize.TrackID]
		if !ok || track.Event != prize.EventID {
			return domain.Prize{}, domain.ErrValidation
		}
	}
	prize.CreatedAt = time.Now().UTC()
	s.prizes[prize.ID] = prize
	return prize, nil
}

func (s *Store) ListPrizes(eventID string) []domain.Prize {
	s.mu.RLock()
	defer s.mu.RUnlock()
	prizes := make([]domain.Prize, 0)
	for _, prize := range s.prizes {
		if prize.EventID == eventID {
			prizes = append(prizes, prize)
		}
	}
	sort.Slice(prizes, func(i, j int) bool {
		if prizes[i].Rank == prizes[j].Rank {
			return prizes[i].ID < prizes[j].ID
		}
		return prizes[i].Rank < prizes[j].Rank
	})
	return prizes
}

func slugify(value string) string {
	lowered := strings.ToLower(strings.TrimSpace(value))
	replaced := nonAlphanumericSlug.ReplaceAllString(lowered, "-")
	return strings.Trim(replaced, "-")
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
	event, ok := s.events[team.EventID]
	if !ok {
		return domain.ErrNotFound
	}
	if _, ok := s.users[team.CaptainID]; !ok {
		return domain.ErrNotFound
	}
	if team.Scope == "" {
		team.Scope = TeamScopeForEvent(event)
	}
	if team.Scope == domain.TeamScopeGlobal && !event.AllowGlobalTeams {
		return domain.ErrValidation
	}
	if team.Availability == "" {
		team.Availability = domain.TeamInviteOnly
	}
	switch team.Availability {
	case domain.TeamOpenForMembers, domain.TeamInviteOnly, domain.TeamClosed, domain.TeamFull:
	default:
		return domain.ErrValidation
	}
	if event.MaxTeamSize > 0 && team.MaxSize == 0 {
		team.MaxSize = event.MaxTeamSize
	}
	if team.MaxSize > 0 && event.MaxTeamSize > 0 && team.MaxSize > event.MaxTeamSize {
		return domain.ErrValidation
	}
	if team.CreatedAt.IsZero() {
		team.CreatedAt = time.Now().UTC()
	}
	if team.Status == "" {
		team.Status = domain.TeamStatusActive
	}
	s.teams[team.ID] = team
	s.memberships[membershipKey(team.ID, team.CaptainID)] = domain.TeamMembership{ID: domain.NewID("tmem"), EventID: team.EventID, TeamID: team.ID, UserID: team.CaptainID, Role: domain.TeamRoleCaptain, Status: "active", JoinedAt: team.CreatedAt, UpdatedAt: team.CreatedAt}
	s.rebuildParticipationLocked(team.CaptainID, team, domain.ParticipationCaptain)
	return nil
}

// TeamScopeForEvent reports whether an event uses per-hackathon or global teams.
func TeamScopeForEvent(event domain.Event) domain.TeamScope {
	if event.AllowGlobalTeams {
		return domain.TeamScopeGlobal
	}
	return domain.TeamScopeHackathon
}

func (s *Store) UpdateTeam(id string, update func(domain.Team) (domain.Team, error)) (domain.Team, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	team, ok := s.teams[id]
	if !ok {
		return domain.Team{}, domain.ErrNotFound
	}
	if team.Status != domain.TeamStatusActive {
		return domain.Team{}, domain.ErrConflict
	}
	updated, err := update(team)
	if err != nil {
		return domain.Team{}, err
	}
	updated.ID = team.ID
	updated.EventID = team.EventID
	updated.CaptainID = team.CaptainID
	updated.CreatedAt = team.CreatedAt
	updated.Status = team.Status
	switch updated.Availability {
	case domain.TeamOpenForMembers, domain.TeamInviteOnly, domain.TeamClosed, domain.TeamFull:
	default:
		return domain.Team{}, domain.ErrValidation
	}
	if updated.Availability == domain.TeamOpenForMembers && len(updated.OpenRoles) == 0 {
		return domain.Team{}, domain.ErrValidation
	}
	if updated.MaxSize > 0 && len(s.teamMembersLocked(id)) > updated.MaxSize {
		return domain.Team{}, domain.ErrValidation
	}
	s.teams[id] = updated
	return updated, nil
}

func (s *Store) rebuildParticipationLocked(userID string, team domain.Team, role domain.ParticipationRole) {
	if team.EventID == "" {
		return
	}
	key := participationKey(userID, team.EventID)
	participation, ok := s.participations[key]
	if !ok {
		participation = domain.Participation{UserID: userID, EventID: team.EventID, CreatedAt: time.Now().UTC()}
	}
	participation.TeamID = team.ID
	if role != "" {
		participation.Role = role
	}
	s.participations[key] = participation
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
		if submission.EventID != eventID || !isEligibleSubmission(submission) {
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

func (s *Store) ReviseSubmission(id, actorID, reason string, update func(domain.Submission) (domain.Submission, error)) (domain.Submission, domain.SubmissionVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	submission, ok := s.submissions[id]
	if !ok {
		return domain.Submission{}, domain.SubmissionVersion{}, domain.ErrNotFound
	}
	if submission.Status == domain.SubmissionLocked || submission.Status == domain.SubmissionDisqualified {
		return domain.Submission{}, domain.SubmissionVersion{}, domain.ErrConflict
	}
	updated, err := update(cloneSubmission(submission))
	if err != nil {
		return domain.Submission{}, domain.SubmissionVersion{}, err
	}
	updated.ID = id
	updated.Version = submission.Version + 1
	updated.UpdatedAt = time.Now().UTC()
	s.submissions[id] = cloneSubmission(updated)
	version := domain.SubmissionVersion{
		ID:            domain.NewID("ver"),
		SubmissionID:  id,
		Version:       updated.Version,
		Title:         updated.Title,
		Summary:       updated.Summary,
		Description:   updated.Description,
		RepositoryURL: updated.RepositoryURL,
		LiveURL:       updated.LiveURL,
		VideoURL:      updated.VideoURL,
		Tags:          append([]string(nil), updated.Tags...),
		CustomAnswers: cloneAnswers(updated.CustomAnswers),
		Status:        updated.Status,
		CreatedBy:     actorID,
		CreatedAt:     updated.UpdatedAt,
		Reason:        strings.TrimSpace(reason),
	}
	s.submissionVersions[versionKey(id, updated.Version)] = version
	return cloneSubmission(updated), version, nil
}

func (s *Store) RecordSubmissionVersion(id, actorID, reason string) (domain.SubmissionVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	submission, ok := s.submissions[id]
	if !ok {
		return domain.SubmissionVersion{}, domain.ErrNotFound
	}
	version := domain.SubmissionVersion{
		ID:            domain.NewID("ver"),
		SubmissionID:  id,
		Version:       submission.Version,
		Title:         submission.Title,
		Summary:       submission.Summary,
		Description:   submission.Description,
		RepositoryURL: submission.RepositoryURL,
		LiveURL:       submission.LiveURL,
		VideoURL:      submission.VideoURL,
		Tags:          append([]string(nil), submission.Tags...),
		CustomAnswers: cloneAnswers(submission.CustomAnswers),
		Status:        submission.Status,
		CreatedBy:     actorID,
		CreatedAt:     submission.UpdatedAt,
		Reason:        strings.TrimSpace(reason),
	}
	s.submissionVersions[versionKey(id, submission.Version)] = version
	return version, nil
}

func (s *Store) SubmissionVersions(submissionID string) []domain.SubmissionVersion {
	s.mu.RLock()
	defer s.mu.RUnlock()
	versions := make([]domain.SubmissionVersion, 0)
	for _, version := range s.submissionVersions {
		if version.SubmissionID == submissionID {
			versions = append(versions, cloneVersion(version))
		}
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i].Version > versions[j].Version })
	return versions
}

func (s *Store) SetSubmissionEligibility(id string, decision domain.EligibilityDecision, note, actorID string) (domain.Submission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	submission, ok := s.submissions[id]
	if !ok {
		return domain.Submission{}, domain.ErrNotFound
	}
	switch decision {
	case domain.EligibilityEligible, domain.EligibilityIneligible, domain.EligibilityPending:
	default:
		return domain.Submission{}, domain.ErrValidation
	}
	if decision == domain.EligibilityIneligible && strings.TrimSpace(note) == "" {
		return domain.Submission{}, domain.ErrValidation
	}
	now := time.Now().UTC()
	submission.Eligibility = decision
	submission.EligibilityNote = strings.TrimSpace(note)
	submission.EligibilityBy = actorID
	submission.EligibilityAt = &now
	s.submissions[id] = cloneSubmission(submission)
	return cloneSubmission(submission), nil
}

func (s *Store) SetSubmissionStatus(id string, status domain.SubmissionStatus, note, actorID string) (domain.Submission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	submission, ok := s.submissions[id]
	if !ok {
		return domain.Submission{}, domain.ErrNotFound
	}
	if !validStatusTransition(submission.Status, status) {
		return domain.Submission{}, domain.ErrConflict
	}
	submission.Status = status
	submission.UpdatedAt = time.Now().UTC()
	if status == domain.SubmissionSubmitted {
		now := time.Now().UTC()
		submission.SubmittedAt = &now
		if submission.Eligibility == "" {
			submission.Eligibility = domain.EligibilityPending
		}
	}
	if status == domain.SubmissionWithdrawn || status == domain.SubmissionDisqualified {
		submission.Eligibility = domain.EligibilityIneligible
		submission.EligibilityNote = strings.TrimSpace(note)
		submission.EligibilityBy = actorID
		now := time.Now().UTC()
		submission.EligibilityAt = &now
	}
	if status == domain.SubmissionSubmitted && submission.Eligibility == domain.EligibilityIneligible {
		submission.Eligibility = domain.EligibilityPending
	}
	s.submissions[id] = cloneSubmission(submission)
	return cloneSubmission(submission), nil
}

func validStatusTransition(current, next domain.SubmissionStatus) bool {
	if current == next {
		return true
	}
	switch current {
	case domain.SubmissionDraft:
		return next == domain.SubmissionSubmitted || next == domain.SubmissionWithdrawn
	case domain.SubmissionSubmitted:
		return next == domain.SubmissionNeedsChange || next == domain.SubmissionLocked || next == domain.SubmissionWithdrawn || next == domain.SubmissionDisqualified
	case domain.SubmissionNeedsChange:
		return next == domain.SubmissionSubmitted || next == domain.SubmissionWithdrawn || next == domain.SubmissionLocked || next == domain.SubmissionDisqualified
	case domain.SubmissionLocked, domain.SubmissionWithdrawn, domain.SubmissionDisqualified:
		return false
	default:
		return false
	}
}

func (s *Store) ScanForDuplicates(eventID string) ([]domain.DuplicateFlag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.events[eventID]; !ok {
		return nil, domain.ErrNotFound
	}
	projects := make([]domain.Submission, 0)
	for _, submission := range s.submissions {
		if submission.EventID == eventID {
			projects = append(projects, submission)
		}
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].ID < projects[j].ID })
	existing := make(map[string]struct{})
	for _, flag := range s.duplicates {
		existing[duplicateKey(flag.ProjectID, flag.DuplicateOfProjectID)] = struct{}{}
	}
	created := make([]domain.DuplicateFlag, 0)
	for i := 0; i < len(projects); i++ {
		for j := i + 1; j < len(projects); j++ {
			key := duplicateKey(projects[i].ID, projects[j].ID)
			if _, ok := existing[key]; ok {
				continue
			}
			signals := domain.DuplicateSignals(projects[i], projects[j])
			if len(signals) == 0 {
				continue
			}
			detail := signals[0].Detail
			if len(signals) > 1 {
				detail = detail + "; multiple matching signals"
			}
			flag := domain.DuplicateFlag{
				ID:                   domain.NewID("dup"),
				EventID:              eventID,
				ProjectID:            projects[i].ID,
				DuplicateOfProjectID: projects[j].ID,
				Signal:               signals[0].Kind,
				Detail:               detail,
				Status:               domain.DuplicateOpen,
				CreatedAt:            time.Now().UTC(),
			}
			s.duplicates[flag.ID] = flag
			existing[key] = struct{}{}
			created = append(created, flag)
		}
	}
	sort.Slice(created, func(i, j int) bool { return created[i].ID < created[j].ID })
	return created, nil
}

func (s *Store) ListDuplicates(eventID string, status domain.DuplicateStatus) []domain.DuplicateFlag {
	s.mu.RLock()
	defer s.mu.RUnlock()
	flags := make([]domain.DuplicateFlag, 0)
	for _, flag := range s.duplicates {
		if flag.EventID != eventID {
			continue
		}
		if status != "" && flag.Status != status {
			continue
		}
		flags = append(flags, flag)
	}
	sort.Slice(flags, func(i, j int) bool { return flags[i].ID < flags[j].ID })
	return flags
}

func (s *Store) ResolveDuplicate(id string, status domain.DuplicateStatus, note, actorID string) (domain.DuplicateFlag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	flag, ok := s.duplicates[id]
	if !ok {
		return domain.DuplicateFlag{}, domain.ErrNotFound
	}
	if status != domain.DuplicateConfirmed && status != domain.DuplicateDismissed {
		return domain.DuplicateFlag{}, domain.ErrValidation
	}
	if strings.TrimSpace(note) == "" {
		return domain.DuplicateFlag{}, domain.ErrValidation
	}
	now := time.Now().UTC()
	flag.Status = status
	flag.ResolvedAt = &now
	flag.ResolvedBy = actorID
	flag.ResolutionNote = strings.TrimSpace(note)
	s.duplicates[id] = flag
	return flag, nil
}

func (s *Store) JudgeProfile(userID string) (domain.JudgeProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	profile, ok := s.judgeProfiles[userID]
	if !ok {
		return domain.JudgeProfile{}, domain.ErrNotFound
	}
	return profile, nil
}

func (s *Store) SetJudgeProfile(profile domain.JudgeProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[profile.UserID]
	if !ok || user.Role != domain.RoleJudge {
		return domain.ErrValidation
	}
	s.judgeProfiles[profile.UserID] = profile
	return nil
}

func (s *Store) ListJudgeProfiles() []domain.JudgeProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.JudgeProfile, 0, len(s.judgeProfiles))
	for _, profile := range s.judgeProfiles {
		result = append(result, profile)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UserID < result[j].UserID })
	return result
}

func (s *Store) ListAssignments(eventID, judgeID string) []domain.Assignment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.Assignment, 0)
	for _, assignment := range s.assignments {
		if assignment.EventID == eventID && (judgeID == "" || assignment.JudgeID == judgeID) {
			result = append(result, assignment)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].JudgeID == result[j].JudgeID {
			return result[i].ProjectID < result[j].ProjectID
		}
		return result[i].JudgeID < result[j].JudgeID
	})
	return result
}

func (s *Store) AssignmentByID(id string) (domain.Assignment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	assignment, ok := s.assignments[id]
	if !ok {
		return domain.Assignment{}, domain.ErrNotFound
	}
	return assignment, nil
}

func (s *Store) CreateAssignment(assignment domain.Assignment) (domain.Assignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if assignment.ID == "" {
		assignment.ID = domain.NewID("asg")
	}
	if assignment.EventID == "" || assignment.JudgeID == "" || assignment.ProjectID == "" {
		return domain.Assignment{}, domain.ErrValidation
	}
	if _, ok := s.events[assignment.EventID]; !ok {
		return domain.Assignment{}, domain.ErrNotFound
	}
	project, ok := s.submissions[assignment.ProjectID]
	if !ok || project.EventID != assignment.EventID {
		return domain.Assignment{}, domain.ErrNotFound
	}
	profile, ok := s.judgeProfiles[assignment.JudgeID]
	if !ok || !profile.Active {
		return domain.Assignment{}, domain.ErrValidation
	}
	if len(profile.Tracks) > 0 && !containsString(profile.Tracks, project.TrackID) {
		return domain.Assignment{}, domain.ErrForbidden
	}
	if s.hasConflictLocked(assignment.EventID, assignment.JudgeID, assignment.ProjectID) {
		return domain.Assignment{}, domain.ErrConflict
	}
	activeCount := 0
	for _, existing := range s.assignments {
		if existing.EventID == assignment.EventID && existing.JudgeID == assignment.JudgeID && existing.RevokedAt == nil {
			if existing.ProjectID == assignment.ProjectID {
				return domain.Assignment{}, domain.ErrAlreadyExists
			}
			activeCount++
		}
	}
	if profile.Capacity > 0 && activeCount >= profile.Capacity {
		return domain.Assignment{}, domain.ErrConflict
	}
	if assignment.CreatedAt.IsZero() {
		assignment.CreatedAt = time.Now().UTC()
	}
	s.assignments[assignment.ID] = assignment
	return assignment, nil
}

func (s *Store) RevokeAssignment(id, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	assignment, ok := s.assignments[id]
	if !ok {
		return domain.ErrNotFound
	}
	if assignment.RevokedAt == nil {
		now := time.Now().UTC()
		assignment.RevokedAt = &now
		s.assignments[id] = assignment
	}
	return nil
}

func (s *Store) DeclareConflict(declaration domain.ConflictDeclaration) (domain.ConflictDeclaration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if declaration.ID == "" {
		declaration.ID = domain.NewID("conf")
	}
	if declaration.EventID == "" || declaration.JudgeID == "" || declaration.ProjectID == "" || declaration.Reason == "" {
		return domain.ConflictDeclaration{}, domain.ErrValidation
	}
	if _, ok := s.submissions[declaration.ProjectID]; !ok {
		return domain.ConflictDeclaration{}, domain.ErrNotFound
	}
	if !s.isAssignedLocked(declaration.EventID, declaration.JudgeID, declaration.ProjectID) {
		return domain.ConflictDeclaration{}, domain.ErrForbidden
	}
	if s.hasConflictLocked(declaration.EventID, declaration.JudgeID, declaration.ProjectID) {
		return domain.ConflictDeclaration{}, domain.ErrConflict
	}
	if declaration.CreatedAt.IsZero() {
		declaration.CreatedAt = time.Now().UTC()
	}
	s.conflicts[declaration.ID] = declaration
	for id, assignment := range s.assignments {
		if assignment.EventID == declaration.EventID && assignment.JudgeID == declaration.JudgeID && assignment.ProjectID == declaration.ProjectID && assignment.RevokedAt == nil {
			now := time.Now().UTC()
			assignment.RevokedAt = &now
			s.assignments[id] = assignment
		}
	}
	return declaration, nil
}

func (s *Store) HasConflict(eventID, judgeID, projectID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hasConflictLocked(eventID, judgeID, projectID)
}

func (s *Store) IsAssigned(eventID, judgeID, projectID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isAssignedLocked(eventID, judgeID, projectID)
}

func (s *Store) isAssignedLocked(eventID, judgeID, projectID string) bool {
	for _, assignment := range s.assignments {
		if assignment.EventID == eventID && assignment.JudgeID == judgeID && assignment.ProjectID == projectID && assignment.RevokedAt == nil {
			return true
		}
	}
	return false
}

func (s *Store) hasConflictLocked(eventID, judgeID, projectID string) bool {
	for _, conflict := range s.conflicts {
		if conflict.EventID == eventID && conflict.JudgeID == judgeID && conflict.ProjectID == projectID {
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

func (s *Store) SaveReview(review domain.Review) (domain.Review, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if review.ID == "" {
		review.ID = domain.NewID("rev")
	}
	if _, exists := s.events[review.EventID]; !exists {
		return domain.Review{}, domain.ErrNotFound
	}
	project, exists := s.submissions[review.ProjectID]
	if !exists {
		return domain.Review{}, domain.ErrNotFound
	}
	if existing, exists := s.reviews[reviewKey(review.JudgeID, review.ProjectID)]; exists {
		review.ID = existing.ID
		if existing.Submitted && !review.Submitted {
			return domain.Review{}, domain.ErrConflict
		}
		if review.Submitted && existing.Submitted && !sameReviewScores(existing, review) {
			return domain.Review{}, domain.ErrConflict
		}
		if existing.RubricID != "" {
			review.RubricID = existing.RubricID
			review.RubricVersion = existing.RubricVersion
		}
	}
	if review.RubricID == "" {
		rubric, ok := s.activeRubricLocked(review.EventID, project.TrackID)
		if !ok {
			return domain.Review{}, domain.ErrValidation
		}
		normalized, err := rubric.NormalizeScores(review.Criteria)
		if err != nil {
			return domain.Review{}, err
		}
		review.RubricID = rubric.ID
		review.RubricVersion = rubric.Version
		review.Normalized = normalized
	} else {
		rubric, ok := s.rubrics[review.RubricID]
		if !ok || rubric.EventID != review.EventID {
			return domain.Review{}, domain.ErrValidation
		}
		normalized, err := rubric.NormalizeScores(review.Criteria)
		if err != nil {
			return domain.Review{}, err
		}
		review.RubricVersion = rubric.Version
		review.Normalized = normalized
	}
	review.UpdatedAt = time.Now().UTC()
	s.reviews[reviewKey(review.JudgeID, review.ProjectID)] = cloneReview(review)
	return cloneReview(review), nil
}

func sameReviewScores(existing, next domain.Review) bool {
	if len(existing.Criteria) != len(next.Criteria) {
		return false
	}
	for key, value := range next.Criteria {
		if existing.Criteria[key] != value {
			return false
		}
	}
	return existing.Comment == next.Comment
}

func (s *Store) CreateRubric(rubric domain.Rubric) (domain.Rubric, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rubric.ID == "" {
		rubric.ID = domain.NewID("rub")
	}
	if _, ok := s.events[rubric.EventID]; !ok {
		return domain.Rubric{}, domain.ErrNotFound
	}
	if rubric.TrackID != "" {
		track, ok := s.tracks[rubric.TrackID]
		if !ok || track.Event != rubric.EventID {
			return domain.Rubric{}, domain.ErrValidation
		}
	}
	if err := rubric.Validate(); err != nil {
		return domain.Rubric{}, err
	}
	if rubric.Status == "" {
		rubric.Status = domain.RubricDraft
	}
	rubric.Version = s.nextRubricVersionLocked(rubric.EventID, rubric.TrackID)
	rubric.CreatedAt = time.Now().UTC()
	s.rubrics[rubric.ID] = rubric.Clone()
	return rubric.Clone(), nil
}

func (s *Store) UpdateRubric(id string, update func(domain.Rubric) (domain.Rubric, error)) (domain.Rubric, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rubric, ok := s.rubrics[id]
	if !ok {
		return domain.Rubric{}, domain.ErrNotFound
	}
	if rubric.Status != domain.RubricDraft {
		return domain.Rubric{}, domain.ErrConflict
	}
	updated, err := update(rubric)
	if err != nil {
		return domain.Rubric{}, err
	}
	if err := updated.Validate(); err != nil {
		return domain.Rubric{}, err
	}
	updated.ID = rubric.ID
	updated.Version = rubric.Version
	updated.Status = rubric.Status
	updated.CreatedAt = rubric.CreatedAt
	updated.CreatedBy = rubric.CreatedBy
	s.rubrics[id] = updated.Clone()
	return updated.Clone(), nil
}

func (s *Store) PublishRubric(id string) (domain.Rubric, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rubric, ok := s.rubrics[id]
	if !ok {
		return domain.Rubric{}, domain.ErrNotFound
	}
	if rubric.Status == domain.RubricPublished {
		return rubric.Clone(), nil
	}
	if rubric.Status != domain.RubricDraft {
		return domain.Rubric{}, domain.ErrConflict
	}
	if err := rubric.Validate(); err != nil {
		return domain.Rubric{}, err
	}
	now := time.Now().UTC()
	rubric.Status = domain.RubricPublished
	rubric.PublishedAt = &now
	s.rubrics[id] = rubric.Clone()
	return rubric.Clone(), nil
}

func (s *Store) ArchiveRubric(id string) (domain.Rubric, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rubric, ok := s.rubrics[id]
	if !ok {
		return domain.Rubric{}, domain.ErrNotFound
	}
	now := time.Now().UTC()
	rubric.Status = domain.RubricArchived
	rubric.ArchivedAt = &now
	s.rubrics[id] = rubric.Clone()
	return rubric.Clone(), nil
}

func (s *Store) RubricByID(id string) (domain.Rubric, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rubric, ok := s.rubrics[id]
	if !ok {
		return domain.Rubric{}, domain.ErrNotFound
	}
	return rubric.Clone(), nil
}

func (s *Store) RubricsForEvent(eventID string) []domain.Rubric {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.Rubric, 0)
	for _, rubric := range s.rubrics {
		if rubric.EventID == eventID {
			result = append(result, rubric.Clone())
		}
	}
	domain.SortRubrics(result)
	return result
}

func (s *Store) ActiveRubric(eventID, trackID string) (domain.Rubric, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rubric, ok := s.activeRubricLocked(eventID, trackID)
	if !ok {
		return domain.Rubric{}, domain.ErrNotFound
	}
	return rubric.Clone(), nil
}

func (s *Store) activeRubricLocked(eventID, trackID string) (domain.Rubric, bool) {
	var best *domain.Rubric
	for _, rubric := range s.rubrics {
		if rubric.EventID != eventID || rubric.Status != domain.RubricPublished {
			continue
		}
		if rubric.TrackID != "" && rubric.TrackID != trackID {
			continue
		}
		candidate := rubric
		if best == nil {
			best = &candidate
			continue
		}
		if rubric.TrackID == trackID && best.TrackID != trackID {
			best = &candidate
			continue
		}
		if rubric.TrackID == best.TrackID && rubric.Version > best.Version {
			best = &candidate
		}
	}
	if best == nil {
		return domain.Rubric{}, false
	}
	return best.Clone(), true
}

func (s *Store) nextRubricVersionLocked(eventID, trackID string) int {
	highest := 0
	for _, rubric := range s.rubrics {
		if rubric.EventID != eventID || rubric.TrackID != trackID {
			continue
		}
		if rubric.Version > highest {
			highest = rubric.Version
		}
	}
	return highest + 1
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
		if isEligibleSubmission(submission) {
			progress.EligibleSubmissions++
		}
	}
	for _, assignment := range s.assignments {
		if assignment.EventID == eventID && assignment.RevokedAt == nil {
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

func isEligibleSubmission(submission domain.Submission) bool {
	if !isPublicSubmission(submission.Status) {
		return false
	}
	return submission.Eligibility != domain.EligibilityIneligible
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
	if review.Normalized != nil {
		copyReview.Normalized = make(map[string]int, len(review.Normalized))
		for key, value := range review.Normalized {
			copyReview.Normalized[key] = value
		}
	}
	return copyReview
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func versionKey(submissionID string, version int) string {
	return submissionID + ":" + strconv.Itoa(version)
}

func duplicateKey(left, right string) string {
	if left > right {
		left, right = right, left
	}
	return left + ":" + right
}

func cloneVersion(version domain.SubmissionVersion) domain.SubmissionVersion {
	copyVersion := version
	copyVersion.Tags = append([]string(nil), version.Tags...)
	copyVersion.CustomAnswers = cloneAnswers(version.CustomAnswers)
	return copyVersion
}

func cloneAnswers(answers map[string]string) map[string]string {
	if answers == nil {
		return nil
	}
	copied := make(map[string]string, len(answers))
	for key, value := range answers {
		copied[key] = value
	}
	return copied
}

var nonAlphanumericSlug = regexp.MustCompile(`[^a-z0-9]+`)

func (s *Store) UpdateEvent(id string, update func(domain.Event) (domain.Event, error)) (domain.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	event, ok := s.events[id]
	if !ok {
		return domain.Event{}, domain.ErrNotFound
	}
	updated, err := update(event)
	if err != nil {
		return domain.Event{}, err
	}
	updated.ID = event.ID
	updated.Slug = event.Slug
	updated.CreatedAt = event.CreatedAt
	updated.ResultsPublished = updated.ResultsPublished || event.ResultsPublished
	updated.UpdatedAt = time.Now().UTC()
	s.events[id] = updated
	return updated, nil
}
