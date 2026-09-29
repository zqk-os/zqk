package storage

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/audit"
)

// BatchingObjectStorage is a decorator that batches Create operations to optimize
// database performance, particularly for Neo4j/MemGraph during swarm writes.
// This replaces the previous caching decorator approach.
//
// CLI / SkipWriteBehind creates always pass through synchronously. Batching +
// BulkCreate was observed ACKing create→get ghosts: ensureObjectID ran, FileObjectStorage.Create
// did not, and BulkCreate's nil error was broadcast to every waiter (repair_draft with
// id/kind/title only). TRACK: core-backlog
type BatchingObjectStorage struct {
	ObjectStorageProvider

	mu            sync.Mutex
	createQueue   []batchCreateOp
	createTimer   *time.Timer
	batchSize     int
	flushInterval time.Duration

	enqueuedTotal atomic.Int64
	flushedTotal  atomic.Int64
}

type batchCreateOp struct {
	ctx    context.Context
	secCtx *pkgctx.SecurityContext
	obj    map[string]any
	errCh  chan error
}

// NewBatchingObjectStorage creates a new batching decorator
func NewBatchingObjectStorage(provider ObjectStorageProvider) *BatchingObjectStorage {
	return &BatchingObjectStorage{
		ObjectStorageProvider: provider,
		batchSize:             100,
		flushInterval:         50 * time.Millisecond,
	}
}

// GetPool returns the connection pool if the underlying provider supports it.
func (b *BatchingObjectStorage) GetPool() provider.ConnectionPool {
	if p, ok := b.ObjectStorageProvider.(interface {
		GetPool() provider.ConnectionPool
	}); ok {
		return p.GetPool()
	}
	return nil
}

// GetBatchingStats returns lifetime counters for enqueued and flushed operations.
func (b *BatchingObjectStorage) GetBatchingStats() (enqueued, flushed int64) {
	return b.enqueuedTotal.Load(), b.flushedTotal.Load()
}

func (b *BatchingObjectStorage) shouldBypassBatching(ctx context.Context, _ *pkgctx.SecurityContext) bool {
	if isSkipWriteBehind(ctx) {
		return true
	}
	if pkgctx.GetPromoteOnCreate(ctx) {
		return true
	}
	// Explicit WithCLIOperation marker only. IsCLIOperation also treats bypass_policy
	// as CLI and would disable batching for every system-context graph swarm write.
	return audit.HasCLIMarker(ctx)
}

// Create batches object creations (graph swarm path). CLI / skip-write-behind pass through.
func (b *BatchingObjectStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	if b.shouldBypassBatching(ctx, secCtx) {
		return b.ObjectStorageProvider.Create(ctx, secCtx, obj)
	}

	b.enqueuedTotal.Add(1)
	b.mu.Lock()
	op := batchCreateOp{
		ctx:    ctx,
		secCtx: secCtx,
		obj:    obj,
		errCh:  make(chan error, 1),
	}
	b.createQueue = append(b.createQueue, op)

	if len(b.createQueue) >= b.batchSize {
		if b.createTimer != nil {
			b.createTimer.Stop()
			b.createTimer = nil
		}
		queue := b.createQueue
		b.createQueue = nil
		b.mu.Unlock()

		goroutinelabels.NewGoroutine("batching_storage_flush", "flush create batch").
			StartSimple(func() { b.flushCreates(queue) })
	} else if len(b.createQueue) == 1 {
		b.createTimer = time.AfterFunc(b.flushInterval, func() {
			b.mu.Lock()
			if len(b.createQueue) == 0 {
				b.mu.Unlock()
				return
			}
			queue := b.createQueue
			b.createQueue = nil
			b.createTimer = nil
			b.mu.Unlock()

			b.flushCreates(queue)
		})
		b.mu.Unlock()
	} else {
		b.mu.Unlock()
	}

	select {
	case err := <-op.errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *BatchingObjectStorage) flushCreates(queue []batchCreateOp) {
	if len(queue) == 0 {
		return
	}

	b.flushedTotal.Add(int64(len(queue)))

	if len(queue) == 1 {
		err := b.ObjectStorageProvider.Create(queue[0].ctx, queue[0].secCtx, queue[0].obj)
		queue[0].errCh <- err
		return
	}

	objs := make([]map[string]any, len(queue))
	for i, op := range queue {
		objs[i] = op.obj
	}

	result, err := b.ObjectStorageProvider.BulkCreate(queue[0].ctx, queue[0].secCtx, objs)
	if err != nil {
		for _, op := range queue {
			op.errCh <- err
		}
		return
	}

	errByIndex := make(map[int]error, len(queue))
	if result != nil {
		for _, be := range result.Errors {
			if be.Error != nil {
				errByIndex[be.Index] = be.Error
				continue
			}
			if be.Message != emptyValue {
				errByIndex[be.Index] = errfmt.Errorf("%s", be.Message)
			}
		}
	}

	for i, op := range queue {
		if e, ok := errByIndex[i]; ok {
			op.errCh <- e
			continue
		}
		// Fail closed: BulkCreate often returns err=nil with FailureCount>0. If this
		// object still has no status after bulk, it never went through Create metadata.
		if objects.GetString(op.obj, objects.FieldKeyStatus) == emptyValue {
			id := objects.GetString(op.obj, objects.FieldKeyID)
			op.errCh <- errfmt.Errorf("batched bulk create left object without status (id=%s); refusing ghost ACK", id)
			continue
		}
		op.errCh <- nil
	}
}
