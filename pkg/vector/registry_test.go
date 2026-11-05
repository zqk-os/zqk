package vector

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/events"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestVectorRegistry_DefaultSeedAndList(t *testing.T) {
	bus := events.NewTopicBus(100)
	reg := NewRegistry(t.TempDir(), bus)

	list := reg.List(false)
	if len(list) < 7 {
		t.Fatalf("expected at least 7 seeded vectors, got: %d", len(list))
	}

	// Verify priority sorting
	prioritized := reg.List(true)
	if len(prioritized) != len(list) {
		t.Fatalf("mismatched length")
	}
	for i := 0; i < len(prioritized)-1; i++ {
		if prioritized[i].PriorityRank < prioritized[i+1].PriorityRank {
			t.Fatalf("expected prioritized list to be sorted descending by PriorityRank, got [%d]=%d, [%d]=%d",
				i, prioritized[i].PriorityRank, i+1, prioritized[i+1].PriorityRank)
		}
	}
}

func TestVectorRegistry_EvaluateAndEvents(t *testing.T) {
	bus := events.NewTopicBus(100)
	tempDir := t.TempDir()
	reg := NewRegistry(tempDir, bus)

	// Set a vector with a high threshold to guarantee a breach event
	highThresholdVec := Vector{
		ID:           "VEC-SECURITY",
		Dimension:    DimensionSecurity,
		PriorityRank: 99,
		Weight:       0.95,
		Threshold:    0.95, // Will breach since score is 0.90
		Status:       "pending",
		Description:  "Strict security",
	}
	if err := reg.Register(highThresholdVec); err != nil {
		t.Fatalf("failed to register vector: %v", err)
	}

	var evaluatedCount int
	var breachedCount int
	bus.Subscribe(EventVectorEvaluated, func(ctx context.Context, topic string, payload any) error {
		evaluatedCount++
		return nil
	})
	bus.Subscribe(EventVectorThresholdBreach, func(ctx context.Context, topic string, payload any) error {
		breachedCount++
		return nil
	})

	results, err := reg.Evaluate(context.Background(), true)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected evaluation results, got 0")
	}

	if evaluatedCount == 0 {
		t.Fatalf("expected vector.evaluated events to be dispatched")
	}
	if breachedCount == 0 {
		t.Fatalf("expected vector.threshold_breached event to be dispatched for VEC-SECURITY")
	}

	// Verify persistence file was written
	if _, err := fileutil.Stat(filepath.Join(tempDir, ".zqk", "vectors.json")); err != nil {
		t.Fatalf("expected vectors.json to be saved: %v", err)
	}
}

func TestVectorRegistry_Steer(t *testing.T) {
	bus := events.NewTopicBus(100)
	reg := NewRegistry(t.TempDir(), bus)

	var steeredEventReceived bool
	bus.Subscribe(EventVectorSteered, func(ctx context.Context, topic string, payload any) error {
		steeredEventReceived = true
		return nil
	})

	deltaWeight := 0.05
	newRank := 98
	steerDelta := VectorSteerDelta{
		VectorID:    "VEC-RELIABILITY",
		WeightDelta: &deltaWeight,
		NewRank:     &newRank,
		Reason:      "Urgent test stabilization required",
	}

	vec, err := reg.Steer(context.Background(), steerDelta)
	if err != nil {
		t.Fatalf("Steer failed: %v", err)
	}

	if vec.PriorityRank != 98 {
		t.Fatalf("expected priority rank 98, got: %d", vec.PriorityRank)
	}
	if !steeredEventReceived {
		t.Fatalf("expected vector.steered event to be dispatched")
	}
}

func TestVector_Validate(t *testing.T) {
	invalidID := Vector{
		ID:           "INVALID-NAME",
		Dimension:    DimensionSecurity,
		PriorityRank: 50,
		Weight:       0.5,
		Threshold:    0.5,
	}
	if err := invalidID.Validate(); err == nil {
		t.Fatalf("expected error for ID without VEC- prefix")
	}

	invalidDim := Vector{
		ID:           "VEC-TEST",
		Dimension:    "unknown-dim",
		PriorityRank: 50,
		Weight:       0.5,
		Threshold:    0.5,
	}
	if err := invalidDim.Validate(); err == nil {
		t.Fatalf("expected error for invalid dimension")
	}

	invalidRank := Vector{
		ID:           "VEC-TEST",
		Dimension:    DimensionSecurity,
		PriorityRank: 105,
		Weight:       0.5,
		Threshold:    0.5,
	}
	if err := invalidRank.Validate(); err == nil {
		t.Fatalf("expected error for rank > 100")
	}
}
