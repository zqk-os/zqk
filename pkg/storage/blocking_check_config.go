package storage

import (
	"maps"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

// BlockingCheckConfig represents the configuration for blocking check bypass rules
// and validation tier blocking rules. This is highly customizable for different workflow configurations.
type BlockingCheckConfig struct {
	Version     string `yaml:"version"`
	Description string `yaml:"description"`

	// WorkflowProfile specifies the workflow profile to use (strict, moderate, lenient, custom)
	// This allows different projects to use different blocking strategies
	WorkflowProfile string `yaml:"workflow_profile,omitempty"`

	// BypassKinds lists object kinds that should bypass blocking checks entirely
	BypassKinds    []string `yaml:"bypass_kinds"`
	BypassPatterns []string `yaml:"bypass_patterns,omitempty"` // Future: regex patterns

	// PerKindRules allows custom blocking rules for specific object kinds
	// This enables workflow-specific configurations (e.g., strict validation for requirements, lenient for drafts)
	PerKindRules map[string]KindBlockingRules `yaml:"per_kind_rules,omitempty"`

	// PerOperationRules allows different blocking behavior for create, update, delete operations
	PerOperationRules map[string]OperationBlockingRules `yaml:"per_operation_rules,omitempty"`

	// ValidationTierBlocking configures which validation error tiers should block saves
	// Default: [1, 2] (blocking and warning tiers)
	ValidationTierBlocking *ValidationTierBlockingConfig `yaml:"validation_tier_blocking,omitempty"`

	// SystemLevelBlocking controls whether to check for blocking issues in other objects
	// before allowing write operations (the existing blocking check behavior)
	SystemLevelBlocking *SystemLevelBlockingConfig `yaml:"system_level_blocking,omitempty"`
}

// KindBlockingRules defines blocking rules for a specific object kind
type KindBlockingRules struct {
	// Bypass indicates this kind should bypass blocking checks
	Bypass bool `yaml:"bypass,omitempty"`

	// BlockingTiersOverride overrides the default blocking tiers for this kind
	BlockingTiersOverride []int `yaml:"blocking_tiers_override,omitempty"`

	// AllowNonBlockingErrors allows saves even with Tier 3/4 errors for this kind
	AllowNonBlockingErrors bool `yaml:"allow_non_blocking_errors,omitempty"`

	// RequireSystemLevelCheck forces system-level blocking check even if bypassed globally
	RequireSystemLevelCheck bool `yaml:"require_system_level_check,omitempty"`
}

// OperationBlockingRules defines blocking rules for specific operations (create, update, delete)
type OperationBlockingRules struct {
	// BlockingTiersOverride overrides the default blocking tiers for this operation
	BlockingTiersOverride []int `yaml:"blocking_tiers_override,omitempty"`

	// RequireSystemLevelCheck forces system-level blocking check for this operation
	RequireSystemLevelCheck bool `yaml:"require_system_level_check,omitempty"`

	// AllowNonBlockingErrors allows saves even with Tier 3/4 errors for this operation
	AllowNonBlockingErrors bool `yaml:"allow_non_blocking_errors,omitempty"`
}

// ValidationTierBlockingConfig configures validation tier blocking behavior
type ValidationTierBlockingConfig struct {
	// BlockingTiers specifies which tiers should block object persistence
	// Default: [1, 2] (blocking and warning tiers)
	BlockingTiers []int `yaml:"blocking_tiers"`

	// RuleToTierMapping maps validation rules to tiers (optional, uses defaults if not specified)
	RuleToTierMapping map[string]int `yaml:"rule_to_tier_mapping,omitempty"`
}

// SystemLevelBlockingConfig configures system-level blocking check behavior
type SystemLevelBlockingConfig struct {
	// Enabled controls whether system-level blocking checks are performed
	Enabled bool `yaml:"enabled"`

	// CheckTiers specifies which tiers to check in other objects (default: [1])
	CheckTiers []int `yaml:"check_tiers,omitempty"`

	// BypassKinds lists kinds that bypass system-level checks
	BypassKinds []string `yaml:"bypass_kinds,omitempty"`
}

var blockingCheckConfigs stampmemo.Table[*BlockingCheckConfig] // keyed by blocking check YAML path (closed set)

// LoadBlockingCheckConfig loads the blocking check configuration from file
func LoadBlockingCheckConfig(configPath string) (*BlockingCheckConfig, error) {
	if configPath == emptyValue {
		configPath = findBlockingCheckConfig()
		if configPath == emptyValue {
			return getDefaultBlockingCheckConfig(), nil
		}
	}

	cfg, err := blockingCheckConfigs.Load(configPath, stampmemo.Of(configPath), func() (*BlockingCheckConfig, error) {
		return parseBlockingCheckConfigFile(configPath), nil
	})
	if err != nil || cfg == nil {
		return getDefaultBlockingCheckConfig(), nil
	}
	return cloneBlockingCheckConfig(cfg), nil
}

func parseBlockingCheckConfigFile(configPath string) *BlockingCheckConfig {
	data, err := fileutil.ReadFile(configPath)
	if err != nil {
		return getDefaultBlockingCheckConfig()
	}

	var config BlockingCheckConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return getDefaultBlockingCheckConfig()
	}

	config.validate()
	return &config
}

// GetGlobalBlockingCheckConfig returns the blocking check config for the discovered file.
func GetGlobalBlockingCheckConfig() *BlockingCheckConfig {
	cfg, err := LoadBlockingCheckConfig(findBlockingCheckConfig())
	if err != nil || cfg == nil {
		return getDefaultBlockingCheckConfig()
	}
	return cfg
}

// getDefaultBlockingCheckConfig returns default blocking check configuration
func getDefaultBlockingCheckConfig() *BlockingCheckConfig {
	return &BlockingCheckConfig{
		Version:     BlockingCheckConfigVersion,
		Description: ConstMiscDefaultBlockingCheckBypassConfiguration,
		BypassKinds: []string{
			objects.KindAuditEvent,
			objects.KindChangeJournalEntry,
			objects.KindSchedulerJob,
			objects.KindAuditAggregationMetric, // System-created aggregation metrics should bypass blocking checks
			// Note: backlog_item, requirement, and criteria are NOT bypassed by default
			// They should pass validation, but during transition periods, reference validation
			// may fail due to cache staleness. The blocking check will still run but won't
			// prevent writes if the check itself fails (see checkForBlockingIssuesBeforeWrite).
		},
	}
}

// validate validates the config and sets defaults if needed
func (c *BlockingCheckConfig) validate() {
	defaultConfig := getDefaultBlockingCheckConfig()

	// Set workflow profile if not specified
	if c.WorkflowProfile == emptyValue {
		c.WorkflowProfile = defaultConfig.WorkflowProfile
	}

	// Apply workflow profile presets if using a standard profile
	c.applyWorkflowProfile()

	// Ensure BypassKinds is initialized
	if c.BypassKinds == nil {
		c.BypassKinds = defaultConfig.BypassKinds
	}

	// Ensure ValidationTierBlocking is initialized
	if c.ValidationTierBlocking == nil {
		c.ValidationTierBlocking = defaultConfig.ValidationTierBlocking
	} else {
		// Merge with defaults for missing fields
		defaultTierConfig := defaultConfig.ValidationTierBlocking
		if len(c.ValidationTierBlocking.BlockingTiers) == 0 {
			c.ValidationTierBlocking.BlockingTiers = defaultTierConfig.BlockingTiers
		}
		if len(c.ValidationTierBlocking.RuleToTierMapping) == 0 {
			c.ValidationTierBlocking.RuleToTierMapping = defaultTierConfig.RuleToTierMapping
		} else {
			// Merge with defaults
			for rule, tier := range defaultTierConfig.RuleToTierMapping {
				if _, exists := c.ValidationTierBlocking.RuleToTierMapping[rule]; !exists {
					c.ValidationTierBlocking.RuleToTierMapping[rule] = tier
				}
			}
		}
	}

	// Ensure SystemLevelBlocking is initialized
	if c.SystemLevelBlocking == nil {
		c.SystemLevelBlocking = defaultConfig.SystemLevelBlocking
	} else {
		defaultSystemConfig := defaultConfig.SystemLevelBlocking
		if len(c.SystemLevelBlocking.CheckTiers) == 0 {
			c.SystemLevelBlocking.CheckTiers = defaultSystemConfig.CheckTiers
		}
		if c.SystemLevelBlocking.BypassKinds == nil {
			c.SystemLevelBlocking.BypassKinds = defaultSystemConfig.BypassKinds
		}
	}

	// Initialize PerKindRules if nil
	if c.PerKindRules == nil {
		c.PerKindRules = make(map[string]KindBlockingRules)
	}

	// Initialize PerOperationRules if nil
	if c.PerOperationRules == nil {
		c.PerOperationRules = make(map[string]OperationBlockingRules)
	}
}

// applyWorkflowProfile applies preset configurations based on workflow profile
func (c *BlockingCheckConfig) applyWorkflowProfile() {
	switch c.WorkflowProfile {
	case "strict":
		// Strict: Block on Tier 1, 2, and 3 errors, require system-level checks
		if c.ValidationTierBlocking == nil {
			c.ValidationTierBlocking = &ValidationTierBlockingConfig{}
		}
		if len(c.ValidationTierBlocking.BlockingTiers) == 0 {
			c.ValidationTierBlocking.BlockingTiers = []int{1, 2, 3}
		}
		if c.SystemLevelBlocking == nil {
			c.SystemLevelBlocking = &SystemLevelBlockingConfig{}
		}
		c.SystemLevelBlocking.Enabled = true
		c.SystemLevelBlocking.CheckTiers = []int{1, 2}

	case "moderate":
		// Moderate: Block on Tier 1 and 2, check Tier 1 system issues (default)
		// This is the default, so no changes needed

	case "lenient":
		// Lenient: Only block on Tier 1 errors, allow Tier 2-4
		if c.ValidationTierBlocking == nil {
			c.ValidationTierBlocking = &ValidationTierBlockingConfig{}
		}
		if len(c.ValidationTierBlocking.BlockingTiers) == 0 {
			c.ValidationTierBlocking.BlockingTiers = []int{1}
		}
		if c.SystemLevelBlocking == nil {
			c.SystemLevelBlocking = &SystemLevelBlockingConfig{}
		}
		c.SystemLevelBlocking.Enabled = true
		c.SystemLevelBlocking.CheckTiers = []int{1}

	case "custom":
		// Custom: Use explicit configuration, don't apply presets
		// User must specify all settings explicitly
		return

	default:
		// Unknown profile, use moderate (default)
		c.WorkflowProfile = "moderate"
	}
}

// GetTierForRule returns the tier for a validation rule
func (c *BlockingCheckConfig) GetTierForRule(rule string) int {
	if c.ValidationTierBlocking == nil {
		return 2 // Default to tier 2 for unknown rules
	}
	if tier, ok := c.ValidationTierBlocking.RuleToTierMapping[rule]; ok {
		return tier
	}
	return 2 // Default to tier 2 for unknown rules
}

// IsBlockingTier checks if a tier should block object persistence
// Takes into account per-kind and per-operation overrides
func (c *BlockingCheckConfig) IsBlockingTier(tier int, kind, operation string) bool {
	// Check per-kind override first
	if kind != emptyValue {
		if kindRules, ok := c.PerKindRules[kind]; ok {
			if len(kindRules.BlockingTiersOverride) > 0 {
				for _, blockingTier := range kindRules.BlockingTiersOverride {
					if blockingTier == tier {
						return true
					}
				}
				return false
			}
		}
	}

	// Check per-operation override
	if operation != emptyValue {
		if opRules, ok := c.PerOperationRules[operation]; ok {
			if len(opRules.BlockingTiersOverride) > 0 {
				for _, blockingTier := range opRules.BlockingTiersOverride {
					if blockingTier == tier {
						return true
					}
				}
				return false
			}
		}
	}

	// Use default blocking tiers
	if c.ValidationTierBlocking == nil {
		// Default: block tiers 1 and 2
		return tier == 1 || tier == 2
	}
	for _, blockingTier := range c.ValidationTierBlocking.BlockingTiers {
		if blockingTier == tier {
			return true
		}
	}
	return false
}

// ShouldBypassBlockingCheck checks if a kind should bypass blocking checks
// Enhanced to check per-kind rules in addition to global bypass kinds
func (c *BlockingCheckConfig) ShouldBypassBlockingCheck(kind string) bool {
	// Check per-kind bypass first
	if kind != emptyValue {
		if kindRules, ok := c.PerKindRules[kind]; ok {
			if kindRules.Bypass {
				return true
			}
		}
	}

	// Check global bypass kinds
	for _, bypassKind := range c.BypassKinds {
		if kind == bypassKind {
			return true
		}
	}

	return false
}

// ShouldRequireSystemLevelCheck checks if system-level blocking check is required
// Takes into account per-kind and per-operation rules
func (c *BlockingCheckConfig) ShouldRequireSystemLevelCheck(kind, operation string) bool {
	// Check if system-level blocking is enabled
	if c.SystemLevelBlocking == nil || !c.SystemLevelBlocking.Enabled {
		return false
	}

	// Check per-kind requirement
	if kind != emptyValue {
		if kindRules, ok := c.PerKindRules[kind]; ok {
			if kindRules.RequireSystemLevelCheck {
				return true
			}
		}
	}

	// Check per-operation requirement
	if operation != emptyValue {
		if opRules, ok := c.PerOperationRules[operation]; ok {
			if opRules.RequireSystemLevelCheck {
				return true
			}
		}
	}

	// Default: require if enabled and kind is not bypassed
	return !c.ShouldBypassBlockingCheck(kind)
}

// GetBlockingValidationErrors filters validation errors to only those that should block saves
// Takes into account per-kind and per-operation rules
func (c *BlockingCheckConfig) GetBlockingValidationErrors(errors []validation.ValidationError, kind, operation string) []validation.ValidationError {
	var blockingErrors []validation.ValidationError
	for _, err := range errors {
		tier := c.GetTierForRule(err.Rule)
		if c.IsBlockingTier(tier, kind, operation) {
			blockingErrors = append(blockingErrors, err)
		}
	}
	return blockingErrors
}

// findBlockingCheckConfig finds the blocking check config file
func findBlockingCheckConfig() string {
	return paths.FirstExistingFromCwd(filepath.Join(paths.ProcessInternalDir, paths.BlockingCheckConfigFile))
}

// GetBypassKinds returns the list of kinds that bypass blocking checks
func (c *BlockingCheckConfig) GetBypassKinds() []string {
	if c == nil {
		return nil
	}
	return slices.Clone(c.BypassKinds)
}

func cloneBlockingCheckConfig(in *BlockingCheckConfig) *BlockingCheckConfig {
	if in == nil {
		return nil
	}
	out := *in
	out.BypassKinds = slices.Clone(in.BypassKinds)
	out.BypassPatterns = slices.Clone(in.BypassPatterns)
	if in.PerKindRules != nil {
		out.PerKindRules = make(map[string]KindBlockingRules, len(in.PerKindRules))
		for k, v := range in.PerKindRules {
			v.BlockingTiersOverride = slices.Clone(v.BlockingTiersOverride)
			out.PerKindRules[k] = v
		}
	}
	if in.PerOperationRules != nil {
		out.PerOperationRules = make(map[string]OperationBlockingRules, len(in.PerOperationRules))
		for k, v := range in.PerOperationRules {
			v.BlockingTiersOverride = slices.Clone(v.BlockingTiersOverride)
			out.PerOperationRules[k] = v
		}
	}
	if in.ValidationTierBlocking != nil {
		tier := *in.ValidationTierBlocking
		tier.BlockingTiers = slices.Clone(in.ValidationTierBlocking.BlockingTiers)
		tier.RuleToTierMapping = maps.Clone(in.ValidationTierBlocking.RuleToTierMapping)
		out.ValidationTierBlocking = &tier
	}
	if in.SystemLevelBlocking != nil {
		sys := *in.SystemLevelBlocking
		sys.CheckTiers = slices.Clone(in.SystemLevelBlocking.CheckTiers)
		sys.BypassKinds = slices.Clone(in.SystemLevelBlocking.BypassKinds)
		out.SystemLevelBlocking = &sys
	}
	return &out
}
