package file

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestFileLock_BasicLockUnlock(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")

	// Create lock
	fileLock, err := NewFileLock(lockFile)
	if err != nil {
		t.Fatalf("Failed to create file lock: %v", err)
	}
	defer fileLock.Close()

	// Acquire lock
	if err := fileLock.Lock(); err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Verify lock is held
	if !fileLock.IsLocked() {
		t.Error("Lock should be held")
	}

	// Release lock
	if err := fileLock.Unlock(); err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}

	// Verify lock is released
	if fileLock.IsLocked() {
		t.Error("Lock should not be held")
	}
}

func TestFileLock_TryLock_Success(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")

	fileLock, err := NewFileLock(lockFile)
	if err != nil {
		t.Fatalf("Failed to create file lock: %v", err)
	}
	defer fileLock.Close()

	// Try to acquire lock (should succeed)
	acquired, err := fileLock.TryLock()
	if err != nil {
		t.Fatalf("TryLock returned error: %v", err)
	}
	if !acquired {
		t.Error("TryLock should have acquired the lock")
	}

	// Release
	if err := fileLock.Unlock(); err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}
}

func TestFileLock_TryLock_AlreadyLocked(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")

	// First lock
	fileLock1, err := NewFileLock(lockFile)
	if err != nil {
		t.Fatalf("Failed to create file lock: %v", err)
	}
	defer fileLock1.Close()

	if err := fileLock1.Lock(); err != nil {
		t.Fatalf("Failed to acquire first lock: %v", err)
	}

	// Second lock (should fail to acquire)
	fileLock2, err := NewFileLock(lockFile)
	if err != nil {
		t.Fatalf("Failed to create second file lock: %v", err)
	}
	defer fileLock2.Close()

	acquired, err := fileLock2.TryLock()
	if err != nil {
		t.Fatalf("TryLock returned error: %v", err)
	}
	if acquired {
		t.Error("TryLock should not have acquired the lock (already held by first lock)")
	}

	// Release first lock
	if err := fileLock1.Unlock(); err != nil {
		t.Fatalf("Failed to release first lock: %v", err)
	}
}

func TestFileLock_LockWithTimeout_Success(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")

	fileLock, err := NewFileLock(lockFile)
	if err != nil {
		t.Fatalf("Failed to create file lock: %v", err)
	}
	defer fileLock.Close()

	// Acquire lock with timeout (should succeed immediately)
	if err := fileLock.LockWithTimeout(1 * time.Second); err != nil {
		t.Fatalf("LockWithTimeout failed: %v", err)
	}

	// Release
	if err := fileLock.Unlock(); err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}
}

func TestFileLock_LockWithTimeout_Timeout(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")

	// First lock (hold it)
	fileLock1, err := NewFileLock(lockFile)
	if err != nil {
		t.Fatalf("Failed to create file lock: %v", err)
	}
	defer fileLock1.Close()

	if err := fileLock1.Lock(); err != nil {
		t.Fatalf("Failed to acquire first lock: %v", err)
	}

	// Second lock (should timeout)
	fileLock2, err := NewFileLock(lockFile)
	if err != nil {
		t.Fatalf("Failed to create second file lock: %v", err)
	}
	defer fileLock2.Close()

	start := time.Now()
	err = fileLock2.LockWithTimeout(100 * time.Millisecond)
	duration := time.Since(start)

	if err == nil {
		t.Error("LockWithTimeout should have failed (lock already held)")
	}
	if duration < 100*time.Millisecond {
		t.Errorf("LockWithTimeout should have waited at least 100ms, waited %v", duration)
	}
	if duration > 200*time.Millisecond {
		t.Errorf("LockWithTimeout should have timed out around 100ms, waited %v", duration)
	}

	// Release first lock
	if err := fileLock1.Unlock(); err != nil {
		t.Fatalf("Failed to release first lock: %v", err)
	}
}

func TestFileLock_WithLock(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")

	fileLock, err := NewFileLock(lockFile)
	if err != nil {
		t.Fatalf("Failed to create file lock: %v", err)
	}
	defer fileLock.Close()

	// Use WithLock convenience method
	called := false
	err = fileLock.WithLock(func() error {
		called = true
		if !fileLock.IsLocked() {
			t.Error("Lock should be held inside WithLock callback")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("WithLock returned error: %v", err)
	}
	if !called {
		t.Error("WithLock callback was not called")
	}
	if fileLock.IsLocked() {
		t.Error("Lock should be released after WithLock")
	}
}

func TestFileLock_WithLockTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")

	fileLock, err := NewFileLock(lockFile)
	if err != nil {
		t.Fatalf("Failed to create file lock: %v", err)
	}
	defer fileLock.Close()

	// Use WithLockTimeout convenience method
	called := false
	err = fileLock.WithLockTimeout(1*time.Second, func() error {
		called = true
		if !fileLock.IsLocked() {
			t.Error("Lock should be held inside WithLockTimeout callback")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("WithLockTimeout returned error: %v", err)
	}
	if !called {
		t.Error("WithLockTimeout callback was not called")
	}
	if fileLock.IsLocked() {
		t.Error("Lock should be released after WithLockTimeout")
	}
}

func TestFileLock_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")

	const numGoroutines = 10
	var wg sync.WaitGroup
	var attemptWG sync.WaitGroup
	successCount := 0
	var mu sync.Mutex
	start := make(chan struct{})
	release := make(chan struct{})

	// Multiple goroutines trying to acquire lock
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		attemptWG.Add(1)
		goroutinelabels.NewGoroutine("storage_test", "concurrent file lock attempt").StartSimple(func() {
			func(id int) {
				defer wg.Done()
				<-start

				fileLock, err := NewFileLock(lockFile)
				if err != nil {
					t.Errorf("Goroutine %d: Failed to create file lock: %v", id, err)
					return
				}
				defer fileLock.Close()

				// Try to acquire lock
				acquired, err := fileLock.TryLock()
				attemptWG.Done()
				if err != nil {
					t.Errorf("Goroutine %d: TryLock returned error: %v", id, err)
					return
				}

				if acquired {
					mu.Lock()
					successCount++
					mu.Unlock()

					// Hold lock until all contenders attempted TryLock once.
					// This makes the test deterministic: only one acquisition should succeed.
					<-release

					// Release lock
					if err := fileLock.Unlock(); err != nil {
						t.Errorf("Goroutine %d: Failed to release lock: %v", id, err)
					}
				}
			}(i)
		})
	}

	close(start)
	attemptWG.Wait()
	close(release)
	wg.Wait()

	// Only one goroutine should have successfully acquired the lock
	if successCount != 1 {
		t.Errorf("Expected exactly 1 successful lock acquisition, got %d", successCount)
	}
}

func TestFileLock_DoubleLock_Error(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")

	fileLock, err := NewFileLock(lockFile)
	if err != nil {
		t.Fatalf("Failed to create file lock: %v", err)
	}
	defer fileLock.Close()

	// Acquire lock
	if err := fileLock.Lock(); err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Try to acquire again (should fail)
	if err := fileLock.Lock(); err == nil {
		t.Error("Double lock should have returned an error")
	}

	// Release
	if err := fileLock.Unlock(); err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}
}

func TestFileLock_UnlockWithoutLock_Error(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")

	fileLock, err := NewFileLock(lockFile)
	if err != nil {
		t.Fatalf("Failed to create file lock: %v", err)
	}
	defer fileLock.Close()

	// Try to unlock without locking (should fail)
	if err := fileLock.Unlock(); err == nil {
		t.Error("Unlock without lock should have returned an error")
	}
}

func TestFileLock_Close_ReleasesLock(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")

	fileLock, err := NewFileLock(lockFile)
	if err != nil {
		t.Fatalf("Failed to create file lock: %v", err)
	}

	// Acquire lock
	if err := fileLock.Lock(); err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Close should release lock
	if err := fileLock.Close(); err != nil {
		t.Fatalf("Failed to close file lock: %v", err)
	}

	// Verify lock is released (by trying to acquire it again)
	fileLock2, err := NewFileLock(lockFile)
	if err != nil {
		t.Fatalf("Failed to create second file lock: %v", err)
	}
	defer fileLock2.Close()

	acquired, err := fileLock2.TryLock()
	if err != nil {
		t.Fatalf("TryLock returned error: %v", err)
	}
	if !acquired {
		t.Error("Lock should be available after Close (first lock released)")
	}

	if err := fileLock2.Unlock(); err != nil {
		t.Fatalf("Failed to release second lock: %v", err)
	}
}

func TestFileLock_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	lockDir := filepath.Join(tmpDir, "nested", "deep", "path")
	lockFile := filepath.Join(lockDir, "test.lock")

	// Directory doesn't exist yet
	if _, err := fileutil.Stat(lockDir); err == nil {
		t.Fatal("Lock directory should not exist yet")
	}

	// Create lock (should create directory)
	fileLock, err := NewFileLock(lockFile)
	if err != nil {
		t.Fatalf("Failed to create file lock: %v", err)
	}
	defer fileLock.Close()

	// Directory should now exist
	if _, err := fileutil.Stat(lockDir); err != nil {
		t.Fatalf("Lock directory should have been created: %v", err)
	}
}
