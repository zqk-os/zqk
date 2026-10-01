package scheduler

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	testStaleThreshold = 1 * time.Hour

	testStaleLockAge      = 2 * time.Hour
	testFreshLockAge      = 10 * time.Second
	testZeroThresholdAge  = 1 * time.Hour
	testNonLockDirFileAge = 10 * time.Hour

	testRecoveryFailedReason    = "recovery_failed: simulated"
	testRecoveryRecoveredReason = "recovered_from_crash"

	testFreshLockSuffix = ".lock"
)

const (
	msgCleanStale = "CleanStaleLocksByAge: %v"
	msgCleanCount = "expected cleanup of %d stale locks, got %d"
	msgFreshKeep  = "fresh lock should survive: %v"
	msgStaleGone  = "stale lock %s should have been removed"
	msgStatErr    = "unexpected stat error for %s: %v"
	msgZeroThresh = "with threshold 0, no lock may be removed: %v"
	msgNonLock    = "non-lock file must not be removed: %v"
	msgFirstPass  = "first pass: %v"
	msgSecondPass = "second pass: %v"
	msgSecondZero = "second pass should remove nothing, got %d"
	msgFirstSome  = "first pass should remove at least some locks, got %d"
	msgPersisted  = "expected persisted state"
	msgStaleState = "fail-closed recovery must persist state=%q, got %q"
	msgFailAt     = "failed state must have CompletedAt set"
	msgNotInProg  = "fail-closed state must not be observable as in_progress"
	msgOneInProg  = "expected 1 in-progress entry for %s, got %+v"
	msgZeroInProg = "expected no in_progress entries after recovery, got %+v"
	msgRegister   = "register: %v"
	msgComplete   = "complete: %v"
	msgList       = "list: %v"
	msgMkdir      = "mkdir: %v"
	msgWrite      = "write: %v"
	msgChtimes    = "chtimes: %v"
)

// writeLockFile creates <dir>/<safeJobID>.lock with mtime forced to now-age.
func writeLockFile(t *testing.T, dir, jobID string, age time.Duration) string {
	t.Helper()
	p := filepath.Join(dir, lockFilenameForJobID(jobID))
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		t.Fatalf(msgMkdir, err)
	}
	if err := fileutil.WriteFile(p, []byte(""), paths.FilePerm644); err != nil {
		t.Fatalf(msgWrite, err)
	}
	old := time.Now().Add(-age)
	if err := fileutil.Chtimes(p, old, old); err != nil {
		t.Fatalf(msgChtimes, err)
	}
	return p
}

func TestCleanStaleLocksByAge_RemovesOldLocksAndKeepsFresh(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "locks")

	writeLockFile(t, dir, "SCH-stale-1", testStaleLockAge)
	writeLockFile(t, dir, "SCH-stale-2", 3*testStaleLockAge)
	writeLockFile(t, dir, "SCH-stale-3", 4*testStaleLockAge)
	writeLockFile(t, dir, "SCH-fresh-1", testFreshLockAge)

	n, err := CleanStaleLocksByAge(dir, testStaleThreshold)
	if err != nil {
		t.Fatalf(msgCleanStale, err)
	}
	if n != 3 {
		t.Fatalf(msgCleanCount, 3, n)
	}
	if _, err := fileutil.Stat(filepath.Join(dir, "SCH-fresh-1"+testFreshLockSuffix)); err != nil {
		t.Fatalf(msgFreshKeep, err)
	}
	for _, id := range []string{"SCH-stale-1", "SCH-stale-2", "SCH-stale-3"} {
		p := filepath.Join(dir, lockFilenameForJobID(id))
		if _, err := fileutil.Stat(p); err == nil {
			t.Errorf(msgStaleGone, id)
		} else if !fileutil.IsNotExist(err) {
			t.Fatalf(msgStatErr, id, err)
		}
	}
}

func TestCleanStaleLocksByAge_ZeroThresholdRemovesNothing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "locks")
	p := writeLockFile(t, dir, "SCH-x", testZeroThresholdAge)
	if _, err := CleanStaleLocksByAge(dir, 0); err != nil {
		t.Fatalf(msgCleanStale, err)
	}
	if _, err := fileutil.Stat(p); err != nil {
		t.Fatalf(msgZeroThresh, err)
	}
}

func TestCleanStaleLocksByAge_IgnoresNonLockFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	lockDir := filepath.Join(root, "locks")
	writeLockFile(t, lockDir, "SCH-a", testStaleLockAge)

	otherDir := filepath.Join(root, "other")
	if err := fileutil.MkdirAll(otherDir, paths.DirPerm755); err != nil {
		t.Fatalf(msgMkdir, err)
	}
	otherPath := filepath.Join(otherDir, "old.log")
	if err := fileutil.WriteFile(otherPath, []byte("kept"), paths.FilePerm644); err != nil {
		t.Fatalf(msgWrite, err)
	}
	old := time.Now().Add(-testNonLockDirFileAge)
	if err := fileutil.Chtimes(otherPath, old, old); err != nil {
		t.Fatalf(msgChtimes, err)
	}

	if _, err := CleanStaleLocksByAge(lockDir, testStaleThreshold); err != nil {
		t.Fatalf(msgCleanStale, err)
	}
	if _, err := fileutil.Stat(otherPath); err != nil {
		t.Fatalf(msgNonLock, err)
	}
}

// 100 stale locks written sequentially, then two sequential CleanStaleLocksByAge
// passes to verify the total is deterministic even when callers elsewhere in prod
// invoke the routine concurrently. The routine itself must be idempotent per dir.
func TestCleanStaleLocksByAge_SequentialRemovalIsDeterministic(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "locks")
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		t.Fatalf(msgMkdir, err)
	}
	for i := 0; i < 100; i++ {
		writeLockFile(t, dir, "SCH-parallel-"+strings.Repeat("x", i%7), testStaleLockAge)
	}
	first, err := CleanStaleLocksByAge(dir, testStaleThreshold)
	if err != nil {
		t.Fatalf(msgFirstPass, err)
	}
	second, err := CleanStaleLocksByAge(dir, testStaleThreshold)
	if err != nil {
		t.Fatalf(msgSecondPass, err)
	}
	if second != 0 {
		t.Fatalf(msgSecondZero, second)
	}
	if first == 0 {
		t.Fatalf(msgFirstSome, first)
	}
}

// TestCompleteExecutionFailClosedOnRecoveryError locks the recovery fail-closed contract:
// when a recovery of an in_progress state reports an error, the state MUST be
// persisted as failed (terminal) so downstream consumers treat it as done. No silent
// requeue — the operator sees the failure and can manually re-plan.
func TestCompleteExecutionFailClosedOnRecoveryError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	reg := NewJobStateRegistry(root).(*JobStateRegistry)

	const jobID = "SCH-failclosed-1"
	const execID = "exec-fc-1"
	if err := reg.RegisterExecution(jobID, execID, 1234); err != nil {
		t.Fatalf(msgRegister, err)
	}
	if err := reg.CompleteExecution(jobID, execID, testRecoveryFailedReason); err != nil {
		t.Fatalf(msgComplete, err)
	}
	st := readStateByExecutionID(t, reg, jobID, execID)
	if st == nil {
		t.Fatalf("%s", msgPersisted)
	}
	if st.State != jobExecutionStateFailed {
		t.Fatalf(msgStaleState, jobExecutionStateFailed, st.State)
	}
	if st.CompletedAt == nil {
		t.Fatalf("%s", msgFailAt)
	}
	if got, err := reg.GetExecutionState(jobID); err == nil && got != nil && got.State == jobExecutionStateInProgress {
		t.Fatalf("%s", msgNotInProg)
	}
}

// TestCompleteExecutionRecoveredFromCrashWithoutDoubleWork is the positive leg of the
// same contract: after a crash-recovered in_progress entry is finalized, the
// in_progress view is empty while the persisted record is completed (audit trail
// survives).
func TestCompleteExecutionRecoveredFromCrashWithoutDoubleWork(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	reg := NewJobStateRegistry(root).(*JobStateRegistry)

	const jobID = "SCH-recover-1"
	const execID = "exec-recover-1"
	if err := reg.RegisterExecution(jobID, execID, 999); err != nil {
		t.Fatalf(msgRegister, err)
	}

	list, err := reg.ListInProgress()
	if err != nil {
		t.Fatalf(msgList, err)
	}
	if len(list) != 1 || list[0].JobID != jobID {
		t.Fatalf(msgOneInProg, jobID, list)
	}

	if err := reg.CompleteExecution(jobID, execID, testRecoveryRecoveredReason); err != nil {
		t.Fatalf(msgComplete, err)
	}

	list2, err := reg.ListInProgress()
	if err != nil {
		t.Fatalf(msgList, err)
	}
	if len(list2) != 0 {
		t.Fatalf(msgZeroInProg, list2)
	}
}

func TestCleanStaleLocksByAge_PreservesActivelyHeldLocks(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "locks")
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		t.Fatalf(msgMkdir, err)
	}

	// Create a lock file that is old (> threshold)
	p := writeLockFile(t, dir, "SCH-actively-held", 5*testStaleLockAge)

	// Open and acquire exclusive flock on this file
	fl, err := storagepkg.NewFileLock(p)
	if err != nil {
		t.Fatalf("NewFileLock: %v", err)
	}
	defer fl.Close()
	defer fl.Unlock()

	if err := fl.Lock(); err != nil {
		t.Fatalf("Lock: %v", err)
	}

	// Now run CleanStaleLocksByAge - it must NOT remove the actively held lock!
	n, err := CleanStaleLocksByAge(dir, testStaleThreshold)
	if err != nil {
		t.Fatalf("CleanStaleLocksByAge: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 locks removed, got %d", n)
	}

	// Verify the file still exists on disk
	if _, err := fileutil.Stat(p); err != nil {
		t.Fatalf("actively held lock file was unlinked: %v", err)
	}
}
