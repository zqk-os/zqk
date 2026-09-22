package validation

import (
	"testing"
)

func TestIDPrefixesConfig_SynonymsAndInference(t *testing.T) {
	ResetGlobalIDPrefixesConfig()

	cfg := getDefaultIDPrefixesConfig()
	if cfg == nil {
		t.Fatal("expected default id prefixes config")
	}

	// HasExplicitMapping
	if !cfg.HasExplicitMapping("backlog_item") {
		t.Errorf("expected explicit mapping for backlog_item")
	}
	if cfg.HasExplicitMapping("completely_custom_unknown_kind") {
		t.Errorf("expected false for nonexistent kind explicit mapping")
	}

	// GetPrefixesForKind with inference
	prefs := cfg.GetPrefixesForKind("feature_item")
	if len(prefs) == 0 || prefs[0] != "FEA-" {
		t.Errorf("expected FEA- prefix for feature_item, got %v", prefs)
	}

	// SynonymProviderAdapter
	adapter := NewSynonymProviderAdapter(cfg)
	syns := adapter.GetSynonymsForKind("backlog_item")
	if len(syns) == 0 {
		t.Errorf("expected synonyms for backlog_item from adapter")
	}

	nilAdapter := NewSynonymProviderAdapter(nil)
	if len(nilAdapter.GetSynonymsForKind("backlog_item")) != 0 {
		t.Errorf("expected empty synonyms for nil config adapter")
	}

	// Custom inference rules for synonyms
	customCfg := &IDPrefixesConfig{
		InferenceRules: InferenceRulesConfig{
			SynonymPatterns: []SynonymInferencePattern{
				{
					Pattern:    "*_item",
					Strategies: []string{"first_letters", "first_4"},
				},
				{
					Pattern:    "two_words",
					Strategies: []string{"abbrev"},
				},
			},
		},
	}
	synsInferred := customCfg.GetSynonymsForKind("custom_item")
	if len(synsInferred) == 0 {
		t.Errorf("expected inferred synonyms for custom_item, got %v", synsInferred)
	}

	// matchPattern
	matched, err := matchPattern("*_suffix", "test_suffix")
	if err != nil || !matched {
		t.Errorf("expected match for suffix wildcard, got %v, %v", matched, err)
	}
	matched, err = matchPattern("prefix_*", "prefix_test")
	if err != nil || !matched {
		t.Errorf("expected match for prefix wildcard, got %v, %v", matched, err)
	}
	matched, err = matchPattern("exact", "exact")
	if err != nil || !matched {
		t.Errorf("expected match for exact, got %v, %v", matched, err)
	}
	matched, err = matchPattern("exact", "different")
	if err != nil || matched {
		t.Errorf("expected no match, got %v, %v", matched, err)
	}

	// isHexString
	if !isHexString("0123456789abcdefABCDEF") {
		t.Errorf("expected valid hex string")
	}
	if isHexString("xyz") {
		t.Errorf("expected false for non-hex string")
	}
}
