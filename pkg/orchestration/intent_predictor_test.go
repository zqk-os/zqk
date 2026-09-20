package orchestration

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestIntentPredictionHeuristicsEngine_PredictFromRecentMessages(t *testing.T) {
	pool := testkit.PrepareGraphConnectionForTest(t)
	ctx := context.Background()

	// Seed the database with some messages
	inbox := NewAutonomyInbox(pool)
	now := time.Now()

	msg1 := Message{
		ID:          "msg-test-1",
		Kind:        "cli_command",
		Destination: "system",
		Payload: map[string]any{
			objects.FieldKeyCommand: "zqk object create goal",
		},
		Timestamp: now.Add(-10 * time.Minute),
	}
	if err := inbox.RouteMessage(ctx, msg1); err != nil {
		t.Fatalf("Failed to seed message: %v", err)
	}

	engine := NewIntentPredictionHeuristicsEngine(pool)

	// Act
	predictions, err := engine.PredictFromRecentMessages(ctx, now.Add(-15*time.Minute))
	if err != nil {
		t.Fatalf("Failed to predict: %v", err)
	}

	// Assert
	if len(predictions) == 0 {
		t.Errorf("Expected predictions from recent messages, got 0")
	}

	found := false
	for _, p := range predictions {
		if p.TargetAction == "create_requirement" || p.TargetAction == "link_goal" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected prediction related to creating a requirement or linking a goal, got: %v", predictions)
	}
}
