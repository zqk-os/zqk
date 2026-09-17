package scheduler

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

func TestNewJobLock(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	config := JobLockConfig{
		LockDir:            filepath.Join(tmpDir, "locks"),
		LockTimeout:        5 * time.Second,
		StaleLockThreshold: 1 * time.Hour,
	}

	lock, err := NewJobLockWithConfig("test-job-1", config)
	if err != nil {
		t.Fatalf("Failed to create job lock: %v", err)
	}
	if lock == nil {
		t.Fatal("Job lock is nil")
	}
	if lock.(*JobLock).jobID != "test-job-1" {
		t.Errorf("Expected jobID 'test-job-1', got '%s'", lock.(*JobLock).jobID)
	}
	expectedPath := filepath.Join(config.LockDir, "test-job-1.lock")
	if lock.(*JobLock).lockPath != expectedPath {
		t.Errorf("Expected lock path '%s', got '%s'", expectedPath, lock.(*JobLock).lockPath)
	}

	// Verify lock directory was created
	_, err = fileutil.Stat(config.LockDir)
	if err != nil {
		t.Errorf("Lock directory was not created: %v", err)
	}

	// Clean up
	err = lock.(*JobLock).Close()
	if err != nil {
		t.Errorf("Failed to close lock: %v", err)
	}
}

func TestLockFilenameForJobID_LongID_UsesHash(t *testing.T) {
	t.Parallel()
	// Job IDs longer than maxLockFilenameJobIDLen get a hash-based filename to avoid "file name too long"
	longID := "SCH-1772046602-scheduler-job-SCH-1772046601-scheduler-job-" + string(make([]byte, 300))
	filename := lockFilenameForJobID(longID)
	if len(filename) > 80 {
		t.Errorf("lock filename for long job ID should be short (hash hex + .lock), got len %d", len(filename))
	}
	if len(filename) < 60 {
		t.Errorf("expected sha256 hex (64 chars) + .lock, got len %d", len(filename))
	}
	if filepath.Ext(filename) != ".lock" {
		t.Errorf("expected .lock suffix, got %q", filepath.Ext(filename))
	}
	// Same long ID must produce same filename
	if lockFilenameForJobID(longID) != filename {
		t.Error("same job ID must produce same lock filename")
	}
}

func TestLockFilenameForJobID_ShortID_UsesID(t *testing.T) {
	t.Parallel()
	shortID := "SCH-001"
	filename := lockFilenameForJobID(shortID)
	expected := "SCH-001.lock"
	if filename != expected {
		t.Errorf("short job ID should use ID as filename, got %q", filename)
	}
}

func TestJobLock_Acquire_Release(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	config := JobLockConfig{
		LockDir:            filepath.Join(tmpDir, "locks"),
		LockTimeout:        5 * time.Second,
		StaleLockThreshold: 1 * time.Hour,
	}

	lock, err := NewJobLockWithConfig("test-job-1", config)
	if err != nil {
		t.Fatalf("Failed to create job lock: %v", err)
	}
	defer lock.(*JobLock).Close()

	// Acquire lock
	err = lock.(*JobLock).Acquire()
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}
	if !lock.(*JobLock).IsLocked() {
		t.Error("Lock should be locked after Acquire()")
	}
	if lock.(*JobLock).AcquiredAt() == nil {
		t.Error("AcquiredAt() should not be nil after acquiring lock")
	}

	// Release lock
	err = lock.(*JobLock).Release()
	if err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}
	if lock.(*JobLock).IsLocked() {
		t.Error("Lock should not be locked after Release()")
	}
	if lock.(*JobLock).AcquiredAt() != nil {
		t.Error("AcquiredAt() should be nil after releasing lock")
	}
}

func TestJobLock_TryAcquire(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	config := JobLockConfig{
		LockDir:            filepath.Join(tmpDir, "locks"),
		LockTimeout:        5 * time.Second,
		StaleLockThreshold: 1 * time.Hour,
	}

	lock1, err := NewJobLockWithConfig("test-job-1", config)
	if err != nil {
		t.Fatalf("Failed to create lock1: %v", err)
	}
	defer lock1.Close()

	lock2, err := NewJobLockWithConfig("test-job-1", config) // Same job ID
	if err != nil {
		t.Fatalf("Failed to create lock2: %v", err)
	}
	defer lock2.Close()

	// First lock should acquire successfully
	acquired, err := lock1.TryAcquire()
	if err != nil {
		t.Fatalf("TryAcquire failed: %v", err)
	}
	if !acquired {
		t.Error("First lock should acquire successfully")
	}
	if !lock1.IsLocked() {
		t.Error("First lock should be locked")
	}

	// Second lock should fail (non-blocking)
	acquired, err = lock2.TryAcquire()
	if err != nil {
		t.Fatalf("TryAcquire failed: %v", err)
	}
	if acquired {
		t.Error("Second lock should not acquire when first is held")
	}
	if lock2.IsLocked() {
		t.Error("Second lock should not be locked")
	}

	// Release first lock
	err = lock1.Release()
	if err != nil {
		t.Fatalf("Failed to release lock1: %v", err)
	}

	// Now second lock should be able to acquire
	acquired, err = lock2.TryAcquire()
	if err != nil {
		t.Fatalf("TryAcquire failed: %v", err)
	}
	if !acquired {
		t.Error("Second lock should acquire after first is released")
	}
	if !lock2.IsLocked() {
		t.Error("Second lock should be locked")
	}
}

func TestJobLock_Acquire_WithTimeout(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	config := JobLockConfig{
		LockDir:            filepath.Join(tmpDir, "locks"),
		LockTimeout:        100 * time.Millisecond, // Short timeout for testing
		StaleLockThreshold: 1 * time.Hour,
	}

	lock1, err := NewJobLockWithConfig("test-job-1", config)
	if err != nil {
		t.Fatalf("Failed to create lock1: %v", err)
	}
	defer lock1.Close()

	lock2, err := NewJobLockWithConfig("test-job-1", config) // Same job ID
	if err != nil {
		t.Fatalf("Failed to create lock2: %v", err)
	}
	defer lock2.Close()

	// First lock acquires
	err = lock1.Acquire()
	if err != nil {
		t.Fatalf("Failed to acquire lock1: %v", err)
	}

	// Second lock should timeout trying to acquire
	start := time.Now()
	err = lock2.Acquire()
	duration := time.Since(start)

	if err == nil {
		t.Fatal("Expected error when acquiring lock that's already held")
	}
	if err.Error() == emptyValue {
		t.Error("Error message should not be empty")
	}
	// Should have waited approximately the timeout duration (with some tolerance)
	if duration < 90*time.Millisecond {
		t.Errorf("Should have waited at least 90ms, waited %v", duration)
	}
	if duration > 500*time.Millisecond {
		t.Errorf("Should have waited at most 500ms, waited %v", duration)
	}
}

func TestJobLock_StaleLockDetection(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	lockDir := filepath.Join(tmpDir, "locks")
	config := JobLockConfig{
		LockDir:            lockDir,
		LockTimeout:        5 * time.Second,
		StaleLockThreshold: 1 * time.Hour,
	}

	// Create a stale lock file manually
	lockPath := filepath.Join(lockDir, "stale-job.lock")
	err := fileutil.MkdirAll(lockDir, paths.DirPerm755)
	if err != nil {
		t.Fatalf("Failed to create lock directory: %v", err)
	}

	// Create a lock file with old modification time
	file, err := fileutil.Create(lockPath)
	if err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}
	_ = file.Close()

	// Set modification time to 2 hours ago (older than threshold)
	oldTime := time.Now().Add(-2 * time.Hour)
	err = fileutil.Chtimes(lockPath, oldTime, oldTime)
	if err != nil {
		t.Fatalf("Failed to set file times: %v", err)
	}

	// Create a new lock with a shorter stale threshold
	config.StaleLockThreshold = 1 * time.Hour
	lock, err := NewJobLockWithConfig("stale-job", config)
	if err != nil {
		t.Fatalf("Failed to create job lock: %v", err)
	}
	defer lock.(*JobLock).Close()

	// Should be able to acquire lock (stale lock should be cleaned)
	err = lock.(*JobLock).Acquire()
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}
	if !lock.(*JobLock).IsLocked() {
		t.Error("Lock should be locked after Acquire()")
	}
}

func TestJobLock_Close_ReleasesLock(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	config := JobLockConfig{
		LockDir:            filepath.Join(tmpDir, "locks"),
		LockTimeout:        5 * time.Second,
		StaleLockThreshold: 1 * time.Hour,
	}

	lock, err := NewJobLockWithConfig("test-job-1", config)
	if err != nil {
		t.Fatalf("Failed to create job lock: %v", err)
	}

	// Acquire lock
	err = lock.(*JobLock).Acquire()
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}
	if !lock.(*JobLock).IsLocked() {
		t.Error("Lock should be locked after Acquire()")
	}

	// Close should release the lock
	err = lock.(*JobLock).Close()
	if err != nil {
		t.Fatalf("Failed to close lock: %v", err)
	}
	if lock.(*JobLock).IsLocked() {
		t.Error("Lock should not be locked after Close()")
	}
}

func TestJobLock_DefaultConfig(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	config := DefaultJobLockConfig(tmpDir)

	expectedDir := filepath.Join(tmpDir, paths.ProjectDataDir, "scheduler", "locks")
	if config.LockDir != expectedDir {
		t.Errorf("Expected LockDir '%s', got '%s'", expectedDir, config.LockDir)
	}
	if config.LockTimeout != 30*time.Second {
		t.Errorf("Expected LockTimeout 30s, got %v", config.LockTimeout)
	}
	if config.StaleLockThreshold != 1*time.Hour {
		t.Errorf("Expected StaleLockThreshold 1h, got %v", config.StaleLockThreshold)
	}
}

func TestJobLock_ConcurrentAcquisition(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	config := JobLockConfig{
		LockDir:            filepath.Join(tmpDir, "locks"),
		LockTimeout:        2 * time.Second,
		StaleLockThreshold: 1 * time.Hour,
	}

	jobID := "concurrent-job"
	numGoroutines := 10
	acquired := make(chan bool, numGoroutines)
	start := make(chan struct{})

	// Launch multiple goroutines trying to acquire the same lock
	for i := 0; i < numGoroutines; i++ {
		i := i
		goroutinelabels.StartTestGoroutine(fmt.Sprintf("test_lock_acquirer_%d", i), fmt.Sprintf("acquiring lock %d in job lock test", i), func() {
			lock, err := NewJobLockWithConfig(jobID, config)
			if err != nil {
				acquired <- false
				return
			}
			defer lock.(*JobLock).Close()

			// Synchronize attempts so only one goroutine can win.
			<-start

			got, err := lock.(*JobLock).TryAcquire()
			if err != nil {
				acquired <- false
				return
			}
			acquired <- got

			// Hold lock briefly
			time.Sleep(50 * time.Millisecond)
			_ = lock.(*JobLock).Release() //nolint:errcheck // Test helper - error handling not critical
		})
	}

	// Allow all goroutines to hit TryAcquire at roughly the same time.
	close(start)

	// Collect results
	acquiredCount := 0
	for i := 0; i < numGoroutines; i++ {
		if <-acquired {
			acquiredCount++
		}
	}

	// Only one should have acquired the lock
	if acquiredCount != 1 {
		t.Errorf("Expected 1 goroutine to acquire the lock, got %d", acquiredCount)
	}
}
