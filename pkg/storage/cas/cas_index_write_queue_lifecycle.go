package cas

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
)

// Shutdown gracefully shuts down the write queue
// Flushes all pending updates before returning
// Thread-safe
func (q *ListingIndexWriteQueue) Shutdown() error {
	var queuesCopy map[string]*indexQueue
	err := concurrency.RunInLockWithLogger(&q.mu, locknames.LockNameListingIndexShutdownCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		queuesCopy = make(map[string]*indexQueue)
		for k, v := range q.queues {
			queuesCopy[k] = v
		}
		q.queues = make(map[string]*indexQueue)
		return nil
	})
	if err != nil {
		return err
	}

	// Close all queues (outside lock)
	var wg sync.WaitGroup
	for _, queue := range queuesCopy {
		close(queue.queue)

		// Wait for workers to finish
		queueCopy := queue
		goroutinelabels.NewGoroutine(ConstStreamCasIndexDrainWait, fmt.Sprintf(ConstStreamWaitingForCasIndexQueueWorkerStr, queueCopy.queueID)).
			WithWaitGroup(&wg).
			StartSimple(func() {
				wgID := fmt.Sprintf("%s_worker", queueCopy.queueID)
				queueCopy.wgManager.Wait(wgID)
			})
	}
	wg.Wait()

	return nil
}

// FlushKind waits for all pending updates for a specific kind to be processed.
// It does not cancel when the caller's context is done; use FlushKindContext for cancellation.
// Thread-safe. Optimized to minimize fixed sleeps when queue already exists (OBJECT_OPERATIONS_PERFORMANCE).
func (q *ListingIndexWriteQueue) FlushKind(kind string, timeout time.Duration) error {
	return q.FlushKindContext(context.Background(), kind, timeout) // Background: request-or-shutdown derived
}

// FlushKindContext is like FlushKind but stops waiting when ctx is canceled and uses the earlier of
// (now+timeout) and ctx's deadline when set. Create passes ctx so this stage respects caller deadlines.
func (q *ListingIndexWriteQueue) FlushKindContext(ctx context.Context, kind string, timeout time.Duration) error {
	var queue *indexQueue
	if err := concurrency.RunInRLockWithLogger(&q.mu, locknames.LockNameListingIndexFlushGet, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if foundQueue, exists := q.queues[kind]; exists {
			queue = foundQueue
		}
		return nil
	}); err != nil {
		return err
	}
	if queue == nil {
		return nil // nothing to flush
	}

	deadline := time.Now().Add(timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for time.Now().Before(deadline) && ctx.Err() == nil {
		var pendingItems int64
		var workerProcessing bool
		if err := concurrency.RunInRLockWithLogger(&queue.mu, locknames.LockNameListingIndexWaitQueueEmpty, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
			pendingItems = queue.pendingItems.Load()
			workerProcessing = queue.workerProcessing.Load() == 1
			return nil
		}); err != nil {
			return err
		}

		if pendingItems == 0 {
			if !workerProcessing {
				return nil
			}
			// If worker is processing, wait for it to finish (loop will block on ticker)
		}

		select {
		case <-ticker.C:
		case <-time.After(time.Until(deadline)):
			return errfmt.Errorf(ConstStreamTimeoutWaitingForQueueToFlushForKindStrPendingInt, kind, pendingItems)
		case <-ctx.Done():
			return errfmt.Errorf(ConstStreamWaitingForListingIndexFlushForKindStrErr, kind, context.Cause(ctx))
		}
	}

	if err := ctx.Err(); err != nil {
		return errfmt.Errorf(ConstStreamWaitingForListingIndexFlushForKindStrErr, kind, context.Cause(ctx))
	}

	var queueLen int
	if err := concurrency.RunInRLockWithLogger(&queue.mu, locknames.LockNameListingIndexFlushFinalCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		queueLen = len(queue.queue)
		return nil
	}); err != nil {
		return err
	}
	return errfmt.Errorf(ConstStreamTimeoutWaitingForQueueToFlushForKindStrPendingInt, kind, queueLen)
}

func (q *ListingIndexWriteQueue) sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return errfmt.Errorf(ConstStreamWaitingForListingIndexQueueErr, context.Cause(ctx))
	}
}

// FlushAll waits for all pending updates across all kinds to be processed.
// Each kind gets the full timeout (total wait can be timeout * len(kinds)).
// For a bounded total wait use FlushAllWithDeadline.
// Thread-safe
func (q *ListingIndexWriteQueue) FlushAll(timeout time.Duration) error {
	return q.FlushAllWithDeadline(time.Now().Add(timeout))
}

// FlushAllWithDeadline waits for all pending updates across all kinds until deadline.
// Each kind gets only the remaining time so total wait is bounded by deadline.
// Thread-safe
func (q *ListingIndexWriteQueue) FlushAllWithDeadline(deadline time.Time) error {
	var kinds []string
	if err := concurrency.RunInRLockWithLogger(&q.mu, locknames.LockNameListingIndexFlushAllCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		kinds = make([]string, 0, len(q.queues))
		for kind := range q.queues {
			kinds = append(kinds, kind)
		}
		return nil
	}); err != nil {
		return err
	}

	for _, kind := range kinds {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return errfmt.Errorf(ConstStreamTimeoutWaitingForAllListingIndexFlushesDeadline)
		}
		if err := q.FlushKind(kind, remaining); err != nil {
			return err
		}
	}

	return nil
}

// FlushAllListingIndexesForProjectRootWithTimeout waits until pending per-kind listing-index
// materialization for projectRoot completes so List() sees recent writes. Use after bulk creates
// (e.g. scenario bundle apply). Bounded by timeout (FlushAllWithDeadline).
func FlushAllListingIndexesForProjectRootWithTimeout(projectRoot string, timeout time.Duration) error {
	q := GetListingIndexWriteQueueForProjectRoot(projectRoot)
	return q.FlushAllWithDeadline(time.Now().Add(timeout))
}

// InitiateShutdown implements QueueShutdownHandler
// Stops accepting new operations
func (q *ListingIndexWriteQueue) InitiateShutdown() error {
	var queuesCopy map[string]*indexQueue
	if err := concurrency.RunInRLockWithLogger(&q.mu, locknames.LockNameListingIndexInitiateShutdownCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		queuesCopy = make(map[string]*indexQueue)
		for k, v := range q.queues {
			queuesCopy[k] = v
		}
		return nil
	}); err != nil {
		return err
	}
	// Cancel all queue contexts to stop accepting new work (outside lock)
	for _, queue := range queuesCopy {
		queue.cancel()
	}
	return nil
}

// Drain implements QueueShutdownHandler
// Processes all pending operations
func (q *ListingIndexWriteQueue) Drain(ctx context.Context) error {
	// Initiate shutdown first
	if err := q.InitiateShutdown(); err != nil {
		return err
	}

	var queues []*indexQueue
	if err := concurrency.RunInRLockWithLogger(&q.mu, locknames.LockNameListingIndexDrainCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		queues = make([]*indexQueue, 0, len(q.queues))
		for _, queue := range q.queues {
			queues = append(queues, queue)
		}
		return nil
	}); err != nil {
		return err
	}

	// Wait for all workers to finish
	var wg sync.WaitGroup
	for _, queue := range queues {
		queueCopy := queue // Capture for goroutine
		goroutinelabels.NewGoroutine(ConstStreamCasIndexDrainWait, fmt.Sprintf(ConstStreamWaitingForCasIndexQueueWorkerStr, queueCopy.queueID)).
			WithWaitGroup(&wg).
			StartSimple(func() {
				wgID := fmt.Sprintf("%s_worker", queueCopy.queueID)
				queueCopy.wgManager.Wait(wgID)
			})
	}

	done := make(chan struct{})
	goroutinelabels.NewGoroutine(ConstStreamCasIndexDrainCollector, ConstStreamWaitingForAllCasIndexQueueDrainsToComplete).
		WithCleanup(func() {
			close(done)
		}).
		StartSimple(func() {
			wg.Wait()
		})

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsDrained implements QueueShutdownHandler
func (q *ListingIndexWriteQueue) IsDrained() bool {
	var drained bool
	if err := concurrency.RunInRLockWithLogger(&q.mu, locknames.LockNameListingIndexIsDrained, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		drained = true
		for _, queue := range q.queues {
			if len(queue.queue) > 0 || queue.workerRunning.Load() == 1 {
				drained = false
				break
			}
		}
		return nil
	}); err != nil {
		logging.
			// If lock fails, we can't reliably say it's drained, but this method returns bool.
			// For best effort, we'll return false and log to stderr as per instructions for best-effort.
			Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ConstStreamFailedToCheckIfListingindexwritequeueIsDrained, err).Log()
		return false
	}
	return drained
}

// FlushKindListingIndexForProjectRootWithTimeout exposes FlushKind for a specific project root.
func FlushKindListingIndexForProjectRootWithTimeout(projectRoot string, kind string, timeout time.Duration) error {
	q := GetListingIndexWriteQueueForProjectRoot(projectRoot)
	return q.FlushKind(kind, timeout)
}
