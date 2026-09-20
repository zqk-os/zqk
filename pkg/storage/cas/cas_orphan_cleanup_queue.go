package cas

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/filecas"

	"sync"
	"sync/atomic"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
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
	storage CASFacade,
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
	wgManager      WaitGroupManager // Centralized WaitGroup management
	metrics        *CASMetrics
	projectRoot    atomic.Value // Stores string (lock-free reads)
	storage        atomic.Value // Stores CASFacade (lock-free reads)
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
			wgManager: GlobalNewWaitGroupManager(),
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
func (q *CASOrphanCleanupQueue) SetStorage(storage CASFacade) {
	q.storage.Store(storage)
	// TODO: Emit coordinator event for state change
}

// GetStorage returns the storage provider (lock-free read)
func (q *CASOrphanCleanupQueue) GetStorage() CASFacade {
	if val := q.storage.Load(); val != nil {
		return val.(CASFacade)
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
	coordinator := GlobalShutdownCoordinator
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

// Worker already running

// Emit worker start event via coordinator

// Create WaitGroup for worker using manager (nil-safe: builder no-ops Add/Done when wg is nil)

// Mark as stopped when done

// Emit worker stop event via coordinator

// If clamped to 0, shut down immediately when there is no work

// Process any remaining items in batch before shutdown

// New work arrived - reset idle timer

// Drain queue up to batch size so we process in larger batches when work has accumulated

// Batch is full - process immediately

// Reset batch

// Partial batch: reset batch ticker so we wait a full timeout from this last item
// before processing, giving time for more enqueues to accumulate

// Batch timeout reached - process current batch

// Reset batch

// Idle timeout reached - check if we should shut down

// Queue is empty and no batch pending - check if we've been idle long enough

// Been idle long enough - shut down worker (on-demand pattern)
// Worker will wake up again when new work arrives (via wakeWorkerIfNeeded)

// Exit worker goroutine

// Not idle long enough yet - reset timer

// There's work - reset idle timer

// processBatch processes a batch of cleanup requests
// Creates audit events and records metrics for visibility

// Cleanup failed - check if we should retry

// Retry later

// Max retries exceeded - log and give up

// Re-queue failed items for retry (with exponential backoff)
// Use RetryConfig pattern for consistent backoff calculation

// Calculate backoff delay using RetryConfig pattern
// For batch retries, use retry count to determine delay

// Cap retry count at reasonable limit to avoid excessive delays

// Log error but continue with other retries

// Record metrics

// Emit batch processing event via coordinator (unified observability)

// Also create direct audit event for backward compatibility (if coordinator not available)
// This ensures we still have audit trail even without coordinator integration

// RemoveOrphanCASHashFileSync removes a superseded CAS hash file using the same rename-then-delete
// strategy as the orphan cleanup queue worker: rename away from the `^[a-f0-9]{64}\.yaml$` pattern
// first so list/validation and discovery scans do not observe two live blobs for the same logical update.
//
// Used by the queue worker and as a synchronous fallback when EnqueueCleanup fails (full channel,
// shutdown) so CAS updates do not leave stale hash files next to the new content.

// refuseOrphanCASHashDelete is true when deleting filePath would wipe the only
// remaining hash-named YAML for that object id (failed CAS write + orphan enqueue,
// or a sole blob misclassified as superseded). Replacement = another live hash
// file in the same kind dir (or a sibling CAS bucket) whose top-level id matches.
// TRACK: BLI-1785723654802038000-b14064bc
func refuseOrphanCASHashDelete(filePath string, keeperHash ...string) bool {
	base := filepath.Base(filePath)
	if !filecas.CasHashFilenameRe.MatchString(base) {
		return false
	}
	keeper := ""
	if len(keeperHash) > 0 {
		keeper = keeperHash[0]
	}
	if keeper != emptyValue {
		keeperBase := keeper + ".yaml"
		if base == keeperBase {
			return true
		}
		// Update just wrote keeper; do not treat the predecessor as sole survivor
		// when the replacement is not yet visible to a directory scan.
		// TRACK: BLI-CEF-R19-CAS-INTERMEDIATE-LEAK-001
		return false
	}
	oid := filecas.CasHashFilePeekObjectID(filePath)
	if oid == emptyValue {
		return true
	}
	dir := filepath.Dir(filePath)
	if anotherLiveCASHashHasObjectID(dir, oid, filePath) {
		return false
	}
	parent := filepath.Dir(dir)
	if parent != emptyValue && parent != dir && anotherLiveCASHashHasObjectID(parent, oid, filePath) {
		return false
	}
	if isTwoCharCASBucketName(filepath.Base(dir)) && parent != emptyValue {
		entries, err := fileutil.ReadDir(parent)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() || e.Name() == filepath.Base(dir) {
					continue
				}
				if !isTwoCharCASBucketName(e.Name()) {
					continue
				}
				if anotherLiveCASHashHasObjectID(filepath.Join(parent, e.Name()), oid, filePath) {
					return false
				}
			}
		}
	}
	return true
}

func isTwoCharCASBucketName(name string) bool {
	if len(name) != 2 {
		return false
	}
	for i := 0; i < 2; i++ {
		c := name[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

func anotherLiveCASHashHasObjectID(dir, oid, exceptPath string) bool {
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		return false
	}
	exceptAbs, exceptErr := filepath.Abs(exceptPath)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !filecas.CasHashFilenameRe.MatchString(e.Name()) {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if exceptErr == nil {
			if abs, absErr := filepath.Abs(p); absErr == nil && abs == exceptAbs {
				continue
			}
		} else if p == exceptPath {
			continue
		}
		if filecas.CasHashFilePeekObjectID(p) == oid {
			return true
		}
	}
	return false
}

// removeRetiredCASHashFileOnIDChange deletes the old CAS blob after an ID rename.
// refuseOrphanCASHashDelete would keep it: the new blob peeks as newID, so the old
// file looks like the sole survivor for oldID.
// TRACK: BLI-1785723654802038000-b14064bc
func removeRetiredCASHashFileOnIDChange(filePath string) error {
	if filePath == emptyValue {
		return nil
	}
	if err := fileutil.RemoveFile(filePath); err != nil && !fileutil.IsNotExist(err) {
		return err
	}
	return nil
}

// RemoveOrphanCASHashFileSync removes a superseded CAS hash file using the same rename-then-delete
// strategy as the orphan cleanup queue worker: rename away from the `^[a-f0-9]{64}\.yaml$` pattern
// first so list/validation and discovery scans do not observe two live blobs for the same logical update.
func RemoveOrphanCASHashFileSync(filePath string, keeperHash ...string) error {
	if refuseOrphanCASHashDelete(filePath, keeperHash...) {
		StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Warn(LogEventStorageCASOrphanRefusedSoleSurvivor).
			String("file_path", filePath).
			Log()
		return nil
	}
	tmpPath := filePath + ".tmp"
	// Hide from validation by renaming to .tmp (hash pattern is ^[a-f0-9]{64}\.yaml$)
	if err := fileutil.RenameFile(filePath, tmpPath); err != nil {
		if fileutil.IsNotExist(err) {
			// Original already gone; maybe already renamed on a prior run
			if rmErr := fileutil.RemoveFile(tmpPath); rmErr != nil && !fileutil.IsNotExist(rmErr) {
				logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ConstStreamFailedToRemoveTmpOrphanFileStrValN, tmpPath, rmErr), nil).Log()
			}
			return nil
		}
		// Fallback: delete in place (e.g. cross-filesystem rename)
		if removeErr := fileutil.RemoveFile(filePath); removeErr != nil && !fileutil.IsNotExist(removeErr) {
			return errfmt.Errorf(ConstStreamFailedToRemoveOrphanedFileStrErr, filePath, removeErr)
		}
		return nil
	}
	if err := fileutil.RemoveFile(tmpPath); err != nil && !fileutil.IsNotExist(err) {
		return errfmt.Errorf(ConstStreamFailedToRemoveOrphanedTmpFileStrErr, tmpPath, err)
	}
	dir := filepath.Dir(filePath)
	if dirFD, err := fileutil.Open(dir); err == nil {
		if syncErr := dirFD.Sync(); syncErr != nil {
			logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ConstStreamFailedToSyncDirStrAfterOrphanRemovalValN, dir, syncErr), nil).Log()
		}
		if closeErr := dirFD.Close(); closeErr != nil {
			logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ConstStreamFailedToCloseDirStrAfterOrphanRemovalValN, dir, closeErr), nil).Log()
		}
	}
	return nil
}

// cleanupFile performs the actual file deletion (delegates to RemoveOrphanCASHashFileSync).
func (q *CASOrphanCleanupQueue) cleanupFile(filePath string) error {
	return RemoveOrphanCASHashFileSync(filePath)
}

// emitBatchProcessingEvent emits a batch processing event via coordinator
// This provides unified observability through coordinator (logging, audit, metrics, operational)
// Skips emission for successful single-file cleanups to avoid log spam when orphans are produced one-at-a-time (e.g. trickle of CAS updates).

// Coordinator not available - skip

// Use system context for background event emission

// Emit via coordinator (async, non-blocking)

// emitWorkerLifecycleEvent emits a worker lifecycle event via coordinator

// Coordinator not available - skip

// Use system context for background event emission

// Emit via coordinator (async, non-blocking)

// createCleanupBatchAuditEvent creates an audit event for a cleanup batch
// This provides visibility into background cleanup operations
// NOTE: This is a fallback for when coordinator is not available
// When coordinator is available, events should flow through coordinator instead

// Lock-free reads using atomic.Value

// Can't create audit event without project root and storage
// Best effort

// Build operation description
// Use instance builder helper to create audit event
// Use system context for background audit event creation

// Use orphan_cleanup_complete event type (validated event type from audit_event spec)
// This is a batch completion event, so use the complete variant

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

// CleanupOrphanedDrafts scans the project's draft directory (.zqk/drafts or custom)
// and enqueues files older than maxAge for background deletion via the orphan cleanup queue.
// Returns the count of enqueued orphan draft files.
func (q *CASOrphanCleanupQueue) CleanupOrphanedDrafts(ctx context.Context, draftsDir string, maxAge time.Duration) (int, error) {
	if draftsDir == emptyValue {
		pRoot := q.GetProjectRoot()
		if pRoot == emptyValue {
			return 0, nil
		}
		draftsDir = filepath.Join(pRoot, paths.ProjectDataDir, paths.DraftsSubdir)
	}

	entries, err := fileutil.ReadDir(draftsDir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return 0, nil
		}
		return 0, errfmt.Newf("read drafts directory %s", draftsDir).Wrap(err)
	}

	cutoff := time.Now().Add(-maxAge)
	enqueued := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filePath := filepath.Join(draftsDir, entry.Name())
		info, ierr := entry.Info()
		if ierr != nil {
			continue
		}
		if maxAge > 0 && info.ModTime().After(cutoff) {
			continue
		}
		if err := q.EnqueueCleanup(filePath); err == nil {
			enqueued++
		}
	}
	return enqueued, nil
}

// Exported for testing root package
func NewCASOrphanCleanupQueueForTest(metrics *CASMetrics) *CASOrphanCleanupQueue {
	ctx, cancel := context.WithCancel(context.Background())
	return &CASOrphanCleanupQueue{
		queue:     make(chan *orphanCleanupRequest, 100),
		ctx:       ctx,
		cancel:    cancel,
		wgManager: GlobalNewWaitGroupManager(),
		metrics:   metrics,
	}
}
