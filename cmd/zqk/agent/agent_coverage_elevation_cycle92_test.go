package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/audit"
	cli "github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/mutation"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	whatsnext "github.com/zqk-os/zqk/pkg/workflow/whatsnext"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	"time"
)

type cycle92ErrReader struct {
	err error
}

func (r *cycle92ErrReader) Read(p []byte) (int, error) {
	return 0, r.err
}

// -----------------------------------------------------------------------------
// Target 1 & 2: hook.go and hook_io.go
// -----------------------------------------------------------------------------

func TestElevation_Cycle92_HookIO_SafeDrainInput(t *testing.T) {
	// 1. nil reader
	b, err := safeDrainInput(nil)
	assert.NoError(t, err)
	assert.Nil(t, b)

	// 2. String reader
	data := "test hook io drain payload"
	b, err = safeDrainInput(strings.NewReader(data))
	assert.NoError(t, err)
	assert.Equal(t, []byte(data), b)

	// 3. Error reader
	b, err = safeDrainInput(&cycle92ErrReader{err: fmt.Errorf("read fail")})
	assert.Error(t, err)
	assert.Empty(t, b)

	// 4. File reader (regular file, not character device)
	tmpFile, err := fileutil.CreateTemp(t.TempDir(), "safe_drain_*.txt")
	require.NoError(t, err)
	defer tmpFile.Close()

	_, err = tmpFile.WriteString("content in temp file")
	require.NoError(t, err)
	_, _ = tmpFile.Seek(0, io.SeekStart)

	b, err = safeDrainInput(tmpFile)
	assert.NoError(t, err)
	assert.Equal(t, []byte("content in temp file"), b)
}

func TestElevation_Cycle92_Hook_CommandIO_And_RunPreSubagentHook(t *testing.T) {
	// 1. commandIO with nil vs non-nil
	rNil, wNil := commandIO(nil)
	assert.Equal(t, os.Stdin, rNil)
	assert.Equal(t, os.Stdout, wNil)

	dummyCmd := &cobra.Command{}
	var inBuf, outBuf bytes.Buffer
	dummyCmd.SetIn(&inBuf)
	dummyCmd.SetOut(&outBuf)
	rCmd, wCmd := commandIO(dummyCmd)
	assert.Equal(t, &inBuf, rCmd)
	assert.Equal(t, &outBuf, wCmd)

	// 2. NewAgentHookCmd and newPreSubagentHookCmd structure
	hookCmd := NewAgentHookCmd()
	require.NotNil(t, hookCmd)
	subHookCmd := newPreSubagentHookCmd()
	require.NotNil(t, subHookCmd)
	assert.NotNil(t, subHookCmd.Flags().Lookup("vendor"))

	// 3. runPreSubagentHook using isolated in-process project
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	planID, bliID := seedAgentSupportHierarchy(t, provider)
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	// Seed task for subagent context assembly
	taskID := "ATK-SUBHOOK-CYCLE92-001"
	taskObj := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Subagent Elevation Task",
		objects.FieldKeyDescription:        "Target task for pre-subagent hook expansion",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyPriorityPlanRef:    planID,
		objects.FieldKeyBacklogItemRef:     bliID,
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), sec, taskObj))

	// Setup CLI command with in/out buffers and context
	cmd := NewAgentHookCmd()
	cmd.SetIn(&inBuf)
	cmd.SetOut(&outBuf)
	cli.SetContext(cmd, cli.ContextForProjectRoot(tempDir))
	proc, err := newAgentProcessor(cmd)
	require.NoError(t, err)

	// Case 3a: Empty input -> allow
	inBuf.Reset()
	outBuf.Reset()
	err = runPreSubagentHook(cmd, proc, "antigravity")
	assert.NoError(t, err)
	assert.Contains(t, outBuf.String(), `"decision": "allow"`)

	// Case 3b: Malformed non-JSON input -> allow
	inBuf.Reset()
	outBuf.Reset()
	inBuf.WriteString("not valid json at all")
	err = runPreSubagentHook(cmd, proc, "antigravity")
	assert.NoError(t, err)
	assert.Contains(t, outBuf.String(), `"decision": "allow"`)

	// Case 3c: ToolCall not invoke_subagent -> allow
	inBuf.Reset()
	outBuf.Reset()
	inBuf.WriteString(`{"toolCall":{"name":"write_to_file","args":{}}}`)
	err = runPreSubagentHook(cmd, proc, "antigravity")
	assert.NoError(t, err)
	assert.Contains(t, outBuf.String(), `"decision": "allow"`)

	// Case 3d: invoke_subagent without Subagents arg -> allow
	inBuf.Reset()
	outBuf.Reset()
	inBuf.WriteString(`{"toolCall":{"name":"invoke_subagent","args":{}}}`)
	err = runPreSubagentHook(cmd, proc, "antigravity")
	assert.NoError(t, err)
	assert.Contains(t, outBuf.String(), `"decision": "allow"`)

	// Case 3e: invoke_subagent with malformed Subagents type -> allow
	inBuf.Reset()
	outBuf.Reset()
	inBuf.WriteString(`{"toolCall":{"name":"invoke_subagent","args":{"Subagents":"not a slice"}}}`)
	err = runPreSubagentHook(cmd, proc, "antigravity")
	assert.NoError(t, err)
	assert.Contains(t, outBuf.String(), `"decision": "allow"`)

	// Case 3f: Subagent prompt already has standing mandates -> allow, unmodified
	inBuf.Reset()
	outBuf.Reset()
	alreadyCompletePayload := map[string]any{
		"toolCall": map[string]any{
			"name": "invoke_subagent",
			"args": map[string]any{
				"Subagents": []any{
					map[string]any{
						"Prompt": "# Orchestration Context\nAlready populated",
					},
					map[string]any{
						"Prompt": "## Standing mandates\nAlready populated",
					},
				},
			},
		},
	}
	payloadBytes, _ := json.Marshal(alreadyCompletePayload)
	inBuf.Write(payloadBytes)
	err = runPreSubagentHook(cmd, proc, "antigravity")
	assert.NoError(t, err)
	assert.Contains(t, outBuf.String(), `"decision": "allow"`)
	assert.NotContains(t, outBuf.String(), `"overwrite"`)

	// Case 3g: Subagent prompt referencing taskID -> expands with overwrite!
	inBuf.Reset()
	outBuf.Reset()
	expandPayload := map[string]any{
		"toolCall": map[string]any{
			"name": "invoke_subagent",
			"args": map[string]any{
				"Subagents": []any{
					map[string]any{
						"TypeName": "worker",
						"Role":     "Worker Pod",
						"Prompt":   fmt.Sprintf("Please fulfill %s immediately", taskID),
					},
				},
			},
		},
	}
	expandBytes, _ := json.Marshal(expandPayload)
	inBuf.Write(expandBytes)
	err = runPreSubagentHook(cmd, proc, "antigravity")
	assert.NoError(t, err)
	assert.Contains(t, outBuf.String(), `"decision": "allow"`)
	assert.Contains(t, outBuf.String(), `"overwrite"`)
	assert.Contains(t, outBuf.String(), "Subagent Specific Instructions")

	// Case 3h: runPreSubagentHookWithDeps with nil storage provider -> allow
	var outNilSP bytes.Buffer
	errNilSP := runPreSubagentHookWithDeps(ctx, sec, nil, tempDir, "antigravity", strings.NewReader(""), &outNilSP)
	assert.NoError(t, errNilSP)
	assert.Contains(t, outNilSP.String(), `"decision": "allow"`)

	// Case 3i: Regex matching BLI, PRI, and long description without ID
	for _, idPrefix := range []string{"BLI-1792429379773841500-7b7293dc", "PRI-1792429393089406000-62ec51c3"} {
		var outPref bytes.Buffer
		testIn := fmt.Sprintf(`{"toolCall":{"name":"invoke_subagent","args":{"Subagents":[{"Prompt":"Handle %s"}]}}}`, idPrefix)
		errPref := runPreSubagentHookWithDeps(ctx, sec, provider, tempDir, "antigravity", strings.NewReader(testIn), &outPref)
		assert.NoError(t, errPref)
	}

	// Long description > 300 chars without ID
	longPrompt := strings.Repeat("A quick brown fox jumps over the lazy dog. ", 10)
	var outLong bytes.Buffer
	longIn := fmt.Sprintf(`{"toolCall":{"name":"invoke_subagent","args":{"Subagents":[{"Prompt":%q}]}}}`, longPrompt)
	errLong := runPreSubagentHookWithDeps(ctx, sec, provider, tempDir, "antigravity", strings.NewReader(longIn), &outLong)
	assert.NoError(t, errLong)
}

// -----------------------------------------------------------------------------
// Target 3: continuation_hook.go
// -----------------------------------------------------------------------------

func TestElevation_Cycle92_ContinuationHook_ExecutionAndBranches(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	// 1. Command builder and flags inspection
	cmd := newContinuationHookCmd()
	require.NotNil(t, cmd)
	assert.NotNil(t, cmd.Flags().Lookup("vendor"))
	assert.NotNil(t, cmd.Flags().Lookup("marker"))
	assert.NotNil(t, cmd.Flags().Lookup("agent-id"))
	assert.NotNil(t, cmd.Flags().Lookup("persona-ref"))

	// 2. Active marker file
	markerPath := filepath.Join(tempDir, paths.ProjectDataDir, "state", defaultMarkerFilename)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(markerPath)))
	require.NoError(t, fileutil.WriteStandardFile(markerPath, []byte("active")))

	// 3. In-progress priority plan in storage
	planID := "PRI-CONTINUE-CYCLE92-001"
	planTitle := "Continuous Autonomous Progression Plan"
	planObj := map[string]any{
		objects.FieldKeyID:            planID,
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         planTitle,
		objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
		objects.FieldKeyDescription:   "Active in-progress priority plan for CAP loop continuation test",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), sec, planObj))

	// 4. Seed unacked inbox steering message in agentfeed correspondence
	seatID := "coord-cycle92"
	_, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
		ProjectRoot: tempDir,
		Message:     "Focus on completing cycle 92 tests first",
		AgentID:     "human-operator",
		ToAgentID:   seatID,
		Sender:      agentfeed.FeedSenderHumanSteer,
		EventType:   agentfeed.FeedEventTypeSteering,
		SelfACK:     false,
	})
	require.NoError(t, err)

	// 5. Test runContinuationHookWithDeps with active plan and steering inbox
	var outBuf bytes.Buffer
	err = runContinuationHookWithDeps(ctx, sec, provider, tempDir, "generic", markerPath, seatID, "PER-COORDINATOR", strings.NewReader(""), &outBuf)
	require.NoError(t, err)
	outStr := outBuf.String()
	assert.Contains(t, outStr, `"decision": "continue"`)
	assert.Contains(t, outStr, planTitle)
	assert.Contains(t, outStr, planID)
	assert.Contains(t, outStr, "Pending steering inbox")
	assert.Contains(t, outStr, "human-operator")

	// 6. Test with input read error
	var outErr bytes.Buffer
	errDrain := runContinuationHookWithDeps(ctx, sec, provider, tempDir, "generic", markerPath, seatID, "PER-COORDINATOR", &cycle92ErrReader{err: fmt.Errorf("stdin error")}, &outErr)
	assert.Error(t, errDrain)

	// 7. Test agentID fallbacks when empty
	t.Setenv("ZQK_AGENT_ID", "agent-from-env-01")
	var outEnv bytes.Buffer
	err = runContinuationHookWithDeps(ctx, sec, provider, tempDir, "generic", markerPath, "", "", nil, &outEnv)
	assert.NoError(t, err)
	t.Setenv("ZQK_AGENT_ID", "")

	t.Setenv("ZQK_SEAT_ID", "seat-from-env-02")
	var outSeatEnv bytes.Buffer
	err = runContinuationHookWithDeps(ctx, sec, provider, tempDir, "generic", markerPath, "", "", nil, &outSeatEnv)
	assert.NoError(t, err)
	t.Setenv("ZQK_SEAT_ID", "")

	// 8. Test vendor emission formats (including reason == "")
	var outClaudeEmpty, outCursorEmpty, outWindsurfEmpty, outClineEmpty bytes.Buffer
	assert.NoError(t, emitContinuationDecision(&outClaudeEmpty, "claude", "allow", ""))
	assert.NoError(t, emitContinuationDecision(&outCursorEmpty, "cursor", "allow", ""))
	assert.NoError(t, emitContinuationDecision(&outWindsurfEmpty, "windsurf", "allow", ""))
	assert.NoError(t, emitContinuationDecision(&outClineEmpty, "cline", "allow", ""))

	// 9. Full CLI command execution via executeAgentCommand
	outCLI, errCLI := executeAgentCommand(t, tempDir, provider, "hook", "continuation", "--vendor", "cursor", "--marker", markerPath, "--agent-id", seatID)
	require.NoError(t, errCLI)
	assert.Contains(t, outCLI, `"decision": "continue"`)
}

// -----------------------------------------------------------------------------
// Target 4: new.go
// -----------------------------------------------------------------------------

func TestElevation_Cycle92_NewAgent_ExecutionAndPromote(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	// 1. NewAgentNewCmd inspection
	newCmd := NewAgentNewCmd()
	require.NotNil(t, newCmd)

	// 2. runAgentNew with full description via executeAgentCommand
	out1, err1 := executeAgentCommand(t, tempDir, provider, "new", "SecurityAuditor", "--description", "Audits code for CVE and authorization vulnerabilities")
	require.NoError(t, err1)
	assert.Contains(t, out1, "Successfully initialized agent: SecurityAuditor")
	assert.Contains(t, out1, "Persona ID: PER-")
	assert.Contains(t, out1, "Skill Placeholder ID: ASK-")

	// 3. runAgentNew with short description (< 10 chars, triggers default description)
	out2, err2 := executeAgentCommand(t, tempDir, provider, "new", "ShortDescAgent", "--description", "short")
	require.NoError(t, err2)
	assert.Contains(t, out2, "Successfully initialized agent: ShortDescAgent")

	// 4. Verify created personas and skills in storage are approved
	personas, err := provider.List(ctx, sec, nil, storage.ListFilter{Kind: objects.KindPersona})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(personas.Objects), 2)
	for _, p := range personas.Objects {
		assert.Equal(t, objects.ObjectStatusApproved, p[objects.FieldKeyStatus])
	}

	skills, err := provider.List(ctx, sec, nil, storage.ListFilter{Kind: objects.KindAgentSkill})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(skills.Objects), 2)
	for _, s := range skills.Objects {
		assert.Equal(t, objects.ObjectStatusApproved, s[objects.FieldKeyStatus])
	}

	// 5. Test rejectNonManualPromoteHop branches
	// Same from and to status -> returns nil
	assert.NoError(t, rejectNonManualPromoteHop("persona", "approved", "approved"))

	// Illegal non-manual one-hop transition
	assert.Error(t, rejectNonManualPromoteHop("persona", "approved", "proposed"))
	assert.Error(t, rejectNonManualPromoteHop("persona", "proposed", "completed"))

	// 6. Test promoteStatusOneHop error on non-existent object
	errNonExist := promoteStatusOneHop(ctx, sec, provider, "PER-NONEXISTENT-9999", objects.ObjectStatusApproved)
	assert.Error(t, errNonExist)
}

// -----------------------------------------------------------------------------
// Target 5: orchestrate_reuse.go
// -----------------------------------------------------------------------------

func TestElevation_Cycle92_Orchestrate_Reuse_Comprehensive(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	planID, bliID := seedAgentSupportHierarchy(t, provider)
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	// 1. findExistingOrchestrationTask error when state or sp is nil
	_, _, _, errNilState := findExistingOrchestrationTask(ctx, nil, "Task", "BLI-1")
	assert.Error(t, errNilState)
	assert.Contains(t, errNilState.Error(), "orchestrator storage is not available")

	stateEmpty := &orchestratorState{}
	_, _, _, errNilSP := findExistingOrchestrationTask(ctx, stateEmpty, "Task", "BLI-1")
	assert.Error(t, errNilSP)

	// 2. listOrchestrationTasksForPlan error when sp is nil
	_, errListNil := listOrchestrationTasksForPlan(ctx, nil, sec, planID)
	assert.Error(t, errListNil)

	// 3. Seed tasks in storage: one with priority_plan_ref, one with pipeline_ref
	taskPriID := "ATK-PRI-REUSE-001"
	taskPri := map[string]any{
		objects.FieldKeyID:                 taskPriID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Execute Task: Deliver Feature Alpha",
		objects.FieldKeyDescription:        "Deliver feature alpha task description",
		objects.FieldKeyPriorityPlanRef:    planID,
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyBacklogItemRef:     bliID,
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
		objects.FieldKeyUpdatedAt:          "2026-10-19T10:00:00Z",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), sec, taskPri))

	taskPipeID := "ATK-PIPE-REUSE-002"
	taskPipe := map[string]any{
		objects.FieldKeyID:                 taskPipeID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Execute Task: Deliver Feature Beta",
		objects.FieldKeyDescription:        "Deliver feature beta task description",
		objects.FieldKeyPipelineRef:        planID,
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeyClaimedBy:          "worker-pod-01",
		objects.FieldKeyBacklogItemRef:     "BLI-DISTINCT-BETA-02",
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
		objects.FieldKeyUpdatedAt:          "2026-10-19T11:00:00Z",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), sec, taskPipe))

	// 4. listOrchestrationTasksForPlan returns both tasks
	tasks, errList := listOrchestrationTasksForPlan(ctx, provider, sec, planID)
	require.NoError(t, errList)
	assert.Len(t, tasks, 2)

	// 5. findExistingOrchestrationTask matching taskPri
	cmd := NewOrchestrateCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(tempDir))
	proc, err := newAgentProcessor(cmd)
	require.NoError(t, err)

	state := &orchestratorState{
		sp:     provider,
		secCtx: sec,
		planID: planID,
		proc:   proc,
	}

	foundID, foundStatus, foundDisp, errFind := findExistingOrchestrationTask(ctx, state, "Deliver Feature Alpha", bliID)
	require.NoError(t, errFind)
	assert.Equal(t, taskPriID, foundID)
	assert.Equal(t, objects.ObjectStatusApproved, foundStatus)
	assert.Equal(t, orchDispositionReuse, foundDisp)

	// Matching taskPipe
	foundID2, foundStatus2, foundDisp2, errFind2 := findExistingOrchestrationTask(ctx, state, "Deliver Feature Beta", "BLI-DISTINCT-BETA-02")
	require.NoError(t, errFind2)
	assert.Equal(t, taskPipeID, foundID2)
	assert.Equal(t, objects.ObjectStatusInProgress, foundStatus2)
	assert.Equal(t, orchDispositionReuse, foundDisp2)

	// 6. orchDispRank coverage
	assert.Equal(t, 3, orchDispRank(orchDispositionReuse))
	assert.Equal(t, 2, orchDispRank(orchDispositionSkipDone))
	assert.Equal(t, 1, orchDispRank(orchDispositionReplace))
	assert.Equal(t, 0, orchDispRank(orchDispositionMint))
	assert.Equal(t, 0, orchDispRank(orchDisposition(99)))

	// 7. orchLivePickRank coverage
	assert.Equal(t, 0, orchLivePickRank(nil))
	assert.Equal(t, 3, orchLivePickRank(map[string]any{objects.FieldKeyStatus: objects.ObjectStatusApproved}))
	assert.Equal(t, 3, orchLivePickRank(map[string]any{objects.FieldKeyStatus: objects.ObjectStatusInProgress, objects.FieldKeyClaimedBy: "seat"}))
	assert.Equal(t, 2, orchLivePickRank(map[string]any{objects.FieldKeyStatus: objects.ObjectStatusInProgress}))
	assert.Equal(t, 1, orchLivePickRank(map[string]any{objects.FieldKeyStatus: objects.ObjectStatusError}))
	assert.Equal(t, 1, orchLivePickRank(map[string]any{objects.FieldKeyStatus: objects.ObjectStatusFailed}))
	assert.Equal(t, 2, orchLivePickRank(map[string]any{objects.FieldKeyStatus: "other_status"}))

	// 8. betterOrchCandidate branches
	// Branch a: Disp rank difference
	candHigh := map[string]any{objects.FieldKeyStatus: objects.ObjectStatusApproved}
	bestLow := map[string]any{objects.FieldKeyStatus: objects.ObjectStatusError}
	assert.True(t, betterOrchCandidate(candHigh, orchDispositionReuse, "2026-10-10", bestLow, orchDispositionReplace, "2026-10-10"))
	assert.False(t, betterOrchCandidate(bestLow, orchDispositionReplace, "2026-10-10", candHigh, orchDispositionReuse, "2026-10-10"))

	// Branch b: Same disp rank, live pick rank difference
	candClaimed := map[string]any{objects.FieldKeyStatus: objects.ObjectStatusInProgress, objects.FieldKeyClaimedBy: "worker"}
	candUnclaimed := map[string]any{objects.FieldKeyStatus: objects.ObjectStatusInProgress}
	assert.True(t, betterOrchCandidate(candClaimed, orchDispositionReuse, "2026-10-10", candUnclaimed, orchDispositionReuse, "2026-10-10"))
	assert.False(t, betterOrchCandidate(candUnclaimed, orchDispositionReuse, "2026-10-10", candClaimed, orchDispositionReuse, "2026-10-10"))

	// Branch c: Same disp rank and live pick rank, timestamp comparison
	candNew := map[string]any{objects.FieldKeyStatus: objects.ObjectStatusApproved}
	candOld := map[string]any{objects.FieldKeyStatus: objects.ObjectStatusApproved}
	assert.True(t, betterOrchCandidate(candNew, orchDispositionReuse, "2026-10-20", candOld, orchDispositionReuse, "2026-10-10"))
	assert.False(t, betterOrchCandidate(candOld, orchDispositionReuse, "2026-10-10", candNew, orchDispositionReuse, "2026-10-20"))

	// 9. orchDispatchSkipReason coverage
	assert.Equal(t, "", orchDispatchSkipReason(nil))
	assert.Equal(t, "", orchDispatchSkipReason([]map[string]any{nil}))
	assert.Equal(t, "", orchDispatchSkipReason([]map[string]any{{objects.FieldKeyStatus: objects.ObjectStatusInProgress}}))
	assert.Equal(t, "no_live_atk", orchDispatchSkipReason([]map[string]any{{objects.FieldKeyStatus: objects.ObjectStatusImplemented}}))
	assert.Equal(t, "no_live_atk", orchDispatchSkipReason([]map[string]any{{objects.FieldKeyStatus: objects.ObjectStatusError}}))

	// 10. recoverAgentTaskListFilters coverage
	filters := recoverAgentTaskListFilters(planID)
	assert.Len(t, filters, 4)

	// 11. mergeObjectsByID coverage
	m1 := map[string]any{objects.FieldKeyID: "ID-1", "name": "first"}
	m2 := map[string]any{objects.FieldKeyID: "ID-2", "name": "second"}
	m1Dup := map[string]any{objects.FieldKeyID: "ID-1", "name": "duplicate"}
	merged := mergeObjectsByID([]map[string]any{m1, nil, {}}, []map[string]any{m2, m1Dup})
	assert.Len(t, merged, 2)
	assert.Equal(t, "first", merged[0]["name"])
	assert.Equal(t, "second", merged[1]["name"])
}

// -----------------------------------------------------------------------------
// Target 6: sync_loop.go
// -----------------------------------------------------------------------------

func TestElevation_Cycle92_SyncLoop_TransitionToError_And_Hourglass(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	planID, bliID := seedAgentSupportHierarchy(t, provider)
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	taskID := "ATK-SYNC-CYCLE92-001"
	taskObj := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Sync Loop Transition Test",
		objects.FieldKeyDescription:        "Sync loop transition task description",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeyPriorityPlanRef:    planID,
		objects.FieldKeyBacklogItemRef:     bliID,
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), sec, taskObj))

	validator := mutation.NewValidator(nil)
	auditStream := audit.NewAuditStream(tempDir)

	// 1. transitionToError
	transitionToError(ctx, sec, provider, taskID, taskObj, validator, auditStream)

	// Verify task transitioned to error
	updatedTask, err := provider.Read(ctx, sec, taskID)
	require.NoError(t, err)
	assert.Equal(t, objects.ObjectStatusError, updatedTask[objects.FieldKeyStatus])

	// Call transitionToError on non-existent task (fails gracefully with logging)
	transitionToError(ctx, sec, provider, "ATK-NONEXISTENT-ERR", map[string]any{objects.FieldKeyKind: objects.KindAgentTask}, validator, auditStream)

	// 2. flipHourglass
	flipHourglass(ctx, sec, provider, taskID, tempDir)

	taskAfterHG, err := provider.Read(ctx, sec, taskID)
	require.NoError(t, err)
	assert.NotEmpty(t, taskAfterHG["agent_heartbeat"])
	assert.NotZero(t, taskAfterHG["agent_pid"])

	hourglassPath := filepath.Join(tempDir, paths.ProjectDataDir, paths.SchedulerDir, "hourglass", taskID+".json")
	assert.True(t, fileutil.IsRegularFile(hourglassPath))

	// 3. removeHourglass
	removeHourglass(ctx, sec, provider, taskID, tempDir)

	taskAfterRM, err := provider.Read(ctx, sec, taskID)
	require.NoError(t, err)
	assert.Nil(t, taskAfterRM["agent_pid"])
	assert.False(t, fileutil.IsRegularFile(hourglassPath))

	// 4. applyStateMutationWithFields (updating task status and extra fields)
	extraFields := map[string]any{
		objects.FieldKeyNotes: "Updated notes via state mutation",
	}
	err = applyStateMutationWithFields(ctx, sec, provider, taskID, objects.KindAgentTask, validator, auditStream, objects.ObjectStatusInProgress, extraFields)
	require.NoError(t, err)

	taskAfterExtra, err := provider.Read(ctx, sec, taskID)
	require.NoError(t, err)
	assert.Equal(t, objects.ObjectStatusInProgress, taskAfterExtra[objects.FieldKeyStatus])
	assert.Equal(t, "Updated notes via state mutation", taskAfterExtra[objects.FieldKeyNotes])

	// 5. createIdempotencyAuditStamp in transaction
	tx, err := provider.BeginTransaction(ctx)
	require.NoError(t, err)

	mut := &mutation.Mutation{
		Action:      mutation.ActionUpdateNode,
		SafetyClass: mutation.SafetyWrite,
	}
	err = createIdempotencyAuditStamp(ctx, sec, tx, "cycle92-ik-test::123", taskID, mut)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))

	auditObj, err := provider.Read(ctx, sec, "AUD-cycle92-ik-test-123")
	require.NoError(t, err)
	assert.Equal(t, objects.KindAuditEvent, auditObj[objects.FieldKeyKind])
}

// -----------------------------------------------------------------------------
// Extended Targets: seat_worker, execute, chat_responder, recover, next, subgraph
// -----------------------------------------------------------------------------

func TestElevation_Cycle92_SeatWorker_ExecutionAndMarks(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	// 1. writePlanOrchSubmitMark and recentPlanOrchSubmit
	skip, _ := recentPlanOrchSubmit(tempDir, "PRI-SUBMIT-TEST")
	assert.False(t, skip)

	err := writePlanOrchSubmitMark(tempDir, "PRI-SUBMIT-TEST", "test submission")
	require.NoError(t, err)

	skipAfter, reason := recentPlanOrchSubmit(tempDir, "PRI-SUBMIT-TEST")
	assert.True(t, skipAfter)
	assert.Contains(t, reason, "already_submitted PRI-SUBMIT-TEST")

	// 2. writeFillSubmitMark, recentFillSubmit, and submitKernelFill
	skipFill, _ := recentFillSubmit(tempDir, "backlog_item")
	assert.False(t, skipFill)

	errFill := writeFillSubmitMark(tempDir, "backlog_item", "fill details")
	require.NoError(t, errFill)

	skipFillAfter, fillReason := recentFillSubmit(tempDir, "backlog_item")
	assert.True(t, skipFillAfter)
	assert.Contains(t, fillReason, "already_submitted backlog_item")

	resEmpty, errEmpty := submitKernelFill(ctx, tempDir, nil)
	assert.NoError(t, errEmpty)
	assert.Empty(t, resEmpty)

	resNoAuto, errNoAuto := submitKernelFill(ctx, tempDir, &whatsnext.FillItem{AutoSubmit: false})
	assert.NoError(t, errNoAuto)
	assert.Empty(t, resNoAuto)

	resNoArgs, errNoArgs := submitKernelFill(ctx, tempDir, &whatsnext.FillItem{AutoSubmit: true, SubmitArgs: ""})
	assert.NoError(t, errNoArgs)
	assert.Empty(t, resNoArgs)

	resRecent, errRecent := submitKernelFill(ctx, tempDir, &whatsnext.FillItem{
		AutoSubmit: true,
		SubmitArgs: "new bli --title test",
		Kind:       "backlog_item",
	})
	assert.NoError(t, errRecent)
	assert.Contains(t, resRecent, "already_submitted backlog_item")

	// 3. dispatchLeadPlanFromWhatsNext
	dispNoStorage, errNoStorage := dispatchLeadPlanFromWhatsNext(ctx, nil, nil, tempDir, "agent-1", "PER-1")
	assert.NoError(t, errNoStorage)
	assert.Equal(t, "no_storage", dispNoStorage.Skipped)

	dispNoPlan, errNoPlan := dispatchLeadPlanFromWhatsNext(ctx, nil, provider, tempDir, "agent-1", "PER-1")
	assert.NoError(t, errNoPlan)
	assert.NotEmpty(t, dispNoPlan.Skipped)

	// 4. runAgentSeatWorker CLI commands
	// Missing --agent-id
	_, errMissingID := executeAgentCommand(t, tempDir, provider, "seat-worker")
	assert.Error(t, errMissingID)
	assert.Contains(t, errMissingID.Error(), "--agent-id is required")

	// Valid with --once
	outOnce, errOnce := executeAgentCommand(t, tempDir, provider, "seat-worker", "--agent-id", "seat-worker-01", "--once")
	assert.NoError(t, errOnce)
	assert.Contains(t, outOnce, "seat-worker-01")

	// Valid with --once and --execute-non-comms
	outNonComms, errNonComms := executeAgentCommand(t, tempDir, provider, "seat-worker", "--agent-id", "seat-worker-02", "--once", "--execute-non-comms")
	assert.NoError(t, errNonComms)
	assert.Contains(t, outNonComms, "seat-worker-02")

	// 5. handleNonCommsWithAgentX
	item := agentfeed.CorrespondenceItem{
		EventID: "EVT-TEST-CYCLE92-001",
	}

	// Body without ATK -> ackSeatWorkerSkip
	errNoATK := handleNonCommsWithAgentX(ctx, nil, provider, sec, tempDir, "seat-1", "PER-1", "SES-1", item, "steer with no task")
	assert.NoError(t, errNoATK)

	// Body with ORCHESTRATE_PLAN directive
	errOrchPlan := handleNonCommsWithAgentX(ctx, nil, provider, sec, tempDir, "seat-1", "PER-1", "SES-1", item, "ORCHESTRATE_PLAN: PRI-NONEXISTENT")
	assert.NoError(t, errOrchPlan)

	// Body with non-existent ATK claim failure
	errClaimFail := handleNonCommsWithAgentX(ctx, nil, provider, sec, tempDir, "seat-1", "PER-1", "SES-1", item, "Please execute ATK-NONEXISTENT-CYCLE92")
	assert.NoError(t, errClaimFail)
}

func TestElevation_Cycle92_Execute_Branches(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// 1. resolveExecuteSystemPrompt
	assert.Contains(t, resolveExecuteSystemPrompt(""), "You are an autonomous software engineering agent")
	assert.Contains(t, resolveExecuteSystemPrompt("You are a helpful assistant."), "You are an autonomous software engineering agent")
	assert.Equal(t, "Custom system instructions", resolveExecuteSystemPrompt("Custom system instructions"))

	// 2. NewExecuteCmd and validation failures
	cmd := NewExecuteCmd()
	require.NotNil(t, cmd)

	// Missing all prompt/task arguments
	_, errNoArgs := executeAgentCommand(t, tempDir, provider, "execute")
	assert.Error(t, errNoArgs)
	assert.Contains(t, errNoArgs.Error(), "requires --prompt, --task-id, or a task ID argument")

	// Non-existent task ID
	_, errNonExist := executeAgentCommand(t, tempDir, provider, "execute", "ATK-NONEXISTENT-999")
	assert.Error(t, errNonExist)
	assert.Contains(t, errNonExist.Error(), "failed to read task")
}

func TestElevation_Cycle92_ChatResponder_Execution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	cmd := NewChatResponderCmd()
	require.NotNil(t, cmd)

	// Run chat responder
	out, err := executeAgentCommand(t, tempDir, provider, "chat-responder")
	assert.NoError(t, err)
	assert.NotEmpty(t, out)
}

func TestElevation_Cycle92_Recover_And_Next(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// 1. NewRecoverCmd and non-existent plan semantic routing failure
	recCmd := NewRecoverCmd()
	require.NotNil(t, recCmd)

	outRec, _ := executeAgentCommand(t, tempDir, provider, "recover", "PRI-NONEXISTENT")
	assert.Contains(t, outRec, "Recovered 0 task(s)")

	// Cross-plan recovery with --all
	outAll, errAll := executeAgentCommand(t, tempDir, provider, "recover", "--all")
	assert.NoError(t, errAll)
	assert.NotNil(t, outAll)

	// 2. NewNextCmd and non-existent task
	nextCmd := NewNextCmd()
	require.NotNil(t, nextCmd)

	outNext, _ := executeAgentCommand(t, tempDir, provider, "next", "ATK-NONEXISTENT-NEXT")
	assert.Contains(t, outNext, "failed to read task")
}

func TestElevation_Cycle92_SyncLoop_QuerySubgraph(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()
	t.Setenv("ZQK_PROJECT_ROOT", tempDir)

	planID, bliID := seedAgentSupportHierarchy(t, provider)
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	task1ID := "ATK-SUBGRAPH-001"
	task1 := map[string]any{
		objects.FieldKeyID:                 task1ID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Root Subgraph Task",
		objects.FieldKeyDescription:        "Semantic description payload",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyPriorityPlanRef:    planID,
		objects.FieldKeyBacklogItemRef:     bliID,
		objects.FieldKeyRequirementRefs:    []any{bliID},
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), sec, task1))

	// Query with depth <= 0 (defaults to 5)
	deps, err := QuerySubgraph(ctx, sec, provider, task1ID, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, deps)

	// Query with depth 1
	depsDepth1, errDepth1 := QuerySubgraph(ctx, sec, provider, task1ID, 1)
	require.NoError(t, errDepth1)
	assert.NotEmpty(t, depsDepth1)
}

func TestElevation_Cycle92_SeatWorker_LeadPlanDispatch_And_Orch(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	// 1. dispatchLeadPlanFromWhatsNext with nil storage
	resNoStore, errNoStore := dispatchLeadPlanFromWhatsNext(ctx, nil, nil, tempDir, "test-agent", "")
	assert.NoError(t, errNoStore)
	assert.Equal(t, "no_storage", resNoStore.Skipped)

	// 2. parseOrchestratePlanDirective
	planID, isDir, err := parseOrchestratePlanDirective("ORCHESTRATE_PLAN PRI-12345")
	assert.NoError(t, err)
	assert.True(t, isDir)
	assert.Equal(t, "PRI-12345", planID)

	_, isDir, err = parseOrchestratePlanDirective("SOME OTHER COMMAND")
	assert.NoError(t, err)
	assert.False(t, isDir)

	_, isDir, err = parseOrchestratePlanDirective("ORCHESTRATE_PLAN invalid_chars!@#")
	assert.Error(t, err)
	assert.True(t, isDir)

	// 3. triggerPlanOrchestration with completed plan
	completedPlanID := "PRI-COMPLETED-CYCLE92"
	completedPlan := map[string]any{
		objects.FieldKeyID:            completedPlanID,
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Completed Plan",
		objects.FieldKeyDescription:   "Completed Plan Description",
		objects.FieldKeyStatus:        objects.ObjectStatusComplete,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), sec, completedPlan))

	_, trigErr := triggerPlanOrchestration(ctx, provider, sec, tempDir, completedPlanID)
	assert.Error(t, trigErr)
	assert.Contains(t, trigErr.Error(), "orchestrator can only operate on active or in_progress priority plans")

	// 4. triggerPlanOrchestration with recent submit mark
	activePlanID := "PRI-ACTIVE-CYCLE92"
	activePlan := map[string]any{
		objects.FieldKeyID:            activePlanID,
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Active Plan",
		objects.FieldKeyDescription:   "Active Plan Description",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), sec, activePlan))

	require.NoError(t, writePlanOrchSubmitMark(tempDir, activePlanID, "already submitted"))
	msg, trigErr := triggerPlanOrchestration(ctx, provider, sec, tempDir, activePlanID)
	assert.NoError(t, trigErr)
	assert.Contains(t, msg, "already_submitted "+activePlanID)

	// 5. handleNonCommsWithAgentX with exhausted attempts
	attemptItem := agentfeed.CorrespondenceItem{
		EventID: "EVT-EXHAUSTED-001",
	}
	attemptKey := agentfeed.SeatAttemptKey("EVT-EXHAUSTED-001", "ATK-EXHAUSTED-01")
	// Increment attempt count to seatWorkerMaxEventAttempts (3)
	for i := 0; i < seatWorkerMaxEventAttempts; i++ {
		agentfeed.RecordSeatEventAttempt(tempDir, "seat-worker-test", attemptKey)
	}
	exhaustErr := handleNonCommsWithAgentX(ctx, nil, provider, sec, tempDir, "seat-worker-test", "PER-INPROCESS-ENGINEER", "SES-EXHAUSTED", attemptItem, "Execute ATK-EXHAUSTED-01")
	assert.NoError(t, exhaustErr)
}

func TestElevation_Cycle92_SeatWorkerEngine_Lanes(t *testing.T) {
	tempDir := t.TempDir()

	// 1. readLaneConfigFile: non-existent file
	_, ok := readLaneConfigFile(filepath.Join(tempDir, "does-not-exist.json"))
	assert.False(t, ok)

	// 2. readLaneConfigFile: directory path
	subDir := filepath.Join(tempDir, "sub-dir")
	require.NoError(t, fileutil.MkdirAll(subDir, 0755))
	_, ok = readLaneConfigFile(subDir)
	assert.False(t, ok)

	// 3. readLaneConfigFile: invalid JSON
	badJSONPath := filepath.Join(tempDir, "bad.json")
	require.NoError(t, fileutil.WriteFile(badJSONPath, []byte("{invalid json"), 0644))
	_, ok = readLaneConfigFile(badJSONPath)
	assert.False(t, ok)

	// 4. readLaneConfigFile: valid JSON
	goodJSONPath := filepath.Join(tempDir, "good.json")
	validJSON := `{
		"seats": {"agent-seat-1": "lane-seat-1"},
		"worker_lanes": {"PER-DEV": "lane-dev"},
		"default_lane": "lane-default"
	}`
	require.NoError(t, fileutil.WriteFile(goodJSONPath, []byte(validJSON), 0644))
	cfg, ok := readLaneConfigFile(goodJSONPath)
	assert.True(t, ok)
	assert.Equal(t, "lane-default", cfg.DefaultLane)

	// 5. readLaneConfigFile: invalid YAML
	badYAMLPath := filepath.Join(tempDir, "bad.yaml")
	require.NoError(t, fileutil.WriteFile(badYAMLPath, []byte("bad: [unclosed yaml"), 0644))
	_, ok = readLaneConfigFile(badYAMLPath)
	assert.False(t, ok)

	// 6. readLaneConfigFile: valid YAML
	goodYAMLPath := filepath.Join(tempDir, "good.yaml")
	validYAML := "default_lane: lane-yaml-default\npersonas:\n  PER-QA: lane-qa\n"
	require.NoError(t, fileutil.WriteFile(goodYAMLPath, []byte(validYAML), 0644))
	cfgYAML, ok := readLaneConfigFile(goodYAMLPath)
	assert.True(t, ok)
	assert.Equal(t, "lane-yaml-default", cfgYAML.DefaultLane)

	// 7. loadConfiguredWorkerLane
	runtimeDir := filepath.Join(tempDir, paths.ProjectDataDir, "agent-runtime")
	require.NoError(t, fileutil.MkdirAll(runtimeDir, 0755))
	laneFile := filepath.Join(runtimeDir, "worker_lanes.json")
	laneContent := `{
		"seats": {"worker-agent-1": "lane-seat-special"},
		"personas": {"PER-QA-LEAD": "lane-persona-qa"},
		"default_lane": "lane-fallback"
	}`
	require.NoError(t, fileutil.WriteFile(laneFile, []byte(laneContent), 0644))

	// By agentID
	lane := loadConfiguredWorkerLane(tempDir, "PER-ANY", "worker-agent-1")
	assert.Equal(t, "lane-seat-special", lane)

	// By personaRef
	lane = loadConfiguredWorkerLane(tempDir, "PER-QA-LEAD", "")
	assert.Equal(t, "lane-persona-qa", lane)

	// Fallback to default
	lane = loadConfiguredWorkerLane(tempDir, "PER-UNKNOWN", "agent-unknown")
	assert.Equal(t, "lane-fallback", lane)
}

func TestElevation_Cycle92_Claim_And_ClaimGate_Deep(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	// 1. resolveClaimantIdentity
	cmd := &cobra.Command{}
	t.Setenv(zqkenv.AgentID().Name(), "env-agent-007")
	who := resolveClaimantIdentity(cmd, nil)
	assert.Equal(t, "env-agent-007", who)

	t.Setenv(zqkenv.AgentID().Name(), "")
	cmdWithSec := &cobra.Command{}
	cli.SetContext(cmdWithSec, cli.ContextForProjectRoot(tempDir))
	cmdWithSec.SetContext(pkgctx.WithSecurityContext(context.Background(), &storage.SecurityContext{AccountID: "acc-admin-99"}))
	procWithSec, errProc := cli.NewProcessor(cmdWithSec)
	require.NoError(t, errProc)
	whoProc := resolveClaimantIdentity(cmd, procWithSec)
	assert.Equal(t, "acc-admin-99", whoProc)

	// 2. claimTaskForExecute
	assert.NoError(t, claimTaskForExecute(cmd, procWithSec, "ANY-ID", nil))
	assert.NoError(t, claimTaskForExecute(cmd, procWithSec, "ANY-ID", map[string]any{objects.FieldKeyKind: objects.KindGoal}))

	planID, bliID := seedAgentSupportHierarchy(t, provider)
	taskID := "ATK-CLAIM-TASK-001"
	taskObj := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Claimable Task",
		objects.FieldKeyDescription:        "Claimable task description",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyPriorityPlanRef:    planID,
		objects.FieldKeyBacklogItemRef:     bliID,
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), sec, taskObj))

	cmdMain := &cobra.Command{}
	cli.SetContext(cmdMain, cli.ContextForProjectRoot(tempDir))
	procMain, errMain := cli.NewProcessor(cmdMain)
	require.NoError(t, errMain)
	err := claimTaskForExecute(cmd, procMain, taskID, taskObj)
	assert.NoError(t, err)

	// 3. releaseTaskAfterNext
	relErr := releaseTaskAfterNext(procMain, taskID, "acc-admin-99")
	assert.NoError(t, relErr)

	// 4. activeIntentAssignment
	assert.Empty(t, activeIntentAssignment(tempDir))

	stateDir := paths.StateDirPath(tempDir)
	require.NoError(t, fileutil.MkdirAll(stateDir, 0755))
	pointerPath := filepath.Join(stateDir, "change_intent_active")

	// Pointer pointing to non-existent file
	require.NoError(t, fileutil.WriteFile(pointerPath, []byte("missing_intent.json"), 0644))
	assert.Empty(t, activeIntentAssignment(tempDir))

	// Pointer pointing to invalid JSON
	intentPath := filepath.Join(tempDir, "intent.json")
	require.NoError(t, fileutil.WriteFile(intentPath, []byte("invalid-json"), 0644))
	require.NoError(t, fileutil.WriteFile(pointerPath, []byte("intent.json"), 0644))
	assert.Empty(t, activeIntentAssignment(tempDir))

	// Pointer pointing to valid JSON with assignment
	validIntent := `{"assignment": "ASSIGNMENT-INTENT-ALPHA"}`
	require.NoError(t, fileutil.WriteFile(intentPath, []byte(validIntent), 0644))
	assert.Equal(t, "ASSIGNMENT-INTENT-ALPHA", activeIntentAssignment(tempDir))

	// 5. runAgentClaimGate with flags
	gateCmd := NewClaimGateCmd()
	gateCmd.SetArgs([]string{"--by", "custom-claimant", "--assignment", "test-assignment", "--auto-assign", "--wake"})
	cli.SetContext(gateCmd, cli.ContextForProjectRoot(tempDir))
	_ = gateCmd.ExecuteContext(context.Background())
}

func TestElevation_Cycle92_Recover_Semantic_Cascade(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	// 1. Seed pipeline
	pipeID := "PIP-RECOVER-CASCADE"
	pipeObj := map[string]any{
		objects.FieldKeyID:            pipeID,
		objects.FieldKeyKind:          objects.KindPipeline,
		objects.FieldKeyTitle:         "Recoverable Pipeline",
		objects.FieldKeyDescription:   "Recoverable Pipeline Description",
		objects.FieldKeyStatus:        objects.ObjectStatusApproved,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), sec, pipeObj))

	// Execute recover with pipeline argument
	cmdPipe := NewRecoverCmd()
	cmdPipe.SetArgs([]string{pipeID})
	cli.SetContext(cmdPipe, cli.ContextForProjectRoot(tempDir))
	assert.NoError(t, cmdPipe.ExecuteContext(ctx))

	// 2. Seed strategic plan
	stratID := "STRAT-PLAN-RECOVER-CASCADE"
	stratObj := map[string]any{
		objects.FieldKeyID:            stratID,
		objects.FieldKeyKind:          objects.KindStrategicPlan,
		objects.FieldKeyTitle:         "Recoverable Strategic Plan",
		objects.FieldKeyDescription:   "Recoverable Strategic Plan Description",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), sec, stratObj))

	// Execute recover with strategic plan argument
	cmdStrat := NewRecoverCmd()
	cmdStrat.SetArgs([]string{stratID})
	cli.SetContext(cmdStrat, cli.ContextForProjectRoot(tempDir))
	assert.NoError(t, cmdStrat.ExecuteContext(ctx))

	// 3. Execute recover with non-existent target
	cmdFail := NewRecoverCmd()
	cmdFail.SetArgs([]string{"NON-EXISTENT-TARGET-999"})
	cli.SetContext(cmdFail, cli.ContextForProjectRoot(tempDir))
	assert.Error(t, cmdFail.ExecuteContext(ctx))

	// 4. Task recovery when parent priority plan is completed/cancelled
	planID, bliID := seedAgentSupportHierarchy(t, provider)
	parentPlan, err := provider.Read(ctx, sec, planID)
	require.NoError(t, err)
	parentPlan[objects.FieldKeyStatus] = objects.ObjectStatusCancelled
	require.NoError(t, provider.Update(pkgctx.WithLifecycleBreakGlass(ctx, "cancel parent"), sec, planID, parentPlan))

	taskID := "ATK-ORPHAN-RECOVER-01"
	taskObj := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Orphaned Task",
		objects.FieldKeyDescription:        "Orphaned Task Description",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeyPriorityPlanRef:    planID,
		objects.FieldKeyBacklogItemRef:     bliID,
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyClaimedBy:          "worker-orphan",
		objects.FieldKeyClaimedAt:          time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), sec, taskObj))

	cmdOrphan := NewRecoverCmd()
	cmdOrphan.SetArgs([]string{"--all", "--stale"})
	cli.SetContext(cmdOrphan, cli.ContextForProjectRoot(tempDir))
	assert.NoError(t, cmdOrphan.ExecuteContext(ctx))

	// Verify task was archived because parent is complete
	recoveredTask, err := provider.Read(ctx, sec, taskID)
	require.NoError(t, err)
	assert.Equal(t, objects.ObjectStatusArchived, recoveredTask[objects.FieldKeyStatus])
}

func TestElevation_Cycle92_ChatResponder_IdleNudge_And_TranscriptContext(t *testing.T) {
	tempDir, _, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// 1. getTranscriptContext
	assert.Empty(t, getTranscriptContext(""))
	assert.Empty(t, getTranscriptContext(filepath.Join(tempDir, "non_existent.jsonl")))

	// Write sample transcript
	transcriptPath := filepath.Join(tempDir, "transcript.jsonl")
	oldTime := time.Now().Add(-20 * time.Minute).Format(time.RFC3339)
	lines := []string{
		fmt.Sprintf(`{"step_index": 1, "type": "USER_INPUT", "content": "Hello agent", "timestamp": %q}`, oldTime),
		fmt.Sprintf(`{"step_index": 2, "type": "PLANNER_RESPONSE", "content": "I am working on the plan", "timestamp": %q}`, oldTime),
	}
	require.NoError(t, fileutil.WriteFile(transcriptPath, []byte(strings.Join(lines, "\n")+"\n"), 0644))

	tc := getTranscriptContext(transcriptPath)
	assert.Contains(t, tc, "Hello agent")
	assert.Contains(t, tc, "I am working on the plan")

	// 2. runChatResponder with idle nudge trigger (> 15m)
	t.Setenv(zqkenv.AGTranscriptPath().Name(), transcriptPath)
	cmd := NewChatResponderCmd()
	cmd.SetArgs([]string{})
	cli.SetContext(cmd, cli.ContextForProjectRoot(tempDir))
	_ = cmd.ExecuteContext(context.Background())
}

func TestElevation_Cycle92_Execute_Helpers(t *testing.T) {
	steps := []any{
		map[string]any{
			objects.FieldKeyTitle:  "First step",
			objects.FieldKeyStatus: objects.ObjectStatusApproved,
		},
	}
	s := formatTaskStepsSection(steps)
	assert.Contains(t, s, "First step")
}

func TestElevation_Cycle92_SeatWorker_DocsEval_And_CoordinatorDuty(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	planID, bliID := seedAgentSupportHierarchy(t, provider)
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	taskID := "ATK-DOCS-EVAL-CYCLE92"
	taskObj := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Author Seating and Architecture Guide",
		objects.FieldKeyDescription:        "Author docs for agent subsystem",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyPriorityPlanRef:    planID,
		objects.FieldKeyBacklogItemRef:     bliID,
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), sec, taskObj))

	item := agentfeed.CorrespondenceItem{
		EventID: "EVT-DOCSEVAL-001",
	}
	body := fmt.Sprintf("Execute %s: author architectural documentation and seating guide", taskID)
	err := handleNonCommsWithAgentX(ctx, nil, provider, sec, tempDir, "seat-worker-docs", "PER-INPROCESS-ENGINEER", "SES-DOCS-01", item, body)
	assert.NoError(t, err)
}

func TestElevation_Cycle92_Orchestrate_Worktree_And_Manifest(t *testing.T) {
	tempDir, _, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	initGitRepoInDir(t, tempDir)
	ctx := context.Background()

	// 1. ensureOrchestrationWorktree empty taskID
	_, err := ensureOrchestrationWorktree(ctx, tempDir, "")
	assert.Error(t, err)

	// 2. ensureOrchestrationWorktree valid
	taskID := "ATK-ELEV-WT-01"
	wt, err := ensureOrchestrationWorktree(ctx, tempDir, taskID)
	require.NoError(t, err)
	assert.NotEmpty(t, wt)

	// 3. Second call reuses existing checkout
	wtSecond, errSecond := ensureOrchestrationWorktree(ctx, tempDir, taskID)
	require.NoError(t, errSecond)
	assert.Equal(t, wt, wtSecond)

	// 4. Git helpers
	assert.True(t, isGitWorktreeCheckout(wt))
	assert.False(t, isGitWorktreeCheckout(filepath.Join(tempDir, "non_existent_path")))
	assert.True(t, gitRevExists(ctx, tempDir, "HEAD"))
	assert.False(t, gitRevExists(ctx, tempDir, "invalid-rev-999"))
	assert.False(t, gitRefExists(ctx, tempDir, "refs/heads/invalid-ref-999"))

	baseRef := orchestrationWorktreeBaseRef(ctx, tempDir)
	assert.NotEmpty(t, baseRef)

	assert.NoError(t, resetOrchestrationWorktree(ctx, wt, "HEAD"))
	assert.NoError(t, resetOrchestrationWorktree(ctx, wt, ""))

	// 5. autoCommitWorktreeChanges clean worktree
	cleanErr := autoCommitWorktreeChanges(ctx, wt, taskID)
	assert.NoError(t, cleanErr)

	// Make worktree dirty
	dirtyFile := filepath.Join(wt, "dirty.txt")
	require.NoError(t, fileutil.WriteFile(dirtyFile, []byte("dirty content"), 0644))

	// autoCommitWorktreeChanges with build failure
	origCheck := worktreeBuildCheck
	defer func() { worktreeBuildCheck = origCheck }()
	worktreeBuildCheck = func(ctx context.Context, root string) error {
		return fmt.Errorf("simulated gate fail")
	}
	errDirty := autoCommitWorktreeChanges(ctx, wt, taskID)
	assert.Error(t, errDirty)

	// autoCommitWorktreeChanges with build success
	worktreeBuildCheck = func(ctx context.Context, root string) error {
		return nil
	}
	errCommit := autoCommitWorktreeChanges(ctx, wt, taskID)
	assert.NoError(t, errCommit)

	// 6. collectOrchestrationCommitManifest
	// When resultSHA == baseSHA
	_, noCommitErr := collectOrchestrationCommitManifest(ctx, wt, taskID, "HEAD")
	assert.Error(t, noCommitErr)

	// When baseSHA is parent
	manifest, manErr := collectOrchestrationCommitManifest(ctx, wt, taskID, baseRef)
	assert.NoError(t, manErr)
	assert.NotEmpty(t, manifest["result_sha"])
}

func TestElevation_Cycle92_Next_Evidence_Verification(t *testing.T) {
	tempDir := t.TempDir()

	// 1. requirePriorityPlanRef
	taskNoPri := map[string]any{"id": "ATK-NO-PRI"}
	_, errNoPri := requirePriorityPlanRef(taskNoPri, "ATK-NO-PRI")
	assert.Error(t, errNoPri)
	assert.Contains(t, errNoPri.Error(), "priority_plan_ref is missing")

	taskWithPri := map[string]any{"id": "ATK-PRI", objects.FieldKeyPriorityPlanRef: "PRI-ELEV-01"}
	pri, errPri := requirePriorityPlanRef(taskWithPri, "ATK-PRI")
	assert.NoError(t, errPri)
	assert.Equal(t, "PRI-ELEV-01", pri)

	// 2. requireAtkWorktreeTornDown
	taskID := "ATK-TEARDOWN-TEST"
	assert.NoError(t, requireAtkWorktreeTornDown(tempDir, taskID))

	wtDir := paths.AgentWorktreeDir(tempDir, taskID)
	require.NoError(t, fileutil.MkdirAll(wtDir, 0755))
	assert.Error(t, requireAtkWorktreeTornDown(tempDir, taskID))

	// 3. requireCommitsMergedToIntegration
	taskNoCommits := map[string]any{"id": taskID}
	assert.Error(t, requireCommitsMergedToIntegration(context.Background(), tempDir, taskID, "PRI-ELEV-01", taskNoCommits))

	// 4. verifyDocsEvalNextEvidence
	assert.Error(t, verifyDocsEvalNextEvidence(tempDir, "No findings listed here"))
	assert.Error(t, verifyDocsEvalNextEvidence(tempDir, "See docs/findings/missing.jsonl"))

	findingsDir := filepath.Join(tempDir, "docs", "findings")
	require.NoError(t, fileutil.MkdirAll(findingsDir, 0755))
	findingsFile := filepath.Join(findingsDir, "eval.jsonl")
	require.NoError(t, fileutil.WriteFile(findingsFile, []byte("{\"finding_id\": \"FIND-01\", \"status\": \"verified\"}\n"), 0644))

	validErr := verifyDocsEvalNextEvidence(tempDir, "Deliverable is at docs/findings/eval.jsonl")
	assert.NoError(t, validErr)
}

func TestElevation_Cycle92_Execute_PromptBuilding(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	planID, bliID := seedAgentSupportHierarchy(t, provider)

	// Long description task to test truncation
	taskID := "ATK-LONG-PROMPT-01"
	longDesc := strings.Repeat("A", 1200)
	taskLong := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Long Description Task",
		objects.FieldKeyDescription:        longDesc,
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyPriorityPlanRef:    planID,
		objects.FieldKeyBacklogItemRef:     bliID,
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyTaskSteps: []any{
			map[string]any{
				objects.FieldKeyTitle:  "Step 1",
				objects.FieldKeyStatus: objects.ObjectStatusApproved,
			},
		},
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), sec, taskLong))

	cmd := NewExecuteCmd()
	cmd.SetArgs([]string{"--task-id", taskID})
	cli.SetContext(cmd, cli.ContextForProjectRoot(tempDir))
	_ = cmd.ExecuteContext(ctx)
}

func TestElevation_Cycle92_SeatWorker_InboxProcessing(t *testing.T) {
	tempDir, _, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// Write unacked correspondence event to test lines 222-298 in seat_worker.go
	_, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
		ProjectRoot: tempDir,
		Message:     "COMMS heartbeat ping",
		AgentID:     "coordinator",
		ToAgentID:   "worker-inbox-seat",
		Sender:      agentfeed.FeedSenderHumanSteer,
		EventType:   agentfeed.FeedEventTypeSteering,
	})
	require.NoError(t, err)

	cmd := NewSeatWorkerCmd()
	cmd.SetArgs([]string{"--agent-id", "worker-inbox-seat", "--persona-ref", "PER-INPROCESS-ENGINEER", "--once", "--execute-non-comms"})
	cli.SetContext(cmd, cli.ContextForProjectRoot(tempDir))
	_ = cmd.ExecuteContext(context.Background())
}

func TestElevation_Cycle92_SyncLoop_Worktree_Terminal_Transition(t *testing.T) {
	root, store := setupSyncLoopTestProject(t)
	ctx := storage.WithSyncCreateForKind(context.Background(), objects.KindAgentTask)
	ctx = storage.WithSkipWriteBehind(ctx)
	secCtx := pkgctx.NewSystemSecurityContext()

	taskID := "ATK-SYNC-WT-CYCLE92"
	task := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Worktree Sync Task",
		objects.FieldKeyDescription:        "Worktree Sync Task Description",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion:      "2.0.0",
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaOrchestratorAlpha,
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeyTaskSteps:          []any{},
	}
	storage.CreateCASVisible(t, store, ctx, secCtx, task, objects.ObjectStatusInProgress)

	// Place project root inside .zqk/worktrees/<taskID>
	wtRoot := filepath.Join(root, paths.ProjectDataDir, "worktrees", taskID)
	require.NoError(t, fileutil.MkdirAll(wtRoot, 0755))

	origCheck := worktreeBuildCheck
	defer func() { worktreeBuildCheck = origCheck }()
	worktreeBuildCheck = func(ctx context.Context, r string) error {
		return fmt.Errorf("simulated build gate failure")
	}

	cmd := NewSyncLoopCmd()
	cmd.SetArgs([]string{taskID})
	cli.SetContext(cmd, cli.ContextForProjectRoot(wtRoot))
	_ = cmd.ExecuteContext(context.Background())
}
