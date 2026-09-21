// Package openai adapts the hosted OpenAI HTTP API.
package openai

import (
	"os"
	"strings"

	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const (
	id             = "openai"
	defaultBaseURL = "https://api.openai.com/v1"
	defaultChat    = "gpt-4o-mini"
	defaultEmbed   = "text-embedding-3-small"
)

func init() {
	llm.RegisterProviderAdapter(Adapter{})
}

// Adapter owns OpenAI cloud host and default model ids.
type Adapter struct{}

func (Adapter) ID() string { return id }

func (Adapter) Match(provider, baseURL string) bool {
	if strings.EqualFold(strings.TrimSpace(provider), id) {
		return true
	}
	return strings.Contains(baseURL, "api.openai.com")
}

func (Adapter) IsPublicCloud(baseURL string) bool {
	u := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return strings.HasPrefix(u, "https://api.openai.com")
}

func (Adapter) Detect(cfg *llm.Config) bool {
	return cfg != nil && strings.TrimSpace(cfg.BaseURL) == ""
}

func (a Adapter) FillDefaults(cfg *llm.Config) {
	if cfg == nil {
		return
	}
	if cfg.BaseURL == "" {
		if v := strings.TrimSpace(zqkenv.OpenAIBaseURL().Get()); v != "" {
			cfg.BaseURL = v
		} else if v := strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")); v != "" {
			cfg.BaseURL = v
		} else {
			cfg.BaseURL = defaultBaseURL
		}
	}
	if cfg.ChatModel == "" {
		if v := strings.TrimSpace(os.Getenv("OPENAI_MODEL")); v != "" {
			cfg.ChatModel = v
		} else {
			cfg.ChatModel = defaultChat
		}
	}
	if cfg.EmbedModel == "" {
		cfg.EmbedModel = defaultEmbed
	}
	if cfg.APIKey == "" {
		if v := strings.TrimSpace(zqkenv.OpenAIAPIKey().Get()); v != "" {
			cfg.APIKey = v
		} else if v := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); v != "" {
			cfg.APIKey = v
		}
	}
	if cfg.Provider == "" {
		cfg.Provider = id
	}
}
