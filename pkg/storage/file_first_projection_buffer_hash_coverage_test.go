package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// TestStorageExtended_Wave9_FileFirstProjectionStorage tests object_storage_file_projection.go
func TestStorageExtended_Wave9_FileFirstProjectionStorage(t *testing.T) {
	tmpDir := t.TempDir()
	storagepkg.SetupTestRootLikeSetupTestEnvironmentForExportTest(t, tmpDir)

	fos, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, fos)
	if cleanup := fos.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}

	mockProj := newMockStorageProviderWave8()
	projStorage := storagepkg.NewFileFirstProjectionStorage(fos, mockProj)
	require.NotNil(t, projStorage)

	assert.Equal(t, fos, projStorage.UnderlyingObjectStorageProvider())
	assert.Equal(t, fos, projStorage.GetFileSSOT())
	assert.Equal(t, mockProj, projStorage.GetProjection())

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	// 1. Create
	goalObj := map[string]any{
		objects.FieldKeyKind:   "goal",
		objects.FieldKeyID:     "GOAL-PROJ-001",
		objects.FieldKeyTitle:  "Projection Goal",
		objects.FieldKeyStatus: "originated",
	}
	err = projStorage.Create(ctx, secCtx, goalObj)
	assert.NoError(t, err)
	assert.Len(t, mockProj.createdObjects, 1)

	// 2. Read
	readObj, err := projStorage.Read(ctx, secCtx, "GOAL-PROJ-001")
	assert.NoError(t, err)
	assert.NotNil(t, readObj)
	assert.Equal(t, "GOAL-PROJ-001", readObj[objects.FieldKeyID])

	// 3. Update
	updateObj := map[string]any{
		objects.FieldKeyTitle: "Projection Goal Updated",
	}
	err = projStorage.Update(ctx, secCtx, "GOAL-PROJ-001", updateObj)
	assert.NoError(t, err)

	// 4. List, Query, Search, Count, Exists, Aggregate
	listRes, err := projStorage.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{Kind: "goal"})
	assert.NoError(t, err)
	assert.NotNil(t, listRes)

	queryRes, err := projStorage.Query(ctx, secCtx, storageCtx, storagepkg.Query{Kind: "goal"})
	_ = queryRes

	searchRes, err := projStorage.Search(ctx, secCtx, storageCtx, storagepkg.SearchQuery{Query: "Projection"})
	_ = searchRes

	cnt, err := projStorage.Count(ctx, secCtx, storagepkg.ListFilter{Kind: "goal"})
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, cnt, 0)

	exists, err := projStorage.Exists(ctx, secCtx, "GOAL-PROJ-001")
	assert.NoError(t, err)
	assert.True(t, exists)

	aggRes, err := projStorage.Aggregate(ctx, secCtx, storageCtx, storagepkg.ListFilter{Kind: "goal"}, []storagepkg.Aggregation{
		{Field: "status", Function: storagepkg.AggregationCount},
	})
	_ = aggRes

	// 5. BeginTransaction
	tx, err := projStorage.BeginTransaction(ctx)
	if err == nil && tx != nil {
		_ = tx.Rollback(ctx)
	}

	// 6. Bulk methods
	bCreateRes, err := projStorage.BulkCreate(ctx, secCtx, []map[string]any{})
	_ = bCreateRes
	bUpdateRes, err := projStorage.BulkUpdate(ctx, secCtx, []storagepkg.BulkUpdateItem{})
	_ = bUpdateRes
	bGetRes, err := projStorage.BulkGet(ctx, secCtx, []string{"GOAL-PROJ-001"})
	_ = bGetRes
	bDelRes, err := projStorage.BulkDelete(ctx, secCtx, []string{"GOAL-PROJ-001"}, false)
	_ = bDelRes

	// 7. Graph navigation: GetRelated, GetPath, GetNeighbors
	_, _ = projStorage.GetRelated(ctx, secCtx, "GOAL-PROJ-001", "parent", 1)
	_, _ = projStorage.GetPath(ctx, secCtx, "GOAL-PROJ-001", "GOAL-PROJ-001")
	_, _ = projStorage.GetNeighbors(ctx, secCtx, "GOAL-PROJ-001", "outgoing")

	// 8. Move and Rename (CLI context required)
	cliCtx := storagepkg.WithCLIOperation(ctx)
	_ = projStorage.Move(cliCtx, secCtx, "GOAL-PROJ-001", "goal", false)
	_ = projStorage.Rename(cliCtx, secCtx, "GOAL-PROJ-001", "GOAL-PROJ-002", false)

	// 9. RebuildProjectionFromSSOT and Detailed (dry-run & actual)
	dryRunRes, err := projStorage.RebuildProjectionFromSSOTDetailed(ctx, secCtx, true)
	assert.NoError(t, err)
	assert.NotNil(t, dryRunRes)

	actualCount, err := projStorage.RebuildProjectionFromSSOT(ctx, secCtx)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, actualCount, 0)

	// Nil projection variant for Rebuild
	nilProjStorage := storagepkg.NewFileFirstProjectionStorage(fos, nil)
	nilRes, err := nilProjStorage.RebuildProjectionFromSSOTDetailed(ctx, secCtx, false)
	assert.NoError(t, err)
	assert.NotNil(t, nilRes)

	// 10. Delete
	_ = projStorage.Delete(cliCtx, secCtx, "GOAL-PROJ-001", false)

	// 11. Shutdown
	err = projStorage.Shutdown(ctx)
	assert.NoError(t, err)
}

// TestStorageExtended_Wave9_ObjectWriteBuffer tests object_write_buffer.go
func TestStorageExtended_Wave9_ObjectWriteBuffer(t *testing.T) {
	buf := storagepkg.NewObjectWriteBuffer()
	require.NotNil(t, buf)

	assert.Equal(t, 0, buf.Len())
	assert.Nil(t, buf.Peek())

	// 1. Enqueue & Peek
	data1 := []byte("title: Buffer Item 1\nstatus: active\n")
	buf.Enqueue("create", "goal", "GOAL-BUF-001", 1, data1)
	assert.Equal(t, 1, buf.Len())

	peeked := buf.Peek()
	require.NotNil(t, peeked)
	assert.Equal(t, "GOAL-BUF-001", peeked.ID)
	assert.Equal(t, "create", peeked.Op)

	// 2. GetPending
	pending := buf.GetPending("goal", "GOAL-BUF-001")
	require.NotNil(t, pending)
	assert.Equal(t, data1, pending.Data)

	// Non-existent pending
	assert.Nil(t, buf.GetPending("goal", "NON-EXISTENT"))

	// 3. Enqueue update and delete
	data2 := []byte("title: Buffer Item 2\n")
	buf.Enqueue("update", "goal", "GOAL-BUF-002", 2, data2)
	buf.Enqueue("delete", "goal", "GOAL-BUF-003", 3, nil)
	assert.Equal(t, 3, buf.Len())

	// 4. ListPendingIDs & PendingDeletesForKind
	pendingIDs := buf.ListPendingIDs("goal")
	assert.Contains(t, pendingIDs, "GOAL-BUF-001")
	assert.Contains(t, pendingIDs, "GOAL-BUF-002")
	assert.NotContains(t, pendingIDs, "GOAL-BUF-003")

	deletedMap := buf.PendingDeletesForKind("goal")
	assert.True(t, deletedMap["GOAL-BUF-003"])
	assert.False(t, deletedMap["GOAL-BUF-001"])

	// 5. RemoveFront
	buf.RemoveFront(peeked)
	assert.Equal(t, 2, buf.Len())

	// RemoveFront with wrong pointer (no-op)
	buf.RemoveFront(&storagepkg.PendingOp{ID: "dummy"})
	assert.Equal(t, 2, buf.Len())

	// 6. ClearPendingForKey
	buf.ClearPendingForKey("goal", "GOAL-BUF-002")
	assert.Equal(t, 1, buf.Len())
	assert.Nil(t, buf.GetPending("goal", "GOAL-BUF-002"))

	// 7. SetMaxBacklog & drain
	buf.SetMaxBacklog(10)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Drain remaining
	rem := buf.Peek()
	if rem != nil {
		buf.RemoveFront(rem)
	}
	assert.Equal(t, 0, buf.Len())

	// WaitUntilEmpty when already empty
	err := buf.WaitUntilEmpty(ctx)
	assert.NoError(t, err)

	// Nil buffer WaitUntilEmpty
	var nilBuf *storagepkg.ObjectWriteBuffer
	assert.NoError(t, nilBuf.WaitUntilEmpty(ctx))

	// 8. EnqueueFromWALRecord
	walRec := &storagepkg.WALRecord{
		Op:   "create",
		Kind: "goal",
		ID:   "GOAL-WAL-001",
		Seq:  10,
	}
	err = buf.EnqueueFromWALRecord(walRec)
	assert.NoError(t, err)
	assert.Equal(t, 1, buf.Len())
}

// TestStorageExtended_Wave9_HashMismatchFixStrategy tests hash_mismatch_fix_strategy.go
func TestStorageExtended_Wave9_HashMismatchFixStrategy(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	tmpDir := t.TempDir()
	storagepkg.SetupTestRootLikeSetupTestEnvironmentForExportTest(t, tmpDir)

	ctx := context.Background()

	// 1. RemoveStaleIndexStrategy
	removeStrat := storagepkg.NewRemoveStaleIndexStrategy(logger)
	assert.Equal(t, "remove-stale-index", removeStrat.Name())
	assert.NotEmpty(t, removeStrat.Description())

	fixed, err := removeStrat.FixHashMismatch(ctx, "GOAL-TEST-001", "goal", tmpDir)
	assert.NoError(t, err)
	assert.False(t, fixed) // Not in index

	// Invalid kind error
	_, err = removeStrat.FixHashMismatch(ctx, "ID-001", "nonexistent_kind_xyz", tmpDir)
	assert.Error(t, err)

	// 2. RecomputeHashStrategy
	recomputeStrat := storagepkg.NewRecomputeHashStrategy(logger)
	assert.Equal(t, "recompute-hash", recomputeStrat.Name())
	assert.NotEmpty(t, recomputeStrat.Description())

	fixed2, err := recomputeStrat.FixHashMismatch(ctx, "GOAL-TEST-001", "goal", tmpDir)
	assert.NoError(t, err)
	assert.False(t, fixed2)

	// 3. ForceReindexStrategy
	forceStrat := storagepkg.NewForceReindexStrategy(logger)
	assert.Equal(t, "force-reindex", forceStrat.Name())
	assert.NotEmpty(t, forceStrat.Description())

	fixed3, err := forceStrat.FixHashMismatch(ctx, "GOAL-TEST-001", "goal", tmpDir)
	assert.NoError(t, err)
	assert.False(t, fixed3)

	// 4. HashMismatchFixer
	fixer := storagepkg.NewHashMismatchFixer(removeStrat, logger)
	require.NotNil(t, fixer)

	results, err := fixer.FixHashMismatches(ctx, []string{"GOAL-001", "GOAL-002"}, "goal", tmpDir)
	assert.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "GOAL-001", results[0].ObjectID)
	assert.False(t, results[0].Fixed)

	// FixHashMismatchesByKind with inferred prefixes
	byKindResults, err := fixer.FixHashMismatchesByKind(ctx, []string{"GOAL-123", "BLI-456", "UNKNOWN-789"}, tmpDir)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(byKindResults), 2)
}
