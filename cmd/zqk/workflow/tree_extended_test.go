package workflow

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestNewTreeCmd(t *testing.T) {
	cmd := NewTreeCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "tree", cmd.Use)
}

func TestPrintTreeAndDescendants(t *testing.T) {
	leaf1 := &TreeNode{ID: "CRIT-1", Status: "validated", Title: "Leaf 1"}
	leaf2 := &TreeNode{ID: "CRIT-2", Status: "validated", Title: "Leaf 2"}
	midNode := &TreeNode{
		ID:       "REQ-1",
		Status:   "active",
		Title:    "Mid Req",
		Children: []*TreeNode{leaf1, leaf2},
	}
	rootNode := &TreeNode{
		ID:       "PRI-1",
		Status:   "in_progress",
		Title:    "Root Plan",
		Children: []*TreeNode{midNode},
	}

	// countTotalDescendants
	assert.Equal(t, 0, countTotalDescendants(leaf1))
	assert.Equal(t, 2, countTotalDescendants(midNode))
	assert.Equal(t, 3, countTotalDescendants(rootNode))

	// printTree with full depth
	var b strings.Builder
	printTree(&b, rootNode, "", true, 0, 5)
	out := b.String()
	assert.Contains(t, out, "PRI-1 [in_progress] Root Plan")
	assert.Contains(t, out, "REQ-1 [active] Mid Req")
	assert.Contains(t, out, "CRIT-1 [validated] Leaf 1")
	assert.Contains(t, out, "CRIT-2 [validated] Leaf 2")

	// printTree with shallow depth (triggers rolled up dependencies)
	b.Reset()
	printTree(&b, rootNode, "", true, 0, 0)
	outShallow := b.String()
	assert.Contains(t, outShallow, "rolled up dependencies")
}

func TestResolvePriorityPlan_EdgeCases(t *testing.T) {
	ctx := context.Background()

	// 1. Explicit plan not found
	storeEmpty := newMemoryWorkflowStore()
	_, err := resolvePriorityPlan(ctx, storeEmpty, "NON-EXISTENT")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "NON-EXISTENT not found")

	// 2. No plan specified, no active/in_progress plans exist
	_, err = resolvePriorityPlan(ctx, storeEmpty, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no active, in_progress, or paused priority plan found")

	// 3. No plan specified, single active plan
	storeActive := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-ACTIVE",
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		},
	)
	planID, err := resolvePriorityPlan(ctx, storeActive, "")
	require.NoError(t, err)
	assert.Equal(t, "PRI-ACTIVE", planID)

	// 4. No plan specified, paused plan
	storePaused := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-PAUSED",
			objects.FieldKeyStatus: objects.ObjectStatusPaused,
		},
	)
	planID, err = resolvePriorityPlan(ctx, storePaused, "")
	require.NoError(t, err)
	assert.Equal(t, "PRI-PAUSED", planID)
}

type mockTreeStorage struct {
	storage.ObjectStorageProvider
	data map[string]map[string]any
}

func (m *mockTreeStorage) Read(ctx context.Context, sec *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.data[id]; ok {
		return obj, nil
	}
	return nil, fmt.Errorf("object %s not found", id)
}

func (m *mockTreeStorage) List(ctx context.Context, sec *pkgctx.SecurityContext, stCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	var out []map[string]any
	for _, obj := range m.data {
		out = append(out, obj)
	}
	return &storage.QueryResult{Objects: out}, nil
}

func (m *mockTreeStorage) Count(ctx context.Context, sec *pkgctx.SecurityContext, filter storage.ListFilter) (int, error) {
	return 0, nil
}

func TestRunTree(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	store := &mockTreeStorage{
		data: map[string]map[string]any{
			"PRI-TREE-1": {
				objects.FieldKeyKind:   objects.KindPriorityPlan,
				objects.FieldKeyID:     "PRI-TREE-1",
				objects.FieldKeyTitle:  "Tree Test Plan",
				objects.FieldKeyStatus: objects.ObjectStatusInProgress,
			},
			"BLI-TREE-1": {
				objects.FieldKeyKind:            objects.KindBacklogItem,
				objects.FieldKeyID:              "BLI-TREE-1",
				objects.FieldKeyTitle:           "Tree Backlog Item",
				objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
				objects.FieldKeyPriorityPlanRef: "PRI-TREE-1",
			},
		},
	}

	// 1. Text output
	cmd := NewTreeCmd()
	initTestCmdContext(t, cmd, proj.Root, store)
	var outBuf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(cmd.Context(), &outBuf))
	require.NoError(t, cmd.Flags().Set("priority-plan", "PRI-TREE-1"))

	err := runTree(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, outBuf.String(), "PRI-TREE-1")

	// 2. JSON output
	cmdJSON := NewTreeCmd()
	require.NoError(t, cmdJSON.Flags().Set("priority-plan", "PRI-TREE-1"))
	require.NoError(t, cmdJSON.Flags().Set("format", "json"))
	initTestCmdContext(t, cmdJSON, proj.Root, store)
	var jsonBuf bytes.Buffer
	cmdJSON.SetContext(pkgctx.WithCommandOutputWriter(cmdJSON.Context(), &jsonBuf))

	err = runTree(cmdJSON, nil)
	require.NoError(t, err)
	assert.Contains(t, jsonBuf.String(), `"id": "PRI-TREE-1"`)
}

func TestFilterBacklogsByPlan_SliceRefs(t *testing.T) {
	// Test backlog item having priority_plan_refs slice instead of singular priority_plan_ref
	backlogs := []map[string]any{
		{
			objects.FieldKeyID:   "BLI-SLICE-1",
			"priority_plan_refs": []any{"PRI-A", "PRI-B"},
		},
		{
			objects.FieldKeyID:   "BLI-SLICE-2",
			"priority_plan_refs": []any{"PRI-B", "PRI-C"},
		},
		{
			objects.FieldKeyID:              "BLI-SLICE-3",
			objects.FieldKeyPriorityPlanRef: "PRI-D",
		},
	}

	filteredA := filterBacklogsByPlan(backlogs, "PRI-A")
	assert.Len(t, filteredA, 1)
	assert.Equal(t, "BLI-SLICE-1", filteredA[0][objects.FieldKeyID])

	filteredB := filterBacklogsByPlan(backlogs, "PRI-B")
	assert.Len(t, filteredB, 2)

	filteredD := filterBacklogsByPlan(backlogs, "PRI-D")
	assert.Len(t, filteredD, 1)
}

func TestResolvePlanGoals_Coverage(t *testing.T) {
	goals := []map[string]any{
		{
			objects.FieldKeyID:              "GOAL-1",
			objects.FieldKeyPriorityPlanRef: "PRI-X",
		},
		{
			objects.FieldKeyID:               "GOAL-2",
			objects.FieldKeyPriorityPlanRefs: []any{"PRI-X"},
		},
		{
			objects.FieldKeyID:                "GOAL-3",
			objects.FieldKeyRelatedObjectRefs: []any{"PRI-X"},
		},
		{
			objects.FieldKeyID:              "GOAL-OTHER",
			objects.FieldKeyPriorityPlanRef: "PRI-OTHER",
		},
	}
	backlogs := []map[string]any{
		{
			objects.FieldKeyID:       "BLI-1",
			objects.FieldKeyGoalRefs: []any{"GOAL-FROM-BLI"},
		},
	}
	set := resolvePlanGoals(goals, backlogs, "PRI-X")
	assert.True(t, set["GOAL-1"])
	assert.True(t, set["GOAL-2"])
	assert.True(t, set["GOAL-3"])
	assert.True(t, set["GOAL-FROM-BLI"])
	assert.False(t, set["GOAL-OTHER"])
}

func TestMapBacklogsToGoals_Coverage(t *testing.T) {
	backlogs := []map[string]any{
		{
			objects.FieldKeyID:       "BLI-CHILD",
			objects.FieldKeyStatus:   "planned",
			objects.FieldKeyTitle:    "Child BLI",
			objects.FieldKeyGoalRefs: []any{"GOAL-P"},
		},
	}
	m := mapBacklogsToGoals(backlogs)
	assert.Len(t, m["GOAL-P"], 1)
	assert.Equal(t, "BLI-CHILD", m["GOAL-P"][0][objects.FieldKeyID])
}
