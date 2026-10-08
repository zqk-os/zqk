package agent

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/agentprompt"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestOrchestrate_BacklogItemEligible(t *testing.T) {
	ineligible := []string{
		objects.ObjectStatusComplete,
		objects.ObjectStatusArchived,
		objects.ObjectStatusCancelled,
		objects.ObjectStatusImplemented,
		objects.ObjectStatusPendingVerification,
	}
	for _, st := range ineligible {
		assert.False(t, backlogItemEligibleForOrchestration(st), "expected status %s to be ineligible", st)
	}

	eligible := []string{
		objects.ObjectStatusDraft,
		objects.ObjectStatusApproved,
		objects.ObjectStatusInProgress,
		objects.ObjectStatusProposed,
		objects.ObjectStatusActive,
		"custom_status",
	}
	for _, st := range eligible {
		assert.True(t, backlogItemEligibleForOrchestration(st), "expected status %s to be eligible", st)
	}
}

func TestOrchestrate_ResolveTimeout(t *testing.T) {
	// Explicit opts timeout
	assert.Equal(t, 10*time.Minute, resolveOrchestrationTimeout(10*time.Minute, nil))

	// Cmd with flag
	cmd := &cobra.Command{}
	cmd.Flags().Duration("timeout", 5*time.Minute, "")
	assert.Equal(t, 5*time.Minute, resolveOrchestrationTimeout(0, cmd))

	// Default fallback
	emptyCmd := &cobra.Command{}
	assert.Equal(t, orchestrationTimeout, resolveOrchestrationTimeout(0, emptyCmd))
}

func TestOrchestrate_HasAnyPersonaAssignment(t *testing.T) {
	assert.False(t, hasAnyPersonaAssignment(nil))
	assert.False(t, hasAnyPersonaAssignment(map[string]any{}))

	// empty persona_refs
	assert.False(t, hasAnyPersonaAssignment(map[string]any{
		objects.FieldKeyPersonaRefs: []any{},
	}))

	// valid persona_refs as []any
	assert.True(t, hasAnyPersonaAssignment(map[string]any{
		objects.FieldKeyPersonaRefs: []any{"PER-1"},
	}))

	// valid persona_refs as []string
	assert.True(t, hasAnyPersonaAssignment(map[string]any{
		objects.FieldKeyPersonaRefs: []string{"PER-1"},
	}))

	// valid persona_refs as string
	assert.True(t, hasAnyPersonaAssignment(map[string]any{
		objects.FieldKeyPersonaRefs: "PER-1",
	}))

	// assignee_persona_ref
	assert.True(t, hasAnyPersonaAssignment(map[string]any{
		objects.FieldKeyAssigneePersonaRef: "PER-2",
	}))
}

func TestOrchestrate_HasPersonaMatch(t *testing.T) {
	// No filter always matches
	assert.True(t, hasPersonaMatch(map[string]any{"id": "foo"}, nil))

	// Unassigned work matches (soft match)
	assert.True(t, hasPersonaMatch(map[string]any{"id": "foo"}, []string{"PER-ME"}))

	// Match in []any
	objAny := map[string]any{
		objects.FieldKeyPersonaRefs: []any{"PER-A", "PER-B"},
	}
	assert.True(t, hasPersonaMatch(objAny, []string{"per-a"}))
	assert.False(t, hasPersonaMatch(objAny, []string{"PER-C"}))

	// Match in []string
	objStr := map[string]any{
		objects.FieldKeyPersonaRefs: []string{"PER-A", "PER-B"},
	}
	assert.True(t, hasPersonaMatch(objStr, []string{"per-b"}))
	assert.False(t, hasPersonaMatch(objStr, []string{"PER-C"}))

	// Match in assignee_persona_ref
	objAssignee := map[string]any{
		objects.FieldKeyAssigneePersonaRef: "PER-ASSIGNEE",
	}
	assert.True(t, hasPersonaMatch(objAssignee, []string{"per-assignee"}))
	assert.False(t, hasPersonaMatch(objAssignee, []string{"other"}))
}

func TestOrchestrate_NativeSwarmEligible(t *testing.T) {
	eligible := []string{
		"tier_1_routine",
		"tier_2_simple",
		"tier_3_light",
		"tier_3_simple",
		"TIER_2_SIMPLE",
	}
	for _, tier := range eligible {
		assert.True(t, nativeSwarmEligible(tier), "expected %s to be native swarm eligible", tier)
	}

	ineligible := []string{
		"tier_1_complex",
		"unknown",
		"",
	}
	for _, tier := range ineligible {
		assert.False(t, nativeSwarmEligible(tier), "expected %s to NOT be native swarm eligible", tier)
	}
}

func TestOrchestrate_ConfigureExecutorProcess_And_Args(t *testing.T) {
	c := exec.Command("echo", "test")
	configureOrchestrationExecutorProcess(c)
	assert.NotNil(t, c.SysProcAttr)
	assert.True(t, c.SysProcAttr.Setpgid)
	assert.NotNil(t, c.Cancel)
	assert.Equal(t, 5*time.Second, c.WaitDelay)

	argsNoTimeout := buildOrchestrationExecutorArgs("ATK-1", "prompt content", 0)
	assert.Equal(t, []string{"agent", "execute", "--task-id", "ATK-1", "--prompt", "prompt content"}, argsNoTimeout)

	argsWithTimeout := buildOrchestrationExecutorArgs("ATK-1", "prompt content", 30*time.Second)
	assert.Contains(t, argsWithTimeout, "--timeout")
	assert.Contains(t, argsWithTimeout, "30s")
}

func TestOrchestrate_ChildEnv(t *testing.T) {
	parent := []string{"FOO=bar", "PATH=/bin"}
	env := orchestrationExecutorChildEnv(parent, "/path/to/kernel", "test-key", "/bin/zqk")
	assert.NotEmpty(t, env)
	hasProjectRoot := false
	expected := zqkenv.ProjectRoot().Name() + "=/path/to/kernel"
	for _, e := range env {
		if e == expected || strings.HasSuffix(e, "PROJECT_ROOT=/path/to/kernel") {
			hasProjectRoot = true
			break
		}
	}
	assert.True(t, hasProjectRoot, "expected child env to include PROJECT_ROOT")
}

func TestOrchestrate_WaitForAgentTaskReadable(t *testing.T) {
	sp := newMockHelperStore()
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Immediately existing
	task := map[string]any{objects.FieldKeyID: "ATK-FOUND", objects.FieldKeyTitle: "Found"}
	require.NoError(t, sp.Create(ctx, secCtx, task))
	err := waitForAgentTaskReadable(ctx, sp, secCtx, "ATK-FOUND")
	require.NoError(t, err)

	// Context cancellation
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	err = waitForAgentTaskReadable(cancelCtx, sp, secCtx, "ATK-MISSING")
	assert.Error(t, err)
}

func TestOrchestrate_PersistTaskOutcome(t *testing.T) {
	sp := newMockHelperStore()
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	task := map[string]any{
		objects.FieldKeyID:     "ATK-OUTCOME-1",
		objects.FieldKeyTitle:  "Task Outcome Test",
		objects.FieldKeyStatus: objects.ObjectStatusApproved,
	}
	require.NoError(t, sp.Create(ctx, secCtx, task))

	state := &orchestratorState{
		sp:     sp,
		secCtx: secCtx,
		opts:   OrchestrateOptions{PersonaID: "PER-WORKER"},
	}

	output := map[string]any{
		"commit_sha": "abc1234",
		"summary":    "All tests pass",
	}

	err := persistOrchestratedTaskOutcome(ctx, state, "ATK-OUTCOME-1", objects.ObjectStatusComplete, output)
	require.NoError(t, err)

	updated, err := sp.Read(ctx, secCtx, "ATK-OUTCOME-1")
	require.NoError(t, err)
	assert.Equal(t, objects.ObjectStatusComplete, updated[objects.FieldKeyStatus])
	assert.NotEmpty(t, updated[objects.FieldKeyOutputs])
}

func TestOrchestrateSteps_RolesAndBoundaries(t *testing.T) {
	// Roles
	assert.Equal(t, "", personaRoleFromObject(nil))
	assert.Equal(t, "operator", personaRoleFromObject(map[string]any{objects.FieldKeyRole: "operator"}))

	// Estimated effort
	assert.Equal(t, "", orchestrationEstimatedEffort(nil))
	assert.Equal(t, "2h", orchestrationEstimatedEffort(map[string]any{objects.FieldKeyEstimatedEffort: "2h"}))

	// normalizePersonaRoleToken
	assert.Equal(t, "system_architect", normalizePersonaRoleToken("System-Architect", ""))
	assert.Equal(t, "coder_agent", normalizePersonaRoleToken("", "Coder Agent"))

	// Boundary for different roles
	roles := []string{
		"operator", "technical_program_manager", "planner", "designer",
		"system_architect", "agent", "software_engineer", "coder_agent",
		"coder", "qa_auditor", "test_agent", "devops_agent",
	}
	for _, r := range roles {
		b := orchestrationTaskBoundaryFor(r, "", agentprompt.WorkClassCoding)
		assert.True(t, b.GraphCompose || b.CodeValidate, "role %s must have at least one boundary flag", r)
	}

	// Docs eval boundary
	docsB := orchestrationTaskBoundaryFor("software_engineer", "", agentprompt.WorkClassDocsEval)
	assert.False(t, docsB.CodeValidate, "docs eval should have CodeValidate=false")

	// buildOrchestrationTaskSteps
	steps := buildOrchestrationTaskSteps("", orchestrationTaskBoundary{GraphCompose: true, CodeValidate: true})
	assert.Len(t, steps, 3)

	stepsNoExtra := buildOrchestrationTaskSteps("zqk", orchestrationTaskBoundary{GraphCompose: false, CodeValidate: false})
	assert.Len(t, stepsNoExtra, 1)
}

func TestOrchestrateReuse_TaskTitlesAndMatches(t *testing.T) {
	titles := orchestrationTaskTitles("Setup DB", "BLI-1")
	assert.Contains(t, titles, "Setup DB")
	assert.Contains(t, titles, "Execute Task: Setup DB")
	assert.Contains(t, titles, "Execute BLI-1")
	assert.Contains(t, titles, "Execute Task: BLI-1")

	assert.True(t, orchestrationTaskTitleMatches("Execute Task: Setup DB", "Setup DB", "BLI-1"))
	assert.False(t, orchestrationTaskTitleMatches("Unrelated", "Setup DB", "BLI-1"))

	// Plan matches
	assert.True(t, orchestrationTaskPlanMatches(nil, ""))
	assert.False(t, orchestrationTaskPlanMatches(nil, "PRI-1"))
	objWithPlan := map[string]any{objects.FieldKeyPriorityPlanRef: "PRI-1"}
	assert.True(t, orchestrationTaskPlanMatches(objWithPlan, "PRI-1"))
	assert.False(t, orchestrationTaskPlanMatches(objWithPlan, "PRI-2"))
}

func TestSeedAgentWorktreeRuntime_Direct(t *testing.T) {
	mainRoot := t.TempDir()
	worktreeRoot := t.TempDir()

	// 1. Missing main process dir returns error
	assert.Error(t, seedAgentWorktreeRuntime(filepath.Join(mainRoot, "nonexistent"), worktreeRoot))

	// 2. Pre-create worktree draft dir to exercise removal and re-creation
	worktreeDraftDir := storage.ObjectDraftPlaneRoot(worktreeRoot)
	require.NoError(t, fileutil.EnsureDir(worktreeDraftDir))
	dummyFile := filepath.Join(worktreeDraftDir, "dummy.yaml")
	require.NoError(t, fileutil.WriteFile(dummyFile, []byte("dummy"), paths.FilePerm600))

	// 3. Seed non-dir entry in main process dir
	processDir := filepath.Join(mainRoot, paths.ProcessDir)
	require.NoError(t, fileutil.EnsureDir(processDir))
	require.NoError(t, fileutil.WriteFile(filepath.Join(processDir, "file_not_dir"), []byte("data"), paths.FilePerm600))

	// 4. Seed kind dir with both .index, .index.json, and non-index file
	kindDir := filepath.Join(processDir, "backlog_items")
	require.NoError(t, fileutil.EnsureDir(kindDir))
	require.NoError(t, fileutil.WriteFile(filepath.Join(kindDir, ".index.json"), []byte(`{"entries":[]}`), paths.FilePerm600))
	require.NoError(t, fileutil.WriteFile(filepath.Join(kindDir, "custom.index"), []byte("index_data"), paths.FilePerm600))
	require.NoError(t, fileutil.WriteFile(filepath.Join(kindDir, "item.yaml"), []byte("yaml_data"), paths.FilePerm600))
	require.NoError(t, fileutil.EnsureDir(filepath.Join(kindDir, "nested_dir")))

	err := seedAgentWorktreeRuntime(mainRoot, worktreeRoot)
	require.NoError(t, err)

	assert.True(t, fileutil.Exists(filepath.Join(worktreeRoot, paths.ProcessDir, "backlog_items", ".index.json")))
	assert.True(t, fileutil.Exists(filepath.Join(worktreeRoot, paths.ProcessDir, "backlog_items", "custom.index")))
	assert.False(t, fileutil.Exists(filepath.Join(worktreeRoot, paths.ProcessDir, "backlog_items", "item.yaml")))
}

func TestFindExistingOrchestrationTask_Scenarios(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Nil state returns error
	_, _, _, err := findExistingOrchestrationTask(ctx, nil, "Title", "BLI-1")
	assert.Error(t, err)

	// 2. State with storage but no existing task returns mint
	sp := newMockHelperStore()
	state := &orchestratorState{
		sp:     sp,
		secCtx: secCtx,
		planID: "PRI-EMPTY-PLAN",
	}
	id, status, disp, err := findExistingOrchestrationTask(ctx, state, "Missing Task", "BLI-MISSING")
	require.NoError(t, err)
	assert.Empty(t, id)
	assert.Empty(t, status)
	assert.Equal(t, orchDispositionMint, disp)

	// 3. State with matching existing task
	existingTask := map[string]any{
		objects.FieldKeyID:              "ATK-FOUND-1",
		objects.FieldKeyKind:            objects.KindAgentTask,
		objects.FieldKeyTitle:           "Execute Task: Found Task",
		objects.FieldKeyStatus:          objects.ObjectStatusImplemented,
		objects.FieldKeyPriorityPlanRef: "PRI-EMPTY-PLAN",
	}
	require.NoError(t, sp.Create(ctx, secCtx, existingTask))

	id, status, disp, err = findExistingOrchestrationTask(ctx, state, "Found Task", "BLI-FOUND")
	require.NoError(t, err)
	assert.Equal(t, "ATK-FOUND-1", id)
	assert.Equal(t, objects.ObjectStatusImplemented, status)
	assert.Equal(t, orchDispositionSkipDone, disp)
}

func TestOrchestrate_CommandExecutionErrors(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// 1. Semantic routing failure for non-existent plan
	_, err := executeAgentCommand(t, tempDir, provider, "orchestrate", "NONEXISTENT_PLAN_XYZ")
	assert.Error(t, err)

	// 2. Help flag succeeds
	out, err := executeAgentCommand(t, tempDir, provider, "orchestrate", "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "orchestrate")
}

func TestPersistOrchestratedTaskOutcome_Branches(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	sp := newMockHelperStore()

	state := &orchestratorState{
		sp:     sp,
		secCtx: secCtx,
		opts:   OrchestrateOptions{PersonaID: "PER-CUSTOM"},
	}

	taskID := "ATK-OUTCOME-1"
	task := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Outcome Task",
		objects.FieldKeyStatus:             objects.ObjectStatusProposed,
		objects.FieldKeyAssigneePersonaRef: "PER-ASSIGNEE",
	}
	require.NoError(t, sp.Create(ctx, secCtx, task))

	// 1. InProgress sets claimant from opts.PersonaID
	err := persistOrchestratedTaskOutcome(ctx, state, taskID, objects.ObjectStatusInProgress, map[string]any{
		"commit_sha": "abc1234",
	})
	require.NoError(t, err)

	updated, err := sp.Read(ctx, secCtx, taskID)
	require.NoError(t, err)
	assert.Equal(t, objects.ObjectStatusInProgress, updated[objects.FieldKeyStatus])
	assert.Equal(t, "PER-CUSTOM", updated[objects.FieldKeyClaimedBy])

	// 2. Clear PersonaID and verify fallback to AssigneePersonaRef
	state.opts.PersonaID = ""
	delete(updated, objects.FieldKeyClaimedBy)
	require.NoError(t, sp.Update(ctx, secCtx, taskID, updated))

	err = persistOrchestratedTaskOutcome(ctx, state, taskID, objects.ObjectStatusInProgress, nil)
	require.NoError(t, err)

	updated, err = sp.Read(ctx, secCtx, taskID)
	require.NoError(t, err)
	assert.Equal(t, "PER-ASSIGNEE", updated[objects.FieldKeyClaimedBy])

	// 3. Nonexistent task returns error
	assert.Error(t, persistOrchestratedTaskOutcome(ctx, state, "ATK-NONEXISTENT", objects.ObjectStatusComplete, nil))
}

func TestBuildOrchestrationTaskEnvelope_FallbackAndAmbient(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", root)

	cmd := NewOrchestrateCmd()
	cmd.SetContext(context.Background())
	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)
	defer func() {
		if proc != nil && proc.Storage() != nil {
			_ = proc.Storage().Shutdown(context.Background())
		}
	}()

	ctx := context.Background()
	state := &orchestratorState{
		proc:   proc,
		sp:     newMockHelperStore(),
		secCtx: pkgctx.NewSystemSecurityContext(),
		opts: OrchestrateOptions{
			AmbientContext: "ambient text",
		},
	}
	env := buildOrchestrationTaskEnvelope(ctx, state, "worker", "PER-1", "coding", "Task Title", "", "upstream text")
	require.NotNil(t, env)
	assert.Contains(t, env.Description, "Task: Task Title")
	assert.Contains(t, env.Description, agentprompt.TaskEnvelopeMarker)
	assert.Contains(t, env.Description, "upstream text")

	// buildOrchestrationTaskPrompt
	prompt := buildOrchestrationTaskPrompt(ctx, state, "worker", "PER-1", "coding", "Prompt Title", "mesh skill", "upstream text")
	assert.Contains(t, prompt, "Prompt Title")
	assert.Contains(t, prompt, "upstream text")
	assert.Contains(t, prompt, "mesh skill")
}

func TestSeedAgentWorktreeRuntime_AllPaths(t *testing.T) {
	// 1. Missing mainProcessDir
	err := seedAgentWorktreeRuntime("/nonexistent/main", t.TempDir())
	assert.Error(t, err)

	// 2. Main root with process dir and indices
	mainRoot := t.TempDir()
	worktreeRoot := t.TempDir()

	kindDir := filepath.Join(mainRoot, paths.ProcessDir, "backlog_item")
	require.NoError(t, fileutil.EnsureDir(kindDir))
	require.NoError(t, fileutil.WriteStandardFile(filepath.Join(kindDir, "items.index"), []byte("index-data")))
	require.NoError(t, fileutil.WriteStandardFile(filepath.Join(kindDir, ".index.json"), []byte("{}")))
	require.NoError(t, fileutil.WriteStandardFile(filepath.Join(kindDir, "readme.txt"), []byte("text")))

	// Pre-create worktreeDraftDir to test replacement path
	draftDir := storage.ObjectDraftPlaneRoot(worktreeRoot)
	require.NoError(t, fileutil.EnsureDir(draftDir))

	err = seedAgentWorktreeRuntime(mainRoot, worktreeRoot)
	require.NoError(t, err)

	// Verify index files copied
	copiedIndex := filepath.Join(worktreeRoot, paths.ProcessDir, "backlog_item", "items.index")
	assert.True(t, fileutil.Exists(copiedIndex))
}
