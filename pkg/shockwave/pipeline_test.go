package shockwave

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestEventPipeline_PublishSubscribe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pipeline := NewEventPipeline(PipelineConfig{
		BufferSize: 50,
	})
	goroutinelabels.NewGoroutine("test.pipeline_run", "event pipeline background worker").StartSimple(func() {
		pipeline.Run(ctx)
	})

	var receivedCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(2)

	// Subscribe to lifecycle events
	sub1 := pipeline.Subscribe("lifecycle.*", func(evt ShockwaveEvent) {
		receivedCount.Add(1)
		wg.Done()
	})
	defer sub1.Unsubscribe()

	// Subscribe to all events
	sub2 := pipeline.Subscribe("*", func(evt ShockwaveEvent) {
		receivedCount.Add(1)
		wg.Done()
	})
	defer sub2.Unsubscribe()

	testEvt := ShockwaveEvent{
		Topic:     "lifecycle.promoted",
		SourceID:  "BLI-TEST-001",
		Timestamp: time.Now(),
		Payload:   map[string]any{"status": "complete"},
	}

	pipeline.Publish(testEvt)

	done := make(chan struct{})
	goroutinelabels.NewGoroutine("test.wait_wg", "wait for shockwave subscribers").StartSimple(func() {
		wg.Wait()
		close(done)
	})

	select {
	case <-done:
		if receivedCount.Load() != 2 {
			t.Fatalf("expected 2 receipts, got %d", receivedCount.Load())
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for subscriber notifications (received: %d)", receivedCount.Load())
	}
}

func TestEventPipeline_GracefulShutdown(t *testing.T) {
	pipeline := NewEventPipeline(PipelineConfig{
		BufferSize: 20,
	})

	ctx, cancel := context.WithCancel(context.Background())
	goroutinelabels.NewGoroutine("test.pipeline_run", "event pipeline background worker").StartSimple(func() {
		pipeline.Run(ctx)
	})

	var delivered atomic.Int32
	sub := pipeline.Subscribe("test.event", func(evt ShockwaveEvent) {
		delivered.Add(1)
	})
	defer sub.Unsubscribe()

	pipeline.Publish(ShockwaveEvent{Topic: "test.event", SourceID: "source-1"})
	cancel()

	// Wait briefly for drain
	time.Sleep(50 * time.Millisecond)

	if delivered.Load() != 1 {
		t.Fatalf("expected 1 delivered event after drain, got %d", delivered.Load())
	}
}
