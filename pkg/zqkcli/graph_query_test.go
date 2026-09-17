package internal

//nolint:errcheck // Test cleanup operations - errors are acceptable

// Tests that set ZQK_TEST_ROOT must not use t.Parallel(): the env var is process-global.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/config"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/zqktime"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestGraphBackendQueryCapabilities tests sorting, filtering, grouping, and pagination
//
//nolint:gocyclo // Test function intentionally exercises many query combinations
func TestGraphBackendQueryCapabilities(t *testing.T) {
	if !config.StorageGraphEnabled().Safe() {
		t.Skip("Graph backend not enabled (set ZQK_GRAPH_ENABLED=true)")
	}

	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "internal.graph_query"})
	testRoot := proj.Root
	// Prefer the testkit pool (Database=test) over mcp studio pool — shared MemGraph
	// otherwise returns hundreds of studio backlog_items and exact-count asserts flake.
	pool := testkit.PrepareGraphConnectionForTest(t)

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 2*time.Minute)
	defer cancel()
	// Promote-on-create keeps shovel-ready statuses (validated/planned/…) instead of
	// draft-first coerce to exploring — required for exact-count status filters.
	ctx = pkgctx.WithPromoteOnCreate(storage.WithTestHardDelete(ctx))

	graphStorage := storage.NewPoolAwareGraphStorage(pool, testRoot)
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	origin := "zqkcli-gq-" + fmt.Sprintf("%d", time.Now().UnixNano())

	// Clear any existing test data
	//nolint:errcheck // Test query - error acceptable
	_, _ = graphStorage.Query(ctx, secCtx, storageCtx, storage.Query{
		Type:       storage.QueryTypeCypher,
		Expression: "MATCH (n) WHERE n.id IN ['BLI-001', 'BLI-002', 'BLI-003', 'BLI-004', 'BLI-005'] DETACH DELETE n",
	})

	// Create test objects with varying properties (IDs must match pattern ^[A-Z]+-\d{3,}$)
	testObjects := []map[string]any{
		{objects.FieldKeyID: "BLI-001", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Alpha Item", objects.FieldKeyStatus: "exploring", objects.FieldKeyPriorityTier: "P1", objects.FieldKeyOriginProject: origin, objects.FieldKeyCreatedAt: "2025-01-01T00:00:00Z", objects.FieldKeyCreatedBy: "test", objects.FieldKeyUpdatedAt: "2025-01-01T00:00:00Z", objects.FieldKeyUpdatedBy: "test", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion},
		{objects.FieldKeyID: "BLI-002", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Beta Item", objects.FieldKeyStatus: "validated", objects.FieldKeyPriorityTier: "P2", objects.FieldKeyOriginProject: origin, objects.FieldKeyCreatedAt: "2025-01-02T00:00:00Z", objects.FieldKeyCreatedBy: "test", objects.FieldKeyUpdatedAt: "2025-01-02T00:00:00Z", objects.FieldKeyUpdatedBy: "test", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion},
		{objects.FieldKeyID: "BLI-003", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Gamma Item", objects.FieldKeyStatus: "exploring", objects.FieldKeyPriorityTier: "P1", objects.FieldKeyOriginProject: origin, objects.FieldKeyCreatedAt: "2025-01-03T00:00:00Z", objects.FieldKeyCreatedBy: "test", objects.FieldKeyUpdatedAt: "2025-01-03T00:00:00Z", objects.FieldKeyUpdatedBy: "test", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion},
		{objects.FieldKeyID: "BLI-004", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Delta Item", objects.FieldKeyStatus: "validated", objects.FieldKeyPriorityTier: "P3", objects.FieldKeyOriginProject: origin, objects.FieldKeyCreatedAt: "2025-01-04T00:00:00Z", objects.FieldKeyCreatedBy: "test", objects.FieldKeyUpdatedAt: "2025-01-04T00:00:00Z", objects.FieldKeyUpdatedBy: "test", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion},
		{objects.FieldKeyID: "BLI-005", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Epsilon Item", objects.FieldKeyStatus: "planned", objects.FieldKeyPriorityTier: "P2", objects.FieldKeyOriginProject: origin, objects.FieldKeyCreatedAt: "2025-01-05T00:00:00Z", objects.FieldKeyCreatedBy: "test", objects.FieldKeyUpdatedAt: "2025-01-05T00:00:00Z", objects.FieldKeyUpdatedBy: "test", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion},
	}

	for _, obj := range testObjects {
		if err := graphStorage.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("failed to create test object %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	scope := map[string]any{objects.FieldKeyOriginProject: origin}

	t.Run("Filtering", func(t *testing.T) {
		// Filter by status (scoped to this test's origin_project)
		filter := storage.ListFilter{
			Kind: "backlog_item",
			Filters: map[string]any{
				objects.FieldKeyOriginProject: origin,
				objects.FieldKeyStatus:        "exploring",
			},
		}
		result, err := graphStorage.List(ctx, secCtx, storageCtx, filter)
		if err != nil {
			t.Fatalf("failed to list with filter: %v", err)
		}
		if len(result.Objects) != 2 {
			t.Errorf("expected 2 objects with status=exploring, got %d", len(result.Objects))
		}
		for _, obj := range result.Objects {
			if obj[objects.FieldKeyStatus] != "exploring" {
				t.Errorf("expected status=exploring, got %v", obj[objects.FieldKeyStatus])
			}
		}

		// Filter by priority_tier
		filter = storage.ListFilter{
			Kind: "backlog_item",
			Filters: map[string]any{
				objects.FieldKeyOriginProject: origin,
				objects.FieldKeyPriorityTier:  "P1",
			},
		}
		result, err = graphStorage.List(ctx, secCtx, storageCtx, filter)
		if err != nil {
			t.Fatalf("failed to list with priority filter: %v", err)
		}
		if len(result.Objects) != 2 {
			t.Errorf("expected 2 objects with priority_tier=P1, got %d", len(result.Objects))
		}
	})

	t.Run("Sorting", func(t *testing.T) {
		// Sort by title ascending
		filter := storage.ListFilter{
			Kind:    "backlog_item",
			Filters: scope,
			SortBy:  "title",
			SortAsc: true,
		}
		result, err := graphStorage.List(ctx, secCtx, storageCtx, filter)
		if err != nil {
			t.Fatalf("failed to list with sort: %v", err)
		}
		if len(result.Objects) < 2 {
			t.Fatalf("expected at least 2 objects, got %d", len(result.Objects))
		}
		// Verify sorted order
		for i := 1; i < len(result.Objects); i++ {
			prevTitle := result.Objects[i-1][objects.FieldKeyTitle].(string)
			currTitle := result.Objects[i][objects.FieldKeyTitle].(string)
			if prevTitle > currTitle {
				t.Errorf("objects not sorted: %s should come before %s", prevTitle, currTitle)
			}
		}

		// Sort by created_at descending
		filter = storage.ListFilter{
			Kind:    "backlog_item",
			Filters: scope,
			SortBy:  "created_at",
			SortAsc: false,
		}
		result, err = graphStorage.List(ctx, secCtx, storageCtx, filter)
		if err != nil {
			t.Fatalf("failed to list with sort desc: %v", err)
		}
		if len(result.Objects) < 2 {
			t.Fatalf("expected at least 2 objects, got %d", len(result.Objects))
		}
		// Verify reverse sorted order
		for i := 1; i < len(result.Objects); i++ {
			prevDate := result.Objects[i-1][objects.FieldKeyCreatedAt].(string)
			currDate := result.Objects[i][objects.FieldKeyCreatedAt].(string)
			if prevDate < currDate {
				t.Errorf("objects not reverse sorted: %s should come after %s", prevDate, currDate)
			}
		}
	})

	t.Run("Pagination", func(t *testing.T) {
		// Test offset and limit
		filter := storage.ListFilter{
			Kind:    "backlog_item",
			Filters: scope,
			Offset:  1,
			Limit:   2,
		}
		result, err := graphStorage.List(ctx, secCtx, storageCtx, filter)
		if err != nil {
			t.Fatalf("failed to list with pagination: %v", err)
		}
		if len(result.Objects) != 2 {
			t.Errorf("expected 2 objects with offset=1 limit=2, got %d", len(result.Objects))
		}

		// Test limit only
		filter = storage.ListFilter{
			Kind:    "backlog_item",
			Filters: scope,
			Limit:   3,
		}
		result, err = graphStorage.List(ctx, secCtx, storageCtx, filter)
		if err != nil {
			t.Fatalf("failed to list with limit: %v", err)
		}
		if len(result.Objects) != 3 {
			t.Errorf("expected 3 objects with limit=3, got %d", len(result.Objects))
		}
	})

	t.Run("Grouping", func(t *testing.T) {
		// Enable grouping in storage context
		groupingCtx := pkgctx.NewStorageContext()
		groupingCtx.EnableGrouping = true
		groupingCtx.MaxGroupSize = 10

		filter := storage.ListFilter{
			Kind:    "backlog_item",
			Filters: scope,
			GroupBy: "status",
		}
		result, err := graphStorage.List(ctx, secCtx, groupingCtx, filter)
		if err != nil {
			t.Fatalf("failed to list with grouping: %v", err)
		}

		// Verify groups exist
		if len(result.Groups) == 0 {
			t.Error("expected groups to be populated")
		}

		// Verify each object appears in the correct group
		allGroupedObjects := 0
		for groupKey, groupObjects := range result.Groups {
			allGroupedObjects += len(groupObjects)
			for _, obj := range groupObjects {
				if obj[objects.FieldKeyStatus] != groupKey {
					t.Errorf("object %s in wrong group: expected status=%s, got %v", obj[objects.FieldKeyID], groupKey, obj[objects.FieldKeyStatus])
				}
			}
		}

		// Verify all objects are grouped
		if allGroupedObjects != len(testObjects) {
			t.Errorf("expected %d grouped objects, got %d", len(testObjects), allGroupedObjects)
		}
	})

	t.Run("CombinedFilterSortPagination", func(t *testing.T) {
		// Combine filter, sort, and pagination
		filter := storage.ListFilter{
			Kind: "backlog_item",
			Filters: map[string]any{
				objects.FieldKeyOriginProject: origin,
				objects.FieldKeyPriorityTier:  "P2",
			},
			SortBy:  "title",
			SortAsc: true,
			Limit:   10,
		}
		result, err := graphStorage.List(ctx, secCtx, storageCtx, filter)
		if err != nil {
			t.Fatalf("failed to list with combined filters: %v", err)
		}
		if len(result.Objects) != 2 {
			t.Errorf("expected 2 objects with priority_tier=P2, got %d", len(result.Objects))
		}
		// Verify sorted
		if len(result.Objects) >= 2 {
			prevTitle := result.Objects[0][objects.FieldKeyTitle].(string)
			currTitle := result.Objects[1][objects.FieldKeyTitle].(string)
			if prevTitle > currTitle {
				t.Errorf("objects not sorted: %s should come before %s", prevTitle, currTitle)
			}
		}
	})

	t.Run("Count", func(t *testing.T) {
		// Test count operation using List and checking length
		filter := storage.ListFilter{
			Kind:    "backlog_item",
			Filters: scope,
		}
		result, err := graphStorage.List(ctx, secCtx, storageCtx, filter)
		if err != nil {
			t.Fatalf("failed to list objects for count: %v", err)
		}
		if len(result.Objects) != len(testObjects) {
			t.Errorf("expected count %d, got %d", len(testObjects), len(result.Objects))
		}

		// Count with filter
		filter = storage.ListFilter{
			Kind: "backlog_item",
			Filters: map[string]any{
				objects.FieldKeyOriginProject: origin,
				objects.FieldKeyStatus:        "exploring",
			},
		}
		result, err = graphStorage.List(ctx, secCtx, storageCtx, filter)
		if err != nil {
			t.Fatalf("failed to list with filter for count: %v", err)
		}
		if len(result.Objects) != 2 {
			t.Errorf("expected count 2 for status=exploring, got %d", len(result.Objects))
		}
	})

	// Cleanup
	for _, obj := range testObjects {
		//nolint:errcheck // Test cleanup - errors are acceptable
		_ = graphStorage.Delete(ctx, secCtx, obj[objects.FieldKeyID].(string), false)
	}
}

// TestGraphBackendAggregation tests aggregation capabilities
//
//nolint:gocyclo // Test function intentionally exercises many aggregation scenarios
func TestGraphBackendAggregation(t *testing.T) {
	if !config.StorageGraphEnabled().Safe() {
		t.Skip("Graph backend not enabled (set ZQK_GRAPH_ENABLED=true)")
	}

	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "internal.graph_aggregation"})
	testRoot := proj.Root
	pool := testkit.PrepareGraphConnectionForTest(t)

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 2*time.Minute)
	defer cancel()
	ctx = pkgctx.WithPromoteOnCreate(storage.WithTestHardDelete(ctx))

	graphStorage := storage.NewPoolAwareGraphStorage(pool, testRoot)
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	origin := "zqkcli-ga-" + fmt.Sprintf("%d", time.Now().UnixNano())

	//nolint:errcheck // Test query - error acceptable
	// Clear any existing test data
	_, _ = graphStorage.Query(ctx, secCtx, storageCtx, storage.Query{
		Type:       storage.QueryTypeCypher,
		Expression: "MATCH (n) WHERE n.id IN ['BLI-100', 'BLI-101', 'BLI-102', 'BLI-103', 'BLI-104', 'BLI-105'] DETACH DELETE n",
	})

	// Create test objects for aggregation
	statuses := []string{"exploring", "validated", "planned", "in_progress", "exploring", "validated"}
	priorities := []string{"P1", "P2", "P3", "P1", "P2", "P1"}

	for i := 0; i < 6; i++ {
		obj := map[string]any{
			objects.FieldKeyID:            fmt.Sprintf("BLI-%03d", 100+i),
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         fmt.Sprintf("Aggregation Test %d", i),
			objects.FieldKeyStatus:        statuses[i],
			objects.FieldKeyPriorityTier:  priorities[i],
			objects.FieldKeyOriginProject: origin,
			objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
			objects.FieldKeyUpdatedBy:     "test",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		if err := graphStorage.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("failed to create test object: %v", err)
		}
	}
	scope := map[string]any{objects.FieldKeyOriginProject: origin}

	t.Run("GroupByStatus", func(t *testing.T) {
		groupingCtx := pkgctx.NewStorageContext()
		groupingCtx.EnableGrouping = true
		groupingCtx.MaxGroupSize = 10

		filter := storage.ListFilter{
			Kind:    "backlog_item",
			Filters: scope,
			GroupBy: "status",
		}
		result, err := graphStorage.List(ctx, secCtx, groupingCtx, filter)
		if err != nil {
			t.Fatalf("failed to list with grouping: %v", err)
		}

		// Verify groups
		expectedGroups := map[string]int{
			"exploring":   2,
			"validated":   2,
			"planned":     1,
			"in_progress": 1,
		}

		for status, expectedCount := range expectedGroups {
			groupObjects, exists := result.Groups[status]
			if !exists {
				t.Errorf("expected group for status=%s", status)
				continue
			}
			if len(groupObjects) != expectedCount {
				t.Errorf("expected %d objects in status=%s group, got %d", expectedCount, status, len(groupObjects))
			}
		}
	})

	t.Run("GroupByPriority", func(t *testing.T) {
		groupingCtx := pkgctx.NewStorageContext()
		groupingCtx.EnableGrouping = true
		groupingCtx.MaxGroupSize = 10

		filter := storage.ListFilter{
			Kind:    "backlog_item",
			Filters: scope,
			GroupBy: "priority_tier",
		}
		result, err := graphStorage.List(ctx, secCtx, groupingCtx, filter)
		if err != nil {
			t.Fatalf("failed to list with grouping: %v", err)
		}

		// Verify groups
		expectedGroups := map[string]int{
			"P1": 3,
			"P2": 2,
			"P3": 1,
		}

		for priority, expectedCount := range expectedGroups {
			groupObjects, exists := result.Groups[priority]
			if !exists {
				t.Errorf("expected group for priority_tier=%s", priority)
				continue
			}
			if len(groupObjects) != expectedCount {
				t.Errorf("expected %d objects in priority_tier=%s group, got %d", expectedCount, priority, len(groupObjects))
			}
		}
	})

	t.Run("GroupByWithLimit", func(t *testing.T) {
		groupingCtx := pkgctx.NewStorageContext()
		groupingCtx.EnableGrouping = true
		groupingCtx.MaxGroupSize = 2 // Limit items per group

		filter := storage.ListFilter{
			Kind:    "backlog_item",
			Filters: scope,
			GroupBy: "status",
		}
		result, err := graphStorage.List(ctx, secCtx, groupingCtx, filter)
		if err != nil {
			t.Fatalf("failed to list with grouping limit: %v", err)
		}

		// Verify group limits are respected
		for status, groupObjects := range result.Groups {
			if len(groupObjects) > 2 {
				t.Errorf("group %s exceeded limit: expected max 2, got %d", status, len(groupObjects))
			}
		}
	})

	t.Run("CustomCypherAggregation", func(t *testing.T) {
		// Test custom Cypher query for aggregation
		query := storage.Query{
			Type: storage.QueryTypeCypher,
			Expression: `
				MATCH (n:BacklogItem:Entity)
				WHERE n.id IN ['BLI-100', 'BLI-101', 'BLI-102', 'BLI-103', 'BLI-104', 'BLI-105']
				RETURN n.status AS status, count(n) AS count
				ORDER BY count DESC
			`,
		}
		result, err := graphStorage.Query(ctx, secCtx, storageCtx, query)
		if err != nil {
			t.Fatalf("failed to execute aggregation query: %v", err)
		}

		// Verify we got results
		if len(result.Objects) == 0 {
			t.Error("expected aggregation results")
		}

		// Results should have status and count fields
		for _, obj := range result.Objects {
			if _, hasStatus := obj[objects.FieldKeyStatus]; !hasStatus {
				t.Error("expected result to have status field")
			}
		}
	})

	// Cleanup
	for i := 0; i < 6; i++ {
		//nolint:errcheck // Test cleanup - errors are acceptable
		_ = graphStorage.Delete(ctx, secCtx, fmt.Sprintf("BLI-%03d", 100+i), false)
	}
}
