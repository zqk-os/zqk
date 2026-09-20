// Package ollama adapts a local OpenAI-compatible Ollama endpoint.
package ollama

import (
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const (
	id             = "ollama"
	defaultPort    = "11434"
	defaultBaseURL = "http://127.0.0.1:11434/v1"
	defaultChat    = "qwen3.8:latest"
	defaultEmbed   = "nomic-embed-text"
	dummyAPIKey    = "ollama"
	defaultTimeout = 900 * time.Second
)

func init() {
	llm.RegisterProviderAdapter(Adapter{})
}

// Adapter owns Ollama host, port, and default local models.
type Adapter struct{}

func (Adapter) ID() string { return id }

func (Adapter) Match(provider, baseURL string) bool {
	if strings.EqualFold(strings.TrimSpace(provider), id) {
		return true
	}
	if strings.Contains(baseURL, defaultPort) {
		return true
	}
	if host := strings.TrimSpace(os.Getenv("OLLAMA_HOST")); host != "" {
		clean := host
		if strings.Contains(clean, "://") {
			if u, err := url.Parse(clean); err == nil && u.Host != "" {
				clean = u.Host
			}
		}
		if clean != "" && strings.Contains(baseURL, clean) {
			return true
		}
	}
	return false
}

func (a Adapter) Detect(cfg *llm.Config) bool {
	if cfg == nil || strings.TrimSpace(cfg.BaseURL) != "" {
		return false
	}
	if disabledLocal() {
		return false
	}
	if !enabledLocal() {
		return false
	}
	return localListening()
}

func (a Adapter) FillDefaults(cfg *llm.Config) {
	if cfg == nil {
		return
	}
	if cfg.BaseURL == "" {
		if host := strings.TrimSpace(os.Getenv("OLLAMA_HOST")); host != "" {
			cfg.BaseURL = normalizeBaseURL(host)
		} else {
			cfg.BaseURL = defaultBaseURL
		}
	} else {
		cfg.BaseURL = ensureV1(cfg.BaseURL)
	}
	if cfg.ChatModel == "" {
		if m := strings.TrimSpace(os.Getenv("OLLAMA_MODEL")); m != "" {
			cfg.ChatModel = m
		} else {
			cfg.ChatModel = defaultChat
		}
	}
	if cfg.EmbedModel == "" {
		if m := strings.TrimSpace(os.Getenv("OLLAMA_EMBED_MODEL")); m != "" {
			cfg.EmbedModel = m
		} else {
			cfg.EmbedModel = defaultEmbed
		}
	}
	if cfg.APIKey == "" {
		cfg.APIKey = dummyAPIKey
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.Provider == "" {
		cfg.Provider = id
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

func disabledLocal() bool {
	if v := zqkenv.Get("LLM_DISABLE_LOCAL_OLLAMA").OrDefault(""); v == "true" || v == "1" {
		return true
	}
	if v := zqkenv.DisableLocalOllama().OrDefault(""); v == "true" || v == "1" {
		return true
	}
	return false
}

func enabledLocal() bool {
	if v := zqkenv.Get("LLM_ENABLE_LOCAL_OLLAMA").OrDefault(""); v == "true" || v == "1" {
		return true
	}
	if v := zqkenv.ForceLocalOllama().OrDefault(""); v == "true" || v == "1" {
		return true
	}
	if v := zqkenv.Get("FORCE_LOCAL_OLLAMA").OrDefault(""); v == "true" || v == "1" {
		return true
	}
	return !zqkenv.IsInTest()
}

func localListening() bool {
	var targets []string
	if host := strings.TrimSpace(os.Getenv("OLLAMA_HOST")); host != "" {
		if t := dialTarget(host); t != "" {
			targets = append(targets, t)
		}
	}
	targets = append(targets, "127.0.0.1:"+defaultPort, "localhost:"+defaultPort, "[::1]:"+defaultPort)
	seen := map[string]bool{}
	for _, target := range targets {
		if target == "" || seen[target] {
			continue
		}
		seen[target] = true
		conn, err := net.DialTimeout("tcp", target, 50*time.Millisecond) //nolint:gosec // G704: loopback probe for local daemon
		if err != nil {
			continue
		}
		_ = conn.Close()
		return true
	}
	return false
}

func normalizeBaseURL(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return defaultBaseURL
	}
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "http://" + host
	}
	return ensureV1(host)
}

func ensureV1(u string) string {
	u = strings.TrimRight(strings.TrimSpace(u), "/")
	if u == "" {
		return u
	}
	if strings.HasSuffix(u, "/v1") || strings.Contains(u, "/v1/") {
		return u
	}
	return u + "/v1"
}

func dialTarget(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if strings.Contains(host, "://") {
		if u, err := url.Parse(host); err == nil && u.Host != "" {
			host = u.Host
		}
	}
	if !strings.Contains(host, ":") {
		host = net.JoinHostPort(host, defaultPort)
	}
	return host
}
