package ambience

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestInMemoryEventMesh(t *testing.T) {
	mesh := NewInMemoryEventMesh()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to EventFileModified
	ch, err := mesh.Subscribe(ctx, []EventType{EventFileModified})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Publish an event that should be received
	expectedEvent := AmbientEvent{
		ID:        "1",
		Type:      EventFileModified,
		URI:       "file:///test/path",
		Timestamp: time.Now().UnixNano(),
		Payload:   []byte("test"),
	}

	goroutinelabels.NewGoroutine("test_eventmesh_publish", "publish expected event").StartSimple(func() {
		err := mesh.Publish(context.Background(), expectedEvent)
		if err != nil {
			t.Errorf("publish error: %v", err)
		}
	})

	select {
	case evt := <-ch:
		if evt.ID != expectedEvent.ID {
			t.Errorf("expected event ID %s, got %s", expectedEvent.ID, evt.ID)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	// Publish an event that should NOT be received
	ignoredEvent := AmbientEvent{
		ID:        "2",
		Type:      EventFocusChanged,
		URI:       "file:///test/path2",
		Timestamp: time.Now().UnixNano(),
		Payload:   []byte("test2"),
	}

	goroutinelabels.NewGoroutine("test_eventmesh_publish_ignored", "publish ignored event").StartSimple(func() {
		err := mesh.Publish(context.Background(), ignoredEvent)
		if err != nil {
			t.Errorf("publish error: %v", err)
		}
	})

	select {
	case <-ch:
		t.Fatal("received unexpected event")
	case <-time.After(100 * time.Millisecond):
		// Success: no event received
	}
}

func TestInMemoryEventMesh_CancelSubscription(t *testing.T) {
	mesh := NewInMemoryEventMesh()

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := mesh.Subscribe(ctx, []EventType{EventFileModified})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Cancel subscription
	cancel()

	deadline := time.Now().Add(2 * time.Second)
	var subs int
	for time.Now().Before(deadline) {
		mesh.mu.RLock()
		subs = len(mesh.subscribers[EventFileModified])
		mesh.mu.RUnlock()
		if subs == 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if subs != 0 {
		t.Errorf("expected 0 subscribers, got %d", subs)
	}

	// Publish after cancel should not block
	err = mesh.Publish(context.Background(), AmbientEvent{
		ID:   "3",
		Type: EventFileModified,
	})
	if err != nil {
		t.Errorf("unexpected error on publish: %v", err)
	}

	select {
	case <-ch:
		t.Fatal("received event on cancelled subscription")
	case <-time.After(50 * time.Millisecond):
		// Success
	}
}
