package goroutinelabels

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestCycle155_BudgetBoundaryAndIdempotentRelease verifies budget reservation boundaries,
// edge cases, and idempotency guarantees under release cycles.
func TestCycle155_BudgetBoundaryAndIdempotentRelease(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 10})

	if max := b.MaxTotal(); max != 10 {
		t.Fatalf("expected MaxTotal 10, got %d", max)
	}

	// Boundary: Reserve 0 should succeed with a no-op release.
	releaseZero, err := b.Reserve(0)
	if err != nil {
		t.Fatalf("Reserve(0) returned unexpected error: %v", err)
	}
	releaseZero()

	// Boundary: Negative reservation request should be treated as no-op.
	releaseNeg, err := b.Reserve(-5)
	if err != nil {
		t.Fatalf("Reserve(-5) returned unexpected error: %v", err)
	}
	releaseNeg()

	// Boundary: Reserve full quota and verify idempotent release.
	release, err := b.Reserve(10)
	if err != nil {
		t.Fatalf("Reserve(10) returned unexpected error: %v", err)
	}
	if avail := b.Available(); avail != 0 {
		t.Fatalf("expected 0 available, got %d", avail)
	}

	// Calling release multiple times must be safely idempotent.
	release()
	release()
	if reserved := b.Reserved(); reserved != 0 {
		t.Fatalf("expected 0 reserved after idempotent release, got %d", reserved)
	}
	if avail := b.Available(); avail != 10 {
		t.Fatalf("expected 10 available, got %d", avail)
	}
}

// TestCycle155_BudgetAvailableClamping verifies that Available clamps to 0 if reserved exceeds maxTotal.
func TestCycle155_BudgetAvailableClamping(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 5})

	// Artificially increase reserved beyond maxTotal to exercise clamp branch.
	b.reserved.Store(10)
	if avail := b.Available(); avail != 0 {
		t.Fatalf("expected Available() to clamp to 0 when reserved > maxTotal, got %d", avail)
	}

	// Reset reserved back to 0.
	b.reserved.Store(0)
	if avail := b.Available(); avail != 5 {
		t.Fatalf("expected 5 available, got %d", avail)
	}
}

// TestCycle155_DefaultBudgetLifecycle verifies SetDefaultBudget and DefaultBudget accessor lifecycle.
func TestCycle155_DefaultBudgetLifecycle(t *testing.T) {
	// Not parallel to avoid global defaultBudget race with other tests.
	original := DefaultBudget()
	defer SetDefaultBudget(original)

	custom := NewBudget(BudgetConfig{MaxTotal: 42})
	SetDefaultBudget(custom)
	if got := DefaultBudget(); got != custom {
		t.Fatalf("expected custom budget %p, got %p", custom, got)
	}

	SetDefaultBudget(nil)
	if got := DefaultBudget(); got != nil {
		t.Fatalf("expected nil default budget, got %p", got)
	}
}

// TestCycle155_PanicHandlerAndCounterTelemetry verifies panic counters and global panic handler callbacks.
func TestCycle155_PanicHandlerAndCounterTelemetry(t *testing.T) {
	// Not parallel to avoid global unhandled panics and default handler collisions.
	ResetUnhandledPanicsTotal()
	if total := GetUnhandledPanicsTotal(); total != 0 {
		t.Fatalf("expected 0 unhandled panics after reset, got %d", total)
	}

	var customHandlerCalled atomic.Bool
	var capturedName, capturedPurpose string
	var capturedVal any
	var mu sync.Mutex

	handler := func(name, purpose string, r any, stack []byte) {
		mu.Lock()
		defer mu.Unlock()
		capturedName = name
		capturedPurpose = purpose
		capturedVal = r
		customHandlerCalled.Store(true)
	}

	SetDefaultPanicHandler(handler)
	defer SetDefaultPanicHandler(nil)

	var wg sync.WaitGroup
	builder := NewGoroutine("telemetry_worker", "testing panic capture")
	builder.WithWaitGroup(&wg)
	builder.StartSimple(func() {
		panic("telemetry boom")
	})
	wg.Wait()

	if !customHandlerCalled.Load() {
		t.Fatal("expected default panic handler to be invoked")
	}

	mu.Lock()
	if capturedName != "telemetry_worker" || capturedPurpose != "testing panic capture" || capturedVal != "telemetry boom" {
		t.Fatalf("unexpected panic capture values: name=%s purpose=%s val=%v", capturedName, capturedPurpose, capturedVal)
	}
	mu.Unlock()

	if total := GetUnhandledPanicsTotal(); total != 1 {
		t.Fatalf("expected 1 unhandled panic recorded, got %d", total)
	}

	ResetUnhandledPanicsTotal()
	if total := GetUnhandledPanicsTotal(); total != 0 {
		t.Fatalf("expected 0 unhandled panics after final reset, got %d", total)
	}
}

// TestCycle155_StartWithResult_ErrorAndPanicBranches exercises error dispatch, panic propagation,
// and nil channel variations of StartWithResult.
func TestCycle155_StartWithResult_ErrorAndPanicBranches(t *testing.T) {
	t.Parallel()

	// 1. Error return with non-nil resultChan and custom onError
	var errCaptured atomic.Bool
	expectedErr := errors.New("deliberate task failure")
	resChan := make(chan any, 1)

	bErr := NewGoroutine("result_err_worker", "testing error branch")
	bErr.WithErrorHandler(func(err error) {
		if errors.Is(err, expectedErr) {
			errCaptured.Store(true)
		}
	})

	fnErr := bErr.StartWithResult(resChan)
	fnErr(func() (any, error) {
		return nil, expectedErr
	})

	select {
	case res := <-resChan:
		err, ok := res.(error)
		if !ok || !errors.Is(err, expectedErr) {
			t.Fatalf("expected %v on resChan, got %v", expectedErr, res)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for error on resChan")
	}
	if !errCaptured.Load() {
		t.Fatal("expected onError handler to be executed")
	}

	// 2. Error return with nil resultChan
	var nilChanErrCaptured atomic.Bool
	bNil := NewGoroutine("result_nil_worker", "testing nil channel")
	bNil.WithErrorHandler(func(err error) {
		if errors.Is(err, expectedErr) {
			nilChanErrCaptured.Store(true)
		}
	})
	fnNil := bNil.StartWithResult(nil)
	fnNil(func() (any, error) {
		return nil, expectedErr
	})
	time.Sleep(30 * time.Millisecond)
	if !nilChanErrCaptured.Load() {
		t.Fatal("expected onError handler to execute even when resultChan is nil")
	}
}

// TestCycle155_StartWithResult_ContextCancellationAndNilChan verifies StartWithResult when context
// is cancelled and resultChan is nil.
func TestCycle155_StartWithResult_ContextCancellationAndNilChan(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var wg sync.WaitGroup
	b := NewGoroutine("ctx_nil_worker", "testing cancelled context with nil chan")
	b.WithContext(ctx)
	b.WithWaitGroup(&wg)

	fn := b.StartWithResult(nil)
	fn(func() (any, error) {
		t.Error("task function should not run when context is already cancelled")
		return "unexpected", nil
	})
	wg.Wait()
}

// TestCycle155_StartWithResult_PanicWithNilChan verifies StartWithResult panic recovery when resultChan is nil.
func TestCycle155_StartWithResult_PanicWithNilChan(t *testing.T) {
	t.Parallel()

	var panicHandled atomic.Bool
	var wg sync.WaitGroup
	b := NewGoroutine("panic_nil_worker", "testing panic with nil chan")
	b.WithWaitGroup(&wg)
	b.WithPanicHandler(func(r any) {
		panicHandled.Store(true)
	})

	fn := b.StartWithResult(nil)
	fn(func() (any, error) {
		panic("panic with nil channel")
	})
	wg.Wait()

	if !panicHandled.Load() {
		t.Fatal("expected panic handler to be invoked when resultChan is nil")
	}
}

// TestCycle155_WorkerPool_FallbackCreationAndNotifications verifies fallback pool instantiation
// when budget is nil, worker count is invalid, or budget is exhausted.
func TestCycle155_WorkerPool_FallbackCreationAndNotifications(t *testing.T) {
	var declineCount atomic.Int32
	notifier := func(name, purpose, reason string) {
		declineCount.Add(1)
	}
	SetPoolCreationDeclinedNotifier(notifier)
	defer SetPoolCreationDeclinedNotifier(nil)

	// Case 1: Nil budget passed to NewPool
	pNil := NewPool(nil, "fallback_nil_pool", "no budget", 4, 10)
	if !pNil.IsFallback() {
		t.Fatal("expected fallback pool when budget is nil")
	}
	if pNil.Size() != 1 {
		t.Fatalf("expected fallback pool size 1, got %d", pNil.Size())
	}
	pNil.Stop()

	// Case 2: Zero or negative workerCount
	b := NewBudget(BudgetConfig{MaxTotal: 10})
	pZero := b.NewPool("fallback_zero_pool", "zero workers", 0, -2)
	if !pZero.IsFallback() {
		t.Fatal("expected fallback pool when workerCount <= 0")
	}
	pZero.Stop()

	// Case 3: Exceeding budget capacity
	bTight := NewBudget(BudgetConfig{MaxTotal: 2})
	pExceed := bTight.NewPool("fallback_exceed_pool", "budget exceeded", 5, 2)
	if !pExceed.IsFallback() {
		t.Fatal("expected fallback pool when budget is exceeded")
	}
	pExceed.Stop()

	if got := declineCount.Load(); got != 3 {
		t.Fatalf("expected 3 decline notifications, got %d", got)
	}
}

// TestCycle155_WorkerPool_AccessorsAndDiagnostics verifies Name, QueuePressureSnapshot,
// and nil pointer safety on Pool methods.
func TestCycle155_WorkerPool_AccessorsAndDiagnostics(t *testing.T) {
	t.Parallel()

	// Nil pool receiver safety
	var nilPool *Pool
	if name := nilPool.Name(); name != "" {
		t.Fatalf("expected empty string for nil pool Name(), got %q", name)
	}
	w, q, capVal := nilPool.QueuePressureSnapshot()
	if w != 0 || q != 0 || capVal != 0 {
		t.Fatalf("expected (0,0,0) for nil pool QueuePressureSnapshot(), got (%d,%d,%d)", w, q, capVal)
	}

	// Normal pool metrics and accessors
	b := NewBudget(BudgetConfig{MaxTotal: 10})
	pool := b.NewPool("diag_pool", "testing diagnostics", 3, 7)
	defer pool.Stop()

	if pool.Name() != "diag_pool" {
		t.Fatalf("expected pool name 'diag_pool', got %q", pool.Name())
	}
	if pool.Size() != 3 {
		t.Fatalf("expected size 3, got %d", pool.Size())
	}

	w, q, capVal = pool.QueuePressureSnapshot()
	if w != 3 || q != 0 || capVal != 7 {
		t.Fatalf("expected (3,0,7) queue pressure snapshot, got (%d,%d,%d)", w, q, capVal)
	}
}

// TestCycle155_WorkerPool_LifecycleEdgeCases exercises Stop on unstarted pool, idempotent Stop calls,
// and redundant Start calls.
func TestCycle155_WorkerPool_LifecycleEdgeCases(t *testing.T) {
	t.Parallel()

	b := NewBudget(BudgetConfig{MaxTotal: 5})
	p := b.NewPool("lifecycle_pool", "testing edge cases", 2, 4)

	// Pool has reserved 2 slots before start.
	if b.Reserved() != 2 {
		t.Fatalf("expected 2 reserved slots, got %d", b.Reserved())
	}

	// Calling Stop on unstarted pool releases budget reservation directly.
	p.Stop()
	if b.Reserved() != 0 {
		t.Fatalf("expected 0 reserved slots after stopping unstarted pool, got %d", b.Reserved())
	}

	// Second Stop call must be safely idempotent.
	p.Stop()

	// Calling Start on an already stopped pool must be a safe no-op.
	p.Start(context.Background())

	// Start on fresh pool, then redundant Start.
	p2 := b.NewPool("redundant_start_pool", "testing redundant start", 2, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p2.Start(ctx)
	p2.Start(ctx) // Redundant start no-op.
	p2.Stop()
	p2.Stop() // Redundant stop no-op.
}

// TestCycle155_WorkerPool_SubmitContextCancellation verifies Submit error paths when
// context is pre-cancelled or cancelled during queue wait.
func TestCycle155_WorkerPool_SubmitContextCancellation(t *testing.T) {
	t.Parallel()

	b := NewBudget(BudgetConfig{MaxTotal: 5})
	p := b.NewPool("submit_ctx_pool", "testing submit cancel", 1, 0) // unbuffered queue
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Start(ctx)
	defer p.Stop()

	// Pre-cancelled context must fail immediately without enqueueing.
	preCancelledCtx, preCancel := context.WithCancel(context.Background())
	preCancel()
	err := p.Submit(preCancelledCtx, func(c context.Context) error {
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled for pre-cancelled Submit, got %v", err)
	}

	// Occupy the single worker with a blocking job.
	workerBusy := make(chan struct{})
	workerBlock := make(chan struct{})
	submitErr := p.Submit(ctx, func(c context.Context) error {
		close(workerBusy)
		<-workerBlock
		return nil
	})
	if submitErr != nil {
		t.Fatalf("unexpected error submitting initial work: %v", submitErr)
	}
	<-workerBusy
	defer close(workerBlock)

	// Submit with timeout on unbuffered channel where worker is busy.
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer waitCancel()
	errWait := p.Submit(waitCtx, func(c context.Context) error {
		return nil
	})
	if !errors.Is(errWait, context.DeadlineExceeded) && !errors.Is(errWait, context.Canceled) {
		t.Fatalf("expected context timeout error, got %v", errWait)
	}
}

// TestCycle155_WorkerPool_SubmitNonBlockingOverflow exercises SubmitNonBlocking queue saturation
// and ErrPoolFull return value.
func TestCycle155_WorkerPool_SubmitNonBlockingOverflow(t *testing.T) {
	t.Parallel()

	b := NewBudget(BudgetConfig{MaxTotal: 5})
	// Buffer size 1 with 1 worker.
	p := b.NewPool("overflow_pool", "testing overflow", 1, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Start(ctx)
	defer p.Stop()

	blockWorker := make(chan struct{})
	workerStarted := make(chan struct{})

	// 1. Occupy the worker.
	err := p.Submit(ctx, func(c context.Context) error {
		close(workerStarted)
		<-blockWorker
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error occupying worker: %v", err)
	}
	<-workerStarted
	defer close(blockWorker)

	// 2. Fill the 1-slot queue.
	err = p.SubmitNonBlocking(ctx, func(c context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected non-blocking enqueue to succeed for queue slot 1, got %v", err)
	}

	// 3. Next SubmitNonBlocking should return ErrPoolFull.
	err = p.SubmitNonBlocking(ctx, func(c context.Context) error {
		return nil
	})
	if !errors.Is(err, ErrPoolFull) {
		t.Fatalf("expected ErrPoolFull when queue is saturated, got %v", err)
	}

	// 4. Submit to unstarted or stopped pool returns context.Canceled.
	stoppedPool := b.NewPool("stopped_pool", "testing stopped submit", 1, 1)
	err = stoppedPool.SubmitNonBlocking(ctx, func(c context.Context) error {
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled when submitting to unstarted pool, got %v", err)
	}
}

// TestCycle155_BuilderBudgetExceededFallbackNotifier exercises WithBudget fallback notifications
// and custom budget exceeded handlers.
func TestCycle155_BuilderBudgetExceededFallbackNotifier(t *testing.T) {
	var fallbackNotified atomic.Bool
	SetBudgetExceededFallbackNotifier(func(name, purpose, reason string) {
		if name == "budget_fallback_worker" && purpose == "testing fallback notification" {
			fallbackNotified.Store(true)
		}
	})
	defer SetBudgetExceededFallbackNotifier(nil)

	// Budget with maxTotal=1 already reserved.
	b := NewBudget(BudgetConfig{MaxTotal: 1})
	release, err := b.Reserve(1)
	if err != nil {
		t.Fatalf("initial reserve failed: %v", err)
	}
	defer release()

	// 1. With custom onBudgetExceeded handler.
	var customExceededCalled atomic.Bool
	workerWithHandler := NewGoroutine("budget_exceeded_worker", "testing budget handler")
	workerWithHandler.WithBudget(b)
	workerWithHandler.WithBudgetExceededHandler(func() {
		customExceededCalled.Store(true)
	})

	var taskExecuted atomic.Bool
	workerWithHandler.StartSimple(func() {
		taskExecuted.Store(true)
	})
	time.Sleep(20 * time.Millisecond)

	if taskExecuted.Load() {
		t.Fatal("expected goroutine to abort when onBudgetExceeded handler is provided")
	}
	if !customExceededCalled.Load() {
		t.Fatal("expected custom budget exceeded handler to be called")
	}

	// 2. Without custom onBudgetExceeded handler -> triggers emergency fallback run and notifier.
	var wg sync.WaitGroup
	workerFallback := NewGoroutine("budget_fallback_worker", "testing fallback notification")
	workerFallback.WithBudget(b)
	workerFallback.WithWaitGroup(&wg)

	var fallbackExecuted atomic.Bool
	workerFallback.StartSimple(func() {
		fallbackExecuted.Store(true)
	})
	wg.Wait()

	if !fallbackExecuted.Load() {
		t.Fatal("expected goroutine to execute using fallback when no onBudgetExceeded handler is set")
	}
	if !fallbackNotified.Load() {
		t.Fatal("expected global budget exceeded fallback notifier to be called")
	}
}

// TestCycle155_ControlPlaneAndCleanupFlags verifies AsControlPlane and AsCleanup configuration
// bypasses worker budget restrictions.
func TestCycle155_ControlPlaneAndCleanupFlags(t *testing.T) {
	t.Parallel()

	b1 := NewGoroutine("control_plane_worker", "testing cp flag").AsControlPlane()
	if !b1.isControlPlane {
		t.Fatal("expected isControlPlane to be true")
	}

	b2 := NewGoroutine("cleanup_worker", "testing cleanup flag").AsCleanup()
	if !b2.isControlPlane {
		t.Fatal("expected isControlPlane to be true for AsCleanup()")
	}

	// Verify control plane goroutine executes even when budget is exhausted.
	exhaustedBudget := NewBudget(BudgetConfig{MaxTotal: 1})
	release, err := exhaustedBudget.Reserve(1)
	if err != nil {
		t.Fatalf("exhaust reserve failed: %v", err)
	}
	defer release()

	var wg sync.WaitGroup
	var ran atomic.Bool

	cpWorker := NewGoroutine("cp_exec", "testing cp execution")
	cpWorker.WithBudget(exhaustedBudget)
	cpWorker.AsControlPlane()
	cpWorker.WithWaitGroup(&wg)
	cpWorker.StartSimple(func() {
		ran.Store(true)
	})
	wg.Wait()

	if !ran.Load() {
		t.Fatal("control plane goroutine must bypass budget exhaustion")
	}
}
