package plugins

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestInteractiveInstantiationPlugin_Name(t *testing.T) {
	plugin := NewInteractiveInstantiationPlugin(nil)
	if plugin.Name() != "InteractiveInstantiationPlugin" {
		t.Errorf("Expected InteractiveInstantiationPlugin, got %s", plugin.Name())
	}
}

func TestInteractiveInstantiationPlugin_Execute(t *testing.T) {
	plugin := NewInteractiveInstantiationPlugin(nil)

	payload := map[string]any{
		objects.FieldKeyKind: "backlog_item",
		// missing required fields to force pending state
	}

	result, err := plugin.Execute(context.Background(), payload)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result["interactive_instantiation"] != "pending_user_input" {
		t.Errorf("Expected interactive_instantiation to be pending_user_input, got %v", result["interactive_instantiation"])
	}

	err = plugin.Validate(context.Background(), result)
	if err == nil {
		t.Errorf("Expected validation error for pending input, got nil")
	}
}
