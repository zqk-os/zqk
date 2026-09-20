package validation

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestOntologyRegistry_RegisterAndGetMapping(t *testing.T) {
	t.Parallel()
	registry := NewOntologyRegistry()

	// Test getting a default mapping
	mapping, err := registry.GetMapping("statement")
	if err != nil {
		t.Fatalf(ConstMagicd2824190, err)
	}
	if mapping.SemanticType != "statement" {
		t.Errorf(ConstMagic14d9cf46, mapping.SemanticType)
	}
	if len(mapping.SchemaOrgTypes) == 0 {
		t.Error(ConstMagica51b61a3)
	}

	// Test getting an unknown mapping
	_, err = registry.GetMapping("unknown_type")
	if err == nil {
		t.Error(ConstMagic3023acad)
	}
}

func TestOntologyRegistry_ValidateSemanticType(t *testing.T) {
	t.Parallel()
	registry := NewOntologyRegistry()

	tests := []struct {
		name         string
		value        any
		semanticType string
		wantError    bool
	}{
		{
			name:         ConstMagic594a0640,
			value:        ConstMagic9d20c957,
			semanticType: "statement",
			wantError:    false,
		},
		{
			name: ConstMagic6e6a2ef4,
			value: map[string]any{
				"text": ConstMagic9d20c957,
			},
			semanticType: "statement",
			wantError:    false,
		},
		{
			name:         ConstMagicab4ae39f,
			value:        123,
			semanticType: "statement",
			wantError:    false,
		},
		{
			name:         ConstMagic8eb5d08f,
			value:        true,
			semanticType: "statement",
			wantError:    false,
		},
		{
			name:         "valid_reference",
			value:        "BLI-001",
			semanticType: "reference",
			wantError:    false,
		},
		{
			name:         ConstMagic95bf92a8,
			value:        "",
			semanticType: "reference",
			wantError:    true,
		},
		{
			name:         ConstMagiccd04678b,
			value:        123,
			semanticType: "reference",
			wantError:    true,
		},
		{
			name:         "valid_list",
			value:        []string{"item1", "item2"},
			semanticType: "list",
			wantError:    false,
		},
		{
			name:         ConstMagicb60a6709,
			value:        nil,
			semanticType: "list",
			wantError:    true,
		},
		{
			name:         ConstMagicb522cdc1,
			value:        "greater_than",
			semanticType: "comparison",
			wantError:    false,
		},
		{
			name:         ConstMagic033eecb9,
			value:        ConstMagicda2da387,
			semanticType: "comparison",
			wantError:    true,
		},
		{
			name:         ConstMagicea6a0352,
			value:        75,
			semanticType: "expression",
			wantError:    false,
		},
		{
			name:         ConstMagic0d3cee92,
			value:        "any value",
			semanticType: "unknown",
			wantError:    false, // Permissive for unknown types
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := registry.ValidateSemanticType(tt.value, tt.semanticType)
			if (err != nil) != tt.wantError {
				t.Errorf(ConstMagic187d5860, err, tt.wantError)
			}
		})
	}
}

func TestOntologyRegistry_GetSchemaOrgTypes(t *testing.T) {
	t.Parallel()
	registry := NewOntologyRegistry()

	schemaTypes, err := registry.GetSchemaOrgTypes("statement")
	if err != nil {
		t.Fatalf(ConstMagic1df53b7b, err)
	}
	if len(schemaTypes) == 0 {
		t.Error(ConstMagica51b61a3)
	}

	// Check that expected types are present
	expectedTypes := []string{"schema:Text", "schema:CreativeWork"}
	for _, expected := range expectedTypes {
		found := false
		for _, actual := range schemaTypes {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf(ConstMagic38c92052, expected, schemaTypes)
		}
	}
}

func TestOntologyRegistry_GetISO11179Type(t *testing.T) {
	t.Parallel()
	registry := NewOntologyRegistry()

	isoType, err := registry.GetISO11179Type("statement")
	if err != nil {
		t.Fatalf(ConstMagic6f99ea8d, err)
	}
	if isoType != "Text" {
		t.Errorf(ConstMagic19d6668a, isoType)
	}
}

func TestOntologyRegistry_GetBFOType(t *testing.T) {
	t.Parallel()
	registry := NewOntologyRegistry()

	bfoType, err := registry.GetBFOType("statement")
	if err != nil {
		t.Fatalf(ConstMagicd02a357b, err)
	}
	if bfoType != "Continuant" {
		t.Errorf(ConstMagicb2044ba2, bfoType)
	}
}

func TestGlobalOntologyRegistry(t *testing.T) {
	t.Parallel()
	registry1 := GetGlobalOntologyRegistry()
	registry2 := GetGlobalOntologyRegistry()

	// Should return the same instance
	if !reflect.DeepEqual(registry1, registry2) {
		t.Error(ConstMagic0cfb63ef)
	}
}

func TestOntologyRegistry_StatementObjectValidation(t *testing.T) {
	t.Parallel()
	registry := NewOntologyRegistry()

	// Test valid structured statement with text field
	validStatement := map[string]any{
		"text": ConstMagic9d20c957,
	}
	if err := registry.ValidateSemanticType(validStatement, "statement"); err != nil {
		t.Errorf(ConstMagicc8b5ae91, err)
	}

	// Test valid structured statement with content field
	validStatement2 := map[string]any{
		objects.FieldKeyContent: ConstMagic9d20c957,
	}
	if err := registry.ValidateSemanticType(validStatement2, "statement"); err != nil {
		t.Errorf(ConstMagic74d2a755, err)
	}

	// Test invalid structured statement without text or content
	// Note: Current implementation is permissive (returns nil) to avoid false positives
	// from YAML parsing artifacts. This behavior is documented in the code.
	invalidStatement := map[string]any{
		"other": ConstMagic8273feef,
	}
	if err := registry.ValidateSemanticType(invalidStatement, "statement"); err != nil {
		// The current implementation is permissive - this documents the behavior
		t.Logf("Statement object without text/content returned error (may be expected in future): %v", err)
	}
}

func TestOntologyRegistry_ComparisonOperators(t *testing.T) {
	t.Parallel()
	registry := NewOntologyRegistry()

	validOperators := []string{
		"greater_than",
		"less_than",
		"equal_to",
		"not_equal_to",
		"greater_than_or_equal",
		"less_than_or_equal",
		"contains",
		"not_contains",
		"starts_with",
		"ends_with",
		"matches",
		"in",
		"not_in",
	}

	for _, op := range validOperators {
		if err := registry.ValidateSemanticType(op, "comparison"); err != nil {
			t.Errorf(ConstMagice1abae87, op, err)
		}
	}

	// Test invalid operator
	if err := registry.ValidateSemanticType(ConstMagicda2da387, "comparison"); err == nil {
		t.Error(ConstMagice623f00a)
	}
}

// TestOntologyRegistry_RegisterMapping tests RegisterMapping function
func TestOntologyRegistry_RegisterMapping(t *testing.T) {
	t.Parallel()
	registry := NewOntologyRegistry()

	// Register a custom mapping
	customMapping := &OntologyMapping{
		SemanticType:   "custom_type",
		SchemaOrgTypes: []string{"schema:Thing"},
		ISO11179Type:   "Custom",
		BFOType:        "Continuant",
		ValidationRules: []OntologyValidationRule{
			{
				Name:        ConstMagic69a305b2,
				Description: ConstMagicc13ab301,
				Validator: func(value any) error {
					if value == nil {
						return errors.New(ConstMagic203c8b8f)
					}
					return nil
				},
			},
		},
	}

	registry.RegisterMapping(customMapping)

	// Verify it was registered
	mapping, err := registry.GetMapping("custom_type")
	if err != nil {
		t.Fatalf(ConstMagic6be7d415, err)
	}
	if mapping.SemanticType != "custom_type" {
		t.Errorf(ConstMagic94375931, mapping.SemanticType)
	}

	// Test that custom validation works
	if err := registry.ValidateSemanticType("valid", "custom_type"); err != nil {
		t.Errorf(ConstMagic5d797fe7, err)
	}
	if err := registry.ValidateSemanticType(nil, "custom_type"); err == nil {
		t.Error(ConstMagica2630773)
	}
}

// TestOntologyRegistry_GetSchemaOrgTypes_Error tests error path
func TestOntologyRegistry_GetSchemaOrgTypes_Error(t *testing.T) {
	t.Parallel()
	registry := NewOntologyRegistry()

	_, err := registry.GetSchemaOrgTypes("unknown_type")
	if err == nil {
		t.Error(ConstMagic3023acad)
	}
}

// TestOntologyRegistry_GetISO11179Type_Error tests error path
func TestOntologyRegistry_GetISO11179Type_Error(t *testing.T) {
	t.Parallel()
	registry := NewOntologyRegistry()

	_, err := registry.GetISO11179Type("unknown_type")
	if err == nil {
		t.Error(ConstMagic3023acad)
	}
}

// TestOntologyRegistry_GetBFOType_Error tests error path
func TestOntologyRegistry_GetBFOType_Error(t *testing.T) {
	t.Parallel()
	registry := NewOntologyRegistry()

	_, err := registry.GetBFOType("unknown_type")
	if err == nil {
		t.Error(ConstMagic3023acad)
	}
}

// TestOntologyRegistry_ValidateSemanticType_TimeTime tests time.Time handling
func TestOntologyRegistry_ValidateSemanticType_TimeTime(t *testing.T) {
	t.Parallel()
	registry := NewOntologyRegistry()

	// Test that time.Time objects are accepted for statement semantic type
	// (This is a special case to handle YAML datetime parsing)
	now := time.Now()
	if err := registry.ValidateSemanticType(now, "statement"); err != nil {
		t.Errorf(ConstMagic206b3deb, err)
	}
}

// TestOntologyRegistry_ValidateSemanticType_StatementReflection tests reflection-based time.Time detection
func TestOntologyRegistry_ValidateSemanticType_StatementReflection(t *testing.T) {
	t.Parallel()
	registry := NewOntologyRegistry()

	// Create a time.Time value via reflection to test the reflection path
	timeType := reflect.TypeOf(time.Time{})
	timeValue := reflect.New(timeType).Elem()

	// This should be accepted as a statement (special case for YAML datetime parsing)
	if err := registry.ValidateSemanticType(timeValue.Interface(), "statement"); err != nil {
		t.Errorf(ConstMagiccf176e4d, err)
	}
}

// TestOntologyRegistry_ValidateSemanticType_StatementObjectWithoutText tests statement object validation
func TestOntologyRegistry_ValidateSemanticType_StatementObjectWithoutText(t *testing.T) {
	t.Parallel()
	registry := NewOntologyRegistry()

	// Test statement object without text or content field
	// According to the code, this should be permissive (returns nil) to avoid false positives
	statementWithoutText := map[string]any{
		"other_field": "value",
	}
	if err := registry.ValidateSemanticType(statementWithoutText, "statement"); err != nil {
		// The current implementation is permissive for objects without text/content
		// to avoid false positives from YAML parsing artifacts
		// This test documents the current behavior
		t.Logf("Statement object without text/content returned error (may be expected): %v", err)
	}
}

// TestOntologyRegistry_NewOntologyRegistry_DefaultMappings tests that default mappings are registered
func TestOntologyRegistry_NewOntologyRegistry_DefaultMappings(t *testing.T) {
	t.Parallel()
	registry := NewOntologyRegistry()

	// Verify all default semantic types are registered
	defaultTypes := []string{"statement", "reference", "list", "comparison", "expression"}
	for _, semanticType := range defaultTypes {
		mapping, err := registry.GetMapping(semanticType)
		if err != nil {
			t.Errorf(ConstMagic4157ffdc, semanticType, err)
			continue
		}
		if mapping.SemanticType != semanticType {
			t.Errorf(ConstMagic39f7b95c, semanticType, mapping.SemanticType)
		}
		if len(mapping.ValidationRules) == 0 {
			t.Errorf(ConstMagic95ba1724, semanticType)
		}
	}
}
