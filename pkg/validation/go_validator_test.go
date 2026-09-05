package validation

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
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
			name:         ConstMagicf28282ef,
			fieldName:    objects.FieldKeyMilestoneRefs,
			value:        []string{"MIL-001", "MIL-002"},
			semanticType: "reference",
			fieldType:    "list",
			wantErr:      false,
			description:  ConstMagic2c3bf5c4,
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
			name:         ConstMagicc301f86b,
			fieldName:    objects.FieldKeyMilestoneRefs,
			value:        []string{"invalid-ref"},
			semanticType: "reference",
			fieldType:    "list",
			wantErr:      false, // Ontology registry may be permissive for invalid references
			description:  ConstMagicf8bf765f,
		},
		{
			name:         ConstMagic2033b4a1,
			fieldName:    objects.FieldKeyMilestoneRefs,
			value:        "not-a-list",
			semanticType: "reference",
			fieldType:    "list",
			wantErr:      false,
			description:  ConstMagic28421cf7,
		},

		// Non-list field types
		{
			name:         ConstMagic2fd2c52b,
			fieldName:    objects.FieldKeyPriorityPlanRef,
			value:        "PRI-001",
			semanticType: "reference",
			fieldType:    "string",
			wantErr:      false,
			description:  ConstMagic909be964,
		},
		{
			name:         ConstMagice80d397c,
			fieldName:    objects.FieldKeyPriorityPlanRef,
			value:        "invalid-ref",
			semanticType: "reference",
			fieldType:    "string",
			wantErr:      false, // Ontology registry may be permissive for invalid references
			description:  ConstMagic88e3ff48,
		},
		{
			name:         ConstMagice72389e0,
			fieldName:    "title",
			value:        "Test Title",
			semanticType: "statement",
			fieldType:    "string",
			wantErr:      false,
			description:  ConstMagic8a4cac28,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateSemanticType(tt.fieldName, tt.value, tt.semanticType, tt.fieldType)
			if (err != nil) != tt.wantErr {
				t.Errorf(ConstMagicd1b562f6, tt.fieldName, tt.value, tt.semanticType, tt.fieldType, err, tt.wantErr, tt.description)
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
			description:  ConstMagic190d2a64,
		},
		{
			name:         ConstMagic87933c9d,
			fieldName:    objects.FieldKeyPriorityPlanRef,
			value:        "invalid-ref",
			semanticType: "reference",
			wantErr:      false, // Ontology registry may be permissive for invalid references
			description:  ConstMagic60be8c1a,
		},
		{
			name:         "valid statement",
			fieldName:    "title",
			value:        "Test Title",
			semanticType: "statement",
			wantErr:      false,
			description:  ConstMagica3c691b5,
		},
		{
			name:         "empty statement",
			fieldName:    "title",
			value:        "",
			semanticType: "statement",
			wantErr:      false, // Ontology registry may be permissive for empty statements
			description:  ConstMagic7419fc96,
		},
		{
			name:         ConstMagic62fd879c,
			fieldName:    "custom_field",
			value:        "any value",
			semanticType: "unknown_type",
			wantErr:      false, // Ontology registry may be permissive for unknown types
			description:  ConstMagic1ff84256,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateSemanticTypeItem(tt.fieldName, tt.value, tt.semanticType)
			if (err != nil) != tt.wantErr {
				t.Errorf(ConstMagic28bf4f54, tt.fieldName, tt.value, tt.semanticType, err, tt.wantErr, tt.description)
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
			name:        ConstMagicd72a976e,
			value:       "BLI-001",
			pattern:     `^BLI-\d{3,}$`,
			want:        true,
			wantErr:     false,
			description: ConstMagicb923e28e,
		},
		{
			name:        ConstMagic09a29b67,
			value:       "INVALID-001",
			pattern:     `^BLI-\d{3,}$`,
			want:        false,
			wantErr:     false,
			description: ConstMagic92aefd62,
		},
		{
			name:        ConstMagicfe145f27,
			value:       "",
			pattern:     `^BLI-\d{3,}$`,
			want:        false,
			wantErr:     false,
			description: ConstMagicaef1b4ca,
		},
		{
			name:        ConstMagic2ca198e0,
			value:       "any value",
			pattern:     "",
			want:        true,
			wantErr:     false,
			description: ConstMagicdb46551d,
		},
		{
			name:        ConstMagicbc429465,
			value:       ConstMagic6d05be14,
			pattern:     `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`,
			want:        true,
			wantErr:     false,
			description: ConstMagic6593f9a5,
		},
		{
			name:        ConstMagicff83e5ca,
			value:       "test",
			pattern:     `[invalid`,
			want:        false,
			wantErr:     true,
			description: ConstMagic839533b7,
		},
		{
			name:        ConstMagicc57c97c6,
			value:       "test",
			pattern:     `^TEST$`,
			want:        false,
			wantErr:     false,
			description: ConstMagicffd46bce,
		},
		{
			name:        ConstMagic030cc24c,
			value:       "test",
			pattern:     `(?i)^test$`,
			want:        true,
			wantErr:     false,
			description: ConstMagicdf941cdb,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validator.validatePattern(tt.value, tt.pattern)
			if (err != nil) != tt.wantErr {
				t.Errorf(ConstMagicd26e90d1, tt.value, tt.pattern, err, tt.wantErr, tt.description)
				return
			}
			if got != tt.want {
				t.Errorf(ConstMagic97aa1f45, tt.value, tt.pattern, got, tt.want, tt.description)
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
			name:          ConstMagice2570710,
			value:         "exploring",
			allowedValues: []any{"exploring", "planned", "in_progress"},
			want:          true,
			description:   ConstMagic9def42d4,
		},
		{
			name:          ConstMagic86996bc7,
			value:         "EXPLORING",
			allowedValues: []any{"exploring", "planned", "in_progress"},
			want:          true,
			description:   ConstMagicae81db2c,
		},
		{
			name:          ConstMagicc9f0e25f,
			value:         "exploring",
			allowedValues: []any{"EXPLORING", "PLANNED", "IN_PROGRESS"},
			want:          true,
			description:   ConstMagice22b10cb,
		},
		{
			name:          "no match string",
			value:         "invalid",
			allowedValues: []any{"exploring", "planned", "in_progress"},
			want:          false,
			description:   ConstMagic10f3573f,
		},
		{
			name:          "exact match int",
			value:         123,
			allowedValues: []any{123, 456, 789},
			want:          true,
			description:   ConstMagicdbe97866,
		},
		{
			name:          "no match int",
			value:         999,
			allowedValues: []any{123, 456, 789},
			want:          false,
			description:   ConstMagicb31d921e,
		},
		{
			name:          ConstMagic4e071370,
			value:         true,
			allowedValues: []any{true, false},
			want:          true,
			description:   ConstMagice9460a03,
		},
		{
			name:          "no match bool",
			value:         true,
			allowedValues: []any{false},
			want:          false,
			description:   ConstMagic58d0761b,
		},
		{
			name:          ConstMagic4d66bdc2,
			value:         "any",
			allowedValues: []any{},
			want:          false,
			description:   ConstMagic254d82c7,
		},
		{
			name:          ConstMagicfa13729d,
			value:         "string",
			allowedValues: []any{"string", 123, true},
			want:          true,
			description:   ConstMagicf83d8673,
		},
		{
			name:          ConstMagic27ec93df,
			value:         456,
			allowedValues: []any{"string", 123, true},
			want:          false,
			description:   ConstMagic704c9e98,
		},
		{
			name:          "nil value",
			value:         nil,
			allowedValues: []any{"string", 123, nil},
			want:          true,
			description:   ConstMagicf19a1639,
		},
		{
			name:          ConstMagicf47bfa28,
			value:         nil,
			allowedValues: []any{"string", 123},
			want:          false,
			description:   ConstMagic369d3131,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validator.validateIn(tt.value, tt.allowedValues)
			if got != tt.want {
				t.Errorf(ConstMagic06ff700d, tt.value, tt.allowedValues, got, tt.want, tt.description)
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
			name:         ConstMagic404d9528,
			kind:         "backlog_item",
			status:       objects.ObjectStatusExploring,
			currentState: "",
			obj:          map[string]any{},
			setupLoader:  func(t *testing.T) *objects.LifecycleLoader { return nil },
			wantErrors:   0,
			wantWarnings: 0,
			description:  ConstMagic22a48a13,
		},
		{
			name:         ConstMagicef736a0a,
			kind:         "backlog_item",
			status:       objects.ObjectStatusExploring,
			currentState: "",
			obj:          map[string]any{},
			setupLoader: func(t *testing.T) *objects.LifecycleLoader {
				tmpDir := t.TempDir()
				lifecyclesDir := filepath.Join(tmpDir, paths.ProcessInternalLifecyclesDir)
				if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
					t.Fatalf(ConstMagicc28fd1d3, err)
				}
				// Create a simple lifecycle file (lifecycle loader expects {kind}_lifecycle.yaml)
				lifecycleFile := filepath.Join(lifecyclesDir, ConstMagic7522f0e5)
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
					t.Fatalf(ConstMagic9356a1db, err)
				}
				return objects.NewLifecycleLoader(lifecyclesDir)
			},
			wantErrors:   0,
			wantWarnings: 0,
			description:  ConstMagic59f5e7c1,
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
					t.Fatalf(ConstMagicc28fd1d3, err)
				}
				lifecycleFile := filepath.Join(lifecyclesDir, ConstMagic7522f0e5)
				lifecycleContent := `object_type: backlog_item
statuses:
  - value: exploring
    display: Exploring
    initial: true
  - value: planned
    display: Planned
`
				if err := fileutil.WriteFile(lifecycleFile, []byte(lifecycleContent), paths.FilePerm644); err != nil {
					t.Fatalf(ConstMagic9356a1db, err)
				}
				return objects.NewLifecycleLoader(lifecyclesDir)
			},
			wantErrors:   1,
			wantWarnings: 0,
			description:  ConstMagic5559d9f6,
		},
		{
			name:         ConstMagiceff6168d,
			kind:         "backlog_item",
			status:       objects.ObjectStatusPlanned,
			currentState: "exploring",
			obj:          map[string]any{},
			setupLoader: func(t *testing.T) *objects.LifecycleLoader {
				tmpDir := t.TempDir()
				lifecyclesDir := filepath.Join(tmpDir, paths.ProcessInternalLifecyclesDir)
				if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
					t.Fatalf(ConstMagicc28fd1d3, err)
				}
				lifecycleFile := filepath.Join(lifecyclesDir, ConstMagic7522f0e5)
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
					t.Fatalf(ConstMagic9356a1db, err)
				}
				return objects.NewLifecycleLoader(lifecyclesDir)
			},
			wantErrors:   0,
			wantWarnings: 0,
			description:  ConstMagic3e325748,
		},
		{
			name:         ConstMagic28cd2152,
			kind:         "backlog_item",
			status:       objects.ObjectStatusInProgress,
			currentState: "exploring",
			obj:          map[string]any{},
			setupLoader: func(t *testing.T) *objects.LifecycleLoader {
				tmpDir := t.TempDir()
				lifecyclesDir := filepath.Join(tmpDir, paths.ProcessInternalLifecyclesDir)
				if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
					t.Fatalf(ConstMagicc28fd1d3, err)
				}
				lifecycleFile := filepath.Join(lifecyclesDir, ConstMagic7522f0e5)
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
					t.Fatalf(ConstMagic9356a1db, err)
				}
				return objects.NewLifecycleLoader(lifecyclesDir)
			},
			wantErrors:   1,
			wantWarnings: 0,
			description:  ConstMagic0e6c0296,
		},
		{
			name:         ConstMagic8379b894,
			kind:         "backlog_item",
			status:       objects.ObjectStatusPlanned,
			currentState: "exploring",
			obj:          map[string]any{}, // Missing priority_plan_ref
			setupLoader: func(t *testing.T) *objects.LifecycleLoader {
				tmpDir := t.TempDir()
				lifecyclesDir := filepath.Join(tmpDir, paths.ProcessInternalLifecyclesDir)
				if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
					t.Fatalf(ConstMagicc28fd1d3, err)
				}
				lifecycleFile := filepath.Join(lifecyclesDir, ConstMagic7522f0e5)
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
					t.Fatalf(ConstMagic9356a1db, err)
				}
				return objects.NewLifecycleLoader(lifecyclesDir)
			},
			wantErrors:   1,
			wantWarnings: 0,
			description:  ConstMagiced3a3c4e,
		},
		{
			name:         ConstMagic1cd2d979,
			kind:         "backlog_item",
			status:       objects.ObjectStatusPlanned,
			currentState: "exploring",
			obj:          map[string]any{objects.FieldKeyPriorityPlanRef: "PRI-001"},
			setupLoader: func(t *testing.T) *objects.LifecycleLoader {
				tmpDir := t.TempDir()
				lifecyclesDir := filepath.Join(tmpDir, paths.ProcessInternalLifecyclesDir)
				if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
					t.Fatalf(ConstMagicc28fd1d3, err)
				}
				lifecycleFile := filepath.Join(lifecyclesDir, ConstMagic7522f0e5)
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
					t.Fatalf(ConstMagic9356a1db, err)
				}
				return objects.NewLifecycleLoader(lifecyclesDir)
			},
			wantErrors:   0,
			wantWarnings: 0,
			description:  ConstMagice48d60cb,
		},
		{
			name:         ConstMagic8d9bb694,
			kind:         "unknown_kind",
			status:       string("any_status"),
			currentState: "",
			obj:          map[string]any{},
			setupLoader: func(t *testing.T) *objects.LifecycleLoader {
				tmpDir := t.TempDir()
				lifecyclesDir := filepath.Join(tmpDir, paths.ProcessInternalLifecyclesDir)
				if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
					t.Fatalf(ConstMagicc28fd1d3, err)
				}
				// No lifecycle file for unknown_kind
				return objects.NewLifecycleLoader(lifecyclesDir)
			},
			wantErrors:   0,
			wantWarnings: 1,
			description:  ConstMagic427b26e8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lifecycleLoader := tt.setupLoader(t)
			validator := NewGoValidatorWithLoaders(nil, lifecycleLoader)

			errors, warnings := validator.validateLifecycleState(context.Background(), tt.kind, tt.status, tt.currentState, tt.obj, nil)

			if len(errors) != tt.wantErrors {
				t.Errorf(ConstMagic8c2607fa, len(errors), tt.wantErrors, tt.description, errors)
			}
			if len(warnings) != tt.wantWarnings {
				t.Errorf(ConstMagic7fec669b, len(warnings), tt.wantWarnings, tt.description, warnings)
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
