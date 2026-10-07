package agent

import (
	stdctx "context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/audit"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/federation/meshbroker"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mutation"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestInProcess_Agent_Evaluate_Execution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// Missing --session-id
	if _, err := executeAgentCommand(t, tempDir, provider, "evaluate-run"); err == nil {
		t.Error("expected error when evaluate-run run without --session-id")
	}

	// Valid evaluate-run
	out, err := executeAgentCommand(t, tempDir, provider, "evaluate-run", "--session-id", "SES-EVAL-001")
	if err != nil {
		t.Fatalf("evaluate-run failed: %v, out: %s", err, out)
	}
	if !strings.Contains(out, "Evaluated session SES-EVAL-001") {
		t.Errorf("expected evaluation confirmation, got: %s", out)
	}
}

func TestInProcess_Agent_PreSubagentHook_CLI(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	out, err := executeAgentCommand(t, tempDir, provider, "hook", "pre-subagent")
	if err != nil {
		t.Fatalf("hook pre-subagent failed: %v, out: %s", err, out)
	}
	if !strings.Contains(out, "allow") {
		t.Errorf("expected allow decision, got: %s", out)
	}
}

func TestInProcess_Agent_ClaimTaskForExecute_Direct(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	planID, bliID := seedAgentSupportHierarchy(t, provider)
	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	taskID := "ATK-CLAIM-EXEC-001"
	taskObj := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Claim For Exec Task",
		objects.FieldKeyDescription:        "Execution prompt",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyBacklogItemRef:     bliID,
		objects.FieldKeyPriorityPlanRef:    planID,
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
	}
	if err := provider.Create(ctx, secCtx, taskObj); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	cmd := &cobra.Command{}
	setAgentCLIContext(t, cmd, tempDir, provider)
	proc, err := newAgentProcessor(cmd)
	if err != nil {
		t.Fatalf("failed to create agent processor: %v", err)
	}

	// 1. Nil task
	if err := claimTaskForExecute(cmd, proc, taskID, nil); err != nil {
		t.Errorf("expected nil error for nil task, got: %v", err)
	}

	// 2. Non-ATK task
	nonAtk := map[string]any{objects.FieldKeyKind: objects.KindBacklogItem}
	if err := claimTaskForExecute(cmd, proc, taskID, nonAtk); err != nil {
		t.Errorf("expected nil error for non-ATK task, got: %v", err)
	}

	// 3. Valid ATK
	if err := claimTaskForExecute(cmd, proc, taskID, taskObj); err != nil {
		t.Fatalf("failed to claim task for execute: %v", err)
	}
}

func TestInProcess_Agent_OrchestrateMesh_LeaseSkill(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Nil broker
	sec, end := leaseOrchestrationMeshSkill(ctx, provider, secCtx, nil, nil, "")
	if sec != "" || end != "" {
		t.Errorf("expected empty returns for nil broker, got: %q, %q", sec, end)
	}

	// 2. Non-nil broker with item references
	broker := meshbroker.NewSkillBroker(provider, nil, tempDir)
	item := map[string]any{
		objects.FieldKeySkillRef: "SKL-MESH-001",
	}
	sec, end = leaseOrchestrationMeshSkill(ctx, provider, secCtx, broker, item, "PER-INPROCESS-ENGINEER")
	if sec != "" || end != "" {
		t.Logf("leaseOrchestrationMeshSkill result: %q, %q", sec, end)
	}
}

func TestInProcess_Agent_OrchestrateReuse_GitHelpers(t *testing.T) {
	tempDir, _, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := stdctx.Background()
	_ = execwrap.CommandContext(ctx, "git", "init", tempDir).Run()
	_ = execwrap.CommandContext(ctx, "git", "-C", tempDir, "config", "user.email", "test@test.com").Run()
	_ = execwrap.CommandContext(ctx, "git", "-C", tempDir, "config", "user.name", "Test").Run()
	_ = execwrap.CommandContext(ctx, "git", "-C", tempDir, "commit", "--allow-empty", "-m", "init").Run()
	_ = execwrap.CommandContext(ctx, "git", "-C", tempDir, "branch", "-M", "main").Run()

	if !gitRevExists(ctx, tempDir, "HEAD") {
		t.Error("expected HEAD to exist")
	}
	if gitRevExists(ctx, tempDir, "nonexistent-rev-12345") {
		t.Error("expected nonexistent rev to not exist")
	}

	_ = gitRefExists(ctx, tempDir, "refs/heads/main")
	if gitRefExists(ctx, tempDir, "refs/heads/nonexistent-branch-12345") {
		t.Error("expected nonexistent ref to not exist")
	}
}

func TestInProcess_Agent_SyncLoop_TransitionToError(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	planID, bliID := seedAgentSupportHierarchy(t, provider)
	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	taskID := "ATK-SYNC-ERR-001"
	taskObj := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Failing Task",
		objects.FieldKeyDescription:        "Task to transition to error",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeyBacklogItemRef:     bliID,
		objects.FieldKeyPriorityPlanRef:    planID,
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
	}
	if err := provider.Create(ctx, secCtx, taskObj); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	validator := mutation.NewValidator(nil)
	auditStream := audit.NewAuditStream(tempDir)
	transitionToError(ctx, secCtx, provider, taskID, taskObj, validator, auditStream)
}

func TestInProcess_Agent_SeatWorker_DirectHelpers(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := stdctx.Background()
	logger := logging.GetLoggerFromProfile("")

	// 1. seatWorkerMCPServePath
	p := seatWorkerMCPServePath(tempDir)
	if p == "" {
		t.Error("expected non-empty MCP serve path")
	}

	// 2. dispatchLeadPlanFromWhatsNext with nil storage
	dNil, err := dispatchLeadPlanFromWhatsNext(ctx, logger, nil, tempDir, "SEAT-TEST-001", "")
	if err != nil {
		t.Fatalf("dispatchLeadPlanFromWhatsNext nil failed: %v", err)
	}
	if dNil.Skipped != "no_storage" {
		t.Errorf("expected skipped: no_storage, got: %s", dNil.Skipped)
	}

	// 3. dispatchLeadPlanFromWhatsNext with storage
	dStorage, err := dispatchLeadPlanFromWhatsNext(ctx, logger, provider, tempDir, "SEAT-TEST-001", "")
	if err != nil {
		t.Fatalf("dispatchLeadPlanFromWhatsNext storage failed: %v", err)
	}
	t.Logf("dispatch result: event_id=%s, skipped=%s", dStorage.EventID, dStorage.Skipped)

	// 4. extractATKIDFromSteer & ackSeatWorkerSkip
	if id := extractATKIDFromSteer("Work on ATK-1791345-abcdef12 right now"); id != "ATK-1791345-abcdef12" {
		t.Errorf("expected ATK ID extracted, got: %s", id)
	}
	if err := ackSeatWorkerSkip(tempDir, "SEAT-1", "PER-1", "SES-1", "EVT-1", "testing skip"); err != nil {
		t.Fatalf("ackSeatWorkerSkip failed: %v", err)
	}

	// 5. firstNonEmptyEnv, withEnvValue, writeSeatWorkerResult
	if v := firstNonEmptyEnv("", "  ", "target", "other"); v != "target" {
		t.Errorf("expected target, got: %s", v)
	}
	if v := firstNonEmptyEnv("", "   "); v != "" {
		t.Errorf("expected empty string, got: %s", v)
	}
	env := withEnvValue([]string{"A=1", "B=2"}, "B", "3")
	if len(env) != 2 || env[1] != "B=3" {
		t.Errorf("expected B=3, got: %v", env)
	}
	envNew := withEnvValue([]string{"A=1"}, "C", "4")
	if len(envNew) != 2 || envNew[1] != "C=4" {
		t.Errorf("expected C=4, got: %v", envNew)
	}
	envEmpty := withEnvValue([]string{"A=1"}, "D", "")
	if len(envEmpty) != 1 {
		t.Errorf("expected unchanged env for empty val, got: %v", envEmpty)
	}
	writeSeatWorkerResult(tempDir, "test-engine", "result-ok")
}

func TestInProcess_Agent_SeatWorker_BuildPrompts(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	logger := logging.GetLoggerFromProfile("")

	// 1. Missing ATK ID in steer
	_, err := buildSeatWorkerAgentXPrompts(ctx, logger, provider, secCtx, tempDir, "SEAT-1", "PER-1", "EVT-1", "no atk here")
	if err != errPreparedContextRequired {
		t.Errorf("expected errPreparedContextRequired, got: %v", err)
	}

	// 2. Seed ATK and build prompt bundle
	planID, bliID := seedAgentSupportHierarchy(t, provider)
	taskID := "ATK-1791345678901234000-a1b2c3d4"
	taskObj := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Seat Worker Prompt Task",
		objects.FieldKeyDescription:        "Execution instructions for prompt builder test",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyBacklogItemRef:     bliID,
		objects.FieldKeyPriorityPlanRef:    planID,
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
		objects.FieldKeyEstimatedEffort:    "2d",
		objects.FieldKeyTaskSteps: []any{
			map[string]any{"description": "step 1"},
			map[string]any{"description": "step 2"},
		},
	}
	if err := provider.Create(ctx, secCtx, taskObj); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	bundle, err := buildSeatWorkerAgentXPrompts(ctx, logger, provider, secCtx, tempDir, "SEAT-1", "PER-INPROCESS-ENGINEER", "EVT-1", "Run "+taskID)
	if err != nil {
		t.Fatalf("buildSeatWorkerAgentXPrompts failed: %v", err)
	}
	if bundle.TaskID != taskID {
		t.Errorf("expected taskID %s, got: %s", taskID, bundle.TaskID)
	}
	if bundle.MaxSteps <= 0 {
		t.Errorf("expected positive MaxSteps, got: %d", bundle.MaxSteps)
	}
}

func TestInProcess_Agent_SeatWorker_HandleNonComms(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	logger := logging.GetLoggerFromProfile("")
	item := agentfeed.CorrespondenceItem{EventID: "EVT-NONCOMMS-001"}

	// 1. Steer body without ATK
	err := handleNonCommsWithAgentX(ctx, logger, provider, secCtx, tempDir, "SEAT-1", "PER-1", "SES-1", item, "steer with no atk")
	if err != nil {
		t.Fatalf("expected nil for skipped non-comms steer, got: %v", err)
	}

	// 2. Steer with grooming plan directive
	plannedPlan := map[string]any{
		objects.FieldKeyID:          "PRI-GROOMING-DIRECTIVE-001",
		objects.FieldKeyKind:        objects.KindPriorityPlan,
		objects.FieldKeyTitle:       "Grooming Directive Plan",
		objects.FieldKeyStatus:      objects.ObjectStatusGrooming,
		objects.FieldKeyDescription: "Grooming plan",
	}
	if err := provider.Create(ctx, secCtx, plannedPlan); err != nil {
		t.Fatalf("failed to seed grooming plan: %v", err)
	}
	err = handleNonCommsWithAgentX(ctx, logger, provider, secCtx, tempDir, "SEAT-1", "PER-1", "SES-1", item, "ORCHESTRATE_PLAN PRI-GROOMING-DIRECTIVE-001")
	if err != nil {
		t.Fatalf("expected skip ack for grooming plan directive, got: %v", err)
	}

	// 3. Steer with non-existent ATK claim failure
	err = handleNonCommsWithAgentX(ctx, logger, provider, secCtx, tempDir, "SEAT-1", "PER-1", "SES-1", item, "Run ATK-9999999-deadbeef")
	if err != nil {
		t.Fatalf("expected skip ack for non-existent ATK claim, got: %v", err)
	}

	// 4. Steer with exhausted attempts
	taskID := "ATK-EXHAUST-001"
	taskObj := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Exhausted Task",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyAssigneePersonaRef: "PER-DEFAULT-OPERATOR",
		objects.FieldKeyDescription:        "Some work",
	}
	if err := provider.Create(ctx, secCtx, taskObj); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}
	attemptKey := agentfeed.SeatAttemptKey("EVT-EXHAUST-001", taskID)
	_, _ = agentfeed.RecordSeatEventAttempt(tempDir, "SEAT-1", attemptKey)
	_, _ = agentfeed.RecordSeatEventAttempt(tempDir, "SEAT-1", attemptKey)
	_, _ = agentfeed.RecordSeatEventAttempt(tempDir, "SEAT-1", attemptKey)
	itemExhaust := agentfeed.CorrespondenceItem{EventID: "EVT-EXHAUST-001"}
	err = handleNonCommsWithAgentX(ctx, logger, provider, secCtx, tempDir, "SEAT-1", objects.ConstPersonaDefaultOperator, "SES-1", itemExhaust, "Execute "+taskID)
	if err != nil {
		t.Fatalf("expected nil for exhausted attempts ack, got: %v", err)
	}
}

func TestInProcess_Agent_SeatWorker_CLI_Execution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// 1. Missing --agent-id
	if _, err := executeAgentCommand(t, tempDir, provider, "seat-worker"); err == nil {
		t.Error("expected error when seat-worker missing --agent-id")
	}

	// 2. Valid seat-worker --once run on empty inbox
	out, err := executeAgentCommand(t, tempDir, provider, "seat-worker", "--agent-id", "SEAT-SW-001", "--once")
	if err != nil {
		t.Fatalf("seat-worker --once failed: %v, out: %s", err, out)
	}
	if !strings.Contains(out, "status: ok") && !strings.Contains(out, `"status": "ok"`) {
		t.Errorf("expected status ok in output, got: %s", out)
	}

	// 3. Valid seat-worker --once --execute-non-comms run
	outNonComms, err := executeAgentCommand(t, tempDir, provider, "seat-worker", "--agent-id", "SEAT-SW-001", "--once", "--execute-non-comms")
	if err != nil {
		t.Fatalf("seat-worker with non-comms failed: %v, out: %s", err, outNonComms)
	}
	if !strings.Contains(outNonComms, "status: ok") && !strings.Contains(outNonComms, `"status": "ok"`) {
		t.Errorf("expected status ok in output, got: %s", outNonComms)
	}
}

func TestInProcess_Agent_Execute_Validation(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// 1. Missing all arguments
	if _, err := executeAgentCommand(t, tempDir, provider, "execute"); err == nil {
		t.Error("expected error when execute called with no prompt or task")
	}

	// 2. Non-existent task ID
	if _, err := executeAgentCommand(t, tempDir, provider, "execute", "NONEXISTENT-ATK"); err == nil {
		t.Error("expected error for nonexistent task ID")
	}

	// 3. Task with empty description
	planID, bliID := seedAgentSupportHierarchy(t, provider)
	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	taskID := "ATK-EMPTY-DESC-001"
	taskObj := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Empty Desc Task",
		objects.FieldKeyDescription:        "",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyBacklogItemRef:     bliID,
		objects.FieldKeyPriorityPlanRef:    planID,
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
	}
	if err := provider.Create(ctx, secCtx, taskObj); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	if _, err := executeAgentCommand(t, tempDir, provider, "execute", taskID); err == nil {
		t.Error("expected error when agent task has empty prompt")
	}
}

func TestInProcess_Agent_SeatWorker_PlanDirectives(t *testing.T) {
	// 1. validPlanDirectiveID
	if !validPlanDirectiveID("PRI-VALID-PLAN-001") {
		t.Error("expected true for valid PRI- prefix")
	}
	if validPlanDirectiveID("INVALID-PREFIX-001") {
		t.Error("expected false for missing PRI- prefix")
	}
	if validPlanDirectiveID("PRI-INVALID!@#") {
		t.Error("expected false for invalid characters")
	}

	// 2. parseOrchestratePlanDirective
	planID, ok, err := parseOrchestratePlanDirective("ORCHESTRATE_PLAN PRI-TEST-001\nsome other details")
	if !ok || err != nil || planID != "PRI-TEST-001" {
		t.Errorf("expected parsed plan ID PRI-TEST-001, got %q, ok=%v, err=%v", planID, ok, err)
	}

	_, ok, _ = parseOrchestratePlanDirective("REGULAR_MESSAGE Hello agent")
	if ok {
		t.Error("expected false for non-orchestrate directive")
	}

	_, ok, err = parseOrchestratePlanDirective("ORCHESTRATE_PLAN INVALID-NO-PRI")
	if !ok || err == nil {
		t.Error("expected error for invalid plan directive ID")
	}
}

func TestInProcess_Agent_SeatWorker_DispatchLeadPlan(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := stdctx.Background()

	// 1. Nil storage
	resNil, err := dispatchLeadPlanFromWhatsNext(ctx, nil, nil, tempDir, "SEAT-TEST-001", "")
	if err != nil || resNil.Skipped != "no_storage" {
		t.Errorf("expected no_storage skip, got: %v, res: %+v", err, resNil)
	}

	// 2. Empty project (no plans)
	resEmpty, err := dispatchLeadPlanFromWhatsNext(ctx, nil, provider, tempDir, "SEAT-TEST-001", "")
	if err != nil || resEmpty.Skipped != "no_lead_plan" {
		t.Errorf("expected no_lead_plan skip, got: %v, res: %+v", err, resEmpty)
	}

	// 3. Active plan with no planned BLIs
	secCtx := pkgctx.NewSystemSecurityContext()
	planID := "PRI-EMPTY-BLIS-001"
	planObj := map[string]any{
		objects.FieldKeyID:          planID,
		objects.FieldKeyKind:        objects.KindPriorityPlan,
		objects.FieldKeyTitle:       "Empty Plan",
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
		objects.FieldKeyDescription: "Plan with no backlog items",
	}
	if err := provider.Create(ctx, secCtx, planObj); err != nil {
		t.Fatalf("failed to create plan: %v", err)
	}

	resNoPlanned, err := dispatchLeadPlanFromWhatsNext(ctx, nil, provider, tempDir, "SEAT-TEST-001", "")
	if err != nil || resNoPlanned.Skipped != "no_planned" {
		t.Errorf("expected no_planned skip, got: %v, res: %+v", err, resNoPlanned)
	}
}

func TestInProcess_Agent_SeatWorker_TriggerPlanOrchestration(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Inactive plan (grooming)
	planID := "PRI-GROOMING-001"
	planObj := map[string]any{
		objects.FieldKeyID:          planID,
		objects.FieldKeyKind:        objects.KindPriorityPlan,
		objects.FieldKeyTitle:       "Grooming Plan",
		objects.FieldKeyStatus:      objects.ObjectStatusGrooming,
		objects.FieldKeyDescription: "Plan in grooming state",
	}
	if err := provider.Create(ctx, secCtx, planObj); err != nil {
		t.Fatalf("failed to create grooming plan: %v", err)
	}

	if _, err := triggerPlanOrchestration(ctx, provider, secCtx, tempDir, planID); err == nil {
		t.Error("expected error when triggering orchestration on grooming plan")
	}
}
