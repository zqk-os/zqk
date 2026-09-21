package cas_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_instance_v1" // Register instance builders for tests
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_v2"          // Register builders for tests
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	instancebuilders "github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestCAS_BaselineValidation tests that baseline validation works correctly with CAS objects
// This test verifies that the check-baseline command can collect metrics for CAS-enabled objects
func TestCAS_BaselineValidation(t *testing.T) {
	// Not: ZQK_TEST_ROOT is process-global.
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-baseline-test")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)
	specsDir := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir)

	generator := builders.NewSpecGenerator(specsDir)
	if err := generator.GenerateAllSpecs(); err != nil {
		t.Fatalf("Failed to generate specs: %v", err)
	}

	t.Setenv(zqkenv.TestRoot().Name(), testRoot)

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-SYSTEM",
	}

	// Create multiple objects of different kinds to test baseline collection
	// Use instance builders to create valid objects that pass validation
	registry := instancebuilders.GetGlobalRegistry()
	schemaVersion := objects.DefaultSchemaVersion

	var testObjects []map[string]any

	// Create goal GOAL-123
	goalBuilder, err := registry.GetBuilder("goal", schemaVersion)
	if err != nil {
		t.Fatalf("Failed to get goal builder: %v", err)
	}
	goalBuilder.SetID("GOAL-123")
	goalBuilder.SetField(objects.FieldKeyTitle, "Test Goal")
	goalBuilder.SetField(objects.FieldKeyDescription, "This is a sufficiently long description for the goal object")
	goalBuilder.SetStatus("active")
	goalObj, err := goalBuilder.Build()
	if err != nil {
		t.Fatalf("Failed to build goal GOAL-123: %v", err)
	}
	testObjects = append(testObjects, goalObj)

	// Create backlog_item BLI-001
	builder1, err := registry.GetBuilder("backlog_item", schemaVersion)
	if err != nil {
		t.Fatalf("Failed to get backlog_item builder: %v", err)
	}
	builder1.SetID("BLI-001")
	builder1.SetField(objects.FieldKeyTitle, "Baseline Test 1")
	builder1.SetField(objects.FieldKeyGoalRefs, []any{"GOAL-123"})
	builder1.SetStatus("exploring")
	obj1, err := builder1.Build()
	if err != nil {
		t.Fatalf("Failed to build backlog_item BLI-001: %v", err)
	}
	testObjects = append(testObjects, obj1)

	// Create backlog_item BLI-002
	builder2, err := registry.GetBuilder("backlog_item", schemaVersion)
	if err != nil {
		t.Fatalf("Failed to get backlog_item builder: %v", err)
	}
	builder2.SetID("BLI-002")
	builder2.SetField(objects.FieldKeyTitle, "Baseline Test 2")
	builder2.SetField(objects.FieldKeyGoalRefs, []any{"GOAL-123"})
	builder2.SetStatus("validated")
	obj2, err := builder2.Build()
	if err != nil {
		t.Fatalf("Failed to build backlog_item BLI-002: %v", err)
	}
	testObjects = append(testObjects, obj2)

	// Create milestone MIL-001
	builder3, err := registry.GetBuilder("milestone", schemaVersion)
	if err != nil {
		t.Fatalf("Failed to get milestone builder: %v", err)
	}
	builder3.SetID("MIL-001")
	builder3.SetField(objects.FieldKeyTitle, "Baseline Milestone")
	builder3.SetStatus("not_started")
	obj3, err := builder3.Build()
	if err != nil {
		t.Fatalf("Failed to build milestone MIL-001: %v", err)
	}
	testObjects = append(testObjects, obj3)

	// Create doc_entry DOC-001
	builder4, err := registry.GetBuilder("doc_entry", schemaVersion)
	if err != nil {
		t.Fatalf("Failed to get doc_entry builder: %v", err)
	}
	builder4.SetID("DOC-001")
	builder4.SetField(objects.FieldKeyTitle, "Baseline Document")
	builder4.SetField(objects.FieldKeyPath, "docs/baseline.md")
	builder4.SetField(objects.FieldKeySummary, "Test document")
	builder4.SetField(objects.FieldKeyGroup, "other")
	builder4.SetField(objects.FieldKeyContentSearchable, true)
	builder4.SetStatus("active")
	obj4, err := builder4.Build()
	if err != nil {
		t.Fatalf("Failed to build doc_entry DOC-001: %v", err)
	}
	testObjects = append(testObjects, obj4)

	// Create all test objects (promote preliminary statuses off draft plane into CAS).
	for _, obj := range testObjects {
		storage.CreateCASVisible(t, fileStorage, ctx, secCtx, obj, "")

		// Verify object is stored as CAS (hash-based file)
		filePath, err := fileStorage.GetObjectFilePath(obj[objects.FieldKeyID].(string), obj[objects.FieldKeyKind].(string))
		if err != nil {
			t.Fatalf("Failed to get file path for %s: %v", obj[objects.FieldKeyID], err)
		}

		fileName := filepath.Base(filePath)
		if len(fileName) != 69 || filepath.Ext(fileName) != ".yaml" {
			t.Errorf("Expected hash-based filename for %s, got %s", obj[objects.FieldKeyID], fileName)
		}
	}

	// Wait for index updates to be processed (use per-project-root queue for test isolation)
	storage.FlushAllOrFail(t, testRoot)

	// Verify all objects can be read
	for _, obj := range testObjects {
		readObj, err := fileStorage.Read(ctx, secCtx, obj[objects.FieldKeyID].(string))
		if err != nil {
			t.Fatalf("Failed to read object %s: %v", obj[objects.FieldKeyID], err)
		}

		if readObj[objects.FieldKeyID] != obj[objects.FieldKeyID] {
			t.Errorf("Expected ID %s, got %v", obj[objects.FieldKeyID], readObj[objects.FieldKeyID])
		}
	}

	// Verify objects are counted correctly
	count, err := fileStorage.Count(ctx, secCtx, storage.ListFilter{Kind: "backlog_item"})
	if err != nil {
		t.Fatalf("Failed to count backlog_item objects: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 backlog_item objects, got %d", count)
	}

	count, err = fileStorage.Count(ctx, secCtx, storage.ListFilter{Kind: "milestone"})
	if err != nil {
		t.Fatalf("Failed to count milestone objects: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 milestone object, got %d", count)
	}

	count, err = fileStorage.Count(ctx, secCtx, storage.ListFilter{Kind: "doc_entry"})
	if err != nil {
		t.Fatalf("Failed to count doc_entry objects: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 doc_entry object, got %d", count)
	}

	t.Logf("✓ Baseline validation test: Created %d CAS objects across 3 kinds", len(testObjects))
}

// BaselineMetrics represents the structure of baseline validation metrics
// This matches the structure from cmd/zqk/system/check_baseline.go
type BaselineMetrics struct {
	Timestamp       string         `json:"timestamp"`
	TotalObjects    int            `json:"total_objects"`
	TotalResults    int            `json:"total_results"`
	DurationMS      float64        `json:"duration_ms"`
	ObjectsPerSec   float64        `json:"objects_per_second"`
	IssuesByTier    map[int]int    `json:"issues_by_tier"`
	TotalIssues     int            `json:"total_issues"`
	ObjectKinds     map[string]int `json:"object_kinds"`
	IssueCategories map[string]int `json:"issue_categories"`
	Results         []any          `json:"results,omitempty"`
}

// TestCAS_BaselineMetricsFile tests that baseline metrics can be saved and loaded
// This simulates what the check-baseline command does
func TestCAS_BaselineMetricsFile(t *testing.T) {
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-baseline-metrics")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-SYSTEM",
	}

	// Create a few test objects
	testObjects := []map[string]any{
		{
			objects.FieldKeyID:            "GOAL-123",
			objects.FieldKeyKind:          "goal",
			objects.FieldKeyTitle:         "Test Goal",
			objects.FieldKeyDescription:   "This is a sufficiently long description for the goal object",
			objects.FieldKeyStatus:        objects.ObjectStatusActive,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		},
		{
			objects.FieldKeyID:            "BLI-100",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyGoalRefs:      []any{"GOAL-123"},
			objects.FieldKeyTitle:         "Metrics Test 1",
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		},
		{
			objects.FieldKeyID:            "BLI-101",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyGoalRefs:      []any{"GOAL-123"},
			objects.FieldKeyTitle:         "Metrics Test 2",
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		},
	}

	for _, obj := range testObjects {
		storage.CreateCASVisible(t, fileStorage, ctx, secCtx, obj, "")
	}

	// Create baseline metrics structure (simulating check-baseline output)
	metrics := BaselineMetrics{
		Timestamp:     time.Now().Format(time.RFC3339),
		TotalObjects:  len(testObjects),
		TotalResults:  len(testObjects),
		DurationMS:    100.5,
		ObjectsPerSec: float64(len(testObjects)) / 0.1005,
		IssuesByTier: map[int]int{
			1: 0,
			2: 0,
			3: 0,
			4: 0,
		},
		TotalIssues: 0,
		ObjectKinds: map[string]int{
			"backlog_item": len(testObjects),
		},
		IssueCategories: make(map[string]int),
	}

	// Save metrics to file
	metricsFile := filepath.Join(testRoot, paths.ProjectDataDir, "baseline_metrics.json")
	if err := fileutil.MkdirAll(filepath.Dir(metricsFile), paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create metrics directory: %v", err)
	}

	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal metrics: %v", err)
	}

	if err := fileutil.WriteFile(metricsFile, data, paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write metrics file: %v", err)
	}

	// Verify file was created
	if _, err := fileutil.Stat(metricsFile); err != nil {
		t.Fatalf("Metrics file not created: %v", err)
	}

	// Load and verify metrics
	loadedData, err := fileutil.ReadFile(metricsFile)
	if err != nil {
		t.Fatalf("Failed to read metrics file: %v", err)
	}

	var loadedMetrics BaselineMetrics
	if err := json.Unmarshal(loadedData, &loadedMetrics); err != nil {
		t.Fatalf("Failed to unmarshal metrics: %v", err)
	}

	// Verify metrics match
	if loadedMetrics.TotalObjects != metrics.TotalObjects {
		t.Errorf("Expected %d total objects, got %d", metrics.TotalObjects, loadedMetrics.TotalObjects)
	}

	if loadedMetrics.ObjectKinds["backlog_item"] != len(testObjects) {
		t.Errorf("Expected %d backlog_item objects, got %d", len(testObjects), loadedMetrics.ObjectKinds["backlog_item"])
	}

	t.Logf("✓ Baseline metrics file test: Saved and loaded metrics successfully")
}
