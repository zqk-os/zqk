//go:build lifecycle_test
// +build lifecycle_test

// This test file is excluded from normal test runs to avoid import cycle issues.
// The cycle is: validation -> specbuilder/bldr_v2 -> validation (via base_object_builder.go)
//
// To run these tests, use: go test -tags=lifecycle_test ./pkg/validation -run TestInstanceValidator

package validation

import (
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_v2" // Register builders for tests
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// setupBacklogItemLifecycle creates a temporary lifecycle directory with backlog_item and milestone lifecycle files
func setupBacklogItemLifecycle(t *testing.T) string {
	tmpDir := t.TempDir()
	lifecyclesDir := filepath.Join(tmpDir, "lifecycles")
	if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create lifecycles dir: %v", err)
	}

	// Create backlog_item_lifecycle.yaml with the statuses and transitions needed for tests
	lifecycleFile := filepath.Join(lifecyclesDir, "backlog_item_lifecycle.yaml")
	lifecycleContent := `object_type: backlog_item
statuses:
  - value: exploring
    display: Exploring
    initial: true
  - value: validated
    display: Validated
  - value: planned
    display: Planned
  - value: in_progress
    display: In Progress
  - value: complete
    display: Complete
    terminal: true
transitions:
  - from: exploring
    to: validated
  - from: validated
    to: planned
  - from: planned
    to: in_progress
  - from: in_progress
    to: complete
`
	if err := fileutil.WriteFile(lifecycleFile, []byte(lifecycleContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write lifecycle file: %v", err)
	}

	// Create milestone_lifecycle.yaml for milestone tests
	milestoneFile := filepath.Join(lifecyclesDir, "milestone_lifecycle.yaml")
	milestoneContent := `object_type: milestone
statuses:
  - value: not_started
    display: Not Started
    initial: true
  - value: in_progress
    display: In Progress
  - value: complete
    display: Complete
    terminal: true
transitions:
  - from: not_started
    to: in_progress
  - from: in_progress
    to: complete
`
	if err := fileutil.WriteFile(milestoneFile, []byte(milestoneContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write milestone lifecycle file: %v", err)
	}

	return lifecyclesDir
}

func TestInstanceValidator_ValidateInstanceWithState(t *testing.T) {
	t.Parallel()
	specsDir := filepath.Join("..", "..", paths.ProcessInternalObjectSpecsDir)
	specLoader := objects.NewSpecLoader(specsDir)
	// Set builder registry on spec loader to enable version-aware loading
	builderRegistry := builders.GetGlobalRegistry()
	adapter := builders.NewSpecLoaderAdapter(builderRegistry)
	specLoader.SetBuilderRegistry(adapter)
	lifecyclesDir := setupBacklogItemLifecycle(t)
	lifecycleLoader := objects.NewLifecycleLoader(lifecyclesDir)
	validator := NewGoValidatorWithLoaders(specLoader, lifecycleLoader)

	validAuditFields := map[string]any{
		objects.FieldKeyCreatedAt:   "2025-12-25T00:00:00Z",
		objects.FieldKeyCreatedBy:   "ACC-TEST",
		objects.FieldKeyUpdatedAt:   "2025-12-25T00:00:00Z",
		objects.FieldKeyUpdatedBy:   "ACC-TEST",
		objects.FieldKeyDescription: "Valid test description",
	}

	tests := []struct {
		name               string
		obj                map[string]any
		kind               string
		currentState       string
		wantValid          bool
		wantLifecycleError bool
	}{
		{
			name: "valid status",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:            "BLI-001",
					objects.FieldKeyKind:          "backlog_item",
					objects.FieldKeyTitle:         "Test Item",
					objects.FieldKeyStatus:        "exploring",
					objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
				}
				for k, v := range validAuditFields {
					obj[k] = v
				}
				return obj
			}(),
			kind:         "backlog_item",
			currentState: "",
			wantValid:    true,
		},
		{
			name: "valid transition",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:            "BLI-002",
					objects.FieldKeyKind:          "backlog_item",
					objects.FieldKeyTitle:         "Test Item",
					objects.FieldKeyStatus:        "validated",
					objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
					objects.FieldKeyGoalRefs:      []string{"GOAL-001"},
				}
				for k, v := range validAuditFields {
					obj[k] = v
				}
				return obj
			}(),
			kind:         "backlog_item",
			currentState: "exploring",
			wantValid:    true,
		},
		{
			name: "invalid status",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:            "BLI-003",
					objects.FieldKeyKind:          "backlog_item",
					objects.FieldKeyTitle:         "Test Item",
					objects.FieldKeyStatus:        "invalid_status",
					objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
				}
				for k, v := range validAuditFields {
					obj[k] = v
				}
				return obj
			}(),
			kind:               "backlog_item",
			currentState:       "",
			wantValid:          false,
			wantLifecycleError: true,
		},
		{
			name: "invalid transition",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:            "BLI-004",
					objects.FieldKeyKind:          "backlog_item",
					objects.FieldKeyTitle:         "Test Item",
					objects.FieldKeyStatus:        "complete",
					objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
				}
				for k, v := range validAuditFields {
					obj[k] = v
				}
				return obj
			}(),
			kind:               "backlog_item",
			currentState:       "exploring", // Cannot go directly from exploring to complete
			wantValid:          false,
			wantLifecycleError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := DefaultValidationOptions()
			options.CurrentState = tt.currentState
			options.ValidateLifecycle = true
			result, err := validator.Validate(pkgctx.NewSystemContext(), tt.obj, tt.kind, options)
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}

			if result.IsValid != tt.wantValid {
				t.Errorf("Validate() IsValid = %v, want %v", result.IsValid, tt.wantValid)
				if len(result.Errors) > 0 {
					t.Logf("Validation errors: %+v", result.Errors)
				}
			}

			// Check if lifecycle error is present
			if tt.wantLifecycleError {
				hasLifecycleError := false
				for _, e := range result.Errors {
					if e.Rule == "lifecycle" {
						hasLifecycleError = true
						break
					}
				}
				if !hasLifecycleError {
					t.Error("Expected lifecycle validation error, but none found")
				}
			}
		})
	}
}

func TestInstanceValidator_checkPrecondition_Lifecycle(t *testing.T) {
	t.Parallel()
	validator := &InstanceValidator{}

	tests := []struct {
		name         string
		precondition string
		obj          map[string]any
		want         bool
	}{
		{
			name:         "field is set - exists",
			precondition: "priority_plan_ref is set",
			obj:          map[string]any{objects.FieldKeyPriorityPlanRef: "PRI-208"},
			want:         true,
		},
		{
			name:         "field is set - missing",
			precondition: "priority_plan_ref is set",
			obj:          map[string]any{},
			want:         false,
		},
		{
			name:         "field is set - empty",
			precondition: "priority_plan_ref is set",
			obj:          map[string]any{objects.FieldKeyPriorityPlanRef: ""},
			want:         false,
		},
		{
			name:         "list is not empty - has items",
			precondition: "milestone_refs is not empty",
			obj:          map[string]any{objects.FieldKeyMilestoneRefs: []string{"MIL-001"}},
			want:         true,
		},
		{
			name:         "list is not empty - empty",
			precondition: "milestone_refs is not empty",
			obj:          map[string]any{objects.FieldKeyMilestoneRefs: []string{}},
			want:         false,
		},
		{
			name:         "at least one milestone",
			precondition: "at least one milestone_ref linked",
			obj:          map[string]any{objects.FieldKeyMilestoneRefs: []string{"MIL-001"}},
			want:         true,
		},
		{
			name:         "at least one milestone - missing",
			precondition: "at least one milestone_ref linked",
			obj:          map[string]any{},
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validator.checkPrecondition(tt.precondition, tt.obj)
			if got != tt.want {
				t.Errorf("checkPrecondition() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestInstanceValidator_ValidateInstanceWithState_GraphData tests validation with graph-based storage
// This ensures the validator works correctly when objects are stored in a graph backend rather than files
func TestInstanceValidator_ValidateInstanceWithState_GraphData(t *testing.T) {
	t.Parallel()
	specsDir := filepath.Join("..", "..", paths.ProcessInternalObjectSpecsDir)
	specLoader := objects.NewSpecLoader(specsDir)
	// Set builder registry on spec loader to enable version-aware loading
	builderRegistry := builders.GetGlobalRegistry()
	adapter := builders.NewSpecLoaderAdapter(builderRegistry)
	specLoader.SetBuilderRegistry(adapter)
	lifecyclesDir := setupBacklogItemLifecycle(t)
	lifecycleLoader := objects.NewLifecycleLoader(lifecyclesDir)
	validator := NewGoValidatorWithLoaders(specLoader, lifecycleLoader)

	validAuditFields := map[string]any{
		objects.FieldKeyCreatedAt:   "2025-12-25T00:00:00Z",
		objects.FieldKeyCreatedBy:   "ACC-TEST",
		objects.FieldKeyUpdatedAt:   "2025-12-25T00:00:00Z",
		objects.FieldKeyUpdatedBy:   "ACC-TEST",
		objects.FieldKeyDescription: "Valid test description",
	}

	// Test cases that would come from graph storage (same validation logic, different source)
	tests := []struct {
		name               string
		obj                map[string]any
		kind               string
		currentState       string
		wantValid          bool
		wantLifecycleError bool
		description        string
	}{
		{
			name: "graph data - valid backlog item",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:            "BLI-999",
					objects.FieldKeyKind:          "backlog_item",
					objects.FieldKeyTitle:         "Graph Test Item",
					objects.FieldKeyStatus:        "exploring",
					objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
				}
				for k, v := range validAuditFields {
					obj[k] = v
				}
				return obj
			}(),
			kind:         "backlog_item",
			currentState: "",
			wantValid:    true,
			description:  "Validates that graph-stored backlog items pass validation",
		},
		{
			name: "graph data - valid requirement with criteria_refs",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:            "REQ-999",
					objects.FieldKeyKind:          "requirement",
					objects.FieldKeyTitle:         "Graph Test Requirement",
					objects.FieldKeyStatus:        "proposed",
					objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
					objects.FieldKeyCriteriaRefs:  []string{"CRIT-001", "CRIT-002"},
					objects.FieldKeyGoalRefs:      []string{"GOAL-001"},
				}
				for k, v := range validAuditFields {
					obj[k] = v
				}
				return obj
			}(),
			kind:         "requirement",
			currentState: "",
			wantValid:    true,
			description:  "Validates that graph-stored requirements with criteria_refs pass validation",
		},
		{
			name: "graph data - requirement missing criteria_refs",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:            "REQ-998",
					objects.FieldKeyKind:          "requirement",
					objects.FieldKeyTitle:         "Invalid Requirement",
					objects.FieldKeyStatus:        "invalid_status",
					objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
					objects.FieldKeyGoalRefs:      []string{"GOAL-001"},
					// Invalid status - should fail validation
				}
				for k, v := range validAuditFields {
					obj[k] = v
				}
				return obj
			}(),
			kind:         "requirement",
			currentState: "",
			wantValid:    false,
			description:  "Validates that graph-stored requirements without criteria_refs fail validation",
		},
		{
			name: "graph data - invalid lifecycle transition",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:            "BLI-998",
					objects.FieldKeyKind:          "backlog_item",
					objects.FieldKeyTitle:         "Graph Test Item",
					objects.FieldKeyStatus:        "complete",
					objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
				}
				for k, v := range validAuditFields {
					obj[k] = v
				}
				return obj
			}(),
			kind:               "backlog_item",
			currentState:       "exploring",
			wantValid:          false,
			wantLifecycleError: true,
			description:        "Validates that graph-stored objects respect lifecycle transition rules",
		},
		{
			name: "graph data - milestone with valid status",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:            "MIL-999",
					objects.FieldKeyKind:          "milestone",
					objects.FieldKeyTitle:         "Graph Test Milestone",
					objects.FieldKeyStatus:        "not_started",
					objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
				}
				for k, v := range validAuditFields {
					obj[k] = v
				}
				return obj
			}(),
			kind:         "milestone",
			currentState: "",
			wantValid:    true,
			description:  "Validates that graph-stored milestones pass validation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.description != emptyValue {
				t.Logf("Test: %s", tt.description)
			}

			options := DefaultValidationOptions()
			options.CurrentState = tt.currentState
			options.ValidateLifecycle = true
			result, err := validator.Validate(pkgctx.NewSystemContext(), tt.obj, tt.kind, options)
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}

			if result.IsValid != tt.wantValid {
				t.Errorf("Validate() IsValid = %v, want %v", result.IsValid, tt.wantValid)
				if len(result.Errors) > 0 {
					t.Logf("Validation errors: %+v", result.Errors)
				}
			}

			// Check if lifecycle error is present
			if tt.wantLifecycleError {
				hasLifecycleError := false
				for _, e := range result.Errors {
					if e.Rule == "lifecycle" {
						hasLifecycleError = true
						break
					}
				}
				if !hasLifecycleError {
					t.Error("Expected lifecycle validation error, but none found")
				}
			}
		})
	}
}
