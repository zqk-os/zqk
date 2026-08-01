package validation

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestGoValidator_ValidateDatatype tests the validateDatatype function
func TestGoValidator_ValidateDatatype(t *testing.T) {
	t.Parallel()
	validator := NewGoValidator()

	tests := []struct {
		name        string
		value       any
		datatype    string
		want        bool
		description string
	}{
		// String types
		{"string valid", "test", "string", true, "String value with string type"},
		{"xsd:string valid", "test", "xsd:string", true, "String value with xsd:string type"},
		{"string invalid", 123, "string", false, "Non-string value with string type"},

		// Integer types
		{"int valid", 123, "int", true, "Int value with int type"},
		{"integer valid", 456, "integer", true, "Int value with integer type"},
		{"xsd:integer valid", 789, "xsd:integer", true, "Int value with xsd:integer type"},
		{"int from float64", float64(123), "int", true, "Float64 value with int type (JSON numbers)"},
		{"int invalid", "123", "int", false, "String value with int type"},

		// Float/Number types
		{"float valid", 123.5, "float", true, "Float value with float type"},
		{"number valid", 456.7, "number", true, "Float value with number type"},
		{"xsd:double valid", 789.9, "xsd:double", true, "Float value with xsd:double type"},
		{"xsd:float valid", 12.34, "xsd:float", true, "Float value with xsd:float type"},
		{"float invalid", "123.5", "float", false, "String value with float type"},
		{"float invalid int", 123, "float", false, "Int value with float type"},

		// Boolean types
		{"bool valid true", true, "bool", true, "Bool true with bool type"},
		{"bool valid false", false, "bool", true, "Bool false with bool type"},
		{"boolean valid", true, "boolean", true, "Bool with boolean type"},
		{"xsd:boolean valid", false, "xsd:boolean", true, "Bool with xsd:boolean type"},
		{"bool invalid", "true", "bool", false, "String value with bool type"},

		// List/Array types
		{"list valid string slice", []string{"a", "b"}, "list", true, "String slice with list type"},
		{"array valid int slice", []int{1, 2}, "array", true, "Int slice with array type"},
		{"list invalid", "not a list", "list", false, "String value with list type"},

		// Object/Map types
		{"object valid", map[string]int{"a": 1}, "object", true, "Map with object type"},
		{"map valid", map[string]string{"key": "value"}, "map", true, "Map with map type"},
		{"object invalid", []string{"a"}, "object", false, "Slice value with object type"},

		// Date types
		{"date valid", "2025-12-25", "date", true, "Valid date string"},
		{"xsd:date valid", "2025-01-01", "xsd:date", true, "Valid date with xsd:date type"},
		{"date invalid format", "12/25/2025", "date", false, "Invalid date format"},
		{"date invalid type", 20251225, "date", false, "Non-string value with date type"},

		// Datetime types
		{"datetime valid", "2025-12-25T10:30:00Z", "datetime", true, "Valid datetime string"},
		{"xsd:dateTime valid", "2025-12-25T10:30:00Z", "xsd:dateTime", true, "Valid datetime with xsd:dateTime type"},
		{"datetime invalid", "2025-12-25", "datetime", false, "Date string with datetime type"},
		{"datetime invalid format", "2025-12-25 10:30:00", "datetime", false, "Invalid datetime format"},
		// Note: time.Time objects are tested in integration tests, not unit tests

		// Unknown type (default case - permissive)
		{"unknown type", "value", "unknown_type", true, "Unknown type is permissive"},
		{"empty type", "value", "", true, "Empty type is permissive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validator.validateDatatype(tt.value, tt.datatype)
			if got != tt.want {
				t.Errorf(ConstMagic9c13a97d, tt.value, tt.datatype, got, tt.want, tt.description)
			}
		})
	}
}

// TestGoValidator_CheckPrecondition tests the checkPrecondition function
func TestGoValidator_CheckPrecondition(t *testing.T) {
	t.Parallel()
	validator := NewGoValidator()

	tests := []struct {
		name         string
		precondition string
		obj          map[string]any
		want         bool
		description  string
	}{
		// "is set" preconditions
		{
			name:         ConstMagicf796b665,
			precondition: ConstMagic866d629a,
			obj:          map[string]any{objects.FieldKeyPriorityPlanRef: "PLAN-001"},
			want:         true,
			description:  ConstMagic7223ad50,
		},
		{
			name:         ConstMagicd138f02a,
			precondition: ConstMagic866d629a,
			obj:          map[string]any{objects.FieldKeyPriorityPlanRef: ""},
			want:         false,
			description:  ConstMagicf66f79a4,
		},
		{
			name:         ConstMagiccd60dc9f,
			precondition: ConstMagic866d629a,
			obj:          map[string]any{objects.FieldKeyPriorityPlanRef: nil},
			want:         false,
			description:  ConstMagic2588f4b2,
		},
		{
			name:         ConstMagic9959a6e4,
			precondition: ConstMagic866d629a,
			obj:          map[string]any{"other_field": "value"},
			want:         false,
			description:  ConstMagic0d9b5647,
		},
		{
			name:         ConstMagic5b0448c0,
			precondition: ConstMagic6263188f,
			obj:          map[string]any{objects.FieldKeyPriorityPlanRef: "PLAN-001"},
			want:         true,
			description:  ConstMagic12fbcc47,
		},

		// "is not empty" preconditions
		{
			name:         ConstMagic88645645,
			precondition: ConstMagic9ba826a5,
			obj:          map[string]any{objects.FieldKeyMilestoneRefs: []string{"MIL-001", "MIL-002"}},
			want:         true,
			description:  ConstMagicdf6641ad,
		},
		{
			name:         ConstMagicc37d5e0d,
			precondition: ConstMagic9ba826a5,
			obj:          map[string]any{objects.FieldKeyMilestoneRefs: []string{}},
			want:         false,
			description:  ConstMagic20f843b8,
		},
		{
			name:         ConstMagic7204b8e5,
			precondition: ConstMagic73187280,
			obj:          map[string]any{objects.FieldKeyTitle: "Test Title"},
			want:         true,
			description:  ConstMagic1a3a9603,
		},
		{
			name:         ConstMagicc3413f5c,
			precondition: ConstMagic73187280,
			obj:          map[string]any{objects.FieldKeyTitle: ""},
			want:         false,
			description:  ConstMagic680da17d,
		},
		{
			name:         ConstMagic0dbd33b5,
			precondition: ConstMagic9ba826a5,
			obj:          map[string]any{"other_field": "value"},
			want:         false,
			description:  ConstMagic0d9b5647,
		},
		{
			name:         ConstMagic53eed2a0,
			precondition: ConstMagic9ba826a5,
			obj:          map[string]any{objects.FieldKeyMilestoneRefs: nil},
			want:         false,
			description:  "Field is nil",
		},

		// "at least" preconditions
		{
			name:         ConstMagic1f7b5cac,
			precondition: ConstMagic35012048,
			obj:          map[string]any{objects.FieldKeyMilestoneRefs: []string{"MIL-001"}},
			want:         true,
			description:  ConstMagic2bea6ec7,
		},
		{
			name:         ConstMagic17a41d91,
			precondition: ConstMagic35012048,
			obj:          map[string]any{objects.FieldKeyMilestoneRefs: []string{}},
			want:         false,
			description:  "List is empty",
		},
		{
			name:         ConstMagic6192b304,
			precondition: ConstMagic35012048,
			obj:          map[string]any{"other_field": "value"},
			want:         false,
			description:  ConstMagic0d9b5647,
		},

		// Unknown precondition format
		{
			name:         "unknown format",
			precondition: ConstMagic8310cf6c,
			obj:          map[string]any{objects.FieldKeyField: "value"},
			want:         true, // Unknown format is permissive
			description:  ConstMagic782bf364,
		},
		{
			name:         ConstMagiceb9522ab,
			precondition: "",
			obj:          map[string]any{objects.FieldKeyField: "value"},
			want:         true, // Empty is permissive
			description:  ConstMagic86d6dd8d,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validator.checkPrecondition(tt.precondition, tt.obj, nil)
			if got != tt.want {
				t.Errorf(ConstMagic84998f2f, tt.precondition, tt.obj, got, tt.want, tt.description)
			}
		})
	}
}

// TestGoValidator_ValidateSemanticType tests the validateSemanticType function
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
			value:        "PLAN-001",
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
			value:        "PLAN-001",
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
			value:       "ITEM-001",
			pattern:     `^ITEM-\d{3,}$`,
			want:        true,
			wantErr:     false,
			description: ConstMagicb923e28e,
		},
		{
			name:        ConstMagic09a29b67,
			value:       "INVALID-001",
			pattern:     `^ITEM-\d{3,}$`,
			want:        false,
			wantErr:     false,
			description: ConstMagic92aefd62,
		},
		{
			name:        ConstMagicfe145f27,
			value:       "",
			pattern:     `^ITEM-\d{3,}$`,
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
				if err := os.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
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
				if err := os.WriteFile(lifecycleFile, []byte(lifecycleContent), paths.FilePerm644); err != nil {
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
				if err := os.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
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
				if err := os.WriteFile(lifecycleFile, []byte(lifecycleContent), paths.FilePerm644); err != nil {
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
				if err := os.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
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
				if err := os.WriteFile(lifecycleFile, []byte(lifecycleContent), paths.FilePerm644); err != nil {
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
				if err := os.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
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
				if err := os.WriteFile(lifecycleFile, []byte(lifecycleContent), paths.FilePerm644); err != nil {
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
				if err := os.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
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
				if err := os.WriteFile(lifecycleFile, []byte(lifecycleContent), paths.FilePerm644); err != nil {
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
			obj:          map[string]any{objects.FieldKeyPriorityPlanRef: "PLAN-001"},
			setupLoader: func(t *testing.T) *objects.LifecycleLoader {
				tmpDir := t.TempDir()
				lifecyclesDir := filepath.Join(tmpDir, paths.ProcessInternalLifecyclesDir)
				if err := os.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
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
				if err := os.WriteFile(lifecycleFile, []byte(lifecycleContent), paths.FilePerm644); err != nil {
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
				if err := os.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
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
