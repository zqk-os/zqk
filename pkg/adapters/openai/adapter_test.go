package openai

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/llm"
)

func TestAdapter_publicCloudAndDefaults(t *testing.T) {
	a := Adapter{}
	if !a.IsPublicCloud(defaultBaseURL) {
		t.Fatal("hosted OpenAI URL must be public cloud")
	}
	cfg := &llm.Config{}
	a.FillDefaults(cfg)
	if cfg.BaseURL != defaultBaseURL {
		t.Fatalf("BaseURL=%q", cfg.BaseURL)
	}
	if cfg.ChatModel != defaultChat {
		t.Fatalf("ChatModel=%q", cfg.ChatModel)
	}
	if !llm.IsPublicCloudBaseURL(cfg.BaseURL) {
		t.Fatal("registry must see the openai adapter")
	}
}
