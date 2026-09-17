package generators

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	mcptesting "github.com/lanceman/zqk/pkg/mcp/testing"
)

// TestParallelImplementation tests that both old and new generators can coexist
func TestParallelImplementation(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	oldDir := filepath.Join(tmpDir, "old")
	newDir := filepath.Join(tmpDir, "new")

	if err := fileutil.MkdirAll(oldDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create old directory: %v", err)
	}
	if err := fileutil.MkdirAll(newDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create new directory: %v", err)
	}

	// Create test spec
	spec := mcptesting.ScenarioSpec{
		Name:        "Parallel Test",
		Description: "Test parallel implementation",
		Tests: []mcptesting.TestStep{
			{Name: "Test", Tool: "zqk_test_echo"},
		},
	}

	// Generate using OLD generator (existing implementation)
	oldGenerator := mcptesting.NewScenarioGenerator(oldDir)
	if err := oldGenerator.GenerateFromSpecs([]mcptesting.ScenarioSpec{spec}); err != nil {
		t.Fatalf("Old generator failed: %v", err)
	}

	// Generate using NEW generator (specbuilder implementation)
	newGenerator := NewScenarioGenerator(newDir)
	if err := newGenerator.GenerateFromSpecs([]mcptesting.ScenarioSpec{spec}); err != nil {
		t.Fatalf("New generator failed: %v", err)
	}

	// Verify both generated files
	oldFiles, err := filepath.Glob(filepath.Join(oldDir, "*.yaml"))
	if err != nil {
		t.Fatalf("Failed to list old files: %v", err)
	}
	if len(oldFiles) != 1 {
		t.Fatalf("Expected 1 old file, got %d", len(oldFiles))
	}

	newFiles, err := filepath.Glob(filepath.Join(newDir, "*.yaml"))
	if err != nil {
		t.Fatalf("Failed to list new files: %v", err)
	}
	if len(newFiles) != 1 {
		t.Fatalf("Expected 1 new file, got %d", len(newFiles))
	}

	// Load and verify both scenarios
	loader := mcptesting.NewScenarioLoader(oldDir)
	oldScenario, err := loader.LoadScenario(oldFiles[0])
	if err != nil {
		t.Fatalf("Failed to load old scenario: %v", err)
	}

	newScenario, err := loader.LoadScenario(newFiles[0])
	if err != nil {
		t.Fatalf("Failed to load new scenario: %v", err)
	}

	// Verify they're equivalent
	if oldScenario.Name != newScenario.Name {
		t.Errorf("Name mismatch: old=%s, new=%s", oldScenario.Name, newScenario.Name)
	}
	if oldScenario.Description != newScenario.Description {
		t.Errorf("Description mismatch: old=%s, new=%s", oldScenario.Description, newScenario.Description)
	}
	if len(oldScenario.Tests) != len(newScenario.Tests) {
		t.Errorf("Tests count mismatch: old=%d, new=%d", len(oldScenario.Tests), len(newScenario.Tests))
	}
}

// TestGenerateFromFile tests the GenerateFromFile method
func TestGenerateFromFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	specDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "generated")

	if err := fileutil.MkdirAll(outputDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create output directory: %v", err)
	}

	// Create test spec file
	specFile := filepath.Join(specDir, "test-scenarios.yaml")
	specContent := `scenarios:
  - name: "File Test"
    description: "Test GenerateFromFile"
    tests:
      - name: "Test step"
        tool: "zqk_test_echo"
        args:
          message: "test"
        expected:
          success: true
`

	if err := fileutil.WriteFile(specFile, []byte(specContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write spec file: %v", err)
	}

	// Generate using new generator
	generator := NewScenarioGenerator(outputDir)
	if err := generator.GenerateFromFile(specFile); err != nil {
		t.Fatalf("Failed to generate from file: %v", err)
	}

	// Verify file was created
	files, err := filepath.Glob(filepath.Join(outputDir, "*.yaml"))
	if err != nil {
		t.Fatalf("Failed to list files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(files))
	}

	// Load and verify
	loader := mcptesting.NewScenarioLoader(outputDir)
	scenario, err := loader.LoadScenario(files[0])
	if err != nil {
		t.Fatalf("Failed to load scenario: %v", err)
	}

	if scenario.Name != "File Test" {
		t.Errorf("Expected name 'File Test', got '%s'", scenario.Name)
	}
}

// TestConvenienceFunction tests the convenience function
func TestConvenienceFunction(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	specDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "generated")

	if err := fileutil.MkdirAll(outputDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create output directory: %v", err)
	}

	// Create test spec file
	specFile := filepath.Join(specDir, "test.yaml")
	specContent := `scenarios:
  - name: "Convenience Test"
    tests:
      - name: "Test"
        tool: "test"
`

	if err := fileutil.WriteFile(specFile, []byte(specContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write spec file: %v", err)
	}

	// Use convenience function
	if err := GenerateScenariosFromSpecFile(specFile, outputDir); err != nil {
		t.Fatalf("Convenience function failed: %v", err)
	}

	// Verify file was created
	files, err := filepath.Glob(filepath.Join(outputDir, "*.yaml"))
	if err != nil {
		t.Fatalf("Failed to list files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(files))
	}
}
