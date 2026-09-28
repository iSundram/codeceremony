package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type CommentStatus string

const (
	CommentVisible CommentStatus = "visible"
	CommentHidden  CommentStatus = "hidden"
	CommentDeleted CommentStatus = "deleted"
)

type Comment struct {
	ID             string        `json:"id"`
	EventID        string        `json:"event_id"`
	ProjectID      string        `json:"project_id"`
	AuthorID       string        `json:"author_id"`
	AuthorName     string        `json:"author_name,omitempty"`
	Body           string        `json:"body"`
	ParentID       string        `json:"parent_id,omitempty"`
	Status         CommentStatus `json:"status"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	ModeratedBy    string        `json:"moderated_by,omitempty"`
	ModerationNote string        `json:"moderation_note,omitempty"`
	ModeratedAt    *time.Time    `json:"moderated_at,omitempty"`
	ReplyCount     int           `json:"reply_count"`
}

func (c Comment) Validate() error {
	if strings.TrimSpace(c.Body) == "" {
		return fmt.Errorf("%w: comment body is required", ErrValidation)
	}
	if len(c.Body) > 2000 {
		return fmt.Errorf("%w: comments must be 2000 characters or fewer", ErrValidation)
	}
	if strings.TrimSpace(c.EventID) == "" || strings.TrimSpace(c.ProjectID) == "" {
		return fmt.Errorf("%w: a comment needs a hackathon and a project", ErrValidation)
	}
	if c.ParentID != "" && c.ParentID == c.ID {
		return fmt.Errorf("%w: a comment cannot reply to itself", ErrValidation)
	}
	switch c.Status {
	case "", CommentVisible, CommentHidden, CommentDeleted:
	default:
		return fmt.Errorf("%w: unknown comment status %q", ErrValidation, c.Status)
	}
	return nil
}

func (c Comment) Public() Comment {
	c.ModerationNote = ""
	c.ModeratedBy = ""
	return c
}

type ReportStatus string

const (
	ReportOpen      ReportStatus = "open"
	ReportActioned  ReportStatus = "actioned"
	ReportDismissed ReportStatus = "dismissed"
)

type CommentReport struct {
	ID             string       `json:"id"`
	CommentID      string       `json:"comment_id"`
	EventID        string       `json:"event_id"`
	ReporterID     string       `json:"reporter_id"`
	Reason         string       `json:"reason"`
	Status         ReportStatus `json:"status"`
	CreatedAt      time.Time    `json:"created_at"`
	ResolvedAt     *time.Time   `json:"resolved_at,omitempty"`
	ResolvedBy     string       `json:"resolved_by,omitempty"`
	ResolutionNote string       `json:"resolution_note,omitempty"`
}

func (r CommentReport) Validate() error {
	if strings.TrimSpace(r.Reason) == "" {
		return fmt.Errorf("%w: a report needs a reason", ErrValidation)
	}
	if len(r.Reason) > 500 {
		return fmt.Errorf("%w: report reasons must be 500 characters or fewer", ErrValidation)
	}
	switch r.Status {
	case "", ReportOpen, ReportActioned, ReportDismissed:
	default:
		return fmt.Errorf("%w: unknown report status %q", ErrValidation, r.Status)
	}
	return nil
}

type CampaignStatus string

const (
	CampaignDraft  CampaignStatus = "draft"
	CampaignOpen   CampaignStatus = "open"
	CampaignClosed CampaignStatus = "closed"
)

type VoteCampaign struct {
	ID                string         `json:"id"`
	EventID           string         `json:"event_id"`
	Name              string         `json:"name"`
	Description       string         `json:"description,omitempty"`
	Status            CampaignStatus `json:"status"`
	MaxChoicesPerUser int            `json:"max_choices_per_user"`
	OpensAt           *time.Time     `json:"opens_at,omitempty"`
	ClosesAt          *time.Time     `json:"closes_at,omitempty"`
	RequireEligible   bool           `json:"require_eligible"`
	CreatedBy         string         `json:"created_by"`
	CreatedAt         time.Time      `json:"created_at"`
	ClosedAt          *time.Time     `json:"closed_at,omitempty"`
	BallotCount       int            `json:"ballot_count"`
}

func (c VoteCampaign) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("%w: campaign name is required", ErrValidation)
	}
	if c.MaxChoicesPerUser < 1 || c.MaxChoicesPerUser > 10 {
		return fmt.Errorf("%w: max_choices_per_user must be between 1 and 10", ErrValidation)
	}
	switch c.Status {
	case "", CampaignDraft, CampaignOpen, CampaignClosed:
	default:
		return fmt.Errorf("%w: unknown campaign status %q", ErrValidation, c.Status)
	}
	return nil
}

func (c VoteCampaign) VotingOpen(now time.Time) (bool, string) {
	switch c.Status {
	case CampaignOpen:
	case CampaignDraft:
		return false, "voting has not opened yet"
	case CampaignClosed:
		return false, "voting has closed"
	}
	if c.OpensAt != nil && now.Before(*c.OpensAt) {
		return false, "voting has not opened yet"
	}
	if c.ClosesAt != nil && now.After(*c.ClosesAt) {
		return false, "voting has closed"
	}
	return true, ""
}

type Ballot struct {
	ID         string    `json:"id"`
	CampaignID string    `json:"campaign_id"`
	UserID     string    `json:"user_id"`
	ProjectID  string    `json:"project_id"`
	CreatedAt  time.Time `json:"created_at"`
}

type VoteResult struct {
	ProjectID string `json:"project_id"`
	Title     string `json:"title"`
	TeamID    string `json:"team_id,omitempty"`
	Votes     int    `json:"votes"`
	Rank      int    `json:"rank"`
}

type CampaignResult struct {
	Campaign   VoteCampaign `json:"campaign"`
	Results    []VoteResult `json:"results"`
	Voters     int          `json:"voters"`
	Closed     bool         `json:"closed"`
	Public     bool         `json:"public"`
	Tiebreaker string       `json:"tiebreaker"`
}

func SortVoteResults(results []VoteResult) {
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Votes == results[j].Votes {
			if results[i].Title == results[j].Title {
				return results[i].ProjectID < results[j].ProjectID
			}
			return results[i].Title < results[j].Title
		}
		return results[i].Votes > results[j].Votes
	})
	for index := range results {
		results[index].Rank = index + 1
	}
}
