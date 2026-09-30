package validation

import (
	"testing"
	"time"
)

func TestPriorityQueue_BasicOperations(t *testing.T) {
	t.Parallel()
	pq := NewPriorityQueue()

	// Enqueue tasks with different priorities
	pq.Enqueue(&ValidationTask{
		ObjectID:   "TEST-003",
		Priority:   3,
		EnqueuedAt: time.Now(),
	})
	pq.Enqueue(&ValidationTask{
		ObjectID:   "TEST-001",
		Priority:   1,
		EnqueuedAt: time.Now(),
	})
	pq.Enqueue(&ValidationTask{
		ObjectID:   "TEST-002",
		Priority:   2,
		EnqueuedAt: time.Now(),
	})

	// Dequeue should return highest priority first
	task1 := pq.Dequeue()
	if task1 == nil || task1.Priority != 1 {
		t.Errorf("expected priority 1, got %v", task1)
	}

	task2 := pq.Dequeue()
	if task2 == nil || task2.Priority != 2 {
		t.Errorf("expected priority 2, got %v", task2)
	}

	task3 := pq.Dequeue()
	if task3 == nil || task3.Priority != 3 {
		t.Errorf("expected priority 3, got %v", task3)
	}

	// Queue should be empty
	if pq.Size() != 0 {
		t.Errorf("expected queue size 0, got %d", pq.Size())
	}
}

func TestPriorityQueue_SamePriorityOrdering(t *testing.T) {
	t.Parallel()
	pq := NewPriorityQueue()

	now := time.Now()
	// Enqueue tasks with same priority but different times
	pq.Enqueue(&ValidationTask{
		ObjectID:   "TEST-002",
		Priority:   1,
		EnqueuedAt: now.Add(100 * time.Millisecond),
	})
	pq.Enqueue(&ValidationTask{
		ObjectID:   "TEST-001",
		Priority:   1,
		EnqueuedAt: now,
	})
	pq.Enqueue(&ValidationTask{
		ObjectID:   "TEST-003",
		Priority:   1,
		EnqueuedAt: now.Add(200 * time.Millisecond),
	})

	// Should dequeue in order of enqueue time (earliest first)
	task1 := pq.Dequeue()
	if task1 == nil || task1.ObjectID != "TEST-001" {
		t.Errorf("expected TEST-001 first, got %v", task1)
	}

	task2 := pq.Dequeue()
	if task2 == nil || task2.ObjectID != "TEST-002" {
		t.Errorf("expected TEST-002 second, got %v", task2)
	}

	task3 := pq.Dequeue()
	if task3 == nil || task3.ObjectID != "TEST-003" {
		t.Errorf("expected TEST-003 third, got %v", task3)
	}
}

func TestPriorityQueue_Peek(t *testing.T) {
	t.Parallel()
	pq := NewPriorityQueue()

	pq.Enqueue(&ValidationTask{
		ObjectID:   "TEST-001",
		Priority:   1,
		EnqueuedAt: time.Now(),
	})
	pq.Enqueue(&ValidationTask{
		ObjectID:   "TEST-002",
		Priority:   2,
		EnqueuedAt: time.Now(),
	})

	// Peek should return highest priority without removing
	peeked := pq.Peek()
	if peeked == nil || peeked.Priority != 1 {
		t.Errorf("expected priority 1, got %v", peeked)
	}

	// Size should still be 2
	if pq.Size() != 2 {
		t.Errorf("expected size 2, got %d", pq.Size())
	}

	// Dequeue should return the same task
	task := pq.Dequeue()
	if task == nil || task.ObjectID != "TEST-001" {
		t.Errorf("expected TEST-001, got %v", task)
	}
}

func TestPriorityQueue_GetByPriority(t *testing.T) {
	t.Parallel()
	pq := NewPriorityQueue()

	pq.Enqueue(&ValidationTask{
		ObjectID:   "TEST-001",
		Priority:   1,
		EnqueuedAt: time.Now(),
	})
	pq.Enqueue(&ValidationTask{
		ObjectID:   "TEST-002",
		Priority:   2,
		EnqueuedAt: time.Now(),
	})
	pq.Enqueue(&ValidationTask{
		ObjectID:   "TEST-003",
		Priority:   1,
		EnqueuedAt: time.Now(),
	})
	pq.Enqueue(&ValidationTask{
		ObjectID:   "TEST-004",
		Priority:   2,
		EnqueuedAt: time.Now(),
	})

	priority1Tasks := pq.GetByPriority(1)
	if len(priority1Tasks) != 2 {
		t.Errorf("expected 2 priority 1 tasks, got %d", len(priority1Tasks))
	}

	priority2Tasks := pq.GetByPriority(2)
	if len(priority2Tasks) != 2 {
		t.Errorf("expected 2 priority 2 tasks, got %d", len(priority2Tasks))
	}
}

func TestPriorityQueue_Clear(t *testing.T) {
	t.Parallel()
	pq := NewPriorityQueue()

	pq.Enqueue(&ValidationTask{
		ObjectID:   "TEST-001",
		Priority:   1,
		EnqueuedAt: time.Now(),
	})
	pq.Enqueue(&ValidationTask{
		ObjectID:   "TEST-002",
		Priority:   2,
		EnqueuedAt: time.Now(),
	})

	if pq.Size() != 2 {
		t.Errorf("expected size 2, got %d", pq.Size())
	}

	pq.Clear()

	if pq.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", pq.Size())
	}
}
