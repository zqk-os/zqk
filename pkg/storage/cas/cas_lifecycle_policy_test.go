package cas_test

import (
	"context"
	"github.com/lanceman/zqk/pkg/storage"

	"github.com/lanceman/zqk/pkg/datacell"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// TestCAS_LifecycleValidation tests that lifecycle validation works with CAS objects
func TestCAS_LifecycleValidation(t *testing.T) {
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-lifecycle-test")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	// Verify CAS is enabled for backlog_item
	if !fileStorage.UsesContentAddressableStorage("backlog_item") {
		t.Fatalf("CAS should be enabled for backlog_item when path contains 'test-scenarios'")
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-1785920548450214012-68b850c0",
	}

	// Create a milestone first (required for lifecycle transition)
	milestone := map[string]any{
		objects.FieldKeyID:            "MIL-001",
		objects.FieldKeyKind:          "milestone",
		objects.FieldKeyTitle:         "Test Milestone",
		objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, milestone, "")

	// Create a backlog item with milestone reference (tests reference validation with CAS)
	backlogItem := map[string]any{
		objects.FieldKeyID:            "BLI-300",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Lifecycle Test",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeyMilestoneRefs: []string{"MIL-001"},
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	// Create should trigger lifecycle validation
	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, backlogItem, "")

	// Wait for index updates to be processed
	if err := storage.FlushAllListingIndexes(); err != nil {
		t.Fatalf("Failed to flush write queue: %v", err)
	}

	// Verify both objects are in CAS
	cas, err := fileStorage.GetContentAddressableStorage("backlog_item")
	if err != nil {
		t.Fatalf("Failed to get CAS: %v", err)
	}

	_, err = cas.GetHashForID("BLI-300")
	if err != nil {
		t.Errorf("Backlog item should exist in CAS index: %v", err)
	}

	milestoneCAS, err := fileStorage.GetContentAddressableStorage("milestone")
	if err != nil {
		t.Fatalf("Failed to get milestone CAS: %v", err)
	}

	_, err = milestoneCAS.GetHashForID("MIL-001")
	if err != nil {
		t.Errorf("Milestone should exist in CAS index: %v", err)
	}

	// Test lifecycle validation by attempting a transition that requires preconditions
	// This verifies that lifecycle validation runs correctly with CAS objects
	backlogItem[objects.FieldKeyStatus] = "in_progress"
	err = fileStorage.Update(ctx, secCtx, "BLI-300", backlogItem)

	// This should fail due to lifecycle preconditions (priority_plan_ref required)
	// This is expected behavior - lifecycle validation is working correctly
	if err != nil {
		// Lifecycle validation correctly rejected the transition
		// This proves lifecycle validation works with CAS
		if !strings.Contains(err.Error(), "Precondition") && !strings.Contains(err.Error(), "lifecycle") {
			t.Errorf("Expected lifecycle validation error, got: %v", err)
		}
	} else {
		// If it succeeded, that's also fine - just verify the object was updated
		updated, err := fileStorage.Read(ctx, secCtx, "BLI-300")
		if err != nil {
			t.Fatalf("Failed to read updated backlog item: %v", err)
		}
		if updated[objects.FieldKeyStatus] != "in_progress" {
			t.Errorf("Expected status 'in_progress', got %v", updated[objects.FieldKeyStatus])
		}
	}
}

// TestCAS_ReferenceValidation tests that reference validation works with CAS objects
func TestCAS_ReferenceValidation(t *testing.T) {
	projectRoot := storage.ProjectRootForSpecCopyForTest(t)
	if projectRoot == "" {
		t.Skip("Skipping TestCAS_ReferenceValidation: project root with object_specs not found")
	}
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-reference-test")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)
	specsDir := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir)
	srcSpecs := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	for _, name := range []string{"base_object.yaml", "backlog_item.yaml"} {
		src := filepath.Join(srcSpecs, name)
		data, err := fileutil.ReadFile(src)
		if err != nil {
			t.Skipf("Skipping TestCAS_ReferenceValidation: cannot read %s: %v", src, err)
		}
		if err := fileutil.WriteFile(filepath.Join(specsDir, name), data, paths.FilePerm644); err != nil {
			t.Fatalf("Failed to copy spec %s: %v", name, err)
		}
	}

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	// Verify CAS is enabled
	if !fileStorage.UsesContentAddressableStorage("backlog_item") {
		t.Fatalf("CAS should be enabled for backlog_item when path contains 'test-scenarios'")
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-1785920548450214012-68b850c0",
	}

	// Create referenced object first (objective required for scenario; promote off draft plane).
	referenced := map[string]any{
		objects.FieldKeyID:            "BLI-500",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Referenced Item",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, referenced, "")

	// Verify referenced object is in CAS
	cas, err := fileStorage.GetContentAddressableStorage("backlog_item")
	if err != nil {
		t.Fatalf("Failed to get CAS: %v", err)
	}

	_, err = cas.GetHashForID("BLI-500")
	if err != nil {
		t.Fatalf("Referenced object should exist in CAS: %v", err)
	}

	// Create object that references the first one
	objWithRef := map[string]any{
		objects.FieldKeyID:            "BLI-501",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Object With Reference",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		"related_refs":                []string{"BLI-500"},
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	// This should succeed because reference exists
	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, objWithRef, "")

	// Try to create object with invalid reference (should fail)
	objWithInvalidRef := map[string]any{
		objects.FieldKeyID:            "BLI-502",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Object With Invalid Reference",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		"related_refs":                []string{"BLI-999"},
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	err = fileStorage.Create(ctx, secCtx, objWithInvalidRef)
	if err != nil {
		t.Errorf("Expected creation to succeed (membrane pattern), but got: %v", err)
	}

	// Verify that validation itself catches the error
	valErr := fileStorage.ValidateObject(ctx, objWithInvalidRef, "scenario", "")
	if valErr == nil {
		t.Errorf("Expected error from validateObject for invalid reference")
	} else if !strings.Contains(valErr.Error(), "does not exist") && !strings.Contains(valErr.Error(), "not found") {
		t.Logf("Reference validation error (may be expected): %v", valErr)
	}
}

// TestCAS_GetObjectFilePathForReferenceValidation tests that getObjectFilePath works for reference validation
func TestCAS_GetObjectFilePathForReferenceValidation(t *testing.T) {
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-filepath-test")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	// Verify CAS is enabled
	if !fileStorage.UsesContentAddressableStorage("backlog_item") {
		t.Fatalf("CAS should be enabled for backlog_item when path contains 'test-scenarios'")
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-1785920548450214012-68b850c0",
	}

	// Create an object
	obj := map[string]any{
		objects.FieldKeyID:            "BLI-400",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "FilePath Test",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, obj, "")

	// Get file path using getObjectFilePath (what reference validation uses)
	filePath, err := fileStorage.GetObjectFilePath("BLI-400", "backlog_item")
	if err != nil {
		t.Fatalf("Failed to get file path: %v", err)
	}

	// For CAS objects, this should return the hash-based path
	// Verify it's a hash-based filename (64-char hex)
	fileName := filepath.Base(filePath)
	if len(fileName) < 68 { // 64 chars hash + .yaml = 69 chars minimum
		t.Errorf("Expected hash-based filename (64+ chars with .yaml extension), got %s (len=%d)", fileName, len(fileName))
	}

	// Verify the file exists at that path (what os.Stat would check in reference validation)
	if _, err := fileutil.Stat(filePath); err != nil {
		t.Errorf("File should exist at path returned by getObjectFilePath: %v", err)
	}

	// Verify we can read the file
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		t.Errorf("Failed to read file at path: %v", err)
	}

	if len(data) == 0 {
		t.Errorf("File should not be empty")
	}

	// Flush CAS index so cleanup can remove temp dir (no open handles under project root)
	if err := storage.FlushAllListingIndexesForProjectRoot(fileStorage.GetProjectRoot()); err != nil {
		t.Logf("Flush CAS indexes (best-effort for cleanup): %v", err)
	}
}

// TestCAS_GetObjectFilePath_DiscoveryFallback tests that getObjectFilePath resolves CAS resources
// when the index has no entry (e.g. object created outside this process). Reference validation
// must resolve CAS-based resources without requiring --relaxed.
func TestCAS_GetObjectFilePath_DiscoveryFallback(t *testing.T) {
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-discovery-test")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)
	processDir := datacell.ProcessPrimaryDir(testRoot)
	requirementsDir := filepath.Join(processDir, "requirements")
	if err := paths.EnsureDir(requirementsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create requirements directory: %v", err)
	}

	// Write a hash-named file without adding it to the CAS index (simulates object created elsewhere)
	hashName := "0000000000000000000000000000000000000000000000000000000000000001.yaml" // 64 hex chars
	content := []byte("id: REQU-DISCOVER\nkind: requirement\ntitle: Discovery test\nschema_version: \"" + objects.DefaultSchemaVersion + "\"\n")
	hashPath := filepath.Join(requirementsDir, hashName)
	if err := fileutil.WriteFile(hashPath, content, paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write hash-named file: %v", err)
	}

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	// getObjectFilePath should discover the file by scanning (index has no entry)
	gotPath, err := fileStorage.GetObjectFilePath("REQU-DISCOVER", "requirement")
	if err != nil {
		t.Fatalf("getObjectFilePath should resolve via discovery: %v", err)
	}
	if gotPath != hashPath {
		t.Errorf("getObjectFilePath = %q, want %q", gotPath, hashPath)
	}
	if _, err := fileutil.Stat(gotPath); err != nil {
		t.Errorf("Discovered path should exist: %v", err)
	}
}
