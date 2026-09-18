package shockwave

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestEventPipeline_BackpressureNonBlocking(t *testing.T) {
	// Buffer size 5 with DropOnOverflow
	pipeline := NewEventPipeline(PipelineConfig{
		BufferSize:     5,
		DropOnOverflow: true,
	})

	// Note: We deliberately do NOT call pipeline.Run(ctx) here to simulate full queue
	published := 0
	for i := 0; i < 20; i++ {
		ok := pipeline.Publish(ShockwaveEvent{
			Topic:    "overflow.test",
			SourceID: fmt.Sprintf("src-%d", i),
		})
		if ok {
			published++
		}
	}

	// Should not deadlock or block indefinitely, and exactly bufferSize items should fit
	if published != 5 {
		t.Fatalf("expected 5 published events before buffer filled, got %d", published)
	}

	if pipeline.DroppedCount() != 15 {
		t.Fatalf("expected 15 dropped events, got %d", pipeline.DroppedCount())
	}
}

func TestEventPipeline_Unsubscribe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pipeline := NewEventPipeline(PipelineConfig{BufferSize: 10})
	goroutinelabels.NewGoroutine("test.pipeline_run", "event pipeline background worker").StartSimple(func() {
		pipeline.Run(ctx)
	})

	var count atomic.Int32
	sub := pipeline.Subscribe("unsub.test", func(evt ShockwaveEvent) {
		count.Add(1)
	})

	pipeline.Publish(ShockwaveEvent{Topic: "unsub.test"})
	time.Sleep(20 * time.Millisecond)

	sub.Unsubscribe()

	pipeline.Publish(ShockwaveEvent{Topic: "unsub.test"})
	time.Sleep(20 * time.Millisecond)

	if count.Load() != 1 {
		t.Fatalf("expected 1 event delivered before unsubscribe, got %d", count.Load())
	}
}
