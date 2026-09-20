package scheduler

import (
	"testing"
	"time"
)

func TestFormatActivityCompactUTC(t *testing.T) {
	t.Parallel()
	ref := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	t.Run("same_year_omits_year", func(t *testing.T) {
		t.Parallel()
		ts := time.Date(2026, 3, 9, 14, 30, 0, 0, time.UTC)
		if got := formatActivityCompactUTC(ts, ref); got != "03-09 14:30" {
			t.Fatalf("got %q want 03-09 14:30", got)
		}
	})
	t.Run("different_year_includes_year", func(t *testing.T) {
		t.Parallel()
		ts := time.Date(2025, 12, 31, 23, 0, 0, 0, time.UTC)
		if got := formatActivityCompactUTC(ts, ref); got != "2025-12-31 23:00" {
			t.Fatalf("got %q want 2025-12-31 23:00", got)
		}
	})
}

func TestStuckJobAssessmentParts(t *testing.T) {
	t.Parallel()
	recent := 2 * time.Minute
	stale := 30 * time.Minute
	t.Run("REC", func(t *testing.T) {
		t.Parallel()
		code, detail := stuckJobAssessmentParts(30*time.Second, recent, stale)
		if code != stuckAssessmentREC || detail == "" {
			t.Fatalf("code=%q detail=%q", code, detail)
		}
	})
	t.Run("WCH", func(t *testing.T) {
		t.Parallel()
		code, _ := stuckJobAssessmentParts(10*time.Minute, recent, stale)
		if code != stuckAssessmentWCH {
			t.Fatalf("want %s got %s", stuckAssessmentWCH, code)
		}
	})
	t.Run("INV", func(t *testing.T) {
		t.Parallel()
		code, _ := stuckJobAssessmentParts(45*time.Minute, recent, stale)
		if code != stuckAssessmentINV {
			t.Fatalf("want %s got %s", stuckAssessmentINV, code)
		}
	})
}
