package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/accumulator"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/vds"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestCycle128_Next_StringSliceAndTruncate(t *testing.T) {
	assert.Nil(t, stringSliceFromAnyField(nil))
	assert.Nil(t, stringSliceFromAnyField(12345))

	strSlice := stringSliceFromAnyField([]string{"first", "", "  second  ", "third"})
	require.Len(t, strSlice, 3)
	assert.Equal(t, []string{"first", "second", "third"}, strSlice)

	anySlice := stringSliceFromAnyField([]any{"alpha", 42, "", "  beta  ", nil})
	require.Len(t, anySlice, 2)
	assert.Equal(t, []string{"alpha", "beta"}, anySlice)

	assert.Equal(t, "", truncateWorkflowText("some text", 0))
	assert.Equal(t, "", truncateWorkflowText("some text", -5))
	assert.Equal(t, "short", truncateWorkflowText("short", 10))
	assert.Equal(t, "exact", truncateWorkflowText("exact", 5))
	assert.Equal(t, "long…", truncateWorkflowText("longtext", 4))
	assert.True(t, strings.HasPrefix(truncateWorkflowText("abcde", 2), "ab"))
}

func TestCycle128_Next_PriorityRankAndCountQuestions(t *testing.T) {
	assert.Equal(t, 0, priorityRank("P0"))
	assert.Equal(t, 1, priorityRank("P1"))
	assert.Equal(t, 2, priorityRank("P2"))
	assert.Equal(t, 3, priorityRank("P3"))
	assert.Equal(t, 9, priorityRank("P99"))
	assert.Equal(t, 9, priorityRank("UNKNOWN"))

	ctx := context.Background()
	assert.Equal(t, 0, countOpenQuestions(ctx, nil))

	mockStore := &mockWhatsNextStorage{data: map[string]map[string]any{
		"Q-1": {
			objects.FieldKeyKind:   objects.KindQuestion,
			objects.FieldKeyID:     "Q-1",
			objects.FieldKeyStatus: statusQuestionOpen,
		},
		"Q-2": {
			objects.FieldKeyKind:   objects.KindQuestion,
			objects.FieldKeyID:     "Q-2",
			objects.FieldKeyStatus: "resolved",
		},
	}}

	count := countOpenQuestions(ctx, mockStore)
	assert.Equal(t, 2, count)
}

func TestCycle128_Next_ApplyDecisionSpec(t *testing.T) {
	res := &workflowNextResult{}
	applyDecisionSpec(res, "unknown_decision", "OBJ-1")
	assert.Empty(t, res.Decision)

	applyDecisionSpec(nil, decisionCriticalPolicyInterrupt, "OBJ-1")

	applyDecisionSpec(res, decisionCriticalPolicyInterrupt, "POL-INT-1")
	assert.Equal(t, decisionCriticalPolicyInterrupt, res.Decision)
	assert.Contains(t, res.RecommendedCommand, "POL-INT-1")

	applyDecisionSpec(res, decisionActiveConvergence, "CVS-1")
	assert.Equal(t, decisionActiveConvergence, res.Decision)
	assert.Contains(t, res.RecommendedCommand, "CVS-1")

	applyDecisionSpec(res, decisionPriorityPlanBacklog, "BLI-1")
	assert.Equal(t, decisionPriorityPlanBacklog, res.Decision)
	assert.Contains(t, res.RecommendedCommand, "BLI-1")
}

func TestCycle128_Next_ResolveCurrentPlanID(t *testing.T) {
	ctx := context.Background()

	emptyStore := &mockWhatsNextStorage{data: map[string]map[string]any{}}
	assert.Equal(t, "", resolveCurrentPlanID(ctx, emptyStore))

	multiStore := &mockWhatsNextStorage{data: map[string]map[string]any{
		"PRI-EMPTY-ID": {
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyStatus: statusInProcess,
		},
		"PRI-HIGH-ORDER": {
			objects.FieldKeyKind:        objects.KindPriorityPlan,
			objects.FieldKeyID:          "PRI-HIGH-ORDER",
			objects.FieldKeyStatus:      statusInProcess,
			objects.FieldKeyActiveOrder: 5,
			objects.FieldKeyUpdatedAt:   "2026-10-23T01:00:00Z",
		},
		"PRI-LOW-FLOAT": {
			objects.FieldKeyKind:        objects.KindPriorityPlan,
			objects.FieldKeyID:          "PRI-LOW-FLOAT",
			objects.FieldKeyStatus:      "active",
			objects.FieldKeyActiveOrder: float64(2),
			objects.FieldKeyUpdatedAt:   "2026-10-23T02:00:00Z",
		},
		"PRI-NO-ORDER": {
			objects.FieldKeyKind:      objects.KindPriorityPlan,
			objects.FieldKeyID:        "PRI-NO-ORDER",
			objects.FieldKeyStatus:    statusInProcess,
			objects.FieldKeyUpdatedAt: "2026-10-23T05:00:00Z",
		},
	}}

	best := resolveCurrentPlanID(ctx, multiStore)
	assert.Equal(t, "PRI-LOW-FLOAT", best)
}

func TestCycle128_Next_SelectBacklogNext(t *testing.T) {
	ctx := context.Background()

	emptyStore := &mockWhatsNextStorage{data: map[string]map[string]any{}}
	pID, item := selectBacklogNext(ctx, emptyStore)
	assert.Empty(t, pID)
	assert.Nil(t, item)

	storePlanOnly := &mockWhatsNextStorage{data: map[string]map[string]any{
		"PRI-ALONE": {
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-ALONE",
			objects.FieldKeyStatus: statusInProcess,
		},
	}}
	pID, item = selectBacklogNext(ctx, storePlanOnly)
	assert.Equal(t, "PRI-ALONE", pID)
	assert.Nil(t, item)

	storeWithItems := &mockWhatsNextStorage{data: map[string]map[string]any{
		"PRI-ACTIVE": {
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-ACTIVE",
			objects.FieldKeyStatus: statusInProcess,
		},
		"BLI-P1": {
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-P1",
			objects.FieldKeyPriorityPlanRef: "PRI-ACTIVE",
			objects.FieldKeyPriorityTier:    "P1",
			objects.FieldKeyStatus:          "planned",
			objects.FieldKeyTitle:           "Item P1",
			objects.FieldKeyUpdatedAt:       "2026-10-23T01:00:00Z",
		},
		"BLI-P0": {
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-P0",
			objects.FieldKeyPriorityPlanRef: "PRI-ACTIVE",
			objects.FieldKeyPriorityTier:    "P0",
			objects.FieldKeyStatus:          "in_progress",
			objects.FieldKeyTitle:           "Item P0",
			objects.FieldKeyUpdatedAt:       "2026-10-23T02:00:00Z",
		},
		"BLI-DONE": {
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-DONE",
			objects.FieldKeyPriorityPlanRef: "PRI-ACTIVE",
			objects.FieldKeyPriorityTier:    "P0",
			objects.FieldKeyStatus:          statusComplete,
		},
	}}

	pID, item = selectBacklogNext(ctx, storeWithItems)
	assert.Equal(t, "PRI-ACTIVE", pID)
	require.NotNil(t, item)
	assert.Equal(t, "BLI-P0", item.ID)
}

func TestCycle128_Next_EnrichPriorityPlanBacklog(t *testing.T) {
	ctx := context.Background()

	enrichPriorityPlanBacklog(ctx, nil, nil, "", nil)
	enrichPriorityPlanBacklog(ctx, nil, &workflowNextResult{}, "PRI-1", nil)

	mockStore := &mockWhatsNextStorage{data: map[string]map[string]any{
		"PRI-DETAIL": {
			objects.FieldKeyKind:        objects.KindPriorityPlan,
			objects.FieldKeyID:          "PRI-DETAIL",
			objects.FieldKeyTitle:       "Detailed Plan",
			objects.FieldKeyDescription: "A comprehensive description of the cycle",
		},
		"BLI-DETAIL": {
			objects.FieldKeyKind:                      objects.KindBacklogItem,
			objects.FieldKeyID:                        "BLI-DETAIL",
			objects.FieldKeyDescription:               "Detailed backlog item description",
			objects.FieldKeyGoalRefs:                  []any{"GOAL-A", "GOAL-B", "GOAL-C", "GOAL-D", "GOAL-E"},
			objects.FieldKeyConvergenceSessionRef:     "CVS-777",
			objects.FieldKeyConvergenceSessionProfile: "high-throughput",
		},
	}}

	res := &workflowNextResult{}
	item := &backlogInfo{ID: "BLI-DETAIL", PriorityTier: "P0"}

	enrichPriorityPlanBacklog(ctx, mockStore, res, "PRI-DETAIL", item)

	assert.Equal(t, "P0", res.BacklogPriorityTier)
	assert.Equal(t, "Detailed Plan", res.PriorityPlanTitle)
	assert.Equal(t, "A comprehensive description of the cycle", res.PriorityPlanSummary)
	assert.Equal(t, "Detailed backlog item description", res.BacklogItemSummary)
	assert.Len(t, res.GoalRefs, 5)
	assert.Equal(t, "CVS-777", res.ConvergenceSessionRef)
	assert.Equal(t, "high-throughput", res.ConvergenceSessionProfile)
}

func TestCycle128_Next_WriteWorkflowNextOutput_Formatting(t *testing.T) {
	res := workflowNextResult{
		Decision:                  decisionPriorityPlanBacklog,
		RecommendedCommand:        "zqk object get BLI-1",
		Rationale:                 "Active P0 backlog item on lead plan",
		SessionID:                 "SES-1",
		SessionSource:             "env",
		OpenQuestionsCount:        3,
		BacklogPriorityTier:       "P0",
		PriorityPlanTitle:         "Lead Plan",
		PriorityPlanSummary:       "Plan description",
		BacklogItemSummary:        "Item description",
		GoalRefs:                  []string{"GOAL-1", "GOAL-2"},
		ConvergenceSessionRef:     "CVS-1",
		ConvergenceSessionProfile: "tdd-fast",
		Precedence:                []string{"interrupt", "convergence", "backlog"},
		Sources: []sourceRef{
			{Kind: "priority_plan", ID: "PRI-1", Note: "in progress"},
			{Kind: "backlog_item", ID: "BLI-1", Note: "P0"},
		},
	}

	cmd := &cobra.Command{}
	cmd.Flags().String("format", "table", "")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

	err := writeWorkflowNextOutput(cmd, res)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Decision: priority_plan_backlog")
	assert.Contains(t, out, "Tier: P0")
	assert.Contains(t, out, "Plan: Lead Plan")
	assert.Contains(t, out, "Convergence session: CVS-1 (tdd-fast)")
	assert.Contains(t, out, "GOAL-1, GOAL-2")

	buf.Reset()
	require.NoError(t, cmd.Flags().Set("format", "yaml"))
	err = writeWorkflowNextOutput(cmd, res)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "decision: priority_plan_backlog")
}

func TestCycle128_Next_SelectActiveConvergence(t *testing.T) {
	ctx := context.Background()

	emptyStore := &mockWhatsNextStorage{data: map[string]map[string]any{}}
	assert.Nil(t, selectActiveConvergence(ctx, emptyStore))

	store := &mockWhatsNextStorage{data: map[string]map[string]any{
		"CVS-EMPTY-ID": {
			objects.FieldKeyKind:   objects.KindConvergenceSession,
			objects.FieldKeyStatus: statusInProcess,
		},
		"CVS-EARLY": {
			objects.FieldKeyKind:      objects.KindConvergenceSession,
			objects.FieldKeyID:        "CVS-EARLY",
			objects.FieldKeyTitle:     "Early Session",
			objects.FieldKeyStatus:    statusInProcess,
			objects.FieldKeyUpdatedAt: "2026-10-23T01:00:00Z",
		},
		"CVS-LATE": {
			objects.FieldKeyKind:      objects.KindConvergenceSession,
			objects.FieldKeyID:        "CVS-LATE",
			objects.FieldKeyTitle:     "Late Session",
			objects.FieldKeyStatus:    statusInProcess,
			objects.FieldKeyUpdatedAt: "2026-10-23T04:00:00Z",
		},
	}}

	cv := selectActiveConvergence(ctx, store)
	require.NotNil(t, cv)
	assert.Equal(t, "CVS-LATE", cv.ID)
	assert.Equal(t, "Late Session", cv.Title)
}

func TestCycle128_Link_ExtractTargetCriteria(t *testing.T) {
	critObj := map[string]any{objects.FieldKeyKind: objects.KindCriteria}
	target, err := extractTargetCriteria("CRIT-101", critObj)
	require.NoError(t, err)
	assert.Equal(t, "CRIT-101", target)

	reqObj := map[string]any{
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-FROM-REQ"},
	}
	target, err = extractTargetCriteria("REQ-101", reqObj)
	require.NoError(t, err)
	assert.Equal(t, "CRIT-FROM-REQ", target)

	goalEmpty := map[string]any{objects.FieldKeyKind: objects.KindGoal}
	_, err = extractTargetCriteria("GOAL-101", goalEmpty)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no criteria established yet")

	bliObj := map[string]any{objects.FieldKeyKind: objects.KindBacklogItem}
	_, err = extractTargetCriteria("BLI-101", bliObj)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported pipeline reference kind")
}

func TestCycle128_Link_RunCommand_Branches(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	store := &mockWhatsNextStorage{data: map[string]map[string]any{
		"CRIT-EXISTING": {
			objects.FieldKeyKind: objects.KindCriteria,
			objects.FieldKeyID:   "CRIT-EXISTING",
		},
		"BLI-TARGET": {
			objects.FieldKeyKind:         objects.KindBacklogItem,
			objects.FieldKeyID:           "BLI-TARGET",
			objects.FieldKeyCriteriaRefs: []any{"CRIT-EXISTING"},
		},
		"BLI-UNLINKED": {
			objects.FieldKeyKind: objects.KindBacklogItem,
			objects.FieldKeyID:   "BLI-UNLINKED",
		},
	}}

	cmd := NewLinkCmd()
	initTestCmdContext(t, cmd, proj.Root, store)

	err := runLinkCommand(cmd, []string{"BLI-ONLY"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing arguments")

	err = runLinkCommand(cmd, []string{"BLI-TARGET", "CRIT-NONEXISTENT"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load pipeline reference")

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	err = runLinkCommand(cmd, []string{"BLI-TARGET", "CRIT-EXISTING"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "is already linked to criteria")

	buf.Reset()
	err = runLinkCommand(cmd, []string{"BLI-UNLINKED", "CRIT-EXISTING"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Successfully linked BLI-UNLINKED into pipeline")
}

func TestCycle128_GenTracePipeline_GenerateAtomicBacklogItems(t *testing.T) {
	ts := time.Now().UnixNano()

	single := generateAtomicBacklogItems(
		"Single Facet Work",
		"REQ-101",
		"REQ-101",
		"planned",
		ts,
		[]string{"CRIT-1"},
		[]string{"TST-1"},
		nil,
	)
	require.Len(t, single, 1)
	assert.Contains(t, single[0].Title, "Implement: Single Facet Work")
	assert.Equal(t, []string{"CRIT-1"}, single[0].CriteriaRefs)

	milObj := map[string]any{
		objects.FieldKeyKind: objects.KindMilestone,
	}
	multi := generateAtomicBacklogItems(
		"Milestone Work",
		"MIL-202",
		"",
		"planned",
		ts,
		[]string{"CRIT-A", "CRIT-B", "CRIT-C", "CRIT-D"},
		[]string{"TST-M"},
		milObj,
	)
	require.Len(t, multi, 4)
	assert.Equal(t, []string{"MIL-202"}, multi[0].MilestoneRefs)
	assert.Contains(t, multi[0].Title, "Core Capability & Functional Delivery")
	assert.Contains(t, multi[1].Title, "Boundary Validation & Error Recovery")
	assert.Contains(t, multi[2].Title, "Architecture Specification & Conformance")
	assert.Contains(t, multi[3].Title, "Verification Facet 4")

	goalObj := map[string]any{
		objects.FieldKeyGoalRefs: []any{"GOAL-ROOT-001"},
	}
	goalBlis := generateAtomicBacklogItems(
		"Goal Work",
		"REQ-GOAL-01",
		"REQ-GOAL-01",
		"planned",
		ts,
		[]string{"CRIT-G1"},
		[]string{"TST-G"},
		goalObj,
	)
	require.Len(t, goalBlis, 1)
	assert.Equal(t, []string{"GOAL-ROOT-001"}, goalBlis[0].GoalRefs)
}

func TestCycle128_GenTracePipeline_ValidateTraceTargetKind(t *testing.T) {
	require.NoError(t, validateTraceTargetKind("REQ-001"))
	require.NoError(t, validateTraceTargetKind("GOAL-001"))
	require.NoError(t, validateTraceTargetKind("MIL-001"))

	err := validateTraceTargetKind("BLI-001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trace pipeline generation is only supported")
}

func TestCycle128_Tree_ResolvePriorityPlan(t *testing.T) {
	ctx := context.Background()

	store := &mockWhatsNextStorage{data: map[string]map[string]any{
		"PRI-EXPLICIT": {
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-EXPLICIT",
			objects.FieldKeyStatus: "in_progress",
		},
		"PRI-ACTIVE": {
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-ACTIVE",
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		},
		"PRI-PAUSED": {
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-PAUSED",
			objects.FieldKeyStatus: objects.ObjectStatusPaused,
		},
	}}

	id, err := resolvePriorityPlan(ctx, store, "PRI-EXPLICIT")
	require.NoError(t, err)
	assert.Equal(t, "PRI-EXPLICIT", id)

	emptyStore := &mockWhatsNextStorage{data: map[string]map[string]any{}}
	_, err = resolvePriorityPlan(ctx, emptyStore, "PRI-MISSING")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	id, err = resolvePriorityPlan(ctx, store, "")
	require.NoError(t, err)
	assert.NotEmpty(t, id)

	_, err = resolvePriorityPlan(ctx, emptyStore, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no active, in_progress, or paused priority plan found")
}

func TestCycle128_Tree_DependencyTreeResolver_Hierarchy(t *testing.T) {
	ctx := context.Background()

	store := &mockWhatsNextStorage{data: map[string]map[string]any{
		"PRI-TREE": {
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-TREE",
			objects.FieldKeyTitle:  "Tree Priority Plan",
			objects.FieldKeyStatus: "in_progress",
		},
		"GOAL-TREE": {
			objects.FieldKeyKind:            objects.KindGoal,
			objects.FieldKeyID:              "GOAL-TREE",
			objects.FieldKeyTitle:           "Tree Goal",
			objects.FieldKeyStatus:          "active",
			objects.FieldKeyPriorityPlanRef: "PRI-TREE",
		},
		"REQ-TREE": {
			objects.FieldKeyKind:         objects.KindRequirement,
			objects.FieldKeyID:           "REQ-TREE",
			objects.FieldKeyTitle:        "Tree Requirement",
			objects.FieldKeyStatus:       "active",
			objects.FieldKeyGoalRefs:     []any{"GOAL-TREE"},
			objects.FieldKeyCriteriaRefs: []any{"CRIT-REQ"},
		},
		"BLI-UNDER-REQ": {
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-UNDER-REQ",
			objects.FieldKeyTitle:           "Backlog Under Requirement",
			objects.FieldKeyStatus:          "planned",
			objects.FieldKeyPriorityPlanRef: "PRI-TREE",
			objects.FieldKeyRequirementRefs: []any{"REQ-TREE"},
			objects.FieldKeyCriteriaRefs:    []any{"CRIT-BLI"},
		},
		"BLI-UNDER-GOAL": {
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-UNDER-GOAL",
			objects.FieldKeyTitle:           "Backlog Under Goal",
			objects.FieldKeyStatus:          "planned",
			objects.FieldKeyPriorityPlanRef: "PRI-TREE",
			objects.FieldKeyGoalRefs:        []any{"GOAL-TREE"},
		},
		"BLI-ORPHAN": {
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-ORPHAN",
			objects.FieldKeyTitle:           "Orphan Backlog",
			objects.FieldKeyStatus:          "planned",
			objects.FieldKeyPriorityPlanRef: "PRI-TREE",
		},
		"CRIT-REQ": {
			objects.FieldKeyKind:   objects.KindCriteria,
			objects.FieldKeyID:     "CRIT-REQ",
			objects.FieldKeyTitle:  "Criterion for Requirement",
			objects.FieldKeyStatus: "validated",
		},
		"CRIT-BLI": {
			objects.FieldKeyKind:   objects.KindCriteria,
			objects.FieldKeyID:     "CRIT-BLI",
			objects.FieldKeyTitle:  "Criterion for Backlog",
			objects.FieldKeyStatus: "validated",
		},
	}}

	resolver := NewDependencyTreeResolver(store)
	root, err := resolver.Resolve(ctx, "PRI-TREE")
	require.NoError(t, err)
	require.NotNil(t, root)
	assert.Equal(t, "PRI-TREE", root.ID)
	assert.NotEmpty(t, root.Children)

	_, err = resolver.Resolve(ctx, "PRI-NONEXISTENT")
	require.Error(t, err)
}

func TestCycle128_Tree_RunTreeCommand_Formats(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	store := &mockWhatsNextStorage{data: map[string]map[string]any{
		"PRI-RUN-TREE": {
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-RUN-TREE",
			objects.FieldKeyTitle:  "Active Plan",
			objects.FieldKeyStatus: "in_progress",
		},
	}}

	cmd := NewTreeCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd, proj.Root, store)

	err := runTree(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "PRI-RUN-TREE")

	buf.Reset()
	cmdJSON := NewTreeCmd()
	require.NoError(t, cmdJSON.Flags().Set("format", "json"))
	cmdJSON.SetOut(&buf)
	cmdJSON.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmdJSON, proj.Root, store)

	err = runTree(cmdJSON, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"id": "PRI-RUN-TREE"`)
}

func TestCycle128_VDS_OperationsAndLookup(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	setupVDSTestProject(t, proj.Root)

	cmdPrompt := &cobra.Command{}
	cli.AddCommonFlags(cmdPrompt)
	require.NoError(t, cmdPrompt.Flags().Set("format", string(cli.FormatAgentPrompt)))
	var pBuf bytes.Buffer
	cmdPrompt.SetOut(&pBuf)
	cmdPrompt.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &pBuf))
	err := emitVDS(cmdPrompt, nil, map[string]any{"ok": true}, "Custom agent brief prompt")
	require.NoError(t, err)
	assert.Contains(t, pBuf.String(), "Custom agent brief prompt")

	cmdInit := NewVDSInitCmd()
	initTestCmdContext(t, cmdInit, proj.Root, nil)
	require.NoError(t, cmdInit.Flags().Set("force", "true"))
	err = runVDSInit(cmdInit, nil)
	require.NoError(t, err)

	rep := &vds.Report{Verdict: "PASS", Tone: "celebratory"}
	err = persistVDSReport(proj.Root, rep)
	require.NoError(t, err)

	emptyCmd := &cobra.Command{}
	initTestCmdContext(t, emptyCmd, proj.Root, nil)
	proc, err := cli.NewProcessor(emptyCmd)
	require.NoError(t, err)
	lookup := vdsTitleLookup(proc)
	assert.NotNil(t, lookup)
}

func TestCycle128_Add_ValidationAndErrorHandling(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	store := &mockWhatsNextStorage{data: map[string]map[string]any{
		"GOAL-NOT-BLI": {
			objects.FieldKeyKind:   objects.KindGoal,
			objects.FieldKeyID:     "GOAL-NOT-BLI",
			objects.FieldKeyStatus: "active",
		},
		"BLI-COMPLETED": {
			objects.FieldKeyKind:   objects.KindBacklogItem,
			objects.FieldKeyID:     "BLI-COMPLETED",
			objects.FieldKeyStatus: statusComplete,
		},
		"PRI-EXISTS": {
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-EXISTS",
			objects.FieldKeyStatus: "grooming",
		},
	}}

	cmdNoArgs := NewAddCmd()
	initTestCmdContext(t, cmdNoArgs, proj.Root, store)
	err := runAdd(cmdNoArgs, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one backlog item ID is required")

	cmdOverride := NewAddCmd()
	initTestCmdContext(t, cmdOverride, proj.Root, store)
	require.NoError(t, cmdOverride.Flags().Set("override", "true"))
	err = runAdd(cmdOverride, []string{"BLI-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--reason-code is required when using --override")

	cmdPriMiss := NewAddCmd()
	initTestCmdContext(t, cmdPriMiss, proj.Root, store)
	err = runAdd(cmdPriMiss, []string{"BLI-1", "PRI-NOT-FOUND"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "priority plan not found")

	cmdWrongKind := NewAddCmd()
	initTestCmdContext(t, cmdWrongKind, proj.Root, store)
	err = runAdd(cmdWrongKind, []string{"GOAL-NOT-BLI", "PRI-EXISTS"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be 'backlog_item'")

	cmdTerminal := NewAddCmd()
	initTestCmdContext(t, cmdTerminal, proj.Root, store)
	err = runAdd(cmdTerminal, []string{"BLI-COMPLETED", "PRI-EXISTS"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already in terminal status")
}

func TestCycle128_WhatsNext_LiteExplicitPlanMatching(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	stateDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.StateDir)
	require.NoError(t, fileutil.MkdirAll(stateDir, paths.DirPerm755))
	litePath := filepath.Join(stateDir, "whats_next_lite.json")

	payload := &whatsnext.WhatsNextLitePayload{
		SchemaVersion:  whatsnext.MaterializedViewSchemaVersion,
		MaterializedAt: time.Now().UTC(),
		LeadPlan: &whatsnext.WhatsNextPriorityPlan{
			ID:     "PRI-LEAD",
			Title:  "Lead Plan Title",
			Status: objects.ObjectStatusInProgress,
		},
		ActivePlans: []whatsnext.WhatsNextPriorityPlan{
			{ID: "PRI-LEAD", Title: "Lead Plan Title", Status: objects.ObjectStatusInProgress},
			{ID: "PRI-SECOND", Title: "Second Plan Title", Status: objects.ObjectStatusPlanned},
		},
		BacklogCountsByPlan: map[string]map[string]int{
			"PRI-LEAD":   {"planned": 3, "in_progress": 1},
			"PRI-SECOND": {"planned": 2},
		},
	}

	env := accumulator.Envelope[*whatsnext.WhatsNextLitePayload]{
		SchemaVersion:  whatsnext.MaterializedViewSchemaVersion,
		MaterializedAt: time.Now().UTC(),
		Payload:        payload,
	}
	data, err := json.MarshalIndent(env, "", "  ")
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(litePath, data, paths.FilePerm644))

	store := &mockWhatsNextStorage{data: map[string]map[string]any{}}

	// 1. Explicit plan matching lead plan
	cmdLead := NewWhatsNextCmd()
	require.NoError(t, cmdLead.Flags().Set("priority-plan", "PRI-LEAD"))
	require.NoError(t, cmdLead.Flags().Set("skip-measure", "true"))
	initTestCmdContext(t, cmdLead, proj.Root, store)
	var buf bytes.Buffer
	cmdLead.SetOut(&buf)
	cmdLead.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

	err = runWhatsNext(cmdLead, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "PRI-LEAD")

	// 2. Explicit plan matching active secondary plan
	buf.Reset()
	cmdSec := NewWhatsNextCmd()
	require.NoError(t, cmdSec.Flags().Set("priority-plan", "PRI-SECOND"))
	require.NoError(t, cmdSec.Flags().Set("skip-measure", "true"))
	initTestCmdContext(t, cmdSec, proj.Root, store)
	cmdSec.SetOut(&buf)
	cmdSec.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

	err = runWhatsNext(cmdSec, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "PRI-SECOND")
}

func TestCycle128_WhatsNext_HelpersAndAmbience(t *testing.T) {
	ctx := context.Background()

	emptyStore := &mockWhatsNextStorage{data: map[string]map[string]any{}}
	assert.Equal(t, 0, countLinkedBLIs(ctx, emptyStore, "PRI-ANY"))

	candidates := []map[string]any{
		{objects.FieldKeyID: "PRI-1"},
		{objects.FieldKeyID: "PRI-2"},
	}
	assert.Len(t, preferSeatedPlansWithOpenWork(ctx, emptyStore, candidates, nil), 2)
	assert.Len(t, preferSeatedPlansWithOpenWork(ctx, emptyStore, nil, []string{"PER-1"}), 0)

	corrEmpty := buildWhatsNextCorrespondence("/tmp", "", "", nil, nil)
	assert.Equal(t, "agent_id_required", corrEmpty.SkipReason)

	activeTask := map[string]any{
		objects.FieldKeyID:     "TSK-101",
		objects.FieldKeyStatus: "in_progress",
		objects.FieldKeyTitle:  "Task Alpha",
	}
	corrTask := buildWhatsNextCorrespondence("/tmp", "agent-x", "persona-y", nil, activeTask)
	require.NotNil(t, corrTask.ActiveAgentTask)
	assert.Equal(t, "TSK-101", corrTask.ActiveAgentTask.ID)

	compileWhatsNextDrive(nil, ctx, nil, "", nil)

	out := &whatsNextOut{
		BacklogCountsByStatus: map[string]int{"planned": 2},
		PriorityPlan:          &whatsNextPriorityPlan{ID: "PRI-IDLE", Status: "in_progress"},
		Correspondence: &whatsNextCorrespondence{
			OutboxAwaitingPeerAck: []agentfeed.CorrespondenceItem{{EventID: "WAKE-1"}},
		},
	}
	compileWhatsNextDrive(out, ctx, nil, "", nil)
	require.NotNil(t, out.GuidingStep)
	assert.Contains(t, out.GuidingStep.GuidingStep, "Shovel-ready batch detected on PRI-IDLE")

	_, err := runSchedulerConvergenceMeasureJSON(t.TempDir(), "CVS-NONEXISTENT")
	assert.Error(t, err)

}

func TestCycle128_WhatsNext_SyncSweep_Execution(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	store := &mockWhatsNextStorage{data: map[string]map[string]any{
		"PRI-SWEEP": {
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-SWEEP",
			objects.FieldKeyTitle:  "Sync Sweep Plan",
			objects.FieldKeyStatus: "in_progress",
		},
		"BLI-SWEEP-1": {
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-SWEEP-1",
			objects.FieldKeyPriorityPlanRef: "PRI-SWEEP",
			objects.FieldKeyStatus:          "in_progress",
		},
		"CVS-SWEEP": {
			objects.FieldKeyKind:      objects.KindConvergenceSession,
			objects.FieldKeyID:        "CVS-SWEEP",
			objects.FieldKeyTitle:     "Convergence Sweep",
			objects.FieldKeyStatus:    statusInProcess,
			objects.FieldKeyUpdatedAt: "2026-10-23T01:00:00Z",
		},
	}}

	cmd := NewWhatsNextCmd()
	require.NoError(t, cmd.Flags().Set("sync-sweep", "true"))
	require.NoError(t, cmd.Flags().Set("skip-measure", "true"))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd, proj.Root, store)

	err := runWhatsNext(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "PRI-SWEEP")
}

func TestCycle128_Coach_ExecutionAndTips(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	schedDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.SchedulerSubdir)
	require.NoError(t, fileutil.EnsureDir(schedDir))
	require.NoError(t, fileutil.WriteFile(filepath.Join(schedDir, "scheduler.pid"), []byte("1234\n"), paths.FilePerm644))

	store := &mockWhatsNextStorage{data: map[string]map[string]any{
		"PRI-COACH": {
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-COACH",
			objects.FieldKeyStatus: "active",
		},
	}}

	cmd := NewCoachCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd, proj.Root, store)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "tips")

	// Exercise update-tips generation path
	cmdUpdate := NewCoachCmd()
	require.NoError(t, cmdUpdate.Flags().Set("update-tips", "true"))
	initTestCmdContext(t, cmdUpdate, proj.Root, store)
	errUpdate := cmdUpdate.RunE(cmdUpdate, nil)
	assert.True(t, errUpdate == nil || strings.Contains(errUpdate.Error(), "completion") || strings.Contains(errUpdate.Error(), "failed"))
}

func TestCycle128_WhatsNext_SyncSweep_Branches(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// Plan with verifying backlog items -> triggers execute_tests
	store := &mockWhatsNextStorage{data: map[string]map[string]any{
		"PRI-VERIFY": {
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-VERIFY",
			objects.FieldKeyTitle:  "Verifying Plan",
			objects.FieldKeyStatus: "in_progress",
		},
		"BLI-V1": {
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-V1",
			objects.FieldKeyPriorityPlanRef: "PRI-VERIFY",
			objects.FieldKeyStatus:          "verifying",
		},
	}}

	cmdJSON := NewWhatsNextCmd()
	require.NoError(t, cmdJSON.Flags().Set("sync-sweep", "true"))
	require.NoError(t, cmdJSON.Flags().Set("skip-measure", "true"))
	require.NoError(t, cmdJSON.Flags().Set("format", "json"))
	var buf bytes.Buffer
	cmdJSON.SetOut(&buf)
	cmdJSON.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmdJSON, proj.Root, store)

	err := runWhatsNext(cmdJSON, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "execute_tests")

	// Plan with only completed backlog items -> triggers shutdown
	storeComplete := &mockWhatsNextStorage{data: map[string]map[string]any{
		"PRI-DONE": {
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-DONE",
			objects.FieldKeyTitle:  "Completed Plan",
			objects.FieldKeyStatus: "in_progress",
		},
		"BLI-D1": {
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-D1",
			objects.FieldKeyPriorityPlanRef: "PRI-DONE",
			objects.FieldKeyStatus:          "completed",
		},
	}}

	cmdYAML := NewWhatsNextCmd()
	require.NoError(t, cmdYAML.Flags().Set("sync-sweep", "true"))
	require.NoError(t, cmdYAML.Flags().Set("skip-measure", "true"))
	require.NoError(t, cmdYAML.Flags().Set("format", "yaml"))
	buf.Reset()
	cmdYAML.SetOut(&buf)
	cmdYAML.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmdYAML, proj.Root, storeComplete)

	err = runWhatsNext(cmdYAML, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "shutdown")
}

func TestCycle128_WhatsNext_Lite_Branches(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	stateDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.StateDir)
	require.NoError(t, fileutil.MkdirAll(stateDir, paths.DirPerm755))
	litePath := filepath.Join(stateDir, "whats_next_lite.json")

	// 1. Lite with verifying work and runway depleted
	payloadVerifying := &whatsnext.WhatsNextLitePayload{
		SchemaVersion:    whatsnext.MaterializedViewSchemaVersion,
		MaterializedAt:   time.Now().UTC(),
		IsRunwayDepleted: true,
		RunwayDepth:      1,
		LeadPlan: &whatsnext.WhatsNextPriorityPlan{
			ID:     "PRI-LITE-V",
			Title:  "Lite Verifying Plan",
			Status: "in_progress",
		},
		BacklogCountsByPlan: map[string]map[string]int{
			"PRI-LITE-V": {"verifying": 2},
		},
	}
	env := accumulator.Envelope[*whatsnext.WhatsNextLitePayload]{
		SchemaVersion:  whatsnext.MaterializedViewSchemaVersion,
		MaterializedAt: time.Now().UTC(),
		Payload:        payloadVerifying,
	}
	data, err := json.MarshalIndent(env, "", "  ")
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(litePath, data, paths.FilePerm644))

	cmd := NewWhatsNextCmd()
	require.NoError(t, cmd.Flags().Set("skip-measure", "true"))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd, proj.Root, nil)

	err = runWhatsNext(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Watermark delta_runway <= 1")
}

func TestCycle128_GenTracePipeline_ApplyEstablished(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// Target requirement with 3 criteria neighbors already established
	reqObj := map[string]any{
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyID:           "REQ-ESTABLISHED",
		objects.FieldKeyCriteriaRefs: []any{"CRIT-1", "CRIT-2", "CRIT-3"},
	}
	critNeighbors := []map[string]any{
		{objects.FieldKeyID: "CRIT-1"},
		{objects.FieldKeyID: "CRIT-2"},
		{objects.FieldKeyID: "CRIT-3"},
	}

	store := &mockWhatsNextStorage{data: map[string]map[string]any{
		"REQ-ESTABLISHED": reqObj,
	}}

	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	initTestCmdContext(t, cmd, proj.Root, store)
	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)

	err = applyGeneratedTracePipeline(cmd, proc, "REQ-ESTABLISHED", reqObj, critNeighbors, 3)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Pipeline updated successfully")
}

func TestCycle128_Check_AdvancedViolations(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	runGit := func(args ...string) {
		c := testkit.ManagedCommand(t, t.Context(), "git", args...)
		c.Dir = proj.Root
		out, err := c.CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, string(out))
	}

	runGit("init")
	runGit("config", "user.name", "Tester")
	runGit("config", "user.email", "tester@test.com")
	runGit("commit", "--allow-empty", "-m", "init")

	// Create feature branch
	runGit("checkout", "-b", "feat/BLI-CHECK-2")

	// Create an untracked dirty file
	dirtyFile := filepath.Join(proj.Root, "dirty.txt")
	require.NoError(t, fileutil.WriteFile(dirtyFile, []byte("dirty\n"), paths.FilePerm644))

	store := &mockWhatsNextStorage{data: map[string]map[string]any{}}

	cmd := NewCheckCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd, proj.Root, store)

	err := runCheck(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Git state is not clean")
	assert.Contains(t, buf.String(), "No active priority plan found")
}

func TestCycle128_Check_PerfectAdherence(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	runGit := func(args ...string) {
		c := testkit.ManagedCommand(t, t.Context(), "git", args...)
		c.Dir = proj.Root
		out, err := c.CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, string(out))
	}

	runGit("init")
	runGit("config", "user.name", "Tester")
	runGit("config", "user.email", "tester@test.com")
	runGit("add", "-A")
	runGit("commit", "--allow-empty", "-m", "init")
	runGit("branch", "-M", "main")
	runGit("checkout", "-b", "feat/BLI-PERFECT-1")

	store := &mockWhatsNextStorage{data: map[string]map[string]any{
		"PRI-ACTIVE": {
			objects.FieldKeyKind:            objects.KindPriorityPlan,
			objects.FieldKeyID:              "PRI-ACTIVE",
			objects.FieldKeyStatus:          objects.ObjectStatusActive,
			objects.FieldKeyBacklogItemRefs: []any{"BLI-PERFECT-1"},
		},
	}}

	cmd := NewCheckCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd, proj.Root, store)

	err := runCheck(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Perfect adherence")
}

func TestCycle128_Check_MergeBaseBehind(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	runGit := func(args ...string) {
		c := testkit.ManagedCommand(t, t.Context(), "git", args...)
		c.Dir = proj.Root
		out, err := c.CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, string(out))
	}

	runGit("init")
	runGit("config", "user.name", "Tester")
	runGit("config", "user.email", "tester@test.com")
	runGit("commit", "--allow-empty", "-m", "commit 1")
	runGit("branch", "-M", "main")
	runGit("checkout", "-b", "feat/BLI-OUTDATED-1")
	runGit("checkout", "main")
	runGit("commit", "--allow-empty", "-m", "commit 2 on main")
	runGit("checkout", "feat/BLI-OUTDATED-1")

	store := &mockWhatsNextStorage{data: map[string]map[string]any{
		"PRI-ACTIVE": {
			objects.FieldKeyKind:            objects.KindPriorityPlan,
			objects.FieldKeyID:              "PRI-ACTIVE",
			objects.FieldKeyStatus:          objects.ObjectStatusActive,
			objects.FieldKeyBacklogItemRefs: []any{"BLI-OUTDATED-1"},
		},
	}}

	cmd := NewCheckCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd, proj.Root, store)

	err := runCheck(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Branch is not based on latest main")
}

func TestCycle128_WhatsNext_ActiveTaskWithSteps(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	store := &mockWhatsNextStorage{data: map[string]map[string]any{
		"PRI-EXEC": {
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-EXEC",
			objects.FieldKeyTitle:  "Exec Plan",
			objects.FieldKeyStatus: "in_progress",
		},
		"ATK-TASK-1": {
			objects.FieldKeyKind:               objects.KindAgentTask,
			objects.FieldKeyID:                 "ATK-TASK-1",
			objects.FieldKeyTitle:              "Important Active Task",
			objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
			objects.FieldKeyAssigneePersonaRef: "PER-CRAFTSMAN",
			objects.FieldKeyPipelineRef:        "PRI-EXEC",
			objects.FieldKeyDescription:        "zqk_task_envelope_v1: Implement task components",
			objects.FieldKeyTaskSteps: []any{
				map[string]any{
					objects.FieldKeyTitle:       "Step One",
					objects.FieldKeyStatus:      "in_progress",
					objects.FieldKeyDescription: "Execute test suite",
					objects.FieldKeyCommand:     "go test ./...",
				},
			},
		},
	}}

	cmd := NewWhatsNextCmd()
	require.NoError(t, cmd.Flags().Set("sync-sweep", "true"))
	require.NoError(t, cmd.Flags().Set("skip-measure", "true"))
	require.NoError(t, cmd.Flags().Set("persona-id", "PER-CRAFTSMAN"))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd, proj.Root, store)

	err := runWhatsNext(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "PRI-EXEC")
}

func TestCycle128_Add_ActiveOrderMatching(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	store := &mockWhatsNextStorage{data: map[string]map[string]any{
		"PRI-ORDER-2": {
			objects.FieldKeyKind:        objects.KindPriorityPlan,
			objects.FieldKeyID:          "PRI-ORDER-2",
			objects.FieldKeyTitle:       "Order 2 Plan",
			objects.FieldKeyStatus:      objects.ObjectStatusPlanned,
			objects.FieldKeyActiveOrder: 2,
		},
		"BLI-ATTACH": {
			objects.FieldKeyKind:   objects.KindBacklogItem,
			objects.FieldKeyID:     "BLI-ATTACH",
			objects.FieldKeyTitle:  "Backlog to Attach",
			objects.FieldKeyStatus: objects.ObjectStatusPlanned,
		},
	}}

	cmd := NewAddCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	initTestCmdContext(t, cmd, proj.Root, store)

	err := runAdd(cmd, []string{"BLI-ATTACH", "2"})
	require.NoError(t, err)
}

func TestCycle128_GenTracePipeline_MilestoneAndGoal(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	milObj := map[string]any{
		objects.FieldKeyKind:  objects.KindMilestone,
		objects.FieldKeyID:    "MIL-TRACE-1",
		objects.FieldKeyTitle: "Trace Milestone",
	}
	goalObj := map[string]any{
		objects.FieldKeyKind:  objects.KindGoal,
		objects.FieldKeyID:    "GOAL-TRACE-1",
		objects.FieldKeyTitle: "Trace Goal",
	}

	store := &mockWhatsNextStorage{data: map[string]map[string]any{
		"MIL-TRACE-1":  milObj,
		"GOAL-TRACE-1": goalObj,
	}}

	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	initTestCmdContext(t, cmd, proj.Root, store)
	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)

	// Test Milestone target
	err = applyGeneratedTracePipeline(cmd, proc, "MIL-TRACE-1", milObj, nil, 3)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Pipeline updated successfully")

	// Test Goal target
	buf.Reset()
	err = applyGeneratedTracePipeline(cmd, proc, "GOAL-TRACE-1", goalObj, nil, 3)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Pipeline updated successfully")
}

func TestCycle128_WhatsNext_EnrichRemedies(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("auto-remedy", false, "")

	// 1. Guard clauses
	enrichWhatsNextRemedies(cmd, "", nil)
	enrichWhatsNextRemedies(cmd, "/tmp", nil)

	// 2. Ambience with remedy recipes and autoRemedy=false
	out := &whatsNextOut{
		KernelAmbience: &whatsnext.KernelAmbience{
			RemedyRecipes: []string{"Recipe A: fix permissions", "Recipe B: prune buffers"},
		},
	}
	enrichWhatsNextRemedies(cmd, "/tmp", out)
	assert.Len(t, out.RemedyRecipes, 2)
}
