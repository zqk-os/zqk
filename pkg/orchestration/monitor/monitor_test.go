package monitor

import (
	"testing"
)

func TestMonitor(t *testing.T) {
	t.Run("Test Ticker Rendering", func(t *testing.T) {
		ticker := NewOmniTicker()
		ticker.Render([]string{"coder_agent"})
	})

	t.Run("Test Narrator", func(t *testing.T) {
		narrator := NewSemanticImpactNarrator()
		story := narrator.Narrate(map[string]any{"data": "test"})
		if story == "" {
			t.Errorf("narrator returned empty story")
		}
	})
}
