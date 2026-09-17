package audit

import (
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestWindowsOverlap(t *testing.T) {
	t.Parallel()
	a := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	b := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	c := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	if !WindowsOverlap(a, b, time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), c) {
		t.Fatal("expected overlap")
	}
	if WindowsOverlap(a, b, b, c) {
		t.Fatal("touching end should not overlap (Before)")
	}
}

func TestFirstOverlappingID(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	objs := []map[string]any{
		{
			objects.FieldKeyID:                     "skip",
			objects.FieldKeyAggregationWindowStart: "2026-02-01T00:00:00Z",
			objects.FieldKeyAggregationWindowEnd:   "2026-02-02T00:00:00Z",
		},
		{
			objects.FieldKeyID:                     "AAM-1",
			objects.FieldKeyAggregationWindowStart: "2026-01-01T12:00:00Z",
			objects.FieldKeyAggregationWindowEnd:   "2026-01-03T00:00:00Z",
		},
	}
	if got := FirstOverlappingID(objs, start, end); got != "AAM-1" {
		t.Fatalf("got %q", got)
	}
}
