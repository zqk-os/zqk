package do

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func createDoCycle136Command(
	t *testing.T,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	args ...string,
) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	cmd := NewDoCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	baseCtx := pkgctx.NewSystemContext()
	baseCtx = pkgctx.WithCommandOutputWriter(baseCtx, &buf)
	if secCtx != nil {
		baseCtx = pkgctx.WithSecurityContext(baseCtx, secCtx)
	}
	cmd.SetContext(cli.WithStorageProvider(baseCtx, storageProvider))

	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	ctx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	ctx.ProjectRoot = projectRoot
	cli.SetContext(cmd, ctx)
	cmd.SetArgs(args)

	return cmd, &buf
}

func executeDoCycle136(
	t *testing.T,
	p testkit.IsolatedTempProject,
	secCtx *pkgctx.SecurityContext,
	args ...string,
) (string, error) {
	t.Helper()
	cmd, buf := createDoCycle136Command(t, p.Root, p.FileStorage, secCtx, args...)
	err := cmd.Execute()
	return buf.String(), err
}

func createCycle136BacklogItem(
	t *testing.T,
	p testkit.IsolatedTempProject,
	secCtx *pkgctx.SecurityContext,
	bliID, status, priority string,
	critRefs []string,
) {
	t.Helper()
	bli := map[string]any{
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyID:           bliID,
		objects.FieldKeyTitle:        "Coverage Elevation Test BLI",
		objects.FieldKeyDescription:  "Backlog item for Cycle 136 autonomous execution tests",
		objects.FieldKeyStatus:       status,
		objects.FieldKeyPriority:     priority,
		objects.FieldKeyCriteriaRefs: critRefs,
	}
	err := p.FileStorage.Create(context.Background(), secCtx, bli)
	require.NoError(t, err)
}

func createCycle136Task(
	t *testing.T,
	p testkit.IsolatedTempProject,
	secCtx *pkgctx.SecurityContext,
	taskID string,
	relatedRefs []string,
) {
	t.Helper()
	task := map[string]any{
		objects.FieldKeyKind:              objects.KindAgentTask,
		objects.FieldKeyID:                taskID,
		objects.FieldKeyTitle:             "Coverage Elevation Test Agent Task",
		objects.FieldKeyDescription:       "Agent task referencing BLI for Cycle 136 tests",
		objects.FieldKeyStatus:            objects.ObjectStatusApproved,
		objects.FieldKeyRelatedObjectRefs: relatedRefs,
	}
	err := p.FileStorage.Create(context.Background(), secCtx, task)
	require.NoError(t, err)
}

func createCycle136CASPlanAndCrit(
	t *testing.T,
	p testkit.IsolatedTempProject,
	secCtx *pkgctx.SecurityContext,
	critID, testID, bliID string,
) {
	t.Helper()
	crit := map[string]any{
		objects.FieldKeyKind:     objects.KindCriteria,
		objects.FieldKeyID:       critID,
		objects.FieldKeyTitle:    "Coverage Criteria",
		objects.FieldKeyStatus:   objects.ObjectStatusOriginated,
		objects.FieldKeyCategory: "acceptance",
	}
	storage.CreateCASVisible(t, p.FileStorage, context.Background(), secCtx, crit, "awaiting_verification")

	tc := map[string]any{
		objects.FieldKeyKind:            objects.KindTestCase,
		objects.FieldKeyID:              testID,
		objects.FieldKeyTitle:           "Coverage Test Case",
		objects.FieldKeyStatus:          objects.ObjectStatusActive,
		objects.FieldKeyCriteriaRefs:    []any{critID},
		objects.FieldKeyBacklogItemRefs: []any{bliID},
	}
	storage.CreateCASVisible(t, p.FileStorage, context.Background(), secCtx, tc, objects.ObjectStatusActive)

	bli := map[string]any{
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyID:           bliID,
		objects.FieldKeyTitle:        "Coverage Elevation Live BLI",
		objects.FieldKeyDescription:  "BLI for testing live transition to in_progress",
		objects.FieldKeyStatus:       objects.ObjectStatusPlanned,
		objects.FieldKeyPriority:     "high",
		objects.FieldKeyCriteriaRefs: []any{critID},
	}
	storage.CreateCASVisible(t, p.FileStorage, context.Background(), secCtx, bli, objects.ObjectStatusPlanned)
}

func TestCycle136_CommandMetadataAndArgsValidation(t *testing.T) {
	cmd := NewDoCmd()
	require.NotNil(t, cmd)

	assert.Equal(t, "do [task_or_bli_id]", cmd.Use)
	assert.Contains(t, cmd.Aliases, "auto-exec")
	assert.Contains(t, cmd.Aliases, "auto-do")
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)

	require.NotNil(t, cmd.Flags().Lookup("dry-run"))
	require.NotNil(t, cmd.Flags().Lookup("verify"))
	require.NotNil(t, cmd.Flags().Lookup("by"))
	require.NotNil(t, cmd.Flags().Lookup("persona-ref"))

	p := testkit.PrepareIsolatedTempProject(t, nil)
	secCtx := pkgctx.NewSystemSecurityContext()

	// Exceeding maximum positional arguments should return an error
	out, err := executeDoCycle136(t, p, secCtx, "BLI-1", "BLI-2")
	require.Error(t, err)
	assert.NotEmpty(t, out)
	assert.Contains(t, err.Error(), "accepts at most 1 arg(s)")
}

func TestCycle136_ClaimantResolutionVariations(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)
	secCtx := pkgctx.NewSystemSecurityContext()
	createCycle136BacklogItem(t, p, secCtx, "BLI-CLAIMANT-TEST-001", objects.ObjectStatusPlanned, "high", nil)

	// 1. Explicit --by flag
	out1, err1 := executeDoCycle136(t, p, secCtx, "BLI-CLAIMANT-TEST-001", "--dry-run", "--by", "seat:custom-engineer", "-f", "json")
	require.NoError(t, err1)
	var res1 map[string]any
	err1 = json.Unmarshal([]byte(out1), &res1)
	require.NoError(t, err1)
	assert.Equal(t, "seat:custom-engineer", res1["claimant"])

	// 2. ZQK_AGENT_ID environment variable
	t.Setenv(zqkenv.AgentID().Name(), "seat:env-worker")
	out2, err2 := executeDoCycle136(t, p, secCtx, "BLI-CLAIMANT-TEST-001", "--dry-run", "-f", "json")
	require.NoError(t, err2)
	var res2 map[string]any
	err2 = json.Unmarshal([]byte(out2), &res2)
	require.NoError(t, err2)
	assert.Equal(t, "seat:env-worker", res2["claimant"])

	// 3. Security context account fallback when env is empty
	t.Setenv(zqkenv.AgentID().Name(), "")
	secWithAccount := pkgctx.NewSecurityContext("ACC-ACCOUNT-RESOLVED", []string{"admin"}, []string{"*"})
	out3, err3 := executeDoCycle136(t, p, secWithAccount, "BLI-CLAIMANT-TEST-001", "--dry-run", "-f", "json")
	require.NoError(t, err3)
	var res3 map[string]any
	err3 = json.Unmarshal([]byte(out3), &res3)
	require.NoError(t, err3)
	assert.Equal(t, "ACC-ACCOUNT-RESOLVED", res3["claimant"])

	// 4. Fallback to default when sec has empty AccountID and env is empty
	secEmptyAccount := &pkgctx.SecurityContext{AccountID: "", Roles: []string{"admin"}}
	out4, err4 := executeDoCycle136(t, p, secEmptyAccount, "BLI-CLAIMANT-TEST-001", "--dry-run", "-f", "json")
	require.NoError(t, err4)
	var res4 map[string]any
	err4 = json.Unmarshal([]byte(out4), &res4)
	require.NoError(t, err4)
	assert.Equal(t, "agent:auto-exec", res4["claimant"])
}

func TestCycle136_OutputFormats(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)
	secCtx := pkgctx.NewSystemSecurityContext()
	createCycle136BacklogItem(t, p, secCtx, "BLI-FORMAT-TEST-001", objects.ObjectStatusPlanned, "high", nil)

	// JSON format
	outJSON, err := executeDoCycle136(t, p, secCtx, "BLI-FORMAT-TEST-001", "--dry-run", "-f", "json")
	require.NoError(t, err)
	var jsonMap map[string]any
	err = json.Unmarshal([]byte(outJSON), &jsonMap)
	require.NoError(t, err)
	assert.Equal(t, "BLI-FORMAT-TEST-001", jsonMap["target_bli_id"])

	// YAML format
	outYAML, err := executeDoCycle136(t, p, secCtx, "BLI-FORMAT-TEST-001", "--dry-run", "-f", "yaml")
	require.NoError(t, err)
	var yamlMap map[string]any
	err = yaml.Unmarshal([]byte(outYAML), &yamlMap)
	require.NoError(t, err)
	assert.Contains(t, outYAML, "BLI-FORMAT-TEST-001")
	assert.Equal(t, "BLI-FORMAT-TEST-001", yamlMap["targetbliid"])

	// JSONL format
	outJSONL, err := executeDoCycle136(t, p, secCtx, "BLI-FORMAT-TEST-001", "--dry-run", "-f", "jsonl")
	require.NoError(t, err)
	assert.Contains(t, outJSONL, `"target_bli_id":"BLI-FORMAT-TEST-001"`)

	// Default text summary
	outText, err := executeDoCycle136(t, p, secCtx, "BLI-FORMAT-TEST-001", "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, outText, "🎯 ZQK Autonomous Execution: BLI-FORMAT-TEST-001")
	assert.Contains(t, outText, "Status: dry_run")
	assert.Contains(t, outText, "Execution Steps:")
}

func TestCycle136_RenderDoSummaryEdgeCases(t *testing.T) {
	cmd := NewDoCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	res := &clipkg.AutoExecResult{
		TargetTaskID:    "ATK-EDGE-001",
		Status:          "failed",
		Claimant:        "agent:test-runner",
		VerifiedTestIDs: []string{"TST-001", "TST-002"},
		LatchedCritIDs:  []string{"CRIT-001", "CRIT-002"},
		Steps: []clipkg.AutoExecStepResult{
			{Step: "discovery", StepStatus: "ok", Detail: "Discovered task"},
			{Step: "claim", StepStatus: "dry_run", Detail: "Simulated claim"},
			{Step: "verification", StepStatus: "failed", Detail: "Assertion failed"},
			{Step: "latch", StepStatus: "skipped", Detail: "Skipped due to prior failure"},
		},
	}

	renderDoSummary(cmd, res)
	rendered := buf.String()

	assert.Contains(t, rendered, "ATK-EDGE-001")
	assert.Contains(t, rendered, "Status: failed (Claimant: agent:test-runner)")
	assert.Contains(t, rendered, "✓ [discovery   ] Discovered task")
	assert.Contains(t, rendered, "✓ [claim       ] Simulated claim")
	assert.Contains(t, rendered, "❌ [verification] Assertion failed")
	assert.Contains(t, rendered, "❌ [latch       ] Skipped due to prior failure")
	assert.Contains(t, rendered, "✓ Verified Tests: TST-001, TST-002")
	assert.Contains(t, rendered, "✓ Latched Criteria: CRIT-001, CRIT-002")
	assert.Contains(t, rendered, "Prompt: \"I've claimed ATK-EDGE-001.")
}

func TestCycle136_TaskTargetResolution(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Task pointing to a valid BLI
	createCycle136BacklogItem(t, p, secCtx, "BLI-TASK-LINKED-001", objects.ObjectStatusPlanned, "high", nil)
	createCycle136Task(t, p, secCtx, "ATK-VALID-001", []string{"BLI-TASK-LINKED-001"})

	out1, err1 := executeDoCycle136(t, p, secCtx, "ATK-VALID-001", "--dry-run", "-f", "json")
	require.NoError(t, err1)
	var res1 map[string]any
	err1 = json.Unmarshal([]byte(out1), &res1)
	require.NoError(t, err1)
	assert.Equal(t, "BLI-TASK-LINKED-001", res1["target_bli_id"])
	assert.Equal(t, "ATK-VALID-001", res1["target_task_id"])

	// 2. Task with no associated BLI reference
	createCycle136Task(t, p, secCtx, "ATK-NO-BLI-001", []string{"DOC-SOMETHING-ELSE"})
	out2, err2 := executeDoCycle136(t, p, secCtx, "ATK-NO-BLI-001", "--dry-run")
	require.Error(t, err2)
	assert.NotEmpty(t, out2)
	assert.Contains(t, err2.Error(), "has no associated backlog item")

	// 3. Task does not exist
	out3, err3 := executeDoCycle136(t, p, secCtx, "ATK-NONEXISTENT-001", "--dry-run")
	require.Error(t, err3)
	assert.NotEmpty(t, out3)
	assert.Contains(t, err3.Error(), "not found")
}

func TestCycle136_ExecutionErrorsAndBoundaries(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)
	secCtx := pkgctx.NewSystemSecurityContext()

	// Target BLI not found
	outNotFound, errNotFound := executeDoCycle136(t, p, secCtx, "BLI-MISSING-001", "--dry-run")
	require.Error(t, errNotFound)
	assert.NotEmpty(t, outNotFound)
	assert.Contains(t, errNotFound.Error(), "not found")

	// No planned work available for auto-discovery
	outNoWork, errNoWork := executeDoCycle136(t, p, secCtx, "--dry-run")
	require.Error(t, errNoWork)
	assert.NotEmpty(t, outNoWork)
	assert.Contains(t, errNoWork.Error(), "no eligible backlog items")

	// Target not matching any backlog item when storage is empty falls back to auto-discovery
	outEmptyAuto, errEmptyAuto := executeDoCycle136(t, p, secCtx, "UNKNOWN-001", "--dry-run")
	require.Error(t, errEmptyAuto)
	assert.NotEmpty(t, outEmptyAuto)
	assert.Contains(t, errEmptyAuto.Error(), "no eligible backlog items")
}

func TestCycle136_LiveExecutionTransition(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)
	secCtx := pkgctx.NewSystemSecurityContext()
	createCycle136CASPlanAndCrit(t, p, secCtx, "CRIT-LIVE-001", "TST-LIVE-001", "BLI-LIVE-CLAIM-001")

	// Live execution without --dry-run
	out, err := executeDoCycle136(t, p, secCtx, "BLI-LIVE-CLAIM-001", "--by", "seat:live-craftsman")
	require.NoError(t, err)
	assert.Contains(t, out, "Status: in_progress")
	assert.Contains(t, out, "Claimed BLI-LIVE-CLAIM-001 for seat:live-craftsman")

	// Verify storage mutation
	bliObj, err := p.FileStorage.Read(context.Background(), secCtx, "BLI-LIVE-CLAIM-001")
	require.NoError(t, err)
	assert.Equal(t, objects.ObjectStatusInProgress, bliObj[objects.FieldKeyStatus])
	assert.Equal(t, "seat:live-craftsman", bliObj[objects.FieldKeyClaimedBy])
}
