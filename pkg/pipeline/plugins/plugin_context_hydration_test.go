package plugins

import (
	"context"
	"testing"
)

func TestContextHydrationPlugin_Execute(t *testing.T) {
	plugin := NewContextHydrationPlugin()

	if plugin.Name() != "ContextHydrationPlugin" {
		t.Errorf("Expected Name() to return 'ContextHydrationPlugin', got %s", plugin.Name())
	}

	ctx := context.Background()
	payload := make(map[string]any)

	result, err := plugin.Execute(ctx, payload)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["hydrated_context"] == nil {
		t.Errorf("Expected 'hydrated_context' to be populated")
	}

	err = plugin.Validate(ctx, result)
	if err != nil {
		t.Errorf("Validate failed on valid output: %v", err)
	}
}

func TestContextHydrationPlugin_Validate_Fails(t *testing.T) {
	plugin := NewContextHydrationPlugin()
	ctx := context.Background()

	// An empty payload should fail validation
	err := plugin.Validate(ctx, make(map[string]any))
	if err == nil {
		t.Errorf("Expected validation to fail for empty payload")
	}
}
