package audit

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestTallyEvents(t *testing.T) {
	t.Parallel()
	got := TallyEvents([]map[string]any{
		{
			objects.FieldKeyID:         "AUD-1",
			objects.FieldKeyEventType:  EventTypeCacheUpdate,
			objects.FieldKeyStatus:     StatusCompleted,
			objects.FieldKeyTargetKind: "backlog_item",
			objects.FieldKeyOperation:  "create",
		},
		{
			objects.FieldKeyID:        "AUD-2",
			objects.FieldKeyEventType: EventTypeCacheUpdate,
			objects.FieldKeyStatus:    "failed",
		},
	})
	if got.EventCount != 2 || len(got.EventIDs) != 2 {
		t.Fatalf("count=%d ids=%v", got.EventCount, got.EventIDs)
	}
	if got.EventTypeCounts[EventTypeCacheUpdate] != 2 || got.StatusCounts[objects.FieldKeyFailed] != 1 {
		t.Fatalf("%+v", got)
	}
	if got.ErrorEventCount != 1 || got.ErrorRate() != 0.5 {
		t.Fatalf("errors=%d rate=%v", got.ErrorEventCount, got.ErrorRate())
	}
	if got.ObjectKindCounts["backlog_item"] != 1 || got.OperationCounts["create"] != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestTallyEventsEmptyTypeFallback(t *testing.T) {
	t.Parallel()
	got := TallyEvents([]map[string]any{{objects.FieldKeyID: "AUD-1"}})
	anyCounts := got.EventTypeCountsAny()
	if IntFromAny(anyCounts[EventTypeCountKeyAggregated]) != 1 {
		t.Fatalf("%v", anyCounts)
	}
}

func TestTallyEventsNil(t *testing.T) {
	t.Parallel()
	got := TallyEvents(nil)
	if got.EventCount != 0 || got.ErrorRate() != 0 {
		t.Fatalf("%+v", got)
	}
}
