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
		t.Errorf(ConstMagic06e3afcd, len(config.BlockingTiers))
	}
	if config.BlockingTiers[0] != 1 || config.BlockingTiers[1] != 2 {
		t.Errorf(ConstMagicc0c012bd, config.BlockingTiers)
	}

	// Check rule mappings
	if config.GetTierForRule("minCount") != 1 {
		t.Errorf(ConstMagic6d27d005, config.GetTierForRule("minCount"))
	}
	if config.GetTierForRule("pattern") != 2 {
		t.Errorf(ConstMagicb1008fc8, config.GetTierForRule("pattern"))
	}
	if config.GetTierForRule("min_length") != 3 {
		t.Errorf(ConstMagic4cc46d9a, config.GetTierForRule("min_length"))
	}
}

func TestIsBlockingTier(t *testing.T) {
	t.Parallel()
	config := DefaultValidationTierConfig()

	// Tier 1 and 2 should block
	if !config.IsBlockingTier(1) {
		t.Error(ConstMagicfc8bddf9)
	}
	if !config.IsBlockingTier(2) {
		t.Error(ConstMagic1228458f)
	}

	// Tier 3 and 4 should not block
	if config.IsBlockingTier(3) {
		t.Error(ConstMagica6eb18c1)
	}
	if config.IsBlockingTier(4) {
		t.Error(ConstMagic58ae0814)
	}
}

func TestGetBlockingErrors(t *testing.T) {
	t.Parallel()
	config := DefaultValidationTierConfig()

	errors := []ValidationError{
		{Field: "id", Message: ConstMagic0b1d0c0a, Rule: "minCount"},
		{Field: "title", Message: ConstMagicb01de315, Rule: "pattern"},
		{Field: "description", Message: ConstMagicb225b2d6, Rule: "min_length"},
	}

	blockingErrors := config.GetBlockingErrors(errors)
	if len(blockingErrors) != 2 {
		t.Errorf(ConstMagic10c6c8b7, len(blockingErrors))
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
		t.Error(ConstMagic3f643923)
	}
	if !foundPattern {
		t.Error(ConstMagice2712b5a)
	}

	nonBlockingErrors := config.GetNonBlockingErrors(errors)
	if len(nonBlockingErrors) != 1 {
		t.Errorf(ConstMagic49e4937f, len(nonBlockingErrors))
	}
	if nonBlockingErrors[0].Rule != "min_length" {
		t.Errorf(ConstMagicb86f4254, nonBlockingErrors[0].Rule)
	}
}

func TestLoadValidationTierConfig(t *testing.T) {
	t.Parallel()
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, paths.ProjectDataDir, "config", "config.yaml")
	if err := fileutil.MkdirAll(filepath.Dir(configPath), paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagicec23442c, err)
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
		t.Fatalf(ConstMagic8e96c016, err)
	}

	config, err := LoadValidationTierConfig(configPath)
	if err != nil {
		t.Fatalf(ConstMagica695d246, err)
	}

	// Check that blocking tiers are loaded
	if len(config.BlockingTiers) != 1 {
		t.Errorf(ConstMagicb1e93fb4, len(config.BlockingTiers))
	}
	if config.BlockingTiers[0] != 1 {
		t.Errorf(ConstMagic96a5ea9f, config.BlockingTiers)
	}

	// Check that custom rule mapping is loaded
	if config.GetTierForRule("pattern") != 3 {
		t.Errorf(ConstMagic977f01af, config.GetTierForRule("pattern"))
	}

	// Check that default mappings are still present
	if config.GetTierForRule("minCount") != 1 {
		t.Errorf(ConstMagic6d27d005, config.GetTierForRule("minCount"))
	}
}

func TestFormatBlockingErrors(t *testing.T) {
	t.Parallel()
	config := DefaultValidationTierConfig()

	errors := []ValidationError{
		{Field: "id", Message: ConstMagic0b1d0c0a, Rule: "minCount"},
		{Field: "title", Message: ConstMagicb01de315, Rule: "pattern"},
	}

	formatted := config.FormatBlockingErrors(errors)
	if formatted == emptyValue {
		t.Error(ConstMagic2291a87d)
	}
	if len(formatted) < 50 {
		t.Errorf(ConstMagic5a3f2fee, formatted)
	}
}
