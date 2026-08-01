package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

// TestHashRegistry_Save_NestedLockDeadlock reproduces the nested lock deadlock
// that was fixed in HashRegistry.Save(). This test would hang with the old implementation
// where Save() held RLock while trying to acquire Lock.
//
// The issue:
// - Multiple goroutines call Save() concurrently
// - Save() acquires RLock, does file I/O (blocking), then tries to acquire Lock
// - If multiple goroutines hold RLock, Lock() waits for all RLock holders
// - But RLock holders are blocked on file.Sync(), creating a deadlock
func TestHashRegistry_Save_NestedLockDeadlock(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping deadlock test in short mode")
	}

	tmpDir := t.TempDir()
	kindDir := filepath.Join(tmpDir, "test_kind")
	if err := os.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create kind directory: %v", err)
	}

	// Create a single registry that all goroutines will use
	// This simulates the real scenario where all objects of the same kind
	// share a hash registry
	registry := NewHashRegistry(pkgctx.NewSystemContext(), "test_kind", kindDir)

	// Load the registry to initialize it
	if err := registry.Load(); err != nil {
		// Registry file doesn't exist yet - that's OK
		t.Logf("Registry file doesn't exist yet (expected): %v", err)
	}

	// Set some initial hashes
	registry.SetHash("file1.yaml", "hash1")
	registry.SetHash("file2.yaml", "hash2")

	// Start many goroutines that all call Save() concurrently
	// This reproduces the scenario where multiple validation goroutines
	// are saving the same registry
	numGoroutines := 50
	var wg sync.WaitGroup
	var saveCount atomic.Int64
	var errorCount atomic.Int64

	// Start goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("storage_test", "nested lock deadlock test").StartSimple(func() {
			func(id int) {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					// Add a new hash
					filename := filepath.Join(kindDir, fmt.Sprintf("file-%d-%d.yaml", id, j))
					registry.SetHash(filename, fmt.Sprintf("hash-%d-%d", id, j))

					// Call Save() - this is where the deadlock would occur
					// With the old implementation:
					// - Save() acquires RLock
					// - Does file I/O (file.Sync() blocks)
					// - Tries to acquire Lock (waits for all RLock holders)
					// - But RLock holders are blocked on file.Sync() - DEADLOCK!
					if err := registry.Save(); err != nil {
						errorCount.Add(1)
						t.Logf("Save() error (may be transient): %v", err)
					} else {
						saveCount.Add(1)
					}

					// Small delay to increase contention
					time.Sleep(1 * time.Millisecond)
				}
			}(i)
		})
	}

	// Wait for all goroutines with timeout
	// If there's a deadlock, this will timeout
	timeout := time.After(30 * time.Second)
	completed := make(chan struct{})

	goroutinelabels.StartTestGoroutine("test_wait_collector", "waiting for test goroutines to complete", func() {
		wg.Wait()
		close(completed)
	})

	select {
	case <-completed:
		t.Logf("✅ All goroutines completed successfully")
		t.Logf("   Successful saves: %d", saveCount.Load())
		t.Logf("   Errors: %d", errorCount.Load())
	case <-timeout:
		t.Fatal("❌ DEADLOCK DETECTED: Test hung - goroutines did not complete within 30 seconds. " +
			"This indicates the nested lock deadlock in HashRegistry.Save()")
	}
}

// TestHashRegistry_Save_ConcurrentAccess tests that concurrent Save() calls
// don't cause data corruption or hangs, even under high contention
func TestHashRegistry_Save_ConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent access test in short mode")
	}

	tmpDir := t.TempDir()
	kindDir := filepath.Join(tmpDir, "test_kind")
	if err := os.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create kind directory: %v", err)
	}

	registry := NewHashRegistry(pkgctx.NewSystemContext(), "test_kind", kindDir)

	// Load the registry
	if err := registry.Load(); err != nil {
		t.Logf("Registry file doesn't exist yet (expected): %v", err)
	}

	// Concurrently add hashes and save
	numGoroutines := 20
	var wg sync.WaitGroup
	var saveCount atomic.Int64
	done := make(chan struct{})

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("storage_test", "concurrent access test").StartSimple(func() {
			func(id int) {
				defer wg.Done()
				for j := 0; j < 5; j++ {
					filename := fmt.Sprintf("file-%d-%d.yaml", id, j)
					hash := fmt.Sprintf("hash-%d-%d", id, j)

					// Set hash and save
					registry.SetHash(filename, hash)
					if err := registry.Save(); err != nil {
						t.Logf("Save() error: %v", err)
					} else {
						saveCount.Add(1)
					}
				}
			}(i)
		})
	}

	// Wait with timeout
	timeout := time.After(10 * time.Second)
	goroutinelabels.NewGoroutine("storage_test", "wait for completion").StartSimple(func() {
		wg.Wait()
		close(done)
	})

	select {
	case <-done:
		t.Logf("✅ Concurrent saves completed: %d successful", saveCount.Load())
	case <-timeout:
		t.Fatal("❌ Test hung - concurrent Save() calls caused deadlock")
	}

	// Verify registry was saved correctly
	if err := registry.Load(); err != nil {
		t.Fatalf("Failed to reload registry: %v", err)
	}

	// Check that some hashes were saved
	allHashes := registry.GetAllHashes()
	if len(allHashes) == 0 {
		t.Error("No hashes were saved")
	} else {
		t.Logf("✅ Registry contains %d hashes after concurrent saves", len(allHashes))
	}
}

// TestHashRegistry_SaveWorker_TimerDrainDeadlock reproduces the timer-drain deadlock
// in the hash registry save worker. Before the fix the worker would deadlock whenever:
//  1. The case <-batchTimer.C select branch ran with an empty batch (batch len == 0).
//  2. The branch consumed the timer value but did NOT reset the timer (the Reset call
//     was gated on len(batch) > 0).
//  3. On the next save request the worker called batchTimer.Stop() → returned false
//     (timer already expired) → tried to drain <-batchTimer.C → channel was already
//     empty → goroutine blocked forever.
//
// With the fix:
//   - All blocking <-timer.C drains are replaced with non-blocking select{case<-C:default:}.
//   - batchTimer.Reset is unconditional after consuming from batchTimer.C.
func TestHashRegistry_SaveWorker_TimerDrainDeadlock(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timer-drain deadlock test in short mode")
	}

	tmpDir := t.TempDir()
	kindDir := filepath.Join(tmpDir, "test_kind")
	if err := os.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create kind directory: %v", err)
	}

	registry := NewHashRegistry(pkgctx.NewSystemContext(), "test_kind", kindDir)

	// Step 1: Let the batch timer fire with an empty batch.
	// The timer fires every saveBatchTimeout (≤100ms). Sleep long enough so the
	// case <-batchTimer.C branch runs at least once with len(batch)==0.
	time.Sleep(300 * time.Millisecond)

	// Step 2: Now issue a save. With the old code the worker would block here
	// indefinitely trying to drain an already-empty batchTimer.C.
	done := make(chan error, 1)
	goroutinelabels.StartTestGoroutine("test_save", "issue save after timer fired", func() {
		registry.SetHash("file1.yaml", "deadlock-regression-hash")
		done <- registry.Save()
	})

	select {
	case err := <-done:
		if err != nil {
			t.Logf("Save returned error (acceptable): %v", err)
		}
		// SUCCESS: save completed without hanging
	case <-time.After(10 * time.Second):
		t.Fatal("DEADLOCK: Save() did not complete within 10s; timer-drain deadlock regression")
	}
}

// TestHashRegistry_Load_ConcurrentAccess tests that concurrent Load() calls
// don't cause deadlocks, even when multiple goroutines are loading the same registry
func TestHashRegistry_Load_ConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent load test in short mode")
	}

	tmpDir := t.TempDir()
	kindDir := filepath.Join(tmpDir, "test_kind")
	if err := os.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create kind directory: %v", err)
	}

	// Create registry file with some data
	registry := NewHashRegistry(pkgctx.NewSystemContext(), "test_kind", kindDir)
	registry.SetHash("file1.yaml", "hash1")
	registry.SetHash("file2.yaml", "hash2")
	if err := registry.Save(); err != nil {
		t.Fatalf("failed to save initial registry: %v", err)
	}

	// Create multiple registries that will all load from the same file
	// This simulates multiple validation goroutines loading the same registry
	numGoroutines := 50
	var wg sync.WaitGroup
	var loadCount atomic.Int64
	var errorCount atomic.Int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("storage_test", "concurrent load test").StartSimple(func() {
			func(_ int) {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					// Create a new registry instance (simulates different goroutines)
					reg := NewHashRegistry(pkgctx.NewSystemContext(), "test_kind", kindDir)

					// Call Load() - this was holding a lock during file I/O before the fix
					// With the old implementation, multiple goroutines would block on the lock
					// while doing os.ReadFile(), causing contention and potential deadlocks
					if err := reg.Load(); err != nil {
						errorCount.Add(1)
						t.Logf("Load() error (may be transient): %v", err)
					} else {
						loadCount.Add(1)
					}

					// Small delay to increase contention
					time.Sleep(1 * time.Millisecond)
				}
			}(i)
		})
	}

	// Wait for all goroutines with timeout
	timeout := time.After(30 * time.Second)
	completed := make(chan struct{})

	goroutinelabels.StartTestGoroutine("test_wait_collector", "waiting for test goroutines to complete", func() {
		wg.Wait()
		close(completed)
	})

	select {
	case <-completed:
		t.Logf("✅ All goroutines completed successfully")
		t.Logf("   Successful loads: %d", loadCount.Load())
		t.Logf("   Errors: %d", errorCount.Load())
	case <-timeout:
		t.Fatal("❌ DEADLOCK DETECTED: Test hung - goroutines did not complete within 30 seconds. " +
			"This indicates the lock+file I/O issue in HashRegistry.Load()")
	}
}
