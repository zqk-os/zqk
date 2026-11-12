package community

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

var (
	// ErrBlankWorkerIdentifier is returned when a worker ID is empty or whitespace.
	ErrBlankWorkerIdentifier = errors.New("work_stealing: worker identifier cannot be blank")
	// ErrWorkerNotRegistered is returned when querying an unregistered worker deque.
	ErrWorkerNotRegistered = errors.New("work_stealing: worker not registered in balancer")
	// ErrWorkerAlreadyRegistered is returned when registering a worker twice.
	ErrWorkerAlreadyRegistered = errors.New("work_stealing: worker already registered")
	// ErrInvalidTask is returned when a task fails validation invariants.
	ErrInvalidTask = errors.New("work_stealing: invalid task item")
	// ErrInvalidBatchSize is returned when steal batch size is non-positive.
	ErrInvalidBatchSize = errors.New("work_stealing: batch size must be greater than zero")
)

// TaskItem represents an executable unit of work.
type TaskItem struct {
	ID        string    `json:"id"`
	Payload   string    `json:"payload"`
	Priority  int       `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
}

// Validate checks task invariant boundaries.
func (t *TaskItem) Validate() error {
	if t == nil {
		return fmt.Errorf("%w: task cannot be nil", ErrInvalidTask)
	}
	if err := strutil.ErrorOnBlank(t.ID, "%w: task id is required", ErrInvalidTask); err != nil {
		return err
	}
	return nil
}

// TaskDeque is a thread-safe double-ended queue implementing work-stealing semantics:
// The owner pushes and pops from the bottom (LIFO).
// Remote thieves steal from the top (FIFO).
type TaskDeque struct {
	mu    sync.Mutex
	items []TaskItem
}

// NewTaskDeque creates an empty TaskDeque.
func NewTaskDeque() *TaskDeque {
	return &TaskDeque{
		items: make([]TaskItem, 0),
	}
}

// PushBottom appends a task to the bottom of the deque (local worker push).
func (q *TaskDeque) PushBottom(task TaskItem) error {
	if err := task.Validate(); err != nil {
		return err
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	q.items = append(q.items, task)
	return nil
}

// PopBottom removes and returns the task at the bottom (local worker pop, LIFO).
func (q *TaskDeque) PopBottom() (TaskItem, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	n := len(q.items)
	if n == 0 {
		return TaskItem{}, false
	}

	item := q.items[n-1]
	q.items = q.items[:n-1]
	return item, true
}

// StealTop steals up to maxBatch tasks from the top of the deque (remote thief, FIFO).
func (q *TaskDeque) StealTop(maxBatch int) []TaskItem {
	if maxBatch <= 0 {
		return nil
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	n := len(q.items)
	if n == 0 {
		return nil
	}

	batchSize := maxBatch
	if batchSize > n {
		batchSize = n
	}

	stolen := make([]TaskItem, batchSize)
	copy(stolen, q.items[:batchSize])
	q.items = q.items[batchSize:]
	return stolen
}

// Len returns the current count of items in the deque.
func (q *TaskDeque) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// WorkStealingBalancer manages multi-worker task deques and coordinates steal operations.
type WorkStealingBalancer struct {
	mu            sync.RWMutex
	deques        map[string]*TaskDeque
	maxStealLimit int
}

// NewWorkStealingBalancer initializes a new balancer with a maximum steal limit per batch.
func NewWorkStealingBalancer(maxStealLimit int) *WorkStealingBalancer {
	if maxStealLimit <= 0 {
		maxStealLimit = 10
	}
	return &WorkStealingBalancer{
		deques:        make(map[string]*TaskDeque),
		maxStealLimit: maxStealLimit,
	}
}

// RegisterWorker registers a worker and instantiates its local deque.
func (b *WorkStealingBalancer) RegisterWorker(workerID string) error {
	if err := strutil.ErrorOnBlank(workerID, "%w: worker_id is required", ErrBlankWorkerIdentifier); err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.deques[workerID]; exists {
		return ErrWorkerAlreadyRegistered
	}
	b.deques[workerID] = NewTaskDeque()
	return nil
}

// SubmitTask assigns a task to a target worker's local deque.
func (b *WorkStealingBalancer) SubmitTask(workerID string, task TaskItem) error {
	if err := strutil.ErrorOnBlank(workerID, "%w: worker_id is required", ErrBlankWorkerIdentifier); err != nil {
		return err
	}

	b.mu.RLock()
	deque, exists := b.deques[workerID]
	b.mu.RUnlock()

	if !exists {
		return ErrWorkerNotRegistered
	}
	return deque.PushBottom(task)
}

// PopLocal retrieves a task from the bottom of the worker's own deque.
func (b *WorkStealingBalancer) PopLocal(workerID string) (TaskItem, bool, error) {
	if err := strutil.ErrorOnBlank(workerID, "%w: worker_id is required", ErrBlankWorkerIdentifier); err != nil {
		return TaskItem{}, false, err
	}

	b.mu.RLock()
	deque, exists := b.deques[workerID]
	b.mu.RUnlock()

	if !exists {
		return TaskItem{}, false, ErrWorkerNotRegistered
	}
	item, ok := deque.PopBottom()
	return item, ok, nil
}

// StealWork allows thiefID to steal tasks from victimID's deque up to maxBatch.
func (b *WorkStealingBalancer) StealWork(thiefID, victimID string, maxBatch int) ([]TaskItem, error) {
	if err := strutil.ErrorOnBlank(thiefID, "%w: thief_id is required", ErrBlankWorkerIdentifier); err != nil {
		return nil, err
	}
	if err := strutil.ErrorOnBlank(victimID, "%w: victim_id is required", ErrBlankWorkerIdentifier); err != nil {
		return nil, err
	}
	if maxBatch <= 0 {
		return nil, ErrInvalidBatchSize
	}

	b.mu.RLock()
	_, thiefExists := b.deques[thiefID]
	victimDeque, victimExists := b.deques[victimID]
	b.mu.RUnlock()

	if !thiefExists || !victimExists {
		return nil, ErrWorkerNotRegistered
	}

	limit := maxBatch
	if limit > b.maxStealLimit {
		limit = b.maxStealLimit
	}

	stolen := victimDeque.StealTop(limit)
	return stolen, nil
}

// GetQueueLength returns the count of pending tasks in a worker's deque.
func (b *WorkStealingBalancer) GetQueueLength(workerID string) (int, error) {
	if err := strutil.ErrorOnBlank(workerID, "%w: worker_id is required", ErrBlankWorkerIdentifier); err != nil {
		return 0, err
	}

	b.mu.RLock()
	deque, exists := b.deques[workerID]
	b.mu.RUnlock()

	if !exists {
		return 0, ErrWorkerNotRegistered
	}
	return deque.Len(), nil
}
