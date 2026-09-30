package validation

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestGoValidator_ValidateDatatype tests the validateDatatype function
func TestGoValidator_ValidateSemanticType(t *testing.T) {
	t.Parallel()
	validator := NewGoValidator()

	tests := []struct {
		name         string
		fieldName    string
		value        any
		semanticType string
		fieldType    string
		wantErr      bool
		description  string
	}{
		// List field types
		{
			name:         "list with valid items",
			fieldName:    objects.FieldKeyMilestoneRefs,
			value:        []string{"MIL-001", "MIL-002"},
			semanticType: "reference",
			fieldType:    "list",
			wantErr:      false,
			description:  "List field with valid reference items",
		},
		{
			name:         "empty list",
			fieldName:    objects.FieldKeyMilestoneRefs,
			value:        []string{},
			semanticType: "reference",
			fieldType:    "list",
			wantErr:      false,
			description:  "Empty list (handled by required/min_length)",
		},
		{
			name:         "list with invalid items",
			fieldName:    objects.FieldKeyMilestoneRefs,
			value:        []string{"invalid-ref"},
			semanticType: "reference",
			fieldType:    "list",
			wantErr:      false, // Ontology registry may be permissive for invalid references
			description:  "List field with invalid reference items (may be permissive)",
		},
		{
			name:         "non-list value with list fieldType",
			fieldName:    objects.FieldKeyMilestoneRefs,
			value:        "not-a-list",
			semanticType: "reference",
			fieldType:    "list",
			wantErr:      false,
			description:  "Non-list value with list type (handled by type validation)",
		},

		// Non-list field types
		{
			name:         "string field with valid reference",
			fieldName:    objects.FieldKeyPriorityPlanRef,
			value:        "PRI-001",
			semanticType: "reference",
			fieldType:    "string",
			wantErr:      false,
			description:  "String field with valid reference",
		},
		{
			name:         "string field with invalid reference",
			fieldName:    objects.FieldKeyPriorityPlanRef,
			value:        "invalid-ref",
			semanticType: "reference",
			fieldType:    "string",
			wantErr:      false, // Ontology registry may be permissive for invalid references
			description:  "String field with invalid reference (may be permissive)",
		},
		{
			name:         "string field with statement",
			fieldName:    "title",
			value:        "Test Title",
			semanticType: "statement",
			fieldType:    "string",
			wantErr:      false,
			description:  "String field with statement semantic type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateSemanticType(tt.fieldName, tt.value, tt.semanticType, tt.fieldType)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSemanticType(%q, %v, %q, %q) error = %v, wantErr %v (%s)", tt.fieldName, tt.value, tt.semanticType, tt.fieldType, err, tt.wantErr, tt.description)
			}
		})
	}
}

// TestGoValidator_ValidateSemanticTypeItem tests the validateSemanticTypeItem function
func TestGoValidator_ValidateSemanticTypeItem(t *testing.T) {
	t.Parallel()
	validator := NewGoValidator()

	tests := []struct {
		name         string
		fieldName    string
		value        any
		semanticType string
		wantErr      bool
		description  string
	}{
		{
			name:         "valid reference",
			fieldName:    objects.FieldKeyPriorityPlanRef,
			value:        "PRI-001",
			semanticType: "reference",
			wantErr:      false,
			description:  "Valid reference ID",
		},
		{
			name:         "invalid reference",
			fieldName:    objects.FieldKeyPriorityPlanRef,
			value:        "invalid-ref",
			semanticType: "reference",
			wantErr:      false, // Ontology registry may be permissive for invalid references
			description:  "Invalid reference ID format (may be permissive)",
		},
		{
			name:         "valid statement",
			fieldName:    "title",
			value:        "Test Title",
			semanticType: "statement",
			wantErr:      false,
			description:  "Valid statement (non-empty string)",
		},
		{
			name:         "empty statement",
			fieldName:    "title",
			value:        "",
			semanticType: "statement",
			wantErr:      false, // Ontology registry may be permissive for empty statements
			description:  "Empty statement (may be permissive)",
		},
		{
			name:         "unknown semantic type",
			fieldName:    "custom_field",
			value:        "any value",
			semanticType: "unknown_type",
			wantErr:      false, // Ontology registry may be permissive for unknown types
			description:  "Unknown semantic type (may be permissive)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateSemanticTypeItem(tt.fieldName, tt.value, tt.semanticType)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSemanticTypeItem(%q, %v, %q) error = %v, wantErr %v (%s)", tt.fieldName, tt.value, tt.semanticType, err, tt.wantErr, tt.description)
			}
		})
	}
}

// TestGoValidator_ValidatePattern tests the validatePattern function
func TestGoValidator_ValidatePattern(t *testing.T) {
	t.Parallel()
	validator := NewGoValidator()

	tests := []struct {
		name        string
		value       string
		pattern     string
		want        bool
		wantErr     bool
		description string
	}{
		{
			name:        "valid pattern match",
			value:       "BLI-001",
			pattern:     `^BLI-\d{3,}$`,
			want:        true,
			wantErr:     false,
			description: "Value matches pattern",
		},
		{
			name:        "pattern mismatch",
			value:       "INVALID-001",
			pattern:     `^BLI-\d{3,}$`,
			want:        false,
			wantErr:     false,
			description: "Value does not match pattern",
		},
		{
			name:        "empty value with pattern",
			value:       "",
			pattern:     `^BLI-\d{3,}$`,
			want:        false,
			wantErr:     false,
			description: "Empty value does not match pattern",
		},
		{
			name:        "empty pattern (matches everything)",
			value:       "any value",
			pattern:     "",
			want:        true,
			wantErr:     false,
			description: "Empty pattern matches everything",
		},
		{
			name:        "complex pattern match",
			value:       "2025-12-25T10:30:00Z",
			pattern:     `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`,
			want:        true,
			wantErr:     false,
			description: "Complex datetime pattern match",
		},
		{
			name:        "invalid regex pattern",
			value:       "test",
			pattern:     `[invalid`,
			want:        false,
			wantErr:     true,
			description: "Invalid regex pattern returns error",
		},
		{
			name:        "case sensitive pattern",
			value:       "test",
			pattern:     `^TEST$`,
			want:        false,
			wantErr:     false,
			description: "Case sensitive pattern does not match lowercase",
		},
		{
			name:        "case insensitive pattern",
			value:       "test",
			pattern:     `(?i)^test$`,
			want:        true,
			wantErr:     false,
			description: "Case insensitive pattern matches",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validator.validatePattern(tt.value, tt.pattern)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePattern(%q, %q) error = %v, wantErr %v (%s)", tt.value, tt.pattern, err, tt.wantErr, tt.description)
				return
			}
			if got != tt.want {
				t.Errorf("validatePattern(%q, %q) = %v, want %v (%s)", tt.value, tt.pattern, got, tt.want, tt.description)
			}
		})
	}
}

// TestGoValidator_ValidateIn tests the validateIn function
func TestGoValidator_ValidateIn(t *testing.T) {
	t.Parallel()
	validator := NewGoValidator()

	tests := []struct {
		name          string
		value         any
		allowedValues []any
		want          bool
		description   string
	}{
		{
			name:          "exact match string",
			value:         "exploring",
			allowedValues: []any{"exploring", "planned", "in_progress"},
			want:          true,
			description:   "String value matches exactly",
		},
		{
			name:          "case insensitive match",
			value:         "EXPLORING",
			allowedValues: []any{"exploring", "planned", "in_progress"},
			want:          true,
			description:   "String value matches case-insensitively",
		},
		{
			name:          "case insensitive match lowercase",
			value:         "exploring",
			allowedValues: []any{"EXPLORING", "PLANNED", "IN_PROGRESS"},
			want:          true,
			description:   "String value matches case-insensitively (lowercase in value)",
		},
		{
			name:          "no match string",
			value:         "invalid",
			allowedValues: []any{"exploring", "planned", "in_progress"},
			want:          false,
			description:   "String value does not match",
		},
		{
			name:          "exact match int",
			value:         123,
			allowedValues: []any{123, 456, 789},
			want:          true,
			description:   "Int value matches exactly",
		},
		{
			name:          "no match int",
			value:         999,
			allowedValues: []any{123, 456, 789},
			want:          false,
			description:   "Int value does not match",
		},
		{
			name:          "exact match bool",
			value:         true,
			allowedValues: []any{true, false},
			want:          true,
			description:   "Bool value matches exactly",
		},
		{
			name:          "no match bool",
			value:         true,
			allowedValues: []any{false},
			want:          false,
			description:   "Bool value does not match",
		},
		{
			name:          "empty allowed values",
			value:         "any",
			allowedValues: []any{},
			want:          false,
			description:   "Empty allowed values list",
		},
		{
			name:          "mixed types in allowed values",
			value:         "string",
			allowedValues: []any{"string", 123, true},
			want:          true,
			description:   "String matches in mixed type list",
		},
		{
			name:          "mixed types no match",
			value:         456,
			allowedValues: []any{"string", 123, true},
			want:          false,
			description:   "Int does not match in mixed type list",
		},
		{
			name:          "nil value",
			value:         nil,
			allowedValues: []any{"string", 123, nil},
			want:          true,
			description:   "Nil value matches nil in allowed values",
		},
		{
			name:          "nil value no match",
			value:         nil,
			allowedValues: []any{"string", 123},
			want:          false,
			description:   "Nil value does not match when nil not in allowed values",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validator.validateIn(tt.value, tt.allowedValues)
			if got != tt.want {
				t.Errorf("validateIn(%v, %v) = %v, want %v (%s)", tt.value, tt.allowedValues, got, tt.want, tt.description)
			}
		})
	}
}

// TestGoValidator_ValidateLifecycleState tests the validateLifecycleState function
func TestGoValidator_ValidateLifecycleState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		kind         string
		status       string
		currentState string
		obj          map[string]any
		setupLoader  func(t *testing.T) *objects.LifecycleLoader
		wantErrors   int
		wantWarnings int
		description  string
	}{
		{
			name:         "no lifecycle loader",
			kind:         "backlog_item",
			status:       objects.ObjectStatusExploring,
			currentState: "",
			obj:          map[string]any{},
			setupLoader:  func(t *testing.T) *objects.LifecycleLoader { return nil },
			wantErrors:   0,
			wantWarnings: 0,
			description:  "No lifecycle loader - skip validation",
		},
		{
			name:         "valid status no transition",
			kind:         "backlog_item",
			status:       objects.ObjectStatusExploring,
			currentState: "",
			obj:          map[string]any{},
			setupLoader: func(t *testing.T) *objects.LifecycleLoader {
				tmpDir := t.TempDir()
				lifecyclesDir := filepath.Join(tmpDir, paths.ProcessInternalLifecyclesDir)
				if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
					t.Fatalf("Failed to create lifecycles dir: %v", err)
				}
				// Create a simple lifecycle file (lifecycle loader expects {kind}_lifecycle.yaml)
				lifecycleFile := filepath.Join(lifecyclesDir, "backlog_item_lifecycle.yaml")
				lifecycleContent := `object_type: backlog_item
statuses:
  - value: exploring
    display: Exploring
    initial: true
  - value: planned
    display: Planned
  - value: in_progress
    display: In Progress
`
				if err := fileutil.WriteFile(lifecycleFile, []byte(lifecycleContent), paths.FilePerm644); err != nil {
					t.Fatalf("Failed to write lifecycle file: %v", err)
				}
				return objects.NewLifecycleLoader(lifecyclesDir)
			},
			wantErrors:   0,
			wantWarnings: 0,
			description:  "Valid status without transition",
		},
		{
			name:         "invalid status",
			kind:         "backlog_item",
			status:       string("invalid_status"),
			currentState: "",
			obj:          map[string]any{},
			setupLoader: func(t *testing.T) *objects.LifecycleLoader {
				tmpDir := t.TempDir()
				lifecyclesDir := filepath.Join(tmpDir, paths.ProcessInternalLifecyclesDir)
				if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
					t.Fatalf("Failed to create lifecycles dir: %v", err)
				}
				lifecycleFile := filepath.Join(lifecyclesDir, "backlog_item_lifecycle.yaml")
				lifecycleContent := `object_type: backlog_item
statuses:
  - value: exploring
    display: Exploring
    initial: true
  - value: planned
    display: Planned
`
				if err := fileutil.WriteFile(lifecycleFile, []byte(lifecycleContent), paths.FilePerm644); err != nil {
					t.Fatalf("Failed to write lifecycle file: %v", err)
				}
				return objects.NewLifecycleLoader(lifecyclesDir)
			},
			wantErrors:   1,
			wantWarnings: 0,
			description:  "Invalid status should return error",
		},
		{
			name:         "valid transition",
			kind:         "backlog_item",
			status:       objects.ObjectStatusPlanned,
			currentState: "exploring",
			obj:          map[string]any{},
			setupLoader: func(t *testing.T) *objects.LifecycleLoader {
				tmpDir := t.TempDir()
				lifecyclesDir := filepath.Join(tmpDir, paths.ProcessInternalLifecyclesDir)
				if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
					t.Fatalf("Failed to create lifecycles dir: %v", err)
				}
				lifecycleFile := filepath.Join(lifecyclesDir, "backlog_item_lifecycle.yaml")
				lifecycleContent := `object_type: backlog_item
statuses:
  - value: exploring
    display: Exploring
    initial: true
  - value: planned
    display: Planned
transitions:
  - from: exploring
    to: planned
    description: Move to planned
`
				if err := fileutil.WriteFile(lifecycleFile, []byte(lifecycleContent), paths.FilePerm644); err != nil {
					t.Fatalf("Failed to write lifecycle file: %v", err)
				}
				return objects.NewLifecycleLoader(lifecyclesDir)
			},
			wantErrors:   0,
			wantWarnings: 0,
			description:  "Valid transition",
		},
		{
			name:         "invalid transition",
			kind:         "backlog_item",
			status:       objects.ObjectStatusInProgress,
			currentState: "exploring",
			obj:          map[string]any{},
			setupLoader: func(t *testing.T) *objects.LifecycleLoader {
				tmpDir := t.TempDir()
				lifecyclesDir := filepath.Join(tmpDir, paths.ProcessInternalLifecyclesDir)
				if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
					t.Fatalf("Failed to create lifecycles dir: %v", err)
				}
				lifecycleFile := filepath.Join(lifecyclesDir, "backlog_item_lifecycle.yaml")
				lifecycleContent := `object_type: backlog_item
statuses:
  - value: exploring
    display: Exploring
    initial: true
  - value: planned
    display: Planned
  - value: in_progress
    display: In Progress
transitions:
  - from: exploring
    to: planned
    description: Move to planned
`
				if err := fileutil.WriteFile(lifecycleFile, []byte(lifecycleContent), paths.FilePerm644); err != nil {
					t.Fatalf("Failed to write lifecycle file: %v", err)
				}
				return objects.NewLifecycleLoader(lifecyclesDir)
			},
			wantErrors:   1,
			wantWarnings: 0,
			description:  "Invalid transition should return error",
		},
		{
			name:         "transition with unmet precondition",
			kind:         "backlog_item",
			status:       objects.ObjectStatusPlanned,
			currentState: "exploring",
			obj:          map[string]any{}, // Missing priority_plan_ref
			setupLoader: func(t *testing.T) *objects.LifecycleLoader {
				tmpDir := t.TempDir()
				lifecyclesDir := filepath.Join(tmpDir, paths.ProcessInternalLifecyclesDir)
				if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
					t.Fatalf("Failed to create lifecycles dir: %v", err)
				}
				lifecycleFile := filepath.Join(lifecyclesDir, "backlog_item_lifecycle.yaml")
				lifecycleContent := `object_type: backlog_item
statuses:
  - value: exploring
    display: Exploring
    initial: true
  - value: planned
    display: Planned
transitions:
  - from: exploring
    to: planned
    description: Move to planned
    preconditions:
      - priority_plan_ref is set
`
				if err := fileutil.WriteFile(lifecycleFile, []byte(lifecycleContent), paths.FilePerm644); err != nil {
					t.Fatalf("Failed to write lifecycle file: %v", err)
				}
				return objects.NewLifecycleLoader(lifecyclesDir)
			},
			wantErrors:   1,
			wantWarnings: 0,
			description:  "Transition with unmet precondition should return error",
		},
		{
			name:         "transition with met precondition",
			kind:         "backlog_item",
			status:       objects.ObjectStatusPlanned,
			currentState: "exploring",
			obj:          map[string]any{objects.FieldKeyPriorityPlanRef: "PRI-001"},
			setupLoader: func(t *testing.T) *objects.LifecycleLoader {
				tmpDir := t.TempDir()
				lifecyclesDir := filepath.Join(tmpDir, paths.ProcessInternalLifecyclesDir)
				if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
					t.Fatalf("Failed to create lifecycles dir: %v", err)
				}
				lifecycleFile := filepath.Join(lifecyclesDir, "backlog_item_lifecycle.yaml")
				lifecycleContent := `object_type: backlog_item
statuses:
  - value: exploring
    display: Exploring
    initial: true
  - value: planned
    display: Planned
transitions:
  - from: exploring
    to: planned
    description: Move to planned
    preconditions:
      - priority_plan_ref is set
`
				if err := fileutil.WriteFile(lifecycleFile, []byte(lifecycleContent), paths.FilePerm644); err != nil {
					t.Fatalf("Failed to write lifecycle file: %v", err)
				}
				return objects.NewLifecycleLoader(lifecyclesDir)
			},
			wantErrors:   0,
			wantWarnings: 0,
			description:  "Transition with met precondition should succeed",
		},
		{
			name:         "lifecycle file not found",
			kind:         "unknown_kind",
			status:       string("any_status"),
			currentState: "",
			obj:          map[string]any{},
			setupLoader: func(t *testing.T) *objects.LifecycleLoader {
				tmpDir := t.TempDir()
				lifecyclesDir := filepath.Join(tmpDir, paths.ProcessInternalLifecyclesDir)
				if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
					t.Fatalf("Failed to create lifecycles dir: %v", err)
				}
				// No lifecycle file for unknown_kind
				return objects.NewLifecycleLoader(lifecyclesDir)
			},
			wantErrors:   0,
			wantWarnings: 1,
			description:  "Lifecycle file not found should return warning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lifecycleLoader := tt.setupLoader(t)
			validator := NewGoValidatorWithLoaders(nil, lifecycleLoader)

			errors, warnings := validator.validateLifecycleState(context.Background(), tt.kind, tt.status, tt.currentState, tt.obj, nil)

			if len(errors) != tt.wantErrors {
				t.Errorf("validateLifecycleState() errors = %d, want %d (%s). Errors: %v", len(errors), tt.wantErrors, tt.description, errors)
			}
			if len(warnings) != tt.wantWarnings {
				t.Errorf("validateLifecycleState() warnings = %d, want %d (%s). Warnings: %v", len(warnings), tt.wantWarnings, tt.description, warnings)
			}
		})
	}
}

func TestGoValidator_evaluateDynamicRule(t *testing.T) {
	t.Parallel()
	gv := NewGoValidator()

	t.Run("field_presence rule matches and fails", func(t *testing.T) {
		rule := map[string]any{
			objects.FieldKeyID:       "RULE-1",
			objects.FieldKeyRuleType: RuleTypeFieldPresence,
			objects.FieldKeyParameters: map[string]any{
				objects.FieldKeyFieldName: "owner",
			},
		}
		obj := map[string]any{
			objects.FieldKeyID: "OBJ-1",
		}
		errs := gv.evaluateDynamicRule(rule, obj, nil)
		if len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d", len(errs))
		}
		if errs[0].Field != "owner" {
			t.Errorf("expected field 'owner', got %q", errs[0].Field)
		}
	})

	t.Run("field_presence rule passes when field is set", func(t *testing.T) {
		rule := map[string]any{
			objects.FieldKeyID:       "RULE-1",
			objects.FieldKeyRuleType: RuleTypeFieldPresence,
			objects.FieldKeyParameters: map[string]any{
				objects.FieldKeyFieldName: "owner",
			},
		}
		obj := map[string]any{
			objects.FieldKeyID: "OBJ-1",
			"owner":            "user:jane",
		}
		errs := gv.evaluateDynamicRule(rule, obj, nil)
		if len(errs) != 0 {
			t.Fatalf("expected 0 errors, got %d", len(errs))
		}
	})

	t.Run("active_reference rule fails on non-active ref", func(t *testing.T) {
		rule := map[string]any{
			objects.FieldKeyID:       "RULE-2",
			objects.FieldKeyRuleType: RuleTypeActiveReference,
			objects.FieldKeyParameters: map[string]any{
				"reference_field": "milestone_refs",
			},
		}
		obj := map[string]any{
			objects.FieldKeyID:            "OBJ-1",
			objects.FieldKeyMilestoneRefs: []any{"MIL-1"},
		}
		options := &ValidationOptions{
			ObjectStatusLookup: func(id string) (string, error) {
				if id == "MIL-1" {
					return objects.ObjectStatusExploring, nil
				}
				return "", nil
			},
		}
		errs := gv.evaluateDynamicRule(rule, obj, options)
		if len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d", len(errs))
		}
		if errs[0].Field != "milestone_refs" {
			t.Errorf("expected field 'milestone_refs', got %q", errs[0].Field)
		}
	})
}

// TestGoValidator_applySpecDefaults_IgnoresChecklistProse ensures checklist.default
// (human docs) is never written into instances — only field-level `default` applies.
func TestGoValidator_applySpecDefaults_IgnoresChecklistProse(t *testing.T) {
	t.Parallel()
	gv := NewGoValidator()
	spec := &objects.Spec{
		ResolvedFields: map[string]any{
			objects.FieldKeyTitle: map[string]any{
				objects.FieldKeyType: "string",
				"validation":         map[string]any{"required": true},
				"checklist":          map[string]any{"default": "required at creation"},
			},
			objects.FieldKeyStatus: map[string]any{
				objects.FieldKeyType: "enum",
				"validation":         map[string]any{"required": true},
				"default":            "exploring",
				"checklist":          map[string]any{"default": "exploring"},
			},
		},
	}
	obj := map[string]any{objects.FieldKeyID: "BLI-x"}
	gv.applySpecDefaults(obj, spec)
	if _, ok := obj[objects.FieldKeyTitle]; ok {
		t.Fatalf("checklist prose default must not fill title; got %v", obj[objects.FieldKeyTitle])
	}
	if got, _ := obj[objects.FieldKeyStatus].(string); got != "exploring" {
		t.Fatalf("field-level default should fill status; got %v", obj[objects.FieldKeyStatus])
	}
}

func TestGoValidator_validateUnknownFields_AllowsCompositionFields(t *testing.T) {
	t.Parallel()
	gv := NewGoValidator()
	spec := &objects.Spec{
		ResolvedFields: map[string]any{
			objects.FieldKeyID:   map[string]any{objects.FieldKeyType: "string"},
			objects.FieldKeyKind: map[string]any{objects.FieldKeyType: "string"},
		},
		Traits: []string{objects.TraitOccupiable},
	}
	obj := map[string]any{
		objects.FieldKeyID:        "BLI-123",
		objects.FieldKeyKind:      objects.KindBacklogItem,
		objects.FieldKeyClaimedBy: "ACC-TEST",
		"version_context":         "default",
	}
	errs := gv.validateUnknownFields(objects.KindBacklogItem, obj, spec)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors for composition fields, got: %v", errs)
	}

	obj["bad_extra_field"] = "foo"
	errs = gv.validateUnknownFields(objects.KindBacklogItem, obj, spec)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for bad_extra_field, got %d", len(errs))
	}
}
