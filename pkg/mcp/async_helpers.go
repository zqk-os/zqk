package mcp

import (
	"context"
	"runtime/debug"
	"sync"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
)

// ParallelExecutor executes multiple functions in parallel and waits for all to complete
// Returns the first error encountered, if any
type ParallelExecutor struct {
	wg     sync.WaitGroup
	mu     sync.Mutex
	errors []error
}

// NewParallelExecutor creates a new parallel executor
func NewParallelExecutor() *ParallelExecutor {
	return &ParallelExecutor{
		errors: make([]error, 0),
	}
}

// Execute runs a function in parallel with panic recovery
func (pe *ParallelExecutor) Execute(fn func() error) {
	goroutinelabels.NewGoroutine("parallel_executor", "executing function in parallel executor").
		WithWaitGroup(&pe.wg).
		WithPanicHandler(func(r any) {
			stackTrace := string(debug.Stack())
			panicErr := errfmt.Errorf("panic in parallel executor: %v\n\nStack trace:\n%s", r, stackTrace)
			_ = concurrency.RunInLockWithLogger(
				&pe.mu, LockNameParallelExecutorPanic, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					pe.errors = append(pe.errors, panicErr)
					return nil
				},
			)
		}).
		WithErrorHandler(func(err error) {
			_ = concurrency.RunInLockWithLogger(
				&pe.mu, LockNameParallelExecutorError, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					pe.errors = append(pe.errors, err)
					return nil
				},
			)
		}).
		Start(func() error {
			return fn()
		})
}

// Wait waits for all parallel operations to complete and returns the first error
func (pe *ParallelExecutor) Wait() error {
	pe.wg.Wait()
	var firstErr error
	_ = concurrency.RunInLockWithLogger(
		&pe.mu, LockNameParallelExecutorWait, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if len(pe.errors) > 0 {
				firstErr = pe.errors[0]
			}
			return nil
		},
	)
	return firstErr
}

// AllErrors returns all errors encountered during parallel execution
func (pe *ParallelExecutor) AllErrors() []error {
	var errors []error
	_ = concurrency.RunInLockWithLogger(
		&pe.mu, LockNameParallelExecutorAllErrors, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			errors = pe.errors
			return nil
		},
	)
	return errors
}

// ParallelExecutorWithContext executes multiple functions in parallel with context cancellation
type ParallelExecutorWithContext struct {
	ctx    context.Context
	wg     sync.WaitGroup
	mu     sync.Mutex
	errors []error
}

// NewParallelExecutorWithContext creates a new parallel executor with context
func NewParallelExecutorWithContext(ctx context.Context) *ParallelExecutorWithContext {
	return &ParallelExecutorWithContext{
		ctx:    ctx,
		errors: make([]error, 0),
	}
}

// Execute runs a function in parallel, respecting context cancellation, with panic recovery
func (pec *ParallelExecutorWithContext) Execute(fn func(context.Context) error) {
	goroutinelabels.NewGoroutine("parallel_executor", "executing function in parallel executor").
		WithWaitGroup(&pec.wg).
		WithPanicHandler(func(r any) {
			stackTrace := string(debug.Stack())
			panicErr := errfmt.Errorf("panic in parallel executor with context: %v\n\nStack trace:\n%s", r, stackTrace)
			_ = concurrency.RunInLockWithLogger(
				&pec.mu, LockNameParallelExecutorCtxPanic, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					pec.errors = append(pec.errors, panicErr)
					return nil
				},
			)
		}).
		StartSimple(func() {
			if err := fn(pec.ctx); err != nil {
				_ = concurrency.RunInLockWithLogger(
					&pec.mu, LockNameParallelExecutorCtxError, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
					func() error {
						pec.errors = append(pec.errors, err)
						return nil
					},
				)
			}
		})
}

// Wait waits for all parallel operations to complete and returns the first error
func (pec *ParallelExecutorWithContext) Wait() error {
	pec.wg.Wait()
	var firstErr error
	_ = concurrency.RunInLockWithLogger(
		&pec.mu, LockNameParallelExecutorCtxWait, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if len(pec.errors) > 0 {
				firstErr = pec.errors[0]
			}
			return nil
		},
	)
	return firstErr
}

// AllErrors returns all errors encountered during parallel execution
func (pec *ParallelExecutorWithContext) AllErrors() []error {
	var errors []error
	_ = concurrency.RunInLockWithLogger(
		&pec.mu, LockNameParallelExecutorCtxAllErrors, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			errors = pec.errors
			return nil
		},
	)
	return errors
}

// AsyncResult represents the result of an async operation
type AsyncResult[T any] struct {
	Value T
	Error error
}

// ExecuteAsync runs a function asynchronously and returns a channel for the result
func ExecuteAsync[T any](fn func() (T, error)) <-chan AsyncResult[T] {
	resultChan := make(chan AsyncResult[T], 1)
	goroutinelabels.NewGoroutine("async_executor", "executing async function").
		WithCleanup(func() {
			close(resultChan)
		}).
		StartSimple(func() {
			value, err := fn()
			resultChan <- AsyncResult[T]{Value: value, Error: err}
		})
	return resultChan
}

// ExecuteAsyncWithContext runs a function asynchronously with context and returns a channel for the result
func ExecuteAsyncWithContext[T any](ctx context.Context, fn func(context.Context) (T, error)) <-chan AsyncResult[T] {
	resultChan := make(chan AsyncResult[T], 1)
	goroutinelabels.NewGoroutine("async_executor_with_context", "executing async function with context").
		WithCleanup(func() {
			close(resultChan)
		}).
		StartWithContext(ctx, func(ctx context.Context) error {
			value, err := fn(ctx)
			resultChan <- AsyncResult[T]{Value: value, Error: err}
			return nil
		})
	return resultChan
}
