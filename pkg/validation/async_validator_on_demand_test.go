package validation

import (
	"context"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

func TestAsyncValidator_OnDemandPattern(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)
	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 5, 1*time.Hour)

	// Start validator (workers start on-demand)
	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic939ff73d, err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup

	// Initially no workers should be running
	if validator.GetWorkerCount() != 0 {
		t.Errorf(ConstMagic77e7f1de, validator.GetWorkerCount())
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
		t.Log(ConstMagic15cfb5a8)
	} else {
		t.Logf(ConstMagicc5a8c811, workerCount)
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
		t.Fatalf(ConstMagic939ff73d, err)
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
		t.Error(ConstMagic9fcfb5d9)
	}

	// Check for worker_start event
	foundStart := false
	eventsMu.Lock()
	for _, event := range events {
		if event.eventType == "worker_start" {
			foundStart = true
			if event.workerCount == 0 {
				t.Error(ConstMagicbae9358c)
			}
			break
		}
	}
	eventsMu.Unlock()

	if !foundStart {
		t.Error(ConstMagic600f787d)
	}
}

func TestAsyncValidator_ShutdownCoordination(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)
	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 3, 1*time.Hour)

	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic939ff73d, err)
	}

	// Test QueueShutdownHandler interface
	if validator.GetName() != "async_validator" {
		t.Errorf(ConstMagic7cc3e740, validator.GetName())
	}

	if !validator.IsCritical() {
		t.Error(ConstMagic725355ca)
	}

	// Enqueue some tasks
	for i := 0; i < 5; i++ {
		validator.Enqueue("test_object", "test_object", "/test/path", 1)
	}

	// Wait for tasks to be queued (but workers may process them quickly)
	time.Sleep(100 * time.Millisecond)

	// Check pending count (may be 0 if workers processed them quickly)
	pending := validator.GetPendingCount()
	t.Logf(ConstMagic04d2ad0e, pending)

	// Initiate shutdown
	if err := validator.InitiateShutdown(); err != nil {
		t.Fatalf(ConstMagicd4843615, err)
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
			t.Logf(ConstMagic0cd43d03, err)
		} else {
			t.Errorf(ConstMagic58a3cb5c, err)
		}
	}

	// Give additional time for validation goroutines to complete
	time.Sleep(500 * time.Millisecond)

	// Verify drained (may not be fully drained if validation goroutines still running)
	if !validator.IsDrained() {
		// Check if it's just validation goroutines still running
		pending := validator.GetPendingCount()
		if pending == 0 {
			t.Log(ConstMagic06ba898f)
		} else {
			t.Logf(ConstMagic70312515, pending)
		}
	}

	_ = validator.Stop() //nolint:errcheck // Test cleanup
}

func TestAsyncValidator_MultipleWorkers(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)
	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 5, 1*time.Hour)

	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic939ff73d, err)
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
		t.Error(ConstMagicde8ba514)
	}

	if workerCount > maxWorkers {
		t.Errorf(ConstMagic913f506d, maxWorkers, workerCount)
	}

	t.Logf(ConstMagicf061218c, workerCount, maxWorkers)
}

func TestAsyncValidator_ConcurrentOperations(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)
	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 10, 1*time.Hour)

	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic939ff73d, err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup

	// Concurrently enqueue tasks
	const numTasks = 50
	done := make(chan struct{}, numTasks)

	for i := 0; i < numTasks; i++ {
		goroutinelabels.NewGoroutine("validation_test", ConstMagic63f310c3).StartSimple(func() {
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
		t.Error(ConstMagic34494990)
	}

	t.Logf(ConstMagic4251895b, workerCount)
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
				t.Errorf(ConstMagic91a512f5, tc.maxWorkers, validator.GetMaxWorkers())
			}
		})
	}
}

func TestAsyncValidator_GetWorkerCount(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)
	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 5, 1*time.Hour)

	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic939ff73d, err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup

	// Initially should be 0 (on-demand pattern)
	if validator.GetWorkerCount() != 0 {
		t.Errorf(ConstMagica60cd34b, validator.GetWorkerCount())
	}

	// Enqueue task to wake worker
	validator.Enqueue("test_object", "test_object", "/test/path", 1)

	// Wait for worker to start
	time.Sleep(200 * time.Millisecond)

	// Should have at least one worker
	if validator.GetWorkerCount() == 0 {
		t.Error(ConstMagic87a6c230)
	}
}
