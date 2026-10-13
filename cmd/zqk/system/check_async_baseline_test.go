package system

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

func TestConvertValidationIssues(t *testing.T) {
	t.Parallel()

	input := []validation.ValidationIssue{
		{
			Tier:        1,
			Category:    "lifecycle",
			Message:     "missing criteria_refs",
			AutoFixable: true,
		},
		{
			Tier:        2,
			Category:    "metadata",
			Message:     "description too short",
			AutoFixable: false,
		},
	}

	converted := convertValidationIssues(input)
	if len(converted) != 2 {
		t.Fatalf("expected 2 converted issues, got %d", len(converted))
	}
	if converted[0].Tier != 1 || converted[0].Category != "lifecycle" || !converted[0].AutoFixable {
		t.Fatalf("unexpected issue 0: %+v", converted[0])
	}
	if converted[1].Tier != 2 || converted[1].Category != "metadata" || converted[1].AutoFixable {
		t.Fatalf("unexpected issue 1: %+v", converted[1])
	}
}

func TestCollectAsyncBaselineMetrics(t *testing.T) {
	t.Parallel()

	results := []CheckResult{
		{
			ObjectID:   "OBJ-1",
			ObjectKind: "goal",
			Issues: []Issue{
				{Tier: 1, Category: "lifecycle", Message: "err1"},
			},
		},
		{
			ObjectID:   "OBJ-2",
			ObjectKind: "milestone",
			Issues:     nil,
		},
	}

	duration := 100 * time.Millisecond
	metrics := collectAsyncBaselineMetrics(results, duration, 4, true)

	if metrics.TotalResults != 2 {
		t.Fatalf("expected 2 total results, got %d", metrics.TotalResults)
	}
	if metrics.Workers != 4 {
		t.Fatalf("expected 4 workers, got %d", metrics.Workers)
	}
	if metrics.TotalIssues != 1 {
		t.Fatalf("expected 1 total issue, got %d", metrics.TotalIssues)
	}
	if len(metrics.Results) != 2 {
		t.Fatalf("expected results included, got %d", len(metrics.Results))
	}

	// Without includeResults
	metricsNoResults := collectAsyncBaselineMetrics(results, duration, 2, false)
	if len(metricsNoResults.Results) != 0 {
		t.Fatalf("expected no results included, got %d", len(metricsNoResults.Results))
	}
}

func TestCompareBaselines(t *testing.T) {
	t.Parallel()

	baseline := &BaselineMetrics{
		TotalObjects:  100,
		TotalIssues:   10,
		Duration:      200 * time.Millisecond,
		ObjectsPerSec: 500.0,
		IssuesByTier:  map[int]int{1: 4, 2: 6},
		IssueCategories: map[string]int{
			"lifecycle": 4,
			"metadata":  6,
		},
	}

	// Case 1: Exact match with 2x speedup
	asyncExact := &AsyncBaselineMetrics{
		TotalObjects:  100,
		TotalIssues:   10,
		Duration:      100 * time.Millisecond,
		ObjectsPerSec: 1000.0,
		IssuesByTier:  map[int]int{1: 4, 2: 6},
		IssueCategories: map[string]int{
			"lifecycle": 4,
			"metadata":  6,
		},
	}

	compExact := compareBaselines(baseline, asyncExact)
	if !compExact.Correctness.ObjectsMatch || !compExact.Correctness.IssuesMatch ||
		!compExact.Correctness.IssueTierMatch || !compExact.Correctness.IssueCategoryMatch {
		t.Fatalf("expected full match, got: %+v", compExact.Correctness)
	}
	if diff := compExact.Correctness.CorrectnessScore - 1.0; diff < -1e-6 || diff > 1e-6 {
		t.Fatalf("expected ~1.0 correctness score, got %f", compExact.Correctness.CorrectnessScore)
	}
	if compExact.Improvement.Speedup != 2.0 {
		t.Fatalf("expected 2.0x speedup, got %f", compExact.Improvement.Speedup)
	}

	// Case 2: Partial mismatch
	asyncMismatched := &AsyncBaselineMetrics{
		TotalObjects:  90,
		TotalIssues:   12,
		Duration:      400 * time.Millisecond,
		ObjectsPerSec: 225.0,
		IssuesByTier:  map[int]int{1: 5, 2: 7},
		IssueCategories: map[string]int{
			"lifecycle": 5,
			"metadata":  7,
		},
	}

	compMismatch := compareBaselines(baseline, asyncMismatched)
	if compMismatch.Correctness.ObjectsMatch || compMismatch.Correctness.IssuesMatch {
		t.Fatalf("expected mismatch, got: %+v", compMismatch.Correctness)
	}
	if compMismatch.Correctness.ObjectCountDiff != -10 {
		t.Fatalf("expected object count diff -10, got %d", compMismatch.Correctness.ObjectCountDiff)
	}
	if compMismatch.Correctness.IssueCountDiff != 2 {
		t.Fatalf("expected issue count diff 2, got %d", compMismatch.Correctness.IssueCountDiff)
	}
	if compMismatch.Improvement.Speedup != 0.5 {
		t.Fatalf("expected 0.5x speedup, got %f", compMismatch.Improvement.Speedup)
	}
}

func TestLoadAndSaveComparisonResult(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	baselinePath := filepath.Join(tmpDir, "baseline.json")

	rawJSON := `{
		"timestamp": "2026-10-13T12:00:00Z",
		"total_objects": 50,
		"total_results": 50,
		"duration_ms": 250.5,
		"objects_per_second": 200.0,
		"issues_by_tier": {"1": 2, "2": 3},
		"total_issues": 5,
		"object_kinds": {"goal": 50},
		"issue_categories": {"lifecycle": 5}
	}`

	if err := fileutil.WriteFile(baselinePath, []byte(rawJSON), 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := loadBaselineMetrics(baselinePath)
	if err != nil {
		t.Fatalf("failed to load baseline metrics: %v", err)
	}
	if loaded.TotalObjects != 50 || loaded.TotalIssues != 5 || loaded.IssuesByTier[1] != 2 {
		t.Fatalf("unexpected loaded baseline: %+v", loaded)
	}

	// Test non-existent file
	_, err = loadBaselineMetrics(filepath.Join(tmpDir, "non_existent.json"))
	if err == nil {
		t.Fatal("expected error for non-existent baseline file, got nil")
	}

	// Test saveComparisonResult
	comp := ComparisonResult{
		Baseline: loaded,
		Async: AsyncBaselineMetrics{
			Timestamp:    time.Now(),
			TotalObjects: 50,
			TotalIssues:  5,
			Duration:     125 * time.Millisecond,
		},
		Improvement: ComparisonImprovement{
			Speedup: 2.0,
		},
		Correctness: CorrectnessCheck{
			ObjectsMatch:     true,
			CorrectnessScore: 1.0,
		},
	}

	outputPath := filepath.Join(tmpDir, "comparison.json")
	if err := saveComparisonResult(outputPath, &comp); err != nil {
		t.Fatalf("failed to save comparison result: %v", err)
	}

	savedData, err := fileutil.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read saved comparison: %v", err)
	}
	var parsedOutput map[string]any
	if err := json.Unmarshal(savedData, &parsedOutput); err != nil {
		t.Fatalf("failed to unmarshal saved comparison JSON: %v", err)
	}
	if _, ok := parsedOutput["baseline"]; !ok {
		t.Fatal("missing baseline in saved comparison")
	}
	if _, ok := parsedOutput["improvement"]; !ok {
		t.Fatal("missing improvement in saved comparison")
	}
}

func TestNewCheckAsyncBaselineCmd(t *testing.T) {
	t.Parallel()

	cmd := NewCheckAsyncBaselineCmd()
	if cmd == nil {
		t.Fatal("NewCheckAsyncBaselineCmd() returned nil")
	}
	if cmd.Use != "check-async-baseline" {
		t.Fatalf("expected Use 'check-async-baseline', got %q", cmd.Use)
	}
	if cmd.Flags().Lookup("baseline-file") == nil {
		t.Error("missing --baseline-file flag")
	}
	if cmd.Flags().Lookup("comparison-output") == nil {
		t.Error("missing --comparison-output flag")
	}
	if cmd.Flags().Lookup("workers") == nil {
		t.Error("missing --workers flag")
	}

	// Test outputComparison helper
	baseline := &BaselineMetrics{Duration: 100 * time.Millisecond, ObjectsPerSec: 10}
	async := &AsyncBaselineMetrics{Duration: 50 * time.Millisecond, ObjectsPerSec: 20}
	comp := &ComparisonResult{
		Improvement: ComparisonImprovement{Speedup: 2.0},
		Correctness: CorrectnessCheck{CorrectnessScore: 1.0},
	}
	cmd.SetOut(os.Stdout)
	if err := outputComparison(cmd, baseline, async, comp); err != nil {
		t.Fatalf("outputComparison failed: %v", err)
	}
}
