package agent

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/agentidle"
	"github.com/zqk-os/zqk/pkg/agentprompt"
	cli "github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/swarm"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

// -----------------------------------------------------------------------------
// Target 1: agent.go - Processor & Storage Helpers
// -----------------------------------------------------------------------------

func TestCycle137_AgentCmd_StructureAndSubcommands(t *testing.T) {
	cmd := NewAgentCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "agent", cmd.Use)

	// Verify key subcommands exist
	subcommands := []string{
		"orchestrate", "status", "evaluate-run", "synthesize-skill",
		"new", "sync-loop", "execute", "prepare-context", "claim",
		"claim-gate", "release", "recover", "next", "chat-responder",
		"validate", "scoreboard", "seat-worker", "guiding-step", "hook",
	}
	found := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		found[sub.Name()] = true
	}
	for _, expected := range subcommands {
		assert.True(t, found[expected], "expected subcommand %s not found on NewAgentCmd()", expected)
	}

	// procSecurity with nil vs valid processor
	nilSec := procSecurity(nil)
	assert.NotNil(t, nilSec)
	assert.Equal(t, "ACC-SYSTEM", nilSec.AccountID)

	// procStorageTuple with nil
	ctx, sec, sp := procStorageTuple(nil)
	assert.NotNil(t, ctx)
	assert.NotNil(t, sec)
	assert.Nil(t, sp)

	// newAgentProcessor with isolated in-process project
	tempDir, _, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	testCmd := &cobra.Command{Use: "test"}
	cli.SetContext(testCmd, cli.ContextForProjectRoot(tempDir))
	proc, err := newAgentProcessor(testCmd)
	require.NoError(t, err)
	require.NotNil(t, proc)

	// procSecurity and procStorageTuple with valid processor
	validSec := procSecurity(proc)
	assert.NotNil(t, validSec)

	vCtx, vSec, vSp := procStorageTuple(proc)
	assert.NotNil(t, vCtx)
	assert.NotNil(t, vSec)
	assert.NotNil(t, vSp)
}

// -----------------------------------------------------------------------------
// Target 2: guiding_step.go - Guidance Evaluation & Resolution
// -----------------------------------------------------------------------------

func TestCycle137_GuidingStep_ResolutionAndExecution(t *testing.T) {
	cmd := NewGuidingStepCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "guiding-step", cmd.Name())

	// resolveGuidingEvent branches
	res, unmatched := resolveGuidingEvent("custom_event", "")
	assert.Equal(t, "custom_event", res)
	assert.False(t, unmatched)

	res, unmatched = resolveGuidingEvent("", "git commit -m 'test'")
	assert.NotEmpty(t, res)
	assert.False(t, unmatched)

	res, unmatched = resolveGuidingEvent("", "unknown_shell_executable_xyz_123")
	assert.Empty(t, res)
	assert.True(t, unmatched)

	res, unmatched = resolveGuidingEvent("", "")
	assert.Empty(t, res)
	assert.False(t, unmatched)

	// unmatchedGuidingStepPayload
	payload := unmatchedGuidingStepPayload()
	assert.False(t, payload["matched"].(bool))

	// In-process execution of runAgentGuidingStep
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// 1. Missing event and shell
	_, err := executeAgentCommand(t, tempDir, provider, "guiding-step")
	assert.Error(t, err)

	// 2. Unmatched shell command returns unmatched payload
	out, err := executeAgentCommand(t, tempDir, provider, "guiding-step", "--shell", "unknown_shell_xyz_456")
	require.NoError(t, err)
	assert.Contains(t, out, "matched: false")

	// 3. Valid shell classification
	out, err = executeAgentCommand(t, tempDir, provider, "guiding-step", "--shell", "git commit -m 'test'")
	require.NoError(t, err)
	assert.Contains(t, out, "event: git_commit")
}

func TestCycle137_GuidingStep_WithPersonaHydration(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	// Seed persona
	personaID := "PER-GUIDING-CYCLE137"
	personaObj := map[string]any{
		objects.FieldKeyID:            personaID,
		objects.FieldKeyKind:          objects.KindPersona,
		objects.FieldKeyTitle:         "Guiding Persona",
		objects.FieldKeyDescription:   "Guiding Persona Description",
		objects.FieldKeyRole:          "software_engineer",
		objects.FieldKeyStatus:        objects.ObjectStatusApproved,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), sec, personaObj))

	// Run with valid persona ref
	out, err := executeAgentCommand(t, tempDir, provider, "guiding-step", "--event", "git_commit", "--persona-ref", personaID)
	require.NoError(t, err)
	assert.Contains(t, out, "event: git_commit")

	// Run with non-existent persona ref -> returns error
	_, err = executeAgentCommand(t, tempDir, provider, "guiding-step", "--event", "git_commit", "--persona-ref", "PER-NONEXISTENT")
	assert.Error(t, err)
}

// -----------------------------------------------------------------------------
// Target 3: scoreboard.go - Idle Metrics Reporting
// -----------------------------------------------------------------------------

func TestCycle137_Scoreboard_Execution(t *testing.T) {
	cmd := NewScoreboardCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "scoreboard", cmd.Name())

	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// Initialize idle store and record duration
	storePath := datacell.AgentIdleStorePath(tempDir)
	store, err := agentidle.NewFileStore(storePath)
	require.NoError(t, err)

	err = store.Accumulate("agent-craftsman-01", "task-1", 4500*time.Millisecond)
	require.NoError(t, err)

	out, err := executeAgentCommand(t, tempDir, provider, "scoreboard")
	require.NoError(t, err)
	assert.Contains(t, out, "agent-craftsman-01:task-1")
}

// -----------------------------------------------------------------------------
// Target 4: synthesize_skill.go - Pipeline & Context
// -----------------------------------------------------------------------------

func TestCycle137_SynthesizeSkill_StructureAndPipeline(t *testing.T) {
	cmd := NewSynthesizeSkillCmd()
	require.NotNil(t, cmd)
	assert.NotNil(t, cmd.Flags().Lookup("capability"))
	assert.NotNil(t, cmd.Flags().Lookup("provider"))

	// synthesizeSkillCommitContext
	nilCtx := synthesizeSkillCommitContext(nil)
	assert.True(t, pkgctx.GetPromoteOnCreate(nilCtx))

	customCtx := context.WithValue(context.Background(), "test-key", "test-val")
	promotedCtx := synthesizeSkillCommitContext(customCtx)
	assert.True(t, pkgctx.GetPromoteOnCreate(promotedCtx))

	// Run in isolated project
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	out, err := executeAgentCommand(t, tempDir, provider, "synthesize-skill", "--capability", "codebase-indexing", "--provider", "anthropic")
	require.NoError(t, err)
	assert.Contains(t, out, "codebase-indexing")

	// Verify skill file was created on disk
	skillPath := filepath.Join(tempDir, paths.ProjectDataDir, paths.SkillsSubdir, "codebase-indexing", "SKILL.md")
	assert.FileExists(t, skillPath)
}

// -----------------------------------------------------------------------------
// Target 5: loop_guard.go - Knobs, Stagnation & Fingerprinting
// -----------------------------------------------------------------------------

func TestCycle137_LoopGuard_KnobsAndStagnation(t *testing.T) {
	cfg := LoadLoopGuardConfig()
	assert.Greater(t, cfg.MaxSyncLoops, 0)
	assert.Greater(t, cfg.MaxVerificationAttempts, 0)
	assert.Greater(t, cfg.MaxStagnantProgressTicks, 0)

	// stagnationGuard behavior
	guard := newStagnationGuard(2)
	require.NotNil(t, guard)

	// First tick of fingerprint A
	abort := guard.Observe("fp-alpha")
	assert.False(t, abort)

	// Second tick of same fingerprint A
	abort = guard.Observe("fp-alpha")
	assert.False(t, abort)

	// Third tick of same fingerprint A (exceeds max 2) -> abort!
	abort = guard.Observe("fp-alpha")
	assert.True(t, abort)

	// New fingerprint resets tick count
	abort = guard.Observe("fp-beta")
	assert.False(t, abort)

	// nil stagnationGuard
	var nilGuard *stagnationGuard
	assert.False(t, nilGuard.Observe("fp"))
}

func TestCycle137_LoopGuard_VerificationAttemptsAndFingerprint(t *testing.T) {
	// verificationAttemptsOf type switches
	assert.Equal(t, 0, verificationAttemptsOf(nil))
	assert.Equal(t, 0, verificationAttemptsOf(map[string]any{}))
	assert.Equal(t, 3, verificationAttemptsOf(map[string]any{objects.FieldKeyVerificationAttempts: 3}))
	assert.Equal(t, 4, verificationAttemptsOf(map[string]any{objects.FieldKeyVerificationAttempts: int64(4)}))
	assert.Equal(t, 5, verificationAttemptsOf(map[string]any{objects.FieldKeyVerificationAttempts: float64(5)}))
	assert.Equal(t, 6, verificationAttemptsOf(map[string]any{objects.FieldKeyVerificationAttempts: "6"}))
	assert.Equal(t, 0, verificationAttemptsOf(map[string]any{objects.FieldKeyVerificationAttempts: true}))

	// taskProgressFingerprint
	assert.Empty(t, taskProgressFingerprint(nil))

	taskObj := map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		objects.FieldKeyTaskSteps: []any{
			map[string]any{
				objects.FieldKeyName:                 "step-1",
				objects.FieldKeyStatus:               "pending",
				objects.FieldKeyVerificationAttempts: 1,
			},
		},
	}
	fp := taskProgressFingerprint(taskObj)
	assert.Contains(t, fp, "in_progress|step-1:pending:1;")

	// completeness_validation steps fallback
	cvTaskObj := map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusPendingVerification,
		objects.FieldKeyCompletenessValidation: []any{
			map[string]any{
				objects.FieldKeyTitle:                "cv-step",
				objects.FieldKeyStatus:               "done",
				objects.FieldKeyVerificationAttempts: 2,
			},
		},
	}
	fp2 := taskProgressFingerprint(cvTaskObj)
	assert.Contains(t, fp2, "pending_verification|cv-step:done:2;")
}

// -----------------------------------------------------------------------------
// Target 6: message_adapters.go & worktree_gate.go
// -----------------------------------------------------------------------------

func TestCycle137_MessageAdapters_And_WorktreeGate(t *testing.T) {
	// message adapter selection
	adapter := GetDeliveryAdapter("/path/to/transcript.jsonl")
	assert.NotNil(t, adapter)

	// isAgentWorktree check
	assert.False(t, isAgentWorktree("/regular/repo/root"))

	// agentWorktreeMainRepo for regular dir
	_, err := agentWorktreeMainRepo("/tmp/regular/dir")
	assert.Error(t, err)

	// defaultWorktreeBuildCheck
	ctx := context.Background()
	tempDir := t.TempDir()
	err = defaultWorktreeBuildCheck(ctx, tempDir)
	assert.NoError(t, err)
}

// -----------------------------------------------------------------------------
// Target 7: seat_worker_gate.go - Seat Writes Revert & Verify
// -----------------------------------------------------------------------------

func TestCycle137_SeatWorkerGate_RevertSeatWritesAndVerify(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	// 1. Untracked file should be removed
	testFile := "untracked_test_artifact.txt"
	absPath := filepath.Join(tempDir, testFile)
	err := fileutil.WriteStandardFile(absPath, []byte("temp payload"))
	require.NoError(t, err)
	assert.FileExists(t, absPath)

	err = revertSeatWrites(ctx, tempDir, []string{testFile})
	require.NoError(t, err)
	assert.NoFileExists(t, absPath)

	// 2. Process data files skipped
	procFile := filepath.Join(paths.ProcessDir, "test_skip.yaml")
	err = revertSeatWrites(ctx, tempDir, []string{procFile})
	require.NoError(t, err)

	// 3. verifyWorkAsCompletion with empty history
	msg, err := verifyWorkAsCompletion(ctx, tempDir, nil)
	require.NoError(t, err)
	assert.Contains(t, msg, "No mutation tool evidence")

	// 4. verifyWorkAsCompletion with write tool evidence
	history := []swarm.ToolCallRecord{
		{Name: "zqk_write_code"},
	}
	_, _ = verifyWorkAsCompletion(ctx, tempDir, history)
}

// -----------------------------------------------------------------------------
// Target 8: seat_worker_fill.go - Kernel Fill Submissions
// -----------------------------------------------------------------------------

func TestCycle137_SeatWorkerFill_MarksAndSubmit(t *testing.T) {
	tempDir := t.TempDir()

	// schedulerCallbackNotify format
	cb := schedulerCallbackNotify("/usr/local/bin/zqk", "/tmp/log.json")
	assert.Equal(t, "/usr/local/bin/zqk callback notify --stamp-mesh-status --log-file /tmp/log.json", cb)

	// fillSubmitMarkPath
	markPath := fillSubmitMarkPath(tempDir, "goal")
	assert.True(t, strings.HasSuffix(markPath, "fill-goal.json"))

	// writeFillSubmitMark and recentFillSubmit
	err := writeFillSubmitMark(tempDir, "goal", "test-detail")
	require.NoError(t, err)

	recent, detail := recentFillSubmit(tempDir, "goal")
	assert.True(t, recent)
	assert.Contains(t, detail, "already_submitted")

	// submitKernelFill with nil or disabled
	ctx := context.Background()
	res, err := submitKernelFill(ctx, tempDir, nil)
	require.NoError(t, err)
	assert.Empty(t, res)

	res, err = submitKernelFill(ctx, tempDir, &whatsnext.FillItem{AutoSubmit: false})
	require.NoError(t, err)
	assert.Empty(t, res)

	res, err = submitKernelFill(ctx, tempDir, &whatsnext.FillItem{AutoSubmit: true, SubmitArgs: ""})
	require.NoError(t, err)
	assert.Empty(t, res)

	// submitKernelFill with recent mark skips submission
	res, err = submitKernelFill(ctx, tempDir, &whatsnext.FillItem{
		AutoSubmit: true,
		SubmitArgs: "do something",
		Kind:       "goal",
	})
	require.NoError(t, err)
	assert.Contains(t, res, "already_submitted")
}

// -----------------------------------------------------------------------------
// Target 9: evaluate.go - Evaluation Run Telemetry
// -----------------------------------------------------------------------------

func TestCycle137_Evaluate_ExecutionAndTelemetry(t *testing.T) {
	cmd := NewEvaluateCmd()
	require.NotNil(t, cmd)

	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// Missing --session-id
	_, err := executeAgentCommand(t, tempDir, provider, "evaluate-run")
	assert.Error(t, err)

	// Valid evaluate-run
	out, err := executeAgentCommand(t, tempDir, provider, "evaluate-run", "--session-id", "SES-CYCLE137-001")
	require.NoError(t, err)
	assert.Contains(t, out, "Evaluated session SES-CYCLE137-001")

	// Verify audit event in storage
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	res, err := provider.List(ctx, sec, pkgctx.NewStorageContext(), storage.ListFilter{
		Kind: objects.KindAuditEvent,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, res.Objects)
}

// -----------------------------------------------------------------------------
// Target 10: status.go - Active Swarm Workers & Stalled Plans
// -----------------------------------------------------------------------------

func TestCycle137_Status_BranchesAndRendering(t *testing.T) {
	cmd := NewStatusCmd()
	require.NotNil(t, cmd)

	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// 1. Initial status with no workers
	out, err := executeAgentCommand(t, tempDir, provider, "status")
	require.NoError(t, err)
	assert.Contains(t, out, "No active swarm workers")

	// 2. Status with active agent task
	planID, bliID := seedAgentSupportHierarchy(t, provider)
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	taskID := "ATK-STATUS-CYCLE137-001"
	taskObj := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Active Status Task",
		objects.FieldKeyDescription:        "Active Status Task Description",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeyPriorityPlanRef:    planID,
		objects.FieldKeyBacklogItemRef:     bliID,
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
		objects.FieldKeyUpdatedAt:          time.Now().Format(time.RFC3339),
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), sec, taskObj))

	out, err = executeAgentCommand(t, tempDir, provider, "status")
	require.NoError(t, err)
	assert.Contains(t, out, taskID)
	assert.Contains(t, out, "Active Status Task")
}

// -----------------------------------------------------------------------------
// Target 11: validate.go, claim_gate.go & continuation_hook.go
// -----------------------------------------------------------------------------

func TestCycle137_ValidateAndHookBuilders(t *testing.T) {
	valCmd := NewValidateCmd()
	require.NotNil(t, valCmd)
	assert.Equal(t, "validate", valCmd.Name())

	claimGateCmd := NewClaimGateCmd()
	require.NotNil(t, claimGateCmd)
	assert.Equal(t, "claim-gate", claimGateCmd.Name())

	releaseCmd := NewReleaseCmd()
	require.NotNil(t, releaseCmd)
	assert.Equal(t, "release", releaseCmd.Name())

	nextCmd := NewNextCmd()
	require.NotNil(t, nextCmd)
	assert.Equal(t, "next", nextCmd.Name())

	prepCmd := NewPrepareContextCmd()
	require.NotNil(t, prepCmd)
	assert.Equal(t, "prepare-context", prepCmd.Name())
}

// -----------------------------------------------------------------------------
// Target 12: continuation_hook.go - Flywheel Continuation Hook
// -----------------------------------------------------------------------------

func TestCycle137_ContinuationHook_WithDepsAndBranches(t *testing.T) {
	hookCmd := newContinuationHookCmd()
	require.NotNil(t, hookCmd)
	assert.Equal(t, "continuation", hookCmd.Name())

	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	tempDir := t.TempDir()

	// 1. Without active marker -> returns "allow"
	var outBuf bytes.Buffer
	inBuf := strings.NewReader("")
	err := runContinuationHookWithDeps(ctx, sec, nil, tempDir, "antigravity", "", "", "", inBuf, &outBuf)
	require.NoError(t, err)
	assert.Contains(t, outBuf.String(), `"decision": "allow"`)

	// 2. With active marker created on disk
	markerPath := filepath.Join(tempDir, defaultMarkerFilename)
	err = fileutil.WriteStandardFile(markerPath, []byte("active"))
	require.NoError(t, err)

	outBuf.Reset()
	err = runContinuationHookWithDeps(ctx, sec, nil, tempDir, "antigravity", markerPath, "", "", strings.NewReader(""), &outBuf)
	require.NoError(t, err)
	assert.Contains(t, outBuf.String(), "CONTINUOUS AUTONOMOUS PROGRESSION")
}

// -----------------------------------------------------------------------------
// Target 13: prepared_context.go - Clipping, Footers & Semantic Context
// -----------------------------------------------------------------------------

func TestCycle137_PreparedContext_ClippingAndAssembly(t *testing.T) {
	// clipSeatWorkerPromptPart within bounds
	shortText := "short prompt content"
	clipped := clipSeatWorkerPromptPart(shortText, 50, "TEST", agentprompt.WorkClassCoding)
	assert.Equal(t, shortText, clipped)

	// clipSeatWorkerPromptPart exceeding bounds
	longText := strings.Repeat("A", 100)
	clipped = clipSeatWorkerPromptPart(longText, 50, "TEST", agentprompt.WorkClassCoding)
	assert.Contains(t, clipped, "... [TEST TRUNCATED FOR WINDOW]")

	// docs eval footer
	footer := seatWorkerPromptTruncationFooter("DOCS", agentprompt.WorkClassDocsEval)
	assert.Contains(t, footer, "read_file or")

	// HasSemanticContext checks
	pEmpty := PreparedContext{}
	assert.False(t, pEmpty.HasSemanticContext())

	pWithDesc := PreparedContext{
		Task: map[string]any{
			objects.FieldKeyDescription: "Task has semantic description",
		},
	}
	assert.True(t, pWithDesc.HasSemanticContext())

	pWithCriteria := PreparedContext{
		Task: map[string]any{
			objects.FieldKeyAcceptanceCriteria: []any{"criterion 1"},
		},
	}
	assert.True(t, pWithCriteria.HasSemanticContext())

	pWithProblem := PreparedContext{
		Task: map[string]any{
			"problem_statement": "Problem statement present",
		},
	}
	assert.True(t, pWithProblem.HasSemanticContext())

	// AssemblePreparedContext validation errors
	_, err := AssemblePreparedContext(context.Background(), nil, nil, PreparedContextInput{})
	assert.Error(t, err)

	sec := pkgctx.NewSystemSecurityContext()
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// Empty taskID and description
	_, err = AssemblePreparedContext(context.Background(), sec, provider, PreparedContextInput{})
	assert.Error(t, err)

	// Valid assembly with description only
	res, err := AssemblePreparedContext(context.Background(), sec, provider, PreparedContextInput{
		Description: "Standalone task description",
		Depth:       -1,
		ProjectRoot: tempDir,
	})
	require.NoError(t, err)
	assert.Equal(t, "Standalone task description", res.Title)
}

// -----------------------------------------------------------------------------
// Target 14: orchestrate_mesh.go - String IDs, Persona Roles & Skills
// -----------------------------------------------------------------------------

func TestCycle137_OrchestrationMesh_Helpers(t *testing.T) {
	// stringIDsFromAny
	assert.Nil(t, stringIDsFromAny(nil))
	assert.Nil(t, stringIDsFromAny(12345))

	sList := stringIDsFromAny([]string{"id1", " id2 ", ""})
	assert.Equal(t, []string{"id1", "id2"}, sList)

	anyList := stringIDsFromAny([]any{"id1", 123, " id3 "})
	assert.Equal(t, []string{"id1", "id3"}, anyList)

	// itemPersonaID
	assert.Empty(t, itemPersonaID(nil))

	itemWithAssignee := map[string]any{
		objects.FieldKeyAssigneePersonaRef: "PER-ASSIGNEE-01",
	}
	assert.Equal(t, "PER-ASSIGNEE-01", itemPersonaID(itemWithAssignee))

	itemWithRefs := map[string]any{
		objects.FieldKeyPersonaRefs: []string{"PER-REF-01"},
	}
	assert.Equal(t, "PER-REF-01", itemPersonaID(itemWithRefs))

	// orchestrationMeshSkillIDs
	itemWithSkill := map[string]any{
		objects.FieldKeySkillRef: "SKL-ITEM-01",
	}
	skills := orchestrationMeshSkillIDs(itemWithSkill, nil)
	assert.Contains(t, skills, "SKL-ITEM-01")

	// orchestrationPersonaRole fallback
	role := orchestrationPersonaRole(context.Background(), nil, nil, "PER-TEST")
	assert.Equal(t, defaultOrchestrationPersonaRole, role)
}

// -----------------------------------------------------------------------------
// Target 15: orchestrate_steps.go - Role Tokens & Boundaries
// -----------------------------------------------------------------------------

func TestCycle137_OrchestrationSteps_BoundariesAndSteps(t *testing.T) {
	// orchestrationCLI
	cliName := orchestrationCLI()
	assert.NotEmpty(t, cliName)

	// personaRoleFromObject
	assert.Empty(t, personaRoleFromObject(nil))
	assert.Equal(t, "architect", personaRoleFromObject(map[string]any{objects.FieldKeyRole: " architect "}))

	// orchestrationEstimatedEffort
	assert.Empty(t, orchestrationEstimatedEffort(nil))
	assert.Equal(t, "2h", orchestrationEstimatedEffort(map[string]any{objects.FieldKeyEstimatedEffort: " 2h "}))

	// normalizePersonaRoleToken
	assert.Equal(t, "software_engineer", normalizePersonaRoleToken("Software-Engineer", ""))
	assert.Equal(t, "tpm_lead", normalizePersonaRoleToken("", "TPM-Lead"))

	// orchestrationTaskBoundaryFor
	bOperator := orchestrationTaskBoundaryFor("operator", "", agentprompt.WorkClassCoding)
	assert.True(t, bOperator.GraphCompose)
	assert.False(t, bOperator.CodeValidate)

	bArchitect := orchestrationTaskBoundaryFor("system_architect", "", agentprompt.WorkClassCoding)
	assert.True(t, bArchitect.GraphCompose)
	assert.True(t, bArchitect.CodeValidate)

	bEngineer := orchestrationTaskBoundaryFor("software_engineer", "", agentprompt.WorkClassCoding)
	assert.False(t, bEngineer.GraphCompose)
	assert.True(t, bEngineer.CodeValidate)

	bDocs := orchestrationTaskBoundaryFor("unknown_role", "", agentprompt.WorkClassDocsEval)
	assert.True(t, bDocs.GraphCompose)
	assert.False(t, bDocs.CodeValidate)

	// buildOrchestrationTaskSteps
	steps := buildOrchestrationTaskSteps("zqk", orchestrationTaskBoundary{GraphCompose: true, CodeValidate: true})
	assert.Len(t, steps, 3)

	stepsBasic := buildOrchestrationTaskSteps("", orchestrationTaskBoundary{})
	assert.Len(t, stepsBasic, 1)
}

// -----------------------------------------------------------------------------
// Target 16: seat_worker_engine.go - Worker Lanes & Engine ID
// -----------------------------------------------------------------------------

func TestCycle137_SeatWorkerEngine_LanesAndSanitize(t *testing.T) {
	// sanitizeLaneName
	assert.Equal(t, "craftsman_lane", sanitizeLaneName(" Craftsman_Lane "))
	assert.Equal(t, "seat", sanitizeLaneName("!@#$%^&*()"))
	assert.Equal(t, "worker-01", sanitizeLaneName("worker-01"))

	// lookupMapInsensitive
	m := map[string]string{
		"per-craftsman": "craftsman-lane",
		"agent-01":      "lane-01",
	}
	res, found := lookupMapInsensitive(m, "PER-CRAFTSMAN", "")
	assert.True(t, found)
	assert.Equal(t, "craftsman-lane", res)

	res, found = lookupMapInsensitive(m, "", "AGENT-01")
	assert.True(t, found)
	assert.Equal(t, "lane-01", res)

	res, found = lookupMapInsensitive(m, "unknown", "unknown")
	assert.False(t, found)
	assert.Empty(t, res)

	// seatWorkerLane & seatWorkerEngineID
	lane := seatWorkerLane("PER-ORCH-PLANNER", "")
	assert.NotEmpty(t, lane)

	engineID := seatWorkerEngineID("PER-ORCH-PLANNER", "agt-test-1")
	assert.True(t, strings.HasPrefix(engineID, "seat-worker-"))
}

// -----------------------------------------------------------------------------
// Target 17: orchestrate_run.go - TDD Ready & Upstream Resolution
// -----------------------------------------------------------------------------

func TestCycle137_OrchestrateRun_BLIReadyAndUpstream(t *testing.T) {
	// isTestCaseReadyStatus
	assert.True(t, isTestCaseReadyStatus("active"))
	assert.True(t, isTestCaseReadyStatus("draft"))
	assert.True(t, isTestCaseReadyStatus("metrics_captured"))
	assert.True(t, isTestCaseReadyStatus("complete"))
	assert.False(t, isTestCaseReadyStatus("rejected"))
	assert.False(t, isTestCaseReadyStatus(""))

	// extractItemUpstreamIDs
	assert.Nil(t, extractItemUpstreamIDs(nil))

	itemDeps := map[string]any{
		"depends_on":         []string{"BLI-UPSTREAM-01"},
		"upstream_task_refs": []string{"ATK-UPSTREAM-02"},
	}
	upIDs := extractItemUpstreamIDs(itemDeps)
	assert.Contains(t, upIDs, "BLI-UPSTREAM-01")
	assert.Contains(t, upIDs, "ATK-UPSTREAM-02")

	// verifyBLITDDReady edge cases
	assert.False(t, verifyBLITDDReady(context.Background(), nil, nil, nil))
	assert.False(t, verifyBLITDDReady(context.Background(), nil, nil, map[string]any{}))

	// localSkillIDByTitle edge cases
	assert.Empty(t, localSkillIDByTitle(context.Background(), nil, nil, ""))
	assert.Empty(t, localSkillIDByTitle(context.Background(), nil, nil, "any-title"))

	// isBlockedByUnverifiedUpstream with nil sp/item
	assert.False(t, isBlockedByUnverifiedUpstream(context.Background(), nil, nil, nil))

	// resolveVerifiedUpstreamDeliverables with nil
	assert.Empty(t, resolveVerifiedUpstreamDeliverables(context.Background(), nil, nil, nil))
}
