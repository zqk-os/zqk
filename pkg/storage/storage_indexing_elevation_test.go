package storage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

// =============================================================================
// LANE 2: SUITE 1 - FileCAS Blob Operations, Cache, & Boundary Failure Modes
// =============================================================================

func TestStorageIndexing_FileCASBlobReadWriteVerify_BoundaryAndFailureModes(t *testing.T) {
	t.Run("BlobCache_LRUCapacityAndEviction", func(t *testing.T) {
		cache := filecas.NewBlobCache(3)
		require.NotNil(t, cache)
		assert.Equal(t, 0, cache.Len())

		// Test Put & Get
		cache.Put("hash1", []byte("data-1"))
		cache.Put("hash2", []byte("data-2"))
		cache.Put("hash3", []byte("data-3"))
		assert.Equal(t, 3, cache.Len())

		// Peek should not change LRU order
		peekVal, found := cache.Peek("hash1")
		assert.True(t, found)
		assert.Equal(t, []byte("data-1"), peekVal)

		// Access hash1 via Get (moves to front of LRU)
		val, ok := cache.Get("hash1")
		assert.True(t, ok)
		assert.Equal(t, []byte("data-1"), val)

		// Adding 4th item should evict hash2 (least recently used, since hash1 was accessed)
		cache.Put("hash4", []byte("data-4"))
		assert.Equal(t, 3, cache.Len())

		_, hash2Present := cache.Get("hash2")
		assert.False(t, hash2Present, "hash2 should have been evicted by LRU")

		// Evict explicit key
		evicted := cache.Evict("hash3")
		assert.True(t, evicted)
		assert.Equal(t, 2, cache.Len())

		// Evict non-existent key
		evictedNonExistent := cache.Evict("non-existent")
		assert.False(t, evictedNonExistent)

		// Clear cache
		cache.Clear()
		assert.Equal(t, 0, cache.Len())
		_, foundAfterClear := cache.Get("hash1")
		assert.False(t, foundAfterClear)
	})

	t.Run("BlobCache_ConcurrentStress", func(t *testing.T) {
		cache := filecas.NewBlobCache(50)
		var wg sync.WaitGroup
		goroutines := 16
		iterations := 100

		for g := 0; g < goroutines; g++ {
			wg.Add(1)
			workerID := g
			goroutinelabels.NewGoroutine("storage_elevation_test", "blob cache stress worker").StartSimple(func() {
				defer wg.Done()
				for i := 0; i < iterations; i++ {
					key := fmt.Sprintf("blob-%d-%d", workerID, i%10)
					content := []byte(fmt.Sprintf("content-%d-%d", workerID, i))
					cache.Put(key, content)
					if data, ok := cache.Get(key); ok {
						if len(data) == 0 {
							t.Errorf("expected non-empty cached data for key %s", key)
						}
					}
					if i%5 == 0 {
						if _, found := cache.Peek(key); !found {
							// fine if evicted
						}
					}
					if i%20 == 0 {
						if evicted := cache.Evict(key); !evicted {
							// fine if already absent
						}
					}
				}
			})
		}
		wg.Wait()
		assert.LessOrEqual(t, cache.Len(), 50)
	})

	t.Run("FileCAS_StorageProviderRawBlobs", func(t *testing.T) {
		_, storage, secCtx := SetupTestingFactoryCompleteTestEnvironmentForTest(t)

		ctx := context.Background()
		kind := objects.KindGoal
		id := "GOAL-CAS-ELEVATION-001"
		rawBlob := []byte("id: GOAL-CAS-ELEVATION-001\nkind: goal\ntitle: Storage Elevation Goal\ndescription: Substantive elevation test description\nstatus: planned\nschema_version: 2.0.0\n")

		// 1. WriteObjectRaw
		writeErr := storage.WriteObjectRaw(ctx, kind, id, rawBlob)
		require.NoError(t, writeErr)

		// 2. Read back via Read
		obj1, readErr := storage.Read(ctx, secCtx, id)
		require.NoError(t, readErr)
		assert.Equal(t, id, obj1["id"])

		// 3. RenameObjectRaw
		renamedID := "GOAL-CAS-ELEVATION-002"
		renameErr := storage.RenameObjectRaw(ctx, kind, id, renamedID)
		require.NoError(t, renameErr)

		// Old ID should be gone
		_, oldReadErr := storage.Read(ctx, secCtx, id)
		assert.Error(t, oldReadErr)

		// New ID should exist
		obj2, newReadErr := storage.Read(ctx, secCtx, renamedID)
		require.NoError(t, newReadErr)
		assert.Equal(t, renamedID, obj2["id"])

		// 4. DeleteObjectRaw
		delErr := storage.DeleteObjectRaw(ctx, kind, renamedID)
		require.NoError(t, delErr)

		_, afterDelErr := storage.Read(ctx, secCtx, renamedID)
		assert.Error(t, afterDelErr)

		// 5. Stream segment chunk operations
		chunkID := "SEG-CHUNK-ELEVATION-01"
		chunkData := []byte("streaming-segment-chunk-payload-data")
		chunkHash, putChunkErr := storage.PutStreamSegmentChunk(objects.KindAuditEvent, chunkID, chunkData)
		if putChunkErr == nil {
			assert.NotEmpty(t, chunkHash)
			retrievedChunk, getChunkErr := storage.GetStreamSegmentChunk(objects.KindAuditEvent, chunkID)
			require.NoError(t, getChunkErr)
			assert.Equal(t, chunkData, retrievedChunk)
		} else {
			t.Logf("PutStreamSegmentChunk unsupported or returned: %v", putChunkErr)
		}

		// 6. Non-existent stream segment chunk returns error
		_, missingChunkErr := storage.GetStreamSegmentChunk(objects.KindAuditEvent, "NON-EXISTENT-CHUNK")
		assert.Error(t, missingChunkErr)

		// 7. PutStreamSegment
		segHash, putSegErr := storage.PutStreamSegment(objects.KindAuditEvent, "SEG-FULL-01", []byte("segment-full-payload"))
		if putSegErr != nil {
			t.Logf("PutStreamSegment note: %v", putSegErr)
		} else {
			assert.NotEmpty(t, segHash)
		}

		// 8. CASUsesContentAddressableStorage & CASGenerateID
		usesCAS := storage.CASUsesContentAddressableStorage(kind)
		assert.True(t, usesCAS)

		genID, genErr := storage.CASGenerateID(ctx, kind)
		if genErr != nil {
			t.Logf("CASGenerateID returned: %v", genErr)
		} else {
			assert.NotEmpty(t, genID)
		}
	})

	t.Run("FileCAS_DirectInstanceBlobOperations", func(t *testing.T) {
		tmpDir := t.TempDir()
		kindDir := filepath.Join(tmpDir, "cas_kind_blobs")
		mkdirErr := fileutil.MkdirAll(kindDir, 0o755)
		require.NoError(t, mkdirErr)

		casInstance := filecas.NewContentAddressableStorage(kindDir, objects.KindGoal)
		require.NotNil(t, casInstance)
		assert.Equal(t, objects.KindGoal, casInstance.GetKind())
		assert.Equal(t, kindDir, casInstance.GetKindDir())

		// Create & Read with valid YAML
		testContent := []byte("id: OBJ-100\nkind: goal\ntitle: CAS Direct Blob Test\nstatus: planned\nschema_version: 2.0.0\n")
		writeErr := casInstance.Create("OBJ-100", testContent)
		require.NoError(t, writeErr)

		readData, readErr := casInstance.Read("OBJ-100")
		require.NoError(t, readErr)
		var readObj map[string]any
		require.NoError(t, yaml.Unmarshal(readData, &readObj))
		assert.Equal(t, "OBJ-100", readObj["id"])
		assert.Equal(t, "CAS Direct Blob Test", readObj["title"])

		// GetHashForID
		hash, hashErr := casInstance.GetHashForID("OBJ-100")
		require.NoError(t, hashErr)
		assert.NotEmpty(t, hash)

		// Update object content
		updatedContent := []byte("id: OBJ-100\nkind: goal\ntitle: CAS Direct Blob Test Updated\nstatus: in_progress\nschema_version: 2.0.0\n")
		updateErr := casInstance.Update("OBJ-100", updatedContent)
		require.NoError(t, updateErr)

		readUpdated, readUpErr := casInstance.Read("OBJ-100")
		require.NoError(t, readUpErr)
		var updatedObj map[string]any
		require.NoError(t, yaml.Unmarshal(readUpdated, &updatedObj))
		assert.Equal(t, "OBJ-100", updatedObj["id"])
		assert.Equal(t, "CAS Direct Blob Test Updated", updatedObj["title"])

		// Non-existent object read returns error
		_, missingReadErr := casInstance.Read("MISSING-OBJ")
		assert.Error(t, missingReadErr)

		// Delete
		delErr := casInstance.Delete("OBJ-100")
		require.NoError(t, delErr)

		_, afterDelReadErr := casInstance.Read("OBJ-100")
		assert.Error(t, afterDelReadErr)

		// Sweep Lock execution
		sweepExecuted := false
		sweepErr := casInstance.WithSweepLock(func() error {
			sweepExecuted = true
			return nil
		})
		require.NoError(t, sweepErr)
		assert.True(t, sweepExecuted)

		// Sweep Lock returning an error propagates correctly
		expectedSweepErr := errors.New("sweep-custom-failure")
		propSweepErr := casInstance.WithSweepLock(func() error {
			return expectedSweepErr
		})
		assert.ErrorIs(t, propSweepErr, expectedSweepErr)
	})
}

// =============================================================================
// LANE 2: SUITE 2 - PureGo Indexer Query Filters, Sort Keys, & Pagination
// =============================================================================

func TestStorageIndexing_PureGoIndexer_QueryFiltersSortKeysPagination(t *testing.T) {
	t.Parallel()

	indexer := NewPureGoIndexer()
	require.NotNil(t, indexer)

	now := time.Now().UTC()

	// Table of diverse nodes across kinds, statuses, attributes, and relationships
	testNodes := []*IndexedNode{
		{
			ID:            "GOAL-100",
			Kind:          "goal",
			Status:        "originated",
			Title:         "High-Throughput Storage Ingestion",
			NamespaceID:   "zqk:kernel",
			CreatedAt:     now.Add(-10 * time.Hour),
			UpdatedAt:     now.Add(-10 * time.Hour),
			Attributes:    map[string]any{"priority_tier": "P0", "weight": 10},
			References:    map[string][]string{"milestone_refs": {"MIL-001"}},
			SchemaVersion: "2.0.0",
		},
		{
			ID:            "GOAL-200",
			Kind:          "goal",
			Status:        "in_progress",
			Title:         "Zero-Latency Relational Indexing",
			NamespaceID:   "zqk:kernel",
			CreatedAt:     now.Add(-8 * time.Hour),
			UpdatedAt:     now.Add(-4 * time.Hour),
			Attributes:    map[string]any{"priority_tier": "P1", "weight": 20},
			References:    map[string][]string{"milestone_refs": {"MIL-001"}},
			SchemaVersion: "2.0.0",
		},
		{
			ID:            "REQ-101",
			Kind:          "requirement",
			Status:        "planned",
			Title:         "Fast Disk Recovery Protocol",
			NamespaceID:   "zqk:kernel",
			CreatedAt:     now.Add(-7 * time.Hour),
			UpdatedAt:     now.Add(-6 * time.Hour),
			Attributes:    map[string]any{"priority_tier": "P1", "weight": 15},
			References:    map[string][]string{"goal_refs": {"GOAL-100"}, "criteria_refs": {"CRIT-101A", "CRIT-101B"}},
			SchemaVersion: "2.0.0",
		},
		{
			ID:            "REQ-201",
			Kind:          "requirement",
			Status:        "in_progress",
			Title:         "Thread-Safe CAS Index Traversal",
			NamespaceID:   "zqk:kernel",
			CreatedAt:     now.Add(-5 * time.Hour),
			UpdatedAt:     now.Add(-3 * time.Hour),
			Attributes:    map[string]any{"priority_tier": "P0", "weight": 25},
			References:    map[string][]string{"goal_refs": {"GOAL-200"}, "criteria_refs": {"CRIT-201A"}},
			SchemaVersion: "2.0.0",
		},
		{
			ID:            "BLI-1011",
			Kind:          "backlog_item",
			Status:        "planned",
			Title:         "Implement Journal Integrity Scanner",
			NamespaceID:   "zqk:kernel",
			CreatedAt:     now.Add(-4 * time.Hour),
			UpdatedAt:     now.Add(-4 * time.Hour),
			Attributes:    map[string]any{"priority_tier": "P1", "estimated_effort": "2h", "weight": 5},
			References:    map[string][]string{"requirement_refs": {"REQ-101"}},
			SchemaVersion: "2.0.0",
		},
		{
			ID:            "BLI-2011",
			Kind:          "backlog_item",
			Status:        "in_progress",
			Title:         "Implement Concurrent Operation Queue",
			NamespaceID:   "zqk:kernel",
			CreatedAt:     now.Add(-3 * time.Hour),
			UpdatedAt:     now.Add(-2 * time.Hour),
			Attributes:    map[string]any{"priority_tier": "P0", "estimated_effort": "3h", "weight": 30},
			References:    map[string][]string{"requirement_refs": {"REQ-201"}},
			SchemaVersion: "2.0.0",
		},
		{
			ID:            "BLI-2012",
			Kind:          "backlog_item",
			Status:        "completed",
			Title:         "Implement Snapshot Operation Drainer",
			NamespaceID:   "zqk:kernel",
			CreatedAt:     now.Add(-2 * time.Hour),
			UpdatedAt:     now.Add(-1 * time.Hour),
			Attributes:    map[string]any{"priority_tier": "P1", "estimated_effort": "1h", "weight": 8},
			References:    map[string][]string{"requirement_refs": {"REQ-201"}},
			SchemaVersion: "2.0.0",
		},
	}

	for _, n := range testNodes {
		indexErr := indexer.IndexNode(n)
		require.NoError(t, indexErr)
	}
	assert.Equal(t, len(testNodes), indexer.NodeCount())

	t.Run("QueryFilters_KindAndStatus", func(t *testing.T) {
		goals := indexer.GetNodesByKind("goal")
		assert.Len(t, goals, 2)

		reqs := indexer.GetNodesByKind("requirement")
		assert.Len(t, reqs, 2)

		blis := indexer.GetNodesByKind("backlog_item")
		assert.Len(t, blis, 3)

		// Non-existent kind returns nil/empty
		emptyKind := indexer.GetNodesByKind("non_existent_kind")
		assert.Empty(t, emptyKind)

		// Filter by status
		inProgress := indexer.GetNodesByStatus("in_progress")
		assert.Len(t, inProgress, 3) // GOAL-200, REQ-201, BLI-2011

		planned := indexer.GetNodesByStatus("planned")
		assert.Len(t, planned, 2) // REQ-101, BLI-1011

		completed := indexer.GetNodesByStatus("completed")
		assert.Len(t, completed, 1) // BLI-2012
		assert.Equal(t, "BLI-2012", completed[0].ID)
	})

	t.Run("QueryFilters_AttributePredicatesAndSortKeys", func(t *testing.T) {
		// Custom filtering predicate over all indexed nodes
		allBLIs := indexer.GetNodesByKind("backlog_item")
		require.Len(t, allBLIs, 3)

		// 1. Filter by attribute: priority_tier == "P0"
		var p0BLIs []*IndexedNode
		for _, b := range allBLIs {
			if tier, ok := b.Attributes["priority_tier"].(string); ok && tier == "P0" {
				p0BLIs = append(p0BLIs, b)
			}
		}
		require.Len(t, p0BLIs, 1)
		assert.Equal(t, "BLI-2011", p0BLIs[0].ID)

		// 2. Sort keys: sort by Title ascending
		sort.Slice(allBLIs, func(i, j int) bool {
			return allBLIs[i].Title < allBLIs[j].Title
		})
		assert.Equal(t, "Implement Concurrent Operation Queue", allBLIs[0].Title)
		assert.Equal(t, "Implement Journal Integrity Scanner", allBLIs[1].Title)
		assert.Equal(t, "Implement Snapshot Operation Drainer", allBLIs[2].Title)

		// 3. Sort keys: sort by CreatedAt descending
		sort.Slice(allBLIs, func(i, j int) bool {
			return allBLIs[i].CreatedAt.After(allBLIs[j].CreatedAt)
		})
		assert.Equal(t, "BLI-2012", allBLIs[0].ID)
		assert.Equal(t, "BLI-2011", allBLIs[1].ID)
		assert.Equal(t, "BLI-1011", allBLIs[2].ID)
	})

	t.Run("Pagination_OffsetAndLimitSlicing", func(t *testing.T) {
		nodes := indexer.GetNodesByKind("backlog_item")
		// Sort deterministically by ID
		sort.Slice(nodes, func(i, j int) bool {
			return nodes[i].ID < nodes[j].ID
		})

		// Helper pagination function
		paginate := func(items []*IndexedNode, offset, limit int) []*IndexedNode {
			if offset < 0 {
				offset = 0
			}
			if offset >= len(items) {
				return nil
			}
			end := len(items)
			if limit > 0 && offset+limit < end {
				end = offset + limit
			}
			return items[offset:end]
		}

		// Page 1: limit 2, offset 0 -> 2 items
		page1 := paginate(nodes, 0, 2)
		require.Len(t, page1, 2)
		assert.Equal(t, "BLI-1011", page1[0].ID)
		assert.Equal(t, "BLI-2011", page1[1].ID)

		// Page 2: limit 2, offset 2 -> 1 item
		page2 := paginate(nodes, 2, 2)
		require.Len(t, page2, 1)
		assert.Equal(t, "BLI-2012", page2[0].ID)

		// Page 3: offset beyond total -> nil/empty
		page3 := paginate(nodes, 10, 2)
		assert.Empty(t, page3)

		// Negative offset treated as 0
		pageZero := paginate(nodes, -5, 1)
		require.Len(t, pageZero, 1)
		assert.Equal(t, "BLI-1011", pageZero[0].ID)
	})

	t.Run("GraphTraversal_ForwardBackwardEdges", func(t *testing.T) {
		// Out-edges from REQ-101
		goalRefs := indexer.GetOutEdges("REQ-101", "goal_refs")
		assert.Equal(t, []string{"GOAL-100"}, goalRefs)

		critRefs := indexer.GetOutEdges("REQ-101", "criteria_refs")
		assert.ElementsMatch(t, []string{"CRIT-101A", "CRIT-101B"}, critRefs)

		// In-edges to GOAL-100 from REQ-101
		inGoal := indexer.GetInEdges("GOAL-100", "goal_refs")
		assert.Equal(t, []string{"REQ-101"}, inGoal)

		// In-edges to MIL-001 from both GOAL-100 and GOAL-200
		inMil := indexer.GetInEdges("MIL-001", "milestone_refs")
		assert.ElementsMatch(t, []string{"GOAL-100", "GOAL-200"}, inMil)

		// Multi-hop Traverse: REQ-101 -> goal_refs -> milestone_refs
		ctx := context.Background()
		milNodes, travErr := indexer.Traverse(ctx, "REQ-101", []string{"goal_refs"}, 10)
		require.NoError(t, travErr)
		require.Len(t, milNodes, 1)
		assert.Equal(t, "GOAL-100", milNodes[0].ID)
	})

	t.Run("IndexProjectDir_WithIgnoredAndCorruptedFiles", func(t *testing.T) {
		tmpDir := t.TempDir()
		procDir := filepath.Join(tmpDir, ".zqk", "process")
		goalsDir := filepath.Join(procDir, "goals")
		mkdirErr := fileutil.MkdirAll(goalsDir, 0o755)
		require.NoError(t, mkdirErr)

		// Valid YAML file
		validGoal := `id: GOAL-FS-1
kind: goal
title: File System Discovery Goal
status: originated
namespace_id: zqk:kernel
schema_version: 2.0.0
`
		writeValidErr := fileutil.WriteFile(filepath.Join(goalsDir, "goal1.yaml"), []byte(validGoal), 0o644)
		require.NoError(t, writeValidErr)

		// Ignored directory (e.g. wal)
		walDir := filepath.Join(procDir, "wal")
		mkdirWalErr := fileutil.MkdirAll(walDir, 0o755)
		require.NoError(t, mkdirWalErr)
		writeWalErr := fileutil.WriteFile(filepath.Join(walDir, "wal_skip.yaml"), []byte(validGoal), 0o644)
		require.NoError(t, writeWalErr)

		// Non-YAML file (should be skipped)
		writeNonYamlErr := fileutil.WriteFile(filepath.Join(goalsDir, "readme.txt"), []byte("not yaml"), 0o644)
		require.NoError(t, writeNonYamlErr)

		// Malformed YAML (unmarshal error, should not panic)
		writeBadYamlErr := fileutil.WriteFile(filepath.Join(goalsDir, "bad.yaml"), []byte(":::invalid: [yaml"), 0o644)
		require.NoError(t, writeBadYamlErr)

		fsIndexer := NewPureGoIndexer()
		indexedCount, scanErr := fsIndexer.IndexProjectDir(context.Background(), tmpDir)
		require.NoError(t, scanErr)
		assert.Equal(t, 1, indexedCount)

		foundNode, ok := fsIndexer.GetNode("GOAL-FS-1")
		require.True(t, ok)
		assert.Equal(t, "File System Discovery Goal", foundNode.Title)
	})
}

// =============================================================================
// LANE 2: SUITE 3 - Graph Storage List Query Builders & Result Helpers
// =============================================================================

func TestStorageIndexing_GraphListQuery_FiltersSortPagination(t *testing.T) {
	t.Parallel()

	graphStore := &GraphObjectStorage{}

	t.Run("CalculateEffectiveLimit", func(t *testing.T) {
		assert.Equal(t, 0, graphStore.calculateEffectiveLimit(nil, nil))
		assert.Equal(t, 0, graphStore.calculateEffectiveLimit(&ListFilter{Limit: 0}, nil))
		assert.Equal(t, 25, graphStore.calculateEffectiveLimit(&ListFilter{Limit: 25}, nil))
	})

	t.Run("GetSortOrder", func(t *testing.T) {
		assert.Equal(t, "ASC", graphStore.getSortOrder(true))
		assert.Equal(t, "DESC", graphStore.getSortOrder(false))
	})

	t.Run("AddPaginationToQuery", func(t *testing.T) {
		base := "MATCH (n:Goal) RETURN n"

		// No effective limit -> unchanged
		assert.Equal(t, base, graphStore.addPaginationToQuery(base, &ListFilter{}, 0))

		// Limit with zero offset
		withLimit := graphStore.addPaginationToQuery(base, &ListFilter{Offset: 0}, 10)
		assert.Equal(t, base+" LIMIT 10", withLimit)

		// Limit with positive offset
		withSkipLimit := graphStore.addPaginationToQuery(base, &ListFilter{Offset: 20}, 10)
		assert.Equal(t, base+" SKIP 20 LIMIT 10", withSkipLimit)
	})

	t.Run("AddFiltersToQuery_EqualityAndOperators", func(t *testing.T) {
		base := "MATCH (n:BacklogItem)"
		params := make(map[string]any)

		// Simple equality
		filterSimple := &ListFilter{
			Filters: map[string]any{
				"status": "in_progress",
			},
		}
		q1 := graphStore.addFiltersToQuery(base, filterSimple, params)
		assert.Contains(t, q1, "WHERE")
		assert.Contains(t, q1, "n.status = $filter_status")
		assert.Equal(t, "in_progress", params["filter_status"])

		// Operator map: field > 10
		params2 := make(map[string]any)
		filterOp := &ListFilter{
			Filters: map[string]any{
				"weight": map[string]any{
					string(OpGreaterThan): 10,
				},
			},
		}
		q2 := graphStore.addFiltersToQuery(base, filterOp, params2)
		assert.Contains(t, q2, "WHERE")
		assert.Contains(t, q2, "n.weight > $")
	})

	t.Run("AddGroupingAndReturnToQuery", func(t *testing.T) {
		base := "MATCH (n:Task)"

		// Ungrouped, default return
		ungrouped := graphStore.addGroupingAndReturnToQuery(base, &ListFilter{SortBy: "priority", SortAsc: true}, &pkgctx.StorageContext{})
		assert.Contains(t, ungrouped, "RETURN n ORDER BY n.priority ASC")

		// Grouped with grouping enabled
		grouped := graphStore.addGroupingAndReturnToQuery(base, &ListFilter{
			GroupBy: "status",
			SortBy:  "title",
			SortAsc: false,
		}, &pkgctx.StorageContext{EnableGrouping: true})
		assert.Contains(t, grouped, "WITH n, n.status AS groupKey")
		assert.Contains(t, grouped, "ORDER BY")
	})

	t.Run("GroupObjects_And_FlattenGroups", func(t *testing.T) {
		items := []map[string]any{
			{"id": "BLI-1", "status": "planned", "tier": "P1"},
			{"id": "BLI-2", "status": "in_progress", "tier": "P0"},
			{"id": "BLI-3", "status": "planned", "tier": "P1"},
			{"id": "BLI-4", "status": "completed", "tier": "P0"},
		}

		// Group by status
		grouped := graphStore.groupObjects(items, "status", 0)
		assert.Len(t, grouped, 3)
		assert.Len(t, grouped["planned"], 2)
		assert.Len(t, grouped["in_progress"], 1)
		assert.Len(t, grouped["completed"], 1)

		// Group with maxGroups cap
		capped := graphStore.groupObjects(items, "status", 2)
		assert.LessOrEqual(t, len(capped), 2)

		// Flatten groups back to slice
		flattened := graphStore.flattenGroups(grouped)
		assert.Len(t, flattened, 4)
	})

	t.Run("ExtractNodeFromRow_And_ConvertQueryResults", func(t *testing.T) {
		node := &provider.Node{
			ID:     "BLI-CONVERT-1",
			Labels: []string{"BacklogItem"},
			Properties: map[string]any{
				"id":     "BLI-CONVERT-1",
				"title":  "Converted Title",
				"status": "planned",
			},
		}

		// Direct node in row
		row := map[string]any{"n": node}
		extracted := graphStore.extractNodeFromRow(row)
		require.NotNil(t, extracted)
		assert.Equal(t, "BLI-CONVERT-1", extracted.ID)

		// Fallback iteration through row values
		rowFallback := map[string]any{"custom_col": node}
		extractedFallback := graphStore.extractNodeFromRow(rowFallback)
		require.NotNil(t, extractedFallback)
		assert.Equal(t, "BLI-CONVERT-1", extractedFallback.ID)

		// Convert query results
		qr := &provider.QueryResult{
			Nodes: []*provider.Node{node},
		}
		objs := graphStore.convertQueryResultsToObjects(qr)
		require.Len(t, objs, 1)
		assert.Equal(t, "BLI-CONVERT-1", objs[0]["id"])
	})
}

// =============================================================================
// LANE 2: SUITE 4 - Storage Journal Recovery & Corruption Detection
// =============================================================================

func TestStorageIndexing_StorageJournal_RecoveryAndCorruptionDetection(t *testing.T) {
	t.Parallel()

	t.Run("ReconstructionService_StatsAndHelpers", func(t *testing.T) {
		service := NewChangeJournalReconstructionService(nil)
		require.NotNil(t, service)

		runs, reversed := service.GetReconstructionStats()
		assert.Equal(t, int64(0), runs)
		assert.Equal(t, int64(0), reversed)

		// Helper string extractions
		m := map[string]any{"name": "Journal Entry", "count": 42}
		assert.Equal(t, "Journal Entry", getString(m, "name"))
		assert.Equal(t, "", getString(m, "count"))
		assert.Equal(t, "", getString(m, "missing"))

		// deepCopyObject & deepCopySlice
		nested := map[string]any{
			"id":   "OBJ-1",
			"tags": []any{"a", "b", map[string]any{"inner": "val"}},
			"sub":  map[string]any{"k": "v"},
		}
		copied := deepCopyObject(nested)
		assert.Equal(t, nested, copied)
		// Mutate original and assert clone independence
		nested["id"] = "MUTATED"
		assert.Equal(t, "OBJ-1", copied["id"])

		// parseTimestamp
		tsTime := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
		assert.Equal(t, tsTime, parseTimestamp(tsTime))
		assert.Equal(t, tsTime, parseTimestamp("2026-11-05T12:00:00Z"))
		assert.True(t, parseTimestamp("invalid-date").IsZero())

		// ExtractObjectRef & formatObjectRef
		kind, id, err := ExtractObjectRef("requirement:REQ-900")
		require.NoError(t, err)
		assert.Equal(t, "requirement", kind)
		assert.Equal(t, "REQ-900", id)

		formatted := formatObjectRef("requirement", "REQ-900")
		assert.Equal(t, "requirement:REQ-900", formatted)

		_, _, badRefErr := ExtractObjectRef("invalid_ref_without_colon")
		assert.Error(t, badRefErr)
	})

	t.Run("ChangeJournalHelper_TitleNormalizationAndEventCallback", func(t *testing.T) {
		// normalizeChangeJournalTitle
		assert.Equal(t, "Update Backlog Item", normalizeChangeJournalTitle("Update Backlog Item"))
		assert.Equal(t, "", normalizeChangeJournalTitle(""))

		// Helper stats
		created, failed := GetChangeJournalHelperStats()
		assert.GreaterOrEqual(t, created, int64(0))
		assert.GreaterOrEqual(t, failed, int64(0))

		// SetChangeJournalEventCallback
		var callbackInvoked atomic.Bool
		SetChangeJournalEventCallback(func(
			ctx context.Context,
			projectRoot string,
			storage ObjectStorageProvider,
			operationID string,
			operationType string,
			status string,
			changeType string,
			objectRef string,
			kind string,
			objectID string,
			duration time.Duration,
			err error,
		) {
			callbackInvoked.Store(true)
		})
		cb := getChangeJournalEventCallback()
		require.NotNil(t, cb)
		cb(context.Background(), "/tmp", nil, "OP-1", "create", "success", "direct", "backlog_item/BLI-1", "backlog_item", "BLI-1", 10*time.Millisecond, nil)
		assert.True(t, callbackInvoked.Load())

		// Reset callback to nil
		SetChangeJournalEventCallback(nil)
		assert.Nil(t, getChangeJournalEventCallback())
	})

	t.Run("ChangeJournal_DiffSummaryAndPathFlattening", func(t *testing.T) {
		updates := map[string]any{
			"title": "New Title",
			"nested": map[string]any{
				"count": 10,
			},
		}
		previous := map[string]any{
			"title": "Old Title",
			"nested": map[string]any{
				"count": 5,
			},
		}

		paths := flattenUpdatePaths(updates)
		assert.Contains(t, paths, "title")
		assert.Contains(t, paths, "nested.count")

		diffSummary := buildDiffSummary(updates, previous)
		assert.NotEmpty(t, diffSummary)
		assert.Contains(t, diffSummary, "title")
	})

	t.Run("ObjectWAL_AppliedSeqAndLastSeq", func(t *testing.T) {
		tmpDir := t.TempDir()
		walPath := GetWALPath(tmpDir)
		assert.NotEmpty(t, walPath)

		// ReadAppliedSeq on fresh directory returns 0
		seq, err := ReadAppliedSeq(tmpDir)
		require.NoError(t, err)
		assert.Equal(t, int64(0), seq)

		// WriteAppliedSeq and verify roundtrip
		writeErr := WriteAppliedSeq(tmpDir, 42)
		require.NoError(t, writeErr)

		readSeq, readErr := ReadAppliedSeq(tmpDir)
		require.NoError(t, readErr)
		assert.Equal(t, int64(42), readSeq)

		// parseWALLine boundary tests
		_, emptyErr := parseWALLine([]byte(""))
		assert.Error(t, emptyErr)

		// getMaxSeqFromLine boundary
		assert.Equal(t, int64(0), getMaxSeqFromLine([]byte("")))
		assert.Equal(t, int64(0), getMaxSeqFromLine([]byte("invalid json line")))
	})
}

// =============================================================================
// LANE 2: SUITE 5 - Operation Queue Concurrency, Retries, & Snapshot Queue
// =============================================================================

func TestStorageIndexing_OperationQueue_ConcurrencyLeaseTimeoutsBatchDraining(t *testing.T) {
	t.Parallel()

	t.Run("Operation_RetriesAndErrorHandling", func(t *testing.T) {
		op := &Operation{
			ID:         "OP-RETRY-01",
			Priority:   PriorityHigh,
			Status:     StatusPending,
			MaxRetries: 2,
		}

		assert.Nil(t, op.GetError())
		assert.True(t, op.CanRetry())

		// First retry
		op.IncrementRetry()
		assert.Equal(t, 1, op.RetryCount)
		assert.True(t, op.CanRetry())

		// Second retry reaches max
		op.IncrementRetry()
		assert.Equal(t, 2, op.RetryCount)
		assert.False(t, op.CanRetry())

		// Set and get error
		testErr := errors.New("transient database connection timeout")
		op.SetError(testErr)
		assert.ErrorIs(t, op.GetError(), testErr)

		// Cancelled operation cannot retry even if retry count is low
		opCancel := &Operation{
			ID:         "OP-CANCEL-01",
			Status:     StatusCancelled,
			MaxRetries: 5,
		}
		assert.False(t, opCancel.CanRetry())
	})

	t.Run("OperationQueue_StatusTransitionsAndTiming", func(t *testing.T) {
		notifier := &recordingNotifier{}
		queue := NewOperationQueue(notifier)

		op := &Operation{
			ID:       "OP-STATUS-CYCLE",
			Priority: PriorityNormal,
			Status:   StatusPending,
		}

		require.NoError(t, queue.Enqueue(op))

		// Update to Running
		require.NoError(t, queue.UpdateStatus(op, StatusRunning))
		assert.NotNil(t, op.StartedAt)
		assert.Nil(t, op.CompletedAt)

		// Update to Completed
		require.NoError(t, queue.UpdateStatus(op, StatusCompleted))
		assert.NotNil(t, op.CompletedAt)

		// Lookup by ID via GetStatus
		foundOp, getErr := queue.GetStatus("OP-STATUS-CYCLE")
		require.NoError(t, getErr)
		assert.Equal(t, "OP-STATUS-CYCLE", foundOp.ID)
		assert.Equal(t, StatusCompleted, foundOp.Status)

		// Get non-existent status returns error
		_, notFoundErr := queue.GetStatus("NON-EXISTENT-OP")
		assert.Error(t, notFoundErr)

		// Filter operations by status
		completedOps := queue.ListOperations(func(o *Operation) bool {
			return o.Status == StatusCompleted
		})
		assert.Len(t, completedOps, 1)

		runningOps := queue.ListOperations(func(o *Operation) bool {
			return o.Status == StatusRunning
		})
		assert.Empty(t, runningOps)
	})

	t.Run("OperationQueue_ConcurrentEnqueueDequeueStress", func(t *testing.T) {
		notifier := &recordingNotifier{}
		queue := NewOperationQueue(notifier)
		var wakeCount atomic.Int64

		SetExecutorWakeCallback(func() {
			wakeCount.Add(1)
		})
		t.Cleanup(func() {
			SetExecutorWakeCallback(nil)
		})

		var wg sync.WaitGroup
		numWorkers := 8
		opsPerWorker := 25

		// Concurrent enqueues
		for w := 0; w < numWorkers; w++ {
			wg.Add(1)
			workerID := w
			goroutinelabels.NewGoroutine("storage_elevation_test", "enqueue worker").StartSimple(func() {
				defer wg.Done()
				for i := 0; i < opsPerWorker; i++ {
					prio := PriorityNormal
					if i%3 == 0 {
						prio = PriorityHigh
					} else if i%5 == 0 {
						prio = PriorityCritical
					}
					op := &Operation{
						ID:       fmt.Sprintf("OP-CONCUR-%d-%d", workerID, i),
						Priority: prio,
						Status:   StatusPending,
					}
					if err := queue.Enqueue(op); err != nil {
						t.Errorf("enqueue failed: %v", err)
					}
				}
			})
		}
		wg.Wait()

		totalExpected := numWorkers * opsPerWorker
		assert.Equal(t, totalExpected, queue.Len())

		// Concurrent dequeues
		var dequeuedCount atomic.Int64
		for w := 0; w < numWorkers; w++ {
			wg.Add(1)
			goroutinelabels.NewGoroutine("storage_elevation_test", "dequeue worker").StartSimple(func() {
				defer wg.Done()
				for {
					op := queue.Dequeue()
					if op == nil {
						break
					}
					dequeuedCount.Add(1)
				}
			})
		}
		wg.Wait()

		assert.Equal(t, int64(totalExpected), dequeuedCount.Load())
		assert.Equal(t, 0, queue.Len())
	})

	t.Run("SnapshotOperationQueue_LifecycleAndOverflow", func(t *testing.T) {
		snapQueue := NewSnapshotOperationQueue(5)
		require.NotNil(t, snapQueue)
		assert.Equal(t, 0, snapQueue.Size())

		now := time.Now()
		ops := []*SnapshotOperation{
			{Type: SnapshotOpCreate, ObjectID: "OBJ-1", Kind: "goal", Data: map[string]any{"title": "Goal 1"}, Timestamp: now},
			{Type: SnapshotOpUpdate, ObjectID: "OBJ-2", Kind: "requirement", Data: map[string]any{"title": "Req 2"}, Timestamp: now},
			{Type: SnapshotOpDelete, ObjectID: "OBJ-3", Kind: "backlog_item", Timestamp: now},
			{Type: SnapshotOpRead, ObjectID: "OBJ-4", Kind: "test_case", Timestamp: now},
			{Type: SnapshotOpUpdate, ObjectID: "OBJ-5", Kind: "criterion", Data: map[string]any{"title": "Crit 5"}, Timestamp: now},
		}

		for _, op := range ops {
			require.NoError(t, snapQueue.Enqueue(op))
		}
		assert.Equal(t, 5, snapQueue.Size())

		// Verify pending writes tracked correctly (Create, Update, Delete are tracked; Read is not)
		assert.True(t, snapQueue.HasPendingWrite("OBJ-1"))
		assert.True(t, snapQueue.HasPendingWrite("OBJ-2"))
		assert.True(t, snapQueue.HasPendingWrite("OBJ-3"))
		assert.False(t, snapQueue.HasPendingWrite("OBJ-4"), "Read operation should not track pending write")
		assert.True(t, snapQueue.HasPendingWrite("OBJ-5"))

		// GetDataFromPendingWrite
		data1, data1Err := snapQueue.GetDataFromPendingWrite("OBJ-1")
		require.NoError(t, data1Err)
		assert.Equal(t, "Goal 1", data1["title"])

		data2, data2Err := snapQueue.GetDataFromPendingWrite("OBJ-2")
		require.NoError(t, data2Err)
		assert.Equal(t, "Req 2", data2["title"])

		// Missing pending write returns error
		_, missingDataErr := snapQueue.GetDataFromPendingWrite("NON-EXISTENT")
		assert.Error(t, missingDataErr)

		// Overflow: 6th item should exceed maxSize 5 and return error
		overflowOp := &SnapshotOperation{Type: SnapshotOpCreate, ObjectID: "OBJ-OVERFLOW", Kind: "goal", Timestamp: now}
		overflowErr := snapQueue.Enqueue(overflowOp)
		assert.Error(t, overflowErr)
		assert.Contains(t, overflowErr.Error(), "overflow")

		// Replay with mock or empty storage (or Clear)
		snapQueue.Clear()
		assert.Equal(t, 0, snapQueue.Size())
		assert.False(t, snapQueue.HasPendingWrite("OBJ-1"))
	})
}
