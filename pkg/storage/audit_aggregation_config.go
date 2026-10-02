package storage

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var aggregationConfigs stampmemo.Table[AuditAggregationConfig] // keyed by projectRoot; stamp is config.yaml

// AuditAggregationConfig represents the audit aggregation configuration
type AuditAggregationConfig struct {
	Enabled    bool              `yaml:"enabled"`
	WindowSize string            `yaml:"window_size"` // e.g., "1h", "24h"
	Threshold  int               `yaml:"threshold"`
	Rules      []AggregationRule `yaml:"rules"`
}

// aggregationRuleYAML is used for YAML unmarshaling (window as string)
type aggregationRuleYAML struct {
	EventTypes      []string `yaml:"event_types"`
	Severities      []string `yaml:"severities"`
	GroupBy         []string `yaml:"group_by"`
	Window          string   `yaml:"window"` // String format: "1h", "2h", etc.
	Threshold       int      `yaml:"threshold"`
	PreserveSamples int      `yaml:"preserve_samples"`
}

// LoadAggregationConfig loads audit aggregation configuration from config/zqk.yaml.
func LoadAggregationConfig(projectRoot string) (*AuditAggregationConfig, error) {
	configPath := paths.FirstProjectYAMLConfig(projectRoot)
	cfg, err := aggregationConfigs.Load(projectRoot, stampmemo.Of(configPath), func() (AuditAggregationConfig, error) {
		parsed, loadErr := readAggregationConfig(configPath)
		if parsed == nil {
			return AuditAggregationConfig{}, loadErr
		}
		return *parsed, loadErr
	})
	out := cfg
	return &out, err
}

func readAggregationConfig(configPath string) (*AuditAggregationConfig, error) {
	if configPath == "" {
		return DefaultAggregationConfig(), nil
	}
	// Check if config file exists
	if _, err := fileutil.Stat(configPath); fileutil.IsNotExist(err) {
		// No config file - return defaults
		return DefaultAggregationConfig(), nil
	}

	// Read config file
	data, err := fileutil.ReadFile(configPath)
	if err != nil {
		return DefaultAggregationConfig(), errfmt.Newf(ErrMsgReadConfig).Wrap(err)
	}

	var config struct {
		Audit struct {
			Aggregation struct {
				Enabled    bool                  `yaml:"enabled"`
				WindowSize string                `yaml:"window_size"`
				Threshold  int                   `yaml:"threshold"`
				Rules      []aggregationRuleYAML `yaml:"rules"`
			} `yaml:"aggregation"`
		} `yaml:"audit"`
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		// If config file exists but can't be parsed, use defaults
		return DefaultAggregationConfig(), nil
	}

	// If aggregation config section is completely missing or empty, check if enabled is explicitly set
	// If enabled is explicitly false, respect that even if other fields are empty
	if len(config.Audit.Aggregation.Rules) == 0 && config.Audit.Aggregation.WindowSize == emptyValue && config.Audit.Aggregation.Threshold == 0 {
		// Config section exists but is empty - check if enabled is explicitly set
		// We need to check if the enabled field was actually set in the YAML
		// Since Go zero value for bool is false, we can't distinguish unset from false
		// So we'll check if the config file has any aggregation section at all
		// For now, if enabled is false and everything else is empty, assume it was explicitly set
		aggConfig := DefaultAggregationConfig()
		// If enabled is false and no other fields are set, assume it was explicitly disabled
		if !config.Audit.Aggregation.Enabled {
			aggConfig.Enabled = false
		}
		return aggConfig, nil
	}

	// Build aggregation config
	aggConfig := AuditAggregationConfig{
		Enabled:    config.Audit.Aggregation.Enabled,
		WindowSize: config.Audit.Aggregation.WindowSize,
		Threshold:  config.Audit.Aggregation.Threshold,
	}

	// Validate and set default window size
	if aggConfig.WindowSize != emptyValue {
		// Validate window size format
		_, err := parseDuration(aggConfig.WindowSize)
		if err != nil {
			// Invalid window size - use default
			aggConfig.WindowSize = DefaultWindowSize
		}
	} else {
		aggConfig.WindowSize = DefaultWindowSize
	}

	// Set default threshold if not specified
	if aggConfig.Threshold == 0 {
		aggConfig.Threshold = DefaultAggregationThreshold
	}

	// Convert YAML rules to AggregationRule structs
	if len(config.Audit.Aggregation.Rules) == 0 {
		aggConfig.Rules = DefaultAggregationRules()
	} else {
		aggConfig.Rules = make([]AggregationRule, len(config.Audit.Aggregation.Rules))
		for i, ruleYAML := range config.Audit.Aggregation.Rules {
			// Parse window duration from string
			window := time.Hour // default
			if ruleYAML.Window != emptyValue {
				parsed, err := parseDuration(ruleYAML.Window)
				if err == nil {
					window = parsed
				}
			}

			aggConfig.Rules[i] = AggregationRule{
				EventTypes:      ruleYAML.EventTypes,
				Severities:      ruleYAML.Severities,
				GroupBy:         ruleYAML.GroupBy,
				Window:          window,
				Threshold:       ruleYAML.Threshold,
				PreserveSamples: ruleYAML.PreserveSamples,
			}

			// Set defaults for rule if not specified
			if aggConfig.Rules[i].Threshold == 0 {
				aggConfig.Rules[i].Threshold = DefaultAggregationThreshold
			}
			if aggConfig.Rules[i].PreserveSamples == 0 {
				aggConfig.Rules[i].PreserveSamples = DefaultPreserveSamples
			}
			if len(aggConfig.Rules[i].GroupBy) == 0 {
				aggConfig.Rules[i].GroupBy = []string{objects.FieldKeyEventType, objects.FieldKeyTargetKind}
			}
		}
	}

	return &aggConfig, nil
}

// DefaultAggregationConfig returns default aggregation configuration
func DefaultAggregationConfig() *AuditAggregationConfig {
	return &AuditAggregationConfig{
		Enabled:    true,
		WindowSize: DefaultWindowSize,
		Threshold:  DefaultAggregationThreshold,
		Rules:      DefaultAggregationRules(),
	}
}

// parseDuration parses a duration string (e.g., "1h", "24h", "7d", "1w")
func parseDuration(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, errfmt.Errorf(ErrMsgInvalidDurFmt, s)
	}

	unit := s[len(s)-1:]
	value := s[:len(s)-1]

	var multiplier time.Duration
	switch unit {
	case UnitHour:
		multiplier = time.Hour
	case UnitDay:
		multiplier = 24 * time.Hour
	case UnitWeek:
		multiplier = 7 * 24 * time.Hour
	case UnitMonth:
		multiplier = 30 * 24 * time.Hour // Approximate month
	default:
		// Try standard Go duration parsing
		return time.ParseDuration(s)
	}

	var num int
	if _, err := fmt.Sscanf(value, "%d", &num); err != nil {
		return 0, errfmt.Errorf(ErrMsgInvalidDurVal, value)
	}

	return time.Duration(num) * multiplier, nil
}

// ParseWindowSize parses a window size string to time.Duration
func ParseWindowSize(windowSize string) (time.Duration, error) {
	return parseDuration(windowSize)
}

// loadBufferConfigAndWindow loads aggregation config and resolves window size for buffer initialization.
func loadBufferConfigAndWindow(projectRoot string) (*AuditAggregationConfig, time.Duration) {
	config, err := LoadAggregationConfig(projectRoot)
	if err != nil {
		config = DefaultAggregationConfig()
	}
	windowSize := time.Hour
	if config.WindowSize != emptyValue {
		if parsed, err := ParseWindowSize(config.WindowSize); err == nil {
			windowSize = parsed
		}
	}
	return config, windowSize
}
