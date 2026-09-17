package audit

import (
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestCreatedAtInclusiveWindow(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	got := CreatedAtInclusiveWindow(start, end)
	inner, _ := got[objects.FieldKeyCreatedAt].(map[string]any)
	if inner[FilterOpGte] != "2026-01-01T00:00:00Z" || inner[FilterOpLte] != "2026-01-02T00:00:00Z" {
		t.Fatalf("%v", got)
	}
}

func TestWithStatusIn(t *testing.T) {
	t.Parallel()
	base := CreatedAtInclusiveWindow(time.Unix(0, 0).UTC(), time.Unix(1, 0).UTC())
	got := WithStatusIn(base, []string{"completed", "failed"})
	st, _ := got[objects.FieldKeyStatus].(map[string]any)
	in, _ := st[FilterOpIn].([]string)
	if len(in) != 2 || in[0] != "completed" {
		t.Fatalf("%v", got)
	}
	same := WithStatusIn(base, nil)
	if _, ok := same[objects.FieldKeyStatus]; ok {
		t.Fatal("empty statuses should not add status")
	}
}

func TestMergeFiltersArchivedBefore(t *testing.T) {
	t.Parallel()
	cutoff := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	got := MergeFilters(StatusEq("archived"), CreatedAtBefore(cutoff))
	if _, ok := got[objects.FieldKeyStatus]; !ok {
		t.Fatalf("%v", got)
	}
	inner, _ := got[objects.FieldKeyCreatedAt].(map[string]any)
	if inner[FilterOpLt] != "2026-02-01T00:00:00Z" {
		t.Fatalf("%v", got)
	}
}

func TestCollectIDsAndExactWindow(t *testing.T) {
	t.Parallel()
	ids := CollectIDs([]map[string]any{
		{objects.FieldKeyID: "AUD-1"},
		{objects.FieldKeyID: ""},
		{objects.FieldKeyKind: "audit_event"},
	})
	if len(ids) != 1 || ids[0] != "AUD-1" {
		t.Fatalf("%v", ids)
	}
	win := ExactAggregationWindow("a", "b")
	inner, _ := win[objects.FieldKeyAggregationWindowStart].(map[string]any)
	if inner[FilterOpEq] != "a" {
		t.Fatalf("%v", win)
	}
	in := IDsIn([]string{"AUD-1"})
	idFilter, _ := in[objects.FieldKeyID].(map[string]any)
	got, _ := idFilter[FilterOpIn].([]string)
	if len(got) != 1 || got[0] != "AUD-1" {
		t.Fatalf("%v", in)
	}
}

func TestWindowStatusFilters(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	got := WindowStatusFilters(start, end)
	st, _ := got[objects.FieldKeyStatus].(map[string]any)
	in, _ := st[FilterOpIn].([]string)
	if len(in) != len(EligibleAggregationStatuses) {
		t.Fatalf("%v", got)
	}
}
