package workflow

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestResolvePriorityPlan_PrefersPlansWithWork verifies that plan selection
// prefers in_progress plans that actually have BLIs linked, over empty plans.
func TestResolvePriorityPlan_PrefersPlansWithWork(t *testing.T) {
	ctx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{
		AccountID: "ACC-SYSTEM",
	})

	store := newMemoryWorkflowStore(
		// Empty plan — in_progress but no BLIs
		map[string]any{
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-EMPTY",
			objects.FieldKeyTitle:  "Empty Plan",
			objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		},
		// Plan with work — in_progress and has BLIs
		map[string]any{
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-HAS-WORK",
			objects.FieldKeyTitle:  "Plan With Work",
			objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		},
		// BLIs linked to PRI-HAS-WORK
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-1",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: "PRI-HAS-WORK",
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-2",
			objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
			objects.FieldKeyPriorityPlanRef: "PRI-HAS-WORK",
		},
	)

	planID, summ, _ := resolvePriorityPlanForWhatsNext(ctx, store, "", nil)
	if planID != "PRI-HAS-WORK" {
		t.Fatalf("expected PRI-HAS-WORK (has BLIs), got %v (title=%v)", planID, summ)
	}
}

// TestResolvePriorityPlan_FallsBackToEmptyPlanWhenNoWorkAnywhere
// verifies that when all plans are empty, we still return a plan.
func TestResolvePriorityPlan_FallsBackToEmptyPlan(t *testing.T) {
	ctx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{
		AccountID: "ACC-SYSTEM",
	})

	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-EMPTY",
			objects.FieldKeyTitle:  "Empty But Only Option",
			objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		},
	)

	planID, _, _ := resolvePriorityPlanForWhatsNext(ctx, store, "", nil)
	if planID != "PRI-EMPTY" {
		t.Fatalf("expected PRI-EMPTY as fallback, got %v", planID)
	}
}

// TestResolvePriorityPlan_ExplicitOverridesWorkCheck verifies that
// an explicitly requested plan is always used, even if empty.
func TestResolvePriorityPlan_ExplicitOverridesWorkCheck(t *testing.T) {
	ctx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{
		AccountID: "ACC-SYSTEM",
	})

	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-EXPLICIT",
			objects.FieldKeyTitle:  "Explicitly Requested",
			objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		},
		map[string]any{
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-HAS-WORK",
			objects.FieldKeyTitle:  "Has Work",
			objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-1",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: "PRI-HAS-WORK",
		},
	)

	planID, _, _ := resolvePriorityPlanForWhatsNext(ctx, store, "PRI-EXPLICIT", nil)
	if planID != "PRI-EXPLICIT" {
		t.Fatalf("explicit plan should always be used, got %v", planID)
	}
}

// TestCountBacklogByStatus_ReturnsCorrectCounts verifies counting works.
func TestCountBacklogByStatus_ReturnsCorrectCounts(t *testing.T) {
	ctx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{
		AccountID: "ACC-SYSTEM",
	})

	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-1",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: "PRI-1",
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-2",
			objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
			objects.FieldKeyPriorityPlanRef: "PRI-1",
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-3",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: "PRI-OTHER",
		},
	)

	counts := countBacklogByStatus(ctx, store, "PRI-1", nil)
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
		t.Fatalf("expected 2 total BLIs for PRI-1, got %d", total)
	}
}
