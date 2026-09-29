package fixtures

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

// distribute has to satisfy the rubric validator for every arity it can be
// asked for: weights must total exactly 100 and none may be zero. An earlier
// hand-rolled version produced 40/70/15 for three criteria, a total of 125, so
// the fixture rubric was being written with weights the portal would reject.
func TestDistributeTotalsOneHundredWithPositiveWeights(t *testing.T) {
	for n := 1; n <= 25; n++ {
		weights := distribute(n)
		if len(weights) != n {
			t.Fatalf("distribute(%d) returned %d weights, want %d", n, len(weights), n)
		}
		total := 0
		for i, weight := range weights {
			if weight <= 0 {
				t.Errorf("distribute(%d)[%d] = %d, want a positive weight", n, i, weight)
			}
			total += weight
		}
		if total != 100 {
			t.Errorf("distribute(%d) totals %d, want exactly 100: %v", n, total, weights)
		}
	}
}

// The three-criterion case is the DOGFOOD default rubric, so it should come out
// as the familiar 40/35/25 ladder in the order the criteria are declared.
func TestDistributeThreeCriteriaMatchesTheDefaultLadder(t *testing.T) {
	weights := distribute(3)
	if len(weights) != 3 || weights[0] != 40 || weights[1] != 35 || weights[2] != 25 {
		t.Fatalf("distribute(3) = %v, want [40 35 25]", weights)
	}
}

// fixtureDocument reads the real shared fixture file from the backend
// directory, skipping the test if it is not present so the package still builds
// in a checkout without it.
func fixtureDocument(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "fixtures.json"))
	if err != nil {
		t.Skipf("fixtures.json is not present: %v", err)
	}
	return raw
}

// Loading the fixture must produce a complete, self-consistent panel. An
// earlier version of the loader assigned the roster and profiles from the base
// seed rather than from the fixture, which silently produced an empty panel:
// the organizer saw no judges at all, and the judge console had nobody to show.
func TestLoadProducesACompletePanel(t *testing.T) {
	data, err := Parse(fixtureDocument(t), "hash")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(data.JudgeRoster) != 30 {
		t.Errorf("roster entries = %d, want 30", len(data.JudgeRoster))
	}
	if len(data.JudgeProfiles) != 30 {
		t.Errorf("judge profiles = %d, want 30", len(data.JudgeProfiles))
	}
	if len(data.Reviews) != 126 {
		t.Errorf("reviews = %d, want 126", len(data.Reviews))
	}
	if len(data.Submissions) != 41 {
		t.Errorf("submissions = %d, want 41", len(data.Submissions))
	}
	if len(data.Duplicates) != 1 {
		t.Errorf("duplicate flags = %d, want 1", len(data.Duplicates))
	}
	if len(data.Tracks) != 8 {
		t.Errorf("tracks = %d, want 8", len(data.Tracks))
	}

	// Every review needs an assignment backing it, or the store refuses it.
	assigned := make(map[string]bool, len(data.Assignments))
	for _, assignment := range data.Assignments {
		assigned[assignment.JudgeID+"|"+assignment.ProjectID] = true
	}
	for _, review := range data.Reviews {
		if !assigned[review.JudgeID+"|"+review.ProjectID] {
			t.Fatalf("review %s has no assignment, so the store would refuse it", review.ID)
		}
	}

	// The built-in demo judges must be retired: they share the demo addresses,
	// so leaving them in place would authenticate a judge with an empty batch.
	live := make(map[string]bool, len(data.Users))
	for _, user := range data.Users {
		live[user.ID] = true
	}
	for _, retired := range []string{"judge_a", "judge_b"} {
		if live[retired] {
			t.Errorf("the built-in demo judge %s survived the fixture load", retired)
		}
	}
	// The portal's own demo identities must survive.
	for _, kept := range []string{"organizer", "admin", "participant", "participant_other"} {
		if !live[kept] {
			t.Errorf("the demo identity %s was dropped by the fixture load", kept)
		}
	}
}

// The rubric must belong to the fixture event. Without an event id the store
// never treats it as active, and every result silently falls back to hardcoded
// weights while reporting the published rubric.
func TestLoadGivesTheRubricItsEvent(t *testing.T) {
	data, err := Parse(fixtureDocument(t), "hash")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(data.Rubrics) != 1 {
		t.Fatalf("rubrics = %d, want 1", len(data.Rubrics))
	}
	rubric := data.Rubrics[0]
	if rubric.EventID == "" {
		t.Error("the fixture rubric has no event id, so it would never be active")
	}
	if err := rubric.Validate(); err != nil {
		t.Errorf("the generated rubric is not valid: %v", err)
	}
	if rubric.TotalWeight() != 100 {
		t.Errorf("rubric weights total %d, want 100", rubric.TotalWeight())
	}
}

// The event's lifecycle must come from the fixture deadline, so that a seeded
// portal honestly reflects a closed event.
func TestLoadDrivesTheEventLifecycleFromTheFixtureDeadline(t *testing.T) {
	data, err := Parse(fixtureDocument(t), "hash")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	var event domain.Event
	for _, candidate := range data.Events {
		if candidate.ID == "evt_01" {
			event = candidate
		}
	}
	if event.ID == "" {
		t.Fatal("no evt_01 in the loaded data")
	}
	if event.SubmissionsOpen {
		t.Error("the fixture event closed in the past but was seeded as open")
	}
	if event.ResultsPublished {
		t.Error("the fixture event was seeded with results already published")
	}
	if event.SubmissionsClose.IsZero() {
		t.Error("no submissions_close was read from the fixture")
	}
}
