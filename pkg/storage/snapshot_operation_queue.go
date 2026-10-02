package storage

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
)

// SnapshotOperationType represents the type of storage operation for snapshot queuing
type SnapshotOperationType int

const (
	SnapshotOpCreate SnapshotOperationType = iota
	SnapshotOpUpdate
	SnapshotOpDelete
	SnapshotOpRead
)

// String returns the string representation of SnapshotOperationType
func (ot SnapshotOperationType) String() string {
	switch ot {
	case SnapshotOpCreate:
		return OpCreate
	case SnapshotOpUpdate:
		return OpUpdate
	case SnapshotOpDelete:
		return OpDelete
	case SnapshotOpRead:
		return "read" // read is distinct from get in snapshot context
	default:
		return "unknown"
	}
}

// SnapshotOperation represents a queued storage operation during snapshot
type SnapshotOperation struct {
	Type      SnapshotOperationType
	ObjectID  string
	Kind      string
	Data      map[string]any // Object data or updates
	Cascade   bool           // For delete operations
	SecCtx    *pkgctx.SecurityContext
	Timestamp time.Time
	Sequence  int64 // For ordering
}

// SnapshotOperationQueue manages queued operations during snapshot capture
type SnapshotOperationQueue struct {
	operations    []*SnapshotOperation
	pendingWrites map[string]bool // Track objects with pending writes
	mu            sync.Mutex
	sequence      int64
	maxSize       int
	logger        logging.Logger
}

// NewSnapshotOperationQueue creates a new operation queue for snapshots
func NewSnapshotOperationQueue(maxSize int) *SnapshotOperationQueue {
	if maxSize <= 0 {
		maxSize = 10000 // Default max size
	}
	return &SnapshotOperationQueue{
		operations:    make([]*SnapshotOperation, 0),
		pendingWrites: make(map[string]bool),
		sequence:      0,
		maxSize:       maxSize,
		logger:        logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// Enqueue adds an operation to the queue
func (q *SnapshotOperationQueue) Enqueue(op *SnapshotOperation) error {
	err := concurrency.RunInLockWithLogger(
		&q.mu, locknames.LockNameSnapshotQueueEnqueue, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Check queue size
			if len(q.operations) >= q.maxSize {
				return errfmt.Errorf(ConstMiscOperationQueueOverflowMaxSizeDReached, q.maxSize)
			}

			// Assign sequence number
			op.Sequence = q.sequence
			q.sequence++

			// Add to queue
			q.operations = append(q.operations, op)

			// Track pending writes
			if op.Type == SnapshotOpCreate || op.Type == SnapshotOpUpdate || op.Type == SnapshotOpDelete {
				q.pendingWrites[op.ObjectID] = true
			}

			StorageLog(q.logger).Debug(LogEventStorageSnapshotOpQueuedDebug).
				String("type", op.Type.String()).
				ObjectID(op.ObjectID).
				String("sequence", fmt.Sprintf("%d", op.Sequence)).
				Log()
			return nil
		},
	)
	return err
}

// HasPendingWrite checks if an object has pending write operations
func (q *SnapshotOperationQueue) HasPendingWrite(objectID string) bool {
	var hasPending bool
	_ = concurrency.RunInLockOrLog(
		&q.mu, locknames.LockNameSnapshotQueueHasPendingWrite, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			hasPending = q.pendingWrites[objectID]
			return nil
		},
	)
	return hasPending
}

// GetDataFromPendingWrite returns data from the most recent pending write for an object
func (q *SnapshotOperationQueue) GetDataFromPendingWrite(objectID string) (map[string]any, error) {
	var result map[string]any
	var resultErr error
	err := concurrency.RunInLockWithLogger(
		&q.mu, locknames.LockNameSnapshotQueueGetPendingWrite, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Find the most recent write for this object
			var lastWrite *SnapshotOperation
			for i := len(q.operations) - 1; i >= 0; i-- {
				queuedOp := q.operations[i]
				if queuedOp.ObjectID == objectID &&
					(queuedOp.Type == SnapshotOpCreate || queuedOp.Type == SnapshotOpUpdate) {
					lastWrite = queuedOp
					break
				}
			}

			if lastWrite != nil {
				// Return data from queued write
				switch lastWrite.Type {
				case SnapshotOpCreate:
					// Return full object data
					result = lastWrite.Data
					return nil
				case SnapshotOpUpdate:
					// For updates, return the updates (caller will need to merge with current state)
					result = lastWrite.Data
					return nil
				}
			}

			// No pending write found (shouldn't happen if HasPendingWrite was true)
			resultErr = errfmt.Errorf(ConstMiscNoPendingWriteFoundForObjectS, objectID)
			return nil
		},
	)
	if err != nil {
		return nil, errfmt.Newf(ConstMiscTimeoutGettingPendingWrite).Wrap(err)
	}
	if resultErr != nil {
		return nil, resultErr
	}
	return result, nil
}

// Replay executes all queued operations in order
func (q *SnapshotOperationQueue) Replay(ctx context.Context, storage ObjectStorageProvider) error {
	var operations []*SnapshotOperation
	err := concurrency.RunInLockWithLogger(
		&q.mu, locknames.LockNameSnapshotQueueReplayCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if len(q.operations) == 0 {
				StorageLog(q.logger).Debug(LogEventStorageSnapshotOpReplayNoneDebug).Log()
				return nil
			}
			// Copy operations for replay (release lock before I/O)
			operations = make([]*SnapshotOperation, len(q.operations))
			copy(operations, q.operations)
			return nil
		},
	)
	if err != nil {
		return errfmt.Newf(ConstMiscTimeoutCopyingOperationsForReplay).Wrap(err)
	}
	if len(operations) == 0 {
		return nil
	}

	// Sort by sequence to ensure order
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].Sequence < operations[j].Sequence
	})

	StorageLog(q.logger).Info(LogEventStorageSnapshotOpReplayStartingInfo).
		Int("count", len(operations)).
		Log()

	// Replay operations
	replayed := 0
	failed := 0
	for _, op := range operations {
		var err error
		switch op.Type {
		case SnapshotOpCreate:
			err = storage.Create(ctx, op.SecCtx, op.Data)
		case SnapshotOpUpdate:
			err = storage.Update(ctx, op.SecCtx, op.ObjectID, op.Data)
		case SnapshotOpDelete:
			err = storage.Delete(ctx, op.SecCtx, op.ObjectID, op.Cascade)
		case SnapshotOpRead:
			// Reads are replayed but result is discarded (already returned to caller)
			_, err = storage.Read(ctx, op.SecCtx, op.ObjectID)
		}

		if err != nil {
			// Log error but continue replay
			StorageLog(q.logger).Warn(LogEventStorageSnapshotOpReplayFailedWarn).
				String(ConstMiscOperationType, op.Type.String()).
				ObjectID(op.ObjectID).
				String("sequence", fmt.Sprintf("%d", op.Sequence)).
				WithError(err).
				Log()
			failed++
		} else {
			replayed++
		}
	}

	StorageLog(q.logger).Info(LogEventStorageSnapshotOpReplayCompletedInfo).
		Int("replayed", replayed).
		Int("failed", failed).
		Log()

	// Clear queue
	q.clearLocked()

	if failed > 0 {
		return errfmt.Errorf(ConstMiscFailedToReplayDOperations, failed)
	}

	return nil
}

func (q *SnapshotOperationQueue) clearLocked() {
	q.operations = nil
	q.pendingWrites = make(map[string]bool)
	q.sequence = 0
}

// Size returns the current queue size
func (q *SnapshotOperationQueue) Size() int {
	var size int
	_ = concurrency.RunInLockOrLog(
		&q.mu, locknames.LockNameSnapshotQueueSize, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			size = len(q.operations)
			return nil
		},
	)
	return size
}

// Clear clears all queued operations
func (q *SnapshotOperationQueue) Clear() {
	_ = concurrency.RunInLockOrLog(
		&q.mu, locknames.LockNameSnapshotQueueClear, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			q.clearLocked()
			return nil
		},
	)
}
