package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// TestStorageExtended_Wave10_HybridObjectStorage tests object_storage_hybrid.go, object_storage_hybrid_bulk.go, object_storage_hybrid_tx.go
func TestStorageExtended_Wave10_HybridObjectStorage(t *testing.T) {
	primary := newMockStorageProviderWave8()
	secondary := newMockStorageProviderWave8()

	hybrid := storagepkg.NewHybridObjectStorage(primary, secondary)
	require.NotNil(t, hybrid)

	assert.Equal(t, primary, hybrid.GetPrimary())
	assert.Equal(t, secondary, hybrid.GetSecondary())

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	// 1. Create - writes to both primary and secondary
	goalObj := map[string]any{
		objects.FieldKeyKind:   "goal",
		objects.FieldKeyID:     "GOAL-HYBRID-001",
		objects.FieldKeyTitle:  "Hybrid Goal",
		objects.FieldKeyStatus: "originated",
	}
	err := hybrid.Create(ctx, secCtx, goalObj)
	assert.NoError(t, err)
	assert.Len(t, primary.createdObjects, 1)
	assert.Len(t, secondary.createdObjects, 1)

	// Create with empty ID
	err = hybrid.Create(ctx, secCtx, map[string]any{"kind": "goal"})
	assert.NoError(t, err)

	// 2. Read - reads from primary with fallback
	readObj, err := hybrid.Read(ctx, secCtx, "GOAL-HYBRID-001")
	assert.NoError(t, err)
	assert.NotNil(t, readObj)
	assert.Contains(t, primary.readIDs, "GOAL-HYBRID-001")

	// Read with empty ID
	_, _ = hybrid.Read(ctx, secCtx, "")

	// 3. Update - normal update both exist
	updateObj := map[string]any{
		objects.FieldKeyTitle: "Hybrid Goal Updated",
	}
	err = hybrid.Update(ctx, secCtx, "GOAL-HYBRID-001", updateObj)
	assert.NoError(t, err)
	assert.Contains(t, primary.updatedIDs, "GOAL-HYBRID-001")
	assert.Contains(t, secondary.updatedIDs, "GOAL-HYBRID-001")

	// 4. Exists
	exists, err := hybrid.Exists(ctx, secCtx, "GOAL-HYBRID-001")
	assert.NoError(t, err)
	assert.True(t, exists)

	// 5. List, Query, Search, Count, Aggregate
	listRes, err := hybrid.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{Kind: "goal"})
	assert.NoError(t, err)
	assert.NotNil(t, listRes)

	// High volume list prefers secondary
	listAudit, err := hybrid.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{Kind: objects.KindAuditEvent})
	assert.NoError(t, err)
	assert.NotNil(t, listAudit)

	queryRes, err := hybrid.Query(ctx, secCtx, storageCtx, storagepkg.Query{Kind: "goal"})
	assert.NoError(t, err)
	assert.NotNil(t, queryRes)

	// High volume query
	queryAudit, err := hybrid.Query(ctx, secCtx, storageCtx, storagepkg.Query{Kind: objects.KindAuditEvent, Type: storagepkg.QueryTypeFilter})
	assert.NoError(t, err)
	assert.NotNil(t, queryAudit)

	searchRes, err := hybrid.Search(ctx, secCtx, storageCtx, storagepkg.SearchQuery{Query: "Hybrid"})
	assert.NoError(t, err)
	assert.NotNil(t, searchRes)

	cnt, err := hybrid.Count(ctx, secCtx, storagepkg.ListFilter{Kind: "goal"})
	assert.NoError(t, err)
	assert.Equal(t, 42, cnt)

	cntAudit, err := hybrid.Count(ctx, secCtx, storagepkg.ListFilter{Kind: objects.KindAuditEvent})
	assert.NoError(t, err)
	assert.Equal(t, 42, cntAudit)

	aggRes, err := hybrid.Aggregate(ctx, secCtx, storageCtx, storagepkg.ListFilter{Kind: "goal"}, []storagepkg.Aggregation{
		{Field: "status", Function: storagepkg.AggregationCount},
	})
	assert.NoError(t, err)
	assert.NotNil(t, aggRes)

	// 6. Graph navigation: GetRelated, GetPath, GetNeighbors
	related, err := hybrid.GetRelated(ctx, secCtx, "GOAL-HYBRID-001", "parent", 1)
	assert.NoError(t, err)
	assert.NotNil(t, related)

	path, err := hybrid.GetPath(ctx, secCtx, "GOAL-HYBRID-001", "GOAL-HYBRID-002")
	assert.NoError(t, err)
	assert.NotNil(t, path)

	neighbors, err := hybrid.GetNeighbors(ctx, secCtx, "GOAL-HYBRID-001", "outgoing")
	assert.NoError(t, err)
	assert.NotNil(t, neighbors)

	// 7. Bulk operations
	bCreateRes, err := hybrid.BulkCreate(ctx, secCtx, []map[string]any{goalObj})
	assert.NoError(t, err)
	assert.Equal(t, 1, bCreateRes.SuccessCount)

	bUpdateRes, err := hybrid.BulkUpdate(ctx, secCtx, []storagepkg.BulkUpdateItem{
		{ID: "GOAL-HYBRID-001", Updates: updateObj},
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, bUpdateRes.SuccessCount)

	bGetRes, err := hybrid.BulkGet(ctx, secCtx, []string{"GOAL-HYBRID-001"})
	assert.NoError(t, err)
	assert.Equal(t, 1, bGetRes.SuccessCount)

	bDelRes, err := hybrid.BulkDelete(ctx, secCtx, []string{"GOAL-HYBRID-001"}, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, bDelRes.SuccessCount)

	// Bulk with empty slices
	_, _ = hybrid.BulkCreate(ctx, secCtx, []map[string]any{})
	_, _ = hybrid.BulkUpdate(ctx, secCtx, []storagepkg.BulkUpdateItem{})
	_, _ = hybrid.BulkDelete(ctx, secCtx, []string{}, false)

	// 8. Move and Rename
	assert.NoError(t, hybrid.Move(ctx, secCtx, "GOAL-HYBRID-001", "milestone", false))
	assert.Contains(t, primary.moveCalls, "GOAL-HYBRID-001")
	assert.Contains(t, secondary.moveCalls, "GOAL-HYBRID-001")

	assert.NoError(t, hybrid.Rename(ctx, secCtx, "GOAL-HYBRID-001", "GOAL-HYBRID-002", false))
	assert.Contains(t, primary.renameCalls, "GOAL-HYBRID-001->GOAL-HYBRID-002")
	assert.Contains(t, secondary.renameCalls, "GOAL-HYBRID-001->GOAL-HYBRID-002")

	// 9. Transaction
	tx, err := hybrid.BeginTransaction(ctx)
	assert.NoError(t, err)
	require.NotNil(t, tx)

	assert.NoError(t, tx.Create(ctx, secCtx, goalObj))
	_, _ = tx.Read(ctx, secCtx, "GOAL-HYBRID-001")
	assert.NoError(t, tx.Update(ctx, secCtx, "GOAL-HYBRID-001", updateObj))
	assert.NoError(t, tx.Delete(ctx, secCtx, "GOAL-HYBRID-001", false))
	assert.NoError(t, tx.Commit(ctx))

	tx2, err := hybrid.BeginTransaction(ctx)
	assert.NoError(t, err)
	assert.NoError(t, tx2.Rollback(ctx))

	// 10. Delete
	assert.NoError(t, hybrid.Delete(ctx, secCtx, "GOAL-HYBRID-001", false))
	assert.Contains(t, primary.deletedIDs, "GOAL-HYBRID-001")
	assert.Contains(t, secondary.deletedIDs, "GOAL-HYBRID-001")

	// 11. Shutdown
	assert.NoError(t, hybrid.Shutdown(ctx))
}
