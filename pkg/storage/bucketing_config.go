// Unused config.yaml overlay for CAS folder layout (storage.bucketing).
// Not wired into FileObjectStorage. Live layout is bucketing_strategy objects
// plus CAS-only defaults in determineDefaultStrategy.
// TRACK: docs/architecture/HIGH_VOLUME_STORAGE_DEPRECATION.md — delete this overlay
// when: no revival of storage.bucketing in project config.yaml.
package storage

import (
	"path/filepath"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// BucketingStrategy defines the bucketing strategy for an object kind
type BucketingStrategy string

const (
	// BucketingStrategyNone means no bucketing (flat storage)
	BucketingStrategyNone BucketingStrategy = "none"

	// BucketingStrategyChronologicalMonthly means monthly chronological bucketing (YYYY-MM)
	BucketingStrategyChronologicalMonthly BucketingStrategy = "chronological_monthly"

	// BucketingStrategyChronologicalDaily means daily chronological bucketing (YYYY-MM-DD)
	BucketingStrategyChronologicalDaily BucketingStrategy = "chronological_daily"

	// BucketingStrategyCategorical means categorical bucketing (by category field)
	BucketingStrategyCategorical BucketingStrategy = "categorical"

	// BucketingStrategyHybrid means hybrid bucketing (date + category)
	BucketingStrategyHybrid BucketingStrategy = "hybrid"
)

// BucketingConfig defines the bucketing configuration for an object kind
type BucketingConfig struct {
	// Strategy is the bucketing strategy to use
	Strategy BucketingStrategy `yaml:"strategy"`

	// CategoryThreshold is the threshold for adding category sub-buckets (for hybrid strategy)
	// If a month exceeds this many files, category sub-buckets are added
	CategoryThreshold int `yaml:"category_threshold,omitempty"`

	// CategoryField is the field name to use for categorical bucketing
	CategoryField string `yaml:"category_field,omitempty"`

	// Enabled indicates whether bucketing is enabled for this kind
	Enabled bool `yaml:"enabled"`
}

// BucketingConfigRegistry manages bucketing configurations per object kind.
// YAML is stamp-invalidated from config/zqk.yaml (legacy .zqk/config copies fallback).
type BucketingConfigRegistry struct {
	configPath string
}

var bucketingConfigs stampmemo.Table[map[string]*BucketingConfig] // keyed by config path

// NewBucketingConfigRegistry creates a new bucketing config registry
func NewBucketingConfigRegistry(projectRoot string) *BucketingConfigRegistry {
	configPath := paths.FirstProjectYAMLConfig(projectRoot)
	return &BucketingConfigRegistry{configPath: configPath}
}

// Load loads bucketing configurations from config/zqk.yaml.
func (bcr *BucketingConfigRegistry) Load() error {
	_, err := bucketingConfigs.Load(bcr.configPath, stampmemo.Of(bcr.configPath), bcr.readConfigs)
	if err != nil {
		bucketingConfigs.Delete(bcr.configPath)
	}
	return err
}

func (bcr *BucketingConfigRegistry) kindConfigs() map[string]*BucketingConfig {
	cfg, _ := bucketingConfigs.Load(bcr.configPath, stampmemo.Of(bcr.configPath), bcr.readConfigs)
	if cfg == nil {
		return map[string]*BucketingConfig{}
	}
	return cfg
}

func (bcr *BucketingConfigRegistry) readConfigs() (map[string]*BucketingConfig, error) {
	out := make(map[string]*BucketingConfig)
	if _, err := fileutil.Stat(bcr.configPath); fileutil.IsNotExist(err) {
		applyBucketingDefaults(out)
		return out, nil
	}

	data, err := fileutil.ReadFile(bcr.configPath)
	if err != nil {
		return nil, errfmt.Newf(ConstMiscFailedToReadConfigFile).Wrap(err)
	}

	var config struct {
		Storage struct {
			Bucketing map[string]*BucketingConfig `yaml:"bucketing"`
		} `yaml:"storage"`
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		applyBucketingDefaults(out)
		return out, nil
	}

	for kind, cfg := range config.Storage.Bucketing {
		configCopy := *cfg
		out[kind] = &configCopy
	}
	applyBucketingDefaults(out)
	return out, nil
}

func applyBucketingDefaults(configs map[string]*BucketingConfig) {
	defaults := map[string]*BucketingConfig{
		objects.KindAuditEvent: {
			Strategy:          BucketingStrategyChronologicalMonthly,
			CategoryThreshold: 1000,
			Enabled:           true,
		},
		objects.KindChangeJournalEntry: {
			Strategy: BucketingStrategyChronologicalMonthly,
			Enabled:  true,
		},
		objects.KindIntegrityManifest: {
			Strategy: BucketingStrategyChronologicalMonthly,
			Enabled:  true,
		},
	}
	for kind, cfg := range defaults {
		if _, exists := configs[kind]; !exists {
			configs[kind] = cfg
		}
	}
}

// GetConfig returns the bucketing configuration for a kind
func (bcr *BucketingConfigRegistry) GetConfig(kind string) *BucketingConfig {
	if cfg, ok := bcr.kindConfigs()[kind]; ok {
		return cfg
	}

	// Return default (no bucketing)
	return &BucketingConfig{
		Strategy: BucketingStrategyNone,
		Enabled:  false,
	}
}

// IsBucketed checks if a kind uses bucketed storage
func (bcr *BucketingConfigRegistry) IsBucketed(kind string) bool {
	cfg := bcr.GetConfig(kind)
	return cfg.Enabled && cfg.Strategy != BucketingStrategyNone
}

// GetBucketPath returns the bucket path for an object based on its configuration
func (bcr *BucketingConfigRegistry) GetBucketPath(kind string, obj map[string]any, baseDir string) (string, error) {
	cfg := bcr.GetConfig(kind)

	if !cfg.Enabled || cfg.Strategy == BucketingStrategyNone {
		return baseDir, nil
	}

	// Extract date from created_at or updated_at
	var objTime time.Time
	var err error

	if createdAt := objects.GetString(obj, objects.FieldKeyCreatedAt); createdAt != emptyValue {
		objTime, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			// Try alternative format
			objTime, err = time.Parse(ConstMisc20060102t150405z, createdAt)
		}
		if err != nil {
			// Fallback to current time
			objTime = time.Now().UTC()
		}
	} else {
		// No created_at, use current time
		objTime = time.Now().UTC()
	}

	switch cfg.Strategy {
	case BucketingStrategyChronologicalMonthly:
		month := objTime.Format("2006-01")
		return filepath.Join(baseDir, month), nil

	case BucketingStrategyChronologicalDaily:
		day := objTime.Format("2006-01-02")
		return filepath.Join(baseDir, day), nil

	case BucketingStrategyCategorical:
		if cfg.CategoryField == emptyValue {
			return baseDir, nil
		}
		category, ok := obj[cfg.CategoryField].(string)
		if !ok || category == emptyValue {
			return baseDir, nil
		}
		// Sanitize category for filesystem
		category = sanitizeCategory(category)
		return filepath.Join(baseDir, category), nil

	case BucketingStrategyHybrid:
		month := objTime.Format("2006-01")
		monthDir := filepath.Join(baseDir, month)

		// Check if category sub-buckets are needed
		if cfg.CategoryThreshold > 0 {
			// Count files in month directory
			count, err := countFilesInDirectory(monthDir)
			if err == nil && count > cfg.CategoryThreshold {
				// Use category sub-bucket
				if cfg.CategoryField == emptyValue {
					// Default to event_type for audit events
					cfg.CategoryField = "event_type"
				}
				category, ok := obj[cfg.CategoryField].(string)
				if ok && category != emptyValue {
					category = sanitizeCategory(category)
					return filepath.Join(monthDir, category), nil
				}
			}
		}

		return monthDir, nil

	default:
		return baseDir, nil
	}
}

// sanitizeCategory sanitizes a category name for use in filesystem paths
func sanitizeCategory(category string) string {
	// Replace invalid filesystem characters, spaces, and dots with underscores
	invalidChars := regexp.MustCompile(`[<>:"/\\|?*\s.]`)
	sanitized := invalidChars.ReplaceAllString(category, "_")

	// Remove leading/trailing underscores
	sanitized = regexp.MustCompile(`^_+|_+$`).ReplaceAllString(sanitized, "")

	// Limit length
	if len(sanitized) > 100 {
		sanitized = sanitized[:100]
	}

	return sanitized
}

// countFilesInDirectory counts the number of YAML files in a directory
func countFilesInDirectory(dir string) (int, error) {
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			name := entry.Name()
			if filepath.Ext(name) == ".yaml" || filepath.Ext(name) == ".yml" {
				count++
			}
		}
	}

	return count, nil
}

// GetBucketPathForDate returns the bucket path for a given date
func (bcr *BucketingConfigRegistry) GetBucketPathForDate(kind string, date time.Time, baseDir string) string {
	cfg := bcr.GetConfig(kind)

	if !cfg.Enabled || cfg.Strategy == BucketingStrategyNone {
		return baseDir
	}

	switch cfg.Strategy {
	case BucketingStrategyChronologicalMonthly:
		month := date.Format("2006-01")
		return filepath.Join(baseDir, month)

	case BucketingStrategyChronologicalDaily:
		day := date.Format("2006-01-02")
		return filepath.Join(baseDir, day)

	default:
		return baseDir
	}
}
