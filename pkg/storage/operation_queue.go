package storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
)

// OperationType represents the type of operation
// TRACK: BLI-CEF-R16-OPQUEUE-001 / CRIT-CEF-R2-CON-OPQUEUE-A / REQ-CEF-R2-CON-OPQUEUE
type OperationType string

const (
	OperationCreate         OperationType = OperationType(OpCreate)
	OperationUpdate         OperationType = OperationType(OpUpdate)
	OperationDelete         OperationType = OperationType(OpDelete)
	OperationCascade        OperationType = "cascade"
	OperationCascadeNullify OperationType = "cascade_nullify"
	OperationCascadeSetNull OperationType = "cascade_set_null"
)

// OperationStatus represents the status of an operation
type OperationStatus string

const (
	StatusPending   OperationStatus = "pending"
	StatusRunning   OperationStatus = "running"
	StatusCompleted OperationStatus = "completed"
	StatusFailed    OperationStatus = "failed"
	StatusRetrying  OperationStatus = "retrying"
	StatusCancelled OperationStatus = "cancelled"
)

// OperationPriority represents operation priority
type OperationPriority int

const (
	PriorityCritical OperationPriority = 1 // Cascade updates, required fixes
	PriorityHigh     OperationPriority = 2 // User-initiated operations
	PriorityNormal   OperationPriority = 3 // Background operations
	PriorityLow      OperationPriority = 4 // Cleanup, optimization
)

// Operation represents a single operation in the queue
type Operation struct {
	ID          string
	Type        OperationType
	ObjectID    string
	ObjectKind  string
	Priority    OperationPriority
	Status      OperationStatus
	RetryCount  int
	MaxRetries  int
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	Data        map[string]any // For create operations
	Updates     map[string]any // For update operations
	Error       error
	Context     context.Context
	SecCtx      *pkgctx.SecurityContext
	Metadata    map[string]string // Additional metadata
	mu          sync.RWMutex
}

// OperationQueue manages a queue of operations with priority
type OperationQueue struct {
	operations []*Operation
	mu         sync.RWMutex
	notifier   OperationNotifier
	logger     *logging.EventLogger
}

// OperationNotifier interface for sending notifications
type OperationNotifier interface {
	NotifyProgress(operation *Operation, progress int, message string) error
	NotifyStatus(operation *Operation, oldStatus, newStatus OperationStatus) error
	NotifyError(operation *Operation, err error) error
	NotifyCompletion(operation *Operation) error
}

// NewOperationQueue creates a new operation queue
func NewOperationQueue(notifier OperationNotifier) *OperationQueue {
	return &OperationQueue{
		operations: make([]*Operation, 0),
		notifier:   notifier,
		logger:     logging.NewEventLogger(pkgctx.NewSystemContext()),
	}
}

// executorWakeCallback is called when operations are enqueued to wake workers
// executorWakeCallback is called when operations are enqueued to wake workers
// This is set by OperationExecutor to enable on-demand worker pattern
var (
	executorWakeCallback func()
	executorWakeMu       sync.RWMutex
	wakeSignalCh         chan context.Context
	wakeInitOnce         sync.Once
)

func initWakeDispatcher() {
	wakeSignalCh = make(chan context.Context, 1)
	goroutinelabels.NewGoroutine(ConstMiscOperationQueueCallback, ConstMiscTriggeringOperationQueueCallback).
		StartWithContext(context.Background(), func(ctx context.Context) error { // Background: request-or-shutdown derived
			for {
				select {
				case <-ctx.Done():
					return nil
				case opCtx, ok := <-wakeSignalCh:
					if !ok {
						return nil
					}
					if opCtx != nil && opCtx.Err() != nil {
						continue
					}
					var cb func()
					_ = concurrency.RunInRLockOrLog(
						&executorWakeMu, locknames.LockNameOperationQueueGetWakeCallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
						func() error {
							cb = executorWakeCallback
							return nil
						},
					)
					if cb != nil {
						cb()
					}
				}
			}
		})
}

// SetExecutorWakeCallback sets the callback to wake executor workers
func SetExecutorWakeCallback(callback func()) {
	_ = concurrency.RunInLockOrLog(
		&executorWakeMu, locknames.LockNameOperationQueueSetWakeCallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			executorWakeCallback = callback
			return nil
		},
	)
}

// Enqueue adds an operation to the queue
func (q *OperationQueue) Enqueue(op *Operation) error {
	ctx := op.Context
	if ctx == nil {
		ctx = context.Background() // Background: request-or-shutdown derived
	}
	return q.EnqueueWithContext(ctx, op)
}

// EnqueueWithContext adds an operation to the queue with explicit context
func (q *OperationQueue) EnqueueWithContext(ctx context.Context, op *Operation) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if op.ID == emptyValue {
		op.ID = generateOperationID()
	}
	if op.CreatedAt.IsZero() {
		op.CreatedAt = time.Now()
	}
	if op.MaxRetries == 0 {
		op.MaxRetries = 3 // Default max retries
	}
	if op.Context == nil && ctx != nil {
		op.Context = ctx
	}

	err := concurrency.RunInLockWithLogger(
		&q.mu, locknames.LockNameOperationQueueEnqueue, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Insert operation in priority order (lower number = higher priority)
			inserted := false
			for i, existing := range q.operations {
				if op.Priority < existing.Priority {
					// Insert before this operation
					q.operations = append(q.operations[:i], append([]*Operation{op}, q.operations[i:]...)...)
					inserted = true
					break
				}
			}
			if !inserted {
				q.operations = append(q.operations, op)
			}

			// Notify that operation was enqueued
			if q.notifier != nil {
				var _err_82889821 = q.notifier.NotifyStatus(op, "", StatusPending)
				if //nolint:errcheck // Notification errors are non-critical
				_err_82889821 != nil {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_82889821).Log()
				}
			}
			return nil
		},
	)
	if err != nil {
		return errfmt.Newf(ConstMiscTimeoutEnqueuingOperation).Wrap(err)
	}

	wakeInitOnce.Do(initWakeDispatcher)

	// Trigger worker wake via bounded non-blocking signal (coalesced)
	if wakeSignalCh != nil {
		select {
		case wakeSignalCh <- ctx:
		default:
			// Signal already pending in dispatcher; coalesced
		}
	}

	return nil
}

// Dequeue removes and returns the highest priority operation
func (q *OperationQueue) Dequeue() *Operation {
	var result *Operation
	_ = concurrency.RunInLockOrLog(
		&q.mu, locknames.LockNameOperationQueueDequeue, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if len(q.operations) == 0 {
				return nil
			}

			// Find highest priority operation that's not running
			for i, op := range q.operations {
				if op.Status == StatusPending || op.Status == StatusRetrying {
					// Remove from queue
					q.operations = append(q.operations[:i], q.operations[i+1:]...)
					result = op
					return nil
				}
			}
			return nil
		},
	)
	return result
}

// Peek returns the next operation without removing it
func (q *OperationQueue) Peek() *Operation {
	var result *Operation
	_ = concurrency.RunInRLockOrLog(
		&q.mu, locknames.LockNameOperationQueuePeek, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			for _, op := range q.operations {
				if op.Status == StatusPending || op.Status == StatusRetrying {
					result = op
					return nil
				}
			}
			return nil
		},
	)
	return result
}

// UpdateStatus updates the status of an operation
func (q *OperationQueue) UpdateStatus(op *Operation, newStatus OperationStatus) error {
	var oldStatus OperationStatus
	err := concurrency.RunInLockWithLogger(
		&op.mu, locknames.LockNameOperationUpdateStatus, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			oldStatus = op.Status
			op.Status = newStatus
			now := time.Now()

			switch newStatus {
			case StatusRunning:
				if op.StartedAt == nil {
					op.StartedAt = &now
				}
			case StatusCompleted, StatusFailed, StatusCancelled:
				if op.CompletedAt == nil {
					op.CompletedAt = &now
				}
			}
			return nil
		},
	)
	if err != nil {
		return errfmt.Newf(ConstMiscTimeoutUpdatingOperationStatus).Wrap(err)
	}

	// Notify status change
	if q.notifier != nil {
		var _err_82892644 = q.notifier.NotifyStatus(op, oldStatus, newStatus)
		if //nolint:errcheck // Notification errors are non-critical
		_err_82892644 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_82892644).Log()
		}
	}

	return nil
}

// GetStatus returns the current status of an operation
func (q *OperationQueue) GetStatus(operationID string) (*Operation, error) {
	var result *Operation
	var found bool
	err := concurrency.RunInRLockWithLogger(
		&q.mu, locknames.LockNameOperationQueueGetStatus, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			for _, op := range q.operations {
				if op.ID == operationID {
					// Acquire operation lock to read status
					_ = concurrency.RunInRLockOrLog(
						&op.mu, locknames.LockNameOperationGetStatus, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
						func() error {
							result = op
							found = true
							return nil
						},
					)
					return nil
				}
			}
			return nil
		},
	)
	if err != nil {
		return nil, errfmt.Newf(ConstMiscTimeoutGettingOperationStatus).Wrap(err)
	}
	if !found {
		return nil, errfmt.Errorf(ConstMiscOperationNotFoundS, operationID)
	}
	return result, nil
}

// ListOperations returns all operations matching the filter
func (q *OperationQueue) ListOperations(filter func(*Operation) bool) []*Operation {
	var result []*Operation
	_ = concurrency.RunInRLockOrLog(
		&q.mu, locknames.LockNameOperationQueueList, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			for _, op := range q.operations {
				if filter == nil || filter(op) {
					_ = concurrency.RunInRLockOrLog(
						&op.mu, locknames.LockNameOperationListRead, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
						func() error {
							result = append(result, op)
							return nil
						},
					)
				}
			}
			return nil
		},
	)
	return result
}

// Len returns the number of operations in the queue
func (q *OperationQueue) Len() int {
	var length int
	_ = concurrency.RunInRLockOrLog(
		&q.mu, locknames.LockNameOperationQueueLen, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			length = len(q.operations)
			return nil
		},
	)
	return length
}

// generateOperationID generates a unique operation ID
func generateOperationID() string {
	return fmt.Sprintf("op-%d", time.Now().UnixNano())
}

// SetError sets the error for an operation
func (op *Operation) SetError(err error) {
	_ = concurrency.RunInLockOrLog(
		&op.mu, locknames.LockNameOperationSetError, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			op.Error = err
			return nil
		},
	)
}

// GetError returns the error for an operation
func (op *Operation) GetError() error {
	var result error
	_ = concurrency.RunInRLockOrLog(
		&op.mu, locknames.LockNameOperationGetError, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			result = op.Error
			return nil
		},
	)
	return result
}

// CanRetry returns true if the operation can be retried
func (op *Operation) CanRetry() bool {
	var canRetry bool
	_ = concurrency.RunInRLockOrLog(
		&op.mu, locknames.LockNameOperationCanRetry, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			canRetry = op.RetryCount < op.MaxRetries && op.Status != StatusCancelled
			return nil
		},
	)
	return canRetry
}

// IncrementRetry increments the retry count
func (op *Operation) IncrementRetry() {
	_ = concurrency.RunInLockOrLog(
		&op.mu, locknames.LockNameOperationIncrementRetry, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			op.RetryCount++
			return nil
		},
	)
}
