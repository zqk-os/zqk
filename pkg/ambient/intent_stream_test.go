package ambient

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// Traceability:
// REQ: REQ-1793597212444417000-c9f17c58 (Ambient Intent Stream Aggregator & Continuous Swarm Runway Depth Arbiter)
// CRIT: CRIT-1793597256231306000-d877033a (Operational Dynamic Proof: Real-Time Stream Ingestion, Predictive Runway Calculation & Watermark Notification Loop)
// BLI: BLI-1793599919818794000-70acfe86

func TestIntentStream_OperationalDynamicProof(t *testing.T) {
	t.Parallel()

	// 1. Initialize stream with 2 active agents
	stream := NewAmbientIntentStream(AmbientIntentStreamConfig{
		Capacity:          10,
		ActiveAgents:      2,
		CriticalThreshold: 1.0,
		WarningThreshold:  2.0,
		AlertBufferSize:   10,
	})

	// Initial evaluation: 0 pending / 2 agents = depth 0.0 -> Critical threshold
	depth, threshold := stream.EvaluateRunwayDepth()
	if threshold != WatermarkThresholdCritical {
		t.Fatalf("expected initial threshold %s, got %s (depth: %.2f)", WatermarkThresholdCritical, threshold, depth)
	}

	// 2. Subscribe to alerts
	alertCh, unsubscribe := stream.SubscribeAlerts()
	defer unsubscribe()

	// 3. Ingest real-time intent events
	events := []IntentEvent{
		{ID: "intent-1", AgentID: "agent-alpha", Intent: "optimize query plan"},
		{ID: "intent-2", AgentID: "agent-beta", Intent: "refactor memory cache"},
		{ID: "intent-3", AgentID: "agent-alpha", Intent: "prune stale worktrees"},
	}

	for _, e := range events {
		if err := stream.Push(e); err != nil {
			t.Fatalf("failed to push event %s: %v", e.ID, err)
		}
	}

	// Now pending = 3, agents = 2 -> depth = 1.5 -> Warning threshold (1.0 < depth <= 2.0)
	depth, threshold = stream.EvaluateRunwayDepth()
	if depth != 1.5 {
		t.Fatalf("expected depth 1.5, got %.2f", depth)
	}
	if threshold != WatermarkThresholdWarning {
		t.Fatalf("expected threshold %s, got %s", WatermarkThresholdWarning, threshold)
	}

	// 4. Emit alert and verify received on subscription
	alert, emitted := stream.CheckAndEmitAlert()
	if !emitted {
		t.Fatal("expected alert to be emitted for warning threshold")
	}
	if alert.Threshold != WatermarkThresholdWarning {
		t.Fatalf("expected alert threshold %s, got %s", WatermarkThresholdWarning, alert.Threshold)
	}

	select {
	case received := <-alertCh:
		if received.Threshold != WatermarkThresholdWarning {
			t.Fatalf("expected received alert threshold %s, got %s", WatermarkThresholdWarning, received.Threshold)
		}
		if received.RunwayDepth != 1.5 {
			t.Fatalf("expected received depth 1.5, got %.2f", received.RunwayDepth)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for runway alert")
	}

	// 5. Ingest additional events to lift runway depth to Nominal (pending = 5, agents = 2 -> depth = 2.5 > 2.0)
	moreEvents := []IntentEvent{
		{ID: "intent-4", AgentID: "agent-gamma", Intent: "compile doc portal"},
		{ID: "intent-5", AgentID: "agent-delta", Intent: "run vet gate suites"},
	}
	if err := stream.PushBatch(moreEvents); err != nil {
		t.Fatalf("failed to push batch: %v", err)
	}

	depth, threshold = stream.EvaluateRunwayDepth()
	if depth != 2.5 {
		t.Fatalf("expected depth 2.5, got %.2f", depth)
	}
	if threshold != WatermarkThresholdNominal {
		t.Fatalf("expected threshold %s, got %s", WatermarkThresholdNominal, threshold)
	}

	// In nominal state, CheckAndEmitAlert should NOT emit
	_, emitted = stream.CheckAndEmitAlert()
	if emitted {
		t.Fatal("expected no alert to be emitted in nominal state")
	}
}

func TestIntentStream_PushAndDrainOrder(t *testing.T) {
	t.Parallel()

	stream := NewAmbientIntentStream(AmbientIntentStreamConfig{
		Capacity:     5,
		ActiveAgents: 1,
	})

	// Push 3 events
	for i := 1; i <= 3; i++ {
		err := stream.Push(IntentEvent{
			ID:     fmt.Sprintf("evt-%d", i),
			Intent: fmt.Sprintf("intent task %d", i),
		})
		if err != nil {
			t.Fatalf("unexpected error pushing evt-%d: %v", i, err)
		}
	}

	if stream.Size() != 3 {
		t.Fatalf("expected size 3, got %d", stream.Size())
	}

	// DrainN 2 events
	drainedN := stream.DrainN(2)
	if len(drainedN) != 2 {
		t.Fatalf("expected 2 drained events, got %d", len(drainedN))
	}
	if drainedN[0].ID != "evt-1" || drainedN[1].ID != "evt-2" {
		t.Fatalf("unexpected order in DrainN: got %s, %s", drainedN[0].ID, drainedN[1].ID)
	}
	if stream.Size() != 1 {
		t.Fatalf("expected 1 remaining event, got %d", stream.Size())
	}

	// Push batch of 2 more events
	batch := []IntentEvent{
		{ID: "evt-4", Intent: "intent task 4"},
		{ID: "evt-5", Intent: "intent task 5"},
	}
	if err := stream.PushBatch(batch); err != nil {
		t.Fatalf("failed to push batch: %v", err)
	}

	// Drain remaining: should be evt-3, evt-4, evt-5
	remaining := stream.Drain()
	if len(remaining) != 3 {
		t.Fatalf("expected 3 remaining events, got %d", len(remaining))
	}
	expectedIDs := []string{"evt-3", "evt-4", "evt-5"}
	for i, exp := range expectedIDs {
		if remaining[i].ID != exp {
			t.Fatalf("index %d expected %s, got %s", i, exp, remaining[i].ID)
		}
	}

	if stream.Size() != 0 {
		t.Fatalf("expected stream size 0 after full drain, got %d", stream.Size())
	}
}

func TestIntentStream_RunwayCalculationAndAlertLoop(t *testing.T) {
	t.Parallel()

	stream := NewAmbientIntentStream(AmbientIntentStreamConfig{
		Capacity:          20,
		ActiveAgents:      4,
		CriticalThreshold: 1.0,
		WarningThreshold:  2.0,
		AlertBufferSize:   10,
	})

	alertCh, cancel := stream.SubscribeAlerts()
	defer cancel()

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	// Start monitor with fast tick
	stream.StartRunwayMonitor(ctx, 20*time.Millisecond)

	// Since pending = 0 and agents = 4, depth = 0.0 <= 1.0 (Critical)
	select {
	case alert := <-alertCh:
		if alert.Threshold != WatermarkThresholdCritical {
			t.Fatalf("expected Critical alert, got %s", alert.Threshold)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for initial critical alert")
	}

	// Now add 6 intents: 6/4 = 1.5 (Warning)
	for i := 1; i <= 6; i++ {
		_ = stream.Push(IntentEvent{
			ID:     fmt.Sprintf("e-%d", i),
			Intent: fmt.Sprintf("work-%d", i),
		})
	}

	// Wait for warning alert from monitor loop
	select {
	case alert := <-alertCh:
		if alert.Threshold != WatermarkThresholdWarning && alert.Threshold != WatermarkThresholdCritical {
			t.Fatalf("unexpected alert threshold: %s", alert.Threshold)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for warning alert")
	}

	// Add 6 more intents: 12/4 = 3.0 (Nominal)
	for i := 7; i <= 12; i++ {
		_ = stream.Push(IntentEvent{
			ID:     fmt.Sprintf("e-%d", i),
			Intent: fmt.Sprintf("work-%d", i),
		})
	}

	depth, threshold := stream.EvaluateRunwayDepth()
	if depth != 3.0 || threshold != WatermarkThresholdNominal {
		t.Fatalf("expected nominal runway depth 3.0, got depth %.2f (threshold: %s)", depth, threshold)
	}
}

func TestIntentStream_MonitorContextCancellation(t *testing.T) {
	t.Parallel()

	stream := NewAmbientIntentStream(AmbientIntentStreamConfig{
		Capacity:     10,
		ActiveAgents: 1,
	})

	ctx, cancel := context.WithCancel(context.Background())

	// Start monitor
	stream.StartRunwayMonitor(ctx, 10*time.Millisecond)

	// Let it run one cycle
	time.Sleep(30 * time.Millisecond)

	// Cancel context to ensure shutdown without hanging
	cancel()
	time.Sleep(30 * time.Millisecond)
}

func TestRingBuffer_Operations(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(3)
	if rb.Capacity() != 3 {
		t.Fatalf("expected capacity 3, got %d", rb.Capacity())
	}
	if !rb.IsEmpty() {
		t.Fatal("expected empty buffer")
	}

	// Push 3 items
	for i := 1; i <= 3; i++ {
		evicted, err := rb.Push(IntentEvent{ID: fmt.Sprintf("item-%d", i), Intent: "valid intent"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if evicted {
			t.Fatalf("did not expect eviction for item %d", i)
		}
	}

	if !rb.IsFull() {
		t.Fatal("expected buffer to be full")
	}
	if rb.Size() != 3 {
		t.Fatalf("expected size 3, got %d", rb.Size())
	}

	// Push 4th item -> should evict item-1
	evicted, err := rb.Push(IntentEvent{ID: "item-4", Intent: "valid intent"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !evicted {
		t.Fatal("expected item-1 to be evicted on overflow")
	}
	if rb.EvictedCount() != 1 {
		t.Fatalf("expected evicted count 1, got %d", rb.EvictedCount())
	}

	// Peek should now see item-2
	peeked, ok := rb.Peek()
	if !ok || peeked.ID != "item-2" {
		t.Fatalf("expected peeked ID item-2, got %s (ok: %v)", peeked.ID, ok)
	}

	// Pop item-2
	popped, ok := rb.Pop()
	if !ok || popped.ID != "item-2" {
		t.Fatalf("expected popped item-2, got %s", popped.ID)
	}

	// Drain remaining: item-3, item-4
	drained := rb.Drain()
	if len(drained) != 2 || drained[0].ID != "item-3" || drained[1].ID != "item-4" {
		t.Fatalf("unexpected drained items: %v", drained)
	}

	if !rb.IsEmpty() {
		t.Fatal("expected buffer to be empty after drain")
	}

	// Clear test
	_, _ = rb.Push(IntentEvent{ID: "clear-me", Intent: "intent"})
	rb.Clear()
	if rb.Size() != 0 {
		t.Fatalf("expected 0 after Clear, got %d", rb.Size())
	}
}

func TestIntentEvent_Validation(t *testing.T) {
	t.Parallel()

	// Missing ID
	e := IntentEvent{Intent: "some intent"}
	if err := e.Validate(); err != ErrIntentEventIDRequired {
		t.Fatalf("expected ErrIntentEventIDRequired, got %v", err)
	}

	// Missing Intent
	e = IntentEvent{ID: "id-1"}
	if err := e.Validate(); err != ErrIntentEventIntentRequired {
		t.Fatalf("expected ErrIntentEventIntentRequired, got %v", err)
	}

	// Corrupt payload with null byte
	e = IntentEvent{ID: "id-1", Intent: "hello\x00world"}
	if err := e.Validate(); err != ErrIntentEventCorruptPayload {
		t.Fatalf("expected ErrIntentEventCorruptPayload, got %v", err)
	}

	// Canceled status
	e = IntentEvent{ID: "id-1", Intent: "valid intent", Status: "canceled"}
	if err := e.Validate(); err != ErrIntentEventCanceled {
		t.Fatalf("expected ErrIntentEventCanceled, got %v", err)
	}
	e.Status = "CANCELLED"
	if err := e.Validate(); err != ErrIntentEventCanceled {
		t.Fatalf("expected ErrIntentEventCanceled, got %v", err)
	}

	// Valid
	e = IntentEvent{ID: "id-1", Intent: "clean valid intent", Status: "active"}
	if err := e.Validate(); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestIntentStream_ConcurrentPushAndDrain(t *testing.T) {
	t.Parallel()

	stream := NewAmbientIntentStream(AmbientIntentStreamConfig{
		Capacity:     50,
		ActiveAgents: 2,
	})

	const producers = 4
	const itemsPerProducer = 50
	var wg sync.WaitGroup
	var pushErrors int64

	// Concurrency compliance: strictly use goroutinelabels.NewGoroutine
	for p := 0; p < producers; p++ {
		pID := p
		wg.Add(1)
		goroutinelabels.NewGoroutine("test_producer", "concurrent stream producer").StartSimple(func() {
			defer wg.Done()
			for i := 0; i < itemsPerProducer; i++ {
				err := stream.Push(IntentEvent{
					ID:     fmt.Sprintf("prod-%d-item-%d", pID, i),
					Intent: fmt.Sprintf("work unit %d from %d", i, pID),
				})
				if err != nil {
					atomic.AddInt64(&pushErrors, 1)
				}
			}
		})
	}

	// Concurrent consumer draining intermittently
	var drainedCount int64
	stopConsumer := make(chan struct{})
	var consumerWg sync.WaitGroup
	consumerWg.Add(1)
	goroutinelabels.NewGoroutine("test_consumer", "concurrent stream consumer").StartSimple(func() {
		defer consumerWg.Done()
		for {
			select {
			case <-stopConsumer:
				d := stream.Drain()
				atomic.AddInt64(&drainedCount, int64(len(d)))
				return
			default:
				d := stream.DrainN(5)
				atomic.AddInt64(&drainedCount, int64(len(d)))
				time.Sleep(1 * time.Millisecond)
			}
		}
	})

	wg.Wait()
	close(stopConsumer)
	consumerWg.Wait()

	if atomic.LoadInt64(&pushErrors) != 0 {
		t.Fatalf("expected 0 push errors under concurrency, got %d", pushErrors)
	}

	totalAccounted := atomic.LoadInt64(&drainedCount) + int64(stream.Size()) + stream.EvictedCount()
	expectedTotal := int64(producers * itemsPerProducer)
	if totalAccounted != expectedTotal {
		t.Fatalf("concurrency balance mismatch: drained(%d) + current(%d) + evicted(%d) = %d != expected(%d)",
			drainedCount, stream.Size(), stream.EvictedCount(), totalAccounted, expectedTotal)
	}
}
