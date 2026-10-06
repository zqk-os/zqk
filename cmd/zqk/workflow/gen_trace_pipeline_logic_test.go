package workflow

import (
	"slices"
	"strings"
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
	if len(bundle.Objects.TestCases[0].RequirementRefs) != 1 || bundle.Objects.TestCases[0].RequirementRefs[0] != "REQ-1" {
		t.Errorf("expected test case to reference REQ-1, got %v", bundle.Objects.TestCases[0].RequirementRefs)
	}
	// 1 backlog item is generated, implementing all 3 criteria
	if len(bundle.Objects.BacklogItems) != 1 {
		t.Errorf("expected 1 backlog item, got %d", len(bundle.Objects.BacklogItems))
	}
	if len(allCriteria) != 3 {
		t.Errorf("expected allCriteria to contain 3 criteria IDs, got %d", len(allCriteria))
	}

	// Verify the Backlog Item references all 3 generated criteria and the test case
	bli := bundle.Objects.BacklogItems[0]
	if len(bli.CriteriaRefs) != 3 {
		t.Errorf("expected backlog item to reference 3 criteria, got %d", len(bli.CriteriaRefs))
	}
	for _, critID := range allCriteria {
		if !slices.Contains(bli.CriteriaRefs, critID) {
			t.Errorf("backlog item missing reference to criteria %s", critID)
		}
	}
	if len(bli.TestCaseRefs) != 1 || bli.TestCaseRefs[0] != bundle.Objects.TestCases[0].IDHint {
		t.Errorf("expected backlog item to reference generated test case, got %v", bli.TestCaseRefs)
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

func TestGenerateTracePipelineBundle_GoalTarget_MintsRequirement(t *testing.T) {
	goalObj := map[string]any{
		"_kind":               objects.KindGoal,
		objects.FieldKeyTitle: "Enterprise Scalability Goal",
	}

	bundle, _, needCriteria := generateTracePipelineBundle("GOAL-001", goalObj, nil)
	if bundle == nil {
		t.Fatal("expected bundle for Goal target")
	}
	if !needCriteria {
		t.Fatal("expected needCriteria to be true")
	}
	if len(bundle.Objects.Requirements) != 1 {
		t.Fatalf("expected 1 Requirement to be minted for Goal, got %d", len(bundle.Objects.Requirements))
	}
	req := bundle.Objects.Requirements[0]
	if len(req.GoalRefs) != 1 || req.GoalRefs[0] != "GOAL-001" {
		t.Errorf("expected Requirement to link GoalRefs ['GOAL-001'], got %v", req.GoalRefs)
	}

	// Criteria must link to the minted Requirement, NOT the Goal ID
	for _, crit := range bundle.Objects.Criteria {
		if crit.RequirementRef != req.IDHint {
			t.Errorf("expected Criteria RequirementRef %q, got %q", req.IDHint, crit.RequirementRef)
		}
	}

	// Test Case must link to the minted Requirement
	tc := bundle.Objects.TestCases[0]
	if len(tc.RequirementRefs) != 1 || tc.RequirementRefs[0] != req.IDHint {
		t.Errorf("expected TestCase RequirementRefs [%q], got %v", req.IDHint, tc.RequirementRefs)
	}

	// Backlog Item must link to the minted Requirement
	bli := bundle.Objects.BacklogItems[0]
	if len(bli.RequirementRefs) != 1 || bli.RequirementRefs[0] != req.IDHint {
		t.Errorf("expected BacklogItem RequirementRefs [%q], got %v", req.IDHint, bli.RequirementRefs)
	}
}

func TestGenerateTracePipelineBundle_MilestoneTarget_SetsMilestoneRefs(t *testing.T) {
	milObj := map[string]any{
		"_kind":               objects.KindMilestone,
		objects.FieldKeyTitle: "Q3 GA Milestone",
	}

	bundle, _, _ := generateTracePipelineBundle("MIL-001", milObj, nil)
	if bundle == nil {
		t.Fatal("expected bundle for Milestone target")
	}
	if len(bundle.Objects.BacklogItems) != 1 {
		t.Fatalf("expected 1 BacklogItem, got %d", len(bundle.Objects.BacklogItems))
	}
	bli := bundle.Objects.BacklogItems[0]
	if len(bli.MilestoneRefs) != 1 || bli.MilestoneRefs[0] != "MIL-001" {
		t.Errorf("expected BacklogItem MilestoneRefs ['MIL-001'], got %v", bli.MilestoneRefs)
	}
}

func TestGenerateTracePipelineBundle_MandatoryDocumentationCriteria(t *testing.T) {
	reqObj := map[string]any{
		"_kind":               objects.KindRequirement,
		objects.FieldKeyTitle: "Public Core Launch",
	}

	bundle, _, needCriteria := generateTracePipelineBundle("REQ-DOC-TEST", reqObj, nil)
	if bundle == nil || !needCriteria {
		t.Fatal("expected bundle to be generated")
	}

	hasDocCriteria := false
	for _, crit := range bundle.Objects.Criteria {
		if strings.Contains(crit.Title, "Documentation") && strings.Contains(crit.Description, "doc_entry") {
			hasDocCriteria = true
			if crit.Category != "compliance" && crit.Category != "acceptance" {
				t.Errorf("expected documentation criteria to have compliance or acceptance category, got %q", crit.Category)
			}
			break
		}
	}

	if !hasDocCriteria {
		t.Error("expected base criteria to include mandatory documentation and doc_entry criterion (POL-DOC-001)")
	}
}

// TestGenerateTracePipelineBundle_Issue504_GoalSetsGoalRefsAndNoDummyPath guards against
// Issue #504 where BLIs lacked goal_refs and test cases contained hardcoded dummy paths.
func TestGenerateTracePipelineBundle_Issue504_GoalSetsGoalRefsAndNoDummyPath(t *testing.T) {
	goalObj := map[string]any{
		"_kind":               objects.KindGoal,
		objects.FieldKeyTitle: "Enterprise Integration Engine",
	}

	bundle, _, _ := generateTracePipelineBundle("GOAL-ENTERPRISE-001", goalObj, nil)
	if bundle == nil {
		t.Fatal("expected bundle for Goal target")
	}

	if len(bundle.Objects.BacklogItems) != 1 {
		t.Fatalf("expected 1 BacklogItem, got %d", len(bundle.Objects.BacklogItems))
	}
	bli := bundle.Objects.BacklogItems[0]
	if len(bli.GoalRefs) != 1 || bli.GoalRefs[0] != "GOAL-ENTERPRISE-001" {
		t.Errorf("expected BacklogItem GoalRefs ['GOAL-ENTERPRISE-001'], got %v", bli.GoalRefs)
	}

	if len(bundle.Objects.TestCases) != 1 {
		t.Fatalf("expected 1 TestCase, got %d", len(bundle.Objects.TestCases))
	}
	tc := bundle.Objects.TestCases[0]
	if tc.PathOrID != "" {
		t.Errorf("expected empty PathOrID (no dummy path), got %q", tc.PathOrID)
	}
}

// TestGenerateTracePipelineBundle_Issue504_ReusesExistingRequirementsAndComponents
// guards against duplicate slice synthesis when requirements or test cases already exist.
func TestGenerateTracePipelineBundle_Issue504_ReusesExistingRequirementsAndComponents(t *testing.T) {
	goalObj := map[string]any{
		"_kind":               objects.KindGoal,
		objects.FieldKeyTitle: "Existing Pipeline Goal",
	}

	// Neighbors contain an already existing requirement, testcase, and bli
	neighbors := []map[string]any{
		{
			"_kind":            objects.KindRequirement,
			objects.FieldKeyID: "REQ-EXISTING-001",
		},
		{
			"_kind":            objects.KindTestCase,
			objects.FieldKeyID: "TST-EXISTING-001",
		},
		{
			"_kind":            objects.KindBacklogItem,
			objects.FieldKeyID: "BLI-EXISTING-001",
		},
	}

	bundle, _, needCriteria := generateTracePipelineBundle("GOAL-EXISTING-001", goalObj, neighbors)
	if bundle == nil {
		t.Fatal("expected bundle for criteria generation")
	}
	if !needCriteria {
		t.Fatal("expected needCriteria to be true")
	}

	// Should NOT mint a new Requirement because REQ-EXISTING-001 was found in neighbors
	if len(bundle.Objects.Requirements) != 0 {
		t.Errorf("expected 0 new Requirements (reuse existing), got %d", len(bundle.Objects.Requirements))
	}

	// Criteria should link to the existing requirement
	for _, crit := range bundle.Objects.Criteria {
		if crit.RequirementRef != "REQ-EXISTING-001" {
			t.Errorf("expected Criteria RequirementRef 'REQ-EXISTING-001', got %q", crit.RequirementRef)
		}
	}

	// Should NOT mint duplicate test case or BLI
	if len(bundle.Objects.TestCases) != 0 {
		t.Errorf("expected 0 new TestCases (reuse existing), got %d", len(bundle.Objects.TestCases))
	}
	if len(bundle.Objects.BacklogItems) != 0 {
		t.Errorf("expected 0 new BacklogItems (reuse existing), got %d", len(bundle.Objects.BacklogItems))
	}
}
