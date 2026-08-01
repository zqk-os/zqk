package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage/locknames"
)

// wakeWorkerIfNeeded starts a worker if there's work and we're under max workers
// Note: This should NOT be called while holding the queue lock to avoid deadlocks
func (e *OperationExecutor) wakeWorkerIfNeeded() {
	// Check if we need to start a worker
	currentWorkers := int(e.activeWorkers.Load())
	if currentWorkers >= e.maxWorkers {
		return // Already at max workers
	}
	var err_swallow_100 = concurrency.RunInLockWithLogger(&e.workerMu, locknames.LockNameOperationExecutorWakeWorker, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {

		currentWorkers = int(e.activeWorkers.Load())
		if currentWorkers >= e.maxWorkers {
			return nil
		}
		if e.queue.Peek() == nil {
			return nil
		}
		workerID := currentWorkers
		e.activeWorkers.Add(1)
		callback := getOperationExecutorEventCallback()
		if callback != nil && e.projectRoot != emptyValue {
			callback(
				e.ctx,
				e.projectRoot,
				e.storage,
				fmt.Sprintf("worker_%d", workerID),
				"worker_start",
				"started",
				int(e.activeWorkers.Load()),
				0,
				0,
				0,
			)
		}
		e.wgManager.Add(ConstMiscOperationExecutorWorkers, 1)
		workerIDForGoroutine := workerID
		execBud := goroutinelabels.DefaultBudget()
		execWorkerBuilder := goroutinelabels.NewGoroutine(fmt.Sprintf(ConstMiscOperationExecutorWorkerD, workerIDForGoroutine),
			fmt.Sprintf(ConstMiscProcessingOperationsWorkerD, workerIDForGoroutine)).
			WithWaitGroup(e.wgManager.GetGroup(ConstMiscOperationExecutorWorkers))
		if execBud != nil {
			execWorkerBuilder = execWorkerBuilder.WithBudget(execBud)
		}
		execWorkerBuilder.StartWithContext(e.ctx, func(ctx context.Context) error {
			defer e.wgManager.Done(ConstMiscOperationExecutorWorkers)
			e.worker(workerIDForGoroutine)
			return nil
		})
		return nil
	})
	if err_swallow_100 !=

		// worker processes operations from the queue
		// Implements on-demand pattern: processes work, then shuts down after idle timeout
		nil {
		logging.LogSwallowedError(err_swallow_100)
	}
}

func (e *OperationExecutor) worker(workerID int) {
	defer func() {
		e.activeWorkers.Add(-1)
		// Note: wg.Done() is called by goroutinelabels.WithWaitGroup() on goroutine exit
		// Do NOT call e.wg.Done() here - it would cause a double Done() and negative WaitGroup counter panic

		// Emit worker shutdown event
		callback := getOperationExecutorEventCallback()
		if callback != nil && e.projectRoot != emptyValue {
			// Use executor's context (or derived context) instead of creating new Background()
			callback(
				e.ctx,
				e.projectRoot,
				e.storage,
				fmt.Sprintf("worker_%d", workerID),
				ConstMiscWorkerShutdown,
				"stopped",
				int(e.activeWorkers.Load()),
				0,
				0,
				0,
			)
		}
	}()

	idleStartTime := time.Now()
	processedCount := 0
	failedCount := 0
	startTime := time.Now()

	for {
		select {
		case <-e.ctx.Done():
			return
		default:
			op := e.queue.Dequeue()
			if op == nil {
				// No operations available - check idle timeout
				idleDuration := time.Since(idleStartTime)
				if idleDuration >= operationExecutorIdleTimeout {
					// Idle timeout reached - shut down worker
					StorageLog(e.logger.Logger()).Info(LogEventStorageOperationExecutorWorkerIdleShutdownInfo).
						WorkerID(workerID).
						IdleDuration(idleDuration.String()).
						ProcessedCount(processedCount).
						FailedCount(failedCount).
						Log()

					// Emit worker idle shutdown event
					callback := getOperationExecutorEventCallback()
					if callback != nil && e.projectRoot != emptyValue {
						// Use executor's context (or derived context) instead of creating new Background()
						callback(
							e.ctx,
							e.projectRoot,
							e.storage,
							fmt.Sprintf("worker_%d", workerID),
							ConstMiscWorkerIdleShutdown,
							"idle_shutdown",
							int(e.activeWorkers.Load()),
							processedCount,
							failedCount,
							time.Since(startTime),
						)
					}
					return
				}

				// Wait a bit before checking again (with cancellation check)
				select {
				case <-e.ctx.Done():
					return
				case <-time.After(operationExecutorCheckInterval):
					// Continue loop to check idle timeout
				}
				continue
			}

			// Reset idle timer when we get work
			idleStartTime = time.Now()

			// Execute operation
			err := e.executeOperationWithTimeout(op)
			if err != nil {
				failedCount++

				// Check if we can retry
				if op.CanRetry() {
					op.IncrementRetry()
					var err_swallow_102 = e.queue.UpdateStatus(op, StatusRetrying)
					if //nolint:errcheck // Status update errors are non-critical
					err_swallow_102 != nil {
						logging.LogSwallowedError(err_swallow_102)
					}
					var err_swallow_103 = e.queue.Enqueue(op)
					if //nolint:errcheck // Enqueue errors are non-critical
					err_swallow_103 != nil {
						logging.LogSwallowedError(err_swallow_103)
					}

					if e.queue.notifier != nil {
						var err_swallow_104 = e.queue.notifier.NotifyError(op, errfmt.Errorf("operation failed, retrying (attempt %d/%d): %w", op.RetryCount, op.MaxRetries, err))
						if //nolint:errcheck // Notification errors are non-critical
						err_swallow_104 != nil {
							logging.LogSwallowedError(err_swallow_104)
						}
					}
					continue
				}

				// Operation failed permanently
				op.SetError(err)
				var err_swallow_101 = e.queue.UpdateStatus(op, StatusFailed)
				if //nolint:errcheck // Status update errors are non-critical
				err_swallow_101 != nil {
					logging.LogSwallowedError(err_swallow_101)
				}

				if e.queue.notifier != nil {
					var err_swallow_105 = e.queue.notifier.NotifyError(op, err)
					if //nolint:errcheck // Notification errors are non-critical
					err_swallow_105 != nil {
						logging.LogSwallowedError(err_swallow_105)
					}
				}
				continue
			}

			// Operation completed successfully
			processedCount++
			if err := e.queue.UpdateStatus(op, StatusCompleted); err != nil {
				logging.LogSwallowedError(err)
			}

			if e.queue.notifier != nil {
				var err_swallow_106 = e.queue.notifier.NotifyProgress(op, 100, fmt.Sprintf(ConstMiscCompletedSOperationOnS, op.Type, op.ObjectID))
				if //nolint:errcheck // Notification errors are non-critical
				err_swallow_106 != nil {
					logging.LogSwallowedError(err_swallow_106)
				}
				var err_swallow_107 = e.queue.notifier.NotifyCompletion(op)
				if err_swallow_107 != nil {
					logging.LogSwallowedError(err_swallow_107)
				} //nolint:errcheck // Notification errors are non-critical
			}

			// Wake another worker if there's more work
			e.wakeWorkerIfNeeded()
		}
	}
}
