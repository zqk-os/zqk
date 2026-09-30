package validation

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestDefaultValidationTierConfig(t *testing.T) {
	t.Parallel()
	config := DefaultValidationTierConfig()

	// Check default blocking tiers
	if len(config.BlockingTiers) != 2 {
		t.Errorf("Expected 2 blocking tiers, got %d", len(config.BlockingTiers))
	}
	if config.BlockingTiers[0] != 1 || config.BlockingTiers[1] != 2 {
		t.Errorf("Expected blocking tiers [1, 2], got %v", config.BlockingTiers)
	}

	// Check rule mappings
	if config.GetTierForRule("minCount") != 1 {
		t.Errorf("Expected minCount to be tier 1, got %d", config.GetTierForRule("minCount"))
	}
	if config.GetTierForRule("pattern") != 2 {
		t.Errorf("Expected pattern to be tier 2, got %d", config.GetTierForRule("pattern"))
	}
	if config.GetTierForRule("min_length") != 3 {
		t.Errorf("Expected min_length to be tier 3, got %d", config.GetTierForRule("min_length"))
	}
}

func TestIsBlockingTier(t *testing.T) {
	t.Parallel()
	config := DefaultValidationTierConfig()

	// Tier 1 and 2 should block
	if !config.IsBlockingTier(1) {
		t.Error("Tier 1 should be blocking")
	}
	if !config.IsBlockingTier(2) {
		t.Error("Tier 2 should be blocking")
	}

	// Tier 3 and 4 should not block
	if config.IsBlockingTier(3) {
		t.Error("Tier 3 should not be blocking")
	}
	if config.IsBlockingTier(4) {
		t.Error("Tier 4 should not be blocking")
	}
}

func TestGetBlockingErrors(t *testing.T) {
	t.Parallel()
	config := DefaultValidationTierConfig()

	errors := []ValidationError{
		{Field: "id", Message: "Required field missing", Rule: "minCount"},
		{Field: "title", Message: "Pattern mismatch", Rule: "pattern"},
		{Field: "description", Message: "Length too short", Rule: "min_length"},
	}

	blockingErrors := config.GetBlockingErrors(errors)
	if len(blockingErrors) != 2 {
		t.Errorf("Expected 2 blocking errors, got %d", len(blockingErrors))
	}

	// Check that minCount (tier 1) and pattern (tier 2) are blocking
	foundMinCount := false
	foundPattern := false
	for _, err := range blockingErrors {
		if err.Rule == "minCount" {
			foundMinCount = true
		}
		if err.Rule == "pattern" {
			foundPattern = true
		}
	}
	if !foundMinCount {
		t.Error("Expected minCount error to be blocking")
	}
	if !foundPattern {
		t.Error("Expected pattern error to be blocking")
	}

	nonBlockingErrors := config.GetNonBlockingErrors(errors)
	if len(nonBlockingErrors) != 1 {
		t.Errorf("Expected 1 non-blocking error, got %d", len(nonBlockingErrors))
	}
	if nonBlockingErrors[0].Rule != "min_length" {
		t.Errorf("Expected min_length to be non-blocking, got %s", nonBlockingErrors[0].Rule)
	}
}

func TestLoadValidationTierConfig(t *testing.T) {
	t.Parallel()
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, paths.ConfigDir, paths.ZqkConfigFileName)
	if err := fileutil.MkdirAll(filepath.Dir(configPath), paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	configYAML := `
validation:
  tier_config:
    blocking_tiers: [1]
    rule_to_tier_mapping:
      minCount: 1
      pattern: 3
`
	if err := fileutil.WriteFile(configPath, []byte(configYAML), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	config, err := LoadValidationTierConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Check that blocking tiers are loaded
	if len(config.BlockingTiers) != 1 {
		t.Errorf("Expected 1 blocking tier, got %d", len(config.BlockingTiers))
	}
	if config.BlockingTiers[0] != 1 {
		t.Errorf("Expected blocking tier [1], got %v", config.BlockingTiers)
	}

	// Check that custom rule mapping is loaded
	if config.GetTierForRule("pattern") != 3 {
		t.Errorf("Expected pattern to be tier 3 from config, got %d", config.GetTierForRule("pattern"))
	}

	// Check that default mappings are still present
	if config.GetTierForRule("minCount") != 1 {
		t.Errorf("Expected minCount to be tier 1, got %d", config.GetTierForRule("minCount"))
	}
}

func TestFormatBlockingErrors(t *testing.T) {
	t.Parallel()
	config := DefaultValidationTierConfig()

	errors := []ValidationError{
		{Field: "id", Message: "Required field missing", Rule: "minCount"},
		{Field: "title", Message: "Pattern mismatch", Rule: "pattern"},
	}

	formatted := config.FormatBlockingErrors(errors)
	if formatted == emptyValue {
		t.Error("Expected formatted error message, got empty string")
	}
	if len(formatted) < 50 {
		t.Errorf("Expected longer formatted message, got: %s", formatted)
	}
}
