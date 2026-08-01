package storage

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/loader"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"gopkg.in/yaml.v3"
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
// Load uses the component loader pattern (pkg/loader) for consistent timeout and wait-for-completion behavior.
type BucketingConfigRegistry struct {
	configs    map[string]*BucketingConfig
	configPath string
	runner     *loader.Runner
	runnerOnce sync.Once
}

// NewBucketingConfigRegistry creates a new bucketing config registry
func NewBucketingConfigRegistry(projectRoot string) *BucketingConfigRegistry {
	configPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.ConfigDir, paths.ProjectConfigFile)
	return &BucketingConfigRegistry{
		configs:    make(map[string]*BucketingConfig),
		configPath: configPath,
	}
}

// getRunner returns the shared loader.Runner for this registry (lazily created).
func (bcr *BucketingConfigRegistry) getRunner() *loader.Runner {
	bcr.runnerOnce.Do(func() {
		bcr.runner = loader.NewRunner(ConstMiscBucketingConfig, func(ctx context.Context) error {
			return bcr.doLoad(ctx)
		})
	})
	return bcr.runner
}

// Load loads bucketing configurations from .zqk/config/config.yaml via the component loader pattern.
func (bcr *BucketingConfigRegistry) Load() error {
	return bcr.getRunner().Load(pkgctx.NewSystemContext())
}

// doLoad performs the actual I/O; called by loader.Runner.
func (bcr *BucketingConfigRegistry) doLoad(ctx context.Context) error {
	// Check if config file exists
	if _, err := os.Stat(bcr.configPath); os.IsNotExist(err) {
		bcr.setDefaults()
		return nil
	}

	data, err := os.ReadFile(bcr.configPath)
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToReadConfigFile).Wrap(err)
	}

	var config struct {
		Storage struct {
			Bucketing map[string]*BucketingConfig `yaml:"bucketing"`
		} `yaml:"storage"`
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		bcr.setDefaults()
		return nil
	}

	for kind, cfg := range config.Storage.Bucketing {
		configCopy := *cfg
		bcr.configs[kind] = &configCopy
	}

	bcr.setDefaults()
	return nil
}

// setDefaults sets default bucketing configurations
func (bcr *BucketingConfigRegistry) setDefaults() {
	// Default bucketing configurations
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

	// Only set defaults if not already configured
	for kind, cfg := range defaults {
		if _, exists := bcr.configs[kind]; !exists {
			bcr.configs[kind] = cfg
		}
	}
}

// GetConfig returns the bucketing configuration for a kind
func (bcr *BucketingConfigRegistry) GetConfig(kind string) *BucketingConfig {
	if cfg, ok := bcr.configs[kind]; ok {
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
	entries, err := os.ReadDir(dir)
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
