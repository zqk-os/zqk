package intent

import (
	"context"
	"testing"
)

func TestDefaultIntentMapper_Map(t *testing.T) {
	mapper := NewDefaultIntentMapper()

	t.Run("Map event to intent", func(t *testing.T) {
		eventID := "evt-123"
		payload := map[string]any{"data": "test"}

		intent, err := mapper.Map(context.Background(), eventID, payload)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if intent.Signature != eventID {
			t.Errorf("expected signature %s, got %s", eventID, intent.Signature)
		}
	})
}
