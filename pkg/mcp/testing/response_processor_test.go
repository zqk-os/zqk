package testing

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestElicitationToSuccessProcessor tests that elicitation errors are converted to success
func TestElicitationToSuccessProcessor(t *testing.T) {
	t.Parallel()
	processor := &ElicitationToSuccessProcessor{}

	// Test with elicitation error
	elicitationErr := mcp.NewElicitationError("Missing fields", []mcp.ElicitationParam{
		{Name: "title", Description: "Title is required", Type: "string", Required: true},
	})

	result, err, shouldContinue := processor.ProcessResponse(nil, elicitationErr)

	if err != nil {
		t.Errorf("Expected no error after processing, got: %v", err)
	}
	if !shouldContinue {
		t.Error("Expected shouldContinue=true")
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected result to be map[string]any, got %T", result)
	}

	if elicitation, ok := resultMap["elicitation"].(bool); !ok || !elicitation {
		t.Errorf("Expected elicitation=true in result, got %v", resultMap["elicitation"])
	}

	if params, ok := resultMap[objects.FieldKeyParameters].([]mcp.ElicitationParam); !ok || len(params) == 0 {
		t.Errorf("Expected parameters in result, got %v", resultMap[objects.FieldKeyParameters])
	}
}

// TestElicitationToSuccessProcessor_NoError tests that non-elicitation errors pass through
func TestElicitationToSuccessProcessor_NoError(t *testing.T) {
	t.Parallel()
	processor := &ElicitationToSuccessProcessor{}

	// Test with no error (success case)
	result, err, shouldContinue := processor.ProcessResponse(map[string]any{"key": "value"}, nil)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if !shouldContinue {
		t.Error("Expected shouldContinue=true")
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected result to be map[string]any, got %T", result)
	}

	if resultMap["key"] != "value" {
		t.Errorf("Expected result to be unchanged, got %v", result)
	}
}

// TestIgnoreErrorsProcessor tests that all errors are converted to success
func TestIgnoreErrorsProcessor(t *testing.T) {
	t.Parallel()
	processor := &IgnoreErrorsProcessor{}

	testErr := &mcp.JSONRPCError{Code: -32603, Message: "Internal error"}
	result, err, shouldContinue := processor.ProcessResponse(nil, testErr)

	if err != nil {
		t.Errorf("Expected no error after processing, got: %v", err)
	}
	if !shouldContinue {
		t.Error("Expected shouldContinue=true")
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected result to be map[string]any, got %T", result)
	}

	if ignored, ok := resultMap["error_ignored"].(bool); !ok || !ignored {
		t.Errorf("Expected error_ignored=true in result, got %v", resultMap["error_ignored"])
	}
}

// TestProcessorRegistry tests the processor registry
func TestProcessorRegistry(t *testing.T) {
	t.Parallel()
	registry := GetGlobalProcessorRegistry()

	// Test built-in processors
	processors := []string{"elicitation_to_success", "ignore_errors", "noop"}
	for _, name := range processors {
		processor, err := registry.Get(name)
		if err != nil {
			t.Errorf("Failed to get processor %s: %v", name, err)
			continue
		}
		if processor == nil {
			t.Errorf("Processor %s is nil", name)
		}
	}

	// Test unknown processor
	_, err := registry.Get("unknown_processor")
	if err == nil {
		t.Error("Expected error for unknown processor")
	}
}
