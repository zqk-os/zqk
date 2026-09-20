// Package gemini adapts Google Gemini generative-language defaults.
package gemini

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const (
	id           = "gemini"
	defaultURL   = "https://generativelanguage.googleapis.com/v1beta"
	defaultChat  = "gemini-2.5-flash"
	defaultEmbed = "gemini-embedding-2"
)

func init() {
	llm.RegisterProviderAdapter(Adapter{})
}

// Adapter owns Gemini host and default model ids.
type Adapter struct{}

func (Adapter) ID() string { return id }

func (Adapter) Match(provider, _ string) bool {
	return strings.EqualFold(strings.TrimSpace(provider), id)
}

func (Adapter) IsPublicCloud(baseURL string) bool {
	u := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return strings.HasPrefix(u, "https://generativelanguage.googleapis.com")
}

func (Adapter) Detect(cfg *llm.Config) bool {
	if cfg == nil || strings.TrimSpace(cfg.Provider) != "" || strings.TrimSpace(cfg.BaseURL) != "" {
		return false
	}
	return strings.TrimSpace(zqkenv.GeminiAPIKey().Get()) != ""
}

func (Adapter) FillDefaults(cfg *llm.Config) {
	if cfg == nil {
		return
	}
	cfg.Provider = id
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultURL
	}
	if cfg.ChatModel == "" {
		cfg.ChatModel = defaultChat
	}
	if cfg.EmbedModel == "" {
		cfg.EmbedModel = defaultEmbed
	}
	if cfg.APIKey == "" {
		if v := strings.TrimSpace(zqkenv.GeminiAPIKey().Get()); v != "" {
			cfg.APIKey = v
		}
	}
}
