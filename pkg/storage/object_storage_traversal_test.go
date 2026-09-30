package storage_test

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"context"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

func copyTraversalKernelLayout(t *testing.T, testRoot string) {
	t.Helper()
	copyTraversalSubtree(t, testRoot, paths.ProcessInternalLifecyclesDir)
	copyTraversalSubtree(t, testRoot, paths.ProcessInternalObjectSpecsDir)
}

func copyTraversalSubtree(t *testing.T, testRoot, rel string) {
	t.Helper()
	src := filepath.Join("..", "..", rel)
	dst := filepath.Join(testRoot, rel)
	if err := fileutil.MkdirAll(dst, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	entries, err := fileutil.ReadDir(src)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		body, err := fileutil.ReadFile(filepath.Join(src, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		if err := fileutil.WriteFile(filepath.Join(dst, entry.Name()), body, paths.FilePerm644); err != nil {
			t.Fatalf("write %s: %v", entry.Name(), err)
		}
	}
}

func setupTraversalObjectStorageTest(t *testing.T) (testRoot string, fos *storage.FileObjectStorage, secCtx *pkgctx.SecurityContext) {
	t.Helper()
	tmpDir, err := fileutil.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "1")
	storage.ResetReverseReferenceIndexForTest(t)
	storage.MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	copyTraversalKernelLayout(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}
	fos, err = storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	secCtx = pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(tmpDir, fos)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
		if err := fileutil.RemoveAll(tmpDir); err != nil {
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
		if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create dir %s: %v", dir, err)
		}
	}

	ctx := context.Background()

	// Create referenced objects first
	referencedObjects := []map[string]any{
		{objects.FieldKeyID: "GOAL-001", objects.FieldKeyKind: "goal", objects.FieldKeyTitle: "Goal 1", objects.FieldKeyDescription: "Substantive description for goal 1", objects.FieldKeyStatus: objects.ObjectStatusActive, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTarget: "100", objects.FieldKeyMetric: "count"},
		{objects.FieldKeyID: "GOAL-002", objects.FieldKeyKind: "goal", objects.FieldKeyTitle: "Goal 2", objects.FieldKeyDescription: "Substantive description for goal 2", objects.FieldKeyStatus: objects.ObjectStatusActive, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTarget: "100", objects.FieldKeyMetric: "count"},
		{objects.FieldKeyID: "MIL-001", objects.FieldKeyKind: "milestone", objects.FieldKeyTitle: "Milestone 1", objects.FieldKeyDescription: "Substantive description for milestone 1", objects.FieldKeyStatus: objects.ObjectStatusInProgress, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion},
		{objects.FieldKeyID: "PRI-001", objects.FieldKeyKind: "priority_plan", objects.FieldKeyTitle: "Priority Plan 1", objects.FieldKeyDescription: "Substantive description for priority plan 1", objects.FieldKeyStatus: objects.ObjectStatusActive, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion},
	}
	for _, obj := range referencedObjects {
		if err := fos.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("failed to create referenced object %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	// Create test object with references
	testObj := map[string]any{
		objects.FieldKeyID:              "BLI-001",
		objects.FieldKeyKind:            "backlog_item",
		objects.FieldKeyTitle:           "Test Item",
		objects.FieldKeyDescription:     "Substantive description for backlog item",
		objects.FieldKeyStatus:          objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyCategory:        "development",
		objects.FieldKeyGoalRefs:        []string{"GOAL-001", "GOAL-002"},
		objects.FieldKeyMilestoneRefs:   []string{"MIL-001"},
		objects.FieldKeyPriorityPlanRef: "PRI-001",
	}
	if err := fos.Create(ctx, secCtx, testObj); err != nil {
		t.Fatalf("failed to create test object: %v", err)
	}

	t.Run("GetRelated depth 1", func(t *testing.T) {
		related, err := fos.GetRelated(ctx, secCtx, "BLI-001", "", 1)
		if err != nil {
			t.Fatalf("GetRelated failed: %v", err)
		}

		ids := make([]string, len(related))
		for i, obj := range related {
			ids[i] = obj[objects.FieldKeyID].(string)
		}
		sort.Strings(ids)
		for _, want := range []string{"GOAL-001", "GOAL-002", "MIL-001", "PRI-001"} {
			if !slices.Contains(ids, want) {
				t.Errorf("missing related %s in %v", want, ids)
			}
		}
	})

	t.Run("GetRelated with relationship filter", func(t *testing.T) {
		related, err := fos.GetRelated(ctx, secCtx, "BLI-001", objects.FieldKeyGoalRefs, 1)
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
			objects.FieldKeyDescription:   "Substantive description for goal 3",
			objects.FieldKeyStatus:        objects.ObjectStatusActive,
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
		if err := fos.Update(ctx, secCtx, "BLI-001", updates); err != nil {
			t.Fatalf("failed to update test object: %v", err)
		}

		related, err := fos.GetRelated(ctx, secCtx, "BLI-001", "", 2)
		if err != nil {
			t.Fatalf("GetRelated failed: %v", err)
		}

		// Should find 4 direct + 1 indirect (GOAL-001 via GOAL-003, but GOAL-001 already found directly)
		// So still 4 unique objects: GOAL-001, GOAL-002, GOAL-003, MIL-001, PRI-001
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
		if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create dir %s: %v", dir, err)
		}
	}

	ctx := context.Background()

	// Create a chain: BLI-001 -> GOAL-001 -> GOAL-002
	pathChain := []map[string]any{
		{objects.FieldKeyID: "GOAL-002", objects.FieldKeyKind: "goal", objects.FieldKeyTitle: "Goal 2", objects.FieldKeyDescription: "Substantive description for path goal 2", objects.FieldKeyStatus: objects.ObjectStatusActive, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTarget: "100", objects.FieldKeyMetric: "count"},
		{objects.FieldKeyID: "GOAL-001", objects.FieldKeyKind: "goal", objects.FieldKeyTitle: "Goal 1", objects.FieldKeyDescription: "Substantive description for path goal 1", objects.FieldKeyStatus: objects.ObjectStatusActive, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTarget: "100", objects.FieldKeyMetric: "count", objects.FieldKeyGoalRefs: []string{"GOAL-002"}},
		{objects.FieldKeyID: "BLI-001", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 1", objects.FieldKeyDescription: "Substantive description for path item 1", objects.FieldKeyStatus: objects.ObjectStatusExploring, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development", objects.FieldKeyGoalRefs: []string{"GOAL-001"}},
	}

	for _, obj := range pathChain {
		if err := fos.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("failed to create object %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	t.Run("GetPath direct reference", func(t *testing.T) {
		path, err := fos.GetPath(ctx, secCtx, "BLI-001", "GOAL-001")
		if err != nil {
			t.Fatalf("GetPath failed: %v", err)
		}

		// Path should be: BLI-001 -> GOAL-001
		if len(path) != 2 {
			t.Errorf("expected path length 2, got %d", len(path))
		}
		if path[0][objects.FieldKeyID] != "BLI-001" {
			t.Errorf("expected first node BLI-001, got %s", path[0][objects.FieldKeyID])
		}
		if path[1][objects.FieldKeyID] != "GOAL-001" {
			t.Errorf("expected second node GOAL-001, got %s", path[1][objects.FieldKeyID])
		}
	})

	t.Run("GetPath indirect reference", func(t *testing.T) {
		path, err := fos.GetPath(ctx, secCtx, "BLI-001", "GOAL-002")
		if err != nil {
			t.Fatalf("GetPath failed: %v", err)
		}

		// Path should be: BLI-001 -> GOAL-001 -> GOAL-002
		if len(path) != 3 {
			t.Errorf("expected path length 3, got %d", len(path))
		}
		if path[0][objects.FieldKeyID] != "BLI-001" {
			t.Errorf("expected first node BLI-001, got %s", path[0][objects.FieldKeyID])
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
			objects.FieldKeyID:            "BLI-999",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Isolated Item",
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCategory:      "development",
		}
		if err := fos.Create(ctx, secCtx, isolated); err != nil {
			t.Fatalf("failed to create isolated object: %v", err)
		}

		path, err := fos.GetPath(ctx, secCtx, "BLI-001", "BLI-999")
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
		if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create dir %s: %v", dir, err)
		}
	}

	ctx := context.Background()

	// Create objects: BLI-001 references GOAL-001, BLI-002 also references GOAL-001
	neighborFixtures := []map[string]any{
		{objects.FieldKeyID: "GOAL-001", objects.FieldKeyKind: "goal", objects.FieldKeyTitle: "Goal 1", objects.FieldKeyStatus: objects.ObjectStatusActive, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTarget: "100", objects.FieldKeyMetric: "count"},
		{objects.FieldKeyID: "BLI-001", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 1", objects.FieldKeyStatus: objects.ObjectStatusExploring, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development", objects.FieldKeyGoalRefs: []string{"GOAL-001"}},
		{objects.FieldKeyID: "BLI-002", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 2", objects.FieldKeyStatus: objects.ObjectStatusExploring, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development", objects.FieldKeyGoalRefs: []string{"GOAL-001"}},
	}

	//
	for _, obj := range neighborFixtures {
		leave := ""
		if obj[objects.FieldKeyKind] == "backlog_item" {
			leave = objects.ObjectStatusValidated
		}
		storage.CreateCASVisible(t, fos, ctx, secCtx, obj, leave)
	}

	t.Run("GetNeighbors outgoing", func(t *testing.T) {
		neighbors, err := fos.GetNeighbors(ctx, secCtx, "BLI-001", "outgoing")
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

		// Should find BLI-001 and BLI-002 (both reference GOAL-001)
		if len(neighbors) != 2 {
			t.Errorf("expected 2 neighbors, got %d", len(neighbors))
		}

		ids := make([]string, len(neighbors))
		for i, obj := range neighbors {
			ids[i] = obj[objects.FieldKeyID].(string)
		}
		sort.Strings(ids)
		expected := []string{"BLI-001", "BLI-002"}
		if !slices.Equal(ids, expected) {
			t.Errorf("expected %v, got %v", expected, ids)
		}
	})

	t.Run("GetNeighbors both", func(t *testing.T) {
		neighbors, err := fos.GetNeighbors(ctx, secCtx, "GOAL-001", "both")
		if err != nil {
			t.Fatalf("GetNeighbors failed: %v", err)
		}

		// Should find BLI-001 and BLI-002 (incoming references)
		// GOAL-001 has no outgoing references, so only incoming
		if len(neighbors) != 2 {
			t.Errorf("expected 2 neighbors, got %d", len(neighbors))
		}
	})
}

func TestFileObjectStorage_GetRelated_edgeRole(t *testing.T) {
	_, fos, secCtx := setupTraversalObjectStorageTest(t)
	ctx := context.Background()

	plan := map[string]any{
		objects.FieldKeyID:            "PRI-EDGE-001",
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Edge Role Plan",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	crit := map[string]any{
		objects.FieldKeyID:            "CRIT-EDGE-001",
		objects.FieldKeyKind:          objects.KindCriteria,
		objects.FieldKeyTitle:         "Edge Role Criteria",
		objects.FieldKeyCategory:      "acceptance",
		objects.FieldKeyStatus:        objects.ObjectStatusAwaitingVerification,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	if err := fos.Create(ctx, secCtx, plan); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	storage.CreateCASVisible(t, fos, ctx, secCtx, crit, objects.ObjectStatusInProgress)
	item := map[string]any{
		objects.FieldKeyID:              "BLI-EDGE-001",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyTitle:           "Edge Role Item",
		objects.FieldKeyStatus:          objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyCategory:        "development",
		objects.FieldKeyPriorityPlanRef: "PRI-EDGE-001",
		objects.FieldKeyCriteriaRefs:    []string{"CRIT-EDGE-001"},
	}
	if err := fos.Create(ctx, secCtx, item); err != nil {
		t.Fatalf("create item: %v", err)
	}
	storage.FlushReverseReferenceIndexPersist()

	t.Run("parent-start membership finds child", func(t *testing.T) {
		related, err := fos.GetRelated(ctx, secCtx, "PRI-EDGE-001", string(objects.EdgeRoleMembership), 1)
		if err != nil {
			t.Fatalf("GetRelated: %v", err)
		}
		ids := make([]string, 0, len(related))
		for _, obj := range related {
			ids = append(ids, obj[objects.FieldKeyID].(string))
		}
		if !slices.Contains(ids, "BLI-EDGE-001") {
			t.Fatalf("membership from plan missing child, got %v", ids)
		}
	})

	t.Run("parent-start composition does not treat membership as composition", func(t *testing.T) {
		related, err := fos.GetRelated(ctx, secCtx, "PRI-EDGE-001", string(objects.EdgeRoleComposition), 1)
		if err != nil {
			t.Fatalf("GetRelated: %v", err)
		}
		for _, obj := range related {
			if obj[objects.FieldKeyID] == "BLI-EDGE-001" {
				t.Fatal("membership child must not appear under composition filter")
			}
		}
	})

	t.Run("GetPath parent to membership child", func(t *testing.T) {
		path, err := fos.GetPath(ctx, secCtx, "PRI-EDGE-001", "BLI-EDGE-001")
		if err != nil {
			t.Fatalf("GetPath: %v", err)
		}
		if len(path) != 2 {
			t.Fatalf("expected path length 2, got %d", len(path))
		}
		if path[0][objects.FieldKeyID] != "PRI-EDGE-001" || path[1][objects.FieldKeyID] != "BLI-EDGE-001" {
			t.Fatalf("path = %v", []any{path[0][objects.FieldKeyID], path[1][objects.FieldKeyID]})
		}
	})

	t.Run("parent-start composition finds child", func(t *testing.T) {
		related, err := fos.GetRelated(ctx, secCtx, "BLI-EDGE-001", string(objects.EdgeRoleComposition), 1)
		if err != nil {
			t.Fatalf("GetRelated: %v", err)
		}
		ids := make([]string, 0, len(related))
		for _, obj := range related {
			ids = append(ids, obj[objects.FieldKeyID].(string))
		}
		if !slices.Contains(ids, "CRIT-EDGE-001") {
			t.Fatalf("composition from BLI missing criteria, got %v", ids)
		}
	})

	t.Run("child-start composition finds parent", func(t *testing.T) {
		bli, err := fos.Read(ctx, secCtx, "BLI-EDGE-001")
		if err != nil {
			t.Fatalf("read BLI: %v", err)
		}
		deps := storage.GetGlobalReverseReferenceIndex().GetDependents("CRIT-EDGE-001")
		related, err := fos.GetRelated(ctx, secCtx, "CRIT-EDGE-001", string(objects.EdgeRoleComposition), 1)
		if err != nil {
			t.Fatalf("GetRelated: %v", err)
		}
		ids := make([]string, 0, len(related))
		for _, obj := range related {
			ids = append(ids, obj[objects.FieldKeyID].(string))
		}
		if !slices.Contains(ids, "BLI-EDGE-001") {
			t.Fatalf("composition from criteria missing parent, got %v; bli.criteria_refs=%v dependents=%v", ids, bli[objects.FieldKeyCriteriaRefs], deps)
		}
	})
}
