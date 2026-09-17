package storage_test

import (
	"github.com/lanceman/zqk/pkg/datacell"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"context"
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
	tmpDir, err := fileutil.MkdirTemp("", "zqk-test-*")
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
		if err := fileutil.RemoveAll(tmpDir); err != nil {
			t.Logf("remove temp dir: %v", err)
		}
	})

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create parent object
	parentObj := map[string]any{
		objects.FieldKeyID:            "PRI-002",
		objects.FieldKeyKind:          "priority_plan",
		objects.FieldKeyTitle:         "Parent Object",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	// Draft-first create parks preliminary intents off CAS List; promote so reverse-ref scan sees them.
	// TRACK: BLI-REDACTED — draft-plane create / promote membrane.
	storage.CreateCASVisible(t, fos, ctx, secCtx, parentObj, "active")

	// Flush CAS index to ensure parent is indexed
	if err := storage.FlushListingIndexForProjectRoot(tmpDir, "priority_plan"); err != nil {
		t.Fatalf("Failed to flush CAS index: %v", err)
	}

	// Create child object that references parent
	childObj := map[string]any{
		objects.FieldKeyID:              "BLI-003",
		objects.FieldKeyKind:            "backlog_item",
		objects.FieldKeyTitle:           "Child Object",
		objects.FieldKeyStatus:          objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyPriorityPlanRef: "PRI-002",
	}

	storage.CreateCASVisible(t, fos, ctx, secCtx, childObj, objects.ObjectStatusValidated)

	// Flush CAS index to ensure child is indexed
	if err := storage.FlushListingIndexForProjectRoot(tmpDir, "backlog_item"); err != nil {
		t.Fatalf("Failed to flush CAS index: %v", err)
	}
	// findDependents uses the reverse-ref index (no processDir scan fallback).
	// TRACK: BLI-REDACTED — do not Clear() here; CUD + flush is the contract.
	storage.FlushReverseReferenceIndexPersist()

	// Call findDependents directly
	dependents, err := storage.FindDependentsForTest(fos, context.Background(), "PRI-002", "priority_plan")
	if err != nil {
		t.Fatalf("findDependents failed: %v", err)
	}

	// Verify child is found as a dependent (audit/change objects may also appear)
	found := false
	for _, depID := range dependents {
		if depID == "BLI-003" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected to find BLI-003 as a dependent, got: %v (processDir=%s)", dependents, processDir)
	}
}

// TestFindDependents_WithDelete tests findDependents in the context of a Delete operation.
// priority_plan_ref is composition → cascade unlinks the child (does not delete it).
func TestFindDependents_WithDelete(t *testing.T) {
	tmpDir, err := fileutil.MkdirTemp("", "zqk-test-*")
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
		if err := fileutil.RemoveAll(tmpDir); err != nil {
			t.Logf("remove temp dir: %v", err)
		}
	})

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create parent object
	parentObj := map[string]any{
		objects.FieldKeyID:            "PRI-002",
		objects.FieldKeyKind:          "priority_plan",
		objects.FieldKeyTitle:         "Parent Object",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	// TRACK: BLI-REDACTED — draft-plane create / promote membrane.
	storage.CreateCASVisible(t, fos, ctx, secCtx, parentObj, "active")

	// Flush CAS index to ensure parent is indexed
	if err := storage.FlushListingIndexForProjectRoot(tmpDir, "priority_plan"); err != nil {
		t.Fatalf("Failed to flush CAS index: %v", err)
	}

	// Create child object that references parent
	childObj := map[string]any{
		objects.FieldKeyID:              "BLI-003",
		objects.FieldKeyKind:            "backlog_item",
		objects.FieldKeyTitle:           "Child Object",
		objects.FieldKeyStatus:          objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyPriorityPlanRef: "PRI-002",
	}

	storage.CreateCASVisible(t, fos, ctx, secCtx, childObj, objects.ObjectStatusValidated)

	// Flush CAS index to ensure child is indexed
	if err := storage.FlushListingIndexForProjectRoot(tmpDir, "backlog_item"); err != nil {
		t.Fatalf("Failed to flush CAS index: %v", err)
	}
	// TRACK: BLI-REDACTED — cascade delete needs CUD reverse-ref, not Clear()+scan.
	storage.FlushReverseReferenceIndexPersist()

	// CLI + core-delete allow: priority_plan/backlog_item are kernel-critical.
	ctx = storage.WithTestHardDelete(ctx)

	// Call Delete directly with cascade=true (findDependents must see BLI-003 to unlink)
	err = fos.Delete(ctx, secCtx, "PRI-002", true)
	if err != nil {
		t.Fatalf("Delete with cascade failed: %v", err)
	}

	// Verify parent is deleted
	_, err = fos.Read(ctx, secCtx, "PRI-002")
	if err == nil {
		t.Error("Parent object should be deleted")
	}

	// Composition/association refs unlink rather than cascade-delete the dependent.
	child, err := fos.Read(ctx, secCtx, "BLI-003")
	if err != nil {
		t.Fatalf("Child should remain after composition unlink, read failed: %v", err)
	}
	if ref, _ := child[objects.FieldKeyPriorityPlanRef].(string); ref != "" {
		t.Errorf("expected priority_plan_ref unlinked, got %q", ref)
	}
}
