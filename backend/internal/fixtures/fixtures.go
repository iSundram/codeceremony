// Package fixtures loads the shared DOGFOOD fixture file into a seeded portal.
//
// The event publishes one fixtures.json so that every entrant's portal is
// looking at the same invented hackathon. That matters for judging: a reviewer
// comparing two portals should be comparing software, not demo data. It also
// matters for this package, because the fixture deliberately contains awkward
// cases that a hand-written demo seed always smooths away:
//
//   - a judge who gave every project they reviewed the same score (jdg_07), so
//     there is no spread to rescale against;
//   - 8 of 30 judges with fewer than three reviews, so their scale is barely
//     estimable;
//   - projects with two reviews next to projects with five, because not every
//     judge finishes every batch;
//   - one duplicate submission (prj_41 repeats prj_07's team, title and repo).
//
// The file is input, not a data model. This loader transforms it into the
// portal's own schema: judges become user accounts, score rows become reviews
// plus the assignments that justify them, and email-only team rosters become
// users and memberships.
package fixtures

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/seed"
)

// file is the on-disk shape published by the event. Every id is a string and
// every timestamp is ISO 8601 UTC.
type file struct {
	Event struct {
		ID               string `json:"id"`
		Name             string `json:"name"`
		SubmissionsClose string `json:"submissions_close"`
	} `json:"event"`
	Tracks []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"tracks"`
	Judges []struct {
		ID     string   `json:"id"`
		Name   string   `json:"name"`
		Email  string   `json:"email"`
		Tracks []string `json:"tracks"`
	} `json:"judges"`
	Teams []struct {
		ID      string   `json:"id"`
		Name    string   `json:"name"`
		Members []string `json:"members"`
	} `json:"teams"`
	Projects []struct {
		ID          string `json:"id"`
		Team        string `json:"team"`
		Track       string `json:"track"`
		Title       string `json:"title"`
		Summary     string `json:"summary"`
		RepoURL     string `json:"repo_url"`
		SubmittedAt string `json:"submitted_at"`
	} `json:"projects"`
	Scores []struct {
		Judge    string         `json:"judge"`
		Project  string         `json:"project"`
		Criteria map[string]int `json:"criteria"`
		Comment  string         `json:"comment"`
	} `json:"scores"`
}

// Load reads the fixture file and folds it into the base seed data.
//
// Fixtures win over the base seed for anything that describes event content,
// because the shared file is the shared source of truth. Base seed rows for the
// same event that fixtures do not mention are kept only when they do not
// collide on id, so the seeded organizer, admin and rubric survive while the
// four hand-written demo projects give way to the real forty-one.
func Load(path, passwordHash string) (seed.Data, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return seed.Data{}, fmt.Errorf("fixtures: %w", err)
	}
	var parsed file
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parsed); err != nil {
		return seed.Data{}, fmt.Errorf("fixtures: %s: %w", path, err)
	}
	return Apply(parsed, passwordHash)
}

// Parse folds an already-decoded fixture document into seed data.
func Parse(document []byte, passwordHash string) (seed.Data, error) {
	var parsed file
	if err := json.Unmarshal(document, &parsed); err != nil {
		return seed.Data{}, fmt.Errorf("fixtures: %w", err)
	}
	return Apply(parsed, passwordHash)
}

// Apply is the transformation itself, separated from file IO so it can be
// tested directly and reused by the bulk importer.
func Apply(parsed file, passwordHash string) (seed.Data, error) {
	data := seed.Default(passwordHash)
	if strings.TrimSpace(parsed.Event.ID) == "" {
		return seed.Data{}, fmt.Errorf("fixtures: event id is required")
	}
	eventID := parsed.Event.ID
	closeAt, err := parseTime(parsed.Event.SubmissionsClose)
	if err != nil {
		return seed.Data{}, fmt.Errorf("fixtures: event submissions_close: %w", err)
	}
	if len(parsed.Projects) == 0 {
		return seed.Data{}, fmt.Errorf("fixtures: at least one project is required")
	}
	if len(parsed.Scores) == 0 {
		return seed.Data{}, fmt.Errorf("fixtures: at least one score is required")
	}

	// The event's lifecycle comes from the fixture so that seeding honestly
	// lands on a closed event. The acceptance checker relies on a closed
	// fixture event refusing submissions, and that is only true if the seeded
	// state is derived from the same deadline the fixture declares.
	event, err := findEvent(data.Events, eventID)
	if err != nil {
		return seed.Data{}, err
	}
	if strings.TrimSpace(parsed.Event.Name) != "" {
		event.Name = parsed.Event.Name
		event.Slug = slugify(parsed.Event.Name)
	}
	event.SubmissionsClose = closeAt
	event.SubmissionsOpen = false
	event.State = domain.HackathonSubmissionsClosed
	event.JudgingClose = closeAt.Add(96 * time.Hour)
	event.ResultsPublished = false
	event.LeaderboardPublic = false
	// Reviewing is complete in the fixture, so the event is sitting in the
	// judging phase with results deliberately withheld.
	event.State = domain.HackathonJudging

	// Users. Fixture judges and fixture team members both arrive as real
	// accounts: a judge has to log in to score, and a team member has to be
	// able to edit the team's submission.
	users := make(map[string]domain.User)
	for _, existing := range data.Users {
		users[existing.ID] = existing
	}
	byEmail := make(map[string]domain.User)
	for _, existing := range data.Users {
		byEmail[strings.ToLower(existing.Email)] = existing
	}

	created := time.Date(2026, time.January, 15, 9, 0, 0, 0, time.UTC)

	judgeProfiles := make([]domain.JudgeProfile, 0, len(parsed.Judges))
	roster := make([]domain.JudgeRosterEntry, 0, len(parsed.Judges))
	judgeEmails := make(map[string]string, len(parsed.Judges))
	for _, judge := range parsed.Judges {
		if _, exists := judgeEmails[judge.ID]; exists {
			return seed.Data{}, fmt.Errorf("fixtures: judge %s appears twice", judge.ID)
		}
		judgeEmails[judge.ID] = judge.Email
		judgeTracks := append([]string(nil), judge.Tracks...)
		sort.Strings(judgeTracks)
		users[judge.ID] = domain.User{
			ID: judge.ID, Email: judge.Email, DisplayName: judge.Name,
			Role: domain.RoleJudge, State: domain.AccountActive,
			PasswordHash: passwordHash, CreatedAt: created,
		}
		judgeProfiles = append(judgeProfiles, domain.JudgeProfile{
			UserID: judge.ID, Bio: "Fixture judge.", Tracks: judgeTracks, Capacity: 25, Active: true,
		})
		roster = append(roster, domain.JudgeRosterEntry{
			EventID: eventID, JudgeID: judge.ID, Scope: domain.JudgeScopeHackathon,
			Headline: strings.Join(judgeTracks, ", "), Active: true, AddedBy: "organizer", CreatedAt: created,
		})
	}

	// Teams. The fixture identifies members only by email, so a stable
	// user id is derived from the address. That keeps ids identical across
	// boots and across machines, which matters because those ids end up in
	// membership rows and in the public gallery.
	teams := make([]domain.Team, 0, len(parsed.Teams))
	memberships := make([]domain.TeamMembership, 0, len(parsed.Teams)*2)
	participations := make([]domain.Participation, 0, len(parsed.Teams)*2)
	trackNames := make(map[string]string, len(parsed.Tracks))

	for _, team := range parsed.Teams {
		if len(team.Members) == 0 {
			return seed.Data{}, fmt.Errorf("fixtures: team %s has no members", team.ID)
		}
		captainID := userIDForEmail(team.Members[0])
		if _, exists := users[captainID]; !exists {
			users[captainID] = domain.User{
				ID: captainID, Email: strings.ToLower(team.Members[0]),
				DisplayName: displayName(team.Members[0]), Role: domain.RoleParticipant,
				State: domain.AccountActive, PasswordHash: passwordHash, CreatedAt: created,
			}
		}
		teams = append(teams, domain.Team{
			ID: team.ID, EventID: eventID, Scope: domain.TeamScopeHackathon,
			Name: team.Name, Description: "Fixture team.", CaptainID: captainID,
			Status: domain.TeamStatusActive, Availability: domain.TeamInviteOnly, CreatedAt: created,
		})
		for index, member := range team.Members {
			userID := userIDForEmail(member)
			if _, exists := users[userID]; !exists {
				users[userID] = domain.User{
					ID: userID, Email: strings.ToLower(member), DisplayName: displayName(member),
					Role: domain.RoleParticipant, State: domain.AccountActive,
					PasswordHash: passwordHash, CreatedAt: created,
				}
			}
			role := domain.TeamRoleMember
			if index == 0 {
				role = domain.TeamRoleCaptain
			}
			memberships = append(memberships, domain.TeamMembership{
				ID:      fmt.Sprintf("tmem_%s_%d", strings.TrimPrefix(team.ID, "tm_"), index+1),
				EventID: eventID, TeamID: team.ID, UserID: userID, Role: role,
				Status: "active", JoinedAt: created, UpdatedAt: created,
			})
			participations = append(participations, domain.Participation{
				ID: "par_" + userID + "_" + team.ID, UserID: userID, EventID: eventID,
				TeamID: team.ID, Role: domain.ParticipationMember, CreatedAt: created,
			})
		}
		participations[len(participations)-len(team.Members)].Role = domain.ParticipationCaptain
	}

	// Tracks. Fixture tracks are authoritative; any base seed track with the
	// same id is replaced, and any base track the fixture does not mention is
	// dropped so the published track list is exactly the fixture's.
	tracks := make([]domain.Track, 0, len(parsed.Tracks))
	order := 0
	for _, track := range parsed.Tracks {
		order++
		trackNames[track.ID] = track.Name
		tracks = append(tracks, domain.Track{
			ID: track.ID, Event: eventID, Name: track.Name, Slug: slugify(track.Name), Order: order,
		})
	}

	// Projects.
	submissions := make([]domain.Submission, 0, len(parsed.Projects))
	knownTeams := make(map[string]bool, len(parsed.Teams))
	for _, team := range parsed.Teams {
		knownTeams[team.ID] = true
	}
	knownTracks := make(map[string]bool, len(parsed.Tracks))
	for _, track := range parsed.Tracks {
		knownTracks[track.ID] = true
	}
	for _, project := range parsed.Projects {
		if !knownTeams[project.Team] {
			return seed.Data{}, fmt.Errorf("fixtures: project %s references unknown team %s", project.ID, project.Team)
		}
		if !knownTracks[project.Track] {
			return seed.Data{}, fmt.Errorf("fixtures: project %s references unknown track %s", project.ID, project.Track)
		}
		submittedAt, err := parseTime(project.SubmittedAt)
		if err != nil {
			return seed.Data{}, fmt.Errorf("fixtures: project %s submitted_at: %w", project.ID, err)
		}
		submissions = append(submissions, domain.Submission{
			ID: project.ID, EventID: eventID, TeamID: project.Team, TrackID: project.Track,
			Title: project.Title, Summary: project.Summary, Description: project.Summary,
			RepositoryURL: project.RepoURL, Tags: []string{strings.ToLower(trackNames[project.Track])},
			Status: domain.SubmissionSubmitted, Eligibility: domain.EligibilityEligible,
			SubmittedAt: &submittedAt, UpdatedAt: submittedAt, Version: 1,
		})
	}

	// Scores become reviews, and each review is backed by the assignment that
	// justifies it. The store refuses a review from a judge who was not
	// assigned the project, so synthesising the assignments here is what keeps
	// the fixture panel self-consistent and lets the judge console show a real
	// batch instead of an empty one.
	knownProjects := make(map[string]bool, len(parsed.Projects))
	for _, project := range parsed.Projects {
		knownProjects[project.ID] = true
	}
	knownJudges := make(map[string]bool, len(parsed.Judges))
	for _, judge := range parsed.Judges {
		knownJudges[judge.ID] = true
	}
	reviews := make([]domain.Review, 0, len(parsed.Scores))
	assignments := make([]domain.Assignment, 0, len(parsed.Scores))
	seen := make(map[string]bool, len(parsed.Scores))
	for index, score := range parsed.Scores {
		if !knownJudges[score.Judge] {
			return seed.Data{}, fmt.Errorf("fixtures: score %d references unknown judge %s", index, score.Judge)
		}
		if !knownProjects[score.Project] {
			return seed.Data{}, fmt.Errorf("fixtures: score %d references unknown project %s", index, score.Project)
		}
		key := score.Judge + "|" + score.Project
		if seen[key] {
			// The store enforces one review per judge per project. A repeated
			// row is a fixture defect, not a second opinion.
			return seed.Data{}, fmt.Errorf("fixtures: judge %s scores project %s more than once", score.Judge, score.Project)
		}
		seen[key] = true
		if len(score.Criteria) == 0 {
			return seed.Data{}, fmt.Errorf("fixtures: score %d has no criteria", index)
		}
		criteria := make(map[string]int, len(score.Criteria))
		for criterion, value := range score.Criteria {
			criteria[criterion] = value
		}
		reviews = append(reviews, domain.Review{
			ID:      fmt.Sprintf("rev_%s_%s", score.Judge, score.Project),
			EventID: eventID, JudgeID: score.Judge, ProjectID: score.Project,
			Criteria: criteria, Comment: score.Comment, Submitted: true,
			SubmittedAt: &closeAt, UpdatedAt: closeAt,
		})
		assignments = append(assignments, domain.Assignment{
			ID:         fmt.Sprintf("asg_%s_%s", score.Judge, score.Project),
			EventID:    eventID,
			JudgeID:    score.Judge,
			ProjectID:  score.Project,
			AssignedBy: "organizer", Strategy: domain.AssignmentBatch, CreatedAt: created,
		})
	}

	// The fixture contains one deliberate duplicate. Flagging it here means the
	// duplicate shows up in the organizer's queue on first boot, which is the
	// whole point of shipping an awkward case in a shared fixture.
	duplicates := detectDuplicates(eventID, submissions, created)

	// Rubric. The criteria in the fixture drive the weights, so a portal that
	// loads a differently shaped fixture still produces a coherent rubric.
	criterionKeys := make([]string, 0, 4)
	seenCriterion := make(map[string]bool)
	for _, review := range reviews {
		for criterion := range review.Criteria {
			if !seenCriterion[criterion] {
				seenCriterion[criterion] = true
				criterionKeys = append(criterionKeys, criterion)
			}
		}
	}
	sort.Strings(criterionKeys)

	// The base seed carries two hand-written demo judges so that a clone with
	// no fixture file still has a working judging console. Once the fixture
	// panel is loaded they are superseded, and leaving them in place is worse
	// than having no demo at all: they share the demo email addresses, so
	// signing in as the documented judge would authenticate a judge with an
	// empty batch while the real panel sat unreachable. Retire them and
	// everything that referenced them, and let the credential table in
	// seed/identities.go point at fixture judges instead.
	superseded := make(map[string]bool)
	panel := make(map[string]bool, len(parsed.Judges))
	for _, judge := range parsed.Judges {
		panel[judge.ID] = true
	}
	for _, user := range data.Users {
		if user.Role == domain.RoleJudge && !panel[user.ID] {
			superseded[user.ID] = true
			delete(users, user.ID)
		}
	}
	data.Users = sortedUsers(users)
	// The fixture panel replaces the base seed's panel wholesale. Assigning from
	// data.JudgeRoster here instead of the roster built above would silently
	// discard all thirty fixture judges and leave the organizer looking at an
	// empty panel.
	data.JudgeProfiles = judgeProfiles
	data.JudgeRoster = roster
	data.Assignments = assignments
	data.Reviews = reviews
	data.Events = replaceEvent(data.Events, *event)
	data.Tracks = tracks
	data.Teams = teams
	data.TeamMemberships = memberships
	data.Participations = participations
	data.Submissions = submissions
	data.Assignments = assignments
	data.Reviews = reviews
	data.Duplicates = duplicates
	data.Rubrics = buildRubrics(eventID, criterionKeys, created)
	return data, nil
}

// buildRubrics produces a published rubric whose weights total exactly 100, as
// the rubric validator requires. With three criteria the ladder is 40/35/25,
// which matches the DOGFOOD default; with any other count the remainder is
// spread evenly so the weights always total 100 and are always positive.
func buildRubrics(eventID string, criteria []string, created time.Time) []domain.Rubric {
	if len(criteria) == 0 {
		return nil
	}
	weights := distribute(len(criteria))
	// EventID is not optional. The store matches an active rubric on its event,
	// so a rubric with no event is never active, and every result would then
	// silently fall back to hardcoded weights instead of the weights the
	// organizer actually configured.
	rubric := domain.Rubric{
		ID: "rub_fixtures", EventID: eventID, Name: "Fixture rubric", Version: 1,
		Status: domain.RubricPublished, CreatedBy: "organizer", CreatedAt: created,
		PublishedAt:  &created,
		Instructions: "Scored on the shared DOGFOOD fixture scale, one to five.",
	}
	for index, criterion := range criteria {
		rubric.Criteria = append(rubric.Criteria, domain.RubricCriterion{
			Key: criterion, Label: titleCase(criterion), Description: "Scored on the shared DOGFOOD fixture scale.",
			MinScore: 1, MaxScore: 5, Weight: weights[index], Required: true,
		})
	}
	return []domain.Rubric{rubric}
}

// weightLadder is the default weighting, most important criterion first. It is
// used for a rubric with a plausible number of criteria.
var weightLadder = []int{40, 35, 25, 20, 15, 12, 10, 8}

// distribute returns n positive integer weights that total exactly 100, which
// the rubric validator requires.
//
// It apportions by largest remainder: take a target for each weight, round
// down, then hand the leftover points to the largest fractional parts. That is
// the standard way to split an indivisible total and it guarantees the sum is
// exactly 100 with no rounding drift.
//
// Past the length of the ladder an even share is used instead, because a 25
// criterion rubric cannot be given 25 strictly positive weights that follow a
// descending ladder, and a zero weight fails validation outright.
func distribute(n int) []int {
	if n <= 0 {
		return nil
	}
	if n == 1 {
		return []int{100}
	}
	if n > 100 {
		// A rubric with more than a hundred criteria cannot have 100 points
		// split into 100-or-more positive parts. Collapse it rather than emit
		// zeros; the caller still gets a valid 100 total.
		n = 100
	}

	targets := make([]int, n)
	if n <= len(weightLadder) {
		copy(targets, weightLadder[:n])
	} else {
		// floor(100/n) is at least 1 for every n this branch can reach.
		share := 100 / n
		for i := range targets {
			targets[i] = share
		}
	}

	total := sumInts(targets)
	weights := make([]int, n)
	remainders := make([]int, n)
	assigned := 0
	for i, value := range targets {
		scaled := value * 100
		weights[i] = scaled / total
		remainders[i] = scaled % total
		assigned += weights[i]
	}
	for point := 100 - assigned; point > 0; point-- {
		best, bestValue := -1, -1
		for i, remainder := range remainders {
			if remainder > bestValue {
				best, bestValue = i, remainder
			}
		}
		if best < 0 {
			break
		}
		weights[best]++
		remainders[best] = -1
	}
	return weights
}

func sumInts(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

// detectDuplicates flags projects that repeat another project's team, title or
// repository. This is the same signal set the organizer's duplicate scan uses,
// run once at seed time so the awkward case is visible without anyone having to
// press the button.
func detectDuplicates(eventID string, submissions []domain.Submission, created time.Time) []domain.DuplicateFlag {
	type key struct{ team, title, repo string }
	groups := make(map[key][]string)
	for _, submission := range submissions {
		group := key{
			team:  submission.TeamID,
			title: domain.NormalizeTitle(submission.Title),
			repo:  domain.NormalizeURL(submission.RepositoryURL),
		}
		groups[group] = append(groups[group], submission.ID)
	}
	flags := make([]domain.DuplicateFlag, 0)
	keys := make([]key, 0, len(groups))
	for group := range groups {
		keys = append(keys, group)
	}
	sort.Slice(keys, func(i, j int) bool { return groups[keys[i]][0] < groups[keys[j]][0] })
	for _, group := range keys {
		members := groups[group]
		if len(members) < 2 {
			continue
		}
		sort.Strings(members)
		// Flag every member against the lowest id, so the pair is recorded
		// symmetrically and the earliest submission is the reference.
		for _, projectID := range members[1:] {
			flags = append(flags, domain.DuplicateFlag{
				ID: "dup_" + projectID + "_" + members[0], EventID: eventID,
				ProjectID: projectID, DuplicateOfProjectID: members[0],
				Signal: "team_title_repo", Detail: "Same team, title and repository as " + members[0] + ".",
				Status: domain.DuplicateOpen, CreatedAt: created,
			})
		}
	}
	return flags
}

func findEvent(events []domain.Event, id string) (*domain.Event, error) {
	for index := range events {
		if events[index].ID == id {
			return &events[index], nil
		}
	}
	return nil, fmt.Errorf("fixtures: base seed has no event %s to extend", id)
}

func replaceEvent(events []domain.Event, event domain.Event) []domain.Event {
	for index := range events {
		if events[index].ID == event.ID {
			events[index] = event
			return events
		}
	}
	return append(events, event)
}

func sortedUsers(users map[string]domain.User) []domain.User {
	ids := make([]string, 0, len(users))
	for id := range users {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	list := make([]domain.User, 0, len(ids))
	for _, id := range ids {
		list = append(list, users[id])
	}
	return list
}

// userIDForEmail derives a stable, collision-resistant account id from an
// address. Fixtures identify team members by email only, so the id has to be
// derived rather than assigned, and it has to be the same on every machine.
func userIDForEmail(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	sum := sha256.Sum256([]byte(normalized))
	return "usr_" + hex.EncodeToString(sum[:])[:12]
}

func displayName(email string) string {
	local := strings.TrimSpace(strings.SplitN(email, "@", 2)[0])
	local = strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(local)
	if local == "" {
		return email
	}
	return titleCase(local)
}

func parseTime(value string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("timestamp is empty")
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is not ISO 8601: %w", value, err)
	}
	return parsed.UTC(), nil
}

var nonAlphanumeric = strings.NewReplacer(" ", "-", "_", "-", "/", "-")

func slugify(value string) string {
	lowered := strings.ToLower(strings.TrimSpace(value))
	slug := nonAlphanumeric.Replace(lowered)
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	return strings.Trim(slug, "-")
}

func titleCase(value string) string {
	fields := strings.Fields(value)
	for index, field := range fields {
		if field == "" {
			continue
		}
		fields[index] = strings.ToUpper(field[:1]) + field[1:]
	}
	return strings.Join(fields, " ")
}
