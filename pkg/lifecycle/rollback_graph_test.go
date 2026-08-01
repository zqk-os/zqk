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
	refs := RecomputeRefsFromScope(ctx, rollback.ScopeTypeLifecycle, "priority_plan:PLAN-1:complete", nil)
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
	t.Parallel()
	ctx := context.Background()
	// backlog_item has no List expansion; should return single ref
	refs := RecomputeRefsFromScope(ctx, rollback.ScopeTypeLifecycle, "backlog_item:ITEM-1:complete", nil)
	if refs != nil {
		t.Errorf("RecomputeRefsFromScope(backlog_item with nil provider): got %+v, want nil (provider required for list)", refs)
	}
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	refs = RecomputeRefsFromScope(ctx, rollback.ScopeTypeLifecycle, "backlog_item:ITEM-1:complete", realStorage)
	if len(refs) != 1 || refs[0].Kind != "backlog_item" || refs[0].ID != "ITEM-1" {
		t.Errorf("RecomputeRefsFromScope(backlog_item): got %+v, want single ITEM-1", refs)
	}
}

func TestRecomputeRefsFromScope_Lifecycle_PriorityPlan(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	sysCtx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	_ = realStorage.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID: "PLAN-1", objects.FieldKeyKind: "priority_plan",
	})
	_ = realStorage.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID: "ITEM-1", objects.FieldKeyKind: "backlog_item", objects.FieldKeyPriorityPlanRef: "PLAN-1",
	})
	_ = realStorage.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID: "ITEM-2", objects.FieldKeyKind: "backlog_item", objects.FieldKeyPriorityPlanRef: "PLAN-1",
	})

	refs := RecomputeRefsFromScope(ctx, rollback.ScopeTypeLifecycle, "priority_plan:PLAN-1:complete", realStorage)
	if len(refs) != 3 {
		t.Fatalf("RecomputeRefsFromScope(priority_plan): got %d refs, want 3", len(refs))
	}
	ids := make(map[string]bool)
	for _, r := range refs {
		ids[r.ID] = true
	}
	if !ids["PLAN-1"] || !ids["ITEM-1"] || !ids["ITEM-2"] {
		t.Errorf("RecomputeRefsFromScope: got refs %+v", refs)
	}
}

func TestStatusRelevantRefs_NonPriorityPlan(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	req := TransitionRequest{Kind: "backlog_item", ID: "ITEM-1", ToStatus: "complete"}
	refs := StatusRelevantRefs(ctx, nil, req)
	if len(refs) != 1 || refs[0].Kind != "backlog_item" || refs[0].ID != "ITEM-1" {
		t.Errorf("StatusRelevantRefs(backlog_item): got %+v, want single ref ITEM-1", refs)
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
	t.Parallel()
	ctx := context.Background()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	sysCtx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	_ = realStorage.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID: "PLAN-1", objects.FieldKeyKind: "priority_plan",
	})
	_ = realStorage.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID: "ITEM-1", objects.FieldKeyKind: "backlog_item", objects.FieldKeyPriorityPlanRef: "PLAN-1",
	})
	_ = realStorage.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID: "ITEM-2", objects.FieldKeyKind: "backlog_item", objects.FieldKeyPriorityPlanRef: "PLAN-1",
	})

	req := TransitionRequest{Kind: "priority_plan", ID: "PLAN-1", ToStatus: "complete"}
	refs := StatusRelevantRefs(ctx, realStorage, req)
	// Primary + 2 backlog items
	if len(refs) != 3 {
		t.Fatalf("StatusRelevantRefs(priority_plan): got %d refs, want 3", len(refs))
	}
	ids := make(map[string]bool)
	for _, r := range refs {
		ids[r.ID] = true
	}
	if !ids["PLAN-1"] || !ids["ITEM-1"] || !ids["ITEM-2"] {
		t.Errorf("StatusRelevantRefs: got refs %+v", refs)
	}
}

func TestStatusRelevantRefs_PriorityPlan_ListError(t *testing.T) {
	t.Parallel()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	// no backlog items created
	ctx := context.Background()
	req := TransitionRequest{Kind: "priority_plan", ID: "PLAN-ERR", ToStatus: "complete"}
	refs := StatusRelevantRefs(ctx, realStorage, req)
	// When provider is nil we don't call List; refs should still have the primary
	if len(refs) != 1 || refs[0].ID != "PLAN-ERR" {
		t.Errorf("StatusRelevantRefs(priority_plan, error case): got %+v", refs)
	}
}
