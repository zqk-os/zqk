package validation

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"gopkg.in/yaml.v3"
)

// ValidationTimeoutConfig holds per-object validation timeout and system-check stuck threshold.
// Loaded from .zqk/config/config.yaml under validation.per_object_timeout and validation.stuck_timeout_seconds.
//
// Example YAML:
//
//	validation:
//	  per_object_timeout:
//	    default_seconds: 30
//	    kind_overrides:
//	      doc_entry: 60
//	  stuck_timeout_seconds: 90  # optional; no progress for this long declares validation stuck
type ValidationTimeoutConfig struct {
	// DefaultSeconds is the default per-object validation timeout in seconds (e.g. 30).
	DefaultSeconds int `yaml:"default_seconds"`
	// KindOverrides maps kind name to timeout in seconds for kinds that need longer (e.g. doc_entry: 60).
	KindOverrides map[string]int `yaml:"kind_overrides"`
	// StuckTimeoutSeconds is how long (seconds) with no progress before declaring validation stuck (0 = use default 90).
	StuckTimeoutSeconds int `yaml:"stuck_timeout_seconds"`
}

var (
	globalTimeoutConfig     *ValidationTimeoutConfig
	globalTimeoutConfigOnce sync.Once
)

// DefaultValidationTimeoutConfig returns the default timeout configuration.
func DefaultValidationTimeoutConfig() *ValidationTimeoutConfig {
	return &ValidationTimeoutConfig{
		DefaultSeconds: 30,
		KindOverrides: map[string]int{
			objects.KindAuditEvent:     90,
			objects.KindBacklogItem:    60,
			objects.KindDocEntry:       60,
			objects.KindFileLockMetric: 90,
			objects.KindRequirement:    60,
			objects.KindMcpSession:     60,
		},
	}
}

// LoadValidationTimeoutConfig loads timeout configuration from file.
// Uses the same config file as tier config: .zqk/config/config.yaml under validation.per_object_timeout.
func LoadValidationTimeoutConfig(configPath string) (*ValidationTimeoutConfig, error) {
	if configPath == emptyValue {
		configPath = findValidationConfigFile()
		if configPath == emptyValue {
			return DefaultValidationTimeoutConfig(), nil
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return DefaultValidationTimeoutConfig(), nil
	}

	var config struct {
		Validation struct {
			PerObjectTimeout    *ValidationTimeoutConfig `yaml:"per_object_timeout"`
			StuckTimeoutSeconds int                      `yaml:"stuck_timeout_seconds"`
		} `yaml:"validation"`
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return DefaultValidationTimeoutConfig(), nil
	}

	tc := DefaultValidationTimeoutConfig()
	if config.Validation.PerObjectTimeout != nil {
		tc = config.Validation.PerObjectTimeout
	}
	if config.Validation.StuckTimeoutSeconds > 0 {
		tc.StuckTimeoutSeconds = config.Validation.StuckTimeoutSeconds
	}

	// Apply defaults for missing fields
	if tc.DefaultSeconds <= 0 {
		tc.DefaultSeconds = DefaultValidationTimeoutConfig().DefaultSeconds
	}
	if tc.KindOverrides == nil {
		tc.KindOverrides = make(map[string]int)
	}
	// Merge kind overrides with defaults so unspecified kinds are not lost
	for k, v := range DefaultValidationTimeoutConfig().KindOverrides {
		if tc.KindOverrides != nil {
			if _, exists := tc.KindOverrides[k]; !exists {
				tc.KindOverrides[k] = v
			}
		}
	}

	return tc, nil
}

// StuckTimeout returns the duration after which validation is declared stuck (no progress).
// Returns 90s if not configured.
func (c *ValidationTimeoutConfig) StuckTimeout() time.Duration {
	if c.StuckTimeoutSeconds > 0 {
		return time.Duration(c.StuckTimeoutSeconds) * time.Second
	}
	return 90 * time.Second
}

// GetGlobalValidationTimeoutConfig returns the singleton timeout config.
func GetGlobalValidationTimeoutConfig() *ValidationTimeoutConfig {
	globalTimeoutConfigOnce.Do(func() {
		config, err := LoadValidationTimeoutConfig("")
		if err != nil {
			globalTimeoutConfig = DefaultValidationTimeoutConfig()
		} else {
			globalTimeoutConfig = config
		}
	})
	return globalTimeoutConfig
}

// TimeoutForKind returns the per-object validation timeout for the given kind.
func (c *ValidationTimeoutConfig) TimeoutForKind(kind string) time.Duration {
	seconds := c.DefaultSeconds
	if override, ok := c.KindOverrides[kind]; ok && override > 0 {
		seconds = override
	}
	if seconds <= 0 {
		seconds = 30
	}
	return time.Duration(seconds) * time.Second
}

// findValidationConfigFile locates .zqk/config/config.yaml (same as tier config).
func findValidationConfigFile() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	current := cwd
	for {
		configPath := filepath.Join(current, paths.ProjectDataDir, paths.ConfigDir, paths.ProjectConfigFile)
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return ""
}
