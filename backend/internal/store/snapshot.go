package store

import (
	"sort"
	"strings"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/authz"
	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/seed"
)

// SnapshotVersion is the on-disk format version. It is checked on load so a
// portal started against an older data directory fails with a clear message
// instead of silently booting with half its data missing.
const SnapshotVersion = 1

// Snapshot is the complete durable state of a portal.
//
// # Why a snapshot and not a database
//
// The target deployment is a self-hosted portal for one event, run on one
// machine, by whoever is organising that event. A snapshot file gives that
// deployment durability and a trivially inspectable backup without adding a
// database process to the one-command rule, and it keeps the offline
// requirement honest: there is nothing to install and nothing to reach.
//
// The cost is that reads are linear scans and writes are whole-file, which is
// the wrong shape for a multi-tenant SaaS and the right shape for a few thousand
// projects. Snapshot is a stable, documented, importable format, so moving to
// Postgres later is a matter of writing a loader rather than a migration story
// that nobody can reconstruct. DATA-MODEL.md documents the format.
//
// # What is deliberately not here
//
// Sessions are excluded. Token hashes are not serialised anywhere in this
// codebase, and a portal restart therefore signs everyone out. That is the
// correct trade for a judging platform: a stolen session cookie should not
// survive a redeploy, and there is no session store to steal from in the first
// place. Seeded identities are re-minted at every boot from the fixed token
// table, so automation that attaches a documented header keeps working.
type Snapshot struct {
	Version int           `json:"version"`
	SavedAt time.Time     `json:"saved_at"`
	Seed    bool          `json:"seeded"`
	Users   []domain.User `json:"users"`
	// MailPreferences and UnsubscribeTokens are here because Restore used to
	// rebuild both maps as empty. A restart therefore reverted every recorded
	// opt-out to the defaults, which is the failure a GDPR-style unsubscribe
	// exists to prevent, and it invalidated every unsubscribe link already in an
	// inbox — the links are mailed with a digest and redeemed days later.
	MailPreferences []domain.MailPreferences     `json:"mail_preferences"`
	Unsubscribe     []domain.UnsubscribeToken    `json:"unsubscribe_tokens"`
	Judges          []domain.JudgeProfile        `json:"judge_profiles"`
	Conflicts       []domain.ConflictDeclaration `json:"conflict_declarations"`
	Rubrics         []domain.Rubric              `json:"rubrics"`
	Profiles        []domain.UserProfile         `json:"user_profiles"`
	Events          []domain.Event               `json:"events"`
	Tracks          []domain.Track               `json:"tracks"`
	Prizes          []domain.Prize               `json:"prizes"`
	Questions       []domain.HackathonQuestion   `json:"hackathon_questions"`
	Milestones      []domain.HackathonMilestone  `json:"hackathon_milestones"`
	Hosts           []domain.HackathonHost       `json:"hackathon_hosts"`
	Roster          []domain.JudgeRosterEntry    `json:"judge_roster"`
	Invites         []domain.TeamInvite          `json:"team_invites"`
	Teams           []domain.Team                `json:"teams"`
	Members         []domain.TeamMembership      `json:"team_memberships"`
	Participations  []domain.Participation       `json:"participations"`
	Submissions     []domain.Submission          `json:"submissions"`
	Versions        []domain.SubmissionVersion   `json:"submission_versions"`
	Duplicates      []domain.DuplicateFlag       `json:"duplicate_flags"`
	Assignments     []domain.Assignment          `json:"assignments"`
	Comparisons     []domain.Comparison          `json:"comparisons"`
	Reviews         []domain.Review              `json:"reviews"`
	Comments        []domain.Comment             `json:"comments"`
	Reports         []domain.CommentReport       `json:"comment_reports"`
	Campaigns       []domain.VoteCampaign        `json:"vote_campaigns"`
	Ballots         []domain.Ballot              `json:"ballots"`
	Webhooks        []domain.Webhook             `json:"webhooks"`
	Staff           []domain.EventStaff          `json:"event_staff"`
	Activity        []domain.ActivityEntry       `json:"activity_entries"`
	Notifications   []domain.Notification        `json:"notifications"`
	AuditEvents     []domain.AuditEvent          `json:"audit_events"`
	Grants          []authz.Grant                `json:"grants"`
}

// Snapshot captures the current durable state.
//
// Callers must hold at least a read lock; Snapshot takes it itself. Every slice
// is returned in a deterministic order so two snapshots of identical state are
// byte-identical, which is what makes the journal's change detection meaningful
// and a data file diffable in review.
func (s *Store) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// SavedAt is deliberately left zero here. It describes the file, not the
	// state: stamping it on every capture would make an unchanged store look
	// changed to the journal's digest and rewrite the data file on every tick.
	snapshot := Snapshot{
		Version:        SnapshotVersion,
		Users:          make([]domain.User, 0, len(s.users)),
		Judges:         make([]domain.JudgeProfile, 0, len(s.judgeProfiles)),
		Conflicts:      make([]domain.ConflictDeclaration, 0, len(s.conflicts)),
		Rubrics:        make([]domain.Rubric, 0, len(s.rubrics)),
		Profiles:       make([]domain.UserProfile, 0, len(s.profiles)),
		Events:         make([]domain.Event, 0, len(s.events)),
		Tracks:         make([]domain.Track, 0, len(s.tracks)),
		Prizes:         make([]domain.Prize, 0, len(s.prizes)),
		Questions:      make([]domain.HackathonQuestion, 0, len(s.questions)),
		Milestones:     make([]domain.HackathonMilestone, 0, len(s.milestones)),
		Hosts:          make([]domain.HackathonHost, 0, len(s.hosts)),
		Roster:         make([]domain.JudgeRosterEntry, 0, len(s.roster)),
		Invites:        make([]domain.TeamInvite, 0, len(s.invites)),
		Teams:          make([]domain.Team, 0, len(s.teams)),
		Members:        make([]domain.TeamMembership, 0, len(s.memberships)),
		Participations: make([]domain.Participation, 0, len(s.participations)),
		Submissions:    make([]domain.Submission, 0, len(s.submissions)),
		Versions:       make([]domain.SubmissionVersion, 0, len(s.submissionVersions)),
		Duplicates:     make([]domain.DuplicateFlag, 0, len(s.duplicates)),
		Assignments:    make([]domain.Assignment, 0, len(s.assignments)),
		Comparisons:    make([]domain.Comparison, 0, len(s.comparisons)),
		Reviews:        make([]domain.Review, 0, len(s.reviews)),
		Comments:       make([]domain.Comment, 0, len(s.comments)),
		Reports:        make([]domain.CommentReport, 0, len(s.reports)),
		Campaigns:      make([]domain.VoteCampaign, 0, len(s.campaigns)),
		Ballots:        make([]domain.Ballot, 0, len(s.ballots)),
		Webhooks:       make([]domain.Webhook, 0, len(s.webhooks)),
		Staff:          make([]domain.EventStaff, 0, len(s.staff)),
		Activity:       make([]domain.ActivityEntry, 0, len(s.activity)),
		Notifications:  make([]domain.Notification, 0, len(s.notifications)),
		AuditEvents:    make([]domain.AuditEvent, 0, len(s.auditEvents)),
	}

	for _, user := range s.users {
		// The bcrypt hash is persisted. See domain.User.PasswordHash for why
		// that trade was made and what it costs: the data file is now something
		// to protect, and it always was, because it holds every review, every
		// grant and every audit entry in the event.
		snapshot.Users = append(snapshot.Users, user)
	}
	for _, preferences := range s.mailPreferences {
		snapshot.MailPreferences = append(snapshot.MailPreferences, preferences)
	}
	for _, token := range s.unsubscribe {
		snapshot.Unsubscribe = append(snapshot.Unsubscribe, token)
	}
	for _, profile := range s.judgeProfiles {
		snapshot.Judges = append(snapshot.Judges, profile)
	}
	for _, conflict := range s.conflicts {
		snapshot.Conflicts = append(snapshot.Conflicts, conflict)
	}
	for _, rubric := range s.rubrics {
		snapshot.Rubrics = append(snapshot.Rubrics, rubric.Clone())
	}
	for _, profile := range s.profiles {
		snapshot.Profiles = append(snapshot.Profiles, profile.Clone())
	}
	for _, event := range s.events {
		snapshot.Events = append(snapshot.Events, event)
	}
	for _, track := range s.tracks {
		snapshot.Tracks = append(snapshot.Tracks, track)
	}
	for _, prize := range s.prizes {
		snapshot.Prizes = append(snapshot.Prizes, prize)
	}
	for _, question := range s.questions {
		snapshot.Questions = append(snapshot.Questions, question)
	}
	for _, milestone := range s.milestones {
		snapshot.Milestones = append(snapshot.Milestones, milestone)
	}
	for _, host := range s.hosts {
		snapshot.Hosts = append(snapshot.Hosts, host)
	}
	for _, invite := range s.invites {
		snapshot.Invites = append(snapshot.Invites, invite.Clone())
	}
	for _, team := range s.teams {
		snapshot.Teams = append(snapshot.Teams, team)
	}
	for _, membership := range s.memberships {
		snapshot.Members = append(snapshot.Members, membership)
	}
	for _, participation := range s.participations {
		snapshot.Participations = append(snapshot.Participations, participation)
	}
	for _, submission := range s.submissions {
		snapshot.Submissions = append(snapshot.Submissions, cloneSubmission(submission))
	}
	for _, version := range s.submissionVersions {
		snapshot.Versions = append(snapshot.Versions, cloneVersion(version))
	}
	for _, duplicate := range s.duplicates {
		snapshot.Duplicates = append(snapshot.Duplicates, duplicate)
	}
	for _, assignment := range s.assignments {
		snapshot.Assignments = append(snapshot.Assignments, assignment)
	}
	for _, comparison := range s.comparisons {
		snapshot.Comparisons = append(snapshot.Comparisons, comparison)
	}
	for _, review := range s.reviews {
		snapshot.Reviews = append(snapshot.Reviews, cloneReview(review))
	}
	for _, comment := range s.comments {
		snapshot.Comments = append(snapshot.Comments, comment)
	}
	for _, report := range s.reports {
		snapshot.Reports = append(snapshot.Reports, report)
	}
	for _, campaign := range s.campaigns {
		snapshot.Campaigns = append(snapshot.Campaigns, campaign)
	}
	for _, ballot := range s.ballots {
		snapshot.Ballots = append(snapshot.Ballots, ballot)
	}
	for _, webhook := range s.webhooks {
		snapshot.Webhooks = append(snapshot.Webhooks, webhook)
	}
	for _, notification := range s.notifications {
		snapshot.Notifications = append(snapshot.Notifications, notification)
	}
	for _, event := range s.auditEvents {
		snapshot.AuditEvents = append(snapshot.AuditEvents, event)
	}
	for _, grant := range s.grants {
		snapshot.Grants = append(snapshot.Grants, grant)
	}
	snapshot.Users = sortBy(snapshot.Users, func(u domain.User) string { return u.ID })
	snapshot.Judges = sortBy(snapshot.Judges, func(p domain.JudgeProfile) string { return p.UserID })
	snapshot.Conflicts = sortBy(snapshot.Conflicts, func(c domain.ConflictDeclaration) string { return c.ID })
	snapshot.Rubrics = sortBy(snapshot.Rubrics, func(r domain.Rubric) string { return r.ID })
	snapshot.Profiles = sortBy(snapshot.Profiles, func(p domain.UserProfile) string { return p.UserID })
	snapshot.Events = sortBy(snapshot.Events, func(e domain.Event) string { return e.ID })
	snapshot.Tracks = sortBy(snapshot.Tracks, func(t domain.Track) string { return t.ID })
	snapshot.Prizes = sortBy(snapshot.Prizes, func(p domain.Prize) string { return p.ID })
	snapshot.Questions = sortBy(snapshot.Questions, func(q domain.HackathonQuestion) string { return q.ID })
	snapshot.Milestones = sortBy(snapshot.Milestones, func(m domain.HackathonMilestone) string { return m.ID })
	snapshot.Hosts = sortBy(snapshot.Hosts, func(h domain.HackathonHost) string { return h.ID })
	snapshot.Invites = sortBy(snapshot.Invites, func(i domain.TeamInvite) string { return i.ID })
	snapshot.Teams = sortBy(snapshot.Teams, func(t domain.Team) string { return t.ID })
	snapshot.Members = sortBy(snapshot.Members, func(m domain.TeamMembership) string { return membershipKey(m.TeamID, m.UserID) })
	snapshot.Participations = sortBy(snapshot.Participations, func(p domain.Participation) string { return participationKey(p.UserID, p.EventID) })
	snapshot.Submissions = sortBy(snapshot.Submissions, func(s domain.Submission) string { return s.ID })
	snapshot.Versions = sortBy(snapshot.Versions, func(v domain.SubmissionVersion) string { return v.ID })
	snapshot.Duplicates = sortBy(snapshot.Duplicates, func(d domain.DuplicateFlag) string { return d.ID })
	snapshot.Assignments = sortBy(snapshot.Assignments, func(a domain.Assignment) string { return a.ID })
	snapshot.Comparisons = sortBy(snapshot.Comparisons, func(c domain.Comparison) string { return c.ID })
	snapshot.Reviews = sortBy(snapshot.Reviews, func(r domain.Review) string { return reviewKey(r.JudgeID, r.ProjectID) })
	snapshot.Comments = sortBy(snapshot.Comments, func(c domain.Comment) string { return c.ID })
	snapshot.Reports = sortBy(snapshot.Reports, func(r domain.CommentReport) string { return r.ID })
	snapshot.Campaigns = sortBy(snapshot.Campaigns, func(c domain.VoteCampaign) string { return c.ID })
	snapshot.Ballots = sortBy(snapshot.Ballots, func(b domain.Ballot) string { return b.ID })
	snapshot.Webhooks = sortBy(snapshot.Webhooks, func(w domain.Webhook) string { return w.ID })
	snapshot.Notifications = sortBy(snapshot.Notifications, func(n domain.Notification) string { return n.ID })
	snapshot.AuditEvents = sortBy(snapshot.AuditEvents, func(a domain.AuditEvent) string { return a.ID })
	snapshot.Grants = sortBy(snapshot.Grants, func(g authz.Grant) string { return g.ID })

	// Roster, staff and activity are keyed by a composite, so they are sorted
	// through the same key helpers the store itself uses.
	roster := make([]domain.JudgeRosterEntry, 0, len(s.roster))
	for _, entry := range s.roster {
		roster = append(roster, entry)
	}
	sort.Slice(roster, func(i, j int) bool {
		return rosterKey(roster[i].EventID, roster[i].JudgeID) < rosterKey(roster[j].EventID, roster[j].JudgeID)
	})
	snapshot.Roster = roster

	staff := make([]domain.EventStaff, 0, len(s.staff))
	for _, member := range s.staff {
		staff = append(staff, member)
	}
	sort.Slice(staff, func(i, j int) bool {
		return staffKey(staff[i].EventID, staff[i].UserID) < staffKey(staff[j].EventID, staff[j].UserID)
	})
	snapshot.Staff = staff

	activity := make([]domain.ActivityEntry, 0, len(s.activity))
	for _, entry := range s.activity {
		activity = append(activity, entry)
	}
	sort.Slice(activity, func(i, j int) bool { return activity[i].ID < activity[j].ID })
	snapshot.Activity = activity

	return snapshot
}

// Restore replaces the store's contents with a snapshot. Existing state is
// discarded, and the secondary email index and composite-keyed maps are rebuilt
// from scratch rather than merged, so a restore can never leave a stale index
// pointing at a row that is no longer there.
// AccountCredentialHealth reports whether a restored store can authenticate
// anyone at all, and how many accounts it can.
//
// It exists because the failure it catches is silent by construction. Password
// hashes were not persisted, so a restart produced a store full of accounts and
// zero passwords: every login returned 401, no route reported anything wrong,
// and the only credentials that still worked were the fixed public seed tokens.
// An operator sees a healthy portal and a login page that never works. A boot
// that checks this can refuse, or at least say so, instead.
func (s *Store) AccountCredentialHealth() (withHash, withoutHash int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, user := range s.users {
		if user.PasswordHash == "" {
			withoutHash++
			continue
		}
		withHash++
	}
	return withHash, withoutHash
}

func (s *Store) Restore(snapshot Snapshot) error {
	if snapshot.Version != SnapshotVersion {
		return ErrSnapshotVersion
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.users = make(map[string]domain.User, len(snapshot.Users))
	s.usersByMail = make(map[string]string, len(snapshot.Users))
	s.judgeProfiles = make(map[string]domain.JudgeProfile, len(snapshot.Judges))
	s.conflicts = make(map[string]domain.ConflictDeclaration, len(snapshot.Conflicts))
	s.rubrics = make(map[string]domain.Rubric, len(snapshot.Rubrics))
	s.profiles = make(map[string]domain.UserProfile, len(snapshot.Profiles))
	s.questions = make(map[string]domain.HackathonQuestion, len(snapshot.Questions))
	s.milestones = make(map[string]domain.HackathonMilestone, len(snapshot.Milestones))
	s.hosts = make(map[string]domain.HackathonHost, len(snapshot.Hosts))
	s.roster = make(map[string]domain.JudgeRosterEntry, len(snapshot.Roster))
	s.invites = make(map[string]domain.TeamInvite, len(snapshot.Invites))
	s.participations = make(map[string]domain.Participation, len(snapshot.Participations))
	s.activity = make(map[string]domain.ActivityEntry, len(snapshot.Activity))
	s.staff = make(map[string]domain.EventStaff, len(snapshot.Staff))
	s.comments = make(map[string]domain.Comment, len(snapshot.Comments))
	s.reports = make(map[string]domain.CommentReport, len(snapshot.Reports))
	s.campaigns = make(map[string]domain.VoteCampaign, len(snapshot.Campaigns))
	s.ballots = make(map[string]domain.Ballot, len(snapshot.Ballots))
	s.webhooks = make(map[string]domain.Webhook, len(snapshot.Webhooks))
	s.events = make(map[string]domain.Event, len(snapshot.Events))
	s.tracks = make(map[string]domain.Track, len(snapshot.Tracks))
	s.prizes = make(map[string]domain.Prize, len(snapshot.Prizes))
	s.teams = make(map[string]domain.Team, len(snapshot.Teams))
	s.memberships = make(map[string]domain.TeamMembership, len(snapshot.Members))
	s.submissions = make(map[string]domain.Submission, len(snapshot.Submissions))
	s.submissionVersions = make(map[string]domain.SubmissionVersion, len(snapshot.Versions))
	s.duplicates = make(map[string]domain.DuplicateFlag, len(snapshot.Duplicates))
	s.assignments = make(map[string]domain.Assignment, len(snapshot.Assignments))
	s.comparisons = make(map[string]domain.Comparison, len(snapshot.Comparisons))
	s.reviews = make(map[string]domain.Review, len(snapshot.Reviews))
	s.notifications = make(map[string]domain.Notification, len(snapshot.Notifications))
	s.auditEvents = make(map[string]domain.AuditEvent, len(snapshot.AuditEvents))
	s.grants = make(map[string]authz.Grant, len(snapshot.Grants))
	s.mail = make(map[string]domain.MailMessage)
	s.mailByDedupe = make(map[string]domain.MailMessage)
	s.mailPreferences = make(map[string]domain.MailPreferences, len(snapshot.MailPreferences))
	s.deliveries = make(map[string]domain.WebhookDelivery)
	s.unsubscribe = make(map[string]domain.UnsubscribeToken, len(snapshot.Unsubscribe))

	// A password field must hold a bcrypt hash or nothing. A value that is
	// present but not a bcrypt hash is a plaintext password in a data file, and
	// that is refused rather than accepted and compared against: accepting it
	// would mean the portal authenticating against a secret it stores in the
	// clear, and the whole point of hashing is that it never has to.
	for _, user := range snapshot.Users {
		if user.PasswordHash != "" && !strings.HasPrefix(user.PasswordHash, "$2") {
			return ErrSnapshotContainsSecrets
		}
		s.users[user.ID] = user
		s.usersByMail[strings.ToLower(user.Email)] = user.ID
	}
	for _, preferences := range snapshot.MailPreferences {
		s.mailPreferences[preferences.UserID] = preferences
	}
	for _, token := range snapshot.Unsubscribe {
		s.unsubscribe[token.Token] = token
	}
	for _, profile := range snapshot.Judges {
		s.judgeProfiles[profile.UserID] = profile
	}
	for _, conflict := range snapshot.Conflicts {
		s.conflicts[conflict.ID] = conflict
	}
	for _, rubric := range snapshot.Rubrics {
		s.rubrics[rubric.ID] = rubric.Clone()
	}
	for _, profile := range snapshot.Profiles {
		s.profiles[profile.UserID] = profile.Clone()
	}
	for _, question := range snapshot.Questions {
		s.questions[question.ID] = question
	}
	for _, milestone := range snapshot.Milestones {
		s.milestones[milestone.ID] = milestone
	}
	for _, host := range snapshot.Hosts {
		s.hosts[host.ID] = host
	}
	for _, entry := range snapshot.Roster {
		s.roster[rosterKey(entry.EventID, entry.JudgeID)] = entry
	}
	for _, invite := range snapshot.Invites {
		s.invites[invite.ID] = invite.Clone()
	}
	for _, participation := range snapshot.Participations {
		s.participations[participationKey(participation.UserID, participation.EventID)] = participation
	}
	for _, entry := range snapshot.Activity {
		s.activity[entry.ID] = entry
	}
	for _, member := range snapshot.Staff {
		s.staff[staffKey(member.EventID, member.UserID)] = member
	}
	for _, comment := range snapshot.Comments {
		s.comments[comment.ID] = comment
	}
	for _, report := range snapshot.Reports {
		s.reports[report.ID] = report
	}
	for _, campaign := range snapshot.Campaigns {
		s.campaigns[campaign.ID] = campaign
	}
	for _, ballot := range snapshot.Ballots {
		s.ballots[ballot.ID] = ballot
	}
	for _, webhook := range snapshot.Webhooks {
		s.webhooks[webhook.ID] = webhook
	}
	for _, event := range snapshot.Events {
		s.events[event.ID] = event
	}
	for _, track := range snapshot.Tracks {
		s.tracks[track.ID] = track
	}
	for _, prize := range snapshot.Prizes {
		s.prizes[prize.ID] = prize
	}
	for _, team := range snapshot.Teams {
		s.teams[team.ID] = team
	}
	for _, membership := range snapshot.Members {
		s.memberships[membershipKey(membership.TeamID, membership.UserID)] = membership
	}
	for _, submission := range snapshot.Submissions {
		s.submissions[submission.ID] = cloneSubmission(submission)
	}
	for _, version := range snapshot.Versions {
		s.submissionVersions[versionKey(version.SubmissionID, version.Version)] = cloneVersion(version)
	}
	for _, duplicate := range snapshot.Duplicates {
		s.duplicates[duplicate.ID] = duplicate
	}
	for _, assignment := range snapshot.Assignments {
		s.assignments[assignment.ID] = assignment
	}
	for _, review := range snapshot.Reviews {
		s.reviews[reviewKey(review.JudgeID, review.ProjectID)] = cloneReview(review)
	}
	for _, comparison := range snapshot.Comparisons {
		s.comparisons[comparison.ID] = comparison
	}
	for _, notification := range snapshot.Notifications {
		s.notifications[notification.ID] = notification
	}
	for _, event := range snapshot.AuditEvents {
		s.auditEvents[event.ID] = event
	}
	for _, grant := range snapshot.Grants {
		s.grants[grant.ID] = grant
	}
	s.restored = true
	return nil
}

// Loaded reports whether the store is running on restored data rather than on a
// fresh seed, so the boot banner and the readiness probe can say which.
func (s *Store) Loaded() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.restored
}

func sortBy[T any](items []T, key func(T) string) []T {
	sort.Slice(items, func(i, j int) bool { return key(items[i]) < key(items[j]) })
	return items
}

// SeedFrom rebuilds store contents from a seed set without going through a
// snapshot. It is used on a cold boot, and it is deliberately separate from
// Restore so that a restore can never be mistaken for a seed.
func (s *Store) SeedFrom(data seed.Data) {
	// Seeding replaces data, never the audit key. A seed set with no key must not
	// be able to blank a key that the composition root configured, or the chain
	// would silently start signing with the empty string.
	if strings.TrimSpace(data.AuditSecret) == "" {
		s.mu.Lock()
		data.AuditSecret = string(s.actionAudit.key)
		s.mu.Unlock()
	}
	seeded := New(data)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users = seeded.users
	s.usersByMail = seeded.usersByMail
	s.judgeProfiles = seeded.judgeProfiles
	s.conflicts = seeded.conflicts
	s.rubrics = seeded.rubrics
	s.profiles = seeded.profiles
	s.questions = seeded.questions
	s.milestones = seeded.milestones
	s.hosts = seeded.hosts
	s.roster = seeded.roster
	s.invites = seeded.invites
	s.participations = seeded.participations
	s.activity = seeded.activity
	s.staff = seeded.staff
	s.comments = seeded.comments
	s.reports = seeded.reports
	s.campaigns = seeded.campaigns
	s.ballots = seeded.ballots
	s.webhooks = seeded.webhooks
	s.events = seeded.events
	s.tracks = seeded.tracks
	s.prizes = seeded.prizes
	s.teams = seeded.teams
	s.memberships = seeded.memberships
	s.submissions = seeded.submissions
	s.submissionVersions = seeded.submissionVersions
	s.duplicates = seeded.duplicates
	s.assignments = seeded.assignments
	s.reviews = seeded.reviews
	s.notifications = seeded.notifications
	s.auditEvents = seeded.auditEvents
	s.grants = seeded.grants
	s.restored = false
}
