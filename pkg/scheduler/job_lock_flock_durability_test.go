package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	testJobIDDurable                 = "flock-durable-job"
	testJobIDConcurrent              = "flock-concurrent-job"
	testJobIDExecution               = "flock-execution-job"
	errTestCreateJobLockFailed       = "NewJobLockWithConfig failed: %v"
	errTestAcquireFailed             = "JobLock.Acquire failed: %v"
	errTestTryAcquireFailed          = "JobLock.TryAcquire failed: %v"
	errTestStatFailed                = "failed to stat lock file: %v"
	errTestInodeChanged              = "lock file inode changed during acquisition (unlink detected): old=%d new=%d"
	errTestMultipleHoldersDetected   = "mutual exclusion breach: multiple concurrent holders detected (%d)"
	errTestExpectedLockHeld          = "expected lock to be held by first caller"
	errTestExpectedLockNotAcquired   = "expected second caller to fail to acquire held lock"
	errTestCloseFailed               = "JobLock.Close failed: %v"
	errTestReleaseFailed             = "JobLock.Release failed: %v"
	errTestSyscallStatExtract        = "could not extract syscall.Stat_t from FileInfo"
	errTestCreateLockDir             = "failed to create lock directory: %v"
	errTestCreateFixtureLockFile     = "failed to create fixture lock file: %v"
	errTestChtimesLockFile           = "failed to chtimes on lock file: %v"
	errTestExpectedLockedTrue        = "expected jl.IsLocked() to be true"
	errTestWorkerTryAcquire          = "worker %d TryAcquire error: %v"
	errTestAtLeastOneAcquire         = "expected at least one successful TryAcquire across concurrent workers"
	errTestPostCloseAcquireSuccess   = "expected jl2.TryAcquire to succeed after jl1 closed"
)

// getFileInode returns the OS inode number for the given file path.
func getFileInode(path string) (uint64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf(errTestSyscallStatExtract)
	}
	return stat.Ino, nil
}

// TestJobLock_FlockDurabilityAndInodePreservation verifies CRIT-1790566371956145000-3882bc1a:
// JobLock relies exclusively on OS flock descriptor binding and does not unlink the lock file
// during acquisition or stale lock evaluation, preserving inode identity and mutual exclusion.
func TestJobLock_FlockDurabilityAndInodePreservation(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	lockDir := filepath.Join(tmpDir, "locks")

	config := JobLockConfig{
		LockDir:            lockDir,
		LockTimeout:        5 * time.Second,
		StaleLockThreshold: 1 * time.Hour,
	}

	if err := fileutil.MkdirAll(lockDir, 0755); err != nil {
		t.Fatalf(errTestCreateLockDir, err)
	}

	lockPath := filepath.Join(lockDir, lockFilenameForJobID(testJobIDDurable))

	// Pre-create the lock file with an old modification timestamp to simulate a stale lock file.
	f, err := fileutil.Create(lockPath)
	if err != nil {
		t.Fatalf(errTestCreateFixtureLockFile, err)
	}
	_ = f.Close()

	oldTime := time.Now().Add(-24 * time.Hour)
	if err := fileutil.Chtimes(lockPath, oldTime, oldTime); err != nil {
		t.Fatalf(errTestChtimesLockFile, err)
	}

	initialInode, err := getFileInode(lockPath)
	if err != nil {
		t.Fatalf(errTestStatFailed, err)
	}

	jl, err := NewJobLockWithConfig(testJobIDDurable, config)
	if err != nil {
		t.Fatalf(errTestCreateJobLockFailed, err)
	}
	defer func() { _ = jl.Close() }()

	if err := jl.Acquire(); err != nil {
		t.Fatalf(errTestAcquireFailed, err)
	}

	postAcquireInode, err := getFileInode(lockPath)
	if err != nil {
		t.Fatalf(errTestStatFailed, err)
	}

	if initialInode != postAcquireInode {
		t.Fatalf(errTestInodeChanged, initialInode, postAcquireInode)
	}

	if !jl.IsLocked() {
		t.Error(errTestExpectedLockedTrue)
	}

	if err := jl.Release(); err != nil {
		t.Fatalf(errTestReleaseFailed, err)
	}

	postReleaseInode, err := getFileInode(lockPath)
	if err != nil {
		t.Fatalf(errTestStatFailed, err)
	}

	if initialInode != postReleaseInode {
		t.Fatalf(errTestInodeChanged, initialInode, postReleaseInode)
	}
}

// TestJobLock_ConcurrentMutualExclusionAndInodeConflict verifies CRIT-1790566371956146000-2b6603a7:
// Concurrent acquisition attempts safely detect held locks, handle non-existent directories,
// and guarantee strict single-holder mutual exclusion without race conditions.
func TestJobLock_ConcurrentMutualExclusionAndInodeConflict(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	lockDir := filepath.Join(tmpDir, "nested", "locks") // non-existent parent directory

	config := JobLockConfig{
		LockDir:            lockDir,
		LockTimeout:        2 * time.Second,
		StaleLockThreshold: 1 * time.Hour,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const concurrentWorkers = 8
	var activeHolders atomic.Int32
	var breachDetected atomic.Bool
	var successfulAcquisitions atomic.Int32

	var wg sync.WaitGroup
	startSignal := make(chan struct{})

	for i := 0; i < concurrentWorkers; i++ {
		wg.Add(1)
		wID := i
		wCtx := ctx
		goroutinelabels.NewGoroutine("test.job_lock_worker", "flock durability concurrent worker").StartSimple(func() {
			defer wg.Done()
			select {
			case <-wCtx.Done():
				return
			case <-startSignal:
			}

			jl, err := NewJobLockWithConfig(testJobIDConcurrent, config)
			if err != nil {
				return
			}
			defer func() { _ = jl.Close() }()

			acquired, err := jl.TryAcquire()
			if err != nil {
				t.Errorf(errTestWorkerTryAcquire, wID, err)
				return
			}

			if acquired {
				current := activeHolders.Add(1)
				if current > 1 {
					breachDetected.Store(true)
				}
				successfulAcquisitions.Add(1)

				// Hold briefly to test exclusivity
				time.Sleep(20 * time.Millisecond)

				activeHolders.Add(-1)
				_ = jl.Release()
			}
		})
	}

	close(startSignal)
	wg.Wait()

	if breachDetected.Load() {
		t.Fatalf(errTestMultipleHoldersDetected, activeHolders.Load())
	}

	if successfulAcquisitions.Load() < 1 {
		t.Fatal(errTestAtLeastOneAcquire)
	}
}

// TestJobLock_SchedulerJobExecutionConformance verifies CRIT-1790566371956147000-6d91cab0:
// Bounded timeout acquisition, TryAcquire fast-path, and clean Close teardown ensure deterministic
// mutual exclusion and immediate handoff across scheduler jobs.
func TestJobLock_SchedulerJobExecutionConformance(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	config := JobLockConfig{
		LockDir:            filepath.Join(tmpDir, "exec-locks"),
		LockTimeout:        100 * time.Millisecond,
		StaleLockThreshold: 1 * time.Hour,
	}

	jl1, err := NewJobLockWithConfig(testJobIDExecution, config)
	if err != nil {
		t.Fatalf(errTestCreateJobLockFailed, err)
	}
	defer func() { _ = jl1.Close() }()

	if err := jl1.Acquire(); err != nil {
		t.Fatalf(errTestAcquireFailed, err)
	}

	if !jl1.IsLocked() {
		t.Fatal(errTestExpectedLockHeld)
	}

	jl2, err := NewJobLockWithConfig(testJobIDExecution, config)
	if err != nil {
		t.Fatalf(errTestCreateJobLockFailed, err)
	}
	defer func() { _ = jl2.Close() }()

	acquired, err := jl2.TryAcquire()
	if err != nil {
		t.Fatalf(errTestTryAcquireFailed, err)
	}
	if acquired {
		t.Fatal(errTestExpectedLockNotAcquired)
	}

	// Release first lock via Close
	if err := jl1.Close(); err != nil {
		t.Fatalf(errTestCloseFailed, err)
	}

	// Second lock should now succeed immediately
	acquired2, err := jl2.TryAcquire()
	if err != nil {
		t.Fatalf(errTestTryAcquireFailed, err)
	}
	if !acquired2 {
		t.Fatal(errTestPostCloseAcquireSuccess)
	}

	if err := jl2.Release(); err != nil {
		t.Fatalf(errTestReleaseFailed, err)
	}
}
