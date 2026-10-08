package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/daemon/singleton"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/objects/koi"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type mockHelperStore struct {
	storage.ObjectStorageProvider
	objs map[string]map[string]any
}

func newMockHelperStore() *mockHelperStore {
	return &mockHelperStore{objs: make(map[string]map[string]any)}
}

func (m *mockHelperStore) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	id := koi.ID(obj)
	m.objs[id] = obj
	return nil
}

func (m *mockHelperStore) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.objs[id]; ok {
		return obj, nil
	}
	return nil, errfmt.Errorf("not found: %s", id)
}

func (m *mockHelperStore) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, obj map[string]any) error {
	m.objs[id] = obj
	return nil
}

func (m *mockHelperStore) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	var res []map[string]any
	for _, obj := range m.objs {
		if filter.Kind != "" && koi.Kind(obj) != filter.Kind {
			continue
		}
		res = append(res, obj)
	}
	return &storage.QueryResult{Objects: res}, nil
}

func (m *mockHelperStore) BeginTransaction(ctx context.Context) (storage.ObjectTransaction, error) {
	return &mockHelperTx{m: m}, nil
}

type mockHelperTx struct {
	m *mockHelperStore
}

func (tx *mockHelperTx) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	return tx.m.Create(ctx, secCtx, obj)
}

func (tx *mockHelperTx) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return tx.m.Read(ctx, secCtx, id)
}

func (tx *mockHelperTx) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	existing, err := tx.m.Read(ctx, secCtx, id)
	if err != nil {
		existing = make(map[string]any)
	}
	for k, v := range updates {
		existing[k] = v
	}
	return tx.m.Update(ctx, secCtx, id, existing)
}

func (tx *mockHelperTx) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	delete(tx.m.objs, id)
	return nil
}

func (tx *mockHelperTx) Commit(ctx context.Context) error   { return nil }
func (tx *mockHelperTx) Rollback(ctx context.Context) error { return nil }

func TestIsTestCaseReadyStatus(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"active", true},
		{"ACTIVE", true},
		{"draft", true},
		{"metrics_captured", true},
		{"complete", true},
		{"in_progress", false},
		{"originated", false},
		{"unknown", false},
	}
	for _, tc := range cases {
		if got := isTestCaseReadyStatus(tc.status); got != tc.want {
			t.Errorf("isTestCaseReadyStatus(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestExtractItemUpstreamIDs(t *testing.T) {
	if ids := extractItemUpstreamIDs(nil); ids != nil {
		t.Errorf("expected nil for nil item, got %v", ids)
	}

	item := map[string]any{
		"depends_on":         []any{"ATK-1", "ATK-2"},
		"upstream_task_refs": []any{"ATK-3"},
	}
	ids := extractItemUpstreamIDs(item)
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %d", len(ids))
	}
	if ids[0] != "ATK-1" || ids[1] != "ATK-2" || ids[2] != "ATK-3" {
		t.Errorf("unexpected IDs order: %v", ids)
	}
}

func TestResolveVerifiedUpstreamDeliverables(t *testing.T) {
	sp := newMockHelperStore()
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Upstream task complete with artifacts and summary
	upTask := map[string]any{
		objects.FieldKeyID:        "ATK-UPSTREAM-1",
		objects.FieldKeyKind:      objects.KindAgentTask,
		objects.FieldKeyStatus:    objects.ObjectStatusComplete,
		objects.FieldKeyTitle:     "Build Parser",
		objects.FieldKeyArtifacts: []any{"pkg/parser/ast.go"},
		"summary":                 "Parser constructed cleanly",
	}
	if err := sp.Create(ctx, secCtx, upTask); err != nil {
		t.Fatalf("failed to create upstream task: %v", err)
	}

	downstreamItem := map[string]any{
		objects.FieldKeyID:   "ATK-DOWNSTREAM-1",
		"upstream_task_refs": []any{"ATK-UPSTREAM-1"},
	}

	out := resolveVerifiedUpstreamDeliverables(ctx, sp, secCtx, downstreamItem)
	if !strings.Contains(out, "Upstream Verified Deliverables") {
		t.Errorf("expected header, got: %s", out)
	}
	if !strings.Contains(out, "pkg/parser/ast.go") {
		t.Errorf("expected artifact, got: %s", out)
	}
	if !strings.Contains(out, "Parser constructed cleanly") {
		t.Errorf("expected summary, got: %s", out)
	}
}

func TestIsBlockedByUnverifiedUpstream(t *testing.T) {
	sp := newMockHelperStore()
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Missing upstream item blocks
	blockedItem := map[string]any{
		"upstream_task_refs": []any{"ATK-NONEXISTENT"},
	}
	if !isBlockedByUnverifiedUpstream(ctx, sp, secCtx, blockedItem) {
		t.Errorf("expected blocked by missing upstream")
	}

	// Incomplete upstream blocks
	upIncomplete := map[string]any{
		objects.FieldKeyID:        "ATK-INCOMPLETE",
		objects.FieldKeyKind:      objects.KindAgentTask,
		objects.FieldKeyStatus:    objects.ObjectStatusInProgress,
		objects.FieldKeyArtifacts: []any{"pkg/lib/lib.go"},
	}
	if err := sp.Create(ctx, secCtx, upIncomplete); err != nil {
		t.Fatalf("failed to create incomplete task: %v", err)
	}

	item2 := map[string]any{
		"upstream_task_refs": []any{"ATK-INCOMPLETE"},
	}
	if !isBlockedByUnverifiedUpstream(ctx, sp, secCtx, item2) {
		t.Errorf("expected blocked by in-progress upstream")
	}

	// Complete upstream with empty artifacts blocks
	upNoArtifacts := map[string]any{
		objects.FieldKeyID:        "ATK-NO-ARTS",
		objects.FieldKeyKind:      objects.KindAgentTask,
		objects.FieldKeyStatus:    objects.ObjectStatusComplete,
		objects.FieldKeyArtifacts: []any{},
	}
	if err := sp.Create(ctx, secCtx, upNoArtifacts); err != nil {
		t.Fatalf("failed to create no artifacts task: %v", err)
	}

	item3 := map[string]any{
		"upstream_task_refs": []any{"ATK-NO-ARTS"},
	}
	if !isBlockedByUnverifiedUpstream(ctx, sp, secCtx, item3) {
		t.Errorf("expected blocked by empty artifacts upstream")
	}

	// Complete upstream with artifacts does not block
	upGood := map[string]any{
		objects.FieldKeyID:        "ATK-GOOD",
		objects.FieldKeyKind:      objects.KindAgentTask,
		objects.FieldKeyStatus:    objects.ObjectStatusComplete,
		objects.FieldKeyArtifacts: []any{"pkg/lib/lib.go"},
	}
	if err := sp.Create(ctx, secCtx, upGood); err != nil {
		t.Fatalf("failed to create good task: %v", err)
	}

	itemGood := map[string]any{
		"upstream_task_refs": []any{"ATK-GOOD"},
	}
	if isBlockedByUnverifiedUpstream(ctx, sp, secCtx, itemGood) {
		t.Errorf("expected unblocked for good upstream")
	}
}

func TestVerifyBLITDDReady_And_LocalSkill(t *testing.T) {
	sp := newMockHelperStore()
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	skill := map[string]any{
		objects.FieldKeyID:    "ASK-GO-CRAFT",
		objects.FieldKeyKind:  objects.KindAgentSkill,
		objects.FieldKeyTitle: "Go Code Craftsmanship",
	}
	if err := sp.Create(ctx, secCtx, skill); err != nil {
		t.Fatalf("failed to create skill: %v", err)
	}

	if id := localSkillIDByTitle(ctx, sp, secCtx, "go code craftsmanship"); id != "ASK-GO-CRAFT" {
		t.Errorf("expected ASK-GO-CRAFT, got: %s", id)
	}
	if id := localSkillIDByTitle(ctx, sp, secCtx, "nonexistent"); id != "" {
		t.Errorf("expected empty string for nonexistent, got: %s", id)
	}

	tc := map[string]any{
		objects.FieldKeyID:     "TC-1",
		objects.FieldKeyKind:   objects.KindTestCase,
		objects.FieldKeyStatus: "complete",
	}
	if err := sp.Create(ctx, secCtx, tc); err != nil {
		t.Fatalf("failed to create test case: %v", err)
	}

	bli := map[string]any{
		objects.FieldKeyTestCaseRefs: []any{"TC-1"},
	}
	if !verifyBLITDDReady(ctx, sp, secCtx, bli) {
		t.Errorf("expected bli with direct complete test case to be TDD ready")
	}
}

func TestRunOrchestrate_ValidationBranches(t *testing.T) {
	tempDir, _, cleanup := setupAgentInProcessProject(t)
	defer cleanup()
	t.Setenv("ZQK_PROJECT_ROOT", tempDir)

	// 1. Deny doer / guest
	cmd := NewOrchestrateCmd()
	cmd.SetContext(pkgctx.WithSecurityContext(context.Background(), pkgctx.NewGuestSecurityContext()))
	err := runOrchestrate(cmd, "PRI-1", OrchestrateOptions{})
	assert.ErrorContains(t, err, "cannot orchestrate")

	// 2. Semantic routing failure
	adminCtx := pkgctx.WithSecurityContext(context.Background(), pkgctx.NewSystemSecurityContext())
	cmdAdmin := NewOrchestrateCmd()
	cmdAdmin.SetContext(adminCtx)
	err = runOrchestrate(cmdAdmin, "NONEXISTENT-PLAN-XYZ", OrchestrateOptions{})
	assert.ErrorContains(t, err, "semantic routing failed")

	// 3. No active plan via whats-next
	cmdEmpty := NewOrchestrateCmd()
	cmdEmpty.SetContext(adminCtx)
	err = runOrchestrate(cmdEmpty, "", OrchestrateOptions{})
	assert.ErrorContains(t, err, "no active priority plan")
}

func TestRunOrchestrate_GuardAlreadyAcquired(t *testing.T) {
	tempDir, _, cleanup := setupAgentInProcessProject(t)
	defer cleanup()
	t.Setenv("ZQK_PROJECT_ROOT", tempDir)

	planID := "PRI-LOCKED-001"
	guardName := "orchestrator-" + strings.ToLower(planID)
	release, err := singleton.Guard(tempDir, guardName)
	require.NoError(t, err)
	defer release()

	adminCtx := pkgctx.WithSecurityContext(context.Background(), pkgctx.NewSystemSecurityContext())
	cmdAdmin := NewOrchestrateCmd()
	cmdAdmin.SetContext(adminCtx)

	err = runOrchestrate(cmdAdmin, planID, OrchestrateOptions{})
	assert.NoError(t, err)
}

func TestRunOrchestrate_PlanKindsAndFailures(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()
	t.Setenv("ZQK_PROJECT_ROOT", tempDir)

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	adminCtx := pkgctx.WithSecurityContext(ctx, secCtx)

	// 1. Missing plan in storage after guard acquired
	cmd1 := NewOrchestrateCmd()
	cmd1.SetContext(adminCtx)
	err := runOrchestrate(cmd1, "PRI-NONEXISTENT-AFTER-GUARD", OrchestrateOptions{})
	assert.ErrorContains(t, err, "failed to load plan")

	// 2. Priority plan with inactive/unsupported status (e.g. proposed)
	planInactive := map[string]any{
		objects.FieldKeyID:            "PRI-INACTIVE-STATUS-001",
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Proposed Priority Plan",
		objects.FieldKeyDescription:   "Proposed plan description for unit test",
		objects.FieldKeyStatus:        objects.ObjectStatusProposed,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, planInactive))

	cmd2 := NewOrchestrateCmd()
	cmd2.SetContext(adminCtx)
	err = runOrchestrate(cmd2, "PRI-INACTIVE-STATUS-001", OrchestrateOptions{})
	assert.ErrorContains(t, err, "orchestrator can only operate on active or in_progress")

	// 3. Pipeline plan kind loads tasks
	pipePlan := map[string]any{
		objects.FieldKeyID:            "PIP-TEST-PIPELINE-001",
		objects.FieldKeyKind:          objects.KindPipeline,
		objects.FieldKeyTitle:         "Test Pipeline",
		objects.FieldKeyDescription:   "Test pipeline description",
		objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyAgentTaskRefs: []any{"ATK-1"},
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, pipePlan))

	cmd3 := NewOrchestrateCmd()
	cmd3.SetContext(adminCtx)
	_ = runOrchestrate(cmd3, "PIP-TEST-PIPELINE-001", OrchestrateOptions{})
}

func TestRunOrchestrate_StrategicPlanRouting(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()
	t.Setenv("ZQK_PROJECT_ROOT", tempDir)

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	adminCtx := pkgctx.WithSecurityContext(ctx, secCtx)

	defaultAgent := map[string]any{
		objects.FieldKeyID:          objects.ConstPersonaDefaultAgent,
		objects.FieldKeyKind:        objects.KindPersona,
		objects.FieldKeyTitle:       "Default Agent",
		objects.FieldKeyName:        "Default Agent",
		objects.FieldKeyRole:        "agent",
		objects.FieldKeyStatus:      objects.ObjectStatusApproved,
		objects.FieldKeyDescription: "Default agent persona for tests",
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, defaultAgent))

	stratPlan := map[string]any{
		objects.FieldKeyID:            "STRAT-PLAN-TEST-001",
		objects.FieldKeyKind:          objects.KindStrategicPlan,
		objects.FieldKeyTitle:         "Test Strategic Plan",
		objects.FieldKeyDescription:   "Strategic plan description for testing review items",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyPersonaRefs:   []any{objects.ConstPersonaDefaultAgent},
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, stratPlan))

	cmd := NewOrchestrateCmd()
	cmd.SetContext(adminCtx)
	err := runOrchestrate(cmd, "STRAT-PLAN-TEST-001", OrchestrateOptions{})
	assert.NoError(t, err)
}

func TestRunOrchestrate_DenyDoer(t *testing.T) {
	tempDir, _, cleanup := setupAgentInProcessProject(t)
	defer cleanup()
	t.Setenv("ZQK_PROJECT_ROOT", tempDir)

	cmd := NewOrchestrateCmd()
	cmd.SetContext(pkgctx.WithSecurityContext(context.Background(), pkgctx.NewSecurityContext("doer-agent-1", []string{"developer"}, []string{"read:*", "write:code"})))
	err := runOrchestrate(cmd, "PRI-ANY-001", OrchestrateOptions{})
	assert.ErrorContains(t, err, "doer seats cannot orchestrate peers")
}

func TestRunOrchestrate_EmptyPlanArg_NoActivePlan(t *testing.T) {
	tempDir, _, cleanup := setupAgentInProcessProject(t)
	defer cleanup()
	t.Setenv("ZQK_PROJECT_ROOT", tempDir)

	cmd := NewOrchestrateCmd()
	cmd.SetContext(pkgctx.WithSecurityContext(context.Background(), pkgctx.NewSystemSecurityContext()))
	err := runOrchestrate(cmd, "", OrchestrateOptions{})
	assert.ErrorContains(t, err, "no active priority plan or strategic plan found to orchestrate")
}

func TestRunOrchestrate_EmptyPlanArg_ActivePlan(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()
	t.Setenv("ZQK_PROJECT_ROOT", tempDir)

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	adminCtx := pkgctx.WithSecurityContext(ctx, secCtx)

	plan := map[string]any{
		objects.FieldKeyID:            "PRI-AUTODISCOVER-001",
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Autodiscovered Active Plan",
		objects.FieldKeyDescription:   "Active plan for empty planArg test",
		objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, plan))

	cmd := NewOrchestrateCmd()
	cmd.SetContext(adminCtx)
	err := runOrchestrate(cmd, "", OrchestrateOptions{})
	assert.NoError(t, err)
}

func TestRunOrchestrate_SkipDoneAndReplaceDispositions(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()
	t.Setenv("ZQK_PROJECT_ROOT", tempDir)
	t.Setenv("ZQK_TEST_BYPASS_GITEVIDENCE", "1")

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	adminCtx := pkgctx.WithSecurityContext(ctx, secCtx)

	_ = execwrap.CommandContext(ctx, "git", "init", tempDir).Run()
	_ = execwrap.CommandContext(ctx, "git", "-C", tempDir, "config", "user.email", "test@test.com").Run()
	_ = execwrap.CommandContext(ctx, "git", "-C", tempDir, "config", "user.name", "Test").Run()
	_ = execwrap.CommandContext(ctx, "git", "-C", tempDir, "commit", "--allow-empty", "-m", "init").Run()
	revOut, err := execwrap.CommandContext(ctx, "git", "-C", tempDir, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	commitHash := strings.TrimSpace(string(revOut))

	reqObj := map[string]any{
		objects.FieldKeyID:            "REQ-TEST-001",
		objects.FieldKeyKind:          objects.KindRequirement,
		objects.FieldKeyTitle:         "Test Requirement",
		objects.FieldKeyDescription:   "Requirement for test validation",
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, reqObj))

	critObj := map[string]any{
		objects.FieldKeyID:            "CRIT-TEST-001",
		objects.FieldKeyKind:          objects.KindCriteria,
		objects.FieldKeyTitle:         "Test Criteria",
		objects.FieldKeyDescription:   "Criteria for test validation",
		"category":                    "acceptance",
		objects.FieldKeyStatus:        "awaiting_verification",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, critObj))

	planID := "PRI-DISP-TEST-001"
	plan := map[string]any{
		objects.FieldKeyID:            planID,
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Disposition Plan",
		objects.FieldKeyDescription:   "Active plan for testing disposition branches",
		objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, plan))

	bliDone := map[string]any{
		objects.FieldKeyID:              "BLI-SKIP-DONE-001",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyTitle:           "Task Already Done",
		objects.FieldKeyDescription:     "Done item description for unit test validation",
		objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
		objects.FieldKeyPriorityPlanRef: planID,
		objects.FieldKeyRequirementRefs: []any{"REQ-TEST-001"},
		objects.FieldKeyCriteriaRefs:    []any{"CRIT-TEST-001"},
		objects.FieldKeyEstimatedEffort: "1h",
		objects.FieldKeyPersonaRefs:     []any{objects.ConstPersonaDefaultOperator},
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, bliDone))

	bliReplace := map[string]any{
		objects.FieldKeyID:              "BLI-REPLACE-001",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyTitle:           "Task To Replace",
		objects.FieldKeyDescription:     "Failed item to replace",
		objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
		objects.FieldKeyPriorityPlanRef: planID,
		objects.FieldKeyRequirementRefs: []any{"REQ-TEST-001"},
		objects.FieldKeyCriteriaRefs:    []any{"CRIT-TEST-001"},
		objects.FieldKeyEstimatedEffort: "1h",
		objects.FieldKeyPersonaRefs:     []any{objects.ConstPersonaDefaultOperator},
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, bliReplace))

	atkDone := map[string]any{
		objects.FieldKeyID:                 "ATK-DONE-001",
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Execute Task: Task Already Done",
		objects.FieldKeyDescription:        "Agent task description for testing execution",
		objects.FieldKeyStatus:             objects.ObjectStatusImplemented,
		objects.FieldKeyPriorityPlanRef:    planID,
		objects.FieldKeyBacklogItemRef:     "BLI-SKIP-DONE-001",
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaDefaultOperator,
		objects.FieldKeyCommitHash:         commitHash,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, atkDone))

	atkFailed := map[string]any{
		objects.FieldKeyID:                 "ATK-FAIL-001",
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Execute Task: Task To Replace",
		objects.FieldKeyDescription:        "Agent task description for testing execution",
		objects.FieldKeyStatus:             objects.ObjectStatusError,
		objects.FieldKeyPriorityPlanRef:    planID,
		objects.FieldKeyBacklogItemRef:     "BLI-REPLACE-001",
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaDefaultOperator,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, atkFailed))

	cmd := NewOrchestrateCmd()
	cmd.SetContext(adminCtx)
	err = runOrchestrate(cmd, planID, OrchestrateOptions{})
	assert.NoError(t, err)
}

func TestOrchestrate_HelperFunctions(t *testing.T) {
	// nativeSwarmEligible
	assert.True(t, nativeSwarmEligible("tier_1_routine"))
	assert.True(t, nativeSwarmEligible("TIER_2_SIMPLE"))
	assert.True(t, nativeSwarmEligible("tier_3_light"))
	assert.True(t, nativeSwarmEligible("tier_3_simple"))
	assert.False(t, nativeSwarmEligible("tier_1_complex"))
	assert.False(t, nativeSwarmEligible("unknown"))

	// appendStringReference
	refs := appendStringReference(nil, "ref-1")
	assert.Equal(t, []any{"ref-1"}, refs)
	refs = appendStringReference([]string{"ref-1"}, "ref-2")
	assert.Equal(t, []any{"ref-1", "ref-2"}, refs)
	refs = appendStringReference([]any{"ref-1", "ref-2"}, "ref-1") // already present
	assert.Equal(t, []any{"ref-1", "ref-2"}, refs)

	// buildOrchestrationExecutorArgs
	argsNoTimeout := buildOrchestrationExecutorArgs("ATK-1", "prompt content", 0)
	assert.Equal(t, []string{"agent", "execute", "--task-id", "ATK-1", "--prompt", "prompt content"}, argsNoTimeout)
	argsTimeout := buildOrchestrationExecutorArgs("ATK-1", "prompt content", 30*time.Second)
	assert.Contains(t, argsTimeout, "--timeout")

	// orchestrationExecutorChildEnv
	childEnv := orchestrationExecutorChildEnv([]string{"PATH=/bin"}, "/tmp/seated", "KEY-123", "/bin/zqk")
	assert.NotEmpty(t, childEnv)

	// configureOrchestrationExecutorProcess
	cmd := exec.Command("true")
	configureOrchestrationExecutorProcess(cmd)
	assert.NotNil(t, cmd.SysProcAttr)
	assert.NotNil(t, cmd.Cancel)
	// Cancel when Process == nil returns os.ErrProcessDone
	assert.ErrorIs(t, cmd.Cancel(), os.ErrProcessDone)
}

func TestSeedAgentWorktreeRuntime_WithIndexAndDraft(t *testing.T) {
	mainRoot := t.TempDir()
	worktreeRoot := t.TempDir()

	// Create main process directory with .index and .index.json files
	procDir := filepath.Join(mainRoot, paths.ProcessDir, "backlog_items")
	require.NoError(t, fileutil.EnsureDir(procDir))
	require.NoError(t, fileutil.WriteFile(filepath.Join(procDir, ".index"), []byte("index-data"), paths.FilePerm600))
	require.NoError(t, fileutil.WriteFile(filepath.Join(procDir, ".index.json"), []byte("{}"), paths.FilePerm600))
	require.NoError(t, fileutil.WriteFile(filepath.Join(procDir, "ignored.txt"), []byte("ignore"), paths.FilePerm600))

	// Pre-create worktree draft directory to trigger cleanup/replacement
	draftDir := storage.ObjectDraftPlaneRoot(worktreeRoot)
	require.NoError(t, fileutil.EnsureDir(draftDir))
	require.NoError(t, fileutil.WriteFile(filepath.Join(draftDir, "draft.yaml"), []byte("draft"), paths.FilePerm600))

	err := seedAgentWorktreeRuntime(mainRoot, worktreeRoot)
	require.NoError(t, err)

	// Verify copied index files
	assert.True(t, fileutil.Exists(filepath.Join(worktreeRoot, paths.ProcessDir, "backlog_items", ".index")))
	assert.True(t, fileutil.Exists(filepath.Join(worktreeRoot, paths.ProcessDir, "backlog_items", ".index.json")))
	assert.False(t, fileutil.Exists(filepath.Join(worktreeRoot, paths.ProcessDir, "backlog_items", "ignored.txt")))
}
