package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/domain"
)

func (s *Server) listComments(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if projectID == "" {
		projectID = strings.TrimSpace(r.URL.Query().Get("project_id"))
	}
	includeHidden := s.isStaff(r)
	comments := s.store.Comments(projectID, includeHidden)
	views := make([]domain.Comment, 0, len(comments))
	for _, comment := range comments {
		if includeHidden {
			views = append(views, comment)
			continue
		}
		views = append(views, comment.Public())
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": views, "count": len(views), "project_id": projectID})
}

func (s *Server) postComment(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	projectID := r.PathValue("projectID")
	project, err := s.store.SubmissionByID(projectID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "project not found")
		return
	}
	var request struct {
		Body     string `json:"body"`
		ParentID string `json:"parent_id"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if strings.TrimSpace(request.Body) == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "a comment body is required")
		return
	}
	comment, err := s.store.CreateComment(domain.Comment{
		EventID:   project.EventID,
		ProjectID: projectID,
		AuthorID:  principal.UserID,
		Body:      strings.TrimSpace(request.Body),
		ParentID:  strings.TrimSpace(request.ParentID),
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrConflict):
			writeError(w, http.StatusConflict, "comment_limit", "you have already posted five comments on this project")
		case errors.Is(err, domain.ErrValidation):
			writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		default:
			writeError(w, http.StatusNotFound, "not_found", "project or parent comment not found")
		}
		return
	}
	s.recordActivity(r, principal.UserID, domain.ActivityCommunication, "comment.posted", "submission", projectID, project.EventID,
		"a new comment was posted on "+project.Title, domain.ActivityPublic, map[string]any{"comment_id": comment.ID})
	s.webhooks.Emit(project.EventID, "comment.posted", s.slugFor(project.EventID), map[string]any{
		"comment_id": comment.ID, "project_id": projectID, "author_id": principal.UserID,
	})
	s.audit(r, principal.UserID, "comment.posted", "comment", comment.ID, project.EventID, "", nil)
	writeJSON(w, http.StatusCreated, map[string]any{"data": comment})
}

func (s *Server) deleteComment(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	comment, err := s.store.CommentByID(r.PathValue("commentID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "comment not found")
		return
	}
	if comment.AuthorID != principal.UserID && !s.isStaff(r) {
		writeError(w, http.StatusForbidden, "forbidden", "only the author or an organizer can remove this comment")
		return
	}
	updated, err := s.store.ModerateComment(comment.ID, domain.CommentDeleted, "removed by "+principal.UserID, principal.UserID)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "the comment could not be removed")
		return
	}
	s.audit(r, principal.UserID, "comment.deleted", "comment", comment.ID, comment.EventID, "comment removed", nil)
	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

func (s *Server) moderateComment(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	var request struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	comment, err := s.store.ModerateComment(r.PathValue("commentID"), domain.CommentStatus(strings.TrimSpace(request.Status)), request.Note, principal.UserID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "comment not found")
		default:
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "status must be hidden, visible, or deleted, and hiding or deleting needs a note")
		}
		return
	}
	s.recordActivity(r, principal.UserID, domain.ActivityCommunication, "comment.moderated", "comment", comment.ID, comment.EventID,
		"a comment was marked "+string(comment.Status), domain.ActivityOrganizers, map[string]any{"note": request.Note})
	writeJSON(w, http.StatusOK, map[string]any{"data": comment})
}

func (s *Server) reportComment(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	var request struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	report, err := s.store.CreateReport(domain.CommentReport{
		CommentID:  r.PathValue("commentID"),
		ReporterID: principal.UserID,
		Reason:     strings.TrimSpace(request.Reason),
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrAlreadyExists):
			writeError(w, http.StatusConflict, "already_reported", "you already reported this comment")
		case errors.Is(err, domain.ErrValidation):
			writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		default:
			writeError(w, http.StatusNotFound, "not_found", "comment not found")
		}
		return
	}
	s.audit(r, principal.UserID, "comment.reported", "comment", report.CommentID, report.EventID, request.Reason, nil)
	writeJSON(w, http.StatusCreated, map[string]any{"data": report})
}

func (s *Server) listReports(w http.ResponseWriter, r *http.Request) {
	eventID := strings.TrimSpace(r.URL.Query().Get("event_id"))
	if eventID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "event_id or event_slug is required")
		return
	}
	reports := s.store.Reports(eventID, domain.ReportStatus(strings.TrimSpace(r.URL.Query().Get("status"))))
	writeJSON(w, http.StatusOK, map[string]any{"data": reports, "count": len(reports), "event_id": eventID})
}

func (s *Server) resolveReport(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	var request struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	report, err := s.store.ResolveReport(r.PathValue("reportID"), domain.ReportStatus(strings.TrimSpace(request.Status)), request.Note, principal.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "report not found")
			return
		}
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "status must be actioned or dismissed and a note is required")
		return
	}
	s.audit(r, principal.UserID, "comment.report_"+string(report.Status), "comment", report.CommentID, report.EventID, request.Note, nil)
	writeJSON(w, http.StatusOK, map[string]any{"data": report})
}

func (s *Server) listCampaigns(w http.ResponseWriter, r *http.Request) {
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	campaigns := s.store.Campaigns(event.ID)
	writeJSON(w, http.StatusOK, map[string]any{"data": campaigns, "count": len(campaigns)})
}

func (s *Server) createCampaign(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	var request struct {
		Name              string `json:"name"`
		Description       string `json:"description"`
		MaxChoicesPerUser int    `json:"max_choices_per_user"`
		OpensAt           string `json:"opens_at"`
		ClosesAt          string `json:"closes_at"`
		RequireEligible   bool   `json:"require_eligible"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if request.MaxChoicesPerUser == 0 {
		request.MaxChoicesPerUser = 3
	}
	campaign := domain.VoteCampaign{
		EventID:           event.ID,
		Name:              strings.TrimSpace(request.Name),
		Description:       strings.TrimSpace(request.Description),
		MaxChoicesPerUser: request.MaxChoicesPerUser,
		RequireEligible:   request.RequireEligible,
		CreatedBy:         principal.UserID,
	}
	if strings.TrimSpace(request.OpensAt) != "" {
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(request.OpensAt)); err == nil {
			campaign.OpensAt = &parsed
		}
	}
	if strings.TrimSpace(request.ClosesAt) != "" {
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(request.ClosesAt)); err == nil {
			campaign.ClosesAt = &parsed
		}
	}
	created, err := s.store.CreateCampaign(campaign)
	if err != nil {
		if errors.Is(err, domain.ErrValidation) {
			writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	s.recordActivity(r, principal.UserID, domain.ActivityResults, "vote.campaign_created", "vote_campaign", created.ID, event.ID,
		"a community vote was created: "+created.Name, domain.ActivityParticipants, map[string]any{"max_choices": created.MaxChoicesPerUser})
	writeJSON(w, http.StatusCreated, map[string]any{"data": created})
}

func (s *Server) setCampaignStatus(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	campaign, err := s.store.SetCampaignStatus(r.PathValue("campaignID"), domain.CampaignStatus(strings.TrimSpace(r.PathValue("status"))))
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrConflict):
			writeError(w, http.StatusConflict, "invalid_state", "a campaign can only move from draft to open and from open to closed")
		case errors.Is(err, domain.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "campaign not found")
		default:
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "status must be open or closed")
		}
		return
	}
	if campaign.Status == domain.CampaignClosed {
		result, _ := s.store.CampaignResult(campaign.ID)
		if len(result.Results) > 0 {
			s.recordActivity(r, principal.UserID, domain.ActivityResults, "vote.closed", "vote_campaign", campaign.ID, campaign.EventID,
				"community voting closed with "+strconv.Itoa(result.Voters)+" voters", domain.ActivityPublic, map[string]any{"voters": result.Voters})
			s.webhooks.Emit(campaign.EventID, "vote.closed", s.slugFor(campaign.EventID), map[string]any{
				"campaign_id": campaign.ID, "voters": result.Voters, "results": result.Results,
			})
			if user, err := s.store.UserByID(principal.UserID); err == nil {
				if message, err := s.mailService.SendTo(campaign.CreatedBy, "results_published", map[string]any{
					"HackathonName": s.nameFor(campaign.EventID), "HackathonSlug": s.slugFor(campaign.EventID), "Placement": "",
				}, "vote_closed:"+campaign.ID); err == nil && message.Status != domain.MailSkipped {
					_ = user
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": campaign})
}

func (s *Server) castVotes(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	var request struct {
		CampaignID string   `json:"campaign_id"`
		ProjectIDs []string `json:"project_ids"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if strings.TrimSpace(request.CampaignID) == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "campaign_id is required")
		return
	}
	ballots, err := s.store.CastBallot(request.CampaignID, principal.UserID, request.ProjectIDs)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrConflict):
			writeError(w, http.StatusConflict, "vote_rejected", strings.TrimPrefix(err.Error(), "conflict: "))
		case errors.Is(err, domain.ErrValidation):
			writeError(w, http.StatusUnprocessableEntity, "validation_error", strings.TrimPrefix(err.Error(), "validation error: "))
		default:
			writeError(w, http.StatusNotFound, "not_found", "campaign or project not found")
		}
		return
	}
	s.audit(r, principal.UserID, "vote.cast", "vote_campaign", request.CampaignID, "", "", map[string]any{"projects": len(ballots)})
	writeJSON(w, http.StatusCreated, map[string]any{"data": ballots, "count": len(ballots)})
}

func (s *Server) voteResults(w http.ResponseWriter, r *http.Request) {
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	campaignID := strings.TrimSpace(r.URL.Query().Get("campaign_id"))
	if campaignID == "" {
		campaigns := s.store.Campaigns(event.ID)
		if len(campaigns) == 0 {
			writeError(w, http.StatusNotFound, "not_found", "this hackathon has no community votes")
			return
		}
		campaignID = campaigns[len(campaigns)-1].ID
	}
	result, err := s.store.CampaignResult(campaignID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "campaign not found")
		return
	}
	isStaff := s.isStaff(r)
	principal, _ := auth.PrincipalFromContext(r.Context())
	showResults := isStaff || result.Closed || event.LeaderboardPublic
	if !showResults {
		writeError(w, http.StatusForbidden, "forbidden", "vote results are not public yet")
		return
	}
	payload := map[string]any{
		"data":       result,
		"count":      len(result.Results),
		"is_staff":   isStaff,
		"tiebreaker": result.Tiebreaker,
	}
	if principal.UserID != "" {
		mine := s.store.BallotsForUser(campaignID, principal.UserID)
		payload["my_votes"] = mine
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) myVotes(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	campaignID := strings.TrimSpace(r.URL.Query().Get("campaign_id"))
	if campaignID == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "campaign_id is required")
		return
	}
	ballots := s.store.BallotsForUser(campaignID, principal.UserID)
	writeJSON(w, http.StatusOK, map[string]any{"data": ballots, "count": len(ballots), "campaign_id": campaignID})
}

func (s *Server) listWebhooks(w http.ResponseWriter, r *http.Request) {
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	webhooks := s.store.Webhooks(event.ID)
	views := make([]map[string]any, 0, len(webhooks))
	for _, webhook := range webhooks {
		views = append(views, map[string]any{
			"id":               webhook.ID,
			"url":              webhook.URL,
			"events":           webhook.Events,
			"active":           webhook.Active,
			"created_at":       webhook.CreatedAt,
			"delivery_count":   webhook.DeliveryCount,
			"failure_count":    webhook.FailureCount,
			"last_delivery_at": webhook.LastDeliveryAt,
			"secret_set":       webhook.Secret != "",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":        views,
		"count":       len(views),
		"event_types": domain.WebhookEventNames(),
		"secret_note": "signing secrets are write-only and never returned",
	})
}

func (s *Server) createWebhook(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	event, err := s.store.EventBySlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	var request struct {
		URL    string   `json:"url"`
		Secret string   `json:"secret"`
		Events []string `json:"events"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	webhook, err := s.store.CreateWebhook(domain.Webhook{
		EventID:   event.ID,
		URL:       strings.TrimSpace(request.URL),
		Secret:    strings.TrimSpace(request.Secret),
		Events:    request.Events,
		CreatedBy: principal.UserID,
	})
	if err != nil {
		if errors.Is(err, domain.ErrValidation) {
			writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "hackathon not found")
		return
	}
	s.audit(r, principal.UserID, "webhook.created", "webhook", webhook.ID, event.ID, webhook.URL, map[string]any{"events": webhook.Events})
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
		"id": webhook.ID, "url": webhook.URL, "events": webhook.Events, "active": webhook.Active, "created_at": webhook.CreatedAt,
	}})
}

func (s *Server) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	webhook, err := s.store.WebhookByID(r.PathValue("webhookID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "webhook not found")
		return
	}
	if err := s.store.DeactivateWebhook(webhook.ID); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "webhook not found")
		return
	}
	s.audit(r, principal.UserID, "webhook.deactivated", "webhook", webhook.ID, webhook.EventID, "webhook deactivated", nil)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": webhook.ID, "active": false}})
}

func (s *Server) testWebhook(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	webhook, err := s.store.WebhookByID(r.PathValue("webhookID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "webhook not found")
		return
	}
	event := "submission.created"
	if len(webhook.Events) > 0 {
		event = webhook.Events[0]
	}
	deliveries := s.webhooks.Emit(webhook.EventID, event, s.slugFor(webhook.EventID), map[string]any{
		"test": true, "requested_by": principal.UserID, "webhook_id": webhook.ID,
	})
	delivered, err := s.webhooks.DeliverOnce(1)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "the test delivery could not be attempted")
		return
	}
	stored := s.store.Deliveries(webhook.ID, 1)
	payload := map[string]any{
		"queued":    len(deliveries),
		"delivered": delivered,
		"event":     event,
	}
	if len(stored) > 0 {
		payload["delivery"] = stored[0]
	}
	s.audit(r, principal.UserID, "webhook.tested", "webhook", webhook.ID, webhook.EventID, event, nil)
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) listDeliveries(w http.ResponseWriter, r *http.Request) {
	webhookID := strings.TrimSpace(r.URL.Query().Get("webhook_id"))
	deliveries := s.store.Deliveries(webhookID, parseQueryInt(r, "limit", 25))
	writeJSON(w, http.StatusOK, map[string]any{"data": deliveries, "count": len(deliveries)})
}

func (s *Server) flushWebhooks(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	delivered, err := s.webhooks.DeliverOnce(parseQueryInt(r, "limit", 20))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "the webhook queue could not be flushed")
		return
	}
	s.audit(r, principal.UserID, "webhook.queue_flushed", "webhook_queue", "", "", "manual flush", map[string]any{"delivered": delivered})
	writeJSON(w, http.StatusOK, map[string]any{"delivered": delivered, "pending": len(s.store.Deliveries("", 0)) - delivered})
}

func (s *Server) slugFor(eventID string) string {
	event, err := s.store.EventByID(eventID)
	if err != nil {
		return ""
	}
	return event.Slug
}

func (s *Server) nameFor(eventID string) string {
	event, err := s.store.EventByID(eventID)
	if err != nil {
		return "your hackathon"
	}
	return event.Name
}

func (s *Server) isStaff(r *http.Request) bool {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		return false
	}
	return principal.Role == domain.RoleOrganizer || principal.Role == domain.RoleAdmin
}
