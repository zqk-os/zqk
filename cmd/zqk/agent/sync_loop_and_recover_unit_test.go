package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/audit"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/mutation"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

func TestSyncLoop_StripGraphBloat(t *testing.T) {
	obj := map[string]any{
		"id": "ATK-1",
		objects.FieldKeyResolvedRelatedObjectRefs: []any{"OBJ-2"},
		objects.FieldKeyStatusHistory:             []any{"history"},
		objects.FieldKeyChangeLog:                 []any{"log"},
		"resolved_children":                       []any{"child-1"},
		"nested": map[string]any{
			"resolved_sub": "remove_me",
			"keep":         "value",
		},
		"list_nested": []any{
			map[string]any{
				"resolved_item": "remove_too",
				"keep":          "item_val",
			},
		},
	}

	stripGraphBloat(obj)

	assert.Equal(t, "ATK-1", obj["id"])
	assert.Nil(t, obj[objects.FieldKeyResolvedRelatedObjectRefs])
	assert.Nil(t, obj[objects.FieldKeyStatusHistory])
	assert.Nil(t, obj[objects.FieldKeyChangeLog])
	assert.Nil(t, obj["resolved_children"])

	nested, ok := obj["nested"].(map[string]any)
	require.True(t, ok)
	assert.Nil(t, nested["resolved_sub"])
	assert.Equal(t, "value", nested["keep"])

	listNested, ok := obj["list_nested"].([]any)
	require.True(t, ok)
	require.Len(t, listNested, 1)
	item, ok := listNested[0].(map[string]any)
	require.True(t, ok)
	assert.Nil(t, item["resolved_item"])
	assert.Equal(t, "item_val", item["keep"])
}

func TestSyncLoop_BuildContextBundle(t *testing.T) {
	task := map[string]any{"id": "ATK-1", "resolved_test": "val"}
	deps := []map[string]any{{"id": "BLI-1", "resolved_dep": "dep_val"}}

	bundle := buildContextBundle(task, deps)
	assert.Equal(t, "ATK-1", bundle.Task["id"])
	assert.Nil(t, bundle.Task["resolved_test"])
	require.Len(t, bundle.Dependencies, 1)
	assert.Nil(t, bundle.Dependencies[0]["resolved_dep"])
}

func TestSyncLoop_Hourglass(t *testing.T) {
	sp := newMockHelperStore()
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	tmpDir := t.TempDir()

	task := map[string]any{
		objects.FieldKeyID:     "ATK-HG-001",
		objects.FieldKeyTitle:  "Hourglass test",
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
	}
	require.NoError(t, sp.Create(ctx, secCtx, task))

	flipHourglass(ctx, secCtx, sp, "ATK-HG-001", tmpDir)

	updated, err := sp.Read(ctx, secCtx, "ATK-HG-001")
	require.NoError(t, err)
	assert.NotEmpty(t, updated["agent_heartbeat"])
	assert.Equal(t, os.Getpid(), updated["agent_pid"])

	hgFile := filepath.Join(tmpDir, paths.ProjectDataDir, paths.SchedulerDir, "hourglass", "ATK-HG-001.json")
	assert.True(t, fileutil.Exists(hgFile))

	removeHourglass(ctx, secCtx, sp, "ATK-HG-001", tmpDir)

	cleaned, err := sp.Read(ctx, secCtx, "ATK-HG-001")
	require.NoError(t, err)
	assert.Nil(t, cleaned["agent_pid"])
	assert.False(t, fileutil.Exists(hgFile))
}

func TestSyncLoop_QuerySubgraph(t *testing.T) {
	sp := newMockHelperStore()
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Child task with no semantic payload
	task := map[string]any{
		objects.FieldKeyID:              "ATK-SUB-001",
		objects.FieldKeyKind:            objects.KindAgentTask,
		objects.FieldKeyRequirementRefs: []any{"REQ-SUB-001"},
	}
	require.NoError(t, sp.Create(ctx, secCtx, task))

	// Requirement with description (stops traversal upward)
	req := map[string]any{
		objects.FieldKeyID:          "REQ-SUB-001",
		objects.FieldKeyKind:        objects.KindRequirement,
		objects.FieldKeyDescription: "Concrete functional requirement",
	}
	require.NoError(t, sp.Create(ctx, secCtx, req))

	deps, err := QuerySubgraph(ctx, secCtx, sp, "ATK-SUB-001", 3)
	require.NoError(t, err)
	require.Len(t, deps, 1)
	assert.Equal(t, "REQ-SUB-001", deps[0]["id"])
}

func TestSeatWorkerFill_Helpers(t *testing.T) {
	notifyCmd := schedulerCallbackNotify("zqk", "/tmp/cb.log")
	assert.Equal(t, "zqk callback notify --log-file /tmp/cb.log", notifyCmd)

	tmp := t.TempDir()
	path := fillSubmitMarkPath(tmp, "plan")
	assert.Contains(t, path, "fill-plan.json")

	isRecent, _ := recentFillSubmit(tmp, "plan")
	assert.False(t, isRecent)

	require.NoError(t, writeFillSubmitMark(tmp, "plan", "output detail"))
	isRecentAfter, msg := recentFillSubmit(tmp, "plan")
	assert.True(t, isRecentAfter)
	assert.Contains(t, msg, "already_submitted")
}

func TestSeatWorkerFill_SubmitKernelFill_Bypasses(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()

	// nil fill
	res, err := submitKernelFill(ctx, tmp, nil)
	require.NoError(t, err)
	assert.Empty(t, res)

	// AutoSubmit false
	res, err = submitKernelFill(ctx, tmp, &whatsnext.FillItem{AutoSubmit: false, SubmitArgs: "run"})
	require.NoError(t, err)
	assert.Empty(t, res)

	// Empty SubmitArgs
	res, err = submitKernelFill(ctx, tmp, &whatsnext.FillItem{AutoSubmit: true, SubmitArgs: ""})
	require.NoError(t, err)
	assert.Empty(t, res)

	// Recently submitted bypass
	require.NoError(t, writeFillSubmitMark(tmp, "bli", "detail"))
	res, err = submitKernelFill(ctx, tmp, &whatsnext.FillItem{AutoSubmit: true, Kind: "bli", SubmitArgs: "run"})
	require.NoError(t, err)
	assert.Contains(t, res, "already_submitted")
}

func TestRecover_Helpers(t *testing.T) {
	filters := recoverAgentTaskListFilters("PRI-1")
	assert.NotEmpty(t, filters)

	objectsToMerge := []map[string]any{
		{"id": "OBJ-1", "title": "First"},
		{"id": "OBJ-2", "title": "Second"},
		{"id": "OBJ-1", "title": "First Updated"},
	}
	merged := mergeObjectsByID(objectsToMerge)
	require.Len(t, merged, 2)
	assert.Equal(t, "OBJ-1", merged[0]["id"])
}

func TestQuerySubgraph_SemanticPayloadBranches(t *testing.T) {
	provider := newMockHelperStore()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	sysCtx := pkgctx.NewSystemContext()

	// Seed root task referencing dependency
	task := map[string]any{
		objects.FieldKeyID:                 "ATK-ROOT-001",
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Root Task",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaDefaultOperator,
		objects.FieldKeyDescription:        "Root Task Description",
		objects.FieldKeyRequirementRefs:    []any{"REQ-SEM-001"},
	}
	require.NoError(t, provider.Create(sysCtx, secCtx, task))

	// Dependency with problem_statement
	dep := map[string]any{
		objects.FieldKeyID:                 "REQ-SEM-001",
		objects.FieldKeyKind:               objects.KindRequirement,
		objects.FieldKeyTitle:              "Sem Requirement",
		objects.FieldKeyStatus:             objects.ObjectStatusProposed,
		"problem_statement":                "A concrete problem statement",
		objects.FieldKeyAcceptanceCriteria: []any{"Criteria 1"},
	}
	require.NoError(t, provider.Create(sysCtx, secCtx, dep))

	// Traverse with depth 0 (defaults to 5)
	subgraph, err := QuerySubgraph(ctx, secCtx, provider, "ATK-ROOT-001", 0)
	require.NoError(t, err)
	assert.NotEmpty(t, subgraph)
}

func TestApplyStateMutationWithFields_ExtraFieldsAndDefaultAssignee(t *testing.T) {
	tempDir := t.TempDir()
	provider := newMockHelperStore()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	sysCtx := pkgctx.NewSystemContext()

	// Seed persona so default assignee_persona_ref passes reference check
	require.NoError(t, provider.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:          objects.ConstPersonaOrchestratorAlpha,
		objects.FieldKeyKind:        objects.KindPersona,
		objects.FieldKeyTitle:       "Orchestrator Alpha",
		objects.FieldKeyDescription: "Orchestrator Alpha Persona",
		objects.FieldKeyStatus:      objects.ObjectStatusApproved,
	}))

	taskID := "ATK-MUT-001"
	task := map[string]any{
		objects.FieldKeyID:          taskID,
		objects.FieldKeyKind:        objects.KindAgentTask,
		objects.FieldKeyTitle:       "Mutation Task",
		objects.FieldKeyStatus:      objects.ObjectStatusProposed,
		objects.FieldKeyDescription: "Task to mutate",
	}
	require.NoError(t, provider.Create(sysCtx, secCtx, task))

	validator := mutation.NewValidator(nil)
	auditStream := audit.NewAuditStream(tempDir)

	extra := map[string]any{
		objects.FieldKeyID:        taskID,                       // skipped
		objects.FieldKeyKind:      objects.KindAgentTask,        // skipped
		objects.FieldKeyStatus:    objects.ObjectStatusApproved, // skipped
		objects.FieldKeyClaimedBy: "agent-custom",
	}

	err := applyStateMutationWithFields(ctx, secCtx, provider, taskID, objects.KindAgentTask, validator, auditStream, objects.ObjectStatusApproved, extra)
	require.NoError(t, err)

	updated, err := provider.Read(ctx, secCtx, taskID)
	require.NoError(t, err)
	assert.Equal(t, objects.ObjectStatusApproved, updated[objects.FieldKeyStatus])
	assert.Equal(t, objects.ConstPersonaOrchestratorAlpha, updated[objects.FieldKeyAssigneePersonaRef])
	assert.Equal(t, "agent-custom", updated[objects.FieldKeyClaimedBy])
}
