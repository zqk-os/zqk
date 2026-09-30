package validation

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	syscallutil "github.com/zqk-os/zqk/pkg/utils/syscallutil"
)

// TestValidationStateCache_StaleLockRecovery tests that stale locks from dead processes
// are automatically detected and recovered
// NOTE: Cannot use t.Parallel() - this test uses os.Setenv() which is incompatible with parallel execution
func TestValidationStateCache_StaleLockRecovery(t *testing.T) {
	testRoot := registerZQKTestRootForTest(t)

	cache := NewValidationStateCache(testRoot, time.Hour)
	// Get lock file path (same pattern as Save method)
	cacheFile := filepath.Join(testRoot, paths.ProjectDataDir, paths.CacheDir, "validation_cache.json")
	lockFile := cacheFile + ".lock"

	// Create cache directory first
	if err := fileutil.MkdirAll(filepath.Dir(lockFile), paths.DirPerm755); err != nil {
		t.Fatalf("failed to create cache directory: %v", err)
	}

	// Simulate a stale lock by creating an old lock file
	// (In real scenario, this would be from a hung/dead process)
	file, err := fileutil.OpenFile(lockFile, fileutil.O_CREATE|fileutil.O_RDWR, paths.FilePerm644)
	if err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}

	// Acquire lock
	err = syscallutil.FileFlock(file, syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		t.Fatalf("failed to acquire initial lock: %v", err)
	}

	// Make the lock file appear old (simulating a hung process)
	oldTime := time.Now().Add(-35 * time.Second) // Older than 30s stale timeout
	if err := fileutil.Chtimes(lockFile, oldTime, oldTime); err != nil {
		t.Fatalf("failed to set old modification time: %v", err)
	}

	// Close the file (simulating process death)
	// On Unix, closing the file descriptor releases the lock
	_ = file.Close()

	// Now try to save - should detect stale lock and recover
	state := &ValidationState{
		ObjectID:      "TEST-001",
		ObjectKind:    "test_object",
		FilePath:      "test.yaml",
		LastValidated: time.Now(),
		Checksum:      "abc123",
		Issues:        []ValidationIssue{},
	}
	cache.Set(state)

	// Save should succeed (stale lock should be detected and removed)
	if err := cache.Save(); err != nil {
		t.Fatalf("failed to save after stale lock recovery: %v", err)
	}

	// Verify cache was saved
	cache2 := NewValidationStateCache(testRoot, time.Hour)
	if err := cache2.Load(); err != nil {
		t.Fatalf("failed to load cache: %v", err)
	}

	_, exists := cache2.Get("TEST-001")
	if !exists {
		t.Error("expected TEST-001 to be in cache after save")
	}
}

// TestValidationStateCache_ActiveLockRespected tests that active locks (recent modification time)
// are respected and not broken
// NOTE: Cannot use t.Parallel() - this test uses os.Setenv() which is incompatible with parallel execution
// NOTE: This test is complex and may be flaky - skip in short mode for now
func TestValidationStateCache_ActiveLockRespected(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping complex lock test in short mode")
	}
	testRoot := registerZQKTestRootForTest(t)

	cache := NewValidationStateCache(testRoot, time.Hour)
	// Get lock file path (same pattern as Save method)
	cacheFile := filepath.Join(testRoot, paths.ProjectDataDir, paths.CacheDir, "validation_cache.json")
	lockFile := cacheFile + ".lock"

	// Create cache directory first
	if err := fileutil.MkdirAll(filepath.Dir(lockFile), paths.DirPerm755); err != nil {
		t.Fatalf("failed to create cache directory: %v", err)
	}

	// Create a lock file with recent modification time (simulating active process)
	file, err := fileutil.OpenFile(lockFile, fileutil.O_CREATE|fileutil.O_RDWR, paths.FilePerm644)
	if err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}
	// Don't defer close - we need to keep the file open to hold the lock
	// We'll close it explicitly after the test

	// Acquire lock
	err = syscallutil.FileFlock(file, syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		_ = file.Close()
		t.Fatalf("failed to acquire lock: %v", err)
	}

	// Update lock file modification time to make it appear recent (active)
	now := time.Now()
	if err := fileutil.Chtimes(lockFile, now, now); err != nil {
		_ = file.Close()
		t.Fatalf("failed to update lock file time: %v", err)
	}

	// Try to save - should fail because lock is active
	state := &ValidationState{
		ObjectID:      "TEST-001",
		ObjectKind:    "test_object",
		FilePath:      "test.yaml",
		LastValidated: time.Now(),
		Checksum:      "abc123",
		Issues:        []ValidationIssue{},
	}
	cache.Set(state)

	err = cache.Save()
	_ = file.Close() // Release lock after test
	if err == nil {
		t.Error("expected save to fail when lock is actively held")
	}
	if err != nil && err.Error() != "cache file is locked by another process" {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestValidationStateCache_ProcessDeathLockRelease tests that locks are automatically
// released when a process dies (OS behavior)
// NOTE: Cannot use t.Parallel() - this test uses os.Setenv() which is incompatible with parallel execution
func TestValidationStateCache_ProcessDeathLockRelease(t *testing.T) {
	testRoot := registerZQKTestRootForTest(t)

	cache := NewValidationStateCache(testRoot, time.Hour)
	// Get lock file path (same pattern as Save method)
	cacheFile := filepath.Join(testRoot, paths.ProjectDataDir, paths.CacheDir, "validation_cache.json")
	lockFile := cacheFile + ".lock"

	// Create cache directory first
	if err := fileutil.MkdirAll(filepath.Dir(lockFile), paths.DirPerm755); err != nil {
		t.Fatalf("failed to create cache directory: %v", err)
	}

	// Simulate process death by acquiring lock and then closing file
	// On Unix, closing the file descriptor releases the lock automatically
	file, err := fileutil.OpenFile(lockFile, fileutil.O_CREATE|fileutil.O_RDWR, paths.FilePerm644)
	if err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}

	err = syscallutil.FileFlock(file, syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}

	// Close file (simulating process death)
	// This releases the lock automatically on Unix systems
	_ = file.Close()

	// Wait a moment for OS to release lock
	time.Sleep(100 * time.Millisecond)

	// Now try to save - should succeed because lock was released
	state := &ValidationState{
		ObjectID:      "TEST-001",
		ObjectKind:    "test_object",
		FilePath:      "test.yaml",
		LastValidated: time.Now(),
		Checksum:      "abc123",
		Issues:        []ValidationIssue{},
	}
	cache.Set(state)

	if err := cache.Save(); err != nil {
		t.Fatalf("failed to save after process death (lock should be released): %v", err)
	}
}
