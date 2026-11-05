package storage

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingNotifier struct {
	progressCalls   []string
	statusCalls     []string
	errorCalls      []string
	completionCalls []string
}

func (r *recordingNotifier) NotifyProgress(op *Operation, progress int, msg string) error {
	r.progressCalls = append(r.progressCalls, msg)
	return nil
}

func (r *recordingNotifier) NotifyStatus(op *Operation, oldStatus, newStatus OperationStatus) error {
	r.statusCalls = append(r.statusCalls, string(oldStatus)+"->"+string(newStatus))
	return nil
}

func (r *recordingNotifier) NotifyError(op *Operation, err error) error {
	r.errorCalls = append(r.errorCalls, err.Error())
	return nil
}

func (r *recordingNotifier) NotifyCompletion(op *Operation) error {
	r.completionCalls = append(r.completionCalls, op.ID)
	return nil
}

func TestNoopOperationNotifier_Coverage(t *testing.T) {
	t.Parallel()

	notifier := NoopOperationNotifier{}
	op := &Operation{ID: "OP-1"}

	assert.Nil(t, notifier.NotifyProgress(op, 50, "halfway"))
	assert.Nil(t, notifier.NotifyStatus(op, StatusPending, StatusRunning))
	assert.Nil(t, notifier.NotifyError(op, errors.New("test err")))
	assert.Nil(t, notifier.NotifyCompletion(op))
}

func TestOperationQueue_PriorityAndOrdering(t *testing.T) {
	t.Parallel()

	notifier := &recordingNotifier{}
	queue := NewOperationQueue(notifier)

	opLow := &Operation{
		ID:       "OP-LOW",
		Priority: PriorityLow,
		Status:   StatusPending,
	}
	opNormal := &Operation{
		ID:       "OP-NORMAL",
		Priority: PriorityNormal,
		Status:   StatusPending,
	}
	opHigh := &Operation{
		ID:       "OP-HIGH",
		Priority: PriorityHigh,
		Status:   StatusPending,
	}
	opCritical := &Operation{
		ID:       "OP-CRITICAL",
		Priority: PriorityCritical,
		Status:   StatusPending,
	}

	// Enqueue out of order: Low, High, Normal, Critical
	require.NoError(t, queue.Enqueue(opLow))
	require.NoError(t, queue.Enqueue(opHigh))
	require.NoError(t, queue.Enqueue(opNormal))
	require.NoError(t, queue.Enqueue(opCritical))

	assert.Equal(t, 4, queue.Len())

	// Peek should return the highest priority: Critical
	peeked := queue.Peek()
	require.NotNil(t, peeked)
	assert.Equal(t, "OP-CRITICAL", peeked.ID)
	assert.Equal(t, 4, queue.Len()) // Did not remove

	// Dequeue order must strictly follow priority (1, 2, 3, 4)
	dequeued1 := queue.Dequeue()
	require.NotNil(t, dequeued1)
	assert.Equal(t, "OP-CRITICAL", dequeued1.ID)

	dequeued2 := queue.Dequeue()
	require.NotNil(t, dequeued2)
	assert.Equal(t, "OP-HIGH", dequeued2.ID)

	dequeued3 := queue.Dequeue()
	require.NotNil(t, dequeued3)
	assert.Equal(t, "OP-NORMAL", dequeued3.ID)

	dequeued4 := queue.Dequeue()
	require.NotNil(t, dequeued4)
	assert.Equal(t, "OP-LOW", dequeued4.ID)

	// Queue is now empty
	assert.Nil(t, queue.Dequeue())
	assert.Nil(t, queue.Peek())
	assert.Equal(t, 0, queue.Len())
}

func TestOperationQueue_StatusAndLifecycle(t *testing.T) {
	t.Parallel()

	notifier := &recordingNotifier{}
	queue := NewOperationQueue(notifier)

	op := &Operation{
		ID:         "OP-LIFECYCLE-1",
		Priority:   PriorityHigh,
		Status:     StatusPending,
		MaxRetries: 2,
	}

	require.NoError(t, queue.Enqueue(op))

	// GetStatus found
	statusOp, err := queue.GetStatus("OP-LIFECYCLE-1")
	require.NoError(t, err)
	assert.Equal(t, StatusPending, statusOp.Status)

	// GetStatus not found
	_, err = queue.GetStatus("OP-NONEXISTENT")
	assert.Error(t, err)

	// Update to Running: StartedAt should be stamped
	require.NoError(t, queue.UpdateStatus(op, StatusRunning))
	assert.Equal(t, StatusRunning, op.Status)
	assert.NotNil(t, op.StartedAt)

	// ListOperations with filter
	runningOps := queue.ListOperations(func(o *Operation) bool {
		return o.Status == StatusRunning
	})
	assert.Len(t, runningOps, 1)

	pendingOps := queue.ListOperations(func(o *Operation) bool {
		return o.Status == StatusPending
	})
	assert.Empty(t, pendingOps)

	// All ops (nil filter)
	allOps := queue.ListOperations(nil)
	assert.Len(t, allOps, 1)

	// Update to Completed: CompletedAt should be stamped
	require.NoError(t, queue.UpdateStatus(op, StatusCompleted))
	assert.Equal(t, StatusCompleted, op.Status)
	assert.NotNil(t, op.CompletedAt)
}

func TestOperation_RetriesAndErrors(t *testing.T) {
	t.Parallel()

	op := &Operation{
		ID:         "OP-RETRY-1",
		MaxRetries: 3,
		Status:     StatusPending,
	}

	assert.Nil(t, op.GetError())
	testErr := errors.New("temporary disk fault")
	op.SetError(testErr)
	assert.Equal(t, testErr, op.GetError())

	// Retry progression
	assert.True(t, op.CanRetry())
	op.IncrementRetry()
	assert.Equal(t, 1, op.RetryCount)
	assert.True(t, op.CanRetry())

	op.IncrementRetry()
	assert.Equal(t, 2, op.RetryCount)
	assert.True(t, op.CanRetry())

	op.IncrementRetry()
	assert.Equal(t, 3, op.RetryCount)
	assert.False(t, op.CanRetry()) // Reached MaxRetries

	// Cancelled operation can never retry even if retries remain
	op2 := &Operation{
		ID:         "OP-CANCELLED-1",
		MaxRetries: 5,
		RetryCount: 1,
		Status:     StatusCancelled,
	}
	assert.False(t, op2.CanRetry())
}

func TestOperationQueue_DefaultAutoFields(t *testing.T) {
	t.Parallel()

	queue := NewOperationQueue(nil)
	op := &Operation{
		// ID, CreatedAt, MaxRetries unset
		Priority: PriorityNormal,
	}

	require.NoError(t, queue.Enqueue(op))
	assert.NotEmpty(t, op.ID)
	assert.False(t, op.CreatedAt.IsZero())
	assert.Equal(t, 3, op.MaxRetries)
}
