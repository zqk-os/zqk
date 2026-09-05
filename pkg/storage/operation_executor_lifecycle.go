package storage

import (
	"context"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
)

// NewOperationExecutor creates a new operation executor with enhanced features (timeout, retry, cache)
// ctx: parent context from command entry point (should not be created here)
func NewOperationExecutor(
	ctx context.Context,
	storage ObjectStorageProvider,
	queue *OperationQueue,
	workers int,
	conflictResolver ConflictResolver,
) *OperationExecutor {
	return NewOperationExecutorWithConfig(ctx, storage, queue, workers, conflictResolver, 0, nil)
}

// NewOperationExecutorWithConfig creates an operation executor with custom configuration
// ctx: parent context from command entry point (should not be created here)
func NewOperationExecutorWithConfig(
	ctx context.Context,
	storage ObjectStorageProvider,
	queue *OperationQueue,
	workers int,
	conflictResolver ConflictResolver,
	ioTimeout time.Duration,
	retryConfig *RetryConfig,
) *OperationExecutor {
	// Derive cancellation context from parent (command context)
	// Individual operations will use their own contexts with timeouts
	ctx, cancel := context.WithCancel(ctx)

	if ioTimeout == 0 {
		ioTimeout = 30 * time.Second // Default I/O timeout
	}
	if retryConfig == nil {
		retryConfig = &RetryConfig{
			MaxAttempts:   3,
			InitialDelay:  100 * time.Millisecond,
			MaxDelay:      5 * time.Second,
			BackoffFactor: 2.0,
		}
	}

	executor := &OperationExecutor{
		storage:    storage,
		queue:      queue,
		maxWorkers: workers,
		// activeWorkers starts at 0 (default for atomic.Int32)
		wgManager:            NewWaitGroupManager(),
		ctx:                  ctx,
		cancel:               cancel,
		logger:               logging.NewEventLogger(ctx),
		conflictResolver:     conflictResolver,
		ioTimeout:            ioTimeout,
		retryConfig:          retryConfig,
		cacheManager:         NewCacheManager(ctx, storage),
		pendingInvalidations: make(map[string]*PendingInvalidation),
	}

	// Create WaitGroup for workers using manager (for observability)
	// The WaitGroup is created on-demand when workers start (see operation_executor_workers.go)
	wgGroup := executor.wgManager.CreateGroup(ConstMiscOperationExecutorWorkers, ConstMiscWorkerGoroutines)
	_ = wgGroup

	// Register wake callback for on-demand pattern
	SetExecutorWakeCallback(executor.wakeWorkerIfNeeded)

	return executor
}

// SetProjectRoot sets the project root for coordination events
func (e *OperationExecutor) SetProjectRoot(projectRoot string) {
	e.projectRoot = projectRoot
}

// Start starts the operation executor workers (deprecated - workers start on-demand)
// Kept for backward compatibility but workers now start automatically when work arrives
func (e *OperationExecutor) Start() error {
	// Workers start on-demand when operations are enqueued
	// This method is kept for backward compatibility but does nothing
	return nil
}

// Stop stops the operation executor
func (e *OperationExecutor) Stop() {
	e.cancel()
	// Use WaitGroupManager for centralized tracking
	e.wgManager.Wait(ConstMiscOperationExecutorWorkers)
}

// GetOperationExecutorStats returns lifetime counters for executed and failed operations.
func (e *OperationExecutor) GetOperationExecutorStats() (executed, failed int64) {
	return e.operationsExecutedTotal.Load(), e.operationsFailedTotal.Load()
}
