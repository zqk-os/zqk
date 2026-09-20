package storage

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/objects"
)

func stringContains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestBucketingConfigRegistry_Defaults(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	registry := NewBucketingConfigRegistry(tmpDir)

	// Load defaults
	if err := registry.Load(); err != nil {
		t.Fatalf("Failed to load defaults: %v", err)
	}

	// Check default configurations
	auditConfig := registry.GetConfig("audit_event")
	if auditConfig == nil {
		t.Fatal("audit_event config is nil")
	}
	if !auditConfig.Enabled {
		t.Error("audit_event should be enabled")
	}
	if auditConfig.Strategy != BucketingStrategyChronologicalMonthly {
		t.Errorf("Expected strategy %s, got %s", BucketingStrategyChronologicalMonthly, auditConfig.Strategy)
	}

	changeJournalConfig := registry.GetConfig("change_journal_entry")
	if changeJournalConfig == nil {
		t.Fatal("change_journal_entry config is nil")
	}
	if !changeJournalConfig.Enabled {
		t.Error("change_journal_entry should be enabled")
	}
	if changeJournalConfig.Strategy != BucketingStrategyChronologicalMonthly {
		t.Errorf("Expected strategy %s, got %s", BucketingStrategyChronologicalMonthly, changeJournalConfig.Strategy)
	}

	// Check non-bucketed kind
	backlogConfig := registry.GetConfig("backlog_item")
	if backlogConfig == nil {
		t.Fatal("backlog_item config is nil")
	}
	if backlogConfig.Enabled {
		t.Error("backlog_item should not be enabled")
	}
	if backlogConfig.Strategy != BucketingStrategyNone {
		t.Errorf("Expected strategy %s, got %s", BucketingStrategyNone, backlogConfig.Strategy)
	}
}

func TestBucketingConfigRegistry_IsBucketed(t *testing.T) {
	tmpDir := t.TempDir()
	registry := NewBucketingConfigRegistry(tmpDir)
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = registry.Load()

	if !registry.IsBucketed("audit_event") {
		t.Error("audit_event should be bucketed")
	}
	if !registry.IsBucketed("change_journal_entry") {
		t.Error("change_journal_entry should be bucketed")
	}
	if registry.IsBucketed("backlog_item") {
		t.Error("backlog_item should not be bucketed")
	}
	if registry.IsBucketed("unknown_kind") {
		t.Error("unknown_kind should not be bucketed")
	}
}

func TestBucketingConfigRegistry_GetBucketPath(t *testing.T) {
	tmpDir := t.TempDir()
	registry := NewBucketingConfigRegistry(tmpDir)
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = registry.Load()

	baseDir := filepath.Join(tmpDir, "audit")

	tests := []struct {
		name     string
		kind     string
		obj      map[string]any
		expected string
	}{
		{
			name: "monthly_bucketing",
			kind: "audit_event",
			obj: map[string]any{
				objects.FieldKeyCreatedAt: "2025-12-29T10:00:00Z",
			},
			expected: filepath.Join(baseDir, "2025-12"),
		},
		{
			name: "no_bucketing",
			kind: "backlog_item",
			obj: map[string]any{
				objects.FieldKeyCreatedAt: "2025-12-29T10:00:00Z",
			},
			expected: baseDir,
		},
		{
			name:     "no_created_at",
			kind:     "audit_event",
			obj:      map[string]any{},
			expected: filepath.Join(baseDir, time.Now().UTC().Format("2006-01")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := registry.GetBucketPath(tt.kind, tt.obj, baseDir)
			if err != nil {
				t.Fatalf("GetBucketPath failed: %v", err)
			}
			// For the no_created_at case, we can't predict the exact month, so just check it's a date path
			if tt.name == "no_created_at" {
				if !stringContains(path, baseDir) {
					t.Errorf("Path %s should contain %s", path, baseDir)
				}
				// Check if base name matches date pattern
				base := filepath.Base(path)
				matched, err := regexp.MatchString(`\d{4}-\d{2}$`, base)
				if err != nil {
					t.Errorf("Regex match failed: %v", err)
				} else if !matched {
					t.Errorf("Path base %s should match date pattern", base)
				}
			} else if path != tt.expected {
				t.Errorf("Expected path %s, got %s", tt.expected, path)
			}
		})
	}
}

func TestBucketingConfigRegistry_GetBucketPathForDate(t *testing.T) {
	tmpDir := t.TempDir()
	registry := NewBucketingConfigRegistry(tmpDir)
	if err := registry.Load(); err != nil {
		t.Fatalf("Failed to load: %v", err)
	}

	baseDir := filepath.Join(tmpDir, "audit")
	date := time.Date(2025, 12, 29, 10, 0, 0, 0, time.UTC)

	// Monthly bucketing
	path := registry.GetBucketPathForDate("audit_event", date, baseDir)
	expected := filepath.Join(baseDir, "2025-12")
	if path != expected {
		t.Errorf("Expected path %s, got %s", expected, path)
	}

	// No bucketing
	path = registry.GetBucketPathForDate("backlog_item", date, baseDir)
	if path != baseDir {
		t.Errorf("Expected path %s, got %s", baseDir, path)
	}
}

func TestBucketingConfigRegistry_LoadFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	// Use the correct path that BucketingConfigRegistry expects (.zqk/config/config.yaml)
	configPath := filepath.Join(tmpDir, paths.ProjectDataDir, "config", "config.yaml")
	_ = fileutil.MkdirAll(filepath.Dir(configPath), paths.DirPerm755)

	// Create config file
	configContent := `storage:
  bucketing:
    audit_event:
      strategy: chronological_monthly
      category_threshold: 500
      enabled: true
    custom_kind:
      strategy: chronological_daily
      enabled: true
`

	err := fileutil.WriteFile(configPath, []byte(configContent), paths.FilePerm644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	registry := NewBucketingConfigRegistry(tmpDir)
	err = registry.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Check configured audit_event
	auditConfig := registry.GetConfig("audit_event")
	if auditConfig == nil {
		t.Fatal("audit_event config is nil")
	}
	if !auditConfig.Enabled {
		t.Error("audit_event should be enabled")
	}
	if auditConfig.Strategy != BucketingStrategyChronologicalMonthly {
		t.Errorf("Expected strategy %s, got %s", BucketingStrategyChronologicalMonthly, auditConfig.Strategy)
	}
	if auditConfig.CategoryThreshold != 500 {
		t.Errorf("Expected category threshold 500, got %d", auditConfig.CategoryThreshold)
	}

	// Check configured custom_kind
	customConfig := registry.GetConfig("custom_kind")
	if customConfig == nil {
		t.Fatal("custom_kind config is nil")
	}
	if !customConfig.Enabled {
		t.Error("custom_kind should be enabled")
	}
	if customConfig.Strategy != BucketingStrategyChronologicalDaily {
		t.Errorf("Expected strategy %s, got %s", BucketingStrategyChronologicalDaily, customConfig.Strategy)
	}
}

func TestSanitizeCategory(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "hash_mismatch_fix", "hash_mismatch_fix"},
		{"with_slash", "hash/mismatch", "hash_mismatch"},
		{"with_colon", "event:type", "event_type"},
		{"with_spaces", "event type", "event_type"},
		{"with_dots", ".event.type.", "event_type"},
		{"long", string(make([]byte, 150)), string(make([]byte, 100))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeCategory(tt.input)
			if tt.name == "long" {
				if len(result) > 100 {
					t.Errorf("Result length %d should be <= 100", len(result))
				}
			} else {
				if result != tt.expected {
					t.Errorf("Expected %s, got %s", tt.expected, result)
				}
			}
		})
	}
}
