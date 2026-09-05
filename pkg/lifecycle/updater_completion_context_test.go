package lifecycle

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// The all_backlog_items_complete_for_plan criterion fired and the updater still could not close
// the plan: DECIDE rejected break_glass on the critical kind because no elevation was stamped
// ("break_glass requires --reason-code for critical kind priority_plan"). The storage guard is
// inert under ZQK_TEST_ROOT, so the contract is asserted on the context the updater builds.
// TRACK: REDACTED
func TestCompletionOverrideContext(t *testing.T) {
	t.Parallel()

	secCtx := pkgctx.NewSystemSecurityContext()

	t.Run("plan_complete_arms_break_glass_and_elevation", func(t *testing.T) {
		t.Parallel()
		ctx := completionOverrideContext(t.Context(), secCtx, TransitionRequest{
			Kind:     objects.KindPriorityPlan,
			ID:       "PRI-TEST-0001",
			ToStatus: statusComplete,
		})
		if !pkgctx.IsLifecycleBreakGlass(ctx) {
			t.Error("expected break_glass armed for priority_plan → complete")
		}
		if !pkgctx.GetAllowCoreObjectDelete(ctx) {
			t.Error("expected elevation stamped so DECIDE accepts break_glass on a critical kind")
		}
	})

	t.Run("backlog_item_complete_arms_break_glass", func(t *testing.T) {
		t.Parallel()
		ctx := completionOverrideContext(t.Context(), secCtx, TransitionRequest{
			Kind:     objects.KindBacklogItem,
			ID:       "BLI-TEST-0001",
			ToStatus: statusComplete,
		})
		if !pkgctx.IsLifecycleBreakGlass(ctx) {
			t.Error("expected break_glass armed for backlog_item → complete")
		}
	})

	t.Run("complete_declares_intent_even_for_unelevated_actor", func(t *testing.T) {
		t.Parallel()
		ctx := completionOverrideContext(t.Context(), &pkgctx.SecurityContext{AccountID: "ACC-TEST"}, TransitionRequest{
			Kind:     objects.KindPriorityPlan,
			ID:       "PRI-TEST-0002",
			ToStatus: statusComplete,
		})
		// Declared intent is the membrane (reason on the transition), not actor privilege.
		// Inferring allow from elevation is the launder the core-delete guard removed.
		if !pkgctx.GetAllowCoreObjectDelete(ctx) {
			t.Error("priority_plan → complete must declare intent even when the actor is not elevated")
		}
	})

	t.Run("other_transitions_untouched", func(t *testing.T) {
		t.Parallel()
		for _, req := range []TransitionRequest{
			{Kind: objects.KindPriorityPlan, ID: "PRI-TEST-0003", ToStatus: objects.ObjectStatusActive},
			{Kind: objects.KindMilestone, ID: "MIL-TEST-0001", ToStatus: statusComplete},
		} {
			ctx := completionOverrideContext(t.Context(), secCtx, req)
			if pkgctx.IsLifecycleBreakGlass(ctx) {
				t.Errorf("break_glass must stay disarmed for %s → %s", req.Kind, req.ToStatus)
			}
		}
	})
}
