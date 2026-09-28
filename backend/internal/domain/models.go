package domain

import "time"

type User struct {
	ID           string       `json:"id"`
	Email        string       `json:"email"`
	DisplayName  string       `json:"display_name"`
	AvatarURL    string       `json:"avatar_url,omitempty"`
	Bio          string       `json:"bio,omitempty"`
	Organization string       `json:"organization,omitempty"`
	Timezone     string       `json:"timezone,omitempty"`
	Locale       string       `json:"locale,omitempty"`
	Role         Role         `json:"role"`
	State        AccountState `json:"state"`
	PasswordHash string       `json:"-"`
	CreatedAt    time.Time    `json:"created_at"`
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
	ID          string     `json:"id"`
	EventID     string     `json:"event_id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	CaptainID   string     `json:"captain_id"`
	Status      TeamStatus `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
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
	ID              string              `json:"id"`
	EventID         string              `json:"event_id"`
	TeamID          string              `json:"team_id"`
	TrackID         string              `json:"track_id"`
	Title           string              `json:"title"`
	Summary         string              `json:"summary"`
	Description     string              `json:"description"`
	ThumbnailURL    string              `json:"thumbnail_url,omitempty"`
	VideoURL        string              `json:"video_url,omitempty"`
	RepositoryURL   string              `json:"repo_url,omitempty"`
	LiveURL         string              `json:"live_url,omitempty"`
	Tags            []string            `json:"tags"`
	CustomAnswers   map[string]string   `json:"custom_answers,omitempty"`
	Status          SubmissionStatus    `json:"status"`
	Eligibility     EligibilityDecision `json:"eligibility"`
	EligibilityNote string              `json:"eligibility_note,omitempty"`
	EligibilityBy   string              `json:"eligibility_by,omitempty"`
	EligibilityAt   *time.Time          `json:"eligibility_at,omitempty"`
	SubmittedAt     *time.Time          `json:"submitted_at,omitempty"`
	UpdatedAt       time.Time           `json:"updated_at"`
	Version         int                 `json:"version"`
}

type Review struct {
	ID            string         `json:"id"`
	EventID       string         `json:"event_id"`
	JudgeID       string         `json:"judge_id"`
	ProjectID     string         `json:"project_id"`
	Criteria      map[string]int `json:"criteria"`
	Normalized    map[string]int `json:"normalized,omitempty"`
	RubricID      string         `json:"rubric_id,omitempty"`
	RubricVersion int            `json:"rubric_version,omitempty"`
	Comment       string         `json:"comment"`
	Submitted     bool           `json:"submitted"`
	SubmittedAt   *time.Time     `json:"submitted_at,omitempty"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type Assignment struct {
	ID         string             `json:"id"`
	EventID    string             `json:"event_id"`
	JudgeID    string             `json:"judge_id"`
	ProjectID  string             `json:"project_id"`
	AssignedBy string             `json:"assigned_by,omitempty"`
	Strategy   AssignmentStrategy `json:"strategy,omitempty"`
	CreatedAt  time.Time          `json:"created_at"`
	RevokedAt  *time.Time         `json:"revoked_at,omitempty"`
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
