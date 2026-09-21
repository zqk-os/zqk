package cas_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/storage"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestCAS_MixedCASAndNonCAS tests that the system handles mixed CAS and non-CAS objects correctly
// This simulates a migration scenario where some objects are in CAS and some are still ID-based.
// Skip with -short or without ZQK_ENABLE_STORAGE_INTEGRATION_TESTS.
func TestCAS_MixedCASAndNonCAS(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping CAS mixed integration test in short mode")
	}
	// Mixed CAS/non-CAS scenario uses an isolated temp tree (test-scenarios path); no live checkout storage root.
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-mixed-test")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)
	backlogDir := filepath.Join(testRoot, paths.ProcessBacklogDir)
	if err := paths.EnsureDir(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

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

	// Verify CAS is enabled
	if !fileStorage.UsesContentAddressableStorage("backlog_item") {
		t.Fatalf("CAS should be enabled for backlog_item when path contains 'test-scenarios'")
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-SYSTEM",
	}

	// Create a CAS object
	casObj := map[string]any{
		objects.FieldKeyID:            "BLI-800",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "CAS Object",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, casObj, "")

	// Manually create an ID-based file (simulating pre-migration state)
	idBasedObj := map[string]any{
		objects.FieldKeyID:            "BLI-801",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "ID-Based Object",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	// Write ID-based file directly
	idBasedPath := filepath.Join(backlogDir, "BLI-801.yaml")
	data, err := yaml.Marshal(idBasedObj)
	if err != nil {
		t.Fatalf("Failed to marshal ID-based object: %v", err)
	}

	if err := fileutil.WriteFile(idBasedPath, data, paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write ID-based file: %v", err)
	}

	// Test List - should return both CAS and ID-based objects
	storageCtx := pkgctx.GetStorageContext()
	filter := storage.ListFilter{
		Kind: "backlog_item",
	}

	result, err := fileStorage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		t.Fatalf("Failed to list objects: %v", err)
	}

	// Should find both objects
	foundCAS := false
	foundIDBased := false
	for _, obj := range result.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		if id == "BLI-800" {
			foundCAS = true
		}
		if id == "BLI-801" {
			foundIDBased = true
		}
	}

	if !foundCAS {
		t.Errorf("CAS object BLI-800 not found in list")
	}
	if !foundIDBased {
		t.Errorf("ID-based object BLI-801 not found in list")
	}

	// Test Read - should work for both
	casRead, err := fileStorage.Read(ctx, secCtx, "BLI-800")
	if err != nil {
		t.Errorf("Failed to read CAS object: %v", err)
	} else if casRead[objects.FieldKeyID] != "BLI-800" {
		t.Errorf("Expected ID BLI-800, got %v", casRead[objects.FieldKeyID])
	}

	idBasedRead, err := fileStorage.Read(ctx, secCtx, "BLI-801")
	if err != nil {
		t.Errorf("Failed to read ID-based object: %v", err)
	} else if idBasedRead[objects.FieldKeyID] != "BLI-801" {
		t.Errorf("Expected ID BLI-801, got %v", idBasedRead[objects.FieldKeyID])
	}

	// Test getObjectFilePath - should return correct paths
	casPath, err := fileStorage.GetObjectFilePath("BLI-800", "backlog_item")
	if err != nil {
		t.Errorf("Failed to get CAS file path: %v", err)
	} else {
		// Should be hash-based
		fileName := filepath.Base(casPath)
		if len(fileName) < 68 || !strings.HasSuffix(fileName, ".yaml") {
			t.Errorf("CAS file path should be hash-based, got %s", fileName)
		}
	}

	idBasedPath2, err := fileStorage.GetObjectFilePath("BLI-801", "backlog_item")
	if err != nil {
		t.Errorf("Failed to get ID-based file path: %v", err)
	} else {
		// Should be ID-based
		fileName := filepath.Base(idBasedPath2)
		if fileName != "BLI-801.yaml" {
			t.Errorf("ID-based file path should be ID-based, got %s", fileName)
		}
	}
}

// TestCAS_ListIncludesBothCASAndIDBased tests that List correctly combines CAS and ID-based objects.
// Skip with -short for faster feedback.
func TestCAS_ListIncludesBothCASAndIDBased(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping CAS list integration test in short mode")
	}
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-list-test")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)
	backlogDir := filepath.Join(testRoot, paths.ProcessBacklogDir)
	if err := paths.EnsureDir(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

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
	storageCtx := pkgctx.GetStorageContext()

	// Create multiple CAS objects
	casObjects := []string{"BLI-900", "BLI-901", "BLI-902"}
	for _, id := range casObjects {
		obj := map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "CAS Object " + id,
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		storage.CreateCASVisible(t, fileStorage, ctx, secCtx, obj, "")
	}

	// Create ID-based objects manually
	idBasedObjects := []string{"BLI-903", "BLI-904"}
	for _, id := range idBasedObjects {
		obj := map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "ID-Based Object " + id,
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		data, _ := yaml.Marshal(obj)
		path := filepath.Join(backlogDir, id+".yaml")
		_ = fileutil.WriteFile(path, data, paths.FilePerm644) //nolint:errcheck // Test setup - write errors are acceptable
	}

	// List should return all objects
	filter := storage.ListFilter{
		Kind: "backlog_item",
	}

	result, err := fileStorage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		t.Fatalf("Failed to list objects: %v", err)
	}

	// Verify all objects are found
	found := make(map[string]bool)
	for _, obj := range result.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		found[id] = true
	}

	//nolint:gocritic // Intentionally creating new slice by appending two slices
	allObjects := append(casObjects, idBasedObjects...)
	for _, id := range allObjects {
		if !found[id] {
			t.Errorf("Object %s not found in list results", id)
		}
	}

	// Verify count is correct
	if len(result.Objects) < len(allObjects) {
		t.Errorf("Expected at least %d objects, got %d", len(allObjects), len(result.Objects))
	}
}
