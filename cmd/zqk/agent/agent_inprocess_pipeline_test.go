package agent

import (
	stdctx "context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/audit"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/mutation"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestInProcess_Agent_Orchestrate_Execution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Seed criterion
	critObj := map[string]any{
		objects.FieldKeyID:          "CRI-ORCH-EXEC-001",
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyTitle:       "Orchestration Criterion",
		objects.FieldKeyStatus:      "awaiting_verification",
		objects.FieldKeyCategory:    "functional",
		objects.FieldKeyDescription: "Criterion for orchestrate test",
	}
	if err := provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, critObj); err != nil {
		t.Fatalf("failed to create criterion: %v", err)
	}

	// Seed requirement
	reqID := "REQ-ORCH-EXEC-001"
	reqObj := map[string]any{
		objects.FieldKeyID:           reqID,
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyTitle:        "Orchestration Requirement",
		objects.FieldKeyStatus:       objects.ObjectStatusActive,
		objects.FieldKeyCriteriaRefs: []string{"CRI-ORCH-EXEC-001"},
		objects.FieldKeyDescription:  "Requirement for orchestrate test",
	}
	if err := provider.Create(ctx, secCtx, reqObj); err != nil {
		t.Fatalf("failed to create requirement: %v", err)
	}

	// Seed grooming plan
	planID := "PRI-ORCH-EXEC-001"
	planObj := map[string]any{
		objects.FieldKeyID:             planID,
		objects.FieldKeyKind:           objects.KindPriorityPlan,
		objects.FieldKeyTitle:          "Orchestration Execution Plan",
		objects.FieldKeyStatus:         objects.ObjectStatusGrooming,
		objects.FieldKeyDescription:    "Plan for testing orchestrate pipeline",
		objects.FieldKeyPersonaRefs:    []any{"PER-DEFAULT-OPERATOR"},
		objects.FieldKeyWorkstreamRefs: []any{"WS-ORCH-001"},
	}
	if err := provider.Create(ctx, secCtx, planObj); err != nil {
		t.Fatalf("failed to create plan: %v", err)
	}

	// Seed shovel-ready BLI
	bliID := "BLI-ORCH-EXEC-001"
	bliObj := map[string]any{
		objects.FieldKeyID:                 bliID,
		objects.FieldKeyKind:               objects.KindBacklogItem,
		objects.FieldKeyTitle:              "Shovel Ready Task",
		objects.FieldKeyStatus:             objects.ObjectStatusPlanned,
		objects.FieldKeyPriorityPlanRef:    planID,
		objects.FieldKeyRequirementRefs:    []string{reqID},
		objects.FieldKeyCriteriaRefs:       []string{"CRI-ORCH-EXEC-001"},
		objects.FieldKeyEstimatedEffort:    "2h",
		objects.FieldKeyAssigneePersonaRef: "PER-DEFAULT-OPERATOR",
		objects.FieldKeyPersonaRefs:        []string{"PER-DEFAULT-OPERATOR"},
		objects.FieldKeyModelTier:          "tier_1_complex",
		objects.FieldKeyDescription:        "Full shovel-ready backlog item description",
	}
	if err := provider.Create(ctx, secCtx, bliObj); err != nil {
		t.Fatalf("failed to create bli: %v", err)
	}

	planObj[objects.FieldKeyStatus] = objects.ObjectStatusActive
	planObj[objects.FieldKeyActiveOrder] = 1
	if err := provider.Update(ctx, secCtx, planID, planObj); err != nil {
		t.Fatalf("failed to activate plan: %v", err)
	}

	out, err := executeAgentCommand(t, tempDir, provider, "orchestrate", planID)
	if err != nil {
		t.Fatalf("orchestrate failed: %v, out: %s", err, out)
	}
	t.Logf("orchestrate output:\n%s", out)
}

func TestInProcess_Agent_SyncLoop_Validation(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	_, bliID := seedAgentSupportHierarchy(t, provider)

	// 1. Non-existent task
	if _, err := executeAgentCommand(t, tempDir, provider, "sync-loop", "ATK-NONEXISTENT"); err == nil {
		t.Error("expected error when sync-loop given nonexistent task")
	}

	// 2. Backlog item instead of agent_task
	if _, err := executeAgentCommand(t, tempDir, provider, "sync-loop", bliID); err == nil {
		t.Error("expected error when sync-loop given backlog_item")
	}
}

func TestInProcess_Agent_New_Execution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	out, err := executeAgentCommand(t, tempDir, provider, "new", "Autonomous Tester", "--description", "Operating agent persona for testing purposes.")
	if err != nil {
		t.Fatalf("agent new failed: %v, out: %s", err, out)
	}
	t.Logf("agent new out: %s", out)
}

func TestInProcess_Agent_Recover_Execution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	planID, _ := seedAgentSupportHierarchy(t, provider)

	// 1. Recover specific plan
	outPlan, err := executeAgentCommand(t, tempDir, provider, "recover", planID)
	if err != nil {
		t.Fatalf("recover plan failed: %v, out: %s", err, outPlan)
	}

	// 2. Recover --all
	outAll, err := executeAgentCommand(t, tempDir, provider, "recover", "--all")
	if err != nil {
		t.Fatalf("recover --all failed: %v, out: %s", err, outAll)
	}

	// 3. Recover --stale --all
	outStale, err := executeAgentCommand(t, tempDir, provider, "recover", "--stale", "--all")
	if err != nil {
		t.Fatalf("recover --stale --all failed: %v, out: %s", err, outStale)
	}
}

func TestInProcess_Agent_Status_Execution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	out, err := executeAgentCommand(t, tempDir, provider, "status")
	if err != nil {
		t.Fatalf("agent status failed: %v, out: %s", err, out)
	}
	t.Logf("agent status out:\n%s", out)
}

func TestInProcess_Agent_Validate_Execution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	out, err := executeAgentCommand(t, tempDir, provider, "validate")
	if err != nil {
		t.Logf("agent validate returned: %v, out: %s", err, out)
	}
}

func TestInProcess_Agent_ClaimGate_ActiveIntent(t *testing.T) {
	tempDir, _, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// 1. Missing pointer
	if got := activeIntentAssignment(tempDir); got != "" {
		t.Errorf("expected empty string for missing pointer, got: %q", got)
	}

	// 2. Pointer pointing to valid intent JSON
	stateDir := paths.StateDirPath(tempDir)
	_ = fileutil.EnsureDir(stateDir)
	intentPath := filepath.Join(stateDir, "intent.json")
	intentData, _ := json.Marshal(map[string]any{"assignment": "BLI-ACTIVE-INTENT-001"})
	_ = fileutil.WriteStandardFile(intentPath, intentData)

	pointerPath := filepath.Join(stateDir, "change_intent_active")
	_ = fileutil.WriteStandardFile(pointerPath, []byte(intentPath+"\n"))

	if got := activeIntentAssignment(tempDir); got != "BLI-ACTIVE-INTENT-001" {
		t.Errorf("expected BLI-ACTIVE-INTENT-001, got: %q", got)
	}
}

func TestInProcess_Agent_SeatWorker_Fill_Direct(t *testing.T) {
	tempDir, _, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := stdctx.Background()

	// 1. Nil fill
	s, err := submitKernelFill(ctx, tempDir, nil)
	if err != nil || s != "" {
		t.Errorf("expected empty returns for nil fill, got: %q, %v", s, err)
	}

	// 2. AutoSubmit false
	fNoAuto := &whatsnext.FillItem{AutoSubmit: false, SubmitArgs: "run"}
	s, err = submitKernelFill(ctx, tempDir, fNoAuto)
	if err != nil || s != "" {
		t.Errorf("expected empty returns for non-autosubmit, got: %q, %v", s, err)
	}

	// 3. Empty submit args
	fNoArgs := &whatsnext.FillItem{AutoSubmit: true, SubmitArgs: ""}
	s, err = submitKernelFill(ctx, tempDir, fNoArgs)
	if err != nil || s != "" {
		t.Errorf("expected empty returns for empty submit args, got: %q, %v", s, err)
	}

	// 4. Recent fill submit skip
	fValid := &whatsnext.FillItem{Kind: "backlog_item", AutoSubmit: true, SubmitArgs: "mock-arg"}
	_ = writeFillSubmitMark(tempDir, "backlog_item", "already_dispatched")
	s, err = submitKernelFill(ctx, tempDir, fValid)
	if err != nil {
		t.Fatalf("expected nil error on recent fill skip, got: %v", err)
	}
	t.Logf("submitKernelFill skip msg: %s", s)

	// 5. schedulerCallbackNotify
	cb := schedulerCallbackNotify("zqk", "/tmp/log.json")
	if !strings.Contains(cb, "callback notify") {
		t.Errorf("expected callback notify in output, got: %s", cb)
	}
}

func TestInProcess_Agent_LoopGuard_DirectHelpers(t *testing.T) {
	// 1. verificationAttemptsOf
	if n := verificationAttemptsOf(nil); n != 0 {
		t.Errorf("expected 0 for nil, got %d", n)
	}
	if n := verificationAttemptsOf(map[string]any{}); n != 0 {
		t.Errorf("expected 0 for empty, got %d", n)
	}
	if n := verificationAttemptsOf(map[string]any{objects.FieldKeyVerificationAttempts: 3}); n != 3 {
		t.Errorf("expected 3 for int, got %d", n)
	}
	if n := verificationAttemptsOf(map[string]any{objects.FieldKeyVerificationAttempts: int64(4)}); n != 4 {
		t.Errorf("expected 4 for int64, got %d", n)
	}
	if n := verificationAttemptsOf(map[string]any{objects.FieldKeyVerificationAttempts: float64(5)}); n != 5 {
		t.Errorf("expected 5 for float64, got %d", n)
	}
	if n := verificationAttemptsOf(map[string]any{objects.FieldKeyVerificationAttempts: "6"}); n != 6 {
		t.Errorf("expected 6 for string, got %d", n)
	}
	if n := verificationAttemptsOf(map[string]any{objects.FieldKeyVerificationAttempts: true}); n != 0 {
		t.Errorf("expected 0 for boolean, got %d", n)
	}

	// 2. LoadLoopGuardConfig
	cfg := LoadLoopGuardConfig()
	if cfg.MaxSyncLoops <= 0 || cfg.MaxVerificationAttempts <= 0 {
		t.Errorf("expected positive loop guard defaults: %+v", cfg)
	}

	// 3. stagnationGuard
	sgZero := newStagnationGuard(0)
	if sgZero == nil || sgZero.max <= 0 {
		t.Errorf("expected default max for zero max, got: %+v", sgZero)
	}
	var nilSG *stagnationGuard
	if nilSG.Observe("fp-1") {
		t.Error("expected false for nil stagnation guard")
	}

	sg := newStagnationGuard(2)
	if sg.Observe("fp-1") {
		t.Error("expected first observation not to abort")
	}
	if sg.Observe("fp-1") {
		t.Error("expected second same observation not to abort")
	}
	if !sg.Observe("fp-1") {
		t.Error("expected third same observation to abort")
	}
}

func TestInProcess_Agent_Claim_ResolveClaimantIdentity(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// 1. With AgentID env set
	t.Setenv(zqkenv.AgentID().Name(), "SEAT-RESOLVE-001")
	if got := resolveClaimantIdentity(nil, nil); got != "SEAT-RESOLVE-001" {
		t.Errorf("expected SEAT-RESOLVE-001, got: %q", got)
	}

	// 2. Without AgentID env, with processor security context
	t.Setenv(zqkenv.AgentID().Name(), "")
	cmd := &cobra.Command{}
	setAgentCLIContext(t, cmd, tempDir, provider)
	proc, err := newAgentProcessor(cmd)
	if err != nil {
		t.Fatalf("failed to create proc: %v", err)
	}
	got := resolveClaimantIdentity(cmd, proc)
	if got == "" {
		t.Error("expected non-empty identity")
	}

	// 3. Fallback when proc is nil
	gotHost := resolveClaimantIdentity(nil, nil)
	if !strings.HasPrefix(gotHost, "host:") && gotHost != "anonymous" {
		t.Errorf("expected host or anonymous, got: %q", gotHost)
	}

	// 4. checkinDue for missing task
	if due := checkinDue(tempDir, "ATK-NONEXISTENT"); due != "" {
		t.Errorf("expected empty due string, got: %q", due)
	}

	// 5. procSecurity and procStorageTuple
	sec := procSecurity(nil)
	if sec == nil {
		t.Error("expected non-nil system security context")
	}
	ctxStd, secStd, spStd := procStorageTuple(nil)
	if ctxStd == nil || secStd == nil || spStd != nil {
		t.Errorf("unexpected procStorageTuple nil results: %v, %v, %v", ctxStd, secStd, spStd)
	}
	emptyCmd := &cobra.Command{}
	emptyCmd.SetContext(stdctx.Background())
	procEmpty, err := newAgentProcessor(emptyCmd)
	if err != nil {
		t.Fatalf("failed to create processor: %v", err)
	}
	if procEmpty.ProjectRoot() == "" {
		t.Error("expected non-empty project root")
	}
}

func TestInProcess_Agent_PreparedContext_HasSemanticContext(t *testing.T) {
	// 1. Empty
	emptyPC := &PreparedContext{}
	if emptyPC.HasSemanticContext() {
		t.Error("expected empty context to have no semantic context")
	}

	// 2. Description in Task
	withDesc := &PreparedContext{Task: map[string]any{objects.FieldKeyDescription: "Some instructions"}}
	if !withDesc.HasSemanticContext() {
		t.Error("expected context with description to have semantic context")
	}

	// 3. Criteria in Deps
	withCrit := &PreparedContext{
		Deps: []map[string]any{
			{objects.FieldKeyAcceptanceCriteria: []any{"criterion 1"}},
		},
	}
	if !withCrit.HasSemanticContext() {
		t.Error("expected context with criteria deps to have semantic context")
	}

	// 4. Problem statement in Deps
	withProb := &PreparedContext{
		Deps: []map[string]any{
			{"problem_statement": "System bottleneck observed"},
		},
	}
	if !withProb.HasSemanticContext() {
		t.Error("expected context with problem_statement to have semantic context")
	}
}

func TestInProcess_Agent_Orchestrate_TDD_And_Readiness(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. waitForAgentTaskReadable: existing task
	taskID := "ATK-READY-CHECK-001"
	taskObj := map[string]any{
		objects.FieldKeyID:          taskID,
		objects.FieldKeyKind:        objects.KindAgentTask,
		objects.FieldKeyTitle:       "Ready Check Task",
		objects.FieldKeyDescription: "Checking readability",
		objects.FieldKeyStatus:      objects.ObjectStatusApproved,
	}
	if err := provider.Create(ctx, secCtx, taskObj); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}
	if err := waitForAgentTaskReadable(ctx, provider, secCtx, taskID); err != nil {
		t.Errorf("expected task to be readable: %v", err)
	}

	// 2. waitForAgentTaskReadable: cancelled context
	cancCtx, cancel := stdctx.WithCancel(ctx)
	cancel()
	if err := waitForAgentTaskReadable(cancCtx, provider, secCtx, "ATK-NONEXISTENT"); err != stdctx.Canceled {
		t.Errorf("expected context canceled, got: %v", err)
	}

	// 3. hasAnyPersonaAssignment & hasPersonaMatch
	itemWithRef := map[string]any{objects.FieldKeyPersonaRefs: []string{"PER-CODER"}}
	if !hasAnyPersonaAssignment(itemWithRef) {
		t.Error("expected true for item with persona_refs")
	}
	if !hasPersonaMatch(itemWithRef, []string{"PER-CODER"}) {
		t.Error("expected match for PER-CODER")
	}
	if hasPersonaMatch(itemWithRef, []string{"PER-OTHER"}) {
		t.Error("expected no match for PER-OTHER")
	}

	// 4. verifyBLITDDReady: nil item
	if verifyBLITDDReady(ctx, provider, secCtx, nil) {
		t.Error("expected false for nil item")
	}

	// 5. verifyBLITDDReady: with direct test case
	tcID := "TST-DIRECT-001"
	tcObj := map[string]any{
		objects.FieldKeyID:     tcID,
		objects.FieldKeyKind:   objects.KindTestCase,
		objects.FieldKeyTitle:  "Direct Test Case",
		objects.FieldKeyStatus: "active",
	}
	_ = provider.Create(ctx, secCtx, tcObj)

	bliWithTC := map[string]any{
		objects.FieldKeyID:           "BLI-WITH-TC-001",
		objects.FieldKeyTestCaseRefs: []string{tcID},
	}
	if !verifyBLITDDReady(ctx, provider, secCtx, bliWithTC) {
		t.Error("expected true for BLI with direct ready test case")
	}

	// 6. localSkillIDByTitle
	if id := localSkillIDByTitle(ctx, nil, secCtx, "Skill"); id != "" {
		t.Error("expected empty string for nil storage")
	}
	if id := localSkillIDByTitle(ctx, provider, secCtx, ""); id != "" {
		t.Error("expected empty string for empty title")
	}
	if id := localSkillIDByTitle(ctx, provider, secCtx, "Nonexistent Skill"); id != "" {
		t.Error("expected empty string for nonexistent title")
	}
	skillID := "ASK-LOCAL-001"
	skillObj := map[string]any{
		objects.FieldKeyID:            skillID,
		objects.FieldKeyKind:          "agent_skill",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:         "Special Craft Skill",
		objects.FieldKeyDescription:   "Description for craft skill",
		objects.FieldKeyStatus:        objects.ObjectStatusApproved,
	}
	if err := provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, skillObj); err != nil {
		t.Fatalf("failed to create skill: %v", err)
	}
	_ = caspkg.FlushKindListingIndexForProjectRootWithTimeout(tempDir, "agent_skill", 2*time.Second)
	if id := localSkillIDByTitle(ctx, provider, secCtx, "Special Craft Skill"); id != skillID {
		t.Errorf("expected skill id %s, got: %s", skillID, id)
	}
}

func TestInProcess_Agent_SyncLoop_ImplementedExit(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	cmdInit := execwrap.CommandContext(ctx, "git", "init")
	cmdInit.Dir = tempDir
	if err := cmdInit.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	cmdConfigName := execwrap.CommandContext(ctx, "git", "config", "user.name", "Test Agent")
	cmdConfigName.Dir = tempDir
	_ = cmdConfigName.Run()
	cmdConfigEmail := execwrap.CommandContext(ctx, "git", "config", "user.email", "agent@test.internal")
	cmdConfigEmail.Dir = tempDir
	_ = cmdConfigEmail.Run()
	cmdCommit := execwrap.CommandContext(ctx, "git", "commit", "--allow-empty", "-m", "init")
	cmdCommit.Dir = tempDir
	if err := cmdCommit.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	gitCmd := execwrap.CommandContext(ctx, "git", "rev-parse", "HEAD")
	gitCmd.Dir = tempDir
	commitOut, err := gitCmd.Output()
	if err != nil {
		t.Fatalf("failed to get git HEAD: %v", err)
	}
	headCommit := strings.TrimSpace(string(commitOut))

	taskID := "ATK-SYNC-IMPL-001"
	taskObj := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Implemented Agent Task",
		objects.FieldKeyStatus:             objects.ObjectStatusImplemented,
		objects.FieldKeyAssigneePersonaRef: "PER-DEFAULT-OPERATOR",
		objects.FieldKeyCommitHash:         headCommit,
		objects.FieldKeyDescription:        "Task already in implemented status",
	}
	if err := provider.Create(ctx, secCtx, taskObj); err != nil {
		t.Fatalf("failed to create implemented task: %v", err)
	}

	out, err := executeAgentCommand(t, tempDir, provider, "sync-loop", taskID)
	if err != nil {
		t.Fatalf("sync-loop failed on implemented task: %v, out: %s", err, out)
	}
	if !strings.Contains(out, "status: implemented") && !strings.Contains(out, "reached terminal state: implemented") {
		t.Errorf("expected terminal state output, got: %s", out)
	}
}

func TestInProcess_Agent_Execute_TaskAndPromptExecution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Direct prompt with invalid mcp path
	_, err := executeAgentCommand(t, tempDir, provider, "execute", "--prompt", "execute test instruction", "--mcp-path", "/nonexistent/mcp")
	if err == nil {
		t.Error("expected error for nonexistent mcp path")
	}

	// 2. ATK with execution prompt
	taskID := "ATK-EXEC-TEST-001"
	taskObj := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Execute Target ATK",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyAssigneePersonaRef: "PER-DEFAULT-OPERATOR",
		objects.FieldKeyDescription:        "Perform repository refactor",
	}
	if err := provider.Create(ctx, secCtx, taskObj); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	_, err = executeAgentCommand(t, tempDir, provider, "execute", taskID, "--mcp-path", "/nonexistent/mcp")
	if err == nil {
		t.Error("expected error during mcp executor initialization for task")
	}

	// 3. Ad-hoc Backlog Item
	bliID := "BLI-EXEC-TEST-001"
	bliObj := map[string]any{
		objects.FieldKeyID:                 bliID,
		objects.FieldKeyKind:               objects.KindBacklogItem,
		objects.FieldKeyTitle:              "Execute Target BLI",
		objects.FieldKeyStatus:             objects.ObjectStatusPlanned,
		objects.FieldKeyAssigneePersonaRef: "PER-DEFAULT-OPERATOR",
		objects.FieldKeyDescription:        "Perform ad-hoc backlog item task",
	}
	if err := provider.Create(ctx, secCtx, bliObj); err != nil {
		t.Fatalf("failed to create bli: %v", err)
	}

	_, err = executeAgentCommand(t, tempDir, provider, "execute", bliID, "--mcp-path", "/nonexistent/mcp")
	if err == nil {
		t.Error("expected error during mcp executor initialization for bli")
	}
}

func TestInProcess_Agent_Orchestrate_SemanticRoutingAndPlans(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Invalid plan ID error
	_, err := executeAgentCommand(t, tempDir, provider, "orchestrate", "INVALID-NONEXISTENT-PLAN")
	if err == nil {
		t.Error("expected error when orchestrating invalid plan ID")
	}

	// 2. Strategic plan execution
	stratID := "STRAT-PLAN-ORCH-001"
	stratObj := map[string]any{
		objects.FieldKeyID:          stratID,
		objects.FieldKeyKind:        objects.KindStrategicPlan,
		objects.FieldKeyTitle:       "Strategic Vision Plan",
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
		objects.FieldKeyDescription: "Strategic plan for testing executive review items",
		"planning_horizon":          "3y",
		"phases":                    []any{"Phase 1"},
		objects.FieldKeyPersonaRefs: []any{objects.ConstPersonaDefaultOperator},
	}
	if err := provider.Create(ctx, secCtx, stratObj); err != nil {
		t.Fatalf("failed to create strategic plan: %v", err)
	}
	outStrat, err := executeAgentCommand(t, tempDir, provider, "orchestrate", stratID)
	if err != nil {
		t.Logf("orchestrate strategic plan returned: %v, out: %s", err, outStrat)
	}

	// 3. Pipeline execution
	pipeID := "PIP-ORCH-TEST-001"
	pipeObj := map[string]any{
		objects.FieldKeyID:          pipeID,
		objects.FieldKeyKind:        objects.KindPipeline,
		objects.FieldKeyTitle:       "Pipeline Execution",
		objects.FieldKeyStatus:      objects.ObjectStatusApproved,
		objects.FieldKeyDescription: "Pipeline for testing task refs",
		"trigger":                   "manual",
	}
	if err := provider.Create(ctx, secCtx, pipeObj); err != nil {
		t.Fatalf("failed to create pipeline: %v", err)
	}
	outPipe, err := executeAgentCommand(t, tempDir, provider, "orchestrate", pipeID)
	if err != nil {
		t.Logf("orchestrate pipeline returned: %v, out: %s", err, outPipe)
	}

	// 4. Auto-discovery via whats-next when no planArg provided
	outAuto, err := executeAgentCommand(t, tempDir, provider, "orchestrate")
	if err == nil {
		t.Logf("orchestrate auto output: %s", outAuto)
	}
}

func TestInProcess_Agent_SyncLoop_ExtendedHelpers(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. QuerySubgraph with depth <= 0
	taskID := "ATK-SUBGRAPH-001"
	taskObj := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Subgraph Task",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeyAssigneePersonaRef: "PER-DEFAULT-OPERATOR",
		objects.FieldKeyDescription:        "Task description",
	}
	if err := provider.Create(ctx, secCtx, taskObj); err != nil {
		t.Fatalf("failed to create subgraph task: %v", err)
	}

	deps, err := QuerySubgraph(ctx, secCtx, provider, taskID, 0)
	if err != nil {
		t.Fatalf("QuerySubgraph failed: %v", err)
	}
	if len(deps) != 0 {
		t.Logf("deps: %v", deps)
	}

	// 2. applyStateMutationWithFields
	validator := mutation.NewValidator(nil)
	auditStream := audit.NewAuditStream(tempDir)
	extra := map[string]any{
		"custom_note":          "verified in test",
		objects.FieldKeyStatus: "ignored_override",
	}
	err = applyStateMutationWithFields(ctx, secCtx, provider, taskID, objects.KindAgentTask, validator, auditStream, objects.ObjectStatusPendingVerification, extra)
	if err != nil {
		t.Logf("applyStateMutationWithFields result: %v", err)
	}
}

func TestInProcess_Agent_SyncLoop_LLMFailurePath(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	taskID := "ATK-SYNC-LLM-001"
	taskObj := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "LLM Sync Task",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeyAssigneePersonaRef: "PER-DEFAULT-OPERATOR",
		objects.FieldKeyDescription:        "Execute sync loop LLM completion step",
	}
	if err := provider.Create(ctx, secCtx, taskObj); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// sync-loop will run 1 tick (2s), build prompt, attempt LLM completion, handle error, and return
	_, err := executeAgentCommand(t, tempDir, provider, "sync-loop", taskID)
	if err == nil {
		t.Error("expected error from LLM client in unit test environment")
	}
}
