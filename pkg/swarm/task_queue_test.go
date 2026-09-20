package swarm_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/swarm"
)

func TestInMemoryTaskQueue(t *testing.T) {
	q := swarm.NewInMemoryTaskQueue()
	ctx := context.Background()

	task1 := swarm.Task{ID: "t1", SystemPrompt: "sys", UserPrompt: "user"}
	task2 := swarm.Task{ID: "t2", SystemPrompt: "sys2", UserPrompt: "user2"}

	_ = q.Enqueue(ctx, task1)
	_ = q.Enqueue(ctx, task2)

	if q.Size() != 2 {
		t.Errorf("expected size 2, got %d", q.Size())
	}

	dequeued, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dequeued.ID != "t1" {
		t.Errorf("expected t1, got %s", dequeued.ID)
	}

	if q.Size() != 1 {
		t.Errorf("expected size 1, got %d", q.Size())
	}
}

func TestInMemoryTaskQueue_DequeueEmpty(t *testing.T) {
	q := swarm.NewInMemoryTaskQueue()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := q.Dequeue(ctx)
	if err == nil {
		t.Errorf("expected error on empty queue with timeout")
	}
}

func TestInMemoryTaskQueue_Close(t *testing.T) {
	q := swarm.NewInMemoryTaskQueue()
	q.Close()

	ctx := context.Background()
	_, err := q.Dequeue(ctx)
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected io.EOF on dequeuing closed queue, got %v", err)
	}
}

func TestInMemoryTaskQueue_EnqueueClosed(t *testing.T) {
	q := swarm.NewInMemoryTaskQueue()
	q.Close()

	ctx := context.Background()
	err := q.Enqueue(ctx, swarm.Task{ID: "t"})
	if err == nil {
		t.Errorf("expected error when enqueuing to closed queue")
	}
}

func TestInMemoryTaskQueue_DoubleClose(t *testing.T) {
	q := swarm.NewInMemoryTaskQueue()
	// Close the queue the first time.
	q.Close()

	// Close the queue a second time. This should not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("unexpected panic on double close: %v", r)
		}
	}()
	q.Close()
}

func TestInMemoryTaskQueue_ConcurrencyStress(t *testing.T) {
	for iter := 0; iter < 50; iter++ {
		q := swarm.NewInMemoryTaskQueue()
		ctx := context.Background()

		var wg sync.WaitGroup
		numPublishers := 10
		numConsumers := 5

		// Start consumers
		for i := 0; i < numConsumers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					_, err := q.Dequeue(ctx)
					if err != nil {
						// Expected to get an error (like io.EOF) when the queue is closed
						return
					}
				}
			}()
		}

		// Start publishers
		for i := 0; i < numPublishers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				taskID := "task"
				for {
					err := q.Enqueue(ctx, swarm.Task{ID: taskID})
					if err != nil {
						// Expected eventually when queue is closed
						return
					}
					time.Sleep(time.Microsecond)
				}
			}()
		}

		// Wait a tiny bit then close
		time.Sleep(5 * time.Millisecond)

		// Double Close test concurrently
		wg.Add(2)
		go func() {
			defer wg.Done()
			q.Close()
		}()
		go func() {
			defer wg.Done()
			q.Close()
		}()

		wg.Wait()
	}
}
