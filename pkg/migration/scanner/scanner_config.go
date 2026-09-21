package scanner

import (
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
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

var scannerConfigs stampmemo.Table[*ScannerConfig] // keyed by discovered path; stamp is the YAML file

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

// ResetGlobalScannerConfig forgets the process-wide scanner memo (tests that chdir).
func ResetGlobalScannerConfig() {
	scannerConfigs.Reset()
}

// GetGlobalScannerConfig returns the stamp-invalidated scanner config.
func GetGlobalScannerConfig() *ScannerConfig {
	configPath := findScannerConfig()
	cfg, err := scannerConfigs.Load(configPath, stampmemo.Of(configPath), func() (*ScannerConfig, error) {
		if configPath == emptyValue {
			return getDefaultScannerConfig(), nil
		}
		loaded, loadErr := LoadScannerConfig(configPath)
		if loadErr != nil || loaded == nil {
			return getDefaultScannerConfig(), nil
		}
		return loaded, nil
	})
	if err != nil || cfg == nil {
		return getDefaultScannerConfig()
	}
	return cfg
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

func findScannerConfig() string {
	return paths.FirstExistingFromCwd(filepath.Join(paths.ProcessInternalDir, paths.ScannerConfigFile))
}
