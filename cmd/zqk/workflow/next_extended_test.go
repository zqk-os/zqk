package workflow

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestNewNextCmd(t *testing.T) {
	cmd := NewNextCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "next", cmd.Use)
}

func TestWriteWorkflowNextOutput_Formats(t *testing.T) {
	fullResult := workflowNextResult{
		SessionID:                 "SES-123",
		SessionSource:             "session.json",
		Precedence:                []string{"crit", "conv", "backlog"},
		Decision:                  decisionPriorityPlanBacklog,
		Rationale:                 "Top priority backlog item selected",
		RecommendedCommand:        "zqk agent claim BLI-1",
		OpenQuestionsCount:        3,
		BacklogPriorityTier:       "P0",
		PriorityPlanTitle:         "Active Plan",
		PriorityPlanSummary:       "Summary of plan",
		BacklogItemSummary:        "Summary of item",
		GoalRefs:                  []string{"GOAL-A", "GOAL-B"},
		ConvergenceSessionRef:     "CVS-001",
		ConvergenceSessionProfile: "tight",
		Sources: []sourceRef{
			{Kind: "backlog_item", ID: "BLI-1", Note: "first item"},
		},
	}

	// 1. JSON Format
	cmdJSON := NewNextCmd()
	cmdJSON.Flags().String("format", "table", "")
	require.NoError(t, cmdJSON.Flags().Set("format", "json"))
	var jsonBuf bytes.Buffer
	cmdJSON.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &jsonBuf))
	err := writeWorkflowNextOutput(cmdJSON, fullResult)
	require.NoError(t, err)
	assert.Contains(t, jsonBuf.String(), `"decision": "priority_plan_backlog"`)

	// 2. YAML Format
	cmdYAML := NewNextCmd()
	cmdYAML.Flags().String("format", "table", "")
	require.NoError(t, cmdYAML.Flags().Set("format", "yaml"))
	var yamlBuf bytes.Buffer
	cmdYAML.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &yamlBuf))
	err = writeWorkflowNextOutput(cmdYAML, fullResult)
	require.NoError(t, err)
	assert.Contains(t, yamlBuf.String(), "decision: priority_plan_backlog")

	// 3. Table Format (all branches)
	cmdTable := NewNextCmd()
	cmdTable.Flags().String("format", "table", "")
	var tableBuf bytes.Buffer
	cmdTable.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &tableBuf))
	err = writeWorkflowNextOutput(cmdTable, fullResult)
	require.NoError(t, err)
	out := tableBuf.String()
	assert.Contains(t, out, "Decision: priority_plan_backlog")
	assert.Contains(t, out, "Recommended command: zqk agent claim BLI-1")
	assert.Contains(t, out, "Session: SES-123 (session.json)")
	assert.Contains(t, out, "Open questions (status=open): 3")
	assert.Contains(t, out, "Tier: P0")
	assert.Contains(t, out, "Plan: Active Plan")
	assert.Contains(t, out, "Plan summary: Summary of plan")
	assert.Contains(t, out, "Item summary: Summary of item")
	assert.Contains(t, out, "Goal refs: GOAL-A, GOAL-B")
	assert.Contains(t, out, "Convergence session: CVS-001 (tight)")
	assert.Contains(t, out, "Sources:")
	assert.Contains(t, out, "- backlog_item BLI-1 :: first item")
	assert.Contains(t, out, "Tip: To focus on a theme")

	// 4. Table Format with none decision
	cmdNone := NewNextCmd()
	cmdNone.Flags().String("format", "table", "")
	var noneBuf bytes.Buffer
	cmdNone.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &noneBuf))
	noneResult := workflowNextResult{
		Decision:           decisionNone,
		RecommendedCommand: "zqk workflow whats-next",
		Rationale:          "No work found",
		Precedence:         []string{"crit", "conv"},
	}
	err = writeWorkflowNextOutput(cmdNone, noneResult)
	require.NoError(t, err)
	assert.Contains(t, noneBuf.String(), "Decision: none")
}

func TestApplyDecision_Branches(t *testing.T) {
	// Nil result is safe
	applyDecision(nil, nil, nil, "", nil)

	// Critical policy interrupt
	res := &workflowNextResult{Precedence: []string{"a"}}
	pol := &policyDecision{DedupeKey: "POL-001", Message: "Security halt"}
	applyDecision(res, pol, nil, "", nil)
	assert.Equal(t, decisionCriticalPolicyInterrupt, res.Decision)
	assert.Len(t, res.Sources, 1)

	// Active convergence
	res2 := &workflowNextResult{Precedence: []string{"a"}}
	cv := &convergenceInfo{ID: "CVS-1", Title: "Convergence active"}
	applyDecision(res2, nil, cv, "", nil)
	assert.Equal(t, decisionActiveConvergence, res2.Decision)
	assert.Len(t, res2.Sources, 1)

	// Priority plan backlog
	res3 := &workflowNextResult{Precedence: []string{"a"}}
	item := &backlogInfo{ID: "BLI-1", Title: "First BLI"}
	applyDecision(res3, nil, nil, "PRI-1", item)
	assert.Equal(t, decisionPriorityPlanBacklog, res3.Decision)
	assert.Len(t, res3.Sources, 2)
}

func TestSelectPolicyInterrupt_EmptyRoot(t *testing.T) {
	dec, ok := selectPolicyInterrupt("")
	assert.Nil(t, dec)
	assert.False(t, ok)

	tmpDir := t.TempDir()
	dec, ok = selectPolicyInterrupt(tmpDir)
	assert.Nil(t, dec)
	assert.False(t, ok)
}

func TestRunNext(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	store := &mockTreeStorage{
		data: map[string]map[string]any{
			"PRI-NEXT-1": {
				objects.FieldKeyKind:   objects.KindPriorityPlan,
				objects.FieldKeyID:     "PRI-NEXT-1",
				objects.FieldKeyTitle:  "Next Plan",
				objects.FieldKeyStatus: objects.ObjectStatusInProgress,
			},
			"BLI-NEXT-1": {
				objects.FieldKeyKind:            objects.KindBacklogItem,
				objects.FieldKeyID:              "BLI-NEXT-1",
				objects.FieldKeyTitle:           "Next BLI",
				objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
				objects.FieldKeyPriorityPlanRef: "PRI-NEXT-1",
				objects.FieldKeyPriorityTier:    "P0",
			},
		},
	}

	cmd := NewNextCmd()
	cmd.Flags().String("format", "table", "")
	require.NoError(t, cmd.Flags().Set("format", "json"))
	initTestCmdContext(t, cmd, proj.Root, store)
	var outBuf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(cmd.Context(), &outBuf))

	err := runNext(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, outBuf.String(), `"decision":`)
}
