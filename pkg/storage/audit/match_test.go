package audit

import "testing"

func TestShouldAggregate(t *testing.T) {
	t.Parallel()
	rules := DefaultRules()
	if ShouldAggregate(false, rules, map[string]any{"event_type": EventTypeCacheInvalidation, "severity": SeverityLow}) {
		t.Fatal("disabled buffer should not aggregate")
	}
	if !ShouldAggregate(true, rules, map[string]any{"event_type": EventTypeCacheInvalidation, "severity": SeverityLow}) {
		t.Fatal("cache invalidation low should aggregate")
	}
	if ShouldAggregate(true, rules, map[string]any{
		"event_type": EventTypeCacheInvalidation,
		"severity":   SeverityLow,
		"target_id":  "BLI-1",
	}) {
		t.Fatal("target_id should block non-scheduler aggregation")
	}
	if !ShouldAggregate(true, rules, map[string]any{
		"event_type": EventTypeSchedulerJobStarted,
		"severity":   SeverityLow,
		"target_id":  "SCH-1",
	}) {
		t.Fatal("scheduler events aggregate even with target_id")
	}
}
