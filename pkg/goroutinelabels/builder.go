package goroutinelabels

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// PanicHandlerFunc defines the signature for custom panic handling with stack traces.
type PanicHandlerFunc func(name, purpose string, r any, stack []byte)

var (
	unhandledPanicsTotal atomic.Int64
	defaultPanicHandler  atomic.Pointer[PanicHandlerFunc]
)

// GetUnhandledPanicsTotal returns the count of all recovered panics across goroutines.
func GetUnhandledPanicsTotal() int64 {
	return unhandledPanicsTotal.Load()
}

// ResetUnhandledPanicsTotal resets the recovered panics counter (primarily for test assertions).
func ResetUnhandledPanicsTotal() {
	unhandledPanicsTotal.Store(0)
}

// SetDefaultPanicHandler configures a global fallback handler for goroutine panics.
func SetDefaultPanicHandler(handler PanicHandlerFunc) {
	if handler == nil {
		defaultPanicHandler.Store(nil)
	} else {
		defaultPanicHandler.Store(&handler)
	}
}

func handlePanic(name, purpose string, onPanic func(any), r any) {
	unhandledPanicsTotal.Add(1)
	if onPanic != nil {
		onPanic(r)
		return
	}
	stack := debug.Stack()
	if fn := defaultPanicHandler.Load(); fn != nil && *fn != nil {
		(*fn)(name, purpose, r, stack)
		return
	}
	// Fail-closed observability: never swallow panics silently
	_, _ = fmt.Fprintf(os.Stderr, "goroutinelabels: unhandled panic in goroutine %q (%s): %v\n%s\n", name, purpose, r, string(stack))
}

// recoverGoroutine recovers from an active panic, reporting it to telemetry and custom handlers.
func recoverGoroutine(name, purpose string, onPanic func(any), onRecovered ...func(r any)) {
	if r := recover(); r != nil {
		handlePanic(name, purpose, onPanic, r)
		for _, fn := range onRecovered {
			if fn != nil {
				fn(r)
			}
		}
	}
}

// deferDone returns a function that guarantees wg.Done() is executed at most once.
func deferDone(wg *sync.WaitGroup) func() {
	if wg == nil {
		return func() {}
	}
	var once sync.Once
	return func() {
		once.Do(wg.Done)
	}
}

// GoroutineBuilder provides a fluent API for creating goroutines with consistent patterns.
// This ensures all goroutines have labels and follow best practices for:
// - Automatic label setting for profiling
// - Panic recovery
// - Context cancellation support
// - Wait group integration
// - Cleanup functions
//
// Example usage:
//
//	// Simple goroutine
//	NewGoroutine("worker_1", "processing tasks").
//		StartSimple(func() {
//			// do work
//		})
//
//	// With context and error handling
//	NewGoroutine("api_handler", "handling API request").
//		WithContext(ctx).
//		WithErrorHandler(func(err error) {
//			log.Error("goroutine error", err)
//		}).
//		Start(func() error {
//			return processRequest()
//		})
//
//	// With wait group
//	var wg sync.WaitGroup
//	NewGoroutine("background_worker", "background processing").
//		WithWaitGroup(&wg).
//		WithCleanup(func() {
//			close(results)
//		}).
//		StartSimple(func() {
//			// do work
//		})
//	wg.Wait()

// GoroutineBuilder provides a fluent API for creating goroutines with consistent patterns
// This ensures all goroutines have labels and follow best practices
type GoroutineBuilder struct {
	name             string
	purpose          string
	ctx              context.Context
	onPanic          func(any)
	onError          func(error)
	waitGroup        *sync.WaitGroup
	signalOnExit     chan bool // If set, one true is sent when goroutine exits (any path); use buffered channel
	cleanupFunc      func()
	preCleanup       func()      // Runs before standard cleanup
	postCleanup      func()      // Runs after standard cleanup
	shutdownCheck    func() bool // Optional shutdown check function
	budget           *Budget     // If set, Reserve(1) before start and release on exit
	onBudgetExceeded func()      // Called when budget is exceeded and goroutine is not started
	isControlPlane   bool        // If true, control-plane/cleanup/coordinator: never throttled by worker budget
}

var (
	BudgetExceededFallbackNotifier   func(name, purpose, reason string)
	budgetExceededFallbackNotifierMu sync.RWMutex
)

// SetBudgetExceededFallbackNotifier sets the notifier invoked when a goroutine budget
// is exceeded but no onBudgetExceeded handler is provided, causing an emergency fallback run.
func SetBudgetExceededFallbackNotifier(notifier func(name, purpose, reason string)) {
	budgetExceededFallbackNotifierMu.Lock()
	defer budgetExceededFallbackNotifierMu.Unlock()
	BudgetExceededFallbackNotifier = notifier
}

func notifyBudgetExceededFallback(name, purpose, reason string) {
	budgetExceededFallbackNotifierMu.RLock()
	notifier := BudgetExceededFallbackNotifier
	budgetExceededFallbackNotifierMu.RUnlock()
	if notifier != nil {
		notifier(name, purpose, reason)
	}
}

// NewGoroutine creates a new goroutine builder
// name: Short identifier for the goroutine (e.g., "validation_worker_1")
// purpose: Description of what the goroutine does (e.g., "processing validation queue")
func NewGoroutine(name, purpose string) *GoroutineBuilder {
	return &GoroutineBuilder{
		name:    name,
		purpose: purpose,
	}
}

// WithLifecycleContext sets a context for lifecycle management (cancellation checks).
// This is used with Start() and StartSimple() to enable early exit if context is cancelled.
// For StartWithContext(), pass the context directly as a parameter instead.
//
// The goroutine will check ctx.Done() before executing the function.
// If context is cancelled, the function won't run.
//
// Example with Start():
//
//	goroutinelabels.NewGoroutine("worker", "processing").
//		WithLifecycleContext(ctx).
//		Start(func() error {
//			return doWork()
//		})
func (b *GoroutineBuilder) WithLifecycleContext(ctx context.Context) *GoroutineBuilder {
	b.ctx = ctx
	return b
}

// WithContext is deprecated. Use WithLifecycleContext() for Start()/StartSimple(),
// or pass context directly to StartWithContext().
// This method is kept for backward compatibility but will be removed in a future version.
func (b *GoroutineBuilder) WithContext(ctx context.Context) *GoroutineBuilder {
	return b.WithLifecycleContext(ctx)
}

// WithPanicHandler sets a custom panic handler
// If not set, panics are recovered and ignored (with label set for debugging)
// The handler receives the recovered panic value
func (b *GoroutineBuilder) WithPanicHandler(handler func(any)) *GoroutineBuilder {
	b.onPanic = handler
	return b
}

// WithErrorHandler sets a custom error handler
// If the function returns an error, this handler is called
// If not set, errors are ignored
func (b *GoroutineBuilder) WithErrorHandler(handler func(error)) *GoroutineBuilder {
	b.onError = handler
	return b
}

// WithWaitGroup adds the goroutine to a wait group
// The wait group is incremented before starting and decremented on exit.
// Callers must NOT call wg.Add(1) before WithWaitGroup—the builder does it.
func (b *GoroutineBuilder) WithWaitGroup(wg *sync.WaitGroup) *GoroutineBuilder {
	b.waitGroup = wg
	return b
}

// WithSignalOnExit ensures the given channel receives one value when the goroutine exits
// (normal return, early return, or panic). Use with a buffered channel (e.g. make(chan bool, 1))
// so the send never blocks. Enables "wait for goroutine to finish" patterns without
// requiring the work function to signal on every exit path.
func (b *GoroutineBuilder) WithSignalOnExit(doneCh chan bool) *GoroutineBuilder {
	b.signalOnExit = doneCh
	return b
}

// WithCleanup sets a cleanup function to run when the goroutine exits
// Useful for closing channels, stopping tickers, etc.
//
// IMPORTANT: Cleanup functions are CONTEXT-AGNOSTIC - they must complete even if
// the goroutine's context is cancelled. For operations requiring lock acquisition
// or other blocking operations, use context.Background() to ensure they complete
// regardless of the parent context state.
func (b *GoroutineBuilder) WithCleanup(cleanup func()) *GoroutineBuilder {
	b.cleanupFunc = cleanup
	return b
}

// WithPreCleanup sets a cleanup function that runs BEFORE the standard cleanup
// Useful for cleanup that must happen first (e.g., releasing locks)
//
// IMPORTANT: Cleanup functions are CONTEXT-AGNOSTIC - they must complete even if
// the goroutine's context is cancelled. For operations requiring lock acquisition
// or other blocking operations, use context.Background() to ensure they complete
// regardless of the parent context state.
func (b *GoroutineBuilder) WithPreCleanup(cleanup func()) *GoroutineBuilder {
	b.preCleanup = cleanup
	return b
}

// WithPostCleanup sets a cleanup function that runs AFTER the standard cleanup
// Useful for final cleanup steps (e.g., updating status, emitting events)
//
// IMPORTANT: Cleanup functions are CONTEXT-AGNOSTIC - they must complete even if
// the goroutine's context is cancelled. For operations requiring lock acquisition
// or other blocking operations, use context.Background() to ensure they complete
// regardless of the parent context state.
//
// Example:
//
//	WithPostCleanup(func() {
//	    // Update status - must complete even if context cancelled
//	    _ = concurrency.WithLockTimeout(
//	        &manager.mu,
//	        context.Background(), // Context-agnostic
//	        nil, logger, "update_status",
//	        func() error {
//	            manager.status = "stopped"
//	            return nil
//	        },
//	    )
//	})
func (b *GoroutineBuilder) WithPostCleanup(cleanup func()) *GoroutineBuilder {
	b.postCleanup = cleanup
	return b
}

// WithShutdownCheck sets a function to check if shutdown is in progress
// If the function returns true, the goroutine will exit immediately
func (b *GoroutineBuilder) WithShutdownCheck(check func() bool) *GoroutineBuilder {
	b.shutdownCheck = check
	return b
}

// WithBudget reserves one goroutine slot from the budget when this goroutine starts
// and releases it when the goroutine exits. If Reserve(1) fails (budget exceeded),
// the goroutine is not started. Use WithBudgetExceededHandler to react when that happens.
func (b *GoroutineBuilder) WithBudget(budget *Budget) *GoroutineBuilder {
	b.budget = budget
	return b
}

// WithBudgetExceededHandler sets a function called when WithBudget is set and Reserve(1) fails.
// The goroutine is not started in that case.
func (b *GoroutineBuilder) WithBudgetExceededHandler(fn func()) *GoroutineBuilder {
	b.onBudgetExceeded = fn
	return b
}

// AsControlPlane marks this goroutine as an essential control-plane, lifecycle,
// coordinator, or cleanup task. Such goroutines are guaranteed to execute and will
// not be throttled or dropped by data-plane worker budgets.
func (b *GoroutineBuilder) AsControlPlane() *GoroutineBuilder {
	b.isControlPlane = true
	return b
}

// AsCleanup marks this goroutine as a cleanup or finalizer task, ensuring it will
// not be dropped by worker budget exhaustion.
func (b *GoroutineBuilder) AsCleanup() *GoroutineBuilder {
	return b.AsControlPlane()
}

// Start starts the goroutine with a function that returns an error
// The function will have labels set and panic recovery enabled
// WaitGroup Done() is called when the goroutine exits (completion or early exit)
func (b *GoroutineBuilder) Start(fn func() error) {
	// Capture builder state before starting goroutine to avoid race conditions if builder is reused
	wg := b.waitGroup
	preCleanup := b.preCleanup
	cleanupFunc := b.cleanupFunc
	postCleanup := b.postCleanup
	onPanic := b.onPanic
	onError := b.onError
	shutdownCheck := b.shutdownCheck
	ctx := b.ctx
	name := b.name
	purpose := b.purpose
	budget := b.budget
	onBudgetExceeded := b.onBudgetExceeded
	isControlPlane := b.isControlPlane

	var releaseBudget func()
	if budget != nil && !isControlPlane {
		release, err := budget.Reserve(1)
		if err != nil {
			if onBudgetExceeded != nil {
				onBudgetExceeded()
				return
			}
			notifyBudgetExceededFallback(name, purpose, err.Error())
		} else {
			releaseBudget = release
		}
	}

	// Add to wait group before starting goroutine
	if wg != nil {
		wg.Add(1)
	}

	go func() {
		if releaseBudget != nil {
			defer releaseBudget()
		}
		defer deferDone(wg)()
		defer recoverGoroutine(name, purpose, onPanic)

		// Always set goroutine label first
		SetGoroutineLabel(name, purpose)

		// Pre-cleanup
		if preCleanup != nil {
			defer preCleanup()
		}

		// Check shutdown before executing - if shutdown, exit early (Done() called in defer)
		if shutdownCheck != nil && shutdownCheck() {
			return // Shutdown ordered, exit immediately
		}

		// Check context before executing - if cancelled, exit early (Done() called in defer)
		if ctx != nil {
			select {
			case <-ctx.Done():
				// Context cancelled before execution - exit early, Done() will be called in defer
				return
			default:
			}
		}

		// Execute the function
		var err error
		func() {
			defer func() {
				if cleanupFunc != nil {
					cleanupFunc()
				}
			}()
			err = fn()
		}()

		// Handle error
		if err != nil {
			if onError != nil {
				onError(err)
			}
		}

		// Post-cleanup after everything
		if postCleanup != nil {
			postCleanup()
		}
	}()
}

// StartWithContext starts the goroutine with a function that takes a context.
// The context is checked before execution and passed to the function.
// This method accepts the context directly as a parameter for clarity.
// WaitGroup Done() is called when the goroutine exits (completion or early exit).
//
// Example:
//
//	goroutinelabels.NewGoroutine("worker", "processing").
//		StartWithContext(ctx, func(ctx context.Context) error {
//			return doWork(ctx)
//		})
func (b *GoroutineBuilder) StartWithContext(ctx context.Context, fn func(ctx context.Context) error) {
	// Capture builder state before starting goroutine to avoid race conditions if builder is reused
	wg := b.waitGroup
	preCleanup := b.preCleanup
	cleanupFunc := b.cleanupFunc
	postCleanup := b.postCleanup
	onPanic := b.onPanic
	onError := b.onError
	name := b.name
	purpose := b.purpose
	budget := b.budget
	onBudgetExceeded := b.onBudgetExceeded
	isControlPlane := b.isControlPlane

	var releaseBudget func()
	if budget != nil && !isControlPlane {
		release, err := budget.Reserve(1)
		if err != nil {
			if onBudgetExceeded != nil {
				onBudgetExceeded()
				return
			}
			notifyBudgetExceededFallback(name, purpose, err.Error())
		} else {
			releaseBudget = release
		}
	}

	// Add to wait group before starting goroutine
	if wg != nil {
		wg.Add(1)
	}

	go func() {
		if releaseBudget != nil {
			defer releaseBudget()
		}
		defer deferDone(wg)()
		defer recoverGoroutine(name, purpose, onPanic)

		// Always set goroutine label first
		SetGoroutineLabel(name, purpose)

		// Pre-cleanup
		if preCleanup != nil {
			defer preCleanup()
		}

		// Check context before executing - if cancelled, exit early (Done() called in defer)
		select {
		case <-ctx.Done():
			// Context cancelled before execution - exit early, Done() will be called in defer
			return
		default:
		}

		// Execute the function with context
		var err error
		func() {
			defer func() {
				if cleanupFunc != nil {
					cleanupFunc()
				}
			}()
			err = fn(ctx)
		}()

		// Handle error
		if err != nil {
			if onError != nil {
				onError(err)
			}
		}

		// Post-cleanup after everything
		if postCleanup != nil {
			postCleanup()
		}
	}()
}

// StartSimple starts the goroutine with a simple function (no return value)
// Useful for fire-and-forget goroutines
// WaitGroup Done() is called when the goroutine exits (completion or early exit)
func (b *GoroutineBuilder) StartSimple(fn func()) {
	// Capture waitGroup reference and other builder state before starting goroutine
	// to avoid race conditions if builder is reused
	wg := b.waitGroup
	signalOnExit := b.signalOnExit
	preCleanup := b.preCleanup
	cleanupFunc := b.cleanupFunc
	postCleanup := b.postCleanup
	onPanic := b.onPanic
	shutdownCheck := b.shutdownCheck
	ctx := b.ctx
	name := b.name
	purpose := b.purpose
	budget := b.budget
	onBudgetExceeded := b.onBudgetExceeded
	isControlPlane := b.isControlPlane

	var releaseBudget func()
	if budget != nil && !isControlPlane {
		release, err := budget.Reserve(1)
		if err != nil {
			if onBudgetExceeded != nil {
				onBudgetExceeded()
				return
			}
			notifyBudgetExceededFallback(name, purpose, err.Error())
		} else {
			releaseBudget = release
		}
	}

	// Add to wait group before starting goroutine
	if wg != nil {
		wg.Add(1)
	}

	go func() {
		if releaseBudget != nil {
			defer releaseBudget()
		}
		defer deferDone(wg)()
		defer recoverGoroutine(name, purpose, onPanic)

		// Always set goroutine label first
		SetGoroutineLabel(name, purpose)

		// Signal completion on any exit (so tests can wait without manual signals in every return path)
		if signalOnExit != nil {
			defer func() {
				select {
				case signalOnExit <- true:
				default:
				}
			}()
		}

		// Pre-cleanup
		if preCleanup != nil {
			defer preCleanup()
		}

		// Check shutdown before executing - if shutdown, exit early (Done() called in defer)
		if shutdownCheck != nil && shutdownCheck() {
			return // Shutdown ordered, exit immediately
		}

		// Check context before executing - if cancelled, exit early (Done() called in defer)
		if ctx != nil {
			select {
			case <-ctx.Done():
				// Context cancelled before execution - exit early, Done() will be called in defer
				return
			default:
			}
		}

		// Execute the function
		func() {
			defer func() {
				if cleanupFunc != nil {
					cleanupFunc()
				}
			}()
			fn()
		}()

		// Post-cleanup after everything
		if postCleanup != nil {
			postCleanup()
		}
	}()
}

// StartWithResult starts the goroutine and sends the result to a channel
// Useful for collecting results from multiple goroutines
func (b *GoroutineBuilder) StartWithResult(resultChan chan<- any) func(func() (any, error)) {
	return func(fn func() (any, error)) {
		if b.waitGroup != nil {
			b.waitGroup.Add(1)
		}

		go func() {
			// Always set goroutine label first
			SetGoroutineLabel(b.name, b.purpose)

			defer deferDone(b.waitGroup)()
			if b.cleanupFunc != nil {
				defer b.cleanupFunc()
			}
			defer recoverGoroutine(b.name, b.purpose, b.onPanic, func(r any) {
				if resultChan != nil {
					resultChan <- errfmt.Errorf("panic in goroutine %s: %v", b.name, r)
				}
			})

			// Check context before executing
			if b.ctx != nil {
				select {
				case <-b.ctx.Done():
					// Context cancelled, send error
					if resultChan != nil {
						resultChan <- b.ctx.Err()
					}
					return
				default:
				}
			}

			// Execute the function and send result
			result, err := fn()
			if err != nil {
				if b.onError != nil {
					b.onError(err)
				}
				if resultChan != nil {
					resultChan <- err
				}
			} else if resultChan != nil {
				resultChan <- result
			}
		}()
	}
}
