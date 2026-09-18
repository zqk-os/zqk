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
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
)

// TestCASOrphanCleanupQueue_CoordinatorIntegration_NonExistentFile tests cleanup of non-existent files
func TestCASOrphanCleanupQueue_CoordinatorIntegration_NonExistentFile(t *testing.T) {
	_, fos, _ := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	queue := fos.GetOrphanCleanupQueue()
	if queue == nil {
		t.Fatal("Expected per-test orphan cleanup queue from factory")
	}

	var callbackCalls atomic.Int32
	var successCount atomic.Int32
	callbackWaiter := storage.NewCallbackWaiterForTest()
	caspkg.SetOrphanCleanupEventCallback(func(
		ctx context.Context,
		projectRoot string,
		_ caspkg.CASFacade,
		operationID string,
		operationType string,
		status string,
		batchSize int,
		successCountArg int,
		failureCount int,
		duration time.Duration,
		failedFiles []string,
	) {
		callbackCalls.Add(1)
		if successCountArg > 0 {
			successCount.Add(int32(successCountArg)) //nolint:gosec // Test code, safe conversion
		}
		callbackWaiter.Invoke()
	})
	defer caspkg.SetOrphanCleanupEventCallback(nil)

	_ = queue.EnqueueCleanup("/nonexistent/file.yaml")

	if !callbackWaiter.WaitForInvocation(context.Background(), 5*time.Second) {
		t.Error("Expected callback to be called within 5 seconds")
	}

	if successCount.Load() == 0 {
		t.Log("Non-existent file cleanup may be counted as success or skipped")
	}
}

// TestCASOrphanCleanupQueue_CoordinatorIntegration_EmptyQueue lives in package cas
// (cas_orphan_cleanup_empty_queue_integration_test.go) so it uses an isolated queue — the global
// singleton is shared with parallel tests that enqueue work.

// TestCASOrphanCleanupQueue_CoordinatorIntegration_WorkerRunning tests when worker is already running
func TestCASOrphanCleanupQueue_CoordinatorIntegration_WorkerRunning(t *testing.T) {
	testRoot, fos, _ := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	queue := fos.GetOrphanCleanupQueue()
	if queue == nil {
		t.Fatal("Expected per-test orphan cleanup queue from factory")
	}

	tmpFile := filepath.Join(testRoot, "test_worker_running.yaml")
	if err := fileutil.WriteFile(tmpFile, []byte("test"), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_ = queue.EnqueueCleanup(tmpFile)

	if !storage.WaitForConditionWithTimeoutForTest(context.Background(),
		func() bool { return queue.IsWorkerRunning() },
		5*time.Second,
		10*time.Millisecond,
	) {
		t.Fatal("Expected worker to start within 5 seconds")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	processed, err := queue.ProcessQueueIfIdle(ctx)
	if err != nil {
		t.Fatalf("ProcessQueueIfIdle() failed: %v", err)
	}

	if processed != 0 {
		t.Errorf("Expected 0 processed items when worker is running, got %d", processed)
	}

	if !storage.WaitForConditionWithTimeoutForTest(context.Background(),
		func() bool {
			_, err := fileutil.Stat(tmpFile)
			return fileutil.IsNotExist(err)
		},
		5*time.Second,
		10*time.Millisecond,
	) {
		t.Log("File may still be processing")
	}

	_ = storage.WaitForConditionWithTimeoutForTest(
		context.Background(),
		func() bool { return !queue.IsWorkerRunning() },
		5*time.Second,
		10*time.Millisecond,
	)
}

// TestCASOrphanCleanupQueue_CoordinatorIntegration_LargeBatch tests large batch processing
func TestCASOrphanCleanupQueue_CoordinatorIntegration_LargeBatch(t *testing.T) {
	testRoot, fos, _ := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	queue := fos.GetOrphanCleanupQueue()
	if queue == nil {
		t.Fatal("Expected per-test orphan cleanup queue from factory")
	}

	var batchSizes []int
	var mu sync.Mutex

	batchWaiter := storage.NewCallbackWaiterForTest()
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
		if operationType != "orphan_cleanup_batch" || batchSize <= 0 {
			return
		}
		mu.Lock()
		batchSizes = append(batchSizes, batchSize)
		mu.Unlock()
		batchWaiter.Invoke()
	})
	defer caspkg.SetOrphanCleanupEventCallback(nil)

	for i := 0; i < 100; i++ {
		tmpFile := filepath.Join(testRoot, fmt.Sprintf("test_large_%d.yaml", i))
		if err := fileutil.WriteFile(tmpFile, []byte("test"), paths.FilePerm644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		_ = queue.EnqueueCleanup(tmpFile)
	}

	if !batchWaiter.WaitForInvocation(context.Background(), 10*time.Second) {
		t.Error("Expected batch events to be emitted within 10 seconds")
	}

	mu.Lock()
	if len(batchSizes) == 0 {
		t.Error("Expected batch events to be emitted")
	}
	limit := caspkg.CasOrphanCleanupBatchSizeForTest
	for _, size := range batchSizes {
		if size > limit {
			t.Errorf("Expected batch size <= %d, got %d", limit, size)
		}
	}
	mu.Unlock()
}

// TestCASOrphanCleanupQueue_CoordinatorIntegration_RetryLogic tests retry logic
func TestCASOrphanCleanupQueue_CoordinatorIntegration_RetryLogic(t *testing.T) {
	testRoot, fos, _ := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	queue := fos.GetOrphanCleanupQueue()
	if queue == nil {
		t.Fatal("Expected per-test orphan cleanup queue from factory")
	}

	var failureCounts []int
	var mu sync.Mutex

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
		if failureCount > 0 {
			mu.Lock()
			failureCounts = append(failureCounts, failureCount)
			mu.Unlock()
		}
	})
	defer caspkg.SetOrphanCleanupEventCallback(nil)

	tmpFile := filepath.Join(testRoot, "test_retry.yaml")
	if err := fileutil.WriteFile(tmpFile, []byte("test"), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_ = queue.EnqueueCleanup(tmpFile)

	if !storage.WaitForConditionWithTimeoutForTest(context.Background(),
		func() bool {
			_, err := fileutil.Stat(tmpFile)
			return fileutil.IsNotExist(err)
		},
		10*time.Second,
		10*time.Millisecond,
	) {
		t.Error("Expected file to be deleted after retries within 10 seconds")
	}
}
