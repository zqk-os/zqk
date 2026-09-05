package audit

import "testing"

func TestMergeMetricsEventTypeCounts(t *testing.T) {
	t.Parallel()
	metric1 := map[string]any{
		MergeKeyEventCount:      2,
		MergeKeyEventTypeCounts: map[string]any{"create": 1, "update": 1},
	}
	metric2 := map[string]any{
		MergeKeyEventCount:      3,
		MergeKeyEventTypeCounts: map[string]any{"create": 2, "delete": 1},
		MergeKeyStatusCounts:    map[string]any{"completed": 1, "failed": 2},
	}
	got := MergeMetrics(Prefix, RangeSeparator, metric1, metric2)
	if got[MergeKeyEventCount] != 5 {
		t.Fatalf("event_count=%v", got[MergeKeyEventCount])
	}
	counts, ok := got[MergeKeyEventTypeCounts].(map[string]any)
	if !ok {
		t.Fatalf("event_type_counts type %T", got[MergeKeyEventTypeCounts])
	}
	if IntFromAny(counts["create"]) != 3 || IntFromAny(counts["update"]) != 1 || IntFromAny(counts["delete"]) != 1 {
		t.Fatalf("counts=%v", counts)
	}
}

func TestMergeAnyMetrics(t *testing.T) {
	t.Parallel()
	got, ok := MergeAnyMetrics(Prefix, RangeSeparator, map[string]any{MergeKeyEventCount: 1}, map[string]any{MergeKeyEventCount: 2})
	if !ok || got[MergeKeyEventCount] != 3 {
		t.Fatalf("got=%v ok=%v", got, ok)
	}
	if _, ok := MergeAnyMetrics(Prefix, RangeSeparator, "x", map[string]any{}); ok {
		t.Fatal("expected reject")
	}
}
