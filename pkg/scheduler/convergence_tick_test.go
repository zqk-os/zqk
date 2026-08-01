package scheduler

import (
	"testing"
	"time"
)

func TestCountMeasureTicksInLastHour(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
	old := now.Add(-2 * time.Hour).Format(time.RFC3339)
	recent := now.Add(-30 * time.Minute).Format(time.RFC3339)
	log := []any{
		map[string]any{"action": "other", "timestamp": recent},
		map[string]any{"action": "measure_test_bundle_health", "timestamp": old},
		map[string]any{"action": "measure_test_bundle_health", "timestamp": recent},
		map[string]any{"action": "measure_test_bundle_health", "timestamp": recent},
	}
	if n := countMeasureTicksInLastHour(log, now); n != 2 {
		t.Fatalf("expected 2 ticks in window, got %d", n)
	}
}

func TestThresholdMaxTicksPerHour(t *testing.T) {
	t.Parallel()
	if n := thresholdMaxTicksPerHour(nil); n != 4 {
		t.Fatalf("default: got %d", n)
	}
	if n := thresholdMaxTicksPerHour(map[string]any{}); n != 4 {
		t.Fatalf("empty map: got %d", n)
	}
	if n := thresholdMaxTicksPerHour(map[string]any{"max_ticks_per_hour": 0}); n != 0 {
		t.Fatalf("explicit zero: got %d", n)
	}
	if n := thresholdMaxTicksPerHour(map[string]any{"max_ticks_per_hour": 12}); n != 12 {
		t.Fatalf("explicit 12: got %d", n)
	}
}
