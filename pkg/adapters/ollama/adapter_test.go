package ollama

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/llm"
)

func TestAdapter_fillsLocalDefaults(t *testing.T) {
	cfg := &llm.Config{BaseURL: "http://127.0.0.1:11434/v1"}
	Adapter{}.FillDefaults(cfg)
	if cfg.ChatModel != defaultChat {
		t.Fatalf("ChatModel=%q", cfg.ChatModel)
	}
	if cfg.EmbedModel != defaultEmbed {
		t.Fatalf("EmbedModel=%q", cfg.EmbedModel)
	}
	if cfg.APIKey != dummyAPIKey {
		t.Fatalf("APIKey=%q", cfg.APIKey)
	}
	if cfg.Timeout != defaultTimeout {
		t.Fatalf("Timeout=%v", cfg.Timeout)
	}
}

func TestDefaultConfig_usesOllamaAdapter(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "http://127.0.0.1:11434/v1")
	t.Setenv("LLM_CHAT_MODEL", "")
	cfg := llm.DefaultConfig(context.Background())
	if cfg.ChatModel != defaultChat {
		t.Fatalf("ChatModel=%q want %q", cfg.ChatModel, defaultChat)
	}
}

func TestClassifyModel_qwenCoder(t *testing.T) {
	role, ok := Adapter{}.ClassifyModel("qwen2.5-coder:7b")
	if !ok || role != llm.ModelRoleCodeDraft {
		t.Fatalf("got %q ok=%v", role, ok)
	}
}
