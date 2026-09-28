package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type ActivityVisibility string

const (
	ActivityPublic       ActivityVisibility = "public"
	ActivityParticipants ActivityVisibility = "participants"
	ActivityJudges       ActivityVisibility = "judges"
	ActivityOrganizers   ActivityVisibility = "organizers"
)

type ActivityCategory string

const (
	ActivityEvent         ActivityCategory = "event"
	ActivityTeam          ActivityCategory = "team"
	ActivitySubmission    ActivityCategory = "submission"
	ActivityJudging       ActivityCategory = "judging"
	ActivityResults       ActivityCategory = "results"
	ActivityAccount       ActivityCategory = "account"
	ActivityCommunication ActivityCategory = "communication"
)

type ActivityEntry struct {
	ID         string             `json:"id"`
	EventID    string             `json:"event_id,omitempty"`
	ActorID    string             `json:"actor_id,omitempty"`
	ActorName  string             `json:"actor_name,omitempty"`
	ActorRole  Role               `json:"actor_role,omitempty"`
	Category   ActivityCategory   `json:"category"`
	Action     string             `json:"action"`
	TargetType string             `json:"target_type,omitempty"`
	TargetID   string             `json:"target_id,omitempty"`
	Summary    string             `json:"summary"`
	Visibility ActivityVisibility `json:"visibility"`
	Metadata   map[string]any     `json:"metadata,omitempty"`
	RequestID  string             `json:"request_id,omitempty"`
	CreatedAt  time.Time          `json:"created_at"`
}

func (a ActivityEntry) Validate() error {
	if strings.TrimSpace(a.Action) == "" {
		return fmt.Errorf("%w: activity action is required", ErrValidation)
	}
	if strings.TrimSpace(a.Summary) == "" {
		return fmt.Errorf("%w: activity summary is required", ErrValidation)
	}
	switch a.Visibility {
	case "":
		return fmt.Errorf("%w: activity visibility is required", ErrValidation)
	case ActivityPublic, ActivityParticipants, ActivityJudges, ActivityOrganizers:
	default:
		return fmt.Errorf("%w: unknown activity visibility %q", ErrValidation, a.Visibility)
	}
	switch a.Category {
	case "":
		return fmt.Errorf("%w: activity category is required", ErrValidation)
	case ActivityEvent, ActivityTeam, ActivitySubmission, ActivityJudging, ActivityResults, ActivityAccount, ActivityCommunication:
	default:
		return fmt.Errorf("%w: unknown activity category %q", ErrValidation, a.Category)
	}
	return nil
}

// VisibleTo reports whether a viewer with the given role and event membership may
// read this activity entry.
func (a ActivityEntry) VisibleTo(role Role, isStaff, isJudge, isParticipant bool) bool {
	switch a.Visibility {
	case ActivityPublic:
		return true
	case ActivityOrganizers:
		return isStaff
	case ActivityJudges:
		return isStaff || isJudge
	case ActivityParticipants:
		return isStaff || isJudge || isParticipant
	default:
		return isStaff
	}
}

type ActivityFilter struct {
	EventID     string
	Category    string
	Action      string
	ActorID     string
	TargetType  string
	Visibility  string
	Limit       int
	Offset      int
	RollupCount bool
}

type ActivityPage struct {
	Entries      []ActivityEntry `json:"entries"`
	Count        int             `json:"count"`
	Total        int             `json:"total"`
	Limit        int             `json:"limit"`
	Offset       int             `json:"offset"`
	ByCategory   map[string]int  `json:"by_category"`
	ByVisibility map[string]int  `json:"by_visibility"`
}

func SortActivity(entries []ActivityEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].CreatedAt.Equal(entries[j].CreatedAt) {
			return entries[i].ID > entries[j].ID
		}
		return entries[i].CreatedAt.After(entries[j].CreatedAt)
	})
}
