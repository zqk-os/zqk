package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
)

func TestCriteriaCascadeDeletionDirection(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage.MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, "process", "requirement"), paths.DirPerm755); err != nil {
		t.Fatalf("failed to create req dir: %v", err)
	}
	if err := paths.EnsureDir(filepath.Join(tmpDir, "process", "criteria"), paths.DirPerm755); err != nil {
		t.Fatalf("failed to create criteria dir: %v", err)
	}
	if err := paths.EnsureDir(filepath.Join(tmpDir, "process", "goals"), paths.DirPerm755); err != nil {
		t.Fatalf("failed to create goals dir: %v", err)
	}

	fos, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() { _ = fos.Shutdown(context.Background()) }()

	// Load field registry for testing
	objects.PrewarmGlobalsForProjectRoot(tmpDir)
	if err := objects.GetGlobalFieldRegistry().LoadFields(); err != nil {
		t.Fatalf("failed to load fields: %v", err)
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := storage.WithCLIOperation(context.Background())

	// Create goal
	goalID := "GOAL-999"
	goal := map[string]any{
		objects.FieldKeyID:            goalID,
		objects.FieldKeyKind:          "goal",
		objects.FieldKeyTitle:         "Test Goal",
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	if err := fos.Create(ctx, secCtx, goal); err != nil {
		t.Fatalf("failed to create goal: %v", err)
	}

	// Create criteria
	critID := "CRIT-999"
	crit := map[string]any{
		objects.FieldKeyID:            critID,
		objects.FieldKeyKind:          "criteria",
		objects.FieldKeyTitle:         "Test Criteria",
		objects.FieldKeyCategory:      "acceptance",
		objects.FieldKeyStatus:        "not_started",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	if err := fos.Create(ctx, secCtx, crit); err != nil {
		t.Fatalf("failed to create criteria: %v", err)
	}

	// Create second criteria so validation passes when first is unlinked
	critID2 := "CRIT-1000"
	crit2 := map[string]any{
		objects.FieldKeyID:            critID2,
		objects.FieldKeyKind:          "criteria",
		objects.FieldKeyTitle:         "Test Criteria 2",
		objects.FieldKeyCategory:      "acceptance",
		objects.FieldKeyStatus:        "not_started",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	if err := fos.Create(ctx, secCtx, crit2); err != nil {
		t.Fatalf("failed to create criteria 2: %v", err)
	}

	// Create requirement referencing both criteria
	reqID := "REQ-999"
	req := map[string]any{
		objects.FieldKeyID:            reqID,
		objects.FieldKeyKind:          "requirement",
		objects.FieldKeyTitle:         "Test Requirement",
		objects.FieldKeyStatus:        "exploring",
		objects.FieldKeyGoalRefs:      []string{"GOAL-999"},
		objects.FieldKeyCriteriaRefs:  []string{critID, critID2},
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	if err := fos.Create(ctx, secCtx, req); err != nil {
		t.Fatalf("failed to create requirement: %v", err)
	}

	// Rebuild reverse reference index
	_ = storage.GetGlobalReverseReferenceIndex().BuildFromScan(tmpDir, filepath.Join(tmpDir, "process"), []string{"criteria", "requirement"})

	// Delete criteria with cascade=true
	if err := fos.Delete(ctx, secCtx, critID, true); err != nil {
		t.Fatalf("failed to delete criteria: %v", err)
	}

	// Verify criteria is deleted
	if _, err := fos.Read(ctx, secCtx, critID); err == nil {
		t.Error("criteria should be deleted")
	}

	// The requirement should NOT be deleted! It should just have the reference nullified.
	reqAfter, err := fos.Read(ctx, secCtx, reqID)
	if err != nil {
		t.Fatalf("requirement WAS DELETED when criteria was cascade-deleted! This is wrong! err: %v", err)
	}

	// The criteria_refs should only contain CRIT-1000
	refs, _ := reqAfter[objects.FieldKeyCriteriaRefs].([]any)
	if len(refs) != 1 || refs[0] != critID2 {
		t.Errorf("criteria reference was not nullified correctly: expected [%s], got %v", critID2, reqAfter[objects.FieldKeyCriteriaRefs])
	}
}
