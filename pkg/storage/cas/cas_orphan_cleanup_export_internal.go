//go:build !production

package cas

import (
	"context"
	"sync"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
)

// CasOrphanCleanupBatchSizeForTest is the max orphan cleanup batch size.
const CasOrphanCleanupBatchSizeForTest = casOrphanCleanupBatchSize

// dummyWaitGroupManager implements WaitGroupManager for testing if needed
type dummyWaitGroupManager struct {
	wg sync.WaitGroup
}

func (d *dummyWaitGroupManager) CreateGroupForGoroutine(id, operation string) *sync.WaitGroup {
	return &d.wg
}
func (d *dummyWaitGroupManager) Add(name string, delta int) { d.wg.Add(delta) }
func (d *dummyWaitGroupManager) Done(name string)           { d.wg.Done() }
func (d *dummyWaitGroupManager) Wait(name string)           { d.wg.Wait() }

func NewCASOrphanCleanupQueueWithCancelForTest(parent context.Context) (queue *CASOrphanCleanupQueue, cancel context.CancelFunc) {
	ctxQueue, cancel := context.WithCancel(parent)
	q := &CASOrphanCleanupQueue{
		queue:     make(chan *orphanCleanupRequest, 1000),
		ctx:       ctxQueue,
		cancel:    cancel,
		wgManager: &dummyWaitGroupManager{},
		metrics:   GetObjectStorageMetrics(),
		secCtx:    pkgctx.NewSystemSecurityContext(),
		logger:    logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
	return q, cancel
}

func (q *CASOrphanCleanupQueue) QueueForTest() chan *orphanCleanupRequest {
	return q.queue
}

func (q *CASOrphanCleanupQueue) ProcessBatchForTest(batch []*orphanCleanupRequest) {
	q.processBatch(batch)
}

type OrphanCleanupRequestForTest = orphanCleanupRequest

func GetOrphanCleanupEventCallbackForTest() OrphanCleanupEventCallback {
	return getOrphanCleanupEventCallback()
}
