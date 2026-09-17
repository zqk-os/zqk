package swarm

import (
	"context"
	"errors"
	"io"
	"sync"
)

// Task represents a unit of work for the LLM swarm.
type Task struct {
	ID             string
	SystemPrompt   string
	UserPrompt     string
	CurrentRetries int
	MaxRetries     int
}

// TaskResult represents the outcome of a Task.
type TaskResult struct {
	Task   Task
	Result string
	Error  error
}

// TaskQueue defines an interface for queueing and retrieving tasks.
type TaskQueue interface {
	Enqueue(ctx context.Context, t Task) error
	Dequeue(ctx context.Context) (Task, error)
	Size() int
}

// InMemoryTaskQueue is a basic memory-backed thread-safe task queue using channels.
type InMemoryTaskQueue struct {
	ch     chan Task
	done   chan struct{}
	mu     sync.Mutex
	closed bool
}

// NewInMemoryTaskQueue creates a new InMemoryTaskQueue.
func NewInMemoryTaskQueue() *InMemoryTaskQueue {
	return &InMemoryTaskQueue{
		ch:   make(chan Task, 10000),
		done: make(chan struct{}),
	}
}

// Enqueue adds a task to the queue.
func (q *InMemoryTaskQueue) Enqueue(ctx context.Context, t Task) error {
	select {
	case <-q.done:
		return errors.New("queue closed")
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	select {
	case q.ch <- t:
		return nil
	case <-q.done:
		return errors.New("queue closed")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Dequeue retrieves a task, blocking until one is available or context is done.
func (q *InMemoryTaskQueue) Dequeue(ctx context.Context) (Task, error) {
	// First check if there is anything in the channel without blocking
	select {
	case t := <-q.ch:
		return t, nil
	default:
	}

	select {
	case t := <-q.ch:
		return t, nil
	case <-q.done:
		// Drain remaining tasks in q.ch
		select {
		case t := <-q.ch:
			return t, nil
		default:
			return Task{}, io.EOF
		}
	case <-ctx.Done():
		return Task{}, ctx.Err()
	}
}

// Size returns the current number of items in the queue.
func (q *InMemoryTaskQueue) Size() int {
	return len(q.ch)
}

// Close closes the underlying task channel.
func (q *InMemoryTaskQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.closed = true
	close(q.done)
}
