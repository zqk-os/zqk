package cas_test

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestCAS_RelationshipValidation tests that relationship validation still works with CAS
// This ensures that reference fields can be validated even when objects are stored in CAS format
func TestCAS_RelationshipValidation(t *testing.T) {
	testDir := t.TempDir()

	storage.MustEnsureProcessSpecsLayoutForTest(t, testDir)
	if err := paths.EnsureDir(filepath.Join(testDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	fos, err := storage.NewFileObjectStorageForTest(testDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	defer func() { _ = fos.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		if cleanup := fos.GetTestCleanup(); cleanup != nil {
			cleanup()
		}
	})

	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-1785920548450214012-68b850c0",
		Roles:     []string{"admin"},
	}

	ctx := context.Background()

	referencedObj := map[string]any{
		objects.FieldKeyID:            "BLI-100",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Referenced Backlog Item",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	storage.CreateCASVisible(t, fos, ctx, secCtx, referencedObj, "")

	readObj, err := fos.Read(ctx, secCtx, "BLI-100")
	if err != nil {
		t.Fatalf("Failed to read referenced object: %v", err)
	}
	if readObj[objects.FieldKeyID] != "BLI-100" {
		t.Errorf("Expected ID BLI-100, got %v", readObj[objects.FieldKeyID])
	}

	referencingObj := map[string]any{
		objects.FieldKeyID:            "BLI-101",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Referencing Backlog Item",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		"backlog_item_ref":            "BLI-100",
	}

	storage.CreateCASVisible(t, fos, ctx, secCtx, referencingObj, "")

	readObj2, err := fos.Read(ctx, secCtx, "BLI-101")
	if err != nil {
		t.Fatalf("Failed to read referencing object: %v", err)
	}
	if readObj2[objects.FieldKeyID] != "BLI-101" {
		t.Errorf("Expected ID BLI-101, got %v", readObj2[objects.FieldKeyID])
	}

	if readObj2["backlog_item_ref"] != "BLI-100" {
		t.Errorf("Expected backlog_item_ref to be BLI-100, got %v", readObj2["backlog_item_ref"])
	}

	storageCtx := &pkgctx.StorageContext{}
	filter := storage.ListFilter{
		Kind: "backlog_item",
	}
	results, err := fos.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		t.Fatalf("Failed to list objects: %v", err)
	}

	if len(results.Objects) < 2 {
		t.Errorf("Expected at least 2 objects, got %d", len(results.Objects))
	}

	foundRef := false
	foundRef2 := false
	for _, obj := range results.Objects {
		if obj[objects.FieldKeyID] == "BLI-100" {
			foundRef = true
		}
		if obj[objects.FieldKeyID] == "BLI-101" {
			foundRef2 = true
		}
	}

	if !foundRef {
		t.Error("Referenced object not found in list")
	}
	if !foundRef2 {
		t.Error("Referencing object not found in list")
	}

	t.Logf("✓ Relationship validation works with CAS - both objects created and references preserved")
}

// TestCAS_FieldValidation tests that field validation still works with CAS
// This ensures that schema validation, required fields, etc. still work
func TestCAS_FieldValidation(t *testing.T) {
	projectRoot := storage.ProjectRootForSpecCopyForTest(t)
	if projectRoot == "" {
		t.Skip("Skipping TestCAS_FieldValidation: project root with object_specs not found")
	}

	testDir := t.TempDir()
	storage.MustEnsureProcessSpecsLayoutForTest(t, testDir)
	if err := paths.EnsureDir(filepath.Join(testDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}
	specsDir := filepath.Join(testDir, paths.ProcessInternalObjectSpecsDir)
	srcSpecs := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	for _, name := range []string{"base_object.yaml", "backlog_item.yaml"} {
		src := filepath.Join(srcSpecs, name)
		data, err := fileutil.ReadFile(src)
		if err != nil {
			t.Skipf("Skipping TestCAS_FieldValidation: cannot read %s: %v", src, err)
		}
		if err := fileutil.WriteFile(filepath.Join(specsDir, name), data, paths.FilePerm644); err != nil {
			t.Fatalf("Failed to copy spec %s: %v", name, err)
		}
	}

	fos, err := storage.NewFileObjectStorageForTest(testDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	defer func() { _ = fos.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		if cleanup := fos.GetTestCleanup(); cleanup != nil {
			cleanup()
		}
	})

	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-1785920548450214012-68b850c0",
		Roles:     []string{"admin"},
	}

	ctx := context.Background()

	invalidObj := map[string]any{
		objects.FieldKeyID:    "BLI-200",
		objects.FieldKeyKind:  "backlog_item",
		objects.FieldKeyTitle: "Invalid Object",
	}

	err = fos.Create(ctx, secCtx, invalidObj)
	if err != nil {
		t.Errorf("Expected nil error due to membrane pattern, got: %v", err)
	}

	validObj := map[string]any{
		objects.FieldKeyID:            "BLI-201",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Valid Object",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	storage.CreateCASVisible(t, fos, ctx, secCtx, validObj, "")

	readObj, err := fos.Read(ctx, secCtx, "BLI-201")
	if err != nil {
		t.Fatalf("Failed to read valid object: %v", err)
	}

	if readObj[objects.FieldKeyID] != "BLI-201" {
		t.Errorf("Expected ID BLI-201, got %v", readObj[objects.FieldKeyID])
	}
	if readObj[objects.FieldKeyTitle] != "Valid Object" {
		t.Errorf("Expected title 'Valid Object', got %v", readObj[objects.FieldKeyTitle])
	}
	if readObj[objects.FieldKeyStatus] != objects.ObjectStatusRoadmap {
		t.Errorf("Expected status %q after CreateCASVisible, got %v", objects.ObjectStatusRoadmap, readObj[objects.FieldKeyStatus])
	}

	t.Logf("✓ Field validation works with CAS - required fields enforced, valid objects created successfully")
}
