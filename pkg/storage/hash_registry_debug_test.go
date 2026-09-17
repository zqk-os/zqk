package storage

import (
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

// TestHashRegistry_ContextCancellationTiming tests the exact timing of context cancellation
func TestHashRegistry_ContextCancellationTiming(t *testing.T) {
	tmpDir := t.TempDir()
	kindDir := tmpDir

	ctx := pkgctx.NewSystemContext()
	registry := NewHashRegistry(ctx, "test_kind", kindDir)

	// Start a save to trigger worker
	registry.SetHash("test.yaml", "hash123")

	// Start save in goroutine so we can cancel while it's running
	saveDone := make(chan error, 1)
	goroutinelabels.NewGoroutine("storage_test", "background save during cancellation test").StartSimple(func() {
		saveDone <- registry.Save()
	})

	// Wait a tiny bit for worker to start
	time.Sleep(1 * time.Millisecond)

	// Verify worker is running
	if registry.workerRunning.Load() == 0 {
		t.Fatal("Worker should be running")
	}

	// Cancel context (simulates InitiateShutdown)
	_ = registry.InitiateShutdown() //nolint:errcheck // Test helper - error handling not needed

	// Wait for save to complete or fail
	select {
	case err := <-saveDone:
		if err == nil {
			t.Error("Save should fail when context is cancelled")
		}
		t.Logf("Save returned: %v", err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Save did not return within 500ms - worker is stuck!")
	}

	// Wait for worker to exit
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if registry.workerRunning.Load() == 0 {
			t.Log("Worker exited successfully")
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Error("Worker did not exit within 200ms after context cancellation")
}
