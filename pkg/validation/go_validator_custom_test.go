package validation

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

const (
	statusExploring  = "exploring"
	statusPlanned    = "planned"
	statusInProgress = "in_progress"
	statusRoadmap    = "roadmap"
)

// TestGoValidator_OrphanRejection validates CRIT-REDACTED
func TestGoValidator_OrphanRejection(t *testing.T) {
	gv := NewGoValidator()

	// 1. Missing both goal_refs and requirement_refs
	objMissing := map[string]any{
		objects.FieldKeyID:     "ITEM-test-1",
		objects.FieldKeyStatus: statusExploring,
	}
	errors := gv.validateCustomRules(context.Background(), "backlog_item", objMissing, nil)
	found := false
	for _, e := range errors {
		if e.Rule == "custom_integrity" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected custom_integrity error for backlog item lacking goal/req refs, got none")
	}

	// 2. Has goal_refs
	objHasGoal := map[string]any{
		objects.FieldKeyID:       "ITEM-test-2",
		objects.FieldKeyStatus:   statusExploring,
		objects.FieldKeyGoalRefs: []any{"GOL-1"},
	}
	errors = gv.validateCustomRules(context.Background(), "backlog_item", objHasGoal, nil)
	for _, e := range errors {
		if e.Rule == "custom_integrity" {
			t.Errorf("Unexpected custom_integrity error: %v", e)
		}
	}

	// 3. Has requirement_refs
	objHasReq := map[string]any{
		objects.FieldKeyID:              "ITEM-test-3",
		objects.FieldKeyStatus:          statusExploring,
		objects.FieldKeyRequirementRefs: []any{"REQ-1"},
	}
	errors = gv.validateCustomRules(context.Background(), "backlog_item", objHasReq, nil)
	for _, e := range errors {
		if e.Rule == "custom_integrity" {
			t.Errorf("Unexpected custom_integrity error: %v", e)
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
		if e.Rule == "custom_integrity" && e.Field == "description" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected custom_integrity error for goal lacking description, got none")
	}

	// 2. Short description (< 20 chars)
	objShort := map[string]any{
		objects.FieldKeyID:          "GOL-test-2",
		objects.FieldKeyDescription: "Too short",
	}
	errors = gv.validateCustomRules(context.Background(), "goal", objShort, nil)
	found = false
	for _, e := range errors {
		if e.Rule == "custom_integrity" && e.Field == "description" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected custom_integrity error for short goal description, got none")
	}

	// 3. Valid description
	objValid := map[string]any{
		objects.FieldKeyID:          "GOL-test-3",
		objects.FieldKeyDescription: "This description is long enough to pass validation.",
	}
	errors = gv.validateCustomRules(context.Background(), "goal", objValid, nil)
	for _, e := range errors {
		if e.Rule == "custom_integrity" && e.Field == "description" {
			t.Errorf("Unexpected description validation error: %v", e)
		}
	}
}

// TestGoValidator_PriorityMandated validates priority checks
func TestGoValidator_PriorityMandated(t *testing.T) {
	gv := NewGoValidator()

	// 1. Planned status with no priority
	objPlannedNoPriority := map[string]any{
		objects.FieldKeyID:       "ITEM-test-p1",
		objects.FieldKeyStatus:   statusPlanned,
		objects.FieldKeyGoalRefs: []any{"GOL-1"},
	}
	errors := gv.validateCustomRules(context.Background(), "backlog_item", objPlannedNoPriority, nil)
	found := false
	for _, e := range errors {
		if e.Rule == "custom_integrity" && e.Field == objects.FieldKeyPriority {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected custom_integrity error for planned backlog item without priority, got none")
	}

	// 2. In-progress status with no priority
	objProgressNoPriority := map[string]any{
		objects.FieldKeyID:       "ITEM-test-p2",
		objects.FieldKeyStatus:   statusInProgress,
		objects.FieldKeyGoalRefs: []any{"GOL-1"},
	}
	errors = gv.validateCustomRules(context.Background(), "backlog_item", objProgressNoPriority, nil)
	found = false
	for _, e := range errors {
		if e.Rule == "custom_integrity" && e.Field == objects.FieldKeyPriority {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected custom_integrity error for in_progress backlog item without priority, got none")
	}

	// 3. Exploring status with no priority (allowed)
	objExploringNoPriority := map[string]any{
		objects.FieldKeyID:       "ITEM-test-p3",
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
		objects.FieldKeyID:       "ITEM-test-p4",
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

// TestGoValidator_ScopeIntegrity validates that adding backlog items to an active priority plan triggers warnings
func TestGoValidator_ScopeIntegrity(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	projectRoot, err := paths.ModuleRootFromPath(wd)
	if err != nil {
		t.Fatalf("ModuleRootFromPath: %v", err)
	}
	tempPlanPath := filepath.Join(projectRoot, "docs/architecture/priority_plans/PLAN-test-active-plan.yaml")

	if err := fileutil.EnsureDir(filepath.Dir(tempPlanPath)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	planContent := `id: PLAN-test-active-plan
status: active
title: Test Active Plan
`
	if err := fileutil.WriteStandardFile(tempPlanPath, []byte(planContent)); err != nil {
		t.Fatalf("WriteFile temp plan: %v", err)
	}
	defer os.Remove(tempPlanPath)

	gv := NewGoValidator()

	// Linked to active plan -> expect validation error
	objLinkedToActive := map[string]any{
		objects.FieldKeyID:              "ITEM-test-scope-1",
		objects.FieldKeyStatus:          statusPlanned,
		objects.FieldKeyGoalRefs:        []any{"GOL-1"},
		objects.FieldKeyPriority:        "high",
		objects.FieldKeyPriorityPlanRef: "PLAN-test-active-plan",
	}
	warnings := gv.validateCustomWarnings(context.Background(), "backlog_item", objLinkedToActive, nil)
	found := false
	for _, w := range warnings {
		if w.Rule == "scope_integrity" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected scope_integrity warning for backlog item linked to active priority plan, got none")
	}

	// Linked to active plan with bypass context -> expect no error
	ctxWithBypass := pkgctx.WithForceLifecycleOverride(context.Background())
	warnings = gv.validateCustomWarnings(ctxWithBypass, "backlog_item", objLinkedToActive, nil)
	for _, w := range warnings {
		if w.Rule == "scope_integrity" {
			t.Errorf("Unexpected scope_integrity warning when bypass context override is set: %v", w)
		}
	}

	// Linked to non-existent plan -> expect no error
	objLinkedToNonExistent := map[string]any{
		objects.FieldKeyID:              "ITEM-test-scope-2",
		objects.FieldKeyStatus:          statusPlanned,
		objects.FieldKeyGoalRefs:        []any{"GOL-1"},
		objects.FieldKeyPriority:        "high",
		objects.FieldKeyPriorityPlanRef: "PLAN-non-existent-plan-id",
	}
	warnings = gv.validateCustomWarnings(context.Background(), "backlog_item", objLinkedToNonExistent, nil)
	for _, w := range warnings {
		if w.Rule == "scope_integrity" {
			t.Errorf("Unexpected scope_integrity warning for non-existent plan: %v", w)
		}
	}
}

// TestGoValidator_ActivePlanRoadmapRejection validates that linking roadmap backlog items to active priority plans is blocked
func TestGoValidator_ActivePlanRoadmapRejection(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	projectRoot, err := paths.ModuleRootFromPath(wd)
	if err != nil {
		t.Fatalf("ModuleRootFromPath: %v", err)
	}
	tempPlanPath := filepath.Join(projectRoot, "docs/architecture/priority_plans/PLAN-test-active-roadmap.yaml")

	if err := fileutil.EnsureDir(filepath.Dir(tempPlanPath)); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	planContent := `id: PLAN-test-active-roadmap
status: active
title: Test Active Roadmap Plan
`
	if err := fileutil.WriteStandardFile(tempPlanPath, []byte(planContent)); err != nil {
		t.Fatalf("WriteFile temp plan: %v", err)
	}
	defer os.Remove(tempPlanPath)

	gv := NewGoValidator()

	// 1. Link roadmap item -> expect validation error
	objRoadmap := map[string]any{
		objects.FieldKeyID:              "ITEM-test-roadmap-1",
		objects.FieldKeyStatus:          statusRoadmap,
		objects.FieldKeyGoalRefs:        []any{"GOL-1"},
		objects.FieldKeyPriorityPlanRef: "PLAN-test-active-roadmap",
	}
	errors := gv.validateCustomRules(context.Background(), "backlog_item", objRoadmap, nil)
	found := false
	for _, e := range errors {
		if e.Rule == "custom_integrity" && e.Field == objects.FieldKeyStatus {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected custom_integrity status error for backlog item in roadmap status linked to active plan, got none")
	}

	// 2. Link planned item -> expect no status error
	objPlanned := map[string]any{
		objects.FieldKeyID:              "ITEM-test-roadmap-2",
		objects.FieldKeyStatus:          statusPlanned,
		objects.FieldKeyGoalRefs:        []any{"GOL-1"},
		objects.FieldKeyPriority:        "medium",
		objects.FieldKeyPriorityPlanRef: "PLAN-test-active-roadmap",
	}
	errors = gv.validateCustomRules(context.Background(), "backlog_item", objPlanned, nil)
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
		if e.Rule == "custom_integrity" {
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
		if e.Rule == "custom_integrity" && e.Field == "criteria_refs" {
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
		if e.Rule == "custom_integrity" && e.Field == "path_or_id" {
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
		if e.Rule == "custom_integrity" && e.Field == "path_or_id" {
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
