package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestWatchdogInterceptor_StalledWorkerAborted(t *testing.T) {
	tracker := NewHeartbeatTracker()
	tempDir := t.TempDir()

	interceptor := NewWatchdogInterceptor(tracker, tempDir, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	workerID := "stalled-worker-01"
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	meta := WorkerMeta{
		Kind:           "daemon",
		Interval:       20 * time.Millisecond,
		StaleThreshold: 50 * time.Millisecond,
		CancelFunc:     workerCancel,
	}

	if err := tracker.Register(workerID, meta); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	var notified atomic.Bool
	interceptor.OnStalledWorker = func(status WorkerStatus) {
		if status.ID == workerID {
			notified.Store(true)
		}
	}

	// Run interceptor in background
	goroutinelabels.NewGoroutine("test.watchdog_interceptor", "interceptor run loop").StartSimple(func() {
		interceptor.Start(ctx)
	})

	// Wait for worker context to be cancelled by the watchdog
	select {
	case <-workerCtx.Done():
		// Context was cancelled by watchdog as expected!
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timeout: worker context was not cancelled by watchdog")
	}

	if !notified.Load() {
		t.Fatalf("expected OnStalledWorker callback to be fired")
	}

	// Check worker status is marked stalled
	status, found := tracker.GetWorker(workerID)
	if !found {
		t.Fatalf("expected worker to be found")
	}
	if status.State != WorkerStateStalled {
		t.Fatalf("expected WorkerStateStalled, got %s", status.State)
	}
}

func TestWatchdogInterceptor_LockDrainOnStall(t *testing.T) {
	tracker := NewHeartbeatTracker()
	tempDir := t.TempDir()

	// Create a dummy stale lock file in tempDir
	lockDir := filepath.Join(tempDir, ".zqk", "locks")
	if err := os.MkdirAll(lockDir, 0750); err != nil {
		t.Fatalf("failed to create lockdir: %v", err)
	}
	staleLockFile := filepath.Join(lockDir, "test_orphan.lock")
	if err := os.WriteFile(staleLockFile, []byte("stale"), 0600); err != nil {
		t.Fatalf("failed to create stale lock file: %v", err)
	}
	// Backdate the lock file to be older than threshold
	oldTime := time.Now().Add(-10 * time.Minute)
	_ = os.Chtimes(staleLockFile, oldTime, oldTime)

	interceptor := NewWatchdogInterceptor(tracker, tempDir, 50*time.Millisecond)
	interceptor.LockSweepThreshold = 1 * time.Minute

	// Trigger manual sweep
	interceptor.SweepAndRecover(context.Background())

	// Stale lock file should be drained/removed by lockhealth
	if _, err := os.Stat(staleLockFile); !os.IsNotExist(err) {
		t.Fatalf("expected stale lock file to be deleted, got err: %v", err)
	}
}
