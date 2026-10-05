package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
)

// mockConnectionPoolWave20 implements provider.ConnectionPool
type mockConnectionPoolWave20 struct {
	conn provider.GraphConnection
}

func (p *mockConnectionPoolWave20) GetConnection(ctx context.Context) (provider.GraphConnection, error) {
	return p.conn, nil
}

func (p *mockConnectionPoolWave20) ReturnConnection(conn provider.GraphConnection) error {
	return nil
}

func (p *mockConnectionPoolWave20) Close() error {
	return nil
}

func (p *mockConnectionPoolWave20) Execute(ctx context.Context, fn func(conn provider.GraphConnection) error) error {
	return fn(p.conn)
}

func (p *mockConnectionPoolWave20) Stats() provider.PoolStats {
	return provider.PoolStats{Active: 1, MaxSize: 5}
}

func TestStorageExtended_GraphStorage_and_Pool(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	conn := newInMemoryGraphConnWave13()
	pool := &mockConnectionPoolWave20{conn: conn}

	t.Run("NewGraphObjectStorage_and_Helpers", func(t *testing.T) {
		graphStore, err := NewGraphObjectStorage(conn, "")
		require.NoError(t, err)
		require.NotNil(t, graphStore)

		assert.NoError(t, graphStore.Shutdown(ctx))

		// buildCypherCondition with different operators
		for _, op := range []FilterOperator{
			OpEqual, OpNotEqual, OpGreaterThan, OpGreaterThanOrEqual, OpLessThan, OpLessThanOrEqual,
			OpContains, OpStartsWith, OpEndsWith, OpIn, OpNotIn, OpHas, OpExists, OpHasAll, OpHasAny,
			OpIsNull, OpBefore, OpAfter, OpOn, OpOnOrBefore, OpOnOrAfter, OpBetween, OpRegex,
		} {
			cond := graphStore.buildCypherCondition("field", op, "param")
			assert.NotEmpty(t, cond)
		}
		// date field branch (toString)
		dateCond := graphStore.buildCypherCondition("created_at", OpEqual, "param")
		assert.Contains(t, dateCond, "toString")

		// extractNodeFromRow and extractObjectsFromRows
		node := &provider.Node{
			ID:     "BLI-NODE-1",
			Labels: []string{"BacklogItem", "Entity"},
			Properties: map[string]any{
				"id":     "BLI-NODE-1",
				"kind":   "backlog_item",
				"title":  "Node Item",
				"status": "planned",
			},
		}
		rows := []map[string]any{
			{"n": node},
			{"other": node},
			{"empty": nil},
		}
		extracted := graphStore.extractObjectsFromRows(rows)
		assert.NotEmpty(t, extracted)

		// groupObjects & flattenGroups
		objsToGroup := []map[string]any{
			{"id": "1", "status": "planned", "title": "A"},
			{"id": "2", "status": "planned", "title": "B"},
			{"id": "3", "status": "in_progress", "title": "C"},
		}
		grouped := graphStore.groupObjects(objsToGroup, "status", 10)
		assert.Equal(t, 2, len(grouped))
		flattened := graphStore.flattenGroups(grouped)
		assert.Equal(t, 3, len(flattened))

		// groupObjects with maxGroups limit
		limitedGroups := graphStore.groupObjects(objsToGroup, "status", 1)
		assert.Equal(t, 1, len(limitedGroups))

		// inferReferenceKind
		k, id, err := graphStore.inferReferenceKind("account:alice")
		assert.NoError(t, err)
		assert.Equal(t, objects.KindAccount, k)
		assert.Equal(t, "alice", id)

		k, id, err = graphStore.inferReferenceKind("backlog_item:BLI-555")
		assert.NoError(t, err)
		assert.Equal(t, "backlog_item", k)
		assert.Equal(t, "BLI-555", id)

		// bulkDeleteCascadePerID
		bulkRes := &BulkResult{Results: make([]map[string]any, 0)}
		res, err := graphStore.bulkDeleteCascadePerID(ctx, secCtx, []string{"OBJ-DEL-1"}, bulkRes)
		assert.NoError(t, err)
		assert.NotNil(t, res)
	})

	t.Run("PoolAwareGraphStorage_Lifecycle_and_Operations", func(t *testing.T) {
		poolStore := NewPoolAwareGraphStorage(pool, "")
		require.NotNil(t, poolStore)
		assert.Equal(t, pool, poolStore.GetPool())
		assert.NoError(t, poolStore.Shutdown(ctx))

		// BeginTransaction & poolAwareTransaction methods
		txObj, err := poolStore.BeginTransaction(ctx)
		if err == nil && txObj != nil {
			pt, ok := txObj.(*poolAwareTransaction)
			if ok {
				_ = pt.Create(ctx, secCtx, map[string]any{"id": "POOL-1", "kind": "backlog_item"})
				_, _ = pt.Read(ctx, secCtx, "POOL-1")
				_ = pt.Update(ctx, secCtx, "POOL-1", map[string]any{"title": "Updated"})
				_ = pt.Delete(ctx, secCtx, "POOL-1", false)
				_ = pt.Rollback(ctx)
			}
		}
	})

	t.Run("Graph_Hops_NormalizeList_and_Snapshot", func(t *testing.T) {
		// OutboundGraphHops
		hops := OutboundGraphHops("backlog_item", map[string]any{
			"id":          "BLI-1",
			"goal_ref":    "GOAL-1",
			"parent_refs": []string{"BLI-PARENT-1"},
		})
		assert.NotNil(t, hops)
		assert.Nil(t, OutboundGraphHops("backlog_item", nil))

		// NormalizeGraphListField
		objWithSlice := map[string]any{
			"refs": []any{"REF-1", "REF-2"},
		}
		norm := NormalizeGraphListField(objWithSlice, "refs")
		assert.Equal(t, []string{"REF-1", "REF-2"}, norm)
		assert.Nil(t, NormalizeGraphListField(nil, "refs"))
		assert.Nil(t, NormalizeGraphListField(map[string]any{"k": 123}, "k"))

		// handleSecondaryUpdateError
		assert.NoError(t, handleSecondaryUpdateError("ID-1", nil))
		assert.NoError(t, handleSecondaryUpdateError("ID-1", errors.New("object not found")))
		assert.Error(t, handleSecondaryUpdateError("ID-1", errors.New("validation failed: invalid field")))
		assert.Error(t, handleSecondaryUpdateError("ID-1", errors.New("other general error")))
	})
}

func TestStorageExtended_OperationQueue_and_CacheManager(t *testing.T) {
	ctx := context.Background()

	t.Run("OperationQueue_Enqueue_and_GetStatus", func(t *testing.T) {
		queue := NewOperationQueue(nil)
		require.NotNil(t, queue)

		op := &Operation{
			ID:         "OP-123",
			Type:       OpCreate,
			ObjectID:   "BLI-OP-1",
			ObjectKind: "backlog_item",
			Priority:   PriorityHigh,
			Status:     StatusPending,
		}

		err := queue.Enqueue(op)
		assert.NoError(t, err)

		retrieved, err := queue.GetStatus("OP-123")
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, "OP-123", retrieved.ID)

		// Wake callback
		SetExecutorWakeCallback(func() {})
	})

	t.Run("OperationHelper_Execute_and_List", func(t *testing.T) {
		_, fos, secCtx := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
		storageCtx := pkgctx.GetStorageContext()

		// Execute helper
		val, err := Execute(func(c context.Context) (string, error) {
			return "success", nil
		})
		assert.NoError(t, err)
		assert.Equal(t, "success", val)

		// List helper
		res, err := List(fos, secCtx, storageCtx, ListFilter{Kind: "backlog_item", Limit: 5})
		assert.NoError(t, err)
		assert.NotNil(t, res)
	})

	t.Run("CacheManager_InvalidateAsync_and_Cleanup", func(t *testing.T) {
		_, fos, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
		cm := NewCacheManager(ctx, fos)
		require.NotNil(t, cm)

		triggered, completed, failed := cm.GetCacheManagerStats()
		assert.Equal(t, int64(0), triggered)
		assert.Equal(t, int64(0), completed)
		assert.Equal(t, int64(0), failed)

		invID := cm.InvalidateAsync(ctx, []string{"BLI-1"}, "testing")
		assert.NotEmpty(t, invID)

		// Invalidation runs asynchronously
		time.Sleep(50 * time.Millisecond)
		cm.cleanupOldInvalidations()

		status := cm.GetConsistencyStatus()
		assert.NotNil(t, status)
	})

	t.Run("ObjectWriteBehindWorker_ShutdownHandler", func(t *testing.T) {
		worker := &ObjectWriteBehindWorker{
			projectRoot: "/test/root",
			buf:         NewObjectWriteBuffer(),
			stopCh:      make(chan struct{}),
			done:        make(chan struct{}),
		}
		close(worker.done) // Allow Stop() / Drain() to complete

		assert.Equal(t, "write-behind-worker-/test/root", worker.GetName())
		assert.True(t, worker.IsCritical())
		assert.True(t, worker.IsDrained())
		assert.Equal(t, int64(0), worker.GetPendingCount())

		assert.NoError(t, worker.Drain(ctx))
	})

	t.Run("MeshObjectStorage_groupObjects", func(t *testing.T) {
		mesh := &MeshObjectStorage{}
		items := []map[string]any{
			{"kind": "backlog_item", "status": "open"},
			{"kind": "backlog_item", "status": "closed"},
		}
		groups := mesh.groupObjects(items, "status", 5)
		assert.Equal(t, 2, len(groups))
	})

	t.Run("FileObjectStorage_BeginEnhancedTransaction", func(t *testing.T) {
		testRoot, fos, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
		enhancedTx, err := fos.BeginEnhancedTransaction(ctx)
		assert.NoError(t, err)
		assert.NotNil(t, enhancedTx)
		_ = filepath.Join(testRoot, "dummy")
	})
}
