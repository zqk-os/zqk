package testing

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/mcp"
)

func TestExecutorForTest_Coverage(t *testing.T) {
	t.Parallel()
	server := mcp.NewServer()
	executor := NewScenarioExecutor(server)

	// Test SetContextForTest
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	executor.SetContextForTest(ctx)

	// Test NormalizeToolNameForTest
	toolName := NormalizeToolNameForTest("zqk_execute_bash")
	if toolName == "" {
		t.Errorf("expected non-empty normalized tool name")
	}

	// Test ValidateStepResultForTest
	step := &TestStep{
		Name: "test step",
	}
	result := StepResult{
		StepName: "test step",
		Success:  true,
	}
	err := executor.ValidateStepResultForTest(step, result)
	if err != nil {
		t.Errorf("unexpected error validating step result: %v", err)
	}

	// Test ScenarioLoader mergeScenarios
	loader := NewScenarioLoader(".")
	base := &TestScenario{
		Name: "Base Scenario",
		Tests: []TestStep{
			{Name: "step1"},
		},
		Setup: []TestStep{
			{Name: "setup1"},
		},
		Cleanup: []TestStep{
			{Name: "cleanup1"},
		},
		Variables: map[string]any{
			"key1": "val1",
		},
	}
	imported := &TestScenario{
		Name: "Imported Scenario",
		Tests: []TestStep{
			{Name: "step2"},
		},
		Setup: []TestStep{
			{Name: "setup2"},
		},
		Cleanup: []TestStep{
			{Name: "cleanup2"},
		},
		Variables: map[string]any{
			"key2": "val2",
		},
	}
	if err := loader.mergeScenarios(base, imported); err != nil {
		t.Fatalf("failed to merge scenarios: %v", err)
	}

	if len(base.Tests) != 2 {
		t.Errorf("expected 2 tests, got %d", len(base.Tests))
	}
	if len(base.Setup) != 2 {
		t.Errorf("expected 2 setups, got %d", len(base.Setup))
	}
	if len(base.Cleanup) != 2 {
		t.Errorf("expected 2 cleanups, got %d", len(base.Cleanup))
	}

	// Test mergeAtPath
	stepTarget := &TestStep{
		Name: "step",
		Args: map[string]any{
			"field1": "val1",
		},
	}
	if err := loader.mergeAtPath(stepTarget, "args", map[string]any{"field2": "val2"}); err != nil {
		t.Fatalf("mergeAtPath failed: %v", err)
	}

	// Test RunScenario with unknown processor
	_, errUnknown := executor.RunScenario(&TestScenario{
		Name:              "Bad Proc",
		ResponseProcessor: "nonexistent_processor_id_123",
	})
	if errUnknown == nil {
		t.Error("expected error for unknown processor")
	}

	// Test RunScenario with valid scenario
	server.RegisterTool("test_tool", "desc", nil, func(ctx context.Context, args map[string]any) (any, error) {
		return map[string]any{"status": "ok"}, nil
	})

	scenario := &TestScenario{
		Name: "Full Scenario",
		Setup: []TestStep{
			{Name: "setup_step", Tool: "test_tool", StoreResult: "setup_res"},
		},
		Tests: []TestStep{
			{Name: "skipped_step", Skip: true},
			{Name: "test_step", Tool: "test_tool", DependsOn: []string{"setup_res"}, StoreResult: "test_res"},
			{Name: "dep_fail_step", Tool: "test_tool", DependsOn: []string{"nonexistent_dep"}},
		},
		Cleanup: []TestStep{
			{Name: "cleanup_step", Tool: "test_tool"},
		},
	}
	_, _ = executor.RunScenario(scenario)

	// Test NewScenarioExecutorWithProcessor
	procCfg := NewDefaultResponseProcessorConfig()
	execWithProc := NewScenarioExecutorWithProcessor(server, procCfg)
	if execWithProc == nil {
		t.Fatal("expected non-nil executor")
	}
}
