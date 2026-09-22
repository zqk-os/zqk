package adapters

import (
	"testing"

	mcptesting "github.com/zqk-os/zqk/pkg/mcp/testing"
)

func TestScenarioBuilderAdapter_GettersAndSteps(t *testing.T) {
	t.Parallel()

	builder := mcptesting.NewScenarioBuilder()
	adapter := NewScenarioBuilderAdapter(builder)
	if adapter.GetBuilder() != builder {
		t.Fatalf("expected GetBuilder to return underlying builder")
	}

	spec := mcptesting.ScenarioSpec{
		Name:              "Full Spec",
		Description:       "Full spec with steps",
		ResponseProcessor: "custom_proc",
		Imports:           []string{"import/a", "import/b"},
		Setup: []mcptesting.TestStep{
			{Name: "setup 1", Tool: "setup_tool"},
		},
		Tests: []mcptesting.TestStep{
			{Name: "test 1", Tool: "test_tool"},
		},
		Cleanup: []mcptesting.TestStep{
			{Name: "cleanup 1", Tool: "cleanup_tool"},
		},
	}

	specAdapter := NewScenarioSpecAdapter(spec)
	if specAdapter.GetName() != "Full Spec" {
		t.Errorf("expected GetName 'Full Spec', got %q", specAdapter.GetName())
	}
	if specAdapter.GetSpec().Name != "Full Spec" {
		t.Errorf("expected GetSpec().Name 'Full Spec'")
	}

	factory := NewScenarioBuilderFactoryAdapter()
	built := factory.CreateBuilder(specAdapter).Build()

	if built.Name != "Full Spec" {
		t.Errorf("expected Name 'Full Spec', got %q", built.Name)
	}
	if built.Description != "Full Spec with steps" && built.Description != "Full spec with steps" {
		t.Errorf("unexpected description: %q", built.Description)
	}
	if built.ResponseProcessor != "custom_proc" {
		t.Errorf("expected custom_proc, got %q", built.ResponseProcessor)
	}
	if len(built.Setup) != 1 {
		t.Errorf("expected 1 setup step, got %d", len(built.Setup))
	}
	if len(built.Tests) != 1 {
		t.Errorf("expected 1 test step, got %d", len(built.Tests))
	}
	if len(built.Cleanup) != 1 {
		t.Errorf("expected 1 cleanup step, got %d", len(built.Cleanup))
	}

	ve := &ValidationError{Field: "f", Message: "m"}
	if ve.Error() != "f: m" {
		t.Errorf("expected 'f: m', got %q", ve.Error())
	}
}
