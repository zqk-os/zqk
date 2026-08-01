package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
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
	mustEnsureProcessSpecsLayout(t, testRoot)
	backlogDir := filepath.Join(testRoot, paths.ProcessBacklogDir)
	if err := paths.EnsureDir(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	fileStorage, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}
	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(testRoot, fileStorage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	// Verify CAS is enabled
	if !fileStorage.usesContentAddressableStorage("backlog_item") {
		t.Fatalf("CAS should be enabled for backlog_item when path contains 'test-scenarios'")
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "account:system",
	}

	// Create a CAS object
	casObj := map[string]any{
		objects.FieldKeyID:            "ITEM-800",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "CAS Object",
		objects.FieldKeyStatus:        "exploring",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	err = fileStorage.Create(ctx, secCtx, casObj)
	if err != nil {
		t.Fatalf("Failed to create CAS object: %v", err)
	}

	// Manually create an ID-based file (simulating pre-migration state)
	idBasedObj := map[string]any{
		objects.FieldKeyID:            "ITEM-801",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "ID-Based Object",
		objects.FieldKeyStatus:        "exploring",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	// Write ID-based file directly
	idBasedPath := filepath.Join(backlogDir, "ITEM-801.yaml")
	data, err := yaml.Marshal(idBasedObj)
	if err != nil {
		t.Fatalf("Failed to marshal ID-based object: %v", err)
	}

	if err := os.WriteFile(idBasedPath, data, paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write ID-based file: %v", err)
	}

	// Test List - should return both CAS and ID-based objects
	storageCtx := pkgctx.GetStorageContext()
	filter := ListFilter{
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
		if id == "ITEM-800" {
			foundCAS = true
		}
		if id == "ITEM-801" {
			foundIDBased = true
		}
	}

	if !foundCAS {
		t.Errorf("CAS object ITEM-800 not found in list")
	}
	if !foundIDBased {
		t.Errorf("ID-based object ITEM-801 not found in list")
	}

	// Test Read - should work for both
	casRead, err := fileStorage.Read(ctx, secCtx, "ITEM-800")
	if err != nil {
		t.Errorf("Failed to read CAS object: %v", err)
	} else if casRead[objects.FieldKeyID] != "ITEM-800" {
		t.Errorf("Expected ID ITEM-800, got %v", casRead[objects.FieldKeyID])
	}

	idBasedRead, err := fileStorage.Read(ctx, secCtx, "ITEM-801")
	if err != nil {
		t.Errorf("Failed to read ID-based object: %v", err)
	} else if idBasedRead[objects.FieldKeyID] != "ITEM-801" {
		t.Errorf("Expected ID ITEM-801, got %v", idBasedRead[objects.FieldKeyID])
	}

	// Test getObjectFilePath - should return correct paths
	casPath, err := fileStorage.getObjectFilePath("ITEM-800", "backlog_item")
	if err != nil {
		t.Errorf("Failed to get CAS file path: %v", err)
	} else {
		// Should be hash-based
		fileName := filepath.Base(casPath)
		if len(fileName) < 68 || !strings.HasSuffix(fileName, ".yaml") {
			t.Errorf("CAS file path should be hash-based, got %s", fileName)
		}
	}

	idBasedPath2, err := fileStorage.getObjectFilePath("ITEM-801", "backlog_item")
	if err != nil {
		t.Errorf("Failed to get ID-based file path: %v", err)
	} else {
		// Should be ID-based
		fileName := filepath.Base(idBasedPath2)
		if fileName != "ITEM-801.yaml" {
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
	mustEnsureProcessSpecsLayout(t, testRoot)
	backlogDir := filepath.Join(testRoot, paths.ProcessBacklogDir)
	if err := paths.EnsureDir(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	fileStorage, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}
	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(testRoot, fileStorage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "account:system",
	}
	storageCtx := pkgctx.GetStorageContext()

	// Create multiple CAS objects
	casObjects := []string{"ITEM-900", "ITEM-901", "ITEM-902"}
	for _, id := range casObjects {
		obj := map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "CAS Object " + id,
			objects.FieldKeyStatus:        "exploring",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		if err := fileStorage.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("Failed to create CAS object %s: %v", id, err)
		}
	}

	// Create ID-based objects manually
	idBasedObjects := []string{"ITEM-903", "ITEM-904"}
	for _, id := range idBasedObjects {
		obj := map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "ID-Based Object " + id,
			objects.FieldKeyStatus:        "exploring",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		data, _ := yaml.Marshal(obj)
		path := filepath.Join(backlogDir, id+".yaml")
		_ = os.WriteFile(path, data, paths.FilePerm644) //nolint:errcheck // Test setup - write errors are acceptable
	}

	// List should return all objects
	filter := ListFilter{
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
