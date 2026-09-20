package mesh

import (
	"context"
	"errors"
	"sync"
	"time"
)

// MessagePriority defines the scheduling priority of an async message.
type MessagePriority int

const (
	PriorityLow MessagePriority = iota
	PriorityNormal
	PriorityHigh
)

// AsyncMessage represents an asynchronous payload dispatched across the agent mesh.
type AsyncMessage struct {
	ID        string          `json:"id"`
	Priority  MessagePriority `json:"priority"`
	Payload   []byte          `json:"payload"`
	Timestamp time.Time       `json:"timestamp"`
	Recipient string          `json:"recipient"`
}

// AsyncDispatchQueue is a thread-safe multi-priority queue with batch draining.
type AsyncDispatchQueue struct {
	mu       sync.Mutex
	notEmpty *sync.Cond
	capacity int
	closed   bool

	highQueue   []AsyncMessage
	normalQueue []AsyncMessage
	lowQueue    []AsyncMessage
}

// NewAsyncDispatchQueue creates an AsyncDispatchQueue with bounded capacity per priority tier.
func NewAsyncDispatchQueue(capacity int) *AsyncDispatchQueue {
	q := &AsyncDispatchQueue{
		capacity:    capacity,
		highQueue:   make([]AsyncMessage, 0, capacity/3+1),
		normalQueue: make([]AsyncMessage, 0, capacity/3+1),
		lowQueue:    make([]AsyncMessage, 0, capacity/3+1),
	}
	q.notEmpty = sync.NewCond(&q.mu)
	return q
}

// Enqueue inserts a message into the appropriate priority sub-queue.
func (q *AsyncDispatchQueue) Enqueue(ctx context.Context, msg AsyncMessage) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return errors.New("dispatch queue closed")
	}

	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	switch msg.Priority {
	case PriorityHigh:
		q.highQueue = append(q.highQueue, msg)
	case PriorityLow:
		q.lowQueue = append(q.lowQueue, msg)
	default:
		q.normalQueue = append(q.normalQueue, msg)
	}

	q.notEmpty.Signal()
	return nil
}

// DequeueBatch drains up to maxBatch messages in strict priority order (High -> Normal -> Low).
func (q *AsyncDispatchQueue) DequeueBatch(ctx context.Context, maxBatch int) ([]AsyncMessage, error) {
	if maxBatch <= 0 {
		return nil, nil
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	for q.totalLen() == 0 && !q.closed {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		q.notEmpty.Wait()
	}

	if q.closed && q.totalLen() == 0 {
		return nil, errors.New("dispatch queue closed")
	}

	batch := make([]AsyncMessage, 0, maxBatch)

	// 1. Drain high priority
	for len(q.highQueue) > 0 && len(batch) < maxBatch {
		batch = append(batch, q.highQueue[0])
		q.highQueue = q.highQueue[1:]
	}

	// 2. Drain normal priority
	for len(q.normalQueue) > 0 && len(batch) < maxBatch {
		batch = append(batch, q.normalQueue[0])
		q.normalQueue = q.normalQueue[1:]
	}

	// 3. Drain low priority
	for len(q.lowQueue) > 0 && len(batch) < maxBatch {
		batch = append(batch, q.lowQueue[0])
		q.lowQueue = q.lowQueue[1:]
	}

	return batch, nil
}

// Drain immediately extracts all remaining messages across all priority bands without blocking.
func (q *AsyncDispatchQueue) Drain() []AsyncMessage {
	q.mu.Lock()
	defer q.mu.Unlock()

	total := q.totalLen()
	res := make([]AsyncMessage, 0, total)

	res = append(res, q.highQueue...)
	res = append(res, q.normalQueue...)
	res = append(res, q.lowQueue...)

	q.highQueue = q.highQueue[:0]
	q.normalQueue = q.normalQueue[:0]
	q.lowQueue = q.lowQueue[:0]

	return res
}

// Len returns the current count of queued messages.
func (q *AsyncDispatchQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.totalLen()
}

func (q *AsyncDispatchQueue) totalLen() int {
	return len(q.highQueue) + len(q.normalQueue) + len(q.lowQueue)
}

// Close marks the queue as closed and wakes up any waiting consumers.
func (q *AsyncDispatchQueue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.closed = true
	q.notEmpty.Broadcast()
	return nil
}
