// Package qwen adapts Qwen / DashScope and local Qwen-on-Ollama defaults.
package qwen

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/llm"
)

const (
	id          = "qwen"
	localPort   = "11434"
	localURL    = "http://127.0.0.1:11434/v1"
	localChat   = "qwen3.8:latest"
	cloudChat   = "qwen-max"
	embedModel  = "text-embedding-v3"
	dummyAPIKey = "ollama"
)

func init() {
	llm.RegisterProviderAdapter(Adapter{})
}

// Adapter owns Qwen model ids and local vs cloud base URLs.
type Adapter struct{}

func (Adapter) ID() string { return id }

func (Adapter) Match(provider, baseURL string) bool {
	if strings.EqualFold(strings.TrimSpace(provider), id) {
		return true
	}
	return strings.Contains(strings.ToLower(baseURL), "dashscope")
}

func (a Adapter) FillDefaults(cfg *llm.Config) {
	if cfg == nil {
		return
	}
	cfg.Provider = id
	local := strings.Contains(cfg.BaseURL, localPort)
	if cfg.BaseURL == "" || strings.Contains(cfg.BaseURL, "api.openai.com") {
		cfg.BaseURL = localURL
		local = true
	}
	if cfg.ChatModel == "" || cfg.ChatModel == "gpt-4o-mini" {
		if local {
			cfg.ChatModel = localChat
		} else {
			cfg.ChatModel = cloudChat
		}
	}
	if cfg.EmbedModel == "" || cfg.EmbedModel == "text-embedding-3-small" {
		cfg.EmbedModel = embedModel
	}
	if cfg.APIKey == "" && local {
		cfg.APIKey = dummyAPIKey
	}
}

func (Adapter) ClassifyModel(model string) (llm.ModelRole, bool) {
	n := strings.ToLower(strings.TrimSpace(model))
	if n == "" {
		return "", false
	}
	if strings.Contains(n, "qwen2.5-coder") || strings.Contains(n, "qwen2.5coder") {
		return llm.ModelRoleCodeDraft, true
	}
	return "", false
}
