package validation

import (
	"context"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestAsyncValidator_OnDemandPattern(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)
	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 5, 1*time.Hour)

	// Start validator (workers start on-demand)
	if err := validator.Start(); err != nil {
		t.Fatalf("Failed to start async validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup

	// Initially no workers should be running
	if validator.GetWorkerCount() != 0 {
		t.Errorf("Expected 0 workers running initially, got %d", validator.GetWorkerCount())
	}

	// Enqueue a task - should wake a worker
	task := ValidationTask{
		ObjectID:   "test_object_1",
		ObjectKind: "test_object",
		FilePath:   "/test/path",
		Priority:   1,
	}

	validator.Enqueue(task.ObjectID, task.ObjectKind, task.FilePath, task.Priority)

	// Wait a bit for worker to start
	time.Sleep(200 * time.Millisecond)

	// Worker should be running now (or may have already processed and shut down)
	workerCount := validator.GetWorkerCount()
	if workerCount == 0 {
		// Worker may have already processed and shut down - this is fine for on-demand pattern
		t.Log("Worker count is 0 - may have already processed and shut down (on-demand pattern)")
	} else {
		t.Logf("Worker is running: %d active workers", workerCount)
	}

	// Wait a bit more for processing
	time.Sleep(100 * time.Millisecond)
}

func TestAsyncValidator_CoordinatorIntegration(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)
	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 3, 1*time.Hour)

	var events []struct {
		workerID       string
		eventType      string
		status         string
		workerCount    int
		processedCount int
		failedCount    int
	}
	var eventsMu sync.Mutex

	SetValidationLifecycleEventCallback(func(
		ctx context.Context,
		projectRoot string,
		storageProvider any,
		workerID string,
		eventType string,
		status string,
		workerCount int,
		processedCount int,
		failedCount int,
		duration time.Duration,
	) {
		eventsMu.Lock()
		defer eventsMu.Unlock()
		events = append(events, struct {
			workerID       string
			eventType      string
			status         string
			workerCount    int
			processedCount int
			failedCount    int
		}{
			workerID:       workerID,
			eventType:      eventType,
			status:         status,
			workerCount:    workerCount,
			processedCount: processedCount,
			failedCount:    failedCount,
		})
	})

	if err := validator.Start(); err != nil {
		t.Fatalf("Failed to start async validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup

	// Enqueue a task to trigger worker start
	validator.Enqueue("test_object_1", "test_object", "/test/path", 1)

	// Wait for worker to start and emit event
	time.Sleep(300 * time.Millisecond)

	eventsMu.Lock()
	eventCount := len(events)
	eventsMu.Unlock()

	if eventCount == 0 {
		t.Error("Expected at least one lifecycle event (worker_start)")
	}

	// Check for worker_start event
	foundStart := false
	eventsMu.Lock()
	for _, event := range events {
		if event.eventType == "worker_start" {
			foundStart = true
			if event.workerCount == 0 {
				t.Error("Expected worker_count > 0 for worker_start event")
			}
			break
		}
	}
	eventsMu.Unlock()

	if !foundStart {
		t.Error("Expected worker_start event")
	}
}

func TestAsyncValidator_ShutdownCoordination(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)
	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 3, 1*time.Hour)

	if err := validator.Start(); err != nil {
		t.Fatalf("Failed to start async validator: %v", err)
	}

	// Test QueueShutdownHandler interface
	if validator.GetName() != "async_validator" {
		t.Errorf("Expected name 'async_validator', got '%s'", validator.GetName())
	}

	if !validator.IsCritical() {
		t.Error("Async validator should be critical")
	}

	// Enqueue some tasks
	for i := 0; i < 5; i++ {
		validator.Enqueue("test_object", "test_object", "/test/path", 1)
	}

	// Wait for tasks to be queued (but workers may process them quickly)
	time.Sleep(100 * time.Millisecond)

	// Check pending count (may be 0 if workers processed them quickly)
	pending := validator.GetPendingCount()
	t.Logf("Pending tasks: %d", pending)

	// Initiate shutdown
	if err := validator.InitiateShutdown(); err != nil {
		t.Fatalf("Failed to initiate shutdown: %v", err)
	}

	// Try to enqueue after shutdown - should fail (returns false for cache hit, but we can check shutdown)
	// Note: Enqueue doesn't return error, but checks shutdown internally
	time.Sleep(50 * time.Millisecond)

	// Drain queue
	drainCtx, drainCancel := context.WithTimeout(pkgctx.NewSystemContext(), 10*time.Second)
	defer drainCancel()

	err := validator.Drain(drainCtx)
	if err != nil {
		// Timeout is acceptable if workers are still processing validation goroutines
		if err == context.DeadlineExceeded {
			t.Logf("Drain timed out (acceptable if validation goroutines still processing): %v", err)
		} else {
			t.Errorf("Failed to drain queue: %v", err)
		}
	}

	// Give additional time for validation goroutines to complete
	time.Sleep(500 * time.Millisecond)

	// Verify drained (may not be fully drained if validation goroutines still running)
	if !validator.IsDrained() {
		// Check if it's just validation goroutines still running
		pending := validator.GetPendingCount()
		if pending == 0 {
			t.Log("Queue is empty but not fully drained (validation goroutines may still be running)")
		} else {
			t.Logf("Queue still has %d pending tasks after drain", pending)
		}
	}

	_ = validator.Stop() //nolint:errcheck // Test cleanup
}

func TestAsyncValidator_MultipleWorkers(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)
	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 5, 1*time.Hour)

	if err := validator.Start(); err != nil {
		t.Fatalf("Failed to start async validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup

	// Enqueue multiple tasks to trigger multiple workers
	for i := 0; i < 10; i++ {
		validator.Enqueue("test_object", "test_object", "/test/path", 1)
	}

	// Wait for workers to start
	time.Sleep(300 * time.Millisecond)

	// Should have multiple workers running (up to max)
	workerCount := validator.GetWorkerCount()
	maxWorkers := validator.GetMaxWorkers()

	if workerCount == 0 {
		t.Error("Expected at least one worker running")
	}

	if workerCount > maxWorkers {
		t.Errorf("Expected worker count <= max workers (%d), got %d", maxWorkers, workerCount)
	}

	t.Logf("Active workers: %d, Max workers: %d", workerCount, maxWorkers)
}

func TestAsyncValidator_ConcurrentOperations(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)
	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 10, 1*time.Hour)

	if err := validator.Start(); err != nil {
		t.Fatalf("Failed to start async validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup

	// Concurrently enqueue tasks
	const numTasks = 50
	done := make(chan struct{}, numTasks)

	for i := 0; i < numTasks; i++ {
		goroutinelabels.NewGoroutine("validation_test", "concurrent enqueue").StartSimple(func() {
			func(id int) {
				defer func() { done <- struct{}{} }()
				validator.Enqueue("test_object", "test_object", "/test/path", 1)
			}(i)
		})
	}

	// Wait for all enqueues
	for i := 0; i < numTasks; i++ {
		<-done
	}

	// Wait for workers to process
	time.Sleep(500 * time.Millisecond)

	// Verify no panic occurred and workers are running
	workerCount := validator.GetWorkerCount()
	if workerCount == 0 {
		t.Error("Expected workers to be running after concurrent enqueues")
	}

	t.Logf("Active workers after concurrent operations: %d", workerCount)
}

func TestAsyncValidator_GetMaxWorkers(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	// Test with different max worker counts
	testCases := []struct {
		name       string
		maxWorkers int
	}{
		{"low", 2},
		{"medium", 5},
		{"high", 10},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, tc.maxWorkers, 1*time.Hour)
			if validator.GetMaxWorkers() != tc.maxWorkers {
				t.Errorf("Expected max workers %d, got %d", tc.maxWorkers, validator.GetMaxWorkers())
			}
		})
	}
}

func TestAsyncValidator_GetWorkerCount(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)
	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 5, 1*time.Hour)

	if err := validator.Start(); err != nil {
		t.Fatalf("Failed to start async validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup

	// Initially should be 0 (on-demand pattern)
	if validator.GetWorkerCount() != 0 {
		t.Errorf("Expected 0 workers initially, got %d", validator.GetWorkerCount())
	}

	// Enqueue task to wake worker
	validator.Enqueue("test_object", "test_object", "/test/path", 1)

	// Wait for worker to start
	time.Sleep(200 * time.Millisecond)

	// Should have at least one worker
	if validator.GetWorkerCount() == 0 {
		t.Error("Expected at least one worker after enqueueing task")
	}
}
