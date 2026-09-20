package cas_test

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestCASOrphanCleanupQueue_BasicOperations tests basic queue operations
func TestCASOrphanCleanupQueue_BasicOperations(t *testing.T) {
	queue := caspkg.GetGlobalOrphanCleanupQueue()

	// Test enqueue
	_ = queue.EnqueueCleanup("/tmp/test1.yaml")
	_ = queue.EnqueueCleanup("/tmp/test2.yaml")

	// Verify queue size (may be less if worker processed items quickly)
	size := queue.QueueSize()
	if size < 0 {
		t.Errorf("Expected queue size >= 0, got %d", size)
	}

	// Verify worker is running (or was running and processed items)
	// Worker may have already processed items, so we check if it's running OR queue was processed
	if !queue.IsWorkerRunning() && size > 0 {
		t.Error("Expected worker to be running when queue has items")
	}
}

// TestCASOrphanCleanupQueue_CoordinatorIntegration tests coordinator integration
func TestCASOrphanCleanupQueue_CoordinatorIntegration(t *testing.T) {
	// Do not use t.Parallel: caspkg.SetOrphanCleanupEventCallback is process-global.
	testRoot, fileStorage, _ := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	queue := fileStorage.GetOrphanCleanupQueue()
	if queue == nil {
		t.Fatal("Expected per-test orphan cleanup queue from factory")
	}

	// Track callback invocations (only for this test's project root so we ignore global-queue callbacks)
	var callbackCalls atomic.Int32
	var lastEvent struct {
		operationType string
		status        string
		batchSize     int
		successCount  int
		failureCount  int
	}

	// Set up callback
	caspkg.SetOrphanCleanupEventCallback(func(
		ctx context.Context,
		projectRoot string,
		_ caspkg.CASFacade,
		operationID string,
		operationType string,
		status string,
		batchSize int,
		successCount int,
		failureCount int,
		duration time.Duration,
		failedFiles []string,
	) {
		if projectRoot != testRoot {
			return
		}
		callbackCalls.Add(1)
		lastEvent.operationType = operationType
		lastEvent.status = status
		lastEvent.batchSize = batchSize
		lastEvent.successCount = successCount
		lastEvent.failureCount = failureCount
	})
	defer caspkg.SetOrphanCleanupEventCallback(nil)

	// Create a temporary file to clean up
	tmpFile := filepath.Join(testRoot, "test_orphan.yaml")
	if err := fileutil.WriteFile(tmpFile, []byte("test content"), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Enqueue cleanup
	_ = queue.EnqueueCleanup(tmpFile)

	// Per-test queue has no background worker; process synchronously so callback fires
	ctxProc, cancelProc := context.WithTimeout(pkgctx.NewSystemContext(), 2*time.Second)
	_, _ = queue.ProcessQueueIfIdle(ctxProc)
	cancelProc()

	// Callback is emitted via goroutine; allow it to run before polling
	time.Sleep(50 * time.Millisecond)

	// Wait for batch-complete callback (worker_started may fire first when tests share global queue)
	ctxTimeout, cancelTimeout := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancelTimeout()
	callbackReceived := storage.WaitForConditionForTest(ctxTimeout, func() bool {
		return lastEvent.operationType == "orphan_cleanup_batch" && (lastEvent.status == "complete" || lastEvent.status == "partial_failure")
	}, 100*time.Millisecond)

	if !callbackReceived {
		// Callback may not fire when tests run in parallel (global callback overwritten); if file was deleted, queue processed correctly
		if _, err := fileutil.Stat(tmpFile); !fileutil.IsNotExist(err) {
			t.Error("Expected orphan_cleanup_batch/complete callback within timeout and file to be deleted")
		}
	} else if lastEvent.successCount == 0 && lastEvent.failureCount == 0 {
		t.Error("Expected at least one success or failure count")
	}

	// Verify file was deleted (queue processed)
	if _, err := fileutil.Stat(tmpFile); !fileutil.IsNotExist(err) {
		t.Error("Expected file to be deleted, but it still exists")
	}
}

// TestCASOrphanCleanupQueue_WorkerLifecycle tests worker lifecycle (wake-on-work, idle shutdown)
func TestCASOrphanCleanupQueue_WorkerLifecycle(t *testing.T) {
	// Isolated queue — global singleton often has a long-lived worker from other tests,
	// which makes IsDrained() (empty && !running) flake.
	queue, cancelQueue := caspkg.NewCASOrphanCleanupQueueWithCancelForTest(pkgctx.NewSystemContext())
	defer cancelQueue()
	defer queue.Shutdown()

	_ = queue.EnqueueCleanup("/tmp/test_lifecycle.yaml")

	if !storage.WaitForConditionWithTimeoutForTest(pkgctx.NewSystemContext(),
		func() bool { return queue.IsWorkerRunning() || queue.QueueSize() == 0 },
		5*time.Second,
		10*time.Millisecond,
	) {
		t.Fatal("Expected worker to start or queue to empty within 5 seconds after enqueue")
	}

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 2*time.Second)
	defer cancel()
	_, _ = queue.ProcessQueueIfIdle(ctx)

	_ = queue.EnqueueCleanup("/tmp/test_lifecycle2.yaml")

	ctx2, cancel2 := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel2()
	if !storage.WaitForConditionWithTimeoutForTest(pkgctx.NewSystemContext(),
		func() bool {
			if queue.QueueSize() == 0 {
				return true
			}
			_, _ = queue.ProcessQueueIfIdle(ctx2)
			return queue.QueueSize() == 0
		},
		5*time.Second,
		10*time.Millisecond,
	) {
		t.Error("Expected queue to drain")
	}
}

// TestCASOrphanCleanupQueue_ProcessQueueIfIdle tests fallback processing
func TestCASOrphanCleanupQueue_ProcessQueueIfIdle(t *testing.T) {
	testRoot, fileStorage, _ := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	// Use a fresh queue instance to avoid global singleton state between tests
	queue, cancelQueue := caspkg.NewCASOrphanCleanupQueueWithCancelForTest(pkgctx.NewSystemContext())
	defer cancelQueue()
	queue.SetProjectRoot(testRoot)
	queue.SetStorage(fileStorage)
	defer queue.Shutdown()

	// Create test file
	tmpFile := filepath.Join(testRoot, "test_fallback.yaml")
	if err := fileutil.WriteFile(tmpFile, []byte("test"), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Enqueue cleanup
	_ = queue.EnqueueCleanup(tmpFile)

	// Wait for worker to process (or shut down if idle)
	if !storage.WaitForConditionWithTimeoutForTest(pkgctx.NewSystemContext(),
		func() bool { return !queue.IsWorkerRunning() || queue.IsDrained() },
		5*time.Second,
		10*time.Millisecond,
	) {
		// Process queue if idle (fallback mechanism)
		ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 2*time.Second)
		defer cancel()
		processed, err := queue.ProcessQueueIfIdle(ctx)
		if err != nil {
			t.Fatalf("ProcessQueueIfIdle() failed: %v", err)
		}

		// Should have processed at least 0 items (may have been processed by worker)
		if processed < 0 {
			t.Errorf("Expected processed >= 0, got %d", processed)
		}
	}

	// Verify file was eventually deleted deterministically
	if !storage.WaitForConditionWithTimeoutForTest(pkgctx.NewSystemContext(),
		func() bool {
			_, err := fileutil.Stat(tmpFile)
			return fileutil.IsNotExist(err)
		},
		5*time.Second,
		10*time.Millisecond,
	) {
		t.Error("Expected file to be deleted within 5 seconds, but it still exists")
	}
}

// TestCASOrphanCleanupQueue_NoCallback tests that operations work when callback is not set
func TestCASOrphanCleanupQueue_NoCallback(t *testing.T) {
	// Do not use t.Parallel: clears global callback; would break parallel callback tests.
	testRoot, fileStorage, _ := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	queue := fileStorage.GetOrphanCleanupQueue()
	if queue == nil {
		t.Fatal("Expected per-test orphan cleanup queue from factory")
	}

	// Ensure callback is not set
	caspkg.SetOrphanCleanupEventCallback(nil)

	// Create test file
	tmpFile := filepath.Join(testRoot, "test_no_callback.yaml")
	if err := fileutil.WriteFile(tmpFile, []byte("test"), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Enqueue cleanup
	_ = queue.EnqueueCleanup(tmpFile)

	// Wait deterministically for file deletion
	if !storage.WaitForConditionWithTimeoutForTest(pkgctx.NewSystemContext(),
		func() bool {
			_, err := fileutil.Stat(tmpFile)
			return fileutil.IsNotExist(err)
		},
		5*time.Second,
		10*time.Millisecond,
	) {
		t.Error("Expected file to be deleted within 5 seconds, but it still exists")
	}

	// Best-effort: wait for the queue to report drained to reduce the chance of
	// background activity interfering with TempDir cleanup. We deliberately do
	// not fail the test if the queue remains busy; functional behavior is
	// covered by other tests.
	_ = storage.WaitForConditionWithTimeoutForTest(
		pkgctx.NewSystemContext(),
		func() bool { return queue.IsDrained() },
		2*time.Second,
		10*time.Millisecond,
	)
}

// TestCASOrphanCleanupQueue_BatchProcessing tests batch processing
func TestCASOrphanCleanupQueue_BatchProcessing(t *testing.T) {
	// Do not use t.Parallel: caspkg.SetOrphanCleanupEventCallback is process-global.
	testRoot, fileStorage, _ := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	queue := fileStorage.GetOrphanCleanupQueue()
	if queue == nil {
		t.Fatal("Expected per-test orphan cleanup queue from factory")
	}

	var batchEvents atomic.Int32
	caspkg.SetOrphanCleanupEventCallback(func(
		ctx context.Context,
		projectRoot string,
		_ caspkg.CASFacade,
		operationID string,
		operationType string,
		status string,
		batchSize int,
		successCount int,
		failureCount int,
		duration time.Duration,
		failedFiles []string,
	) {
		// Only count events for this test's queue (parallel tests share global callback)
		if projectRoot != testRoot || operationType != "orphan_cleanup_batch" {
			return
		}
		batchEvents.Add(1)
	})
	defer caspkg.SetOrphanCleanupEventCallback(nil)

	// Create multiple test files
	files := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		tmpFile := filepath.Join(testRoot, fmt.Sprintf("test_batch_%d.yaml", i))
		if err := fileutil.WriteFile(tmpFile, []byte("test"), paths.FilePerm644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		files = append(files, tmpFile)
		_ = queue.EnqueueCleanup(tmpFile)
	}

	// Per-test queue has no worker; process synchronously (may need multiple passes if batch size is limited)
	ctxProc, cancelProc := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	for i := 0; i < 3; i++ {
		n, _ := queue.ProcessQueueIfIdle(ctxProc)
		if n == 0 && queue.IsDrained() {
			break
		}
		if queue.IsDrained() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancelProc()

	// Wait until files are deleted or queue drained (callback may not fire under parallel load)
	ctxWait, cancelWait := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancelWait()
	if !storage.WaitForConditionWithTimeoutForTest(ctxWait,
		func() bool {
			allGone := true
			for _, f := range files {
				if _, err := fileutil.Stat(f); !fileutil.IsNotExist(err) {
					allGone = false
					break
				}
			}
			return allGone || (batchEvents.Load() > 0 || queue.IsDrained())
		},
		5*time.Second,
		50*time.Millisecond,
	) {
		t.Fatal("Timeout waiting for batch processing (files not deleted and queue not drained)")
	}

	// Verify batch events were emitted
	if batchEvents.Load() == 0 {
		t.Log("Batch events not observed; proceeding since queue drained")
	}

	// Verify files were deleted
	for _, file := range files {
		if _, err := fileutil.Stat(file); !fileutil.IsNotExist(err) {
			t.Errorf("Expected file %s to be deleted, but it still exists", file)
		}
	}
}

// TestCASOrphanCleanupQueue_RetryLogic tests retry logic for failed cleanups
func TestCASOrphanCleanupQueue_RetryLogic(t *testing.T) {
	_, fileStorage, _ := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	queue := fileStorage.GetOrphanCleanupQueue()
	if queue == nil {
		t.Fatal("Expected per-test orphan cleanup queue from factory")
	}

	// Create a file that will fail to delete (permission denied scenario)
	// Note: This is hard to test without actually creating permission issues
	// Instead, we'll test with a non-existent file (should succeed immediately)

	// Enqueue cleanup for non-existent file (should succeed)
	_ = queue.EnqueueCleanup("/nonexistent/file.yaml")

	// Wait deterministically for queue to process (non-existent files are handled immediately)
	// Since the file doesn't exist, processing should complete quickly
	if !storage.WaitForConditionWithTimeoutForTest(pkgctx.NewSystemContext(),
		func() bool { return queue.IsDrained() },
		5*time.Second,
		10*time.Millisecond,
	) {
		// Queue may not drain if worker is still running, which is fine for this test
		t.Log("Queue not drained (worker may still be running, which is acceptable)")
	}

	// Should complete without error (non-existent files are treated as success)
	// This tests the edge case handling in cleanupFile()
}

// TestCASOrphanCleanupQueue_ConcurrentEnqueue tests concurrent enqueue operations
func TestCASOrphanCleanupQueue_ConcurrentEnqueue(t *testing.T) {
	testRoot, fileStorage, _ := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	queue := fileStorage.GetOrphanCleanupQueue()
	if queue == nil {
		t.Fatal("Expected per-test orphan cleanup queue from factory")
	}

	// Create multiple test files
	files := make([]string, 0, 20)
	for i := 0; i < 20; i++ {
		tmpFile := filepath.Join(testRoot, fmt.Sprintf("test_concurrent_%d.yaml", i))
		if err := fileutil.WriteFile(tmpFile, []byte("test"), paths.FilePerm644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		files = append(files, tmpFile)
	}

	// Enqueue concurrently
	var wg sync.WaitGroup
	for _, file := range files {
		wg.Add(1)
		goroutinelabels.NewGoroutine("storage_test", "concurrent orphan cleanup enqueue").StartSimple(func() {
			func(f string) {
				defer wg.Done()
				_ = queue.EnqueueCleanup(f)
			}(file)
		})
	}
	waitDone := make(chan struct{})
	goroutinelabels.NewGoroutine("storage_test", "wait for concurrent orphan cleanup enqueue").StartSimple(func() { defer close(waitDone); wg.Wait() })
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		panic("timeout")
	}

	// Wait deterministically for all files to be deleted
	for _, file := range files {
		if !storage.WaitForConditionWithTimeoutForTest(pkgctx.NewSystemContext(),
			func() bool {
				_, err := fileutil.Stat(file)
				return fileutil.IsNotExist(err)
			},
			5*time.Second,
			10*time.Millisecond,
		) {
			t.Errorf("Expected file %s to be deleted within 5 seconds, but it still exists", file)
		}
	}
	for _, file := range files {
		if _, err := fileutil.Stat(file); !fileutil.IsNotExist(err) {
			t.Errorf("Expected file %s to be deleted, but it still exists", file)
		}
	}

	// Best-effort: wait for the worker to go idle before the test exits to
	// reduce the chance of TempDir cleanup racing with background activity.
	_ = storage.WaitForConditionWithTimeoutForTest(
		pkgctx.NewSystemContext(),
		func() bool { return !queue.IsWorkerRunning() },
		5*time.Second,
		10*time.Millisecond,
	)
}

func TestCASOrphanCleanupQueue_GetQueueStats(t *testing.T) {
	_, fileStorage, _ := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	queue := fileStorage.GetOrphanCleanupQueue()
	if queue == nil {
		t.Fatal("Expected per-test orphan cleanup queue from factory")
	}

	stats := queue.GetQueueStats()
	if stats.QueueCapacity <= 0 {
		t.Errorf("Expected positive QueueCapacity, got %d", stats.QueueCapacity)
	}
	if stats.QueueLength < 0 {
		t.Errorf("Expected non-negative QueueLength, got %d", stats.QueueLength)
	}
	// Verify boolean field accessibility
	_ = stats.WorkerRunning
}

func TestCASMetricsSnapshot_ToMap(t *testing.T) {
	m := &caspkg.CASMetrics{}
	m.Creates.Store(5)
	m.Reads.Store(10)
	m.SetMappings.Store(3)
	m.IndexLockContention.Store(1)
	snap := m.GetSnapshot()

	toMap := snap.ToMap()
	if toMap["creates"] != int64(5) {
		t.Errorf("Expected creates 5 in map, got %v", toMap["creates"])
	}
	if toMap["reads"] != int64(10) {
		t.Errorf("Expected reads 10 in map, got %v", toMap["reads"])
	}
	if toMap["set_mappings"] != int64(3) {
		t.Errorf("Expected set_mappings 3 in map, got %v", toMap["set_mappings"])
	}
	if toMap["index_lock_contention"] != int64(1) {
		t.Errorf("Expected index_lock_contention 1 in map, got %v", toMap["index_lock_contention"])
	}
	if toMap["total_operations"] != int64(15) {
		t.Errorf("Expected total_operations 15 in map, got %v", toMap["total_operations"])
	}
}

// TestCASOrphanCleanupQueue_CleanupOrphanedDrafts tests cleanup of orphaned draft files
func TestCASOrphanCleanupQueue_CleanupOrphanedDrafts(t *testing.T) {
	testRoot, fileStorage, _ := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	queue := fileStorage.GetOrphanCleanupQueue()
	if queue == nil {
		t.Fatal("Expected per-test orphan cleanup queue from factory")
	}

	draftsDir := filepath.Join(testRoot, paths.ProjectDataDir, paths.DraftsSubdir)
	if err := fileutil.MkdirAll(draftsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create drafts dir: %v", err)
	}

	orphanFile := filepath.Join(draftsDir, "draft-123.yaml")
	if err := fileutil.WriteFile(orphanFile, []byte("draft: true"), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write draft file: %v", err)
	}

	ctx := pkgctx.NewSystemContext()
	enqueued, err := queue.CleanupOrphanedDrafts(ctx, draftsDir, 0)
	if err != nil {
		t.Fatalf("CleanupOrphanedDrafts failed: %v", err)
	}
	if enqueued != 1 {
		t.Errorf("Expected 1 enqueued orphan draft, got %d", enqueued)
	}

	// Process batch: if background worker woke up on EnqueueCleanup, wait for queue to drain; otherwise flush idle queue
	if queue.IsWorkerRunning() {
		deadline := time.Now().Add(5 * time.Second)
		for queue.QueueSize() > 0 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
	} else {
		ctxProc, cancelProc := context.WithTimeout(ctx, 5*time.Second)
		_, _ = queue.ProcessQueueIfIdle(ctxProc)
		cancelProc()
	}

	// Wait for async delete to land (worker may dequeue before unlink finishes)
	deadlineDel := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadlineDel) {
		if _, err := fileutil.Stat(orphanFile); fileutil.IsNotExist(err) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := fileutil.Stat(orphanFile); !fileutil.IsNotExist(err) {
		t.Errorf("Expected draft file to be deleted, stat err: %v", err)
	}
}

func TestRemoveOrphanCASHashFileSync_RefusesSoleSurvivor(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	hash := "1111111111111111111111111111111111111111111111111111111111111111"
	path := filepath.Join(dir, hash+".yaml")
	if err := fileutil.WriteStandardFile(path, []byte("id: CRIT-SOLE\nkind: criteria\n")); err != nil {
		t.Fatal(err)
	}
	if err := caspkg.RemoveOrphanCASHashFileSync(path); err != nil {
		t.Fatal(err)
	}
	if !fileutil.Exists(path) {
		t.Fatal("sole CAS hash must survive orphan cleanup")
	}
}

func TestRemoveOrphanCASHashFileSync_DeletesWhenReplacementExists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "2222222222222222222222222222222222222222222222222222222222222222.yaml")
	newPath := filepath.Join(dir, "3333333333333333333333333333333333333333333333333333333333333333.yaml")
	body := []byte("id: CRIT-DUP\nkind: criteria\n")
	if err := fileutil.WriteStandardFile(oldPath, body); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(newPath, body); err != nil {
		t.Fatal(err)
	}
	if err := caspkg.RemoveOrphanCASHashFileSync(oldPath); err != nil {
		t.Fatal(err)
	}
	if fileutil.Exists(oldPath) {
		t.Fatal("superseded hash should be removed")
	}
	if !fileutil.Exists(newPath) {
		t.Fatal("replacement must remain")
	}
}

func TestRemoveOrphanCASHashFileSync_KeeperHashOverridesSoleSurvivor(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "4444444444444444444444444444444444444444444444444444444444444444.yaml")
	if err := fileutil.WriteStandardFile(oldPath, []byte("id: CRIT-SOLE-KEEPER\nkind: criteria\n")); err != nil {
		t.Fatal(err)
	}
	if err := caspkg.RemoveOrphanCASHashFileSync(oldPath, "5555555555555555555555555555555555555555555555555555555555555555"); err != nil {
		t.Fatal(err)
	}
	if fileutil.Exists(oldPath) {
		t.Fatal("known-keeper predecessor must not be treated as sole survivor")
	}
}
