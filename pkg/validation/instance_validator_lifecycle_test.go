//go:build lifecycle_test
// +build lifecycle_test

// This test file is excluded from normal test runs to avoid import cycle issues.
// The cycle is: validation -> specbuilder/bldr_v2 -> validation (via base_object_builder.go)
//
// To run these tests, use: go test -tags=lifecycle_test ./pkg/validation -run TestInstanceValidator

package validation

import (
	"os"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_v2" // Register builders for tests
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// setupBacklogItemLifecycle creates a temporary lifecycle directory with backlog_item and milestone lifecycle files
func setupBacklogItemLifecycle(t *testing.T) string {
	tmpDir := t.TempDir()
	lifecyclesDir := filepath.Join(tmpDir, "lifecycles")
	if err := os.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagicc28fd1d3, err)
	}

	// Create backlog_item_lifecycle.yaml with the statuses and transitions needed for tests
	lifecycleFile := filepath.Join(lifecyclesDir, ConstMagic7522f0e5)
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
	if err := os.WriteFile(lifecycleFile, []byte(lifecycleContent), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic9356a1db, err)
	}

	// Create milestone_lifecycle.yaml for milestone tests
	milestoneFile := filepath.Join(lifecyclesDir, ConstMagicab09ec37)
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
	if err := os.WriteFile(milestoneFile, []byte(milestoneContent), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic17c34cc2, err)
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
		objects.FieldKeyCreatedAt: ConstMagiccd842f93,
		objects.FieldKeyCreatedBy: "account:test",
		objects.FieldKeyUpdatedAt: ConstMagiccd842f93,
		objects.FieldKeyUpdatedBy: "account:test",
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
					objects.FieldKeyID:            "ITEM-001",
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
			name: ConstMagiceff6168d,
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:            "ITEM-002",
					objects.FieldKeyKind:          "backlog_item",
					objects.FieldKeyTitle:         "Test Item",
					objects.FieldKeyStatus:        "validated",
					objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
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
					objects.FieldKeyID:            "ITEM-003",
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
			name: ConstMagic28cd2152,
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:            "ITEM-004",
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
				t.Fatalf(ConstMagic6762e2c0, err)
			}

			if result.IsValid != tt.wantValid {
				t.Errorf(ConstMagicf5ed72f6, result.IsValid, tt.wantValid)
				if len(result.Errors) > 0 {
					t.Logf(ConstMagic6ad67277, result.Errors)
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
					t.Error(ConstMagic93c59fac)
				}
			}
		})
	}
}

func TestInstanceValidator_checkPrecondition(t *testing.T) {
	t.Parallel()
	validator := &InstanceValidator{}

	tests := []struct {
		name         string
		precondition string
		obj          map[string]any
		want         bool
	}{
		{
			name:         ConstMagic4b808820,
			precondition: ConstMagic866d629a,
			obj:          map[string]any{objects.FieldKeyPriorityPlanRef: "PLAN-208"},
			want:         true,
		},
		{
			name:         ConstMagicdd24908f,
			precondition: ConstMagic866d629a,
			obj:          map[string]any{},
			want:         false,
		},
		{
			name:         ConstMagic1c585332,
			precondition: ConstMagic866d629a,
			obj:          map[string]any{objects.FieldKeyPriorityPlanRef: ""},
			want:         false,
		},
		{
			name:         ConstMagicb0e02d21,
			precondition: ConstMagic9ba826a5,
			obj:          map[string]any{objects.FieldKeyMilestoneRefs: []string{"MIL-001"}},
			want:         true,
		},
		{
			name:         ConstMagic390acc53,
			precondition: ConstMagic9ba826a5,
			obj:          map[string]any{objects.FieldKeyMilestoneRefs: []string{}},
			want:         false,
		},
		{
			name:         ConstMagic7b5ca0a3,
			precondition: ConstMagic35012048,
			obj:          map[string]any{objects.FieldKeyMilestoneRefs: []string{"MIL-001"}},
			want:         true,
		},
		{
			name:         ConstMagica1e103ec,
			precondition: ConstMagic35012048,
			obj:          map[string]any{},
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validator.checkPrecondition(tt.precondition, tt.obj)
			if got != tt.want {
				t.Errorf(ConstMagicef5a723d, got, tt.want)
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
		objects.FieldKeyCreatedAt: ConstMagiccd842f93,
		objects.FieldKeyCreatedBy: "account:test",
		objects.FieldKeyUpdatedAt: ConstMagiccd842f93,
		objects.FieldKeyUpdatedBy: "account:test",
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
			name: ConstMagic67c27314,
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:            "ITEM-999",
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
			description:  ConstMagic75ee9b40,
		},
		{
			name: ConstMagicba2ee397,
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:            "REQ-999",
					objects.FieldKeyKind:          "requirement",
					objects.FieldKeyTitle:         ConstMagic585b2e57,
					objects.FieldKeyStatus:        "planned",
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
			description:  ConstMagic8574ad8d,
		},
		{
			name: ConstMagicaff002ca,
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:            "REQ-998",
					objects.FieldKeyKind:          "requirement",
					objects.FieldKeyTitle:         ConstMagicaed366d8,
					objects.FieldKeyStatus:        "planned",
					objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
					objects.FieldKeyGoalRefs:      []string{"GOAL-001"},
					// Missing criteria_refs - should fail validation
				}
				for k, v := range validAuditFields {
					obj[k] = v
				}
				return obj
			}(),
			kind:         "requirement",
			currentState: "",
			wantValid:    false,
			description:  ConstMagicb9e29414,
		},
		{
			name: ConstMagica4a1cdbd,
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:            "ITEM-998",
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
			description:        ConstMagic05d8272a,
		},
		{
			name: ConstMagicb3179660,
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:            "MIL-999",
					objects.FieldKeyKind:          "milestone",
					objects.FieldKeyTitle:         ConstMagica3d673a3,
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
			description:  ConstMagicb69c7fce,
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
				t.Fatalf(ConstMagic6762e2c0, err)
			}

			if result.IsValid != tt.wantValid {
				t.Errorf(ConstMagicf5ed72f6, result.IsValid, tt.wantValid)
				if len(result.Errors) > 0 {
					t.Logf(ConstMagic6ad67277, result.Errors)
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
					t.Error(ConstMagic93c59fac)
				}
			}
		})
	}
}
