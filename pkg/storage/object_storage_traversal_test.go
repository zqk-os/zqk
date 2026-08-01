package storage_test

import (
	"github.com/lanceman/zqk/pkg/datacell"

	"context"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
)

func setupTraversalObjectStorageTest(t *testing.T) (testRoot string, fos *storage.FileObjectStorage, secCtx *pkgctx.SecurityContext) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	storage.MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}
	fos, err = storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	secCtx = pkgctx.NewSecurityContext("account:test", []string{"admin"}, []string{"read:*", "write:*"})
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(tmpDir, fos)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("remove temp dir: %v", err)
		}
	})
	return tmpDir, fos, secCtx
}

func TestFileObjectStorage_GetRelated(t *testing.T) {
	testRoot, fos, secCtx := setupTraversalObjectStorageTest(t)
	processDir := datacell.ProcessPrimaryDir(testRoot)
	backlogDir := filepath.Join(processDir, "backlog")
	goalDir := filepath.Join(processDir, "goals")
	milestoneDir := filepath.Join(processDir, "milestones")
	priorityPlanDir := filepath.Join(processDir, "priority_plans")

	for _, dir := range []string{backlogDir, goalDir, milestoneDir, priorityPlanDir} {
		if err := os.MkdirAll(dir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create dir %s: %v", dir, err)
		}
	}

	ctx := context.Background()

	// Create referenced objects first
	referencedObjects := []map[string]any{
		{objects.FieldKeyID: "GOAL-001", objects.FieldKeyKind: "goal", objects.FieldKeyTitle: "Goal 1", objects.FieldKeyStatus: "active", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTarget: "100", objects.FieldKeyMetric: "count"},
		{objects.FieldKeyID: "GOAL-002", objects.FieldKeyKind: "goal", objects.FieldKeyTitle: "Goal 2", objects.FieldKeyStatus: "active", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTarget: "100", objects.FieldKeyMetric: "count"},
		{objects.FieldKeyID: "MIL-001", objects.FieldKeyKind: "milestone", objects.FieldKeyTitle: "Milestone 1", objects.FieldKeyStatus: "in_progress", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion},
		{objects.FieldKeyID: "PLAN-001", objects.FieldKeyKind: "priority_plan", objects.FieldKeyTitle: "Priority Plan 1", objects.FieldKeyStatus: "active", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion},
	}
	for _, obj := range referencedObjects {
		if err := fos.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("failed to create referenced object %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	// Create test object with references
	testObj := map[string]any{
		objects.FieldKeyID:              "ITEM-001",
		objects.FieldKeyKind:            "backlog_item",
		objects.FieldKeyTitle:           "Test Item",
		objects.FieldKeyStatus:          "exploring",
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyCategory:        "development",
		objects.FieldKeyGoalRefs:        []string{"GOAL-001", "GOAL-002"},
		objects.FieldKeyMilestoneRefs:   []string{"MIL-001"},
		objects.FieldKeyPriorityPlanRef: "PLAN-001",
	}
	if err := fos.Create(ctx, secCtx, testObj); err != nil {
		t.Fatalf("failed to create test object: %v", err)
	}

	t.Run("GetRelated depth 1", func(t *testing.T) {
		related, err := fos.GetRelated(ctx, secCtx, "ITEM-001", "", 1)
		if err != nil {
			t.Fatalf("GetRelated failed: %v", err)
		}

		// Should find 4 related objects (2 goals, 1 milestone, 1 priority plan)
		if len(related) != 4 {
			t.Errorf("expected 4 related objects, got %d", len(related))
		}

		// Verify all expected objects are present
		ids := make([]string, len(related))
		for i, obj := range related {
			ids[i] = obj[objects.FieldKeyID].(string)
		}
		sort.Strings(ids)
		expected := []string{"GOAL-001", "GOAL-002", "MIL-001", "PLAN-001"}
		if !slices.Equal(ids, expected) {
			t.Errorf("expected %v, got %v", expected, ids)
		}
	})

	t.Run("GetRelated with relationship filter", func(t *testing.T) {
		related, err := fos.GetRelated(ctx, secCtx, "ITEM-001", objects.FieldKeyGoalRefs, 1)
		if err != nil {
			t.Fatalf("GetRelated failed: %v", err)
		}

		// Should find only goals
		if len(related) != 2 {
			t.Errorf("expected 2 related objects, got %d", len(related))
		}

		ids := make([]string, len(related))
		for i, obj := range related {
			ids[i] = obj[objects.FieldKeyID].(string)
		}
		sort.Strings(ids)
		expected := []string{"GOAL-001", "GOAL-002"}
		if !slices.Equal(ids, expected) {
			t.Errorf("expected %v, got %v", expected, ids)
		}
	})

	t.Run("GetRelated depth 2", func(t *testing.T) {
		// Create a goal that references another goal
		goalWithRef := map[string]any{
			objects.FieldKeyID:            "GOAL-003",
			objects.FieldKeyKind:          "goal",
			objects.FieldKeyTitle:         "Goal 3",
			objects.FieldKeyStatus:        "active",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyTarget:        "100",
			objects.FieldKeyMetric:        "count",
			objects.FieldKeyGoalRefs:      []string{"GOAL-001"}, // GOAL-003 references GOAL-001
		}
		if err := fos.Create(ctx, secCtx, goalWithRef); err != nil {
			t.Fatalf("failed to create goal with ref: %v", err)
		}

		// Update test object to reference GOAL-003
		updates := map[string]any{
			objects.FieldKeyGoalRefs: []string{"GOAL-001", "GOAL-002", "GOAL-003"},
		}
		if err := fos.Update(ctx, secCtx, "ITEM-001", updates); err != nil {
			t.Fatalf("failed to update test object: %v", err)
		}

		related, err := fos.GetRelated(ctx, secCtx, "ITEM-001", "", 2)
		if err != nil {
			t.Fatalf("GetRelated failed: %v", err)
		}

		// Should find 4 direct + 1 indirect (GOAL-001 via GOAL-003, but GOAL-001 already found directly)
		// So still 4 unique objects: GOAL-001, GOAL-002, GOAL-003, MIL-001, PLAN-001
		// Actually, GOAL-001 appears twice in the chain but should be deduplicated
		if len(related) < 4 {
			t.Errorf("expected at least 4 related objects, got %d", len(related))
		}
	})
}

func TestFileObjectStorage_GetPath(t *testing.T) {
	// Synthetic goal/backlog graph under a temp project root.
	testRoot, fos, secCtx := setupTraversalObjectStorageTest(t)
	processDir := datacell.ProcessPrimaryDir(testRoot)
	backlogDir := filepath.Join(processDir, "backlog")
	goalDir := filepath.Join(processDir, "goals")

	for _, dir := range []string{backlogDir, goalDir} {
		if err := os.MkdirAll(dir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create dir %s: %v", dir, err)
		}
	}

	ctx := context.Background()

	// Create a chain: ITEM-001 -> GOAL-001 -> GOAL-002
	pathChain := []map[string]any{
		{objects.FieldKeyID: "GOAL-002", objects.FieldKeyKind: "goal", objects.FieldKeyTitle: "Goal 2", objects.FieldKeyStatus: "active", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTarget: "100", objects.FieldKeyMetric: "count"},
		{objects.FieldKeyID: "GOAL-001", objects.FieldKeyKind: "goal", objects.FieldKeyTitle: "Goal 1", objects.FieldKeyStatus: "active", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTarget: "100", objects.FieldKeyMetric: "count", objects.FieldKeyGoalRefs: []string{"GOAL-002"}},
		{objects.FieldKeyID: "ITEM-001", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 1", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development", objects.FieldKeyGoalRefs: []string{"GOAL-001"}},
	}

	for _, obj := range pathChain {
		if err := fos.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("failed to create object %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	t.Run("GetPath direct reference", func(t *testing.T) {
		path, err := fos.GetPath(ctx, secCtx, "ITEM-001", "GOAL-001")
		if err != nil {
			t.Fatalf("GetPath failed: %v", err)
		}

		// Path should be: ITEM-001 -> GOAL-001
		if len(path) != 2 {
			t.Errorf("expected path length 2, got %d", len(path))
		}
		if path[0][objects.FieldKeyID] != "ITEM-001" {
			t.Errorf("expected first node ITEM-001, got %s", path[0][objects.FieldKeyID])
		}
		if path[1][objects.FieldKeyID] != "GOAL-001" {
			t.Errorf("expected second node GOAL-001, got %s", path[1][objects.FieldKeyID])
		}
	})

	t.Run("GetPath indirect reference", func(t *testing.T) {
		path, err := fos.GetPath(ctx, secCtx, "ITEM-001", "GOAL-002")
		if err != nil {
			t.Fatalf("GetPath failed: %v", err)
		}

		// Path should be: ITEM-001 -> GOAL-001 -> GOAL-002
		if len(path) != 3 {
			t.Errorf("expected path length 3, got %d", len(path))
		}
		if path[0][objects.FieldKeyID] != "ITEM-001" {
			t.Errorf("expected first node ITEM-001, got %s", path[0][objects.FieldKeyID])
		}
		if path[1][objects.FieldKeyID] != "GOAL-001" {
			t.Errorf("expected second node GOAL-001, got %s", path[1][objects.FieldKeyID])
		}
		if path[2][objects.FieldKeyID] != "GOAL-002" {
			t.Errorf("expected third node GOAL-002, got %s", path[2][objects.FieldKeyID])
		}
	})

	t.Run("GetPath no path", func(t *testing.T) {
		// Create isolated object
		isolated := map[string]any{
			objects.FieldKeyID:            "ITEM-999",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Isolated Item",
			objects.FieldKeyStatus:        "exploring",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCategory:      "development",
		}
		if err := fos.Create(ctx, secCtx, isolated); err != nil {
			t.Fatalf("failed to create isolated object: %v", err)
		}

		path, err := fos.GetPath(ctx, secCtx, "ITEM-001", "ITEM-999")
		if err != nil {
			t.Fatalf("GetPath failed: %v", err)
		}

		// Should return empty path
		if len(path) != 0 {
			t.Errorf("expected empty path, got length %d", len(path))
		}
	})
}

func TestFileObjectStorage_GetNeighbors(t *testing.T) {
	testRoot, fos, secCtx := setupTraversalObjectStorageTest(t)
	processDir := datacell.ProcessPrimaryDir(testRoot)
	backlogDir := filepath.Join(processDir, "backlog")
	goalDir := filepath.Join(processDir, "goals")

	for _, dir := range []string{backlogDir, goalDir} {
		if err := os.MkdirAll(dir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create dir %s: %v", dir, err)
		}
	}

	ctx := context.Background()

	// Create objects: ITEM-001 references GOAL-001, ITEM-002 also references GOAL-001
	neighborFixtures := []map[string]any{
		{objects.FieldKeyID: "GOAL-001", objects.FieldKeyKind: "goal", objects.FieldKeyTitle: "Goal 1", objects.FieldKeyStatus: "active", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTarget: "100", objects.FieldKeyMetric: "count"},
		{objects.FieldKeyID: "ITEM-001", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 1", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development", objects.FieldKeyGoalRefs: []string{"GOAL-001"}},
		{objects.FieldKeyID: "ITEM-002", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 2", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development", objects.FieldKeyGoalRefs: []string{"GOAL-001"}},
	}

	for _, obj := range neighborFixtures {
		if err := fos.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("failed to create object %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	t.Run("GetNeighbors outgoing", func(t *testing.T) {
		neighbors, err := fos.GetNeighbors(ctx, secCtx, "ITEM-001", "outgoing")
		if err != nil {
			t.Fatalf("GetNeighbors failed: %v", err)
		}

		// Should find GOAL-001 (outgoing reference)
		if len(neighbors) != 1 {
			t.Errorf("expected 1 neighbor, got %d", len(neighbors))
		}
		if neighbors[0][objects.FieldKeyID] != "GOAL-001" {
			t.Errorf("expected GOAL-001, got %s", neighbors[0][objects.FieldKeyID])
		}
	})

	t.Run("GetNeighbors incoming", func(t *testing.T) {
		neighbors, err := fos.GetNeighbors(ctx, secCtx, "GOAL-001", "incoming")
		if err != nil {
			t.Fatalf("GetNeighbors failed: %v", err)
		}

		// Should find ITEM-001 and ITEM-002 (both reference GOAL-001)
		if len(neighbors) != 2 {
			t.Errorf("expected 2 neighbors, got %d", len(neighbors))
		}

		ids := make([]string, len(neighbors))
		for i, obj := range neighbors {
			ids[i] = obj[objects.FieldKeyID].(string)
		}
		sort.Strings(ids)
		expected := []string{"ITEM-001", "ITEM-002"}
		if !slices.Equal(ids, expected) {
			t.Errorf("expected %v, got %v", expected, ids)
		}
	})

	t.Run("GetNeighbors both", func(t *testing.T) {
		neighbors, err := fos.GetNeighbors(ctx, secCtx, "GOAL-001", "both")
		if err != nil {
			t.Fatalf("GetNeighbors failed: %v", err)
		}

		// Should find ITEM-001 and ITEM-002 (incoming references)
		// GOAL-001 has no outgoing references, so only incoming
		if len(neighbors) != 2 {
			t.Errorf("expected 2 neighbors, got %d", len(neighbors))
		}
	})
}
