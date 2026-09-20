package storage

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/storage/file"
)

type mockLockHandle struct {
	path     string
	released bool
	mu       sync.Mutex
}

func (h *mockLockHandle) Release() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.released = true
	return nil
}

func (h *mockLockHandle) IsLocked() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return !h.released
}

type mockLockStrategy struct {
	failOnPath string
	handles    map[string]*mockLockHandle
	mu         sync.Mutex
}

func newMockLockStrategy() *mockLockStrategy {
	return &mockLockStrategy{
		handles: make(map[string]*mockLockHandle),
	}
}

func (s *mockLockStrategy) AcquireLock(lockPath string, timeout time.Duration) (file.LockHandle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failOnPath != "" && lockPath == s.failOnPath {
		return nil, errors.New("simulated lock acquisition failure")
	}
	handle := &mockLockHandle{path: lockPath}
	s.handles[lockPath] = handle
	return handle, nil
}

func (s *mockLockStrategy) CleanupStaleLocks(lockPath string) error {
	return nil
}

func (s *mockLockStrategy) Name() string {
	return "mock"
}

// TestFileTransactionCoordinator_AcquireAndRelease tests standard happy path locking and unlocking.
func TestFileTransactionCoordinator_AcquireAndRelease(t *testing.T) {
	defer goleak.VerifyNone(t)
	strategy := newMockLockStrategy()
	coord := NewFileTransactionCoordinator("/test/project", strategy)

	ctx := context.Background()
	files := []string{"/test/project/fileA.json", "/test/project/fileB.json"}

	err := coord.AcquireLocks(ctx, files, 5*time.Second)
	if err != nil {
		t.Fatalf("AcquireLocks failed unexpectedly: %v", err)
	}

	lockedFiles := coord.GetLockedFiles()
	if len(lockedFiles) != 2 {
		t.Fatalf("expected 2 locked files, got %d", len(lockedFiles))
	}

	for _, f := range files {
		if !coord.IsLocked(f) {
			t.Errorf("expected file %s to be locked", f)
		}
	}

	err = coord.ReleaseLocks(ctx)
	if err != nil {
		t.Fatalf("ReleaseLocks failed: %v", err)
	}

	lockedFilesAfter := coord.GetLockedFiles()
	if len(lockedFilesAfter) != 0 {
		t.Fatalf("expected 0 locked files after release, got %d", len(lockedFilesAfter))
	}
}

// TestFileTransactionCoordinator_AcquireFailureRollback tests BLI-CEF-CON-001:
// on lock failure, previously acquired locks are released safely under coordinator mutex.
func TestFileTransactionCoordinator_AcquireFailureRollback(t *testing.T) {
	defer goleak.VerifyNone(t)
	strategy := newMockLockStrategy()
	// Set failure on the second file in sorted order
	strategy.failOnPath = "/test/project/fileB.json.txn.lock"
	coord := NewFileTransactionCoordinator("/test/project", strategy)

	ctx := context.Background()
	files := []string{"/test/project/fileA.json", "/test/project/fileB.json"}

	err := coord.AcquireLocks(ctx, files, 5*time.Second)
	if err == nil {
		t.Fatalf("expected AcquireLocks to fail, but succeeded")
	}

	// Verify that fileA's lock was acquired and then rolled back/released
	handleA := strategy.handles["/test/project/fileA.json.txn.lock"]
	if handleA == nil {
		t.Fatalf("expected lock handle for fileA to have been created")
	}
	if !handleA.released {
		t.Errorf("expected handle for fileA to be released on rollback")
	}

	lockedFiles := coord.GetLockedFiles()
	if len(lockedFiles) != 0 {
		t.Fatalf("expected 0 locked files after failure rollback, got %d", len(lockedFiles))
	}
}

// TestFileTransactionCoordinator_ConcurrentTransactions tests concurrency safety under multiple goroutines.
func TestFileTransactionCoordinator_ConcurrentTransactions(t *testing.T) {
	defer goleak.VerifyNone(t)
	tempDir := t.TempDir()
	strategy := newMockLockStrategy()
	coord := NewFileTransactionCoordinator(tempDir, strategy)

	ctx := context.Background()
	var wg sync.WaitGroup
	const workers = 10

	for i := 0; i < workers; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("test.concurrent.txn", "concurrent file transaction coordinator test").
			StartSimple(func() {
				defer wg.Done()
				f := filepath.Join(tempDir, "shared.json")
				files := []string{f}
				_ = coord.AcquireLocks(ctx, files, time.Second)
				_ = coord.ReleaseLocks(ctx)
			})
	}

	wg.Wait()
}
