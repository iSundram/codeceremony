package store

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

func (s *Store) CreateComment(comment domain.Comment) (domain.Comment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := comment.Validate(); err != nil {
		return domain.Comment{}, err
	}
	if _, ok := s.submissions[comment.ProjectID]; !ok {
		return domain.Comment{}, domain.ErrNotFound
	}
	if _, ok := s.events[comment.EventID]; !ok {
		return domain.Comment{}, domain.ErrNotFound
	}
	if _, ok := s.users[comment.AuthorID]; !ok {
		return domain.Comment{}, domain.ErrNotFound
	}
	if comment.ParentID != "" {
		parent, ok := s.comments[comment.ParentID]
		if !ok || parent.ProjectID != comment.ProjectID {
			return domain.Comment{}, domain.ErrValidation
		}
	}
	authorCount := 0
	for _, existing := range s.comments {
		if existing.ProjectID == comment.ProjectID && existing.AuthorID == comment.AuthorID && existing.Status == domain.CommentVisible {
			authorCount++
		}
	}
	if authorCount >= maxCommentsPerAuthorPerProject {
		return domain.Comment{}, domain.ErrConflict
	}
	now := time.Now().UTC()
	if comment.ID == "" {
		comment.ID = domain.NewID("cmt")
	}
	if comment.Status == "" {
		comment.Status = domain.CommentVisible
	}
	comment.CreatedAt = now
	comment.UpdatedAt = now
	comment.AuthorName = s.users[comment.AuthorID].DisplayName
	s.comments[comment.ID] = comment
	return comment, nil
}

const maxCommentsPerAuthorPerProject = 5

func (s *Store) Comments(projectID string, includeHidden bool) []domain.Comment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.Comment, 0)
	replyCounts := map[string]int{}
	for _, comment := range s.comments {
		if comment.ParentID != "" {
			replyCounts[comment.ParentID]++
		}
	}
	for _, comment := range s.comments {
		if projectID != "" && comment.ProjectID != projectID {
			continue
		}
		if !includeHidden && comment.Status != domain.CommentVisible {
			continue
		}
		comment.ReplyCount = replyCounts[comment.ID]
		result = append(result, comment)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result
}

func (s *Store) CommentByID(id string) (domain.Comment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	comment, ok := s.comments[id]
	if !ok {
		return domain.Comment{}, domain.ErrNotFound
	}
	return comment, nil
}

func (s *Store) ModerateComment(id string, status domain.CommentStatus, note, actorID string) (domain.Comment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	comment, ok := s.comments[id]
	if !ok {
		return domain.Comment{}, domain.ErrNotFound
	}
	if status != domain.CommentHidden && status != domain.CommentVisible && status != domain.CommentDeleted {
		return domain.Comment{}, domain.ErrValidation
	}
	if status != domain.CommentVisible && strings.TrimSpace(note) == "" {
		return domain.Comment{}, domain.ErrValidation
	}
	now := time.Now().UTC()
	comment.Status = status
	comment.ModerationNote = strings.TrimSpace(note)
	comment.ModeratedBy = actorID
	comment.ModeratedAt = &now
	comment.UpdatedAt = now
	s.comments[id] = comment
	return comment, nil
}

func (s *Store) CreateReport(report domain.CommentReport) (domain.CommentReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := report.Validate(); err != nil {
		return domain.CommentReport{}, err
	}
	comment, ok := s.comments[report.CommentID]
	if !ok {
		return domain.CommentReport{}, domain.ErrNotFound
	}
	report.EventID = comment.EventID
	if _, ok := s.users[report.ReporterID]; !ok {
		return domain.CommentReport{}, domain.ErrNotFound
	}
	for _, existing := range s.reports {
		if existing.CommentID == report.CommentID && existing.ReporterID == report.ReporterID && existing.Status == domain.ReportOpen {
			return domain.CommentReport{}, domain.ErrAlreadyExists
		}
	}
	if report.ID == "" {
		report.ID = domain.NewID("rep")
	}
	if report.Status == "" {
		report.Status = domain.ReportOpen
	}
	report.CreatedAt = time.Now().UTC()
	s.reports[report.ID] = report
	return report, nil
}

func (s *Store) Reports(eventID string, status domain.ReportStatus) []domain.CommentReport {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.CommentReport, 0)
	for _, report := range s.reports {
		if eventID != "" && report.EventID != eventID {
			continue
		}
		if status != "" && report.Status != status {
			continue
		}
		result = append(result, report)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result
}

func (s *Store) ResolveReport(id string, status domain.ReportStatus, note, actorID string) (domain.CommentReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	report, ok := s.reports[id]
	if !ok {
		return domain.CommentReport{}, domain.ErrNotFound
	}
	if status != domain.ReportActioned && status != domain.ReportDismissed {
		return domain.CommentReport{}, domain.ErrValidation
	}
	if strings.TrimSpace(note) == "" {
		return domain.CommentReport{}, domain.ErrValidation
	}
	now := time.Now().UTC()
	report.Status = status
	report.ResolutionNote = strings.TrimSpace(note)
	report.ResolvedBy = actorID
	report.ResolvedAt = &now
	s.reports[id] = report
	return report, nil
}

func (s *Store) CreateCampaign(campaign domain.VoteCampaign) (domain.VoteCampaign, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if campaign.ID == "" {
		campaign.ID = domain.NewID("cmp")
	}
	if _, ok := s.events[campaign.EventID]; !ok {
		return domain.VoteCampaign{}, domain.ErrNotFound
	}
	if err := campaign.Validate(); err != nil {
		return domain.VoteCampaign{}, err
	}
	if campaign.Status == "" {
		campaign.Status = domain.CampaignDraft
	}
	campaign.CreatedAt = time.Now().UTC()
	s.campaigns[campaign.ID] = campaign
	return campaign, nil
}

func (s *Store) UpdateCampaign(id string, update func(domain.VoteCampaign) (domain.VoteCampaign, error)) (domain.VoteCampaign, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	campaign, ok := s.campaigns[id]
	if !ok {
		return domain.VoteCampaign{}, domain.ErrNotFound
	}
	updated, err := update(campaign)
	if err != nil {
		return domain.VoteCampaign{}, err
	}
	if err := updated.Validate(); err != nil {
		return domain.VoteCampaign{}, err
	}
	updated.ID = campaign.ID
	updated.EventID = campaign.EventID
	updated.CreatedBy = campaign.CreatedBy
	updated.CreatedAt = campaign.CreatedAt
	updated.BallotCount = campaign.BallotCount
	s.campaigns[id] = updated
	return updated, nil
}

func (s *Store) SetCampaignStatus(id string, status domain.CampaignStatus) (domain.VoteCampaign, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	campaign, ok := s.campaigns[id]
	if !ok {
		return domain.VoteCampaign{}, domain.ErrNotFound
	}
	switch status {
	case domain.CampaignOpen:
		if campaign.Status != domain.CampaignDraft {
			return domain.VoteCampaign{}, domain.ErrConflict
		}
		campaign.Status = domain.CampaignOpen
		now := time.Now().UTC()
		campaign.OpensAt = &now
	case domain.CampaignClosed:
		if campaign.Status != domain.CampaignOpen {
			return domain.VoteCampaign{}, domain.ErrConflict
		}
		campaign.Status = domain.CampaignClosed
		now := time.Now().UTC()
		campaign.ClosedAt = &now
	default:
		return domain.VoteCampaign{}, domain.ErrValidation
	}
	s.campaigns[id] = campaign
	return campaign, nil
}

func (s *Store) Campaigns(eventID string) []domain.VoteCampaign {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.VoteCampaign, 0)
	for _, campaign := range s.campaigns {
		if eventID != "" && campaign.EventID != eventID {
			continue
		}
		campaign.BallotCount = s.ballotCountLocked(campaign.ID)
		result = append(result, campaign)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result
}

func (s *Store) CampaignByID(id string) (domain.VoteCampaign, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	campaign, ok := s.campaigns[id]
	if !ok {
		return domain.VoteCampaign{}, domain.ErrNotFound
	}
	campaign.BallotCount = s.ballotCountLocked(campaign.ID)
	return campaign, nil
}

func (s *Store) CastBallot(campaignID, userID string, projectIDs []string) ([]domain.Ballot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	campaign, ok := s.campaigns[campaignID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	open, reason := campaign.VotingOpen(time.Now().UTC())
	if !open {
		return nil, fmt.Errorf("%w: %s", domain.ErrConflict, reason)
	}
	if len(projectIDs) == 0 {
		return nil, domain.ErrValidation
	}
	if len(projectIDs) > campaign.MaxChoicesPerUser {
		return nil, fmt.Errorf("%w: at most %d projects can be chosen", domain.ErrValidation, campaign.MaxChoicesPerUser)
	}
	unique := make(map[string]struct{}, len(projectIDs))
	for _, projectID := range projectIDs {
		projectID = strings.TrimSpace(projectID)
		if projectID == "" {
			return nil, domain.ErrValidation
		}
		if _, seen := unique[projectID]; seen {
			return nil, fmt.Errorf("%w: a project can only be chosen once", domain.ErrValidation)
		}
		project, ok := s.submissions[projectID]
		if !ok || project.EventID != campaign.EventID {
			return nil, domain.ErrNotFound
		}
		if campaign.RequireEligible && project.Eligibility == domain.EligibilityIneligible {
			return nil, domain.ErrValidation
		}
		unique[projectID] = struct{}{}
	}
	existing := s.ballotsByUserLocked(campaignID, userID)
	if len(existing)+len(unique) > campaign.MaxChoicesPerUser {
		return nil, fmt.Errorf("%w: you have already used your %d choices", domain.ErrConflict, campaign.MaxChoicesPerUser)
	}
	now := time.Now().UTC()
	created := make([]domain.Ballot, 0, len(unique))
	for projectID := range unique {
		ballot := domain.Ballot{ID: domain.NewID("bal"), CampaignID: campaignID, UserID: userID, ProjectID: projectID, CreatedAt: now}
		s.ballots[ballot.ID] = ballot
		created = append(created, ballot)
	}
	sort.Slice(created, func(i, j int) bool { return created[i].ProjectID < created[j].ProjectID })
	return created, nil
}

func (s *Store) BallotsForUser(campaignID, userID string) []domain.Ballot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ballotsByUserLocked(campaignID, userID)
}

func (s *Store) ballotsByUserLocked(campaignID, userID string) []domain.Ballot {
	result := make([]domain.Ballot, 0)
	for _, ballot := range s.ballots {
		if ballot.CampaignID == campaignID && ballot.UserID == userID {
			result = append(result, ballot)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ProjectID < result[j].ProjectID })
	return result
}

func (s *Store) ballotCountLocked(campaignID string) int {
	count := 0
	for _, ballot := range s.ballots {
		if ballot.CampaignID == campaignID {
			count++
		}
	}
	return count
}

func (s *Store) CampaignResult(campaignID string) (domain.CampaignResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	campaign, ok := s.campaigns[campaignID]
	if !ok {
		return domain.CampaignResult{}, domain.ErrNotFound
	}
	campaign.BallotCount = s.ballotCountLocked(campaignID)
	tallies := map[string]int{}
	voters := map[string]struct{}{}
	for _, ballot := range s.ballots {
		if ballot.CampaignID != campaignID {
			continue
		}
		tallies[ballot.ProjectID]++
		voters[ballot.UserID] = struct{}{}
	}
	results := make([]domain.VoteResult, 0, len(tallies))
	for projectID, votes := range tallies {
		result := domain.VoteResult{ProjectID: projectID, Votes: votes}
		if project, ok := s.submissions[projectID]; ok {
			result.Title = project.Title
			result.TeamID = project.TeamID
		}
		results = append(results, result)
	}
	domain.SortVoteResults(results)
	return domain.CampaignResult{
		Campaign:   campaign,
		Results:    results,
		Voters:     len(voters),
		Closed:     campaign.Status == domain.CampaignClosed,
		Tiebreaker: "equal votes are ordered by project title, then project id, so results are deterministic",
	}, nil
}

func (s *Store) CreateWebhook(webhook domain.Webhook) (domain.Webhook, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if webhook.ID == "" {
		webhook.ID = domain.NewID("whk")
	}
	if _, ok := s.events[webhook.EventID]; !ok {
		return domain.Webhook{}, domain.ErrNotFound
	}
	if err := webhook.Validate(); err != nil {
		return domain.Webhook{}, err
	}
	webhook.CreatedAt = time.Now().UTC()
	webhook.Active = true
	s.webhooks[webhook.ID] = webhook
	return webhook, nil
}

func (s *Store) Webhooks(eventID string) []domain.Webhook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.Webhook, 0)
	for _, webhook := range s.webhooks {
		if eventID != "" && webhook.EventID != eventID {
			continue
		}
		result = append(result, webhook)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result
}

func (s *Store) WebhookByID(id string) (domain.Webhook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	webhook, ok := s.webhooks[id]
	if !ok {
		return domain.Webhook{}, domain.ErrNotFound
	}
	return webhook, nil
}

func (s *Store) DeactivateWebhook(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	webhook, ok := s.webhooks[id]
	if !ok {
		return domain.ErrNotFound
	}
	webhook.Active = false
	s.webhooks[id] = webhook
	return nil
}

func (s *Store) EnqueueDelivery(delivery domain.WebhookDelivery) (domain.WebhookDelivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if delivery.ID == "" {
		delivery.ID = domain.NewID("dlv")
	}
	if delivery.Status == "" {
		delivery.Status = domain.DeliveryPending
	}
	if delivery.CreatedAt.IsZero() {
		delivery.CreatedAt = time.Now().UTC()
	}
	if delivery.NextAttempt.IsZero() {
		delivery.NextAttempt = delivery.CreatedAt
	}
	s.deliveries[delivery.ID] = delivery
	return delivery, nil
}

func (s *Store) ClaimDeliveries(now time.Time, limit int) []domain.WebhookDelivery {
	s.mu.Lock()
	defer s.mu.Unlock()
	claimed := make([]domain.WebhookDelivery, 0, limit)
	for id, delivery := range s.deliveries {
		if len(claimed) >= limit {
			break
		}
		if delivery.Status != domain.DeliveryPending || delivery.NextAttempt.After(now) {
			continue
		}
		delivery.Attempts++
		s.deliveries[id] = delivery
		claimed = append(claimed, delivery)
	}
	sort.Slice(claimed, func(i, j int) bool { return claimed[i].NextAttempt.Before(claimed[j].NextAttempt) })
	return claimed
}

func (s *Store) MarkDelivery(id string, delivery domain.WebhookDelivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.deliveries[id]; !ok {
		return domain.ErrNotFound
	}
	if webhook, ok := s.webhooks[delivery.WebhookID]; ok {
		if delivery.Status == domain.DeliveryDelivered {
			webhook.DeliveryCount++
			webhook.FailureCount = 0
			now := time.Now().UTC()
			webhook.LastDeliveryAt = &now
		} else {
			webhook.FailureCount++
		}
		s.webhooks[delivery.WebhookID] = webhook
	}
	s.deliveries[id] = delivery
	return nil
}

func (s *Store) Deliveries(webhookID string, limit int) []domain.WebhookDelivery {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.WebhookDelivery, 0)
	for _, delivery := range s.deliveries {
		if webhookID != "" && delivery.WebhookID != webhookID {
			continue
		}
		result = append(result, delivery)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result
}
