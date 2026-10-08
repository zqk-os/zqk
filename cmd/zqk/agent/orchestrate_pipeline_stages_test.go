package agent

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/agentdelivery"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestOrchestrate_StrategicPlanExecution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	stratPlan := map[string]any{
		objects.FieldKeyID:            "STRAT-PLAN-001",
		objects.FieldKeyKind:          objects.KindStrategicPlan,
		objects.FieldKeyTitle:         "Autonomous Test Strategy",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: "2.0.0",
		objects.FieldKeyPersonaRefs:   []any{objects.ConstPersonaDefaultOperator},
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, stratPlan, objects.ObjectStatusActive)

	out, err := executeAgentCommand(t, tempDir, provider, "orchestrate", "STRAT-PLAN-001")
	require.NoError(t, err)
	assert.Contains(t, out, "delivery_result: success")
	assert.Contains(t, out, "status: routed")
	assert.Contains(t, out, "CEO Strategic Review")
}

func TestOrchestrate_SemanticRoutingFailure(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	_, err := executeAgentCommand(t, tempDir, provider, "orchestrate", "INVALID-ROUTING-ARG")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "semantic routing failed")
}

func TestOrchestrate_WithAmbientAndPersona(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	stratPlan := map[string]any{
		objects.FieldKeyID:            "STRAT-PLAN-002",
		objects.FieldKeyKind:          objects.KindStrategicPlan,
		objects.FieldKeyTitle:         "Ambient Strategy",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: "2.0.0",
		objects.FieldKeyPersonaRefs:   []any{objects.ConstPersonaDefaultOperator},
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, stratPlan, objects.ObjectStatusActive)

	ambientFile := filepath.Join(tempDir, "ambient.txt")
	require.NoError(t, fileutil.WriteStandardFile(ambientFile, []byte("IDE Context Signal")))

	out, err := executeAgentCommand(t, tempDir, provider, "orchestrate", "STRAT-PLAN-002",
		"--ambient-context", ambientFile,
		"--persona-id", objects.ConstPersonaDefaultOperator,
	)
	require.NoError(t, err)
	assert.Contains(t, out, "delivery_result: success")
}

type mockStageDeliverer struct {
	mu    sync.Mutex
	calls []agentdelivery.Prompt
}

func (m *mockStageDeliverer) Name() string { return "mock_stage" }
func (m *mockStageDeliverer) Deliver(_ context.Context, p agentdelivery.Prompt) (agentdelivery.Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, p)
	return agentdelivery.Result{DeliveredTo: "mock:stage"}, nil
}

func TestOrchestrate_WithDelivererOverride_Success(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	stratPlan := map[string]any{
		objects.FieldKeyID:            "STRAT-PLAN-DELIVER-OK",
		objects.FieldKeyKind:          objects.KindStrategicPlan,
		objects.FieldKeyTitle:         "Deliverer Success Strategy",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: "2.0.0",
		objects.FieldKeyPersonaRefs:   []any{objects.ConstPersonaDefaultOperator},
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, stratPlan, objects.ObjectStatusActive)

	mock := &mockStageDeliverer{}
	TestDelivererOverride = mock
	defer func() { TestDelivererOverride = nil }()

	out, err := executeAgentCommand(t, tempDir, provider, "orchestrate", "STRAT-PLAN-DELIVER-OK")
	require.NoError(t, err)
	assert.Contains(t, out, "delivery_result: success")
	assert.NotEmpty(t, mock.calls)
}

type mockFailingDeliverer struct{}

func (m *mockFailingDeliverer) Name() string { return "mock_failing" }
func (m *mockFailingDeliverer) Deliver(_ context.Context, _ agentdelivery.Prompt) (agentdelivery.Result, error) {
	return agentdelivery.Result{}, assert.AnError
}

func TestOrchestrate_WithDelivererOverride_Error(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	stratPlan := map[string]any{
		objects.FieldKeyID:            "STRAT-PLAN-DELIVER-FAIL",
		objects.FieldKeyKind:          objects.KindStrategicPlan,
		objects.FieldKeyTitle:         "Deliverer Failure Strategy",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: "2.0.0",
		objects.FieldKeyPersonaRefs:   []any{objects.ConstPersonaDefaultOperator},
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, stratPlan, objects.ObjectStatusActive)

	TestDelivererOverride = &mockFailingDeliverer{}
	defer func() { TestDelivererOverride = nil }()

	_, err := executeAgentCommand(t, tempDir, provider, "orchestrate", "STRAT-PLAN-DELIVER-FAIL")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to deliver agent prompt")
}

func TestOrchestrate_PriorityPlan_NotActive(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	plan := map[string]any{
		objects.FieldKeyID:            "PRI-COMPLETED-ONLY",
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Completed Priority Plan",
		objects.FieldKeyStatus:        objects.ObjectStatusComplete,
		objects.FieldKeySchemaVersion: "2.0.0",
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, plan, objects.ObjectStatusComplete)

	_, err := executeAgentCommand(t, tempDir, provider, "orchestrate", "PRI-COMPLETED-ONLY")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "can only operate on active or in_progress priority plans")
}

func TestOrchestrate_WhatsNextFallback_NoActivePlan(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	_, err := executeAgentCommand(t, tempDir, provider, "orchestrate")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no active priority plan or strategic plan found to orchestrate")
}

func TestOrchestrate_BacklogItem_ShovelReadyGate(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	plan := map[string]any{
		objects.FieldKeyID:             "PRI-SHOVEL-GATE",
		objects.FieldKeyKind:           objects.KindPriorityPlan,
		objects.FieldKeyTitle:          "Shovel Ready Gate Plan",
		objects.FieldKeyStatus:         objects.ObjectStatusGrooming,
		objects.FieldKeySchemaVersion:  "2.0.0",
		objects.FieldKeyPersonaRefs:    []any{objects.ConstPersonaDefaultOperator},
		objects.FieldKeyWorkstreamRefs: []any{"WS-001"},
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, plan, objects.ObjectStatusGrooming)

	bli := map[string]any{
		objects.FieldKeyID:              "BLI-UNSHOVEL-001",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyTitle:           "Unshovel Ready BLI",
		objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
		objects.FieldKeyPriorityPlanRef: "PRI-SHOVEL-GATE",
		objects.FieldKeySchemaVersion:   "2.0.0",
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, bli, objects.ObjectStatusPlanned)

	plan[objects.FieldKeyStatus] = objects.ObjectStatusActive
	plan[objects.FieldKeyActiveOrder] = 1
	require.NoError(t, provider.Update(ctx, secCtx, "PRI-SHOVEL-GATE", plan))

	out, err := executeAgentCommand(t, tempDir, provider, "orchestrate", "PRI-SHOVEL-GATE")
	require.NoError(t, err)
	assert.Contains(t, out, "Shovel-ready gate")
}

func TestOrchestrate_BacklogItem_ComplexTier(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	req := map[string]any{
		objects.FieldKeyID:            "REQ-001",
		objects.FieldKeyKind:          objects.KindRequirement,
		objects.FieldKeyTitle:         "Requirement 1",
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemaVersion: "2.0.0",
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, req, "active")

	cri := map[string]any{
		objects.FieldKeyID:            "CRI-001",
		objects.FieldKeyKind:          objects.KindCriteria,
		objects.FieldKeyTitle:         "Test Criteria",
		objects.FieldKeyCategory:      "functional",
		objects.FieldKeyStatus:        "awaiting_verification",
		objects.FieldKeySchemaVersion: "2.0.0",
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, cri, "awaiting_verification")

	plan := map[string]any{
		objects.FieldKeyID:             "PRI-COMPLEX-TIER",
		objects.FieldKeyKind:           objects.KindPriorityPlan,
		objects.FieldKeyTitle:          "Complex Tier Plan",
		objects.FieldKeyStatus:         objects.ObjectStatusGrooming,
		objects.FieldKeySchemaVersion:  "2.0.0",
		objects.FieldKeyPersonaRefs:    []any{objects.ConstPersonaDefaultOperator},
		objects.FieldKeyWorkstreamRefs: []any{"WS-001"},
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, plan, objects.ObjectStatusGrooming)

	bli := map[string]any{
		objects.FieldKeyID:              "BLI-COMPLEX-001",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyTitle:           "Complex Unit of Work",
		objects.FieldKeyDescription:     "Complex architecture refactor",
		objects.FieldKeyRequirementRefs: []any{"REQ-001"},
		objects.FieldKeyCriteriaRefs:    []any{"CRI-001"},
		objects.FieldKeyEstimatedEffort: "1d",
		objects.FieldKeyPersonaRef:      objects.ConstPersonaDefaultOperator,
		objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
		objects.FieldKeyPriorityPlanRef: "PRI-COMPLEX-TIER",
		objects.FieldKeyModelTier:       "tier_1_complex",
		objects.FieldKeySchemaVersion:   "2.0.0",
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, bli, objects.ObjectStatusPlanned)

	plan[objects.FieldKeyStatus] = objects.ObjectStatusActive
	plan[objects.FieldKeyActiveOrder] = 1
	require.NoError(t, provider.Update(ctx, secCtx, "PRI-COMPLEX-TIER", plan))

	out, err := executeAgentCommand(t, tempDir, provider, "orchestrate", "PRI-COMPLEX-TIER")
	require.NoError(t, err)
	assert.Contains(t, out, "skipped native swarm for tier_1_complex")
}

func TestOrchestrate_ExistingATK_SkipDone(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()
	commitHash := initGitRepoInDir(t, tempDir)

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	req := map[string]any{
		objects.FieldKeyID:            "REQ-001",
		objects.FieldKeyKind:          objects.KindRequirement,
		objects.FieldKeyTitle:         "Requirement 1",
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemaVersion: "2.0.0",
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, req, "active")

	cri := map[string]any{
		objects.FieldKeyID:            "CRI-001",
		objects.FieldKeyKind:          objects.KindCriteria,
		objects.FieldKeyTitle:         "Test Criteria",
		objects.FieldKeyCategory:      "functional",
		objects.FieldKeyStatus:        "awaiting_verification",
		objects.FieldKeySchemaVersion: "2.0.0",
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, cri, "awaiting_verification")

	plan := map[string]any{
		objects.FieldKeyID:             "PRI-EXISTING-ATK",
		objects.FieldKeyKind:           objects.KindPriorityPlan,
		objects.FieldKeyTitle:          "Existing ATK Plan",
		objects.FieldKeyStatus:         objects.ObjectStatusGrooming,
		objects.FieldKeySchemaVersion:  "2.0.0",
		objects.FieldKeyPersonaRefs:    []any{objects.ConstPersonaDefaultOperator},
		objects.FieldKeyWorkstreamRefs: []any{"WS-001"},
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, plan, objects.ObjectStatusGrooming)

	bli := map[string]any{
		objects.FieldKeyID:              "BLI-EXISTING-001",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyTitle:           "Existing Work",
		objects.FieldKeyDescription:     "Historic task",
		objects.FieldKeyRequirementRefs: []any{"REQ-001"},
		objects.FieldKeyCriteriaRefs:    []any{"CRI-001"},
		objects.FieldKeyEstimatedEffort: "1d",
		objects.FieldKeyPersonaRef:      objects.ConstPersonaDefaultOperator,
		objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
		objects.FieldKeyPriorityPlanRef: "PRI-EXISTING-ATK",
		objects.FieldKeyModelTier:       "tier_1_complex",
		objects.FieldKeySchemaVersion:   "2.0.0",
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, bli, objects.ObjectStatusPlanned)

	plan[objects.FieldKeyStatus] = objects.ObjectStatusActive
	plan[objects.FieldKeyActiveOrder] = 1
	require.NoError(t, provider.Update(ctx, secCtx, "PRI-EXISTING-ATK", plan))

	atk := map[string]any{
		objects.FieldKeyID:                 "ATK-HISTORIC-DONE",
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Execute Task: Existing Work",
		objects.FieldKeyStatus:             objects.ObjectStatusImplemented,
		objects.FieldKeyPriorityPlanRef:    "PRI-EXISTING-ATK",
		objects.FieldKeySchemaVersion:      "2.0.0",
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaDefaultOperator,
		objects.FieldKeyCommitHash:         commitHash,
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, atk, objects.ObjectStatusImplemented)

	out, err := executeAgentCommand(t, tempDir, provider, "orchestrate", "PRI-EXISTING-ATK")
	require.NoError(t, err)
	assert.Contains(t, out, "is implemented")
}
