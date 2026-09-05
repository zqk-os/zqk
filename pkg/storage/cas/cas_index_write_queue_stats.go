package cas

import (
	"fmt"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/locknames"
)

// GetQueueStats returns statistics about queue state (for monitoring)
// Thread-safe
func (q *ListingIndexWriteQueue) GetQueueStats() map[string]any {
	var queuesCopy map[string]*indexQueue
	if err := concurrency.RunInRLockWithLogger(&q.mu, locknames.LockNameListingIndexGetStatsCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		queuesCopy = make(map[string]*indexQueue)
		for k, v := range q.queues {
			queuesCopy[k] = v
		}
		return nil
	}); err != nil {
		logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ConstStreamFailedToGetCasIndexQueueStatsCopyValN, err), nil).Log()
	}

	stats := make(map[string]any)
	stats["queue_count"] = len(queuesCopy)

	queueDetails := make(map[string]any)
	for kind, queue := range queuesCopy {
		var queueLen int
		if err := concurrency.RunInRLockWithLogger(&queue.mu, locknames.LockNameListingIndexGetStatsQueue, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
			queueLen = len(queue.queue)
			return nil
		}); err != nil {
			logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ConstStreamFailedToGetCasIndexQueueStatsForKindStrValN, kind, err), nil).Log()
		}

		workerRunning := queue.workerRunning.Load() == 1
		queueDetails[kind] = map[string]any{
			ConstStreamPendingUpdates: queueLen,
			objects.FieldKeyBatchSize: queue.batchSize,
			"timeout":                 queue.timeout.String(),
			ConstStreamWorkerRunning:  workerRunning,
			"total_enqueued":          queue.enqueuedTotal.Load(),
			"total_processed":         queue.processedTotal.Load(),
		}
	}
	stats["queues"] = queueDetails

	return stats
}

// GetPendingCount implements QueueShutdownHandler
func (q *ListingIndexWriteQueue) GetPendingCount() int64 {
	var total int64
	if err := concurrency.RunInRLockWithLogger(&q.mu, locknames.LockNameListingIndexGetPendingCount, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		for _, queue := range q.queues {
			total += int64(len(queue.queue))
		}
		return nil
	}); err != nil {
		logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ConstStreamFailedToGetCasIndexTotalPendingCountValN, err), nil).Log()
	}
	return total
}

// GetName implements QueueShutdownHandler
func (q *ListingIndexWriteQueue) GetName() string {
	return ConstStreamListingIndexWriteQueue
}

// IsCritical implements QueueShutdownHandler
// CAS index writes are critical - must complete before shutdown
func (q *ListingIndexWriteQueue) IsCritical() bool {
	return true
}
