package store

import (
	"sort"
	"strings"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

func (s *Store) Profile(userID string) (domain.UserProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	profile, ok := s.profiles[userID]
	if !ok {
		return domain.UserProfile{}, domain.ErrNotFound
	}
	return profile.Clone(), nil
}

func (s *Store) ProfileOrDefault(userID string) (domain.UserProfile, error) {
	profile, err := s.Profile(userID)
	if err == domain.ErrNotFound {
		return domain.UserProfile{UserID: userID, Availability: domain.UserAvailabilitySolo, OpenToInvites: true}, nil
	}
	return profile, err
}

func (s *Store) SaveProfile(profile domain.UserProfile) (domain.UserProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[profile.UserID]; !ok {
		return domain.UserProfile{}, domain.ErrNotFound
	}
	if err := profile.Validate(); err != nil {
		return domain.UserProfile{}, err
	}
	profile.UpdatedAt = time.Now().UTC()
	s.profiles[profile.UserID] = profile.Clone()
	return profile.Clone(), nil
}

func (s *Store) SearchProfiles(eventID string, seekingOnly bool) []domain.UserProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.UserProfile, 0, len(s.profiles))
	for userID, profile := range s.profiles {
		if seekingOnly && !(profile.SeekingTeam || profile.OpenToInvites) {
			continue
		}
		if seekingOnly && eventID != "" && profile.SeekingEventID != "" && profile.SeekingEventID != eventID {
			continue
		}
		_ = userID
		result = append(result, profile.Clone())
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UserID < result[j].UserID })
	return result
}

func (s *Store) CreateQuestion(question domain.HackathonQuestion) (domain.HackathonQuestion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if question.ID == "" {
		question.ID = domain.NewID("qst")
	}
	if _, ok := s.events[question.EventID]; !ok {
		return domain.HackathonQuestion{}, domain.ErrNotFound
	}
	if err := question.Validate(); err != nil {
		return domain.HackathonQuestion{}, err
	}
	if question.Key == "" {
		question.Key = questionKey(question.Prompt)
	}
	for _, existing := range s.questions {
		if existing.EventID == question.EventID && existing.Key == question.Key && existing.Audience == question.Audience {
			return domain.HackathonQuestion{}, domain.ErrAlreadyExists
		}
	}
	now := time.Now().UTC()
	question.CreatedAt = now
	question.UpdatedAt = now
	if question.Position <= 0 {
		question.Position = len(s.questionsForEventLocked(question.EventID, question.Audience)) + 1
	}
	s.questions[question.ID] = question
	return question, nil
}

func (s *Store) UpdateQuestion(id string, update func(domain.HackathonQuestion) (domain.HackathonQuestion, error)) (domain.HackathonQuestion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	question, ok := s.questions[id]
	if !ok {
		return domain.HackathonQuestion{}, domain.ErrNotFound
	}
	updated, err := update(question)
	if err != nil {
		return domain.HackathonQuestion{}, err
	}
	if err := updated.Validate(); err != nil {
		return domain.HackathonQuestion{}, err
	}
	updated.ID = question.ID
	updated.EventID = question.EventID
	updated.Key = question.Key
	updated.UpdatedAt = time.Now().UTC()
	s.questions[id] = updated
	return updated, nil
}

func (s *Store) DeleteQuestion(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.questions[id]; !ok {
		return domain.ErrNotFound
	}
	delete(s.questions, id)
	return nil
}

func (s *Store) Questions(eventID string, audience domain.QuestionAudience) []domain.HackathonQuestion {
	s.mu.RLock()
	defer s.mu.RUnlock()
	questions := s.questionsForEventLocked(eventID, audience)
	domain.SortQuestions(questions)
	return questions
}

func (s *Store) questionsForEventLocked(eventID string, audience domain.QuestionAudience) []domain.HackathonQuestion {
	questions := make([]domain.HackathonQuestion, 0)
	for _, question := range s.questions {
		if question.EventID != eventID {
			continue
		}
		if audience != "" && question.Audience != audience {
			continue
		}
		questions = append(questions, question)
	}
	return questions
}

func (s *Store) ValidateAnswers(eventID string, audience domain.QuestionAudience, answers map[string]string) error {
	questions := s.Questions(eventID, audience)
	byKey := make(map[string]domain.HackathonQuestion, len(questions))
	for _, question := range questions {
		byKey[question.Key] = question
	}
	for key := range answers {
		if _, ok := byKey[key]; !ok {
			return domain.ErrValidation
		}
	}
	for _, question := range questions {
		if err := question.ValidateAnswer(answers[question.Key]); err != nil {
			return err
		}
	}
	return nil
}

func questionKey(prompt string) string {
	lowered := strings.ToLower(strings.TrimSpace(prompt))
	fields := strings.Fields(lowered)
	if len(fields) > 6 {
		fields = fields[:6]
	}
	return strings.Join(fields, "_")
}

func (s *Store) CreateMilestone(milestone domain.HackathonMilestone) (domain.HackathonMilestone, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if milestone.ID == "" {
		milestone.ID = domain.NewID("mil")
	}
	if _, ok := s.events[milestone.EventID]; !ok {
		return domain.HackathonMilestone{}, domain.ErrNotFound
	}
	if strings.TrimSpace(milestone.Title) == "" {
		return domain.HackathonMilestone{}, domain.ErrValidation
	}
	milestone.CreatedAt = time.Now().UTC()
	if milestone.Position <= 0 {
		milestone.Position = len(s.milestones) + 1
	}
	s.milestones[milestone.ID] = milestone
	return milestone, nil
}

func (s *Store) Milestones(eventID string) []domain.HackathonMilestone {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.HackathonMilestone, 0)
	for _, milestone := range s.milestones {
		if milestone.EventID == eventID {
			result = append(result, milestone)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Position < result[j].Position })
	return result
}

func (s *Store) AddHost(host domain.HackathonHost) (domain.HackathonHost, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if host.ID == "" {
		host.ID = domain.NewID("hst")
	}
	if _, ok := s.events[host.EventID]; !ok {
		return domain.HackathonHost{}, domain.ErrNotFound
	}
	if strings.TrimSpace(host.Name) == "" {
		return domain.HackathonHost{}, domain.ErrValidation
	}
	host.CreatedAt = time.Now().UTC()
	s.hosts[host.ID] = host
	return host, nil
}

func (s *Store) Hosts(eventID string) []domain.HackathonHost {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.HackathonHost, 0)
	for _, host := range s.hosts {
		if host.EventID == eventID {
			result = append(result, host)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func (s *Store) AddJudgeToRoster(entry domain.JudgeRosterEntry) (domain.JudgeRosterEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[entry.JudgeID]
	if !ok {
		return domain.JudgeRosterEntry{}, domain.ErrNotFound
	}
	if user.Role != domain.RoleJudge && user.Role != domain.RoleAdmin {
		return domain.JudgeRosterEntry{}, domain.ErrValidation
	}
	if entry.Scope == "" {
		entry.Scope = domain.JudgeScopeHackathon
	}
	if entry.Scope == domain.JudgeScopeHackathon {
		if _, ok := s.events[entry.EventID]; !ok {
			return domain.JudgeRosterEntry{}, domain.ErrNotFound
		}
	}
	entry.CreatedAt = time.Now().UTC()
	entry.Active = true
	s.roster[rosterKey(entry.EventID, entry.JudgeID)] = entry
	return entry, nil
}

func (s *Store) RemoveJudgeFromRoster(eventID, judgeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.roster[rosterKey(eventID, judgeID)]
	if !ok {
		return domain.ErrNotFound
	}
	entry.Active = false
	s.roster[rosterKey(eventID, judgeID)] = entry
	return nil
}

func (s *Store) JudgeRoster(eventID string) []domain.JudgeRosterEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.JudgeRosterEntry, 0)
	for _, entry := range s.roster {
		if entry.EventID != eventID || !entry.Active {
			continue
		}
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].JudgeID < result[j].JudgeID })
	return result
}

func (s *Store) JudgeOnRoster(eventID, judgeID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.roster[rosterKey(eventID, judgeID)]
	return ok && entry.Active
}

func (s *Store) GlobalJudges() []domain.JudgeRosterEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.JudgeRosterEntry, 0)
	for _, entry := range s.roster {
		if entry.Scope == domain.JudgeScopeGlobal && entry.Active {
			result = append(result, entry)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].JudgeID < result[j].JudgeID })
	return result
}

func rosterKey(eventID, judgeID string) string {
	return eventID + ":" + judgeID
}

func (s *Store) CreateInvite(invite domain.TeamInvite) (domain.TeamInvite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	team, ok := s.teams[invite.TeamID]
	if !ok {
		return domain.TeamInvite{}, domain.ErrNotFound
	}
	if team.Status != domain.TeamStatusActive {
		return domain.TeamInvite{}, domain.ErrConflict
	}
	if strings.TrimSpace(invite.InviterID) == "" {
		return domain.TeamInvite{}, domain.ErrValidation
	}
	if _, ok := s.memberships[membershipKey(invite.TeamID, invite.InviterID)]; !ok {
		return domain.TeamInvite{}, domain.ErrForbidden
	}
	if invite.InviteeID == "" && strings.TrimSpace(invite.InviteeMail) == "" {
		return domain.TeamInvite{}, domain.ErrValidation
	}
	if invite.InviteeID != "" {
		if _, ok := s.users[invite.InviteeID]; !ok {
			return domain.TeamInvite{}, domain.ErrNotFound
		}
		if _, ok := s.memberships[membershipKey(invite.TeamID, invite.InviteeID)]; ok {
			return domain.TeamInvite{}, domain.ErrAlreadyExists
		}
	}
	if invite.Role == "" {
		invite.Role = domain.TeamRoleMember
	}
	if invite.Role != domain.TeamRoleMember {
		return domain.TeamInvite{}, domain.ErrValidation
	}
	if invite.Status == "" {
		invite.Status = domain.InvitePending
	}
	if invite.ID == "" {
		invite.ID = domain.NewID("inv")
	}
	invite.CreatedAt = time.Now().UTC()
	if invite.ExpiresAt.IsZero() {
		invite.ExpiresAt = invite.CreatedAt.Add(14 * 24 * time.Hour)
	}
	s.invites[invite.ID] = invite.Clone()
	return invite.Clone(), nil
}

func (s *Store) InvitesForUser(userID string) []domain.TeamInvite {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.TeamInvite, 0)
	for _, invite := range s.invites {
		if invite.InviteeID == userID || strings.EqualFold(invite.InviteeMail, s.userMailLocked(userID)) {
			result = append(result, invite.Clone())
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result
}

func (s *Store) InvitesForTeam(teamID string) []domain.TeamInvite {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.TeamInvite, 0)
	for _, invite := range s.invites {
		if invite.TeamID == teamID {
			result = append(result, invite.Clone())
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result
}

func (s *Store) InviteByID(id string) (domain.TeamInvite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	invite, ok := s.invites[id]
	if !ok {
		return domain.TeamInvite{}, domain.ErrNotFound
	}
	return invite.Clone(), nil
}

func (s *Store) InviteTargetsUser(invite domain.TeamInvite, userID, email string) bool {
	if invite.InviteeID != "" {
		return invite.InviteeID == userID
	}
	return strings.EqualFold(invite.InviteeMail, email)
}

func (s *Store) RespondToInvite(id, userID string, status domain.InviteStatus) (domain.TeamInvite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	invite, ok := s.invites[id]
	if !ok {
		return domain.TeamInvite{}, domain.ErrNotFound
	}
	if invite.Status != domain.InvitePending {
		return domain.TeamInvite{}, domain.ErrConflict
	}
	if invite.Expired(time.Now().UTC()) {
		invite.Status = domain.InviteExpired
		s.invites[id] = invite
		return domain.TeamInvite{}, domain.ErrConflict
	}
	if status != domain.InviteAccepted && status != domain.InviteDeclined {
		return domain.TeamInvite{}, domain.ErrValidation
	}
	now := time.Now().UTC()
	invite.Status = status
	invite.RespondedAt = &now
	if status == domain.InviteAccepted {
		team, ok := s.teams[invite.TeamID]
		if !ok {
			return domain.TeamInvite{}, domain.ErrNotFound
		}
		if team.Status != domain.TeamStatusActive {
			return domain.TeamInvite{}, domain.ErrConflict
		}
		if team.MaxSize > 0 && len(s.teamMembersLocked(invite.TeamID)) >= team.MaxSize {
			return domain.TeamInvite{}, domain.ErrConflict
		}
		if _, exists := s.memberships[membershipKey(invite.TeamID, userID)]; exists {
			return domain.TeamInvite{}, domain.ErrAlreadyExists
		}
		eventID := invite.EventID
		if eventID == "" {
			eventID = team.EventID
		}
		if eventID == "" {
			eventID = team.EventID
		}
		if err := s.addMembershipLocked(domain.TeamMembership{
			TeamID:   invite.TeamID,
			UserID:   userID,
			Role:     domain.TeamRoleMember,
			EventID:  eventID,
			Status:   "active",
			JoinedAt: now,
		}); err != nil {
			return domain.TeamInvite{}, err
		}
	}
	s.invites[id] = invite
	return invite.Clone(), nil
}

func (s *Store) RevokeInvite(id string, actorID string) (domain.TeamInvite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	invite, ok := s.invites[id]
	if !ok {
		return domain.TeamInvite{}, domain.ErrNotFound
	}
	if invite.InviterID != actorID {
		return domain.TeamInvite{}, domain.ErrForbidden
	}
	if invite.Status != domain.InvitePending {
		return domain.TeamInvite{}, domain.ErrConflict
	}
	now := time.Now().UTC()
	invite.Status = domain.InviteRevoked
	invite.RespondedAt = &now
	s.invites[id] = invite
	return invite.Clone(), nil
}

func (s *Store) userMailLocked(userID string) string {
	if user, ok := s.users[userID]; ok {
		return user.Email
	}
	return ""
}

func (s *Store) teamMembersLocked(teamID string) []domain.TeamMembership {
	members := make([]domain.TeamMembership, 0)
	for _, membership := range s.memberships {
		if membership.TeamID == teamID {
			members = append(members, membership)
		}
	}
	sort.Slice(members, func(i, j int) bool { return members[i].UserID < members[j].UserID })
	return members
}

func (s *Store) RecordParticipation(participation domain.Participation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := participationKey(participation.UserID, participation.EventID)
	if _, ok := s.participations[key]; ok {
		return
	}
	if participation.CreatedAt.IsZero() {
		participation.CreatedAt = time.Now().UTC()
	}
	s.participations[key] = participation
}

func (s *Store) Participations(userID, eventID string) []domain.Participation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.Participation, 0)
	for _, participation := range s.participations {
		if userID != "" && participation.UserID != userID {
			continue
		}
		if eventID != "" && participation.EventID != eventID {
			continue
		}
		result = append(result, participation)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].EventID == result[j].EventID {
			return result[i].UserID < result[j].UserID
		}
		return result[i].EventID > result[j].EventID
	})
	return result
}

func participationKey(userID, eventID string) string {
	return userID + ":" + eventID
}

func (s *Store) TeamOpportunities(eventID string) []domain.TeamOpportunity {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.TeamOpportunity, 0)
	for _, team := range s.teams {
		if team.Status != domain.TeamStatusActive {
			continue
		}
		if eventID != "" && team.Scope == domain.TeamScopeHackathon && team.EventID != eventID {
			continue
		}
		if team.Availability != domain.TeamOpenForMembers {
			continue
		}
		opportunity := domain.TeamOpportunity{Team: team, OpenRoles: append([]string(nil), team.OpenRoles...)}
		if event, err := s.eventLocked(team.EventID); err == nil {
			opportunity.Event = event
		}
		for _, membership := range s.teamMembersLocked(team.ID) {
			view := domain.TeamMemberView{UserID: membership.UserID, Role: string(membership.Role)}
			if user, ok := s.users[membership.UserID]; ok {
				view.DisplayName = user.DisplayName
				view.AvatarURL = user.AvatarURL
			}
			if profile, ok := s.profiles[membership.UserID]; ok {
				view.Headline = profile.Headline
			}
			opportunity.Members = append(opportunity.Members, view)
		}
		result = append(result, opportunity)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Team.Name < result[j].Team.Name })
	return result
}

func (s *Store) eventLocked(id string) (domain.Event, error) {
	event, ok := s.events[id]
	return event, ok2(ok)
}

func ok2(value bool) error {
	if value {
		return nil
	}
	return domain.ErrNotFound
}

func (s *Store) addMembershipLocked(membership domain.TeamMembership) error {
	if !membership.Role.Valid() {
		return domain.ErrValidation
	}
	if membership.TeamID == "" || membership.UserID == "" || membership.EventID == "" {
		return domain.ErrValidation
	}
	if _, ok := s.teams[membership.TeamID]; !ok {
		return domain.ErrNotFound
	}
	if _, ok := s.users[membership.UserID]; !ok {
		return domain.ErrNotFound
	}
	if membership.ID == "" {
		membership.ID = domain.NewID("tmem")
	}
	if membership.JoinedAt.IsZero() {
		membership.JoinedAt = time.Now().UTC()
	}
	membership.UpdatedAt = time.Now().UTC()
	if membership.Status == "" {
		membership.Status = "active"
	}
	s.memberships[membershipKey(membership.TeamID, membership.UserID)] = membership
	return nil
}

func (s *Store) AddMembershipRecord(userID, teamID, eventID string, role domain.ParticipationRole) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	team, ok := s.teams[teamID]
	if !ok {
		return domain.ErrNotFound
	}
	if eventID == "" {
		eventID = team.EventID
	}
	s.rebuildParticipationLocked(userID, team, role)
	return nil
}
