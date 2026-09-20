package crud_test

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCriteriaCascadeDeletionDirection(t *testing.T) {
	tmpDir, err := fileutil.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tmpDir)

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
	ctx := storage.WithTestHardDelete(context.Background())

	// Create goal
	goalID := "GOAL-999"
	goal := map[string]any{
		objects.FieldKeyID:            goalID,
		objects.FieldKeyKind:          "goal",
		objects.FieldKeyTitle:         "Test Goal",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	// TRACK: BLI-1785443942668406000-1ec5c811 — draft-plane create / promote membrane.
	storage.CreateCASVisible(t, fos, ctx, secCtx, goal, "active")

	// Create criteria
	critID := "CRIT-999"
	crit := map[string]any{
		objects.FieldKeyID:       critID,
		objects.FieldKeyKind:     "criteria",
		objects.FieldKeyTitle:    "Test Criteria",
		objects.FieldKeyCategory: "acceptance",
		// awaiting_verification is the criteria lifecycle's initial status. not_started is not a
		// criteria status at all, so Create refused it before the cascade under test was reached.
		objects.FieldKeyStatus:        objects.ObjectStatusAwaitingVerification,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, fos, ctx, secCtx, crit, objects.ObjectStatusInProgress)

	// Create second criteria so validation passes when first is unlinked
	critID2 := "CRIT-1000"
	crit2 := map[string]any{
		objects.FieldKeyID:            critID2,
		objects.FieldKeyKind:          "criteria",
		objects.FieldKeyTitle:         "Test Criteria 2",
		objects.FieldKeyCategory:      "acceptance",
		objects.FieldKeyStatus:        objects.ObjectStatusAwaitingVerification,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, fos, ctx, secCtx, crit2, objects.ObjectStatusInProgress)

	// Create requirement referencing both criteria
	reqID := "REQ-999"
	req := map[string]any{
		objects.FieldKeyID:            reqID,
		objects.FieldKeyKind:          "requirement",
		objects.FieldKeyTitle:         "Test Requirement",
		objects.FieldKeyStatus:        objects.ObjectStatusProposed,
		objects.FieldKeyGoalRefs:      []string{"GOAL-999"},
		objects.FieldKeyCriteriaRefs:  []string{critID, critID2},
		objects.FieldKeyPriority:      "p1",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	// requirement goes proposed → active; it has no validated status.
	storage.CreateCASVisible(t, fos, ctx, secCtx, req, objects.ObjectStatusActive)

	// CUD reverse-ref + flush (no processDir scan fallback).
	// TRACK: BLI-1785351629281373000-752ddc48
	storage.FlushReverseReferenceIndexPersist()

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
