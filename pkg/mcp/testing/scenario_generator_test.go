package testing

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestScenarioGenerator_GenerateFromSpecs(t *testing.T) {
	t.Parallel()
	// Create temporary output directory
	tmpDir := t.TempDir()

	// Define test specs
	specs := []ScenarioSpec{
		{
			Name:        "Test Scenario 1",
			Description: "First test scenario",
			Tests: []TestStep{
				{
					Name: "Test step",
					Tool: "test_echo",
					Args: map[string]any{
						"message": "test",
					},
					Expected: ExpectSuccess(),
				},
			},
		},
		{
			Name:        "Test Scenario 2",
			Description: "Second test scenario",
			Tests: []TestStep{
				{
					Name: "Another test step",
					Tool: "test_echo",
					Args: map[string]any{
						"message": "test2",
					},
					Expected: ExpectSuccess(),
				},
			},
		},
	}

	// Generate scenarios
	generator := NewScenarioGenerator(tmpDir)
	if err := generator.GenerateFromSpecs(specs); err != nil {
		t.Fatalf("Failed to generate scenarios: %v", err)
	}

	// Verify files were created
	expectedFiles := []string{
		"Test_Scenario_1.yaml",
		"Test_Scenario_2.yaml",
	}

	for _, expectedFile := range expectedFiles {
		filePath := filepath.Join(tmpDir, expectedFile)
		if _, err := fileutil.Stat(filePath); fileutil.IsNotExist(err) {
			t.Errorf("Expected file %s was not created", expectedFile)
		}

		// Verify file can be loaded
		loader := NewScenarioLoader(tmpDir)
		scenario, err := loader.LoadScenario(filePath)
		if err != nil {
			t.Errorf("Failed to load generated scenario %s: %v", expectedFile, err)
			continue
		}

		if scenario.Name == emptyValue {
			t.Errorf("Generated scenario %s has empty name", expectedFile)
		}
	}
}

func TestScenarioGenerator_GenerateFromFile(t *testing.T) {
	t.Parallel()
	// Create temporary directories
	tmpDir := t.TempDir()
	specDir := t.TempDir()

	// Create a test spec file
	specFile := filepath.Join(specDir, "test-scenarios.yaml")
	specContent := `scenarios:
  - name: "Generated Test Scenario"
    description: "A test scenario generated from YAML"
    tests:
      - name: "Test step"
        tool: "test_echo"
        args:
          message: "test"
        expected:
          success: true
`

	if err := fileutil.WriteFile(specFile, []byte(specContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write spec file: %v", err)
	}

	// Generate scenarios
	generator := NewScenarioGenerator(tmpDir)
	if err := generator.GenerateFromFile(specFile); err != nil {
		t.Fatalf("Failed to generate scenarios from file: %v", err)
	}

	// Verify file was created
	expectedFile := "Generated_Test_Scenario.yaml"
	filePath := filepath.Join(tmpDir, expectedFile)
	if _, err := fileutil.Stat(filePath); fileutil.IsNotExist(err) {
		t.Fatalf("Expected file %s was not created", expectedFile)
	}

	// Verify file can be loaded and matches spec
	loader := NewScenarioLoader(tmpDir)
	scenario, err := loader.LoadScenario(filePath)
	if err != nil {
		t.Fatalf("Failed to load generated scenario: %v", err)
	}

	if scenario.Name != "Generated Test Scenario" {
		t.Errorf("Expected name 'Generated Test Scenario', got %s", scenario.Name)
	}
	if scenario.Description != "A test scenario generated from YAML" {
		t.Errorf("Expected description 'A test scenario generated from YAML', got %s", scenario.Description)
	}
	if len(scenario.Tests) != 1 {
		t.Errorf("Expected 1 test step, got %d", len(scenario.Tests))
	}
	if scenario.Tests[0].Tool != "test_echo" {
		t.Errorf("Expected tool 'test_echo', got %s", scenario.Tests[0].Tool)
	}
}

func TestScenarioGenerator_BuildScenarioFromSpec(t *testing.T) {
	t.Parallel()
	generator := NewScenarioGenerator("")

	spec := ScenarioSpec{
		Name:              "Test Scenario",
		Description:       "Test description",
		ResponseProcessor: "noop",
		Tests: []TestStep{
			{
				Name: "Test step",
				Tool: "test_echo",
				Args: map[string]any{
					"message": "test",
				},
			},
		},
	}

	scenario := generator.buildScenarioFromSpec(spec)

	if scenario.Name != spec.Name {
		t.Errorf("Expected name %s, got %s", spec.Name, scenario.Name)
	}
	if scenario.Description != spec.Description {
		t.Errorf("Expected description %s, got %s", spec.Description, scenario.Description)
	}
	if scenario.ResponseProcessor != spec.ResponseProcessor {
		t.Errorf("Expected response_processor %s, got %s", spec.ResponseProcessor, scenario.ResponseProcessor)
	}
	if len(scenario.Tests) != 1 {
		t.Errorf("Expected 1 test step, got %d", len(scenario.Tests))
	}
	if scenario.Tests[0].Name != "Test step" {
		t.Errorf("Expected test step name 'Test step', got %s", scenario.Tests[0].Name)
	}
}

func TestScenarioGenerator_GenerateFilename(t *testing.T) {
	t.Parallel()
	generator := NewScenarioGenerator("")

	tests := []struct {
		name     string
		index    int
		expected string
	}{
		{"Test Scenario", 0, "Test_Scenario.yaml"},
		{"Interactive Creation - All Kinds", 0, "Interactive_Creation_All_Kinds.yaml"},
		{"System Status Check", 1, "System_Status_Check.yaml"},
		{"Object List Operations", 2, "Object_List_Operations.yaml"},
		{"", 5, "scenario_5.yaml"},
	}

	for _, tt := range tests {
		filename := generator.generateFilename(tt.name, tt.index)
		if filename != tt.expected {
			t.Errorf("generateFilename(%q, %d) = %s, want %s", tt.name, tt.index, filename, tt.expected)
		}
	}
}

func TestGenerateScenariosFromSpecFile(t *testing.T) {
	t.Parallel()
	// Create temporary directories
	tmpDir := t.TempDir()
	specDir := t.TempDir()

	// Create a test spec file
	specFile := filepath.Join(specDir, "test-scenarios.yaml")
	specContent := `scenarios:
  - name: "Scenario 1"
    tests:
      - name: "Step 1"
        tool: "test_echo"
        args:
          message: "test"
        expected:
          success: true
`

	if err := fileutil.WriteFile(specFile, []byte(specContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write spec file: %v", err)
	}

	// Generate scenarios using convenience function
	if err := GenerateScenariosFromSpecFile(specFile, tmpDir); err != nil {
		t.Fatalf("Failed to generate scenarios: %v", err)
	}

	// Verify file was created
	expectedFile := "Scenario_1.yaml"
	filePath := filepath.Join(tmpDir, expectedFile)
	if _, err := fileutil.Stat(filePath); fileutil.IsNotExist(err) {
		t.Fatalf("Expected file %s was not created", expectedFile)
	}
}
