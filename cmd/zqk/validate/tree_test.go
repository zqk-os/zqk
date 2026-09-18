package validate

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestResolvePriorityPlan(t *testing.T) {
	ctx := context.Background()

	// Case 1: Explicit plan
	store1 := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-1",
			objects.FieldKeyStatus: objects.ObjectStatusGrooming,
		},
	)
	planID, err := resolvePriorityPlan(ctx, store1, "PRI-1")
	if err != nil {
		t.Fatalf("resolvePriorityPlan failed: %v", err)
	}
	if planID != "PRI-1" {
		t.Errorf("expected PRI-1, got %s", planID)
	}

	// Case 2: Auto-resolve to in_progress
	store2 := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-in_progress",
			objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		},
		map[string]any{
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-active",
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		},
	)
	planID, err = resolvePriorityPlan(ctx, store2, "")
	if err != nil {
		t.Fatalf("resolvePriorityPlan failed: %v", err)
	}
	if planID != "PRI-in_progress" {
		t.Errorf("expected PRI-in_progress, got %s", planID)
	}
}

func TestTreeResolver_Resolve(t *testing.T) {
	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyID:     "PRI-plan",
			objects.FieldKeyStatus: objects.ObjectStatusInProgress,
			objects.FieldKeyTitle:  "Test Plan",
		},
		map[string]any{
			objects.FieldKeyKind:             objects.KindGoal,
			objects.FieldKeyID:               "GOAL-1",
			objects.FieldKeyStatus:           objects.ObjectStatusActive,
			objects.FieldKeyTitle:            "Test Goal",
			objects.FieldKeyPriorityPlanRefs: []any{"PRI-plan"},
		},
		map[string]any{
			objects.FieldKeyKind:     objects.KindRequirement,
			objects.FieldKeyID:       "REQ-1",
			objects.FieldKeyStatus:   objects.ObjectStatusComplete,
			objects.FieldKeyTitle:    "Test Requirement",
			objects.FieldKeyGoalRefs: []any{"GOAL-1"},
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-1",
			objects.FieldKeyStatus:          objects.ObjectStatusComplete,
			objects.FieldKeyTitle:           "Test Backlog Item",
			objects.FieldKeyPriorityPlanRef: "PRI-plan",
			objects.FieldKeyRequirementRefs: []any{"REQ-1"},
			objects.FieldKeyCriteriaRefs:    []any{"CRIT-1"},
		},
		map[string]any{
			objects.FieldKeyKind:   objects.KindCriteria,
			objects.FieldKeyID:     "CRIT-1",
			objects.FieldKeyStatus: objects.ObjectStatusComplete,
			objects.FieldKeyTitle:  "Test Criteria",
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-orphan",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyTitle:           "Orphan Item",
			objects.FieldKeyPriorityPlanRef: "PRI-plan",
		},
	)

	ctx := context.Background()
	resolver := NewDependencyTreeResolver(store)

	rootNode, err := resolver.Resolve(ctx, "PRI-plan")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// Verify root plan
	if rootNode.ID != "PRI-plan" || rootNode.Title != "Test Plan" {
		t.Errorf("unexpected root plan node: %+v", rootNode)
	}

	// Root should have 2 children: GOAL-1 (goal) and BLI-orphan (orphan backlog item)
	if len(rootNode.Children) != 2 {
		t.Fatalf("expected 2 children under root, got %d", len(rootNode.Children))
	}

	var goalNode, orphanNode *TreeNode
	for _, child := range rootNode.Children {
		if child.ID == "GOAL-1" {
			goalNode = child
		} else if child.ID == "BLI-orphan" {
			orphanNode = child
		}
	}

	if goalNode == nil {
		t.Fatalf("GOAL-1 node not found under root")
	}
	if orphanNode == nil {
		t.Fatalf("BLI-orphan node not found under root")
	}

	// GOAL-1 should have 1 child: REQ-1 (requirement)
	if len(goalNode.Children) != 1 {
		t.Fatalf("expected 1 child under goalNode, got %d", len(goalNode.Children))
	}
	reqNode := goalNode.Children[0]
	if reqNode.ID != "REQ-1" {
		t.Errorf("expected child to be REQ-1, got %s", reqNode.ID)
	}

	// REQ-1 should have 1 child: BLI-1
	if len(reqNode.Children) != 1 {
		t.Fatalf("expected 1 child under reqNode (BLI-1), got %d", len(reqNode.Children))
	}

	bliNode := reqNode.Children[0]
	if bliNode.ID != "BLI-1" {
		t.Fatalf("expected child of REQ-1 to be BLI-1, got %s", bliNode.ID)
	}

	// BLI-1 should have 1 child: CRIT-1 (acceptance criteria)
	if len(bliNode.Children) != 1 || bliNode.Children[0].ID != "CRIT-1" {
		t.Errorf("expected CRIT-1 under bliNode, got %+v", bliNode.Children)
	}
}
