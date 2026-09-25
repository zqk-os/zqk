package goroutinelabels

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPool_NewPool_ReservesBudget(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 20})

	pool := b.NewPool("test", "test pool", 5, 10)
	defer pool.Stop()

	if b.Reserved() != 5 {
		t.Errorf("after NewPool: Reserved() = %d, want 5", b.Reserved())
	}
	if pool.Size() != 5 {
		t.Errorf("Size() = %d, want 5", pool.Size())
	}
}

func TestPool_Stop_WithoutStart_ReleasesBudget(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 20})

	pool := b.NewPool("test", "test pool", 4, 10)
	if b.Reserved() != 4 {
		t.Errorf("after NewPool: Reserved() = %d, want 4", b.Reserved())
	}

	pool.Stop()
	if b.Reserved() != 0 {
		t.Errorf("after Stop without Start: Reserved() = %d, want 0", b.Reserved())
	}
}

func TestPool_Start_Submit_Stop(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 20})
	pool := b.NewPool("test", "test pool", 2, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool.Start(ctx)

	var count atomic.Int32
	for i := 0; i < 5; i++ {
		err := pool.Submit(ctx, func(ctx context.Context) error {
			count.Add(1)
			return nil
		})
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}

	// Give workers time to process
	time.Sleep(50 * time.Millisecond)
	if count.Load() != 5 {
		t.Errorf("expected 5 tasks run, got %d", count.Load())
	}

	pool.Stop()
	if b.Reserved() != 0 {
		t.Errorf("after Stop: Reserved() = %d, want 0", b.Reserved())
	}
}

func TestPool_Submit_BeforeStart_ReturnsCanceled(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 20})
	pool := b.NewPool("test", "test pool", 2, 10)
	defer pool.Stop()

	ctx := context.Background()
	err := pool.Submit(ctx, func(context.Context) error { return nil })
	if err != context.Canceled {
		t.Errorf("Submit before Start: got err %v, want context.Canceled", err)
	}
}

func TestPool_Submit_AfterStop_ReturnsCanceled(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 20})
	pool := b.NewPool("test", "test pool", 2, 10)

	ctx, cancel := context.WithCancel(context.Background())
	pool.Start(ctx)
	cancel()
	pool.Stop()

	err := pool.Submit(context.Background(), func(context.Context) error { return nil })
	if err != context.Canceled {
		t.Errorf("Submit after Stop: got err %v, want context.Canceled", err)
	}
}

func TestPool_SubmitNonBlocking_Full(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 20})
	// Queue size 1, 1 worker - fill the queue and the worker's current work
	pool := b.NewPool("test", "test pool", 1, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx)
	defer pool.Stop()

	// Block the single worker so it doesn't drain the queue
	block := make(chan struct{})
	defer close(block)
	_ = pool.Submit(ctx, func(context.Context) error {
		<-block
		return nil
	})

	// Fill the one slot in the queue
	_ = pool.Submit(ctx, func(context.Context) error { return nil })

	// Next submit should fail with ErrPoolFull (non-blocking)
	err := pool.SubmitNonBlocking(ctx, func(context.Context) error { return nil })
	if err != ErrPoolFull {
		t.Errorf("SubmitNonBlocking when full: got %v, want ErrPoolFull", err)
	}
}

// TestPool_Submit_DeadlineExceededWhenWorkerHeldByLongTask documents the same handoff semantics the
// scheduler relies on (pkg/scheduler.submitTriggeredJob → Pool.Submit). A single task that keeps the
// sole worker busy prevents accepting further work until the submitter's context expires — no large
// job backlog is required. This is the mechanical basis for scheduler-events dispatch_pressure rows
// with reason dispatch_resource_wait_deadline_exceeded and source cron_pool_submit.
func TestPool_Submit_DeadlineExceededWhenWorkerHeldByLongTask(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 20})
	// One worker, unbuffered queue: second Submit blocks on channel send until the worker finishes
	// the first task or the submit context times out.
	pool := b.NewPool("test", "stall_semantics", 1, 0)

	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	pool.Start(runCtx)
	defer pool.Stop()

	hold := make(chan struct{})

	if err := pool.Submit(runCtx, func(context.Context) error {
		<-hold
		return nil
	}); err != nil {
		t.Fatalf("first Submit: %v", err)
	}

	waitCtx, cancelWait := context.WithTimeout(runCtx, 150*time.Millisecond)
	defer cancelWait()

	errSecond := pool.Submit(waitCtx, func(context.Context) error { return nil })
	if !errors.Is(errSecond, context.DeadlineExceeded) {
		t.Fatalf("second Submit: got %v, want DeadlineExceeded", errSecond)
	}

	close(hold) // unblocks the worker; pool.Stop() waits for goroutine exit
}

func TestPool_QueuePressureSnapshot(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 20})
	pool := b.NewPool("snap", "snapshot test", 1, 3)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx)
	defer pool.Stop()

	hold := make(chan struct{})
	if err := pool.Submit(ctx, func(context.Context) error {
		<-hold
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := pool.Submit(ctx, func(context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if pool.Name() != "snap" {
		t.Fatalf("Name() = %q want snap", pool.Name())
	}
	w, q, c := pool.QueuePressureSnapshot()
	if w != 1 || q != 3 || c != 3 {
		t.Fatalf("QueuePressureSnapshot() = (%d,%d,%d) want (1,3,3)", w, q, c)
	}
	close(hold)
}

func TestPool_Stop_ReleasesBudget(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 10})
	pool := b.NewPool("test", "test pool", 3, 5)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx)

	if b.Reserved() != 3 {
		t.Errorf("after Start: Reserved() = %d, want 3", b.Reserved())
	}

	pool.Stop()
	if b.Reserved() != 0 {
		t.Errorf("after Stop: Reserved() = %d, want 0", b.Reserved())
	}

	// Stop again is idempotent
	pool.Stop()
	if b.Reserved() != 0 {
		t.Errorf("after second Stop: Reserved() = %d, want 0", b.Reserved())
	}
}

func TestPool_Start_Idempotent(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 20})
	pool := b.NewPool("test", "test pool", 2, 10)
	defer pool.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool.Start(ctx)
	pool.Start(ctx) // second Start is no-op

	var count atomic.Int32
	for i := 0; i < 4; i++ {
		_ = pool.Submit(ctx, func(context.Context) error {
			count.Add(1)
			return nil
		})
	}
	time.Sleep(50 * time.Millisecond)
	if count.Load() != 4 {
		t.Errorf("expected 4 tasks run, got %d", count.Load())
	}
}

func TestPool_NewPool_InvalidWorkerCount_ReturnsFallback(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 10})

	pool := b.NewPool("test", "test", 0, 0)
	defer pool.Stop()
	if !pool.IsFallback() {
		t.Error("NewPool(0 workers) expected fallback pool")
	}
	if pool.Size() != 1 {
		t.Errorf("fallback pool Size() = %d, want 1", pool.Size())
	}

	pool2 := b.NewPool("test", "test", -1, 0)
	defer pool2.Stop()
	if !pool2.IsFallback() {
		t.Error("NewPool(-1 workers) expected fallback pool")
	}
}

func TestPool_NewPool_BudgetExceeded_ReturnsFallback(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 5})

	pool := b.NewPool("test", "test", 10, 0)
	defer pool.Stop()
	if !pool.IsFallback() {
		t.Error("NewPool(10) with max 5 expected fallback pool")
	}
	if b.Reserved() != 0 {
		t.Errorf("after fallback NewPool: Reserved() = %d, want 0", b.Reserved())
	}
	if pool.Size() != 1 {
		t.Errorf("fallback pool Size() = %d, want 1", pool.Size())
	}
}

func TestPool_NewPool_NilBudget_ReturnsFallback(t *testing.T) {
	t.Parallel()

	pool := NewPool(nil, "test", "test pool", 4, 10)
	defer pool.Stop()
	if !pool.IsFallback() {
		t.Error("NewPool(nil budget) expected fallback pool")
	}
	if pool.Size() != 1 {
		t.Errorf("fallback pool Size() = %d, want 1", pool.Size())
	}
	ctx := context.Background()
	pool.Start(ctx)
	done := make(chan struct{})
	var n atomic.Int32
	_ = pool.Submit(ctx, func(context.Context) error { n.Add(1); close(done); return nil })
	<-done
	pool.Stop()
	if n.Load() != 1 {
		t.Errorf("fallback pool ran %d tasks, want 1", n.Load())
	}
}

func TestPool_Submit_RespectsCallerContext(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 20})
	pool := b.NewPool("test", "test pool", 2, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx)
	defer pool.Stop()

	callerCtx, callerCancel := context.WithCancel(context.Background())
	callerCancel() // cancel immediately

	err := pool.Submit(callerCtx, func(context.Context) error { return nil })
	if err != context.Canceled {
		t.Errorf("Submit with canceled context: got %v, want context.Canceled", err)
	}
}

func TestPool_ConcurrentSubmitAndStop(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 50})
	pool := b.NewPool("test", "test pool", 4, 20)

	ctx, cancel := context.WithCancel(context.Background())
	pool.Start(ctx)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		NewGoroutine("pool_test", "concurrent submit and stop").StartSimple(func() {
			defer wg.Done()
			_ = pool.Submit(ctx, func(context.Context) error {
				time.Sleep(5 * time.Millisecond)
				return nil
			})
		})
	}

	time.Sleep(20 * time.Millisecond)
	cancel()
	pool.Stop()
	wg.Wait()
}

func TestPool_WorkerRecoversFromPanic(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 10})
	pool := b.NewPool("test_panic", "worker panic recovery test", 1, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool.Start(ctx)

	// Task 1: panics intentionally
	_ = pool.Submit(ctx, func(context.Context) error {
		panic("intentional task panic for test")
	})

	// Wait briefly for panic to be recovered
	time.Sleep(20 * time.Millisecond)

	// Task 2: runs on the same worker (size=1) and should complete successfully
	var ran atomic.Bool
	err := pool.Submit(ctx, func(context.Context) error {
		ran.Store(true)
		return nil
	})
	if err != nil {
		t.Fatalf("Submit after panic: %v", err)
	}

	time.Sleep(30 * time.Millisecond)
	if !ran.Load() {
		t.Errorf("worker died after task panic; subsequent task was not processed")
	}

	pool.Stop()
}

