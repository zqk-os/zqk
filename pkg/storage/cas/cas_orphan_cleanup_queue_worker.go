// Extracted from pkg/storage/cas/cas_orphan_cleanup_queue.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package cas

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

// startWorker starts the background worker that processes cleanup operations in batches
// Implements "On-Demand Worker" pattern: wakes on work, shuts down after idle timeout
// Batches are processed when either:
//   - Batch size reaches casOrphanCleanupBatchSize (50 requests), or
//   - casOrphanCleanupBatchTimeout (2s) elapses since last enqueue (partial batch)
func (q *CASOrphanCleanupQueue) startWorker() {
	if !atomic.CompareAndSwapInt32(&q.workerRunning, 0, 1) {

		return
	}

	StorageLog(q.logger).Info(LogEventStorageCASOrphanWorkerStarting).Log()

	q.emitWorkerLifecycleEvent("start", ConstStreamWorkerStarted, 0, 0, 0, 0, nil)

	wg := q.wgManager.CreateGroupForGoroutine(ConstStreamCasOrphanCleanupWorker, ConstStreamCasOrphanCleanupWorker)
	runWorker := func(ctx context.Context) error {
		defer func() {
			atomic.StoreInt32(&q.workerRunning, 0)
			StorageLog(q.logger).Info(LogEventStorageCASOrphanWorkerStoppedIdle).Log()

			q.emitWorkerLifecycleEvent("complete", ConstStreamWorkerStopped, 0, 0, 0, 0, nil)
		}()

		batch := make([]*orphanCleanupRequest, 0, casOrphanCleanupBatchSize)
		batchTicker := time.NewTicker(casOrphanCleanupBatchTimeout)
		defer batchTicker.Stop()

		var idleTicker *time.Ticker
		var idleTickerC <-chan time.Time
		if casOrphanCleanupIdleTimeout > 0 {
			idleTicker = time.NewTicker(casOrphanCleanupIdleTimeout)
			defer idleTicker.Stop()
			idleTickerC = idleTicker.C
		}

		lastWorkTime := time.Now()

		for {

			if casOrphanCleanupIdleTimeout == 0 && len(q.queue) == 0 && len(batch) == 0 {
				StorageLog(q.logger).Debug(LogEventStorageCASOrphanWorkerIdleShutdown).
					IdleDuration("0s").
					Log()
				return nil
			}

			select {
			case <-ctx.Done():

				if len(batch) > 0 {
					q.processBatch(batch)
				}
				return ctx.Err()
			case req := <-q.queue:

				lastWorkTime = time.Now()
				if idleTicker != nil {
					idleTicker.Reset(casOrphanCleanupIdleTimeout)
				}

				batch = append(batch, req)

			drainLoop:
				for len(batch) < casOrphanCleanupBatchSize {
					select {
					case next := <-q.queue:
						batch = append(batch, next)
					default:
						break drainLoop
					}
				}
				if len(batch) >= casOrphanCleanupBatchSize {

					q.processBatch(batch)
					batch = batch[:0]
					batchTicker.Reset(casOrphanCleanupBatchTimeout)
				} else {

					batchTicker.Reset(casOrphanCleanupBatchTimeout)
				}
			case <-batchTicker.C:

				if len(batch) > 0 {
					lastWorkTime = time.Now()
					q.processBatch(batch)
					batch = batch[:0]
				}
			case <-idleTickerC:

				queueSize := len(q.queue)
				if queueSize == 0 && len(batch) == 0 {

					idleDuration := time.Since(lastWorkTime)
					if idleDuration >= casOrphanCleanupIdleTimeout {

						StorageLog(q.logger).Debug(LogEventStorageCASOrphanWorkerIdleShutdown).
							IdleDuration(idleDuration.String()).
							Log()
						return nil
					}

					if idleTicker != nil {
						idleTicker.Reset(casOrphanCleanupIdleTimeout - idleDuration)
					}
				} else {

					lastWorkTime = time.Now()
					if idleTicker != nil {
						idleTicker.Reset(casOrphanCleanupIdleTimeout)
					}
				}
			}
		}
	}
	goroutinelabels.NewGoroutine(ConstStreamCasOrphanCleanupWorker, ConstStreamProcessingOrphanedHashFileCleanupOperations).
		WithWaitGroup(wg).
		StartWithContext(q.ctx, runWorker)
}

// processBatch processes a batch of cleanup requests
// Creates audit events and records metrics for visibility
func (q *CASOrphanCleanupQueue) processBatch(batch []*orphanCleanupRequest) {
	if len(batch) == 0 {
		return
	}

	q.processedTotal.Add(int64(len(batch)))
	start := time.Now()
	successCount := 0
	failureCount := 0
	retryQueue := make([]*orphanCleanupRequest, 0)
	failedFiles := make([]string, 0)

	for _, req := range batch {
		if err := q.cleanupFile(req.filePath); err != nil {

			retries := atomic.LoadInt32(&req.retries)
			if retries < casOrphanCleanupMaxRetries {

				atomic.AddInt32(&req.retries, 1)
				retryQueue = append(retryQueue, req)
				failureCount++
				failedFiles = append(failedFiles, req.filePath)
			} else {

				failureCount++
				failedFiles = append(failedFiles, req.filePath)
			}
		} else {
			successCount++
		}
	}

	if len(retryQueue) > 0 {
		retryCount := len(retryQueue)
		retryConfig := &RetryConfig{
			InitialDelay:  1 * time.Second,
			MaxDelay:      10 * time.Second,
			BackoffFactor: 2.0,
		}

		backoffDelay := retryConfig.InitialDelay

		maxRetryCountForDelay := 4
		if retryCount > maxRetryCountForDelay {
			retryCount = maxRetryCountForDelay
		}
		for i := 1; i < retryCount; i++ {
			backoffDelay = time.Duration(float64(backoffDelay) * retryConfig.BackoffFactor)
			if backoffDelay > retryConfig.MaxDelay {
				backoffDelay = retryConfig.MaxDelay
				break
			}
		}

		goroutinelabels.NewGoroutine(ConstStreamCasOrphanCleanupRetry, fmt.Sprintf(ConstStreamRetryingIntFailedCleanupOperations, len(retryQueue))).
			StartSimple(func() {
				time.Sleep(backoffDelay)
				for _, req := range retryQueue {
					if err := q.EnqueueCleanup(req.filePath); err != nil {

						StorageLog(q.logger).Warn(LogEventStorageCASOrphanRequeueFailed).
							Path(req.filePath).
							WithError(err).
							Log()
					}
				}
				StorageLog(q.logger).Debug(LogEventStorageCASOrphanRequeuedForRetry).
					Int("retry_count", len(retryQueue)).
					String("backoff_delay", backoffDelay.String()).
					Log()
			})
	}

	duration := time.Since(start)

	if q.metrics != nil {
		q.metrics.RecordOrphanCleanupBatch(duration, len(batch), successCount, failureCount)
	}

	if callback := getOrphanCleanupEventCallback(); callback != nil {
		q.emitBatchProcessingEvent(len(batch), successCount, failureCount, duration, failedFiles)
	} else {
		q.createCleanupBatchAuditEvent(len(batch), successCount, failureCount, duration, failedFiles)
	}
}

// emitBatchProcessingEvent emits a batch processing event via coordinator
// This provides unified observability through coordinator (logging, audit, metrics, operational)
// Skips emission for successful single-file cleanups to avoid log spam when orphans are produced one-at-a-time (e.g. trickle of CAS updates).
func (q *CASOrphanCleanupQueue) emitBatchProcessingEvent(
	batchSize, successCount, failureCount int,
	duration time.Duration,
	failedFiles []string,
) {
	if batchSize == 1 && failureCount == 0 {
		return
	}
	callback := getOrphanCleanupEventCallback()
	projectRoot := q.GetProjectRoot()
	storage := q.GetStorage()
	if callback == nil || projectRoot == emptyValue || storage == nil {

		return
	}

	ctx := pkgctx.NewSystemContext()
	operationID := fmt.Sprintf(ConstStreamOrphanCleanupBatchInt, time.Now().Unix())
	operationType := ConstStreamOrphanCleanupBatch
	status := "complete"
	if failureCount > 0 {
		status = ConstStreamPartialFailure
	}

	goroutinelabels.NewGoroutine(ConstStreamOrphanCleanupCoordinatorEvent, ConstStreamEmittingBatchProcessingEventViaCoordinator).
		StartSimple(func() {
			callback(
				ctx,
				projectRoot,
				storage,
				operationID,
				operationType,
				status,
				batchSize,
				successCount,
				failureCount,
				duration,
				failedFiles,
			)
		})
}

// emitWorkerLifecycleEvent emits a worker lifecycle event via coordinator
func (q *CASOrphanCleanupQueue) emitWorkerLifecycleEvent(
	status, operationType string,
	batchSize, successCount, failureCount int,
	duration time.Duration,
	failedFiles []string,
) {
	callback := getOrphanCleanupEventCallback()
	projectRoot := q.GetProjectRoot()
	storage := q.GetStorage()
	if callback == nil || projectRoot == emptyValue || storage == nil {

		return
	}

	ctx := pkgctx.NewSystemContext()
	operationID := fmt.Sprintf(ConstStreamOrphanCleanupWorkerInt, time.Now().Unix())

	goroutinelabels.NewGoroutine(ConstStreamOrphanCleanupCoordinatorEvent, ConstStreamEmittingWorkerLifecycleEventViaCoordinator).
		StartSimple(func() {
			callback(
				ctx,
				projectRoot,
				storage,
				operationID,
				operationType,
				status,
				batchSize,
				successCount,
				failureCount,
				duration,
				failedFiles,
			)
		})
}

// createCleanupBatchAuditEvent creates an audit event for a cleanup batch
// This provides visibility into background cleanup operations
// NOTE: This is a fallback for when coordinator is not available
// When coordinator is available, events should flow through coordinator instead
func (q *CASOrphanCleanupQueue) createCleanupBatchAuditEvent(
	batchSize, successCount, failureCount int,
	duration time.Duration,
	failedFiles []string,
) {
	if batchSize == 1 && failureCount == 0 {
		return
	}

	projectRoot := q.GetProjectRoot()
	storage := q.GetStorage()
	secCtx := q.secCtx

	if projectRoot == emptyValue || storage == nil {
		return
	}

	ctx := pkgctx.NewSystemContext()

	if creator := GetAuditCreator(); creator != nil {
		creator(ctx, secCtx, projectRoot, storage, batchSize, successCount, failureCount, duration.Milliseconds(), failedFiles)
	}
}
