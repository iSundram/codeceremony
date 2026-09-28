package mailer

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

type Service struct {
	store      *store.Store
	dispatcher *Dispatcher
	registry   *Registry
	logger     *slog.Logger
	appURL     string
	now        func() time.Time
}

type Options struct {
	AppURL      string
	FromAddress string
	Clock       func() time.Time
	Logger      *slog.Logger
}

func NewService(data *store.Store, dispatcher *Dispatcher, registry *Registry, options Options) *Service {
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	appURL := strings.TrimRight(strings.TrimSpace(options.AppURL), "/")
	if appURL == "" {
		appURL = "http://localhost:3000"
	}
	return &Service{
		store:      data,
		dispatcher: dispatcher,
		registry:   registry,
		logger:     options.Logger,
		appURL:     appURL,
		now:        options.Clock,
	}
}

func (s *Service) Registry() *Registry { return s.registry }

func (s *Service) Dispatcher() *Dispatcher { return s.dispatcher }

func (s *Service) AppURL() string { return s.appURL }

func (s *Service) baseContext(user domain.User) Context {
	preferencesURL := s.appURL + "/settings/notifications"
	return Context{
		"DisplayName":    user.DisplayName,
		"AppURL":         s.appURL,
		"PreferencesURL": preferencesURL,
		"UserID":         user.ID,
	}
}

func (s *Service) PreferencesURL() string {
	return s.appURL + "/settings/notifications"
}

func (s *Service) UnsubscribeURL(token string) string {
	return s.appURL + "/unsubscribe?token=" + token
}

// SendTo queues a rendered template for a user, skipping silently when the user
// does not exist.
func (s *Service) SendTo(userID, template string, values map[string]any, dedupeKey string) (domain.MailMessage, error) {
	user, err := s.store.UserByID(userID)
	if err != nil {
		return domain.MailMessage{}, err
	}
	context := s.baseContext(user)
	for key, value := range values {
		context[key] = value
	}
	if dedupeKey == "" {
		dedupeKey = template + ":" + userID
	}
	return s.dispatcher.Queue(domain.MailMessage{
		UserID:    user.ID,
		Email:     user.Email,
		Template:  template,
		DedupeKey: dedupeKey,
	}, context)
}

func (s *Service) SendVerification(userID, token string) (domain.MailMessage, error) {
	verifyURL := s.appURL + "/verify-email?token=" + token
	return s.SendTo(userID, "verify_email", map[string]any{"VerifyURL": verifyURL}, "")
}

func (s *Service) SendPasswordReset(userID, token string, resetHours int) (domain.MailMessage, error) {
	resetURL := s.appURL + "/reset-password?token=" + token
	return s.SendTo(userID, "password_reset", map[string]any{"ResetURL": resetURL, "ResetHours": resetHours}, "password_reset:"+token)
}

func (s *Service) SendDeletionScheduled(userID string, scheduledFor time.Time) (domain.MailMessage, error) {
	return s.SendTo(userID, "account_deletion_scheduled", map[string]any{
		"ScheduledFor": scheduledFor.UTC().Format("2 January 2006"),
		"CancelURL":    s.appURL + "/account/deletion",
	}, "")
}

func (s *Service) SendTeamInvite(invite domain.TeamInvite, team domain.Team, inviter, invitee domain.User) (domain.MailMessage, error) {
	values := map[string]any{
		"TeamName":    team.Name,
		"InviterName": inviter.DisplayName,
		"Message":     invite.Message,
	}
	if team.EventID != "" {
		if event, err := s.store.EventByID(team.EventID); err == nil {
			values["HackathonName"] = event.Name
		}
	}
	return s.SendTo(invitee.ID, "team_invite", values, "")
}

func (s *Service) SendSubmissionReceived(userID, eventID, projectID, title string, version int) (domain.MailMessage, error) {
	values := map[string]any{"ProjectID": projectID, "ProjectTitle": title, "Version": version}
	if event, err := s.store.EventByID(eventID); err == nil {
		values["HackathonName"] = event.Name
	}
	return s.SendTo(userID, "submission_received", values, "")
}

func (s *Service) SendReviewReminder(userID, eventID string, pending int) (domain.MailMessage, error) {
	values := map[string]any{"PendingCount": pending}
	if event, err := s.store.EventByID(eventID); err == nil {
		values["HackathonName"] = event.Name
		if !event.JudgingClose.IsZero() {
			values["JudgingClose"] = event.JudgingClose.UTC().Format("2 January 2006 15:04 MST")
		}
	}
	return s.SendTo(userID, "review_reminder", values, fmt.Sprintf("review_reminder:%s:%d", eventID, pending))
}

func (s *Service) SendResultsPublished(userID, eventID, placement string) (domain.MailMessage, error) {
	values := map[string]any{"Placement": placement}
	if event, err := s.store.EventByID(eventID); err == nil {
		values["HackathonName"] = event.Name
		values["HackathonSlug"] = event.Slug
	}
	return s.SendTo(userID, "results_published", values, fmt.Sprintf("results_published:%s", eventID))
}

type AnnouncementTarget struct {
	UserID string
	Email  string
	Reason string
}

// ResolveAnnouncementTargets picks the recipients for an organizer announcement
// and records why anyone was excluded.
func (s *Service) ResolveAnnouncementTargets(event domain.Event, audience string) ([]AnnouncementTarget, []string) {
	recipients := map[string]domain.User{}
	excluded := make([]string, 0)
	addUser := func(userID string) {
		if _, ok := recipients[userID]; ok {
			return
		}
		user, err := s.store.UserByID(userID)
		if err != nil {
			return
		}
		recipients[userID] = user
	}
	for _, participation := range s.store.Participations("", event.ID) {
		switch audience {
		case "judges":
			if participation.Role == domain.ParticipationJudge {
				addUser(participation.UserID)
			}
		case "team_captains":
			if participation.Role == domain.ParticipationCaptain {
				addUser(participation.UserID)
			}
		default:
			addUser(participation.UserID)
		}
	}
	if audience == "seeking" {
		filtered := make(map[string]domain.User, len(recipients))
		for userID, user := range recipients {
			profile, err := s.store.Profile(userID)
			if err != nil || !profile.SeekingTeam {
				excluded = append(excluded, userID+": not seeking a team")
				continue
			}
			filtered[userID] = user
		}
		recipients = filtered
	}
	targets := make([]AnnouncementTarget, 0, len(recipients))
	for userID, user := range recipients {
		preferences := s.store.MailPreferences(userID)
		if !preferences.Allows(domain.MailTopicMarketing) {
			excluded = append(excluded, userID+": marketing not enabled")
			continue
		}
		targets = append(targets, AnnouncementTarget{UserID: userID, Email: user.Email})
	}
	return targets, excluded
}

func (s *Service) SendAnnouncement(event domain.Event, headline, body, audience string) (map[string]any, error) {
	targets, excluded := s.ResolveAnnouncementTargets(event, audience)
	queued := make([]string, 0, len(targets))
	skipped := make([]string, 0)
	for _, target := range targets {
		token, err := NewUnsubscribeToken(target.UserID, "marketing")
		if err != nil {
			return nil, err
		}
		if _, err := s.store.SaveUnsubscribeToken(token); err != nil {
			return nil, err
		}
		message, err := s.SendTo(target.UserID, "hackathon_announcement", map[string]any{
			"HackathonName":  event.Name,
			"HackathonURL":   s.appURL + "/hackathons/" + event.Slug,
			"Headline":       headline,
			"Body":           body,
			"UnsubscribeURL": s.UnsubscribeURL(token.Token),
		}, "")
		if err != nil {
			s.logger.Warn("announcement could not be queued", "user_id", target.UserID, "error", err)
			skipped = append(skipped, target.UserID)
			continue
		}
		if message.Status == domain.MailSkipped {
			skipped = append(skipped, target.UserID)
			continue
		}
		queued = append(queued, target.UserID)
	}
	return map[string]any{
		"audience": audience,
		"queued":   queued,
		"skipped":  skipped,
		"excluded": excluded,
		"total":    len(targets),
	}, nil
}

func (s *Service) SendWeeklyDigest(userID string, hackathons, invites, assignments int) (domain.MailMessage, error) {
	token, err := NewUnsubscribeToken(userID, "marketing")
	if err != nil {
		return domain.MailMessage{}, err
	}
	if _, err := s.store.SaveUnsubscribeToken(token); err != nil {
		return domain.MailMessage{}, err
	}
	return s.SendTo(userID, "weekly_digest", map[string]any{
		"HackathonCount":     hackathons,
		"PendingInvites":     invites,
		"PendingAssignments": assignments,
		"UnsubscribeURL":     s.UnsubscribeURL(token.Token),
	}, "")
}

func NewUnsubscribeToken(userID, scope string) (domain.UnsubscribeToken, error) {
	value, err := RandomToken(24)
	if err != nil {
		return domain.UnsubscribeToken{}, err
	}
	now := time.Now().UTC()
	return domain.UnsubscribeToken{
		Token:     value,
		UserID:    userID,
		Scope:     scope,
		CreatedAt: now,
		ExpiresAt: now.Add(90 * 24 * time.Hour),
	}, nil
}

func RandomToken(bytes int) (string, error) {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
