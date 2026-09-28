package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

var allowedProfileLinks = map[string]struct{}{
	"github":    {},
	"linkedin":  {},
	"portfolio": {},
	"website":   {},
	"x":         {},
	"bluesky":   {},
	"devto":     {},
	"youtube":   {},
	"discord":   {},
}

var allowedLinkSchemes = map[string][]string{
	"github":    {"https://github.com/"},
	"linkedin":  {"https://www.linkedin.com/", "https://linkedin.com/"},
	"portfolio": {"https://"},
	"website":   {"https://", "http://"},
	"x":         {"https://x.com/", "https://twitter.com/"},
	"bluesky":   {"https://bsky.app/"},
	"devto":     {"https://dev.to/"},
	"youtube":   {"https://youtube.com/", "https://www.youtube.com/"},
	"discord":   {"https://discord.gg/", "https://discord.com/"},
}

type UserProfile struct {
	UserID         string            `json:"user_id"`
	Headline       string            `json:"headline,omitempty"`
	Bio            string            `json:"bio,omitempty"`
	Location       string            `json:"location,omitempty"`
	Skills         []string          `json:"skills,omitempty"`
	Links          map[string]string `json:"links,omitempty"`
	Availability   UserAvailability  `json:"availability"`
	OpenToInvites  bool              `json:"open_to_invites"`
	SeekingTeam    bool              `json:"seeking_team"`
	SeekingRole    string            `json:"seeking_role,omitempty"`
	SeekingEventID string            `json:"seeking_event_id,omitempty"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

func (p UserProfile) Clone() UserProfile {
	copyProfile := p
	copyProfile.Skills = append([]string(nil), p.Skills...)
	if p.Links != nil {
		copyProfile.Links = make(map[string]string, len(p.Links))
		for key, value := range p.Links {
			copyProfile.Links[key] = value
		}
	}
	return copyProfile
}

func (p UserProfile) Validate() error {
	switch p.Availability {
	case "", UserAvailabilitySolo, UserAvailabilityLookingForTeam, UserAvailabilityTeamed:
	default:
		return fmt.Errorf("%w: unknown availability %q", ErrValidation, p.Availability)
	}
	if len(p.Bio) > 2000 {
		return fmt.Errorf("%w: bio must be 2000 characters or fewer", ErrValidation)
	}
	if len(p.Headline) > 140 {
		return fmt.Errorf("%w: headline must be 140 characters or fewer", ErrValidation)
	}
	for key := range p.Links {
		if _, ok := allowedProfileLinks[key]; !ok {
			return fmt.Errorf("%w: unsupported link %q", ErrValidation, key)
		}
	}
	for key, value := range p.Links {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			delete(p.Links, key)
			continue
		}
		schemes, ok := allowedLinkSchemes[key]
		if !ok {
			return fmt.Errorf("%w: unsupported link %q", ErrValidation, key)
		}
		matched := false
		for _, scheme := range schemes {
			if strings.HasPrefix(strings.ToLower(trimmed), scheme) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%w: %s link must start with %s", ErrValidation, key, strings.Join(schemes, " or "))
		}
	}
	if p.Availability == UserAvailabilityTeamed && p.SeekingTeam {
		return fmt.Errorf("%w: a teamed participant cannot also be seeking a team", ErrValidation)
	}
	return nil
}

func (p UserProfile) LinkList() []map[string]string {
	keys := make([]string, 0, len(p.Links))
	for key := range p.Links {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	links := make([]map[string]string, 0, len(keys))
	for _, key := range keys {
		links = append(links, map[string]string{"kind": key, "url": p.Links[key]})
	}
	return links
}

type InviteStatus string

const (
	InvitePending  InviteStatus = "pending"
	InviteAccepted InviteStatus = "accepted"
	InviteDeclined InviteStatus = "declined"
	InviteRevoked  InviteStatus = "revoked"
	InviteExpired  InviteStatus = "expired"
)

type TeamInvite struct {
	ID          string       `json:"id"`
	TeamID      string       `json:"team_id"`
	EventID     string       `json:"event_id,omitempty"`
	InviterID   string       `json:"inviter_id"`
	InviteeID   string       `json:"invitee_id,omitempty"`
	InviteeMail string       `json:"invitee_email,omitempty"`
	Role        TeamRole     `json:"role"`
	Status      InviteStatus `json:"status"`
	Message     string       `json:"message,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	ExpiresAt   time.Time    `json:"expires_at"`
	RespondedAt *time.Time   `json:"responded_at,omitempty"`
}

func (i TeamInvite) Clone() TeamInvite {
	copyInvite := i
	return copyInvite
}

func (i TeamInvite) Expired(now time.Time) bool {
	return !i.ExpiresAt.IsZero() && now.After(i.ExpiresAt)
}

type TeamOpportunity struct {
	Team      Team             `json:"team"`
	Event     Event            `json:"event,omitempty"`
	Members   []TeamMemberView `json:"members"`
	OpenRoles []string         `json:"open_roles,omitempty"`
}

type TeamMemberView struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Headline    string `json:"headline,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
}
