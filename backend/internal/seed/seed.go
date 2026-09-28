package seed

import (
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

type Data struct {
	Users       []domain.User
	Events      []domain.Event
	Tracks      []domain.Track
	Teams       []domain.Team
	Submissions []domain.Submission
	Assignments []domain.Assignment
	Reviews     []domain.Review
}

func Default(passwordHash string) Data {
	created := time.Date(2026, time.February, 1, 12, 0, 0, 0, time.UTC)
	closed := time.Date(2026, time.March, 1, 18, 0, 0, 0, time.UTC)
	submitted := time.Date(2026, time.February, 28, 22, 14, 0, 0, time.UTC)
	reviewed := time.Date(2026, time.March, 2, 9, 0, 0, 0, time.UTC)

	return Data{
		Users: []domain.User{
			{ID: "organizer", Email: "organizer@example.org", DisplayName: "Rhea Organizer", Role: domain.RoleOrganizer, PasswordHash: passwordHash, CreatedAt: created},
			{ID: "judge_a", Email: "judge-a@example.org", DisplayName: "Judge A", Role: domain.RoleJudge, PasswordHash: passwordHash, CreatedAt: created},
			{ID: "judge_b", Email: "judge-b@example.org", DisplayName: "Judge B", Role: domain.RoleJudge, PasswordHash: passwordHash, CreatedAt: created},
			{ID: "participant", Email: "participant@example.org", DisplayName: "Pia Participant", Role: domain.RoleParticipant, PasswordHash: passwordHash, CreatedAt: created},
			{ID: "admin", Email: "admin@example.org", DisplayName: "Ari Admin", Role: domain.RoleAdmin, PasswordHash: passwordHash, CreatedAt: created},
		},
		Events: []domain.Event{{
			ID:               "evt_01",
			Slug:             "sample-hack-2026",
			Name:             "Sample Hack 2026",
			Description:      "A seeded event for local CodeCeremony development.",
			Timezone:         "UTC",
			RegistrationOpen: true,
			SubmissionsOpen:  false,
			SubmissionsClose: closed,
			ResultsPublished: false,
			CreatedAt:        created,
		}},
		Tracks: []domain.Track{
			{ID: "trk_01", Event: "evt_01", Name: "Developer tools", Slug: "developer-tools"},
			{ID: "trk_02", Event: "evt_01", Name: "Data and analytics", Slug: "data-and-analytics"},
			{ID: "trk_03", Event: "evt_01", Name: "Accessibility", Slug: "accessibility"},
		},
		Teams: []domain.Team{
			{ID: "tm_01", EventID: "evt_01", Name: "NorthKiln", Description: "A seeded team.", CaptainID: "participant", CreatedAt: created},
			{ID: "tm_02", EventID: "evt_01", Name: "LoudQuarry", Description: "A seeded team.", CaptainID: "participant", CreatedAt: created},
		},
		Submissions: []domain.Submission{
			{
				ID: "prj_01", EventID: "evt_01", TeamID: "tm_01", TrackID: "trk_01", Title: "Glass Signal",
				Summary: "One line of what it does.", Description: "A seeded project for the public gallery.",
				RepositoryURL: "https://example.org/repo/01", Tags: []string{"go", "platform"}, Status: domain.SubmissionSubmitted,
				SubmittedAt: timePtr(submitted), UpdatedAt: submitted, Version: 1,
			},
			{
				ID: "prj_02", EventID: "evt_01", TeamID: "tm_02", TrackID: "trk_03", Title: "Small Meadow",
				Summary: "One line of what it does.", Description: "A seeded project for the public gallery.",
				RepositoryURL: "https://example.org/repo/02", Tags: []string{"accessibility"}, Status: domain.SubmissionSubmitted,
				SubmittedAt: timePtr(submitted), UpdatedAt: submitted, Version: 1,
			},
			{
				ID: "prj_03", EventID: "evt_01", TeamID: "tm_01", TrackID: "trk_02", Title: "Deep Compass",
				Summary: "One line of what it does.", Description: "A seeded project for the public gallery.",
				RepositoryURL: "https://example.org/repo/03", Tags: []string{"data"}, Status: domain.SubmissionSubmitted,
				SubmittedAt: timePtr(submitted), UpdatedAt: submitted, Version: 1,
			},
			{
				ID: "prj_04", EventID: "evt_01", TeamID: "tm_02", TrackID: "trk_01", Title: "Green Switch",
				Summary: "One line of what it does.", Description: "A seeded project for the public gallery.",
				RepositoryURL: "https://example.org/repo/04", Tags: []string{"platform"}, Status: domain.SubmissionSubmitted,
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
