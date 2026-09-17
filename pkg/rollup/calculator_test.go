package rollup

import (
	"math"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestCalculate_Empty(t *testing.T) {
	summary := Calculate(nil)
	if summary.TotalCount != 0 || summary.CompletedCount != 0 || summary.PercentComplete != 0 {
		t.Fatalf("expected all zeros for empty children; got %+v", summary)
	}
	if summary.EstimatedEffort.TotalHours != 0 || summary.ActualEffort.TotalHours != 0 {
		t.Fatalf("expected 0 effort hours; got est=%v act=%v", summary.EstimatedEffort.TotalHours, summary.ActualEffort.TotalHours)
	}
	if summary.EstimatedEffort.Formatted != "0h" || summary.ActualEffort.Formatted != "0h" {
		t.Fatalf("expected '0h' strings; got est=%q act=%q", summary.EstimatedEffort.Formatted, summary.ActualEffort.Formatted)
	}
}

func TestCalculate_MixedStatusesAndEfforts(t *testing.T) {
	children := []map[string]any{
		{
			objects.FieldKeyID:              "BLI-001",
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyStatus:          objects.ObjectStatusComplete,
			objects.FieldKeyEstimatedEffort: "2h",
			objects.FieldKeyActualEffort:    "1.5h",
		},
		{
			objects.FieldKeyID:              "BLI-002",
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
			objects.FieldKeyEstimatedEffort: "4h",
			objects.FieldKeyActualEffort:    "30m", // 0.5h
		},
		{
			objects.FieldKeyID:              "GOAL-003",
			objects.FieldKeyKind:            objects.KindGoal,
			objects.FieldKeyStatus:          objects.ObjectStatusBlocked,
			objects.FieldKeyEstimatedEffort: "1d", // 24h
			objects.FieldKeyActualEffort:    "unspecified", // unparsable -> ignored
		},
		{
			objects.FieldKeyID:              "BLI-004",
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyStatus:          "exploring", // open / not started
			objects.FieldKeyEstimatedEffort: "6h",
			objects.FieldKeyActualEffort:    "",
		},
	}

	summary := Calculate(children)

	// Counts
	if summary.TotalCount != 4 {
		t.Fatalf("total_count=%d want 4", summary.TotalCount)
	}
	if summary.CompletedCount != 1 {
		t.Fatalf("completed_count=%d want 1", summary.CompletedCount)
	}
	if summary.InProgressCount != 1 {
		t.Fatalf("in_progress_count=%d want 1", summary.InProgressCount)
	}
	if summary.HaltedCount != 1 {
		t.Fatalf("halted_count=%d want 1", summary.HaltedCount)
	}
	if summary.OpenCount != 3 {
		t.Fatalf("open_count=%d want 3", summary.OpenCount)
	}
	if math.Abs(summary.PercentComplete-25.0) > 1e-6 {
		t.Fatalf("percent_complete=%v want 25.0", summary.PercentComplete)
	}

	// Estimated effort: 2h + 4h + 24h + 6h = 36h
	// values: [2, 4, 24, 6] -> min=2, max=24, avg=36/4 = 9
	if math.Abs(summary.EstimatedEffort.TotalHours-36.0) > 1e-6 {
		t.Fatalf("estimated_effort.total_hours=%v want 36.0", summary.EstimatedEffort.TotalHours)
	}
	if math.Abs(summary.EstimatedEffort.MinHours-2.0) > 1e-6 {
		t.Fatalf("estimated_effort.min_hours=%v want 2.0", summary.EstimatedEffort.MinHours)
	}
	if math.Abs(summary.EstimatedEffort.MaxHours-24.0) > 1e-6 {
		t.Fatalf("estimated_effort.max_hours=%v want 24.0", summary.EstimatedEffort.MaxHours)
	}
	if math.Abs(summary.EstimatedEffort.AvgHours-9.0) > 1e-6 {
		t.Fatalf("estimated_effort.avg_hours=%v want 9.0", summary.EstimatedEffort.AvgHours)
	}
	if summary.EstimatedEffort.Formatted != "36h" {
		t.Fatalf("estimated_effort.formatted=%q want '36h'", summary.EstimatedEffort.Formatted)
	}

	// Actual effort: 1.5h + 0.5h = 2h
	// values: [1.5, 0.5] -> min=0.5, max=1.5, avg=1.0
	if math.Abs(summary.ActualEffort.TotalHours-2.0) > 1e-6 {
		t.Fatalf("actual_effort.total_hours=%v want 2.0", summary.ActualEffort.TotalHours)
	}
	if math.Abs(summary.ActualEffort.MinHours-0.5) > 1e-6 {
		t.Fatalf("actual_effort.min_hours=%v want 0.5", summary.ActualEffort.MinHours)
	}
	if math.Abs(summary.ActualEffort.MaxHours-1.5) > 1e-6 {
		t.Fatalf("actual_effort.max_hours=%v want 1.5", summary.ActualEffort.MaxHours)
	}
	if math.Abs(summary.ActualEffort.AvgHours-1.0) > 1e-6 {
		t.Fatalf("actual_effort.avg_hours=%v want 1.0", summary.ActualEffort.AvgHours)
	}
	if summary.ActualEffort.Formatted != "2h" {
		t.Fatalf("actual_effort.formatted=%q want '2h'", summary.ActualEffort.Formatted)
	}

	// Test ToMap
	m := summary.ToMap()
	estMap, ok := m["estimated_effort"].(map[string]any)
	if !ok || estMap["formatted"] != "36h" || estMap["total_hours"] != 36.0 {
		t.Fatalf("ToMap conversion mismatch: %+v", m)
	}
}
