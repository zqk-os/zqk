package router

import (
	"strings"
	"sync"
)

// Valid reports whether the ModelTier is one of the recognized tiers.
func (t ModelTier) Valid() bool {
	switch t {
	case TierFast, TierStandard, TierHeavy, TierLocal:
		return true
	default:
		return false
	}
}

// String returns the string representation of the model tier.
func (t ModelTier) String() string {
	return string(t)
}

var (
	tierClassifierMu sync.RWMutex
	customTierMap    = make(map[string]ModelTier)
)

// RegisterModelTier maps an exact model identifier to a specific tier.
func RegisterModelTier(model string, tier ModelTier) {
	tierClassifierMu.Lock()
	defer tierClassifierMu.Unlock()
	customTierMap[strings.ToLower(strings.TrimSpace(model))] = tier
}

// ClearModelTierOverrides removes all registered tier overrides (useful in testing).
func ClearModelTierOverrides() {
	tierClassifierMu.Lock()
	defer tierClassifierMu.Unlock()
	customTierMap = make(map[string]ModelTier)
}

// FastModelSubstrings are substrings that indicate a fast/low-latency model tier.
var FastModelSubstrings = []string{
	"flash", "haiku", "small", "turbo", "fast", "3.5-turbo",
}

// HeavyModelSubstrings are substrings that indicate a deep-reasoning or heavy tier.
var HeavyModelSubstrings = []string{
	"o1", "o3", "opus", "reasoning", "thinking", "r1", "qwq",
}

// LocalModelSubstrings are substrings that indicate a local/air-gapped tier.
var LocalModelSubstrings = []string{
	"ollama", "local", "llama", "deepseek-local", "qwen-local", "mistral-local", "phi",
}

// ClassifyModelTier determines the ModelTier for a model name using explicit overrides,
// known keyword heuristics, or falls back to TierStandard.
func ClassifyModelTier(model string) ModelTier {
	trimmed := strings.ToLower(strings.TrimSpace(model))
	if trimmed == "" {
		return TierStandard
	}

	tierClassifierMu.RLock()
	if tier, found := customTierMap[trimmed]; found {
		tierClassifierMu.RUnlock()
		return tier
	}
	tierClassifierMu.RUnlock()

	// Direct local prefixes
	if strings.HasPrefix(trimmed, "ollama/") || strings.HasPrefix(trimmed, "local/") {
		return TierLocal
	}

	// Fast indicators with priority (e.g. hosted turbo variants of open models)
	if strings.Contains(trimmed, "turbo") || strings.Contains(trimmed, "flash") || strings.Contains(trimmed, "haiku") {
		return TierFast
	}

	// Reasoning / Heavy models
	for _, sub := range HeavyModelSubstrings {
		if strings.Contains(trimmed, sub) {
			return TierHeavy
		}
	}

	// Local model families
	for _, sub := range LocalModelSubstrings {
		if strings.Contains(trimmed, sub) {
			return TierLocal
		}
	}

	// Fast models (mini / small / fast), taking care not to misclassify "gemini"
	if isFastSubstring(trimmed) {
		return TierFast
	}

	return TierStandard
}

func isFastSubstring(s string) bool {
	if strings.Contains(s, "4o-mini") || strings.Contains(s, "mini") && !strings.Contains(s, "gemini") {
		return true
	}
	for _, sub := range FastModelSubstrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
