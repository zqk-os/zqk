package ambient

import (
	"context"
	"reflect"
	"sync"
	"testing"
)

// C1: Constructor Power-of-Two Rounding
func TestAutonomyInbox_C1_PowerOfTwoRounding(t *testing.T) {
	inbox := NewAutonomyInbox(100)
	if inbox.Cap() != 128 {
		t.Fatalf("expected cap 128, got %d", inbox.Cap())
	}
}

// C2: Constructor Default Capacity
func TestAutonomyInbox_C2_DefaultCapacity(t *testing.T) {
	inbox := NewAutonomyInbox()
	if inbox.Cap() != 4096 {
		t.Fatalf("expected cap 4096, got %d", inbox.Cap())
	}
}

// C3: Constructor Minimum Capacity Panic
func TestAutonomyInbox_C3_MinCapacityPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic, got none")
		}
	}()
	NewAutonomyInbox(4)
}

// C4: Push Single Event
func TestAutonomyInbox_C4_PushSingleEvent(t *testing.T) {
	inbox := NewAutonomyInbox(16)
	err := inbox.Push(context.Background(), ContextEvent{ID: "evt-1"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if inbox.Len() != 1 {
		t.Fatalf("expected len 1, got %d", inbox.Len())
	}
	if inbox.Pushed() != 1 {
		t.Fatalf("expected pushed 1, got %d", inbox.Pushed())
	}
}

// C5: Pop Returns Pushed Event
func TestAutonomyInbox_C5_PopReturnsPushed(t *testing.T) {
	inbox := NewAutonomyInbox(16)
	inbox.Push(context.Background(), ContextEvent{ID: "evt-1"})
	event, ok := inbox.Pop()
	if !ok || event.ID != "evt-1" {
		t.Fatalf("expected evt-1 and ok=true, got %v, %v", event.ID, ok)
	}
	if inbox.Len() != 0 {
		t.Fatalf("expected len 0, got %d", inbox.Len())
	}
}

// C6: Pop Empty Returns False
func TestAutonomyInbox_C6_PopEmptyReturnsFalse(t *testing.T) {
	inbox := NewAutonomyInbox(16)
	event, ok := inbox.Pop()
	if ok {
		t.Fatal("expected ok=false")
	}
	if event.ID != "" {
		t.Fatal("expected zero-value event")
	}
}

// C7: FIFO Ordering
func TestAutonomyInbox_C7_FIFOOrdering(t *testing.T) {
	inbox := NewAutonomyInbox(16)
	inbox.Push(context.Background(), ContextEvent{ID: "A"})
	inbox.Push(context.Background(), ContextEvent{ID: "B"})
	inbox.Push(context.Background(), ContextEvent{ID: "C"})

	e1, _ := inbox.Pop()
	e2, _ := inbox.Pop()
	e3, _ := inbox.Pop()

	if e1.ID != "A" || e2.ID != "B" || e3.ID != "C" {
		t.Fatalf("expected A, B, C; got %v, %v, %v", e1.ID, e2.ID, e3.ID)
	}
}

// C8: Full Buffer Returns ErrInboxFull
func TestAutonomyInbox_C8_FullBufferReturnsErrInboxFull(t *testing.T) {
	inbox := NewAutonomyInbox(16)
	for i := 0; i < 16; i++ {
		inbox.Push(context.Background(), ContextEvent{ID: "E"})
	}
	err := inbox.Push(context.Background(), ContextEvent{ID: "17"})
	if err != ErrInboxFull {
		t.Fatalf("expected ErrInboxFull, got %v", err)
	}
	if inbox.Dropped() != 1 {
		t.Fatalf("expected dropped 1, got %d", inbox.Dropped())
	}
}

// C9: Wraparound Correctness
func TestAutonomyInbox_C9_WraparoundCorrectness(t *testing.T) {
	inbox := NewAutonomyInbox(16)
	for i := 0; i < 32; i++ {
		inbox.Push(context.Background(), ContextEvent{ID: "evt"})
		e, ok := inbox.Pop()
		if !ok || e.ID != "evt" {
			t.Fatalf("wraparound failed at %d", i)
		}
	}
}

// C10: Drain Batch Retrieval
func TestAutonomyInbox_C10_DrainBatchRetrieval(t *testing.T) {
	inbox := NewAutonomyInbox(16)
	for i := 0; i < 10; i++ {
		inbox.Push(context.Background(), ContextEvent{ID: "E"})
	}
	batch := inbox.Drain(5)
	if len(batch) != 5 {
		t.Fatalf("expected 5 events, got %d", len(batch))
	}
	if inbox.Len() != 5 {
		t.Fatalf("expected len 5, got %d", inbox.Len())
	}
}

// C11: Drain Empty Returns Empty Slice
func TestAutonomyInbox_C11_DrainEmpty(t *testing.T) {
	inbox := NewAutonomyInbox(16)
	batch := inbox.Drain(10)
	if len(batch) != 0 {
		t.Fatalf("expected 0 events, got %d", len(batch))
	}
}

// C12: Concurrent Multi-Producer Safety
func TestAutonomyInbox_C12_ConcurrentMultiProducer(t *testing.T) {
	inbox := NewAutonomyInbox(4096)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				inbox.Push(context.Background(), ContextEvent{})
			}
		}()
	}
	wg.Wait()
	if inbox.Pushed()+inbox.Dropped() != 4000 {
		t.Fatalf("expected 4000 total pushes, got %d", inbox.Pushed()+inbox.Dropped())
	}
}

// C13: Metrics Accuracy
func TestAutonomyInbox_C13_MetricsAccuracy(t *testing.T) {
	inbox := NewAutonomyInbox(16)
	for i := 0; i < 20; i++ {
		inbox.Push(context.Background(), ContextEvent{})
	}
	for i := 0; i < 10; i++ {
		inbox.Pop()
	}
	if inbox.Pushed() != 16 {
		t.Fatalf("expected 16 pushed, got %d", inbox.Pushed())
	}
	if inbox.Dropped() != 4 {
		t.Fatalf("expected 4 dropped, got %d", inbox.Dropped())
	}
	if inbox.Len() != 6 {
		t.Fatalf("expected 6 len, got %d", inbox.Len())
	}
}

// C14: GC Safety — Payload Reference Released
func TestAutonomyInbox_C14_GCSafety(t *testing.T) {
	inbox := NewAutonomyInbox(16)
	inbox.Push(context.Background(), ContextEvent{Payload: make([]byte, 1024)})
	inbox.Pop()

	// Inspect the underlying slot to ensure event payload is cleared
	headIdx := (inbox.head - 1) & inbox.mask
	if inbox.slots[headIdx].event.Payload != nil {
		t.Fatal("payload reference was not released")
	}
}

// C15: Context Cancellation
func TestAutonomyInbox_C15_ContextCancellation(t *testing.T) {
	inbox := NewAutonomyInbox(16)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := inbox.Push(ctx, ContextEvent{})
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// C16, C17, C18: Interface Satisfactions
var _ InboxPusher = (*AutonomyInbox)(nil)
var _ InboxDrainer = (*AutonomyInbox)(nil)
var _ InboxMetrics = (*AutonomyInbox)(nil)

// C19: Transceiver Uses InboxPusher
func TestAutonomyInbox_C19_TransceiverUsesInboxPusher(t *testing.T) {
	tr := Transceiver{}
	field, ok := reflect.TypeOf(tr).FieldByName("inbox")
	if !ok {
		t.Fatal("Transceiver missing inbox field")
	}
	if field.Type.Name() != "InboxPusher" {
		t.Fatalf("expected inbox field to be InboxPusher, got %v", field.Type.Name())
	}
}

// C20: Benchmark Push Latency
func BenchmarkPush(b *testing.B) {
	inbox := NewAutonomyInbox(4096)
	ctx := context.Background()
	event := ContextEvent{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		inbox.Push(ctx, event)
		inbox.Pop() // keep it from filling
	}
}

// Test Transceiver (migrated from old test)
func TestTransceiver_EmitEnvelope(t *testing.T) {
	t.Run("emit envelope", func(t *testing.T) {
		inbox := NewAutonomyInbox()
		transceiver := NewTransceiver(inbox)

		err := transceiver.EmitEnvelope(context.Background(), "main.go", []byte("payload"))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if inbox.Len() != 1 {
			t.Fatalf("expected 1 event, got %d", inbox.Len())
		}
	})
}
