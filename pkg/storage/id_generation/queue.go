package id_generation

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
)

// IDQueue is a thread-safe queue that holds pre-generated IDs
// This eliminates contention by allowing threads to pop IDs without directory locks
type IDQueue struct {
	mu          sync.RWMutex
	ids         []string          // Pre-generated IDs
	maxSize     int               // Maximum queue size (target size)
	minSize     int               // Minimum queue size (triggers refill)
	kindDir     string            // Directory for this queue
	kind        string            // Object kind
	prefix      string            // ID prefix
	minDigits   int               // Minimum digits for formatting
	startAt     int               // Starting sequence number
	generator   *BatchIDGenerator // Underlying generator (for refilling)
	lastRefill  time.Time         // Last time queue was refilled
	refillCount int64             // Total number of refills (for metrics)
	ctx         context.Context   // Parent context from command entry point
}

// NewIDQueue creates a new ID queue with specified buffer size
// The queue is pre-filled immediately to reduce initial latency
// ctx: parent context from command entry point (should not be created here)
func NewIDQueue(ctx context.Context, kindDir, kind, prefix string, minDigits, startAt, bufferSize int) *IDQueue {
	// Create underlying generator for refilling
	generator := NewBatchIDGenerator(ctx, kindDir, kind, prefix, minDigits, startAt)

	// Calculate min/max sizes based on buffer size
	// minSize triggers refill, maxSize is target after refill
	minSize := bufferSize / 4 // Refill when queue drops to 25%
	if minSize < 10 {
		minSize = 10 // Minimum threshold
	}
	maxSize := bufferSize
	if maxSize < 50 {
		maxSize = 50 // Minimum buffer size
	}

	queue := &IDQueue{
		ids:         make([]string, 0, maxSize),
		maxSize:     maxSize,
		minSize:     minSize,
		kindDir:     kindDir,
		kind:        kind,
		prefix:      prefix,
		minDigits:   minDigits,
		startAt:     startAt,
		generator:   generator,
		lastRefill:  time.Time{},
		refillCount: 0,
		ctx:         ctx,
	}

	// Pre-fill synchronously so no goroutine holds kindDir files after QueueManager.Stop() / test TempDir cleanup.
	// (Async initial refill raced Stop(): TempDir RemoveAll "directory not empty" on darwin.)
	initCtx, cancelInit := context.WithTimeout(ctx, 30*time.Second)
	var err_swallow_3 = queue.Refill(initCtx)
	if err_swallow_3 != nil {
		logging.LogSwallowedError(

			// Pop removes and returns an ID from the queue
			// Returns error if queue is empty (should trigger refill)
			// This method signals consumption activity - if queue drops below threshold,
			// the manager's worker will detect it during its next check cycle
			err_swallow_3)
	}
	cancelInit()

	return queue
}

func (q *IDQueue) Pop() (string, error) {
	var id string
	var err error
	_ = concurrency.RunInLockOrLog(
		&q.mu, locknames.LockNameIdQueuePop, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if len(q.ids) == 0 {
				err = errfmt.Errorf("queue empty")
				return nil
			}

			// Pop from front (FIFO)
			id = q.ids[0]
			q.ids = q.ids[1:]

			// Note: We don't wake worker here because:
			// 1. Worker checks queues periodically (1 second interval)
			// 2. Worker will detect low queue on next check
			// 3. This avoids overhead of atomic operations on every Pop()
			// 4. Worker stays active as long as any queue needs refill

			return nil
		},
	)
	return id, err
}

// Size returns the current queue size
func (q *IDQueue) Size() int {
	var size int
	_ = concurrency.RunInRLockOrLog(
		&q.mu, locknames.LockNameIdQueueSize, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			size = len(q.ids)
			return nil
		},
	)
	return size
}

// NeedsRefill returns true if queue size is below minimum threshold
func (q *IDQueue) NeedsRefill() bool {
	return q.Size() < q.minSize
}

// Refill generates a batch of IDs and adds them to the queue
// This is called by the background worker when queue is low
func (q *IDQueue) Refill(ctx context.Context) error {
	var needed int
	var generator *BatchIDGenerator
	_ = concurrency.RunInLockOrLog(
		&q.mu, locknames.LockNameIdQueueRefillCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Calculate how many IDs to generate (fill to maxSize)
			currentSize := len(q.ids)
			needed = q.maxSize - currentSize
			generator = q.generator
			return nil
		},
	)

	if needed <= 0 {
		return nil // Already at or above max size
	}

	// Generate batch of IDs using underlying generator (outside lock - may do I/O)
	// This may scan directory, but only happens during refill (not on every ID request)
	ids, err := generator.GenerateBatchIDs(needed)
	if err != nil {
		return errfmt.Newf(ConstFailedToGenerateBatchIDs).Wrap(err)
	}

	// Append to queue (re-acquire lock)
	_ = concurrency.RunInLockOrLog(
		&q.mu, locknames.LockNameIdQueueRefillAppend, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			q.ids = append(q.ids, ids...)
			q.lastRefill = time.Now()
			q.refillCount++
			return nil
		},
	)

	return nil
}

// ForceRefill forces a directory scan and refill (used when queue loses sync)
func (q *IDQueue) ForceRefill(ctx context.Context) error {
	// Reset generator to force rescan
	q.generator.Reset()

	// Clear queue and refill
	_ = concurrency.RunInLockOrLog(
		&q.mu, locknames.LockNameIdQueueForceRefillClear, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			q.ids = q.ids[:0] // Clear but keep capacity
			return nil
		},
	)

	return q.Refill(ctx)
}

// GetStats returns queue statistics
func (q *IDQueue) GetStats() QueueStats {
	var stats QueueStats
	_ = concurrency.RunInRLockOrLog(
		&q.mu, locknames.LockNameIdQueueGetStats, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			stats = QueueStats{
				CurrentSize: len(q.ids),
				MaxSize:     q.maxSize,
				MinSize:     q.minSize,
				LastRefill:  q.lastRefill,
				RefillCount: q.refillCount,
			}
			return nil
		},
	)
	return stats
}

// QueueStats contains statistics about an ID queue
type QueueStats struct {
	CurrentSize int
	MaxSize     int
	MinSize     int
	LastRefill  time.Time
	RefillCount int64
}

// QueueManager manages multiple ID queues and maintains them via background worker
// Uses on-demand pattern: worker wakes when queues need refill, shuts down when idle
type QueueManager struct {
	mu             sync.RWMutex
	queues         map[string]*IDQueue // Key: kindDir:kind:prefix
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	refillInterval time.Duration // How often to check queues (when worker is active)
	checkInterval  time.Duration // How often to check individual queues
	workerRunning  atomic.Int32  // Atomic flag: 1 if worker is running, 0 if not
}

var (
	globalQueueManager     *QueueManager
	globalQueueManagerOnce sync.Once
)

const (
	// idQueueRefillCheckInterval is how often to check queues when worker is active
	idQueueRefillCheckInterval = 1 * time.Second
	// idQueueIdleTimeout is how long to wait before shutting down worker when idle
	idQueueIdleTimeout = 5 * time.Minute
)

// GetGlobalQueueManager returns the global queue manager (singleton)
// ctx: parent context from command entry point (should not be created here)
func GetGlobalQueueManager(ctx context.Context) *QueueManager {
	globalQueueManagerOnce.Do(func() {
		// Derive cancellation context from parent (command context)
		ctx, cancel := context.WithCancel(ctx)
		globalQueueManager = newQueueManager(ctx, cancel)
	})
	return globalQueueManager
}

// NewQueueManager creates a new QueueManager with its own context and worker.
// Use this for tests or when a non-global (isolated) manager is needed so tests
// can run in parallel without sharing state. Caller must call Stop() or cancel
// the context when done to clean up the worker.
func NewQueueManager(ctx context.Context) *QueueManager {
	ctx, cancel := context.WithCancel(ctx)
	return newQueueManager(ctx, cancel)
}

// newQueueManager creates a QueueManager with the given context and cancel.
func newQueueManager(ctx context.Context, cancel context.CancelFunc) *QueueManager {
	return &QueueManager{
		queues:         make(map[string]*IDQueue),
		ctx:            ctx,
		cancel:         cancel,
		refillInterval: idQueueRefillCheckInterval,
		checkInterval:  5 * time.Second, // Not used in on-demand pattern
		// workerRunning starts at 0 (default for atomic.Int32) - Worker starts stopped (on-demand)
	}
}

// GetOrCreateQueue gets or creates an ID queue for a specific kind+directory+prefix
// bufferSize: target queue size (configurable based on volume)
func (qm *QueueManager) GetOrCreateQueue(kindDir, kind, prefix string, minDigits, startAt, bufferSize int) *IDQueue {
	key := fmt.Sprintf("%s:%s:%s", kindDir, kind, prefix)

	// Fast path: check if queue exists
	var queue *IDQueue
	var exists bool
	_ = concurrency.RunInRLockOrLog(
		&qm.mu, locknames.LockNameIdQueueManagerGetFast, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			queue, ok = qm.queues[key]
			exists = ok
			return nil
		},
	)

	if exists {
		return queue
	}

	// Slow path: create new queue
	_ = concurrency.RunInLockOrLog(
		&qm.mu, locknames.LockNameIdQueueManagerGetCreate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Double-check after acquiring write lock
			if existingQueue, ok := qm.queues[key]; ok {
				queue = existingQueue
				return nil
			}

			// Create new queue (pre-fills in background)
			queue = NewIDQueue(qm.ctx, kindDir, kind, prefix, minDigits, startAt, bufferSize)
			qm.queues[key] = queue

			// Wake worker if needed (on-demand pattern)
			// Worker will check this queue and refill if needed
			qm.wakeWorkerIfNeeded()
			return nil
		},
	)

	return queue
}

// wakeWorkerIfNeeded starts the worker if it's not already running
// Called when queues are created or when consumption activity is detected
func (qm *QueueManager) wakeWorkerIfNeeded() {
	// Try to set workerRunning from 0 to 1 (atomic compare-and-swap)
	if qm.workerRunning.CompareAndSwap(0, 1) {
		// Successfully acquired - start worker
		qm.startBackgroundWorker()
	}
}

// startBackgroundWorker starts the background worker that maintains queue sizes
// Uses on-demand pattern: processes queues, then shuts down after idle timeout
func (qm *QueueManager) startBackgroundWorker() {
	goroutinelabels.NewGoroutine(ConstIdQueueManagerRefillWorker, ConstOnDemandRefillingOfIDQueues).
		WithWaitGroup(&qm.wg).
		StartWithContext(qm.ctx, func(ctx context.Context) error {
			defer qm.workerRunning.Store(0) // Reset flag on exit

			// Emit worker lifecycle event via coordinator (if available)
			qm.emitWorkerLifecycleEvent("start", nil)

			// Create tickers for refill checks and idle timeout
			refillTicker := time.NewTicker(qm.refillInterval)
			defer refillTicker.Stop()

			idleTicker := time.NewTimer(idQueueIdleTimeout)
			defer idleTicker.Stop()

			lastWorkTime := time.Now()

			for {
				select {
				case <-ctx.Done():
					// Process any pending refills before shutdown
					qm.refillQueues()
					qm.emitWorkerLifecycleEvent("stop", ctx.Err())
					return ctx.Err()

				case <-refillTicker.C:
					// Check queues and refill if needed
					hasWork := qm.refillQueues()
					if hasWork {
						// Reset idle timer - we did work
						lastWorkTime = time.Now()
						if !idleTicker.Stop() {
							<-idleTicker.C
						}
						idleTicker.Reset(idQueueIdleTimeout)
					}

				case <-idleTicker.C:
					// Idle timeout reached - check if we should shut down
					hasWork := qm.hasQueuesNeedingRefill()
					if !hasWork {
						// No work - check if we've been idle long enough
						idleDuration := time.Since(lastWorkTime)
						if idleDuration >= idQueueIdleTimeout {
							// Been idle long enough - shut down worker (on-demand pattern)
							qm.emitWorkerLifecycleEvent("stop", nil)
							return nil // Exit worker goroutine
						}
						// Not idle long enough yet - reset timer
						idleTicker.Reset(idQueueIdleTimeout - idleDuration)
					} else {
						// There's work - reset idle timer
						lastWorkTime = time.Now()
						idleTicker.Reset(idQueueIdleTimeout)
					}
				}
			}
		})
}

// hasQueuesNeedingRefill checks if any queues need refill (without actually refilling)
func (qm *QueueManager) hasQueuesNeedingRefill() bool {
	var needsRefill bool
	var queues []*IDQueue
	_ = concurrency.RunInRLockOrLog(
		&qm.mu, locknames.LockNameIdQueueManagerHasQueuesNeedingRefill, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			queues = make([]*IDQueue, 0, len(qm.queues))
			for _, queue := range qm.queues {
				queues = append(queues, queue)
			}
			return nil
		},
	)

	// Check queues outside lock
	for _, queue := range queues {
		if queue.NeedsRefill() {
			needsRefill = true
			break
		}
	}
	return needsRefill
}

// refillQueues checks all queues and refills those that need it
// Returns true if any refill work was performed
func (qm *QueueManager) refillQueues() bool {
	var queues []*IDQueue
	_ = concurrency.RunInRLockOrLog(
		&qm.mu, locknames.LockNameIdQueueManagerRefillCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			queues = make([]*IDQueue, 0, len(qm.queues))
			for _, queue := range qm.queues {
				queues = append(queues, queue)
			}
			return nil
		},
	)

	if len(queues) == 0 {
		return false
	}

	// Check each queue and refill if needed.
	// Use bounded concurrency so we don't spawn unbounded refill goroutines.
	// IMPORTANT: Wait for every spawned refill to finish before returning — otherwise Stop()/TempDir cleanup
	// can run while refill goroutines still hold files under kindDir (macOS: TempDir RemoveAll directory not empty).
	const maxConcurrentRefills = 10
	semaphore := make(chan struct{}, maxConcurrentRefills)
	var refillDone sync.WaitGroup
	hasWork := false

	for _, queue := range queues {
		if queue.NeedsRefill() {
			hasWork = true
			semaphore <- struct{}{} // Acquire semaphore
			refillDone.Add(1)
			queueRef := queue
			goroutinelabels.NewGoroutine(ConstIdQueueRefill, fmt.Sprintf(ConstRefillingIDQueueForS, queueRef.kind)).
				StartSimple(func() {
					defer refillDone.Done()
					defer func() { <-semaphore }() // Release semaphore

					ctx, cancel := context.WithTimeout(qm.ctx, 10*time.Second)
					defer cancel()

					if err := queueRef.Refill(ctx); err != nil {
						logging.LogSwallowedError(queueRef.ForceRefill(ctx))
					}
				})
		}
	}

	refillDone.Wait()
	return hasWork
}

// Stop stops the background worker
func (qm *QueueManager) Stop() {
	qm.cancel()

	// Wait for worker to finish with deterministic timeout
	stopTimeout := 30 * time.Second
	done := make(chan struct{})
	goroutinelabels.NewGoroutine(ConstIdQueueStopWait, ConstWaitingForIDGenerationQueueWorkerToStop).
		WithCleanup(func() {
			close(done)
		}).
		StartSimple(func() {
			qm.wg.Wait()
		})

	select {
	case <-done:
		// Worker stopped normally
	case <-time.After(stopTimeout):
		// Timeout - log warning but proceed
		// Worker may still be running, but we proceed with stop
	}

	qm.workerRunning.Store(0)
}

// IsWorkerRunning returns true if the background worker is currently running
func (qm *QueueManager) IsWorkerRunning() bool {
	return qm.workerRunning.Load() == 1
}

// InitiateShutdown stops accepting new queue creation/refill requests
// This is called via callback from shutdown coordinator to avoid import cycles
func (qm *QueueManager) InitiateShutdown() error {
	qm.cancel()
	return nil
}

// Drain processes all pending queue refills
// This is called via callback from shutdown coordinator to avoid import cycles
func (qm *QueueManager) Drain(ctx context.Context) error {
	// Cancel context to stop accepting new work
	qm.cancel()

	// Wait for worker to finish with deterministic timeout
	done := make(chan struct{})
	goroutinelabels.NewGoroutine(ConstIdQueueDrainWait, ConstWaitingForIDGenerationQueueWorkerToDrain).
		WithContext(ctx).
		WithCleanup(func() {
			close(done)
		}).
		StartSimple(func() {
			qm.wg.Wait()
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
		return errfmt.Errorf(ConstDrainTimeoutAfterV, drainTimeout)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsDrained returns true if no queues need refill and worker is stopped
func (qm *QueueManager) IsDrained() bool {
	var queues []*IDQueue
	_ = concurrency.RunInRLockOrLog(
		&qm.mu, locknames.LockNameIdQueueManagerIsDrained, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			queues = make([]*IDQueue, 0, len(qm.queues))
			for _, queue := range qm.queues {
				queues = append(queues, queue)
			}
			return nil
		},
	)

	// Check queues outside lock
	for _, queue := range queues {
		if queue.NeedsRefill() {
			return false
		}
	}
	return qm.workerRunning.Load() == 0
}

// GetPendingCount returns the number of queues needing refill
func (qm *QueueManager) GetPendingCount() int64 {
	var queues []*IDQueue
	_ = concurrency.RunInRLockOrLog(
		&qm.mu, locknames.LockNameIdQueueManagerGetPendingCount, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			queues = make([]*IDQueue, 0, len(qm.queues))
			for _, queue := range qm.queues {
				queues = append(queues, queue)
			}
			return nil
		},
	)

	// Count queues needing refill outside lock
	var count int64
	for _, queue := range queues {
		if queue.NeedsRefill() {
			count++
		}
	}
	return count
}

// GetName returns the queue manager name
func (qm *QueueManager) GetName() string {
	return ConstIdGenerationQueueManager
}

// IsCritical returns false - ID generation is not critical
func (qm *QueueManager) IsCritical() bool {
	return false
}

// emitWorkerLifecycleEvent emits worker lifecycle events via coordinator callback
// This allows unified observability without import cycles
func (qm *QueueManager) emitWorkerLifecycleEvent(status string, err error) {
	callback := getIDQueueEventCallback()
	if callback == nil {
		return // No callback set - skip
	}

	ctx := qm.ctx
	callback(
		ctx,
		ConstIdQueueManager,
		ConstWorkerLifecycle,
		status,
		err,
	)
}

// IDQueueEventCallback is a callback function type for emitting lifecycle events
// This allows the queue manager to emit events without creating import cycles
type IDQueueEventCallback func(
	ctx context.Context,
	operationType string,
	eventType string,
	status string,
	err error,
)

var (
	globalIDQueueEventCallback IDQueueEventCallback
	globalIDQueueCallbackMu    sync.RWMutex
)

// SetIDQueueEventCallback sets the callback for emitting lifecycle events
// This should be called by the CLI layer to wire up coordinator integration
func SetIDQueueEventCallback(callback IDQueueEventCallback) {
	_ = concurrency.RunInLockOrLog(
		&globalIDQueueCallbackMu, locknames.LockNameIdQueueSetCallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			globalIDQueueEventCallback = callback
			return nil
		},
	)
}

// getIDQueueEventCallback returns the current callback (if set)
func getIDQueueEventCallback() IDQueueEventCallback {
	var callback IDQueueEventCallback
	_ = concurrency.RunInRLockOrLog(
		&globalIDQueueCallbackMu, locknames.LockNameIdQueueGetCallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			callback = globalIDQueueEventCallback
			return nil
		},
	)
	return callback
}

// GetQueueStats returns statistics for all queues
func (qm *QueueManager) GetQueueStats() map[string]QueueStats {
	var queues map[string]*IDQueue
	_ = concurrency.RunInRLockOrLog(
		&qm.mu, locknames.LockNameIdQueueManagerGetStats, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			queues = make(map[string]*IDQueue)
			for key, queue := range qm.queues {
				queues[key] = queue
			}
			return nil
		},
	)

	// Get stats for each queue outside lock
	stats := make(map[string]QueueStats)
	for key, queue := range queues {
		stats[key] = queue.GetStats()
	}
	return stats
}
