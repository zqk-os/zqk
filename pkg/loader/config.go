package loader

import (
	"maps"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/kindnames"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// LoaderTimeoutConfigKey is the config file key for component loader timeouts (under component_loaders.<name>).
const LoaderTimeoutConfigKey = "component_loaders"

const (
	configKeyTimeoutSeconds           = "timeout_seconds"
	configKeyWaitForCompletionSeconds = "wait_for_completion_seconds"
	configKeyPublishChannelSeconds    = "publish_channel_seconds"
	configKeyLoadOperationSeconds     = "load_operation_seconds"
	emptyValue                        = ""
	durationSecond                    = time.Second
)

// loaderTimeoutConfigFile is the struct we unmarshal from YAML (seconds for readability).
// TimeoutSeconds sets both wait_for_completion and load_operation to the same value when present.
type loaderTimeoutConfigFile struct {
	TimeoutSeconds           int `yaml:"timeout_seconds"` // optional: sets both wait and load to same value
	WaitForCompletionSeconds int `yaml:"wait_for_completion_seconds"`
	PublishChannelSeconds    int `yaml:"publish_channel_seconds"`
	LoadOperationSeconds     int `yaml:"load_operation_seconds"`
}

// DefaultLoaderTimeoutConfig returns default timeouts for a loader (e.g. id_patterns).
func DefaultLoaderTimeoutConfig() LoaderTimeoutConfig {
	return LoaderTimeoutConfig{
		WaitForCompletion: 5 * time.Second,
		PublishChannel:    1 * time.Second,
		LoadOperation:     0, // no separate limit; load runs until WaitForCompletion or context
	}
}

// DefaultLoaderTimeoutConfigMap returns default timeouts keyed by loader name.
// Used when no config file is present; also merged with file config so missing keys get defaults.
//
// Components that use the loader pattern (Runner + this config): id_patterns, lifecycle,
// bucketing_config, bucket_strategy_loader, spec_loader, kind_mapper, kind_synonym_resolver, object_id_cache.
// Other caches (list cache, activity cache, storage provider cache, hash registry cache) do not
// have a single "ensure ready" load step with timeout—they are either on-demand or built inline.
func DefaultLoaderTimeoutConfigMap() map[string]LoaderTimeoutConfig {
	return map[string]LoaderTimeoutConfig{
		"id_patterns": {
			WaitForCompletion: 15 * time.Second,
			PublishChannel:    1 * time.Second,
		},
		kindnames.Lifecycle: {
			WaitForCompletion: 10 * time.Second,
			PublishChannel:    1 * time.Second,
		},
		"bucketing_config": {
			WaitForCompletion: 5 * time.Second,
			PublishChannel:    1 * time.Second,
		},
		"bucket_strategy_loader": {
			WaitForCompletion: 15 * time.Second,
			PublishChannel:    2 * time.Second,
		},
		"spec_loader": {
			WaitForCompletion: 10 * time.Second,
			PublishChannel:    2 * time.Second,
		},
		"kind_synonym_resolver": {
			WaitForCompletion: 10 * time.Second,
			PublishChannel:    2 * time.Second,
		},
		"kind_mapper": {
			WaitForCompletion: 15 * time.Second,
			PublishChannel:    2 * time.Second,
		},
		"object_id_cache": {
			WaitForCompletion: 900 * time.Second, // 15m: refresh-cache + warm can be very slow on large repos
			PublishChannel:    5 * time.Second,
			LoadOperation:     900 * time.Second,
		},
	}
}

var (
	globalLoaderConfig     map[string]LoaderTimeoutConfig
	globalLoaderConfigOnce sync.Once
)

// LoadLoaderTimeoutConfig loads component loader timeouts from the project config file.
// Path: .zqk/config/config.yaml under "component_loaders". Missing or invalid file returns defaults.
// Profile/thematic overrides are not applied here; call MergeLoaderTimeoutOverrides after with profile data.
func LoadLoaderTimeoutConfig(configPath string) (map[string]LoaderTimeoutConfig, error) {
	if configPath == emptyValue {
		configPath = findLoaderConfigFile()
		if configPath == emptyValue {
			return DefaultLoaderTimeoutConfigMap(), nil
		}
	}

	data, err := fileutil.ReadFile(configPath)
	if err != nil {
		return DefaultLoaderTimeoutConfigMap(), nil
	}

	var root struct {
		ComponentLoaders map[string]loaderTimeoutConfigFile `yaml:"component_loaders"`
	}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return DefaultLoaderTimeoutConfigMap(), nil
	}

	defaults := DefaultLoaderTimeoutConfigMap()
	out := make(map[string]LoaderTimeoutConfig, len(defaults))
	maps.Copy(out, defaults)
	for name, f := range root.ComponentLoaders {
		c := out[name]
		if f.TimeoutSeconds > 0 {
			d := time.Duration(f.TimeoutSeconds) * durationSecond
			c.WaitForCompletion = d
			c.LoadOperation = d
		}
		if f.WaitForCompletionSeconds > 0 {
			c.WaitForCompletion = time.Duration(f.WaitForCompletionSeconds) * durationSecond
		}
		if f.PublishChannelSeconds > 0 {
			c.PublishChannel = time.Duration(f.PublishChannelSeconds) * durationSecond
		}
		if f.LoadOperationSeconds > 0 {
			c.LoadOperation = time.Duration(f.LoadOperationSeconds) * durationSecond
		}
		out[name] = c
	}
	return out, nil
}

// MergeLoaderTimeoutOverrides applies profile or thematic overrides on top of base config.
// Overrides is keyed by loader name; each value is a map of keys (e.g. "wait_for_completion_seconds") to values (int seconds).
// Used by context/profile layer after loading base config.
func MergeLoaderTimeoutOverrides(base map[string]LoaderTimeoutConfig, overrides map[string]map[string]any) map[string]LoaderTimeoutConfig {
	if len(overrides) == 0 {
		return base
	}
	out := make(map[string]LoaderTimeoutConfig, len(base))
	maps.Copy(out, base)
	for name, ov := range overrides {
		c := out[name]
		if s := intFromOverride(ov[configKeyTimeoutSeconds]); s > 0 {
			d := time.Duration(s) * durationSecond
			c.WaitForCompletion = d
			c.LoadOperation = d
		}
		if s := intFromOverride(ov[configKeyWaitForCompletionSeconds]); s > 0 {
			c.WaitForCompletion = time.Duration(s) * durationSecond
		}
		if s := intFromOverride(ov[configKeyPublishChannelSeconds]); s > 0 {
			c.PublishChannel = time.Duration(s) * durationSecond
		}
		if s := intFromOverride(ov[configKeyLoadOperationSeconds]); s > 0 {
			c.LoadOperation = time.Duration(s) * durationSecond
		}
		out[name] = c
	}
	return out
}

// GetLoaderTimeoutConfig returns timeout config for the named loader (e.g. "id_patterns").
// Uses global config loaded from default file; call GetGlobalLoaderTimeoutConfigMap() first if you need to apply profile overrides elsewhere.
func GetLoaderTimeoutConfig(loaderName string) LoaderTimeoutConfig {
	m := GetGlobalLoaderTimeoutConfigMap()
	if c, ok := m[loaderName]; ok {
		return c
	}
	return DefaultLoaderTimeoutConfig()
}

// GetGlobalLoaderTimeoutConfigMap returns the singleton loader timeout config map (from default file).
// Profile/thematic overrides are not applied; use MergeLoaderTimeoutOverrides in the context layer with profile data.
func GetGlobalLoaderTimeoutConfigMap() map[string]LoaderTimeoutConfig {
	globalLoaderConfigOnce.Do(func() {
		m, err := LoadLoaderTimeoutConfig(emptyValue)
		if err != nil {
			globalLoaderConfig = DefaultLoaderTimeoutConfigMap()
		} else {
			globalLoaderConfig = m
		}
	})
	return globalLoaderConfig
}

// intFromOverride extracts an int from a profile override value (may be int or float64 from YAML/JSON).
func intFromOverride(v any) int {
	if v == nil {
		return 0
	}
	switch v := v.(type) {
	case int:
		return v
	case float64:
		return int(v)
	}

	return 0
}

func findLoaderConfigFile() string {
	cwd, err := fileutil.Getwd()
	if err != nil {
		return emptyValue
	}
	current := cwd
	for {
		configPath := filepath.Join(current, paths.ProjectDataDir, paths.ConfigDir, paths.ProjectConfigFile)
		if _, err := fileutil.Stat(configPath); err == nil {
			return configPath
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return emptyValue
}
