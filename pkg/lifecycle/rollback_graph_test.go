package lifecycle

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/rollback"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
)

func TestRecomputeRefsFromScope_NilProvider(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	refs := RecomputeRefsFromScope(ctx, rollback.ScopeTypeLifecycle, "priority_plan:PRI-1:complete", nil)
	if refs != nil {
		t.Errorf("RecomputeRefsFromScope(nil provider): got %+v, want nil", refs)
	}
}

func TestRecomputeRefsFromScope_WrongScopeType(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	refs := RecomputeRefsFromScope(ctx, "maintenance", "task-1", nil)
	if refs != nil {
		t.Errorf("RecomputeRefsFromScope(maintenance): got %+v, want nil", refs)
	}
}

func TestRecomputeRefsFromScope_EmptyScopeID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	refs := RecomputeRefsFromScope(ctx, rollback.ScopeTypeLifecycle, "", nil)
	if refs != nil {
		t.Errorf("RecomputeRefsFromScope(empty scopeID): got %+v, want nil", refs)
	}
}

func TestRecomputeRefsFromScope_MalformedScopeID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	refs := RecomputeRefsFromScope(ctx, rollback.ScopeTypeLifecycle, "only:two", nil)
	if refs != nil {
		t.Errorf("RecomputeRefsFromScope(malformed): got %+v, want nil", refs)
	}
}

func TestRecomputeRefsFromScope_Lifecycle_BacklogItem(t *testing.T) {
	// TRACK: REDACTED — no t.Parallel: shared Memgraph + fixed fixture IDs.
	ctx := context.Background()
	// backlog_item has no List expansion; should return single ref
	refs := RecomputeRefsFromScope(ctx, rollback.ScopeTypeLifecycle, "backlog_item:BLI-1:complete", nil)
	if refs != nil {
		t.Errorf("RecomputeRefsFromScope(backlog_item with nil provider): got %+v, want nil (provider required for list)", refs)
	}
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	refs = RecomputeRefsFromScope(ctx, rollback.ScopeTypeLifecycle, "backlog_item:BLI-1:complete", realStorage)
	if len(refs) != 1 || refs[0].Kind != "backlog_item" || refs[0].ID != "BLI-1" {
		t.Errorf("RecomputeRefsFromScope(backlog_item): got %+v, want single BLI-1", refs)
	}
}

func TestRecomputeRefsFromScope_Lifecycle_PriorityPlan(t *testing.T) {
	// TRACK: REDACTED — no t.Parallel: shared Memgraph + fixed fixture IDs.
	ctx := context.Background()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	sysCtx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	planID := fixtureID(t, "PRI")
	bli1 := fixtureID(t, "BLI")
	bli2 := fixtureID(t, "BLI")

	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID: planID, objects.FieldKeyKind: "priority_plan",
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID: bli1, objects.FieldKeyKind: "backlog_item", objects.FieldKeyStatus: "planned", objects.FieldKeyPriorityPlanRef: planID,
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID: bli2, objects.FieldKeyKind: "backlog_item", objects.FieldKeyStatus: "planned", objects.FieldKeyPriorityPlanRef: planID,
	})

	refs := RecomputeRefsFromScope(ctx, rollback.ScopeTypeLifecycle, "priority_plan:"+planID+":complete", realStorage)
	if len(refs) != 3 {
		t.Fatalf("RecomputeRefsFromScope(priority_plan): got %d refs, want 3", len(refs))
	}
	ids := make(map[string]bool)
	for _, r := range refs {
		ids[r.ID] = true
	}
	if !ids[planID] || !ids[bli1] || !ids[bli2] {
		t.Errorf("RecomputeRefsFromScope: got refs %+v", refs)
	}
}

func TestStatusRelevantRefs_NonPriorityPlan(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	req := TransitionRequest{Kind: "backlog_item", ID: "BLI-1", ToStatus: "complete"}
	refs := StatusRelevantRefs(ctx, nil, req)
	if len(refs) != 1 || refs[0].Kind != "backlog_item" || refs[0].ID != "BLI-1" {
		t.Errorf("StatusRelevantRefs(backlog_item): got %+v, want single ref BLI-1", refs)
	}
}

func TestStatusRelevantRefs_PriorityPlan_EmptyID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	req := TransitionRequest{Kind: "priority_plan", ID: "", ToStatus: "complete"}
	refs := StatusRelevantRefs(ctx, nil, req)
	if len(refs) != 1 || refs[0].ID != emptyValue {
		t.Errorf("StatusRelevantRefs(priority_plan, empty id): got %+v", refs)
	}
}

func TestStatusRelevantRefs_PriorityPlan_WithList(t *testing.T) {
	// TRACK: REDACTED — no t.Parallel: shared Memgraph + fixed fixture IDs.
	ctx := context.Background()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	sysCtx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	planID := fixtureID(t, "PRI")
	bli1 := fixtureID(t, "BLI")
	bli2 := fixtureID(t, "BLI")

	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID: planID, objects.FieldKeyKind: "priority_plan",
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID: bli1, objects.FieldKeyKind: "backlog_item", objects.FieldKeyStatus: "planned", objects.FieldKeyPriorityPlanRef: planID,
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID: bli2, objects.FieldKeyKind: "backlog_item", objects.FieldKeyStatus: "planned", objects.FieldKeyPriorityPlanRef: planID,
	})

	req := TransitionRequest{Kind: "priority_plan", ID: planID, ToStatus: "complete"}
	refs := StatusRelevantRefs(ctx, realStorage, req)
	// Primary + 2 backlog items
	if len(refs) != 3 {
		t.Fatalf("StatusRelevantRefs(priority_plan): got %d refs, want 3 (%+v)", len(refs), refs)
	}
	ids := make(map[string]bool)
	for _, r := range refs {
		ids[r.ID] = true
	}
	if !ids[planID] || !ids[bli1] || !ids[bli2] {
		t.Errorf("StatusRelevantRefs: got refs %+v", refs)
	}
}

func TestStatusRelevantRefs_PriorityPlan_ListError(t *testing.T) {
	// TRACK: REDACTED — no t.Parallel: shared Memgraph + fixed fixture IDs.
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	// no backlog items created
	ctx := context.Background()
	req := TransitionRequest{Kind: "priority_plan", ID: "PRI-ERR", ToStatus: "complete"}
	refs := StatusRelevantRefs(ctx, realStorage, req)
	// When provider is nil we don't call List; refs should still have the primary
	if len(refs) != 1 || refs[0].ID != "PRI-ERR" {
		t.Errorf("StatusRelevantRefs(priority_plan, error case): got %+v", refs)
	}
}
