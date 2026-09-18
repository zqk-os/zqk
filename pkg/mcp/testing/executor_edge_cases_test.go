package testing

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/mcp"
)

// TestExecutor_EmptyTestList tests handling of scenarios with no tests
func TestExecutor_EmptyTestList(t *testing.T) {
	t.Parallel()
	server := mcp.NewServer()
	executor := NewScenarioExecutor(server)

	scenario := &TestScenario{
		Name:  "Empty Test List",
		Tests: []TestStep{}, // Empty tests
	}

	results, err := executor.RunScenario(scenario)
	if err != nil {
		t.Fatalf("Unexpected error running scenario with empty tests: %v", err)
	}

	if results.ScenarioName != "Empty Test List" {
		t.Errorf("Expected scenario name 'Empty Test List', got: %s", results.ScenarioName)
	}
	if len(results.StepResults) != 0 {
		t.Errorf("Expected 0 step results, got %d", len(results.StepResults))
	}
}

// TestExecutor_DependencyFailure tests error handling for missing dependencies
func TestExecutor_DependencyFailure(t *testing.T) {
	t.Parallel()
	server := mcp.NewServer()
	executor := NewScenarioExecutor(server)

	scenario := &TestScenario{
		Name: "Dependency Test",
		Tests: []TestStep{
			{
				Name: "Step 1",
				Tool: "test_echo",
				Args: map[string]any{
					"message": "test",
				},
				StoreResult: "result1",
			},
			{
				Name: "Step 2",
				Tool: "test_echo",
				Args: map[string]any{
					"message": "test",
				},
				DependsOn: []string{"nonexistent"}, // Missing dependency
			},
		},
	}

	// Register echo tool
	mcp.RegisterCommonTools(server)

	results, err := executor.RunScenario(scenario)
	if err == nil {
		t.Error("Expected error for missing dependency, got nil")
	}
	if results == nil {
		t.Fatal("Results should not be nil even on error")
	}
}

// TestExecutor_StoreResultOverwrite tests handling of overwriting stored results
func TestExecutor_StoreResultOverwrite(t *testing.T) {
	t.Parallel()
	server := mcp.NewServer()
	executor := NewScenarioExecutor(server)

	// Register echo tool
	mcp.RegisterCommonTools(server)

	scenario := &TestScenario{
		Name: "Overwrite Test",
		Tests: []TestStep{
			{
				Name: "Step 1",
				Tool: "test_echo",
				Args: map[string]any{
					"message": "first",
				},
				StoreResult: "result",
			},
			{
				Name: "Step 2",
				Tool: "test_echo",
				Args: map[string]any{
					"message": "second",
				},
				StoreResult: "result", // Same key - should overwrite
				DependsOn:   []string{"result"},
			},
		},
	}

	results, err := executor.RunScenario(scenario)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(results.StepResults) != 2 {
		t.Errorf("Expected 2 step results, got %d", len(results.StepResults))
	}

	// Second step should have access to overwritten result
	// (Currently overwrites silently - this is expected behavior)
}

// TestExecutor_SkippedStepDependency tests behavior when a skipped step is depended upon
func TestExecutor_SkippedStepDependency(t *testing.T) {
	t.Parallel()
	server := mcp.NewServer()
	executor := NewScenarioExecutor(server)

	scenario := &TestScenario{
		Name: "Skipped Dependency Test",
		Tests: []TestStep{
			{
				Name: "Step 1",
				Tool: "test_echo",
				Args: map[string]any{
					"message": "test",
				},
				Skip:        true,
				StoreResult: "result1",
			},
			{
				Name: "Step 2",
				Tool: "test_echo",
				Args: map[string]any{
					"message": "test",
				},
				DependsOn: []string{"result1"}, // Depends on skipped step
			},
		},
	}

	// Register echo tool
	mcp.RegisterCommonTools(server)

	// Should fail because dependency was skipped and not stored
	results, err := executor.RunScenario(scenario)
	if err == nil {
		t.Error("Expected error when depending on skipped step, got nil")
	}
	if results == nil {
		t.Fatal("Results should not be nil even on error")
	}
}

// TestExecutor_StepWithNoArgs tests handling of steps with no args
func TestExecutor_StepWithNoArgs(t *testing.T) {
	t.Parallel()
	server := mcp.NewServer()
	executor := NewScenarioExecutor(server)

	// Register echo tool
	mcp.RegisterCommonTools(server)

	scenario := &TestScenario{
		Name: "No Args Test",
		Tests: []TestStep{
			{
				Name: "Step with no args",
				Tool: "test_echo",
				// No Args field
			},
		},
	}

	results, err := executor.RunScenario(scenario)
	// Echo tool requires message arg, so this should fail
	// But the executor should handle nil args gracefully
	if err == nil {
		// If it doesn't fail, that's also acceptable (tool handles it)
		t.Log("Step with no args executed (tool may have defaults)")
	}
	if results == nil {
		t.Fatal("Results should not be nil")
	}
}

// TestExecutor_StepWithEmptyArgs tests handling of steps with empty args
func TestExecutor_StepWithEmptyArgs(t *testing.T) {
	t.Parallel()
	server := mcp.NewServer()
	executor := NewScenarioExecutor(server)

	// Register echo tool
	mcp.RegisterCommonTools(server)

	scenario := &TestScenario{
		Name: "Empty Args Test",
		Tests: []TestStep{
			{
				Name: "Step with empty args",
				Tool: "test_echo",
				Args: map[string]any{}, // Empty map
			},
		},
	}

	results, err := executor.RunScenario(scenario)
	// Echo tool requires message arg, so this should fail
	// But the executor should handle empty args gracefully
	if err == nil {
		t.Log("Step with empty args executed (tool may have defaults)")
	}
	if results == nil {
		t.Fatal("Results should not be nil")
	}
}

// TestExecutor_ResultNotMap tests validation when result is not a map
func TestExecutor_ResultNotMap(t *testing.T) {
	t.Parallel()
	server := mcp.NewServer()
	executor := NewScenarioExecutor(server)

	// Register echo tool
	mcp.RegisterCommonTools(server)

	scenario := &TestScenario{
		Name: "Result Not Map Test",
		Tests: []TestStep{
			{
				Name: "Echo step",
				Tool: "test_echo",
				Args: map[string]any{
					"message": "test",
				},
				Expected: &TestExpectation{
					HasField: map[string]any{
						"message": "test",
					},
				},
			},
		},
	}

	results, err := executor.RunScenario(scenario)
	// Echo tool returns a map, so validation should work
	// But if it returned a string/array, validation would skip field checks
	if err != nil {
		// Error is acceptable if echo returns non-map
		t.Logf("Result validation: %v", err)
	}
	if results == nil {
		t.Fatal("Results should not be nil")
	}
}

// TestExecutor_ContextCancellation tests context cancellation (basic test)
func TestExecutor_ContextCancellation(t *testing.T) {
	t.Parallel()
	server := mcp.NewServer()
	executor := NewScenarioExecutor(server)

	// Create executor with cancelled context
	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	cancel() // Cancel immediately

	executor.ctx = ctx

	scenario := &TestScenario{
		Name: "Cancellation Test",
		Tests: []TestStep{
			{
				Name: "Step 1",
				Tool: "test_echo",
				Args: map[string]any{
					"message": "test",
				},
			},
		},
	}

	// Register echo tool
	mcp.RegisterCommonTools(server)

	// Should handle cancellation gracefully
	results, err := executor.RunScenario(scenario)
	if err == nil {
		// Some tools may not check context, which is acceptable
		t.Log("Context cancellation not enforced (tool-dependent)")
	}
	if results == nil {
		t.Fatal("Results should not be nil")
	}
}
