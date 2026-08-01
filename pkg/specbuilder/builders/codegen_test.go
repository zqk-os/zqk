package builders

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"gopkg.in/yaml.v3"
)

// TestGenerateFieldConstantsCode_Auditable tests that auditable constants only include
// fields defined in auditable.yaml (not inherited fields)
func TestGenerateFieldConstantsCode_Auditable(t *testing.T) {
	t.Parallel()
	// Load auditable.yaml
	specsDir := findSpecsDir()
	if specsDir == emptyValue {
		t.Fatal("Could not find specs directory")
	}

	auditablePath := filepath.Join(specsDir, "auditable.yaml")
	data, err := os.ReadFile(auditablePath)
	if err != nil {
		t.Fatalf("Failed to read auditable.yaml: %v", err)
	}

	var spec objects.Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("Failed to parse auditable.yaml: %v", err)
	}

	// Normalize "null" extends
	if spec.Extends == "null" {
		spec.Extends = ""
	}

	// Generate constants code using ConstantsFactory
	constantsFactory := NewConstantsFactory("bldr_v2")
	constants, err := constantsFactory.CreateConstants(&spec, "auditable")
	if err != nil {
		t.Fatalf("Failed to generate constants: %v", err)
	}
	specConstants, ok := constants.(*SpecConstants)
	if !ok {
		t.Fatalf("Unexpected constants type: %T", constants)
	}
	constantsCode := specConstants.ToGoCode()

	// Expected fields from auditable.yaml (9 fields)
	expectedFields := map[string]bool{
		"archived_at":    true,
		"archived_by":    true,
		"change_log":     true,
		"created_at":     true,
		"created_by":     true,
		"origin_project": true,
		"origin_system":  true,
		"updated_at":     true,
		"updated_by":     true,
	}

	// Fields that should NOT be in auditable_constants.go (these are from base_object or system)
	unexpectedFields := map[string]bool{
		"id":             true,
		"kind":           true,
		"schema_version": true,
		"namespace_id":   true,
		"status":         true,
		"title":          true,
		"description":    true,
	}

	// Verify expected fields are present
	for fieldName := range expectedFields {
		constantName := "Field" + toCamelCase(fieldName)
		if !strings.Contains(constantsCode, constantName+" =") {
			t.Errorf("Expected constant %s not found in generated code", constantName)
		}
	}

	// Verify unexpected fields are NOT present
	for fieldName := range unexpectedFields {
		constantName := "Field" + toCamelCase(fieldName)
		if strings.Contains(constantsCode, constantName+" =") {
			t.Errorf("Unexpected constant %s found in auditable_constants.go (should only be in base_object_constants.go)", constantName)
		}
	}

	// Verify we have exactly the expected number of field constants
	// Count the number of "FieldX = " patterns
	constantCount := strings.Count(constantsCode, " = \"")
	if constantCount != len(expectedFields) {
		t.Errorf("Expected %d field constants, found %d", len(expectedFields), constantCount)
		t.Logf("Generated code:\n%s", constantsCode)
	}
}

// TestGenerateFieldConstantsCode_BaseObject tests that base_object constants only include
// fields defined in base_object.yaml (not inherited from auditable)
func TestGenerateFieldConstantsCode_BaseObject(t *testing.T) {
	t.Parallel()
	// Load base_object.yaml
	specsDir := findSpecsDir()
	if specsDir == emptyValue {
		t.Fatal("Could not find specs directory")
	}

	baseObjectPath := filepath.Join(specsDir, "base_object.yaml")
	data, err := os.ReadFile(baseObjectPath)
	if err != nil {
		t.Fatalf("Failed to read base_object.yaml: %v", err)
	}

	var spec objects.Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("Failed to parse base_object.yaml: %v", err)
	}

	// Normalize "null" extends
	if spec.Extends == "null" {
		spec.Extends = ""
	}

	// Generate constants code using ConstantsFactory
	constantsFactory := NewConstantsFactory("bldr_v2")
	constants, err := constantsFactory.CreateConstants(&spec, "base_object")
	if err != nil {
		t.Fatalf("Failed to generate constants: %v", err)
	}
	specConstants, ok := constants.(*SpecConstants)
	if !ok {
		t.Fatalf("Unexpected constants type: %T", constants)
	}
	constantsCode := specConstants.ToGoCode()

	// Fields that should be in base_object_constants.go (defined in base_object.yaml)
	// Note: description is not a field in base_object.yaml, it's metadata
	expectedFields := map[string]bool{
		"id":             true,
		"kind":           true,
		"schema_version": true,
		"status":         true,
		"title":          true,
		"namespace_id":   true,
		// Add more base_object-specific fields as needed
	}

	// Fields that should NOT be in base_object_constants.go (these are from auditable)
	unexpectedFields := map[string]bool{
		"created_at":     true,
		"created_by":     true,
		"updated_at":     true,
		"updated_by":     true,
		"archived_at":    true,
		"archived_by":    true,
		"change_log":     true,
		"origin_project": true,
		"origin_system":  true,
	}

	// Verify expected fields are present
	for fieldName := range expectedFields {
		constantName := "Field" + toCamelCase(fieldName)
		if !strings.Contains(constantsCode, constantName+" =") {
			t.Errorf("Expected constant %s not found in generated code", constantName)
		}
	}

	// Verify unexpected fields are NOT present
	for fieldName := range unexpectedFields {
		constantName := "Field" + toCamelCase(fieldName)
		if strings.Contains(constantsCode, constantName+" =") {
			t.Errorf("Unexpected constant %s found in base_object_constants.go (should only be in auditable_constants.go)", constantName)
		}
	}
}

// TestGenerateFieldConstantsCode_FieldFiltering tests the core filtering logic
// by verifying that fields in parent's ResolvedFields are excluded
func TestGenerateFieldConstantsCode_FieldFiltering(t *testing.T) {
	t.Parallel()
	// Initialize spec loader
	_ = objects.GetGlobalSpecLoader()

	// Load base_object.yaml
	specsDir := findSpecsDir()
	if specsDir == emptyValue {
		t.Fatal("Could not find specs directory")
	}

	baseObjectPath := filepath.Join(specsDir, "base_object.yaml")
	data, err := os.ReadFile(baseObjectPath)
	if err != nil {
		t.Fatalf("Failed to read base_object.yaml: %v", err)
	}

	var spec objects.Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("Failed to parse base_object.yaml: %v", err)
	}

	// Normalize "null" extends
	if spec.Extends == "null" {
		spec.Extends = ""
	}

	// Log what fields are in spec.Fields (directly defined in base_object.yaml)
	t.Logf("base_object.yaml defines %d fields directly:", len(spec.Fields))
	for fieldName := range spec.Fields {
		t.Logf("  - %s", fieldName)
	}

	// Generate constants code using ConstantsFactory
	constantsFactory := NewConstantsFactory("bldr_v2")
	constants, err := constantsFactory.CreateConstants(&spec, "base_object")
	if err != nil {
		t.Fatalf("Failed to generate constants: %v", err)
	}
	specConstants, ok := constants.(*SpecConstants)
	if !ok {
		t.Fatalf("Unexpected constants type: %T", constants)
	}
	constantsCode := specConstants.ToGoCode()

	// Extract all generated constants from the code
	generatedConstants := extractConstantsFromCode(constantsCode)
	t.Logf("Generated %d constants in base_object_constants.go:", len(generatedConstants))
	for _, constName := range generatedConstants {
		t.Logf("  - %s", constName)
	}

	// Verify that auditable fields are NOT in the generated constants
	auditableFields := []string{
		"created_at", "created_by", "updated_at", "updated_by",
		"archived_at", "archived_by", "change_log", "origin_project", "origin_system",
	}

	for _, fieldName := range auditableFields {
		constantName := "Field" + toCamelCase(fieldName)
		if strings.Contains(constantsCode, constantName+" =") {
			t.Errorf("FAIL: Found auditable field constant %s in base_object_constants.go", constantName)
		}
	}

	// Verify that base_object-specific fields ARE in the generated constants
	baseObjectFields := []string{"id", "kind", "schema_version", "status", "title"}
	for _, fieldName := range baseObjectFields {
		constantName := "Field" + toCamelCase(fieldName)
		if !strings.Contains(constantsCode, constantName+" =") {
			t.Errorf("FAIL: Expected base_object field constant %s not found", constantName)
		}
	}
}

// extractConstantsFromCode extracts constant names from generated Go code
func extractConstantsFromCode(code string) []string {
	var constants []string
	for line := range strings.SplitSeq(code, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Field") && strings.Contains(line, " = ") {
			// Extract constant name (e.g., "FieldId" from "FieldId = \"id\"")
			parts := strings.Split(line, " ")
			if len(parts) > 0 {
				constants = append(constants, parts[0])
			}
		}
	}
	return constants
}

// findSpecsDir finds the specs directory (same logic as in objects package)
func findSpecsDir() string {
	// Use the same logic as objects.findSpecsDir
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}

	// Try common locations relative to working directory
	possiblePaths := []string{
		paths.ProcessInternalObjectSpecsDir,
		filepath.Join("..", paths.ProcessInternalObjectSpecsDir),
		filepath.Join("..", "..", paths.ProcessInternalObjectSpecsDir),
	}

	for _, path := range possiblePaths {
		absPath := filepath.Join(wd, path)
		if info, err := os.Stat(absPath); err == nil && info.IsDir() {
			return absPath
		}
	}

	// Walk up directory tree
	dir := wd
	for {
		potentialPath := filepath.Join(dir, paths.ProcessInternalObjectSpecsDir)
		if _, err := os.Stat(potentialPath); err == nil {
			return potentialPath
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return ""
}

// TestGenerateBuilderFromYAML_ConstantsGeneration tests the full codegen flow
// to verify constants are generated correctly using ConstantsFactory
func TestGenerateBuilderFromYAML_ConstantsGeneration(t *testing.T) {
	t.Parallel()
	specsDir := findSpecsDir()
	if specsDir == emptyValue {
		t.Fatal("Could not find specs directory")
	}

	auditablePath := filepath.Join(specsDir, "auditable.yaml")

	// Create a temporary output directory
	tmpDir := t.TempDir()

	// Call GenerateBuilderFromYAML (the actual codegen function)
	// Create a constants factory for this test
	constantsFactory := NewConstantsFactory("bldr_v2")
	err := GenerateBuilderFromYAML(auditablePath, tmpDir, "", constantsFactory)
	if err != nil {
		t.Fatalf("GenerateBuilderFromYAML failed: %v", err)
	}

	// Read the generated constants file
	constantsFile := filepath.Join(filepath.Dir(tmpDir), "bldr_v2", "auditable_constants.go")
	data, err := os.ReadFile(constantsFile)
	if err != nil {
		t.Fatalf("Failed to read generated constants file: %v", err)
	}

	constantsCode := string(data)
	t.Logf("Generated constants code:\n%s", constantsCode)

	// Verify it only contains auditable fields
	expectedFields := map[string]bool{
		"archived_at":    true,
		"archived_by":    true,
		"change_log":     true,
		"created_at":     true,
		"created_by":     true,
		"origin_project": true,
		"origin_system":  true,
		"updated_at":     true,
		"updated_by":     true,
	}

	unexpectedFields := map[string]bool{
		"id":             true,
		"kind":           true,
		"schema_version": true,
		"namespace_id":   true,
		"status":         true,
	}

	// Check for unexpected fields
	for fieldName := range unexpectedFields {
		constantName := "Field" + toCamelCase(fieldName)
		if strings.Contains(constantsCode, constantName+" =") {
			t.Errorf("FAIL: Found unexpected constant %s in auditable_constants.go", constantName)
		}
	}

	// Count constants
	constantCount := strings.Count(constantsCode, " = \"")
	if constantCount != len(expectedFields) {
		t.Errorf("Expected %d field constants, found %d", len(expectedFields), constantCount)
	}
}
