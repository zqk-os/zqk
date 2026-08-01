package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage/locknames"
)

// HashRegistryEventCallback is a callback for emitting events via coordinator
// This avoids import cycles by using dependency injection
type HashRegistryEventCallback func(
	ctx context.Context,
	projectRoot string,
	storage ObjectStorageProvider,
	kind string,
	batchSize int,
	hashCount int,
	duration time.Duration,
	status string,
	err error,
)

var (
	globalHashRegistryEventCallback atomic.Pointer[HashRegistryEventCallback]

	globalHashRegistryTracker   []*HashRegistry
	globalHashRegistryTrackerMu sync.Mutex
)

// SetHashRegistryEventCallback sets the global callback for emitting events via coordinator
// This should be called during system initialization to wire up coordinator integration
func SetHashRegistryEventCallback(callback HashRegistryEventCallback) {
	if callback == nil {
		globalHashRegistryEventCallback.Store(nil)
		return
	}
	ptr := new(HashRegistryEventCallback)
	*ptr = callback
	globalHashRegistryEventCallback.Store(ptr)
}

func getHashRegistryEventCallback() HashRegistryEventCallback {
	ptr := globalHashRegistryEventCallback.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

const (
	// saveBatchSize is the maximum number of save requests to process in a single batch.
	// Kept small to limit memory: each request holds a full copy of the hash map (path→hash).
	saveBatchSize = 100
	// saveQueueCapacity is the max number of save requests queued. When full, SaveAsync drops
	// the new request (worker will persist a recent snapshot). Reduces memory from 1000 full
	// copies to at most saveQueueCapacity + saveBatchSize (see SCHEDULER_DUMP_ANALYSIS_20260302.md).
	saveQueueCapacity = 100
	// saveBatchTimeout is the maximum time to wait before processing a batch (even if not full)
	// Reduced from 100ms to 5ms to prevent Nagle-like delays in sequential object creation (e.g. system init).
	saveBatchTimeout = 5 * time.Millisecond
	// hashRegistryIdleTimeout is the time to wait with empty queue before shutting down worker
	hashRegistryIdleTimeout = 5 * time.Minute
	// hashRegistrySaveMaxWait is the maximum time a caller blocks waiting for Save() to complete.
	// processSave no longer calls file.Sync() (F_FULLFSYNC on macOS), so the actual I/O is fast
	// (write + rename for a small file). 60s is a generous safety net for queue-processing overhead.
	hashRegistrySaveMaxWait = 60 * time.Second
)

// saveRequest represents a save operation queued for processing
type saveRequest struct {
	data map[string]string // Snapshot of hashes to save
	done chan error        // Channel to signal completion
}

// HashRegistry manages hashes for all files of a given object kind
// Uses a single file per kind (e.g., ".backlog_item.hashes") instead of
// individual hash files per object file.
//
// This is the file-based implementation of HashRegistryProvider.
// For graph-based storage, use GraphHashRegistry instead.
//
// Thread Safety:
// - Uses sync.RWMutex for in-process thread safety
// - Mutex is process-local: if a process dies, the lock is automatically released
// - No file-based locking: concurrent access from multiple processes is not protected
// - File operations (ReadFile/WriteFile) are not wrapped with timeouts - they rely on OS defaults
// - Save operations are serialized through a background worker to prevent race conditions
//
// Note: For cross-process locking, file-based locks would be needed, but that's not
// currently implemented as hash registry operations are expected to be fast and
// typically single-process.
type HashRegistry struct {
	kind         string
	dir          string
	hashes       map[string]string // filename -> hash
	mu           sync.RWMutex      // Process-local mutex (released automatically if process dies)
	filePath     string
	lastLoadTime time.Time // Track when registry was last loaded
	fileMtime    time.Time // Track file modification time to avoid unnecessary reloads

	// Save queue for serialized saves
	saveQueue     chan *saveRequest
	workerRunning atomic.Int32 // Atomic flag to track if worker is currently running (on-demand pattern)
	ctx           context.Context
	cancel        context.CancelFunc
	wgManager     *WaitGroupManager // Centralized WaitGroup management

	// Coordinator integration (optional - set via SetProjectRoot/SetStorage)
	projectRoot atomic.Value // Stores string (lock-free reads)
	storage     atomic.Value // Stores ObjectStorageProvider (lock-free reads)

	// skipShutdownCoordinatorCheck when true skips the global coordinator check in Save().
	// Set for registries created by test storage so global shutdown from another test does not fail saves.
	skipShutdownCoordinatorCheck atomic.Bool
}

// SetProjectRoot sets the project root for coordinator event emission
// Uses atomic.Value for lock-free reads (replaces mutex)
func (hr *HashRegistry) SetProjectRoot(projectRoot string) {
	hr.projectRoot.Store(projectRoot)
	// TODO: Emit coordinator event for state change
}

// GetProjectRoot returns the project root (lock-free read)
func (hr *HashRegistry) GetProjectRoot() string {
	if val := hr.projectRoot.Load(); val != nil {
		return val.(string)
	}
	return ""
}

// SetStorage sets the storage provider for coordinator event emission
// Uses atomic.Value for lock-free reads (replaces mutex)
func (hr *HashRegistry) SetStorage(storage ObjectStorageProvider) {
	hr.storage.Store(storage)
	// TODO: Emit coordinator event for state change
}

// GetStorage returns the storage provider (lock-free read)
func (hr *HashRegistry) GetStorage() ObjectStorageProvider {
	if val := hr.storage.Load(); val != nil {
		return val.(ObjectStorageProvider)
	}
	return nil
}

// SetSkipShutdownCoordinatorCheck sets whether Save() should skip the global shutdown coordinator check.
// Used by test storage so one test's global shutdown does not fail another test's saves.
func (hr *HashRegistry) SetSkipShutdownCoordinatorCheck(skip bool) {
	hr.skipShutdownCoordinatorCheck.Store(skip)
}

// Ensure HashRegistry implements HashRegistryProvider
var _ HashRegistryProvider = (*HashRegistry)(nil)

// NewHashRegistry creates a new hash registry for a given object kind
// ctx: parent context from command entry point (should not be created here)
func NewHashRegistry(ctx context.Context, kind, dir string) *HashRegistry {
	// Use kind name for hash file (e.g., "backlog_item" -> ".backlog_item.hashes")
	hashFileName := fmt.Sprintf(".%s.hashes", kind)
	ctx, cancel := context.WithCancel(ctx) //nolint:gosec // G118: cancel stored on HashRegistry; invoked from Close/shutdown
	hr := &HashRegistry{
		kind:      kind,
		dir:       dir,
		hashes:    make(map[string]string),
		filePath:  filepath.Join(dir, hashFileName),
		saveQueue: make(chan *saveRequest, saveQueueCapacity),
		// workerRunning starts at 0 (default for atomic.Int32) - Worker starts stopped (on-demand)
		ctx:       ctx,
		cancel:    cancel,
		wgManager: NewWaitGroupManager(),
	}

	// Register with global tracker for test teardown
	_ = concurrency.RunInLock(&globalHashRegistryTrackerMu, func() error {
		globalHashRegistryTracker = append(globalHashRegistryTracker, hr)
		return nil
	})

	return hr
}

// ShutdownAllHashRegistriesForTesting forcefully shuts down all created HashRegistries.
// Called by project_test_teardown.go to prevent goroutine leaks.
func ShutdownAllHashRegistriesForTesting() {
	ShutdownHashRegistriesForProjectTesting("")
}

// ShutdownHashRegistriesForProjectTesting shuts down HashRegistries belonging to a specific project.
func ShutdownHashRegistriesForProjectTesting(projectRoot string) {
	var regsToShutdown []*HashRegistry
	_ = concurrency.RunInLock(&globalHashRegistryTrackerMu, func() error {
		var remaining []*HashRegistry
		for _, hr := range globalHashRegistryTracker {
			if projectRoot == "" || strings.HasPrefix(hr.dir, projectRoot) {
				regsToShutdown = append(regsToShutdown, hr)
			} else {
				remaining = append(remaining, hr)
			}
		}
		globalHashRegistryTracker = remaining
		return nil
	})
	for _, hr := range regsToShutdown {
		_ = hr.InitiateShutdown()
	}
}

// wakeWorkerIfNeeded starts the worker if it's not already running
// Thread-safe (uses atomic operations)
func (hr *HashRegistry) wakeWorkerIfNeeded() {
	// Check if context is already cancelled before starting worker
	if hr.ctx.Err() != nil {
		// Context cancelled - don't start worker
		return
	}

	// Try to set workerRunning from 0 to 1 (atomic compare-and-swap)
	if hr.workerRunning.CompareAndSwap(0, 1) {
		// Double-check context after acquiring lock (might have been cancelled)
		if hr.ctx.Err() != nil {
			// Context cancelled - reset flag and don't start worker
			hr.workerRunning.Store(0)
			return
		}

		// Successfully acquired lock - start worker
		wgID := fmt.Sprintf(DescHashRegWorker, hr.kind)
		wg := hr.wgManager.CreateGroupForGoroutine(wgID, DescHashRegSaveWorker)
		goroutinelabels.NewGoroutine(DescHashRegSaveWorker, DescProcessBatchHashReg).
			WithWaitGroup(wg).
			StartSimple(hr.startSaveWorker)
	}
	// If worker is already running, do nothing (worker will process the new item)
}

// startSaveWorker starts the background worker that processes save operations in batches
// Implements on-demand pattern: processes batches until idle timeout, then shuts down
// This ensures all saves are serialized and prevents race conditions
// Batches are processed when either:
//   - Batch size reaches saveBatchSize
//   - saveBatchTimeout (100ms) elapses
//
// Only the latest snapshot in each batch is saved (since each request contains the full registry snapshot)
func (hr *HashRegistry) startSaveWorker() {
	defer func() {
		hr.workerRunning.Store(0)
		// Note: wg.Done() is called by goroutinelabels.WithWaitGroup() on goroutine exit
	}()

	// Check context immediately before starting work
	select {
	case <-hr.ctx.Done():
		// Context already cancelled - exit immediately
		return
	default:
	}

	// Emit worker start event via coordinator
	hr.emitBatchEvent("start", 0, 0, 0, nil)

	batch := make([]*saveRequest, 0, saveBatchSize)
	batchTimer := time.NewTimer(saveBatchTimeout)
	defer batchTimer.Stop()

	// Use timer for idle detection (reset when work arrives)
	idleTimer := time.NewTimer(hashRegistryIdleTimeout)
	defer idleTimer.Stop()

	lastWorkTime := time.Now()

	for {
		select {
		case <-hr.ctx.Done():
			// Context cancelled - stop timers immediately and exit
			batchTimer.Stop()
			idleTimer.Stop()
			// Process any pending batch
			if len(batch) > 0 {
				hr.processBatch(batch)
			}
			// Drain any remaining requests and close their done channels
			hr.drainQueueAndCloseChannels(errfmt.Errorf(DescHashRegCtxCancelled))
			hr.emitBatchEvent("shutdown", 0, 0, 0, nil)
			return

		case req, ok := <-hr.saveQueue:
			if !ok {
				// Channel closed - shutdown initiated (event-driven)
				// Stop timers immediately
				batchTimer.Stop()
				idleTimer.Stop()
				// Process any pending batch
				if len(batch) > 0 {
					hr.processBatch(batch)
				}
				// Drain any remaining requests and close their done channels
				hr.drainQueueAndCloseChannels(errfmt.Errorf(DescHashRegShutdownInit))
				hr.emitBatchEvent("shutdown", 0, 0, 0, nil)
				return
			}
			// New work arrived - reset timers
			lastWorkTime = time.Now()
			if !batchTimer.Stop() {
				// Non-blocking drain: the channel may already be empty if the case <-batchTimer.C
				// branch consumed the value without resetting the timer (e.g. empty batch path).
				// A blocking <-batchTimer.C here would deadlock the worker.
				select {
				case <-batchTimer.C:
				default:
				}
			}
			batchTimer.Reset(saveBatchTimeout)
			if !idleTimer.Stop() {
				select {
				case <-idleTimer.C:
				default:
				}
			}
			idleTimer.Reset(hashRegistryIdleTimeout)

			batch = append(batch, req)
			if len(batch) >= saveBatchSize {
				// Batch is full - process immediately
				hr.processBatch(batch)
				batch = batch[:0] // Reset batch
				if !batchTimer.Stop() {
					select {
					case <-batchTimer.C:
					default:
					}
				}
				batchTimer.Reset(saveBatchTimeout)
			}

		case <-batchTimer.C:
			// Batch timeout - process current batch
			// CRITICAL: Check context FIRST - it may have been cancelled while we were waiting
			if hr.ctx.Err() != nil {
				// Context cancelled - stop timers immediately and exit
				batchTimer.Stop()
				idleTimer.Stop()
				// Process any pending batch
				if len(batch) > 0 {
					hr.processBatch(batch)
				}
				// Drain any remaining requests and close their done channels
				hr.drainQueueAndCloseChannels(errfmt.Errorf(DescHashRegCtxCancelled))
				hr.emitBatchEvent("shutdown", 0, 0, 0, nil)
				return
			}
			if len(batch) > 0 {
				lastWorkTime = time.Now()
				hr.processBatch(batch)
				batch = batch[:0] // Reset batch
			}
			// DO NOT reset batchTimer here if batch is empty!
			// Resetting it causes 5ms polling loops which thrash the CPU.
			// The non-blocking drain in the req arrival path handles the expired state safely.

		case <-idleTimer.C:
			// Idle timeout - check if we should shut down (on-demand pattern)
			// CRITICAL: Check context FIRST - it may have been cancelled while we were waiting
			if hr.ctx.Err() != nil {
				// Context cancelled - stop timers immediately and exit
				batchTimer.Stop()
				idleTimer.Stop()
				// Process any pending batch
				if len(batch) > 0 {
					hr.processBatch(batch)
				}
				// Drain any remaining requests and close their done channels
				hr.drainQueueAndCloseChannels(errfmt.Errorf(DescHashRegCtxCancelled))
				hr.emitBatchEvent("shutdown", 0, 0, 0, nil)
				return
			}
			queueSize := len(hr.saveQueue)
			if queueSize == 0 && len(batch) == 0 {
				idleDuration := time.Since(lastWorkTime)
				if idleDuration >= hashRegistryIdleTimeout {
					// Been idle long enough - shut down worker
					// Prevent race condition: Save() might enqueue a request right after len(hr.saveQueue) check
					hr.workerRunning.Store(0)
					if len(hr.saveQueue) > 0 {
						// Work arrived between our check and marking worker as stopped
						if hr.workerRunning.CompareAndSwap(0, 1) {
							// Reclaimed worker status - stay alive and process
							idleTimer.Reset(hashRegistryIdleTimeout)
							continue
						}
					}
					hr.emitBatchEvent("shutdown", 0, 0, 0, nil)
					return
				}
				// Not idle long enough - reset timer
				idleTimer.Reset(hashRegistryIdleTimeout - idleDuration)
			} else {
				// There's work - reset idle timer
				lastWorkTime = time.Now()
				idleTimer.Reset(hashRegistryIdleTimeout)
			}
		}
	}
}

// drainQueueAndCloseChannels drains the save queue and closes all done channels
// This ensures Save() calls don't wait forever when worker exits
// Note: Queue may be closed, so we use non-blocking reads
func (hr *HashRegistry) drainQueueAndCloseChannels(err error) {
	// Drain queue using non-blocking reads (queue may be closed)
	for {
		select {
		case req, ok := <-hr.saveQueue:
			if !ok {
				// Queue closed - can't drain more
				return
			}
			// Send error and close channel (non-blocking)
			select {
			case req.done <- err:
			default:
			}
			close(req.done)
		default:
			// Queue empty or closed - done draining
			return
		}
	}
}

// processBatch processes a batch of save requests.
// Only the latest snapshot is persisted; we nil out other requests' .data so the GC can reclaim
// those maps immediately (reduces peak memory when batch holds many full registry copies).
func (hr *HashRegistry) processBatch(batch []*saveRequest) {
	if len(batch) == 0 {
		return
	}

	// Process only the latest snapshot (since each request contains full registry)
	latestReq := batch[len(batch)-1]
	batchSize := len(batch)
	hashCount := len(latestReq.data)

	// Release other requests' data so GC can reclaim; we only need latestReq.data for processSave
	for i := 0; i < len(batch)-1; i++ {
		batch[i].data = nil
	}

	// Emit batch start event via coordinator
	hr.emitBatchEvent("start", batchSize, hashCount, 0, nil)

	// Process save
	startTime := time.Now()
	err := hr.processSave(latestReq.data)
	duration := time.Since(startTime)

	// Emit batch completion event via coordinator
	status := "complete"
	if err != nil {
		status = "error"
	}
	hr.emitBatchEvent(status, batchSize, hashCount, duration, err)

	// Signal completion to all requests in the batch
	// All requests in the batch get the same result since we only save the latest
	for _, req := range batch {
		// Send error (buffered channel, non-blocking)
		select {
		case req.done <- err:
		default:
		}
		// Always close channel so Save() doesn't wait forever
		close(req.done)
	}
}

// Load loads the hash registry from disk
// OPTIMIZATION: Skip re-read when the file's mtime matches the mtime from the last successful load.
// Do not compare file mtime to lastLoadTime (wall clock): coarse mtimes can appear "before"
// lastLoadTime even when the file changed, causing stale in-memory hashes.
// CRITICAL FIX: Release lock before doing blocking file I/O to prevent deadlocks
func (hr *HashRegistry) Load() error {
	var fileInfo os.FileInfo
	var fileMtime time.Time
	var shouldReload bool
	err := concurrency.RunInLockWithLogger(&hr.mu, locknames.LockNameHashRegistryLoadCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		var statErr error
		fileInfo, statErr = os.Stat(hr.filePath)
		if os.IsNotExist(statErr) {
			hr.hashes = make(map[string]string)
			hr.lastLoadTime = time.Now()
			hr.fileMtime = time.Time{}
			return nil
		}
		if statErr != nil {
			return errfmt.Newf(ErrMsgStatHashReg).Wrap(statErr)
		}
		fileMtime = fileInfo.ModTime()
		if !hr.fileMtime.IsZero() && fileMtime.Equal(hr.fileMtime) {
			return nil
		}
		shouldReload = true
		return nil
	})
	if err != nil {
		return err
	}
	if !shouldReload {
		return nil
	}

	data, err := os.ReadFile(hr.filePath)
	if err != nil {
		return errfmt.Newf(ErrMsgReadHashRegistry).Wrap(err)
	}
	var registry struct {
		Hashes map[string]string `json:"hashes"`
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		return errfmt.Newf(ErrMsgParseHashReg).Wrap(err)
	}

	err = concurrency.RunInLockWithLogger(&hr.mu, locknames.LockNameHashRegistryLoadUpdate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if !hr.fileMtime.IsZero() && fileMtime.Equal(hr.fileMtime) {
			return nil
		}
		hr.hashes = registry.Hashes
		if hr.hashes == nil {
			hr.hashes = make(map[string]string)
		}
		hr.lastLoadTime = time.Now()
		hr.fileMtime = fileMtime
		return nil
	})
	return err
}

// SaveAsync enqueues a save request and returns immediately without waiting for completion.
// Use during WAL replay so apply path stays fast; the background worker persists when it can.
// Caller must not rely on durability before the next synchronous Save() or process exit.
func (hr *HashRegistry) SaveAsync() {
	if hr.ctx.Err() != nil {
		return
	}
	if !hr.skipShutdownCoordinatorCheck.Load() {
		coordinator := GetGlobalShutdownCoordinator()
		if coordinator != nil && coordinator.IsShutdownInitiated() {
			return
		}
	}
	var dataCopy map[string]string
	if err := concurrency.RunInRLockWithLogger(&hr.mu, locknames.LockNameHashRegistrySaveAsyncCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		dataCopy = make(map[string]string, len(hr.hashes))
		maps.Copy(dataCopy, hr.hashes)
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockFailedGen, err).Log()
	}

	req := &saveRequest{
		data: dataCopy,
		done: make(chan error, 1),
	}

	select {
	case hr.saveQueue <- req:
		// Enqueued; worker will process and close req.done (caller does not wait)
		hr.wakeWorkerIfNeeded()
	default:
		// Queue full - drop this request; next Save() or batch will persist latest state
	}
}

// Save saves the hash registry to disk
// Uses a thread-safe queue to serialize all save operations, preventing race conditions
// from concurrent saves. The method enqueues a save request and waits for completion.
// Wakes worker if needed (on-demand pattern)
// Returns ErrShutdownInProgress if shutdown has been initiated
func (hr *HashRegistry) Save() error {
	// CRITICAL: Check context FIRST (fast path, prevents race condition)
	// This ensures we never start workers after shutdown is initiated
	if hr.ctx.Err() != nil {
		return errfmt.Errorf(DescHashRegCtxCancelNoSave)
	}

	// Defensive check: also verify coordinator shutdown status (skip for test registries)
	if !hr.skipShutdownCoordinatorCheck.Load() {
		coordinator := GetGlobalShutdownCoordinator()
		if coordinator != nil && coordinator.IsShutdownInitiated() {
			return errfmt.Errorf(DescShutdownInProgressNoSave)
		}
	}

	// Double-check context before starting worker (prevent TOCTOU race)
	if hr.ctx.Err() != nil {
		return errfmt.Errorf(DescHashRegCtxCancelNoSave)
	}

	// Acquire read lock, copy data, release lock BEFORE enqueuing
	var dataCopy map[string]string
	err := concurrency.RunInRLockWithLogger(&hr.mu, locknames.LockNameHashRegistrySaveCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		dataCopy = make(map[string]string, len(hr.hashes))
		maps.Copy(dataCopy, hr.hashes)
		return nil
	})
	if err != nil {
		return errfmt.Newf(DescCopyingHashRegData).Wrap(err)
	}

	// Create save request
	req := &saveRequest{
		data: dataCopy,
		done: make(chan error, 1),
	}

	// Enqueue save request (non-blocking due to buffered channel)
	select {
	case hr.saveQueue <- req:
		// Request enqueued successfully
		// Wakes worker if needed (on-demand pattern). MUST happen after enqueue.
		hr.wakeWorkerIfNeeded()
	case <-hr.ctx.Done():
		// Context cancelled while enqueueing - don't wait for completion
		return errfmt.Errorf(DescHashRegCtxCancelEnqueue)
	default:
		// Channel is full (shouldn't happen with buffer size 100, but handle gracefully)
		return errfmt.Errorf(DescSaveQueueFull)
	}

	select {
	case <-hr.ctx.Done():
		return errfmt.Errorf(DescHashRegCtxCancelWait)
	default:
	}

	// Wait for completion with a bounded timeout. file.Sync() is no longer called inside processSave
	// so the worker completes quickly (write + atomic rename). 60s is a safety net for queue overhead.
	saveTimer := time.NewTimer(hashRegistrySaveMaxWait)
	defer saveTimer.Stop()
	select {
	case err := <-req.done:
		if hr.ctx.Err() != nil {
			return errfmt.Errorf(DescHashRegCtxCancelWait)
		}
		return err
	case <-hr.ctx.Done():
		return errfmt.Errorf(DescHashRegCtxCancelWait)
	case <-saveTimer.C:
		// Timeout waiting for worker to process the request. This should be rare now that
		// file.Sync() (F_FULLFSYNC on macOS) has been removed from processSave; the worker
		// should complete each batch in milliseconds. Log diagnostics to aid investigation.
		queueLen := len(hr.saveQueue)
		hashCount := len(dataCopy)
		workerRunning := hr.workerRunning.Load() != 0
		var fileSize int64
		if fi, err := os.Stat(hr.filePath); err == nil {
			fileSize = fi.Size()
		}
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		timeoutErr := errfmt.Errorf(DescHashRegSaveNotComplete, hashRegistrySaveMaxWait)
		StorageLog(logger).Error(LogEventStorageHashRegistrySaveTimeoutErr, timeoutErr).
			Kind(hr.kind).
			String("file", hr.filePath).
			Int("hash_count", hashCount).
			Int("queue_len", queueLen).
			Bool(TagWorkerRunning, workerRunning).
			String(TagFileSizeBytes, fmt.Sprintf("%d", fileSize)).
			String("max_wait", hashRegistrySaveMaxWait.String()).
			Log()
		return errfmt.Errorf(DescHashRegSaveNotCompleteBg, hashRegistrySaveMaxWait)
	}
}

// SaveWithContext is like Save but stops waiting when ctx is cancelled (e.g. init timeout).
// Use this during storage init so the daemon does not hang indefinitely on HashRegistry I/O.
// If ctx is nil, behaves as Save() (no timeout).
func (hr *HashRegistry) SaveWithContext(ctx context.Context) error {
	if ctx == nil {
		return hr.Save()
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// Same checks as Save()
	if hr.ctx.Err() != nil {
		return errfmt.Errorf(DescHashRegCtxCancelNoSave)
	}
	if !hr.skipShutdownCoordinatorCheck.Load() {
		coordinator := GetGlobalShutdownCoordinator()
		if coordinator != nil && coordinator.IsShutdownInitiated() {
			return errfmt.Errorf(DescShutdownInProgressNoSave)
		}
	}
	if hr.ctx.Err() != nil {
		return errfmt.Errorf(DescHashRegCtxCancelNoSave)
	}
	var dataCopy map[string]string
	err := concurrency.RunInRLockWithLogger(&hr.mu, locknames.LockNameHashRegistrySaveCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		dataCopy = make(map[string]string, len(hr.hashes))
		maps.Copy(dataCopy, hr.hashes)
		return nil
	})
	if err != nil {
		return errfmt.Newf(DescCopyingHashRegData).Wrap(err)
	}

	req := &saveRequest{
		data: dataCopy,
		done: make(chan error, 1),
	}

	select {
	case hr.saveQueue <- req:
		hr.wakeWorkerIfNeeded()
	case <-hr.ctx.Done():
		return errfmt.Errorf(DescHashRegCtxCancelEnqueue)
	case <-ctx.Done():
		return ctx.Err()
	default:
		return errfmt.Errorf(DescSaveQueueFull)
	}

	select {
	case <-hr.ctx.Done():
		return errfmt.Errorf(DescHashRegCtxCancelWait)
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Wait for completion; respect both registry shutdown and caller context
	select {
	case err := <-req.done:
		if hr.ctx.Err() != nil {
			return errfmt.Errorf(DescHashRegCtxCancelWait)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	case <-hr.ctx.Done():
		return errfmt.Errorf(DescHashRegCtxCancelWait)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// processSave performs the actual file I/O to save the hash registry
// This is called by the background worker, ensuring all saves are serialized.
// Uses json.Marshal (not MarshalIndent) for speed and smaller I/O on large registries.
func (hr *HashRegistry) processSave(hashes map[string]string) error {
	registry := struct {
		Hashes map[string]string `json:"hashes"`
	}{
		Hashes: hashes,
	}
	data, err := json.Marshal(registry)
	if err != nil {
		return errfmt.Newf(ErrMsgMarshalHashReg).Wrap(err)
	}
	data = append(data, '\n')

	// Ensure directory exists before writing
	if err := os.MkdirAll(filepath.Dir(hr.filePath), paths.DirPerm755); err != nil {
		return errfmt.Newf(ErrMsgCreateHashRegDir).Wrap(err)
	}

	// Use atomic file write pattern (write to temp file, then rename)
	tmpFile := hr.filePath + ".tmp"
	file, err := os.OpenFile(tmpFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, paths.FilePerm644)
	if err != nil {
		return errfmt.Newf(ErrMsgOpenTempFile).Wrap(err)
	}

	writeErr := func() error {
		defer file.Close()

		if _, err := file.Write(data); err != nil {
			return errfmt.Newf(ErrMsgWriteHashRegData).Wrap(err)
		}

		// file.Sync() (F_FULLFSYNC on macOS) is intentionally omitted here.
		// It blocked for 10+ minutes when Time Machine or Spotlight was active on the
		// docs/process directory, causing object-creation rollbacks for the hash registry
		// even though the CAS write had already succeeded.
		// The hash registry is a derived/rebuildable cache (see `system check --auto-fix`);
		// hardware-flush durability is not required. The atomic write-to-tmp then rename
		// pattern provides sufficient crash-safety within normal OS operation.
		return nil
	}()

	if writeErr != nil {
		// Clean up temp file on error
		if rmErr := os.Remove(tmpFile); rmErr != nil && !os.IsNotExist(rmErr) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgRemoveTempFile, rmErr).Log()
		}
		return writeErr
	}

	// Ensure directory still exists before rename (defensive check for race conditions)
	// This can happen if the directory was deleted between creation and rename
	dirPath := filepath.Dir(hr.filePath)
	if err := os.MkdirAll(dirPath, paths.DirPerm755); err != nil {
		if rmErr := os.Remove(tmpFile); rmErr != nil && !os.IsNotExist(rmErr) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgRemoveTempFile, rmErr).Log()
		}
		return errfmt.Newf(ErrMsgEnsureDirBeforeRename).Wrap(err)
	}

	// Sync parent directory to ensure it's visible to os.Rename on macOS
	// On macOS, directory metadata caching can cause os.Rename to fail if directory
	// was just created, even though MkdirAll succeeded
	if dir, err := os.Open(dirPath); err == nil {
		//nolint:errcheck // Best effort - directory sync is not critical
		dir.Close()
	}

	// Verify directory exists and is accessible before rename
	// This helps catch cases where directory was deleted between MkdirAll and rename
	if _, err := os.Stat(dirPath); err != nil {
		if rmErr := os.Remove(tmpFile); rmErr != nil && !os.IsNotExist(rmErr) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgRemoveTempFile, rmErr).Log()
		}
		return errfmt.Newf(ErrMsgDirNotAccessible).Wrap(err)
	}

	// Atomic rename: on most filesystems, rename is atomic
	// Retry with exponential backoff if directory doesn't exist (race condition)
	// This handles cases where the directory is deleted between checks
	retryConfig := &RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  10 * time.Millisecond,
		MaxDelay:      100 * time.Millisecond,
		BackoffFactor: 2.0,
	}

	renameErr := ExecuteSimpleRetry(hr.ctx, retryConfig, func() error {
		// Always ensure directory exists before rename (even on retries)
		// This is critical for CAS-based files in date-based directories that might not exist yet
		if mkdirErr := os.MkdirAll(dirPath, paths.DirPerm755); mkdirErr != nil {
			return errfmt.Newf(ErrMsgEnsureDirHashReg).Wrap(mkdirErr)
		}

		// Sync directory after creation/recreation to ensure it's visible
		if dir, dirErr := os.Open(dirPath); dirErr == nil {
			dir.Close()
		}

		// Verify directory exists and is accessible before rename
		if _, statErr := os.Stat(dirPath); statErr != nil {
			return errfmt.Newf(ErrMsgDirNotAccessible).Wrap(statErr)
		}

		// Verify temp file exists before rename (defensive check)
		// The temp file should exist since we just wrote it, but verify to catch issues early
		if _, statErr := os.Stat(tmpFile); statErr != nil {
			return errfmt.Newf(ErrMsgTempFileNotExist).Wrap(statErr)
		}

		// Attempt rename
		if renameErr := os.Rename(tmpFile, hr.filePath); renameErr != nil {
			// Only retry on "no such file or directory" errors
			if !os.IsNotExist(renameErr) {
				// Non-recoverable error - don't retry
				if rmErr := os.Remove(tmpFile); rmErr != nil && !os.IsNotExist(rmErr) {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgRemoveTempFile, rmErr).Log()
				}
				return errfmt.Newf(ErrMsgRenameTempFile).Wrap(renameErr)
			}
			return errfmt.Newf(ErrMsgRenameNotFoundRetry).Wrap(renameErr)
		}

		return nil
	}, nil) // Use default error checker

	if renameErr != nil {
		if rmErr := os.Remove(tmpFile); rmErr != nil && !os.IsNotExist(rmErr) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgRemoveTempFile, rmErr).Log()
		}
		return errfmt.Newf(ConstMiscFailedToSaveHashRegistryAfterRetriesW).Wrap(renameErr)
	}

	// Touch and sync parent dir (best-effort; helps cache-staleness detection).
	parentDir := filepath.Dir(hr.filePath)
	now := time.Now()
	if err := os.Chtimes(parentDir, now, now); err != nil {
		// Best effort
	}
	if dir, err := os.Open(parentDir); err == nil {
		dir.Close()
	}

	// Update file mtime after save (for Load() optimization)
	// This ensures subsequent Load() calls know the file was just updated
	if fileInfo, statErr := os.Stat(hr.filePath); statErr == nil {
		if err := concurrency.RunInLockWithLogger(&hr.mu, locknames.LockNameHashRegistryUpdateMtime, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
			hr.fileMtime = fileInfo.ModTime()
			hr.lastLoadTime = time.Now()
			return nil
		}); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockFailedGen, err).Log()
		}

	}

	return nil
}

// emitBatchEvent emits a batch processing event via coordinator
func (hr *HashRegistry) emitBatchEvent(
	status string,
	batchSize, hashCount int,
	duration time.Duration,
	err error,
) {
	callback := getHashRegistryEventCallback()
	if callback == nil {
		// Coordinator not available - skip
		return
	}

	// Lock-free reads using atomic.Value
	projectRoot := hr.GetProjectRoot()
	storage := hr.GetStorage()
	kind := hr.kind

	if projectRoot == emptyValue || storage == nil {
		// Can't emit without project root and storage
		return
	}

	ctx := hr.ctx

	// Emit via coordinator (async, non-blocking)
	goroutinelabels.NewGoroutine(DescHashRegCoordEvent, fmt.Sprintf(DescEmitBatchProcessEvent, kind)).
		StartSimple(func() {
			callback(
				ctx,
				projectRoot,
				storage,
				kind,
				batchSize,
				hashCount,
				duration,
				status,
				err,
			)
		})
}

// GetHash returns the hash for a given filename, or empty string if not found
func (hr *HashRegistry) GetHash(filename string) string {
	var result string
	if err := concurrency.RunInRLockWithLogger(&hr.mu, locknames.LockNameHashRegistryGetHash, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		result = hr.hashes[filename]
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// SetHash sets the hash for a given filename
			Error(ErrMsgLockFailedGen, err).Log()
	}

	return result
}

func (hr *HashRegistry) SetHash(filename, hash string) {
	if err := concurrency.RunInLockWithLogger(&hr.mu, locknames.LockNameHashRegistrySetHash, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		hr.hashes[filename] = hash
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// DeleteHash removes the hash for a given filename
			Error(ErrMsgLockFailedGen, err).Log()
	}

}

func (hr *HashRegistry) DeleteHash(filename string) {
	if err := concurrency.RunInLockWithLogger(&hr.mu, locknames.LockNameHashRegistryDeleteHash, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		delete(hr.hashes, filename)
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// HasHash returns true if a hash exists for the given filename
			Error(ErrMsgLockFailedGen, err).Log()
	}

}

func (hr *HashRegistry) HasHash(filename string) bool {
	var exists bool
	if err := concurrency.RunInRLockWithLogger(&hr.mu, locknames.LockNameHashRegistryHasHash, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		_, exists = hr.hashes[filename]
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// GetAllHashes returns a copy of all hashes
			Error(ErrMsgLockFailedGen, err).Log()
	}

	return exists
}

func (hr *HashRegistry) GetAllHashes() map[string]string {
	var result map[string]string
	if err := concurrency.RunInRLockWithLogger(&hr.mu, locknames.LockNameHashRegistryGetAllHashes, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		result = make(map[string]string, len(hr.hashes))
		maps.Copy(result, hr.hashes)
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// InitiateShutdown implements QueueShutdownHandler
			// Stops accepting new save operations
			Error(ErrMsgLockFailedGen, err).Log()
	}

	return result
}

func (hr *HashRegistry) InitiateShutdown() error {
	hr.cancel()
	return nil
}

// Drain implements QueueShutdownHandler
// Processes all pending save operations
func (hr *HashRegistry) Drain(ctx context.Context) error {
	// Cancel context to stop accepting new work
	hr.cancel()

	// Wait for worker to finish
	wgID := fmt.Sprintf(DescHashRegWorker, hr.kind)
	done := make(chan struct{})

	// Use callback pattern: wait in goroutine, notify via channel
	// The goroutine will exit when the context is cancelled
	goroutinelabels.NewGoroutine(DescHashRegDrainWait, fmt.Sprintf(DescWaitHashRegWorker, hr.kind)).
		WithContext(ctx).
		WithCleanup(func() {
			select {
			case <-done:
				// Already closed
			default:
				close(done)
			}
		}).
		StartSimple(func() {
			// Wait for worker to complete, but check context periodically
			waitDone := make(chan struct{})
			goroutinelabels.NewGoroutine(DescHashRegDrainWaitInner, fmt.Sprintf(DescWaitWaitGroup, wgID)).
				StartSimple(func() {
					hr.wgManager.Wait(wgID)
					close(waitDone)
				})

			select {
			case <-waitDone:
				// Worker completed
				select {
				case <-done:
				default:
					close(done)
				}
			case <-ctx.Done():
				// Timeout or context cancelled - exit without closing done
				return
			}
		})

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsDrained implements QueueShutdownHandler
func (hr *HashRegistry) IsDrained() bool {
	return len(hr.saveQueue) == 0 && hr.workerRunning.Load() == 0
}

// GetPendingCount implements QueueShutdownHandler
func (hr *HashRegistry) GetPendingCount() int64 {
	return int64(len(hr.saveQueue))
}

// GetName implements QueueShutdownHandler
func (hr *HashRegistry) GetName() string {
	return fmt.Sprintf(DescHashRegName, hr.kind)
}

// IsCritical implements QueueShutdownHandler
// Hash registry saves are critical - must complete before shutdown
func (hr *HashRegistry) IsCritical() bool {
	return true
}
