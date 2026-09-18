package workflow

import (
	"slices"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestGenerateTracePipelineBundle_FullGeneration(t *testing.T) {
	reqObj := map[string]any{
		"_kind":               objects.KindRequirement,
		objects.FieldKeyTitle: "Full Gen Test",
	}

	bundle, allCriteria, needCriteria := generateTracePipelineBundle("REQ-1", reqObj, nil)

	if bundle == nil {
		t.Fatal("expected bundle to be generated")
	}
	if !needCriteria {
		t.Fatal("expected needCriteria to be true")
	}
	// Requirement to criteria is NOT 1:1; base structure provides at least 3 criteria
	if len(bundle.Objects.Criteria) != 3 {
		t.Errorf("expected 3 base criteria, got %d", len(bundle.Objects.Criteria))
	}
	// Test case is 1:1 with requirement, containing all criteria
	if len(bundle.Objects.TestCases) != 1 {
		t.Errorf("expected 1 test case for requirement, got %d", len(bundle.Objects.TestCases))
	}
	if len(bundle.Objects.TestCases[0].CriteriaRefs) != 3 {
		t.Errorf("expected test case to contain 3 criteria refs, got %d", len(bundle.Objects.TestCases[0].CriteriaRefs))
	}
	// 1 backlog item is generated, implementing all 3 criteria
	if len(bundle.Objects.BacklogItems) != 1 {
		t.Errorf("expected 1 backlog item, got %d", len(bundle.Objects.BacklogItems))
	}
	if len(allCriteria) != 3 {
		t.Errorf("expected allCriteria to contain 3 criteria IDs, got %d", len(allCriteria))
	}

	// Verify the Backlog Item references all 3 generated criteria
	bli := bundle.Objects.BacklogItems[0]
	if len(bli.CriteriaRefs) != 3 {
		t.Errorf("expected backlog item to reference 3 criteria, got %d", len(bli.CriteriaRefs))
	}
	for _, critID := range allCriteria {
		if !slices.Contains(bli.CriteriaRefs, critID) {
			t.Errorf("backlog item missing reference to criteria %s", critID)
		}
	}
}

func TestGenerateTracePipelineBundle_PartialExisting_RejectsSingle1to1Ratio(t *testing.T) {
	// A requirement with only 1 criteria should NOT be considered "all good".
	// The pipeline must scaffold supplemental criteria to complete the multi-criteria base structure.
	reqObj := map[string]any{
		"_kind":                      objects.KindRequirement,
		objects.FieldKeyTitle:        "Multi Criteria Completion Test",
		objects.FieldKeyCriteriaRefs: []any{"CRIT-EXISTING-1"},
	}

	bundle, allCriteria, needCriteria := generateTracePipelineBundle("REQ-2", reqObj, nil)

	if bundle == nil {
		t.Fatal("expected bundle to be generated because 1 criteria is insufficient for a complete base structure")
	}
	if !needCriteria {
		t.Fatal("expected needCriteria to be true because requirement:criteria is not 1:1")
	}
	// With 1 existing criteria and a base structure of 3, 2 supplemental criteria must be generated
	if len(bundle.Objects.Criteria) != 2 {
		t.Errorf("expected 2 supplemental criteria, got %d", len(bundle.Objects.Criteria))
	}
	if len(allCriteria) != 3 {
		t.Errorf("expected allCriteria to contain 3 criteria (1 existing + 2 new), got %d", len(allCriteria))
	}
	if !slices.Contains(allCriteria, "CRIT-EXISTING-1") {
		t.Errorf("expected allCriteria to include existing criteria CRIT-EXISTING-1")
	}

	// Backlog item must reference all 3 criteria (both existing and supplemental)
	if len(bundle.Objects.BacklogItems) != 1 {
		t.Fatalf("expected 1 backlog item, got %d", len(bundle.Objects.BacklogItems))
	}
	bli := bundle.Objects.BacklogItems[0]
	if len(bli.CriteriaRefs) != 3 {
		t.Errorf("expected backlog item to reference all 3 criteria, got %d", len(bli.CriteriaRefs))
	}
}

func TestGenerateTracePipelineBundle_IdempotencyShortCircuit(t *testing.T) {
	// When a requirement already has a full multi-criteria base structure (>=3) and all have test cases + BLI
	reqObj := map[string]any{
		"_kind":                      objects.KindRequirement,
		objects.FieldKeyTitle:        "Idempotent Test",
		objects.FieldKeyCriteriaRefs: []any{"CRIT-1", "CRIT-2", "CRIT-3"},
	}

	neighbors := []map[string]any{
		{"_kind": objects.KindTestCase, objects.FieldKeyID: "TST-1", objects.FieldKeyCriteriaRefs: []any{"CRIT-1"}},
		{"_kind": objects.KindTestCase, objects.FieldKeyID: "TST-2", objects.FieldKeyCriteriaRefs: []any{"CRIT-2"}},
		{"_kind": objects.KindTestCase, objects.FieldKeyID: "TST-3", objects.FieldKeyCriteriaRefs: []any{"CRIT-3"}},
		{"_kind": objects.KindBacklogItem, objects.FieldKeyID: "BLI-1"},
	}

	bundle, allCriteria, needCriteria := generateTracePipelineBundle("REQ-3", reqObj, neighbors)

	if bundle != nil {
		t.Fatal("expected bundle to be nil for fully established multi-criteria pipeline")
	}
	if needCriteria {
		t.Fatal("expected needCriteria to be false")
	}
	if len(allCriteria) != 3 {
		t.Errorf("expected allCriteria to have 3 criteria, got %d", len(allCriteria))
	}
}

func TestGenerateTracePipelineBundle_CustomCriteriaCount(t *testing.T) {
	reqObj := map[string]any{
		"_kind":               objects.KindRequirement,
		objects.FieldKeyTitle: "Custom Count Test",
	}

	customCount := 4
	bundle, allCriteria, needCriteria := generateTracePipelineBundle("REQ-4", reqObj, nil, customCount)

	if bundle == nil {
		t.Fatal("expected bundle to be generated")
	}
	if !needCriteria {
		t.Fatal("expected needCriteria to be true")
	}
	if len(bundle.Objects.Criteria) != customCount {
		t.Errorf("expected %d criteria, got %d", customCount, len(bundle.Objects.Criteria))
	}
	if len(bundle.Objects.TestCases) != 1 {
		t.Errorf("expected 1 test case, got %d", len(bundle.Objects.TestCases))
	}
	if len(bundle.Objects.TestCases[0].CriteriaRefs) != customCount {
		t.Errorf("expected test case to contain %d criteria refs, got %d", customCount, len(bundle.Objects.TestCases[0].CriteriaRefs))
	}
	if len(allCriteria) != customCount {
		t.Errorf("expected %d criteria IDs, got %d", customCount, len(allCriteria))
	}
}

func TestCoerceRequirementPriorityForCASUpdate(t *testing.T) {
	obj := map[string]any{objects.FieldKeyPriority: "medium"}
	coerceRequirementPriorityForCASUpdate(obj)
	if obj[objects.FieldKeyPriority] != "p2" {
		t.Fatalf("medium -> p2, got %#v", obj[objects.FieldKeyPriority])
	}
}

