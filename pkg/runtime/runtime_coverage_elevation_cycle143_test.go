package runtime

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestGoroutineManager_LifecycleHooksAndCallbacks(t *testing.T) {
	t.Parallel()
	mockCoord := &MockCoordinator{}
	manager := NewGoroutineManager(pkgctx.NewSystemContext(), mockCoord)
	t.Cleanup(func() {
		err := manager.Shutdown()
		if err != nil {
			t.Logf("cleanup shutdown returned: %v", err)
		}
	})

	var startCalled atomic.Bool
	var stopCalled atomic.Bool
	var stoppedID atomic.Value
	stoppedID.Store("")

	unblock := make(chan struct{})
	id, _, err := manager.Start(GoroutineConfig{
		Name:     "hook_test",
		Purpose:  "test lifecycle hooks",
		Category: "worker",
		OnStart: func(startedID string) {
			startCalled.Store(true)
		},
		OnStop: func(id string, stopErr error) {
			stoppedID.Store(id)
			stopCalled.Store(true)
		},
	}, func(ctx context.Context) error {
		<-unblock
		return nil
	})
	if err != nil {
		t.Fatalf("failed to start goroutine: %v", err)
	}

	if !startCalled.Load() {
		t.Fatalf("expected OnStart callback to be invoked immediately")
	}

	trackedList := manager.ListGoroutines()
	if len(trackedList) == 0 {
		t.Fatalf("expected non-empty list from ListGoroutines")
	}

	close(unblock)

	waitForCondition(t, 2*time.Second, func() bool {
		return stopCalled.Load()
	}, "waiting for OnStop callback")

	if stoppedID.Load().(string) != id {
		t.Fatalf("expected stopped id %s, got %v", id, stoppedID.Load())
	}

	if manager.TotalStarted() < 1 {
		t.Fatalf("expected TotalStarted >= 1, got %d", manager.TotalStarted())
	}
	if manager.TotalStopped() < 1 {
		t.Fatalf("expected TotalStopped >= 1, got %d", manager.TotalStopped())
	}
}

func TestGoroutineManager_PanicHandlerAndRecovery(t *testing.T) {
	t.Parallel()
	mockCoord := &MockCoordinator{}
	manager := NewGoroutineManager(pkgctx.NewSystemContext(), mockCoord)
	t.Cleanup(func() {
		err := manager.Shutdown()
		if err != nil {
			t.Logf("cleanup shutdown returned: %v", err)
		}
	})

	var stopInvoked atomic.Bool
	var capturedErr atomic.Value
	capturedErr.Store("")

	id, _, err := manager.Start(GoroutineConfig{
		Name:     "panic_goroutine",
		Purpose:  "testing panic recovery",
		Category: "resilience",
		OnStop: func(id string, stopErr error) {
			stopInvoked.Store(true)
			if stopErr != nil {
				capturedErr.Store(stopErr.Error())
			}
		},
	}, func(ctx context.Context) error {
		panic("deliberate worker panic")
	})
	if err != nil {
		t.Fatalf("failed to start panicking goroutine: %v", err)
	}

	waitForCondition(t, 2*time.Second, func() bool {
		return stopInvoked.Load()
	}, "waiting for panic stop handler")

	errStr := capturedErr.Load().(string)
	if !strings.Contains(errStr, "panic in goroutine") || !strings.Contains(errStr, "deliberate worker panic") {
		t.Fatalf("expected panic error message, got: %s", errStr)
	}

	tracked, getErr := manager.GetGoroutine(id)
	if getErr != nil {
		t.Fatalf("failed to get tracked goroutine: %v", getErr)
	}
	if tracked.Status != StatusError {
		t.Fatalf("expected StatusError, got %s", tracked.Status)
	}
	if manager.TotalErrors() < 1 {
		t.Fatalf("expected TotalErrors >= 1, got %d", manager.TotalErrors())
	}
}

func TestGoroutineManager_GetGoroutineAndLookup(t *testing.T) {
	t.Parallel()
	mockCoord := &MockCoordinator{}
	manager := NewGoroutineManager(pkgctx.NewSystemContext(), mockCoord)
	t.Cleanup(func() {
		err := manager.Shutdown()
		if err != nil {
			t.Logf("cleanup shutdown: %v", err)
		}
	})

	unblock := make(chan struct{})
	id, _, err := manager.Start(GoroutineConfig{
		Name:     "lookup_test",
		Purpose:  "test lookups",
		Category: "test",
	}, func(ctx context.Context) error {
		<-unblock
		return nil
	})
	if err != nil {
		t.Fatalf("failed to start goroutine: %v", err)
	}

	tracked, err := manager.GetGoroutine(id)
	if err != nil {
		t.Fatalf("failed to get existing goroutine: %v", err)
	}
	if tracked.ID != id {
		t.Fatalf("expected ID %s, got %s", id, tracked.ID)
	}
	if tracked.Mu() == nil {
		t.Fatalf("expected non-nil mutex from TrackedGoroutine.Mu()")
	}

	var nilTracked *TrackedGoroutine
	if nilTracked.Mu() != nil {
		t.Fatalf("expected nil mutex from nil TrackedGoroutine.Mu()")
	}

	rawTracked := NewTrackedGoroutine("raw_id", GoroutineConfig{Name: "raw"}, context.Background(), func() {})
	if rawTracked.Metadata == nil {
		t.Fatalf("expected initialized metadata for raw tracked goroutine")
	}

	_, missingErr := manager.GetGoroutine("non-existent-id")
	if missingErr == nil {
		t.Fatalf("expected error getting non-existent goroutine")
	}

	stopMissingErr := manager.Stop("non-existent-id")
	if stopMissingErr == nil {
		t.Fatalf("expected error stopping non-existent goroutine")
	}

	close(unblock)
}

func TestGoroutineManager_ResourceCleanupFailure(t *testing.T) {
	t.Parallel()
	mockCoord := &MockCoordinator{}
	manager := NewGoroutineManager(pkgctx.NewSystemContext(), mockCoord)
	t.Cleanup(func() {
		err := manager.Shutdown()
		if err != nil {
			t.Logf("cleanup shutdown: %v", err)
		}
	})

	id, _, err := manager.Start(GoroutineConfig{
		Name:     "res_cleanup_fail",
		Purpose:  "test cleanup error event",
		Category: "storage",
		Resources: []Resource{
			{
				Type:        "test_resource",
				ID:          "res_fail_1",
				Description: "resource that fails cleanup",
				CleanupFunc: func() error {
					return errors.New("simulated cleanup failure")
				},
			},
		},
	}, func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	})
	if err != nil {
		t.Fatalf("failed to start goroutine: %v", err)
	}

	stopErr := manager.Stop(id)
	if stopErr != nil {
		t.Fatalf("manager.Stop failed: %v", stopErr)
	}

	manager.stopGoroutine(id, nil, nil)
	manager.stopGoroutine("completely-unknown-id", nil, nil)
}

func TestGoroutineManager_DetectLeaksThreshold(t *testing.T) {
	t.Parallel()
	mockCoord := &MockCoordinator{}
	manager := NewGoroutineManager(pkgctx.NewSystemContext(), mockCoord)
	t.Cleanup(func() {
		err := manager.Shutdown()
		if err != nil {
			t.Logf("cleanup shutdown: %v", err)
		}
	})

	unblock := make(chan struct{})
	id, _, err := manager.Start(GoroutineConfig{
		Name:     "leak_threshold_test",
		Purpose:  "test leak detection heuristic",
		Category: "leak_test",
	}, func(ctx context.Context) error {
		<-unblock
		return nil
	})
	if err != nil {
		t.Fatalf("failed to start goroutine: %v", err)
	}

	tracked, err := manager.GetGoroutine(id)
	if err != nil {
		t.Fatalf("failed to get goroutine: %v", err)
	}

	tracked.StartTime = time.Now().Add(-2 * time.Hour)

	leaks := manager.DetectLeaks()
	if len(leaks) == 0 {
		t.Fatalf("expected leak to be detected for goroutine running >1 hour")
	}

	close(unblock)
	waitForCondition(t, 2*time.Second, func() bool {
		tItem, getErr := manager.GetGoroutine(id)
		return getErr == nil && tItem.Status == StatusStopped
	}, "waiting for goroutine to stop")

	leaksAfterStop := manager.DetectLeaks()
	if len(leaksAfterStop) != 0 {
		t.Fatalf("expected 0 leaks after goroutine stopped, got %d", len(leaksAfterStop))
	}
}

func TestGoroutineManager_FullLeakChannelAndStats(t *testing.T) {
	t.Parallel()
	mockCoord := &MockCoordinator{}
	manager := NewGoroutineManager(pkgctx.NewSystemContext(), mockCoord)
	manager.SetShutdownTimeout(50 * time.Millisecond)

	fullCh := make(chan string, 1)
	fullCh <- "prefilled_item"
	manager.SetLeakNotificationChannel(fullCh)

	goroutineStarted := make(chan struct{})
	unblock := make(chan struct{})
	t.Cleanup(func() {
		close(unblock)
	})

	_, _, err := manager.Start(GoroutineConfig{
		Name:     "blocking_leak_candidate",
		Purpose:  "test full leak notification buffer",
		Category: "blocking",
		Metadata: map[string]any{"shard": "shard-1"},
	}, func(ctx context.Context) error {
		close(goroutineStarted)
		<-unblock
		return nil
	})
	if err != nil {
		t.Fatalf("failed to start blocking goroutine: %v", err)
	}

	select {
	case <-goroutineStarted:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for blocking goroutine to start")
	}

	stats := manager.GetRuntimeStats()
	if stats["active_count"].(int) != 1 {
		t.Fatalf("expected active_count=1 in stats, got %v", stats["active_count"])
	}
	if stats["runtime_goroutines"].(int) <= 0 {
		t.Fatalf("expected runtime_goroutines > 0 in stats")
	}

	shutdownErr := manager.Shutdown()
	if shutdownErr == nil {
		t.Fatalf("expected shutdown timeout error for un-stopped goroutine")
	}
}

func TestGoroutineManager_NilCoordinatorEvents(t *testing.T) {
	t.Parallel()
	manager := NewGoroutineManager(pkgctx.NewSystemContext(), nil)

	var stopInvoked atomic.Bool
	id, _, err := manager.Start(GoroutineConfig{
		Name:     "nil_coord_task",
		Purpose:  "test nil coordinator resilience",
		Category: "safe",
		OnStop: func(id string, stopErr error) {
			stopInvoked.Store(true)
		},
	}, func(ctx context.Context) error {
		return errors.New("safe error with nil coord")
	})
	if err != nil {
		t.Fatalf("failed to start task with nil coord: %v", err)
	}

	waitForCondition(t, 2*time.Second, func() bool {
		return stopInvoked.Load()
	}, "waiting for onStop with nil coordinator")

	tracked, getErr := manager.GetGoroutine(id)
	if getErr != nil {
		t.Fatalf("failed to retrieve goroutine: %v", getErr)
	}
	if tracked.Status != StatusError {
		t.Fatalf("expected StatusError, got %s", tracked.Status)
	}

	shutdownErr := manager.Shutdown()
	if shutdownErr != nil {
		t.Fatalf("expected clean shutdown, got: %v", shutdownErr)
	}
}

func TestHelpers_WorkerPoolCapacityExceeded(t *testing.T) {
	t.Parallel()
	mockCoord := &MockCoordinator{}
	manager := NewGoroutineManager(pkgctx.NewSystemContext(), mockCoord)
	t.Cleanup(func() {
		err := manager.Shutdown()
		if err != nil {
			t.Logf("cleanup shutdown: %v", err)
		}
	})

	manager.SetMaxGoroutines(2)

	unblock := make(chan struct{})
	t.Cleanup(func() {
		close(unblock)
	})

	workerIDs, poolErr := StartWorkerPool(manager, "cap_pool", 4, func(ctx context.Context, id int) error {
		<-unblock
		return nil
	})
	if poolErr == nil {
		t.Fatalf("expected StartWorkerPool to fail when exceeding maxGoroutines limit")
	}
	if len(workerIDs) != 0 {
		t.Fatalf("expected 0 workerIDs returned on failure, got %v", workerIDs)
	}
	if !strings.Contains(poolErr.Error(), "failed to start worker") {
		t.Fatalf("expected error wrapping 'failed to start worker', got: %v", poolErr)
	}
}

func TestHelpers_ExecuteAsync_ManagerLimit(t *testing.T) {
	t.Parallel()
	mockCoord := &MockCoordinator{}
	manager := NewGoroutineManager(pkgctx.NewSystemContext(), mockCoord)
	t.Cleanup(func() {
		err := manager.Shutdown()
		if err != nil {
			t.Logf("cleanup shutdown: %v", err)
		}
	})

	manager.SetMaxGoroutines(1)
	unblock := make(chan struct{})
	t.Cleanup(func() {
		close(unblock)
	})

	_, _, err := manager.Start(GoroutineConfig{
		Name:     "slot_filler",
		Purpose:  "fill slot",
		Category: "test",
	}, func(ctx context.Context) error {
		<-unblock
		return nil
	})
	if err != nil {
		t.Fatalf("failed to start slot filler: %v", err)
	}

	execID, _, errChan := ExecuteAsync(manager, "over_capacity_async", func(ctx context.Context) error {
		return nil
	})
	if execID != "" {
		t.Fatalf("expected empty execID on start failure, got: %s", execID)
	}

	select {
	case execErr := <-errChan:
		if execErr == nil {
			t.Fatalf("expected error in errChan when max limit reached")
		}
		if !strings.Contains(execErr.Error(), "max goroutines limit reached") {
			t.Fatalf("expected max goroutines limit error, got: %v", execErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for ExecuteAsync error")
	}
}

func TestHelpers_PeriodicTask_CancellationAndLoop(t *testing.T) {
	t.Parallel()
	mockCoord := &MockCoordinator{}
	manager := NewGoroutineManager(pkgctx.NewSystemContext(), mockCoord)
	t.Cleanup(func() {
		err := manager.Shutdown()
		if err != nil {
			t.Logf("cleanup shutdown: %v", err)
		}
	})

	var executionCount atomic.Int32
	id, _, err := StartPeriodicTask(manager, "fast_periodic", 15*time.Millisecond, func(ctx context.Context) error {
		executionCount.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("failed to start periodic task: %v", err)
	}

	waitForCondition(t, 2*time.Second, func() bool {
		return executionCount.Load() >= 2
	}, "waiting for periodic ticks")

	stopErr := manager.Stop(id)
	if stopErr != nil {
		t.Fatalf("failed to stop periodic task: %v", stopErr)
	}
}

func TestLockOpNamesConstants(t *testing.T) {
	t.Parallel()
	names := []string{
		LockNameGoroutineManagerDetectLeaksCopy,
		LockNameGoroutineManagerGetStatsCopy,
		LockNameGoroutineManagerLeakCheck,
		LockNameGoroutineManagerMarkLeaked,
		LockNameGoroutineManagerPostcleanupCheck,
		LockNameGoroutineManagerSetLeakChannel,
		LockNameGoroutineManagerSetMax,
		LockNameGoroutineManagerSetRunning,
		LockNameGoroutineManagerSetShutdownTimeout,
		LockNameGoroutineManagerShutdownTimeoutCopy,
		LockNameGoroutineManagerStatsRead,
		LockNameGoroutineManagerStopAllGetIds,
		LockNameGoroutineManagerStopCheck,
		LockNameGoroutineManagerStopGet,
		LockNameGoroutineManagerStopUpdate,
	}

	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			t.Fatalf("expected non-empty lock operation name")
		}
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool, desc string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out after %v waiting for condition: %s", timeout, desc)
}
