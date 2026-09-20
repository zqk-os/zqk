package testing

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestExecutor_RegexMatching tests regex pattern matching in validation
func TestExecutor_RegexMatching(t *testing.T) {
	t.Parallel()
	server := mcp.NewServer()
	executor := NewScenarioExecutor(server)

	step := &TestStep{
		Name: "Regex Test",
		Expected: &TestExpectation{
			Matches: map[string]string{
				objects.FieldKeyID:     "^BLI-\\d+$",            // Pattern: BLI- followed by digits
				objects.FieldKeyStatus: "^(exploring|planned)$", // Pattern: exploring or planned
			},
		},
	}

	result := StepResult{
		StepName: "Regex Test",
		Success:  true,
		Result: map[string]any{
			objects.FieldKeyID:     "BLI-123",
			objects.FieldKeyStatus: "exploring",
			objects.FieldKeyTitle:  "Test Item",
		},
	}

	err := executor.validateStepResult(step, result)
	if err != nil {
		t.Errorf("Regex validation should pass, got error: %v", err)
	}
}

// TestExecutor_RegexMatching_Fail tests regex pattern matching failures
func TestExecutor_RegexMatching_Fail(t *testing.T) {
	t.Parallel()
	server := mcp.NewServer()
	executor := NewScenarioExecutor(server)

	step := &TestStep{
		Name: "Regex Test Fail",
		Expected: &TestExpectation{
			Matches: map[string]string{
				objects.FieldKeyID: "^BLI-\\d+$", // Pattern: BLI- followed by digits
			},
		},
	}

	result := StepResult{
		StepName: "Regex Test Fail",
		Success:  true,
		Result: map[string]any{
			objects.FieldKeyID: "INVALID-123", // Doesn't match pattern
		},
	}

	err := executor.validateStepResult(step, result)
	if err == nil {
		t.Error("Regex validation should fail for non-matching pattern, got nil error")
	}
}

// TestExecutor_ErrorValidation tests error detail validation
func TestExecutor_ErrorValidation(t *testing.T) {
	t.Parallel()
	server := mcp.NewServer()
	executor := NewScenarioExecutor(server)

	step := &TestStep{
		Name: "Error Validation Test",
		Expected: &TestExpectation{
			Error: &ErrorExpectation{
				Type:    "elicitation",
				Message: "Missing required",
			},
		},
	}

	elicitationErr := mcp.NewElicitationError("Missing required fields", []mcp.ElicitationParam{
		{Name: "title", Description: "Title is required", Type: "string", Required: true},
	})

	result := StepResult{
		StepName: "Error Validation Test",
		Success:  false,
		Error:    elicitationErr,
	}

	err := executor.validateStepResult(step, result)
	if err != nil {
		t.Errorf("Error validation should pass, got error: %v", err)
	}
}

// TestExecutor_ErrorValidation_Code tests error code validation
func TestExecutor_ErrorValidation_Code(t *testing.T) {
	t.Parallel()
	server := mcp.NewServer()
	executor := NewScenarioExecutor(server)

	code := -32602 // InvalidParams
	step := &TestStep{
		Name: "Error Code Validation Test",
		Expected: &TestExpectation{
			Error: &ErrorExpectation{
				Code:    &code,
				Message: "Invalid.*", // Regex pattern
			},
		},
	}

	jsonrpcErr := &mcp.JSONRPCError{
		Code:    -32602,
		Message: "Invalid parameters",
		Data:    nil,
	}

	result := StepResult{
		StepName: "Error Code Validation Test",
		Success:  false,
		Error:    jsonrpcErr,
	}

	err := executor.validateStepResult(step, result)
	if err != nil {
		t.Errorf("Error code validation should pass, got error: %v", err)
	}
}

// TestExecutor_DeepEquality tests deep equality comparison
func TestExecutor_DeepEquality(t *testing.T) {
	t.Parallel()
	server := mcp.NewServer()
	executor := NewScenarioExecutor(server)

	step := &TestStep{
		Name: "Deep Equality Test",
		Expected: &TestExpectation{
			Equals: map[string]any{
				objects.FieldKeyTags: []any{"tag1", "tag2"},
				"nested":             map[string]any{"key": "value"},
				"numbers":            []any{1, 2, 3},
			},
		},
	}

	result := StepResult{
		StepName: "Deep Equality Test",
		Success:  true,
		Result: map[string]any{
			objects.FieldKeyTags: []any{"tag1", "tag2"},
			"nested":             map[string]any{"key": "value"},
			"numbers":            []any{1, 2, 3},
		},
	}

	err := executor.validateStepResult(step, result)
	if err != nil {
		t.Errorf("Deep equality validation should pass, got error: %v", err)
	}
}

// TestExecutor_DeepEquality_Fail tests deep equality comparison failures
func TestExecutor_DeepEquality_Fail(t *testing.T) {
	t.Parallel()
	server := mcp.NewServer()
	executor := NewScenarioExecutor(server)

	step := &TestStep{
		Name: "Deep Equality Test Fail",
		Expected: &TestExpectation{
			Equals: map[string]any{
				objects.FieldKeyTags: []any{"tag1", "tag2"},
			},
		},
	}

	result := StepResult{
		StepName: "Deep Equality Test Fail",
		Success:  true,
		Result: map[string]any{
			objects.FieldKeyTags: []any{"tag1", "tag3"}, // Different value
		},
	}

	err := executor.validateStepResult(step, result)
	if err == nil {
		t.Error("Deep equality validation should fail for different values, got nil error")
	}
}
