package validation

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/objects"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_v2" // Register builders for tests
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

func TestInstanceValidator_ValidateInstance(t *testing.T) {
	t.Parallel()
	// Create a spec loader with actual specs directory
	specsDir := filepath.Join("..", "..", paths.ProcessInternalObjectSpecsDir)
	specLoader := objects.NewSpecLoader(specsDir)

	// Set builder registry on spec loader to enable version-aware loading
	builderRegistry := builders.GetGlobalRegistry()
	adapter := builders.NewSpecLoaderAdapter(builderRegistry)
	specLoader.SetBuilderRegistry(adapter)

	// Validate dependency graph first
	errors := specLoader.ValidateDependencyGraph()
	if len(errors) > 0 {
		t.Logf(ConstMagic7b8406ae, errors)
		// Continue anyway for testing
	}

	// Create test-specific IDValidator for isolation
	testIDValidator := NewIDValidatorWithConfigs(
		getTestSpecsDir(t),
		getTestIDPrefixesConfig(),
		getTestPathsConfig(t),
	)
	if err := testIDValidator.LoadPatterns(); err != nil {
		t.Logf(ConstMagicd3d06733, err)
	}

	// Create GoValidator with injected IDValidator for test isolation
	lifecyclesDir := "" // Will use default
	lifecycleLoader := objects.NewLifecycleLoader(lifecyclesDir)
	goValidator := NewGoValidatorWithIDValidator(specLoader, lifecycleLoader, testIDValidator)

	// Register the custom validator in the global registry for this test
	// Note: This modifies global state, but tests should be isolated enough
	registry := GetGlobalRegistry()
	originalValidator := registry.Get("go")
	registry.Register("go", goValidator)
	defer func() {
		// Restore original validator after test
		if originalValidator != nil {
			registry.Register("go", originalValidator)
		}
	}()

	validator := NewInstanceValidator(specLoader)

	// Helper to create a minimal valid object with all required audit fields
	validAuditFields := map[string]any{
		objects.FieldKeyCreatedAt: ConstMagiccd842f93,
		objects.FieldKeyCreatedBy: "ACC-TEST",
		objects.FieldKeyUpdatedAt: ConstMagiccd842f93,
		objects.FieldKeyUpdatedBy: "ACC-TEST",
	}

	tests := []struct {
		name      string
		obj       map[string]any
		kind      string
		wantErr   bool
		wantValid bool
	}{
		{
			name: ConstMagic89cc97f5,
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:            "BLI-001",
					objects.FieldKeyKind:          "backlog_item",
					objects.FieldKeyTitle:         "Test Item",
					objects.FieldKeyStatus:        "exploring",
					objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
					objects.FieldKeyGoalRefs:      []any{"GOAL-123"},
				}
				// Add required audit fields
				for k, v := range validAuditFields {
					obj[k] = v
				}
				return obj
			}(),
			kind:      "backlog_item",
			wantErr:   false,
			wantValid: true,
		},
		{
			name: ConstMagicb26bea69,
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:            "BLI-002",
					objects.FieldKeyKind:          "backlog_item",
					objects.FieldKeyStatus:        "exploring",
					objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
					// Missing required "title" (no machine field-level default — checklist.default is docs-only)
				}
				// Add required audit fields
				for k, v := range validAuditFields {
					obj[k] = v
				}
				return obj
			}(),
			kind:      "backlog_item",
			wantErr:   false, // Validation doesn't return error, just invalid result
			wantValid: false,
		},
		{
			name: ConstMagiceaa72e25,
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:            "BLI-003",
					objects.FieldKeyKind:          "backlog_item",
					objects.FieldKeyTitle:         "Test Item",
					objects.FieldKeyStatus:        "invalid_status", // Invalid enum value
					objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
				}
				// Add required audit fields
				for k, v := range validAuditFields {
					obj[k] = v
				}
				return obj
			}(),
			kind:      "backlog_item",
			wantErr:   false,
			wantValid: false,
		},
		{
			name: "invalid pattern",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:            "INVALID-ID", // Doesn't match ^[A-Z]+-\d{3,}$ pattern
					objects.FieldKeyKind:          "backlog_item",
					objects.FieldKeyTitle:         "Test Item",
					objects.FieldKeyStatus:        "exploring",
					objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
				}
				// Add required audit fields
				for k, v := range validAuditFields {
					obj[k] = v
				}
				return obj
			}(),
			kind:      "backlog_item",
			wantErr:   false,
			wantValid: false, // Pattern validation should fail
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validator.ValidateInstance(tt.obj, tt.kind)
			if (err != nil) != tt.wantErr {
				t.Errorf(ConstMagicd6c62505, err, tt.wantErr)
				return
			}
			if result != nil && result.IsValid != tt.wantValid {
				t.Errorf(ConstMagic31defe61, result.IsValid, tt.wantValid)
				if len(result.Errors) > 0 {
					t.Logf(ConstMagic6ad67277, result.Errors)
				}
			}
		})
	}
}

// TestInstanceValidator_ValidateField tests the validateField function
// NOTE: Cannot use t.Parallel() - this test uses os.Setenv() which is incompatible with parallel execution
func TestInstanceValidator_ValidateField(t *testing.T) {
	testRoot := registerZQKTestRootForTest(t)

	specsDir := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagice81c38f8, err)
	}

	specLoader := objects.NewSpecLoader(specsDir)
	validator := NewInstanceValidator(specLoader)

	tests := []struct {
		name         string
		fieldName    string
		fieldValue   any
		fieldDef     map[string]any
		exists       bool
		obj          map[string]any
		wantErrors   int
		wantWarnings int
		description  string
	}{
		{
			name:         ConstMagic881eeb3b,
			fieldName:    "title",
			fieldValue:   nil,
			fieldDef:     map[string]any{objects.FieldKeyType: "string", "validation": map[string]any{"required": true}},
			exists:       false,
			obj:          map[string]any{},
			wantErrors:   1,
			wantWarnings: 0,
			description:  ConstMagic5614c851,
		},
		{
			name:         ConstMagic71aff506,
			fieldName:    "title",
			fieldValue:   "Test Title",
			fieldDef:     map[string]any{objects.FieldKeyType: "string", "validation": map[string]any{"required": true}},
			exists:       true,
			obj:          map[string]any{objects.FieldKeyTitle: "Test Title"},
			wantErrors:   0,
			wantWarnings: 0,
			description:  ConstMagic4538a076,
		},
		{
			name:         ConstMagic2f072f33,
			fieldName:    "description",
			fieldValue:   nil,
			fieldDef:     map[string]any{objects.FieldKeyType: "string", "validation": map[string]any{"required": false}},
			exists:       false,
			obj:          map[string]any{},
			wantErrors:   0,
			wantWarnings: 0,
			description:  ConstMagic4235e89e,
		},
		{
			name:         ConstMagic3e8e3dd1,
			fieldName:    "count",
			fieldValue:   123,
			fieldDef:     map[string]any{objects.FieldKeyType: "int", "validation": map[string]any{}},
			exists:       true,
			obj:          map[string]any{"count": 123},
			wantErrors:   0,
			wantWarnings: 0,
			description:  ConstMagic23e2d009,
		},
		{
			name:         ConstMagic884f86d7,
			fieldName:    "count",
			fieldValue:   "not a number",
			fieldDef:     map[string]any{objects.FieldKeyType: "int", "validation": map[string]any{}},
			exists:       true,
			obj:          map[string]any{"count": "not a number"},
			wantErrors:   1,
			wantWarnings: 0,
			description:  ConstMagic8eea0c95,
		},
		{
			name:         ConstMagic86e17216,
			fieldName:    "id",
			fieldValue:   "BLI-001",
			fieldDef:     map[string]any{objects.FieldKeyType: "string", "validation": map[string]any{"pattern": `^BLI-\d{3,}$`}},
			exists:       true,
			obj:          map[string]any{objects.FieldKeyID: "BLI-001"},
			wantErrors:   0,
			wantWarnings: 0,
			description:  ConstMagice4de4e3a,
		},
		{
			name:         ConstMagic7f929358,
			fieldName:    "title", // Use non-id field so regex validation is used (id field uses IDValidator)
			fieldValue:   "INVALID-001",
			fieldDef:     map[string]any{objects.FieldKeyType: "string", "validation": map[string]any{"pattern": `^BLI-\d{3,}$`}},
			exists:       true,
			obj:          map[string]any{objects.FieldKeyTitle: "INVALID-001"},
			wantErrors:   1,
			wantWarnings: 0,
			description:  ConstMagic44721b77,
		},
		{
			name:         ConstMagic1989dd0c,
			fieldName:    "status",
			fieldValue:   "exploring",
			fieldDef:     map[string]any{objects.FieldKeyType: "string", "validation": map[string]any{"enum": []any{"exploring", "planned", "in_progress"}}},
			exists:       true,
			obj:          map[string]any{objects.FieldKeyStatus: "exploring"},
			wantErrors:   0,
			wantWarnings: 0,
			description:  ConstMagic5fa5c9ad,
		},
		{
			name:         ConstMagic0c0fcfd4,
			fieldName:    "status",
			fieldValue:   "invalid",
			fieldDef:     map[string]any{objects.FieldKeyType: "string", "validation": map[string]any{"enum": []any{"exploring", "planned", "in_progress"}}},
			exists:       true,
			obj:          map[string]any{objects.FieldKeyStatus: "invalid"},
			wantErrors:   1,
			wantWarnings: 0,
			description:  ConstMagic04a5571d,
		},
		{
			name:         ConstMagic8962cc6d,
			fieldName:    "title", // Use non-id field so regex validation is used (id field uses IDValidator)
			fieldValue:   "Test Title",
			fieldDef:     map[string]any{objects.FieldKeyType: "string", "validation": map[string]any{"pattern": `[invalid`}},
			exists:       true,
			obj:          map[string]any{objects.FieldKeyTitle: "Test Title"},
			wantErrors:   0,
			wantWarnings: 1,
			description:  ConstMagic14af375a,
		},
		{
			name:         ConstMagica0c94438,
			fieldName:    "title",
			fieldValue:   "Test Title",
			fieldDef:     map[string]any{objects.FieldKeyType: "string", "semantic_type": "statement"},
			exists:       true,
			obj:          map[string]any{objects.FieldKeyTitle: "Test Title"},
			wantErrors:   0,
			wantWarnings: 0,
			description:  ConstMagic530fb297,
		},
		{
			name:         ConstMagicbd047d05,
			fieldName:    "optional_field",
			fieldValue:   "value",
			fieldDef:     map[string]any{objects.FieldKeyType: "string"},
			exists:       true,
			obj:          map[string]any{"optional_field": "value"},
			wantErrors:   0,
			wantWarnings: 0,
			description:  ConstMagice40bad19,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors, warnings := validator.validateField(tt.fieldName, tt.fieldValue, tt.fieldDef, tt.exists, tt.obj, "test_kind")

			if len(errors) != tt.wantErrors {
				t.Errorf(ConstMagic79b256ff, len(errors), tt.wantErrors, tt.description, errors)
			}
			if len(warnings) != tt.wantWarnings {
				t.Errorf(ConstMagic17bf0d84, len(warnings), tt.wantWarnings, tt.description, warnings)
			}
		})
	}
}

// TestInstanceValidator_ValidateEnum tests the validateEnum function
func TestInstanceValidator_ValidateEnum(t *testing.T) {
	t.Parallel()
	validator := &InstanceValidator{}

	tests := []struct {
		name        string
		value       any
		enumValues  []any
		want        bool
		description string
	}{
		{
			name:        ConstMagice2570710,
			value:       "exploring",
			enumValues:  []any{"exploring", "planned", "in_progress"},
			want:        true,
			description: ConstMagic9def42d4,
		},
		{
			name:        ConstMagic86996bc7,
			value:       "EXPLORING",
			enumValues:  []any{"exploring", "planned", "in_progress"},
			want:        true,
			description: ConstMagicae81db2c,
		},
		{
			name:        "no match string",
			value:       "invalid",
			enumValues:  []any{"exploring", "planned", "in_progress"},
			want:        false,
			description: ConstMagic10f3573f,
		},
		{
			name:        "exact match int",
			value:       123,
			enumValues:  []any{123, 456, 789},
			want:        true,
			description: ConstMagicdbe97866,
		},
		{
			name:        "no match int",
			value:       999,
			enumValues:  []any{123, 456, 789},
			want:        false,
			description: ConstMagicb31d921e,
		},
		{
			name:        ConstMagice162af11,
			value:       "any",
			enumValues:  []any{},
			want:        false,
			description: ConstMagic02ae36a7,
		},
		{
			name:        "nil value",
			value:       nil,
			enumValues:  []any{"exploring", "planned"},
			want:        false,
			description: ConstMagicbca5e6d7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validator.validateEnum(tt.value, tt.enumValues)
			if got != tt.want {
				t.Errorf(ConstMagiccf263a2f, tt.value, tt.enumValues, got, tt.want, tt.description)
			}
		})
	}
}

// TestInstanceValidator_ValidateInstanceLegacy tests the validateInstanceLegacy function
// NOTE: Cannot use t.Parallel() - this test uses os.Setenv() which is incompatible with parallel execution
func TestInstanceValidator_ValidateInstanceLegacy(t *testing.T) {
	testRoot := registerZQKTestRootForTest(t)

	specsDir := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagice81c38f8, err)
	}

	// Create a simple test spec
	specFile := filepath.Join(specsDir, ConstMagic507c7ab6)
	specContent := `kind: test_object
fields:
  id:
    type: string
    validation:
      required: true
      pattern: ^TEST-\d{3,}$
  title:
    type: string
    validation:
      required: true
  status:
    type: string
    validation:
      enum:
        - exploring
        - planned
`
	if err := fileutil.WriteFile(specFile, []byte(specContent), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic31f618d5, err)
	}

	// Create lifecycle directory and file for test_object to avoid lifecycle validation errors
	lifecyclesDir := filepath.Join(testRoot, paths.ProcessInternalLifecyclesDir)
	if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagicc92d317d, err)
	}

	// Lifecycle loader expects filename: {kind}_lifecycle.yaml
	lifecycleFile := filepath.Join(lifecyclesDir, ConstMagica9fa0215)
	lifecycleContent := `object_type: test_object
statuses:
  - value: exploring
    display: Exploring
    initial: true
  - value: planned
    display: Planned
transitions:
  - from: exploring
    to: planned
`
	if err := fileutil.WriteFile(lifecycleFile, []byte(lifecycleContent), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagicaf7f45a0, err)
	}

	specLoader := objects.NewSpecLoader(specsDir)
	lifecycleLoader := objects.NewLifecycleLoader(lifecyclesDir)
	validator := NewInstanceValidatorWithLifecycle(specLoader, lifecycleLoader)

	// Ensure IDValidator patterns are loaded for ID validation
	// validateInstanceLegacy uses IDValidator for id field validation
	idValidator := GetIDValidator()
	if err := idValidator.LoadPatterns(); err != nil {
		t.Logf(ConstMagicd3d06733, err)
	}

	tests := []struct {
		name         string
		obj          map[string]any
		kind         string
		currentState string
		wantValid    bool
		wantErrors   int
		wantWarnings int
		description  string
	}{
		{
			name: "valid object",
			obj: map[string]any{
				objects.FieldKeyID:     "TEST-001",
				objects.FieldKeyTitle:  "Test Title",
				objects.FieldKeyStatus: "exploring",
			},
			kind:         "test_object",
			currentState: "",
			wantValid:    true,
			wantErrors:   0,
			wantWarnings: 0, // Lifecycle file exists now
			description:  ConstMagic5f134809,
		},
		{
			name: ConstMagicb26bea69,
			obj: map[string]any{
				objects.FieldKeyID: "TEST-001",
				// title missing
			},
			kind:         "test_object",
			currentState: "",
			wantValid:    false,
			wantErrors:   1,
			wantWarnings: 0,
			description:  ConstMagic5f95c751,
		},
		{
			name: "invalid pattern",
			obj: map[string]any{
				objects.FieldKeyID:     "INVALID-001",
				objects.FieldKeyTitle:  "Test Title",
				objects.FieldKeyStatus: "exploring",
			},
			kind:         "test_object",
			currentState: "",
			wantValid:    false,
			wantErrors:   1,
			wantWarnings: 0, // Lifecycle file exists now
			description:  ConstMagice0cd0b5d,
		},
		{
			name: ConstMagiceaa72e25,
			obj: map[string]any{
				objects.FieldKeyID:     "TEST-001",
				objects.FieldKeyTitle:  "Test Title",
				objects.FieldKeyStatus: "invalid_status",
			},
			kind:         "test_object",
			currentState: "",
			wantValid:    false,
			wantErrors:   2, // Both enum validation error and lifecycle validation error
			wantWarnings: 0, // Lifecycle file exists now
			description:  ConstMagic2e2f81a7,
		},
		{
			name: ConstMagic07df3c1a,
			obj: map[string]any{
				objects.FieldKeyID: "TEST-001",
			},
			kind:         "unknown_kind",
			currentState: "",
			wantValid:    false,
			wantErrors:   0, // Function returns error, not validation result
			wantWarnings: 0,
			description:  ConstMagicf7ebf2fb,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validator.validateInstanceLegacy(tt.obj, tt.kind, tt.currentState)

			if tt.name == ConstMagic07df3c1a {
				if err == nil {
					t.Error(ConstMagica3f25fa0)
				}
				return
			}

			if err != nil {
				t.Fatalf(ConstMagicd8f587ea, err, tt.description)
			}

			if result == nil {
				t.Fatalf(ConstMagicb099800a, tt.description)
			}

			if result.IsValid != tt.wantValid {
				t.Errorf(ConstMagiccd7ba621, result.IsValid, tt.wantValid, tt.description)
			}

			if len(result.Errors) != tt.wantErrors {
				t.Errorf(ConstMagic70f66ee0, len(result.Errors), tt.wantErrors, tt.description, result.Errors)
			}

			if len(result.Warnings) != tt.wantWarnings {
				t.Errorf(ConstMagic221a33de, len(result.Warnings), tt.wantWarnings, tt.description, result.Warnings)
			}
		})
	}
}

func TestInstanceValidator_validateType(t *testing.T) {
	t.Parallel()
	validator := &InstanceValidator{}

	tests := []struct {
		name  string
		value any
		typ   string
		want  bool
	}{
		// String type
		{"string valid", "test", "string", true},
		{"string invalid", 123, "string", false},
		{"string invalid nil", nil, "string", false},

		// Integer type
		{"int valid", 123, "int", true},
		{"int valid integer alias", 456, "integer", true},
		{"int from int64", int64(789), "int", true}, // int64 should be accepted as integer
		{"int from float", 123.0, "int", true},
		{"int from float64", float64(789), "int", true},
		{"int invalid", "123", "int", false},
		{"int invalid nil", nil, "int", false},
		// Note: JSON numbers are float64, so 123.5 is accepted as int (permissive)
		{"int from float64 with decimals", 123.5, "int", true},

		// Float/Number type
		{"float valid", 123.5, "float", true},
		{"number valid", 456.7, "number", true},
		{"float invalid", "123.5", "float", false},
		// Note: int is not valid for float (only float64 is accepted)
		{"float invalid int", 123, "float", false},

		// Boolean type
		{"bool valid true", true, "bool", true},
		{"bool valid false", false, "bool", true},
		{"boolean valid", true, "boolean", true},
		{"bool invalid", "true", "bool", false},
		{"bool invalid number", 1, "bool", false},

		// List/Array type
		{"list valid string slice", []string{"a", "b"}, "list", true},
		{"array valid int slice", []int{1, 2}, "array", true},
		{"list valid empty", []string{}, "list", true},
		{"list invalid", "not a list", "list", false},
		{"list invalid map", map[string]int{}, "list", false},

		// Object/Map type
		{"object valid", map[string]int{"a": 1}, "object", true},
		{"map valid", map[string]string{"key": "value"}, "map", true},
		{"object valid empty", map[string]any{}, "object", true},
		{"object invalid", []string{"a"}, "object", false},
		{"object invalid string", "not an object", "object", false},

		// Date type
		{"date valid", "2025-12-25", "date", true},
		{"date valid with leading zeros", "2025-01-01", "date", true},
		{"date invalid format", "12/25/2025", "date", false},
		{"date invalid type", 20251225, "date", false},
		{"date invalid string", "not a date", "date", false},
		{"date invalid short", "2025-12", "date", false},
		{"date invalid long", "2025-12-25-extra", "date", false},

		// Datetime type
		{"datetime valid", "2025-12-25T10:30:00Z", "datetime", true},
		{"datetime valid with milliseconds", "2025-12-25T10:30:00.123Z", "datetime", true}, // Pattern supports fractional seconds (milliseconds)
		{"datetime invalid", "2025-12-25", "datetime", false},
		{"datetime invalid format", "2025-12-25 10:30:00", "datetime", false},
		{"datetime invalid type", 20251225103000, "datetime", false},
		{"datetime_valid_with_milliseconds", "2025-12-25T10:30:00.123Z", "datetime", true}, // Fractional seconds are valid per ISO-8601

		// Text type
		{"text valid", "any string", "text", true},
		{"text valid empty", "", "text", true},
		{"text invalid", 123, "text", false},

		// Unknown type (default case - permissive)
		{"unknown type", "value", "unknown_type", true}, // Unknown types are permissive
		{"empty type", "value", "", true},               // Empty type is permissive
		{"enum type", "value", "enum", true},            // Enum type always returns true
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validator.validateType(tt.value, tt.typ)
			if got != tt.want {
				t.Errorf(ConstMagic71343441, tt.value, tt.typ, got, tt.want)
			}
		})
	}
}

func TestInstanceValidator_validateEnum(t *testing.T) {
	t.Parallel()
	validator := &InstanceValidator{}

	tests := []struct {
		name       string
		value      any
		enumValues []any
		want       bool
	}{
		{"valid enum", "exploring", []any{"exploring", "validated", "planned"}, true},
		{"invalid enum", "invalid", []any{"exploring", "validated", "planned"}, false},
		{"case insensitive", "Exploring", []any{"exploring", "validated", "planned"}, true},
		{"int enum", 1, []any{1, 2, 3}, true},
		{"int enum invalid", 4, []any{1, 2, 3}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validator.validateEnum(tt.value, tt.enumValues)
			if got != tt.want {
				t.Errorf(ConstMagicfb5f0fe9, got, tt.want)
			}
		})
	}
}

func TestInstanceValidator_validateSemanticType(t *testing.T) {
	t.Parallel()
	validator := &InstanceValidator{}

	tests := []struct {
		name         string
		fieldName    string
		value        any
		semanticType string
		wantErr      bool
	}{
		{"statement valid", "title", "Test", "statement", false},
		{"statement int valid", "sort_order", 123, "statement", false},
		{"statement invalid", "title", []string{"x"}, "statement", true},
		{"list valid", "components", []string{"a", "b"}, "list", false},
		{"list invalid", "components", nil, "list", true},
		{"reference valid", objects.FieldKeyGoalRefs, "GOAL-123", "reference", false},
		{"reference empty", objects.FieldKeyGoalRefs, "", "reference", true},
		{"reference invalid type", objects.FieldKeyGoalRefs, 123, "reference", true},
		{"comparison valid", "priority", "greater_than", "comparison", false},
		{"comparison invalid", "priority", 123, "comparison", true},
		{"expression valid", "computed", "any value", "expression", false},
		{"unknown semantic type", "field", "value", "unknown", false}, // Permissive
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateSemanticType(tt.fieldName, tt.value, tt.semanticType)
			if (err != nil) != tt.wantErr {
				t.Errorf(ConstMagic1b8faf07, err, tt.wantErr)
			}
		})
	}
}

func TestInstanceValidator_checkPrecondition(t *testing.T) {
	t.Parallel()
	iv := &InstanceValidator{}

	// is set
	objSet := map[string]any{
		"priority_plan_ref": "PLAN-1",
	}
	if !iv.checkPrecondition("priority_plan_ref is set", objSet) {
		t.Errorf("expected true for priority_plan_ref is set")
	}
	if iv.checkPrecondition("other_ref is set", objSet) {
		t.Errorf("expected false for other_ref is set")
	}

	// is not empty
	objList := map[string]any{
		"milestone_refs": []string{"MS-1"},
		"empty_list":     []string{},
	}
	if !iv.checkPrecondition("milestone_refs is not empty", objList) {
		t.Errorf("expected true for milestone_refs is not empty")
	}
	if iv.checkPrecondition("empty_list is not empty", objList) {
		t.Errorf("expected false for empty_list is not empty")
	}

	// at least one
	objAtLeast := map[string]any{
		"milestone_refs": []string{"MS-1"},
	}
	if !iv.checkPrecondition("at least one milestone_ref linked", objAtLeast) {
		t.Errorf("expected true for at least one milestone_ref linked")
	}
	if iv.checkPrecondition("at least one goal_ref linked", objAtLeast) {
		t.Errorf("expected false for at least one goal_ref linked")
	}

	// unknown precondition defaults to true (permissive)
	if !iv.checkPrecondition("unknown custom precondition rule", objAtLeast) {
		t.Errorf("expected true for unknown precondition")
	}
}

