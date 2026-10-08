package agent

import (
	"bytes"
	stdctx "context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	clitool "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

var agentTestMu sync.Mutex

func setupAgentInProcessProject(t *testing.T) (string, storage.ObjectStorageProvider, func()) {
	agentTestMu.Lock()
	t.Cleanup(func() { agentTestMu.Unlock() })

	t.Setenv("ZQK_TEST_BYPASS_GITEVIDENCE", "1")
	p := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:            "cmd.agent.comprehensive",
		SeedSchemaPlane: true,
	})
	provider, err := storage.NewFileObjectStorageForTest(p.Root)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, p.Root, provider)
	_ = datacell.WriteAgentChatChannelConfig(p.Root, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModePaste,
	})
	hooksDir := filepath.Join(p.Root, paths.ProjectDataDir, paths.LogsDir, "ide-hooks")
	_ = fileutil.EnsureDir(hooksDir)
	_ = fileutil.WriteStandardFile(filepath.Join(hooksDir, "agent_chat_channel.jsonl"), []byte(""))

	defaultOp := map[string]any{
		objects.FieldKeyID:          objects.ConstPersonaDefaultOperator,
		objects.FieldKeyKind:        objects.KindPersona,
		objects.FieldKeyTitle:       "Default Operator",
		objects.FieldKeyName:        "Default Operator",
		objects.FieldKeyRole:        "operator",
		objects.FieldKeyStatus:      objects.ObjectStatusApproved,
		objects.FieldKeyDescription: "Default operator persona for tests",
	}
	_ = provider.Create(stdctx.Background(), pkgctx.NewSystemSecurityContext(), defaultOp)

	return p.Root, provider, func() {}
}

func setAgentCLIContext(t *testing.T, cmd *cobra.Command, projectRoot string, storageProvider storage.ObjectStorageProvider) stdctx.Context {
	baseCtx := stdctx.Background()
	baseCtx = cli.WithStorageProvider(baseCtx, storageProvider)
	cmd.SetContext(baseCtx)

	initCtx := pkgctx.NewCliInitializationContext(func(s string) string { return projectRoot }, projectRoot)
	cliContextWrapper, err := cli.GetContextFromCommand(cmd, initCtx)
	if err != nil {
		t.Fatalf("Failed to get CLI context wrapper: %v", err)
	}
	cli.SetContext(cmd, cliContextWrapper)
	return baseCtx
}

func executeAgentCommand(t *testing.T, projectRoot string, provider storage.ObjectStorageProvider, args ...string) (string, error) {
	rootCmd := clitool.NewCommandBuilder("zqk").Build()
	if rootCmd.PersistentFlags().Lookup("format") == nil {
		rootCmd.PersistentFlags().String("format", "table", "Output format")
	}
	agentCmd := NewAgentCmd()
	rootCmd.AddCommand(agentCmd)

	fullArgs := append([]string{"agent"}, args...)
	rootCmd.SetArgs(fullArgs)

	var buf bytes.Buffer
	testCtx := setAgentCLIContext(t, rootCmd, projectRoot, provider)
	testCtx = pkgctx.WithCommandOutputWriter(testCtx, &buf)

	rootCmd.SetIn(bytes.NewReader(nil))
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetContext(testCtx)

	var propagate func(c *cobra.Command)
	propagate = func(c *cobra.Command) {
		c.SetContext(testCtx)
		c.SetIn(bytes.NewReader(nil))
		c.SetOut(&buf)
		c.SetErr(&buf)
		for _, child := range c.Commands() {
			propagate(child)
		}
	}
	propagate(rootCmd)

	err := rootCmd.ExecuteContext(testCtx)
	return buf.String(), err
}

func seedAgentSupportHierarchy(t *testing.T, provider storage.ObjectStorageProvider) (string, string) {
	t.Helper()
	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	persona := map[string]any{
		objects.FieldKeyID:          "PER-INPROCESS-ENGINEER",
		objects.FieldKeyKind:        objects.KindPersona,
		objects.FieldKeyTitle:       "InProcess Test Engineer",
		objects.FieldKeyName:        "Test Engineer",
		objects.FieldKeyRole:        "architect",
		objects.FieldKeyStatus:      objects.ObjectStatusApproved,
		objects.FieldKeyDescription: "Autonomous verification architect",
	}
	if err := provider.Create(ctx, secCtx, persona); err != nil {
		t.Fatalf("failed to seed persona: %v", err)
	}

	plan := map[string]any{
		objects.FieldKeyID:             "PRI-INPROCESS-PLAN-001",
		objects.FieldKeyKind:           objects.KindPriorityPlan,
		objects.FieldKeyTitle:          "InProcess Comprehensive Test Plan",
		objects.FieldKeyStatus:         objects.ObjectStatusGrooming,
		objects.FieldKeyDescription:    "Priority plan for testing agent subsystems",
		objects.FieldKeyPersonaRefs:    []any{"PER-INPROCESS-ENGINEER"},
		objects.FieldKeyWorkstreamRefs: []any{"WS-INPROCESS-001"},
	}
	if err := provider.Create(ctx, secCtx, plan); err != nil {
		t.Fatalf("failed to seed plan: %v", err)
	}

	bli := map[string]any{
		objects.FieldKeyID:                 "BLI-INPROCESS-ITEM-001",
		objects.FieldKeyKind:               objects.KindBacklogItem,
		objects.FieldKeyTitle:              "Elevate Agent Subsystem Coverage",
		objects.FieldKeyStatus:             objects.ObjectStatusPlanned,
		objects.FieldKeyPriorityPlanRef:    "PRI-INPROCESS-PLAN-001",
		objects.FieldKeyEstimatedEffort:    "2h",
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
		objects.FieldKeyDescription:        "Backlog item for agent comprehensive test",
	}
	if err := provider.Create(ctx, secCtx, bli); err != nil {
		t.Fatalf("failed to seed bli: %v", err)
	}

	plan[objects.FieldKeyStatus] = objects.ObjectStatusActive
	plan[objects.FieldKeyActiveOrder] = 1
	if err := provider.Update(ctx, secCtx, "PRI-INPROCESS-PLAN-001", plan); err != nil {
		t.Fatalf("failed to activate plan: %v", err)
	}

	return "PRI-INPROCESS-PLAN-001", "BLI-INPROCESS-ITEM-001"
}

func TestInProcess_Agent_SynthesizeSkill(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// Execute synthesize-skill command
	out, err := executeAgentCommand(t, tempDir, provider, "synthesize-skill", "--capability", "autonomous-indexing", "--provider", "anthropic")
	if err != nil {
		t.Fatalf("synthesize-skill failed: %v, out: %s", err, out)
	}

	// Verify skill file was created
	skillPath := filepath.Join(tempDir, paths.ProjectDataDir, paths.SkillsSubdir, "autonomous-indexing", "SKILL.md")
	if !fileutil.IsRegularFile(skillPath) {
		t.Errorf("expected skill file to exist at %s", skillPath)
	}

	// Verify agent_skill object in storage
	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	skills, err := provider.List(ctx, secCtx, nil, storage.ListFilter{Kind: objects.KindAgentSkill})
	if err != nil || len(skills.Objects) == 0 {
		t.Errorf("expected agent_skill object in storage, got err=%v, count=%d", err, len(skills.Objects))
	}
}

func TestInProcess_Agent_PrepareContext(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	_, bliID := seedAgentSupportHierarchy(t, provider)

	// Format: JSON
	outJSON, err := executeAgentCommand(t, tempDir, provider, "prepare-context", bliID, "--persona-ref", "PER-INPROCESS-ENGINEER", "--format", "json")
	if err != nil {
		t.Fatalf("prepare-context json failed: %v", err)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(outJSON), &res); err != nil {
		t.Fatalf("unmarshal prepare-context json failed: %v, raw: %s", err, outJSON)
	}
	if res["task_id"] != bliID {
		t.Errorf("expected task_id %s, got %v", bliID, res["task_id"])
	}

	// Format: agent_prompt
	outPrompt, err := executeAgentCommand(t, tempDir, provider, "prepare-context", bliID, "--persona-ref", "PER-INPROCESS-ENGINEER", "--format", "agent_prompt")
	if err != nil {
		t.Fatalf("prepare-context agent_prompt failed: %v", err)
	}
	if len(outPrompt) == 0 {
		t.Error("expected non-empty prompt output")
	}
}

func TestInProcess_Agent_PreSubagentHook(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Nil storage -> allow
	var out bytes.Buffer
	if err := runPreSubagentHookWithDeps(ctx, secCtx, nil, tempDir, "antigravity", bytes.NewReader(nil), &out); err != nil {
		t.Fatalf("hook with nil storage failed: %v", err)
	}
	if !strings.Contains(out.String(), `"allow"`) {
		t.Errorf("expected allow decision, got: %s", out.String())
	}

	// 2. Empty input -> allow
	out.Reset()
	if err := runPreSubagentHookWithDeps(ctx, secCtx, provider, tempDir, "antigravity", bytes.NewReader([]byte("   ")), &out); err != nil {
		t.Fatalf("hook with empty input failed: %v", err)
	}
	if !strings.Contains(out.String(), `"allow"`) {
		t.Errorf("expected allow decision, got: %s", out.String())
	}

	// 3. Non-invoke_subagent tool call -> allow
	out.Reset()
	nonSubagentInput := `{"toolCall":{"name":"read_file","args":{"path":"foo.go"}}}`
	if err := runPreSubagentHookWithDeps(ctx, secCtx, provider, tempDir, "antigravity", strings.NewReader(nonSubagentInput), &out); err != nil {
		t.Fatalf("hook with other tool call failed: %v", err)
	}
	if !strings.Contains(out.String(), `"allow"`) {
		t.Errorf("expected allow decision, got: %s", out.String())
	}

	// 4. Valid invoke_subagent tool call -> allow
	out.Reset()
	subagentInput := `{
		"toolCall": {
			"name": "invoke_subagent",
			"args": {
				"Subagents": [
					{"TypeName": "self", "Role": "Architect", "Prompt": "Review design"}
				]
			}
		}
	}`
	if err := runPreSubagentHookWithDeps(ctx, secCtx, provider, tempDir, "antigravity", strings.NewReader(subagentInput), &out); err != nil {
		t.Fatalf("hook with invoke_subagent failed: %v", err)
	}
	if !strings.Contains(out.String(), `"allow"`) {
		t.Errorf("expected allow decision, got: %s", out.String())
	}
}

func TestInProcess_Agent_Orchestrate_DirectHelpers(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()
	_ = tempDir

	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	planID, bliID := seedAgentSupportHierarchy(t, provider)

	// orchestrationCLI
	if cliName := orchestrationCLI(); cliName == "" {
		t.Error("expected non-empty orchestrationCLI")
	}

	// personaRoleFromObject & orchestrationEstimatedEffort
	if r := personaRoleFromObject(nil); r != "" {
		t.Errorf("expected empty role for nil, got %q", r)
	}
	if r := personaRoleFromObject(map[string]any{"role": "architect"}); r != "architect" {
		t.Errorf("expected architect, got %q", r)
	}
	if e := orchestrationEstimatedEffort(nil); e != "" {
		t.Errorf("expected empty effort for nil, got %q", e)
	}
	if e := orchestrationEstimatedEffort(map[string]any{"estimated_effort": "4h"}); e != "4h" {
		t.Errorf("expected 4h, got %q", e)
	}

	// isTestCaseReadyStatus
	if !isTestCaseReadyStatus(objects.ObjectStatusActive) || !isTestCaseReadyStatus(objects.ObjectStatusComplete) || !isTestCaseReadyStatus(objects.ObjectStatusDraft) || isTestCaseReadyStatus(objects.ObjectStatusArchived) {
		t.Error("unexpected isTestCaseReadyStatus results")
	}

	// stringIDsFromAny & itemPersonaID
	if len(stringIDsFromAny([]string{" A ", "B"})) != 2 {
		t.Error("unexpected stringIDsFromAny []string result")
	}
	if len(stringIDsFromAny([]any{" A ", 123, "B"})) != 2 {
		t.Error("unexpected stringIDsFromAny []any result")
	}
	if pid := itemPersonaID(map[string]any{objects.FieldKeyAssigneePersonaRef: "PER-1"}); pid != "PER-1" {
		t.Errorf("expected PER-1, got %q", pid)
	}
	if pid := itemPersonaID(map[string]any{objects.FieldKeyPersonaRefs: []string{"PER-2"}}); pid != "PER-2" {
		t.Errorf("expected PER-2, got %q", pid)
	}

	// orchestrationPersonaRole
	if r := orchestrationPersonaRole(ctx, nil, secCtx); r != defaultOrchestrationPersonaRole {
		t.Errorf("expected default role for nil sp, got %q", r)
	}
	if r := orchestrationPersonaRole(ctx, provider, secCtx, "PER-INPROCESS-ENGINEER"); r != "architect" {
		t.Errorf("expected architect, got %q", r)
	}

	// orchestrationTaskBacklogMatches
	if !orchestrationTaskBacklogMatches(map[string]any{objects.FieldKeyBacklogItemRef: "BLI-1"}, "BLI-1") {
		t.Error("expected backlog match")
	}
	if orchestrationTaskBacklogMatches(nil, "BLI-1") || orchestrationTaskBacklogMatches(map[string]any{}, "") {
		t.Error("unexpected backlog match on nil/empty")
	}

	// listOrchestrationTasksForPlan
	if _, err := listOrchestrationTasksForPlan(ctx, nil, secCtx, planID); err == nil {
		t.Error("expected error for nil storage")
	}
	tasks, err := listOrchestrationTasksForPlan(ctx, provider, secCtx, planID)
	if err != nil {
		t.Fatalf("listOrchestrationTasksForPlan failed: %v", err)
	}
	_ = tasks

	// verifyBLITDDReady
	if verifyBLITDDReady(ctx, provider, secCtx, nil) {
		t.Error("expected false for nil item")
	}
	bliObj, _ := provider.Read(ctx, secCtx, bliID)
	_ = verifyBLITDDReady(ctx, provider, secCtx, bliObj)

	// localSkillIDByTitle
	_ = localSkillIDByTitle(ctx, provider, secCtx, "autonomous-indexing")
}

func TestInProcess_Agent_TaskOutcome_And_Envelope(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	planID, bliID := seedAgentSupportHierarchy(t, provider)

	taskID := "ATK-INPROCESS-TEST-001"
	taskObj := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "InProcess Agent Task",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyBacklogItemRef:     bliID,
		objects.FieldKeyPriorityPlanRef:    planID,
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeyDescription:        "Comprehensive test agent task description",
	}
	if err := provider.Create(ctx, secCtx, taskObj); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	rootCmd := clitool.NewCommandBuilder("zqk").Build()
	_ = setAgentCLIContext(t, rootCmd, tempDir, provider)
	proc, err := newAgentProcessor(rootCmd)
	if err != nil {
		t.Fatalf("failed to create processor: %v", err)
	}

	state := &orchestratorState{
		proc:   proc,
		secCtx: secCtx,
		sp:     provider,
		planID: planID,
		opts:   OrchestrateOptions{PersonaID: "PER-INPROCESS-ENGINEER"},
	}

	prompt := buildOrchestrationTaskPrompt(ctx, state, "worker", "PER-INPROCESS-ENGINEER", "coding", "Build Feature", "", "upstream artifact")
	if len(prompt) == 0 {
		t.Error("expected non-empty prompt")
	}

	envelope := buildOrchestrationTaskEnvelope(ctx, state, "worker", "PER-INPROCESS-ENGINEER", "coding", "Build Feature", "", "upstream artifact")
	if envelope == nil || len(envelope.Description) == 0 {
		t.Error("expected non-nil envelope")
	}

	id, status, disp, fErr := findExistingOrchestrationTask(ctx, state, "InProcess Agent Task", bliID)
	if fErr != nil {
		t.Fatalf("findExistingOrchestrationTask failed: %v", fErr)
	}
	_ = id
	_ = status
	_ = disp

	outPayload := map[string]any{
		"commit_sha":           "abc1234",
		objects.FieldKeyStatus: objects.ObjectStatusComplete,
	}
	if err := persistOrchestratedTaskOutcome(ctx, state, taskID, objects.ObjectStatusInProgress, outPayload); err != nil {
		t.Fatalf("persistOrchestratedTaskOutcome failed: %v", err)
	}
}

func TestInProcess_Agent_RunOrchestrate_Validation(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// Semantic routing error: nonexistent plan
	_, err := executeAgentCommand(t, tempDir, provider, "orchestrate", "PRI-NONEXISTENT-PLAN")
	if err == nil {
		t.Error("expected error orchestrating nonexistent plan")
	}

	// Plan status error: plan in grooming status
	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	groomingPlan := map[string]any{
		objects.FieldKeyID:     "PRI-GROOMING-ONLY",
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyTitle:  "Grooming Only Plan",
		objects.FieldKeyStatus: objects.ObjectStatusGrooming,
	}
	if err := provider.Create(ctx, secCtx, groomingPlan); err != nil {
		t.Fatalf("failed to create grooming plan: %v", err)
	}

	_, err = executeAgentCommand(t, tempDir, provider, "orchestrate", "PRI-GROOMING-ONLY")
	if err == nil {
		t.Error("expected error orchestrating grooming-only plan")
	}
}

func TestInProcess_Agent_SyncLoop_DirectHelpers(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	planID, bliID := seedAgentSupportHierarchy(t, provider)

	// stripGraphBloat
	objWithBloat := map[string]any{
		"title":                                   "Task",
		"resolved_upstream":                       "foo",
		objects.FieldKeyStatusHistory:             []any{"a", "b"},
		objects.FieldKeyChangeLog:                 []any{"c"},
		objects.FieldKeyResolvedRelatedObjectRefs: []any{"r1"},
		"nested": map[string]any{
			"resolved_child": "bar",
			"clean_field":    "keep",
		},
	}
	stripGraphBloat(objWithBloat)
	if _, ok := objWithBloat["resolved_upstream"]; ok {
		t.Error("expected resolved_upstream to be stripped")
	}
	if _, ok := objWithBloat[objects.FieldKeyStatusHistory]; ok {
		t.Error("expected status_history to be stripped")
	}

	// flipHourglass and removeHourglass
	flipHourglass(ctx, secCtx, provider, bliID, tempDir)
	removeHourglass(ctx, secCtx, provider, bliID, tempDir)

	// QuerySubgraph
	subgraph, err := QuerySubgraph(ctx, secCtx, provider, planID, 3)
	if err != nil {
		t.Fatalf("QuerySubgraph failed: %v", err)
	}
	_ = subgraph
}

func TestInProcess_Agent_SeatWorker_Helpers(t *testing.T) {
	tempDir, _, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := stdctx.Background()

	// parseOrchestratePlanDirective & validPlanDirectiveID
	if planID, ok, err := parseOrchestratePlanDirective("ORCHESTRATE_PLAN PRI-1234"); !ok || err != nil || planID != "PRI-1234" {
		t.Errorf("expected PRI-1234, got %s, ok=%v, err=%v", planID, ok, err)
	}
	if _, ok, err := parseOrchestratePlanDirective("ORCHESTRATE_PLAN INVALID@!"); !ok || err == nil {
		t.Error("expected error for invalid plan directive ID")
	}
	if _, ok, _ := parseOrchestratePlanDirective("NOT_A_DIRECTIVE"); ok {
		t.Error("expected ok=false for non-directive")
	}
	if !validPlanDirectiveID("PRI-VALID-123") || validPlanDirectiveID("INVALID") {
		t.Error("unexpected validPlanDirectiveID results")
	}

	// submitKernelFill edge cases
	if res, err := submitKernelFill(ctx, tempDir, nil); res != "" || err != nil {
		t.Errorf("expected empty result for nil fill, got %q, %v", res, err)
	}
	if res, err := submitKernelFill(ctx, tempDir, &whatsnext.FillItem{AutoSubmit: false}); res != "" || err != nil {
		t.Errorf("expected empty result for non-autosubmit fill, got %q, %v", res, err)
	}
	if res, err := submitKernelFill(ctx, tempDir, &whatsnext.FillItem{AutoSubmit: true, SubmitArgs: ""}); res != "" || err != nil {
		t.Errorf("expected empty result for empty args fill, got %q, %v", res, err)
	}
}

func TestInProcess_Agent_Next_VerifyDocsEval(t *testing.T) {
	tempDir, _, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// No findings path
	if err := verifyDocsEvalNextEvidence(tempDir, "regular description without findings path"); err == nil {
		t.Error("expected error when no findings jsonl in description")
	}

	// Valid findings path
	relPath := "docs/findings/test_eval.jsonl"
	absPath := filepath.Join(tempDir, relPath)
	if err := fileutil.EnsureDir(filepath.Dir(absPath)); err == nil {
		_ = fileutil.WriteStandardFile(absPath, []byte(`{"finding_id": "FIND-001", "summary": "test"}`+"\n"))
		descWithFindings := fmt.Sprintf("Docs eval task output at %s for verification", relPath)
		if err := verifyDocsEvalNextEvidence(tempDir, descWithFindings); err != nil {
			t.Errorf("expected success for valid findings jsonl, got: %v", err)
		}
	}
}

func TestInProcess_Agent_Scoreboard_Execution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	out, err := executeAgentCommand(t, tempDir, provider, "scoreboard", "--format", "json")
	if err != nil {
		t.Fatalf("scoreboard failed: %v, out: %s", err, out)
	}
}

func TestInProcess_Agent_Next_Execution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	planID, bliID := seedAgentSupportHierarchy(t, provider)

	// Seed findings file for docs eval task
	relPath := "docs/findings/task_eval.jsonl"
	absPath := filepath.Join(tempDir, relPath)
	if err := fileutil.EnsureDir(filepath.Dir(absPath)); err != nil {
		t.Fatalf("failed to ensure dir: %v", err)
	}
	if err := fileutil.WriteStandardFile(absPath, []byte(`{"finding_id": "FIND-EVAL-1"}`+"\n")); err != nil {
		t.Fatalf("failed to write findings: %v", err)
	}

	taskID := "ATK-DOCS-EVAL-001"
	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	taskObj := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "docs_eval verification item",
		objects.FieldKeyDescription:        fmt.Sprintf("Docs eval findings at %s", relPath),
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeyBacklogItemRef:     bliID,
		objects.FieldKeyPriorityPlanRef:    planID,
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
	}
	if err := provider.Create(ctx, secCtx, taskObj); err != nil {
		t.Fatalf("failed to create docs eval task: %v", err)
	}

	out, err := executeAgentCommand(t, tempDir, provider, "next", taskID)
	if err != nil {
		t.Fatalf("agent next failed: %v, out: %s", err, out)
	}

	// Verify status updated to pending_verification
	readTask, err := provider.Read(ctx, secCtx, taskID)
	if err != nil {
		t.Fatalf("failed to read task: %v", err)
	}
	if readTask[objects.FieldKeyStatus] != objects.ObjectStatusPendingVerification {
		t.Errorf("expected pending_verification, got: %v", readTask[objects.FieldKeyStatus])
	}
}

func TestInProcess_Agent_ClaimAndRelease_Execution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	planID, bliID := seedAgentSupportHierarchy(t, provider)

	taskID := "ATK-CLAIM-RELEASE-001"
	ctx := stdctx.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	taskObj := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Claimable Task",
		objects.FieldKeyDescription:        "Task to test claim and release commands",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyBacklogItemRef:     bliID,
		objects.FieldKeyPriorityPlanRef:    planID,
		objects.FieldKeyAssigneePersonaRef: "PER-INPROCESS-ENGINEER",
		objects.FieldKeyEstimatedEffort:    "1h",
	}
	if err := provider.Create(ctx, secCtx, taskObj); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Claim
	claimOut, err := executeAgentCommand(t, tempDir, provider, "claim", taskID)
	if err != nil {
		t.Fatalf("claim failed: %v, out: %s", err, claimOut)
	}

	// Release
	relOut, err := executeAgentCommand(t, tempDir, provider, "release", taskID, "--force")
	if err != nil {
		t.Fatalf("release failed: %v, out: %s", err, relOut)
	}
}

func TestInProcess_Agent_ClaimGate_Execution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	out, err := executeAgentCommand(t, tempDir, provider, "claim-gate", "--by", "SEAT-TEST-001")
	if err != nil {
		t.Logf("claim-gate result: %v, out: %s", err, out)
	}
}

func TestInProcess_Agent_ChatResponder_Execution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	out, err := executeAgentCommand(t, tempDir, provider, "chat-responder")
	if err != nil {
		t.Fatalf("chat-responder failed: %v, out: %s", err, out)
	}
	if !strings.Contains(out, "No active Antigravity transcript found") {
		t.Logf("chat-responder output: %s", out)
	}
}

func TestInProcess_Agent_GuidingStep_Execution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// Missing args
	if _, err := executeAgentCommand(t, tempDir, provider, "guiding-step"); err == nil {
		t.Error("expected error when neither --event nor --shell provided")
	}

	// With --shell
	outShell, err := executeAgentCommand(t, tempDir, provider, "guiding-step", "--shell", "git status")
	if err != nil {
		t.Fatalf("guiding-step with shell failed: %v, out: %s", err, outShell)
	}

	// With --event
	outEvent, err := executeAgentCommand(t, tempDir, provider, "guiding-step", "--event", "session_start")
	if err != nil {
		t.Fatalf("guiding-step with event failed: %v, out: %s", err, outEvent)
	}
}

func TestProcSecurityAndTuple(t *testing.T) {
	// nil processor
	sec := procSecurity(nil)
	if sec == nil || sec.AccountID != pkgctx.SystemAccountID {
		t.Fatalf("expected system secCtx for nil proc, got %v", sec)
	}

	c, s, p := procStorageTuple(nil)
	if c == nil || s == nil || p != nil {
		t.Fatalf("unexpected tuple for nil proc: c=%v, s=%v, p=%v", c, s, p)
	}
}
