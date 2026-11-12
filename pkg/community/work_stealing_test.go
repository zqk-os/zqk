package community

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// TestWorkStealing_StaticFloorInvariant verifies CRIT-1794507290799786000-0f4a5784.
func TestWorkStealing_StaticFloorInvariant(t *testing.T) {
	balancer := NewWorkStealingBalancer(5)

	err := balancer.RegisterWorker("worker-alpha")
	if err != nil {
		t.Fatalf("expected RegisterWorker to succeed, got: %v", err)
	}

	task := TaskItem{
		ID:        "task-1",
		Payload:   "execute-build",
		Priority:  1,
		CreatedAt: time.Now(),
	}

	err = balancer.SubmitTask("worker-alpha", task)
	if err != nil {
		t.Fatalf("expected SubmitTask to succeed, got: %v", err)
	}

	len1, err := balancer.GetQueueLength("worker-alpha")
	if err != nil || len1 != 1 {
		t.Errorf("expected queue length 1, got len=%d, err=%v", len1, err)
	}
}

// TestWorkStealing_OperationalDynamicProof verifies CRIT-1794507298156700000-58ff9f55.
func TestWorkStealing_OperationalDynamicProof(t *testing.T) {
	balancer := NewWorkStealingBalancer(10)

	_ = balancer.RegisterWorker("worker-saturated")
	_ = balancer.RegisterWorker("worker-idle")

	// Submit 6 tasks to saturated worker (tasks 0 through 5)
	for i := 0; i < 6; i++ {
		_ = balancer.SubmitTask("worker-saturated", TaskItem{
			ID:        fmt.Sprintf("task-%d", i),
			Payload:   fmt.Sprintf("payload-%d", i),
			Priority:  i,
			CreatedAt: time.Now(),
		})
	}

	// Idle worker steals 3 tasks from saturated worker
	stolen, err := balancer.StealWork("worker-idle", "worker-saturated", 3)
	if err != nil {
		t.Fatalf("expected StealWork to succeed, got: %v", err)
	}
	if len(stolen) != 3 {
		t.Fatalf("expected 3 stolen tasks, got %d", len(stolen))
	}

	// Work-stealing is FIFO from top of deque: should have stolen task-0, task-1, task-2
	for i, task := range stolen {
		expectedID := fmt.Sprintf("task-%d", i)
		if task.ID != expectedID {
			t.Errorf("expected stolen[%d] to be %s, got %s", i, expectedID, task.ID)
		}
	}

	// Saturated worker executes local pop: LIFO from bottom (task-5, task-4, task-3)
	popped, ok, err := balancer.PopLocal("worker-saturated")
	if err != nil || !ok {
		t.Fatalf("expected PopLocal to succeed, got ok=%v, err=%v", ok, err)
	}
	if popped.ID != "task-5" {
		t.Errorf("expected local LIFO pop to return task-5, got %s", popped.ID)
	}

	// Verify remaining count in saturated deque is 2 (task-3, task-4)
	remLen, _ := balancer.GetQueueLength("worker-saturated")
	if remLen != 2 {
		t.Errorf("expected 2 tasks remaining, got %d", remLen)
	}
}

// TestWorkStealing_NegativeAdversarialBoundary verifies CRIT-1794507306413718000-989eb1bc.
func TestWorkStealing_NegativeAdversarialBoundary(t *testing.T) {
	balancer := NewWorkStealingBalancer(3) // Max steal limit = 3

	// 1. Unregistered Worker Rejections
	_, err := balancer.StealWork("ghost-thief", "ghost-victim", 1)
	if !errors.Is(err, ErrWorkerNotRegistered) {
		t.Errorf("expected ErrWorkerNotRegistered, got: %v", err)
	}

	// 2. Blank Worker ID Rejections
	err = balancer.RegisterWorker("  ")
	if !errors.Is(err, ErrBlankWorkerIdentifier) {
		t.Errorf("expected ErrBlankWorkerIdentifier, got: %v", err)
	}

	_ = balancer.RegisterWorker("worker-a")
	_ = balancer.RegisterWorker("worker-b")

	// 3. Invalid Batch Size Rejection
	_, err = balancer.StealWork("worker-a", "worker-b", 0)
	if !errors.Is(err, ErrInvalidBatchSize) {
		t.Errorf("expected ErrInvalidBatchSize for 0 batch, got: %v", err)
	}

	// 4. Stealing from Empty Deque returns nil/empty gracefully without panic
	stolen, err := balancer.StealWork("worker-a", "worker-b", 2)
	if err != nil {
		t.Fatalf("steal from empty deque should not return error: %v", err)
	}
	if len(stolen) != 0 {
		t.Errorf("expected 0 stolen tasks from empty deque, got %d", len(stolen))
	}

	// 5. Throttling: Requesting more than maxStealLimit (3) is clamped to limit
	for i := 0; i < 5; i++ {
		_ = balancer.SubmitTask("worker-b", TaskItem{ID: fmt.Sprintf("t-%d", i)})
	}
	stolenClamped, err := balancer.StealWork("worker-a", "worker-b", 100)
	if err != nil {
		t.Fatalf("unexpected error on clamped steal: %v", err)
	}
	if len(stolenClamped) != 3 {
		t.Errorf("expected clamped steal of 3 tasks (max limit), got %d", len(stolenClamped))
	}
}
