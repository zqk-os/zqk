package llm

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestDefaultConfig_UnprefixedEnv(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "http://127.0.0.1:9/v1")
	t.Setenv("LLM_CHAT_MODEL", "custom-model")
	t.Setenv("LLM_PROVIDER", "openai")

	cfg := DefaultConfig(context.Background())
	if cfg.BaseURL != "http://127.0.0.1:9/v1" {
		t.Fatalf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.ChatModel != "custom-model" {
		t.Fatalf("ChatModel = %q", cfg.ChatModel)
	}
	if cfg.Provider != "openai" {
		t.Fatalf("Provider = %q", cfg.Provider)
	}
}

func TestDefaultConfig_noVendorInferenceWithoutAdapters(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "http://127.0.0.1:11434/v1")
	t.Setenv("LLM_CHAT_MODEL", "")
	t.Setenv(zqkenv.LLMChatModel().Name(), "")
	t.Setenv("LLM_MODEL", "")
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("OLLAMA_MODEL", "")

	cfg := DefaultConfig(context.Background())
	if cfg.ChatModel != "" {
		t.Fatalf("core must not invent a chat model, got %q", cfg.ChatModel)
	}
	if cfg.Timeout != 300*time.Second {
		t.Fatalf("generic timeout = %v, want 300s", cfg.Timeout)
	}
}

func TestDefaultConfig_UnprefixedAPIKey(t *testing.T) {
	t.Run("LLM_API_KEY", func(t *testing.T) {
		t.Setenv(zqkenv.LLMAPIKey().Name(), "")
		t.Setenv("LLM_API_KEY", "test-key-123")
		t.Setenv("OPENAI_API_KEY", "")

		cfg := DefaultConfig(context.Background())
		if cfg.APIKey != "test-key-123" {
			t.Fatalf("APIKey = %q, want test-key-123", cfg.APIKey)
		}
	})
}

func TestDefaultConfig_UnprefixedModelVariants(t *testing.T) {
	t.Setenv("LLM_CHAT_MODEL", "")
	t.Setenv("LLM_MODEL", "meta-llama/Llama-3-70b")

	cfg := DefaultConfig(context.Background())
	if cfg.ChatModel != "meta-llama/Llama-3-70b" {
		t.Fatalf("ChatModel = %q, want meta-llama/Llama-3-70b", cfg.ChatModel)
	}
}

func TestDefaultConfig_zeroConfigStaysEmptyWithoutAdapters(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv(zqkenv.LLMBaseURL().Name(), "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv(zqkenv.LLMAPIKey().Name(), "")
	t.Setenv("LLM_CHAT_MODEL", "")
	t.Setenv("LLM_MODEL", "")
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("OLLAMA_MODEL", "")
	t.Setenv(zqkenv.LLMChatModel().Name(), "")
	t.Setenv("LLM_PROVIDER", "")
	t.Setenv(zqkenv.LLMProvider().Name(), "")

	cfg := DefaultConfig(context.Background())
	if strings.Contains(cfg.BaseURL, "openai.com") || strings.Contains(cfg.BaseURL, "11434") {
		t.Fatalf("core invented a vendor URL: %q", cfg.BaseURL)
	}
	if strings.Contains(strings.ToLower(cfg.ChatModel), "qwen") || strings.Contains(cfg.ChatModel, "gpt-") {
		t.Fatalf("core invented a vendor model: %q", cfg.ChatModel)
	}
}

func TestOpenAIClient_localEndpointNotMocked(t *testing.T) {
	cfg := &Config{
		Provider:  "openai",
		BaseURL:   "http://127.0.0.1:9/v1",
		APIKey:    "",
		ChatModel: "any",
	}
	client := NewOpenAIClient(context.Background(), cfg)
	if client.shouldMock() {
		t.Fatalf("shouldMock() = true for local endpoint %q", cfg.BaseURL)
	}
}

func TestOpenAIClient_emptyURLMocks(t *testing.T) {
	client := NewOpenAIClient(context.Background(), &Config{APIKey: ""})
	if !client.shouldMock() {
		t.Fatal("empty BaseURL with empty key should mock")
	}
}
