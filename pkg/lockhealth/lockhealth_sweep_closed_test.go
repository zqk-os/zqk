package lockhealth

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/utils/syscallutil"
)

func createTestLock(t *testing.T, dir, name string, age time.Duration, ref time.Time) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := fileutil.WriteFile(p, []byte("lock content"), paths.FilePerm600); err != nil {
		t.Fatalf("failed to write test lock file %s: %v", name, err)
	}
	modTime := ref.Add(-age)
	if err := os.Chtimes(p, modTime, modTime); err != nil {
		t.Fatalf("failed to chtimes for %s: %v", name, err)
	}
	return p
}

func holdExclusiveFlock(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := fileutil.OpenFile(path, fileutil.O_RDWR, 0)
	if err != nil {
		t.Fatalf("failed to open lock for holding flock: %v", err)
	}
	if err := syscallutil.FileFlock(f, syscall.LOCK_EX); err != nil {
		closeErr := f.Close()
		if closeErr != nil {
			t.Logf("close error: %v", closeErr)
		}
		t.Fatalf("failed to acquire flock: %v", err)
	}
	return f
}

func releaseFlockAndClose(t *testing.T, f *os.File) {
	t.Helper()
	if err := syscallutil.FileFlock(f, syscall.LOCK_UN); err != nil {
		t.Fatalf("failed to release flock: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("failed to close file descriptor: %v", err)
	}
}

func TestSweep_RealTimeSweepExecution(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	now := time.Now()
	stale := createTestLock(t, dir, "realtime_stale.lock", 2*time.Hour, now)
	active := createTestLock(t, dir, "realtime_active.lock", 2*time.Second, now)

	rep, err := Sweep(dir, time.Minute)
	if err != nil {
		t.Fatalf("Sweep returned unexpected error: %v", err)
	}
	if rep.Err != nil {
		t.Fatalf("report contains unexpected error: %v", rep.Err)
	}
	if rep.Root != dir {
		t.Fatalf("expected root %s, got %s", dir, rep.Root)
	}
	if rep.Stats.ScannedFiles != 2 {
		t.Fatalf("expected 2 scanned files, got %d", rep.Stats.ScannedFiles)
	}
	if rep.Stats.RemovedStale != 1 {
		t.Fatalf("expected 1 removed stale lock, got %d", rep.Stats.RemovedStale)
	}
	if rep.Stats.PreservedActive != 1 {
		t.Fatalf("expected 1 preserved active lock, got %d", rep.Stats.PreservedActive)
	}
	if rep.Stats.SweepDuration < 0 {
		t.Fatalf("expected positive sweep duration, got %v", rep.Stats.SweepDuration)
	}

	mustNotExist(t, stale)
	mustExist(t, active)
}

func TestSweep_FailClosedRootValidation(t *testing.T) {
	t.Parallel()

	// Empty root
	repEmpty, errEmpty := Sweep("", 10*time.Second)
	if !errors.Is(errEmpty, ErrEmptyRoot) {
		t.Fatalf("expected ErrEmptyRoot for empty string, got %v", errEmpty)
	}
	if !errors.Is(repEmpty.Err, ErrEmptyRoot) {
		t.Fatalf("expected repEmpty.Err to be ErrEmptyRoot, got %v", repEmpty.Err)
	}

	// Root equal to separator
	sep := string(filepath.Separator)
	repSep, errSep := Sweep(sep, 10*time.Second)
	if !errors.Is(errSep, ErrEmptyRoot) {
		t.Fatalf("expected ErrEmptyRoot for root separator, got %v", errSep)
	}
	if !errors.Is(repSep.Err, ErrEmptyRoot) {
		t.Fatalf("expected repSep.Err to be ErrEmptyRoot, got %v", repSep.Err)
	}

	// Missing root directory
	missingDir := filepath.Join(t.TempDir(), "non_existent_subdir")
	repMissing, errMissing := Sweep(missingDir, 10*time.Second)
	if errMissing == nil {
		t.Fatalf("expected error for missing root dir, got nil")
	}
	if repMissing.Err == nil {
		t.Fatalf("expected report error for missing root dir, got nil")
	}

	// Non-directory root
	filePath := filepath.Join(t.TempDir(), "regular_file.txt")
	if err := fileutil.WriteFile(filePath, []byte("content"), paths.FilePerm600); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	repFile, errFile := Sweep(filePath, 10*time.Second)
	if errFile == nil {
		t.Fatalf("expected error for non-directory root, got nil")
	}
	if repFile.Err == nil {
		t.Fatalf("expected report error for non-directory root, got nil")
	}
}

func TestSweep_FailClosedThresholdValidation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createTestLock(t, dir, "intact.lock", time.Hour, referenceNow)

	testCases := []struct {
		name      string
		threshold time.Duration
	}{
		{name: "zero threshold", threshold: 0},
		{name: "negative nanosecond", threshold: -1 * time.Nanosecond},
		{name: "negative minute", threshold: -10 * time.Minute},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rep, err := SweepAt(dir, tc.threshold, referenceNow)
			if !errors.Is(err, ErrNonPositiveThreshold) {
				t.Fatalf("expected ErrNonPositiveThreshold, got %v", err)
			}
			if !errors.Is(rep.Err, ErrNonPositiveThreshold) {
				t.Fatalf("expected rep.Err to be ErrNonPositiveThreshold, got %v", rep.Err)
			}
			mustExist(t, filepath.Join(dir, "intact.lock"))
		})
	}
}

func TestSweep_UnreadableDirectoryFailsClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 0000 directory permission semantics not applicable on Windows")
	}

	dir := t.TempDir()
	subDir := filepath.Join(dir, "unreadable_locks")
	if err := fileutil.MkdirAll(subDir, paths.DirPerm750); err != nil {
		t.Fatalf("failed to create subDir: %v", err)
	}

	createTestLock(t, subDir, "stale.lock", 2*time.Hour, referenceNow)

	// Remove read/execute permissions from subDir
	if err := os.Chmod(subDir, 0000); err != nil {
		t.Fatalf("failed to chmod 0000: %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := os.Chmod(subDir, paths.DirPerm750); cleanupErr != nil {
			t.Logf("cleanup chmod error: %v", cleanupErr)
		}
	})

	rep, err := SweepAt(subDir, 30*time.Second, referenceNow)
	if !errors.Is(err, ErrDirNotReadable) {
		t.Fatalf("expected ErrDirNotReadable, got %v", err)
	}
	if !errors.Is(rep.Err, ErrDirNotReadable) {
		t.Fatalf("expected rep.Err to be ErrDirNotReadable, got %v", rep.Err)
	}
}

func TestSweep_ScannedVsCollateralPreservation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	now := referenceNow

	// Collateral files of various types (should never be swept)
	collateralFiles := []string{
		"state_hints",
		"checkpoint.json",
		"active_worker.tmp",
		"README.md",
		"lock_metadata",
		".dotfile.lock.bak",
	}
	for _, cf := range collateralFiles {
		p := filepath.Join(dir, cf)
		if err := fileutil.WriteFile(p, []byte("collateral data"), paths.FilePerm600); err != nil {
			t.Fatalf("failed to write collateral file %s: %v", cf, err)
		}
		ancient := now.Add(-48 * time.Hour)
		if err := os.Chtimes(p, ancient, ancient); err != nil {
			t.Fatalf("failed to chtimes for %s: %v", cf, err)
		}
	}

	// Legitimate lock files
	staleLock := createTestLock(t, dir, "expired.lock", 2*time.Hour, now)
	activeLock := createTestLock(t, dir, "active.lock", 10*time.Second, now)
	exactBoundaryLock := createTestLock(t, dir, "boundary.lock", 30*time.Second, now)

	rep, err := SweepAt(dir, 30*time.Second, now)
	if err != nil {
		t.Fatalf("SweepAt failed: %v", err)
	}
	if rep.Err != nil {
		t.Fatalf("report error: %v", rep.Err)
	}

	totalExpectedScanned := len(collateralFiles) + 3
	if rep.Stats.ScannedFiles != totalExpectedScanned {
		t.Fatalf("expected %d scanned files, got %d", totalExpectedScanned, rep.Stats.ScannedFiles)
	}
	if rep.Stats.RemovedStale != 1 {
		t.Fatalf("expected 1 removed stale lock, got %d", rep.Stats.RemovedStale)
	}
	expectedPreserved := len(collateralFiles) + 2
	if rep.Stats.PreservedActive != expectedPreserved {
		t.Fatalf("expected %d preserved active, got %d", expectedPreserved, rep.Stats.PreservedActive)
	}

	mustNotExist(t, staleLock)
	mustExist(t, activeLock)
	mustExist(t, exactBoundaryLock)
	for _, cf := range collateralFiles {
		mustExist(t, filepath.Join(dir, cf))
	}
}

func TestSweep_DanglingSymlinkHandledGracefully(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires admin privileges on Windows")
	}
	t.Parallel()

	dir := t.TempDir()
	now := referenceNow

	// Create a dangling symlink ending in .lock
	danglingSymlink := filepath.Join(dir, "dangling_pointer.lock")
	target := filepath.Join(dir, "non_existent_target.lock")
	if err := os.Symlink(target, danglingSymlink); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Create valid stale and active locks
	staleLock := createTestLock(t, dir, "stale_entry.lock", 2*time.Hour, now)
	activeLock := createTestLock(t, dir, "active_entry.lock", 10*time.Second, now)

	rep, err := SweepAt(dir, 30*time.Second, now)
	if err != nil {
		t.Fatalf("SweepAt failed: %v", err)
	}
	if rep.Err != nil {
		t.Fatalf("unexpected report error: %v", rep.Err)
	}

	// 3 entries scanned: danglingSymlink (statErr != nil, skipped), staleLock (removed), activeLock (preserved)
	if rep.Stats.ScannedFiles != 3 {
		t.Fatalf("expected 3 scanned files, got %d", rep.Stats.ScannedFiles)
	}
	if rep.Stats.RemovedStale != 1 {
		t.Fatalf("expected 1 removed stale, got %d", rep.Stats.RemovedStale)
	}
	if rep.Stats.PreservedActive != 1 {
		t.Fatalf("expected 1 preserved active, got %d", rep.Stats.PreservedActive)
	}

	mustNotExist(t, staleLock)
	mustExist(t, activeLock)
}

func TestIsFlockHeld_DirectProbing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("flock probing not supported on windows")
	}
	t.Parallel()

	dir := t.TempDir()
	p := createTestLock(t, dir, "probe.lock", time.Hour, referenceNow)

	// Unlocked file returns false
	if isFlockHeld(p) {
		t.Fatalf("expected isFlockHeld to return false for unlocked file")
	}

	// Exclusively locked file returns true
	holder := holdExclusiveFlock(t, p)
	if !isFlockHeld(p) {
		t.Fatalf("expected isFlockHeld to return true when flock is held")
	}
	releaseFlockAndClose(t, holder)

	// After release, returns false
	if isFlockHeld(p) {
		t.Fatalf("expected isFlockHeld to return false after lock release")
	}

	// Non-existent path returns false
	nonExistent := filepath.Join(dir, "does_not_exist.lock")
	if isFlockHeld(nonExistent) {
		t.Fatalf("expected isFlockHeld to return false for non-existent file")
	}
}

func TestIsFlockHeld_ReadOnlyFileFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only flock semantics differ on Windows")
	}
	t.Parallel()

	dir := t.TempDir()
	p := createTestLock(t, dir, "readonly_probe.lock", time.Hour, referenceNow)

	// Make file read-only (0400). fileutil.OpenFile(O_RDWR) will fail, triggering O_RDONLY fallback.
	if err := os.Chmod(p, 0400); err != nil {
		t.Fatalf("failed to chmod 0400: %v", err)
	}

	if isFlockHeld(p) {
		t.Fatalf("expected isFlockHeld to return false for unlocked read-only file")
	}

	// Hold a shared flock via O_RDONLY
	f, err := fileutil.OpenFile(p, fileutil.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("failed to open read-only lock: %v", err)
	}
	if err := syscallutil.FileFlock(f, syscall.LOCK_SH); err != nil {
		if closeErr := f.Close(); closeErr != nil {
			t.Logf("close error: %v", closeErr)
		}
		t.Fatalf("failed to acquire shared lock: %v", err)
	}

	// Non-blocking LOCK_EX in isFlockHeld should detect conflict and return true
	if !isFlockHeld(p) {
		t.Fatalf("expected isFlockHeld to return true when shared flock is held")
	}

	releaseFlockAndClose(t, f)
}

func TestOwnerVerifiedDead_Validation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX kill(0) probing not supported on Windows")
	}
	t.Parallel()

	// Fail-closed bounds: 0, negative PIDs
	invalidPIDs := []int{0, -1, -42, -99999}
	for _, pid := range invalidPIDs {
		if OwnerVerifiedDead(pid) {
			t.Fatalf("OwnerVerifiedDead(%d) must return false (fail-closed)", pid)
		}
	}

	// Live current process
	if OwnerVerifiedDead(os.Getpid()) {
		t.Fatalf("OwnerVerifiedDead for current process (%d) must return false", os.Getpid())
	}

	// Spawn a short-lived process and wait for it to terminate
	cmd := testkit.ManagedCommand(t, t.Context(), "echo", "probe")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to execute probe process: %v", err)
	}
	deadPid := cmd.Process.Pid

	// Dead process PID should return true (OwnerVerifiedDead)
	if !OwnerVerifiedDead(deadPid) {
		t.Fatalf("OwnerVerifiedDead for reaped process PID %d must return true", deadPid)
	}
}

func TestSweep_ConcurrentSweepsSafety(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	now := referenceNow
	threshold := 10 * time.Second

	// Seed directory with multiple lock files
	for i := 0; i < 20; i++ {
		name := filepath.Join(dir, "concurrency_"+string(rune('a'+i))+".lock")
		age := 5 * time.Second
		if i%2 == 0 {
			age = 30 * time.Second // stale
		}
		if err := fileutil.WriteFile(name, []byte("lock"), paths.FilePerm600); err != nil {
			t.Fatalf("write file: %v", err)
		}
		modTime := now.Add(-age)
		if err := os.Chtimes(name, modTime, modTime); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}

	const goroutines = 5
	var wg sync.WaitGroup
	wg.Add(goroutines)

	errChan := make(chan error, goroutines)

	for g := 0; g < goroutines; g++ {
		goroutinelabels.NewGoroutine("test.lockhealth.concurrent_sweep", "concurrent sweep test").
			StartSimple(func() {
				defer wg.Done()
				rep, err := SweepAt(dir, threshold, now)
				if err != nil {
					errChan <- err
					return
				}
				if rep.Err != nil {
					errChan <- rep.Err
					return
				}
			})
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Fatalf("concurrent sweep failed: %v", err)
	}

	// Final verification sweep
	finalRep, finalErr := SweepAt(dir, threshold, now)
	if finalErr != nil {
		t.Fatalf("final sweep error: %v", finalErr)
	}
	if finalRep.Stats.RemovedStale != 0 {
		t.Fatalf("expected 0 stale removed in final sweep, got %d", finalRep.Stats.RemovedStale)
	}
	if finalRep.Stats.PreservedActive != 10 {
		t.Fatalf("expected 10 active preserved in final sweep, got %d", finalRep.Stats.PreservedActive)
	}
}
