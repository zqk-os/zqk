package lockhealth

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/utils/syscallutil"
)

// referenceNow is the pinned reference clock for deterministic age checks.
var referenceNow = time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

func writeLockFile(t *testing.T, dir, name string, modTime time.Time) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(""), paths.FilePerm600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	if err := os.Chtimes(p, modTime, modTime); err != nil {
		t.Fatalf("chtimes %s: %v", name, err)
	}
	return p
}

func mustNotExist(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected %s to be removed, stat err=%v", p, err)
	}
}

func mustExist(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected %s to persist: %v", p, err)
	}
}

func TestSweep_RemovesStalePreservesActive(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	stalePath := writeLockFile(t, dir, "old.lock", referenceNow.Add(-1*time.Hour))
	activePath := writeLockFile(t, dir, "active.lock", referenceNow.Add(-10*time.Second))
	// Non-lock collateral must never be removed, even if stale.
	hintsPath := writeLockFile(t, dir, "state_hints", referenceNow.Add(-10*time.Hour))
	// A stale .lock file that is exactly at the threshold boundary (== threshold
	// is NOT stale; strictly older is). Boundary check: age == threshold is kept.
	boundaryPath := writeLockFile(t, dir, "boundary.lock", referenceNow.Add(-30*time.Second))

	rep, err := SweepAt(dir, 30*time.Second, referenceNow)
	if err != nil {
		t.Fatalf("SweepAt: %v", err)
	}
	if rep.Err != nil {
		t.Fatalf("report error: %v", rep.Err)
	}
	if rep.Stats.ScannedFiles != 4 {
		t.Fatalf("expected 4 scanned lock files, got %d", rep.Stats.ScannedFiles)
	}
	if rep.Stats.RemovedStale != 1 {
		t.Fatalf("expected 1 removed, got %d", rep.Stats.RemovedStale)
	}
	if rep.Stats.PreservedActive != 3 {
		t.Fatalf("expected 3 preserved, got %d", rep.Stats.PreservedActive)
	}

	mustNotExist(t, stalePath)
	mustExist(t, activePath)
	mustExist(t, boundaryPath)
	mustExist(t, hintsPath)
}

func TestSweep_MissingDirFailsClosed(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "does-not-exist")
	_, err := Sweep(missing, time.Minute)
	if err == nil {
		t.Fatalf("expected hard error for missing lock dir (fail-closed), got nil")
	}
}

func TestSweep_NonPositiveThresholdFailsClosed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Create a file that a careless default would have swept.
	writeLockFile(t, dir, "trap.lock", referenceNow.Add(-time.Hour))

	_, err := SweepAt(dir, 0, referenceNow)
	if !errors.Is(err, ErrNonPositiveThreshold) {
		t.Fatalf("expected ErrNonPositiveThreshold, got %v", err)
	}
	// Fail-closed: no I/O was performed, the file must be untouched.
	mustExist(t, filepath.Join(dir, "trap.lock"))

	_, err = SweepAt(dir, -time.Second, referenceNow)
	if !errors.Is(err, ErrNonPositiveThreshold) {
		t.Fatalf("expected ErrNonPositiveThreshold for negative threshold, got %v", err)
	}
}

func TestSweep_EmptyRootFailsClosed(t *testing.T) {
	t.Parallel()

	_, err := Sweep("/tmp/lockhealth-empty-root-guard", 30*time.Second)
	// Root is not empty syntactically; this test actually guards the clean
	// path: an empty string root must fail before any I/O.
	_ = err

	_, err = SweepAt("", 30*time.Second, referenceNow)
	if !errors.Is(err, ErrEmptyRoot) {
		t.Fatalf("expected ErrEmptyRoot for empty root, got %v", err)
	}
}

func TestSweep_RootIsFileFailsClosed(t *testing.T) {
	t.Parallel()

	f := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(f, []byte(""), paths.FilePerm600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := Sweep(f, 30*time.Second)
	if err == nil {
		t.Fatalf("expected hard error when root is a file (fail-closed), got nil")
	}
}

func TestSweep_SubdirectoriesAreUntouched(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sub := filepath.Join(dir, "nested")
	if err := os.MkdirAll(sub, paths.DirPerm750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Even a stale .lock inside a subdirectory must NOT be removed: the sweep
	// is intentionally single-directory (callers opt into deeper trees
	// explicitly, e.g. package maintenance, not via an unbounded walk).
	writeLockFile(t, sub, "deep.lock", referenceNow.Add(-time.Hour))

	rep, err := SweepAt(dir, 30*time.Second, referenceNow)
	if err != nil {
		t.Fatalf("SweepAt: %v", err)
	}
	if rep.Stats.ScannedFiles != 0 {
		t.Fatalf("expected 0 scanned files (nested dir out of scope), got %d", rep.Stats.ScannedFiles)
	}
	mustExist(t, filepath.Join(sub, "deep.lock"))
}

func TestOwnerVerifiedDead_UnknownOwnerFailClosed(t *testing.T) {
	t.Parallel()

	// pid 0 and negative pids are UNKNOWN owners: fail-closed means we must
	// NOT be able to verify them dead.
	if OwnerVerifiedDead(0) {
		t.Fatalf("unknown owner (pid=0) must be treated as alive (fail-closed)")
	}
	if OwnerVerifiedDead(-1) {
		t.Fatalf("unknown owner (pid=-1) must be treated as alive (fail-closed)")
	}
}

func TestOwnerVerifiedDead_CurrentProcessIsAlive(t *testing.T) {
	t.Parallel()

	// The current test process is by definition alive.
	if OwnerVerifiedDead(os.Getpid()) {
		t.Fatalf("current process (%d) must not be considered verified-dead", os.Getpid())
	}
}

func TestSweep_RepeatedSweepIsIdempotent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeLockFile(t, dir, "a.lock", referenceNow.Add(-2*time.Hour))
	writeLockFile(t, dir, "b.lock", referenceNow.Add(-time.Minute))

	r1, err := SweepAt(dir, 30*time.Second, referenceNow)
	if err != nil {
		t.Fatalf("first sweep: %v", err)
	}
	if r1.Stats.RemovedStale != 2 {
		t.Fatalf("expected 2 removed on first sweep, got %d", r1.Stats.RemovedStale)
	}

	r2, err := SweepAt(dir, 30*time.Second, referenceNow)
	if err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if r2.Stats.RemovedStale != 0 || r2.Stats.ScannedFiles != 0 {
		t.Fatalf("expected idempotent second sweep (0/0), got removed=%d scanned=%d",
			r2.Stats.RemovedStale, r2.Stats.ScannedFiles)
	}
}

func TestSweep_PreservesHeldFlockEvenIfStale(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("flock probing not supported on windows")
	}

	dir := t.TempDir()
	staleLock := writeLockFile(t, dir, "held_job.lock", referenceNow.Add(-2*time.Hour))

	// Acquire advisory flock on the stale lock file
	f, err := fileutil.OpenFile(staleLock, fileutil.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open lock file: %v", err)
	}
	defer f.Close()

	if err := syscallutil.FileFlock(f, syscall.LOCK_EX); err != nil {
		t.Fatalf("acquire flock: %v", err)
	}

	// First sweep: modTime is stale, but lock is actively held. MUST NOT be removed!
	rep1, err := SweepAt(dir, 30*time.Second, referenceNow)
	if err != nil {
		t.Fatalf("SweepAt: %v", err)
	}
	if rep1.Stats.RemovedStale != 0 {
		t.Fatalf("expected 0 removed while flock is held, got %d", rep1.Stats.RemovedStale)
	}
	if rep1.Stats.PreservedActive != 1 {
		t.Fatalf("expected 1 preserved active, got %d", rep1.Stats.PreservedActive)
	}
	mustExist(t, staleLock)

	// Release flock
	if err := syscallutil.FileFlock(f, syscall.LOCK_UN); err != nil {
		t.Fatalf("release flock: %v", err)
	}
	_ = f.Close()

	// Second sweep: flock is released. Now stale lock should be swept!
	rep2, err := SweepAt(dir, 30*time.Second, referenceNow)
	if err != nil {
		t.Fatalf("SweepAt: %v", err)
	}
	if rep2.Stats.RemovedStale != 1 {
		t.Fatalf("expected 1 removed after flock released, got %d", rep2.Stats.RemovedStale)
	}
	mustNotExist(t, staleLock)
}
