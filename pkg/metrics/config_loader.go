package metrics

import (
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// SamplerConfigFile represents the sampler configuration file structure.
// Profile refs resolve from `.zqk/metrics/profiles/` (`paths.MetricsProfilesDir`).
type SamplerConfigFile struct {
	Defaults struct {
		Enabled         bool   `yaml:"enabled"`
		BatchSize       int    `yaml:"batch_size"`
		MaxBatchSize    int    `yaml:"max_batch_size"`
		FlushInterval   string `yaml:"flush_interval"` // Duration string (e.g., "5m")
		GroupByObjectID bool   `yaml:"group_by_object_id"`
	} `yaml:"defaults"`

	// Profile references — names of profiles to load from MetricsProfilesDir
	ProfileRefs []string `yaml:"profile_refs,omitempty"`

	// Inline profiles (alternative to profile_refs, for quick overrides)
	// These follow the unified profile schema but are defined inline
	Profiles map[string]map[string]any `yaml:"profiles,omitempty"`

	// Legacy: Direct sampler configs (deprecated, use profiles instead)
	Samplers []struct {
		ObjectKind      string `yaml:"object_kind"`
		FieldName       string `yaml:"field_name,omitempty"`
		MetricType      string `yaml:"metric_type"`
		Enabled         bool   `yaml:"enabled"`
		BatchSize       int    `yaml:"batch_size"`
		MaxBatchSize    int    `yaml:"max_batch_size"`
		FlushInterval   string `yaml:"flush_interval"`
		GroupByObjectID bool   `yaml:"group_by_object_id"`
		Description     string `yaml:"description,omitempty"`
	} `yaml:"samplers,omitempty"`

	OptOut []string `yaml:"opt_out"` // Object kinds that should never use sampling
	OptIn  []string `yaml:"opt_in"`  // Object kinds that require explicit opt-in
}

// LoadSamplerConfig loads sampler configuration from a YAML file
func LoadSamplerConfig(configPath string) (*SamplerConfigFile, error) {
	data, err := fileutil.ReadFile(configPath)
	if err != nil {
		return nil, errfmt.Newf("failed to read sampler config").Wrap(err)
	}

	var config SamplerConfigFile
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, errfmt.Newf("failed to parse sampler config").Wrap(err)
	}

	return &config, nil
}

// FindSamplerConfigFile finds the sampler configuration file in the project
func FindSamplerConfigFile(projectRoot string) (string, error) {
	// Try standard location (legacy path)
	configPath := filepath.Join(projectRoot, paths.ProcessInternalConfigsDir, "metrics_sampler_config"+paths.YAMLExtension)
	if _, err := fileutil.Stat(configPath); err == nil {
		return configPath, nil
	}

	// Try alternative location
	configPath = filepath.Join(projectRoot, paths.ProjectDataDir, paths.MetricsDir, paths.SamplerConfigFile)
	if _, err := fileutil.Stat(configPath); err == nil {
		return configPath, nil
	}

	return "", errfmt.Errorf("sampler config file not found")
}

// ApplySamplerConfig applies sampler configuration to a registry
func ApplySamplerConfig(registry *SamplerRegistry, configFile *SamplerConfigFile, projectRoot string) error {
	// Parse default flush interval
	defaultFlushInterval, err := time.ParseDuration(configFile.Defaults.FlushInterval)
	if err != nil {
		return errfmt.Newf("invalid default flush_interval").Wrap(err)
	}

	// Create metrics namespace profile loader
	profileLoader := NewProfileLoader("")

	// Load profiles from profile_refs (metrics namespace profiles)
	if len(configFile.ProfileRefs) > 0 {
		for _, profileName := range configFile.ProfileRefs {
			samplerProfile, err := profileLoader.LoadProfile(profileName)
			if err != nil {
				return errfmt.Errorf("failed to load metrics profile %s: %w", profileName, err)
			}

			// Apply the profile
			if err := applySamplerProfile(registry, samplerProfile, configFile, defaultFlushInterval); err != nil {
				return errfmt.Errorf("failed to apply profile %s: %w", profileName, err)
			}
		}
	}

	// Apply inline profiles (if any) - these are parsed as metrics profiles
	if configFile.Profiles != nil {
		for profileName, profileData := range configFile.Profiles {
			// Parse inline profile as metrics sampler profile
			profileBytes, err := yaml.Marshal(profileData)
			if err != nil {
				return errfmt.Errorf("failed to marshal inline profile %s: %w", profileName, err)
			}

			var samplerProfile SamplerProfile
			if err := yaml.Unmarshal(profileBytes, &samplerProfile); err != nil {
				return errfmt.Errorf("failed to parse inline profile %s: %w", profileName, err)
			}

			// Validate namespace_id (white-label aware)
			expectedNamespaceID := paths.MetricsNamespaceID
			if samplerProfile.NamespaceID == emptyValue {
				samplerProfile.NamespaceID = expectedNamespaceID
			}
			if samplerProfile.NamespaceID != expectedNamespaceID {
				// Accept legacy prefixes as long as the suffix matches, then normalize.
				if strings.HasSuffix(samplerProfile.NamespaceID, ":kernel:metrics") {
					samplerProfile.NamespaceID = expectedNamespaceID
				} else {
					return errfmt.Errorf("inline profile %s has invalid namespace_id: %s (expected %s)",
						profileName, samplerProfile.NamespaceID, expectedNamespaceID)
				}
			}

			// Resolve inheritance if needed
			if samplerProfile.Extends != emptyValue {
				parentProfile, err := profileLoader.LoadProfile(samplerProfile.Extends)
				if err != nil {
					return errfmt.Errorf("failed to load parent profile %s for inline profile %s: %w", samplerProfile.Extends, profileName, err)
				}
				// Merge parent's resolved spec
				for k, v := range parentProfile.ResolvedSpec {
					if _, exists := samplerProfile.ResolvedSpec[k]; !exists {
						samplerProfile.ResolvedSpec[k] = v
					}
				}
			} else {
				// Initialize resolved spec
				samplerProfile.ResolvedSpec = samplerProfile.Spec
			}

			// Apply the profile
			if err := applySamplerProfile(registry, &samplerProfile, configFile, defaultFlushInterval); err != nil {
				return errfmt.Errorf("failed to apply inline profile %s: %w", profileName, err)
			}
		}
	}

	// Apply legacy direct sampler configurations (for backward compatibility)
	for _, samplerConfig := range configFile.Samplers {
		// Parse flush interval
		flushInterval := defaultFlushInterval
		if samplerConfig.FlushInterval != emptyValue {
			parsed, err := time.ParseDuration(samplerConfig.FlushInterval)
			if err != nil {
				return errfmt.Errorf("invalid flush_interval for %s: %w", samplerConfig.ObjectKind, err)
			}
			flushInterval = parsed
		}

		// Use defaults if not specified
		batchSize := samplerConfig.BatchSize
		if batchSize == 0 {
			batchSize = configFile.Defaults.BatchSize
		}

		maxBatchSize := samplerConfig.MaxBatchSize
		if maxBatchSize == 0 {
			maxBatchSize = configFile.Defaults.MaxBatchSize
		}

		enabled := samplerConfig.Enabled
		if !configFile.Defaults.Enabled {
			enabled = false // Global disable overrides
		}

		groupByObjectID := samplerConfig.GroupByObjectID
		// If not explicitly set, use default
		if !samplerConfig.GroupByObjectID && !configFile.Defaults.GroupByObjectID {
			groupByObjectID = configFile.Defaults.GroupByObjectID
		}

		// Check opt-out list
		if contains(configFile.OptOut, samplerConfig.ObjectKind) {
			enabled = false
		}

		// Check opt-in list (if not empty, only listed kinds are enabled)
		if len(configFile.OptIn) > 0 && !contains(configFile.OptIn, samplerConfig.ObjectKind) {
			enabled = false
		}

		// Create sampler config
		samplerCfg := &SamplerConfig{
			Enabled:         enabled,
			BatchSize:       batchSize,
			MaxBatchSize:    maxBatchSize,
			FlushInterval:   flushInterval,
			GroupByObjectID: groupByObjectID,
			MetricType:      samplerConfig.MetricType,
			ObjectKind:      samplerConfig.ObjectKind,
			FieldName:       samplerConfig.FieldName,
		}

		// Register sampler
		if err := registry.RegisterSampler(samplerCfg); err != nil {
			return errfmt.Errorf("failed to register sampler for %s: %w", samplerConfig.ObjectKind, err)
		}
	}

	return nil
}

// applySamplerProfile applies a metrics sampler profile to the registry
func applySamplerProfile(registry *SamplerRegistry, samplerProfile *SamplerProfile, configFile *SamplerConfigFile, defaultFlushInterval time.Duration) error {
	// Extract sampler-specific fields from resolved spec
	spec := samplerProfile.ResolvedSpec

	// Parse flush interval
	flushInterval := defaultFlushInterval
	if flushIntervalStr, ok := spec[objects.FieldKeyFlushInterval].(string); ok && flushIntervalStr != emptyValue {
		parsed, err := time.ParseDuration(flushIntervalStr)
		if err != nil {
			return errfmt.Newf("invalid flush_interval").Wrap(err)
		}
		flushInterval = parsed
	}

	// Extract other fields
	batchSize := configFile.Defaults.BatchSize
	if bs, ok := spec[objects.FieldKeyBatchSize].(int); ok && bs > 0 {
		batchSize = bs
	}

	maxBatchSize := configFile.Defaults.MaxBatchSize
	if mbs, ok := spec[objects.FieldKeyMaxBatchSize].(int); ok && mbs > 0 {
		maxBatchSize = mbs
	}

	enabled := configFile.Defaults.Enabled
	if e, ok := spec[objects.FieldKeyEnabled].(bool); ok {
		enabled = e
	}

	groupByObjectID := configFile.Defaults.GroupByObjectID
	if gboid, ok := spec[objects.FieldKeyGroupByObjectID].(bool); ok {
		groupByObjectID = gboid
	}

	metricType := MetricTypeSystem
	if mt, ok := spec[objects.FieldKeyMetricType].(string); ok && mt != emptyValue {
		metricType = mt
	}

	// Get applies_to list
	var appliesTo []string
	if at, ok := spec[objects.FieldKeyAppliesTo].([]any); ok {
		for _, item := range at {
			if str, ok := item.(string); ok {
				appliesTo = append(appliesTo, str)
			}
		}
	}

	// Check if this is the default profile
	isDefault := false
	if id, ok := spec[objects.FieldKeyIsDefault].(bool); ok {
		isDefault = id
	}

	// Apply profile to specified object kinds
	if len(appliesTo) == 0 && !isDefault {
		// Profile doesn't specify applies_to and isn't default - skip
		return nil
	}

	// If default profile, we'll apply it when no specific profile matches
	// For now, we'll register it for all metric-enabled objects (handled elsewhere)
	if isDefault {
		// Store default profile for later use
		// This would be handled by a registry method like SetDefaultProfile
		return nil
	}

	// Apply to each object kind
	for _, objectKind := range appliesTo {
		// Check opt-out list
		if contains(configFile.OptOut, objectKind) {
			continue
		}

		// Check opt-in list (if not empty, only listed kinds are enabled)
		if len(configFile.OptIn) > 0 && !contains(configFile.OptIn, objectKind) {
			continue
		}

		// Create sampler config from profile
		samplerCfg := &SamplerConfig{
			Enabled:         enabled,
			BatchSize:       batchSize,
			MaxBatchSize:    maxBatchSize,
			FlushInterval:   flushInterval,
			GroupByObjectID: groupByObjectID,
			MetricType:      metricType,
			ObjectKind:      objectKind,
			FieldName:       "", // Profiles don't specify field names
		}

		// Register sampler
		if err := registry.RegisterSampler(samplerCfg); err != nil {
			return errfmt.Errorf("failed to register sampler from profile %s for %s: %w", samplerProfile.Name, objectKind, err)
		}
	}

	return nil
}

// contains checks if a string slice contains a value
func contains(slice []string, value string) bool {
	for _, s := range slice {
		if s == value {
			return true
		}
	}
	return false
}

// ShouldUseSampling determines if sampling should be used for an object kind
func ShouldUseSampling(config *SamplerConfigFile, objectKind string) bool {
	// Check opt-out list
	if contains(config.OptOut, objectKind) {
		return false
	}

	// Check opt-in list (if not empty, only listed kinds are enabled)
	if len(config.OptIn) > 0 {
		return contains(config.OptIn, objectKind)
	}

	// Default: enabled if global defaults are enabled
	return config.Defaults.Enabled
}
