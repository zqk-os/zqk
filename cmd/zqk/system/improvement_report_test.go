package system

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCadenceToCron(t *testing.T) {
	t.Parallel()

	if c := cadenceToCron("30m"); c != "*/30 * * * *" {
		t.Fatalf("expected '*/30 * * * *', got %q", c)
	}
	if c := cadenceToCron("2h"); c != "0 */2 * * *" {
		t.Fatalf("expected '0 */2 * * *', got %q", c)
	}
	if c := cadenceToCron("1h"); c != "0 * * * *" {
		t.Fatalf("expected '0 * * * *', got %q", c)
	}
	if c := cadenceToCron("unknown"); c != "0 * * * *" {
		t.Fatalf("expected '0 * * * *' for unknown cadence, got %q", c)
	}
}

func TestParseTimeRFC3339(t *testing.T) {
	t.Parallel()

	if _, ok := parseTimeRFC3339(""); ok {
		t.Fatal("expected ok=false for empty string")
	}
	if _, ok := parseTimeRFC3339("invalid-time"); ok {
		t.Fatal("expected ok=false for invalid string")
	}

	valid := "2026-10-13T12:00:00Z"
	parsed, ok := parseTimeRFC3339(valid)
	if !ok || parsed.Year() != 2026 || parsed.Month() != 10 || parsed.Day() != 13 {
		t.Fatalf("unexpected parsed time: %v, ok=%v", parsed, ok)
	}
}

func TestGetTotalObjectsFromCountMap(t *testing.T) {
	t.Parallel()

	if n := getTotalObjectsFromCountMap(nil); n != -1 {
		t.Fatalf("expected -1 for nil, got %d", n)
	}
	if n := getTotalObjectsFromCountMap(map[string]any{"other": 10}); n != -1 {
		t.Fatalf("expected -1 for missing key, got %d", n)
	}
	if n := getTotalObjectsFromCountMap(map[string]any{"total_objects": 42}); n != 42 {
		t.Fatalf("expected 42 for int, got %d", n)
	}
	if n := getTotalObjectsFromCountMap(map[string]any{"total_objects": float64(99)}); n != 99 {
		t.Fatalf("expected 99 for float64, got %d", n)
	}
	if n := getTotalObjectsFromCountMap(map[string]any{"total_objects": "invalid"}); n != -1 {
		t.Fatalf("expected -1 for string, got %d", n)
	}
}

func TestRetentionOverKinds(t *testing.T) {
	t.Parallel()

	if kinds := retentionOverKinds(nil); kinds != nil {
		t.Fatalf("expected nil for nil map, got %v", kinds)
	}

	ret := map[string]any{
		"over_target": []any{
			map[string]any{objects.FieldKeyKind: "audit_event"},
			map[string]any{objects.FieldKeyKind: "scheduler_job_run"},
			"not-a-map",
		},
	}
	kinds := retentionOverKinds(ret)
	if len(kinds) != 2 || kinds[0] != "audit_event" || kinds[1] != "scheduler_job_run" {
		t.Fatalf("unexpected retention over kinds: %v", kinds)
	}
}

func TestDeriveAdaptivePlan(t *testing.T) {
	t.Parallel()

	// 1. Nil previous -> standard mode
	plan := deriveAdaptivePlan(nil)
	if plan.Mode != "standard" || plan.RecommendedNextCadence != "1h" {
		t.Fatalf("unexpected default plan: %+v", plan)
	}

	// 2. High load signals -> lite mode
	highLoadPrev := &ImprovementReportOutput{
		Snapshot: ImprovementReportSnapshot{
			HealthData: map[string]any{
				"scheduler_pool_declined": int64(3),
			},
			MetricsAnalysis: &improvementMetricsSum{
				TimeoutCount: 12,
			},
		},
	}
	litePlan := deriveAdaptivePlan(highLoadPrev)
	if litePlan.Mode != "lite" || litePlan.RecommendedNextCadence != "2h" {
		t.Fatalf("expected lite mode with 2h cadence, got: %+v", litePlan)
	}
	if litePlan.CollectStatus || litePlan.CollectObjectCount {
		t.Fatalf("expected lite mode to skip heavy collection, got: %+v", litePlan)
	}

	// 3. Unstable signals -> focused mode
	unstablePrev := &ImprovementReportOutput{
		Snapshot: ImprovementReportSnapshot{
			HealthData: map[string]any{
				"check_blocking": 5,
			},
			MetricsAnalysis: &improvementMetricsSum{
				HighFailureCount: 2,
			},
			Retention: map[string]any{
				"over_target": []any{
					map[string]any{objects.FieldKeyKind: "audit_event"},
				},
			},
		},
	}
	focusedPlan := deriveAdaptivePlan(unstablePrev)
	if focusedPlan.Mode != "focused" || focusedPlan.RecommendedNextCadence != "30m" {
		t.Fatalf("expected focused mode with 30m cadence, got: %+v", focusedPlan)
	}
}

func TestBuildComparativeAndPrioritizeWorkItems(t *testing.T) {
	t.Parallel()

	previous := &ImprovementReportOutput{
		GeneratedAt: "2026-10-13T10:00:00Z",
		Snapshot: ImprovementReportSnapshot{
			ObjectCount: map[string]any{"total_objects": 500},
			HealthData:  map[string]any{"check_blocking": 1},
			Retention: map[string]any{
				"over_target": []any{
					map[string]any{objects.FieldKeyKind: "audit_event"},
				},
			},
		},
	}

	currentSnap := ImprovementReportSnapshot{
		GeneratedAt: "2026-10-13T11:00:00Z",
		ObjectCount: map[string]any{"total_objects": 750}, // +250 growth
		HealthData: map[string]any{
			"check_blocking":         3,
			"check_warnings":         10,
			"quarantine_total_files": 2,
		},
		Retention: map[string]any{
			"over_target": []any{
				map[string]any{objects.FieldKeyKind: "audit_event"},
				map[string]any{objects.FieldKeyKind: "scheduler_job_run"},
			},
		},
		MetricsAnalysis: &improvementMetricsSum{
			HighFailureCount: 1,
			TimeoutCount:     2,
			SlowCount:        4,
			ChurnCount:       1,
		},
	}

	comp := buildComparative(previous, currentSnap)
	if comp.PreviousAt != "2026-10-13T10:00:00Z" {
		t.Fatalf("unexpected PreviousAt: %s", comp.PreviousAt)
	}
	if comp.ObjectCountDelta != 250 {
		t.Fatalf("expected object count delta 250, got %d", comp.ObjectCountDelta)
	}
	if comp.RetentionOverDelta != 1 {
		t.Fatalf("expected retention over delta 1, got %d", comp.RetentionOverDelta)
	}
	if comp.BlockingDelta != 2 || !comp.NewBlocking {
		t.Fatalf("expected blocking delta 2 and new blocking, got %+v", comp)
	}

	workItems := prioritizeWorkItems(currentSnap, comp, previous)
	if len(workItems) == 0 {
		t.Fatal("expected prioritized work items, got none")
	}

	counts := countWorkItemsByPriority(workItems)
	if counts["P0"] == 0 {
		t.Error("expected at least one P0 item for blocking issues")
	}
	if counts["P1"] == 0 {
		t.Error("expected at least one P1 item for retention or failure rate")
	}
	if counts["P2"] == 0 {
		t.Error("expected at least one P2 item for warnings or growth")
	}
	if counts["P3"] == 0 {
		t.Error("expected at least one P3 item for churn or quarantine")
	}

	// Verify priority ordering: P0 before P1 before P2 before P3
	order := map[string]int{"P0": 0, "P1": 1, "P2": 2, "P3": 3}
	for i := 1; i < len(workItems); i++ {
		if order[workItems[i].Priority] < order[workItems[i-1].Priority] {
			t.Fatalf("work items not sorted by priority: %+v", workItems)
		}
	}
}

func TestImprovementReportFormattingAndMetadata(t *testing.T) {
	t.Parallel()

	currentSnap := ImprovementReportSnapshot{
		GeneratedAt: "2026-10-13T11:00:00Z",
		ObjectCount: map[string]any{"total_objects": 750},
	}
	comp := ImprovementReportComparative{
		PreviousAt:       "2026-10-13T10:00:00Z",
		ObjectCountDelta: 250,
	}
	workItems := []ImprovementWorkItem{
		{Priority: "P0", Title: "Fix blocking issue", Category: "behavior"},
	}

	summary := buildSummary(currentSnap, comp, workItems)
	if !strings.Contains(summary, "System improvement report") || !strings.Contains(summary, "Prioritized work items") {
		t.Fatalf("unexpected summary: %s", summary)
	}

	reportOut := ImprovementReportOutput{
		GeneratedAt: currentSnap.GeneratedAt,
		Summary:     summary,
		Snapshot:    currentSnap,
		Comparative: comp,
		WorkItems:   workItems,
	}

	txt := formatImprovementReportTxt(reportOut)
	if len(txt) == 0 || !strings.Contains(string(txt), "=== System Improvement Report ===") {
		t.Fatalf("unexpected formatted text: %s", string(txt))
	}

	metadata := compactSnapshotMetadata(reportOut)
	if metadata["work_items_total"] != len(workItems) {
		t.Fatalf("unexpected metadata total: %+v", metadata)
	}
}

func TestLoadPreviousImprovementReport(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Missing or empty directory
	if prev := loadPreviousImprovementReport(tmpDir); prev != nil {
		t.Fatalf("expected nil for empty dir, got %+v", prev)
	}

	// Create two report files
	older := ImprovementReportOutput{GeneratedAt: "2026-10-13T08:00:00Z"}
	newer := ImprovementReportOutput{GeneratedAt: "2026-10-13T09:00:00Z"}

	olderBytes, err := json.Marshal(older)
	if err != nil {
		t.Fatal(err)
	}
	newerBytes, err := json.Marshal(newer)
	if err != nil {
		t.Fatal(err)
	}

	if err := fileutil.WriteFile(filepath.Join(tmpDir, "improvement-report-20261013-080000.json"), olderBytes, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(tmpDir, "improvement-report-20261013-090000.json"), newerBytes, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	loaded := loadPreviousImprovementReport(tmpDir)
	if loaded == nil {
		t.Fatal("expected loaded previous report, got nil")
	}
	if loaded.GeneratedAt != "2026-10-13T09:00:00Z" {
		t.Fatalf("expected newest report (09:00:00Z), got %s", loaded.GeneratedAt)
	}
}
