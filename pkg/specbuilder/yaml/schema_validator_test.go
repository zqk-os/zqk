package yaml

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSchemaValidator_ValidateYAML(t *testing.T) {
	t.Parallel()
	// Create a temporary test schema
	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "test_schema.json")
	schemaContent := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type": "object",
		"required": ["name"],
		"properties": {
			"name": {
				"type": "string"
			}
		}
	}`
	if err := fileutil.WriteFile(schemaPath, []byte(schemaContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to create test schema: %v", err)
	}

	// Create a valid YAML file
	yamlPath := filepath.Join(tmpDir, "test.yaml")
	yamlContent := "name: test\n"
	if err := fileutil.WriteFile(yamlPath, []byte(yamlContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to create test YAML: %v", err)
	}

	validator := NewSchemaValidator(tmpDir)

	// Test valid YAML
	if err := validator.ValidateYAML(yamlPath, "test_schema.json"); err != nil {
		t.Errorf("expected validation to pass, got error: %v", err)
	}

	// Test invalid YAML (missing required field)
	invalidYamlPath := filepath.Join(tmpDir, "invalid.yaml")
	invalidYamlContent := "other: value\n"
	if err := fileutil.WriteFile(invalidYamlPath, []byte(invalidYamlContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to create invalid YAML: %v", err)
	}

	if err := validator.ValidateYAML(invalidYamlPath, "test_schema.json"); err == nil {
		t.Error("expected validation to fail for invalid YAML, but it passed")
	}
}

func TestSchemaValidator_ValidateYAMLWithAutoSchema(t *testing.T) {
	t.Parallel()
	// Create a temporary test schema
	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "test_schema.json")
	schemaContent := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type": "object",
		"required": ["name"],
		"properties": {
			"name": {
				"type": "string"
			}
		}
	}`
	if err := fileutil.WriteFile(schemaPath, []byte(schemaContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to create test schema: %v", err)
	}

	// Create a YAML file with $schema reference
	yamlPath := filepath.Join(tmpDir, "test.yaml")
	yamlContent := `$schema: "test_schema.json"
name: test
`
	if err := fileutil.WriteFile(yamlPath, []byte(yamlContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to create test YAML: %v", err)
	}

	validator := NewSchemaValidator(tmpDir)

	// Test auto-schema detection
	if err := validator.ValidateYAMLWithAutoSchema(yamlPath); err != nil {
		t.Errorf("expected validation to pass, got error: %v", err)
	}
}
