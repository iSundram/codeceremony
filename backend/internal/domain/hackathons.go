package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type HackathonState string

const (
	HackathonDraft             HackathonState = "draft"
	HackathonRegistrationOpen  HackathonState = "registration_open"
	HackathonSubmissionsOpen   HackathonState = "submissions_open"
	HackathonSubmissionsClosed HackathonState = "submissions_closed"
	HackathonJudging           HackathonState = "judging"
	HackathonResultsPublished  HackathonState = "results_published"
	HackathonArchived          HackathonState = "archived"
)

type JudgingMode string

const (
	JudgingAutomatic JudgingMode = "automatic"
	JudgingManual    JudgingMode = "manual"
)

type TeamScope string

const (
	TeamScopeHackathon TeamScope = "hackathon"
	TeamScopeGlobal    TeamScope = "global"
)

type TeamAvailability string

const (
	TeamOpenForMembers TeamAvailability = "open"
	TeamInviteOnly     TeamAvailability = "invite_only"
	TeamClosed         TeamAvailability = "closed"
	TeamFull           TeamAvailability = "full"
)

type UserAvailability string

const (
	UserAvailabilitySolo           UserAvailability = "solo"
	UserAvailabilityLookingForTeam UserAvailability = "looking_for_team"
	UserAvailabilityTeamed         UserAvailability = "teamed"
)

type QuestionType string

const (
	QuestionShortText QuestionType = "short_text"
	QuestionLongText  QuestionType = "long_text"
	QuestionURL       QuestionType = "url"
	QuestionSelect    QuestionType = "select"
	QuestionCheckbox  QuestionType = "checkbox"
	QuestionNumber    QuestionType = "number"
)

type QuestionAudience string

const (
	AudienceSubmission  QuestionAudience = "submission"
	AudienceTeam        QuestionAudience = "team"
	AudienceParticipant QuestionAudience = "participant"
)

type HackathonQuestion struct {
	ID        string           `json:"id"`
	EventID   string           `json:"event_id"`
	Key       string           `json:"key"`
	Prompt    string           `json:"prompt"`
	HelpText  string           `json:"help_text,omitempty"`
	Type      QuestionType     `json:"type"`
	Audience  QuestionAudience `json:"audience"`
	Required  bool             `json:"required"`
	Options   []string         `json:"options,omitempty"`
	Position  int              `json:"position"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

func (q HackathonQuestion) Validate() error {
	if strings.TrimSpace(q.Prompt) == "" {
		return fmt.Errorf("%w: question prompt is required", ErrValidation)
	}
	switch q.Type {
	case QuestionShortText, QuestionLongText, QuestionURL, QuestionNumber, QuestionCheckbox, QuestionSelect:
	default:
		return fmt.Errorf("%w: unsupported question type %q", ErrValidation, q.Type)
	}
	switch q.Audience {
	case AudienceSubmission, AudienceTeam, AudienceParticipant:
	default:
		return fmt.Errorf("%w: question audience must be submission, team, or participant", ErrValidation)
	}
	if q.Type == QuestionSelect && len(q.Options) == 0 {
		return fmt.Errorf("%w: select questions need at least one option", ErrValidation)
	}
	if q.Type == QuestionCheckbox && len(q.Options) == 0 {
		return fmt.Errorf("%w: checkbox questions need at least one option", ErrValidation)
	}
	return nil
}

func (q HackathonQuestion) ValidateAnswer(answer string) error {
	value := strings.TrimSpace(answer)
	if q.Required && value == "" {
		return fmt.Errorf("%w: %q is required", ErrValidation, q.Prompt)
	}
	if value == "" {
		return nil
	}
	switch q.Type {
	case QuestionShortText, QuestionLongText:
		if q.Type == QuestionShortText && len(value) > 280 {
			return fmt.Errorf("%w: %q must be 280 characters or fewer", ErrValidation, q.Prompt)
		}
		if q.Type == QuestionLongText && len(value) > 5000 {
			return fmt.Errorf("%w: %q must be 5000 characters or fewer", ErrValidation, q.Prompt)
		}
	case QuestionURL:
		if !strings.HasPrefix(value, "https://") && !strings.HasPrefix(value, "http://") {
			return fmt.Errorf("%w: %q must be a valid http or https url", ErrValidation, q.Prompt)
		}
	case QuestionNumber:
		if strings.Trim(value, "0123456789") != "" {
			return fmt.Errorf("%w: %q must be a number", ErrValidation, q.Prompt)
		}
	case QuestionSelect:
		for _, option := range q.Options {
			if option == value {
				return nil
			}
		}
		return fmt.Errorf("%w: %q is not an allowed option", ErrValidation, q.Prompt)
	case QuestionCheckbox:
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			allowed := false
			for _, option := range q.Options {
				if option == part {
					allowed = true
					break
				}
			}
			if !allowed {
				return fmt.Errorf("%w: %q includes an unknown option", ErrValidation, q.Prompt)
			}
		}
	}
	return nil
}

func SortQuestions(questions []HackathonQuestion) {
	sort.SliceStable(questions, func(i, j int) bool {
		if questions[i].Position == questions[j].Position {
			return questions[i].ID < questions[j].ID
		}
		return questions[i].Position < questions[j].Position
	})
}

type HackathonMilestone struct {
	ID        string    `json:"id"`
	EventID   string    `json:"event_id"`
	Title     string    `json:"title"`
	Detail    string    `json:"detail,omitempty"`
	DueAt     time.Time `json:"due_at"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
}

type HackathonHost struct {
	ID        string    `json:"id"`
	EventID   string    `json:"event_id"`
	Name      string    `json:"name"`
	URL       string    `json:"url,omitempty"`
	LogoURL   string    `json:"logo_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type JudgeScope string

const (
	JudgeScopeHackathon JudgeScope = "hackathon"
	JudgeScopeGlobal    JudgeScope = "global"
)

type JudgeRosterEntry struct {
	EventID   string     `json:"event_id"`
	JudgeID   string     `json:"judge_id"`
	Scope     JudgeScope `json:"scope"`
	Headline  string     `json:"headline,omitempty"`
	Expertise []string   `json:"expertise,omitempty"`
	Active    bool       `json:"active"`
	AddedBy   string     `json:"added_by,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type ParticipationRole string

const (
	ParticipationCaptain   ParticipationRole = "captain"
	ParticipationMember    ParticipationRole = "member"
	ParticipationJudge     ParticipationRole = "judge"
	ParticipationOrganizer ParticipationRole = "organizer"
)

type Participation struct {
	ID        string            `json:"id"`
	UserID    string            `json:"user_id"`
	EventID   string            `json:"event_id"`
	TeamID    string            `json:"team_id,omitempty"`
	Role      ParticipationRole `json:"role"`
	ProjectID string            `json:"project_id,omitempty"`
	Result    string            `json:"result,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}
