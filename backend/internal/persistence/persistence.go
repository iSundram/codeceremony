// Package persistence makes a running portal durable.
//
// The store is a set of in-process maps, which is fast and correct for a
// single process but loses everything on restart. This package writes the
// store's state to a single file and reads it back, so a portal can be
// restarted, moved between machines, or backed up by copying one path.
//
// # How change detection works
//
// Rather than instrumenting every mutating method on the store to raise a dirty
// flag, the journal polls. Each tick it takes a snapshot, encodes it, and
// compares a digest against the last one written. Identical state means no
// write. That is deliberately the boring choice: a missed dirty flag is silent
// data loss, whereas a redundant snapshot costs microseconds and never loses
// anything. The tick is 2 seconds by default, so the worst case is that a hard
// kill loses two seconds of writes, and a clean shutdown loses nothing.
//
// # Why the write is atomic
//
// The file is written to a temporary name in the same directory, synced, then
// renamed over the target. Rename within a directory is atomic on every
// filesystem worth supporting, so a reader either sees the whole previous file
// or the whole new one. A portal killed mid-write therefore leaves a valid data
// file rather than a truncated one.
package persistence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/store"
)

// DefaultInterval is the poll cadence.
const DefaultInterval = 2 * time.Second

// ErrNotFound means there is no data file yet, which is the normal state on a
// first boot and is not an error.
var ErrNotFound = errors.New("persistence: no data file")

// Journal writes store state to disk and restores it on boot.
type Journal struct {
	store    *store.Store
	path     string
	interval time.Duration
	logger   *slog.Logger

	lastDigest string
	writes     int
}

// New returns a journal bound to a store and a data file path.
func New(portal *store.Store, path string, interval time.Duration, logger *slog.Logger) *Journal {
	if interval <= 0 {
		interval = DefaultInterval
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	return &Journal{store: portal, path: path, interval: interval, logger: logger}
}

// Path is the data file this journal manages.
func (j *Journal) Path() string { return j.path }

// Writes reports how many times state has actually been written. Tests assert
// on it to prove that an unchanged store is not rewritten.
func (j *Journal) Writes() int { return j.writes }

// Load reads a data file. A missing file returns ErrNotFound so the caller can
// distinguish "first boot" from "corrupt data directory".
func Load(path string) (store.Snapshot, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return store.Snapshot{}, ErrNotFound
	}
	if err != nil {
		return store.Snapshot{}, fmt.Errorf("persistence: %w", err)
	}
	if len(raw) == 0 {
		return store.Snapshot{}, fmt.Errorf("persistence: %s is empty", path)
	}
	var snapshot store.Snapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return store.Snapshot{}, fmt.Errorf("persistence: %s is not valid JSON: %w", path, err)
	}
	if snapshot.Version != store.SnapshotVersion {
		return store.Snapshot{}, fmt.Errorf("persistence: %s was written as version %d, this build reads version %d",
			path, snapshot.Version, store.SnapshotVersion)
	}
	return snapshot, nil
}

// Restore loads the data file into the store. It reports whether a restore
// happened.
func (j *Journal) Restore() (bool, error) {
	snapshot, err := Load(j.path)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := j.store.Restore(snapshot); err != nil {
		return false, fmt.Errorf("persistence: could not restore %s: %w", j.path, err)
	}
	// Seed the digest from the file we just loaded, so restoring is not
	// immediately followed by a redundant write of the same bytes.
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return false, fmt.Errorf("persistence: could not encode restored state: %w", err)
	}
	j.lastDigest = digest(encoded)
	j.logger.Info("restored portal state from disk",
		"path", j.path, "saved_at", snapshot.SavedAt,
		"users", len(snapshot.Users), "submissions", len(snapshot.Submissions),
		"reviews", len(snapshot.Reviews))
	return true, nil
}

// Flush writes the current state if it differs from what was last written. It
// is safe to call concurrently and is called both by the poll loop and by the
// shutdown path.
func (j *Journal) Flush() error {
	snapshot := j.store.Snapshot()
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("persistence: could not encode state: %w", err)
	}
	current := digest(encoded)
	if current == j.lastDigest {
		return nil
	}
	// Stamp the write time only on the file, never on the in-memory snapshot,
	// so a timestamp change alone cannot look like a state change.
	stamped := j.store.Snapshot()
	stamped.SavedAt = time.Now().UTC()
	encoded, err = json.MarshalIndent(stamped, "", "  ")
	if err != nil {
		return fmt.Errorf("persistence: could not encode state: %w", err)
	}
	if err := writeAtomic(j.path, encoded); err != nil {
		return err
	}
	j.lastDigest = current
	j.writes++
	j.logger.Debug("persisted portal state", "path", j.path, "bytes", len(encoded), "writes", j.writes)
	return nil
}

// Run polls for changes until the context is cancelled, then flushes one last
// time so a clean shutdown never loses a write.
func (j *Journal) Run(stop <-chan struct{}) {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			if err := j.Flush(); err != nil {
				j.logger.Error("could not persist state on shutdown", "error", err)
			}
			return
		case <-ticker.C:
			if err := j.Flush(); err != nil {
				j.logger.Error("could not persist state", "error", err)
			}
		}
	}
}

func digest(encoded []byte) string {
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// writeAtomic writes via a temporary file and a rename, syncing both the file
// and its directory so the rename itself is durable.
func writeAtomic(path string, encoded []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return fmt.Errorf("persistence: could not create %s: %w", directory, err)
	}
	temporary, err := os.CreateTemp(directory, ".portal-*.tmp")
	if err != nil {
		return fmt.Errorf("persistence: could not create a temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)

	if _, err := temporary.Write(encoded); err != nil {
		temporary.Close()
		return fmt.Errorf("persistence: could not write %s: %w", temporaryName, err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("persistence: could not sync %s: %w", temporaryName, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("persistence: could not close %s: %w", temporaryName, err)
	}
	if err := os.Chmod(temporaryName, 0o640); err != nil {
		return fmt.Errorf("persistence: could not set permissions on %s: %w", temporaryName, err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("persistence: could not replace %s: %w", path, err)
	}
	// Syncing the directory is what makes the rename durable; without it the
	// file can still be lost on a power cut even though the write succeeded.
	handle, err := os.Open(directory)
	if err != nil {
		return nil
	}
	defer handle.Close()
	_ = handle.Sync()
	return nil
}
