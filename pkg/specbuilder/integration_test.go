package specbuilder_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"

	mcptesting "github.com/lanceman/zqk/pkg/mcp/testing"
	sbcore "github.com/lanceman/zqk/pkg/specbuilder/core"
	sbyaml "github.com/lanceman/zqk/pkg/specbuilder/yaml"
)

const emptyValue = ""

// TestScenarioSpec adapts mcptesting.ScenarioSpec to implement core.Spec
type TestScenarioSpec mcptesting.ScenarioSpec

func (s TestScenarioSpec) Validate() error {
	if s.Name == emptyValue {
		return &ValidationError{Field: "name", Message: "name is required"}
	}
	if len(s.Tests) == 0 {
		return &ValidationError{Field: "tests", Message: "at least one test is required"}
	}
	return nil
}

func (s TestScenarioSpec) GetName() string {
	return s.Name
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// TestScenarioBuilderFactory creates scenario builders from specs
type TestScenarioBuilderFactory struct{}

func (f *TestScenarioBuilderFactory) CreateBuilder(spec TestScenarioSpec) sbcore.Builder[*mcptesting.TestScenario] {
	// Use the existing scenario builder API
	builder := mcptesting.NewScenarioBuilder().
		Name(spec.Name)

	if spec.Description != emptyValue {
		builder.Description(spec.Description)
	}

	if spec.ResponseProcessor != emptyValue {
		builder.ResponseProcessor(spec.ResponseProcessor)
	}

	// Add imports
	for _, importPath := range spec.Imports {
		builder.AddImport(importPath)
	}

	// Add setup steps
	for _, step := range spec.Setup {
		stepCopy := step
		builder.AddSetupStep(&stepCopy)
	}

	// Add test steps
	for _, step := range spec.Tests {
		stepCopy := step
		builder.AddTestStep(&stepCopy)
	}

	// Add cleanup steps
	for _, step := range spec.Cleanup {
		stepCopy := step
		builder.AddCleanupStep(&stepCopy)
	}

	// Wrap the builder to implement core.Builder
	return &scenarioBuilderWrapper{builder: builder}
}

// scenarioBuilderWrapper wraps mcptesting.ScenarioBuilder to implement core.Builder
type scenarioBuilderWrapper struct {
	builder *mcptesting.ScenarioBuilder
}

func (w *scenarioBuilderWrapper) Build() *mcptesting.TestScenario {
	return w.builder.Build()
}

// TestScenarioGenerator uses specbuilder core to generate scenarios
type TestScenarioGenerator struct {
	*sbcore.BaseGenerator[TestScenarioSpec, *mcptesting.TestScenario]
}

func NewTestScenarioGenerator(outputDir string) *TestScenarioGenerator {
	factory := &TestScenarioBuilderFactory{}
	writer := sbyaml.NewYAMLWriter[*mcptesting.TestScenario]()
	base := sbcore.NewBaseGenerator(factory, writer, outputDir)
	return &TestScenarioGenerator{BaseGenerator: base}
}

// GenerateFromFile loads specs from a YAML file and generates scenarios
func (g *TestScenarioGenerator) GenerateFromFile(filePath string) error {
	specs, err := sbyaml.LoadYAMLSpecList[TestScenarioSpec](filePath, "scenarios")
	if err != nil {
		return err
	}
	return g.GenerateAndWriteFromSpecs(specs)
}

// TestSpecBuilderIntegration tests the full specbuilder pattern with test scenarios
func TestSpecBuilderIntegration(t *testing.T) {
	t.Parallel()
	// Create temporary output directory
	tmpDir := t.TempDir()

	// Create a test spec file
	specFile := filepath.Join(tmpDir, "test-scenarios.yaml")
	specContent := `scenarios:
  - name: "Integration Test Scenario"
    description: "Test scenario generated using specbuilder core"
    response_processor: "noop"
    tests:
      - name: "Test step"
        tool: "zqk_test_echo"
        args:
          message: "test"
        expected:
          success: true
`

	if err := os.WriteFile(specFile, []byte(specContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write spec file: %v", err)
	}

	// Create generator using specbuilder core
	outputDir := filepath.Join(tmpDir, "generated")
	if err := os.MkdirAll(outputDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create output directory: %v", err)
	}

	generator := NewTestScenarioGenerator(outputDir)

	// Generate scenarios from file
	if err := generator.GenerateFromFile(specFile); err != nil {
		t.Fatalf("Failed to generate scenarios: %v", err)
	}

	// Verify file was created
	expectedFile := "Integration_Test_Scenario.yaml"
	filePath := filepath.Join(outputDir, expectedFile)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatalf("Expected file %s was not created", expectedFile)
	}

	// Load and verify the generated scenario
	loader := mcptesting.NewScenarioLoader(outputDir)
	scenario, err := loader.LoadScenario(filePath)
	if err != nil {
		t.Fatalf("Failed to load generated scenario: %v", err)
	}

	// Verify scenario content
	if scenario.Name != "Integration Test Scenario" {
		t.Errorf("Expected name 'Integration Test Scenario', got '%s'", scenario.Name)
	}
	if scenario.Description != "Test scenario generated using specbuilder core" {
		t.Errorf("Expected description 'Test scenario generated using specbuilder core', got '%s'", scenario.Description)
	}
	if scenario.ResponseProcessor != "noop" {
		t.Errorf("Expected response_processor 'noop', got '%s'", scenario.ResponseProcessor)
	}
	if len(scenario.Tests) != 1 {
		t.Errorf("Expected 1 test step, got %d", len(scenario.Tests))
	}
	if len(scenario.Tests) > 0 {
		if scenario.Tests[0].Name != "Test step" {
			t.Errorf("Expected test step name 'Test step', got '%s'", scenario.Tests[0].Name)
		}
		if scenario.Tests[0].Tool != "zqk_test_echo" {
			t.Errorf("Expected tool 'zqk_test_echo', got '%s'", scenario.Tests[0].Tool)
		}
	}
}

// TestSpecBuilderWithExistingScenarios tests generating from the actual scenarios.yaml file
func TestSpecBuilderWithExistingScenarios(t *testing.T) {
	t.Parallel()
	// Find the test-scenarios/scenarios.yaml file
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	projectRoot := filepath.Join(cwd, "..", "..", "..")
	scenariosFile := filepath.Join(projectRoot, "test-scenarios", "scenarios.yaml")

	// Skip if file doesn't exist
	if _, err := os.Stat(scenariosFile); os.IsNotExist(err) {
		t.Skipf("Scenarios file not found: %s", scenariosFile)
	}

	// Create temporary output directory
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "generated")
	if err := os.MkdirAll(outputDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create output directory: %v", err)
	}

	// Create generator using specbuilder core
	generator := NewTestScenarioGenerator(outputDir)

	// Generate scenarios from file
	if err := generator.GenerateFromFile(scenariosFile); err != nil {
		t.Fatalf("Failed to generate scenarios: %v", err)
	}

	// Load the original specs to compare
	originalSpecs, err := sbyaml.LoadYAMLSpecList[TestScenarioSpec](scenariosFile, "scenarios")
	if err != nil {
		t.Fatalf("Failed to load original specs: %v", err)
	}

	// Verify all scenarios were generated
	generatedFiles, err := filepath.Glob(filepath.Join(outputDir, "*.yaml"))
	if err != nil {
		t.Fatalf("Failed to list generated files: %v", err)
	}

	if len(generatedFiles) != len(originalSpecs) {
		t.Errorf("Expected %d generated files, got %d", len(originalSpecs), len(generatedFiles))
	}

	// Load and verify each generated scenario matches the spec
	loader := mcptesting.NewScenarioLoader(outputDir)
	for i, spec := range originalSpecs {
		if i >= len(generatedFiles) {
			break
		}

		scenario, err := loader.LoadScenario(generatedFiles[i])
		if err != nil {
			t.Errorf("Failed to load generated scenario %d: %v", i, err)
			continue
		}

		// Verify key fields match
		if scenario.Name != spec.Name {
			t.Errorf("Scenario %d: Expected name '%s', got '%s'", i, spec.Name, scenario.Name)
		}
		if scenario.Description != spec.Description {
			t.Errorf("Scenario %d: Expected description '%s', got '%s'", i, spec.Description, scenario.Description)
		}
		if scenario.ResponseProcessor != spec.ResponseProcessor {
			t.Errorf("Scenario %d: Expected response_processor '%s', got '%s'", i, spec.ResponseProcessor, scenario.ResponseProcessor)
		}
		if len(scenario.Tests) != len(spec.Tests) {
			t.Errorf("Scenario %d: Expected %d test steps, got %d", i, len(spec.Tests), len(scenario.Tests))
		}
	}
}

// TestBaseGeneratorFunctionality tests the BaseGenerator directly
func TestBaseGeneratorFunctionality(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "generated")
	if err := os.MkdirAll(outputDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create output directory: %v", err)
	}

	factory := &TestScenarioBuilderFactory{}
	writer := sbyaml.NewYAMLWriter[*mcptesting.TestScenario]()
	base := sbcore.NewBaseGenerator(factory, writer, outputDir)

	// Create a test spec
	spec := TestScenarioSpec{
		Name:        "Base Generator Test",
		Description: "Testing BaseGenerator functionality",
		Tests: []mcptesting.TestStep{
			{
				Name: "Test step",
				Tool: "zqk_test_echo",
				Args: map[string]any{
					"message": "test",
				},
			},
		},
	}

	// Generate artifact
	artifact, err := base.GenerateFromSpec(spec)
	if err != nil {
		t.Fatalf("Failed to generate artifact: %v", err)
	}

	// Verify artifact
	if artifact.Name != spec.Name {
		t.Errorf("Expected artifact name '%s', got '%s'", spec.Name, artifact.Name)
	}

	// Test GenerateAndWriteFromSpec
	if err := base.GenerateAndWriteFromSpec(spec, ""); err != nil {
		t.Fatalf("Failed to generate and write: %v", err)
	}

	// Verify file was created
	expectedFile := "Base_Generator_Test.yaml"
	filePath := filepath.Join(outputDir, expectedFile)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatalf("Expected file %s was not created", expectedFile)
	}

	// Test filename sanitization
	filename := sbcore.SanitizeFilename("Test Scenario - With Dashes")
	expected := "Test_Scenario_With_Dashes"
	if filename != expected {
		t.Errorf("Expected filename '%s', got '%s'", expected, filename)
	}
}

// TestSpecValidation tests spec validation
func TestSpecValidation(t *testing.T) {
	t.Parallel()
	// Test valid spec
	spec := TestScenarioSpec{
		Name: "Valid Spec",
		Tests: []mcptesting.TestStep{
			{Name: "Test", Tool: "test"},
		},
	}
	if err := spec.Validate(); err != nil {
		t.Errorf("Expected valid spec to pass validation, got error: %v", err)
	}

	// Test invalid spec (empty name)
	invalidSpec := TestScenarioSpec{
		Name:  "",
		Tests: []mcptesting.TestStep{{Name: "Test", Tool: "test"}},
	}
	if err := invalidSpec.Validate(); err == nil {
		t.Error("Expected validation error for empty name")
	}

	// Test invalid spec (no tests)
	invalidSpec2 := TestScenarioSpec{
		Name:  "No Tests",
		Tests: []mcptesting.TestStep{},
	}
	if err := invalidSpec2.Validate(); err == nil {
		t.Error("Expected validation error for no tests")
	}
}

// TestYAMLLoader tests the YAML loader utilities
func TestYAMLLoader(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	specFile := filepath.Join(tmpDir, "test.yaml")
	specContent := `scenarios:
  - name: "Test Scenario"
    tests:
      - name: "Test"
        tool: "test"
`

	if err := os.WriteFile(specFile, []byte(specContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write spec file: %v", err)
	}

	// Test LoadYAMLSpecList
	specs, err := sbyaml.LoadYAMLSpecList[TestScenarioSpec](specFile, "scenarios")
	if err != nil {
		t.Fatalf("Failed to load specs: %v", err)
	}

	if len(specs) != 1 {
		t.Errorf("Expected 1 spec, got %d", len(specs))
	}

	if len(specs) > 0 {
		if specs[0].Name != "Test Scenario" {
			t.Errorf("Expected spec name 'Test Scenario', got '%s'", specs[0].Name)
		}
	}
}

// TestYAMLWriter tests the YAML writer
func TestYAMLWriter(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	writer := sbyaml.NewYAMLWriter[*mcptesting.TestScenario]()

	scenario := &mcptesting.TestScenario{
		Name:        "Writer Test",
		Description: "Testing YAML writer",
		Tests: []mcptesting.TestStep{
			{Name: "Test", Tool: "test"},
		},
	}

	// Test WriteToString
	yamlStr, err := writer.WriteToString(scenario)
	if err != nil {
		t.Fatalf("Failed to write to string: %v", err)
	}

	if yamlStr == emptyValue {
		t.Error("Expected non-empty YAML string")
	}

	// Test WriteToFile
	filePath := filepath.Join(tmpDir, "test.yaml")
	if err := writer.WriteToFile(scenario, filePath); err != nil {
		t.Fatalf("Failed to write to file: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatalf("File was not created: %s", filePath)
	}

	// Test WriteToBytes
	bytes, err := writer.WriteToBytes(scenario)
	if err != nil {
		t.Fatalf("Failed to write to bytes: %v", err)
	}

	if len(bytes) == 0 {
		t.Error("Expected non-empty bytes")
	}
}
