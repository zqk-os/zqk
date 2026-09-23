package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestStorageExtended_Wave21_CoreFileOps_and_Executors(t *testing.T) {
	ctx := context.Background()

	t.Run("FileObjectStorage_WriteBuffer_Direct_Apply", func(t *testing.T) {
		_, fos, secCtx := SetupTestingFactoryCompleteTestEnvironmentForTest(t)

		// 1. applyCreateFromBuffer
		dataCreate := []byte("kind: backlog_item\nid: BLI-BUF-1\ntitle: Buf Item 1\nstatus: planned\ndescription: Buffer item desc\n")
		err := fos.applyCreateFromBuffer(ctx, "BLI-BUF-1", "backlog_item", dataCreate, secCtx)
		assert.NoError(t, err)

		// Verify empty data error
		err = fos.applyCreateFromBuffer(ctx, "BLI-BUF-ERR", "backlog_item", nil, secCtx)
		assert.Error(t, err)

		// 2. applyUpdateFromBuffer
		dataUpdate := []byte("kind: backlog_item\nid: BLI-BUF-1\ntitle: Buf Item 1 Updated\nstatus: in_progress\ndescription: Buffer item desc\n")
		err = fos.applyUpdateFromBuffer(ctx, "BLI-BUF-1", "backlog_item", dataUpdate, secCtx)
		assert.NoError(t, err)

		// Verify empty data error for update
		err = fos.applyUpdateFromBuffer(ctx, "BLI-BUF-ERR", "backlog_item", nil, secCtx)
		assert.Error(t, err)

		// 3. applyDeleteFromBuffer
		cliCtx := WithCLIOperation(ctx)
		err = fos.applyDeleteFromBuffer(cliCtx, "BLI-BUF-1", "backlog_item", secCtx)
		assert.NoError(t, err)

		// Stream-backed empty data paths (should return nil early)
		err = fos.applyCreateFromBuffer(ctx, "EVT-BUF-1", "audit_event", nil, secCtx)
		assert.NoError(t, err)
		err = fos.applyUpdateFromBuffer(ctx, "EVT-BUF-1", "audit_event", nil, secCtx)
		assert.NoError(t, err)
	})

	t.Run("FileObjectStorage_UpdateNonCASPath_Export", func(t *testing.T) {
		tmpDir := t.TempDir()
		fos, err := NewFileObjectStorageForTest(tmpDir)
		require.NoError(t, err)
		if cleanup := fos.GetTestCleanup(); cleanup != nil {
			defer cleanup()
		}
		defer func() { _ = fos.Shutdown(ctx) }()

		secCtx := pkgctx.NewSystemSecurityContext()
		subDir := filepath.Join(tmpDir, "priority_plans")
		_ = fileutil.MkdirAll(subDir, 0755)
		oldPath := filepath.Join(subDir, "PRI-1.yaml")
		existing := map[string]any{
			objects.FieldKeyID:        "PRI-1",
			objects.FieldKeyKind:      "priority_plan",
			objects.FieldKeyTitle:     "Old Plan",
			objects.FieldKeyUpdatedAt: "2026-01-01T00:00:00Z",
		}
		_ = fileutil.WriteFile(oldPath, []byte("id: PRI-1\nkind: priority_plan\ntitle: Old Plan\nupdated_at: 2026-01-01T00:00:00Z\n"), 0644)

		updates := map[string]any{
			objects.FieldKeyTitle: "Updated Plan Title",
		}

		// UpdateNonCASPathForTest without ID change
		_ = fos.UpdateNonCASPathForTest(ctx, secCtx, "priority_plan", "PRI-1", "PRI-1", false, existing, updates, nil, "", "", updates, "")
	})

	t.Run("OperationExecutor_Cache_Aware_Execution", func(t *testing.T) {
		testRoot, fos, secCtx := SetupTestingFactoryCompleteTestEnvironmentForTest(t)

		queue := NewOperationQueue(nil)
		executor := NewOperationExecutor(ctx, fos, queue, 2, nil)
		executor.SetProjectRoot(testRoot)

		// 1. executeCreateWithCache (create item)
		opCreate := &Operation{
			ID:         "OP-C1",
			Type:       OperationCreate,
			ObjectID:   "BLI-EXEC-1",
			ObjectKind: "backlog_item",
			Context:    ctx,
			SecCtx:     secCtx,
			Data: map[string]any{
				objects.FieldKeyKind:        "backlog_item",
				objects.FieldKeyID:          "BLI-EXEC-1",
				"title":                     "Exec Item 1",
				"status":                    "planned",
				objects.FieldKeyDescription: "Exec Description",
			},
		}
		err := executor.executeCreateWithCache(ctx, opCreate)
		assert.NoError(t, err)

		// Duplicate create fails
		err = executor.executeCreateWithCache(ctx, opCreate)
		assert.Error(t, err)

		// 2. executeUpdateWithCache
		opUpdate := &Operation{
			ID:         "OP-U1",
			Type:       OperationUpdate,
			ObjectID:   "BLI-EXEC-1",
			ObjectKind: "backlog_item",
			Context:    ctx,
			SecCtx:     secCtx,
			Updates: map[string]any{
				"title": "Exec Item 1 Updated",
			},
		}
		err = executor.executeUpdateWithCache(ctx, opUpdate)
		assert.NoError(t, err)

		// Version conflict branch
		opUpdateConflict := &Operation{
			ID:         "OP-U2",
			Type:       OperationUpdate,
			ObjectID:   "BLI-EXEC-1",
			ObjectKind: "backlog_item",
			Context:    ctx,
			SecCtx:     secCtx,
			Metadata: map[string]string{
				opMetadataExpectedUpdated: "1970-01-01T00:00:00Z", // guaranteed mismatch
			},
			Updates: map[string]any{
				"title": "Conflict",
			},
		}
		err = executor.executeUpdateWithCache(ctx, opUpdateConflict)
		assert.ErrorIs(t, err, ErrVersionConflict)

		// 3. executeDeleteWithCache
		cliCtx := pkgctx.WithAllowCoreObjectDelete(WithCLIOperation(ctx))
		opDelete := &Operation{
			ID:         "OP-D1",
			Type:       OperationDelete,
			ObjectID:   "BLI-EXEC-1",
			ObjectKind: "backlog_item",
			Context:    cliCtx,
			SecCtx:     secCtx,
		}
		err = executor.executeDeleteWithCache(cliCtx, opDelete)
		assert.NoError(t, err)

		// Idempotent delete when already deleted
		err = executor.executeDeleteWithCache(cliCtx, opDelete)
		assert.NoError(t, err)
	})

	t.Run("OperationExecutor_executeOperationWithTimeout_and_Stats", func(t *testing.T) {
		testRoot, fos, secCtx := SetupTestingFactoryCompleteTestEnvironmentForTest(t)

		queue := NewOperationQueue(nil)
		executor := NewOperationExecutorWithConfig(ctx, fos, queue, 1, nil, 5*time.Second, nil)
		executor.SetProjectRoot(testRoot)

		opSuccess := &Operation{
			ID:         "OP-T1",
			Type:       OperationCreate,
			ObjectID:   "BLI-TIM-1",
			ObjectKind: "backlog_item",
			Context:    ctx,
			SecCtx:     secCtx,
			Data: map[string]any{
				objects.FieldKeyKind:        "backlog_item",
				objects.FieldKeyID:          "BLI-TIM-1",
				"title":                     "Timeout test item",
				"status":                    "planned",
				objects.FieldKeyDescription: "Item description",
			},
		}
		err := executor.executeOperationWithTimeout(opSuccess)
		assert.NoError(t, err)

		executed, failed := executor.GetOperationExecutorStats()
		assert.Equal(t, int64(1), executed)
		assert.Equal(t, int64(0), failed)

		// Trigger failure
		opFail := &Operation{
			ID:         "OP-T2",
			Type:       OperationType("unknown_op"),
			ObjectID:   "BLI-TIM-2",
			ObjectKind: "backlog_item",
			Context:    ctx,
			SecCtx:     secCtx,
		}
		err = executor.executeOperationWithTimeout(opFail)
		assert.Error(t, err)

		executed, failed = executor.GetOperationExecutorStats()
		assert.Equal(t, int64(1), executed)
		assert.Equal(t, int64(1), failed)
	})
}
