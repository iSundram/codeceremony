package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/seed"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

// openSubmissionServer is the shared server for the locked-callback tests.
//
// It cannot use newTestServerWithSessions: that fixture's only event has
// submissions closed, so every rewrite is refused as a business rule and the
// test cannot tell a refusal from a deadlocked store. The demo event is open and
// carries a submission with a track on it, which is the shape the deadlock
// needed.
func openSubmissionServer(t *testing.T) (*Server, auth.TokenIssuer, *store.Store) {
	t.Helper()
	hash, err := auth.HashPassword(testPassword)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	data := store.New(seed.WithDemoEvent(seed.Default(hash), time.Now()))
	tokens := auth.NewSessionManager(time.Hour, data)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := New(testServerConfig(), data, tokens, logger)
	server.SetClock(func() time.Time { return time.Date(2026, time.February, 20, 12, 0, 0, 0, time.UTC) })
	return server, tokens, data
}

// The store's write lock is not reentrant, and Go's RWMutex will not let a
// goroutine holding the write lock take the read lock on the same mutex.
//
// One handler did exactly that: it looked a track up from inside the callback
// that ReviseSubmission runs under the write lock. The effect was not a slow
// request. The read lock blocked forever, the write lock was never released, and
// every other request in the process — including reads of unrelated data —
// queued behind it. The portal was dead until it was restarted, and nothing in
// the logs said why.
//
// The assertions are on completion, not on speed. Reintroducing a store call
// inside the callback makes the first request block, and the test fails on its
// deadline. A hang is the honest failure here: there is no wrong number to
// return, there is a portal that never answers again.
func TestARewriteCarryingATrackDoesNotWedgeTheStore(t *testing.T) {
	server, tokens, data := openSubmissionServer(t)
	captain := tokenFor(t, tokens, data, "participant")

	project, err := data.SubmissionByID("prj_demo")
	if err != nil {
		t.Fatalf("SubmissionByID() error = %v", err)
	}
	if project.TrackID == "" {
		t.Skip("the demo submission has no track, so the lookup path is not exercised")
	}

	rewritten := make(chan int, 1)
	go func() {
		rewritten <- request(t, server, http.MethodPatch, "/v1/submissions/"+project.ID, captain,
			map[string]any{"title": "A rewrite that carries a track", "track_id": project.TrackID}).Code
	}()

	select {
	case code := <-rewritten:
		if code != http.StatusOK {
			t.Errorf("the rewrite returned %d, want 200", code)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the rewrite never returned: the store's write lock is held across a store call")
	}

	// The store has to be serving other work too. If the write lock is still
	// held, this blocks and the test fails on the same deadline.
	read := make(chan int, 1)
	go func() {
		read <- request(t, server, http.MethodGet, "/v1/submissions/"+project.ID, captain, nil).Code
	}()
	select {
	case code := <-read:
		if code != http.StatusOK {
			t.Errorf("the unrelated read returned %d, want 200", code)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("an unrelated read blocked after the rewrite: the write lock was never released")
	}
}

// A track from another event has to be refused. Hoisting the lookup out of the
// locked callback must not have weakened the check the callback used to do.
func TestARewriteCannotMoveASubmissionOntoAnotherEventsTrack(t *testing.T) {
	server, tokens, data := openSubmissionServer(t)
	captain := tokenFor(t, tokens, data, "participant")

	project, err := data.SubmissionByID("prj_demo")
	if err != nil {
		t.Fatalf("SubmissionByID() error = %v", err)
	}
	foreign := ""
	for _, event := range data.ListEvents() {
		if event.ID == project.EventID {
			continue
		}
		for _, track := range data.ListTracks(event.ID) {
			foreign = track.ID
			break
		}
		if foreign != "" {
			break
		}
	}
	if foreign == "" {
		t.Skip("no track in the fixtures belongs to another event")
	}

	got := request(t, server, http.MethodPatch, "/v1/submissions/"+project.ID, captain,
		map[string]any{"title": "Borrowed track", "track_id": foreign})
	if got.Code == http.StatusOK {
		t.Errorf("a track from another event was accepted: %d", got.Code)
	}
	after, err := data.SubmissionByID(project.ID)
	if err != nil {
		t.Fatalf("SubmissionByID() error = %v", err)
	}
	if after.TrackID != project.TrackID {
		t.Errorf("the submission's track changed to %s despite the refusal", after.TrackID)
	}
}
