package coordination

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// TestCoordinator_HighConcurrency_DispatchAndDrainReset exercises hundreds of concurrent
// goroutines invoking DispatchEvent and runRouterAsync while background goroutines repeatedly
// call Drain and Reset, ensuring zero sync.WaitGroup misuse panics or race conditions.
func TestCoordinator_HighConcurrency_DispatchAndDrainReset(t *testing.T) {
	coord := NewCoordinator(CoordinatorConfig{})
	stop := make(chan struct{})

	const workerCount = 150
	var wg sync.WaitGroup
	var dispatchedCount atomic.Int64
	var rejectedCount atomic.Int64
	var asyncCompleted atomic.Int64

	// Concurrently dispatch events via DispatchEvent and runRouterAsync
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		workerID := i
		goroutinelabels.NewGoroutine("coordinator_concurrency_test", fmt.Sprintf("dispatch worker %d", workerID)).StartSimple(func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if workerID%2 == 0 {
						// Dispatch an event through Coordinator pipeline
						ev := NewEventContext("op-concurrency", "test_op", "test_event").
							WithChannels(true, false, false, false)
						_ = coord.DispatchEvent(pkgctx.NewSystemContext(), ev)
						dispatchedCount.Add(1)
					} else {
						// Dispatch async work directly
						scheduled := coord.runRouterAsync(pkgctx.NewSystemContext(), "worker", "concurrency_test", func(ctx context.Context) {
							asyncCompleted.Add(1)
							time.Sleep(50 * time.Microsecond)
						})
						if scheduled {
							dispatchedCount.Add(1)
						} else {
							rejectedCount.Add(1)
						}
					}
					time.Sleep(20 * time.Microsecond)
				}
			}
		})
	}

	// Concurrently invoke Drain() and Reset() repeatedly
	var drainResetWG sync.WaitGroup
	drainResetWG.Add(1)
	goroutinelabels.NewGoroutine("coordinator_concurrency_test", "drain reset worker 1").StartSimple(func() {
		defer drainResetWG.Done()
		for i := 0; i < 40; i++ {
			select {
			case <-stop:
				return
			default:
				// Drain with short timeout
				_ = coord.Drain(5 * time.Millisecond)
				time.Sleep(2 * time.Millisecond)
				coord.Reset()
			}
		}
	})

	drainResetWG.Add(1)
	goroutinelabels.NewGoroutine("coordinator_concurrency_test", "drain reset worker 2").StartSimple(func() {
		defer drainResetWG.Done()
		for i := 0; i < 40; i++ {
			select {
			case <-stop:
				return
			default:
				// Drain with zero timeout (synchronous wait)
				_ = coord.Drain(0)
				coord.Reset()
				time.Sleep(3 * time.Millisecond)
			}
		}
	})

	// Let the storm run for 500ms
	time.Sleep(500 * time.Millisecond)
	close(stop)

	wg.Wait()
	drainResetWG.Wait()

	// Final drain to ensure all in-flight tasks finish cleanly
	coord.Reset()
	if err := coord.Drain(2 * time.Second); err != nil {
		t.Fatalf("final drain failed: %v", err)
	}

	t.Logf("High concurrency test passed: dispatched=%d, rejected=%d, asyncCompleted=%d",
		dispatchedCount.Load(), rejectedCount.Load(), asyncCompleted.Load())
}

// TestCoordinator_Drain_RejectionWhileDraining verifies that when Drain is active,
// new calls to runRouterAsync are rejected (returning false) and not added to the waiting WaitGroup.
func TestCoordinator_Drain_RejectionWhileDraining(t *testing.T) {
	coord := NewCoordinator(CoordinatorConfig{})

	inFlightStarted := make(chan struct{})
	inFlightRelease := make(chan struct{})
	inFlightDone := make(chan struct{})

	// Start a worker that stays in-flight during drain
	ok := coord.runRouterAsync(pkgctx.NewSystemContext(), "worker", "inflight", func(ctx context.Context) {
		close(inFlightStarted)
		<-inFlightRelease
		close(inFlightDone)
	})
	if !ok {
		t.Fatalf("expected initial runRouterAsync to succeed")
	}

	<-inFlightStarted

	// Start Drain in separate goroutine
	drainErr := make(chan error, 1)
	goroutinelabels.NewGoroutine("coordinator_concurrency_test", "async drain during rejection test").StartSimple(func() {
		drainErr <- coord.Drain(2 * time.Second)
	})

	// Wait until coordinator enters draining state
	timeout := time.After(500 * time.Millisecond)
	for !coord.IsDraining() {
		select {
		case <-timeout:
			t.Fatalf("coordinator did not enter draining state")
		default:
			time.Sleep(100 * time.Microsecond)
		}
	}

	// Any new async tasks should be rejected while draining
	scheduledDuringDrain := coord.runRouterAsync(pkgctx.NewSystemContext(), "worker", "rejected", func(ctx context.Context) {
		t.Errorf("task scheduled during drain should not have run")
	})
	if scheduledDuringDrain {
		t.Fatalf("expected runRouterAsync to return false while draining")
	}

	// Release in-flight worker
	close(inFlightRelease)
	<-inFlightDone

	// Drain should now complete with nil error
	select {
	case err := <-drainErr:
		if err != nil {
			t.Fatalf("unexpected drain error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("drain did not complete in time")
	}

	// After successful drain, coordinator auto-resets to active state
	if coord.IsDraining() {
		t.Fatalf("expected coordinator to be restored to non-draining state after drain")
	}

	// New task should now succeed
	taskRan := make(chan struct{})
	if !coord.runRouterAsync(pkgctx.NewSystemContext(), "worker", "after_drain", func(ctx context.Context) {
		close(taskRan)
	}) {
		t.Fatalf("expected runRouterAsync to succeed after drain completed")
	}

	select {
	case <-taskRan:
	case <-time.After(1 * time.Second):
		t.Fatalf("task after drain failed to run")
	}

	if err := coord.Drain(500 * time.Millisecond); err != nil {
		t.Fatalf("clean drain after test failed: %v", err)
	}
}

// TestCoordinator_Drain_TimeoutAndResetLifecycle verifies that when Drain times out,
// coordinator state remains consistent, and Reset cleanly re-enables scheduling.
func TestCoordinator_Drain_TimeoutAndResetLifecycle(t *testing.T) {
	coord := NewCoordinator(CoordinatorConfig{})

	hangBlock := make(chan struct{})
	defer close(hangBlock)

	// Launch a task that hangs longer than drain timeout
	coord.runRouterAsync(pkgctx.NewSystemContext(), "hanging_task", "test", func(ctx context.Context) {
		<-hangBlock
	})

	// Drain with very short timeout
	err := coord.Drain(20 * time.Millisecond)
	if err == nil {
		t.Fatalf("expected drain timeout error, got nil")
	}

	// Coordinator should still be in draining state due to timeout
	if !coord.IsDraining() {
		t.Fatalf("expected coordinator to remain draining after timeout")
	}

	// New task should be rejected
	if coord.runRouterAsync(pkgctx.NewSystemContext(), "rejected", "test", func(ctx context.Context) {}) {
		t.Fatalf("expected task to be rejected after drain timeout")
	}

	// Explicit Reset should clear draining state and allocate a fresh WaitGroup
	coord.Reset()
	if coord.IsDraining() {
		t.Fatalf("expected coordinator to not be draining after Reset()")
	}

	// New task can now be scheduled
	freshRan := make(chan struct{})
	if !coord.runRouterAsync(pkgctx.NewSystemContext(), "fresh_task", "test", func(ctx context.Context) {
		close(freshRan)
	}) {
		t.Fatalf("expected fresh task to be accepted after Reset()")
	}

	select {
	case <-freshRan:
	case <-time.After(1 * time.Second):
		t.Fatalf("fresh task did not execute")
	}
}

// TestCoordinator_ConcurrentDrainsAndResets verifies that multiple goroutines calling
// Drain and Reset concurrently do not deadlock or panic.
func TestCoordinator_ConcurrentDrainsAndResets(t *testing.T) {
	coord := NewCoordinator(CoordinatorConfig{})
	stop := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		workerID := i
		goroutinelabels.NewGoroutine("coordinator_concurrency_test", fmt.Sprintf("concurrent drain reset worker %d", workerID)).StartSimple(func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = coord.Drain(2 * time.Millisecond)
					coord.Reset()
					time.Sleep(50 * time.Microsecond)
				}
			}
		})
	}

	for i := 0; i < 20; i++ {
		wg.Add(1)
		workerID := i
		goroutinelabels.NewGoroutine("coordinator_concurrency_test", fmt.Sprintf("concurrent churn worker %d", workerID)).StartSimple(func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					coord.runRouterAsync(pkgctx.NewSystemContext(), "worker", "churn", func(ctx context.Context) {
						time.Sleep(10 * time.Microsecond)
					})
					time.Sleep(50 * time.Microsecond)
				}
			}
		})
	}

	time.Sleep(300 * time.Millisecond)
	close(stop)
	wg.Wait()

	coord.Reset()
	if err := coord.Drain(1 * time.Second); err != nil {
		t.Fatalf("final drain failed: %v", err)
	}
}
