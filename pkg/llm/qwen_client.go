package llm

import (
	"context"
	"strings"

	zqkctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// NewQwenClient creates a new Qwen API client by wrapping the OpenAI-compatible client
// and configuring it for local execution (e.g., Ollama, vLLM).
func NewQwenClient(ctx context.Context, config *Config) *OpenAIClient {
	if config == nil {
		config = DefaultConfig(ctx)
	}

	// If the user has a specific local Qwen endpoint, use it. Otherwise, default to local Ollama.
	secCtx := zqkctx.GetSecurityContext(ctx)
	if localUrl := secCtx.GetLLMBaseURL("qwen"); localUrl != "" {
		config.BaseURL = localUrl
	} else if config.BaseURL == "https://api.openai.com/v1" || config.BaseURL == "" {
		config.BaseURL = "http://127.0.0.1:11434/v1"
	}
	if config.ChatModel == "gpt-4o-mini" || config.ChatModel == "" {
		if strings.Contains(config.BaseURL, "11434") {
			config.ChatModel = "qwen3.8:latest"
		} else {
			config.ChatModel = "qwen-max"
		}
	}
	if config.EmbedModel == "text-embedding-3-small" || config.EmbedModel == "" {
		config.EmbedModel = "text-embedding-v3"
	}
	if config.ContextWindowSize == 0 {
		config.ContextWindowSize = zqkenv.Get(zqkenv.LLMContextWindowSize().Name()).IntOrDefault(32768)
	}

	// Fallback to ZQK_QWEN_API_KEY if specific key is desired over generic LLM_API_KEY
	if key := secCtx.GetLLMAPIKey("qwen"); key != "" {
		config.APIKey = key
	} else if config.APIKey == "" && strings.Contains(config.BaseURL, "11434") {
		config.APIKey = "ollama" // Dummy key for local endpoints
	}

	return NewOpenAIClient(ctx, config)
}
