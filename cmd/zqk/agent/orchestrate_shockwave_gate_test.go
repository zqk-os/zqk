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
	upstreamID := "BLI-SPECIALIST-EVAL-001"
	store.objs[upstreamID] = map[string]any{
		objects.FieldKeyID:        upstreamID,
		objects.FieldKeyKind:      objects.KindBacklogItem,
		objects.FieldKeyTitle:     "Evaluate Security Findings",
		objects.FieldKeyStatus:    objects.ObjectStatusInProgress,
		objects.FieldKeyArtifacts: []any{},
	}

	// Downstream Adversarial Critique Task (depends_on upstream specialist task)
	downstreamItem := map[string]any{
		objects.FieldKeyID:     "BLI-ADVERSARIAL-CRITIQUE-001",
		objects.FieldKeyKind:   objects.KindBacklogItem,
		objects.FieldKeyTitle:  "Adversarial Critique of Security Findings",
		objects.FieldKeyStatus: objects.ObjectStatusPlanned,
		"depends_on":           []any{upstreamID},
	}

	// Independent Task without upstream dependencies
	independentItem := map[string]any{
		objects.FieldKeyID:     "BLI-INDEPENDENT-TASK-001",
		objects.FieldKeyKind:   objects.KindBacklogItem,
		objects.FieldKeyTitle:  "Independent Scaffolding Task",
		objects.FieldKeyStatus: objects.ObjectStatusPlanned,
	}

	// Stage 1 Verification: Downstream task MUST be withheld when upstream is in_progress
	blocked := isBlockedByUnverifiedUpstream(ctx, store, secCtx, downstreamItem)
	require.True(t, blocked, "downstream critique task must be withheld while upstream is in_progress")

	// Independent task must NOT be blocked
	indepBlocked := isBlockedByUnverifiedUpstream(ctx, store, secCtx, independentItem)
	require.False(t, indepBlocked, "independent task must not be blocked")

	// Stage 2: Upstream task reaches complete but artifacts are still missing
	store.objs[upstreamID][objects.FieldKeyStatus] = objects.ObjectStatusComplete
	blockedNoArtifacts := isBlockedByUnverifiedUpstream(ctx, store, secCtx, downstreamItem)
	require.True(t, blockedNoArtifacts, "downstream critique task must be withheld if upstream complete without artifacts")

	// Stage 3: Upstream task receives shockwave verified status transition delivering artifacts
	store.objs[upstreamID][objects.FieldKeyArtifacts] = []any{
		".zqk/runs/code-eval/findings/findings_SEC.jsonl",
	}
	unblocked := isBlockedByUnverifiedUpstream(ctx, store, secCtx, downstreamItem)
	require.False(t, unblocked, "downstream critique task must be unblocked once shockwave delivers verified upstream artifacts")
}
