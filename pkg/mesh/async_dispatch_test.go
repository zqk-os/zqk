package mesh

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestAsyncDispatchPriorityOrder(t *testing.T) {
	q := NewAsyncDispatchQueue(100)
	defer q.Close()

	ctx := context.Background()

	// Enqueue Low, Normal, High
	err := q.Enqueue(ctx, AsyncMessage{ID: "low-1", Priority: PriorityLow, Recipient: "agent-1"})
	if err != nil {
		t.Fatalf("failed to enqueue low: %v", err)
	}
	err = q.Enqueue(ctx, AsyncMessage{ID: "high-1", Priority: PriorityHigh, Recipient: "agent-1"})
	if err != nil {
		t.Fatalf("failed to enqueue high: %v", err)
	}
	err = q.Enqueue(ctx, AsyncMessage{ID: "norm-1", Priority: PriorityNormal, Recipient: "agent-1"})
	if err != nil {
		t.Fatalf("failed to enqueue norm: %v", err)
	}

	batch, err := q.DequeueBatch(ctx, 3)
	if err != nil {
		t.Fatalf("failed to dequeue batch: %v", err)
	}
	if len(batch) != 3 {
		t.Fatalf("expected 3 items, got %d", len(batch))
	}

	if batch[0].ID != "high-1" {
		t.Errorf("expected high-1 first, got %s", batch[0].ID)
	}
	if batch[1].ID != "norm-1" {
		t.Errorf("expected norm-1 second, got %s", batch[1].ID)
	}
	if batch[2].ID != "low-1" {
		t.Errorf("expected low-1 third, got %s", batch[2].ID)
	}
}

func TestAsyncDispatchConcurrentPriority(t *testing.T) {
	q := NewAsyncDispatchQueue(500)
	defer q.Close()

	var wg sync.WaitGroup
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		wg.Add(1)
		idx := i
		goroutinelabels.NewGoroutine("test", "concurrent-producer").StartSimple(func() {
			defer wg.Done()
			p := PriorityNormal
			if idx%5 == 0 {
				p = PriorityHigh
			} else if idx%3 == 0 {
				p = PriorityLow
			}
			_ = q.Enqueue(ctx, AsyncMessage{
				ID:        fmt.Sprintf("msg-%d", idx),
				Priority:  p,
				Recipient: "agent-pool",
			})
		})
	}
	wg.Wait()

	batch, err := q.DequeueBatch(ctx, 20)
	if err != nil {
		t.Fatalf("batch dequeue error: %v", err)
	}
	if len(batch) != 20 {
		t.Fatalf("expected 20 items, got %d", len(batch))
	}

	// Verify all High items precede Normal and Low items
	sawNormalOrLow := false
	for _, m := range batch {
		if m.Priority < PriorityHigh {
			sawNormalOrLow = true
		}
		if sawNormalOrLow && m.Priority == PriorityHigh {
			t.Errorf("high priority message encountered after lower priority")
		}
	}
}

func TestAsyncDispatchBatchDrain(t *testing.T) {
	q := NewAsyncDispatchQueue(50)
	defer q.Close()
	ctx := context.Background()

	for i := 0; i < 15; i++ {
		_ = q.Enqueue(ctx, AsyncMessage{
			ID:        string(rune('0' + i)),
			Priority:  PriorityNormal,
			Recipient: "agent-x",
		})
	}

	batch1, err := q.DequeueBatch(ctx, 10)
	if err != nil || len(batch1) != 10 {
		t.Fatalf("expected 10 items in batch 1, got %d (err: %v)", len(batch1), err)
	}

	remaining := q.Drain()
	if len(remaining) != 5 {
		t.Fatalf("expected 5 remaining items, got %d", len(remaining))
	}

	if q.Len() != 0 {
		t.Fatalf("expected queue length 0, got %d", q.Len())
	}
}
