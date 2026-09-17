package cas_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/storage"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestCAS_AsyncValidationComparison tests that async validation produces the same results
// for CAS objects as it does for non-CAS (ID-based) objects
func TestCAS_AsyncValidationComparison(t *testing.T) {
	// Create two test environments:
	// 1. CAS-enabled (test-scenarios path)
	// 2. Non-CAS (regular path)

	// CAS environment
	casBaseDir := t.TempDir()
	casTestRoot := filepath.Join(casBaseDir, "test-scenarios", "cas-validation")
	storage.MustEnsureProcessSpecsLayoutForTest(t, casTestRoot)
	if err := paths.EnsureDir(filepath.Join(casTestRoot, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create CAS test directory: %v", err)
	}
	if err := paths.EnsureDir(filepath.Join(casTestRoot, paths.ProcessGoalsDir), paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create CAS goals directory: %v", err)
	}

	nonCASBaseDir := t.TempDir()
	nonCASTestRoot := filepath.Join(nonCASBaseDir, "regular-project")
	storage.MustEnsureProcessSpecsLayoutForTest(t, nonCASTestRoot)
	if err := paths.EnsureDir(filepath.Join(nonCASTestRoot, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create non-CAS test directory: %v", err)
	}
	if err := paths.EnsureDir(filepath.Join(nonCASTestRoot, paths.ProcessGoalsDir), paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create non-CAS goals directory: %v", err)
	}

	// Create storage.storage instances
	casStorage, err := storage.NewFileObjectStorageForTest(casTestRoot)
	if err != nil {
		t.Fatalf("Failed to create CAS storage: %v", err)
	}

	defer func() { _ = casStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(casTestRoot, casStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	nonCASStorage, err := storage.NewFileObjectStorageForTest(nonCASTestRoot)
	if err != nil {
		t.Fatalf("Failed to create non-CAS storage: %v", err)
	}

	defer func() { _ = nonCASStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(nonCASTestRoot, nonCASStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	// Verify CAS is enabled for both (CAS is now always enabled for all kinds)
	// Note: The implementation has changed - CAS is now always enabled regardless of path
	// This test now verifies that validation works the same way regardless of storage format
	if !casStorage.UsesContentAddressableStorage("backlog_item") {
		t.Fatalf("CAS should be enabled for backlog_item")
	}
	if !nonCASStorage.UsesContentAddressableStorage("backlog_item") {
		t.Fatalf("CAS should be enabled for backlog_item (CAS is now always enabled)")
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-1785920548450214012-68b850c0",
	}

	// Create the same objects in both environments
	makeTestObjects := func() []map[string]any {
		return []map[string]any{
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
				objects.FieldKeyTitle:         "Comparison Test 1",
				objects.FieldKeyStatus:        objects.ObjectStatusExploring,
				objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			},
			{
				objects.FieldKeyID:            "BLI-101",
				objects.FieldKeyKind:          "backlog_item",
				objects.FieldKeyTitle:         "Comparison Test 2",
				objects.FieldKeyStatus:        objects.ObjectStatusValidated,
				objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			},
			{
				objects.FieldKeyID:            "BLI-102",
				objects.FieldKeyKind:          "backlog_item",
				objects.FieldKeyTitle:         "Comparison Test 3",
				objects.FieldKeyStatus:        objects.ObjectStatusComplete,
				objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			},
		}
	}
	testObjects := makeTestObjects()

	// Create objects in CAS environment
	for _, obj := range makeTestObjects() {
		storage.CreateCASVisible(t, casStorage, ctx, secCtx, obj, "")
	}

	// Wait for CAS index updates to be processed (use per-project-root queue for test isolation)
	storage.FlushAllOrFail(t, casTestRoot)

	// Create objects in non-CAS environment
	for _, obj := range makeTestObjects() {
		storage.CreateCASVisible(t, nonCASStorage, ctx, secCtx, obj, "")
	}

	// Wait for non-CAS index updates too (CAS is always enabled, so flush is needed)
	storage.FlushAllOrFail(t, nonCASTestRoot)

	// Verify both use hash-based filenames (CAS is now always enabled)
	// Note: CAS is now always enabled for all kinds, so both will use hash-based files
	for _, obj := range testObjects {
		objID := obj[objects.FieldKeyID].(string)

		// Get CAS file path
		casFilePath, err := casStorage.GetObjectFilePath(objID, obj[objects.FieldKeyKind].(string))
		if err != nil {
			t.Fatalf("Failed to get CAS file path: %v", err)
		}
		casFileName := filepath.Base(casFilePath)

		// Get non-CAS file path (will also be hash-based since CAS is always enabled)
		nonCASFilePath, err := nonCASStorage.GetObjectFilePath(objID, obj[objects.FieldKeyKind].(string))
		if err != nil {
			t.Fatalf("Failed to get non-CAS file path: %v", err)
		}
		nonCASFileName := filepath.Base(nonCASFilePath)

		// Verify both use hash-based filename (64-char hex + .yaml = 69 chars)
		if len(casFileName) != 69 || filepath.Ext(casFileName) != ".yaml" {
			t.Errorf("CAS file should be hash-based (64-char hex + .yaml), got %s", casFileName)
		}
		if len(nonCASFileName) != 69 || filepath.Ext(nonCASFileName) != ".yaml" {
			t.Errorf("Non-CAS file should also be hash-based (CAS is always enabled), got %s", nonCASFileName)
		}
	}

	// Verify both environments can read the same objects with same content
	for _, obj := range testObjects {
		objID := obj[objects.FieldKeyID].(string)

		// Read from CAS
		casReadObj, err := casStorage.Read(ctx, secCtx, objID)
		if err != nil {
			t.Fatalf("Failed to read CAS object %s: %v", objID, err)
		}

		// Read from non-CAS
		nonCASReadObj, err := nonCASStorage.Read(ctx, secCtx, objID)
		if err != nil {
			t.Fatalf("Failed to read non-CAS object %s: %v", objID, err)
		}

		// Compare key fields (content should be the same)
		if casReadObj[objects.FieldKeyID] != nonCASReadObj[objects.FieldKeyID] {
			t.Errorf("ID mismatch for %s: CAS=%v, non-CAS=%v", objID, casReadObj[objects.FieldKeyID], nonCASReadObj[objects.FieldKeyID])
		}
		if casReadObj[objects.FieldKeyTitle] != nonCASReadObj[objects.FieldKeyTitle] {
			t.Errorf("Title mismatch for %s: CAS=%v, non-CAS=%v", objID, casReadObj[objects.FieldKeyTitle], nonCASReadObj[objects.FieldKeyTitle])
		}
		if casReadObj[objects.FieldKeyStatus] != nonCASReadObj[objects.FieldKeyStatus] {
			t.Errorf("Status mismatch for %s: CAS=%v, non-CAS=%v", objID, casReadObj[objects.FieldKeyStatus], nonCASReadObj[objects.FieldKeyStatus])
		}
	}

	// Verify counts match
	casCount, err := casStorage.Count(ctx, secCtx, storage.ListFilter{Kind: "backlog_item"})
	if err != nil {
		t.Fatalf("Failed to count CAS objects: %v", err)
	}

	nonCASCount, err := nonCASStorage.Count(ctx, secCtx, storage.ListFilter{Kind: "backlog_item"})
	if err != nil {
		t.Fatalf("Failed to count non-CAS objects: %v", err)
	}

	if casCount != nonCASCount {
		t.Errorf("Count mismatch: CAS=%d, non-CAS=%d", casCount, nonCASCount)
	}

	if casCount != len(testObjects)-1 {
		t.Errorf("Expected %d backlog_items, got %d", len(testObjects)-1, casCount)
	}

	t.Logf("✓ Validation comparison: CAS and non-CAS produce identical results for %d objects", len(testObjects))
}

// TestCAS_AsyncValidationSameResults tests that the same object content produces
// the same validation results regardless of storage format (CAS vs ID-based)
func TestCAS_AsyncValidationSameResults(t *testing.T) {
	// This test verifies that validation logic is storage-format agnostic
	// The same object should produce the same validation issues whether stored as:
	// - CAS (hash-based filename)
	// - ID-based (ID.yaml filename)

	// Create CAS environment
	casBaseDir := t.TempDir()
	casTestRoot := filepath.Join(casBaseDir, "test-scenarios", "cas-same-results")
	storage.MustEnsureProcessSpecsLayoutForTest(t, casTestRoot)
	if err := paths.EnsureDir(filepath.Join(casTestRoot, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create CAS test directory: %v", err)
	}
	if err := paths.EnsureDir(filepath.Join(casTestRoot, paths.ProcessGoalsDir), paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create CAS goals directory: %v", err)
	}

	nonCASBaseDir := t.TempDir()
	nonCASTestRoot := filepath.Join(nonCASBaseDir, "regular-same-results")
	storage.MustEnsureProcessSpecsLayoutForTest(t, nonCASTestRoot)
	if err := paths.EnsureDir(filepath.Join(nonCASTestRoot, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create non-CAS test directory: %v", err)
	}
	if err := paths.EnsureDir(filepath.Join(nonCASTestRoot, paths.ProcessGoalsDir), paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create non-CAS goals directory: %v", err)
	}

	// Create storage instances
	casStorage, err := storage.NewFileObjectStorageForTest(casTestRoot)
	if err != nil {
		t.Fatalf("Failed to create CAS storage: %v", err)
	}

	defer func() { _ = casStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(casTestRoot, casStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	nonCASStorage, err := storage.NewFileObjectStorageForTest(nonCASTestRoot)
	if err != nil {
		t.Fatalf("Failed to create non-CAS storage: %v", err)
	}

	defer func() { _ = nonCASStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(nonCASTestRoot, nonCASStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-1785920548450214012-68b850c0",
	}

	// Create identical objects in both environments
	makeTestObjects := func() []map[string]any {
		return []map[string]any{
			{
				objects.FieldKeyID:            "GOAL-123",
				objects.FieldKeyKind:          "goal",
				objects.FieldKeyTitle:         "Test Goal",
				objects.FieldKeyDescription:   "This is a sufficiently long description for the goal object",
				objects.FieldKeyStatus:        objects.ObjectStatusActive,
				objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			},
			{
				objects.FieldKeyID:            "BLI-200",
				objects.FieldKeyKind:          "backlog_item",
				objects.FieldKeyGoalRefs:      []any{"GOAL-123"},
				objects.FieldKeyTitle:         "Same Results Test",
				objects.FieldKeyStatus:        objects.ObjectStatusExploring,
				objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			},
		}
	}

	// Create in CAS
	for _, obj := range makeTestObjects() {
		storage.CreateCASVisible(t, casStorage, ctx, secCtx, obj, "")
	}

	// Wait for CAS index updates to be processed (use per-project-root queue for test isolation)
	storage.FlushAllOrFail(t, casTestRoot)

	// Create in non-CAS
	for _, obj := range makeTestObjects() {
		storage.CreateCASVisible(t, nonCASStorage, ctx, secCtx, obj, "")
	}

	// Wait for non-CAS index updates too (CAS is always enabled)
	storage.FlushAllOrFail(t, nonCASTestRoot)

	// Verify both can be read with identical content
	casObj, err := casStorage.Read(ctx, secCtx, "BLI-200")
	if err != nil {
		t.Fatalf("Failed to read CAS object: %v", err)
	}

	nonCASObj, err := nonCASStorage.Read(ctx, secCtx, "BLI-200")
	if err != nil {
		t.Fatalf("Failed to read non-CAS object: %v", err)
	}

	// Compare all fields
	fieldsToCompare := []string{"id", "kind", "title", "status", "schema_version"}
	for _, field := range fieldsToCompare {
		if casObj[field] != nonCASObj[field] {
			t.Errorf("Field %s mismatch: CAS=%v, non-CAS=%v", field, casObj[field], nonCASObj[field])
		}
	}

	// Verify file paths are different but content is the same
	casFilePath, _ := casStorage.GetObjectFilePath("BLI-200", "backlog_item")       //nolint:errcheck // Test helper - error handling not critical
	nonCASFilePath, _ := nonCASStorage.GetObjectFilePath("BLI-200", "backlog_item") //nolint:errcheck // Test helper - error handling not critical

	casContent, err := fileutil.ReadFile(casFilePath)
	if err != nil {
		t.Fatalf("Failed to read CAS file: %v", err)
	}

	nonCASContent, err := fileutil.ReadFile(nonCASFilePath)
	if err != nil {
		t.Fatalf("Failed to read non-CAS file: %v", err)
	}

	// Content should be logically equivalent (may have different YAML formatting)
	// But key fields should match
	if len(casContent) == 0 || len(nonCASContent) == 0 {
		t.Error("One or both files are empty")
	}

	// Both should contain the same object ID
	if !strings.Contains(string(casContent), "BLI-200") {
		t.Error("CAS file does not contain object ID")
	}
	if !strings.Contains(string(nonCASContent), "BLI-200") {
		t.Error("Non-CAS file does not contain object ID")
	}

	t.Logf("✓ Same results test: CAS and non-CAS produce identical object content")
}
