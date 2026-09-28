package seed

import (
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

type Data struct {
	Users           []domain.User
	JudgeProfiles   []domain.JudgeProfile
	Events          []domain.Event
	Tracks          []domain.Track
	Prizes          []domain.Prize
	Profiles        []domain.UserProfile
	Questions       []domain.HackathonQuestion
	Milestones      []domain.HackathonMilestone
	Hosts           []domain.HackathonHost
	JudgeRoster     []domain.JudgeRosterEntry
	Invites         []domain.TeamInvite
	Participations  []domain.Participation
	Teams           []domain.Team
	TeamMemberships []domain.TeamMembership
	Submissions     []domain.Submission
	Assignments     []domain.Assignment
	Reviews         []domain.Review
	Rubrics         []domain.Rubric
	Notifications   []domain.Notification
	AuditEvents     []domain.AuditEvent
}

func Default(passwordHash string) Data {
	created := time.Date(2026, time.February, 1, 12, 0, 0, 0, time.UTC)
	closed := time.Date(2026, time.March, 1, 18, 0, 0, 0, time.UTC)
	submitted := time.Date(2026, time.February, 28, 22, 14, 0, 0, time.UTC)
	reviewed := time.Date(2026, time.March, 2, 9, 0, 0, 0, time.UTC)

	return Data{
		Users: []domain.User{
			{ID: "organizer", Email: "organizer@example.org", DisplayName: "Rhea Organizer", Role: domain.RoleOrganizer, State: domain.AccountActive, PasswordHash: passwordHash, CreatedAt: created},
			{ID: "judge_a", Email: "judge-a@example.org", DisplayName: "Judge A", Role: domain.RoleJudge, State: domain.AccountActive, PasswordHash: passwordHash, CreatedAt: created},
			{ID: "judge_b", Email: "judge-b@example.org", DisplayName: "Judge B", Role: domain.RoleJudge, State: domain.AccountActive, PasswordHash: passwordHash, CreatedAt: created},
			{ID: "participant", Email: "participant@example.org", DisplayName: "Pia Participant", Role: domain.RoleParticipant, State: domain.AccountActive, PasswordHash: passwordHash, CreatedAt: created},
			{ID: "participant_other", Email: "participant-other@example.org", DisplayName: "Pia Teammate", Role: domain.RoleParticipant, State: domain.AccountActive, PasswordHash: passwordHash, CreatedAt: created},
			{ID: "admin", Email: "admin@example.org", DisplayName: "Ari Admin", Role: domain.RoleAdmin, State: domain.AccountActive, PasswordHash: passwordHash, CreatedAt: created},
		},
		JudgeProfiles: []domain.JudgeProfile{
			{UserID: "judge_a", Bio: "Seeded judge A.", Tracks: []string{"trk_01", "trk_03"}, Capacity: 20, Active: true},
			{UserID: "judge_b", Bio: "Seeded judge B.", Tracks: []string{"trk_01", "trk_02", "trk_03"}, Capacity: 20, Active: true},
		},
		Rubrics: []domain.Rubric{{
			ID:      "rub_01",
			EventID: "evt_01",
			Name:    "Standard judging rubric",
			Version: 1,
			Status:  domain.RubricPublished,
			Criteria: []domain.RubricCriterion{
				{Key: "functionality", Label: "Functionality", Description: "Does the project work end to end?", MinScore: 1, MaxScore: 5, Weight: 40, Required: true},
				{Key: "quality", Label: "Quality", Description: "Is the work clear, complete, and well built?", MinScore: 1, MaxScore: 5, Weight: 35, Required: true},
				{Key: "innovation", Label: "Innovation", Description: "Is the idea original and meaningfully different?", MinScore: 1, MaxScore: 5, Weight: 25, Required: true},
			},
			CreatedBy:    "organizer",
			CreatedAt:    created,
			PublishedAt:  timePtr(created),
			Instructions: "Score every criterion. Judges cannot see other judges' scores until results are published.",
		}},
		Events: []domain.Event{{
			ID:                "evt_01",
			Slug:              "sample-hack-2026",
			Name:              "Sample Hack 2026",
			Description:       "A seeded event for local CodeCeremony development.",
			Timezone:          "UTC",
			State:             domain.HackathonSubmissionsClosed,
			JudgingMode:       domain.JudgingAutomatic,
			RegistrationOpen:  true,
			SubmissionsOpen:   false,
			SubmissionsClose:  closed,
			JudgingClose:      time.Date(2026, time.March, 5, 18, 0, 0, 0, time.UTC),
			MinTeamSize:       1,
			MaxTeamSize:       5,
			AllowGlobalTeams:  true,
			ReviewsPerProject: 2,
			UpdatedAt:         submitted,
			ResultsPublished:  false,
			CreatedAt:         created,
		}},
		Tracks: []domain.Track{
			{ID: "trk_01", Event: "evt_01", Name: "Developer tools", Slug: "developer-tools"},
			{ID: "trk_02", Event: "evt_01", Name: "Data and analytics", Slug: "data-and-analytics"},
			{ID: "trk_03", Event: "evt_01", Name: "Accessibility", Slug: "accessibility"},
		},
		Prizes: []domain.Prize{
			{ID: "prz_01", EventID: "evt_01", Name: "Overall first place", Description: "Best overall project.", Rank: 1, CreatedAt: created},
			{ID: "prz_02", EventID: "evt_01", TrackID: "trk_03", Name: "Accessibility prize", Description: "Most thoughtful accessibility work.", Rank: 1, CreatedAt: created},
		},
		Profiles: []domain.UserProfile{
			{
				UserID: "participant", Headline: "Backend and platform engineer", Bio: "Builds tools for hackathons.",
				Location: "Bengaluru, India", Skills: []string{"go", "postgres", "distributed systems"},
				Links: map[string]string{
					"github":    "https://github.com/participant",
					"linkedin":  "https://www.linkedin.com/in/participant",
					"portfolio": "https://participant.example.org",
					"x":         "https://x.com/participant",
				},
				Availability: domain.UserAvailabilityLookingForTeam, OpenToInvites: true, SeekingTeam: true,
				SeekingRole: "backend or platform", SeekingEventID: "evt_01", UpdatedAt: created,
			},
			{
				UserID: "participant_other", Headline: "Frontend and accessibility", Bio: "Designs accessible interfaces.",
				Location: "Lisbon, Portugal", Skills: []string{"react", "accessibility", "design systems"},
				Links: map[string]string{
					"github":  "https://github.com/participant-other",
					"website": "https://other.example.org",
					"bluesky": "https://bsky.app/profile/other.example.org",
				},
				Availability: domain.UserAvailabilityTeamed, OpenToInvites: false, UpdatedAt: created,
			},
			{
				UserID: "judge_a", Headline: "Staff engineer, platform", Bio: "Reviews systems and infrastructure work.",
				Location: "Toronto, Canada", Skills: []string{"infrastructure", "security"},
				Links:        map[string]string{"linkedin": "https://www.linkedin.com/in/judge-a"},
				Availability: domain.UserAvailabilityTeamed, OpenToInvites: false, UpdatedAt: created,
			},
			{
				UserID: "judge_b", Headline: "Product engineer", Bio: "Focuses on usefulness and polish.",
				Location: "Nairobi, Kenya", Skills: []string{"product", "frontend"},
				Links:        map[string]string{"github": "https://github.com/judge-b"},
				Availability: domain.UserAvailabilityTeamed, OpenToInvites: false, UpdatedAt: created,
			},
		},
		Questions: []domain.HackathonQuestion{
			{ID: "qst_01", EventID: "evt_01", Key: "problem_you_solved", Prompt: "What problem did you solve?", HelpText: "One or two sentences.", Type: domain.QuestionLongText, Audience: domain.AudienceSubmission, Required: true, Position: 1, CreatedAt: created, UpdatedAt: created},
			{ID: "qst_02", EventID: "evt_01", Key: "demo_url", Prompt: "Where can judges try it?", Type: domain.QuestionURL, Audience: domain.AudienceSubmission, Required: false, Position: 2, CreatedAt: created, UpdatedAt: created},
			{ID: "qst_03", EventID: "evt_01", Key: "built_with", Prompt: "Which of these did you use?", Type: domain.QuestionCheckbox, Audience: domain.AudienceSubmission, Required: false, Options: []string{"go", "python", "rust", "typescript"}, Position: 3, CreatedAt: created, UpdatedAt: created},
			{ID: "qst_04", EventID: "evt_01", Key: "team_size", Prompt: "How many people are on the team?", Type: domain.QuestionNumber, Audience: domain.AudienceTeam, Required: true, Position: 1, CreatedAt: created, UpdatedAt: created},
		},
		Milestones: []domain.HackathonMilestone{
			{ID: "mil_01", EventID: "evt_01", Title: "Registration opens", Detail: "Teams can register.", DueAt: time.Date(2026, time.February, 1, 12, 0, 0, 0, time.UTC), Position: 1, CreatedAt: created},
			{ID: "mil_02", EventID: "evt_01", Title: "Submissions close", Detail: "Final submissions lock at the deadline.", DueAt: time.Date(2026, time.March, 1, 18, 0, 0, 0, time.UTC), Position: 2, CreatedAt: created},
			{ID: "mil_03", EventID: "evt_01", Title: "Judging ends", Detail: "Organizers close judging.", DueAt: time.Date(2026, time.March, 5, 18, 0, 0, 0, time.UTC), Position: 3, CreatedAt: created},
		},
		Hosts: []domain.HackathonHost{
			{ID: "hst_01", EventID: "evt_01", Name: "Northwind Labs", URL: "https://example.org/northwind", CreatedAt: created},
			{ID: "hst_02", EventID: "evt_01", Name: "CodeCeremony", URL: "https://example.org/codeceremony", CreatedAt: created},
		},
		JudgeRoster: []domain.JudgeRosterEntry{
			{EventID: "evt_01", JudgeID: "judge_a", Scope: domain.JudgeScopeHackathon, Headline: "Platform and infrastructure", Expertise: []string{"infrastructure", "security"}, Active: true, AddedBy: "organizer", CreatedAt: created},
			{EventID: "evt_01", JudgeID: "judge_b", Scope: domain.JudgeScopeHackathon, Headline: "Product and design", Expertise: []string{"product", "frontend"}, Active: true, AddedBy: "organizer", CreatedAt: created},
			{EventID: "", JudgeID: "judge_b", Scope: domain.JudgeScopeGlobal, Headline: "Global reviewer pool", Active: true, AddedBy: "admin", CreatedAt: created},
		},
		Participations: []domain.Participation{
			{ID: "par_01", UserID: "participant", EventID: "evt_01", TeamID: "tm_01", Role: domain.ParticipationCaptain, CreatedAt: created},
			{ID: "par_02", UserID: "participant_other", EventID: "evt_01", TeamID: "tm_01", Role: domain.ParticipationMember, CreatedAt: created},
			{ID: "par_03", UserID: "judge_a", EventID: "evt_01", Role: domain.ParticipationJudge, CreatedAt: created},
			{ID: "par_04", UserID: "judge_b", EventID: "evt_01", Role: domain.ParticipationJudge, CreatedAt: created},
			{ID: "par_05", UserID: "organizer", EventID: "evt_01", Role: domain.ParticipationOrganizer, CreatedAt: created},
		},
		Teams: []domain.Team{
			{ID: "tm_01", EventID: "evt_01", Scope: domain.TeamScopeHackathon, Name: "NorthKiln", Description: "A seeded team.", CaptainID: "participant", Status: domain.TeamStatusActive, Availability: domain.TeamOpenForMembers, MaxSize: 4, OpenRoles: []string{"frontend", "design"}, CreatedAt: created},
			{ID: "tm_02", EventID: "evt_01", Name: "LoudQuarry", Description: "A seeded team.", CaptainID: "participant", Status: domain.TeamStatusActive, CreatedAt: created},
		},
		TeamMemberships: []domain.TeamMembership{
			{ID: "tmem_01", EventID: "evt_01", TeamID: "tm_01", UserID: "participant", Role: domain.TeamRoleCaptain, Status: "active", JoinedAt: created, UpdatedAt: created},
			{ID: "tmem_02", EventID: "evt_01", TeamID: "tm_02", UserID: "participant", Role: domain.TeamRoleCaptain, Status: "active", JoinedAt: created, UpdatedAt: created},
			{ID: "tmem_03", EventID: "evt_01", TeamID: "tm_01", UserID: "participant_other", Role: domain.TeamRoleMember, Status: "active", JoinedAt: created, UpdatedAt: created},
		},
		Submissions: []domain.Submission{
			{
				ID: "prj_01", EventID: "evt_01", TeamID: "tm_01", TrackID: "trk_01", Title: "Glass Signal",
				Summary: "One line of what it does.", Description: "A seeded project for the public gallery.",
				RepositoryURL: "https://example.org/repo/01", Tags: []string{"go", "platform"}, Status: domain.SubmissionSubmitted, Eligibility: domain.EligibilityEligible,
				SubmittedAt: timePtr(submitted), UpdatedAt: submitted, Version: 1,
			},
			{
				ID: "prj_02", EventID: "evt_01", TeamID: "tm_02", TrackID: "trk_03", Title: "Small Meadow",
				Summary: "One line of what it does.", Description: "A seeded project for the public gallery.",
				RepositoryURL: "https://example.org/repo/02", Tags: []string{"accessibility"}, Status: domain.SubmissionSubmitted, Eligibility: domain.EligibilityEligible,
				SubmittedAt: timePtr(submitted), UpdatedAt: submitted, Version: 1,
			},
			{
				ID: "prj_03", EventID: "evt_01", TeamID: "tm_01", TrackID: "trk_02", Title: "Deep Compass",
				Summary: "One line of what it does.", Description: "A seeded project for the public gallery.",
				RepositoryURL: "https://example.org/repo/03", Tags: []string{"data"}, Status: domain.SubmissionSubmitted, Eligibility: domain.EligibilityEligible,
				SubmittedAt: timePtr(submitted), UpdatedAt: submitted, Version: 1,
			},
			{
				ID: "prj_04", EventID: "evt_01", TeamID: "tm_02", TrackID: "trk_01", Title: "Green Switch",
				Summary: "One line of what it does.", Description: "A seeded project for the public gallery.",
				RepositoryURL: "https://example.org/repo/04", Tags: []string{"platform"}, Status: domain.SubmissionSubmitted, Eligibility: domain.EligibilityEligible,
				SubmittedAt: timePtr(submitted), UpdatedAt: submitted, Version: 1,
			},
		},
		Assignments: []domain.Assignment{
			{ID: "asg_01", EventID: "evt_01", JudgeID: "judge_a", ProjectID: "prj_01", CreatedAt: created},
			{ID: "asg_02", EventID: "evt_01", JudgeID: "judge_b", ProjectID: "prj_01", CreatedAt: created},
		},
		Reviews: []domain.Review{
			{ID: "rev_01", EventID: "evt_01", JudgeID: "judge_a", ProjectID: "prj_01", Criteria: map[string]int{"functionality": 4, "quality": 5, "innovation": 4}, Comment: "Runs clean.", Submitted: true, SubmittedAt: timePtr(reviewed), UpdatedAt: reviewed},
			{ID: "rev_02", EventID: "evt_01", JudgeID: "judge_b", ProjectID: "prj_01", Criteria: map[string]int{"functionality": 3, "quality": 4, "innovation": 5}, Comment: "Solid.", Submitted: true, SubmittedAt: timePtr(reviewed), UpdatedAt: reviewed},
		},
	}
}

func timePtr(value time.Time) *time.Time {
	return &value
}
