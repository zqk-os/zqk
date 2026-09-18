package orchestration

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestIntentPrewarmer_Prewarm(t *testing.T) {
	pool := testkit.PrepareGraphConnectionForTest(t)

	prewarmer := NewIntentPrewarmer(pool)

	prediction := IntentPrediction{
		TargetAction: "create_requirement",
		TargetObject: "requirement",
		Confidence:   0.9,
		Reasoning:    "testing prewarm",
	}

	err := prewarmer.Prewarm(context.Background(), prediction)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
