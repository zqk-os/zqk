package cas

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
)

// getOrCreateQueue gets or creates a queue for a specific kind
// Thread-safe
func (q *ListingIndexWriteQueue) getOrCreateQueue(kind string, cas *filecas.ContentAddressableStorage) *indexQueue {
	var queue *indexQueue
	var exists bool
	if err := concurrency.RunInRLockWithLogger(&q.mu, locknames.LockNameListingIndexGetQueue, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		var ok bool
		queue, ok = q.queues[kind]
		exists = ok
		return nil
	}); err != nil {
		logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ConstStreamFailedToGetCasIndexQueueForKindStrValN, kind, err), nil).Log()
	}

	if exists {
		// Update queue dependencies in case project root / CAS instance changed (common in tests).
		if err := concurrency.RunInLockWithLogger(&queue.mu, locknames.LockNameListingIndexUpdateQueue, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
			queue.cas = cas
			if q.secCtx != nil {
				queue.secCtx = q.secCtx
			}
			return nil
		}); err != nil {
			logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ConstStreamFailedToUpdateCasIndexQueueForKindStrValN, kind, err), nil).Log()
		}
		// Lock-free updates using atomic.Value
		if projectRoot := q.GetProjectRoot(); projectRoot != emptyValue {
			queue.projectRoot.Store(projectRoot)
		}
		if storage := q.GetStorage(); storage != nil {
			queue.storage.Store(storage)
		}
		return queue
	}

	// Need to create queue - acquire write lock
	var createdQueue *indexQueue
	err := concurrency.RunInLockWithLogger(&q.mu, locknames.LockNameListingIndexCreateQueue, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		// Double-check after acquiring write lock (another goroutine might have created it)
		if existingQueue, ok := q.queues[kind]; ok {
			createdQueue = existingQueue
			return nil
		}

		// Create new queue
		// Use project root and storage from parent queue if available (lock-free reads)
		projectRoot := q.GetProjectRoot()
		storageProvider := q.GetStorage()
		secCtx := q.secCtx

		// Use system context for queue initialization
		ctx, cancel := context.WithCancel(pkgctx.NewSystemContext()) //nolint:gosec // G118: cancel stored on indexQueue
		queueID := fmt.Sprintf(ConstStreamCasIndexQueueStrInt, kind, time.Now().UnixNano())
		createdQueue = &indexQueue{
			parentQueue: q,
			kind:        kind,
			queue:       make(chan *indexUpdateRequest, 1000), // Buffered channel for high concurrency
			cas:         cas,
			batchSize:   casIndexBatchSize,
			timeout:     casIndexBatchTimeout,
			// workerRunning starts at 0 (default for atomic.Int32) - Start with worker not running
			secCtx:    secCtx,
			ctx:       ctx,
			cancel:    cancel,
			wgManager: GlobalNewWaitGroupManager(),
			queueID:   queueID,
		}
		// Set projectRoot and storage using atomic.Value (lock-free)
		if projectRoot != emptyValue {
			createdQueue.projectRoot.Store(projectRoot)
		}
		if storageProvider != nil {
			createdQueue.storage.Store(storageProvider)
		}

		q.queues[kind] = createdQueue
		return nil
	})
	if err != nil {
		return nil
	}
	if createdQueue != nil {
		// Wake worker if needed (on-demand pattern) - outside lock
		createdQueue.wakeWorkerIfNeeded()
		return createdQueue
	}
	// Queue was created by another goroutine - get it again
	return q.getOrCreateQueue(kind, cas)
}

// EnqueueUpdate queues an index update for batched processing
// Returns immediately after queuing (file is already written)
// Thread-safe
// Wakes worker if needed (on-demand pattern)
// Returns error if shutdown has been initiated
func (q *ListingIndexWriteQueue) EnqueueUpdate(kind, objectID, hash string, cas *filecas.ContentAddressableStorage) error {
	_, err := q.EnqueueInternal(kind, objectID, hash, "", "", cas, nil, false, false)
	return err
}

// EnqueueUpdateWithCallback queues an update and signals completion via channel
// Thread-safe
// Wakes worker if needed (on-demand pattern)
func (q *ListingIndexWriteQueue) EnqueueUpdateWithCallback(kind, objectID, hash string, cas *filecas.ContentAddressableStorage) (<-chan error, error) {
	return q.EnqueueInternal(kind, objectID, hash, "", "", cas, nil, true, false)
}

// EnqueueUpdateWithOperationCallback queues an update and invokes the provided OperationCallback.
// bucketKey is from the bucket strategy at create time; empty means base dir.
func (q *ListingIndexWriteQueue) EnqueueUpdateWithOperationCallback(kind, objectID, hash, bucketKey string, cas *filecas.ContentAddressableStorage, opCallback concurrency.OperationCallback) (<-chan error, error) {
	return q.EnqueueInternal(kind, objectID, hash, bucketKey, "", cas, opCallback, true, false)
}

// EnqueueUpdateWithOperationCallbackAndCreatedAt queues an update with optional created_at (RFC3339) for high-volume kinds.
// Use when creating audit_event or metrics so the index can serve OldestIDs without a separate cache build.
func (q *ListingIndexWriteQueue) EnqueueUpdateWithOperationCallbackAndCreatedAt(kind, objectID, hash, bucketKey, createdAt string, cas *filecas.ContentAddressableStorage, opCallback concurrency.OperationCallback) (<-chan error, error) {
	return q.EnqueueInternal(kind, objectID, hash, bucketKey, createdAt, cas, opCallback, true, false)
}

// EnqueueRemove queues a remove of the objectID from the CAS index and waits for the queue to process it.
// Used by CAS Delete so the index update is serialized with any pending SetMapping and not overwritten.
// Returns a done channel; caller should FlushKind after enqueue to ensure the remove is persisted.
func (q *ListingIndexWriteQueue) EnqueueRemove(kind, objectID string, cas *filecas.ContentAddressableStorage) (<-chan error, error) {
	return q.EnqueueInternal(kind, objectID, "", "", "", cas, nil, true, true)
}

// EnqueueRemoveNoWait queues a remove without a completion channel. Used by BatchDelete so the caller
// can enqueue many removes then call FlushKind once instead of waiting per ID (avoids 5k waits + 5k FlushKind).
func (q *ListingIndexWriteQueue) EnqueueRemoveNoWait(kind, objectID string, cas *filecas.ContentAddressableStorage) error {
	_, err := q.EnqueueInternal(kind, objectID, "", "", "", cas, nil, false, true)
	return err
}

// enqueue is the common implementation for enqueueing requests with optional completion channel and OperationCallback.
// createdAt is RFC3339 for high-volume kinds (enables OldestIDs); empty = not set.
func (q *ListingIndexWriteQueue) EnqueueInternal(kind, objectID, hash, bucketKey, createdAt string, cas *filecas.ContentAddressableStorage, opCallback concurrency.OperationCallback, wantDone bool, isRemove bool) (<-chan error, error) {
	// Check if shutdown has been initiated (skip for test queues)
	if !q.SkipShutdownCoordinatorCheck.Load() {
		coordinator := GlobalShutdownCoordinator
		if coordinator != nil && coordinator.IsShutdownInitiated() {
			return nil, errfmt.Errorf(ConstStreamShutdownInProgressCannotEnqueueIndexUpdate)
		}
	}

	queue := q.getOrCreateQueue(kind, cas)

	// Prepare callback plumbing
	callback := opCallback
	if callback == nil {
		callback = &concurrency.NoOpOperationCallback{}
	}
	opName := ConstStreamCasIndexUpdate
	if isRemove {
		opName = ConstStreamCasIndexRemove
	}
	operationID := fmt.Sprintf("%s_%s_%s_%d", opName, kind, objectID, time.Now().UnixNano())
	startedAt := time.Now()
	callback.OnStart(operationID, map[string]any{
		objects.FieldKeyKind: kind,
		"objectID":           objectID,
		"hash":               hash,
		"remove":             isRemove,
	})

	// Create request with optional done channel
	var done chan error
	if wantDone {
		done = make(chan error, 1)
	}
	req := &indexUpdateRequest{
		objectID:    objectID,
		hash:        hash,
		bucketKey:   bucketKey,
		createdAt:   createdAt,
		remove:      isRemove,
		done:        done,
		opCallback:  callback,
		operationID: operationID,
		startedAt:   startedAt,
	}

	// Non-blocking send (channel is buffered)
	queue.pendingItems.Add(1)
	queue.enqueuedTotal.Add(1)
	select {
	case queue.queue <- req:
		queue.wakeWorkerIfNeeded()
		return done, nil
	default:
		// Channel full - this shouldn't happen with buffered channel, but handle gracefully
		// In this case, we could block or return error
		// For now, block to ensure update is queued (better than losing it)
		queue.queue <- req
		queue.wakeWorkerIfNeeded()
		return done, nil
	}
}
