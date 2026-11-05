package macro

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/dna"
)

func TestSchemaMacroExpander_ExpandMetaSchema_ValidationErrors(t *testing.T) {
	t.Parallel()

	expander := NewSchemaMacroExpander()
	if expander == nil {
		t.Fatal("expected non-nil SchemaMacroExpander")
	}

	// Nil schema
	_, err := expander.ExpandMetaSchema("mypkg", nil)
	if err == nil || !strings.Contains(err.Error(), "schema cannot be nil") {
		t.Errorf("expected nil schema error, got: %v", err)
	}

	// Empty TargetKind
	_, err = expander.ExpandMetaSchema("mypkg", &dna.MetaSchema{TargetKind: ""})
	if err == nil || !strings.Contains(err.Error(), "schema target kind is required") {
		t.Errorf("expected target kind error, got: %v", err)
	}

	// Whitespace TargetKind
	_, err = expander.ExpandMetaSchema("mypkg", &dna.MetaSchema{TargetKind: "   "})
	if err == nil || !strings.Contains(err.Error(), "schema target kind is required") {
		t.Errorf("expected target kind error, got: %v", err)
	}
}

func TestSchemaMacroExpander_ExpandMetaSchema_FieldTypeMappings(t *testing.T) {
	t.Parallel()

	expander := NewSchemaMacroExpander()

	schema := &dna.MetaSchema{
		TargetKind: "system_event",
		Fields: map[string]dna.FieldMetaSpec{
			"count":        {Name: "count", Type: dna.TypeInt, Required: true},
			"enabled":      {Name: "enabled", Type: dna.TypeBool, Required: false},
			"tags":         {Name: "tags", Type: dna.TypeList, Required: false},
			"metadata":     {Name: "metadata", Type: dna.TypeMap, Required: false},
			"resource_urn": {Name: "resource_urn", Type: dna.TypeURN, Required: true},
			"description":  {Name: "description", Type: dna.TypeString, Required: false},
			"fallback":     {Name: "fallback", Type: "unknown_custom_type", Required: false},
		},
	}

	spec, err := expander.ExpandMetaSchema("eventpkg", schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spec.PackageName != "eventpkg" {
		t.Errorf("expected package name eventpkg, got %s", spec.PackageName)
	}
	if spec.EntityName != "SystemEvent" {
		t.Errorf("expected entity name SystemEvent, got %s", spec.EntityName)
	}
	if spec.Kind != "system_event" {
		t.Errorf("expected kind system_event, got %s", spec.Kind)
	}
	if !spec.IncludeAudit || !spec.IncludeLifecycle {
		t.Errorf("expected IncludeAudit and IncludeLifecycle to be true")
	}

	fieldTypes := make(map[string]string)
	for _, f := range spec.Fields {
		fieldTypes[f.JSONTag] = f.Type
	}

	expectedTypes := map[string]string{
		"count":        "int64",
		"enabled":      "bool",
		"tags":         "[]string",
		"metadata":     "map[string]any",
		"resource_urn": "dna.URN",
		"description":  "string",
		"fallback":     "string",
	}

	for fieldName, expectedType := range expectedTypes {
		actualType, ok := fieldTypes[fieldName]
		if !ok {
			t.Errorf("missing field %s in expanded spec", fieldName)
		} else if actualType != expectedType {
			t.Errorf("field %s: expected type %s, got %s", fieldName, expectedType, actualType)
		}
	}
}

func TestToPascalCase(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"simple", "Simple"},
		{"already_snake_case", "AlreadySnakeCase"},
		{"kebab-case-string", "KebabCaseString"},
		{"mixed_delimiters-and spaces", "MixedDelimitersAndSpaces"},
		{"multiple___underscores", "MultipleUnderscores"},
		{"Single", "Single"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			actual := toPascalCase(tc.input)
			if actual != tc.expected {
				t.Errorf("toPascalCase(%q) = %q, want %q", tc.input, actual, tc.expected)
			}
		})
	}
}
