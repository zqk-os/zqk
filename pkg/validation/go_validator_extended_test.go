package validation

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

func TestGoValidator_BasicsAndFeatures(t *testing.T) {
	gv := NewGoValidator()
	if gv.Name() != "go" {
		t.Errorf("expected name 'go', got %s", gv.Name())
	}

	if !gv.SupportsFeature("lifecycle") {
		t.Errorf("expected lifecycle feature supported")
	}
	if !gv.SupportsFeature("semantic_types") {
		t.Errorf("expected semantic_types feature supported")
	}
	if !gv.SupportsFeature("custom_rules") {
		t.Errorf("expected custom_rules feature supported")
	}
	if gv.SupportsFeature("unknown_future_feature") {
		t.Errorf("expected false for unknown feature")
	}

	// Validate nil object returns error
	_, err := gv.Validate(context.Background(), nil, objects.KindBacklogItem, nil)
	if err == nil {
		t.Error("expected error validating nil object")
	}

	// NewGoValidatorWithLoaders
	specLoader := objects.NewSpecLoader("")
	lifecycleLoader := objects.NewLifecycleLoader("")
	gv2 := NewGoValidatorWithLoaders(specLoader, lifecycleLoader)
	if gv2.Name() != "go" {
		t.Errorf("expected name 'go'")
	}

	// NewGoValidatorWithIDValidator
	idValidator := NewIDValidator("")
	gv3 := NewGoValidatorWithIDValidator(specLoader, lifecycleLoader, idValidator)
	if gv3 == nil || gv3.idValidator != idValidator {
		t.Errorf("expected injected IDValidator to match")
	}
}

func TestGoValidator_CycleAndLinkBack(t *testing.T) {
	gv := NewGoValidator()

	// detectCycle when currentID == targetID
	found, hops := gv.detectCycle("A", "A", map[string]bool{}, map[string]bool{}, nil)
	if !found || hops != 0 {
		t.Errorf("detectCycle('A', 'A') = (%v, %d), want (true, 0)", found, hops)
	}

	// detectCycle visited or explored
	visiting := map[string]bool{"B": true}
	found, _ = gv.detectCycle("B", "A", visiting, map[string]bool{}, nil)
	if found {
		t.Errorf("expected false when visiting")
	}

	explored := map[string]bool{"C": true}
	found, _ = gv.detectCycle("C", "A", map[string]bool{}, explored, nil)
	if found {
		t.Errorf("expected false when explored")
	}

	// isRefActive without lookup returns true
	if !gv.isRefActive("REF-1", nil, nil) {
		t.Errorf("expected isRefActive true when options nil")
	}

	optsActive := &ValidationOptions{
		ObjectStatusLookup: func(id string) (string, error) {
			return objects.ObjectStatusActive, nil
		},
	}
	if !gv.isRefActive("REF-1", nil, optsActive) {
		t.Errorf("expected isRefActive true when status active")
	}

	// firstRefToken: "ref" must be a substring
	if tok := firstRefToken("check goal_ref in target", ","); tok != "goal_ref" {
		t.Errorf("firstRefToken = %s, want goal_ref", tok)
	}
	if tok := firstRefToken("no pointer here", ""); tok != "" {
		t.Errorf("firstRefToken = %s, want empty", tok)
	}

	// linkBackSubjectFallback
	if s := linkBackSubjectFallback("milestone link"); s != objects.FieldKeyMilestoneRefs {
		t.Errorf("linkBackSubjectFallback = %s, want %s", s, objects.FieldKeyMilestoneRefs)
	}
	if s := linkBackSubjectFallback("backlog_item link"); s != objects.FieldKeyBacklogItemRefs {
		t.Errorf("linkBackSubjectFallback = %s, want %s", s, objects.FieldKeyBacklogItemRefs)
	}
	if s := linkBackSubjectFallback("requirement link"); s != objects.FieldKeyRequirementRefs {
		t.Errorf("linkBackSubjectFallback = %s, want %s", s, objects.FieldKeyRequirementRefs)
	}

	// linkBackTargetFallback
	if tg := linkBackTargetFallback("goal target"); tg != objects.FieldKeyGoalRefs {
		t.Errorf("linkBackTargetFallback = %s, want %s", tg, objects.FieldKeyGoalRefs)
	}
	if tg := linkBackTargetFallback("mission target"); tg != objects.FieldKeyMissionRefs {
		t.Errorf("linkBackTargetFallback = %s, want %s", tg, objects.FieldKeyMissionRefs)
	}
	if tg := linkBackTargetFallback("vision target"); tg != fieldKeyVisionRefs {
		t.Errorf("linkBackTargetFallback = %s, want %s", tg, fieldKeyVisionRefs)
	}

	// idsIntersect
	if !idsIntersect([]string{"A", "B"}, []string{"B", "C"}) {
		t.Errorf("expected intersect")
	}
	if idsIntersect([]string{"A"}, []string{"B"}) {
		t.Errorf("expected no intersect")
	}

	// skipLinkBackChild
	if skipLinkBackChild("C-1", nil) {
		t.Errorf("expected false on nil options")
	}
	optsDraft := &ValidationOptions{
		ObjectStatusLookup: func(id string) (string, error) {
			return objects.ObjectStatusDraft, nil
		},
	}
	if !skipLinkBackChild("C-1", optsDraft) {
		t.Errorf("expected true on draft status")
	}
}

func TestGoValidator_ClosureEvidenceAndAlignment(t *testing.T) {
	gv := NewGoValidator()

	// checkMachineCheckableClosureEvidence: grandfathered complete
	objComplete := map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusComplete,
	}
	optsComplete := &ValidationOptions{
		CurrentState: objects.ObjectStatusComplete,
	}
	if !gv.checkMachineCheckableClosureEvidence(objComplete, optsComplete) {
		t.Errorf("expected grandfathered complete to return true")
	}

	// evaluateAlignment: empty params
	errs := evaluateAlignment(gv, nil, map[string]any{}, nil, "rule1", map[string]any{})
	if len(errs) != 0 {
		t.Errorf("expected 0 errors for empty params")
	}

	// evaluateAlignment: matching parent and child with lookup
	childObj := map[string]any{
		"parent_ref": "P-1",
	}
	parentObj := map[string]any{
		"parent_ref": "P-1",
		"child_ref":  "C-1",
	}
	params := map[string]any{
		"parent_field": "parent_ref",
		"child_field":  "child_ref",
	}
	optsAligned := &ValidationOptions{
		ObjectLookup: func(id string) (map[string]any, error) {
			if id == "C-1" {
				return childObj, nil
			}
			return nil, nil
		},
	}
	errs = evaluateAlignment(gv, nil, parentObj, optsAligned, "rule1", params)
	if len(errs) != 0 {
		t.Errorf("expected 0 errors for aligned child/parent, got %v", errs)
	}
}

func TestGoValidator_BranchAndUnknownFields(t *testing.T) {
	gv := NewGoValidator()

	// validateUnknownFields
	spec := &objects.Spec{
		ResolvedFields: map[string]any{
			"known_field": map[string]any{"type": "string"},
		},
	}
	obj := map[string]any{
		objects.FieldKeyID: "ID-1",
		"known_field":      "val",
		"unknown_field":    "val2",
	}
	errs := gv.validateUnknownFields("my_kind", obj, spec)
	if len(errs) != 1 || errs[0].Field != "unknown_field" {
		t.Errorf("expected 1 error for unknown_field, got %v", errs)
	}

	// isStandardMetadataField
	if !isStandardMetadataField(objects.FieldKeyID) || !isStandardMetadataField(objects.FieldKeyKind) {
		t.Errorf("expected standard metadata fields to return true")
	}
	if isStandardMetadataField("custom_prop") {
		t.Errorf("expected false for custom_prop")
	}
}

func TestGoValidator_TDDTestRedPhase(t *testing.T) {
	gv := NewGoValidator()

	// checkTDDTestRedPhase: nil options
	if gv.checkTDDTestRedPhase(map[string]any{}, nil) {
		t.Errorf("expected false for nil options")
	}

	// empty criteria_refs
	opts := &ValidationOptions{
		ObjectStatusLookup: func(id string) (string, error) {
			return "active", nil
		},
		DependentsLookup: func(id string) []string {
			return []string{"TST-1"}
		},
	}
	if gv.checkTDDTestRedPhase(map[string]any{}, opts) {
		t.Errorf("expected false for empty criteria_refs")
	}

	// valid criteria_refs and test_case
	obj := map[string]any{
		"criteria_refs": []string{"CRIT-1"},
	}
	if !gv.checkTDDTestRedPhase(obj, opts) {
		t.Errorf("expected true when valid criteria and test case")
	}
}

func TestGoValidator_DynamicRulesAndCustomRules(t *testing.T) {
	gv := NewGoValidator()

	// composeObjectLookup: nil options
	if composeObjectLookup(nil) != nil {
		t.Errorf("expected nil for nil options")
	}

	// composeObjectLookup: with status lookup
	opts := &ValidationOptions{
		ObjectStatusLookup: func(id string) (string, error) {
			return "planned", nil
		},
	}
	lookup := composeObjectLookup(opts)
	if lookup == nil {
		t.Fatalf("expected non-nil lookup function")
	}
	obj, err := lookup("OBJ-1")
	if err != nil || obj[objects.FieldKeyStatus] != "planned" {
		t.Errorf("lookup('OBJ-1') = (%v, %v), want planned", obj, err)
	}

	// evaluateFieldPresence
	rulePresence := map[string]any{
		objects.FieldKeyID:       "rule_fp",
		objects.FieldKeyRuleType: RuleTypeFieldPresence,
		objects.FieldKeyParameters: map[string]any{
			objects.FieldKeyFieldName: "required_field",
		},
	}
	errs := gv.evaluateDynamicRule(rulePresence, map[string]any{}, nil)
	if len(errs) != 1 || errs[0].Field != "required_field" {
		t.Errorf("expected 1 error for missing field, got %v", errs)
	}

	errs = gv.evaluateDynamicRule(rulePresence, map[string]any{"required_field": "val"}, nil)
	if len(errs) != 0 {
		t.Errorf("expected 0 errors when field present, got %v", errs)
	}

	// evaluateActiveReference
	ruleActiveRef := map[string]any{
		objects.FieldKeyID:       "rule_ar",
		objects.FieldKeyRuleType: RuleTypeActiveReference,
		objects.FieldKeyParameters: map[string]any{
			"reference_field": "target_ref",
		},
	}
	objRef := map[string]any{
		"target_ref": "TARGET-1",
	}
	optsDraft := &ValidationOptions{
		ObjectStatusLookup: func(id string) (string, error) {
			return "draft", nil
		},
	}
	errs = gv.evaluateDynamicRule(ruleActiveRef, objRef, optsDraft)
	if len(errs) != 1 || errs[0].Field != "target_ref" {
		t.Errorf("expected 1 error when referenced object in draft status, got %v", errs)
	}

	optsActive := &ValidationOptions{
		ObjectStatusLookup: func(id string) (string, error) {
			return "active", nil
		},
	}
	errs = gv.evaluateDynamicRule(ruleActiveRef, objRef, optsActive)
	if len(errs) != 0 {
		t.Errorf("expected 0 errors when referenced object active, got %v", errs)
	}

	// Unknown rule type returns nil
	unknownRule := map[string]any{
		objects.FieldKeyRuleType: "unknown_type",
	}
	if gv.evaluateDynamicRule(unknownRule, nil, nil) != nil {
		t.Errorf("expected nil for unknown rule type")
	}
}

func TestGoValidator_CustomRulesCriteriaAndOwnerRef(t *testing.T) {
	gv := NewGoValidator()

	// Criteria with refactor in title without measured deltas
	critObj := map[string]any{
		objects.FieldKeyTitle: "Refactor validation cache",
		objects.FieldKeyCompletenessValidation: []string{
			"it exists in codebase",
		},
	}
	errs := gv.validateCustomRules(context.Background(), objects.KindCriteria, critObj, nil)
	foundRefactorErr := false
	for _, e := range errs {
		if e.Rule == "criteria_refactor_measured_deltas" {
			foundRefactorErr = true
			break
		}
	}
	if !foundRefactorErr {
		t.Errorf("expected criteria_refactor_measured_deltas error, got %v", errs)
	}

	// Criteria with refactor in title with measured deltas
	critObjValid := map[string]any{
		objects.FieldKeyTitle: "Refactor validation cache",
		objects.FieldKeyCompletenessValidation: []string{
			"assert symbol absent and measure file count down by 2",
		},
	}
	errsValid := gv.validateCustomRules(context.Background(), objects.KindCriteria, critObjValid, nil)
	for _, e := range errsValid {
		if e.Rule == "criteria_refactor_measured_deltas" {
			t.Errorf("unexpected criteria_refactor_measured_deltas error when measured deltas present")
		}
	}
}

func TestGoValidator_LoadDynamicRulesForKind(t *testing.T) {
	tmpDir := t.TempDir()
	rulesDir := filepath.Join(tmpDir, paths.ProcessDir, "validation_rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatalf("failed to create rulesDir: %v", err)
	}

	ruleYaml := `
target_kind: backlog_item
target_status: in_progress
rule_type: field_presence
parameters:
  field_name: assignee
`
	if err := os.WriteFile(filepath.Join(rulesDir, "rule1.yaml"), []byte(ruleYaml), 0644); err != nil {
		t.Fatalf("failed to write rule file: %v", err)
	}

	rules, err := loadDynamicRulesForKind(tmpDir, "backlog_item", "in_progress")
	if err != nil {
		t.Fatalf("loadDynamicRulesForKind failed: %v", err)
	}
	if len(rules) != 1 {
		t.Errorf("expected 1 rule loaded, got %d", len(rules))
	}

	// Non-matching status
	rulesOther, err := loadDynamicRulesForKind(tmpDir, "backlog_item", "complete")
	if err != nil {
		t.Fatalf("loadDynamicRulesForKind failed: %v", err)
	}
	if len(rulesOther) != 0 {
		t.Errorf("expected 0 rules for complete status, got %d", len(rulesOther))
	}

	// Non-existent directory
	rulesEmpty, err := loadDynamicRulesForKind(filepath.Join(tmpDir, "nonexistent"), "backlog_item", "in_progress")
	if err != nil {
		t.Fatalf("expected nil error for nonexistent dir, got %v", err)
	}
	if len(rulesEmpty) != 0 {
		t.Errorf("expected 0 rules for nonexistent dir")
	}
}






