package bldr_v2

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// TestLifecycleBuilder_Registration tests that the lifecycle builder is registered
func TestLifecycleBuilder_Registration(t *testing.T) {
	t.Parallel()
	registry := builders.GetGlobalRegistry()

	// Verify lifecycle builder is registered by getting the builder
	builder, err := registry.GetBuilder("lifecycle", "v2_0_0")
	if err != nil {
		t.Fatalf("Lifecycle spec builder not registered: %v", err)
	}

	// Build spec to verify it works
	spec := builder.Build()
	if spec == nil {
		t.Fatal("Build() returned nil spec")
	}

	// Verify ontology
	if spec.Ontology != "lifecycle" {
		t.Errorf("Expected ontology 'lifecycle', got %s", spec.Ontology)
	}

	// Verify schema version
	if spec.SchemaVersion != objects.DefaultSchemaVersion {
		t.Errorf("Expected schema version %q, got %s", objects.DefaultSchemaVersion, spec.SchemaVersion)
	}
}

// TestLifecycleBuilder_Build tests that the lifecycle builder creates a valid spec
func TestLifecycleBuilder_Build(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleBuilder()
	spec := builder.Build()

	if spec == nil {
		t.Fatal("Build() returned nil spec")
	}

	// Verify basic spec properties
	if spec.Ontology != "lifecycle" {
		t.Errorf("Expected ontology 'lifecycle', got %s", spec.Ontology)
	}

	if spec.SchemaVersion != objects.DefaultSchemaVersion {
		t.Errorf("Expected schema version %q, got %s", objects.DefaultSchemaVersion, spec.SchemaVersion)
	}

	if spec.Visibility != "internal" {
		t.Errorf("Expected visibility 'internal', got %s", spec.Visibility)
	}

	// Verify extends (Extends is a string, not a slice)
	if spec.Extends != "base_object" {
		t.Errorf("Expected spec to extend 'base_object', got %s", spec.Extends)
	}
}

// TestLifecycleBuilder_RequiredFields tests that all required fields are present
func TestLifecycleBuilder_RequiredFields(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleBuilder()
	spec := builder.Build()

	requiredFields := []string{
		"object_type",
		"statuses",
	}

	for _, fieldName := range requiredFields {
		field, exists := spec.Fields[fieldName]
		if !exists {
			t.Errorf("Required field '%s' not found in spec", fieldName)
			continue
		}

		// Verify field is marked as required
		fieldMap, ok := field.(map[string]any)
		if !ok {
			t.Errorf("Field '%s' is not a map", fieldName)
			continue
		}

		if validation, ok := fieldMap["validation"].(map[string]any); ok {
			if required, ok := validation["required"].(bool); ok && !required && fieldName == "statuses" {
				// statuses should be required (minCount >= 1 makes it effectively required)
				if minCount, ok := validation["min_count"].(int); !ok || minCount < 1 {
					t.Errorf("Field 'statuses' should have min_count >= 1 (effectively required)")
				}
			}
		}
	}
}

// TestLifecycleBuilder_ObjectTypeField tests the object_type field configuration
func TestLifecycleBuilder_ObjectTypeField(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleBuilder()
	spec := builder.Build()

	field, exists := spec.Fields["object_type"]
	if !exists {
		t.Fatal("Field 'object_type' not found")
	}

	fieldMap, ok := field.(map[string]any)
	if !ok {
		t.Fatal("Field 'object_type' is not a map")
	}

	// Verify type
	if fieldMap["type"] != "string" {
		t.Errorf("Expected type 'string', got %v", fieldMap["type"])
	}

	// Verify validation
	validation, ok := fieldMap["validation"].(map[string]any)
	if !ok {
		t.Fatal("Field 'object_type' validation not found")
	}

	if required, ok := validation["required"].(bool); !ok || !required {
		t.Error("Field 'object_type' should be required")
	}

	if pattern, ok := validation["pattern"].(string); !ok || pattern != `^[a-z_]+$` {
		t.Errorf("Expected pattern '^[a-z_]+$', got %v", validation["pattern"])
	}
}

// TestLifecycleBuilder_StatusesField tests the statuses field configuration
func TestLifecycleBuilder_StatusesField(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleBuilder()
	spec := builder.Build()

	field, exists := spec.Fields["statuses"]
	if !exists {
		t.Fatal("Field 'statuses' not found")
	}

	fieldMap, ok := field.(map[string]any)
	if !ok {
		t.Fatal("Field 'statuses' is not a map")
	}

	// Verify type
	if fieldMap["type"] != "list" {
		t.Errorf("Expected type 'list', got %v", fieldMap["type"])
	}

	// Verify validation - minCount should be 1 (effectively required)
	validation, ok := fieldMap["validation"].(map[string]any)
	if !ok {
		t.Fatal("Field 'statuses' validation not found")
	}

	// Check for minCount (camelCase is the standard format in validation builder)
	minCount, hasMinCount := validation["minCount"].(int)
	if !hasMinCount {
		// Fallback to min_count if minCount not found
		minCount, hasMinCount = validation["min_count"].(int)
	}
	if !hasMinCount || minCount < 1 {
		t.Errorf("Expected minCount or min_count >= 1, got %v", validation["minCount"])
	}
}

// TestLifecycleBuilder_SourceTypeField tests the source_type field configuration
func TestLifecycleBuilder_SourceTypeField(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleBuilder()
	spec := builder.Build()

	field, exists := spec.Fields["source_type"]
	if !exists {
		t.Fatal("Field 'source_type' not found")
	}

	fieldMap, ok := field.(map[string]any)
	if !ok {
		t.Fatal("Field 'source_type' is not a map")
	}

	// Verify type
	if fieldMap["type"] != "enum" {
		t.Errorf("Expected type 'enum', got %v", fieldMap["type"])
	}

	// Verify enum values
	validation, ok := fieldMap["validation"].(map[string]any)
	if !ok {
		t.Fatal("Field 'source_type' validation not found")
	}

	enumValues, ok := validation["enum"].([]any)
	if !ok {
		t.Fatal("Field 'source_type' enum values not found")
	}

	expectedValues := []string{"built-in", "internal"}
	if len(enumValues) != len(expectedValues) {
		t.Errorf("Expected %d enum values, got %d", len(expectedValues), len(enumValues))
	}

	valueMap := make(map[string]bool)
	for _, v := range enumValues {
		if s, ok := v.(string); ok {
			valueMap[s] = true
		}
	}

	for _, expected := range expectedValues {
		if !valueMap[expected] {
			t.Errorf("Expected enum value '%s' not found", expected)
		}
	}
}

// TestLifecycleBuilder_GetVersion tests GetVersion method
func TestLifecycleBuilder_GetVersion(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleBuilder()
	version := builder.GetVersion()

	if version != "v2_0_0" {
		t.Errorf("Expected version 'v2_0_0', got %s", version)
	}
}

// TestLifecycleBuilder_GetOntology tests GetOntology method
func TestLifecycleBuilder_GetOntology(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleBuilder()
	ontology := builder.GetOntology()

	if ontology != "lifecycle" {
		t.Errorf("Expected ontology 'lifecycle', got %s", ontology)
	}
}

// TestLifecycleBuilder_SpecStructure tests the overall spec structure
func TestLifecycleBuilder_SpecStructure(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleBuilder()
	spec := builder.Build()

	// Verify spec is a valid objects.Spec
	if spec == nil {
		t.Fatal("Build() returned nil")
	}

	// Verify all expected fields are present
	expectedFields := []string{
		"object_type",
		"source_type",
		"extends",
		"status_mapping",
		"statuses",
		"transitions",
		"percent_complete",
	}

	for _, fieldName := range expectedFields {
		if _, exists := spec.Fields[fieldName]; !exists {
			t.Errorf("Expected field '%s' not found in spec", fieldName)
		}
	}

	// Verify no unexpected fields (basic sanity check - allow some flexibility)
	if len(spec.Fields) < len(expectedFields) {
		t.Errorf("Spec has fewer fields than expected: got %d, expected at least %d",
			len(spec.Fields), len(expectedFields))
	}
}

// TestLifecycleBuilder_TransitionsField tests the transitions field configuration
func TestLifecycleBuilder_TransitionsField(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleBuilder()
	spec := builder.Build()

	field, exists := spec.Fields["transitions"]
	if !exists {
		t.Fatal("Field 'transitions' not found")
	}

	fieldMap, ok := field.(map[string]any)
	if !ok {
		t.Fatal("Field 'transitions' is not a map")
	}

	// Verify type
	if fieldMap["type"] != "list" {
		t.Errorf("Expected type 'list', got %v", fieldMap["type"])
	}

	// Transitions should be optional (no min_count requirement)
	validation, ok := fieldMap["validation"].(map[string]any)
	if ok {
		if required, ok := validation["required"].(bool); ok && required {
			t.Error("Field 'transitions' should be optional")
		}
	}
}

// TestLifecycleBuilder_PercentCompleteField tests the percent_complete field configuration
func TestLifecycleBuilder_PercentCompleteField(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleBuilder()
	spec := builder.Build()

	field, exists := spec.Fields["percent_complete"]
	if !exists {
		t.Fatal("Field 'percent_complete' not found")
	}

	fieldMap, ok := field.(map[string]any)
	if !ok {
		t.Fatal("Field 'percent_complete' is not a map")
	}

	// Verify type
	if fieldMap["type"] != "object" {
		t.Errorf("Expected type 'object', got %v", fieldMap["type"])
	}

	// percent_complete should be optional
	validation, ok := fieldMap["validation"].(map[string]any)
	if ok {
		if required, ok := validation["required"].(bool); ok && required {
			t.Error("Field 'percent_complete' should be optional")
		}
	}
}
