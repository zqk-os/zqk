package storage

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
)

// CasOrphanCleanupBatchSizeForTest is the max orphan cleanup batch size (see casOrphanCleanupBatchSize).
const CasOrphanCleanupBatchSizeForTest = casOrphanCleanupBatchSize

// NewCASOrphanCleanupQueueForTest returns a fresh queue backed by a cancellable context, matching
// the construction used in TestCASOrphanCleanupQueue_ProcessQueueIfIdle for an isolated instance.
func NewCASOrphanCleanupQueueForTest(parent context.Context) (queue *CASOrphanCleanupQueue, cancel context.CancelFunc) {
	ctxQueue, cancel := context.WithCancel(parent)
	q := &CASOrphanCleanupQueue{
		queue:     make(chan *orphanCleanupRequest, 1000),
		ctx:       ctxQueue,
		cancel:    cancel,
		wgManager: NewWaitGroupManager(),
		metrics:   GetObjectStorageMetrics(),
		secCtx:    pkgctx.NewSystemSecurityContext(),
		logger:    logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
	return q, cancel
}
