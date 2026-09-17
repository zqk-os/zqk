package scenario

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestCreateOrderFromSpecIndex_NoIndexUsesDefaultOrder(t *testing.T) {
	t.Parallel()
	// Temp dir has no spec index -> default order and deferred refs.
	root := t.TempDir()
	// Ensure no spec index exists (temp dir is empty).
	createOrder, deferred, err := CreateOrderFromSpecIndex(root, []string{"requirement", "criteria", "test_case", "backlog_item"})
	if err != nil {
		t.Fatalf("CreateOrderFromSpecIndex: %v", err)
	}
	// Default order: criteria before requirement so parent.criteria_refs can resolve.
	if len(createOrder) != 4 {
		t.Errorf("create order length = %d, want 4", len(createOrder))
	}
	criteriaIdx, requirementIdx := -1, -1
	for i, k := range createOrder {
		if k == "criteria" {
			criteriaIdx = i
		}
		if k == "requirement" {
			requirementIdx = i
		}
	}
	if criteriaIdx >= 0 && requirementIdx >= 0 && criteriaIdx > requirementIdx {
		t.Errorf("criteria should come before requirement in create order; got %v", createOrder)
	}
	if len(deferred) != 0 {
		t.Errorf("REQ↔CRIT is a DAG; expected no deferred reverse refs, got %v", deferred)
	}
}

func TestCreateOrderFromSpecIndex_DefaultOrderIncludesMilestoneAndPriorityPlan(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	bundleKinds := []string{"milestone", "priority_plan", "requirement", "criteria", "backlog_item"}
	createOrder, _, err := CreateOrderFromSpecIndex(root, bundleKinds)
	if err != nil {
		t.Fatalf("CreateOrderFromSpecIndex: %v", err)
	}
	// milestone and priority_plan should appear before requirement and backlog_item (they are referenced by them).
	idx := func(k string) int {
		for i, c := range createOrder {
			if c == k {
				return i
			}
		}
		return -1
	}
	if i, j := idx("milestone"), idx("requirement"); i >= 0 && j >= 0 && i > j {
		t.Errorf("milestone should come before requirement in default order; got %v", createOrder)
	}
	if i, j := idx("priority_plan"), idx("backlog_item"); i >= 0 && j >= 0 && i > j {
		t.Errorf("priority_plan should come before backlog_item in default order; got %v", createOrder)
	}
}

func TestCreateOrderFromSpecIndex_WithIndexUsesSpecOrder(t *testing.T) {
	// When run from repo root, spec index exists; order should be derived from spec index.
	// Skip if not in repo (e.g. no .zqk/specs/spec_index.json).
	cwd, err := fileutil.Getwd()
	if err != nil {
		t.Skipf("getwd: %v", err)
	}
	// Walk up to find a directory that contains .zqk/specs/spec_index.json
	root := cwd
	for {
		idxPath := filepath.Join(datacell.CellCASPrimaryDir(root, "_internal"), "spec_index.json")
		if _, err := fileutil.Stat(idxPath); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Skip("spec index not found (not in repo?)")
		}
		root = parent
	}
	createOrder, deferred, err := CreateOrderFromSpecIndex(root, []string{"requirement", "criteria", "test_case", "backlog_item"})
	if err != nil {
		t.Fatalf("CreateOrderFromSpecIndex: %v", err)
	}
	if len(createOrder) != 4 {
		t.Errorf("create order length = %d, want 4", len(createOrder))
	}
	// Requirement.criteria_refs depends on criteria; criteria has no reverse — DAG.
	criteriaIdx, requirementIdx := -1, -1
	for i, k := range createOrder {
		if k == "criteria" {
			criteriaIdx = i
		}
		if k == "requirement" {
			requirementIdx = i
		}
	}
	if criteriaIdx >= 0 && requirementIdx >= 0 && criteriaIdx > requirementIdx {
		t.Errorf("with spec index, criteria should come before requirement; got %v", createOrder)
	}
	if len(deferred) != 0 {
		t.Errorf("expected no deferred REQ↔CRIT reverse, got %v", deferred)
	}
}

func TestRefFieldNameToTargetKind(t *testing.T) {
	t.Parallel()
	tests := []struct {
		field  string
		target string
	}{
		{objects.FieldKeyGoalRefs, "goal"},
		{objects.FieldKeyCriteriaRefs, "criteria"},
		{objects.FieldKeyRequirementRefs, "requirement"},
		{"requirement_ref", "requirement"},
		{objects.FieldKeyBacklogItemRefs, "backlog_item"},
		{objects.FieldKeyTestCaseRefs, "test_case"},
		{objects.FieldKeyMilestoneRefs, "milestone"},
		{objects.FieldKeyPriorityPlanRef, "priority_plan"},
	}
	for _, tt := range tests {
		got := refFieldNameToTargetKind(tt.field)
		if got != tt.target {
			t.Errorf("refFieldNameToTargetKind(%q) = %q, want %q", tt.field, got, tt.target)
		}
	}
}
