package domain

import "time"

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	Role         Role      `json:"role"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type Track struct {
	ID    string `json:"id"`
	Event string `json:"event_id"`
	Name  string `json:"name"`
	Slug  string `json:"slug"`
}

type Event struct {
	ID               string    `json:"id"`
	Slug             string    `json:"slug"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	Timezone         string    `json:"timezone"`
	RegistrationOpen bool      `json:"registration_open"`
	SubmissionsOpen  bool      `json:"submissions_open"`
	SubmissionsClose time.Time `json:"submissions_close"`
	ResultsPublished bool      `json:"results_published"`
	CreatedAt        time.Time `json:"created_at"`
}

type Team struct {
	ID          string    `json:"id"`
	EventID     string    `json:"event_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CaptainID   string    `json:"captain_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type SubmissionStatus string

const (
	SubmissionDraft        SubmissionStatus = "draft"
	SubmissionSubmitted    SubmissionStatus = "submitted"
	SubmissionNeedsChange  SubmissionStatus = "needs_changes"
	SubmissionWithdrawn    SubmissionStatus = "withdrawn"
	SubmissionLocked       SubmissionStatus = "locked"
	SubmissionDisqualified SubmissionStatus = "disqualified"
)

type Submission struct {
	ID            string            `json:"id"`
	EventID       string            `json:"event_id"`
	TeamID        string            `json:"team_id"`
	TrackID       string            `json:"track_id"`
	Title         string            `json:"title"`
	Summary       string            `json:"summary"`
	Description   string            `json:"description"`
	ThumbnailURL  string            `json:"thumbnail_url,omitempty"`
	VideoURL      string            `json:"video_url,omitempty"`
	RepositoryURL string            `json:"repo_url,omitempty"`
	LiveURL       string            `json:"live_url,omitempty"`
	Tags          []string          `json:"tags"`
	CustomAnswers map[string]string `json:"custom_answers,omitempty"`
	Status        SubmissionStatus  `json:"status"`
	SubmittedAt   *time.Time        `json:"submitted_at,omitempty"`
	UpdatedAt     time.Time         `json:"updated_at"`
	Version       int               `json:"version"`
}

type Review struct {
	ID          string         `json:"id"`
	EventID     string         `json:"event_id"`
	JudgeID     string         `json:"judge_id"`
	ProjectID   string         `json:"project_id"`
	Criteria    map[string]int `json:"criteria"`
	Comment     string         `json:"comment"`
	Submitted   bool           `json:"submitted"`
	SubmittedAt *time.Time     `json:"submitted_at,omitempty"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type Assignment struct {
	ID        string    `json:"id"`
	EventID   string    `json:"event_id"`
	JudgeID   string    `json:"judge_id"`
	ProjectID string    `json:"project_id"`
	CreatedAt time.Time `json:"created_at"`
}

type Progress struct {
	EventID             string `json:"event_id"`
	Submissions         int    `json:"submissions"`
	EligibleSubmissions int    `json:"eligible_submissions"`
	Assignments         int    `json:"assignments"`
	ReviewsStarted      int    `json:"reviews_started"`
	ReviewsCompleted    int    `json:"reviews_completed"`
	ReviewsPending      int    `json:"reviews_pending"`
}
