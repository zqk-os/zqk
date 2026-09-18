// Package lockhealth provides verified stale-lock cleanup primitives with
// fail-closed semantics, usable across subsystems (scheduler, file lock
// strategies, state registries, daemon maintenance) that otherwise each
// implement their own best-effort lock-file removal.
//
// Design invariants:
//
//   - Fail-closed: invalid configuration (non-positive threshold, empty lock
//     dir), a missing lock dir, and an unreadable lock dir are hard errors,
//     never silent no-ops. This prevents a caller from assuming "cleanup
//     succeeded" when the directory was never actually inspected.
//   - Observability: every sweep produces a structured Report suitable for
//     metrics wiring via pkg/observability.
//   - Owner-verified removal: callers that can identify the owning process
//     of a still-held flock must confirm OwnerVerifiedDead before relying on
//     removal, to avoid breaking a live holder.
package lockhealth

import (
	"errors"
	"path"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	// ErrMsgDirMissing is the fail-closed message for a missing lock dir.
	ErrMsgDirMissing = "lockhealth: lock directory does not exist: "

	// ErrMsgRootNotDir is the fail-closed message for a non-directory root.
	ErrMsgRootNotDir = "lockhealth: root is not a directory: "

	// ErrMsgDirNotReadable is the fail-closed message for an unreadable dir.
	ErrMsgDirNotReadable = "lockhealth: lock directory was not readable"

	// ErrMsgNonPositiveThreshold guards the threshold check.
	ErrMsgNonPositiveThreshold = "lockhealth: stale lock sweep threshold must be > 0"

	// ErrMsgEmptyRoot guards the empty-root check.
	ErrMsgEmptyRoot = "lockhealth: lock directory must be non-empty"
)

// ErrNonPositiveThreshold is returned for non-positive sweep thresholds.
var ErrNonPositiveThreshold = errors.New(ErrMsgNonPositiveThreshold)

// ErrEmptyRoot is returned when the lock directory is empty.
var ErrEmptyRoot = errors.New(ErrMsgEmptyRoot)

// ErrDirNotReadable is returned when the lock directory exists but cannot
// be listed (fail-closed: we must not report success without enumerating it).
var ErrDirNotReadable = errors.New(ErrMsgDirNotReadable)

// SweepStats summarizes a single stale-lock sweep for observability metrics.
type SweepStats struct {
	ScannedFiles    int           // file entries inspected under Root
	RemovedStale    int           // stale lock files actually deleted
	PreservedActive int           // lock files kept because they are NOT stale or non-lock
	SweepDuration   time.Duration // wall time for the sweep
	Now             time.Time     // reference time the age comparison used
}

// Report is the observable result of one Sweep call. A non-nil Err means the
// sweep did NOT complete; callers must not treat the lock directory as clean.
type Report struct {
	Root  string
	Stats SweepStats
	Err   error
}

// Sweep removes stale *.lock files (modification time older than threshold)
// from a single lock directory with fail-closed semantics:
//
//   - threshold <= 0 or root empty -> immediate error, no I/O attempted.
//   - A missing root directory is a hard error: a caller that expects a
//     specific locks directory to exist must detect its absence rather than
//     silently succeeding as if nothing needed cleaning.
//   - A root that cannot be listed is a hard error (ErrDirNotReadable).
//   - Only files with a ".lock" extension are ever removed; non-lock
//     collateral (state hints, metrics sidecar files) is never touched.
//   - Individual files that cannot be stat'd are skipped (best effort) and
//     do not abort the sweep, matching the "log but don't fail" convention
//     already used in pkg/storage/file lock strategies.
func Sweep(root string, threshold time.Duration) (Report, error) {
	return SweepAt(root, threshold, time.Now())
}

// SweepAt is the deterministic variant of Sweep for tests: the caller pins
// the reference time so age comparisons are reproducible.
func SweepAt(root string, threshold time.Duration, referenceNow time.Time) (Report, error) {
	if root == "" || root == string(filepath.Separator) {
		return Report{Root: root, Err: ErrEmptyRoot}, ErrEmptyRoot
	}
	root = filepath.Clean(root)
	if threshold <= 0 {
		return Report{Root: root, Err: ErrNonPositiveThreshold}, ErrNonPositiveThreshold
	}

	info, err := fileutil.Stat(root)
	if fileutil.IsNotExist(err) {
		dirMissing := errors.New(ErrMsgDirMissing + root)
		return Report{Root: root, Err: dirMissing}, dirMissing
	}
	if err != nil {
		return Report{Root: root, Err: ErrDirNotReadable}, ErrDirNotReadable
	}
	if !info.IsDir() {
		notDir := errors.New(ErrMsgRootNotDir + root)
		return Report{Root: root, Err: notDir}, notDir
	}

	start := time.Now()
	stats := SweepStats{Now: referenceNow}

	entries, err := fileutil.ReadDir(root)
	if err != nil {
		return Report{Root: root, Stats: stats, Err: ErrDirNotReadable}, ErrDirNotReadable
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue // subdirectories are out of scope for a single-dir sweep
		}
		stats.ScannedFiles++
		name := entry.Name()
		if !isLockFile(name) {
			stats.PreservedActive++
			continue // never sweep non-lock collateral
		}
		fullPath := filepath.Join(root, name)

		fileInfo, statErr := fileutil.Stat(fullPath)
		if statErr != nil {
			// Entry disappeared between ReadDir and Stat, or is unreadable:
			// best effort, keep sweeping.
			continue
		}

		if isStale(fileInfo.ModTime(), referenceNow, threshold) {
			removeErr := fileutil.Remove(fullPath)
			if removeErr == nil || fileutil.IsNotExist(removeErr) {
				stats.RemovedStale++
			}
		} else {
			stats.PreservedActive++
		}
	}

	stats.SweepDuration = time.Since(start)
	return Report{Root: root, Stats: stats}, nil
}

// isStale reports whether a lock file's modification time is older than
// threshold relative to now.
func isStale(modTime, now time.Time, threshold time.Duration) bool {
	return now.Sub(modTime) > threshold
}

// isLockFile reports whether a filename is a lock file we are permitted to
// sweep. Uses the ".lock" suffix convention shared by pkg/scheduler
// (job_lock.go) and pkg/storage/file (file_lock_strategy.go).
func isLockFile(name string) bool {
	return path.Ext(path.Base(name)) == ".lock"
}

// OwnerVerifiedDead reports whether the process owning a held lock can be
// verified dead before its lock file is broken. Fail-closed: pid <= 0
// (unknown owner) is treated as NOT verified dead, so callers must keep
// the lock intact.
func OwnerVerifiedDead(pid int) bool {
	if runtime.GOOS == "windows" {
		// No POSIX kill(0) probe on Windows; unknown owner stays alive.
		return false
	}
	if pid <= 0 {
		return false // unknown owner: fail-closed, assume alive
	}
	return !processAlive(pid)
}

func processAlive(pid int) bool {
	// kill(pid, 0) is the cheapest liveness probe that does not deliver a
	// signal: ESRCH means the process does not exist; any other result
	// (including EPERM: exists but not ours) counts as alive.
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return err != syscall.ESRCH
}
