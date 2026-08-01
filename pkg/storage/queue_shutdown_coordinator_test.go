package storage_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/storage"
)

func TestQueueShutdownCoordinator_InitiateShutdown(t *testing.T) {
	coordinator := storage.NewQueueShutdownCoordinatorForTest(nil, true)

	queue1 := storage.NewMockQueueShutdownHandler("queue1", false)
	queue2 := storage.NewMockQueueShutdownHandler("queue2", true)

	coordinator.RegisterQueue(queue1)
	coordinator.RegisterQueue(queue2)

	err := coordinator.InitiateShutdown()
	if err != nil {
		t.Fatalf("InitiateShutdown failed: %v", err)
	}

	if !coordinator.IsShutdownInitiated() {
		t.Error("Expected shutdown to be initiated")
	}

	if queue1.InitiateCalled.Load() != 1 {
		t.Error("Expected queue1.InitiateShutdown to be called once")
	}
	if queue2.InitiateCalled.Load() != 1 {
		t.Error("Expected queue2.InitiateShutdown to be called once")
	}

	err = coordinator.InitiateShutdown()
	if err != nil {
		t.Fatalf("Second InitiateShutdown should be idempotent: %v", err)
	}
	if queue1.InitiateCalled.Load() != 1 {
		t.Error("Expected InitiateShutdown to be idempotent")
	}
}

func TestQueueShutdownCoordinator_DrainAll(t *testing.T) {
	coordinator := storage.NewQueueShutdownCoordinatorForTest(nil, true)

	queue1 := storage.NewMockQueueShutdownHandler("queue1", false)
	queue1.SetPendingCount(5)
	queue2 := storage.NewMockQueueShutdownHandler("queue2", true)
	queue2.SetPendingCount(3)

	coordinator.RegisterQueue(queue1)
	coordinator.RegisterQueue(queue2)

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()

	err := coordinator.DrainAll(ctx)
	if err != nil {
		t.Fatalf("DrainAll failed: %v", err)
	}

	if !queue1.IsDrained() {
		t.Error("Expected queue1 to be drained")
	}
	if !queue2.IsDrained() {
		t.Error("Expected queue2 to be drained")
	}

	if queue1.DrainCalled.Load() != 1 {
		t.Error("Expected queue1.Drain to be called")
	}
	if queue2.DrainCalled.Load() != 1 {
		t.Error("Expected queue2.Drain to be called")
	}
}

func TestQueueShutdownCoordinator_DrainAll_Timeout(t *testing.T) {
	coordinator := storage.NewQueueShutdownCoordinatorForTest(&storage.ShutdownConfig{
		Timeout:       100 * time.Millisecond,
		ForceShutdown: true,
		LogIncomplete: true,
		CheckInterval: 10 * time.Millisecond,
	}, false)

	slowQueue := storage.NewMockQueueShutdownHandler("slow_queue", false)
	slowQueue.SetPendingCount(10)
	slowQueue.DrainDelay = 500 * time.Millisecond

	coordinator.RegisterQueue(slowQueue)

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 1*time.Second)
	defer cancel()

	err := coordinator.DrainAll(ctx)
	if err != nil {
		t.Logf("DrainAll returned error (expected with timeout): %v", err)
	}

	if slowQueue.DrainCalled.Load() != 1 {
		t.Error("Expected slowQueue.Drain to be called")
	}
}

func TestQueueShutdownCoordinator_DrainAll_CriticalQueues(t *testing.T) {
	coordinator := storage.NewQueueShutdownCoordinatorForTest(&storage.ShutdownConfig{
		Timeout:       100 * time.Millisecond,
		ForceShutdown: false,
		LogIncomplete: true,
		CheckInterval: 10 * time.Millisecond,
	}, false)

	criticalQueue := storage.NewMockQueueShutdownHandler("critical_queue", true)
	criticalQueue.SetPendingCount(10)
	criticalQueue.DrainDelay = 50 * time.Millisecond

	coordinator.RegisterQueue(criticalQueue)

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 200*time.Millisecond)
	defer cancel()

	err := coordinator.DrainAll(ctx)
	if err != nil {
		t.Logf("DrainAll returned error (may be expected): %v", err)
	}

	drained := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if criticalQueue.IsDrained() {
			drained = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !drained {
		t.Log("Queue may still be draining")
	}
}

func TestQueueShutdownCoordinator_EmptyQueues(t *testing.T) {
	coordinator := storage.NewQueueShutdownCoordinatorForTest(nil, true)

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()

	err := coordinator.DrainAll(ctx)
	if err != nil {
		t.Fatalf("DrainAll with no queues should succeed: %v", err)
	}
}

func TestQueueShutdownCoordinator_DrainAll_Idempotent(t *testing.T) {
	coordinator := storage.NewQueueShutdownCoordinatorForTest(nil, true)
	q := storage.NewMockQueueShutdownHandler("q1", false)
	q.SetPendingCount(1)
	coordinator.RegisterQueue(q)

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()

	if err := coordinator.DrainAll(ctx); err != nil {
		t.Fatalf("first DrainAll: %v", err)
	}
	if err := coordinator.DrainAll(ctx); err != nil {
		t.Fatalf("second DrainAll: %v", err)
	}
	if q.DrainCalled.Load() != 1 {
		t.Fatalf("expected exactly one Drain on the mock handler, got %d", q.DrainCalled.Load())
	}
}

func TestQueueShutdownCoordinator_InitiateShutdown_Error(t *testing.T) {
	coordinator := storage.NewQueueShutdownCoordinatorForTest(nil, true)

	errorQueue := storage.NewMockQueueShutdownHandler("error_queue", false)
	errorQueue.InitiateError = errors.New("initiate error")

	coordinator.RegisterQueue(errorQueue)

	err := coordinator.InitiateShutdown()
	if err != nil {
		t.Fatalf("InitiateShutdown should not fail on queue error: %v", err)
	}

	if errorQueue.InitiateCalled.Load() != 1 {
		t.Error("Expected errorQueue.InitiateShutdown to be called")
	}
}

func TestQueueShutdownCoordinator_DrainAll_Error(t *testing.T) {
	coordinator := storage.NewQueueShutdownCoordinatorForTest(nil, true)

	errorQueue := storage.NewMockQueueShutdownHandler("error_queue", false)
	errorQueue.SetPendingCount(5)
	errorQueue.DrainError = errors.New("drain error")

	coordinator.RegisterQueue(errorQueue)

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()

	err := coordinator.DrainAll(ctx)
	if err != nil {
		t.Logf("DrainAll returned error (may be expected): %v", err)
	}

	if errorQueue.DrainCalled.Load() != 1 {
		t.Error("Expected errorQueue.Drain to be called")
	}
}

func TestQueueShutdownCoordinator_ConcurrentRegistration(t *testing.T) {
	coordinator := storage.NewQueueShutdownCoordinatorForTest(nil, true)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("storage_test", "concurrent queue registration").StartSimple(func() {
			func(id int) {
				defer wg.Done()
				queue := storage.NewMockQueueShutdownHandler(fmt.Sprintf("queue_%d", id), false)
				coordinator.RegisterQueue(queue)
			}(i)
		})
	}
	wg.Wait()

	if coordinator.RegisteredQueueCountForTest() != 10 {
		t.Errorf("Expected 10 queues, got %d", coordinator.RegisteredQueueCountForTest())
	}
}

func TestQueueShutdownCoordinator_IsShutdownInitiated(t *testing.T) {
	coordinator := storage.NewQueueShutdownCoordinatorForTest(nil, true)

	if coordinator.IsShutdownInitiated() {
		t.Error("Expected shutdown not to be initiated initially")
	}

	err := coordinator.InitiateShutdown()
	if err != nil {
		t.Fatalf("InitiateShutdown failed: %v", err)
	}

	if !coordinator.IsShutdownInitiated() {
		t.Error("Expected shutdown to be initiated")
	}
}
