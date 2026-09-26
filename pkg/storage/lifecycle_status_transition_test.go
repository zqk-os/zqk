package storage_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func setupLifecycleTestStorage(t *testing.T) (*storage.FileObjectStorage, string, *pkgctx.SecurityContext) {
	t.Helper()
	tmpDir, err := fileutil.MkdirTemp("", "zqk-lifecycle-test-*")
	require.NoError(t, err)

	storage.MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	for _, dir := range []string{
		objects.GetDirectoryFromKind(objects.KindBacklogItem),
		objects.GetDirectoryFromKind(objects.KindCriteria),
		objects.GetDirectoryFromKind("test_case"),
	} {
		if dir != "" {
			require.NoError(t, paths.EnsureDir(filepath.Join(tmpDir, dir), paths.DirPerm755))
		}
	}

	fos, err := storage.NewFileObjectStorageForTest(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = fos.Shutdown(context.Background())
		opts := storage.TempProjectTeardown(tmpDir, fos)
		_ = storage.RunProjectTestTeardown(opts)
		_ = fileutil.RemoveAll(tmpDir)
	})

	secCtx := pkgctx.NewSecurityContext(pkgctx.SystemAccountID, []string{"admin", "scheduler"}, []string{"read:*", "write:*"})
	return fos, tmpDir, secCtx
}

func TestStorageUpdate_RejectsIllegalLifecycleStatusTransition(t *testing.T) {
	fos, _, secCtx := setupLifecycleTestStorage(t)
	ctx := context.Background()

	t.Run("rejects_criteria_conceptual_to_complete", func(t *testing.T) {
		critID := "CRIT-TRANS-001"
		critObj := map[string]any{
			objects.FieldKeyID:            critID,
			objects.FieldKeyKind:          "criteria",
			objects.FieldKeyTitle:         "Initial Criterion",
			objects.FieldKeyDescription:   "Detailed description of criterion for testing",
			objects.FieldKeyStatus:        "conceptual",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyPriorityTier:  "P2",
			objects.FieldKeyPriority:      "medium",
			"category":                    "functional",
		}
		require.NoError(t, fos.Create(ctx, secCtx, critObj))

		// Attempt illegal jump: conceptual -> complete
		err := fos.Update(ctx, secCtx, critID, map[string]any{
			objects.FieldKeyStatus: "complete",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "illegal lifecycle status transition")
		require.Contains(t, err.Error(), critID)
		require.Contains(t, err.Error(), "criteria")
		require.Contains(t, err.Error(), "cannot transition from")
		require.Contains(t, err.Error(), "allowed transition target(s)")

		readObj, err := fos.Read(ctx, secCtx, critID)
		require.NoError(t, err)
		require.Equal(t, "conceptual", readObj[objects.FieldKeyStatus])
	})

	t.Run("rejects_backlog_item_conceptual_to_complete", func(t *testing.T) {
		bliID := "BLI-TRANS-001"
		bliObj := map[string]any{
			objects.FieldKeyID:            bliID,
			objects.FieldKeyKind:          objects.KindBacklogItem,
			objects.FieldKeyTitle:         "Conceptual Backlog Item",
			objects.FieldKeyDescription:   "Detailed description of backlog item for testing",
			objects.FieldKeyStatus:        "conceptual",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyPriorityTier:  "P1",
			objects.FieldKeyPriority:      "high",
		}
		require.NoError(t, fos.Create(ctx, secCtx, bliObj))

		// Attempt illegal jump: conceptual -> complete
		err := fos.Update(ctx, secCtx, bliID, map[string]any{
			objects.FieldKeyStatus: "complete",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "illegal lifecycle status transition")
		require.Contains(t, err.Error(), bliID)
		require.Contains(t, err.Error(), "backlog_item")
		require.Contains(t, err.Error(), "allowed transition target(s)")

		readObj, err := fos.Read(ctx, secCtx, bliID)
		require.NoError(t, err)
		require.Equal(t, "conceptual", readObj[objects.FieldKeyStatus])
	})

	t.Run("rejects_backlog_item_exploring_to_complete", func(t *testing.T) {
		bliID := "BLI-TRANS-002"
		bliObj := map[string]any{
			objects.FieldKeyID:            bliID,
			objects.FieldKeyKind:          objects.KindBacklogItem,
			objects.FieldKeyTitle:         "Backlog Item under exploration",
			objects.FieldKeyDescription:   "Detailed description of backlog item for testing",
			objects.FieldKeyStatus:        "exploring",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyPriorityTier:  "P1",
			objects.FieldKeyPriority:      "high",
		}
		require.NoError(t, fos.Create(ctx, secCtx, bliObj))

		// Attempt illegal skip: exploring -> complete
		err := fos.Update(ctx, secCtx, bliID, map[string]any{
			objects.FieldKeyStatus: "complete",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "illegal lifecycle status transition")
		require.Contains(t, err.Error(), bliID)
		require.Contains(t, err.Error(), "backlog_item")
		require.Contains(t, err.Error(), "allowed transition target(s)")
	})
}

func TestStorageUpdate_AcceptsValidLifecycleStatusTransition(t *testing.T) {
	fos, _, secCtx := setupLifecycleTestStorage(t)
	ctx := context.Background()

	t.Run("accepts_test_case_draft_to_active", func(t *testing.T) {
		tcID := "TST-TRANS-002"
		tcObj := map[string]any{
			objects.FieldKeyID:            tcID,
			objects.FieldKeyKind:          "test_case",
			objects.FieldKeyTitle:         "Valid test case",
			objects.FieldKeyDescription:   "Detailed description of test case for testing",
			objects.FieldKeyStatus:        "draft",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyPriorityTier:  "P2",
			objects.FieldKeyPriority:      "medium",
		}
		require.NoError(t, fos.Create(ctx, secCtx, tcObj))

		// Legal transition: draft -> active
		err := fos.Update(ctx, secCtx, tcID, map[string]any{
			objects.FieldKeyStatus: "active",
		})
		require.NoError(t, err)

		readObj, err := fos.Read(ctx, secCtx, tcID)
		require.NoError(t, err)
		require.Equal(t, "active", readObj[objects.FieldKeyStatus])
	})

	t.Run("accepts_criteria_awaiting_verification_to_in_progress", func(t *testing.T) {
		critID := "CRIT-TRANS-002"
		critObj := map[string]any{
			objects.FieldKeyID:            critID,
			objects.FieldKeyKind:          "criteria",
			objects.FieldKeyTitle:         "Criterion awaiting verification",
			objects.FieldKeyDescription:   "Detailed description of criterion for testing",
			objects.FieldKeyStatus:        "awaiting_verification",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyPriorityTier:  "P2",
			objects.FieldKeyPriority:      "medium",
			"category":                    "functional",
		}
		require.NoError(t, fos.Create(ctx, secCtx, critObj))

		// Legal transition: awaiting_verification -> in_progress
		err := fos.Update(ctx, secCtx, critID, map[string]any{
			objects.FieldKeyStatus: "in_progress",
		})
		require.NoError(t, err)

		readObj, err := fos.Read(ctx, secCtx, critID)
		require.NoError(t, err)
		require.Equal(t, "in_progress", readObj[objects.FieldKeyStatus])
	})

	t.Run("accepts_backlog_item_exploring_to_validated", func(t *testing.T) {
		bliID := "BLI-TRANS-003"
		bliObj := map[string]any{
			objects.FieldKeyID:                     bliID,
			objects.FieldKeyKind:                   objects.KindBacklogItem,
			objects.FieldKeyTitle:                  "Exploring backlog item",
			objects.FieldKeyDescription:            "Detailed description of backlog item for testing",
			objects.FieldKeyStatus:                 "exploring",
			objects.FieldKeyProblemStatement:       "Substantive problem statement for validation testing.",
			objects.FieldKeyAcceptanceConsiderations: "Clear acceptance considerations for validation testing.",
			objects.FieldKeySchemaVersion:          objects.DefaultSchemaVersion,
			objects.FieldKeyPriorityTier:           "P2",
			objects.FieldKeyPriority:               "medium",
		}
		require.NoError(t, fos.Create(ctx, secCtx, bliObj))

		// Legal transition: exploring -> validated
		err := fos.Update(ctx, secCtx, bliID, map[string]any{
			objects.FieldKeyStatus: "validated",
		})
		require.NoError(t, err)

		readObj, err := fos.Read(ctx, secCtx, bliID)
		require.NoError(t, err)
		require.Equal(t, "validated", readObj[objects.FieldKeyStatus])
	})
}

func TestStorageUpdate_BreakGlassAllowsEmergencyTransition(t *testing.T) {
	fos, _, secCtx := setupLifecycleTestStorage(t)
	ctx := context.Background()

	critID := "CRIT-TRANS-004"
	critObj := map[string]any{
		objects.FieldKeyID:            critID,
		objects.FieldKeyKind:          "criteria",
		objects.FieldKeyTitle:         "Emergency Criterion",
		objects.FieldKeyDescription:   "Detailed description of emergency criterion for testing",
		objects.FieldKeyStatus:        "conceptual",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyPriorityTier:  "P1",
		objects.FieldKeyPriority:      "high",
		"category":                    "functional",
	}
	require.NoError(t, fos.Create(ctx, secCtx, critObj))

	// Without break-glass: fails
	err := fos.Update(ctx, secCtx, critID, map[string]any{
		objects.FieldKeyStatus: "in_progress",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "illegal lifecycle status transition")

	// With elevated break-glass: succeeds
	bgCtx := pkgctx.WithLifecycleBreakGlass(pkgctx.WithAllowCoreObjectDelete(ctx), "emergency hotfix completion bypass")
	err = fos.Update(bgCtx, secCtx, critID, map[string]any{
		objects.FieldKeyStatus: "in_progress",
	})
	require.NoError(t, err)

	readObj, err := fos.Read(ctx, secCtx, critID)
	require.NoError(t, err)
	require.Equal(t, "in_progress", readObj[objects.FieldKeyStatus])
}

func TestStorageUpdate_SameStatusNoOpDoesNotTriggerTransitionError(t *testing.T) {
	fos, _, secCtx := setupLifecycleTestStorage(t)
	ctx := context.Background()

	bliID := "BLI-TRANS-005"
	bliObj := map[string]any{
		objects.FieldKeyID:            bliID,
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyTitle:         "Title before update",
		objects.FieldKeyDescription:   "Detailed description for testing",
		objects.FieldKeyStatus:        "exploring",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyPriorityTier:  "P1",
		objects.FieldKeyPriority:      "high",
	}
	require.NoError(t, fos.Create(ctx, secCtx, bliObj))

	// Update only non-status field
	err := fos.Update(ctx, secCtx, bliID, map[string]any{
		objects.FieldKeyTitle: "Title updated",
	})
	require.NoError(t, err)

	// Update with identical status
	err = fos.Update(ctx, secCtx, bliID, map[string]any{
		objects.FieldKeyStatus: "exploring",
		objects.FieldKeyTitle:  "Title updated again",
	})
	require.NoError(t, err)

	readObj, err := fos.Read(ctx, secCtx, bliID)
	require.NoError(t, err)
	require.Equal(t, "exploring", readObj[objects.FieldKeyStatus])
	require.Equal(t, "Title updated again", readObj[objects.FieldKeyTitle])
}
