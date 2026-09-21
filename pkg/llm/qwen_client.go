package llm

import (
	"context"

	zqkctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// NewQwenClient creates a Qwen-configured OpenAI-compatible client.
// Vendor host/model defaults come from the registered qwen adapter.
func NewQwenClient(ctx context.Context, config *Config) *OpenAIClient {
	if config == nil {
		config = DefaultConfig(ctx)
	}
	if config.Provider == "" {
		config.Provider = "qwen"
	}
	if secCtx := zqkctx.GetSecurityContext(ctx); secCtx != nil {
		if u := secCtx.GetLLMBaseURL("qwen"); u != "" {
			config.BaseURL = u
		}
		if key := secCtx.GetLLMAPIKey("qwen"); key != "" {
			config.APIKey = key
		}
	}
	ApplyProviderAdapters(config)
	if config.ContextWindowSize == 0 {
		config.ContextWindowSize = zqkenv.LLMContextWindowSize().IntOrDefault(32768)
	}
	return NewOpenAIClient(ctx, config)
}
