package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestOrchestrateCmd_UsageAndNoExecuteFlag(t *testing.T) {
	cmd := NewOrchestrateCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "orchestrate [plan-id]", cmd.Use)

	flag := cmd.Flags().Lookup("no-execute")
	require.NotNil(t, flag, "expected --no-execute flag to be registered")
	assert.Equal(t, "bool", flag.Value.Type())

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	require.NoError(t, err)

	helpOutput := buf.String()
	assert.Contains(t, helpOutput, "orchestrate [plan-id]")
	assert.Contains(t, helpOutput, "--no-execute")
}

func TestOrchestrateCmd_NoExecute_MintsTaskEnvelopesWithoutExecution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	req := map[string]any{
		objects.FieldKeyID:            "REQ-NOEXEC-001",
		objects.FieldKeyKind:          objects.KindRequirement,
		objects.FieldKeyTitle:         "Requirement for NoExecute",
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemaVersion: "2.0.0",
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, req, "active")

	cri := map[string]any{
		objects.FieldKeyID:            "CRI-NOEXEC-001",
		objects.FieldKeyKind:          objects.KindCriteria,
		objects.FieldKeyTitle:         "Criterion for NoExecute",
		objects.FieldKeyCategory:      "functional",
		objects.FieldKeyStatus:        "awaiting_verification",
		objects.FieldKeySchemaVersion: "2.0.0",
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, cri, "awaiting_verification")

	plan := map[string]any{
		objects.FieldKeyID:             "PRI-NOEXEC-001",
		objects.FieldKeyKind:           objects.KindPriorityPlan,
		objects.FieldKeyTitle:          "Plan for NoExecute Test",
		objects.FieldKeyStatus:         objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion:  "2.0.0",
		objects.FieldKeyPersonaRefs:    []any{objects.ConstPersonaDefaultOperator},
		objects.FieldKeyWorkstreamRefs: []any{"WS-001"},
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, plan, objects.ObjectStatusInProgress)

	bli := map[string]any{
		objects.FieldKeyID:              "BLI-NOEXEC-001",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyTitle:           "Backlog Item for NoExecute",
		objects.FieldKeyDescription:     "Description with concrete semantic context for verification",
		objects.FieldKeyRequirementRefs: []any{"REQ-NOEXEC-001"},
		objects.FieldKeyCriteriaRefs:    []any{"CRI-NOEXEC-001"},
		objects.FieldKeyEstimatedEffort: "1d",
		objects.FieldKeyPersonaRef:      objects.ConstPersonaDefaultOperator,
		objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
		objects.FieldKeyPriorityPlanRef: "PRI-NOEXEC-001",
		objects.FieldKeyModelTier:       "tier_2_simple",
		objects.FieldKeySchemaVersion:   "2.0.0",
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, bli, objects.ObjectStatusPlanned)

	out, err := executeAgentCommand(t, tempDir, provider, "orchestrate", "PRI-NOEXEC-001", "--no-execute", "--format", "json")
	require.NoError(t, err)

	var routingResult map[string]any
	err = json.Unmarshal([]byte(out), &routingResult)
	require.NoError(t, err, "output should be valid JSON: %s", out)

	assert.Equal(t, "envelopes_minted", routingResult["delivery_result"])
	assert.Equal(t, "routed", routingResult["status"])
	assert.Equal(t, "PRI-NOEXEC-001", routingResult["priority_plan"])

	rawItems, ok := routingResult["items"].([]any)
	require.True(t, ok, "items should be an array")
	require.Len(t, rawItems, 1, "expected 1 minted task ID")

	taskID, ok := rawItems[0].(string)
	require.True(t, ok)
	assert.NotEmpty(t, taskID)

	// Verify the minted agent_task exists in CAS storage
	taskObj, err := provider.Read(ctx, secCtx, taskID)
	require.NoError(t, err, "agent_task %s must be saved and readable in CAS", taskID)
	require.NotNil(t, taskObj)

	assert.Equal(t, objects.KindAgentTask, taskObj[objects.FieldKeyKind])
	assert.Equal(t, "PRI-NOEXEC-001", taskObj[objects.FieldKeyPriorityPlanRef])
	assert.Contains(t, taskObj[objects.FieldKeyTitle], "Backlog Item for NoExecute")
}
