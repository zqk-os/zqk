package lifecycle

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
)

func TestApplyComputeHooks_BacklogItem_Complete(t *testing.T) {
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	id := "BL-123"
	obj := map[string]any{
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyID:              id,
		objects.FieldKeyEstimatedEffort: "10.0",
		objects.FieldKeyActualEffort:    "15.0",
		objects.FieldKeyStatus:          "in_progress",
	}

	err := realStorage.Create(ctx, secCtx, obj)
	if err != nil {
		t.Fatalf("failed to create test object: %v", err)
	}

	req := TransitionRequest{
		Kind:     objects.KindBacklogItem,
		ID:       id,
		ToStatus: statusComplete,
	}

	updates := map[string]any{
		objects.FieldKeyStatus: statusComplete,
	}

	ApplyComputeHooks(ctx, realStorage, secCtx, req, updates)

	// variance = ((15.0 - 10.0) / 10.0) * 100 = 50.0
	val, ok := updates[objects.FieldKeyEffortVariance]
	if !ok {
		t.Fatalf("expected effort_variance in updates")
	}
	if val != 50.0 {
		t.Errorf("expected 50.0, got %v", val)
	}
}

func TestApplyComputeHooks_NotComplete(t *testing.T) {
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	id := "BL-123"
	obj := map[string]any{
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyID:              id,
		objects.FieldKeyEstimatedEffort: "10.0",
		objects.FieldKeyActualEffort:    "15.0",
		objects.FieldKeyStatus:          "in_progress",
	}

	err := realStorage.Create(ctx, secCtx, obj)
	if err != nil {
		t.Fatalf("failed to create test object: %v", err)
	}

	req := TransitionRequest{
		Kind:     objects.KindBacklogItem,
		ID:       id,
		ToStatus: "in_progress", // not complete
	}

	updates := make(map[string]any)

	ApplyComputeHooks(ctx, realStorage, secCtx, req, updates)

	if _, ok := updates[objects.FieldKeyEffortVariance]; ok {
		t.Errorf("expected no effort_variance in updates")
	}
}
