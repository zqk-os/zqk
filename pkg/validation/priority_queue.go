package validation

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
)

// ValidationTask represents a validation task with priority
type ValidationTask struct {
	ObjectID   string
	ObjectKind string
	FilePath   string
	Priority   int    // 1 (highest) to 4 (lowest) - matches Tier system
	Checksum   string // File checksum to detect changes
	EnqueuedAt time.Time
	RetryCount int
	MaxRetries int
}

// PriorityQueue implements a striped, tiered-priority queue for validation tasks
type PriorityQueue struct {
	tasks [5][]*ValidationTask // Tiers 1 to 4 are used
	mu    [5]sync.Mutex        // Striped locks per priority tier
	size  int64                // Atomic counter for O(1) lock-free size checks
}

// NewPriorityQueue creates a new priority queue
func NewPriorityQueue() *PriorityQueue {
	pq := &PriorityQueue{}
	for i := 1; i <= 4; i++ {
		pq.tasks[i] = make([]*ValidationTask, 0)
	}
	return pq
}

// Enqueue adds a task to the queue with thread safety
func (pq *PriorityQueue) Enqueue(task *ValidationTask) {
	priority := task.Priority
	if priority < 1 || priority > 4 {
		priority = 4 // Default to lowest priority if invalid
	}

	if err := concurrency.RunInLockWithLogger(
		&pq.mu[priority],
		LockNamePriorityQueueEnqueue,
		lockLoggerSystem(),
		func() error {
			// TRACK: REDACTED — O(1) fast path for chronological enqueues
			n := len(pq.tasks[priority])
			if n == 0 || !task.EnqueuedAt.Before(pq.tasks[priority][n-1].EnqueuedAt) {
				pq.tasks[priority] = append(pq.tasks[priority], task)
			} else {
				// Binary search for insertion point
				idx := sort.Search(n, func(i int) bool {
					return task.EnqueuedAt.Before(pq.tasks[priority][i].EnqueuedAt)
				})
				pq.tasks[priority] = append(pq.tasks[priority], nil)
				copy(pq.tasks[priority][idx+1:], pq.tasks[priority][idx:])
				pq.tasks[priority][idx] = task
			}
			atomic.AddInt64(&pq.size, 1)
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Error(ConstMagicb67edcad, err).Log()
	}
}

// Dequeue removes and returns the highest priority task with thread safety
func (pq *PriorityQueue) Dequeue() *ValidationTask {
	var task *ValidationTask
	// Scan from highest priority (1) to lowest (4)
	for priority := 1; priority <= 4; priority++ {
		var found bool
		if err := concurrency.RunInLockWithLogger(
			&pq.mu[priority],
			LockNamePriorityQueueDequeue,
			lockLoggerSystem(),
			func() error {
				if len(pq.tasks[priority]) > 0 {
					task = pq.tasks[priority][0]
					// Clear slot to allow GC
					pq.tasks[priority][0] = nil
					pq.tasks[priority] = pq.tasks[priority][1:]
					atomic.AddInt64(&pq.size, -1)
					found = true
				}
				return nil
			},
		); err == nil && found {
			return task
		}
	}
	return nil
}

// Peek returns the highest priority task without removing it
func (pq *PriorityQueue) Peek() *ValidationTask {
	var task *ValidationTask
	for priority := 1; priority <= 4; priority++ {
		var found bool
		if err := concurrency.RunInLockWithLogger(
			&pq.mu[priority],
			LockNamePriorityQueuePeek,
			lockLoggerSystem(),
			func() error {
				if len(pq.tasks[priority]) > 0 {
					task = pq.tasks[priority][0]
					found = true
				}
				return nil
			},
		); err == nil && found {
			return task
		}
	}
	return nil
}

// Size returns the number of tasks in the queue (atomic/lock-free)
func (pq *PriorityQueue) Size() int {
	return int(atomic.LoadInt64(&pq.size))
}

// Clear removes all tasks from the queue
func (pq *PriorityQueue) Clear() {
	for priority := 1; priority <= 4; priority++ {
		_ = concurrency.RunInLockWithLogger(
			&pq.mu[priority],
			LockNamePriorityQueueClear,
			lockLoggerSystem(),
			func() error {
				pq.tasks[priority] = pq.tasks[priority][:0]
				return nil
			},
		)
	}
	atomic.StoreInt64(&pq.size, 0)
}

// GetByPriority returns all tasks of a specific priority
func (pq *PriorityQueue) GetByPriority(priority int) []*ValidationTask {
	if priority < 1 || priority > 4 {
		return nil
	}
	var results []*ValidationTask
	_ = concurrency.RunInLockWithLogger(
		&pq.mu[priority],
		LockNamePriorityQueueGetByPriority,
		lockLoggerSystem(),
		func() error {
			if len(pq.tasks[priority]) > 0 {
				results = make([]*ValidationTask, len(pq.tasks[priority]))
				copy(results, pq.tasks[priority])
			}
			return nil
		},
	)
	return results
}

// SnapshotObjectIDs returns a copy of object IDs and kinds currently in the queue
func (pq *PriorityQueue) SnapshotObjectIDs() []string {
	var ids []string
	for priority := 1; priority <= 4; priority++ {
		_ = concurrency.RunInLockWithLogger(
			&pq.mu[priority],
			LockNamePriorityQueueSnapshotIds,
			lockLoggerSystem(),
			func() error {
				for _, task := range pq.tasks[priority] {
					if task != nil && task.ObjectID != emptyValue {
						ids = append(ids, task.ObjectID+" ("+task.ObjectKind+")")
					}
				}
				return nil
			},
		)
	}
	return ids
}
