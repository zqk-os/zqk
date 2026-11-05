package steer

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/events"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/vector"
)

func TestParseIntent_MultipliersAndDimensions(t *testing.T) {
	cases := []struct {
		input             string
		expectedIntensity float64
		expectedDirection string
		expectedDimension vector.Dimension
	}{
		{
			input:             "prioritize security urgently",
			expectedIntensity: 1.5,
			expectedDirection: "boost",
			expectedDimension: vector.DimensionSecurity,
		},
		{
			input:             "strictly harden reliability and tests",
			expectedIntensity: 1.5,
			expectedDirection: "boost",
			expectedDimension: vector.DimensionReliability,
		},
		{
			input:             "slightly relax docstrings",
			expectedIntensity: 0.5,
			expectedDirection: "reduce",
			expectedDimension: vector.DimensionReadability,
		},
		{
			input:             "boost speed and conserve tokens",
			expectedIntensity: 1.0,
			expectedDirection: "boost",
			expectedDimension: vector.DimensionEfficiency,
		},
	}

	for _, c := range cases {
		intent := ParseIntent(c.input)
		if intent.Intensity != c.expectedIntensity {
			t.Errorf("for %q, expected intensity %.1f, got %.1f", c.input, c.expectedIntensity, intent.Intensity)
		}
		if intent.Direction != c.expectedDirection {
			t.Errorf("for %q, expected direction %q, got %q", c.input, c.expectedDirection, intent.Direction)
		}
		hasDim := false
		for _, d := range intent.Dimensions {
			if d == c.expectedDimension {
				hasDim = true
				break
			}
		}
		if !hasDim {
			t.Errorf("for %q, expected dimension %q in %v", c.input, c.expectedDimension, intent.Dimensions)
		}
	}
}

func TestMetabolizer_MetabolizeAndEvents(t *testing.T) {
	tempDir := t.TempDir()
	bus := events.NewTopicBus(100)
	reg := vector.NewRegistry(tempDir, bus)
	metabolizer := NewMetabolizer(tempDir, reg, bus)

	var eventReceived bool
	bus.Subscribe(EventSteeringApplied, func(ctx context.Context, topic string, payload any) error {
		eventReceived = true
		return nil
	})

	outcome, err := metabolizer.Metabolize(context.Background(), "prioritize security urgently")
	if err != nil {
		t.Fatalf("Metabolize failed: %v", err)
	}

	if !eventReceived {
		t.Fatalf("expected steering.applied event to be dispatched")
	}

	if len(outcome.Adjustments) == 0 {
		t.Fatalf("expected vector adjustments, got none")
	}

	// Verify persistence file
	statePath := filepath.Join(tempDir, ".zqk", "steer", "state.json")
	if _, err := fileutil.Stat(statePath); err != nil {
		t.Fatalf("expected state.json to exist: %v", err)
	}

	// Test Reset
	resetOutcome, err := metabolizer.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset failed: %v", err)
	}
	if !resetOutcome.IsReset {
		t.Fatalf("expected IsReset=true")
	}
}
