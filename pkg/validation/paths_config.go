package validation

import (
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
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

// ResetGlobalPathsConfig drops the process-wide paths memo (tests that chdir).
func ResetGlobalPathsConfig() {
	pathsConfigs.Reset()
	resetDiscoveredPaths()
}

// LoadPathsConfig loads the paths configuration from file

func LoadPathsConfig(configPath string) (*PathsConfig, error) {
	if configPath == emptyValue {
		configPath = findPathsConfig()
		if configPath == emptyValue {
			return getDefaultPathsConfig(), nil
		}
	}

	cfg, err := pathsConfigs.Load(configPath, stampmemo.Of(configPath), func() (*PathsConfig, error) {
		return parsePathsConfigFile(configPath), nil
	})
	if err != nil || cfg == nil {
		return getDefaultPathsConfig(), nil
	}
	return clonePathsConfig(cfg), nil
}

func parsePathsConfigFile(configPath string) *PathsConfig {
	data, err := fileutil.ReadFile(configPath)
	if err != nil {
		return getDefaultPathsConfig()
	}

	var config PathsConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return getDefaultPathsConfig()
	}

	config.validate()
	return &config
}

// GetGlobalPathsConfig returns the paths config for the discovered file.
func GetGlobalPathsConfig() *PathsConfig {
	cfg, err := LoadPathsConfig(findPathsConfig())
	if err != nil || cfg == nil {
		return getDefaultPathsConfig()
	}
	return cfg
}

// getDefaultPathsConfig returns default path configuration
func getDefaultPathsConfig() *PathsConfig {
	config := &PathsConfig{
		Paths: map[string]string{
			paths.PathKeyCommandSpecs: paths.CLICommandSpecsDir,
			"object_specs":            paths.ProcessInternalObjectSpecsDir,
			"config_dir":              paths.ProcessInternalDir, ConstMagicExtracted_66: filepath.Join(paths.ProcessInternalConfigsDir, paths.IdPrefixesConfigFile), ConstMagicExtracted_67: filepath.Join(paths.ProcessInternalConfigsDir, paths.KindMappingsConfigFile), "paths_config": filepath.Join(paths.ProcessInternalConfigsDir, paths.PathsConfigFile),
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
	wd, err := fileutil.Getwd()
	if err != nil {
		return ""
	}
	found, _ := resolvedFindPaths.Load(wd+"\x00"+pathKey, stampmemo.Of(wd), func() (string, error) {
		if foundPath := c.findPathInRelativePaths(pathKey, path); foundPath != emptyValue {
			return foundPath, nil
		}
		return c.findPathByWalkingUp(pathKey, path), nil
	})
	return found
}

// findPathInRelativePaths searches for path in relative paths from current working directory
func (c *PathsConfig) findPathInRelativePaths(pathKey, path string) string {
	wd, err := fileutil.Getwd()
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
	wd, err := fileutil.Getwd()
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
		if _, err := fileutil.Stat(markerPath); err == nil {
			return true
		}
	}
	return false
}

// isValidPath checks if a path exists and matches the expected type for the path key
func (c *PathsConfig) isValidPath(fullPath, pathKey string) bool {
	info, err := fileutil.Stat(fullPath)
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
	wd, err := fileutil.Getwd()
	if err != nil {
		return ""
	}

	for _, relPath := range defaultConfig.SearchStrategy.RelativePaths {
		fullPath := filepath.Join(wd, relPath, defaultConfig.Paths["paths_config"])
		if _, err := fileutil.Stat(fullPath); err == nil {
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
			if _, err := fileutil.Stat(markerPath); err == nil {
				hasMarker = true
				break
			}
		}

		if hasMarker {
			// Found project root, try the path from here
			fullPath := filepath.Join(dir, defaultConfig.Paths["paths_config"])
			if _, err := fileutil.Stat(fullPath); err == nil {
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
