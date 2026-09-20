package ambience

import (
	"context"
	"testing"
	"time"
)

func TestSignalAggregator(t *testing.T) {
	mesh := NewInMemoryEventMesh()
	agg := NewSignalAggregator(mesh, 50*time.Millisecond, 3)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := agg.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start: %v", err)
	}

	// Publish 3 events to trigger count flush
	for i := 0; i < 3; i++ {
		_ = mesh.Publish(ctx, AmbientEvent{Type: EventFileModified, URI: "file.go"})
	}

	select {
	case events := <-agg.Aggregated():
		if len(events) != 3 {
			t.Errorf("expected 3 events, got %d", len(events))
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for flush")
	}

	// Publish 1 event and wait for time flush
	_ = mesh.Publish(ctx, AmbientEvent{Type: EventFileModified, URI: "file2.go"})

	select {
	case events := <-agg.Aggregated():
		if len(events) != 1 {
			t.Errorf("expected 1 event, got %d", len(events))
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for flush")
	}

	_ = agg.Stop()
}
