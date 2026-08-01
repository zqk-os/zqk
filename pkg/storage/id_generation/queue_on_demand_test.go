package id_generation

import (
	"context"
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

// newTestQueueManager creates a QueueManager for a single test. The test runs in
// parallel with others; each has its own manager and context. Caller must defer
// qm.Stop() to clean up the worker.
func newTestQueueManager(t *testing.T) *QueueManager {
	t.Helper()
	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	t.Cleanup(cancel)
	qm := NewQueueManager(ctx)
	t.Cleanup(func() { qm.Stop() })
	return qm
}

// TestQueueManager_OnDemandPattern tests the on-demand worker pattern.
// Uses a per-test manager so it can run in parallel.
func TestQueueManager_OnDemandPattern(t *testing.T) {
	t.Parallel()
	qm := newTestQueueManager(t)

	if qm.IsWorkerRunning() {
		t.Error("Expected worker not to be running initially")
	}

	tmpDir := t.TempDir()
	kindDir := filepath.Join(tmpDir, "test_kind")
	if err := os.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create kind directory: %v", err)
	}

	queue := qm.GetOrCreateQueue(kindDir, "test_kind", "TEST", 3, 1, 100)
	if queue == nil {
		t.Fatal("GetOrCreateQueue returned nil")
	}

	if !waitForConditionWithTimeout(pkgctx.NewSystemContext(),
		func() bool { return qm.IsWorkerRunning() },
		10*time.Second,
		20*time.Millisecond,
	) {
		t.Fatal("Expected worker to be running within 10 seconds after queue creation")
	}

	if !waitForConditionWithTimeout(pkgctx.NewSystemContext(),
		func() bool { return queue.Size() > 0 },
		10*time.Second,
		20*time.Millisecond,
	) {
		t.Error("Expected queue to be refilled within 10 seconds")
	}
}

// TestQueueManager_CoordinatorIntegration tests coordinator integration.
// Uses a per-test manager so it can run in parallel and safely stop at the end.
func TestQueueManager_CoordinatorIntegration(t *testing.T) {
	t.Parallel()
	qm := newTestQueueManager(t)

	var callbackCalls atomic.Int32
	var lastEvent struct {
		mu            sync.Mutex
		operationType string
		eventType     string
		status        string
		err           error
	}

	SetIDQueueEventCallback(func(
		_ context.Context,
		operationType string,
		eventType string,
		status string,
		err error,
	) {
		callbackCalls.Add(1)
		lastEvent.mu.Lock()
		lastEvent.operationType = operationType
		lastEvent.eventType = eventType
		lastEvent.status = status
		lastEvent.err = err
		lastEvent.mu.Unlock()
	})
	defer SetIDQueueEventCallback(nil)

	tmpDir := t.TempDir()
	kindDir := filepath.Join(tmpDir, "test_kind")
	if err := os.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create kind directory: %v", err)
	}

	_ = qm.GetOrCreateQueue(kindDir, "test_kind", "TEST", 3, 1, 100)

	if !waitForConditionWithTimeout(pkgctx.NewSystemContext(),
		func() bool { return qm.IsWorkerRunning() },
		10*time.Second,
		20*time.Millisecond,
	) {
		t.Fatal("Expected worker to start within 10 seconds after queue creation")
	}

	callbackWaiter := newTestCallbackWaiter()
	initialCalls := callbackCalls.Load()
	SetIDQueueEventCallback(func(
		_ context.Context,
		operationType string,
		eventType string,
		status string,
		err error,
	) {
		callbackCalls.Add(1)
		lastEvent.mu.Lock()
		lastEvent.operationType = operationType
		lastEvent.eventType = eventType
		lastEvent.status = status
		lastEvent.err = err
		lastEvent.mu.Unlock()
		callbackWaiter.Invoke()
	})

	if !callbackWaiter.WaitForInvocation(pkgctx.NewSystemContext(), 10*time.Second) {
		if callbackCalls.Load() == initialCalls && !qm.IsWorkerRunning() {
			t.Error("Expected callback to be invoked or worker to be running")
		}
	}

	finalCalls := callbackCalls.Load()
	if finalCalls == initialCalls && !qm.IsWorkerRunning() {
		t.Error("Expected callback to be called or worker to be running, but neither happened")
		return
	}
	if finalCalls > initialCalls {
		lastEvent.mu.Lock()
		opType, evType, st := lastEvent.operationType, lastEvent.eventType, lastEvent.status
		lastEvent.mu.Unlock()
		if opType != "id_queue_manager" {
			t.Errorf("Expected operation type 'id_queue_manager', got '%s'", opType)
		}
		if evType != "worker_lifecycle" {
			t.Errorf("Expected event type 'worker_lifecycle', got '%s'", evType)
		}
		if st != "start" && st != "stop" {
			t.Errorf("Expected status 'start' or 'stop', got '%s'", st)
		}
	}
}

// TestQueueManager_IdleShutdown tests idle shutdown behavior.
// Uses a per-test manager so it can run in parallel.
func TestQueueManager_IdleShutdown(t *testing.T) {
	t.Parallel()
	qm := newTestQueueManager(t)

	tmpDir := t.TempDir()
	kindDir := filepath.Join(tmpDir, "test_kind")
	if err := os.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create kind directory: %v", err)
	}

	queue := qm.GetOrCreateQueue(kindDir, "test_kind", "TEST", 3, 1, 100)
	if queue == nil {
		t.Fatal("GetOrCreateQueue returned nil")
	}

	if !waitForConditionWithTimeout(pkgctx.NewSystemContext(),
		func() bool { return qm.IsWorkerRunning() },
		10*time.Second,
		20*time.Millisecond,
	) {
		t.Fatal("Expected worker to start within 10 seconds")
	}

	if !waitForConditionWithTimeout(pkgctx.NewSystemContext(),
		func() bool { return queue.Size() > 0 },
		10*time.Second,
		20*time.Millisecond,
	) {
		t.Error("Expected queue to be refilled within 10 seconds")
	}

	for queue.Size() > 0 {
		_, _ = queue.Pop()
	}

	_ = qm.GetOrCreateQueue(kindDir, "test_kind2", "TEST2", 3, 1, 100)

	if !waitForConditionWithTimeout(pkgctx.NewSystemContext(),
		func() bool { return qm.IsWorkerRunning() },
		10*time.Second,
		20*time.Millisecond,
	) {
		t.Error("Expected worker to wake up when new queue is created")
	}
}

// TestQueueManager_NoCallback tests that operations work when callback is not set.
// Uses a per-test manager so it can run in parallel.
func TestQueueManager_NoCallback(t *testing.T) {
	t.Parallel()
	SetIDQueueEventCallback(nil)
	defer SetIDQueueEventCallback(nil)

	qm := newTestQueueManager(t)
	tmpDir := t.TempDir()
	kindDir := filepath.Join(tmpDir, "test_kind")
	if err := os.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create kind directory: %v", err)
	}

	queue := qm.GetOrCreateQueue(kindDir, "test_kind", "TEST", 3, 1, 100)
	if queue == nil {
		t.Fatal("GetOrCreateQueue returned nil")
	}

	if !waitForConditionWithTimeout(pkgctx.NewSystemContext(),
		func() bool { return queue.Size() > 0 },
		10*time.Second,
		20*time.Millisecond,
	) {
		t.Error("Expected queue to be refilled within 10 seconds")
	}
}

// TestQueueManager_ConcurrentQueueCreation tests concurrent queue creation.
// Uses a per-test manager so it can run in parallel.
func TestQueueManager_ConcurrentQueueCreation(t *testing.T) {
	t.Parallel()
	qm := newTestQueueManager(t)
	tmpDir := t.TempDir()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("storage_test", "concurrent queue creation").StartSimple(func() {
			func(idx int) {
				defer wg.Done()
				kindDir := filepath.Join(tmpDir, fmt.Sprintf("test_kind_%d", idx))
				if err := os.MkdirAll(kindDir, paths.DirPerm755); err != nil {
					t.Errorf("failed to create kind directory: %v", err)
					return
				}
				_ = qm.GetOrCreateQueue(kindDir, fmt.Sprintf("test_kind_%d", idx), "TEST", 3, 1, 100)
			}(i)
		})
	}
	wg.Wait()

	if !waitForConditionWithTimeout(pkgctx.NewSystemContext(),
		func() bool { return qm.IsWorkerRunning() },
		10*time.Second,
		20*time.Millisecond,
	) {
		t.Error("Expected worker to be running after concurrent queue creation")
	}
}
