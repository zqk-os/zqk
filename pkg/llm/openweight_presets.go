package llm

import (
	"fmt"
	"strings"
)

const (
	// OpenAI-compatible open-weight inference providers (Engine 3 Category 1 Ecosystem Playbook).
	EngineTogether   ProviderEngineType = "together"
	EngineFireworks  ProviderEngineType = "fireworks"
	EngineBaseten    ProviderEngineType = "baseten"
	EngineGroq       ProviderEngineType = "groq"
	EngineOpenRouter ProviderEngineType = "openrouter"
	EngineDeepSeek   ProviderEngineType = "deepseek"
)

// OpenWeightProviderPreset defines a canonical OpenAI-compatible open-weight inference endpoint
// tuned to pair with ZQK's token-budgeted zgrep + Knowledge Kernel workflows.
type OpenWeightProviderPreset struct {
	EngineType       ProviderEngineType `json:"engine_type" yaml:"engine_type"`
	DisplayName      string             `json:"display_name" yaml:"display_name"`
	BaseURL          string             `json:"base_url" yaml:"base_url"`
	AuthEnvVar       string             `json:"auth_env_var,omitempty" yaml:"auth_env_var,omitempty"`
	DefaultModel     string             `json:"default_model" yaml:"default_model"`
	MaxContextTokens int                `json:"max_context_tokens" yaml:"max_context_tokens"`
	SupportsTools    bool               `json:"supports_tools" yaml:"supports_tools"`
	LocalAirGapped   bool               `json:"local_air_gapped" yaml:"local_air_gapped"`
}

// ToEndpointProfile converts a preset into a validated EndpointProfile for EngineRegistry.
func (p OpenWeightProviderPreset) ToEndpointProfile() EndpointProfile {
	return EndpointProfile{
		EngineType:       p.EngineType,
		BaseURL:          p.BaseURL,
		DefaultModel:     p.DefaultModel,
		MaxContextTokens: p.MaxContextTokens,
		SupportsTools:    p.SupportsTools,
	}
}

// OpenWeightPresets returns the canonical list of OpenAI-compatible open-weight inference presets
// (Ollama local air-gapped, Together AI, Fireworks AI, Baseten, and Groq).
func OpenWeightPresets() []OpenWeightProviderPreset {
	return []OpenWeightProviderPreset{
		{
			EngineType:       EngineOllama,
			DisplayName:      "Ollama (Local Air-Gapped)",
			BaseURL:          "http://127.0.0.1:11434/v1",
			DefaultModel:     "qwen2.5-coder:32b",
			MaxContextTokens: 32768,
			SupportsTools:    true,
			LocalAirGapped:   true,
		},
		{
			EngineType:       EngineTogether,
			DisplayName:      "Together AI",
			BaseURL:          "https://api.together.xyz/v1",
			AuthEnvVar:       "TOGETHER_API_KEY",
			DefaultModel:     "Qwen/Qwen2.5-Coder-32B-Instruct",
			MaxContextTokens: 32768,
			SupportsTools:    true,
			LocalAirGapped:   false,
		},
		{
			EngineType:       EngineFireworks,
			DisplayName:      "Fireworks AI",
			BaseURL:          "https://api.fireworks.ai/inference/v1",
			AuthEnvVar:       "FIREWORKS_API_KEY",
			DefaultModel:     "accounts/fireworks/models/qwen2p5-coder-32b-instruct",
			MaxContextTokens: 32768,
			SupportsTools:    true,
			LocalAirGapped:   false,
		},
		{
			EngineType:       EngineBaseten,
			DisplayName:      "Baseten",
			BaseURL:          "https://inference.baseten.co/v1",
			AuthEnvVar:       "BASETEN_API_KEY",
			DefaultModel:     "deepseek-ai/DeepSeek-R1-Distill-Qwen-32B",
			MaxContextTokens: 32768,
			SupportsTools:    true,
			LocalAirGapped:   false,
		},
		{
			EngineType:       EngineGroq,
			DisplayName:      "Groq",
			BaseURL:          "https://api.groq.com/openai/v1",
			AuthEnvVar:       "GROQ_API_KEY",
			DefaultModel:     "llama-3.3-70b-versatile",
			MaxContextTokens: 32768,
			SupportsTools:    true,
			LocalAirGapped:   false,
		},
		{
			EngineType:       EngineOpenRouter,
			DisplayName:      "OpenRouter",
			BaseURL:          "https://openrouter.ai/api/v1",
			AuthEnvVar:       "OPENROUTER_API_KEY",
			DefaultModel:     "qwen/qwen-2.5-coder-32b-instruct",
			MaxContextTokens: 32768,
			SupportsTools:    true,
			LocalAirGapped:   false,
		},
		{
			EngineType:       EngineDeepSeek,
			DisplayName:      "DeepSeek",
			BaseURL:          "https://api.deepseek.com/v1",
			AuthEnvVar:       "DEEPSEEK_API_KEY",
			DefaultModel:     "deepseek-chat",
			MaxContextTokens: 65536,
			SupportsTools:    true,
			LocalAirGapped:   false,
		},
	}
}

// LookupOpenWeightPreset looks up a preset by case-insensitive engine identifier.
func LookupOpenWeightPreset(engine ProviderEngineType) (OpenWeightProviderPreset, bool) {
	needle := ProviderEngineType(strings.ToLower(strings.TrimSpace(string(engine))))
	for _, p := range OpenWeightPresets() {
		if p.EngineType == needle {
			return p, true
		}
	}
	return OpenWeightProviderPreset{}, false
}

// RegisterOpenWeightPresets registers all canonical open-weight provider presets into reg.
func RegisterOpenWeightPresets(reg *EngineRegistry) error {
	if reg == nil {
		return fmt.Errorf("engine registry is required")
	}
	for _, preset := range OpenWeightPresets() {
		if err := reg.Register(preset.ToEndpointProfile()); err != nil {
			return fmt.Errorf("failed registering open-weight preset %s: %w", preset.EngineType, err)
		}
	}
	return nil
}
