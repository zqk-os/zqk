package validation

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ValidationTierConfig represents configuration for validation tier blocking
type ValidationTierConfig struct {
	// BlockingTiers specifies which tiers should block object persistence
	// Default: [1, 2] (blocking and warning tiers)
	BlockingTiers []int `yaml:"blocking_tiers"`

	// RuleToTierMapping maps validation rules to tiers
	// This allows custom tier assignment for different validation rules
	RuleToTierMapping map[string]int `yaml:"rule_to_tier_mapping"`
}

// DefaultValidationTierConfig returns the default tier configuration
func DefaultValidationTierConfig() *ValidationTierConfig {
	return &ValidationTierConfig{
		BlockingTiers: []int{1, 2}, // Block tiers 1 (blocking) and 2 (warning)
		RuleToTierMapping: map[string]int{
			// Tier 1: Critical blocking errors
			"minCount":           1, // Required field missing
			"required":           1, // Required field missing (alias)
			"datatype":           1, // Type mismatch
			objects.FieldKeyType: 1, // Type mismatch (alias)
			"reference":          1, // Reference validation failed
			"partial_data":       1, // Partial/incomplete data
			// validated/exploring×active|in_progress plan — was tier-2 warning and invisible to
			// pristine glances that only read blocking_issues / error_status_objects.
			// TRACK: BLI-KERNEL-CHECK-UNBLIND-MEMBERSHIP-001
			"execution_facing_membership": 1,
			"scope_creep_protection":      1,

			// Tier 2: Warnings that should block saves
			"pattern":         2, // Pattern mismatch
			"in":              2, // Enum value invalid
			"enum":            2, // Enum value invalid (alias)
			"scope_integrity": 2, // Discourage scope changes in active plans

			// Tier 3: Informational (allowed)
			"min_length":    3,
			"max_length":    3,
			"semantic_type": 3, "recommendation": // Tier 4: Recommendations (allowed)
			4,
		},
	}
}

// LoadValidationTierConfig loads tier configuration from file
func LoadValidationTierConfig(configPath string) (*ValidationTierConfig, error) {
	if configPath == emptyValue {
		configPath = findValidationTierConfig()
		if configPath == emptyValue {
			return DefaultValidationTierConfig(), nil
		}
	}

	cfg, err := tierConfigs.Load(configPath, stampmemo.Of(configPath), func() (*ValidationTierConfig, error) {
		return parseValidationTierConfigFile(configPath), nil
	})
	if err != nil || cfg == nil {
		return DefaultValidationTierConfig(), nil
	}
	return cloneValidationTierConfig(cfg), nil
}

func parseValidationTierConfigFile(configPath string) *ValidationTierConfig {
	data, err := fileutil.ReadFile(configPath)
	if err != nil {
		return DefaultValidationTierConfig()
	}

	var config struct {
		Validation struct {
			TierConfig *ValidationTierConfig `yaml:"tier_config"`
		} `yaml:"validation"`
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return DefaultValidationTierConfig()
	}

	if config.Validation.TierConfig == nil {
		return DefaultValidationTierConfig()
	}

	tierConfig := config.Validation.TierConfig
	if len(tierConfig.BlockingTiers) == 0 {
		tierConfig.BlockingTiers = DefaultValidationTierConfig().BlockingTiers
	}
	if len(tierConfig.RuleToTierMapping) == 0 {
		tierConfig.RuleToTierMapping = DefaultValidationTierConfig().RuleToTierMapping
	} else {
		defaultMapping := DefaultValidationTierConfig().RuleToTierMapping
		for rule, tier := range defaultMapping {
			if _, exists := tierConfig.RuleToTierMapping[rule]; !exists {
				tierConfig.RuleToTierMapping[rule] = tier
			}
		}
	}

	return tierConfig
}

// GetGlobalValidationTierConfig returns the tier config for the discovered file.
func GetGlobalValidationTierConfig() *ValidationTierConfig {
	cfg, err := LoadValidationTierConfig(findValidationTierConfig())
	if err != nil || cfg == nil {
		return DefaultValidationTierConfig()
	}
	return cfg
}

// findValidationTierConfig searches for validation tier config file
func findValidationTierConfig() string {
	return paths.FirstExistingFromCwdAny(paths.ProjectYAMLConfigRelatives())
}

// GetTierForRule returns the tier for a validation rule
func (c *ValidationTierConfig) GetTierForRule(rule string) int {
	if tier, ok := c.RuleToTierMapping[rule]; ok {
		return tier
	}
	// Default to tier 2 (warning) for unknown rules
	return 2
}

// IsBlockingTier checks if a tier should block object persistence
func (c *ValidationTierConfig) IsBlockingTier(tier int) bool {
	for _, blockingTier := range c.BlockingTiers {
		if blockingTier == tier {
			return true
		}
	}
	return false
}

// GetBlockingErrors filters validation errors to only those that should block saves
func (c *ValidationTierConfig) GetBlockingErrors(errors []ValidationError) []ValidationError {
	var blockingErrors []ValidationError
	for _, err := range errors {
		tier := c.GetTierForRule(err.Rule)
		if c.IsBlockingTier(tier) {
			blockingErrors = append(blockingErrors, err)
		}
	}
	return blockingErrors
}

// GetNonBlockingErrors filters validation errors to only those that should NOT block saves
func (c *ValidationTierConfig) GetNonBlockingErrors(errors []ValidationError) []ValidationError {
	var nonBlockingErrors []ValidationError
	for _, err := range errors {
		tier := c.GetTierForRule(err.Rule)
		if !c.IsBlockingTier(tier) {
			nonBlockingErrors = append(nonBlockingErrors, err)
		}
	}
	return nonBlockingErrors
}

// FormatBlockingErrors formats blocking errors with tier information
func (c *ValidationTierConfig) FormatBlockingErrors(errors []ValidationError) string {
	var messages []string
	for _, err := range errors {
		tier := c.GetTierForRule(err.Rule)
		messages = append(messages, fmt.Sprintf("%s (Tier %d): %s", err.Field, tier, err.Message))
	}
	return fmt.Sprintf("blocking validation errors: %s", fmt.Sprintf("%v", messages))
}
