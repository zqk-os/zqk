package testing

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/mcp"
	"github.com/lanceman/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

// TestBuildAllKindsScenario tests building the all-kinds.yaml scenario using the builder API
// and validates it matches the existing file
func TestBuildAllKindsScenario(t *testing.T) {
	t.Parallel()
	// Get project root
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	projectRoot := filepath.Join(cwd, "..", "..", "..")

	// Path to the existing scenario file
	existingFilePath := filepath.Join(projectRoot, "test-scenarios", "interactive", "all-kinds.yaml")
	if _, err := os.Stat(existingFilePath); os.IsNotExist(err) {
		t.Skipf("Existing scenario file not found: %s", existingFilePath)
	}

	// Load the existing scenario to compare
	loader := NewScenarioLoader(filepath.Dir(existingFilePath))
	existingScenario, err := loader.LoadScenario(existingFilePath)
	if err != nil {
		t.Fatalf("Failed to load existing scenario: %v", err)
	}

	// Build the same scenario using the builder API
	builtScenario := buildAllKindsScenario()

	// Compare key fields (reflect.DeepEqual is too strict for empty slices vs nil)
	if existingScenario.Name != builtScenario.Name {
		t.Errorf("Name mismatch: expected %s, got %s", existingScenario.Name, builtScenario.Name)
	}
	if existingScenario.Description != builtScenario.Description {
		t.Errorf("Description mismatch: expected %s, got %s", existingScenario.Description, builtScenario.Description)
	}
	if existingScenario.ResponseProcessor != builtScenario.ResponseProcessor {
		t.Errorf("ResponseProcessor mismatch: expected %s, got %s", existingScenario.ResponseProcessor, builtScenario.ResponseProcessor)
	}
	if len(existingScenario.Tests) != len(builtScenario.Tests) {
		t.Errorf("Tests count mismatch: expected %d, got %d", len(existingScenario.Tests), len(builtScenario.Tests))
	}
	// Compare each test step
	for i := 0; i < len(existingScenario.Tests) && i < len(builtScenario.Tests); i++ {
		existingStep := existingScenario.Tests[i]
		builtStep := builtScenario.Tests[i]
		if existingStep.Name != builtStep.Name {
			t.Errorf("Test %d name mismatch: expected %s, got %s", i, existingStep.Name, builtStep.Name)
		}
		// Normalize tool names for comparison (legacy zqk_ prefix vs current brand)
		existingToolNormalized := normalizeToolName(existingStep.Tool)
		builtToolNormalized := normalizeToolName(builtStep.Tool)
		if existingToolNormalized != builtToolNormalized {
			t.Errorf("Test %d tool mismatch: expected %s (normalized from %s), got %s (normalized from %s)",
				i, existingToolNormalized, existingStep.Tool, builtToolNormalized, builtStep.Tool)
		}
		if !reflect.DeepEqual(existingStep.Args, builtStep.Args) {
			t.Errorf("Test %d args mismatch: expected %v, got %v", i, existingStep.Args, builtStep.Args)
		}
		if existingStep.Expected != nil && builtStep.Expected != nil {
			if existingStep.Expected.Success != nil && builtStep.Expected.Success != nil {
				if *existingStep.Expected.Success != *builtStep.Expected.Success {
					t.Errorf("Test %d expected success mismatch: expected %v, got %v", i, *existingStep.Expected.Success, *builtStep.Expected.Success)
				}
			}
		}
	}

	// Also test that the built scenario can be written to YAML and loaded back
	writer := NewScenarioWriter()
	yamlStr, err := writer.WriteString(builtScenario)
	if err != nil {
		t.Fatalf("Failed to write built scenario to YAML: %v", err)
	}

	// Write to temporary file and load it back to verify round-trip
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test-all-kinds.yaml")
	if err := writer.WriteToFile(builtScenario, tmpFile); err != nil {
		t.Fatalf("Failed to write scenario to file: %v", err)
	}

	// Load it back
	loadedScenario, err := loader.LoadScenario(tmpFile)
	if err != nil {
		t.Fatalf("Failed to load written scenario: %v", err)
	}

	// Compare key fields (round-trip may have empty slices vs nil differences)
	if builtScenario.Name != loadedScenario.Name {
		t.Errorf("Round-trip name mismatch: expected %s, got %s", builtScenario.Name, loadedScenario.Name)
	}
	if builtScenario.Description != loadedScenario.Description {
		t.Errorf("Round-trip description mismatch: expected %s, got %s", builtScenario.Description, loadedScenario.Description)
	}
	if builtScenario.ResponseProcessor != loadedScenario.ResponseProcessor {
		t.Errorf("Round-trip response_processor mismatch: expected %s, got %s", builtScenario.ResponseProcessor, loadedScenario.ResponseProcessor)
	}
	if len(builtScenario.Tests) != len(loadedScenario.Tests) {
		t.Errorf("Round-trip tests count mismatch: expected %d, got %d", len(builtScenario.Tests), len(loadedScenario.Tests))
	}

	// Verify YAML structure is valid by parsing it
	var yamlData map[string]any
	if err := yaml.Unmarshal([]byte(yamlStr), &yamlData); err != nil {
		t.Fatalf("Generated YAML is not valid: %v", err)
	}

	// Verify key fields are present
	if yamlData[objects.FieldKeyName] != existingScenario.Name {
		t.Errorf("YAML name mismatch: expected %s, got %v", existingScenario.Name, yamlData[objects.FieldKeyName])
	}
	if yamlData[objects.FieldKeyDescription] != existingScenario.Description {
		t.Errorf("YAML description mismatch: expected %s, got %v", existingScenario.Description, yamlData[objects.FieldKeyDescription])
	}
	if yamlData["response_processor"] != existingScenario.ResponseProcessor {
		t.Errorf("YAML response_processor mismatch: expected %s, got %v", existingScenario.ResponseProcessor, yamlData["response_processor"])
	}
}

// buildAllKindsScenario builds the all-kinds scenario using the builder API
func buildAllKindsScenario() *TestScenario {
	// Define object kinds to test
	kinds := []string{
		"backlog_item",
		"milestone",
		"goal",
		"requirement",
		"criteria",
	}

	// Build scenario using builder API
	builder := NewScenarioBuilder().
		Name("Interactive Creation - All Kinds").
		Description("Test creating one object of each kind using interactive creation tool").
		ResponseProcessor("noop") // Use noop since tool succeeds, not elicitation

	// Add test steps for each kind
	for _, kind := range kinds {
		step := NewStepBuilder().
			Name("Create "+kind+" - verify tool works").
			Tool(mcp.GetToolName("create_object_interactive")).
			Arg("kind", kind).
			Expected(ExpectSuccess()).
			Build()
		builder.AddTestStep(step)
	}

	return builder.Build()
}

// TestBuildAllKindsScenario_Structure validates the structure of the built scenario
func TestBuildAllKindsScenario_Structure(t *testing.T) {
	t.Parallel()
	scenario := buildAllKindsScenario()

	// Validate scenario structure
	if scenario.Name != "Interactive Creation - All Kinds" {
		t.Errorf("Expected scenario name 'Interactive Creation - All Kinds', got %s", scenario.Name)
	}

	if scenario.Description != "Test creating one object of each kind using interactive creation tool" {
		t.Errorf("Expected specific description, got %s", scenario.Description)
	}

	if scenario.ResponseProcessor != "noop" {
		t.Errorf("Expected response_processor 'noop', got %s", scenario.ResponseProcessor)
	}

	// Validate test steps
	expectedKinds := []string{"backlog_item", "milestone", "goal", "requirement", "criteria"}
	if len(scenario.Tests) != len(expectedKinds) {
		t.Fatalf("Expected %d test steps, got %d", len(expectedKinds), len(scenario.Tests))
	}

	for i, kind := range expectedKinds {
		step := scenario.Tests[i]
		expectedName := "Create " + kind + " - verify tool works"
		if step.Name != expectedName {
			t.Errorf("Step %d: expected name '%s', got '%s'", i, expectedName, step.Name)
		}
		expectedTool := mcp.GetToolName("create_object_interactive")
		if step.Tool != expectedTool {
			t.Errorf("Step %d: expected tool '%s', got '%s'", i, expectedTool, step.Tool)
		}
		if step.Args[objects.FieldKeyKind] != kind {
			t.Errorf("Step %d: expected arg kind='%s', got '%v'", i, kind, step.Args[objects.FieldKeyKind])
		}
		if step.Expected == nil {
			t.Errorf("Step %d: expected expectation to be set", i)
		} else if step.Expected.Success == nil || !*step.Expected.Success {
			t.Errorf("Step %d: expected success to be true", i)
		}
	}

	// Validate no setup or cleanup steps (as per the file)
	if len(scenario.Setup) != 0 {
		t.Errorf("Expected 0 setup steps, got %d", len(scenario.Setup))
	}
	if len(scenario.Cleanup) != 0 {
		t.Errorf("Expected 0 cleanup steps, got %d", len(scenario.Cleanup))
	}
}

// TestBuildAllKindsScenario_YAMLOutput validates the YAML output format
func TestBuildAllKindsScenario_YAMLOutput(t *testing.T) {
	t.Parallel()
	scenario := buildAllKindsScenario()
	writer := NewScenarioWriter()

	// Test writing to string
	yamlStr, err := writer.WriteString(scenario)
	if err != nil {
		t.Fatalf("Failed to write scenario to YAML string: %v", err)
	}

	// Verify YAML contains expected content
	if !contains(yamlStr, "name:") {
		t.Error("YAML should contain 'name:'")
	}
	if !contains(yamlStr, "Interactive Creation - All Kinds") {
		t.Error("YAML should contain scenario name")
	}
	if !contains(yamlStr, "response_processor:") {
		t.Error("YAML should contain 'response_processor:'")
	}
	if !contains(yamlStr, "noop") {
		t.Error("YAML should contain 'noop'")
	}
	if !contains(yamlStr, "tests:") {
		t.Error("YAML should contain 'tests:'")
	}
	if !contains(yamlStr, "create_object_interactive") {
		t.Error("YAML should contain tool name")
	}

	// Verify all kinds are present in YAML
	expectedKinds := []string{"backlog_item", "milestone", "goal", "requirement", "criteria"}
	for _, kind := range expectedKinds {
		if !strings.Contains(yamlStr, kind) {
			t.Errorf("YAML should contain kind '%s'", kind)
		}
	}
}
