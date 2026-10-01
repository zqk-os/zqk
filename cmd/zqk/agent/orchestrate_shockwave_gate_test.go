package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type mockShockwaveGateStore struct {
	storage.ObjectStorageProvider
	objs map[string]map[string]any
}

func (m *mockShockwaveGateStore) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.objs[id]; ok {
		return obj, nil
	}
	return nil, errfmt.Errorf("not found: %s", id)
}

const (
	testUpstreamID        = "BLI-SPECIALIST-EVAL-001"
	testUpstreamTitle     = "Evaluate Security Findings"
	testDownstreamID      = "BLI-ADVERSARIAL-CRITIQUE-001"
	testDownstreamTitle   = "Adversarial Critique of Security Findings"
	testIndependentID     = "BLI-INDEPENDENT-TASK-001"
	testIndependentTitle  = "Independent Scaffolding Task"
	testArtifactPath      = ".zqk/runs/code-eval/findings/findings_SEC.jsonl"
	testErrStage1Withheld = "downstream critique task must be withheld while upstream is in_progress"
	testErrStage1Indep    = "independent task must not be blocked"
	testErrStage2NoArt    = "downstream critique task must be withheld if upstream complete without artifacts"
	testErrStage3Pass     = "downstream critique task must be unblocked once shockwave delivers verified upstream artifacts"
)

// Satisfies CRIT-QA-SHOCKWAVE-EVENT-DISPATCH:
// Swarm orchestrator must withhold downstream dependent task dispatch until a shockwave event
// delivers verified upstream artifacts, preventing blind critiques of unproduced findings.
func TestOrchestrate_ShockwaveDependencyGate(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	store := &mockShockwaveGateStore{
		objs: make(map[string]map[string]any),
	}

	// 1. Upstream Specialist Finding Task (in_progress, no artifacts yet)
	store.objs[testUpstreamID] = map[string]any{
		objects.FieldKeyID:        testUpstreamID,
		objects.FieldKeyKind:      objects.KindBacklogItem,
		objects.FieldKeyTitle:     testUpstreamTitle,
		objects.FieldKeyStatus:    objects.ObjectStatusInProgress,
		objects.FieldKeyArtifacts: []any{},
	}

	// Downstream Adversarial Critique Task (depends_on upstream specialist task)
	downstreamItem := map[string]any{
		objects.FieldKeyID:     testDownstreamID,
		objects.FieldKeyKind:   objects.KindBacklogItem,
		objects.FieldKeyTitle:  testDownstreamTitle,
		objects.FieldKeyStatus: objects.ObjectStatusPlanned,
		"depends_on":           []any{testUpstreamID},
	}

	// Independent Task without upstream dependencies
	independentItem := map[string]any{
		objects.FieldKeyID:     testIndependentID,
		objects.FieldKeyKind:   objects.KindBacklogItem,
		objects.FieldKeyTitle:  testIndependentTitle,
		objects.FieldKeyStatus: objects.ObjectStatusPlanned,
	}

	// Stage 1 Verification: Downstream task MUST be withheld when upstream is in_progress
	blocked := isBlockedByUnverifiedUpstream(ctx, store, secCtx, downstreamItem)
	require.True(t, blocked, testErrStage1Withheld)

	// Independent task must NOT be blocked
	indepBlocked := isBlockedByUnverifiedUpstream(ctx, store, secCtx, independentItem)
	require.False(t, indepBlocked, testErrStage1Indep)

	// Stage 2: Upstream task reaches complete but artifacts are still missing
	store.objs[testUpstreamID][objects.FieldKeyStatus] = objects.ObjectStatusComplete
	blockedNoArtifacts := isBlockedByUnverifiedUpstream(ctx, store, secCtx, downstreamItem)
	require.True(t, blockedNoArtifacts, testErrStage2NoArt)

	// Stage 3: Upstream task receives shockwave verified status transition delivering artifacts
	store.objs[testUpstreamID][objects.FieldKeyArtifacts] = []any{
		testArtifactPath,
	}
	unblocked := isBlockedByUnverifiedUpstream(ctx, store, secCtx, downstreamItem)
	require.False(t, unblocked, testErrStage3Pass)

	// Stage 4 Verification (CRIT-SWARM-PROVENANCE-001):
	// Downstream tasks receive verified upstream deliverable artifact paths in their task prompt context.
	deliverables := resolveVerifiedUpstreamDeliverables(ctx, store, secCtx, downstreamItem)
	require.Contains(t, deliverables, "## Upstream Verified Deliverables")
	require.Contains(t, deliverables, testArtifactPath)
	require.Contains(t, deliverables, testUpstreamID)
}
