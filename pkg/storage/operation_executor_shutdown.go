package storage

import (
	"context"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
)

// InitiateShutdown implements QueueShutdownHandler
// Stops accepting new operations
func (e *OperationExecutor) InitiateShutdown() error {
	e.cancel()
	return nil
}

// Drain implements QueueShutdownHandler
// Processes all pending operations
func (e *OperationExecutor) Drain(ctx context.Context) error {
	// Initiate shutdown first
	if err := e.InitiateShutdown(); err != nil {
		return err
	}

	// Wait for all workers to finish
	done := make(chan struct{})
	waitCtx, waitCancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Minute)
	defer waitCancel()
	execDrainBud := goroutinelabels.DefaultBudget()
	execDrainBuilder := goroutinelabels.NewGoroutine(ConstMiscOperationExecutorDrainWait, ConstMiscWaitingForOperationExecutorWorkersToComp).
		WithCleanup(func() {
			close(done)
		})
	if execDrainBud != nil {
		execDrainBuilder = execDrainBuilder.WithBudget(execDrainBud)
	}
	execDrainBuilder.StartWithContext(waitCtx, func(ctx context.Context) error {
		e.wgManager.Wait(ConstMiscOperationExecutorWorkers)
		return nil
	})

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-waitCtx.Done():
		// Timeout waiting for workers
		return waitCtx.Err()
	}
}

// IsDrained implements QueueShutdownHandler
func (e *OperationExecutor) IsDrained() bool {
	// Check if there are active workers
	if e.activeWorkers.Load() > 0 {
		return false
	}

	// Check if queue has pending operations
	return e.queue.Peek() == nil
}

// GetPendingCount implements QueueShutdownHandler
func (e *OperationExecutor) GetPendingCount() int64 {
	var count int64
	_ = concurrency.RunInRLockOrLog(&e.queue.mu, locknames.LockNameOperationExecutorGetPending, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		count = 0
		for _, op := range e.queue.operations {
			if op.Status == StatusPending || op.Status == StatusRetrying {
				count++
			}
		}
		return nil
	})
	return count
}

// GetName implements QueueShutdownHandler
func (e *OperationExecutor) GetName() string {
	return ConstMiscOperationExecutor
}

// IsCritical implements QueueShutdownHandler
// Operation executor is critical - must complete all operations before shutdown
func (e *OperationExecutor) IsCritical() bool {
	return true
}
