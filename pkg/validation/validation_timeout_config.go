package validation

import (
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ValidationTimeoutConfig holds per-object validation timeout and system-check stuck threshold.
// Loaded from config/zqk.yaml under validation.per_object_timeout and validation.stuck_timeout_seconds.
//
// Budgets are fail-fast: a healthy object validates in milliseconds. Multi-second kind
// overrides paper over lock contention and must not be the default (see SYSTEM_CHECK_RESOLUTION.md).
//
// Example YAML:
//
//	validation:
//	  per_object_timeout:
//	    default_seconds: 5
//	    kind_overrides:
//	      audit_event: 15  # only if a kind is proven heavier; prefer fixing the stall
//	  stuck_timeout_seconds: 30  # optional; no progress for this long declares validation stuck
type ValidationTimeoutConfig struct {
	// DefaultSeconds is the default per-object validation timeout in seconds.
	DefaultSeconds int `yaml:"default_seconds"`
	// KindOverrides maps kind name to timeout in seconds for kinds that need longer.
	KindOverrides map[string]int `yaml:"kind_overrides"`
	// StuckTimeoutSeconds is how long (seconds) with no progress before declaring validation stuck (0 = use default).
	StuckTimeoutSeconds int `yaml:"stuck_timeout_seconds"`
}

// DefaultValidationTimeoutConfig returns fail-fast timeout configuration.
// Kind overrides stay empty: project config may add them; code must not re-inject
// legacy 60s/90s budgets that hide hangs (TRACK: BLI-1785723654802038000-b14064bc).
func DefaultValidationTimeoutConfig() *ValidationTimeoutConfig {
	return &ValidationTimeoutConfig{
		DefaultSeconds:      5,
		KindOverrides:       map[string]int{},
		StuckTimeoutSeconds: 30,
	}
}

// LoadValidationTimeoutConfig loads timeout configuration from file.
// Uses the same config file as tier config: config/zqk.yaml under validation.per_object_timeout.
func LoadValidationTimeoutConfig(configPath string) (*ValidationTimeoutConfig, error) {
	if configPath == emptyValue {
		configPath = findValidationConfigFile()
		if configPath == emptyValue {
			return DefaultValidationTimeoutConfig(), nil
		}
	}

	cfg, err := timeoutConfigs.Load(configPath, stampmemo.Of(configPath), func() (*ValidationTimeoutConfig, error) {
		return parseValidationTimeoutConfigFile(configPath), nil
	})
	if err != nil || cfg == nil {
		return DefaultValidationTimeoutConfig(), nil
	}
	return cloneValidationTimeoutConfig(cfg), nil
}

func parseValidationTimeoutConfigFile(configPath string) *ValidationTimeoutConfig {
	data, err := fileutil.ReadFile(configPath)
	if err != nil {
		return DefaultValidationTimeoutConfig()
	}

	var config struct {
		Validation struct {
			PerObjectTimeout    *ValidationTimeoutConfig `yaml:"per_object_timeout"`
			StuckTimeoutSeconds int                      `yaml:"stuck_timeout_seconds"`
		} `yaml:"validation"`
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return DefaultValidationTimeoutConfig()
	}

	tc := DefaultValidationTimeoutConfig()
	if config.Validation.PerObjectTimeout != nil {
		fileTC := config.Validation.PerObjectTimeout
		if fileTC.DefaultSeconds > 0 {
			tc.DefaultSeconds = fileTC.DefaultSeconds
		}
		if fileTC.StuckTimeoutSeconds > 0 {
			tc.StuckTimeoutSeconds = fileTC.StuckTimeoutSeconds
		}
		// File kind_overrides replace defaults entirely (no merge of legacy slow budgets).
		if fileTC.KindOverrides != nil {
			tc.KindOverrides = make(map[string]int, len(fileTC.KindOverrides))
			for k, v := range fileTC.KindOverrides {
				if v > 0 {
					tc.KindOverrides[k] = v
				}
			}
		}
	}
	if config.Validation.StuckTimeoutSeconds > 0 {
		tc.StuckTimeoutSeconds = config.Validation.StuckTimeoutSeconds
	}

	if tc.DefaultSeconds <= 0 {
		tc.DefaultSeconds = DefaultValidationTimeoutConfig().DefaultSeconds
	}
	if tc.KindOverrides == nil {
		tc.KindOverrides = make(map[string]int)
	}

	return tc
}

// StuckTimeout returns the duration after which validation is declared stuck (no progress).
func (c *ValidationTimeoutConfig) StuckTimeout() time.Duration {
	if c != nil && c.StuckTimeoutSeconds > 0 {
		return time.Duration(c.StuckTimeoutSeconds) * time.Second
	}
	return 30 * time.Second
}

// GetGlobalValidationTimeoutConfig returns the timeout config for the discovered file.
func GetGlobalValidationTimeoutConfig() *ValidationTimeoutConfig {
	cfg, err := LoadValidationTimeoutConfig(findValidationConfigFile())
	if err != nil || cfg == nil {
		return DefaultValidationTimeoutConfig()
	}
	return cfg
}

// TimeoutForKind returns the per-object validation timeout for the given kind.
func (c *ValidationTimeoutConfig) TimeoutForKind(kind string) time.Duration {
	seconds := c.DefaultSeconds
	if override, ok := c.KindOverrides[kind]; ok && override > 0 {
		seconds = override
	}
	if seconds <= 0 {
		seconds = 5
	}
	return time.Duration(seconds) * time.Second
}

// findValidationConfigFile locates config/zqk.yaml.
func findValidationConfigFile() string {
	return paths.FirstExistingFromCwdAny(paths.ProjectYAMLConfigRelatives())
}
