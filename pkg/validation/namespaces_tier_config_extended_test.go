package validation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNamespacesConfig_Helpers(t *testing.T) {
	ResetGlobalNamespacesConfig()

	// Default config
	def := getDefaultNamespacesConfig()
	if def == nil {
		t.Fatal("expected default namespaces config to be non-nil")
	}
	if def.DefaultNamespace == "" {
		t.Errorf("expected non-empty default namespace")
	}

	// GetAllKindsForNamespace
	kinds := def.GetAllKindsForNamespace("zqk:kernel")
	if len(kinds) == 0 {
		t.Errorf("expected non-empty kinds for namespace zqk:kernel")
	}

	// Nonexistent namespace
	emptyKinds := def.GetAllKindsForNamespace("nonexistent_ns")
	if len(emptyKinds) != 0 {
		t.Errorf("expected empty kinds for nonexistent namespace, got %v", emptyKinds)
	}

	// Validation pattern
	pattern := def.GetNamespaceValidationPattern()
	if pattern == "" {
		t.Errorf("expected non-empty validation pattern")
	}

	// Global config getter
	g := GetGlobalNamespacesConfig()
	if g == nil {
		t.Fatal("expected global namespaces config to be non-nil")
	}

	// Load from non-existent file falls back to default
	loaded, err := LoadNamespacesConfig("/nonexistent/path/namespaces.yaml")
	if err != nil {
		t.Fatalf("unexpected error loading non-existent config: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected loaded config to fall back to default, got nil")
	}
}

func TestValidationTierConfig_Helpers(t *testing.T) {
	// DefaultValidationTierConfig
	def := DefaultValidationTierConfig()
	if def == nil {
		t.Fatal("expected default tier config to be non-nil")
	}
	if len(def.BlockingTiers) == 0 {
		t.Errorf("expected blocking tiers in default tier config")
	}
	if len(def.RuleToTierMapping) == 0 {
		t.Errorf("expected rule mappings in default tier config")
	}

	// LoadValidationTierConfig from non-existent file
	loaded, err := LoadValidationTierConfig("/nonexistent/path/tiers.yaml")
	if err != nil {
		t.Fatalf("unexpected error loading non-existent tier config: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil default tier config fallback")
	}

	// Load from valid YAML content
	tmpDir := t.TempDir()
	tierFile := filepath.Join(tmpDir, "tiers.yaml")
	yamlContent := `
validation:
  tier_config:
    blocking_tiers: [1, 2, 3]
    rule_to_tier_mapping:
      required: 1
      pattern: 2
`
	if err := os.WriteFile(tierFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write temp tier file: %v", err)
	}

	loadedCustom, err := LoadValidationTierConfig(tierFile)
	if err != nil {
		t.Fatalf("failed to load custom tier file: %v", err)
	}
	if len(loadedCustom.BlockingTiers) != 3 {
		t.Errorf("expected 3 blocking tiers, got %v", loadedCustom.BlockingTiers)
	}
}
