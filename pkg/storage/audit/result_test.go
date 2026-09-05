package audit

import (
	"testing"
	"time"
)

func TestAggregationResultConstructors(t *testing.T) {
	t.Parallel()
	empty := EmptyAggregationResult()
	if empty.MetricsCreated != 0 || len(empty.EventsProcessed) != 0 {
		t.Fatalf("%+v", empty)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := CompletedAggregationResult("AAM-1", []string{"AUD-1", "AUD-2"}, 2, now, now)
	if got.EventCount != 2 || got.MetricsCreated != 1 || got.MetricID != "AAM-1" || got.EventsUpdated != 2 {
		t.Fatalf("%+v", got)
	}
}
