package storage

import (
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// TestHashRegistry_WorkerLeak reproduces the worker leak issue
func TestHashRegistry_WorkerLeak(t *testing.T) {
	tmpDir := t.TempDir()
	kindDir := tmpDir

	// Create registry with system context
	ctx := pkgctx.NewSystemContext()
	registry := NewHashRegistry(ctx, "test_kind", kindDir)

	// Trigger a save to start the worker
	registry.SetHash("test.yaml", "hash123")
	if err := registry.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Wait a bit for worker to start
	time.Sleep(50 * time.Millisecond)

	// Initiate shutdown (simulates what happens in tests)
	if err := registry.InitiateShutdown(); err != nil {
		t.Fatalf("InitiateShutdown failed: %v", err)
	}

	// Wait for worker to exit
	// If worker doesn't exit, this test will leak
	time.Sleep(200 * time.Millisecond)

	// Verify worker is stopped
	if registry.workerRunning.Load() != 0 {
		t.Error("Worker should be stopped after InitiateShutdown")
	}
}

// TestHashRegistry_WorkerLeakAfterShutdown tests worker starting after shutdown
func TestHashRegistry_WorkerLeakAfterShutdown(t *testing.T) {
	tmpDir := t.TempDir()
	kindDir := tmpDir

	// Create registry
	ctx := pkgctx.NewSystemContext()
	registry := NewHashRegistry(ctx, "test_kind", kindDir)

	// Initiate shutdown FIRST
	if err := registry.InitiateShutdown(); err != nil {
		t.Fatalf("InitiateShutdown failed: %v", err)
	}

	// Try to save AFTER shutdown (should fail, not start worker)
	registry.SetHash("test.yaml", "hash123")
	err := registry.Save()
	if err == nil {
		t.Error("Save should fail after shutdown")
	}

	// Verify no worker started
	time.Sleep(50 * time.Millisecond)
	if registry.workerRunning.Load() != 0 {
		t.Error("Worker should not start after shutdown")
	}
}

// TestHashRegistry_WorkerLeakParallel tests parallel execution
func TestHashRegistry_WorkerLeakParallel(t *testing.T) {

	tmpDir := t.TempDir()
	kindDir := tmpDir

	ctx := pkgctx.NewSystemContext()
	registry := NewHashRegistry(ctx, "test_kind", kindDir)

	// Simulate parallel saves
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		goroutinelabels.NewGoroutine("storage_test", "parallel save during leak test").StartSimple(func() {
			func(id int) {
				registry.SetHash("test.yaml", "hash123")
				done <- registry.Save()
			}(i)
		})
	}

	// Wait for some saves to complete
	time.Sleep(50 * time.Millisecond)

	// Initiate shutdown while saves might still be in progress
	if err := registry.InitiateShutdown(); err != nil {
		t.Fatalf("InitiateShutdown failed: %v", err)
	}

	// Wait for all saves to complete or fail
	for i := 0; i < 10; i++ {
		select {
		case err := <-done:
			if err != nil && err.Error() != "hash registry context cancelled, cannot save" {
				t.Logf("Save error (expected): %v", err)
			}
		case <-time.After(1 * time.Second):
			t.Error("Save did not complete or fail within timeout")
		}
	}

	// Wait for worker to exit
	time.Sleep(200 * time.Millisecond)

	// Verify worker is stopped
	if registry.workerRunning.Load() != 0 {
		t.Error("Worker should be stopped after InitiateShutdown")
	}
}
