package llm

import (
	"os"
	"strings"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

// NewQwenClient creates a new Qwen API client by wrapping the OpenAI-compatible client
// and configuring it for local execution (e.g., Ollama, vLLM).
func NewQwenClient(config *Config) *OpenAIClient {
	if config == nil {
		config = DefaultConfig()
	}

	// If the user has a specific local Qwen endpoint, use it. Otherwise, default to local Ollama.
	if localUrl := os.Getenv(zqkenv.QwenBaseURL()); localUrl != "" {
		config.BaseURL = localUrl
	} else if config.BaseURL == "https://api.openai.com/v1" || config.BaseURL == "" {
		config.BaseURL = "http://localhost:11434/v1"
	}
	if config.ChatModel == "gpt-4o-mini" || config.ChatModel == "" {
		if config.BaseURL == "http://localhost:11434/v1" {
			config.ChatModel = "qwen3.6:latest"
		} else {
			config.ChatModel = "qwen-max"
		}
	}
	if config.EmbedModel == "text-embedding-3-small" || config.EmbedModel == "" {
		config.EmbedModel = "text-embedding-v3"
	}
	if config.ContextWindowSize == 0 {
		config.ContextWindowSize = zqkenv.Get(zqkenv.LLMContextWindowSize()).IntOrDefault(32768)
	}

	// Fallback to ZQK_QWEN_API_KEY if specific key is desired over generic LLM_API_KEY
	if key := os.Getenv(zqkenv.QwenAPIKey()); key != "" {
		config.APIKey = key
	} else if config.APIKey == "" && strings.Contains(config.BaseURL, "localhost:11434") {
		config.APIKey = "ollama" // Dummy key for local endpoints
	}

	return NewOpenAIClient(config)
}
