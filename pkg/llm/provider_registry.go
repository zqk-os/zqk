package llm

import (
	"errors"
	"fmt"
	"net/url"
	"sync"
)

// ProviderEngineType denotes the underlying local inference serving architecture.
type ProviderEngineType string

const (
	EngineOllama      ProviderEngineType = "ollama"
	EngineVLLM        ProviderEngineType = "vllm"
	EngineLlamaServer ProviderEngineType = "llama-server"
	EngineCustomLocal ProviderEngineType = "custom-local"
)

// EndpointProfile holds configuration, capabilities, and telemetry for an inference provider.
type EndpointProfile struct {
	EngineType       ProviderEngineType `json:"engine_type"`
	BaseURL          string             `json:"base_url"`
	DefaultModel     string             `json:"default_model"`
	DiscoveredModels []string           `json:"discovered_models,omitempty"`
	MaxContextTokens int                `json:"max_context_tokens"`
	SupportsTools    bool               `json:"supports_tools"`
	IsHealthy        bool               `json:"is_healthy"`
	LatencyMs        int64              `json:"latency_ms"`
}

// Validate ensures an EndpointProfile adheres to operational constraints.
func (p *EndpointProfile) Validate() error {
	if p.EngineType == "" {
		return errors.New("engine_type is required")
	}
	if p.BaseURL == "" {
		return errors.New("base_url is required")
	}
	u, err := url.Parse(p.BaseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("invalid base_url %q: must include scheme and host", p.BaseURL)
	}
	if p.MaxContextTokens <= 0 {
		p.MaxContextTokens = 4096 // Safe default
	}
	return nil
}

// EngineRegistry provides thread-safe management and lookup for inference engines.
type EngineRegistry struct {
	mu       sync.RWMutex
	profiles map[ProviderEngineType]EndpointProfile
}

// NewEngineRegistry constructs an initialized EngineRegistry.
func NewEngineRegistry() *EngineRegistry {
	return &EngineRegistry{
		profiles: make(map[ProviderEngineType]EndpointProfile),
	}
}

// Register adds or updates an engine profile in the registry.
func (r *EngineRegistry) Register(profile EndpointProfile) error {
	if err := profile.Validate(); err != nil {
		return fmt.Errorf("registry: failed validating profile: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.profiles[profile.EngineType] = profile
	return nil
}

// Get retrieves an engine profile by engine type.
func (r *EngineRegistry) Get(engineType ProviderEngineType) (EndpointProfile, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.profiles[engineType]
	return p, ok
}

// List returns a snapshot of all registered profiles.
func (r *EngineRegistry) List() []EndpointProfile {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]EndpointProfile, 0, len(r.profiles))
	for _, p := range r.profiles {
		out = append(out, p)
	}
	return out
}

// BestAvailable selects the highest-capability healthy local engine profile.
func (r *EngineRegistry) BestAvailable() (EndpointProfile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Prioritize: vLLM -> LlamaServer -> Ollama -> CustomLocal
	order := []ProviderEngineType{
		EngineVLLM,
		EngineLlamaServer,
		EngineOllama,
		EngineCustomLocal,
	}

	for _, eng := range order {
		if p, ok := r.profiles[eng]; ok && p.IsHealthy {
			return p, nil
		}
	}

	// Fallback to any healthy profile
	for _, p := range r.profiles {
		if p.IsHealthy {
			return p, nil
		}
	}

	return EndpointProfile{}, errors.New("no healthy inference engines available in registry")
}
