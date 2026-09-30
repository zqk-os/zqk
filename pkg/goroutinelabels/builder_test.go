package goroutinelabels

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestGoroutineBuilder_StartSimple(t *testing.T) {
	t.Parallel()
	var executed bool
	var mu sync.Mutex

	builder := NewGoroutine("test_goroutine", "testing simple start")
	builder.StartSimple(func() {
		mu.Lock()
		executed = true
		mu.Unlock()
	})

	// Wait a bit for goroutine to execute
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	if !executed {
		t.Error("Goroutine did not execute")
	}
	mu.Unlock()
}

func TestGoroutineBuilder_StartWithError(t *testing.T) {
	t.Parallel()
	var receivedError error
	var mu sync.Mutex

	builder := NewGoroutine("test_goroutine", "testing error handling")
	builder.WithErrorHandler(func(err error) {
		mu.Lock()
		receivedError = err
		mu.Unlock()
	})

	builder.Start(func() error {
		return errors.New("test error")
	})

	// Wait a bit for goroutine to execute
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	if receivedError == nil || receivedError.Error() != "test error" {
		t.Errorf("Expected test error, got: %v", receivedError)
	}
	mu.Unlock()
}

func TestGoroutineBuilder_StartWithContext(t *testing.T) {
	t.Parallel()
	var executed bool
	var mu sync.Mutex

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	builder := NewGoroutine("test_goroutine", "testing context cancellation")
	builder.WithContext(ctx)
	builder.StartSimple(func() {
		mu.Lock()
		executed = true
		mu.Unlock()
	})

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	if executed {
		t.Error("Goroutine should not execute when context is cancelled")
	}
	mu.Unlock()
}

func TestGoroutineBuilder_StartWithWaitGroup(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	var executed bool
	var mu sync.Mutex

	builder := NewGoroutine("test_goroutine", "testing wait group")
	builder.WithWaitGroup(&wg)
	builder.StartSimple(func() {
		mu.Lock()
		executed = true
		mu.Unlock()
	})

	// Wait for goroutine to complete
	wg.Wait()

	mu.Lock()
	if !executed {
		t.Error("Goroutine did not execute")
	}
	mu.Unlock()
}

func TestGoroutineBuilder_StartWithPanicRecovery(t *testing.T) {
	t.Parallel()
	var recoveredPanic any
	var mu sync.Mutex

	builder := NewGoroutine("test_goroutine", "testing panic recovery")
	builder.WithPanicHandler(func(r any) {
		mu.Lock()
		recoveredPanic = r
		mu.Unlock()
	})

	builder.StartSimple(func() {
		panic("test panic")
	})

	// Wait a bit for panic recovery
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	if recoveredPanic != "test panic" {
		t.Errorf("Expected panic 'test panic', got: %v", recoveredPanic)
	}
	mu.Unlock()
}

func TestGoroutineBuilder_DefaultPanicHandlerAndMetric(t *testing.T) {
	var (
		mu              sync.Mutex
		capturedName    string
		capturedPurpose string
		capturedVal     any
		capturedStack   []byte
	)

	initialPanics := GetUnhandledPanicsTotal()
	SetDefaultPanicHandler(func(name, purpose string, r any, stack []byte) {
		mu.Lock()
		defer mu.Unlock()
		capturedName = name
		capturedPurpose = purpose
		capturedVal = r
		capturedStack = stack
	})
	defer SetDefaultPanicHandler(nil)

	var wg sync.WaitGroup
	builder := NewGoroutine("worker_fault_test", "simulating unhandled panic")
	builder.WithWaitGroup(&wg)
	builder.StartSimple(func() {
		panic("unhandled worker panic")
	})

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if capturedName != "worker_fault_test" {
		t.Fatalf("expected name worker_fault_test, got %s", capturedName)
	}
	if capturedPurpose != "simulating unhandled panic" {
		t.Fatalf("expected purpose simulating unhandled panic, got %s", capturedPurpose)
	}
	if capturedVal != "unhandled worker panic" {
		t.Fatalf("expected panic value 'unhandled worker panic', got %v", capturedVal)
	}
	if len(capturedStack) == 0 {
		t.Fatalf("expected captured stack trace, got empty")
	}
	if GetUnhandledPanicsTotal() <= initialPanics {
		t.Fatalf("expected panic metric to increment, got %d <= %d", GetUnhandledPanicsTotal(), initialPanics)
	}
}

func TestGoroutineBuilder_StartWithCleanup(t *testing.T) {
	t.Parallel()
	var cleanupExecuted bool
	var mu sync.Mutex

	builder := NewGoroutine("test_goroutine", "testing cleanup")
	builder.WithCleanup(func() {
		mu.Lock()
		cleanupExecuted = true
		mu.Unlock()
	})

	var wg sync.WaitGroup
	builder.WithWaitGroup(&wg)
	builder.StartSimple(func() {
		// Do some work
	})

	wg.Wait()

	mu.Lock()
	if !cleanupExecuted {
		t.Error("Cleanup function was not executed")
	}
	mu.Unlock()
}

func TestGoroutineBuilder_StartWithContextFunction(t *testing.T) {
	t.Parallel()
	var executed bool
	var mu sync.Mutex

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	builder := NewGoroutine("test_goroutine", "testing context function")
	builder.StartWithContext(ctx, func(ctx context.Context) error {
		mu.Lock()
		executed = true
		mu.Unlock()
		return nil
	})

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	if !executed {
		t.Error("Goroutine did not execute")
	}
	mu.Unlock()
}

func TestGoroutineBuilder_StartWithResult(t *testing.T) {
	t.Parallel()
	resultChan := make(chan any, 1)

	builder := NewGoroutine("test_goroutine", "testing result channel")
	startFn := builder.StartWithResult(resultChan)

	startFn(func() (any, error) {
		return "test result", nil
	})

	// Wait for result
	select {
	case result := <-resultChan:
		if result != "test result" {
			t.Errorf("Expected 'test result', got: %v", result)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for result")
	}
}

func TestGoroutineBuilder_WithBudget_ReserveAndRelease(t *testing.T) {
	t.Parallel()
	budget := NewBudget(BudgetConfig{MaxTotal: 10})

	var wg sync.WaitGroup
	NewGoroutine("budget_goroutine", "testing budget reserve/release").
		WithBudget(budget).
		WithWaitGroup(&wg).
		StartSimple(func() {
			// Goroutine runs; budget should have 1 reserved
		})

	wg.Wait()
	// After goroutine exits, slot should be released
	if budget.Reserved() != 0 {
		t.Errorf("after goroutine exit: Reserved() = %d, want 0", budget.Reserved())
	}
}

func TestGoroutineBuilder_WithBudget_Exceeded(t *testing.T) {
	t.Parallel()
	budget := NewBudget(BudgetConfig{MaxTotal: 1})

	block := make(chan struct{})
	defer close(block)

	// First goroutine: holds the only slot
	var wg sync.WaitGroup
	NewGoroutine("budget_first", "holds slot").
		WithBudget(budget).
		WithWaitGroup(&wg).
		StartSimple(func() {
			<-block
		})

	time.Sleep(20 * time.Millisecond)
	if budget.Reserved() != 1 {
		t.Errorf("after first start: Reserved() = %d, want 1", budget.Reserved())
	}

	// Second goroutine: should not start (budget exceeded)
	var handlerCalled bool
	var mu sync.Mutex
	NewGoroutine("budget_second", "should not start").
		WithBudget(budget).
		WithBudgetExceededHandler(func() {
			mu.Lock()
			handlerCalled = true
			mu.Unlock()
		}).
		StartSimple(func() {
			t.Error("second goroutine should not run")
		})

	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	if !handlerCalled {
		t.Error("WithBudgetExceededHandler should have been called")
	}
	mu.Unlock()

	if budget.Reserved() != 1 {
		t.Errorf("Reserved() = %d, want 1 (only first goroutine)", budget.Reserved())
	}

	block <- struct{}{} // unblock first goroutine
	wg.Wait()
	if budget.Reserved() != 0 {
		t.Errorf("after first exit: Reserved() = %d, want 0", budget.Reserved())
	}
}

func TestGoroutineBuilder_WithBudget_StartWithContext(t *testing.T) {
	t.Parallel()
	budget := NewBudget(BudgetConfig{MaxTotal: 5})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup
	NewGoroutine("budget_ctx", "testing WithBudget + StartWithContext").
		WithBudget(budget).
		WithWaitGroup(&wg).
		StartWithContext(ctx, func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		})

	wg.Wait()
	if budget.Reserved() != 0 {
		t.Errorf("after goroutine exit: Reserved() = %d, want 0", budget.Reserved())
	}
}

func TestGoroutineBuilder_AsControlPlane_BypassesBudget(t *testing.T) {
	t.Parallel()
	// Budget with 0 slots available
	budget := NewBudget(BudgetConfig{MaxTotal: 1})
	release, err := budget.Reserve(1)
	if err != nil {
		t.Fatalf("failed initial reserve: %v", err)
	}
	defer release()

	ran := make(chan bool, 1)
	var wg sync.WaitGroup
	// Control plane goroutine should execute even when budget is 100% full
	NewGoroutine("control_plane_closer", "closing critical channels").
		WithBudget(budget).
		AsControlPlane().
		WithWaitGroup(&wg).
		StartSimple(func() {
			ran <- true
		})

	select {
	case <-ran:
		// Succeeded
	case <-time.After(500 * time.Millisecond):
		t.Fatal("control plane goroutine did not run when budget was full")
	}
	wg.Wait()
}

func TestGoroutineBuilder_WithBudget_ExceededWithoutHandlerRunsFallback(t *testing.T) {
	t.Parallel()
	// Budget with 1 slot
	budget := NewBudget(BudgetConfig{MaxTotal: 1})
	release, err := budget.Reserve(1)
	if err != nil {
		t.Fatalf("failed initial reserve: %v", err)
	}
	defer release()

	var fallbackNotified bool
	var mu sync.Mutex
	SetBudgetExceededFallbackNotifier(func(name, purpose, reason string) {
		mu.Lock()
		fallbackNotified = true
		mu.Unlock()
	})
	defer SetBudgetExceededFallbackNotifier(nil)

	ran := make(chan bool, 1)
	var wg sync.WaitGroup
	// When onBudgetExceeded is nil, the goroutine must NOT be dropped silently;
	// it must run in fallback mode to prevent pipeline deadlocks.
	NewGoroutine("unhandled_exceeded", "should run in fallback").
		WithBudget(budget).
		WithWaitGroup(&wg).
		StartSimple(func() {
			ran <- true
		})

	select {
	case <-ran:
		// Succeeded
	case <-time.After(500 * time.Millisecond):
		t.Fatal("goroutine was silently dropped instead of running in fallback")
	}
	wg.Wait()

	mu.Lock()
	if !fallbackNotified {
		t.Error("SetBudgetExceededFallbackNotifier was not called on fallback")
	}
	mu.Unlock()
}
