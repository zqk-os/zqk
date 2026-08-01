package llm

import (
	"context"
	"strings"
	"testing"
)

func TestGeminiClient_GenerateCompletion_NoAPIKey(t *testing.T) {
	client := NewGeminiClient(nil)

	res, err := client.GenerateCompletion(context.Background(), "test prompt", "system prompt")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(res, "APIKey missing") {
		t.Errorf("expected fallback response for missing API key, got %q", res)
	}
}
