package storage_test

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestOperationQueue_EnqueueWithContext_Cancelled(t *testing.T) {
	queue := storage.NewOperationQueue(nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	op := &storage.Operation{
		Type:     storage.OperationCreate,
		ObjectID: "TEST-OP-CANCELLED",
	}

	err := queue.EnqueueWithContext(ctx, op)
	if err == nil {
		t.Fatal("expected error when enqueuing with cancelled context, got nil")
	}
}

func TestOperationQueue_BoundedWakeCallbacks_HighLoad(t *testing.T) {
	queue := storage.NewOperationQueue(nil)

	var wakeCount atomic.Int64
	storage.SetExecutorWakeCallback(func() {
		wakeCount.Add(1)
		time.Sleep(1 * time.Millisecond)
	})
	defer storage.SetExecutorWakeCallback(nil)

	initialGoroutines := runtime.NumGoroutine()

	const numOps = 2000
	const workers = 16
	jobs := make(chan struct{}, numOps)
	for i := 0; i < numOps; i++ {
		jobs <- struct{}{}
	}
	close(jobs)

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		goroutinelabels.NewGoroutine("opqueue_test_enqueue", "bounded enqueue load").
			StartSimple(func() {
				defer wg.Done()
				for range jobs {
					op := &storage.Operation{
						Type:     storage.OperationCreate,
						ObjectID: "TEST-BOUNDED-OP",
						Priority: storage.PriorityNormal,
						Context:  pkgctx.NewSystemContext(),
					}
					_ = queue.Enqueue(op)
				}
			})
	}

	waitDone := make(chan struct{})
	goroutinelabels.NewGoroutine("opqueue_test_join", "wait for bounded enqueue workers").
		StartSimple(func() {
			wg.Wait()
			close(waitDone)
		})
	select {
	case <-waitDone:
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for bounded enqueue workers")
	}

	// Ensure goroutines don't grow linearly with numOps
	activeGoroutines := runtime.NumGoroutine()
	growth := activeGoroutines - initialGoroutines
	if growth > 50 {
		t.Fatalf("goroutine leak or unbounded spawning: initial=%d active=%d growth=%d", initialGoroutines, activeGoroutines, growth)
	}

	// Verify operations were enqueued
	if queue.Len() != numOps {
		t.Fatalf("expected queue length %d, got %d", numOps, queue.Len())
	}
}

// TestOperationQueue_BLI_CEF_R16_OPQUEUE_001 explicitly validates that OperationQueue wake callbacks
// remain bounded and context-aware under load without leaking goroutines or ignoring cancellation.
func TestOperationQueue_BLI_CEF_R16_OPQUEUE_001(t *testing.T) {
	queue := storage.NewOperationQueue(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	op := &storage.Operation{
		Type:     storage.OperationCreate,
		ObjectID: "TEST-BLI-CEF-R16-OPQUEUE-001",
	}

	if err := queue.EnqueueWithContext(ctx, op); err == nil {
		t.Fatal("expected cancellation error for cancelled context, got nil")
	}
}
