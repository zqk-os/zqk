// Traceability: BLI-SYM-008, BLI-SYM-010, REQ-SYM-005
package ambient

import (
	"context"
	"testing"
	"time"
)

func TestEventHub_PublishAndSubscribe(t *testing.T) {
	hub := NewEventHub()

	if status := hub.Status(); status != "idle" {
		t.Errorf("Expected status idle, got %s", status)
	}

	var received bool
	hub.Subscribe(EventTypeFilesystem, func(ctx context.Context, event Event) error {
		received = true
		return nil
	})

	if status := hub.Status(); status != "active" {
		t.Errorf("Expected status active, got %s", status)
	}

	event := Event{
		Type:      EventTypeFilesystem,
		Payload:   "test payload",
		Timestamp: time.Now(),
	}

	err := hub.Publish(context.Background(), event)
	if err != nil {
		t.Fatalf("Failed to publish event: %v", err)
	}

	if !received {
		t.Error("Expected handler to receive event")
	}
}
