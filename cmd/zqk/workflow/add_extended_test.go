package workflow

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestMilestoneReferencesPlan(t *testing.T) {
	planID := "PRI-TEST-PLAN"

	// 1. Empty plan or nil obj
	assert.False(t, milestoneReferencesPlan(nil, planID))
	assert.False(t, milestoneReferencesPlan(map[string]any{}, ""))

	// 2. related_object_refs []any
	mAny := map[string]any{
		objects.FieldKeyRelatedObjectRefs: []any{planID},
	}
	assert.True(t, milestoneReferencesPlan(mAny, planID))

	// 3. related_object_refs []string
	mStr := map[string]any{
		objects.FieldKeyRelatedObjectRefs: []string{planID},
	}
	assert.True(t, milestoneReferencesPlan(mStr, planID))

	// 4. priority_plan_ref string
	mSingular := map[string]any{
		objects.FieldKeyPriorityPlanRef: planID,
	}
	assert.True(t, milestoneReferencesPlan(mSingular, planID))

	// 5. priority_plan_refs []any
	mRefsAny := map[string]any{
		objects.FieldKeyPriorityPlanRefs: []any{planID},
	}
	assert.True(t, milestoneReferencesPlan(mRefsAny, planID))

	// 6. priority_plan_refs []string
	mRefsStr := map[string]any{
		objects.FieldKeyPriorityPlanRefs: []string{planID},
	}
	assert.True(t, milestoneReferencesPlan(mRefsStr, planID))

	// 7. Unrelated
	mOther := map[string]any{
		objects.FieldKeyPriorityPlanRef: "PRI-OTHER",
	}
	assert.False(t, milestoneReferencesPlan(mOther, planID))
}

func TestRunAdd_ValidationErrors(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// 1. Missing args
	cmd := NewAddCmd()
	initTestCmdContext(t, cmd, proj.Root, nil)
	err := runAdd(cmd, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one backlog item ID is required")

	// 2. Override without reason code
	cmdOverride := NewAddCmd()
	initTestCmdContext(t, cmdOverride, proj.Root, nil)
	require.NoError(t, cmdOverride.Flags().Set("override", "true"))
	err = runAdd(cmdOverride, []string{"BLI-001"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--reason-code is required when using --override")
}

func TestRunAdd_SuccessfulAdd(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	store := &mockWhatsNextStorage{
		data: map[string]map[string]any{
			"PRI-ADD-001": {
				objects.FieldKeyID:          "PRI-ADD-001",
				objects.FieldKeyKind:        objects.KindPriorityPlan,
				objects.FieldKeyTitle:       "Add Plan",
				objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
				objects.FieldKeyActiveOrder: 1,
				objects.FieldKeyGoalRefs:    []any{"GOAL-ADD-001"},
			},
			"BLI-ADD-001": {
				objects.FieldKeyID:           "BLI-ADD-001",
				objects.FieldKeyKind:         objects.KindBacklogItem,
				objects.FieldKeyTitle:        "Add BLI",
				objects.FieldKeyStatus:       objects.ObjectStatusPlanned,
				objects.FieldKeyPriority:     "p1",
				objects.FieldKeyCriteriaRefs: []any{"CRIT-ADD-001"},
			},
			"MIL-ADD-001": {
				objects.FieldKeyID:                "MIL-ADD-001",
				objects.FieldKeyKind:              objects.KindMilestone,
				objects.FieldKeyTitle:             "Add Milestone",
				objects.FieldKeyStatus:            objects.ObjectStatusNotStarted,
				objects.FieldKeyRelatedObjectRefs: []any{"PRI-ADD-001"},
			},
			"GOAL-ADD-001": {
				objects.FieldKeyID:     "GOAL-ADD-001",
				objects.FieldKeyKind:   objects.KindGoal,
				objects.FieldKeyTitle:  "Add Goal",
				objects.FieldKeyStatus: objects.ObjectStatusActive,
			},
		},
	}

	cmd := NewAddCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd, proj.Root, store)

	// Add BLI to plan
	err := runAdd(cmd, []string{"BLI-ADD-001", "PRI-ADD-001"})
	require.NoError(t, err)

	// Add BLI to plan with explicit milestone flag
	cmdMilestone := NewAddCmd()
	cmdMilestone.SetOut(&buf)
	cmdMilestone.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	require.NoError(t, cmdMilestone.Flags().Set("milestone", "MIL-ADD-001"))
	initTestCmdContext(t, cmdMilestone, proj.Root, store)
	err = runAdd(cmdMilestone, []string{"BLI-ADD-001", "PRI-ADD-001"})
	require.NoError(t, err)
}

func TestScaffoldPlanMilestoneGoal(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	store := &mockWhatsNextStorage{
		data: make(map[string]map[string]any),
	}

	cmd := NewAddCmd()
	initTestCmdContext(t, cmd, proj.Root, store)
	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)

	planID, milestoneID, goalID, err := scaffoldPlanMilestoneGoal(proc.OperationContext(), proc, 1)
	require.NoError(t, err)
	assert.NotEmpty(t, planID)
	assert.NotEmpty(t, milestoneID)
	assert.NotEmpty(t, goalID)
}

func TestRunAdd_EdgeCases(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	store := &mockWhatsNextStorage{
		data: map[string]map[string]any{
			"GOAL-NOT-BLI": {
				objects.FieldKeyID:     "GOAL-NOT-BLI",
				objects.FieldKeyKind:   objects.KindGoal,
				objects.FieldKeyStatus: objects.ObjectStatusActive,
			},
			"BLI-COMPLETED": {
				objects.FieldKeyID:     "BLI-COMPLETED",
				objects.FieldKeyKind:   objects.KindBacklogItem,
				objects.FieldKeyStatus: objects.ObjectStatusComplete,
			},
			"PRI-EXISTING": {
				objects.FieldKeyID:          "PRI-EXISTING",
				objects.FieldKeyKind:        objects.KindPriorityPlan,
				objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
				objects.FieldKeyActiveOrder: 1,
			},
		},
	}

	cmd := NewAddCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd, proj.Root, store)

	// 1. Not backlog_item kind
	err := runAdd(cmd, []string{"GOAL-NOT-BLI", "PRI-EXISTING"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be 'backlog_item'")

	// 2. Terminal status
	err = runAdd(cmd, []string{"BLI-COMPLETED", "PRI-EXISTING"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "terminal status")
}

func TestRunAdd_AutoScaffold(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	store := &mockWhatsNextStorage{
		data: map[string]map[string]any{
			"BLI-SCAFFOLD": {
				objects.FieldKeyID:       "BLI-SCAFFOLD",
				objects.FieldKeyKind:     objects.KindBacklogItem,
				objects.FieldKeyTitle:    "Scaffold BLI",
				objects.FieldKeyStatus:   objects.ObjectStatusPlanned,
				objects.FieldKeyPriority: "p1",
			},
		},
	}

	cmd := NewAddCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd, proj.Root, store)

	// Add without planID -> triggers auto scaffold
	err := runAdd(cmd, []string{"BLI-SCAFFOLD"})
	require.NoError(t, err)
}
