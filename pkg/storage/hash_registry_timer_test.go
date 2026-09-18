package storage

import (
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// TestHashRegistry_TimerVsContext tests if context cancellation is seen when waiting on timer
func TestHashRegistry_TimerVsContext(t *testing.T) {
	tmpDir := t.TempDir()
	kindDir := tmpDir

	ctx := pkgctx.NewSystemContext()
	registry := NewHashRegistry(ctx, "test_kind", kindDir)

	// Start a save to trigger worker
	registry.SetHash("test.yaml", "hash123")
	_ = registry.Save() //nolint:errcheck // Test helper - error handling not needed

	// Wait for worker to be running and waiting on idle timer
	time.Sleep(150 * time.Millisecond) // Wait past batch timeout

	// Verify worker is running
	if registry.workerRunning.Load() == 0 {
		t.Fatal("Worker should be running")
	}

	// Cancel context while worker is waiting on idle timer (5 minutes)
	_ = registry.InitiateShutdown() //nolint:errcheck // Test helper - error handling not needed

	// Worker should exit immediately, not wait 5 minutes
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if registry.workerRunning.Load() == 0 {
			t.Log("Worker exited successfully within 500ms")
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("Worker did not exit within 500ms - it's waiting on the 5-minute idle timer instead of seeing context cancellation")
}
