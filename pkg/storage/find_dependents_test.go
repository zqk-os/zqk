package storage_test

import (
	"github.com/lanceman/zqk/pkg/datacell"

	"context"
	"os"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
)

// TestFindDependents_Isolated tests findDependents in isolation
// This helps verify that the core dependency finding logic works correctly
func TestFindDependents_Isolated(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	storage.MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}
	processDir := datacell.ProcessPrimaryDir(tmpDir)

	fos, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer func() { _ = fos.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(tmpDir, fos)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("remove temp dir: %v", err)
		}
	})

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create parent object
	parentObj := map[string]any{
		objects.FieldKeyID:            "PLAN-002",
		objects.FieldKeyKind:          "priority_plan",
		objects.FieldKeyTitle:         "Parent Object",
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	err = fos.Create(ctx, secCtx, parentObj)
	if err != nil {
		t.Fatalf("Failed to create parent object: %v", err)
	}

	// Flush CAS index to ensure parent is indexed
	if err := storage.FlushListingIndexForProjectRoot(tmpDir, "priority_plan"); err != nil {
		t.Fatalf("Failed to flush CAS index: %v", err)
	}

	// Create child object that references parent
	childObj := map[string]any{
		objects.FieldKeyID:              "ITEM-003",
		objects.FieldKeyKind:            "backlog_item",
		objects.FieldKeyTitle:           "Child Object",
		objects.FieldKeyStatus:          "exploring",
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyPriorityPlanRef: "PLAN-002",
	}

	err = fos.Create(ctx, secCtx, childObj)
	if err != nil {
		t.Fatalf("Failed to create child object: %v", err)
	}

	// Flush CAS index to ensure child is indexed
	if err := storage.FlushListingIndexForProjectRoot(tmpDir, "backlog_item"); err != nil {
		t.Fatalf("Failed to flush CAS index: %v", err)
	}

	// Clear global reverse reference index to force fallback scanning in isolated test environment
	storage.GetGlobalReverseReferenceIndex().Clear()

	// Call findDependents directly
	dependents, err := storage.FindDependentsForTest(fos, context.Background(), "PLAN-002", "priority_plan")
	if err != nil {
		t.Fatalf("findDependents failed: %v", err)
	}

	// Verify child is found as a dependent
	if len(dependents) == 0 {
		t.Error("Expected to find at least one dependent, got none")
		t.Logf("Process dir: %s", processDir)

		// Debug: Check what's in the backlog directory
		backlogDir := filepath.Join(processDir, "backlog")
		entries, err := os.ReadDir(backlogDir)
		if err != nil {
			t.Logf("Failed to read backlog directory: %v", err)
		} else {
			t.Logf("Files in backlog directory: %d", len(entries))
			for _, entry := range entries {
				t.Logf("  - %s (dir: %v)", entry.Name(), entry.IsDir())
			}
		}

		// Debug: Check CAS index
		cas, err := storage.GetContentAddressableStorageForTest(fos, "backlog_item")
		if err == nil {
			index := cas.GetIndex()
			mappings := index.SnapshotMappings()
			t.Logf("CAS index mappings: %d entries", len(mappings))
			for objID := range mappings {
				t.Logf("  - %s", objID)
			}
		}
		return
	}

	found := false
	for _, depID := range dependents {
		if depID == "ITEM-003" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected to find ITEM-003 as a dependent, got: %v", dependents)
	}
}

// TestFindDependents_WithDelete tests findDependents in the context of a Delete operation
// This verifies that findDependents works when called from Delete (which is how cascade delete works)
func TestFindDependents_WithDelete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	storage.MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	fos, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer func() { _ = fos.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(tmpDir, fos)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("remove temp dir: %v", err)
		}
	})

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create parent object
	parentObj := map[string]any{
		objects.FieldKeyID:            "PLAN-002",
		objects.FieldKeyKind:          "priority_plan",
		objects.FieldKeyTitle:         "Parent Object",
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	err = fos.Create(ctx, secCtx, parentObj)
	if err != nil {
		t.Fatalf("Failed to create parent object: %v", err)
	}

	// Flush CAS index to ensure parent is indexed
	if err := storage.FlushListingIndexForProjectRoot(tmpDir, "priority_plan"); err != nil {
		t.Fatalf("Failed to flush CAS index: %v", err)
	}

	// Create child object that references parent
	childObj := map[string]any{
		objects.FieldKeyID:              "ITEM-003",
		objects.FieldKeyKind:            "backlog_item",
		objects.FieldKeyTitle:           "Child Object",
		objects.FieldKeyStatus:          "exploring",
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyPriorityPlanRef: "PLAN-002",
	}

	err = fos.Create(ctx, secCtx, childObj)
	if err != nil {
		t.Fatalf("Failed to create child object: %v", err)
	}

	// Flush CAS index to ensure child is indexed
	if err := storage.FlushListingIndexForProjectRoot(tmpDir, "backlog_item"); err != nil {
		t.Fatalf("Failed to flush CAS index: %v", err)
	}

	// Mark context as CLI operation to allow deletions (same as migration step does)
	ctx = storage.WithCLIOperation(ctx)

	// Clear global reverse reference index to force fallback scanning in isolated test environment
	storage.GetGlobalReverseReferenceIndex().Clear()

	// Call Delete directly with cascade=true (this is what the migration step does)
	err = fos.Delete(ctx, secCtx, "PLAN-002", true)
	if err != nil {
		t.Fatalf("Delete with cascade failed: %v", err)
	}

	// Verify parent is deleted
	_, err = fos.Read(ctx, secCtx, "PLAN-002")
	if err == nil {
		t.Error("Parent object should be deleted")
	}

	// Verify child is also deleted (cascade)
	_, err = fos.Read(ctx, secCtx, "ITEM-003")
	if err == nil {
		t.Error("Child object should be deleted (cascade)")
	}
}
