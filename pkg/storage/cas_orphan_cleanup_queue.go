package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

const (
	// casOrphanCleanupBatchSize is the maximum number of cleanup operations to process in a single batch
	casOrphanCleanupBatchSize = 50
	// casOrphanCleanupBatchTimeout is the maximum time to wait before processing a partial batch.
	// We flush when either (1) batch size reaches casOrphanCleanupBatchSize, or (2) this timeout
	// elapses after the last enqueue. Size-based flush gives batching under load; the timeout
	// ensures low-volume trickles (e.g. a few orphans) are processed within ~2s instead of
	// waiting indefinitely for 50 items.
	casOrphanCleanupBatchTimeout = 2 * time.Second
	// casOrphanCleanupMaxRetries is the maximum number of retries for failed cleanup operations
	casOrphanCleanupMaxRetries = 3
	// casOrphanCleanupIdleTimeout is the time to wait with empty queue before shutting down worker
	casOrphanCleanupIdleTimeout = 5 * 60000000000
)

// orphanCleanupRequest represents a single orphan file cleanup request
type orphanCleanupRequest struct {
	filePath string
	retries  int32 // Atomic counter for retry attempts
}

// OrphanCleanupEventCallback is a callback for emitting events via coordinator
// This avoids import cycles by using dependency injection
type OrphanCleanupEventCallback func(
	ctx context.Context,
	projectRoot string,
	storage ObjectStorageProvider,
	operationID string,
	operationType string,
	status string,
	batchSize int,
	successCount int,
	failureCount int,
	duration time.Duration,
	failedFiles []string,
)

var (
	globalOrphanCleanupEventCallback atomic.Pointer[OrphanCleanupEventCallback]
)

// SetOrphanCleanupEventCallback sets the global callback for emitting events via coordinator
// This should be called during system initialization to wire up coordinator integration
func SetOrphanCleanupEventCallback(callback OrphanCleanupEventCallback) {
	if callback == nil {
		globalOrphanCleanupEventCallback.Store(nil)
		return
	}
	ptr := new(OrphanCleanupEventCallback)
	*ptr = callback
	globalOrphanCleanupEventCallback.Store(ptr)
}

// getOrphanCleanupEventCallback returns the global event callback (if set)
func getOrphanCleanupEventCallback() OrphanCleanupEventCallback {
	ptr := globalOrphanCleanupEventCallback.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

// CASOrphanCleanupQueue manages batched cleanup of orphaned hash files
// This queues cleanup operations to avoid blocking CAS updates
// Workers shut down when idle and wake up when new work arrives
// Implements the "On-Demand Worker" pattern (wake-on-work with idle shutdown)
type CASOrphanCleanupQueue struct {
	queue          chan *orphanCleanupRequest
	workerRunning  int32        // Atomic flag to track if worker is currently running
	enqueuedTotal  atomic.Int64 // Atomic counter for total enqueued cleanup operations
	processedTotal atomic.Int64 // Atomic counter for total processed cleanup operations
	mu             sync.RWMutex // Protects complex state (if needed in future)
	ctx            context.Context
	cancel         context.CancelFunc
	wgManager      *WaitGroupManager // Centralized WaitGroup management
	metrics        *CASMetrics
	projectRoot    atomic.Value // Stores string (lock-free reads)
	storage        atomic.Value // Stores ObjectStorageProvider (lock-free reads)
	secCtx         *pkgctx.SecurityContext
	logger         logging.Logger
}

var (
	globalCASOrphanCleanupQueue *CASOrphanCleanupQueue
	globalCleanupQueueOnce      sync.Once
)

// GetGlobalCASOrphanCleanupQueue returns the global orphan cleanup queue instance
// Thread-safe singleton pattern
func GetGlobalCASOrphanCleanupQueue() *CASOrphanCleanupQueue {
	globalCleanupQueueOnce.Do(func() {
		// Use system context for long-lived background queue
		systemCtx := pkgctx.NewSystemContext()
		ctx, cancel := context.WithCancel(systemCtx)
		globalCASOrphanCleanupQueue = &CASOrphanCleanupQueue{
			queue:     make(chan *orphanCleanupRequest, 1000), // Buffered channel
			ctx:       ctx,
			cancel:    cancel,
			wgManager: NewWaitGroupManager(),
			metrics:   GetObjectStorageMetrics(),
			secCtx:    pkgctx.NewSystemSecurityContext(),
			logger:    logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
		}
		// Worker starts on first enqueue, not here (on-demand pattern)
	})
	return globalCASOrphanCleanupQueue
}

// OrphanCleanupQueue is a type alias for [CASOrphanCleanupQueue] (batched cleanup of orphaned hash files).
type OrphanCleanupQueue = CASOrphanCleanupQueue

// GetGlobalOrphanCleanupQueue returns the global orphan cleanup queue (same singleton as [GetGlobalCASOrphanCleanupQueue]).
func GetGlobalOrphanCleanupQueue() *OrphanCleanupQueue {
	return GetGlobalCASOrphanCleanupQueue()
}

// SetProjectRoot sets the project root for audit event creation
// Uses atomic.Value for lock-free reads (replaces mutex)
func (q *CASOrphanCleanupQueue) SetProjectRoot(projectRoot string) {
	q.projectRoot.Store(projectRoot)
	// TODO: Emit coordinator event for state change
}

// GetProjectRoot returns the project root (lock-free read)
func (q *CASOrphanCleanupQueue) GetProjectRoot() string {
	if val := q.projectRoot.Load(); val != nil {
		return val.(string)
	}
	return ""
}

// SetStorage sets the storage provider for audit event creation
// Uses atomic.Value for lock-free reads (replaces mutex)
func (q *CASOrphanCleanupQueue) SetStorage(storage ObjectStorageProvider) {
	q.storage.Store(storage)
	// TODO: Emit coordinator event for state change
}

// GetStorage returns the storage provider (lock-free read)
func (q *CASOrphanCleanupQueue) GetStorage() ObjectStorageProvider {
	if val := q.storage.Load(); val != nil {
		return val.(ObjectStorageProvider)
	}
	return nil
}

// EnqueueCleanup queues an orphan file for cleanup
// Non-blocking: returns immediately after queuing
// Wakes up worker if it's idle
// Thread-safe
// Returns error if shutdown has been initiated or queue is full
func (q *CASOrphanCleanupQueue) EnqueueCleanup(filePath string) error {
	// Check if shutdown has been initiated
	coordinator := GetGlobalShutdownCoordinator()
	if coordinator.IsShutdownInitiated() {
		return errfmt.Errorf(ConstStreamShutdownInProgressCannotEnqueueCleanupOperation)
	}

	req := &orphanCleanupRequest{
		filePath: filePath,
		retries:  0,
	}

	// Non-blocking send (channel is buffered)
	select {
	case q.queue <- req:
		// Successfully queued - increment lifetime counter and wake up worker if idle
		q.enqueuedTotal.Add(1)
		q.wakeWorkerIfNeeded()
		return nil
	default:
		// Channel full - return error
		return errfmt.Errorf(ConstStreamOrphanCleanupQueueIsFullCannotEnqueueCleanupFor, filePath)
	}
}

// wakeWorkerIfNeeded starts the worker if it's not running
// Part of the "On-Demand Worker" pattern - wakes worker when work arrives
func (q *CASOrphanCleanupQueue) wakeWorkerIfNeeded() {
	if atomic.LoadInt32(&q.workerRunning) == 0 {
		// Worker is idle - start it (wake-on-work)
		StorageLog(q.logger).Debug(LogEventStorageCASOrphanWakeWorker).Log()
		q.startWorker()
	}
}

// startWorker starts the background worker that processes cleanup operations in batches
// Implements "On-Demand Worker" pattern: wakes on work, shuts down after idle timeout
// Batches are processed when either:
//   - Batch size reaches casOrphanCleanupBatchSize (50 requests), or
//   - casOrphanCleanupBatchTimeout (2s) elapses since last enqueue (partial batch)
func (q *CASOrphanCleanupQueue) startWorker() {
	if !atomic.CompareAndSwapInt32(&q.workerRunning, 0, 1) {
		// Worker already running
		return
	}

	StorageLog(q.logger).Info(LogEventStorageCASOrphanWorkerStarting).Log()

	// Emit worker start event via coordinator
	q.emitWorkerLifecycleEvent("start", ConstStreamWorkerStarted, 0, 0, 0, 0, nil)

	// Create WaitGroup for worker using manager (nil-safe: builder no-ops Add/Done when wg is nil)
	wg := q.wgManager.CreateGroupForGoroutine(ConstStreamCasOrphanCleanupWorker, ConstStreamCasOrphanCleanupWorker)
	runWorker := func(ctx context.Context) error {
		defer func() {
			atomic.StoreInt32(&q.workerRunning, 0) // Mark as stopped when done
			StorageLog(q.logger).Info(LogEventStorageCASOrphanWorkerStoppedIdle).Log()
			// Emit worker stop event via coordinator
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
			// If clamped to 0, shut down immediately when there is no work
			if casOrphanCleanupIdleTimeout == 0 && len(q.queue) == 0 && len(batch) == 0 {
				StorageLog(q.logger).Debug(LogEventStorageCASOrphanWorkerIdleShutdown).
					IdleDuration("0s").
					Log()
				return nil
			}

			select {
			case <-ctx.Done():
				// Process any remaining items in batch before shutdown
				if len(batch) > 0 {
					q.processBatch(batch)
				}
				return ctx.Err()
			case req := <-q.queue:
				// New work arrived - reset idle timer
				lastWorkTime = time.Now()
				if idleTicker != nil {
					idleTicker.Reset(casOrphanCleanupIdleTimeout)
				}

				batch = append(batch, req)
				// Drain queue up to batch size so we process in larger batches when work has accumulated
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
					// Batch is full - process immediately
					q.processBatch(batch)
					batch = batch[:0] // Reset batch
					batchTicker.Reset(casOrphanCleanupBatchTimeout)
				} else {
					// Partial batch: reset batch ticker so we wait a full timeout from this last item
					// before processing, giving time for more enqueues to accumulate
					batchTicker.Reset(casOrphanCleanupBatchTimeout)
				}
			case <-batchTicker.C:
				// Batch timeout reached - process current batch
				if len(batch) > 0 {
					lastWorkTime = time.Now()
					q.processBatch(batch)
					batch = batch[:0] // Reset batch
				}
			case <-idleTickerC:
				// Idle timeout reached - check if we should shut down
				queueSize := len(q.queue)
				if queueSize == 0 && len(batch) == 0 {
					// Queue is empty and no batch pending - check if we've been idle long enough
					idleDuration := time.Since(lastWorkTime)
					if idleDuration >= casOrphanCleanupIdleTimeout {
						// Been idle long enough - shut down worker (on-demand pattern)
						// Worker will wake up again when new work arrives (via wakeWorkerIfNeeded)
						StorageLog(q.logger).Debug(LogEventStorageCASOrphanWorkerIdleShutdown).
							IdleDuration(idleDuration.String()).
							Log()
						return nil // Exit worker goroutine
					}
					// Not idle long enough yet - reset timer
					if idleTicker != nil {
						idleTicker.Reset(casOrphanCleanupIdleTimeout - idleDuration)
					}
				} else {
					// There's work - reset idle timer
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
			// Cleanup failed - check if we should retry
			retries := atomic.LoadInt32(&req.retries)
			if retries < casOrphanCleanupMaxRetries {
				// Retry later
				atomic.AddInt32(&req.retries, 1)
				retryQueue = append(retryQueue, req)
				failureCount++
				failedFiles = append(failedFiles, req.filePath)
			} else {
				// Max retries exceeded - log and give up
				failureCount++
				failedFiles = append(failedFiles, req.filePath)
			}
		} else {
			successCount++
		}
	}

	// Re-queue failed items for retry (with exponential backoff)
	// Use RetryConfig pattern for consistent backoff calculation
	if len(retryQueue) > 0 {
		retryCount := len(retryQueue)
		retryConfig := &RetryConfig{
			MaxAttempts:   3, // Not used here, but follows pattern
			InitialDelay:  1 * time.Second,
			MaxDelay:      10 * time.Second,
			BackoffFactor: 2.0,
		}

		// Calculate backoff delay using RetryConfig pattern
		// For batch retries, use retry count to determine delay
		backoffDelay := retryConfig.InitialDelay
		// Cap retry count at reasonable limit to avoid excessive delays
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
						// Log error but continue with other retries
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

	// Record metrics
	if q.metrics != nil {
		q.metrics.RecordOrphanCleanupBatch(duration, len(batch), successCount, failureCount)
	}

	// Emit batch processing event via coordinator (unified observability)
	q.emitBatchProcessingEvent(len(batch), successCount, failureCount, duration, failedFiles)

	// Also create direct audit event for backward compatibility (if coordinator not available)
	// This ensures we still have audit trail even without coordinator integration
	q.createCleanupBatchAuditEvent(len(batch), successCount, failureCount, duration, failedFiles)
}

// removeOrphanCASHashFileSync removes a superseded CAS hash file using the same rename-then-delete
// strategy as the orphan cleanup queue worker: rename away from the `^[a-f0-9]{64}\.yaml$` pattern
// first so list/validation and discovery scans do not observe two live blobs for the same logical update.
//
// Used by the queue worker and as a synchronous fallback when EnqueueCleanup fails (full channel,
// shutdown) so CAS updates do not leave stale hash files next to the new content.
func removeOrphanCASHashFileSync(filePath string) error {
	tmpPath := filePath + ".tmp"
	// Hide from validation by renaming to .tmp (hash pattern is ^[a-f0-9]{64}\.yaml$)
	if err := os.Rename(filePath, tmpPath); err != nil {
		if os.IsNotExist(err) {
			// Original already gone; maybe already renamed on a prior run
			if rmErr := os.Remove(tmpPath); rmErr != nil && !os.IsNotExist(rmErr) {
				logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ConstStreamFailedToRemoveTmpOrphanFileStrValN, tmpPath, rmErr), nil).Log()
			}
			return nil
		}
		// Fallback: delete in place (e.g. cross-filesystem rename)
		if removeErr := os.Remove(filePath); removeErr != nil && !os.IsNotExist(removeErr) {
			return errfmt.Errorf(ConstStreamFailedToRemoveOrphanedFileStrErr, filePath, removeErr)
		}
		return nil
	}
	if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
		return errfmt.Errorf(ConstStreamFailedToRemoveOrphanedTmpFileStrErr, tmpPath, err)
	}
	dir := filepath.Dir(filePath)
	if dirFD, err := os.Open(dir); err == nil {
		if syncErr := dirFD.Sync(); syncErr != nil {
			logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ConstStreamFailedToSyncDirStrAfterOrphanRemovalValN, dir, syncErr), nil).Log()
		}
		if closeErr := dirFD.Close(); closeErr != nil {
			logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ConstStreamFailedToCloseDirStrAfterOrphanRemovalValN, dir, closeErr), nil).Log()
		}
	}
	return nil
}

// cleanupFile performs the actual file deletion (delegates to removeOrphanCASHashFileSync).
func (q *CASOrphanCleanupQueue) cleanupFile(filePath string) error {
	return removeOrphanCASHashFileSync(filePath)
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
		// Coordinator not available - skip
		return
	}

	// Use system context for background event emission
	ctx := pkgctx.NewSystemContext()
	operationID := fmt.Sprintf(ConstStreamOrphanCleanupBatchInt, time.Now().Unix())
	operationType := ConstStreamOrphanCleanupBatch
	status := "complete"
	if failureCount > 0 {
		status = ConstStreamPartialFailure
	}

	// Emit via coordinator (async, non-blocking)
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
		// Coordinator not available - skip
		return
	}

	// Use system context for background event emission
	ctx := pkgctx.NewSystemContext()
	operationID := fmt.Sprintf(ConstStreamOrphanCleanupWorkerInt, time.Now().Unix())

	// Emit via coordinator (async, non-blocking)
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
	// Lock-free reads using atomic.Value
	projectRoot := q.GetProjectRoot()
	storage := q.GetStorage()
	secCtx := q.secCtx

	if projectRoot == emptyValue || storage == nil {
		// Can't create audit event without project root and storage
		return // Best effort
	}

	// Build operation description
	operation := fmt.Sprintf(ConstStreamCleanedUpIntOrphanedCasHashFilesIntSucceededInt,
		batchSize, successCount, failureCount, duration.Round(time.Millisecond))

	// Build metadata
	metadata := map[string]any{
		objects.FieldKeySource:       ConstStreamBackgroundWorker,
		objects.FieldKeyBatchSize:    batchSize,
		objects.FieldKeySuccessCount: successCount,
		objects.FieldKeyFailureCount: failureCount,
		"duration_ms":                duration.Milliseconds(),
		ConstStreamOperationType:     ConstStreamOrphanCleanup,
	}
	if len(failedFiles) > 0 {
		// Include first few failed files for debugging (don't include all to avoid huge events)
		maxFailedFiles := 5
		if len(failedFiles) > maxFailedFiles {
			metadata["failed_files"] = failedFiles[:maxFailedFiles]
			metadata[ConstStreamFailedFilesTruncated] = true
			metadata[ConstStreamTotalFailedFiles] = len(failedFiles)
		} else {
			metadata["failed_files"] = failedFiles
		}
	}

	// Use instance builder helper to create audit event
	// Use system context for background audit event creation
	ctx := pkgctx.NewSystemContext()
	// Use orphan_cleanup_complete event type (validated event type from audit_event spec)
	// This is a batch completion event, so use the complete variant
	eventType := ConstStreamOrphanCleanupComplete
	if failureCount > 0 {
		// If there were failures, use error variant instead
		eventType = ConstStreamOrphanCleanupError
	}
	options := &AuditEventOptions{
		EventType:  eventType,
		Operation:  operation,
		Severity:   "low", // Cleanup is typically low severity
		TargetKind: ConstStreamCasOrphanCleanup,
		TargetID:   fmt.Sprintf("batch-%d", time.Now().Unix()),
		Metadata:   metadata,
		CreatedBy:  secCtx.AccountID,
	}

	// Create audit event (best effort - don't fail if it doesn't work)
	if err := CreateAuditEventWithBuilder(ctx, projectRoot, secCtx, storage, options); err != nil {
		logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ConstStreamFailedToCreateCasOrphanCleanupBatchAuditEventValN, err), nil).Log()
	}
}

// Shutdown gracefully shuts down the cleanup queue
// Processes any remaining items before stopping
func (q *CASOrphanCleanupQueue) Shutdown() {
	q.cancel()

	// Wait for worker to finish with deterministic timeout
	shutdownTimeout := 30 * time.Second
	done := make(chan struct{})
	goroutinelabels.NewGoroutine(ConstStreamCasOrphanCleanupShutdownWait, ConstStreamWaitingForCasOrphanCleanupWorkerToStop).
		WithCleanup(func() {
			close(done)
		}).
		StartSimple(func() {
			q.wgManager.Wait(ConstStreamCasOrphanCleanupWorker)
		})

	select {
	case <-done:
		// Worker stopped normally
		StorageLog(q.logger).Info(LogEventStorageCASOrphanWorkerStoppedOK).Log()
	case <-time.After(shutdownTimeout):
		// Timeout - log warning but proceed
		// Worker may still be running, but we proceed with shutdown
		StorageLog(q.logger).Warn(LogEventStorageCASOrphanShutdownWaitTimeout).
			String("timeout", shutdownTimeout.String()).
			Log()
		// Emit shutdown timeout event via coordinator
		q.emitWorkerLifecycleEvent("timeout", ConstStreamShutdownTimeout, 0, 0, 0, shutdownTimeout, []string{})
	}
}

// InitiateShutdown implements QueueShutdownHandler
// Stops accepting new operations
func (q *CASOrphanCleanupQueue) InitiateShutdown() error {
	// Cancel context to stop accepting new work
	q.cancel()
	return nil
}

// Drain implements QueueShutdownHandler
// Processes all pending operations
func (q *CASOrphanCleanupQueue) Drain(ctx context.Context) error {
	// Cancel our context to stop accepting new work
	q.cancel()

	// Wait for worker to finish processing with deterministic timeout
	done := make(chan struct{})
	goroutinelabels.NewGoroutine(ConstStreamCasOrphanCleanupDrainWait, ConstStreamWaitingForCasOrphanCleanupWorkerToDrain).
		WithContext(ctx).
		WithCleanup(func() {
			close(done)
		}).
		StartSimple(func() {
			q.wgManager.Wait(ConstStreamCasOrphanCleanupWorker)
		})

	// Use context timeout or default timeout (whichever is shorter)
	drainTimeout := 5 * time.Minute
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < drainTimeout {
			drainTimeout = remaining
		}
	}

	select {
	case <-done:
		return nil
	case <-time.After(drainTimeout):
		// Timeout - return error
		return errfmt.Errorf(ConstStreamDrainTimeoutAfterVal, drainTimeout)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsDrained implements QueueShutdownHandler
func (q *CASOrphanCleanupQueue) IsDrained() bool {
	return len(q.queue) == 0 && !q.IsWorkerRunning()
}

// GetPendingCount implements QueueShutdownHandler
func (q *CASOrphanCleanupQueue) GetPendingCount() int64 {
	return int64(len(q.queue))
}

// GetName implements QueueShutdownHandler
func (q *CASOrphanCleanupQueue) GetName() string {
	return ConstStreamCasOrphanCleanupQueue
}

// IsCritical implements QueueShutdownHandler
// Orphan cleanup is not critical - can be deferred
func (q *CASOrphanCleanupQueue) IsCritical() bool {
	return false
}

// QueueSize returns the current number of items in the queue
func (q *CASOrphanCleanupQueue) QueueSize() int {
	return len(q.queue)
}

// IsWorkerRunning returns whether the worker is currently running
func (q *CASOrphanCleanupQueue) IsWorkerRunning() bool {
	return atomic.LoadInt32(&q.workerRunning) == 1
}

// ProcessQueueIfIdle processes the queue if the worker is not running
// This is a fallback method that can be called by scheduled jobs
// Returns the number of items processed
func (q *CASOrphanCleanupQueue) ProcessQueueIfIdle(ctx context.Context) (int, error) {
	queueSize := len(q.queue)
	if queueSize == 0 {
		return 0, nil // Nothing to process
	}

	// Check if worker is running
	if atomic.LoadInt32(&q.workerRunning) == 1 {
		// Worker is running - let it handle it
		return 0, nil
	}

	// Worker is idle but queue has items - process a batch
	// This is a fallback mechanism for when the worker failed to start or crashed
	batchSize := queueSize
	if batchSize > casOrphanCleanupBatchSize {
		batchSize = casOrphanCleanupBatchSize
	}

	batch := make([]*orphanCleanupRequest, 0, batchSize)
batchLoop:
	for i := 0; i < batchSize; i++ {
		select {
		case <-ctx.Done():
			return len(batch), ctx.Err()
		case req := <-q.queue:
			batch = append(batch, req)
		default:
			// No more items available
			break batchLoop
		}
	}

	if len(batch) > 0 {
		q.processBatch(batch)
		// Wake up worker to handle remaining items
		q.wakeWorkerIfNeeded()
	}

	return len(batch), nil
}

// CASOrphanCleanupQueueStats provides a snapshot of CAS orphan cleanup queue status
type CASOrphanCleanupQueueStats struct {
	QueueLength    int   `json:"queue_length"`
	QueueCapacity  int   `json:"queue_capacity"`
	WorkerRunning  bool  `json:"worker_running"`
	EnqueuedTotal  int64 `json:"enqueued_total"`
	ProcessedTotal int64 `json:"processed_total"`
}

// GetQueueStats returns current stats for the orphan cleanup queue
func (q *CASOrphanCleanupQueue) GetQueueStats() CASOrphanCleanupQueueStats {
	if q == nil {
		return CASOrphanCleanupQueueStats{}
	}
	return CASOrphanCleanupQueueStats{
		QueueLength:    len(q.queue),
		QueueCapacity:  cap(q.queue),
		WorkerRunning:  atomic.LoadInt32(&q.workerRunning) == 1,
		EnqueuedTotal:  q.enqueuedTotal.Load(),
		ProcessedTotal: q.processedTotal.Load(),
	}
}
