package config

import (
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const emptyValue = ""

// KindTolerance defines archive and cleanup tolerance for one object kind.
// When triggered (e.g. by retention_tolerance scheduler job):
// - Objects older than ArchiveAfter are updated to status "archived" (if kind supports it).
// - Objects older than CleanupAfter are deleted (only if status is not in ProtectStatuses).
// - If MaxCount > 0 and kind count exceeds MaxCount, oldest objects not in ProtectStatuses are deleted until at or under MaxCount.
type KindTolerance struct {
	// ArchiveAfter is the duration after which objects are marked archived (e.g. "24h", "720h"). Zero = skip archive.
	ArchiveAfter string `yaml:"archive_after,omitempty"`
	// CleanupAfter is the duration after which objects are deleted (e.g. "24h", "720h"). Zero = skip cleanup by age.
	CleanupAfter string `yaml:"cleanup_after,omitempty"`
	// MaxCount is the maximum number of objects to keep for this kind. Excess oldest are deleted. Zero = no limit.
	MaxCount int `yaml:"max_count,omitempty"`
	// ProtectStatuses lists statuses that must not be removed (active-like). Objects with these statuses are never deleted by cleanup or max_count.
	// If empty, DefaultProtectStatuses() is used so active/work-in-progress objects are never deleted.
	ProtectStatuses []string `yaml:"protect_statuses,omitempty"`
}

// RetentionToleranceConfig is the root config for retention tolerance (archive and cleanup) per kind.
type RetentionToleranceConfig struct {
	// Default applies to any kind not listed in Kinds.
	Default KindTolerance `yaml:"default,omitempty"`
	// Kinds maps object kind -> tolerance. Only kinds listed here (or using default) are processed when job runs.
	Kinds map[string]KindTolerance `yaml:"kinds,omitempty"`
}

// ParsedKindTolerance holds parsed durations and protect_statuses for a kind.
type ParsedKindTolerance struct {
	ArchiveAfter   time.Duration
	CleanupAfter   time.Duration
	MaxCount       int
	ArchiveEnabled bool
	CleanupEnabled bool
	// ProtectStatuses: objects with these statuses are never deleted (strategy does not remove active-like objects).
	ProtectStatuses []string
}

// DefaultProtectStatuses returns the default active-like statuses that must not be deleted when not overridden.
func DefaultProtectStatuses() []string {
	return []string{
		"in_progress", "planned", "draft", "open", "exploring", "validated",
		"published", "active", "pending", "running", "implemented",
		"in_review", "reviewed", "submitted", "staged",
	}
}

// ParseDuration parses a duration string. Supports Go format (24h, 720h) and "Nd" (days) as N*24h.
func ParseDuration(s string) (time.Duration, error) {
	if s == emptyPath || s == "0" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err == nil {
		return d, nil
	}
	// Try "Nd" (days)
	if strings.HasSuffix(s, "d") {
		daysStr := strings.TrimSuffix(s, "d")
		if days, err := strconv.Atoi(daysStr); err == nil && days >= 0 {
			return time.Duration(days) * 24 * time.Hour, nil
		}
	}
	return 0, errfmt.Errorf("invalid duration %q (use e.g. 24h, 720h, 30d)", s)
}

// GetToleranceForKind returns the effective (parsed) tolerance for a kind (kind-specific or default).
func (c *RetentionToleranceConfig) GetToleranceForKind(kind string) (ParsedKindTolerance, error) {
	t := c.Default
	if c.Kinds != nil {
		if k, ok := c.Kinds[kind]; ok {
			if k.ArchiveAfter != emptyValue {
				t.ArchiveAfter = k.ArchiveAfter
			}
			if k.CleanupAfter != emptyValue {
				t.CleanupAfter = k.CleanupAfter
			}
			if k.MaxCount > 0 {
				t.MaxCount = k.MaxCount
			}
			if k.ProtectStatuses != nil {
				t.ProtectStatuses = k.ProtectStatuses
			}
		}
	}
	var out ParsedKindTolerance
	out.MaxCount = t.MaxCount
	out.ProtectStatuses = t.ProtectStatuses
	if out.ProtectStatuses == nil {
		out.ProtectStatuses = DefaultProtectStatuses()
	}
	archive, err := ParseDuration(t.ArchiveAfter)
	if err != nil {
		return out, errfmt.Errorf("kind %q archive_after: %w", kind, err)
	}
	out.ArchiveAfter = archive
	out.ArchiveEnabled = archive > 0
	cleanup, err := ParseDuration(t.CleanupAfter)
	if err != nil {
		return out, errfmt.Errorf("kind %q cleanup_after: %w", kind, err)
	}
	out.CleanupAfter = cleanup
	out.CleanupEnabled = cleanup > 0
	return out, nil
}

// EnabledKinds returns the list of kinds that have explicit config (and thus are processed).
// If Kinds is nil or empty, returns nil (caller may treat as "use default for no kinds" or discover kinds).
func (c *RetentionToleranceConfig) EnabledKinds() []string {
	if len(c.Kinds) == 0 {
		return nil
	}
	kinds := make([]string, 0, len(c.Kinds))
	for k := range c.Kinds {
		kinds = append(kinds, k)
	}
	return kinds
}

// RetentionToleranceLoader loads RetentionToleranceConfig from YAML.
type RetentionToleranceLoader struct {
	configPath string
	cache      *RetentionToleranceConfig
	mu         sync.RWMutex
}

// NewRetentionToleranceLoader creates a loader. configPath is optional; if empty, path is resolved from projectRoot.
func NewRetentionToleranceLoader(projectRoot string) *RetentionToleranceLoader {
	path := MaintenanceRetentionToleranceConfig().Safe()
	if path == emptyPath && projectRoot != emptyPath {
		path = filepath.Join(projectRoot, paths.ProcessInternalConfigsDir, "retention_tolerance.yaml")
	}
	return &RetentionToleranceLoader{configPath: path}
}

// Load loads and returns the retention tolerance config. Missing file returns default (no kinds) and nil error.
func (l *RetentionToleranceLoader) Load() (*RetentionToleranceConfig, error) {
	var c *RetentionToleranceConfig
	err := concurrency.RunInLock(&l.mu, func() error {
		if l.configPath == emptyPath {
			c = &RetentionToleranceConfig{}
			return nil
		}
		data, readErr := fileutil.ReadFile(l.configPath)
		if readErr != nil {
			if fileutil.IsNotExist(readErr) {
				c = &RetentionToleranceConfig{}
				return nil
			}
			return errfmt.Newf("read retention tolerance config").Wrap(readErr)
		}
		var parsed RetentionToleranceConfig
		if parseErr := yaml.Unmarshal(data, &parsed); parseErr != nil {
			return errfmt.Newf("parse retention tolerance config").Wrap(parseErr)
		}
		l.cache = &parsed
		c = &parsed
		return nil
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

// GetConfigPath returns the config file path used by this loader.
func (l *RetentionToleranceLoader) GetConfigPath() string {
	return l.configPath
}

// ParseToleranceFromMap parses a retention_tolerance map (from YAML or bucketing_strategy object) into ParsedKindTolerance.
// Used when merging strategy-level retention_tolerance from bucketing_strategy spec.
func ParseToleranceFromMap(m map[string]any, kind string) (ParsedKindTolerance, error) {
	var out ParsedKindTolerance
	if m == nil {
		out.ProtectStatuses = DefaultProtectStatuses()
		return out, nil
	}
	if s, ok := m["archive_after"].(string); ok && s != emptyPath {
		d, err := ParseDuration(s)
		if err != nil {
			return out, errfmt.Errorf("kind %q archive_after: %w", kind, err)
		}
		out.ArchiveAfter = d
		out.ArchiveEnabled = d > 0
	}
	if s, ok := m["cleanup_after"].(string); ok && s != emptyPath {
		d, err := ParseDuration(s)
		if err != nil {
			return out, errfmt.Errorf("kind %q cleanup_after: %w", kind, err)
		}
		out.CleanupAfter = d
		out.CleanupEnabled = d > 0
	}
	switch v := m["max_count"].(type) {
	case int:
		if v > 0 {
			out.MaxCount = v
		}
	case float64:
		if v > 0 {
			out.MaxCount = int(v)
		}
	}
	if arr, ok := m["protect_statuses"].([]any); ok {
		out.ProtectStatuses = make([]string, 0, len(arr))
		for _, v := range arr {
			if s, ok := v.(string); ok && s != emptyPath {
				out.ProtectStatuses = append(out.ProtectStatuses, s)
			}
		}
	}
	if out.ProtectStatuses == nil {
		out.ProtectStatuses = DefaultProtectStatuses()
	}
	return out, nil
}
