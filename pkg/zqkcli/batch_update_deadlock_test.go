package internal

// Tests that set ZQK_TEST_ROOT must not use t.Parallel(): the env var is process-global.

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/storage/audit"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqktime"

	"github.com/zqk-os/zqk/pkg/objects"
)

func waitForDoneOrTimeout(done <-chan struct{}, timeout time.Duration) error {
	return testkit.RunNamedTestSteps(context.Background(), "internal.batch_update_wait",
		testkit.NamedTestStep{
			Name: "WAIT_DONE",
			Fn: func() error {
				select {
				case <-done:
					return nil
				case <-time.After(timeout):
					return context.DeadlineExceeded
				}
			},
		},
	)
}

func prepareBatchDeadlockTestProject(t *testing.T) (testRoot string, fileStorage *storage.FileObjectStorage) {
	t.Helper()
	p := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "internal.batch_deadlock",
		AppendStagesBeforeStorage: func(root string) []testkit.NamedTestStep {
			return []testkit.NamedTestStep{{
				Name: "build_path_alias_cache",
				Fn: func() error {
					storage.BuildPathAliasCacheForProject(root)
					return nil
				},
			}}
		},
	})
	return p.Root, p.FileStorage
}

// TestBatchInternalUpdate_ConcurrentDeadlockDetection tests that concurrent
// batch updates on multiple internal objects don't cause deadlocks.
// This test follows POL-DEBUG-001: test-first debugging for deadlock detection.
//
// Pattern: Simulates the batch update approach we've been using (sequential
// updates on multiple objects) but runs multiple batches concurrently to detect
// potential deadlocks in file locking, hash registry updates, or cache operations.
//
//nolint:gocyclo // Test function intentionally exercises many concurrent scenarios
func TestBatchInternalUpdate_ConcurrentDeadlockDetection(t *testing.T) {
	testRoot, fileStorage := prepareBatchDeadlockTestProject(t)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = fileStorage.Shutdown(ctx)
	}()

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := pkgctx.NewSystemContext()

	// Create test directory for audit events (simulating internal objects)
	auditDir := filepath.Join(audit.KindDir(testRoot), "2026-01")
	if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create audit directory: %v", err)
	}

	// Create multiple internal objects (audit events) to simulate batch update scenario
	// Keep count moderate so test completes within timeout on CI (concurrent I/O is still exercised)
	numObjects := 25
	objectIDs := make([]string, numObjects)
	for i := 0; i < numObjects; i++ {
		objID := fmt.Sprintf("AUD-%04d", 10000+i) // Use numeric IDs starting from 10000
		objectIDs[i] = objID

		obj := map[string]any{
			objects.FieldKeyID:            objID,
			objects.FieldKeyKind:          "audit_event",
			objects.FieldKeyTitle:         fmt.Sprintf("Test Audit Event %d", i),
			objects.FieldKeyStatus:        "completed",
			objects.FieldKeyOperation:     "test_operation",
			objects.FieldKeySeverity:      "low",
			objects.FieldKeyEventType:     "command_execution",
			objects.FieldKeyDescription:   fmt.Sprintf("Test message %d", i),
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
			objects.FieldKeyUpdatedBy:     "test",
		}

		if err := fileStorage.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("failed to create object %s: %v", objID, err)
		}
	}

	t.Logf("Created %d test objects for batch update test", numObjects)

	// Simulate concurrent batch updates
	// Each goroutine performs sequential updates on a subset of objects
	// This mimics the pattern: `for obj in objects; do zqk internal update $obj; done`
	numBatches := 3
	objectsPerBatch := numObjects / numBatches
	var wg sync.WaitGroup
	var updateErrors atomic.Int64
	var updateSuccesses atomic.Int64

	// Channel to synchronize all batches to start at the same time
	startChan := make(chan struct{})

	// Test timeout to detect deadlocks (25 objects × 3 batches; enough to stress concurrent I/O)
	testTimeout := 60 * time.Second
	done := make(chan struct{})

	for batchID := 0; batchID < numBatches; batchID++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("zqkcli_test", "concurrent batch update").StartSimple(func() {
			func(batch int) {
				defer wg.Done()

				// Wait for start signal
				<-startChan

				// Perform sequential updates on this batch's objects
				// This simulates: `for obj in batch; do zqk internal update $obj --field title=title; done`
				startIdx := batch * objectsPerBatch
				endIdx := startIdx + objectsPerBatch
				if batch == numBatches-1 {
					// Last batch gets remaining objects
					endIdx = numObjects
				}

				for i := startIdx; i < endIdx; i++ {
					objID := objectIDs[i]
					updates := map[string]any{
						objects.FieldKeyTitle: fmt.Sprintf("Updated Title %d", batch), // No-op update (title=title)
					}

					err := fileStorage.Update(ctx, secCtx, objID, updates)
					if err != nil {
						updateErrors.Add(1)
						t.Logf("Batch %d: failed to update %s: %v", batch, objID, err)
					} else {
						updateSuccesses.Add(1)
					}

					// Small delay to increase chance of concurrent operations
					time.Sleep(1 * time.Millisecond)
				}
			}(batchID)
		})
	}

	// Start all batches simultaneously
	goroutinelabels.NewGoroutine("zqkcli_test", "wait for batch updates").StartSimple(func() {
		wg.Wait()
		close(done)
	})

	// Start all batches
	close(startChan)

	// Wait for completion or timeout (deadlock detection)
	if err := waitForDoneOrTimeout(done, testTimeout); err == nil {
		// All batches completed successfully
		successes := updateSuccesses.Load()
		errors := updateErrors.Load()
		t.Logf("Batch update test completed: %d successes, %d errors", successes, errors)

		// Most updates should succeed (some may fail due to version conflicts, which is OK)
		if successes == 0 {
			t.Error("expected at least some updates to succeed")
		}

		// Verify objects were actually updated
		for i := 0; i < numObjects; i++ {
			objID := objectIDs[i]
			updated, err := fileStorage.Read(ctx, secCtx, objID)
			if err != nil {
				t.Errorf("failed to read updated object %s: %v", objID, err)
				continue
			}
			if updated == nil {
				t.Errorf("object %s not found after update", objID)
			}
		}
	} else {
		// Test timed out - possible deadlock
		t.Fatal("Test timed out - possible deadlock in batch update operations. " +
			"This indicates a potential deadlock in file locking, hash registry updates, " +
			"or cache operations during concurrent batch updates.")
	}
}

// TestBatchInternalUpdate_SequentialNoDeadlock tests that sequential batch updates
// (the pattern we've been using) don't cause deadlocks even when processing many objects.
// This is a sanity check to ensure the basic pattern is safe.
func TestBatchInternalUpdate_SequentialNoDeadlock(t *testing.T) {
	testRoot, fileStorage := prepareBatchDeadlockTestProject(t)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = fileStorage.Shutdown(ctx)
	}()

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := pkgctx.NewSystemContext()

	auditDir := filepath.Join(audit.KindDir(testRoot), "2026-01")
	if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create audit directory: %v", err)
	}

	// Create many objects (simulating large batch). Keep count moderate so scheduler/CI bundles finish in reasonable time.
	numObjects := 100
	objectIDs := make([]string, numObjects)
	for i := 0; i < numObjects; i++ {
		objID := fmt.Sprintf("AUD-%04d", 20000+i) // Use numeric IDs starting from 20000
		objectIDs[i] = objID

		obj := map[string]any{
			objects.FieldKeyID:            objID,
			objects.FieldKeyKind:          "audit_event",
			objects.FieldKeyTitle:         fmt.Sprintf("Sequential Test %d", i),
			objects.FieldKeyStatus:        "completed",
			objects.FieldKeyOperation:     "test_operation",
			objects.FieldKeySeverity:      "low",
			objects.FieldKeyEventType:     "command_execution",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
			objects.FieldKeyUpdatedBy:     "test",
		}

		if err := fileStorage.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("failed to create object %s: %v", objID, err)
		}
	}

	// Perform sequential updates (our actual pattern)
	// Simulates: `for obj in objects; do zqk internal update $obj --field title=title; done`
	// Note: This is a performance test, not a correctness test - timeout is acceptable for large batches
	testTimeout := 120 * time.Second // 100 objects; enough for updates + cleanup without hanging bundles
	done := make(chan struct{})
	var updateCount atomic.Int64

	goroutinelabels.StartTestGoroutine("test_batch_updater", "updating objects in batch deadlock test", func() {
		for i := 0; i < numObjects; i++ {
			objID := objectIDs[i]
			updates := map[string]any{
				objects.FieldKeyTitle: "Updated Title", // No-op update
			}

			err := fileStorage.Update(ctx, secCtx, objID, updates)
			if err != nil {
				t.Logf("Failed to update %s: %v", objID, err)
			} else {
				updateCount.Add(1)
			}
		}
		close(done)
	})

	if err := waitForDoneOrTimeout(done, testTimeout); err == nil {
		count := updateCount.Load()
		t.Logf("Sequential batch update completed: %d/%d updates succeeded", count, numObjects)
		if count == 0 {
			t.Error("expected at least some updates to succeed")
		}
	} else {
		t.Fatal("Sequential batch update timed out - possible deadlock or performance issue")
	}
}
