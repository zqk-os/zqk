package audit

import (
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestAggregationKey(t *testing.T) {
	t.Parallel()
	got := AggregationKey(map[string]any{
		objects.FieldKeyEventType:  "cache_update",
		objects.FieldKeyTargetKind: "backlog_item",
	}, []string{"event_type", "target_kind"})
	want := "event_type:cache_update|target_kind:backlog_item"
	if got != want {
		t.Fatalf("%q want %q", got, want)
	}
	got = AggregationKey(map[string]any{objects.FieldKeyEventType: "cache_update"}, []string{"event_type", "target_kind"})
	if got != "event_type:cache_update|target_kind:unknown" {
		t.Fatalf("unknown: %q", got)
	}
}

func TestAppendEventFlushAtThreshold(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	event := map[string]any{objects.FieldKeyEventType: EventTypeCacheInvalidation, objects.FieldKeySeverity: SeverityLow}
	g := NewGroup("k", EventTypeCacheInvalidation, SeverityLow, event, 2, now)
	if AppendEvent(g, event, 2, 3, now) {
		t.Fatal("count 1 should not flush")
	}
	if AppendEvent(g, event, 2, 3, now) {
		t.Fatal("count 2 should not flush")
	}
	if !AppendEvent(g, event, 2, 3, now) {
		t.Fatal("count 3 should flush")
	}
	if g.Count != 3 || len(g.SampleEvents) != 2 {
		t.Fatalf("count=%d samples=%d", g.Count, len(g.SampleEvents))
	}
}

func TestAggregatedSummaryMap(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	g := &Group{
		Key:       "k",
		EventType: EventTypeCacheInvalidation,
		Severity:  SeverityLow,
		Count:     4,
		FirstSeen: now,
		LastSeen:  now,
	}
	got := AggregatedSummaryMap("AUD-1", "k", "ACC-1", g, now)
	if objects.GetString(got, objects.FieldKeyEventType) != EventTypeAggregatedSummary {
		t.Fatalf("event_type=%v", got[objects.FieldKeyEventType])
	}
	if objects.GetString(got, objects.FieldKeyID) != "AUD-1" {
		t.Fatalf("id=%v", got[objects.FieldKeyID])
	}
	if objects.GetString(got, objects.FieldKeyOperation) != "Aggregated 4 cache_invalidation events" {
		t.Fatalf("op=%v", got[objects.FieldKeyOperation])
	}
}
