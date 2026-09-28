package domain

import (
	"regexp"
	"strings"
	"time"
)

type EligibilityDecision string

const (
	EligibilityPending    EligibilityDecision = "pending"
	EligibilityEligible   EligibilityDecision = "eligible"
	EligibilityIneligible EligibilityDecision = "ineligible"
)

type DuplicateStatus string

const (
	DuplicateOpen      DuplicateStatus = "open"
	DuplicateConfirmed DuplicateStatus = "confirmed"
	DuplicateDismissed DuplicateStatus = "dismissed"
)

type SubmissionVersion struct {
	ID            string            `json:"id"`
	SubmissionID  string            `json:"submission_id"`
	Version       int               `json:"version"`
	Title         string            `json:"title"`
	Summary       string            `json:"summary"`
	Description   string            `json:"description"`
	RepositoryURL string            `json:"repo_url,omitempty"`
	LiveURL       string            `json:"live_url,omitempty"`
	VideoURL      string            `json:"video_url,omitempty"`
	Tags          []string          `json:"tags"`
	CustomAnswers map[string]string `json:"custom_answers,omitempty"`
	Status        SubmissionStatus  `json:"status"`
	CreatedBy     string            `json:"created_by"`
	CreatedAt     time.Time         `json:"created_at"`
	Reason        string            `json:"reason,omitempty"`
}

type DuplicateFlag struct {
	ID                   string          `json:"id"`
	EventID              string          `json:"event_id"`
	ProjectID            string          `json:"project_id"`
	DuplicateOfProjectID string          `json:"duplicate_of_project_id"`
	Signal               string          `json:"signal"`
	Detail               string          `json:"detail"`
	Status               DuplicateStatus `json:"status"`
	CreatedAt            time.Time       `json:"created_at"`
	ResolvedAt           *time.Time      `json:"resolved_at,omitempty"`
	ResolvedBy           string          `json:"resolved_by,omitempty"`
	ResolutionNote       string          `json:"resolution_note,omitempty"`
}

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

func NormalizeTitle(title string) string {
	return strings.TrimSpace(nonAlphanumeric.ReplaceAllString(strings.ToLower(title), " "))
}

func NormalizeURL(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	trimmed = strings.TrimPrefix(trimmed, "https://")
	trimmed = strings.TrimPrefix(trimmed, "http://")
	trimmed = strings.TrimPrefix(trimmed, "www.")
	return strings.TrimSuffix(trimmed, "/")
}

type DuplicateSignal struct {
	Kind   string `json:"signal"`
	Detail string `json:"detail"`
}

func DuplicateSignals(left, right Submission) []DuplicateSignal {
	signals := make([]DuplicateSignal, 0, 2)
	leftRepo := NormalizeURL(left.RepositoryURL)
	rightRepo := NormalizeURL(right.RepositoryURL)
	if leftRepo != "" && leftRepo == rightRepo {
		signals = append(signals, DuplicateSignal{"repository", "both submissions point to the same repository"})
	}
	leftLive := NormalizeURL(left.LiveURL)
	rightLive := NormalizeURL(right.LiveURL)
	if leftLive != "" && leftLive == rightLive {
		signals = append(signals, DuplicateSignal{"live_url", "both submissions point to the same live demo"})
	}
	leftTitle := NormalizeTitle(left.Title)
	rightTitle := NormalizeTitle(right.Title)
	if leftTitle != "" && leftTitle == rightTitle {
		signals = append(signals, DuplicateSignal{"title", "normalized titles are identical"})
	}
	return signals
}
