// BLI-STARTER-COMMUNITY-028 / PRI-STARTER-COMMUNITY-028 coverage elevation
package gemini

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestAdapter_MatchDetectFill(t *testing.T) {
	a := Adapter{}
	if a.ID() != id {
		t.Fatalf("ID=%q", a.ID())
	}
	if !a.Match("Gemini", "") {
		t.Fatal("provider")
	}
	if a.Match("openai", defaultURL) {
		t.Fatal("openai")
	}
	if !a.IsPublicCloud(defaultURL) {
		t.Fatal("public")
	}
	if a.IsPublicCloud("http://127.0.0.1:11434/v1") {
		t.Fatal("local")
	}
	if a.Detect(nil) {
		t.Fatal("nil")
	}
	if a.Detect(&llm.Config{Provider: "openai"}) {
		t.Fatal("provider set")
	}
	t.Setenv(zqkenv.GeminiAPIKey().Name(), "gem-test")
	if !a.Detect(&llm.Config{}) {
		t.Fatal("key should detect")
	}
	cfg := &llm.Config{}
	a.FillDefaults(cfg)
	if cfg.Provider != id || cfg.BaseURL != defaultURL || cfg.ChatModel != defaultChat || cfg.EmbedModel != defaultEmbed {
		t.Fatalf("%+v", cfg)
	}
	if cfg.APIKey != "gem-test" {
		t.Fatalf("key %q", cfg.APIKey)
	}
	a.FillDefaults(nil)
}
