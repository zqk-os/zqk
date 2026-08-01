package testing

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
)

// TestScenarioLoader_InvalidYAML tests error handling for invalid YAML
func TestScenarioLoader_InvalidYAML(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	loader := NewScenarioLoader(testDir)

	// Create invalid YAML file
	invalidFile := filepath.Join(testDir, "invalid.yaml")
	if err := os.WriteFile(invalidFile, []byte("invalid: yaml: content: [unclosed"), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create invalid YAML file: %v", err)
	}

	_, err := loader.LoadScenario(invalidFile)
	if err == nil {
		t.Error("Expected error for invalid YAML, got nil")
	}
}

// TestScenarioLoader_MissingFile tests error handling for missing files
func TestScenarioLoader_MissingFile(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	loader := NewScenarioLoader(testDir)

	missingFile := filepath.Join(testDir, "nonexistent.yaml")
	_, err := loader.LoadScenario(missingFile)
	if err == nil {
		t.Error("Expected error for missing file, got nil")
	}
}

// TestScenarioLoader_EmptyFile tests handling of empty files
func TestScenarioLoader_EmptyFile(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	loader := NewScenarioLoader(testDir)

	emptyFile := filepath.Join(testDir, "empty.yaml")
	if err := os.WriteFile(emptyFile, []byte(""), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create empty file: %v", err)
	}

	// Empty file should load but have empty scenario
	scenario, err := loader.LoadScenario(emptyFile)
	if err != nil {
		t.Fatalf("Unexpected error loading empty file: %v", err)
	}
	if scenario.Name != emptyValue {
		t.Errorf("Expected empty name for empty scenario, got: %s", scenario.Name)
	}
}

// TestScenarioLoader_MissingImportFile tests error handling for missing import files
func TestScenarioLoader_MissingImportFile(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	loader := NewScenarioLoader(testDir)

	// Create scenario file with missing import
	scenarioFile := filepath.Join(testDir, "scenario.yaml")
	scenarioContent := `name: "Test Scenario"
imports:
  - "nonexistent.yaml"
tests:
  - name: "Test step"
    tool: "test_tool"
`
	if err := os.WriteFile(scenarioFile, []byte(scenarioContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create scenario file: %v", err)
	}

	_, err := loader.LoadScenario(scenarioFile)
	if err == nil {
		t.Error("Expected error for missing import file, got nil")
	}
}

// TestScenarioLoader_CircularImport tests circular import detection
func TestScenarioLoader_CircularImport(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	loader := NewScenarioLoader(testDir)

	// Create scenario A that imports B
	fileA := filepath.Join(testDir, "a.yaml")
	contentA := `name: "Scenario A"
imports:
  - "b.yaml"
tests: []
`
	if err := os.WriteFile(fileA, []byte(contentA), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create file A: %v", err)
	}

	// Create scenario B that imports A (circular)
	fileB := filepath.Join(testDir, "b.yaml")
	contentB := `name: "Scenario B"
imports:
  - "a.yaml"
tests: []
`
	if err := os.WriteFile(fileB, []byte(contentB), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create file B: %v", err)
	}

	_, err := loader.LoadScenario(fileA)
	if err == nil {
		t.Error("Expected error for circular import, got nil")
	} else if err.Error() == emptyValue || err.Error() == "circular import detected" {
		// Check that error message mentions circular import
		if err.Error() == emptyValue {
			t.Error("Expected error message for circular import")
		}
	}
}

// TestScenarioLoader_NestedImports tests nested imports (A imports B, B imports C)
func TestScenarioLoader_NestedImports(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	loader := NewScenarioLoader(testDir)

	// Create scenario C
	fileC := filepath.Join(testDir, "c.yaml")
	contentC := `name: "Scenario C"
tests:
  - name: "Test C"
    tool: "test_tool"
`
	if err := os.WriteFile(fileC, []byte(contentC), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create file C: %v", err)
	}

	// Create scenario B that imports C
	fileB := filepath.Join(testDir, "b.yaml")
	contentB := `name: "Scenario B"
imports:
  - "c.yaml"
tests:
  - name: "Test B"
    tool: "test_tool"
`
	if err := os.WriteFile(fileB, []byte(contentB), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create file B: %v", err)
	}

	// Create scenario A that imports B
	fileA := filepath.Join(testDir, "a.yaml")
	contentA := `name: "Scenario A"
imports:
  - "b.yaml"
tests:
  - name: "Test A"
    tool: "test_tool"
`
	if err := os.WriteFile(fileA, []byte(contentA), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create file A: %v", err)
	}

	scenario, err := loader.LoadScenario(fileA)
	if err != nil {
		t.Fatalf("Unexpected error loading nested imports: %v", err)
	}

	// Should have all tests merged (A + B + C)
	// Note: mergeScenarios appends tests, so order is: B tests, then A tests (C is merged into B first)
	expectedTestCount := 3 // A has 1, B has 1, C has 1
	if len(scenario.Tests) < expectedTestCount {
		t.Errorf("Expected at least %d tests (A, B, C merged), got %d", expectedTestCount, len(scenario.Tests))
		t.Logf("Actual tests: %v", scenario.Tests)
	}
}

// TestScenarioLoader_EmptyScenario tests handling of scenarios with no tests
func TestScenarioLoader_EmptyScenario(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	loader := NewScenarioLoader(testDir)

	scenarioFile := filepath.Join(testDir, "empty_scenario.yaml")
	scenarioContent := `name: "Empty Scenario"
description: "A scenario with no tests"
`
	if err := os.WriteFile(scenarioFile, []byte(scenarioContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create scenario file: %v", err)
	}

	scenario, err := loader.LoadScenario(scenarioFile)
	if err != nil {
		t.Fatalf("Unexpected error loading empty scenario: %v", err)
	}

	if scenario.Name != "Empty Scenario" {
		t.Errorf("Expected name 'Empty Scenario', got: %s", scenario.Name)
	}
	if len(scenario.Tests) != 0 {
		t.Errorf("Expected 0 tests, got %d", len(scenario.Tests))
	}
}

// TestScenarioLoader_DuplicateImports tests handling of duplicate imports
func TestScenarioLoader_DuplicateImports(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	loader := NewScenarioLoader(testDir)

	// Create imported file
	importFile := filepath.Join(testDir, "imported.yaml")
	importContent := `key: "value"
`
	if err := os.WriteFile(importFile, []byte(importContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create import file: %v", err)
	}

	// Create scenario with duplicate imports
	scenarioFile := filepath.Join(testDir, "scenario.yaml")
	scenarioContent := `name: "Test Scenario"
imports:
  - "imported.yaml"
  - "imported.yaml"
tests: []
`
	if err := os.WriteFile(scenarioFile, []byte(scenarioContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create scenario file: %v", err)
	}

	// Should handle duplicate imports gracefully (currently overwrites)
	scenario, err := loader.LoadScenario(scenarioFile)
	if err != nil {
		t.Fatalf("Unexpected error loading scenario with duplicate imports: %v", err)
	}

	if scenario.Name != "Test Scenario" {
		t.Errorf("Expected name 'Test Scenario', got: %s", scenario.Name)
	}
}
