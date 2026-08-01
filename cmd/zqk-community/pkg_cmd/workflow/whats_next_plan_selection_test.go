package workflow

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// TestResolvePriorityPlan_PrefersPlansWithWork verifies that plan selection
// prefers in_progress plans that actually have BLIs linked, over empty plans.
func TestResolvePriorityPlan_PrefersPlansWithWork(t *testing.T) {
	ctx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{
		AccountID: "account:system",
	})

	store := newMemoryWorkflowStore(
		// Empty plan — in_progress but no BLIs
		map[string]any{
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PLAN-EMPTY",
			objects.FieldKeyTitle:  "Empty Plan",
			objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		},
		// Plan with work — in_progress and has BLIs
		map[string]any{
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PLAN-HAS-WORK",
			objects.FieldKeyTitle:  "Plan With Work",
			objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		},
		// BLIs linked to PLAN-HAS-WORK
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "ITEM-1",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: "PLAN-HAS-WORK",
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "ITEM-2",
			objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
			objects.FieldKeyPriorityPlanRef: "PLAN-HAS-WORK",
		},
	)

	planID, summ, _ := resolvePriorityPlanForWhatsNext(ctx, store, "", nil)
	if planID != "PLAN-HAS-WORK" {
		t.Fatalf("expected PLAN-HAS-WORK (has BLIs), got %v (title=%v)", planID, summ)
	}
}

// TestResolvePriorityPlan_FallsBackToEmptyPlanWhenNoWorkAnywhere
// verifies that when all plans are empty, we still return a plan.
func TestResolvePriorityPlan_FallsBackToEmptyPlan(t *testing.T) {
	ctx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{
		AccountID: "account:system",
	})

	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PLAN-EMPTY",
			objects.FieldKeyTitle:  "Empty But Only Option",
			objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		},
	)

	planID, _, _ := resolvePriorityPlanForWhatsNext(ctx, store, "", nil)
	if planID != "PLAN-EMPTY" {
		t.Fatalf("expected PLAN-EMPTY as fallback, got %v", planID)
	}
}

// TestResolvePriorityPlan_ExplicitOverridesWorkCheck verifies that
// an explicitly requested plan is always used, even if empty.
func TestResolvePriorityPlan_ExplicitOverridesWorkCheck(t *testing.T) {
	ctx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{
		AccountID: "account:system",
	})

	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PLAN-EXPLICIT",
			objects.FieldKeyTitle:  "Explicitly Requested",
			objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		},
		map[string]any{
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PLAN-HAS-WORK",
			objects.FieldKeyTitle:  "Has Work",
			objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "ITEM-1",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: "PLAN-HAS-WORK",
		},
	)

	planID, _, _ := resolvePriorityPlanForWhatsNext(ctx, store, "PLAN-EXPLICIT", nil)
	if planID != "PLAN-EXPLICIT" {
		t.Fatalf("explicit plan should always be used, got %v", planID)
	}
}

// TestCountBacklogByStatus_ReturnsCorrectCounts verifies counting works.
func TestCountBacklogByStatus_ReturnsCorrectCounts(t *testing.T) {
	ctx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{
		AccountID: "account:system",
	})

	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "ITEM-1",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: "PLAN-1",
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "ITEM-2",
			objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
			objects.FieldKeyPriorityPlanRef: "PLAN-1",
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "ITEM-3",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: "PLAN-OTHER",
		},
	)

	counts := countBacklogByStatus(ctx, store, "PLAN-1", nil)
	if counts["planned"] != 1 {
		t.Fatalf("expected 1 planned, got %d", counts["planned"])
	}
	if counts["in_progress"] != 1 {
		t.Fatalf("expected 1 in_progress, got %d", counts["in_progress"])
	}
	total := 0
	for _, v := range counts {
		total += v
	}
	if total != 2 {
		t.Fatalf("expected 2 total BLIs for PLAN-1, got %d", total)
	}
}
