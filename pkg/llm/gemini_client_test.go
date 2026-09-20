package llm

import (
	"context"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestGeminiClient_GenerateCompletion_NoAPIKey(t *testing.T) {
	// Scheduler/daemon shells often export ZQK_GEMINI_API_KEY; clear so shouldMock() engages.
	t.Setenv(zqkenv.GeminiAPIKey().Name(), "")
	t.Setenv(zqkenv.LLMAPIKey().Name(), "")
	t.Setenv(zqkenv.APIKey().Name(), "")
	t.Setenv(zqkenv.LLMBaseURL().Name(), "")

	client := NewGeminiClient(context.Background(), nil)

	res, err := client.GenerateCompletion(context.Background(), "test prompt", "system prompt")
	if err != nil {
		t.Fatalf("expected no error, got %v. Config: APIKey=%q, BaseURL=%q, shouldMock=%v", err, client.config.APIKey, client.config.BaseURL, client.shouldMock())
	}

	if !strings.Contains(res, "APIKey missing") {
		t.Errorf("expected fallback response for missing API key, got %q", res)
	}
}
