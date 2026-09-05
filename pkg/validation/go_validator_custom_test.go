package validation

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

const (
	statusExploring  = "exploring"
	statusPlanned    = "planned"
	statusInProgress = "in_progress"
	statusRoadmap    = "roadmap"
)

// TestGoValidator_OwnerRefPrototypeRejection validates REQ-OWNER-REF-NO-PROTOTYPE-ACC-001
func TestGoValidator_OwnerRefPrototypeRejection(t *testing.T) {
	gv := NewGoValidator()
	ctx := context.Background()

	// Test cases
	tests := []struct {
		name     string
		ownerRef string
		wantErr  bool
	}{
		{"Valid account", "ACC-123", false},
		{"Valid account different prefix", "ACC-899", false},
		{"Prototype account ACC-9xx", "ACC-901", false},
		{"Prototype account ACC-TEST", "ACC-TESTUSER", true},
		{"Prototype account with PROTOTYPE", "ACC-PROTOTYPE-1", true},
		{"Valid user with prototype in name but not as prefix", "ACC-123-PROTOTYPE", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := map[string]any{
				objects.FieldKeyID:       "WS-TEST-1",
				objects.FieldKeyStatus:   statusExploring,
				objects.FieldKeyOwnerRef: tt.ownerRef,
			}
			errors := gv.validateCustomRules(ctx, "workstream", obj, nil)

			hasPrototypeErr := false
			for _, e := range errors {
				if e.Rule == "owner_ref_no_prototype" {
					hasPrototypeErr = true
					break
				}
			}

			if tt.wantErr && !hasPrototypeErr {
				t.Errorf("Expected owner_ref_no_prototype error for %q, got none", tt.ownerRef)
			}
			if !tt.wantErr && hasPrototypeErr {
				t.Errorf("Unexpected owner_ref_no_prototype error for %q", tt.ownerRef)
			}
		})
	}
}

// TestGoValidator_OrphanRejection validates [REDACTED-ID]
func TestGoValidator_OrphanRejection(t *testing.T) {
	gv := NewGoValidator()

	// 1. Missing both goal_refs and requirement_refs, past the draft plane.
	// The status matters: bli_hierarchical_chain sets skip_preliminary, so an unlinked
	// item is allowed to exist at 'exploring' and is refused from 'planned' onward.
	// Asserting the refusal at 'exploring' would test the opposite of that contract.
	objMissing := map[string]any{
		objects.FieldKeyID:     "BLI-test-1",
		objects.FieldKeyStatus: objects.ObjectStatusPlanned,
	}
	errors := gv.validateCustomRules(context.Background(), "backlog_item", objMissing, nil)
	found := false
	for _, e := range errors {
		if e.Rule == "composed_integrity" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected composed_integrity error for planned backlog item lacking goal/req refs, got none")
	}

	// 1b. The same object in the draft plane is allowed to be unlinked.
	objDraft := map[string]any{
		objects.FieldKeyID:     "BLI-test-1b",
		objects.FieldKeyStatus: statusExploring,
	}
	for _, e := range gv.validateCustomRules(context.Background(), "backlog_item", objDraft, nil) {
		if e.Rule == "composed_integrity" && e.Field == "goal_refs/requirement_refs" {
			t.Errorf("Draft backlog item must be writable without hierarchy links, got %v", e)
		}
	}

	// 2. Has goal_refs
	objHasGoal := map[string]any{
		objects.FieldKeyID:       "BLI-test-2",
		objects.FieldKeyStatus:   statusExploring,
		objects.FieldKeyGoalRefs: []any{"GOL-1"},
	}
	errors = gv.validateCustomRules(context.Background(), "backlog_item", objHasGoal, nil)
	for _, e := range errors {
		if e.Rule == "composed_integrity" {
			t.Errorf("Unexpected composed_integrity error: %v", e)
		}
	}

	// 3. Has requirement_refs
	objHasReq := map[string]any{
		objects.FieldKeyID:              "BLI-test-3",
		objects.FieldKeyStatus:          statusExploring,
		objects.FieldKeyRequirementRefs: []any{"REQ-1"},
	}
	errors = gv.validateCustomRules(context.Background(), "backlog_item", objHasReq, nil)
	for _, e := range errors {
		if e.Rule == "composed_integrity" {
			t.Errorf("Unexpected composed_integrity error: %v", e)
		}
	}
}

// TestGoValidator_GoalDescription validates description length rules for goals
func TestGoValidator_GoalDescription(t *testing.T) {
	gv := NewGoValidator()

	// 1. Missing description
	objMissing := map[string]any{
		objects.FieldKeyID: "GOL-test-1",
	}
	errors := gv.validateCustomRules(context.Background(), "goal", objMissing, nil)
	found := false
	for _, e := range errors {
		if e.Rule == "composed_integrity" && e.Field == "description" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected composed_integrity error for goal lacking description, got none")
	}

	// 2. Short description (< 20 chars)
	objShort := map[string]any{
		objects.FieldKeyID:          "GOL-test-2",
		objects.FieldKeyDescription: "Too short",
	}
	errors = gv.validateCustomRules(context.Background(), "goal", objShort, nil)
	found = false
	for _, e := range errors {
		if e.Rule == "composed_integrity" && e.Field == "description" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected composed_integrity error for short goal description, got none")
	}

	// 3. Valid description
	objValid := map[string]any{
		objects.FieldKeyID:          "GOL-test-3",
		objects.FieldKeyDescription: "This description is long enough to pass validation.",
	}
	errors = gv.validateCustomRules(context.Background(), "goal", objValid, nil)
	for _, e := range errors {
		if e.Rule == "composed_integrity" && e.Field == "description" {
			t.Errorf("Unexpected description validation error: %v", e)
		}
	}
}

// TestGoValidator_PriorityMandated validates priority checks
func TestGoValidator_PriorityMandated(t *testing.T) {
	gv := NewGoValidator()

	// 1. Planned status with no priority
	objPlannedNoPriority := map[string]any{
		objects.FieldKeyID:       "BLI-test-p1",
		objects.FieldKeyStatus:   statusPlanned,
		objects.FieldKeyGoalRefs: []any{"GOL-1"},
	}
	errors := gv.validateCustomRules(context.Background(), "backlog_item", objPlannedNoPriority, nil)
	found := false
	for _, e := range errors {
		if e.Rule == "composed_integrity" && e.Field == objects.FieldKeyPriority {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected composed_integrity error for planned backlog item without priority, got none")
	}

	// 2. In-progress status with no priority
	objProgressNoPriority := map[string]any{
		objects.FieldKeyID:       "BLI-test-p2",
		objects.FieldKeyStatus:   statusInProgress,
		objects.FieldKeyGoalRefs: []any{"GOL-1"},
	}
	errors = gv.validateCustomRules(context.Background(), "backlog_item", objProgressNoPriority, nil)
	found = false
	for _, e := range errors {
		if e.Rule == "composed_integrity" && e.Field == objects.FieldKeyPriority {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected composed_integrity error for in_progress backlog item without priority, got none")
	}

	// 3. Exploring status with no priority (allowed)
	objExploringNoPriority := map[string]any{
		objects.FieldKeyID:       "BLI-test-p3",
		objects.FieldKeyStatus:   statusExploring,
		objects.FieldKeyGoalRefs: []any{"GOL-1"},
	}
	errors = gv.validateCustomRules(context.Background(), "backlog_item", objExploringNoPriority, nil)
	for _, e := range errors {
		if e.Field == objects.FieldKeyPriority {
			t.Errorf("Unexpected priority validation error for exploring: %v", e)
		}
	}

	// 4. Planned status with priority (valid)
	objPlannedWithPriority := map[string]any{
		objects.FieldKeyID:       "BLI-test-p4",
		objects.FieldKeyStatus:   statusPlanned,
		objects.FieldKeyGoalRefs: []any{"GOL-1"},
		objects.FieldKeyPriority: "medium",
	}
	errors = gv.validateCustomRules(context.Background(), "backlog_item", objPlannedWithPriority, nil)
	for _, e := range errors {
		if e.Field == objects.FieldKeyPriority {
			t.Errorf("Unexpected error for valid priority: %v", e)
		}
	}
}

// TestGoValidator_ScopeIntegrity: soft scope_integrity warnings were deleted with Go customWarningValidators.
// Ready-only hard refuse for exploring/roadmap on active plans lives in priority_plan_membership.go.
func TestGoValidator_ScopeIntegrity(t *testing.T) {
	t.Skip("scope_integrity warnings removed with composed overlays; see TestValidateBacklogItemPlanMembership_ActivePlan")
}

// TestGoValidator_ActivePlanRoadmapRejection validates that linking roadmap backlog items to active priority plans is blocked
func TestGoValidator_ActivePlanRoadmapRejection(t *testing.T) {
	projectRoot := t.TempDir()
	tempPlanPath := filepath.Join(projectRoot, "docs/process/priority_plans/PRI-test-active-roadmap.yaml")

	if err := fileutil.EnsureDir(filepath.Dir(tempPlanPath)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	planContent := `id: PRI-test-active-roadmap
status: active
title: Test Active Roadmap Plan
`
	if err := fileutil.WriteStandardFile(tempPlanPath, []byte(planContent)); err != nil {
		t.Fatalf("WriteFile temp plan: %v", err)
	}
	defer fileutil.Remove(tempPlanPath)

	gv := NewGoValidator()
	opts := &ValidationOptions{
		ObjectStatusLookup: func(id string) (string, error) {
			if id == "PRI-test-active-roadmap" {
				return objects.ObjectStatusActive, nil
			}
			return "", fmt.Errorf("not found")
		},
	}

	// 1. Link roadmap item -> expect validation error
	objRoadmap := map[string]any{
		objects.FieldKeyID:              "BLI-test-roadmap-1",
		objects.FieldKeyStatus:          statusRoadmap,
		objects.FieldKeyGoalRefs:        []any{"GOL-1"},
		objects.FieldKeyPriorityPlanRef: "PRI-test-active-roadmap",
	}
	errors := gv.validateCustomRules(context.Background(), "backlog_item", objRoadmap, opts)
	found := false
	for _, e := range errors {
		// Membership (compose OpRefuseExecutionFacingMembership) owns roadmap×active refuse.
		if e.Field == objects.FieldKeyStatus && e.Rule == "execution_facing_membership" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected status error for backlog item in roadmap status linked to active plan, got %#v", errors)
	}

	// 2. Link planned item -> expect no status error
	objPlanned := map[string]any{
		objects.FieldKeyID:              "BLI-test-roadmap-2",
		objects.FieldKeyStatus:          statusPlanned,
		objects.FieldKeyGoalRefs:        []any{"GOL-1"},
		objects.FieldKeyPriority:        "medium",
		objects.FieldKeyPriorityPlanRef: "PRI-test-active-roadmap",
	}
	errors = gv.validateCustomRules(context.Background(), "backlog_item", objPlanned, opts)
	for _, e := range errors {
		if e.Field == objects.FieldKeyStatus {
			t.Errorf("Unexpected status validation error for planned item: %v", e)
		}
	}
}

// TestGoValidator_GoalMetricsAndTargets verifies goal metric and target checks
func TestGoValidator_GoalMetricsAndTargets(t *testing.T) {
	gv := NewGoValidator()

	// 1. Active goal missing metric and target
	objActiveMissing := map[string]any{
		objects.FieldKeyID:          "GOL-active-missing",
		objects.FieldKeyDescription: "This description is longer than twenty characters.",
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
	}
	errors := gv.validateCustomRules(context.Background(), "goal", objActiveMissing, nil)
	hasMetricErr := false
	hasTargetErr := false
	for _, e := range errors {
		if e.Rule == "composed_integrity" {
			if e.Field == "metric" {
				hasMetricErr = true
			}
			if e.Field == "target" {
				hasTargetErr = true
			}
		}
	}
	if !hasMetricErr {
		t.Errorf("Expected goal validation error on field 'metric' for active goal, got none")
	}
	if !hasTargetErr {
		t.Errorf("Expected goal validation error on field 'target' for active goal, got none")
	}

	// 2. Active goal with both metric and target
	objActiveValid := map[string]any{
		objects.FieldKeyID:          "GOL-active-valid",
		objects.FieldKeyDescription: "This description is longer than twenty characters.",
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
		objects.FieldKeyMetric:      "system-security-compliance",
		objects.FieldKeyTarget:      "100",
	}
	errors = gv.validateCustomRules(context.Background(), "goal", objActiveValid, nil)
	for _, e := range errors {
		if e.Field == "metric" || e.Field == "target" {
			t.Errorf("Unexpected goal validation error on active valid goal: %v", e)
		}
	}
}

// TestGoValidator_MilestoneCriteriaRefs verifies active milestone criteria references
func TestGoValidator_MilestoneCriteriaRefs(t *testing.T) {
	gv := NewGoValidator()

	// 1. Active milestone missing criteria refs
	objActiveMissing := map[string]any{
		objects.FieldKeyID:     "MIL-active-missing",
		objects.FieldKeyStatus: objects.ObjectStatusActive,
	}
	errors := gv.validateCustomRules(context.Background(), "milestone", objActiveMissing, nil)
	hasCriteriaErr := false
	for _, e := range errors {
		if e.Rule == "composed_integrity" && e.Field == "criteria_refs" {
			hasCriteriaErr = true
			break
		}
	}
	if !hasCriteriaErr {
		t.Errorf("Expected milestone validation error on field 'criteria_refs' for active milestone, got none")
	}

	// 2. Active milestone with criteria refs
	objActiveValid := map[string]any{
		objects.FieldKeyID:           "MIL-active-valid",
		objects.FieldKeyStatus:       objects.ObjectStatusActive,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-test-1"},
	}
	errors = gv.validateCustomRules(context.Background(), "milestone", objActiveValid, nil)
	for _, e := range errors {
		if e.Field == "criteria_refs" {
			t.Errorf("Unexpected milestone validation error on active valid milestone: %v", e)
		}
	}
}

// TestGoValidator_TestCasePathOrID verifies active/complete test case path_or_id checks
func TestGoValidator_TestCasePathOrID(t *testing.T) {
	gv := NewGoValidator()

	// 1. Active test case missing path_or_id
	objActiveMissing := map[string]any{
		objects.FieldKeyID:     "TST-active-missing",
		objects.FieldKeyStatus: objects.ObjectStatusActive,
	}
	errors := gv.validateCustomRules(context.Background(), "test_case", objActiveMissing, nil)
	hasPathErr := false
	for _, e := range errors {
		if e.Rule == "composed_integrity" && e.Field == "path_or_id" {
			hasPathErr = true
			break
		}
	}
	if !hasPathErr {
		t.Errorf("Expected test_case validation error on field 'path_or_id' for active test case, got none")
	}

	// 2. Complete test case missing path_or_id
	objCompleteMissing := map[string]any{
		objects.FieldKeyID:     "TST-complete-missing",
		objects.FieldKeyStatus: objects.ObjectStatusComplete,
	}
	errors = gv.validateCustomRules(context.Background(), "test_case", objCompleteMissing, nil)
	hasPathErr = false
	for _, e := range errors {
		if e.Rule == "composed_integrity" && e.Field == "path_or_id" {
			hasPathErr = true
			break
		}
	}
	if !hasPathErr {
		t.Errorf("Expected test_case validation error on field 'path_or_id' for complete test case, got none")
	}

	// 3. Active test case with path_or_id
	objActiveValid := map[string]any{
		objects.FieldKeyID:       "TST-active-valid",
		objects.FieldKeyStatus:   objects.ObjectStatusActive,
		objects.FieldKeyPathOrID: "pkg/validation/go_validator_test.go",
	}
	errors = gv.validateCustomRules(context.Background(), "test_case", objActiveValid, nil)
	for _, e := range errors {
		if e.Field == "path_or_id" {
			t.Errorf("Unexpected test_case validation error on active valid test case: %v", e)
		}
	}
}

func TestGoValidator_CriteriaRefactorMeasuredDeltas(t *testing.T) {
	t.Parallel()
	gv := NewGoValidator()
	ctx := context.Background()

	// 1. Refactor criteria without completeness_validation fails
	obj1 := map[string]any{
		objects.FieldKeyTitle: "Refactor storage layer",
	}
	errs := gv.validateCustomRules(ctx, objects.KindCriteria, obj1, nil)
	hasErr := false
	for _, e := range errs {
		if e.Rule == "criteria_refactor_measured_deltas" {
			hasErr = true
		}
	}
	if !hasErr {
		t.Errorf("Expected error for refactor criteria without completeness_validation")
	}

	// 2. Refactor criteria with non-measured existence prose fails
	obj2 := map[string]any{
		objects.FieldKeyTitle: "Refactor storage layer",
		objects.FieldKeyCompletenessValidation: []any{
			"check if file exists",
		},
	}
	errs = gv.validateCustomRules(ctx, objects.KindCriteria, obj2, nil)
	hasErr = false
	for _, e := range errs {
		if e.Rule == "criteria_refactor_measured_deltas" {
			hasErr = true
		}
	}
	if !hasErr {
		t.Errorf("Expected error for refactor criteria with existence prose")
	}

	// 3. Refactor criteria with measured deltas passes
	obj3 := map[string]any{
		objects.FieldKeyTitle: "Refactor storage layer",
		objects.FieldKeyCompletenessValidation: []any{
			"measure root package file count down by N",
			"assert specific symbol absent from old package",
		},
	}
	errs = gv.validateCustomRules(ctx, objects.KindCriteria, obj3, nil)
	hasErr = false
	for _, e := range errs {
		if e.Rule == "criteria_refactor_measured_deltas" {
			hasErr = true
		}
	}
	if hasErr {
		t.Errorf("Expected no error for refactor criteria with measured deltas")
	}
}
