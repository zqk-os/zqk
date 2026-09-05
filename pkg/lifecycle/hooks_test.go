package lifecycle

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
)

func TestEffortStringUnset(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"", true},
		{"0", true},
		{"0.0", true},
		{"0h", true},
		{"10h", false},
		{"15.0", false},
	} {
		if got := effortStringUnset(tc.in); got != tc.want {
			t.Errorf("effortStringUnset(%q)=%v want %v", tc.in, got, tc.want)
		}
	}
}

func TestApplyComputeHooks_BacklogItem_Complete(t *testing.T) {
	// TRACK: REDACTED — BL-123 is not a valid backlog_item id prefix.
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	id := fixtureID(t, "BLI")
	obj := map[string]any{
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyID:              id,
		objects.FieldKeyEstimatedEffort: "10h",
		objects.FieldKeyActualEffort:    "15h",
		objects.FieldKeyStatus:          "in_progress",
	}
	mustCreateCASVisible(t, realStorage, ctx, secCtx, obj)

	req := TransitionRequest{
		Kind:     objects.KindBacklogItem,
		ID:       id,
		ToStatus: statusComplete,
	}

	updates := map[string]any{
		objects.FieldKeyStatus:          statusComplete,
		objects.FieldKeyEstimatedEffort: "10h",
		objects.FieldKeyActualEffort:    "15h",
	}

	ApplyComputeHooks(ctx, realStorage, secCtx, req, updates)

	// variance = ((15 - 10) / 10) * 100 = 50. Effort on updates is the transition payload;
	// graph persist of actual_effort can default to 0 and is not what this hook test covers.
	val, ok := updates[objects.FieldKeyEffortVariance]
	if !ok {
		t.Fatalf("expected effort_variance in updates")
	}
	if val != 50.0 {
		t.Errorf("expected 50.0, got %v", val)
	}
}

func TestApplyComputeHooks_NotComplete(t *testing.T) {
	// TRACK: REDACTED — BL-123 is not a valid backlog_item id prefix.
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	id := fixtureID(t, "BLI")
	obj := map[string]any{
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyID:              id,
		objects.FieldKeyEstimatedEffort: "10.0",
		objects.FieldKeyActualEffort:    "15.0",
		objects.FieldKeyStatus:          "in_progress",
	}
	mustCreateCASVisible(t, realStorage, ctx, secCtx, obj)

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

func TestApplyComputeHooks_BacklogItem_Complete_AutoFillActualEffort(t *testing.T) {
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	id := fixtureID(t, "BLI")
	obj := map[string]any{
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyID:              id,
		objects.FieldKeyEstimatedEffort: "10.0",
		objects.FieldKeyStatus:          "in_progress",
	}
	mustCreateCASVisible(t, realStorage, ctx, secCtx, obj)

	req := TransitionRequest{
		Kind:     objects.KindBacklogItem,
		ID:       id,
		ToStatus: statusComplete,
	}

	updates := map[string]any{
		objects.FieldKeyStatus: statusComplete,
	}

	ApplyComputeHooks(ctx, realStorage, secCtx, req, updates)

	val, ok := updates[objects.FieldKeyActualEffort]
	if !ok {
		t.Fatalf("expected actual_effort in updates")
	}
	if val != "10.0" {
		t.Errorf("expected '10.0', got %v", val)
	}
}
