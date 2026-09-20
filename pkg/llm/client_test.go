package llm

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/zqkenv"
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

func TestLLMHTTPTimeout_usesConfigNotHardcodedFloor(t *testing.T) {
	if got := llmHTTPTimeout(900 * time.Second); got != 900*time.Second {
		t.Fatalf("llmHTTPTimeout(900s) = %v, want 900s (Ollama DefaultConfig must reach http.Client)", got)
	}
	if got := llmHTTPTimeout(0); got != defaultLLMHTTPTimeout {
		t.Fatalf("llmHTTPTimeout(0) = %v, want %v", got, defaultLLMHTTPTimeout)
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

	t.Run("OPENAI_API_KEY", func(t *testing.T) {
		t.Setenv(zqkenv.LLMAPIKey().Name(), "")
		t.Setenv("LLM_API_KEY", "")
		t.Setenv("OPENAI_API_KEY", "test-openai-key-456")

		cfg := DefaultConfig(context.Background())
		if cfg.APIKey != "test-openai-key-456" {
			t.Fatalf("APIKey = %q, want test-openai-key-456", cfg.APIKey)
		}
	})
}

func TestDefaultConfig_UnprefixedModelVariants(t *testing.T) {
	t.Run("LLM_MODEL fallback", func(t *testing.T) {
		t.Setenv("LLM_CHAT_MODEL", "")
		t.Setenv("LLM_MODEL", "meta-llama/Llama-3-70b")

		cfg := DefaultConfig(context.Background())
		if cfg.ChatModel != "meta-llama/Llama-3-70b" {
			t.Fatalf("ChatModel = %q, want meta-llama/Llama-3-70b", cfg.ChatModel)
		}
	})

	t.Run("OPENAI_MODEL fallback", func(t *testing.T) {
		t.Setenv("LLM_CHAT_MODEL", "")
		t.Setenv("LLM_MODEL", "")
		t.Setenv("OPENAI_MODEL", "gpt-4o")

		cfg := DefaultConfig(context.Background())
		if cfg.ChatModel != "gpt-4o" {
			t.Fatalf("ChatModel = %q, want gpt-4o", cfg.ChatModel)
		}
	})

	t.Run("OLLAMA_MODEL fallback", func(t *testing.T) {
		t.Setenv("LLM_CHAT_MODEL", "")
		t.Setenv("LLM_MODEL", "")
		t.Setenv("OPENAI_MODEL", "")
		t.Setenv("OLLAMA_MODEL", "deepseek-coder:latest")

		cfg := DefaultConfig(context.Background())
		if cfg.ChatModel != "deepseek-coder:latest" {
			t.Fatalf("ChatModel = %q, want deepseek-coder:latest", cfg.ChatModel)
		}
	})

	t.Run("LLM_EMBED_MODEL and OLLAMA_EMBED_MODEL", func(t *testing.T) {
		t.Setenv(zqkenv.LLMEmbedModel().Name(), "")
		t.Setenv("LLM_EMBED_MODEL", "text-embedding-ada-002")

		cfg := DefaultConfig(context.Background())
		if cfg.EmbedModel != "text-embedding-ada-002" {
			t.Fatalf("EmbedModel = %q, want text-embedding-ada-002", cfg.EmbedModel)
		}

		t.Setenv("LLM_EMBED_MODEL", "")
		t.Setenv("OLLAMA_EMBED_MODEL", "bge-large-en")
		cfg2 := DefaultConfig(context.Background())
		if cfg2.EmbedModel != "bge-large-en" {
			t.Fatalf("EmbedModel = %q, want bge-large-en", cfg2.EmbedModel)
		}
	})
}

func TestDefaultConfig_OllamaHost(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on loopback: %v", err)
	}
	defer ln.Close()

	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv(zqkenv.LLMBaseURL().Name(), "")
	t.Setenv("OLLAMA_HOST", ln.Addr().String())
	t.Setenv("LLM_ENABLE_LOCAL_OLLAMA", "true")

	cfg := DefaultConfig(context.Background())
	expectedURL := "http://" + ln.Addr().String() + "/v1"
	if cfg.BaseURL != expectedURL {
		t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, expectedURL)
	}
	if cfg.ChatModel != "qwen3.8:latest" {
		t.Fatalf("ChatModel = %q, want qwen3.8:latest", cfg.ChatModel)
	}
	if cfg.EmbedModel != "nomic-embed-text" {
		t.Fatalf("EmbedModel = %q, want nomic-embed-text", cfg.EmbedModel)
	}
}

func TestDefaultConfig_ZeroConfigOllamaDetection_Active(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on loopback: %v", err)
	}
	defer ln.Close()

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
	t.Setenv("OLLAMA_HOST", ln.Addr().String())
	t.Setenv("LLM_ENABLE_LOCAL_OLLAMA", "true")

	if !isLocalOllamaRunning() {
		t.Fatal("expected isLocalOllamaRunning() to be true when listener is active")
	}

	cfg := DefaultConfig(context.Background())
	expectedURL := "http://" + ln.Addr().String() + "/v1"
	if cfg.BaseURL != expectedURL {
		t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, expectedURL)
	}
	if cfg.ChatModel != "qwen3.8:latest" {
		t.Fatalf("ChatModel = %q, want qwen3.8:latest", cfg.ChatModel)
	}
	if cfg.EmbedModel != "nomic-embed-text" {
		t.Fatalf("EmbedModel = %q, want nomic-embed-text", cfg.EmbedModel)
	}
}

func TestDefaultConfig_ZeroConfigOllamaDetection_Disabled(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on loopback: %v", err)
	}
	defer ln.Close()

	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv(zqkenv.LLMBaseURL().Name(), "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv(zqkenv.LLMAPIKey().Name(), "")
	t.Setenv("OLLAMA_HOST", ln.Addr().String())
	t.Setenv("LLM_ENABLE_LOCAL_OLLAMA", "true")
	t.Setenv("LLM_DISABLE_LOCAL_OLLAMA", "true")

	if !isLocalOllamaDisabled() {
		t.Fatal("expected isLocalOllamaDisabled() to be true")
	}
	if shouldDetectLocalOllama() {
		t.Fatal("expected shouldDetectLocalOllama() to be false when disabled")
	}
}

func TestDefaultConfig_ZeroConfigOllamaDetection_OfflineFallback(t *testing.T) {
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
	t.Setenv("OLLAMA_HOST", "127.0.0.1:65432") // offline port
	t.Setenv("LLM_DISABLE_LOCAL_OLLAMA", "true")

	cfg := DefaultConfig(context.Background())
	if cfg.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("BaseURL = %q, want https://api.openai.com/v1", cfg.BaseURL)
	}
	if cfg.ChatModel != "gpt-4o-mini" {
		t.Fatalf("ChatModel = %q, want gpt-4o-mini", cfg.ChatModel)
	}
	if cfg.EmbedModel != "text-embedding-3-small" {
		t.Fatalf("EmbedModel = %q, want text-embedding-3-small", cfg.EmbedModel)
	}
}

func TestOpenAIClient_LocalOllamaNoMockFallback(t *testing.T) {
	cfg := &Config{
		Provider:  "openai",
		BaseURL:   "http://127.0.0.1:11434/v1",
		APIKey:    "",
		ChatModel: "qwen3.8:latest",
	}

	client := NewOpenAIClient(context.Background(), cfg)
	if client.shouldMock() {
		t.Fatalf("shouldMock() = true for local Ollama endpoint %q; want false", cfg.BaseURL)
	}

	publicCfg := &Config{
		Provider:  "openai",
		BaseURL:   "https://api.openai.com/v1",
		APIKey:    "",
		ChatModel: "gpt-4o-mini",
	}
	publicClient := NewOpenAIClient(context.Background(), publicCfg)
	if !publicClient.shouldMock() {
		t.Fatalf("shouldMock() = false for public OpenAI with empty key; want true (mock fallback)")
	}
}
