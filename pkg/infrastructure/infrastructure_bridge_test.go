package infrastructure_test

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/infrastructure"
	_ "github.com/lanceman/zqk/pkg/infrastructure/drivers/kafka" // Register driver
	"github.com/lanceman/zqk/pkg/objects"
)

func TestInfrastructureEngagement(t *testing.T) {
	ctx := context.Background()
	registry := infrastructure.GetRegistry()

	// 1. Engage Kafka
	spine, err := registry.Engage(ctx, "INF-KAFKA-001", "kafka", "localhost:9092", "secret")
	if err != nil {
		t.Fatalf("failed to engage kafka: %v", err)
	}

	// 2. Publish an event
	event := infrastructure.Event{
		ObjectID: "OBJ-001",
		Kind:     "backlog_item",
		Op:       "create",
		Payload:  map[string]any{objects.FieldKeyTitle: "Scaling to 100B Triples"},
	}

	err = spine.Publish(ctx, event)
	if err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	// 3. Subscribe
	err = spine.Subscribe(ctx, "backlog_item", func(ctx context.Context, e infrastructure.Event) error {
		return nil
	})
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}
}
