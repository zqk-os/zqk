package llm

import (
	"strings"
	"sync"
)

// ProviderAdapter supplies vendor-specific LLM defaults. Core code must not
// hard-code model names, provider hostnames, or well-known vendor ports;
// those live in pkg/adapters/<vendor> and register here from the product binary.
type ProviderAdapter interface {
	ID() string
	Match(provider, baseURL string) bool
	FillDefaults(cfg *Config)
}

// Detector is an optional adapter that can notice a local vendor endpoint
// when no base URL is configured.
type Detector interface {
	Detect(cfg *Config) bool
}

// PublicCloudChecker reports whether a base URL is that vendor's hosted API.
type PublicCloudChecker interface {
	IsPublicCloud(baseURL string) bool
}

// ModelClassifier optionally maps a chat-model id onto a harness role.
type ModelClassifier interface {
	ClassifyModel(model string) (ModelRole, bool)
}

var (
	providerMu       sync.RWMutex
	providerAdapters []ProviderAdapter
)

// RegisterProviderAdapter adds a vendor adapter. Safe for init().
func RegisterProviderAdapter(a ProviderAdapter) {
	if a == nil {
		return
	}
	providerMu.Lock()
	defer providerMu.Unlock()
	id := strings.ToLower(strings.TrimSpace(a.ID()))
	for i, existing := range providerAdapters {
		if strings.EqualFold(existing.ID(), id) {
			providerAdapters[i] = a
			return
		}
	}
	providerAdapters = append(providerAdapters, a)
}

func snapshotProviderAdapters() []ProviderAdapter {
	providerMu.RLock()
	defer providerMu.RUnlock()
	out := make([]ProviderAdapter, len(providerAdapters))
	copy(out, providerAdapters)
	return out
}

// ApplyProviderAdapters fills unset Config fields from registered vendor adapters.
func ApplyProviderAdapters(cfg *Config) {
	if cfg == nil {
		return
	}
	adapters := snapshotProviderAdapters()
	for _, a := range adapters {
		if a.Match(cfg.Provider, cfg.BaseURL) {
			a.FillDefaults(cfg)
		}
	}
	if cfg.BaseURL == "" {
		for _, a := range adapters {
			d, ok := a.(Detector)
			if ok && d.Detect(cfg) {
				a.FillDefaults(cfg)
				break
			}
		}
	}
	for _, a := range adapters {
		if a.Match(cfg.Provider, cfg.BaseURL) {
			a.FillDefaults(cfg)
		}
	}
}

// IsPublicCloudBaseURL reports whether a registered vendor adapter claims this URL.
func IsPublicCloudBaseURL(baseURL string) bool {
	for _, a := range snapshotProviderAdapters() {
		if c, ok := a.(PublicCloudChecker); ok && c.IsPublicCloud(baseURL) {
			return true
		}
	}
	u := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return strings.HasPrefix(u, "https://api.openai.com") ||
		strings.HasPrefix(u, "https://generativelanguage.googleapis.com")
}

// ProviderIDFor picks a registered adapter ID from endpoint/model labels.
func ProviderIDFor(endpointType, modelID string) string {
	blob := strings.ToLower(strings.TrimSpace(endpointType) + " " + strings.TrimSpace(modelID))
	for _, a := range snapshotProviderAdapters() {
		id := strings.ToLower(strings.TrimSpace(a.ID()))
		if id != "" && strings.Contains(blob, id) {
			return a.ID()
		}
		if a.Match(id, "") && strings.Contains(blob, id) {
			return a.ID()
		}
	}
	return ""
}
