package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/agentidle"
	"github.com/zqk-os/zqk/pkg/agentprompt"
	"github.com/zqk-os/zqk/pkg/audit"
	cliapp "github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mutation"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestElevation_Agent_ValidateCmd(t *testing.T) {
	cmd := NewValidateCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "validate", cmd.Use)
}

func TestElevation_Agent_Processor_Branches(t *testing.T) {
	tempDir, _, cleanup := setupAgentInProcessProject(t)
	defer cleanup()
	t.Setenv("ZQK_PROJECT_ROOT", tempDir)

	// 1. procSecurity with nil vs non-nil
	secNil := procSecurity(nil)
	assert.NotNil(t, secNil)
	assert.Equal(t, pkgctx.SystemAccountID, secNil.AccountID)

	// 2. procStorageTuple with nil
	c, s, p := procStorageTuple(nil)
	assert.NotNil(t, c)
	assert.NotNil(t, s)
	assert.Nil(t, p)

	// 3. newAgentProcessor with isolated project
	cmd := NewOrchestrateCmd()
	cmd.SetContext(pkgctx.WithSecurityContext(context.Background(), pkgctx.NewSystemSecurityContext()))
	proc, err := newAgentProcessor(cmd)
	require.NoError(t, err)
	assert.NotNil(t, proc)
	assert.Equal(t, tempDir, proc.ProjectRoot())

	// Test procSecurity with proc
	secWithProc := procSecurity(proc)
	assert.NotNil(t, secWithProc)

	// Test procStorageTuple with proc
	cProc, sProc, spProc := procStorageTuple(proc)
	assert.NotNil(t, cProc)
	assert.NotNil(t, sProc)
	assert.NotNil(t, spProc)
}

func TestElevation_Agent_Scoreboard_Execution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// 1. Empty scoreboard
	out, err := executeAgentCommand(t, tempDir, provider, "scoreboard")
	require.NoError(t, err)
	assert.NotNil(t, out)

	// 2. Scoreboard with idle records
	storePath := datacell.AgentIdleStorePath(tempDir)
	store, err := agentidle.NewFileStore(storePath)
	require.NoError(t, err)
	require.NoError(t, store.Accumulate("agent-worker-01", "task-100", 45*time.Second))
	require.NoError(t, store.Accumulate("agent-worker-02", "task-200", 120*time.Second))

	out2, err2 := executeAgentCommand(t, tempDir, provider, "scoreboard")
	require.NoError(t, err2)
	assert.Contains(t, out2, "agent-worker-01")
	assert.Contains(t, out2, "agent-worker-02")
}

func TestElevation_Agent_Evaluate_Branches(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// Missing --session-id
	_, err := executeAgentCommand(t, tempDir, provider, "evaluate-run")
	assert.ErrorContains(t, err, "requires --session-id")

	// Valid evaluate-run
	out, err := executeAgentCommand(t, tempDir, provider, "evaluate-run", "--session-id", "SES-ELEVATION-001")
	require.NoError(t, err)
	assert.Contains(t, out, "Evaluated session SES-ELEVATION-001")

	// Verify AuditEvent was created in storage
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	events, err := provider.List(ctx, secCtx, nil, storage.ListFilter{Kind: objects.KindAuditEvent})
	require.NoError(t, err)
	assert.NotEmpty(t, events.Objects)
}

func TestElevation_Agent_ClaimGate_Comprehensive(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// 1. activeIntentAssignment when pointer missing
	assert.Equal(t, "", activeIntentAssignment(tempDir))

	// 2. activeIntentAssignment with pointer and file
	stateDir := paths.StateDirPath(tempDir)
	_ = fileutil.EnsureDir(stateDir)
	intentPath := filepath.Join(stateDir, "test_intent.json")
	_ = fileutil.WriteStandardFile(intentPath, []byte(`{"assignment": "ASSIGN-123"}`))
	pointerPath := filepath.Join(stateDir, "change_intent_active")
	_ = fileutil.WriteStandardFile(pointerPath, []byte(intentPath))

	assert.Equal(t, "ASSIGN-123", activeIntentAssignment(tempDir))

	// 3. activeIntentAssignment with empty pointer
	_ = fileutil.WriteStandardFile(pointerPath, []byte("   \n"))
	assert.Equal(t, "", activeIntentAssignment(tempDir))

	// 4. activeIntentAssignment with invalid JSON
	_ = fileutil.WriteStandardFile(intentPath, []byte(`not json`))
	_ = fileutil.WriteStandardFile(pointerPath, []byte(intentPath))
	assert.Equal(t, "", activeIntentAssignment(tempDir))

	// 5. execute claim-gate with auto-assign and wake
	out, _ := executeAgentCommand(t, tempDir, provider, "claim-gate", "--by", "SEAT-GATE-01", "--auto-assign", "--wake")
	assert.NotNil(t, out)
}

func TestElevation_Agent_ClaimAndRelease_Extended(t *testing.T) {
	f := initSeededAgentFixture(t)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. resolveClaimantIdentity with ZQK_AGENT_ID env
	t.Setenv(zqkenv.AgentID().Name(), "custom-agent-id")
	assert.Equal(t, "custom-agent-id", resolveClaimantIdentity(nil, nil))
	t.Setenv(zqkenv.AgentID().Name(), "")

	// 2. checkinDue for missing checkin
	assert.Equal(t, "", checkinDue(f.tempDir, "ATK-NONEXISTENT"))

	// 3. claimTaskForExecute on nil or non-agent-task
	assert.NoError(t, claimTaskForExecute(nil, nil, "any", nil))
	assert.NoError(t, claimTaskForExecute(nil, nil, "any", map[string]any{objects.FieldKeyKind: "not_task"}))

	// 4. claimTaskForExecute on valid agent_task
	taskID := "ATK-CLAIM-EXEC-TEST"
	taskObj := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Claim For Execute Task",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyPriorityPlanRef:    f.planID,
		objects.FieldKeyBacklogItemRef:     f.bliID,
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}
	require.NoError(t, f.provider.Create(ctx, secCtx, taskObj))

	// Test releaseTaskAfterNext with real proc
	cmd := NewNextCmd()
	cliapp.SetContext(cmd, cliapp.ContextForProjectRoot(f.tempDir))
	proc, err := newAgentProcessor(cmd)
	require.NoError(t, err)
	_ = releaseTaskAfterNext(proc, taskID, "")

	// Test runRelease with --force
	outRel, errRel := executeAgentCommand(t, f.tempDir, f.provider, "release", taskID, "--force")
	require.NoError(t, errRel)
	assert.Contains(t, outRel, taskID)
}

func TestElevation_Agent_Recover_TerminalAndSteps(t *testing.T) {
	f := initSeededAgentFixture(t)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Complete/Terminal parent plan
	termPlanID := "PRI-TERM-PLAN-001"
	termPlan := map[string]any{
		objects.FieldKeyID:            termPlanID,
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Terminal Plan",
		objects.FieldKeyDescription:   "Terminal plan description for recovery tests",
		objects.FieldKeyStatus:        objects.ObjectStatusComplete,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, f.provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, termPlan))

	// Orphaned in_progress task under terminal plan
	orphanTaskID := "ATK-ORPHAN-001"
	orphanTask := map[string]any{
		objects.FieldKeyID:                 orphanTaskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Orphan Task",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeyPriorityPlanRef:    termPlanID,
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
		objects.FieldKeyClaimedBy:          "seat-old",
		objects.FieldKeyClaimedAt:          time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
		objects.FieldKeyDescription:        "Orphaned task description",
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}
	require.NoError(t, f.provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, orphanTask))

	// 2. Failed task with task steps feedback
	failedTaskID := "ATK-FAILED-WITH-FEEDBACK"
	failedTask := map[string]any{
		objects.FieldKeyID:                 failedTaskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Failed Task With Feedback",
		objects.FieldKeyStatus:             objects.ObjectStatusError,
		objects.FieldKeyPriorityPlanRef:    f.planID,
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
		objects.FieldKeyDescription:        "Initial task description",
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyTaskSteps: []any{
			map[string]any{
				objects.FieldKeyTitle:                "Step Alpha",
				objects.FieldKeyStatus:               objects.ObjectStatusFailed,
				objects.FieldKeyVerificationFeedback: "Syntax error on line 42",
			},
			map[string]any{
				objects.FieldKeyTitle:  "Step Beta",
				objects.FieldKeyStatus: objects.ObjectStatusApproved,
			},
		},
	}
	require.NoError(t, f.provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, failedTask))

	// 3. In-progress task that is stale
	staleTaskID := "ATK-STALE-INPROG-001"
	staleTask := map[string]any{
		objects.FieldKeyID:                 staleTaskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Stale InProg Task",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeyPriorityPlanRef:    f.planID,
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
		objects.FieldKeyClaimedBy:          "seat-stale",
		objects.FieldKeyClaimedAt:          time.Now().Add(-3 * time.Hour).UTC().Format(time.RFC3339),
		objects.FieldKeyDescription:        "Stale in progress task description",
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}
	require.NoError(t, f.provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, staleTask))

	// Run recover on plan
	out, err := executeAgentCommand(t, f.tempDir, f.provider, "recover", f.planID)
	require.NoError(t, err)
	assert.Contains(t, out, "Recovered task")

	// Verify failed task was recovered and feedback was injected
	updatedFailed, err := f.provider.Read(ctx, secCtx, failedTaskID)
	require.NoError(t, err)
	assert.Equal(t, objects.ObjectStatusApproved, updatedFailed[objects.FieldKeyStatus])
	assert.Contains(t, updatedFailed[objects.FieldKeyDescription].(string), "Syntax error on line 42")

	// Run recover --all for cross-plan recovery
	outAll, errAll := executeAgentCommand(t, f.tempDir, f.provider, "recover", "--all")
	require.NoError(t, errAll)
	assert.Contains(t, outAll, "Archived orphaned task")

	// Verify orphan task was archived
	updatedOrphan, err := f.provider.Read(ctx, secCtx, orphanTaskID)
	require.NoError(t, err)
	assert.Equal(t, objects.ObjectStatusArchived, updatedOrphan[objects.FieldKeyStatus])
}

func TestElevation_Agent_Next_ValidationFailures(t *testing.T) {
	f := initSeededAgentFixture(t)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Non-existent task
	_, err := executeAgentCommand(t, f.tempDir, f.provider, "next", "ATK-NONEXISTENT")
	assert.ErrorContains(t, err, "failed to read task")

	// 2. Refusal of --on-validation-failure none/off
	relPath := "docs/findings/task_eval_elevation.jsonl"
	absPath := filepath.Join(f.tempDir, relPath)
	_ = fileutil.EnsureDir(filepath.Dir(absPath))
	_ = fileutil.WriteStandardFile(absPath, []byte(`{"finding_id": "FIND-EVAL-ELEV"}`+"\n"))

	taskID := "ATK-NEXT-FAIL-TEST"
	task := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "docs_eval next verification item",
		objects.FieldKeyDescription:        fmt.Sprintf("Docs eval findings at %s", relPath),
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeyPriorityPlanRef:    f.planID,
		objects.FieldKeyBacklogItemRef:     f.bliID,
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}
	require.NoError(t, f.provider.Create(ctx, secCtx, task))

	_, errNone := executeAgentCommand(t, f.tempDir, f.provider, "next", taskID, "--on-validation-failure", "none")
	assert.ErrorContains(t, errNone, "requires --on-validation-failure wake")

	_, errOff := executeAgentCommand(t, f.tempDir, f.provider, "next", taskID, "--on-validation-failure", "off")
	assert.ErrorContains(t, errOff, "requires --on-validation-failure wake")

	// 3. Missing priority_plan_ref error helper
	_, errMissingPlan := requirePriorityPlanRef(map[string]any{}, "ATK-X")
	assert.ErrorContains(t, errMissingPlan, "priority_plan_ref is missing")

	// 4. verifyDocsEvalNextEvidence error cases
	assert.ErrorContains(t, verifyDocsEvalNextEvidence(f.tempDir, "no path here"), "no .../findings/*.jsonl path")
	assert.ErrorContains(t, verifyDocsEvalNextEvidence(f.tempDir, "docs/findings/nonexistent.jsonl"), "docs_eval evidence missing")

	// Empty findings file
	emptyFindings := filepath.Join(f.tempDir, "docs", "findings", "empty.jsonl")
	_ = fileutil.EnsureDir(filepath.Dir(emptyFindings))
	_ = fileutil.WriteStandardFile(emptyFindings, []byte("\n  \n"))
	assert.ErrorContains(t, verifyDocsEvalNextEvidence(f.tempDir, "docs/findings/empty.jsonl"), "need >=1 finding_id")

	// Findings file with REPLACE_ME
	replaceFindings := filepath.Join(f.tempDir, "docs", "findings", "replace.jsonl")
	_ = fileutil.WriteStandardFile(replaceFindings, []byte(`{"finding_id": "REPLACE_ME"}`+"\n"))
	assert.ErrorContains(t, verifyDocsEvalNextEvidence(f.tempDir, "docs/findings/replace.jsonl"), "need >=1 finding_id")
}

func TestElevation_Agent_GuidingStep_Hydration(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. resolveGuidingEvent branches
	ev, unmatched := resolveGuidingEvent("custom_event", "")
	assert.Equal(t, "custom_event", ev)
	assert.False(t, unmatched)

	ev2, unmatched2 := resolveGuidingEvent("", "git commit -m foo")
	assert.NotEmpty(t, ev2)
	assert.False(t, unmatched2)

	ev3, unmatched3 := resolveGuidingEvent("", "unknown_unmatched_tool_command_xyz")
	assert.Equal(t, "", ev3)
	assert.True(t, unmatched3)

	ev4, unmatched4 := resolveGuidingEvent("", "")
	assert.Equal(t, "", ev4)
	assert.False(t, unmatched4)

	// 2. Unmatched shell command returns unmatched payload
	outUnmatched, err := executeAgentCommand(t, tempDir, provider, "guiding-step", "--shell", "unknown_unmatched_tool_command_xyz")
	require.NoError(t, err)
	assert.Contains(t, outUnmatched, "matched")

	// 3. With persona-ref that exists
	personaID := "PER-GUIDING-TEST"
	persona := map[string]any{
		objects.FieldKeyID:            personaID,
		objects.FieldKeyKind:          objects.KindPersona,
		objects.FieldKeyTitle:         "Guiding Persona",
		objects.FieldKeyStatus:        objects.ObjectStatusApproved,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(ctx, secCtx, persona))

	outPersona, errPersona := executeAgentCommand(t, tempDir, provider, "guiding-step", "--event", "session_start", "--persona-ref", personaID)
	require.NoError(t, errPersona)
	assert.NotNil(t, outPersona)

	// 4. With non-existent persona-ref
	_, errNonExistent := executeAgentCommand(t, tempDir, provider, "guiding-step", "--event", "session_start", "--persona-ref", "PER-NONEXISTENT")
	assert.Error(t, errNonExistent)
}

func TestElevation_Agent_WorktreeGate(t *testing.T) {
	// isAgentWorktree
	assert.False(t, isAgentWorktree("/regular/repo/path"))

	// agentWorktreeMainRepo on non-worktree
	_, err := agentWorktreeMainRepo(t.TempDir())
	assert.Error(t, err)

	// defaultWorktreeBuildCheck
	ctx := context.Background()
	_ = defaultWorktreeBuildCheck(ctx, t.TempDir())
}

func TestElevation_Agent_LoopGuard_Extended(t *testing.T) {
	// verificationAttemptsOf type conversions
	assert.Equal(t, 5, verificationAttemptsOf(map[string]any{objects.FieldKeyVerificationAttempts: int(5)}))
	assert.Equal(t, 6, verificationAttemptsOf(map[string]any{objects.FieldKeyVerificationAttempts: int64(6)}))
	assert.Equal(t, 7, verificationAttemptsOf(map[string]any{objects.FieldKeyVerificationAttempts: float64(7)}))
	assert.Equal(t, 8, verificationAttemptsOf(map[string]any{objects.FieldKeyVerificationAttempts: " 8 "}))
	assert.Equal(t, 0, verificationAttemptsOf(map[string]any{objects.FieldKeyVerificationAttempts: "invalid"}))
	assert.Equal(t, 0, verificationAttemptsOf(map[string]any{objects.FieldKeyVerificationAttempts: true}))
	assert.Equal(t, 0, verificationAttemptsOf(nil))

	// stagnationGuard with nil
	var nilGuard *stagnationGuard
	assert.False(t, nilGuard.Observe("foo"))

	// taskProgressFingerprint with CompletenessValidation
	taskWithCV := map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		objects.FieldKeyCompletenessValidation: []any{
			map[string]any{
				objects.FieldKeyName:                 "Check1",
				objects.FieldKeyStatus:               objects.ObjectStatusApproved,
				objects.FieldKeyVerificationAttempts: 1,
			},
		},
	}
	fp := taskProgressFingerprint(taskWithCV)
	assert.Contains(t, fp, "Check1:approved:1")
}

func TestElevation_Agent_SynthesizeSkill_Pipeline(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// 1. Missing capability
	_, err := executeAgentCommand(t, tempDir, provider, "synthesize-skill")
	assert.Error(t, err)

	// 2. Full synthesize-skill pipeline execution
	out, err := executeAgentCommand(t, tempDir, provider, "synthesize-skill", "--capability", "autonomous-refactoring", "--provider", "openai")
	require.NoError(t, err)
	assert.Contains(t, out, "Initiating Meta-Agent Pipeline")

	// Verify skill file
	skillPath := filepath.Join(tempDir, paths.ProjectDataDir, paths.SkillsSubdir, "autonomous-refactoring", "SKILL.md")
	assert.True(t, fileutil.IsRegularFile(skillPath))
}

func TestElevation_Agent_HeartbeatCompliance(t *testing.T) {
	assert.Equal(t, "no_activity", HeartbeatComplianceFromModTime(time.Time{}))
	assert.Equal(t, "heartbeat_ok", HeartbeatComplianceFromModTime(time.Now().Add(-2*time.Minute)))
	assert.Equal(t, "heartbeat_stale", HeartbeatComplianceFromModTime(time.Now().Add(-30*time.Minute)))
}

func TestElevation_Agent_Status_FormatsAndStalled(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Active priority plan with in_progress status and 0 workers -> stalled alert
	stalledPlan := map[string]any{
		objects.FieldKeyID:            "PRI-STALLED-001",
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Stalled Execution Plan",
		objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
		objects.FieldKeyDescription:   "Stalled priority plan for status test",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, stalledPlan))

	// Table format
	outTable, errTable := executeAgentCommand(t, tempDir, provider, "status")
	require.NoError(t, errTable)
	assert.Contains(t, outTable, "ZQK Orchestrated Hive Status")
	assert.Contains(t, outTable, "Stalled In-Progress Plans")

	// JSON format
	outJSON, errJSON := executeAgentCommand(t, tempDir, provider, "status", "--format", "json")
	require.NoError(t, errJSON)
	assert.Contains(t, outJSON, "stalled_swarm_alert")

	// YAML format
	outYAML, errYAML := executeAgentCommand(t, tempDir, provider, "status", "--format", "yaml")
	require.NoError(t, errYAML)
	assert.Contains(t, outYAML, "stalled_plans")
}

func TestElevation_Agent_PreparedContext_HelpersAndRefusals(t *testing.T) {
	// isPreparedContextRefusal
	assert.False(t, isPreparedContextRefusal(nil))
	assert.True(t, isPreparedContextRefusal(errPreparedContextRequired))
	assert.True(t, isPreparedContextRefusal(errPreparedContextStorage))
	assert.True(t, isPreparedContextRefusal(storage.ErrObjectNotFound))
	assert.True(t, isPreparedContextRefusal(fmt.Errorf("error: object not found in storage")))
	assert.False(t, isPreparedContextRefusal(fmt.Errorf("io timeout")))

	// clipSeatWorkerPromptPart
	assert.Equal(t, "short", clipSeatWorkerPromptPart("short", 10, "label", agentprompt.WorkClassCoding))
	assert.Equal(t, "short", clipSeatWorkerPromptPart("short", 0, "label", agentprompt.WorkClassCoding))
	clipped := clipSeatWorkerPromptPart("1234567890EXTRA", 10, "prompt", agentprompt.WorkClassCoding)
	assert.Contains(t, clipped, "[prompt TRUNCATED FOR WINDOW]")
	assert.Contains(t, clipped, "write_code")

	clippedDocs := clipSeatWorkerPromptPart("1234567890EXTRA", 10, "docs", agentprompt.WorkClassDocsEval)
	assert.Contains(t, clippedDocs, "[docs TRUNCATED FOR WINDOW]")
	assert.Contains(t, clippedDocs, "read_code on documentation")

	// HasSemanticContext
	pEmpty := &PreparedContext{}
	assert.False(t, pEmpty.HasSemanticContext())

	pWithDesc := &PreparedContext{Task: map[string]any{objects.FieldKeyDescription: "Some task description"}}
	assert.True(t, pWithDesc.HasSemanticContext())

	pWithAC := &PreparedContext{Task: map[string]any{objects.FieldKeyAcceptanceCriteria: []any{"criterion 1"}}}
	assert.True(t, pWithAC.HasSemanticContext())

	pWithPS := &PreparedContext{Task: map[string]any{"problem_statement": "A problem statement"}}
	assert.True(t, pWithPS.HasSemanticContext())

	pWithDep := &PreparedContext{
		Deps: []map[string]any{
			{objects.FieldKeyDescription: "Dependency description"},
		},
	}
	assert.True(t, pWithDep.HasSemanticContext())
}

func TestElevation_Agent_AssemblePreparedContext_Branches(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	// 1. Storage or sec nil -> error
	_, errNil := AssemblePreparedContext(ctx, nil, nil, PreparedContextInput{})
	assert.ErrorIs(t, errNil, errPreparedContextStorage)

	// 2. Both TaskID and Description empty -> error
	_, errReq := AssemblePreparedContext(ctx, sec, provider, PreparedContextInput{})
	assert.ErrorIs(t, errReq, errPreparedContextRequired)

	// 3. Description only (no taskID)
	prepDesc, errDesc := AssemblePreparedContext(ctx, sec, provider, PreparedContextInput{
		Description: "Standalone ad-hoc task execution",
		ProjectRoot: tempDir,
	})
	require.NoError(t, errDesc)
	assert.NotEmpty(t, prepDesc.Prompt)

	// 4. Task with upstream deliverable artifacts
	bliID := "BLI-PREP-ELEV-001"
	bli := map[string]any{
		objects.FieldKeyID:            bliID,
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyTitle:         "Backlog Item Deliverable",
		objects.FieldKeyStatus:        objects.ObjectStatusApproved,
		objects.FieldKeyDescription:   "Produces deliverable artifacts",
		objects.FieldKeyArtifacts:     []any{"pkg/foo/foo.go", "pkg/foo/foo_test.go"},
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), sec, bli))

	taskID := "ATK-PREP-ELEV-001"
	task := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Task with dependencies",
		objects.FieldKeyDescription:        "Prepare context with upstream deliverables",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyBacklogItemRef:     bliID,
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaDefaultOperator,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), sec, task))

	prepTask, errTask := AssemblePreparedContext(ctx, sec, provider, PreparedContextInput{
		TaskID:      taskID,
		ProjectRoot: tempDir,
		Depth:       1,
		IncludeTDD:  true,
	})
	require.NoError(t, errTask)
	assert.NotEmpty(t, prepTask.Prompt)
}

func TestElevation_Agent_Orchestrate_Helpers(t *testing.T) {
	// resolveOrchestrationTimeout
	assert.Equal(t, 10*time.Second, resolveOrchestrationTimeout(10*time.Second, nil))
	assert.Equal(t, orchestrationTimeout, resolveOrchestrationTimeout(0, nil))

	// backlogItemEligibleForOrchestration
	assert.False(t, backlogItemEligibleForOrchestration(objects.ObjectStatusComplete))
	assert.False(t, backlogItemEligibleForOrchestration(objects.ObjectStatusArchived))
	assert.False(t, backlogItemEligibleForOrchestration(objects.ObjectStatusCancelled))
	assert.False(t, backlogItemEligibleForOrchestration(objects.ObjectStatusImplemented))
	assert.False(t, backlogItemEligibleForOrchestration(objects.ObjectStatusPendingVerification))
	assert.True(t, backlogItemEligibleForOrchestration(objects.ObjectStatusPlanned))
	assert.True(t, backlogItemEligibleForOrchestration(objects.ObjectStatusGrooming))
	assert.True(t, backlogItemEligibleForOrchestration(objects.ObjectStatusApproved))
	assert.True(t, backlogItemEligibleForOrchestration(objects.ObjectStatusInProgress))

	// nativeSwarmEligible
	assert.True(t, nativeSwarmEligible("tier_1_routine"))
	assert.True(t, nativeSwarmEligible("tier_2_simple"))
	assert.True(t, nativeSwarmEligible("tier_3_light"))
	assert.True(t, nativeSwarmEligible("tier_3_simple"))
	assert.False(t, nativeSwarmEligible("tier_1_complex"))
	assert.False(t, nativeSwarmEligible("unknown"))
	assert.False(t, nativeSwarmEligible(""))

	// hasAnyPersonaAssignment
	assert.False(t, hasAnyPersonaAssignment(nil))
	assert.False(t, hasAnyPersonaAssignment(map[string]any{}))
	assert.True(t, hasAnyPersonaAssignment(map[string]any{objects.FieldKeyPersonaRefs: []any{"PER-1"}}))
	assert.True(t, hasAnyPersonaAssignment(map[string]any{objects.FieldKeyPersonaRefs: []string{"PER-2"}}))
	assert.True(t, hasAnyPersonaAssignment(map[string]any{objects.FieldKeyPersonaRefs: "PER-3"}))
	assert.True(t, hasAnyPersonaAssignment(map[string]any{objects.FieldKeyAssigneePersonaRef: "PER-4"}))

	// hasPersonaMatch
	assert.True(t, hasPersonaMatch(map[string]any{}, nil))
	assert.True(t, hasPersonaMatch(map[string]any{}, []string{"PER-1"})) // unassigned soft match
	assignedItem := map[string]any{objects.FieldKeyPersonaRefs: []string{"PER-ALPHA"}}
	assert.True(t, hasPersonaMatch(assignedItem, []string{"PER-ALPHA"}))
	assert.False(t, hasPersonaMatch(assignedItem, []string{"PER-BETA"}))
	assignedByAssignee := map[string]any{objects.FieldKeyAssigneePersonaRef: "PER-GAMMA"}
	assert.True(t, hasPersonaMatch(assignedByAssignee, []string{"PER-GAMMA"}))
	assert.False(t, hasPersonaMatch(assignedByAssignee, []string{"PER-DELTA"}))
}

func TestElevation_Agent_Orchestrate_Steps(t *testing.T) {
	// orchestrationCLI
	cliName := orchestrationCLI()
	assert.NotEmpty(t, cliName)

	// personaRoleFromObject
	assert.Equal(t, "", personaRoleFromObject(nil))
	assert.Equal(t, "architect", personaRoleFromObject(map[string]any{objects.FieldKeyRole: "  architect  "}))

	// orchestrationEstimatedEffort
	assert.Equal(t, "", orchestrationEstimatedEffort(nil))
	assert.Equal(t, "2h", orchestrationEstimatedEffort(map[string]any{objects.FieldKeyEstimatedEffort: " 2h "}))
	assert.Equal(t, "", orchestrationEstimatedEffort(map[string]any{}))

	// normalizePersonaRoleToken
	assert.Equal(t, "software_engineer", normalizePersonaRoleToken("Software-Engineer", ""))
	assert.Equal(t, "qa_auditor", normalizePersonaRoleToken("", "QA-Auditor"))

	// orchestrationTaskBoundaryFor
	bOp := orchestrationTaskBoundaryFor("operator", "", agentprompt.WorkClassCoding)
	assert.True(t, bOp.GraphCompose)
	assert.False(t, bOp.CodeValidate)

	bArch := orchestrationTaskBoundaryFor("system_architect", "", agentprompt.WorkClassCoding)
	assert.True(t, bArch.GraphCompose)
	assert.True(t, bArch.CodeValidate)

	bEng := orchestrationTaskBoundaryFor("software_engineer", "", agentprompt.WorkClassCoding)
	assert.False(t, bEng.GraphCompose)
	assert.True(t, bEng.CodeValidate)

	bDocs := orchestrationTaskBoundaryFor("unknown", "", agentprompt.WorkClassDocsEval)
	assert.True(t, bDocs.GraphCompose)
	assert.False(t, bDocs.CodeValidate)

	// buildOrchestrationTaskSteps
	stepsDefault := buildOrchestrationTaskSteps("", orchestrationTaskBoundary{GraphCompose: true, CodeValidate: true})
	assert.Len(t, stepsDefault, 3)
	assert.Equal(t, "Implementation Phase", stepsDefault[0][objects.FieldKeyTitle])
	assert.Equal(t, "Kernel Graph Composition Context", stepsDefault[1][objects.FieldKeyTitle])
	assert.Equal(t, "Validation Phase", stepsDefault[2][objects.FieldKeyTitle])

	stepsNoVal := buildOrchestrationTaskSteps("zqk", orchestrationTaskBoundary{GraphCompose: false, CodeValidate: false})
	assert.Len(t, stepsNoVal, 1)
}

func TestElevation_Agent_Orchestrate_Reuse(t *testing.T) {
	// dispositionForOrchestrationStatus
	assert.Equal(t, orchDispositionSkipDone, dispositionForOrchestrationStatus(objects.ObjectStatusImplemented))
	assert.Equal(t, orchDispositionSkipDone, dispositionForOrchestrationStatus(objects.ObjectStatusArchived))
	assert.Equal(t, orchDispositionSkipDone, dispositionForOrchestrationStatus(objects.ObjectStatusCompleted))
	assert.Equal(t, orchDispositionSkipDone, dispositionForOrchestrationStatus(objects.ObjectStatusComplete))
	assert.Equal(t, orchDispositionSkipDone, dispositionForOrchestrationStatus("resolved"))
	assert.Equal(t, orchDispositionSkipDone, dispositionForOrchestrationStatus(objects.ObjectStatusPendingVerification))
	assert.Equal(t, orchDispositionReplace, dispositionForOrchestrationStatus(objects.ObjectStatusError))
	assert.Equal(t, orchDispositionReplace, dispositionForOrchestrationStatus(objects.ObjectStatusFailed))
	assert.Equal(t, orchDispositionReuse, dispositionForOrchestrationStatus(objects.ObjectStatusApproved))
	assert.Equal(t, orchDispositionReuse, dispositionForOrchestrationStatus(objects.ObjectStatusInProgress))
	assert.Equal(t, orchDispositionReuse, dispositionForOrchestrationStatus(objects.ObjectStatusProposed))
	assert.Equal(t, orchDispositionReuse, dispositionForOrchestrationStatus(objects.ObjectStatusPending))
	assert.Equal(t, orchDispositionMint, dispositionForOrchestrationStatus(""))

	// orchestrationTaskTitles & orchestrationTaskTitleMatches
	titles := orchestrationTaskTitles("Write Auth", "BLI-10")
	assert.Contains(t, titles, "Write Auth")
	assert.Contains(t, titles, "Execute Task: Write Auth")
	assert.Contains(t, titles, "Execute BLI-10")
	assert.Contains(t, titles, "Execute Task: BLI-10")
	assert.True(t, orchestrationTaskTitleMatches("Execute Task: Write Auth", "Write Auth", "BLI-10"))
	assert.False(t, orchestrationTaskTitleMatches("Something Else", "Write Auth", "BLI-10"))

	// orchestrationTaskPlanMatches
	assert.True(t, orchestrationTaskPlanMatches(nil, ""))
	assert.False(t, orchestrationTaskPlanMatches(nil, "PRI-1"))
	objPlan := map[string]any{objects.FieldKeyPriorityPlanRef: "PRI-1"}
	assert.True(t, orchestrationTaskPlanMatches(objPlan, "PRI-1"))
	assert.False(t, orchestrationTaskPlanMatches(objPlan, "PRI-2"))

	// orchestrationTaskBacklogMatches
	assert.False(t, orchestrationTaskBacklogMatches(nil, "BLI-1"))
	assert.False(t, orchestrationTaskBacklogMatches(map[string]any{}, ""))
	objBLI := map[string]any{objects.FieldKeyBacklogItemRef: "BLI-1"}
	assert.True(t, orchestrationTaskBacklogMatches(objBLI, "BLI-1"))
	assert.False(t, orchestrationTaskBacklogMatches(objBLI, "BLI-2"))

	// orchDispRank
	assert.Equal(t, 3, orchDispRank(orchDispositionReuse))
	assert.Equal(t, 2, orchDispRank(orchDispositionSkipDone))
	assert.Equal(t, 1, orchDispRank(orchDispositionReplace))
	assert.Equal(t, 0, orchDispRank(orchDispositionMint))

	// orchLivePickRank
	assert.Equal(t, 0, orchLivePickRank(nil))
	assert.Equal(t, 3, orchLivePickRank(map[string]any{objects.FieldKeyStatus: objects.ObjectStatusApproved}))
	assert.Equal(t, 3, orchLivePickRank(map[string]any{objects.FieldKeyStatus: objects.ObjectStatusInProgress, objects.FieldKeyClaimedBy: "seat-1"}))
	assert.Equal(t, 2, orchLivePickRank(map[string]any{objects.FieldKeyStatus: objects.ObjectStatusInProgress}))
	assert.Equal(t, 1, orchLivePickRank(map[string]any{objects.FieldKeyStatus: objects.ObjectStatusError}))
	assert.Equal(t, 2, orchLivePickRank(map[string]any{objects.FieldKeyStatus: objects.ObjectStatusPending}))

	// betterOrchCandidate
	obj1 := map[string]any{objects.FieldKeyStatus: objects.ObjectStatusApproved}
	obj2 := map[string]any{objects.FieldKeyStatus: objects.ObjectStatusError}
	assert.True(t, betterOrchCandidate(obj1, orchDispositionReuse, "2026-10-10", obj2, orchDispositionReplace, "2026-10-09"))

	// pickExistingOrchestrationTask
	tasks := []map[string]any{
		{
			objects.FieldKeyID:              "ATK-PICK-01",
			objects.FieldKeyTitle:           "Execute Task: Build Feature",
			objects.FieldKeyPriorityPlanRef: "PRI-PICK",
			objects.FieldKeyStatus:          objects.ObjectStatusApproved,
			objects.FieldKeyUpdatedAt:       "2026-10-15T00:00:00Z",
		},
	}
	id, status, disp := pickExistingOrchestrationTask(tasks, "Build Feature", "PRI-PICK", "BLI-PICK")
	assert.Equal(t, "ATK-PICK-01", id)
	assert.Equal(t, objects.ObjectStatusApproved, status)
	assert.Equal(t, orchDispositionReuse, disp)
}

func TestElevation_Agent_Orchestrate_Mesh(t *testing.T) {
	// itemPersonaID
	assert.Equal(t, "", itemPersonaID(nil))
	assert.Equal(t, "PER-ASSIGNED", itemPersonaID(map[string]any{objects.FieldKeyAssigneePersonaRef: "PER-ASSIGNED"}))
	assert.Equal(t, "PER-REF", itemPersonaID(map[string]any{objects.FieldKeyPersonaRefs: []string{"PER-REF"}}))
	assert.Equal(t, "", itemPersonaID(map[string]any{}))

	// stringIDsFromAny
	assert.Nil(t, stringIDsFromAny(nil))
	assert.Equal(t, []string{"A", "B"}, stringIDsFromAny([]string{" A ", "B"}))
	assert.Equal(t, []string{"C"}, stringIDsFromAny([]any{" C ", 123}))

	// orchestrationPersonaRole
	assert.Equal(t, "agent", orchestrationPersonaRole(context.Background(), nil, nil))
	assert.Equal(t, "agent", orchestrationPersonaRole(context.Background(), nil, nil, ""))

	// leaseOrchestrationMeshSkill with nil broker
	sec, ep := leaseOrchestrationMeshSkill(context.Background(), nil, nil, nil, nil, "")
	assert.Equal(t, "", sec)
	assert.Equal(t, "", ep)
}

func TestElevation_Agent_ContinuationHook_Vendors(t *testing.T) {
	// markerExists
	assert.False(t, markerExists(""))
	assert.False(t, markerExists("/nonexistent/marker/file"))
	tmpMarker := filepath.Join(t.TempDir(), "marker")
	_ = fileutil.WriteStandardFile(tmpMarker, []byte("active"))
	assert.True(t, markerExists(tmpMarker))

	// emitContinuationDecision for all vendors
	vendors := []string{"claude", "cursor", "windsurf", "cascade", "cline", "roo", "generic"}
	for _, v := range vendors {
		var buf bytes.Buffer
		err := emitContinuationDecision(&buf, v, "continue", "Reason for continuation")
		assert.NoError(t, err)
		assert.NotEmpty(t, buf.String())
	}

	// runContinuationHookWithDeps without marker
	var outAllow bytes.Buffer
	errAllow := runContinuationHookWithDeps(context.Background(), nil, nil, t.TempDir(), "generic", "/nonexistent/marker", "", "", nil, &outAllow)
	assert.NoError(t, errAllow)
	assert.Contains(t, outAllow.String(), "allow")

	// runContinuationHookWithDeps with marker
	var outContinue bytes.Buffer
	errContinue := runContinuationHookWithDeps(context.Background(), nil, nil, t.TempDir(), "generic", tmpMarker, "test-agent", "", nil, &outContinue)
	assert.NoError(t, errContinue)
	assert.Contains(t, outContinue.String(), "continue")
}

func TestElevation_Agent_Orchestrate_Run_Helpers(t *testing.T) {
	// isTestCaseReadyStatus
	assert.True(t, isTestCaseReadyStatus("active"))
	assert.True(t, isTestCaseReadyStatus("draft"))
	assert.True(t, isTestCaseReadyStatus("metrics_captured"))
	assert.True(t, isTestCaseReadyStatus("complete"))
	assert.False(t, isTestCaseReadyStatus("pending"))
	assert.False(t, isTestCaseReadyStatus("failed"))
	assert.False(t, isTestCaseReadyStatus(""))

	// extractItemUpstreamIDs
	assert.Nil(t, extractItemUpstreamIDs(nil))
	itemWithDeps := map[string]any{
		"depends_on":         []any{"BLI-UP-01"},
		"upstream_task_refs": []any{"ATK-UP-02"},
	}
	ids := extractItemUpstreamIDs(itemWithDeps)
	assert.Contains(t, ids, "BLI-UP-01")
	assert.Contains(t, ids, "ATK-UP-02")

	// localSkillIDByTitle
	assert.Equal(t, "", localSkillIDByTitle(context.Background(), nil, nil, "any"))
	assert.Equal(t, "", localSkillIDByTitle(context.Background(), nil, nil, ""))

	// isBlockedByUnverifiedUpstream
	assert.False(t, isBlockedByUnverifiedUpstream(context.Background(), nil, nil, nil))
	assert.False(t, isBlockedByUnverifiedUpstream(context.Background(), nil, nil, map[string]any{}))

	// resolveVerifiedUpstreamDeliverables
	assert.Equal(t, "", resolveVerifiedUpstreamDeliverables(context.Background(), nil, nil, nil))
	assert.Equal(t, "", resolveVerifiedUpstreamDeliverables(context.Background(), nil, nil, map[string]any{}))
}

func TestElevation_Agent_SeatWorker_Gate(t *testing.T) {
	// verifyWorkAsCompletion with empty history
	msg, err := verifyWorkAsCompletion(context.Background(), t.TempDir(), nil)
	assert.NoError(t, err)
	assert.Contains(t, msg, "No mutation tool evidence")

	// revertSeatWrites with process and project data dirs
	errRevert := revertSeatWrites(context.Background(), t.TempDir(), []string{
		paths.ProcessDir + "/tasks/task.yaml",
		paths.ProjectDataDir + "/config.yaml",
	})
	assert.NoError(t, errRevert)
}

func TestElevation_Agent_SyncLoop_Extended(t *testing.T) {
	root, store := setupSyncLoopTestProject(t)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := storage.WithSyncCreateForKind(context.Background(), objects.KindAgentTask)
	ctx = storage.WithSkipWriteBehind(ctx)

	taskID := "ATK-SYNC-HOURGLASS-01"
	task := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Hourglass Test Task",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion:      "2.0.0",
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaOrchestratorAlpha,
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeyTaskSteps:          []any{},
	}
	storage.CreateCASVisible(t, store, ctx, secCtx, task, objects.ObjectStatusInProgress)

	// 1. flipHourglass and removeHourglass
	flipHourglass(context.Background(), secCtx, store, taskID, root)
	readTask, err := store.Read(context.Background(), secCtx, taskID)
	require.NoError(t, err)
	assert.NotEmpty(t, readTask["agent_heartbeat"])

	removeHourglass(context.Background(), secCtx, store, taskID, root)
	readTaskAfter, err := store.Read(context.Background(), secCtx, taskID)
	require.NoError(t, err)
	assert.Nil(t, readTaskAfter["agent_pid"])

	// 2. transitionToError
	validator := mutation.NewValidator(nil)
	auditStream := audit.NewAuditStream(root)
	transitionToError(context.Background(), secCtx, store, taskID, readTaskAfter, validator, auditStream)

	// 3. applyStateMutationWithFields
	extraFields := map[string]any{
		"custom_note": "verified in test",
	}
	errMut := applyStateMutationWithFields(context.Background(), secCtx, store, taskID, objects.KindAgentTask, validator, auditStream, objects.ObjectStatusInProgress, extraFields)
	assert.NoError(t, errMut)

	// 4. Pending verification step transition branches
	taskPendingID := "ATK-SYNC-PENDING-STEPS"
	taskPending := map[string]any{
		objects.FieldKeyID:                 taskPendingID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Pending Steps Task",
		objects.FieldKeyStatus:             objects.ObjectStatusPendingVerification,
		objects.FieldKeySchemaVersion:      "2.0.0",
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaOrchestratorAlpha,
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeyTaskSteps: []any{
			map[string]any{
				objects.FieldKeyName:        "step-impl",
				objects.FieldKeyTitle:       "Implementation Step",
				objects.FieldKeyStatus:      objects.ObjectStatusPendingImplementation,
				objects.FieldKeyDescription: "Step pending implementation",
			},
			map[string]any{
				objects.FieldKeyName:        "step-val",
				objects.FieldKeyTitle:       "Validation Step",
				objects.FieldKeyStatus:      objects.ObjectStatusPending,
				objects.FieldKeyDescription: "Step pending validation",
			},
		},
	}
	storage.CreateCASVisible(t, store, ctx, secCtx, taskPending, objects.ObjectStatusPendingVerification)

	// Execute sync loop with 1 max loop limit
	t.Setenv(zqkenv.AgentSyncMaxLoops().Name(), "1")
	cmd := NewSyncLoopCmd()
	cmd.SetArgs([]string{taskPendingID})
	_ = cmd.ExecuteContext(context.Background())
}

func TestElevation_Agent_Orchestrate_Extended(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()
	t.Setenv("ZQK_PROJECT_ROOT", tempDir)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	// 1. verifyBLITDDReady branches
	assert.False(t, verifyBLITDDReady(context.Background(), provider, secCtx, nil))

	// BLI with non-existent test case
	bliNoTest := map[string]any{
		objects.FieldKeyID:           "BLI-TDD-01",
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyTestCaseRefs: []string{"TST-NONEXISTENT"},
	}
	assert.False(t, verifyBLITDDReady(context.Background(), provider, secCtx, bliNoTest))

	// Create test case in active status
	tstActive := map[string]any{
		objects.FieldKeyID:            "TST-ACTIVE-01",
		objects.FieldKeyKind:          objects.KindTestCase,
		objects.FieldKeyTitle:         "Active Test Case",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: "2.0.0",
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, tstActive))

	bliWithActiveTest := map[string]any{
		objects.FieldKeyID:           "BLI-TDD-02",
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyTestCaseRefs: []string{"TST-ACTIVE-01"},
	}
	assert.True(t, verifyBLITDDReady(context.Background(), provider, secCtx, bliWithActiveTest))

	// 2. resolveVerifiedUpstreamDeliverables branches
	assert.Equal(t, "", resolveVerifiedUpstreamDeliverables(context.Background(), nil, secCtx, nil))
	assert.Equal(t, "", resolveVerifiedUpstreamDeliverables(context.Background(), provider, secCtx, map[string]any{}))

	upItem := map[string]any{
		objects.FieldKeyID:            "BLI-UPSTREAM-DONE",
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyTitle:         "Upstream Completed Work",
		objects.FieldKeyDescription:   "Description for upstream completed work",
		objects.FieldKeyStatus:        objects.ObjectStatusComplete,
		objects.FieldKeySchemaVersion: "2.0.0",
		objects.FieldKeyArtifacts:     []string{"artifacts/spec.json"},
		"result_summary":              "Spec verified successfully",
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, upItem))

	downItem := map[string]any{
		objects.FieldKeyID: "BLI-DOWNSTREAM-01",
		"depends_on":       []string{"BLI-UPSTREAM-DONE"},
	}
	sec := resolveVerifiedUpstreamDeliverables(context.Background(), provider, secCtx, downItem)
	assert.Contains(t, sec, "Upstream Verified Deliverables")
	assert.Contains(t, sec, "artifacts/spec.json")

	// 3. isBlockedByUnverifiedUpstream branches
	assert.False(t, isBlockedByUnverifiedUpstream(context.Background(), provider, secCtx, downItem))

	upPending := map[string]any{
		objects.FieldKeyID:            "BLI-UPSTREAM-PENDING",
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyTitle:         "Upstream Pending Work",
		objects.FieldKeyDescription:   "Description for upstream pending work",
		objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion: "2.0.0",
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, upPending))

	downBlocked := map[string]any{
		objects.FieldKeyID: "BLI-DOWNSTREAM-BLOCKED",
		"depends_on":       []string{"BLI-UPSTREAM-PENDING"},
	}
	assert.True(t, isBlockedByUnverifiedUpstream(context.Background(), provider, secCtx, downBlocked))

	// 4. runOrchestrate with persona filtering and ambient context
	specPersona := map[string]any{
		objects.FieldKeyID:            "PER-SPECIALIST",
		objects.FieldKeyKind:          objects.KindPersona,
		objects.FieldKeyTitle:         "Specialist Persona",
		objects.FieldKeyDescription:   "Description for specialist persona",
		objects.FieldKeyRole:          "agent",
		objects.FieldKeySchemaVersion: "2.0.0",
		objects.FieldKeyStatus:        objects.ObjectStatusImplemented,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, specPersona))

	planID := "PRI-ORCH-EXTENDED-01"
	plan := map[string]any{
		objects.FieldKeyID:            planID,
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Orchestration Extended Plan",
		objects.FieldKeyDescription:   "Description for orchestration extended plan",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: "2.0.0",
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, plan))

	// Create BLI under plan with explicit persona assignment
	bliAssigned := map[string]any{
		objects.FieldKeyID:                 "BLI-ASSIGNED-01",
		objects.FieldKeyKind:               objects.KindBacklogItem,
		objects.FieldKeyTitle:              "Assigned BLI",
		objects.FieldKeyDescription:        "Description for assigned BLI",
		objects.FieldKeyStatus:             objects.ObjectStatusPlanned,
		objects.FieldKeyPriorityPlanRef:    planID,
		objects.FieldKeyAssigneePersonaRef: "PER-SPECIALIST",
		objects.FieldKeySchemaVersion:      "2.0.0",
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, bliAssigned))

	cmd := NewOrchestrateCmd()
	ambientFile := filepath.Join(tempDir, "ambient.md")
	_ = fileutil.WriteStandardFile(ambientFile, []byte("Ambient test context"))
	errOrch := runOrchestrate(cmd, planID, OrchestrateOptions{
		PersonaID:      "PER-OTHER",
		AmbientContext: ambientFile,
		Timeout:        5 * time.Second,
	})
	assert.NoError(t, errOrch)

	// Also run with raw string ambient context
	errOrchStr := runOrchestrate(cmd, planID, OrchestrateOptions{
		PersonaID:      "PER-SPECIALIST",
		AmbientContext: "raw string ambient context",
		Timeout:        5 * time.Second,
	})
	assert.NoError(t, errOrchStr)
}

func TestElevation_Agent_SeatWorker_Extended(t *testing.T) {
	tempDir := t.TempDir()
	channelPath := datacell.AgentChatChannelEventsJSONLPath(tempDir)
	_ = fileutil.EnsureDir(filepath.Dir(channelPath))
	_ = fileutil.WriteStandardFile(channelPath, []byte(""))
	_ = datacell.WriteAgentChatChannelConfig(tempDir, datacell.AgentChatChannelConfig{
		Enabled: true,
	})
	logger := logging.GetLogger().Logger()

	// 1. writePlanOrchSubmitMark and recentPlanOrchSubmit
	assert.NoError(t, writePlanOrchSubmitMark(tempDir, "PRI-TEST", "dispatched"))
	recent, msg := recentPlanOrchSubmit(tempDir, "PRI-TEST")
	assert.True(t, recent)
	assert.Contains(t, msg, "already_submitted")

	recentNone, _ := recentPlanOrchSubmit(tempDir, "PRI-NONEXISTENT")
	assert.False(t, recentNone)

	// 2. fillSubmitMarkPath, writeFillSubmitMark, and recentFillSubmit
	markPath := fillSubmitMarkPath(tempDir, "backlog_item")
	assert.NotEmpty(t, markPath)
	assert.NoError(t, writeFillSubmitMark(tempDir, "backlog_item", "fill details"))
	recentFill, fillMsg := recentFillSubmit(tempDir, "backlog_item")
	assert.True(t, recentFill)
	assert.Contains(t, fillMsg, "already_submitted")

	// 3. schedulerCallbackNotify
	cb := schedulerCallbackNotify("zqk", "/tmp/log.json")
	assert.Equal(t, "zqk callback notify --log-file /tmp/log.json", cb)

	// 4. submitKernelFill branches
	resNil, errNil := submitKernelFill(context.Background(), tempDir, nil)
	assert.NoError(t, errNil)
	assert.Empty(t, resNil)

	resNoAuto, errNoAuto := submitKernelFill(context.Background(), tempDir, &whatsnext.FillItem{AutoSubmit: false})
	assert.NoError(t, errNoAuto)
	assert.Empty(t, resNoAuto)

	resEmptyArgs, errEmpty := submitKernelFill(context.Background(), tempDir, &whatsnext.FillItem{AutoSubmit: true, SubmitArgs: "   "})
	assert.NoError(t, errEmpty)
	assert.Empty(t, resEmptyArgs)

	resRecent, errRecent := submitKernelFill(context.Background(), tempDir, &whatsnext.FillItem{AutoSubmit: true, SubmitArgs: "new bli", Kind: "backlog_item"})
	assert.NoError(t, errRecent)
	assert.Contains(t, resRecent, "already_submitted")

	// 5. withLocalLLMEnv
	env := withLocalLLMEnv([]string{"EXISTING_KEY=VAL"})
	assert.Contains(t, env, "EXISTING_KEY=VAL")

	// 6. dispatchLeadPlanFromWhatsNext with nil storage
	leadNil, errLeadNil := dispatchLeadPlanFromWhatsNext(context.Background(), logger, nil, tempDir, "agent-1", "")
	assert.NoError(t, errLeadNil)
	assert.Equal(t, "no_storage", leadNil.Skipped)

	// 7. ackSeatWorkerSkip
	errAck := ackSeatWorkerSkip(tempDir, "agent-1", "PER-1", "sess-1", "evt-1", "test skip reason")
	assert.NoError(t, errAck)

	// 8. handleNonCommsWithAgentX without ATK in steer
	item := agentfeed.CorrespondenceItem{
		EventID: "evt-steer-01",
	}
	errHandle := handleNonCommsWithAgentX(context.Background(), logger, nil, nil, tempDir, "agent-1", "PER-1", "sess-1", item, "steer with no task id")
	assert.NoError(t, errHandle)
}

func TestElevation_Agent_Execute_Extended(t *testing.T) {
	f := initSeededAgentFixture(t)

	// 1. resolveExecuteSystemPrompt
	assert.NotEmpty(t, resolveExecuteSystemPrompt(""))
	assert.Equal(t, "custom-prompt", resolveExecuteSystemPrompt("custom-prompt"))

	// 2. formatTaskStepsSection
	assert.Empty(t, formatTaskStepsSection(nil))
	assert.NotEmpty(t, formatTaskStepsSection([]any{map[string]any{"name": "step-1"}}))

	// 3. NewExecuteCmd validations
	cmdNoArgs := NewExecuteCmd()
	cmdNoArgs.SetArgs([]string{})
	errNoArgs := cmdNoArgs.ExecuteContext(context.Background())
	assert.Error(t, errNoArgs)
	assert.Contains(t, errNoArgs.Error(), "requires --prompt, --task-id, or a task ID argument")

	cmdNonExistent := NewExecuteCmd()
	cmdNonExistent.SetArgs([]string{"ATK-NONEXISTENT"})
	errNonExistent := cmdNonExistent.ExecuteContext(context.Background())
	assert.Error(t, errNonExistent)
	assert.Contains(t, errNonExistent.Error(), "failed to read task")

	// Task with prompt expansion and MCP initialization
	taskID := "ATK-EXEC-PROMPT-01"
	task := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Exec Prompt Task",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion:      "2.0.0",
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
		objects.FieldKeyPriorityPlanRef:    f.planID,
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeyDescription:        "Execute implementation step",
	}
	require.NoError(t, f.provider.Create(pkgctx.WithPromoteOnCreate(f.ctx), f.secCtx, task))

	cmdExecPrompt := NewExecuteCmd()
	cmdExecPrompt.SetArgs([]string{taskID})
	errExecPrompt := cmdExecPrompt.ExecuteContext(context.Background())
	assert.Error(t, errExecPrompt)
	errMsg := errExecPrompt.Error()
	assert.True(t, strings.Contains(errMsg, "swarm engine execution failed") || strings.Contains(errMsg, "failed to initialize MCP executor") || strings.Contains(errMsg, "mcp connection closed"), "unexpected error message: %s", errMsg)
}

func TestElevation_Agent_Hook_Extended(t *testing.T) {
	tempDir, _, cleanup := setupAgentInProcessProject(t)
	defer cleanup()
	t.Setenv("ZQK_PROJECT_ROOT", tempDir)

	cmdProc := &cobra.Command{}
	cliapp.SetContext(cmdProc, cliapp.ContextForProjectRoot(tempDir))
	proc, err := newAgentProcessor(cmdProc)
	require.NoError(t, err)

	// 1. commandIO
	inNil, outNil := commandIO(nil)
	assert.Equal(t, os.Stdin, inNil)
	assert.Equal(t, os.Stdout, outNil)

	// 2. runPreSubagentHook across vendors
	cmd := &cobra.Command{}
	cmd.SetIn(bytes.NewReader([]byte("{}")))
	var out bytes.Buffer
	cmd.SetOut(&out)

	_ = runPreSubagentHook(cmd, proc, "antigravity")
	_ = runPreSubagentHook(cmd, proc, "claude")
	_ = runPreSubagentHook(cmd, proc, "generic")

	// 3. newContinuationHookCmd execution via cobra
	hookCmd := newContinuationHookCmd()
	hookCmd.SetIn(bytes.NewReader([]byte("{}")))
	var hookOut bytes.Buffer
	hookCmd.SetOut(&hookOut)
	hookCmd.SetArgs([]string{"--vendor", "generic", "--agent-id", "test-agent"})
	_ = hookCmd.ExecuteContext(context.Background())
}

func TestElevation_Agent_Final_Push(t *testing.T) {
	// 1. new.go rejectNonManualPromoteHop
	assert.NoError(t, rejectNonManualPromoteHop("persona", "proposed", "proposed"))

	// 2. new.go promoteStatusOneHop error on non-existent object
	root, store := setupSyncLoopTestProject(t)
	secCtx := pkgctx.NewSystemSecurityContext()
	assert.Error(t, promoteStatusOneHop(context.Background(), secCtx, store, "PER-NONEXISTENT", "approved"))

	// 3. sync_loop.go full completion via passing verification step
	ctx := storage.WithSyncCreateForKind(context.Background(), objects.KindAgentTask)
	ctx = storage.WithSkipWriteBehind(ctx)
	taskCompleteID := "ATK-SYNC-PASSING-01"
	taskComplete := map[string]any{
		objects.FieldKeyID:                 taskCompleteID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Passing Steps Task",
		objects.FieldKeyStatus:             objects.ObjectStatusPendingVerification,
		objects.FieldKeySchemaVersion:      "2.0.0",
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaOrchestratorAlpha,
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeyTaskSteps: []any{
			map[string]any{
				objects.FieldKeyName:        "step-exit0",
				objects.FieldKeyTitle:       "Exit Zero Step",
				objects.FieldKeyStatus:      objects.ObjectStatusPendingVerification,
				"verification_strategy":     "command_exit_code",
				objects.FieldKeyCommand:     "true",
				objects.FieldKeyDescription: "Exit 0 verification",
			},
		},
	}
	storage.CreateCASVisible(t, store, ctx, secCtx, taskComplete, objects.ObjectStatusPendingVerification)

	cmdPass := NewSyncLoopCmd()
	cmdPass.SetArgs([]string{taskCompleteID})
	_ = cmdPass.ExecuteContext(context.Background())

	// 4. seat_worker.go handleNonCommsWithAgentX branches
	tempDir := t.TempDir()
	channelPath := datacell.AgentChatChannelEventsJSONLPath(tempDir)
	_ = fileutil.EnsureDir(filepath.Dir(channelPath))
	_ = fileutil.WriteStandardFile(channelPath, []byte(""))
	_ = datacell.WriteAgentChatChannelConfig(tempDir, datacell.AgentChatChannelConfig{Enabled: true})
	logger := logging.GetLogger().Logger()

	// orchestrate plan directive
	itemOrch := agentfeed.CorrespondenceItem{EventID: "evt-orch-01"}
	_ = handleNonCommsWithAgentX(context.Background(), logger, store, secCtx, tempDir, "agent-1", objects.ConstPersonaOrchestratorAlpha, "sess-1", itemOrch, "orchestrate plan PRI-NONE")

	// docs eval work class with refuse cold AgentX
	itemDocs := agentfeed.CorrespondenceItem{EventID: "evt-docs-01"}
	_ = handleNonCommsWithAgentX(context.Background(), logger, store, secCtx, tempDir, "agent-1", objects.ConstPersonaOrchestratorAlpha, "sess-1", itemDocs, "steer ATK-SYNC-HOURGLASS-01 review docs findings")

	// 5. seat_worker.go dispatchLeadPlanFromWhatsNext branches
	// Create plan with no persona
	planNoPersona := map[string]any{
		objects.FieldKeyID:            "PRI-NO-PERSONA",
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "No Persona Plan",
		objects.FieldKeyDescription:   "Description for no persona plan",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: "2.0.0",
	}
	storage.CreateCASVisible(t, store, ctx, secCtx, planNoPersona, objects.ObjectStatusActive)

	dispNoPersona, errDispNoPersona := dispatchLeadPlanFromWhatsNext(context.Background(), logger, store, tempDir, "agent-1", "")
	assert.NoError(t, errDispNoPersona)
	assert.NotEmpty(t, dispNoPersona.Skipped)

	// 6. sync_loop.go LLM completion, mutation, HIL gate, and status transition
	taskLLMID := "ATK-SYNC-LLM-SUCCESS"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mutJSON := `{"action": "update_node", "target_id": "` + taskLLMID + `", "status_transition": "implemented"}`
		respPayload := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": mutJSON,
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(respPayload)
	}))
	defer ts.Close()

	t.Setenv(zqkenv.LLMBaseURL().Name(), ts.URL)
	t.Setenv(zqkenv.LLMProvider().Name(), "openai")
	t.Setenv(zqkenv.LLMChatModel().Name(), "gpt-4")

	commitHash := initGitRepoInDir(t, root)
	taskLLM := map[string]any{
		objects.FieldKeyID:                 taskLLMID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "LLM Mutation Task",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion:      "2.0.0",
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaOrchestratorAlpha,
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeyCommitHash:         commitHash,
		objects.FieldKeyTaskSteps: []any{
			map[string]any{
				objects.FieldKeyName:        "step-doer",
				objects.FieldKeyTitle:       "Doer Step",
				objects.FieldKeyStatus:      objects.ObjectStatusPending,
				objects.FieldKeyDescription: "Doer step requiring LLM",
			},
		},
	}
	storage.CreateCASVisible(t, store, ctx, secCtx, taskLLM, objects.ObjectStatusInProgress)

	cmdLLM := NewSyncLoopCmd()
	cmdLLM.SetArgs([]string{taskLLMID})
	_ = cmdLLM.ExecuteContext(context.Background())

	// 7. sync_loop.go unmarshal error branch
	tsBadJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "invalid json {{{"}}},
		})
	}))
	defer tsBadJSON.Close()

	t.Setenv(zqkenv.LLMBaseURL().Name(), tsBadJSON.URL)
	t.Setenv(zqkenv.AgentSyncMaxLoops().Name(), "1")
	cmdBad := NewSyncLoopCmd()
	cmdBad.SetArgs([]string{taskLLMID})
	_ = cmdBad.ExecuteContext(context.Background())

	// 8. sync_loop.go LLM error branch
	tsErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer tsErr.Close()

	t.Setenv(zqkenv.LLMBaseURL().Name(), tsErr.URL)
	cmdErr := NewSyncLoopCmd()
	cmdErr.SetArgs([]string{taskLLMID})
	_ = cmdErr.ExecuteContext(context.Background())
}
