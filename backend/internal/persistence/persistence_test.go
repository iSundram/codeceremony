package persistence

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/seed"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

// testBcryptHash is a well-formed bcrypt digest used only as a fixture. It is
// not the hash of any password; the store never verifies it in these tests. It
// is shaped correctly because the restore path rejects a password field that is
// not a bcrypt hash, which is how a plaintext password in a data file is caught.
const testBcryptHash = "$2a$10$4M0PJg2VnNWu3T3vB2n1ueOY0Q7Zt3YbM2Xh1Kq1L9pG4Q6r0zC"

func testStore(t *testing.T) *store.Store {
	t.Helper()
	created := time.Date(2026, time.February, 1, 12, 0, 0, 0, time.UTC)
	submitted := time.Date(2026, time.February, 28, 22, 14, 0, 0, time.UTC)
	data := seed.Data{
		Users: []domain.User{
			{ID: "organizer", Email: "organizer@example.org", DisplayName: "Rhea", Role: domain.RoleOrganizer, State: domain.AccountActive, PasswordHash: testBcryptHash, CreatedAt: created},
			{ID: "judge_a", Email: "judge-a@example.org", DisplayName: "JA", Role: domain.RoleJudge, State: domain.AccountActive, PasswordHash: testBcryptHash, CreatedAt: created},
			{ID: "participant", Email: "participant@example.org", DisplayName: "Pia", Role: domain.RoleParticipant, State: domain.AccountActive, PasswordHash: testBcryptHash, CreatedAt: created},
		},
		Events: []domain.Event{{
			ID: "evt_01", Slug: "sample", Name: "Sample", Timezone: "UTC",
			State: domain.HackathonJudging, SubmissionsClose: submitted.Add(72 * time.Hour),
			ResultsPublished: false, CreatedAt: created, UpdatedAt: created,
		}},
		Tracks:          []domain.Track{{ID: "trk_01", Event: "evt_01", Name: "Dev", Slug: "dev", Order: 1}},
		Prizes:          []domain.Prize{{ID: "prz_01", EventID: "evt_01", Name: "First", Rank: 1, CreatedAt: created}},
		Teams:           []domain.Team{{ID: "tm_01", EventID: "evt_01", Name: "North", CaptainID: "participant", Status: domain.TeamStatusActive, CreatedAt: created}},
		TeamMemberships: []domain.TeamMembership{{ID: "tmem_01", EventID: "evt_01", TeamID: "tm_01", UserID: "participant", Role: domain.TeamRoleCaptain, Status: "active", JoinedAt: created, UpdatedAt: created}},
		Submissions: []domain.Submission{{
			ID: "prj_01", EventID: "evt_01", TeamID: "tm_01", TrackID: "trk_01", Title: "Glass Signal",
			Summary: "One line.", Tags: []string{"go"}, CustomAnswers: map[string]string{"qst_01": "yes"},
			Status: domain.SubmissionSubmitted, Eligibility: domain.EligibilityEligible,
			SubmittedAt: &submitted, UpdatedAt: submitted, Version: 1,
		}},
		Assignments: []domain.Assignment{{ID: "asg_01", EventID: "evt_01", JudgeID: "judge_a", ProjectID: "prj_01", CreatedAt: created}},
		Reviews: []domain.Review{{
			ID: "rev_01", EventID: "evt_01", JudgeID: "judge_a", ProjectID: "prj_01",
			Criteria: map[string]int{"functionality": 4, "quality": 5}, Comment: "Runs clean.",
			Submitted: true, UpdatedAt: submitted,
		}},
		Rubrics: []domain.Rubric{{
			ID: "rub_01", EventID: "evt_01", Name: "R", Version: 1, Status: domain.RubricPublished,
			Criteria:  []domain.RubricCriterion{{Key: "functionality", Label: "F", MinScore: 1, MaxScore: 5, Weight: 100, Required: true}},
			CreatedAt: created,
		}},
		JudgeProfiles: []domain.JudgeProfile{{UserID: "judge_a", Bio: "b", Tracks: []string{"trk_01"}, Capacity: 10, Active: true}},
		JudgeRoster:   []domain.JudgeRosterEntry{{EventID: "evt_01", JudgeID: "judge_a", Scope: domain.JudgeScopeHackathon, Active: true, CreatedAt: created}},
		EventStaff:    []domain.EventStaff{{EventID: "evt_01", UserID: "organizer", Role: domain.EventRoleOwner, CreatedAt: created}},
		Activity: []domain.ActivityEntry{{
			ID: "act_01", EventID: "evt_01", Category: domain.ActivityEvent, Action: "event.created",
			Visibility: domain.ActivityPublic, Metadata: map[string]any{"k": "v"}, CreatedAt: created,
		}},
		Duplicates: []domain.DuplicateFlag{{
			ID: "dup_01", EventID: "evt_01", ProjectID: "prj_02", DuplicateOfProjectID: "prj_01",
			Signal: "team_title_repo", Status: domain.DuplicateOpen, CreatedAt: created,
		}},
		AuditEvents: []domain.AuditEvent{{ID: "aud_01", EventID: "evt_01", Action: "test", TargetType: "event", CreatedAt: created}},
	}
	return store.New(data)
}

// A snapshot has to carry every collection. An earlier version of this file
// allocated all the slices but only populated four of them, which produced a
// data file full of empty arrays and a portal that booted with no events at all
// while reporting a successful restore.
func TestSnapshotCarriesEveryCollection(t *testing.T) {
	snapshot := testStore(t).Snapshot()

	checks := map[string]int{
		"Users": len(snapshot.Users), "Events": len(snapshot.Events), "Tracks": len(snapshot.Tracks),
		"Prizes": len(snapshot.Prizes), "Teams": len(snapshot.Teams), "Members": len(snapshot.Members),
		"Submissions": len(snapshot.Submissions), "Assignments": len(snapshot.Assignments),
		"Reviews": len(snapshot.Reviews), "Rubrics": len(snapshot.Rubrics),
		"Judges": len(snapshot.Judges), "Roster": len(snapshot.Roster),
		"Staff": len(snapshot.Staff), "Activity": len(snapshot.Activity),
		"Duplicates": len(snapshot.Duplicates), "AuditEvents": len(snapshot.AuditEvents),
	}
	for name, count := range checks {
		if count == 0 {
			t.Errorf("snapshot.%s is empty, want the seeded row to survive", name)
		}
	}
	if snapshot.Version != store.SnapshotVersion {
		t.Errorf("snapshot version = %d, want %d", snapshot.Version, store.SnapshotVersion)
	}
}

// The bcrypt hash is persisted, and the plaintext is not — which is the whole
// point of hashing it. The data file has to be protected like any other
// credential store now, and it always had to be: it holds every review, every
// grant and every audit entry in the event.
//
// The inverse decision — never persisting the hash — was tried and reverted. It
// destroyed every account's password on restart, so the only credentials that
// still worked afterwards were the fixed public seed tokens, and a portal
// configured for production had no way in at all.
func TestSnapshotCarriesBcryptHashesAndNeverAPlaintextPassword(t *testing.T) {
	snapshot := testStore(t).Snapshot()
	carried := 0
	for _, user := range snapshot.Users {
		if user.PasswordHash == "" {
			continue
		}
		carried++
		if !strings.HasPrefix(user.PasswordHash, "$2") {
			t.Errorf("user %s: password hash is not bcrypt: %q", user.ID, user.PasswordHash)
		}
	}
	if carried == 0 {
		t.Fatal("no password hash was persisted at all, so a restart would destroy every password again")
	}

	// And the loader refuses a value that is not a hash, which is what stops a
	// plaintext password in a data file from being trusted as a credential.
	plaintext := snapshot
	plaintext.Users = append([]domain.User(nil), snapshot.Users...)
	plaintext.Users[0].PasswordHash = "hunter2"
	if err := store.New(seed.Data{}).Restore(plaintext); !errors.Is(err, store.ErrSnapshotContainsSecrets) {
		t.Errorf("Restore() with a plaintext password = %v, want ErrSnapshotContainsSecrets", err)
	}
}

// Mail preferences and unsubscribe tokens are part of the state, not a cache.
// Both maps used to be rebuilt empty by Restore, which reverted every recorded
// opt-out and invalidated every unsubscribe link already sitting in an inbox.
func TestRestoreKeepsMailPreferencesAndUnsubscribeTokens(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "portal.json")

	original := testStore(t)
	if _, err := original.SaveMailPreferences(domain.MailPreferences{
		UserID: "participant", Marketing: false, WeeklyDigest: true, DigestOnly: true,
	}); err != nil {
		t.Fatalf("SaveMailPreferences() error = %v", err)
	}
	if _, err := original.SaveUnsubscribeToken(domain.UnsubscribeToken{
		Token: "tok-persist", UserID: "participant", Scope: "weekly_digest",
		ExpiresAt: time.Now().Add(72 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateUnsubscribeToken() error = %v", err)
	}

	journal := New(original, path, time.Second, nil)
	if err := journal.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	restored := store.New(seed.Data{})
	ok, err := New(restored, path, time.Second, nil).Restore()
	if err != nil || !ok {
		t.Fatalf("Restore() = %v, %v; want true, nil", ok, err)
	}

	preferences := restored.MailPreferences("participant")
	if !preferences.WeeklyDigest || !preferences.DigestOnly || preferences.Marketing {
		t.Errorf("mail preferences were not restored: %+v", preferences)
	}
	if _, err := restored.UnsubscribeTokenByValue("tok-persist"); err != nil {
		t.Errorf("the unsubscribe token did not survive a restart: %v", err)
	}
}

// Restoring a snapshot must rebuild the secondary indexes too. A restore that
// only filled the primary maps would look correct until the first email login
// or the first composite-keyed lookup.
func TestRestoreRebuildsIndexes(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "portal.json")

	original := testStore(t)
	journal := New(original, path, time.Second, nil)
	if err := journal.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	restored := store.New(seed.Data{})
	restoreJournal := New(restored, path, time.Second, nil)
	ok, err := restoreJournal.Restore()
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if !ok {
		t.Fatal("Restore() reported no restore from an existing data file")
	}
	if !restored.Loaded() {
		t.Error("store does not report that it is running on restored data")
	}

	// Primary key lookups.
	if _, err := restored.EventBySlug("sample"); err != nil {
		t.Errorf("restored event not found by slug: %v", err)
	}
	if _, err := restored.SubmissionByID("prj_01"); err != nil {
		t.Errorf("restored submission not found: %v", err)
	}
	// Secondary index rebuilt by Restore.
	if _, err := restored.UserByEmail("organizer@example.org"); err != nil {
		t.Errorf("restored user not found by email, so the email index was not rebuilt: %v", err)
	}
	// Composite keys rebuilt by Restore.
	if !restored.IsAssigned("evt_01", "judge_a", "prj_01") {
		t.Error("restored assignment not found, so a composite key was not rebuilt")
	}
	if reviews := restored.ReviewsForJudge("evt_01", "judge_a"); len(reviews) != 1 {
		t.Errorf("restored review count = %d, want 1", len(reviews))
	}
	// Reference fields survive the round trip rather than aliasing the source.
	submission, _ := restored.SubmissionByID("prj_01")
	if len(submission.Tags) != 1 || submission.Tags[0] != "go" {
		t.Errorf("restored tags = %v, want [go]", submission.Tags)
	}
	if submission.CustomAnswers["qst_01"] != "yes" {
		t.Errorf("restored custom answers = %v, want qst_01=yes", submission.CustomAnswers)
	}
}

// Two snapshots of unchanged state must be byte-identical, otherwise the
// journal's change detection cannot work and every tick rewrites the file.
func TestSnapshotIsDeterministic(t *testing.T) {
	portal := testStore(t)
	first := portal.Snapshot()
	second := portal.Snapshot()
	for i := range first.Users {
		if first.Users[i].ID != second.Users[i].ID {
			t.Fatalf("user order differs between snapshots at %d", i)
		}
	}
	for i := range first.Submissions {
		if first.Submissions[i].ID != second.Submissions[i].ID {
			t.Fatalf("submission order differs between snapshots at %d", i)
		}
	}
	for i := range first.Roster {
		if first.Roster[i].JudgeID != second.Roster[i].JudgeID {
			t.Fatalf("roster order differs between snapshots at %d", i)
		}
	}
}

// An unchanged store must not be rewritten. This is the whole point of polling
// and diffing rather than instrumenting every mutator.
func TestFlushSkipsUnchangedState(t *testing.T) {
	journal := New(testStore(t), filepath.Join(t.TempDir(), "portal.json"), time.Second, nil)
	for i := 0; i < 5; i++ {
		if err := journal.Flush(); err != nil {
			t.Fatalf("Flush() error = %v", err)
		}
	}
	if journal.Writes() != 1 {
		t.Errorf("writes = %d, want exactly 1 for an unchanged store", journal.Writes())
	}
}

func TestFlushPersistsChanges(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "portal.json")
	portal := testStore(t)
	journal := New(portal, path, time.Second, nil)
	if err := journal.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	created := time.Date(2026, time.March, 1, 9, 0, 0, 0, time.UTC)
	if err := portal.CreateSubmission(domain.Submission{
		ID: "prj_99", EventID: "evt_01", TeamID: "tm_01", TrackID: "trk_01", Title: "Late Arrival",
		Summary: "Written after the first flush.", Status: domain.SubmissionSubmitted,
		Eligibility: domain.EligibilityEligible, UpdatedAt: created, Version: 1,
	}); err != nil {
		t.Fatalf("CreateSubmission() error = %v", err)
	}
	if err := journal.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if journal.Writes() != 2 {
		t.Errorf("writes = %d, want 2 after a change", journal.Writes())
	}

	restored := store.New(seed.Data{})
	if ok, err := New(restored, path, time.Second, nil).Restore(); err != nil || !ok {
		t.Fatalf("Restore() = %v, %v, want true, nil", ok, err)
	}
	submission, err := restored.SubmissionByID("prj_99")
	if err != nil {
		t.Fatalf("submission written after the first flush did not survive: %v", err)
	}
	if submission.Title != "Late Arrival" {
		t.Errorf("restored title = %q, want %q", submission.Title, "Late Arrival")
	}
}

func TestLoadRejectsAFutureVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "portal.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"users":[]}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load() error = nil, want a version mismatch to be reported")
	}
}

func TestLoadReportsMissingFileSeparatelyFromCorruption(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "absent.json")); err != ErrNotFound {
		t.Errorf("Load(absent) error = %v, want ErrNotFound so a first boot is distinguishable", err)
	}
	path := filepath.Join(t.TempDir(), "corrupt.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := Load(path); err == nil || err == ErrNotFound {
		t.Errorf("Load(corrupt) error = %v, want a corruption error", err)
	}
}

// The write is a rename, so a reader never sees a half-written file and no
// temporary file is left behind.
func TestWriteAtomicLeavesNoTemporaryFiles(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "portal.json")
	journal := New(testStore(t), path, time.Second, nil)
	if err := journal.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Errorf("temporary file %s was left behind", entry.Name())
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("data file was not created: %v", err)
	}
}
