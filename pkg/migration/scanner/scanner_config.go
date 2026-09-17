package scanner

import (
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// ScannerConfig represents the configuration for scanner exclusion patterns
type ScannerConfig struct {
	Version            string                     `yaml:"version"`
	Description        string                     `yaml:"description"`
	ExcludeDirectories []string                   `yaml:"exclude_directories"`
	ExcludePatterns    []string                   `yaml:"exclude_patterns"`
	Scanners           map[string]ScannerOverride `yaml:"scanners"`
}

// ScannerOverride defines scanner-specific overrides
type ScannerOverride struct {
	ExcludeDirectories []string `yaml:"exclude_directories"`
	ExcludePatterns    []string `yaml:"exclude_patterns"`
}

var (
	globalScannerConfig     *ScannerConfig
	globalScannerConfigOnce sync.Once
)

// LoadScannerConfig loads the scanner configuration from file
func LoadScannerConfig(configPath string) (*ScannerConfig, error) {
	if configPath == emptyValue {
		configPath = findScannerConfig()
		if configPath == emptyValue {
			// Return default config if file not found
			return getDefaultScannerConfig(), nil
		}
	}

	data, err := fileutil.ReadFile(configPath)
	if err != nil {
		// Return default config if file can't be read
		return getDefaultScannerConfig(), nil
	}

	var config ScannerConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		// Return default config if file can't be parsed
		return getDefaultScannerConfig(), nil
	}

	// Validate and set defaults
	config.validate()
	return &config, nil
}

// GetGlobalScannerConfig returns the singleton instance of the scanner config
func GetGlobalScannerConfig() *ScannerConfig {
	globalScannerConfigOnce.Do(func() {
		config, err := LoadScannerConfig("")
		if err != nil {
			globalScannerConfig = getDefaultScannerConfig()
		} else {
			globalScannerConfig = config
		}
	})
	return globalScannerConfig
}

// getDefaultScannerConfig returns default scanner configuration
func getDefaultScannerConfig() *ScannerConfig {
	config := &ScannerConfig{
		ExcludeDirectories: []string{
			"_internal",
			".git",
			"node_modules",
			paths.ProjectDataDir,
			"__pycache__",
		},
		ExcludePatterns: []string{
			"*.hash",
			".*",
		},
		Scanners: map[string]ScannerOverride{
			"yaml_scanner": {
				ExcludeDirectories: []string{
					"_internal",
					".git",
					"node_modules",
					paths.ProjectDataDir,
				},
				ExcludePatterns: []string{
					"*.hash",
					".*",
				},
			},
			"documentation_discoverer": {
				ExcludeDirectories: []string{
					".git",
					"node_modules",
					"__pycache__",
					paths.ProjectDataDir,
				},
				ExcludePatterns: []string{
					".*",
				},
			},
		},
	}
	config.validate()
	return config
}

// validate validates the scanner config and sets defaults if needed
func (c *ScannerConfig) validate() {
	// Ensure ExcludeDirectories is initialized
	if c.ExcludeDirectories == nil {
		c.ExcludeDirectories = []string{"_internal", ".git", "node_modules", paths.ProjectDataDir, "__pycache__"}
	}

	// Ensure ExcludePatterns is initialized
	if c.ExcludePatterns == nil {
		c.ExcludePatterns = []string{"*.hash", ".*"}
	}

	// Ensure Scanners map is initialized
	if c.Scanners == nil {
		c.Scanners = make(map[string]ScannerOverride)
	}
}

// GetExcludeDirectories returns exclude directories for a scanner
func (c *ScannerConfig) GetExcludeDirectories(scannerName string) []string {
	// Check for scanner-specific override
	if override, ok := c.Scanners[scannerName]; ok && len(override.ExcludeDirectories) > 0 {
		return override.ExcludeDirectories
	}
	// Use global defaults
	return c.ExcludeDirectories
}

// GetExcludePatterns returns exclude patterns for a scanner
func (c *ScannerConfig) GetExcludePatterns(scannerName string) []string {
	// Check for scanner-specific override
	if override, ok := c.Scanners[scannerName]; ok && len(override.ExcludePatterns) > 0 {
		return override.ExcludePatterns
	}
	// Use global defaults
	return c.ExcludePatterns
}

// findScannerConfig finds the scanner config file
func findScannerConfig() string {
	// Try standard locations
	possiblePaths := []string{
		filepath.Join(paths.ProcessInternalDir, paths.ScannerConfigFile),
		filepath.Join("..", paths.ProcessInternalDir, paths.ScannerConfigFile),
		filepath.Join("..", "..", paths.ProcessInternalDir, paths.ScannerConfigFile),
	}

	wd, err := fileutil.Getwd()
	if err != nil {
		return ""
	}

	for _, path := range possiblePaths {
		absPath := filepath.Join(wd, path)
		if _, err := fileutil.Stat(absPath); err == nil {
			return absPath
		}
	}

	// Walk up directory tree looking for markers
	dir := wd
	markers := []string{paths.ProcessInternalDir, paths.ProjectDataDir, "go.mod"}

	for {
		// Check if this directory has a marker
		hasMarker := false
		for _, marker := range markers {
			markerPath := filepath.Join(dir, marker)
			if _, err := fileutil.Stat(markerPath); err == nil {
				hasMarker = true
				break
			}
		}

		if hasMarker {
			// Found project root, try the path from here
			potentialPath := filepath.Join(dir, paths.ProcessInternalDir, paths.ScannerConfigFile)
			if _, err := fileutil.Stat(potentialPath); err == nil {
				return potentialPath
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
