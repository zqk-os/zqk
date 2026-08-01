package validation

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"gopkg.in/yaml.v3"
)

// PathsConfig represents the configuration for standard directory paths
type PathsConfig struct {
	Version        string               `yaml:"version"`
	Description    string               `yaml:"description"`
	Paths          map[string]string    `yaml:"paths"`
	SearchStrategy SearchStrategyConfig `yaml:"search_strategy"`
}

// SearchStrategyConfig defines how to search for paths
type SearchStrategyConfig struct {
	RelativePaths []string `yaml:"relative_paths"`
	Markers       []string `yaml:"markers"`
}

var (
	globalPathsConfig   *PathsConfig
	globalPathsConfigMu sync.Mutex
)

// ResetGlobalPathsConfig resets the global paths config singleton, forcing it to reload
// This is useful when the config file changes or when testing
func ResetGlobalPathsConfig() {
	if err := concurrency.RunInLockWithLogger(
		&globalPathsConfigMu,
		LockNamePathsResetConfig,
		lockLoggerSystem(),
		func() error {
			globalPathsConfig = nil
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.

			// LoadPathsConfig loads the paths configuration from file
			ProfileSystem))).Error(ConstMagic297bf952, err).Log()
	}
}

func LoadPathsConfig(configPath string) (*PathsConfig, error) {
	if configPath == emptyValue {
		configPath = findPathsConfig()
		if configPath == emptyValue {
			// Return default config if file not found
			return getDefaultPathsConfig(), nil
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		// Return default config if file can't be read
		return getDefaultPathsConfig(), nil
	}

	var config PathsConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		// Return default config if file can't be parsed
		return getDefaultPathsConfig(), nil
	}

	// Validate and set defaults
	config.validate()
	return &config, nil
}

// GetGlobalPathsConfig returns the singleton instance of the paths config
func GetGlobalPathsConfig() *PathsConfig {
	var config *PathsConfig
	if err := concurrency.RunInLockWithLogger(
		&globalPathsConfigMu,
		LockNamePathsGetConfig,
		lockLoggerSystem(),
		func() error {
			// Check if we need to reload (config is nil or was reset)
			if globalPathsConfig == nil {
				loadedConfig, err := LoadPathsConfig("")
				if err != nil {
					globalPathsConfig = getDefaultPathsConfig()
				} else {
					globalPathsConfig = loadedConfig
				}
			}
			config = globalPathsConfig
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.

			// Fallback to default if locking fails and we don't have a config yet
			ProfileSystem))).Error(ConstMagic31a779e6, err).Log()

		if config == nil {
			config = getDefaultPathsConfig()
		}
	}
	return config
}

// getDefaultPathsConfig returns default path configuration
func getDefaultPathsConfig() *PathsConfig {
	config := &PathsConfig{
		Paths: map[string]string{
			"object_specs": paths.ProcessInternalObjectSpecsDir,
			"config_dir":   paths.ProcessInternalDir, ConstMagicExtracted_66: filepath.Join(paths.ProcessInternalConfigsDir, paths.IdPrefixesConfigFile), ConstMagicExtracted_67: filepath.Join(paths.ProcessInternalConfigsDir, paths.KindMappingsConfigFile), "paths_config": filepath.Join(paths.ProcessInternalConfigsDir, paths.PathsConfigFile),
		},
		SearchStrategy: SearchStrategyConfig{
			RelativePaths: []string{"", "../", "../../", "../../../"},
			Markers:       []string{paths.ProcessInternalDir, paths.ProjectDataDir, "go.mod"},
		},
	}
	config.validate()
	return config
}

// validate validates the paths config and sets defaults if needed
func (c *PathsConfig) validate() {
	// Ensure Paths map is initialized
	if c.Paths == nil {
		c.Paths = make(map[string]string)
	}

	// Ensure SearchStrategy has defaults
	if len(c.SearchStrategy.RelativePaths) == 0 {
		c.SearchStrategy.RelativePaths = []string{"", "../", "../../", "../../../"}
	}
	if len(c.SearchStrategy.Markers) == 0 {
		c.SearchStrategy.Markers = []string{paths.ProcessInternalDir, paths.ProjectDataDir, "go.mod"}
	}
}

// FindPath searches for a configured path using the search strategy
// FindPath finds a path by key using the configured search strategy
//
//nolint:gocyclo // Function orchestrates multiple search strategies; complexity reduced via helper methods
func (c *PathsConfig) FindPath(pathKey string) string {
	path, ok := c.Paths[pathKey]
	if !ok {
		return ""
	}

	// Try relative paths first
	if foundPath := c.findPathInRelativePaths(pathKey, path); foundPath != emptyValue {
		return foundPath
	}

	// Walk up directory tree looking for markers
	return c.findPathByWalkingUp(pathKey, path)
}

// findPathInRelativePaths searches for path in relative paths from current working directory
func (c *PathsConfig) findPathInRelativePaths(pathKey, path string) string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}

	for _, relPath := range c.SearchStrategy.RelativePaths {
		fullPath := filepath.Join(wd, relPath, path)
		if c.isValidPath(fullPath, pathKey) {
			return fullPath
		}
	}

	return ""
}

// findPathByWalkingUp walks up directory tree looking for markers and path
func (c *PathsConfig) findPathByWalkingUp(pathKey, path string) string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}

	dir := wd
	for {
		if c.hasMarker(dir) {
			fullPath := filepath.Join(dir, path)
			if c.isValidPath(fullPath, pathKey) {
				return fullPath
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return ""
}

// hasMarker checks if a directory has any of the configured markers
func (c *PathsConfig) hasMarker(dir string) bool {
	for _, marker := range c.SearchStrategy.Markers {
		markerPath := filepath.Join(dir, marker)
		if _, err := os.Stat(markerPath); err == nil {
			return true
		}
	}
	return false
}

// isValidPath checks if a path exists and matches the expected type for the path key
func (c *PathsConfig) isValidPath(fullPath, pathKey string) bool {
	info, err := os.Stat(fullPath)
	if err != nil {
		return false
	}

	if pathKey == "object_specs" && info.IsDir() {
		return true
	}
	if strings.HasSuffix(pathKey, "_config") && !info.IsDir() {
		return true
	}

	return false
}

// findPathsConfig finds the paths config file
// Uses default search strategy (bootstrap - can't use config to find itself)
func findPathsConfig() string {
	// Bootstrap: use default search strategy since we can't use config to find itself
	defaultConfig := getDefaultPathsConfig()

	// Use default relative paths
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}

	for _, relPath := range defaultConfig.SearchStrategy.RelativePaths {
		fullPath := filepath.Join(wd, relPath, defaultConfig.Paths["paths_config"])
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath
		}
	}

	// Walk up directory tree looking for markers
	dir := wd
	for {
		// Check if this directory has a marker
		hasMarker := false
		for _, marker := range defaultConfig.SearchStrategy.Markers {
			markerPath := filepath.Join(dir, marker)
			if _, err := os.Stat(markerPath); err == nil {
				hasMarker = true
				break
			}
		}

		if hasMarker {
			// Found project root, try the path from here
			fullPath := filepath.Join(dir, defaultConfig.Paths["paths_config"])
			if _, err := os.Stat(fullPath); err == nil {
				return fullPath
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return ""
}
