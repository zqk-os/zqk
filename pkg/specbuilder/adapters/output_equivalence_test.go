package adapters

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	mcptesting "github.com/lanceman/zqk/pkg/mcp/testing"
	sbyaml "github.com/lanceman/zqk/pkg/specbuilder/yaml"
)

// TestOutputEquivalence tests that the adapter produces equivalent output to the existing generator
func TestOutputEquivalence(t *testing.T) {
	t.Parallel()
	// Create temporary directories
	tmpDir := t.TempDir()
	oldDir := filepath.Join(tmpDir, "old")
	newDir := filepath.Join(tmpDir, "new")

	if err := fileutil.MkdirAll(oldDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create old directory: %v", err)
	}
	if err := fileutil.MkdirAll(newDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create new directory: %v", err)
	}

	// Create a test spec
	spec := mcptesting.ScenarioSpec{
		Name:              "Equivalence Test",
		Description:       "Test output equivalence",
		ResponseProcessor: "noop",
		Tests: []mcptesting.TestStep{
			{
				Name: "Test step",
				Tool: "zqk_test_echo",
				Args: map[string]any{
					"message": "test",
				},
				Expected: mcptesting.ExpectSuccess(),
			},
		},
	}

	// Generate using OLD path (existing generator)
	oldGenerator := mcptesting.NewScenarioGenerator(oldDir)
	specs := []mcptesting.ScenarioSpec{spec}
	if err := oldGenerator.GenerateFromSpecs(specs); err != nil {
		t.Fatalf("Failed to generate using old path: %v", err)
	}

	// Generate using NEW path (adapter + specbuilder core)
	specAdapter := NewScenarioSpecAdapter(spec)
	factory := NewScenarioBuilderFactoryAdapter()
	builder := factory.CreateBuilder(specAdapter)
	artifact := builder.Build()

	writer := sbyaml.NewYAMLWriter[*mcptesting.TestScenario]()
	filename := "Equivalence_Test.yaml"
	newFilePath := filepath.Join(newDir, filename)
	if err := writer.WriteToFile(artifact, newFilePath); err != nil {
		t.Fatalf("Failed to write using new path: %v", err)
	}

	// Load both outputs
	loader := mcptesting.NewScenarioLoader(oldDir)
	oldFiles, err := filepath.Glob(filepath.Join(oldDir, "*.yaml"))
	if err != nil {
		t.Fatalf("Failed to list old files: %v", err)
	}
	if len(oldFiles) != 1 {
		t.Fatalf("Expected 1 old file, got %d", len(oldFiles))
	}

	oldScenario, err := loader.LoadScenario(oldFiles[0])
	if err != nil {
		t.Fatalf("Failed to load old scenario: %v", err)
	}

	newScenario, err := loader.LoadScenario(newFilePath)
	if err != nil {
		t.Fatalf("Failed to load new scenario: %v", err)
	}

	// Compare key fields
	if oldScenario.Name != newScenario.Name {
		t.Errorf("Name mismatch: old=%s, new=%s", oldScenario.Name, newScenario.Name)
	}
	if oldScenario.Description != newScenario.Description {
		t.Errorf("Description mismatch: old=%s, new=%s", oldScenario.Description, newScenario.Description)
	}
	if oldScenario.ResponseProcessor != newScenario.ResponseProcessor {
		t.Errorf("ResponseProcessor mismatch: old=%s, new=%s", oldScenario.ResponseProcessor, newScenario.ResponseProcessor)
	}
	if len(oldScenario.Tests) != len(newScenario.Tests) {
		t.Errorf("Tests count mismatch: old=%d, new=%d", len(oldScenario.Tests), len(newScenario.Tests))
	}

	// Compare test steps
	for i := 0; i < len(oldScenario.Tests) && i < len(newScenario.Tests); i++ {
		oldStep := oldScenario.Tests[i]
		newStep := newScenario.Tests[i]

		if oldStep.Name != newStep.Name {
			t.Errorf("Step %d name mismatch: old=%s, new=%s", i, oldStep.Name, newStep.Name)
		}
		if oldStep.Tool != newStep.Tool {
			t.Errorf("Step %d tool mismatch: old=%s, new=%s", i, oldStep.Tool, newStep.Tool)
		}
		if !reflect.DeepEqual(oldStep.Args, newStep.Args) {
			t.Errorf("Step %d args mismatch: old=%v, new=%v", i, oldStep.Args, newStep.Args)
		}
	}
}

// TestAdapterIntegration tests the adapter works with specbuilder core
func TestAdapterIntegration(t *testing.T) {
	t.Parallel()
	spec := mcptesting.ScenarioSpec{
		Name: "Adapter Test",
		Tests: []mcptesting.TestStep{
			{Name: "Test", Tool: "test"},
		},
	}

	// Create adapter
	specAdapter := NewScenarioSpecAdapter(spec)

	// Validate spec
	if err := specAdapter.Validate(); err != nil {
		t.Fatalf("Spec validation failed: %v", err)
	}

	// Create builder using factory
	factory := NewScenarioBuilderFactoryAdapter()
	builder := factory.CreateBuilder(specAdapter)

	// Build scenario
	scenario := builder.Build()

	// Verify scenario
	if scenario.Name != "Adapter Test" {
		t.Errorf("Expected name 'Adapter Test', got '%s'", scenario.Name)
	}
	if len(scenario.Tests) != 1 {
		t.Errorf("Expected 1 test step, got %d", len(scenario.Tests))
	}
	if len(scenario.Tests) > 0 && scenario.Tests[0].Name != "Test" {
		t.Errorf("Expected test step name 'Test', got '%s'", scenario.Tests[0].Name)
	}
}

// TestAdapterValidation tests spec validation through adapter
func TestAdapterValidation(t *testing.T) {
	t.Parallel()
	// Test valid spec
	validSpec := mcptesting.ScenarioSpec{
		Name:  "Valid",
		Tests: []mcptesting.TestStep{{Name: "Test", Tool: "test"}},
	}
	adapter := NewScenarioSpecAdapter(validSpec)
	if err := adapter.Validate(); err != nil {
		t.Errorf("Valid spec should pass validation: %v", err)
	}

	// Test invalid spec (empty name)
	invalidSpec := mcptesting.ScenarioSpec{
		Name:  "",
		Tests: []mcptesting.TestStep{{Name: "Test", Tool: "test"}},
	}
	invalidAdapter := NewScenarioSpecAdapter(invalidSpec)
	if err := invalidAdapter.Validate(); err == nil {
		t.Error("Invalid spec (empty name) should fail validation")
	}

	// Test invalid spec (no tests)
	invalidSpec2 := mcptesting.ScenarioSpec{
		Name:  "No Tests",
		Tests: []mcptesting.TestStep{},
	}
	invalidAdapter2 := NewScenarioSpecAdapter(invalidSpec2)
	if err := invalidAdapter2.Validate(); err == nil {
		t.Error("Invalid spec (no tests) should fail validation")
	}
}
