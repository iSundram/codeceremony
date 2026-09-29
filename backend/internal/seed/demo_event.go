package seed

import (
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

// DemoEventID is the identifier of the second seeded event.
const DemoEventID = "evt_demo"

// DemoEventSlug is the public slug of the second seeded event.
const DemoEventSlug = "open-call-2026"

// Demo identifiers. They sit outside the fixture id space on purpose: the
// shared fixtures are the same file for every entrant, and the portal's own
// demo rows must not be mistaken for fixture rows or collide with them.
const (
	DemoTrackID    = "trk_demo"
	DemoTeamID     = "tm_demo"
	DemoProjectID  = "prj_demo"
	DemoRubricID   = "rub_demo"
	DemoActivityID = "act_demo_01"
)

// WithDemoEvent adds a second event that is deliberately still open.
//
// The shared fixture event closed in the past, which is exactly right for the
// acceptance checker and exactly wrong for a walkthrough: there is no way to
// demonstrate a live submission, a live review, or a publish against an event
// that refuses submissions. This second event exists so the whole lifecycle can
// be exercised on a running portal without touching a clock.
//
// The two events are independent. Closing, scoring or publishing the demo event
// changes nothing about the fixture event, and the acceptance routes keep
// pointing at the fixture event.
func WithDemoEvent(data Data, now time.Time) Data {
	now = now.UTC()
	created := now.Add(-21 * 24 * time.Hour)
	submissionsClose := now.Add(30 * 24 * time.Hour)
	judgingClose := submissionsClose.Add(7 * 24 * time.Hour)
	published := &created

	data.Events = append(data.Events, domain.Event{
		ID: DemoEventID, Slug: DemoEventSlug, Name: "Open Call 2026",
		Summary: "A second seeded event, left open so the full lifecycle can be walked through.",
		Description: "An open event for demonstrating submission, judging and publication without " +
			"manipulating the clock. The fixture event stays closed on purpose.",
		Timezone: "UTC", State: domain.HackathonSubmissionsOpen, JudgingMode: domain.JudgingAutomatic,
		RegistrationOpen: true, SubmissionsOpen: true,
		SubmissionsClose: submissionsClose, JudgingClose: judgingClose,
		TeamScope: domain.TeamScopeHackathon, MinTeamSize: 1, MaxTeamSize: 5,
		AllowGlobalTeams: true, ReviewsPerProject: 2, LeaderboardPublic: false,
		ResultsPublished: false, CreatedAt: created, UpdatedAt: created,
	})

	data.Tracks = append(data.Tracks, domain.Track{
		ID: DemoTrackID, Event: DemoEventID, Name: "Showcase", Slug: "showcase",
		Summary: "Anything goes.", Order: 1,
	})

	data.Teams = append(data.Teams, domain.Team{
		ID: DemoTeamID, EventID: DemoEventID, Scope: domain.TeamScopeHackathon,
		Name: "Blue Hour", Description: "A demo team that can still submit.",
		CaptainID: "participant", Status: domain.TeamStatusActive,
		Availability: domain.TeamOpenForMembers, MaxSize: 4, OpenRoles: []string{"design"},
		CreatedAt: created,
	})
	data.TeamMemberships = append(data.TeamMemberships,
		domain.TeamMembership{
			ID: "tmem_demo_01", EventID: DemoEventID, TeamID: DemoTeamID, UserID: "participant",
			Role: domain.TeamRoleCaptain, Status: "active", JoinedAt: created, UpdatedAt: created,
		},
		domain.TeamMembership{
			ID: "tmem_demo_02", EventID: DemoEventID, TeamID: DemoTeamID, UserID: "participant_other",
			Role: domain.TeamRoleMember, Status: "active", JoinedAt: created, UpdatedAt: created,
		},
	)
	data.Participations = append(data.Participations,
		domain.Participation{
			ID: "par_demo_01", UserID: "participant", EventID: DemoEventID, TeamID: DemoTeamID,
			Role: domain.ParticipationCaptain, CreatedAt: created,
		},
		domain.Participation{
			ID: "par_demo_02", UserID: "participant_other", EventID: DemoEventID, TeamID: DemoTeamID,
			Role: domain.ParticipationMember, CreatedAt: created,
		},
	)

	// One draft submission, so the demo starts from "there is something to
	// submit" rather than from an empty form.
	data.Submissions = append(data.Submissions, domain.Submission{
		ID: DemoProjectID, EventID: DemoEventID, TeamID: DemoTeamID, TrackID: DemoTrackID,
		Title: "Lantern Index", Summary: "A draft waiting to be submitted.",
		Description:   "Seeded as a draft so the submit transition can be demonstrated.",
		RepositoryURL: "https://example.org/lantern-index", Tags: []string{"demo"},
		Status: domain.SubmissionDraft, Eligibility: domain.EligibilityEligible,
		UpdatedAt: created, Version: 1,
	})

	// Both seeded judges are assigned to the demo project with no review yet,
	// so a judge can open the console and score it live.
	for index, judgeID := range []string{"jdg_24", "jdg_07"} {
		data.JudgeRoster = append(data.JudgeRoster, domain.JudgeRosterEntry{
			EventID: DemoEventID, JudgeID: judgeID, Scope: domain.JudgeScopeHackathon,
			Headline: "Seeded demo panelist", Active: true, AddedBy: "organizer", CreatedAt: created,
		})
		data.JudgeProfiles = append(data.JudgeProfiles, domain.JudgeProfile{
			UserID: judgeID, Bio: "Seeded demo panelist.", Capacity: 25, Active: true,
		})
		data.Assignments = append(data.Assignments, domain.Assignment{
			ID: DemoAssignmentID(index), EventID: DemoEventID, JudgeID: judgeID,
			ProjectID: DemoProjectID, AssignedBy: "organizer",
			Strategy: domain.AssignmentManual, CreatedAt: created,
		})
	}

	data.Rubrics = append(data.Rubrics, domain.Rubric{
		ID: DemoRubricID, EventID: DemoEventID, Name: "Demo rubric", Version: 1,
		Status: domain.RubricPublished, CreatedBy: "organizer", CreatedAt: created, PublishedAt: published,
		Criteria: []domain.RubricCriterion{
			{Key: "functionality", Label: "Functionality", Description: "Does it work end to end?", MinScore: 1, MaxScore: 5, Weight: 40, Required: true},
			{Key: "quality", Label: "Quality", Description: "Is it clear, complete and well built?", MinScore: 1, MaxScore: 5, Weight: 35, Required: true},
			{Key: "innovation", Label: "Innovation", Description: "Is the idea meaningfully different?", MinScore: 1, MaxScore: 5, Weight: 25, Required: true},
		},
	})

	data.EventStaff = append(data.EventStaff, domain.EventStaff{
		EventID: DemoEventID, UserID: "organizer", Role: domain.EventRoleOwner,
		Title: "Lead organizer", AddedBy: "organizer", CreatedAt: created,
	})

	data.Activity = append(data.Activity, domain.ActivityEntry{
		ID: DemoActivityID, EventID: DemoEventID, Category: domain.ActivityEvent,
		Visibility: domain.ActivityPublic, Action: "event.created",
		TargetType: "event", TargetID: DemoEventID,
		Summary: "Open Call 2026 was created and submissions are open.",
		ActorID: "organizer", ActorName: "Rhea Organizer", ActorRole: domain.RoleOrganizer,
		Metadata: map[string]any{"submissions_open": true}, CreatedAt: created,
	})

	return data
}

// DemoAssignmentID is the stable assignment id for a demo panelist.
func DemoAssignmentID(index int) string {
	return "asg_demo_" + string(rune('a'+index))
}
