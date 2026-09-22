// BLI-STARTER-COMMUNITY-028 / PRI-STARTER-COMMUNITY-028 coverage elevation
package openai

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestAdapter_MatchDetectID(t *testing.T) {
	t.Parallel()
	a := Adapter{}
	if a.ID() != id {
		t.Fatalf("ID=%q", a.ID())
	}
	if !a.Match("OpenAI", "") {
		t.Fatal("provider")
	}
	if !a.Match("", "https://api.openai.com/v1") {
		t.Fatal("host")
	}
	if a.Match("ollama", "http://127.0.0.1:11434/v1") {
		t.Fatal("local")
	}
	if a.IsPublicCloud("http://127.0.0.1:11434/v1") {
		t.Fatal("local not public")
	}
	if a.Detect(nil) {
		t.Fatal("nil")
	}
	if !a.Detect(&llm.Config{}) {
		t.Fatal("empty URL is detect")
	}
	if a.Detect(&llm.Config{BaseURL: defaultBaseURL}) {
		t.Fatal("already set")
	}
}

func TestAdapter_FillDefaultsFromEnv(t *testing.T) {
	t.Setenv(zqkenv.OpenAIBaseURL().Name(), "https://example.invalid/v1")
	t.Setenv(zqkenv.OpenAIAPIKey().Name(), "sk-test")
	t.Setenv("OPENAI_MODEL", "gpt-test")
	cfg := &llm.Config{}
	Adapter{}.FillDefaults(cfg)
	if cfg.BaseURL != "https://example.invalid/v1" {
		t.Fatalf("url %q", cfg.BaseURL)
	}
	if cfg.APIKey != "sk-test" {
		t.Fatalf("key %q", cfg.APIKey)
	}
	if cfg.ChatModel != "gpt-test" {
		t.Fatalf("model %q", cfg.ChatModel)
	}
	if cfg.EmbedModel != defaultEmbed || cfg.Provider != id {
		t.Fatalf("embed/provider %q %q", cfg.EmbedModel, cfg.Provider)
	}
	Adapter{}.FillDefaults(nil)
}

func TestAdapter_FillDefaultsOpenAIEnvFallback(t *testing.T) {
	t.Setenv(zqkenv.OpenAIBaseURL().Name(), "")
	t.Setenv(zqkenv.OpenAIAPIKey().Name(), "")
	t.Setenv("OPENAI_BASE_URL", "https://fallback.example/v1")
	t.Setenv("OPENAI_API_KEY", "sk-fallback")
	cfg := &llm.Config{}
	Adapter{}.FillDefaults(cfg)
	if cfg.BaseURL != "https://fallback.example/v1" {
		t.Fatalf("url %q", cfg.BaseURL)
	}
	if cfg.APIKey != "sk-fallback" {
		t.Fatalf("key %q", cfg.APIKey)
	}
}
