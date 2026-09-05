package bldr_v2

import (
	"testing"

	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// TestLifecycleBuilder_InvalidFieldTypes tests handling of invalid field configurations
func TestLifecycleBuilder_InvalidFieldTypes(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleBuilder()
	spec := builder.Build()

	// Verify that field definitions have correct types
	objectTypeField, exists := spec.Fields["object_type"]
	if !exists {
		t.Fatal("object_type field not found")
	}

	fieldMap, ok := objectTypeField.(map[string]any)
	if !ok {
		t.Fatal("object_type field is not a map")
	}

	// Verify type is string (not something invalid)
	if fieldMap["type"] != "string" {
		t.Errorf("object_type should have type 'string', got %v", fieldMap["type"])
	}
}

// TestLifecycleBuilder_EmptyStatuses tests that statuses field requires at least one item
func TestLifecycleBuilder_EmptyStatuses(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleBuilder()
	spec := builder.Build()

	statusesField, exists := spec.Fields["statuses"]
	if !exists {
		t.Fatal("statuses field not found")
	}

	fieldMap, ok := statusesField.(map[string]any)
	if !ok {
		t.Fatal("statuses field is not a map")
	}

	validation, ok := fieldMap["validation"].(map[string]any)
	if !ok {
		t.Fatal("statuses validation not found")
	}

	// Verify minCount is at least 1 (prevents empty statuses)
	minCount, hasMinCount := validation["minCount"].(int)
	if !hasMinCount {
		minCount, hasMinCount = validation["min_count"].(int)
	}
	if !hasMinCount || minCount < 1 {
		t.Error("statuses field should have minCount >= 1 to prevent empty lists")
	}
}

// TestLifecycleBuilder_InvalidSourceTypeEnum tests source_type enum validation
func TestLifecycleBuilder_InvalidSourceTypeEnum(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleBuilder()
	spec := builder.Build()

	sourceTypeField, exists := spec.Fields["source_type"]
	if !exists {
		t.Fatal("source_type field not found")
	}

	fieldMap, ok := sourceTypeField.(map[string]any)
	if !ok {
		t.Fatal("source_type field is not a map")
	}

	validation, ok := fieldMap["validation"].(map[string]any)
	if !ok {
		t.Fatal("source_type validation not found")
	}

	enumValues, ok := validation["enum"].([]any)
	if !ok {
		t.Fatal("source_type enum values not found")
	}

	// Verify enum values are valid
	validValues := map[string]bool{
		"built-in": true,
		"internal": true,
	}

	for _, val := range enumValues {
		if strVal, ok := val.(string); ok {
			if !validValues[strVal] {
				t.Errorf("Invalid enum value in source_type: %s", strVal)
			}
		}
	}
}

// TestLifecycleBuilder_ObjectTypePattern tests object_type pattern validation
func TestLifecycleBuilder_ObjectTypePattern(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleBuilder()
	spec := builder.Build()

	objectTypeField, exists := spec.Fields["object_type"]
	if !exists {
		t.Fatal("object_type field not found")
	}

	fieldMap, ok := objectTypeField.(map[string]any)
	if !ok {
		t.Fatal("object_type field is not a map")
	}

	validation, ok := fieldMap["validation"].(map[string]any)
	if !ok {
		t.Fatal("object_type validation not found")
	}

	pattern, ok := validation["pattern"].(string)
	if !ok {
		t.Fatal("object_type pattern not found")
	}

	// Pattern should enforce lowercase with underscores
	if pattern != `^[a-z_]+$` {
		t.Errorf("object_type pattern should be '^[a-z_]+$', got %s", pattern)
	}
}

// TestLifecycleBuilder_ExtendsOptional tests that extends field is optional
func TestLifecycleBuilder_ExtendsOptional(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleBuilder()
	spec := builder.Build()

	extendsField, exists := spec.Fields["extends"]
	if !exists {
		t.Fatal("extends field not found")
	}

	fieldMap, ok := extendsField.(map[string]any)
	if !ok {
		t.Fatal("extends field is not a map")
	}

	validation, ok := fieldMap["validation"].(map[string]any)
	if ok {
		if required, ok := validation["required"].(bool); ok && required {
			t.Error("extends field should be optional")
		}
	}
}

// TestLifecycleBuilder_TransitionsOptional tests that transitions field is optional
func TestLifecycleBuilder_TransitionsOptional(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleBuilder()
	spec := builder.Build()

	transitionsField, exists := spec.Fields["transitions"]
	if !exists {
		t.Fatal("transitions field not found")
	}

	fieldMap, ok := transitionsField.(map[string]any)
	if !ok {
		t.Fatal("transitions field is not a map")
	}

	validation, ok := fieldMap["validation"].(map[string]any)
	if ok {
		if required, ok := validation["required"].(bool); ok && required {
			t.Error("transitions field should be optional")
		}
	}
}

// TestLifecycleBuilder_MultipleBuilds tests that builder can build multiple times
func TestLifecycleBuilder_MultipleBuilds(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleBuilder()

	// Build first spec
	spec1 := builder.Build()
	if spec1 == nil {
		t.Fatal("First build returned nil")
	}

	// Build second spec
	spec2 := builder.Build()
	if spec2 == nil {
		t.Fatal("Second build returned nil")
	}

	// Verify both specs have same structure
	if spec1.Ontology != spec2.Ontology {
		t.Error("Multiple builds should return consistent ontology")
	}

	if len(spec1.Fields) != len(spec2.Fields) {
		t.Error("Multiple builds should return same number of fields")
	}
}

// TestLifecycleBuilder_GetVersionConsistency tests version consistency
func TestLifecycleBuilder_GetVersionConsistency(t *testing.T) {
	t.Parallel()
	builder1 := NewLifecycleBuilder()
	builder2 := NewLifecycleBuilder()

	version1 := builder1.GetVersion()
	version2 := builder2.GetVersion()

	if version1 != version2 {
		t.Errorf("Multiple builders should return same version: %s != %s", version1, version2)
	}

	if version1 != "v2_0_0" {
		t.Errorf("Expected version 'v2_0_0', got %s", version1)
	}
}

// TestLifecycleBuilder_GetOntologyConsistency tests ontology consistency
func TestLifecycleBuilder_GetOntologyConsistency(t *testing.T) {
	t.Parallel()
	builder1 := NewLifecycleBuilder()
	builder2 := NewLifecycleBuilder()

	ontology1 := builder1.GetOntology()
	ontology2 := builder2.GetOntology()

	if ontology1 != ontology2 {
		t.Errorf("Multiple builders should return same ontology: %s != %s", ontology1, ontology2)
	}

	if ontology1 != "lifecycle" {
		t.Errorf("Expected ontology 'lifecycle', got %s", ontology1)
	}
}

// TestLifecycleBuilder_RegistryConsistency tests that builder is consistently registered
func TestLifecycleBuilder_RegistryConsistency(t *testing.T) {
	t.Parallel()
	registry := builders.GetGlobalRegistry()

	// Get builder multiple times
	builder1, err1 := registry.GetBuilder("lifecycle", "v2_0_0")
	if err1 != nil {
		t.Fatalf("Failed to get builder first time: %v", err1)
	}

	builder2, err2 := registry.GetBuilder("lifecycle", "v2_0_0")
	if err2 != nil {
		t.Fatalf("Failed to get builder second time: %v", err2)
	}

	// Verify both builders return same version and ontology
	if builder1.GetVersion() != builder2.GetVersion() {
		t.Error("Registry should return consistent builders")
	}

	if builder1.GetOntology() != builder2.GetOntology() {
		t.Error("Registry should return consistent builders")
	}
}

// TestLifecycleBuilder_InvalidVersion tests accessing non-existent version
func TestLifecycleBuilder_InvalidVersion(t *testing.T) {
	t.Parallel()
	registry := builders.GetGlobalRegistry()

	_, err := registry.GetBuilder("lifecycle", "v9_9_9")
	if err == nil {
		t.Error("Expected error when accessing non-existent version")
	}
}

// TestLifecycleBuilder_InvalidOntology tests accessing non-existent ontology
func TestLifecycleBuilder_InvalidOntology(t *testing.T) {
	t.Parallel()
	registry := builders.GetGlobalRegistry()

	_, err := registry.GetBuilder("nonexistent_kind", "v2_0_0")
	if err == nil {
		t.Error("Expected error when accessing non-existent ontology")
	}
}
