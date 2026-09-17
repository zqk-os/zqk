package audit

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestShouldAggregate(t *testing.T) {
	t.Parallel()
	rules := DefaultRules()
	if ShouldAggregate(false, rules, map[string]any{objects.FieldKeyEventType: EventTypeCacheInvalidation, objects.FieldKeySeverity: SeverityLow}) {
		t.Fatal("disabled buffer should not aggregate")
	}
	if !ShouldAggregate(true, rules, map[string]any{objects.FieldKeyEventType: EventTypeCacheInvalidation, objects.FieldKeySeverity: SeverityLow}) {
		t.Fatal("cache invalidation low should aggregate")
	}
	if ShouldAggregate(true, rules, map[string]any{
		objects.FieldKeyEventType: EventTypeCacheInvalidation,
		objects.FieldKeySeverity:  SeverityLow,
		objects.FieldKeyTargetID:  "BLI-1",
	}) {
		t.Fatal("target_id should block non-scheduler aggregation")
	}
	if ShouldAggregate(true, rules, map[string]any{
		objects.FieldKeyEventType: EventTypeSchedulerJobStarted,
		objects.FieldKeySeverity:  SeverityLow,
		objects.FieldKeyTargetID:  "SCH-1",
	}) {
		t.Fatal("scheduler events with target_id must not aggregate")
	}
	if ShouldAggregate(true, rules, map[string]any{
		objects.FieldKeyEventType: EventTypeSchedulerJobCompleted,
		objects.FieldKeySeverity:  SeverityLow,
		objects.FieldKeyTargetID:  "SCH-1",
	}) {
		t.Fatal("scheduler completed events with target_id must not aggregate")
	}
}
