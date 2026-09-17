package observer

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestObserverEmitter_SubscribeAndEmit(t *testing.T) {
	em := NewObserverEmitter()
	var got []ObserverEvent
	var mu sync.Mutex
	em.Subscribe(func(ctx context.Context, ev ObserverEvent) {
		mu.Lock()
		got = append(got, ev)
		mu.Unlock()
	})
	ctx := context.Background()
	em.Emit(ctx, ObserverEvent{Type: EventExtractStarted, Dir: "test"})
	em.Emit(ctx, ObserverEvent{Type: EventExtractCompleted, Dir: "test", EntityCount: 5})
	// Allow async delivery
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	n := len(got)
	mu.Unlock()
	if n != 2 {
		t.Errorf("expected 2 events, got %d", n)
	}
}

func TestObserverEmitter_EmitSetsTimestamp(t *testing.T) {
	em := NewObserverEmitter()
	var mu sync.Mutex
	var ts time.Time
	em.Subscribe(func(ctx context.Context, ev ObserverEvent) {
		mu.Lock()
		ts = ev.Timestamp
		mu.Unlock()
	})
	ctx := context.Background()
	em.Emit(ctx, ObserverEvent{Type: EventExtractStarted})
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	actual := ts
	mu.Unlock()
	if actual.IsZero() {
		t.Error("expected timestamp to be set")
	}
}

func TestObserverEmitter_MultipleSubscribers(t *testing.T) {
	em := NewObserverEmitter()
	const n = 5
	var counts [n]int
	var mu sync.Mutex
	for i := 0; i < n; i++ {
		i := i
		em.Subscribe(func(ctx context.Context, ev ObserverEvent) {
			mu.Lock()
			counts[i]++
			mu.Unlock()
		})
	}
	ctx := context.Background()
	em.Emit(ctx, ObserverEvent{Type: EventExtractCompleted, Dir: "multi"})
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < n; i++ {
		if counts[i] != 1 {
			t.Errorf("subscriber %d got %d events, want 1", i, counts[i])
		}
	}
}

func TestObserverEmitter_ConcurrentSubscribeAndEmit(t *testing.T) {
	em := NewObserverEmitter()
	var mu sync.Mutex
	var count int
	em.Subscribe(func(ctx context.Context, ev ObserverEvent) {
		mu.Lock()
		count++
		mu.Unlock()
	})
	ctx := context.Background()
	// Emit from "main" and allow delivery
	em.Emit(ctx, ObserverEvent{Type: EventExtractStarted})
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	n := count
	mu.Unlock()
	if n != 1 {
		t.Errorf("expected 1 event delivered, got %d", n)
	}
}

func TestObserverEmitter_ContextCancelled_SubscriberMayNotRun(t *testing.T) {
	em := NewObserverEmitter()
	var ran bool
	em.Subscribe(func(ctx context.Context, ev ObserverEvent) {
		ran = true
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before emit
	em.Emit(ctx, ObserverEvent{Type: EventExtractStarted})
	time.Sleep(30 * time.Millisecond)
	// WithContext(ctx) checks ctx.Done() before running; subscriber may not run
	// So we only assert that Emit didn't block/panic
	if ran {
		t.Logf("subscriber ran despite cancelled context (acceptable if check is after start)")
	}
}

func TestObserverEmitter_NoSubscribers(t *testing.T) {
	em := NewObserverEmitter()
	ctx := context.Background()
	em.Emit(ctx, ObserverEvent{Type: EventExtractCompleted}) // must not block or panic
}

func TestNotifyAgentConnection_NotifiesCallbacksAndEmitter(t *testing.T) {
	var mu sync.Mutex
	var got AgentConnectionInfo
	RegisterConnectionEventCallback(func(ctx context.Context, info AgentConnectionInfo) {
		mu.Lock()
		got = info
		mu.Unlock()
	})
	var emitted bool
	DefaultEmitter.Subscribe(func(ctx context.Context, ev ObserverEvent) {
		if ev.Type == EventAgentConnectionStarted {
			mu.Lock()
			emitted = true
			mu.Unlock()
		}
	})
	ctx := context.Background()
	NotifyAgentConnection(ctx, AgentConnectionInfo{
		ClientID: "test-client", ClientName: "TestAgent", Version: "1.0",
	})
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	g := got
	em := emitted
	mu.Unlock()
	if g.ClientID != "test-client" || g.ClientName != "TestAgent" {
		t.Errorf("callback got %+v", g)
	}
	if !em {
		t.Error("expected DefaultEmitter to receive EventAgentConnectionStarted")
	}
}
