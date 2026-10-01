package storage_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// mockTxWave8 implements ObjectTransaction
type mockTxWave8 struct{}

func (m *mockTxWave8) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	return nil
}
func (m *mockTxWave8) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return nil, nil
}
func (m *mockTxWave8) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	return nil
}
func (m *mockTxWave8) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	return nil
}
func (m *mockTxWave8) Commit(ctx context.Context) error {
	return nil
}
func (m *mockTxWave8) Rollback(ctx context.Context) error {
	return nil
}

// mockStorageProviderWave8 records calls for verification.
type mockStorageProviderWave8 struct {
	storagepkg.NoopObjectStorage
	createdObjects []map[string]any
	readIDs        []string
	updatedIDs     []string
	deletedIDs     []string
	bulkCreated    [][]map[string]any
	bulkUpdated    [][]storagepkg.BulkUpdateItem
	bulkGetIDs     [][]string
	bulkDeleteIDs  [][]string
	moveCalls      []string
	renameCalls    []string
	readReturnObj  map[string]any
}

func newMockStorageProviderWave8() *mockStorageProviderWave8 {
	return &mockStorageProviderWave8{
		NoopObjectStorage: storagepkg.NoopObjectStorage{},
		readReturnObj:     map[string]any{"kind": "goal", "id": "GOAL-001", "status": "active"},
	}
}

func (m *mockStorageProviderWave8) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	m.createdObjects = append(m.createdObjects, obj)
	return nil
}

func (m *mockStorageProviderWave8) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	m.readIDs = append(m.readIDs, id)
	return m.readReturnObj, nil
}

func (m *mockStorageProviderWave8) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	m.updatedIDs = append(m.updatedIDs, id)
	return nil
}

func (m *mockStorageProviderWave8) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	m.deletedIDs = append(m.deletedIDs, id)
	return nil
}

func (m *mockStorageProviderWave8) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storagepkg.ListFilter) (*storagepkg.QueryResult, error) {
	return &storagepkg.QueryResult{Objects: []map[string]any{{"id": "test-1"}}}, nil
}

func (m *mockStorageProviderWave8) Query(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query storagepkg.Query) (*storagepkg.QueryResult, error) {
	return &storagepkg.QueryResult{Objects: []map[string]any{{"id": "query-1"}}}, nil
}

func (m *mockStorageProviderWave8) Search(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query storagepkg.SearchQuery) (*storagepkg.SearchResult, error) {
	return &storagepkg.SearchResult{TotalCount: 1}, nil
}

func (m *mockStorageProviderWave8) BeginTransaction(ctx context.Context) (storagepkg.ObjectTransaction, error) {
	return &mockTxWave8{}, nil
}

func (m *mockStorageProviderWave8) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, objs []map[string]any) (*storagepkg.BulkResult, error) {
	m.bulkCreated = append(m.bulkCreated, objs)
	return &storagepkg.BulkResult{SuccessCount: len(objs)}, nil
}

func (m *mockStorageProviderWave8) BulkUpdate(ctx context.Context, secCtx *pkgctx.SecurityContext, updates []storagepkg.BulkUpdateItem) (*storagepkg.BulkResult, error) {
	m.bulkUpdated = append(m.bulkUpdated, updates)
	return &storagepkg.BulkResult{SuccessCount: len(updates)}, nil
}

func (m *mockStorageProviderWave8) BulkGet(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string) (*storagepkg.BulkResult, error) {
	m.bulkGetIDs = append(m.bulkGetIDs, ids)
	return &storagepkg.BulkResult{SuccessCount: len(ids)}, nil
}

func (m *mockStorageProviderWave8) BulkDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, cascade bool) (*storagepkg.BulkResult, error) {
	m.bulkDeleteIDs = append(m.bulkDeleteIDs, ids)
	return &storagepkg.BulkResult{SuccessCount: len(ids)}, nil
}

func (m *mockStorageProviderWave8) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
	return true, nil
}

func (m *mockStorageProviderWave8) Count(ctx context.Context, secCtx *pkgctx.SecurityContext, filter storagepkg.ListFilter) (int, error) {
	return 42, nil
}

func (m *mockStorageProviderWave8) Aggregate(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storagepkg.ListFilter, aggregations []storagepkg.Aggregation) (*storagepkg.AggregateResult, error) {
	return &storagepkg.AggregateResult{}, nil
}

func (m *mockStorageProviderWave8) GetRelated(ctx context.Context, secCtx *pkgctx.SecurityContext, id, relationshipType string, depth int) ([]map[string]any, error) {
	return []map[string]any{{"id": "related-1"}}, nil
}

func (m *mockStorageProviderWave8) GetPath(ctx context.Context, secCtx *pkgctx.SecurityContext, fromID, toID string) ([]map[string]any, error) {
	return []map[string]any{{"id": fromID}, {"id": toID}}, nil
}

func (m *mockStorageProviderWave8) GetNeighbors(ctx context.Context, secCtx *pkgctx.SecurityContext, id, direction string) ([]map[string]any, error) {
	return []map[string]any{{"id": "neighbor-1"}}, nil
}

func (m *mockStorageProviderWave8) Move(ctx context.Context, secCtx *pkgctx.SecurityContext, id, newKind string, updateReferences bool) error {
	m.moveCalls = append(m.moveCalls, id)
	return nil
}

func (m *mockStorageProviderWave8) Rename(ctx context.Context, secCtx *pkgctx.SecurityContext, oldID, newID string, updateReferences bool) error {
	m.renameCalls = append(m.renameCalls, oldID+"->"+newID)
	return nil
}

// TestStorageExtended_Wave8_RoutingObjectStorage tests routing_storage.go
func TestStorageExtended_Wave8_RoutingObjectStorage(t *testing.T) {
	mockStorage := newMockStorageProviderWave8()
	factory := storagepkg.NewStorageFactoryForTesting(mockStorage)
	require.NotNil(t, factory)

	routing := storagepkg.NewRoutingObjectStorage(factory)
	require.NotNil(t, routing)
	assert.Equal(t, factory, routing.GetStorageFactory())

	// Test GetPool
	pool := routing.GetPool()
	assert.Nil(t, pool)

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	// 1. Create with inferred/explicit kind
	goalObj := map[string]any{
		objects.FieldKeyKind:   "goal",
		objects.FieldKeyID:     "GOAL-ROUTE-001",
		objects.FieldKeyTitle:  "Routing Goal",
		objects.FieldKeyStatus: "originated",
	}
	err := routing.Create(ctx, secCtx, goalObj)
	assert.NoError(t, err)
	assert.Len(t, mockStorage.createdObjects, 1)

	// 2. Read with inferred kind
	readObj, err := routing.Read(ctx, secCtx, "GOAL-ROUTE-001")
	assert.NoError(t, err)
	assert.NotNil(t, readObj)
	assert.Equal(t, "GOAL-001", readObj[objects.FieldKeyID])
	assert.Contains(t, mockStorage.readIDs, "GOAL-ROUTE-001")

	// 3. Update with kind in map and without kind in map
	updateWithKind := map[string]any{
		objects.FieldKeyKind:  "goal",
		objects.FieldKeyTitle: "Routing Goal Updated",
	}
	err = routing.Update(ctx, secCtx, "GOAL-ROUTE-001", updateWithKind)
	assert.NoError(t, err)
	assert.Contains(t, mockStorage.updatedIDs, "GOAL-ROUTE-001")

	updateWithoutKind := map[string]any{
		objects.FieldKeyTitle: "Routing Goal Updated 2",
	}
	err = routing.Update(ctx, secCtx, "GOAL-ROUTE-001", updateWithoutKind)
	assert.NoError(t, err)

	// Update fallback when ID cannot be inferred
	err = routing.Update(ctx, secCtx, "unknown-uninferrable-id", map[string]any{"some_prop": "val"})
	assert.NoError(t, err)

	// 4. Exists
	exists, err := routing.Exists(ctx, secCtx, "GOAL-ROUTE-001")
	assert.NoError(t, err)
	assert.True(t, exists)

	// 5. List
	listRes, err := routing.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{Kind: "goal"})
	assert.NoError(t, err)
	assert.NotNil(t, listRes)

	// 6. Query
	queryRes, err := routing.Query(ctx, secCtx, storageCtx, storagepkg.Query{Kind: "goal"})
	assert.NoError(t, err)
	assert.NotNil(t, queryRes)

	// 7. Search
	searchRes, err := routing.Search(ctx, secCtx, storageCtx, storagepkg.SearchQuery{Query: "Routing"})
	assert.NoError(t, err)
	assert.NotNil(t, searchRes)

	// 8. Count
	cnt, err := routing.Count(ctx, secCtx, storagepkg.ListFilter{Kind: "goal"})
	assert.NoError(t, err)
	assert.Equal(t, 42, cnt)

	// 9. Aggregate
	aggRes, err := routing.Aggregate(ctx, secCtx, storageCtx, storagepkg.ListFilter{Kind: "goal"}, []storagepkg.Aggregation{
		{Field: "status", Function: storagepkg.AggregationCount},
	})
	assert.NoError(t, err)
	assert.NotNil(t, aggRes)

	// 10. Graph navigation: GetRelated, GetPath, GetNeighbors
	related, err := routing.GetRelated(ctx, secCtx, "GOAL-ROUTE-001", "parent", 1)
	assert.NoError(t, err)
	assert.NotNil(t, related)

	path, err := routing.GetPath(ctx, secCtx, "GOAL-ROUTE-001", "GOAL-ROUTE-001")
	assert.NoError(t, err)
	assert.NotNil(t, path)

	neighbors, err := routing.GetNeighbors(ctx, secCtx, "GOAL-ROUTE-001", "outgoing")
	assert.NoError(t, err)
	assert.NotNil(t, neighbors)

	// 11. Bulk methods empty cases
	bCreateRes, err := routing.BulkCreate(ctx, secCtx, []map[string]any{})
	assert.NoError(t, err)
	assert.Equal(t, 0, bCreateRes.SuccessCount)

	bUpdateRes, err := routing.BulkUpdate(ctx, secCtx, []storagepkg.BulkUpdateItem{})
	assert.NoError(t, err)
	assert.Equal(t, 0, bUpdateRes.SuccessCount)

	bGetRes, err := routing.BulkGet(ctx, secCtx, []string{})
	assert.NoError(t, err)
	assert.Equal(t, 0, bGetRes.SuccessCount)

	bDelRes, err := routing.BulkDelete(ctx, secCtx, []string{}, false)
	assert.NoError(t, err)
	assert.Equal(t, 0, bDelRes.SuccessCount)

	// Bulk methods non-empty
	bGetRes2, err := routing.BulkGet(ctx, secCtx, []string{"GOAL-ROUTE-001"})
	assert.NoError(t, err)
	assert.Equal(t, 1, bGetRes2.SuccessCount)

	bCreateRes2, err := routing.BulkCreate(ctx, secCtx, []map[string]any{goalObj})
	assert.NoError(t, err)
	assert.Equal(t, 1, bCreateRes2.SuccessCount)

	bUpdateRes2, err := routing.BulkUpdate(ctx, secCtx, []storagepkg.BulkUpdateItem{{ID: "GOAL-ROUTE-001", Updates: goalObj}})
	assert.NoError(t, err)
	assert.Equal(t, 1, bUpdateRes2.SuccessCount)

	bDelRes2, err := routing.BulkDelete(ctx, secCtx, []string{"GOAL-ROUTE-001"}, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, bDelRes2.SuccessCount)

	// BeginTransaction
	tx, err := routing.BeginTransaction(ctx)
	assert.NoError(t, err)
	if tx != nil {
		_ = tx.Rollback(ctx)
	}

	// 12. Move and Rename
	assert.NoError(t, routing.Move(ctx, secCtx, "GOAL-ROUTE-001", "goal", false))
	assert.Contains(t, mockStorage.moveCalls, "GOAL-ROUTE-001")

	assert.NoError(t, routing.Rename(ctx, secCtx, "GOAL-ROUTE-001", "GOAL-ROUTE-002", false))
	assert.Contains(t, mockStorage.renameCalls, "GOAL-ROUTE-001->GOAL-ROUTE-002")

	// 13. Delete
	err = routing.Delete(ctx, secCtx, "GOAL-ROUTE-001", false)
	assert.NoError(t, err)
	assert.Contains(t, mockStorage.deletedIDs, "GOAL-ROUTE-001")

	// 14. Shutdown
	err = routing.Shutdown(ctx)
	assert.NoError(t, err)
}

// TestStorageExtended_Wave8_BucketingStrategies tests bucketing_strategy.go, bucketing_strategy_chrono.go, bucketing_strategy_defaults.go
func TestStorageExtended_Wave8_BucketingStrategies(t *testing.T) {
	// 1. ChronoBucketStrategy with different granularities
	t.Run("ChronoBucketStrategy", func(t *testing.T) {
		parseRFC3339 := func(s string) (time.Time, error) {
			return time.Parse(time.RFC3339, s)
		}

		chronoMonthly := storagepkg.NewMonthlyChronoStrategy("created_at")
		assert.Equal(t, "chrono-2006-01", chronoMonthly.Name())

		chronoYearly := storagepkg.NewYearlyChronoStrategy("created_at")
		assert.Equal(t, "chrono-2006", chronoYearly.Name())

		testTimeStr := "2026-03-15T14:30:45Z"
		obj := map[string]any{"created_at": testTimeStr}

		// Monthly key & dir
		monthKey := chronoMonthly.GetBucketKey(obj, "")
		assert.Equal(t, "2026-03", monthKey)
		dir := chronoMonthly.GetBucketDirectory("/data/audit", monthKey)
		assert.Contains(t, dir, "2026-03")
		regKey := chronoMonthly.GetHashRegistryKey("audit_event", monthKey)
		assert.Equal(t, "audit_event:chrono:2026-03", regKey)

		// Yearly key
		yearKey := chronoYearly.GetBucketKey(obj, "")
		assert.Equal(t, "2026", yearKey)

		// Granularities
		granularities := []struct {
			granularity string
			expectedDir string
		}{
			{"hourly", "2026-03-15"},
			{"half_hourly", "2026-03-15"},
			{"qtr_hourly", "2026-03-15"},
			{"tenths", "2026-03-15"},
			{"weekly", "2026-W"},
			{"daily", "2026-03-15"},
			{"monthly", "2026-03"},
		}

		for _, g := range granularities {
			strat := &storagepkg.ChronoBucketStrategy{
				Field:       "created_at",
				Granularity: g.granularity,
				ParseFunc:   parseRFC3339,
			}
			assert.Equal(t, fmt.Sprintf("chrono-%s", g.granularity), strat.Name())
			key := strat.GetBucketKey(obj, "")
			assert.NotEmpty(t, key)
			bDir := strat.GetBucketDirectory("/base", key)
			assert.Contains(t, bDir, "/base")

			// Empty key returns baseDir
			emptyDir := strat.GetBucketDirectory("/base", "")
			assert.Equal(t, "/base", emptyDir)
		}

		// Missing field
		missingObj := map[string]any{"other": "value"}
		assert.Empty(t, chronoMonthly.GetBucketKey(missingObj, ""))

		// Invalid field type
		badTypeObj := map[string]any{"created_at": 12345}
		assert.Empty(t, chronoMonthly.GetBucketKey(badTypeObj, ""))

		// Unparseable timestamp fallback to path extraction
		badTimeObj := map[string]any{"created_at": "not-a-date"}
		pathKey := chronoMonthly.GetBucketKey(badTimeObj, "/data/audit/2026-02/event.yaml")
		assert.Equal(t, "2026-02", pathKey)

		// Custom format with T separator
		customStrat := &storagepkg.ChronoBucketStrategy{
			Field:     "created_at",
			Format:    "2006-01-02T15:04",
			ParseFunc: parseRFC3339,
		}
		cKey := customStrat.GetBucketKey(obj, "")
		cDir := customStrat.GetBucketDirectory("/base", cKey)
		assert.Contains(t, cDir, "2026-03-15")
	})

	// 2. StateBucketStrategy
	t.Run("StateBucketStrategy", func(t *testing.T) {
		statusStrat := storagepkg.NewStatusStateStrategy()
		assert.Equal(t, "state-status", statusStrat.Name())

		obj := map[string]any{"status": "in_progress"}
		key := statusStrat.GetBucketKey(obj, "")
		assert.Equal(t, "in_progress", key)

		regKey := statusStrat.GetHashRegistryKey("task", key)
		assert.Equal(t, "task:state:in_progress", regKey)

		dir := statusStrat.GetBucketDirectory("/base", key)
		assert.Contains(t, dir, "in_progress")
		assert.Equal(t, "/base", statusStrat.GetBucketDirectory("/base", ""))

		// Missing/non-string
		assert.Equal(t, "unknown", statusStrat.GetBucketKey(map[string]any{}, ""))
		assert.Equal(t, "unknown", statusStrat.GetBucketKey(map[string]any{"status": 999}, ""))
	})

	// 3. SizeBucketStrategy
	t.Run("SizeBucketStrategy", func(t *testing.T) {
		sizeStrat := &storagepkg.SizeBucketStrategy{
			Field: "file_size",
			Ranges: []storagepkg.SizeRange{
				{Min: 0, Max: 1024, Label: "small"},
				{Min: 1025, Max: 1048576, Label: "medium"},
				{Min: 1048577, Max: 1073741824, Label: "large"},
			},
			Unit: "bytes",
		}
		assert.Equal(t, "size-file_size", sizeStrat.Name())

		assert.Equal(t, "small", sizeStrat.GetBucketKey(map[string]any{"file_size": 500}, ""))
		assert.Equal(t, "medium", sizeStrat.GetBucketKey(map[string]any{"file_size": int64(2000)}, ""))
		assert.Equal(t, "large", sizeStrat.GetBucketKey(map[string]any{"file_size": float64(5000000)}, ""))
		assert.Equal(t, "unknown", sizeStrat.GetBucketKey(map[string]any{"file_size": 2000000000}, "")) // out of range
		assert.Equal(t, "unknown", sizeStrat.GetBucketKey(map[string]any{"file_size": "string-val"}, ""))
		assert.Equal(t, "unknown", sizeStrat.GetBucketKey(map[string]any{}, ""))

		regKey := sizeStrat.GetHashRegistryKey("blob", "medium")
		assert.Equal(t, "blob:size:medium", regKey)

		dir := sizeStrat.GetBucketDirectory("/base", "medium")
		assert.Contains(t, dir, "medium")
		assert.Equal(t, "/base", sizeStrat.GetBucketDirectory("/base", ""))
	})

	// 4. FirstLetterBucketStrategy
	t.Run("FirstLetterBucketStrategy", func(t *testing.T) {
		firstLetter := &storagepkg.FirstLetterBucketStrategy{Field: "title"}
		assert.Equal(t, "first-letter", firstLetter.Name())

		assert.Equal(t, "a", firstLetter.GetBucketKey(map[string]any{"title": "Agent Guidelines"}, ""))
		assert.Equal(t, "p", firstLetter.GetBucketKey(map[string]any{"title": "Process Data"}, ""))
		assert.Equal(t, "0", firstLetter.GetBucketKey(map[string]any{"title": "123 Numbers"}, ""))
		assert.Equal(t, "_", firstLetter.GetBucketKey(map[string]any{"title": "--- special ---"}, ""))
		assert.Equal(t, "_", firstLetter.GetBucketKey(map[string]any{"title": ""}, ""))
		assert.Equal(t, "_", firstLetter.GetBucketKey(map[string]any{}, ""))
		assert.Equal(t, "_", firstLetter.GetBucketKey(map[string]any{"title": 1234}, ""))

		regKey := firstLetter.GetHashRegistryKey("glossary_term", "a")
		assert.Equal(t, "glossary_term:first_letter:a", regKey)

		assert.Equal(t, "/base", firstLetter.GetBucketDirectory("/base", ""))
		assert.Contains(t, firstLetter.GetBucketDirectory("/base", "a"), "a")
	})

	// 5. PathBasedBucketStrategy
	t.Run("PathBasedBucketStrategy", func(t *testing.T) {
		pathStrat := &storagepkg.PathBasedBucketStrategy{BaseDir: "/data/audit"}
		assert.Equal(t, "path-based", pathStrat.Name())

		key := pathStrat.GetBucketKey(nil, "/data/audit/2026-01/event.yaml")
		assert.Equal(t, "2026-01", key)

		// File in base dir directly
		rootKey := pathStrat.GetBucketKey(nil, "/data/audit/event.yaml")
		assert.Equal(t, "", rootKey)

		// Base dir handling
		assert.Equal(t, "/base", pathStrat.GetBucketDirectory("/base", ""))
		assert.Contains(t, pathStrat.GetBucketDirectory("/base", "2026-01"), "2026-01")

		regKey := pathStrat.GetHashRegistryKey("audit_event", "2026-01")
		assert.Equal(t, "audit_event:path:2026-01", regKey)
	})

	// 6. CompositeBucketStrategy
	t.Run("CompositeBucketStrategy", func(t *testing.T) {
		statusStrat := storagepkg.NewStatusStateStrategy()
		firstLetter := &storagepkg.FirstLetterBucketStrategy{Field: "title"}

		comp := &storagepkg.CompositeBucketStrategy{
			Strategies: []storagepkg.BucketStrategy{statusStrat, firstLetter},
			Separator:  "/",
		}
		assert.Contains(t, comp.Name(), "composite[")

		obj := map[string]any{
			"status": "active",
			"title":  "Kernel",
		}
		key := comp.GetBucketKey(obj, "")
		assert.Equal(t, "active/k", key)

		dir := comp.GetBucketDirectory("/base", key)
		assert.Contains(t, dir, "active")
		assert.Equal(t, "/base", comp.GetBucketDirectory("/base", ""))

		regKey := comp.GetHashRegistryKey("custom_kind", key)
		assert.Contains(t, regKey, "custom_kind:composite:active/k")

		// Empty composite
		emptyComp := &storagepkg.CompositeBucketStrategy{}
		assert.Empty(t, emptyComp.GetBucketKey(obj, ""))
	})

	// 7. BucketStrategyRegistry
	t.Run("BucketStrategyRegistry", func(t *testing.T) {
		reg := storagepkg.NewBucketStrategyRegistry()
		statusStrat := storagepkg.NewStatusStateStrategy()
		firstLetter := &storagepkg.FirstLetterBucketStrategy{Field: "title"}

		// No strategy registered
		assert.Empty(t, reg.GetBucketKey("unknown_kind", map[string]any{}, "/data/unknown/file.yaml"))
		hashKey := reg.GetHashRegistryKey("unknown_kind", map[string]any{}, "/data/unknown/file.yaml")
		assert.Contains(t, hashKey, "unknown_kind:")

		// Single strategy registered
		reg.RegisterStrategy("task", statusStrat)
		assert.Len(t, reg.GetStrategies("task"), 1)
		taskObj := map[string]any{"status": "blocked"}
		assert.Equal(t, "blocked", reg.GetBucketKey("task", taskObj, ""))
		assert.Equal(t, "task:state:blocked", reg.GetHashRegistryKey("task", taskObj, ""))

		// Multiple strategies registered
		reg.RegisterStrategy("task", firstLetter)
		assert.Len(t, reg.GetStrategies("task"), 2)
		taskObjMulti := map[string]any{"status": "blocked", "title": "Database Refactor"}
		assert.Equal(t, "blocked/d", reg.GetBucketKey("task", taskObjMulti, ""))
		assert.Contains(t, reg.GetHashRegistryKey("task", taskObjMulti, ""), "task:composite:blocked/d")
	})
}

// TestStorageExtended_Wave8_SnapshotProxy tests snapshot_proxy.go and snapshot_operation_queue.go
func TestStorageExtended_Wave8_SnapshotProxy(t *testing.T) {
	mockUnderlying := newMockStorageProviderWave8()
	queue := storagepkg.NewSnapshotOperationQueue(50)
	proxy := storagepkg.NewProxyStorage(mockUnderlying, queue)
	require.NotNil(t, proxy)

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	// Initially not active
	assert.False(t, proxy.IsSnapshotActive())

	// Normal operations passthrough
	obj := map[string]any{"id": "PASS-001", "kind": "goal"}
	assert.NoError(t, proxy.Create(ctx, secCtx, obj))
	assert.Len(t, mockUnderlying.createdObjects, 1)

	readObj, err := proxy.Read(ctx, secCtx, "PASS-001")
	assert.NoError(t, err)
	assert.NotNil(t, readObj)
	assert.Len(t, mockUnderlying.readIDs, 1)

	assert.NoError(t, proxy.Update(ctx, secCtx, "PASS-001", map[string]any{"title": "Updated"}))
	assert.Len(t, mockUnderlying.updatedIDs, 1)

	assert.NoError(t, proxy.Delete(ctx, secCtx, "PASS-001", false))
	assert.Len(t, mockUnderlying.deletedIDs, 1)

	// Activate snapshot proxy
	proxy.BeginSnapshot()
	assert.True(t, proxy.IsSnapshotActive())

	// Operations should now be queued, NOT hitting mockUnderlying
	createObj := map[string]any{"id": "QUEUED-001", "kind": "goal", "title": "Queued Goal"}
	assert.NoError(t, proxy.Create(ctx, secCtx, createObj))
	assert.Len(t, mockUnderlying.createdObjects, 1) // unchanged
	assert.Equal(t, 1, proxy.GetQueueSize())

	// Read pending write should return queued data
	pendingRead, err := proxy.Read(ctx, secCtx, "QUEUED-001")
	assert.NoError(t, err)
	assert.Equal(t, "Queued Goal", pendingRead["title"])

	// Update queued
	assert.NoError(t, proxy.Update(ctx, secCtx, "QUEUED-001", map[string]any{"title": "Queued Updated"}))
	assert.Equal(t, 2, proxy.GetQueueSize())

	// Read pending write returns latest update
	updatedRead, err := proxy.Read(ctx, secCtx, "QUEUED-001")
	assert.NoError(t, err)
	assert.Equal(t, "Queued Updated", updatedRead["title"])

	// Bulk operations queued
	bulkObjs := []map[string]any{
		{"id": "BULK-1", "kind": "goal"},
		{"id": "BULK-2", "kind": "goal"},
	}
	bRes, err := proxy.BulkCreate(ctx, secCtx, bulkObjs)
	assert.NoError(t, err)
	assert.Equal(t, 2, bRes.SuccessCount)

	bUpdRes, err := proxy.BulkUpdate(ctx, secCtx, []storagepkg.BulkUpdateItem{
		{ID: "BULK-1", Updates: map[string]any{"title": "U1"}},
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, bUpdRes.SuccessCount)

	bDelRes, err := proxy.BulkDelete(ctx, secCtx, []string{"BULK-2"}, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, bDelRes.SuccessCount)

	// Move and Rename queued
	assert.NoError(t, proxy.Move(ctx, secCtx, "QUEUED-001", "milestone", false))
	assert.NoError(t, proxy.Rename(ctx, secCtx, "QUEUED-001", "QUEUED-002", false))

	// Non-proxied operations execute normally
	_, _ = proxy.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{})
	_, _ = proxy.Query(ctx, secCtx, storageCtx, storagepkg.Query{})
	_, _ = proxy.Search(ctx, secCtx, storageCtx, storagepkg.SearchQuery{})
	_, _ = proxy.BeginTransaction(ctx)
	_, _ = proxy.BulkGet(ctx, secCtx, []string{"QUEUED-001"})
	_, _ = proxy.GetRelated(ctx, secCtx, "QUEUED-001", "parent", 1)
	_, _ = proxy.GetPath(ctx, secCtx, "QUEUED-001", "QUEUED-002")
	_, _ = proxy.GetNeighbors(ctx, secCtx, "QUEUED-001", "outgoing")

	// End snapshot
	proxy.EndSnapshot()
	assert.False(t, proxy.IsSnapshotActive())

	// Replay queued operations
	assert.Greater(t, proxy.GetQueueSize(), 0)
	err = proxy.ReplayQueuedOperations(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 0, proxy.GetQueueSize())

	// Queue operations coverage: String, Enqueue overflow, Clear
	q := storagepkg.NewSnapshotOperationQueue(2)
	assert.Equal(t, "create", storagepkg.SnapshotOpCreate.String())
	assert.Equal(t, "update", storagepkg.SnapshotOpUpdate.String())
	assert.Equal(t, "delete", storagepkg.SnapshotOpDelete.String())
	assert.Equal(t, "read", storagepkg.SnapshotOpRead.String())
	assert.Equal(t, "unknown", storagepkg.SnapshotOperationType(99).String())

	assert.NoError(t, q.Enqueue(&storagepkg.SnapshotOperation{Type: storagepkg.SnapshotOpCreate, ObjectID: "Q1"}))
	assert.NoError(t, q.Enqueue(&storagepkg.SnapshotOperation{Type: storagepkg.SnapshotOpUpdate, ObjectID: "Q2"}))
	// Overflow error
	err = q.Enqueue(&storagepkg.SnapshotOperation{Type: storagepkg.SnapshotOpDelete, ObjectID: "Q3"})
	assert.Error(t, err)

	q.Clear()
	assert.Equal(t, 0, q.Size())
}
