package validate

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestResolvePriorityPlan(t *testing.T) {
	ctx := context.Background()

	// Case 1: Explicit plan
	store1 := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-1",
			objects.FieldKeyStatus: objects.ObjectStatusGrooming,
		},
	)
	planID, err := resolvePriorityPlan(ctx, store1, "PRI-1")
	if err != nil {
		t.Fatalf("resolvePriorityPlan failed: %v", err)
	}
	if planID != "PRI-1" {
		t.Errorf("expected PRI-1, got %s", planID)
	}

	// Case 2: Auto-resolve to in_progress
	store2 := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-in_progress",
			objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		},
		map[string]any{
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-active",
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		},
	)
	planID, err = resolvePriorityPlan(ctx, store2, "")
	if err != nil {
		t.Fatalf("resolvePriorityPlan failed: %v", err)
	}
	if planID != "PRI-in_progress" {
		t.Errorf("expected PRI-in_progress, got %s", planID)
	}

	// Case 3: Explicit plan not found
	_, err = resolvePriorityPlan(ctx, store1, "PRI-NON-EXISTENT")
	if err == nil {
		t.Errorf("expected error for non-existent explicit plan")
	}

	// Case 4: Auto-resolve to active when in_progress not present
	store4 := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-active-only",
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		},
	)
	planID, err = resolvePriorityPlan(ctx, store4, "")
	if err != nil || planID != "PRI-active-only" {
		t.Errorf("expected PRI-active-only, got %s, err: %v", planID, err)
	}

	// Case 5: Auto-resolve to paused when in_progress and active not present
	store5 := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-paused-only",
			objects.FieldKeyStatus: objects.ObjectStatusPaused,
		},
	)
	planID, err = resolvePriorityPlan(ctx, store5, "")
	if err != nil || planID != "PRI-paused-only" {
		t.Errorf("expected PRI-paused-only, got %s, err: %v", planID, err)
	}

	// Case 6: No plans found
	store6 := newMemoryWorkflowStore()
	_, err = resolvePriorityPlan(ctx, store6, "")
	if err == nil {
		t.Errorf("expected error when no plans found")
	}
}

func TestTreeResolver_Resolve(t *testing.T) {
	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-plan",
			objects.FieldKeyStatus: objects.ObjectStatusInProgress,
			objects.FieldKeyTitle:  "Test Plan",
		},
		map[string]any{
			objects.FieldKeyKind:             objects.KindGoal,
			objects.FieldKeyID:               "GOAL-1",
			objects.FieldKeyStatus:           objects.ObjectStatusActive,
			objects.FieldKeyTitle:            "Test Goal",
			objects.FieldKeyPriorityPlanRefs: []any{"PRI-plan"},
		},
		map[string]any{
			objects.FieldKeyKind:     objects.KindRequirement,
			objects.FieldKeyID:       "REQ-1",
			objects.FieldKeyStatus:   objects.ObjectStatusComplete,
			objects.FieldKeyTitle:    "Test Requirement",
			objects.FieldKeyGoalRefs: []any{"GOAL-1"},
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-1",
			objects.FieldKeyStatus:          objects.ObjectStatusComplete,
			objects.FieldKeyTitle:           "Test Backlog Item",
			objects.FieldKeyPriorityPlanRef: "PRI-plan",
			objects.FieldKeyRequirementRefs: []any{"REQ-1"},
			objects.FieldKeyCriteriaRefs:    []any{"CRIT-1"},
		},
		map[string]any{
			objects.FieldKeyKind:   objects.KindCriteria,
			objects.FieldKeyID:     "CRIT-1",
			objects.FieldKeyStatus: objects.ObjectStatusComplete,
			objects.FieldKeyTitle:  "Test Criteria",
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-orphan",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyTitle:           "Orphan Item",
			objects.FieldKeyPriorityPlanRef: "PRI-plan",
		},
	)

	ctx := context.Background()
	resolver := NewDependencyTreeResolver(store)

	rootNode, err := resolver.Resolve(ctx, "PRI-plan")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// Verify root plan
	if rootNode.ID != "PRI-plan" || rootNode.Title != "Test Plan" {
		t.Errorf("unexpected root plan node: %+v", rootNode)
	}

	// Root should have 2 children: GOAL-1 (goal) and BLI-orphan (orphan backlog item)
	if len(rootNode.Children) != 2 {
		t.Fatalf("expected 2 children under root, got %d", len(rootNode.Children))
	}

	var goalNode, orphanNode *TreeNode
	for _, child := range rootNode.Children {
		if child.ID == "GOAL-1" {
			goalNode = child
		} else if child.ID == "BLI-orphan" {
			orphanNode = child
		}
	}

	if goalNode == nil {
		t.Fatalf("GOAL-1 node not found under root")
	}
	if orphanNode == nil {
		t.Fatalf("BLI-orphan node not found under root")
	}

	// GOAL-1 should have 1 child: REQ-1 (requirement)
	if len(goalNode.Children) != 1 {
		t.Fatalf("expected 1 child under goalNode, got %d", len(goalNode.Children))
	}
	reqNode := goalNode.Children[0]
	if reqNode.ID != "REQ-1" {
		t.Errorf("expected child to be REQ-1, got %s", reqNode.ID)
	}

	// REQ-1 should have 1 child: BLI-1
	if len(reqNode.Children) != 1 {
		t.Fatalf("expected 1 child under reqNode (BLI-1), got %d", len(reqNode.Children))
	}

	bliNode := reqNode.Children[0]
	if bliNode.ID != "BLI-1" {
		t.Fatalf("expected child of REQ-1 to be BLI-1, got %s", bliNode.ID)
	}

	// BLI-1 should have 1 child: CRIT-1 (acceptance criteria)
	if len(bliNode.Children) != 1 || bliNode.Children[0].ID != "CRIT-1" {
		t.Errorf("expected CRIT-1 under bliNode, got %+v", bliNode.Children)
	}
}

func TestTreeCmd(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	fs := proj.FileStorage
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	planID := "PRI-test"
	planObj := map[string]any{
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyID:            planID,
		objects.FieldKeyTitle:         "Tree Test Plan",
		objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, fs, ctx, secCtx, planObj, objects.ObjectStatusInProgress)

	goalID := "GOAL-test"
	goalObj := map[string]any{
		objects.FieldKeyKind:          objects.KindGoal,
		objects.FieldKeyID:            goalID,
		objects.FieldKeyTitle:         "Tree Goal",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, fs, ctx, secCtx, goalObj, objects.ObjectStatusActive)

	bliID := "BLI-test"
	bliObj := map[string]any{
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyID:              bliID,
		objects.FieldKeyTitle:           "Tree Backlog",
		objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
		objects.FieldKeyPriorityPlanRef: planID,
		objects.FieldKeyGoalRefs:        []string{goalID},
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, fs, ctx, secCtx, bliObj, objects.ObjectStatusPlanned)

	cmd := NewTreeCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	baseCtx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, secCtx)
	cmd.SetContext(cli.WithStorageProvider(baseCtx, fs))
	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	cctx.ProjectRoot = proj.Root
	cli.SetContext(cmd, cctx)

	cmd.SetArgs([]string{"--priority-plan", planID, "--depth", "5"})
	err = cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Tree Test Plan")
	assert.Contains(t, buf.String(), "Tree Goal")

	// Also test without priority-plan flag (auto-resolves in_progress plan)
	cmdAuto := NewTreeCmd()
	var bufAuto bytes.Buffer
	cmdAuto.SetOut(&bufAuto)
	cmdAuto.SetErr(&bufAuto)
	baseCtxAuto := pkgctx.WithCommandOutputWriter(context.Background(), &bufAuto)
	baseCtxAuto = pkgctx.WithSecurityContext(baseCtxAuto, secCtx)
	cmdAuto.SetContext(cli.WithStorageProvider(baseCtxAuto, fs))
	cctxAuto, err := cli.GetContextFromCommand(cmdAuto, initCtx)
	require.NoError(t, err)
	cctxAuto.ProjectRoot = proj.Root
	cli.SetContext(cmdAuto, cctxAuto)

	cmdAuto.SetArgs([]string{})
	err = cmdAuto.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufAuto.String(), "Tree Test Plan")
}

func TestToStringSlice(t *testing.T) {
	assert.Nil(t, toStringSlice(nil))
	assert.Nil(t, toStringSlice(12345))
	assert.Nil(t, toStringSlice(""))

	assert.Equal(t, []string{"single"}, toStringSlice("single"))
	assert.Equal(t, []string{"a", "b"}, toStringSlice([]string{"a", "b"}))
	assert.Equal(t, []string{"x", "y"}, toStringSlice([]any{"x", "", 123, "y"}))
}

func TestFilterBacklogsByPlan(t *testing.T) {
	items := []map[string]any{
		{
			objects.FieldKeyID:              "BLI-1",
			objects.FieldKeyPriorityPlanRef: "PRI-TARGET",
		},
		{
			objects.FieldKeyID:               "BLI-2",
			objects.FieldKeyPriorityPlanRefs: []any{"PRI-OTHER", "PRI-TARGET"},
		},
		{
			objects.FieldKeyID:              "BLI-3",
			objects.FieldKeyPriorityPlanRef: "PRI-OTHER",
		},
		{
			objects.FieldKeyID: "BLI-4",
		},
	}

	filtered := filterBacklogsByPlan(items, "PRI-TARGET")
	require.Len(t, filtered, 2)
	assert.Equal(t, "BLI-1", filtered[0][objects.FieldKeyID])
	assert.Equal(t, "BLI-2", filtered[1][objects.FieldKeyID])
}
