package testing

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/mcp"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestScenarioBuilder_Basic(t *testing.T) {
	t.Parallel()
	scenario := NewScenarioBuilder().
		Name("Test Scenario").
		Description("A test scenario").
		ResponseProcessor("noop").
		Build()

	if scenario.Name != "Test Scenario" {
		t.Errorf("Expected name 'Test Scenario', got %s", scenario.Name)
	}
	if scenario.Description != "A test scenario" {
		t.Errorf("Expected description 'A test scenario', got %s", scenario.Description)
	}
	if scenario.ResponseProcessor != "noop" {
		t.Errorf("Expected response processor 'noop', got %s", scenario.ResponseProcessor)
	}
}

func TestScenarioBuilder_WithSteps(t *testing.T) {
	t.Parallel()
	setupStep := NewStepBuilder().
		Name("Setup Step").
		Tool("test_echo").
		Arg("message", "setup").
		Build()

	testStep := NewStepBuilder().
		Name("Test Step").
		Tool("test_echo").
		Arg("message", "test").
		Expected(ExpectSuccess()).
		Build()

	cleanupStep := NewStepBuilder().
		Name("Cleanup Step").
		Tool("test_echo").
		Arg("message", "cleanup").
		Build()

	scenario := NewScenarioBuilder().
		Name("Test With Steps").
		AddSetupStep(setupStep).
		AddTestStep(testStep).
		AddCleanupStep(cleanupStep).
		Build()

	if len(scenario.Setup) != 1 {
		t.Errorf("Expected 1 setup step, got %d", len(scenario.Setup))
	}
	if len(scenario.Tests) != 1 {
		t.Errorf("Expected 1 test step, got %d", len(scenario.Tests))
	}
	if len(scenario.Cleanup) != 1 {
		t.Errorf("Expected 1 cleanup step, got %d", len(scenario.Cleanup))
	}

	if scenario.Setup[0].Name != "Setup Step" {
		t.Errorf("Expected setup step name 'Setup Step', got %s", scenario.Setup[0].Name)
	}
	if scenario.Tests[0].Name != "Test Step" {
		t.Errorf("Expected test step name 'Test Step', got %s", scenario.Tests[0].Name)
	}
}

func TestStepBuilder_Complex(t *testing.T) {
	t.Parallel()
	step := NewStepBuilder().
		Name("Complex Step").
		Description("A complex step with many options").
		Tool(mcp.GetToolName("object_create")).
		Arg("kind", "backlog_item").
		Arg("title", "Test Item").
		Arg("status", "exploring").
		StoreResult("created_item").
		DependsOn("milestone").
		Expected(NewExpectationBuilder().
			Success(true).
			HasField("id", nil).
			HasFields("id", "title", "status").
			Matches("id", "^BLI-\\d+$").
			Equals("status", "exploring").
			Build()).
		Build()

	if step.Name != "Complex Step" {
		t.Errorf("Expected name 'Complex Step', got %s", step.Name)
	}
	if step.Tool != mcp.GetToolName("object_create") {
		t.Errorf("Expected tool '%s', got %s", mcp.GetToolName("object_create"), step.Tool)
	}
	if step.StoreResult != "created_item" {
		t.Errorf("Expected store result 'created_item', got %s", step.StoreResult)
	}
	if len(step.DependsOn) != 1 || step.DependsOn[0] != "milestone" {
		t.Errorf("Expected dependency 'milestone', got %v", step.DependsOn)
	}
	if step.Expected == nil {
		t.Fatal("Expected expectation to be set")
	}
	if step.Expected.Success == nil || !*step.Expected.Success {
		t.Error("Expected success to be true")
	}
}

func TestExpectationBuilder_Complete(t *testing.T) {
	t.Parallel()
	exp := NewExpectationBuilder().
		Success(true).
		HasField("id", nil).
		HasFields("title", "status").
		NotHasFields("deleted", "archived").
		Matches("id", "^BLI-\\d+$").
		Equals("status", "exploring").
		Build()

	if exp.Success == nil || !*exp.Success {
		t.Error("Expected success to be true")
	}
	if len(exp.HasField) != 1 {
		t.Errorf("Expected 1 has_field, got %d", len(exp.HasField))
	}
	if len(exp.HasFields) != 2 {
		t.Errorf("Expected 2 has_fields, got %d", len(exp.HasFields))
	}
	if len(exp.Matches) != 1 {
		t.Errorf("Expected 1 match, got %d", len(exp.Matches))
	}
	if len(exp.Equals) != 1 {
		t.Errorf("Expected 1 equals, got %d", len(exp.Equals))
	}
}

func TestErrorExpectationBuilder(t *testing.T) {
	t.Parallel()
	code := -32602
	errExp := NewErrorExpectationBuilder().
		Code(code).
		Message("Invalid.*parameters").
		Type("elicitation").
		Build()

	if errExp.Code == nil || *errExp.Code != code {
		t.Errorf("Expected code %d, got %v", code, errExp.Code)
	}
	if errExp.Message != "Invalid.*parameters" {
		t.Errorf("Expected message 'Invalid.*parameters', got %s", errExp.Message)
	}
	if errExp.Type != "elicitation" {
		t.Errorf("Expected type 'elicitation', got %s", errExp.Type)
	}
}

func TestConvenienceFunctions(t *testing.T) {
	t.Parallel()
	// Test SimpleStep
	step := SimpleStep("Simple", mcp.GetToolName("test_echo"))
	if step.Name != "Simple" {
		t.Errorf("Expected name 'Simple', got %s", step.Name)
	}
	if step.Tool != mcp.GetToolName("test_echo") {
		t.Errorf("Expected tool '%s', got %s", mcp.GetToolName("test_echo"), step.Tool)
	}

	// Test StepWithArgs
	step = StepWithArgs("With Args", mcp.GetToolName("test_echo"), map[string]any{
		"message": "test",
	})
	if step.Name != "With Args" {
		t.Errorf("Expected name 'With Args', got %s", step.Name)
	}
	if step.Args["message"] != "test" {
		t.Errorf("Expected arg 'message'='test', got %v", step.Args["message"])
	}

	// Test ExpectSuccess
	exp := ExpectSuccess()
	if exp.Success == nil || !*exp.Success {
		t.Error("Expected success to be true")
	}

	// Test ExpectFailure
	exp = ExpectFailure()
	if exp.Success == nil || *exp.Success {
		t.Error("Expected success to be false")
	}

	// Test ExpectElicitation
	exp = ExpectElicitation()
	if exp.Error == nil || exp.Error.Type != "elicitation" {
		t.Error("Expected elicitation error")
	}
}

func TestScenarioBuilder_Integration(t *testing.T) {
	t.Parallel()
	// Build a complete scenario using the builder
	scenario := NewScenarioBuilder().
		Name("Integration Test Scenario").
		Description("A complete integration test").
		ResponseProcessor("elicitation_to_success").
		AddImport("common/setup.yaml").
		AddSetupStep(NewStepBuilder().
			Name("Setup").
			Tool("test_echo").
			Arg("message", "setup").
			Build()).
		AddTestStep(NewStepBuilder().
			Name("Test 1").
			Tool(mcp.GetToolName("create_object_interactive")).
			Arg("kind", "backlog_item").
			Expected(ExpectSuccess()).
			StoreResult("item1").
			Build()).
		AddTestStep(NewStepBuilder().
			Name("Test 2").
			Tool("object_get").
			Arg("id", "${item1.id}").
			DependsOn("item1").
			Expected(NewExpectationBuilder().
				Success(true).
				HasFields("id", "title").
				Build()).
			Build()).
		AddCleanupStep(SimpleStep("Cleanup", "test_echo")).
		Build()

	// Verify structure
	if scenario.Name != "Integration Test Scenario" {
		t.Errorf("Expected name 'Integration Test Scenario', got %s", scenario.Name)
	}
	if len(scenario.Imports) != 1 {
		t.Errorf("Expected 1 import, got %d", len(scenario.Imports))
	}
	if len(scenario.Setup) != 1 {
		t.Errorf("Expected 1 setup step, got %d", len(scenario.Setup))
	}
	if len(scenario.Tests) != 2 {
		t.Errorf("Expected 2 test steps, got %d", len(scenario.Tests))
	}
	if len(scenario.Cleanup) != 1 {
		t.Errorf("Expected 1 cleanup step, got %d", len(scenario.Cleanup))
	}
}

func TestScenarioWriter_WriteToFile(t *testing.T) {
	t.Parallel()
	// Create a test scenario
	scenario := NewScenarioBuilder().
		Name("Writer Test").
		Description("Test scenario writer").
		AddTestStep(SimpleStep("Test", "zqk_test_echo")).
		Build()

	// Write to temporary file
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test-scenario.yaml")

	writer := NewScenarioWriter()
	if err := writer.WriteToFile(scenario, filePath); err != nil {
		t.Fatalf("Failed to write scenario: %v", err)
	}

	// Verify file exists
	if _, err := fileutil.Stat(filePath); fileutil.IsNotExist(err) {
		t.Fatalf("File was not created: %s", filePath)
	}

	// Load and verify
	loader := NewScenarioLoader(tmpDir)
	loaded, err := loader.LoadScenario(filePath)
	if err != nil {
		t.Fatalf("Failed to load scenario: %v", err)
	}

	if loaded.Name != scenario.Name {
		t.Errorf("Expected name '%s', got '%s'", scenario.Name, loaded.Name)
	}
	if len(loaded.Tests) != 1 {
		t.Errorf("Expected 1 test, got %d", len(loaded.Tests))
	}
}

func TestScenarioWriter_WriteToString(t *testing.T) {
	t.Parallel()
	scenario := NewScenarioBuilder().
		Name("String Test").
		AddTestStep(SimpleStep("Test", "zqk_test_echo")).
		Build()

	writer := NewScenarioWriter()
	yamlStr, err := writer.WriteString(scenario)
	if err != nil {
		t.Fatalf("Failed to write scenario to string: %v", err)
	}

	if yamlStr == emptyValue {
		t.Error("Expected non-empty YAML string")
	}

	// Verify it contains expected content
	if !contains(yamlStr, "name:") {
		t.Error("Expected YAML to contain 'name:'")
	}
	if !contains(yamlStr, "String Test") {
		t.Error("Expected YAML to contain scenario name")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
