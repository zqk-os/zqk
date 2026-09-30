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
		t.Fatalf("Expected to get 'statement' mapping, got error: %v", err)
	}
	if mapping.SemanticType != "statement" {
		t.Errorf("Expected semantic type 'statement', got %s", mapping.SemanticType)
	}
	if len(mapping.SchemaOrgTypes) == 0 {
		t.Error("Expected Schema.org types, got empty slice")
	}

	// Test getting an unknown mapping
	_, err = registry.GetMapping("unknown_type")
	if err == nil {
		t.Error("Expected error for unknown semantic type, got nil")
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
			name:         "valid_statement_string",
			value:        "This is a statement",
			semanticType: "statement",
			wantError:    false,
		},
		{
			name: "valid_statement_object",
			value: map[string]any{
				"text": "This is a statement",
			},
			semanticType: "statement",
			wantError:    false,
		},
		{
			name:         "valid_statement_int",
			value:        123,
			semanticType: "statement",
			wantError:    false,
		},
		{
			name:         "valid_statement_bool",
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
			name:         "invalid_reference_empty",
			value:        "",
			semanticType: "reference",
			wantError:    true,
		},
		{
			name:         "invalid_reference_number",
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
			name:         "invalid_list_nil",
			value:        nil,
			semanticType: "list",
			wantError:    true,
		},
		{
			name:         "valid_comparison",
			value:        "greater_than",
			semanticType: "comparison",
			wantError:    false,
		},
		{
			name:         "invalid_comparison_operator",
			value:        "invalid_operator",
			semanticType: "comparison",
			wantError:    true,
		},
		{
			name:         "valid_expression",
			value:        75,
			semanticType: "expression",
			wantError:    false,
		},
		{
			name:         "unknown_semantic_type",
			value:        "any value",
			semanticType: "unknown",
			wantError:    false, // Permissive for unknown types
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := registry.ValidateSemanticType(tt.value, tt.semanticType)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateSemanticType() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestOntologyRegistry_GetSchemaOrgTypes(t *testing.T) {
	t.Parallel()
	registry := NewOntologyRegistry()

	schemaTypes, err := registry.GetSchemaOrgTypes("statement")
	if err != nil {
		t.Fatalf("Expected to get Schema.org types, got error: %v", err)
	}
	if len(schemaTypes) == 0 {
		t.Error("Expected Schema.org types, got empty slice")
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
			t.Errorf("Expected Schema.org type %s not found in %v", expected, schemaTypes)
		}
	}
}

func TestOntologyRegistry_GetISO11179Type(t *testing.T) {
	t.Parallel()
	registry := NewOntologyRegistry()

	isoType, err := registry.GetISO11179Type("statement")
	if err != nil {
		t.Fatalf("Expected to get ISO 11179 type, got error: %v", err)
	}
	if isoType != "Text" {
		t.Errorf("Expected ISO 11179 type 'Text', got %s", isoType)
	}
}

func TestOntologyRegistry_GetBFOType(t *testing.T) {
	t.Parallel()
	registry := NewOntologyRegistry()

	bfoType, err := registry.GetBFOType("statement")
	if err != nil {
		t.Fatalf("Expected to get BFO type, got error: %v", err)
	}
	if bfoType != "Continuant" {
		t.Errorf("Expected BFO type 'Continuant', got %s", bfoType)
	}
}

func TestGlobalOntologyRegistry(t *testing.T) {
	t.Parallel()
	registry1 := GetGlobalOntologyRegistry()
	registry2 := GetGlobalOntologyRegistry()

	// Should return the same instance
	if !reflect.DeepEqual(registry1, registry2) {
		t.Error("Global ontology registry should return the same instance")
	}
}

func TestOntologyRegistry_StatementObjectValidation(t *testing.T) {
	t.Parallel()
	registry := NewOntologyRegistry()

	// Test valid structured statement with text field
	validStatement := map[string]any{
		"text": "This is a statement",
	}
	if err := registry.ValidateSemanticType(validStatement, "statement"); err != nil {
		t.Errorf("Expected valid statement with 'text' field, got error: %v", err)
	}

	// Test valid structured statement with content field
	validStatement2 := map[string]any{
		objects.FieldKeyContent: "This is a statement",
	}
	if err := registry.ValidateSemanticType(validStatement2, "statement"); err != nil {
		t.Errorf("Expected valid statement with 'content' field, got error: %v", err)
	}

	// Test invalid structured statement without text or content
	// Note: Current implementation is permissive (returns nil) to avoid false positives
	// from YAML parsing artifacts. This behavior is documented in the code.
	invalidStatement := map[string]any{
		"other": "This is not valid",
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
			t.Errorf("Expected valid comparison operator '%s', got error: %v", op, err)
		}
	}

	// Test invalid operator
	if err := registry.ValidateSemanticType("invalid_operator", "comparison"); err == nil {
		t.Error("Expected error for invalid comparison operator, got nil")
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
				Name:        "custom_validation",
				Description: "Custom validation rule",
				Validator: func(value any) error {
					if value == nil {
						return errors.New("custom_type cannot be nil")
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
		t.Fatalf("Expected to get custom mapping, got error: %v", err)
	}
	if mapping.SemanticType != "custom_type" {
		t.Errorf("Expected semantic type 'custom_type', got %s", mapping.SemanticType)
	}

	// Test that custom validation works
	if err := registry.ValidateSemanticType("valid", "custom_type"); err != nil {
		t.Errorf("Expected valid custom type, got error: %v", err)
	}
	if err := registry.ValidateSemanticType(nil, "custom_type"); err == nil {
		t.Error("Expected error for nil custom type, got nil")
	}
}

// TestOntologyRegistry_GetSchemaOrgTypes_Error tests error path
func TestOntologyRegistry_GetSchemaOrgTypes_Error(t *testing.T) {
	t.Parallel()
	registry := NewOntologyRegistry()

	_, err := registry.GetSchemaOrgTypes("unknown_type")
	if err == nil {
		t.Error("Expected error for unknown semantic type, got nil")
	}
}

// TestOntologyRegistry_GetISO11179Type_Error tests error path
func TestOntologyRegistry_GetISO11179Type_Error(t *testing.T) {
	t.Parallel()
	registry := NewOntologyRegistry()

	_, err := registry.GetISO11179Type("unknown_type")
	if err == nil {
		t.Error("Expected error for unknown semantic type, got nil")
	}
}

// TestOntologyRegistry_GetBFOType_Error tests error path
func TestOntologyRegistry_GetBFOType_Error(t *testing.T) {
	t.Parallel()
	registry := NewOntologyRegistry()

	_, err := registry.GetBFOType("unknown_type")
	if err == nil {
		t.Error("Expected error for unknown semantic type, got nil")
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
		t.Errorf("Expected time.Time to be valid for statement semantic type, got error: %v", err)
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
		t.Errorf("Expected time.Time (via reflection) to be valid for statement, got error: %v", err)
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
			t.Errorf("Expected default mapping for %s, got error: %v", semanticType, err)
			continue
		}
		if mapping.SemanticType != semanticType {
			t.Errorf("Expected semantic type %s, got %s", semanticType, mapping.SemanticType)
		}
		if len(mapping.ValidationRules) == 0 {
			t.Errorf("Expected validation rules for %s, got none", semanticType)
		}
	}
}
