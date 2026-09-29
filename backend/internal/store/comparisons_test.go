package store

import (
	"strings"
	"testing"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/seed"
)

// comparisonStore returns a store with two events, so the event fence can be
// tested against a project that genuinely belongs somewhere else.
func comparisonStore(t *testing.T) *Store {
	t.Helper()
	data := seed.Default("hash")
	second := domain.Event{
		ID: "evt_two", Slug: "second-event", Name: "Second",
		Description: "another event", Timezone: "UTC",
		State: domain.HackathonSubmissionsOpen, CreatedAt: time.Now().UTC(),
	}
	data.Events = append(data.Events, second)
	data.Submissions = append(data.Submissions, domain.Submission{
		ID: "prj_two", EventID: "evt_two", TeamID: "tm_02", TrackID: "trk_01",
		Title: "Elsewhere", Description: "a project in another event",
		Version: 1, UpdatedAt: time.Now().UTC(),
	})
	return New(data)
}

func TestAComparisonCannotReachIntoAnotherEvent(t *testing.T) {
	portal := comparisonStore(t)
	_, err := portal.RecordComparison(domain.Comparison{
		EventID: "evt_01", JudgeID: "judge_a",
		Left: "prj_01", Right: "prj_two", Verdict: domain.ComparisonLeftWins,
	})
	if err == nil {
		t.Fatal("a comparison naming a project from another event was accepted")
	}
	if !strings.Contains(err.Error(), "another event") {
		t.Errorf("error = %v, want a message about the event fence", err)
	}
	if got := portal.ComparisonsForEvent("evt_01"); len(got) != 0 {
		t.Errorf("the refused comparison was stored anyway: %v", got)
	}
}

func TestComparisonValidation(t *testing.T) {
	portal := comparisonStore(t)
	base := domain.Comparison{
		EventID: "evt_01", JudgeID: "judge_a",
		Left: "prj_01", Right: "prj_02", Verdict: domain.ComparisonLeftWins,
	}
	cases := []struct {
		name   string
		mutate func(c *domain.Comparison)
		want   string
	}{
		{"no judge", func(c *domain.Comparison) { c.JudgeID = "" }, "needs a judge"},
		{"no left", func(c *domain.Comparison) { c.Left = "" }, "two projects"},
		{"same project", func(c *domain.Comparison) { c.Right = c.Left }, "cannot be compared with itself"},
		{"bad verdict", func(c *domain.Comparison) { c.Verdict = "better" }, "left, right or tie"},
		{"unknown event", func(c *domain.Comparison) { c.EventID = "evt_nope" }, ""},
		{"unknown left", func(c *domain.Comparison) { c.Left = "prj_nope" }, ""},
		{"unknown right", func(c *domain.Comparison) { c.Right = "prj_nope" }, ""},
	}
	for _, c := range cases {
		comparison := base
		c.mutate(&comparison)
		_, err := portal.RecordComparison(comparison)
		if err == nil {
			t.Errorf("%s was accepted", c.name)
			continue
		}
		if c.want != "" && !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error = %v, want it to mention %q", c.name, err, c.want)
		}
	}
}

// Reversing the pair updates the existing verdict rather than adding a second
// one, so a judge who changes their mind is counted once.
func TestReversingAPairUpdatesTheSameVerdict(t *testing.T) {
	portal := comparisonStore(t)
	first, err := portal.RecordComparison(domain.Comparison{
		EventID: "evt_01", JudgeID: "judge_a",
		Left: "prj_01", Right: "prj_02", Verdict: domain.ComparisonLeftWins,
	})
	if err != nil {
		t.Fatalf("RecordComparison() error = %v", err)
	}
	second, err := portal.RecordComparison(domain.Comparison{
		EventID: "evt_01", JudgeID: "judge_a",
		Left: "prj_02", Right: "prj_01", Verdict: domain.ComparisonRightWins,
	})
	if err != nil {
		t.Fatalf("RecordComparison() error = %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("reversing the pair created a second verdict: %v then %v", first.ID, second.ID)
	}
	if got := portal.ComparisonsForEvent("evt_01"); len(got) != 1 {
		t.Fatalf("the event has %d verdicts, want 1", len(got))
	}
	if second.Verdict != domain.ComparisonRightWins {
		t.Errorf("verdict = %v, want the later answer", second.Verdict)
	}
	if second.CreatedAt != first.CreatedAt {
		t.Error("the update reset the creation time, so the change of mind is not traceable")
	}
}

// Two judges comparing the same pair are two different answers.
func TestComparisonsAreScopedPerJudge(t *testing.T) {
	portal := comparisonStore(t)
	for _, judgeID := range []string{"judge_a", "judge_b"} {
		if _, err := portal.RecordComparison(domain.Comparison{
			EventID: "evt_01", JudgeID: judgeID,
			Left: "prj_01", Right: "prj_02", Verdict: domain.ComparisonLeftWins,
		}); err != nil {
			t.Fatalf("RecordComparison(%s) error = %v", judgeID, err)
		}
	}
	if got := portal.ComparisonsForEvent("evt_01"); len(got) != 2 {
		t.Errorf("the event has %d verdicts, want one per judge (2)", len(got))
	}
	if got := portal.ComparisonsForJudge("evt_01", "judge_a"); len(got) != 1 {
		t.Errorf("judge_a sees %d verdicts, want 1", len(got))
	}
}

func TestComparisonListingOrderIsStable(t *testing.T) {
	portal := comparisonStore(t)
	for _, judgeID := range []string{"judge_b", "judge_a"} {
		for _, pair := range [][2]string{{"prj_02", "prj_01"}, {"prj_01", "prj_03"}} {
			if _, err := portal.RecordComparison(domain.Comparison{
				EventID: "evt_01", JudgeID: judgeID,
				Left: pair[0], Right: pair[1], Verdict: domain.ComparisonLeftWins,
			}); err != nil {
				t.Fatalf("RecordComparison() error = %v", err)
			}
		}
	}
	first := portal.ComparisonsForEvent("evt_01")
	second := portal.ComparisonsForEvent("evt_01")
	if len(first) != 4 {
		t.Fatalf("listed %d verdicts, want 4", len(first))
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatalf("listing order changed between identical calls at %d", i)
		}
	}
	// Sorted by judge, then left, then right.
	if first[0].JudgeID != "judge_a" || first[2].JudgeID != "judge_b" {
		t.Errorf("listing is not grouped by judge: %v then %v", first[0].JudgeID, first[2].JudgeID)
	}
}

func TestADeletedComparisonIsGoneButRetrievableByNobody(t *testing.T) {
	portal := comparisonStore(t)
	created, err := portal.RecordComparison(domain.Comparison{
		EventID: "evt_01", JudgeID: "judge_a",
		Left: "prj_01", Right: "prj_02", Verdict: domain.ComparisonLeftWins,
	})
	if err != nil {
		t.Fatalf("RecordComparison() error = %v", err)
	}
	if err := portal.DeleteComparison(created.ID); err != nil {
		t.Fatalf("DeleteComparison() error = %v", err)
	}
	if _, err := portal.ComparisonByID(created.ID); err == nil {
		t.Error("a deleted comparison is still readable")
	}
	if err := portal.DeleteComparison(created.ID); err == nil {
		t.Error("deleting a missing comparison reported success")
	}
}

// Recorded verdicts have to survive a restart, or a judge's answers would vanish
// while the derived comparisons they were meant to replace persisted.
func TestComparisonsSurviveASnapshotRoundTrip(t *testing.T) {
	portal := comparisonStore(t)
	created, err := portal.RecordComparison(domain.Comparison{
		EventID: "evt_01", JudgeID: "judge_a",
		Left: "prj_01", Right: "prj_02", Verdict: domain.ComparisonTied, Comment: "genuinely even",
	})
	if err != nil {
		t.Fatalf("RecordComparison() error = %v", err)
	}
	snapshot := portal.Snapshot()

	restored := New(seed.Default("hash"))
	if err := restored.Restore(snapshot); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	got, err := restored.ComparisonByID(created.ID)
	if err != nil {
		t.Fatalf("ComparisonByID() after restore error = %v", err)
	}
	if got.Verdict != domain.ComparisonTied || got.Comment != "genuinely even" {
		t.Errorf("the restored verdict is %+v, want the recorded one", got)
	}
	if len(restored.ComparisonsForEvent("evt_01")) != 1 {
		t.Errorf("the restored store has %d verdicts, want 1", len(restored.ComparisonsForEvent("evt_01")))
	}
}
