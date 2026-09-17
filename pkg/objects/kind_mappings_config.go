package objects

import (
	"errors"
	"maps"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const (
	defaultBackendType  = "file"
	backendDefaultKey   = "default"
	yamlExt             = ".yaml"
	ymlExt              = ".yml"
	conditionNoExplicit = "no_explicit_mapping"
	ruleTypeDirPattern  = "directory_pattern"
	ruleTypeKindPattern = "kind_pattern"
	errConfigNotFound   = "kind mappings config file not found"
	errReadConfigFmt    = "failed to read kind mappings config: %w"
	errParseConfigFmt   = "failed to parse kind mappings config: %w"
	warnRegexCompileMsg = "Failed to compile regex pattern in kind mappings config"
	logPatternField     = "pattern"
	logRuleTypeField    = "rule_type"
)

// KindMappingsConfig represents the configuration for kind-to-directory mappings
type KindMappingsConfig struct {
	Version         string                   `yaml:"version"`
	Description     string                   `yaml:"description"`
	Backends        map[string]BackendConfig `yaml:"backends"` // Backend-specific configs
	SkipDirectories []string                 `yaml:"skip_directories"`
	SkipSpecs       []string                 `yaml:"skip_specs"`
	InferenceRules  InferenceRules           `yaml:"inference_rules"`
	OnDemandKinds   []any                    `yaml:"on_demand_kinds"` // Can be strings or pattern objects

	// Legacy fields for backward compatibility (deprecated, use backends.default)
	KindToDirectory map[string]string          `yaml:"kind_to_directory,omitempty"`
	DirectoryToKind map[string]string          `yaml:"directory_to_kind,omitempty"`
	MultiKindDirs   map[string]MultiKindConfig `yaml:"multi_kind_directories,omitempty"`

	// Computed fields (set after loading)
	currentBackend string       // Current backend type (file, graph, etc.)
	mu             sync.RWMutex // Protects currentBackend and backend operations
}

// BackendConfig represents configuration for a specific backend
type BackendConfig struct {
	KindToDirectory map[string]string          `yaml:"kind_to_directory,omitempty"`
	DirectoryToKind map[string]string          `yaml:"directory_to_kind,omitempty"`
	MultiKindDirs   map[string]MultiKindConfig `yaml:"multi_kind_directories,omitempty"`
}

// InferenceRules defines patterns for inferring directory names from kinds and vice versa
type InferenceRules struct {
	DirectoryPatterns []PatternRule `yaml:"directory_patterns"`
	KindPatterns      []PatternRule `yaml:"kind_patterns"`
}

// PatternRule defines a regex pattern and replacement for inference
type PatternRule struct {
	Pattern     string `yaml:"pattern"`
	Replacement string `yaml:"replacement"`
	Priority    int    `yaml:"priority,omitempty"`
	Condition   string `yaml:"condition,omitempty"`
	Example     string `yaml:"example,omitempty"`
}

// MultiKindConfig defines configuration for directories that contain multiple kinds
type MultiKindConfig struct {
	DefaultKind string   `yaml:"default_kind"`
	Kinds       []string `yaml:"kinds"`
}

var (
	globalKindMappingsConfig     *KindMappingsConfig
	globalKindMappingsConfigOnce sync.Once
)

// LoadKindMappingsConfig loads the kind mappings configuration from file
func LoadKindMappingsConfig(configPath string) (*KindMappingsConfig, error) {
	start := time.Now()
	metrics := GetKindMappingsMetrics()

	defer func() {
		metrics.RecordConfigLoad(time.Since(start), nil)
	}()

	if configPath == emptyValue {
		// Try to find config file in standard location
		configPath = findKindMappingsConfig()
		if configPath == emptyValue {
			err := errors.New(errConfigNotFound)
			metrics.RecordConfigLoad(time.Since(start), err)
			return nil, err
		}
	}

	data, err := fileutil.ReadFile(configPath)
	if err != nil {
		err = errfmt.Errorf(errReadConfigFmt, err)
		metrics.RecordConfigLoad(time.Since(start), err)
		return nil, err
	}

	var config KindMappingsConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		err = errfmt.Errorf(errParseConfigFmt, err)
		metrics.RecordConfigLoad(time.Since(start), err)
		return nil, err
	}

	// Validate and set defaults
	config.validate()

	return &config, nil
}

// validate validates the kind mappings config and sets defaults if needed
func (c *KindMappingsConfig) validate() {
	// Set default backend type if not set
	if c.currentBackend == emptyValue {
		c.currentBackend = defaultBackendType // Default to file backend
	}

	// Ensure SkipDirectories and SkipSpecs are initialized if nil
	if c.SkipDirectories == nil {
		c.SkipDirectories = []string{"_internal", "_external", "architecture", "ontology", "planning", "audit"}
	}
	if c.SkipSpecs == nil {
		// extensible_object is a concrete kind (CLI create/template); only pure bases are skipped.
		c.SkipSpecs = []string{KindBaseObject, KindAuditable, KindWorkInterval, KindWorkUnit, KindOccupancy, KindRemainingOpen}
	}

	// Ensure Backends map is initialized
	if c.Backends == nil {
		c.Backends = make(map[string]BackendConfig)
	}
}

// GetGlobalKindMappingsConfig returns the singleton instance of the config
// backendType can be "file", "graph", or "" (defaults to "file" for backward compatibility)
func GetGlobalKindMappingsConfig(backendType ...string) *KindMappingsConfig {
	globalKindMappingsConfigOnce.Do(func() {
		config, err := LoadKindMappingsConfig("")
		if err != nil {
			// If config file doesn't exist, create a default config
			// This maintains backward compatibility
			globalKindMappingsConfig = &KindMappingsConfig{
				Backends:        make(map[string]BackendConfig),
				KindToDirectory: make(map[string]string),
				DirectoryToKind: make(map[string]string),
				SkipDirectories: []string{"_internal", "_external", "architecture", "ontology", "planning", "audit"},
				SkipSpecs:       []string{KindBaseObject, KindAuditable, KindWorkInterval, KindWorkUnit, KindOccupancy, KindRemainingOpen},
				currentBackend:  defaultBackendType, // Default to file backend
			}
		} else {
			globalKindMappingsConfig = config
			// Set current backend type
			if len(backendType) > 0 && backendType[0] != emptyValue {
				globalKindMappingsConfig.currentBackend = backendType[0]
			} else {
				globalKindMappingsConfig.currentBackend = defaultBackendType // Default
			}
		}
	})
	return globalKindMappingsConfig
}

// SetBackendType sets the current backend type for the config
// Should only be called during initialization to avoid race conditions
func (c *KindMappingsConfig) SetBackendType(backendType string) {
	_ = concurrency.RunInLockWithLogger(
		&c.mu, LockNameKindMappingsConfigSetBackendType, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if c.currentBackend != backendType {
				c.currentBackend = backendType
				GetKindMappingsMetrics().RecordBackendSwitch()
			}
			return nil
		},
	)
}

// GetBackendType returns the current backend type
func (c *KindMappingsConfig) GetBackendType() string {
	var backendType string
	_ = concurrency.RunInRLockWithLogger(
		&c.mu, LockNameKindMappingsConfigGetBackendType, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if c.currentBackend == emptyValue {
				backendType = defaultBackendType // Default
			} else {
				backendType = c.currentBackend
			}
			return nil
		},
	)
	return backendType
}

// getBackendConfig returns the configuration for the current backend
// Falls back to "default" if backend-specific config doesn't exist
func (c *KindMappingsConfig) getBackendConfig() BackendConfig {
	var currentBackend string
	var defaultConfig, backendConfig BackendConfig
	var kindToDirectory, directoryToKind map[string]string
	var multiKindDirs map[string]MultiKindConfig
	_ = concurrency.RunInRLockWithLogger(
		&c.mu, LockNameKindMappingsConfigGetBackendConfig, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			currentBackend = c.currentBackend
			if currentBackend == emptyValue {
				currentBackend = defaultBackendType // Default
			}
			defaultConfig = c.Backends[backendDefaultKey]
			backendConfig = c.Backends[currentBackend]
			kindToDirectory = c.KindToDirectory
			directoryToKind = c.DirectoryToKind
			multiKindDirs = c.MultiKindDirs
			return nil
		},
	)

	// Try backend-specific config first
	if len(backendConfig.KindToDirectory) > 0 || len(backendConfig.DirectoryToKind) > 0 || len(backendConfig.MultiKindDirs) > 0 {
		// Merge with default if needed (inherit missing fields)
		return c.mergeBackendConfig(backendConfig, defaultConfig)
	}

	// Fall back to default
	if len(defaultConfig.KindToDirectory) > 0 || len(defaultConfig.DirectoryToKind) > 0 || len(defaultConfig.MultiKindDirs) > 0 {
		return defaultConfig
	}

	// Last resort: use legacy fields for backward compatibility
	return BackendConfig{
		KindToDirectory: kindToDirectory,
		DirectoryToKind: directoryToKind,
		MultiKindDirs:   multiKindDirs,
	}
}

// mergeBackendConfig merges backend-specific config with default
// Backend-specific values take precedence
func (c *KindMappingsConfig) mergeBackendConfig(backend, defaultBackend BackendConfig) BackendConfig {
	// Record merge operation
	GetKindMappingsMetrics().RecordBackendConfigMerge()

	result := BackendConfig{
		KindToDirectory: make(map[string]string),
		DirectoryToKind: make(map[string]string),
		MultiKindDirs:   make(map[string]MultiKindConfig),
	}

	// Start with default values
	if defaultBackend.KindToDirectory != nil {
		maps.Copy(result.KindToDirectory, defaultBackend.KindToDirectory)
	}
	if defaultBackend.DirectoryToKind != nil {
		maps.Copy(result.DirectoryToKind, defaultBackend.DirectoryToKind)
	}
	if defaultBackend.MultiKindDirs != nil {
		maps.Copy(result.MultiKindDirs, defaultBackend.MultiKindDirs)
	}

	// Override with backend-specific values
	if backend.KindToDirectory != nil {
		maps.Copy(result.KindToDirectory, backend.KindToDirectory)
	}
	if backend.DirectoryToKind != nil {
		maps.Copy(result.DirectoryToKind, backend.DirectoryToKind)
	}
	if backend.MultiKindDirs != nil {
		maps.Copy(result.MultiKindDirs, backend.MultiKindDirs)
	}

	return result
}

// ShouldSkipDirectory checks if a directory should be skipped during discovery
func (c *KindMappingsConfig) ShouldSkipDirectory(dirName string) bool {
	for _, skipDir := range c.SkipDirectories {
		if dirName == skipDir || strings.HasPrefix(dirName, skipDir) {
			return true
		}
	}
	return false
}

// ShouldSkipSpec checks if a spec file should be skipped during discovery
func (c *KindMappingsConfig) ShouldSkipSpec(specName string) bool {
	baseName := strings.TrimSuffix(specName, yamlExt)
	baseName = strings.TrimSuffix(baseName, ymlExt)

	for _, skipSpec := range c.SkipSpecs {
		if baseName == skipSpec {
			return true
		}
	}
	return false
}

// GetDirectoryFromKind returns the directory name for a kind using config
func (c *KindMappingsConfig) GetDirectoryFromKind(kind string) string {
	metrics := GetKindMappingsMetrics()
	backendConfig := c.getBackendConfig()

	// Check explicit mapping first
	if dir, ok := backendConfig.KindToDirectory[kind]; ok {
		metrics.RecordDirectoryLookup(true, false)
		return dir
	}

	// Fall back to legacy field for backward compatibility
	if c.KindToDirectory != nil {
		if dir, ok := c.KindToDirectory[kind]; ok {
			metrics.RecordDirectoryLookup(true, false)
			return dir
		}
	}

	// Apply inference rules
	dir := c.inferDirectoryFromKind(kind)
	metrics.RecordDirectoryLookup(false, dir != kind)
	return dir
}

// GetKindFromDirectory returns the kind name for a directory using config
func (c *KindMappingsConfig) GetKindFromDirectory(dirName string) string {
	metrics := GetKindMappingsMetrics()
	backendConfig := c.getBackendConfig()

	// Check multi-kind directories first (they override explicit mappings)
	if multiKind, ok := backendConfig.MultiKindDirs[dirName]; ok {
		metrics.RecordKindLookup(true, false)
		return multiKind.DefaultKind
	}

	// Fall back to legacy field for backward compatibility
	if c.MultiKindDirs != nil {
		if multiKind, ok := c.MultiKindDirs[dirName]; ok {
			metrics.RecordKindLookup(true, false)
			return multiKind.DefaultKind
		}
	}

	// Check explicit mapping
	if kind, ok := backendConfig.DirectoryToKind[dirName]; ok {
		metrics.RecordKindLookup(true, false)
		return kind
	}

	// Fall back to legacy field for backward compatibility
	if c.DirectoryToKind != nil {
		if kind, ok := c.DirectoryToKind[dirName]; ok {
			metrics.RecordKindLookup(true, false)
			return kind
		}
	}

	// Apply inference rules
	kind := c.inferKindFromDirectory(dirName)
	metrics.RecordKindLookup(false, kind != dirName)
	return kind
}

// inferDirectoryFromKind applies inference rules to determine directory from kind
func (c *KindMappingsConfig) inferDirectoryFromKind(kind string) string {
	// Sort patterns by priority (lower number = higher priority)
	patterns := make([]PatternRule, len(c.InferenceRules.DirectoryPatterns))
	copy(patterns, c.InferenceRules.DirectoryPatterns)

	// Sort by priority (lower number = higher priority)
	sort.Slice(patterns, func(i, j int) bool {
		priI := patterns[i].Priority
		priJ := patterns[j].Priority
		// If priority is 0 (default), treat as lowest priority
		if priI == 0 {
			priI = 1000
		}
		if priJ == 0 {
			priJ = 1000
		}
		return priI < priJ
	})

	eventLogger := logging.NewEventLogger(pkgctx.NewSystemContext())

	for _, rule := range patterns {
		// Skip rules with conditions that aren't met
		if rule.Condition == conditionNoExplicit {
			// Only apply if no explicit mapping exists (already checked above)
		}

		re, err := getCachedRegexp(rule.Pattern)
		if err != nil {
			eventLogger.LogWarning(warnRegexCompileMsg,
				logging.String(logPatternField, rule.Pattern),
				logging.String(logRuleTypeField, ruleTypeDirPattern),
				logging.Error(err))
			continue
		}

		if re.MatchString(kind) {
			result := re.ReplaceAllString(kind, rule.Replacement)
			if result != kind {
				return result
			}
		}
	}

	// Default: return kind as-is (or could add 's' for plural)
	return kind
}

// inferKindFromDirectory applies inference rules to determine kind from directory
func (c *KindMappingsConfig) inferKindFromDirectory(dirName string) string {
	// Sort patterns by priority (lower number = higher priority)
	patterns := make([]PatternRule, len(c.InferenceRules.KindPatterns))
	copy(patterns, c.InferenceRules.KindPatterns)

	// Sort by priority (lower number = higher priority)
	sort.Slice(patterns, func(i, j int) bool {
		priI := patterns[i].Priority
		priJ := patterns[j].Priority
		// If priority is 0 (default), treat as lowest priority
		if priI == 0 {
			priI = 1000
		}
		if priJ == 0 {
			priJ = 1000
		}
		return priI < priJ
	})

	eventLogger := logging.NewEventLogger(pkgctx.NewSystemContext())

	for _, rule := range patterns {
		re, err := getCachedRegexp(rule.Pattern)
		if err != nil {
			eventLogger.LogWarning(warnRegexCompileMsg,
				logging.String(logPatternField, rule.Pattern),
				logging.String(logRuleTypeField, ruleTypeKindPattern),
				logging.Error(err))
			continue
		}

		if re.MatchString(dirName) {
			result := re.ReplaceAllString(dirName, rule.Replacement)
			if result != dirName {
				return result
			}
		}
	}

	// Default: return directory name as-is
	return dirName
}

// IsOnDemandKind checks if a kind should be recognized even if directory doesn't exist
func (c *KindMappingsConfig) IsOnDemandKind(kind string) bool {
	for _, onDemand := range c.OnDemandKinds {
		switch v := onDemand.(type) {
		case string:
			if v == kind {
				return true
			}
		case map[string]any:
			// Handle pattern objects like {"pattern": "^.+_metric$"}
			if pattern, ok := v["pattern"].(string); ok {
				re, err := getCachedRegexp(pattern)
				if err == nil && re.MatchString(kind) {
					return true
				}
			}
		}
	}
	return false
}

// findKindMappingsConfig finds the kind mappings config file
// Uses paths configuration instead of hardcoded paths
func findKindMappingsConfig() string {
	// Try to use validation package's paths config (avoid circular dependency by using direct path lookup)
	// We'll use a simple approach that doesn't require importing validation package
	configsRel := filepath.Join(paths.ProcessInternalConfigsDir, paths.KindMappingsConfigFile)
	possiblePaths := []string{
		configsRel,
		filepath.Join("..", configsRel),
		filepath.Join("..", "..", configsRel),
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

	// Walk up directory tree looking for markers (similar to paths_config logic)
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
			potentialPath := filepath.Join(dir, paths.ProcessInternalConfigsDir, paths.KindMappingsConfigFile)
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
