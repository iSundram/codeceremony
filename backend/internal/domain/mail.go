package domain

import (
	"fmt"
	"strings"
	"time"
)

type MailKind string

const (
	MailKindTransactional MailKind = "transactional"
	MailKindPromotional   MailKind = "promotional"
)

type MailStatus string

const (
	MailQueued    MailStatus = "queued"
	MailSending   MailStatus = "sending"
	MailSent      MailStatus = "sent"
	MailFailed    MailStatus = "failed"
	MailSkipped   MailStatus = "skipped"
	MailCancelled MailStatus = "cancelled"
)

type MailTopic string

const (
	MailTopicAccountSecurity  MailTopic = "account_security"
	MailTopicAccountLifecycle MailTopic = "account_lifecycle"
	MailTopicTeamActivity     MailTopic = "team_activity"
	MailTopicEventActivity    MailTopic = "event_activity"
	MailTopicJudging          MailTopic = "judging"
	MailTopicResults          MailTopic = "results"
	MailTopicMarketing        MailTopic = "marketing"
)

type MailMessage struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id,omitempty"`
	Email       string     `json:"email"`
	EventID     string     `json:"event_id,omitempty"`
	Template    string     `json:"template"`
	Topic       MailTopic  `json:"topic"`
	Kind        MailKind   `json:"kind"`
	Status      MailStatus `json:"status"`
	Subject     string     `json:"subject"`
	BodyText    string     `json:"body_text"`
	BodyHTML    string     `json:"body_html,omitempty"`
	Attempts    int        `json:"attempts"`
	LastError   string     `json:"last_error,omitempty"`
	DedupeKey   string     `json:"dedupe_key,omitempty"`
	ScheduledAt time.Time  `json:"scheduled_at"`
	SentAt      *time.Time `json:"sent_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (m MailMessage) Validate() error {
	if strings.TrimSpace(m.Email) == "" || !strings.Contains(m.Email, "@") {
		return fmt.Errorf("%w: a valid recipient email is required", ErrValidation)
	}
	if strings.TrimSpace(m.Template) == "" {
		return fmt.Errorf("%w: a template name is required", ErrValidation)
	}
	if strings.TrimSpace(m.Subject) == "" {
		return fmt.Errorf("%w: a subject is required", ErrValidation)
	}
	if m.Kind != MailKindTransactional && m.Kind != MailKindPromotional {
		return fmt.Errorf("%w: unknown mail kind %q", ErrValidation, m.Kind)
	}
	switch m.Status {
	case "", MailQueued, MailSending, MailSent, MailFailed, MailSkipped, MailCancelled:
	default:
		return fmt.Errorf("%w: unknown mail status %q", ErrValidation, m.Status)
	}
	return nil
}

type MailPreferences struct {
	UserID           string     `json:"user_id"`
	Transactional    bool       `json:"transactional"`
	AccountSecurity  bool       `json:"account_security"`
	AccountLifecycle bool       `json:"account_lifecycle"`
	TeamActivity     bool       `json:"team_activity"`
	EventActivity    bool       `json:"event_activity"`
	Judging          bool       `json:"judging"`
	Results          bool       `json:"results"`
	Marketing        bool       `json:"marketing"`
	DigestOnly       bool       `json:"digest_only"`
	WeeklyDigest     bool       `json:"weekly_digest"`
	UnsubscribedAll  bool       `json:"unsubscribed_all"`
	UnsubscribedAt   *time.Time `json:"unsubscribed_at,omitempty"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func DefaultMailPreferences(userID string) MailPreferences {
	return MailPreferences{
		UserID:           userID,
		Transactional:    true,
		AccountSecurity:  true,
		AccountLifecycle: true,
		TeamActivity:     true,
		EventActivity:    true,
		Judging:          true,
		Results:          true,
		Marketing:        false,
		DigestOnly:       false,
		WeeklyDigest:     false,
	}
}

func (p MailPreferences) Allows(topic MailTopic) bool {
	if p.UnsubscribedAll {
		return false
	}
	if !p.Transactional && isTransactionalTopic(topic) {
		return false
	}
	switch topic {
	case MailTopicAccountSecurity:
		return p.AccountSecurity
	case MailTopicAccountLifecycle:
		return p.AccountLifecycle
	case MailTopicTeamActivity:
		return p.TeamActivity
	case MailTopicEventActivity:
		return p.EventActivity
	case MailTopicJudging:
		return p.Judging
	case MailTopicResults:
		return p.Results
	case MailTopicMarketing:
		return p.Marketing
	default:
		return false
	}
}

func isTransactionalTopic(topic MailTopic) bool {
	switch topic {
	case MailTopicAccountSecurity, MailTopicAccountLifecycle:
		return true
	default:
		return false
	}
}

type UnsubscribeToken struct {
	Token     string     `json:"token"`
	UserID    string     `json:"user_id"`
	Scope     string     `json:"scope"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
}

func (t UnsubscribeToken) Expired(now time.Time) bool {
	return !t.ExpiresAt.IsZero() && now.After(t.ExpiresAt)
}
