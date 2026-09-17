package llm

import (
	"context"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestDefaultConfig_UnprefixedEnv(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "http://127.0.0.1:11434/v1")
	t.Setenv("LLM_CHAT_MODEL", "qwen2.5-coder:latest")
	t.Setenv("LLM_PROVIDER", "openai")

	cfg := DefaultConfig(context.Background())
	if cfg.BaseURL != "http://127.0.0.1:11434/v1" {
		t.Fatalf("BaseURL = %q, want http://127.0.0.1:11434/v1", cfg.BaseURL)
	}
	if cfg.ChatModel != "qwen2.5-coder:latest" {
		t.Fatalf("ChatModel = %q, want qwen2.5-coder:latest", cfg.ChatModel)
	}
	if cfg.Provider != "openai" {
		t.Fatalf("Provider = %q, want openai", cfg.Provider)
	}
}

func TestDefaultConfig_OllamaFallbackModel(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "http://127.0.0.1:11434/v1")
	t.Setenv("LLM_CHAT_MODEL", "")
	t.Setenv(zqkenv.LLMChatModel().Name(), "")

	cfg := DefaultConfig(context.Background())
	if cfg.ChatModel != "qwen3.8:latest" {
		t.Fatalf("ChatModel = %q, want qwen3.8:latest", cfg.ChatModel)
	}
}

func TestDefaultConfig_OllamaTimeout(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "http://127.0.0.1:11434/v1")
	t.Setenv("LLM_TIMEOUT", "")
	t.Setenv(zqkenv.LLMTimeout().Name(), "")

	cfg := DefaultConfig(context.Background())
	if cfg.Timeout != 900*time.Second {
		t.Fatalf("Timeout = %v, want 900s", cfg.Timeout)
	}
}
