package verification

import (
	"context"
	"testing"
)

func TestASTSemanticMatchStrategy_MissingConfig(t *testing.T) {
	strategy := &ASTSemanticMatchStrategy{}

	// Create a step without a config block
	step := map[string]any{
		"verification_strategy": "ast_semantic_match",
	}

	result, err := strategy.Verify(context.Background(), nil, nil, step)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// The logic should allow it to pass if no config is provided,
	// OR it should gracefully default.
	// Currently it fails with: "Verification Failed: Missing 'config' block for AST validation."
	if !result.Passed {
		t.Errorf("Expected strategy to pass (or handle default) when config is missing, but it failed with feedback: %s", result.Feedback)
	}
}
