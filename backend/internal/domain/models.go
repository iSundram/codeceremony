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
	// PasswordHash is a bcrypt hash and is persisted, which makes the data file
	// something to protect like any other credential store. It used to be
	// `json:"-"` and blanked in Snapshot, on the reasoning that a backup is the
	// thing most likely to be copied around. The cost of that was that every
	// restart destroyed every password in the portal, leaving the fixed public
	// seed tokens as the only credentials that still worked — a worse outcome
	// than a salted, one-way hash sitting in a file the operator already has to
	// protect. The plaintext is never stored. The loader refuses a value that is
	// not a bcrypt hash, so a plaintext password in a data file is an error
	// rather than a silently trusted credential.
	PasswordHash string       `json:"password_hash,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
}

type Event struct {
	ID                 string         `json:"id"`
	Slug               string         `json:"slug"`
	Name               string         `json:"name"`
	Summary            string         `json:"summary,omitempty"`
	Description        string         `json:"description"`
	Timezone           string         `json:"timezone"`
	State              HackathonState `json:"state,omitempty"`
	JudgingMode        JudgingMode    `json:"judging_mode,omitempty"`
	RegistrationOpen   bool           `json:"registration_open"`
	SubmissionsOpen    bool           `json:"submissions_open"`
	SubmissionsClose   time.Time      `json:"submissions_close"`
	JudgingClose       time.Time      `json:"judging_close,omitempty"`
	TeamScope          TeamScope      `json:"team_scope,omitempty"`
	MinTeamSize        int            `json:"min_team_size,omitempty"`
	MaxTeamSize        int            `json:"max_team_size,omitempty"`
	AllowGlobalTeams   bool           `json:"allow_global_teams,omitempty"`
	ReviewsPerProject  int            `json:"reviews_per_project,omitempty"`
	LeaderboardPublic  bool           `json:"leaderboard_public"`
	ResultsPublished   bool           `json:"results_published"`
	ResultsPublishedAt *time.Time     `json:"results_published_at,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

type Team struct {
	ID           string           `json:"id"`
	EventID      string           `json:"event_id"`
	Scope        TeamScope        `json:"scope"`
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	CaptainID    string           `json:"captain_id"`
	Status       TeamStatus       `json:"status"`
	Availability TeamAvailability `json:"availability"`
	MaxSize      int              `json:"max_size,omitempty"`
	OpenRoles    []string         `json:"open_roles,omitempty"`
	CreatedAt    time.Time        `json:"created_at"`
	DeletedAt    *time.Time       `json:"deleted_at,omitempty"`
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
	Story           string              `json:"story,omitempty"`
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

// Comparison is one recorded head-to-head verdict.
//
// This is the first-class form of the pairwise question. The estimator in
// internal/judging can also derive comparisons from rubric scores, but a derived
// comparison is only as good as the assumption that a judge's 4 out of 5 on one
// project and 3 out of 5 on another means they preferred the first. A recorded
// verdict is the judge answering the question directly, and it is attributed,
// timestamped and reversible, which a derived one is not.
type Comparison struct {
	ID      string `json:"id"`
	EventID string `json:"event_id"`
	JudgeID string `json:"judge_id"`
	// Left and Right are the two projects compared. The verdict is stored on
	// this pair as written rather than being normalised to a sorted pair, so
	// that reversing a verdict is a visible change of mind rather than a
	// silently different row.
	Left      string            `json:"left"`
	Right     string            `json:"right"`
	Verdict   ComparisonVerdict `json:"verdict"`
	Comment   string            `json:"comment,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// ComparisonVerdict is what the judge concluded.
type ComparisonVerdict string

const (
	// ComparisonLeftWins means the judge preferred Left over Right.
	ComparisonLeftWins ComparisonVerdict = "left"
	// ComparisonRightWins means the judge preferred Right over Left.
	ComparisonRightWins ComparisonVerdict = "right"
	// ComparisonTied is a genuine draw. It is a real answer, not missing data:
	// treating it as half a win is the standard handling and is why the
	// estimator has a tie weight.
	ComparisonTied ComparisonVerdict = "tie"
)

// Valid reports whether a verdict is one of the three defined answers.
func (v ComparisonVerdict) Valid() bool {
	switch v {
	case ComparisonLeftWins, ComparisonRightWins, ComparisonTied:
		return true
	default:
		return false
	}
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
