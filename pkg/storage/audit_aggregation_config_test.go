package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestLoadAggregationConfig_Default(t *testing.T) {
	tmpDir := t.TempDir()

	// No config file - should return defaults
	config, err := LoadAggregationConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadAggregationConfig() error = %v", err)
	}

	if !config.Enabled {
		t.Error("Expected config.Enabled = true (default)")
	}
	if config.WindowSize != "1h" {
		t.Errorf("Expected config.WindowSize = '1h', got %v", config.WindowSize)
	}
	if config.Threshold != 10 {
		t.Errorf("Expected config.Threshold = 10, got %v", config.Threshold)
	}
	if len(config.Rules) == 0 {
		t.Error("Expected default rules to be loaded")
	}
}

func TestLoadAggregationConfig_FromFile(t *testing.T) {
	tmpDir := t.TempDir()
	// Use the correct path that LoadAggregationConfig expects (.zqk/config/config.yaml)
	configDir := filepath.Join(tmpDir, paths.ProjectDataDir, "config")
	if err := fileutil.MkdirAll(configDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")
	configYAML := `
audit:
  aggregation:
    enabled: true
    window_size: "2h"
    threshold: 20
    rules:
      - event_types: ["cache_invalidation", "cache_update"]
        severities: ["low"]
        group_by: ["event_type", "target_kind"]
        window: "2h"
        threshold: 20
        preserve_samples: 10
      - event_types: ["cache_bulk_invalidation"]
        severities: ["low", "medium"]
        group_by: ["event_type"]
        window: "1h"
        threshold: 5
        preserve_samples: 3
`

	if err := fileutil.WriteFile(configPath, []byte(configYAML), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	config, err := LoadAggregationConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadAggregationConfig() error = %v", err)
	}

	if !config.Enabled {
		t.Error("Expected config.Enabled = true")
	}
	if config.WindowSize != "2h" {
		t.Errorf("Expected config.WindowSize = '2h', got %v", config.WindowSize)
	}
	if config.Threshold != 20 {
		t.Errorf("Expected config.Threshold = 20, got %v", config.Threshold)
	}
	if len(config.Rules) != 2 {
		t.Errorf("Expected 2 rules, got %d", len(config.Rules))
	}

	// Check first rule
	rule1 := config.Rules[0]
	if len(rule1.EventTypes) != 2 {
		t.Errorf("Expected 2 event types in rule 1, got %d", len(rule1.EventTypes))
	}
	if rule1.Threshold != 20 {
		t.Errorf("Expected rule 1 threshold = 20, got %v", rule1.Threshold)
	}
	if rule1.PreserveSamples != 10 {
		t.Errorf("Expected rule 1 preserve_samples = 10, got %v", rule1.PreserveSamples)
	}
}

func TestLoadAggregationConfig_Disabled(t *testing.T) {
	tmpDir := t.TempDir()
	// Use the correct path that LoadAggregationConfig expects (.zqk/config/config.yaml)
	configDir := filepath.Join(tmpDir, paths.ProjectDataDir, "config")
	if err := fileutil.MkdirAll(configDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")
	configYAML := `
audit:
  aggregation:
    enabled: false
`

	if err := fileutil.WriteFile(configPath, []byte(configYAML), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	config, err := LoadAggregationConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadAggregationConfig() error = %v", err)
	}

	if config.Enabled {
		t.Error("Expected config.Enabled = false")
	}
}

func TestLoadAggregationConfig_InvalidWindowSize(t *testing.T) {
	tmpDir := t.TempDir()
	// Use the same path LoadAggregationConfig expects: .zqk/config/config.yaml
	configDir := filepath.Join(tmpDir, paths.ProjectDataDir, "config")
	if err := fileutil.MkdirAll(configDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")
	configYAML := `
audit:
  aggregation:
    enabled: true
    window_size: "invalid"
`

	if err := fileutil.WriteFile(configPath, []byte(configYAML), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	config, err := LoadAggregationConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadAggregationConfig() error = %v", err)
	}

	// Should fall back to default window size
	if config.WindowSize != "1h" {
		t.Errorf("Expected config.WindowSize = '1h' (default), got %v", config.WindowSize)
	}
}

func TestParseWindowSize(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"1h", time.Hour, false},
		{"24h", 24 * time.Hour, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"1w", 7 * 24 * time.Hour, false},
		{"2h", 2 * time.Hour, false},
		{"invalid", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWindowSize(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseWindowSize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("ParseWindowSize() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDefaultAggregationConfig(t *testing.T) {
	config := DefaultAggregationConfig()

	if !config.Enabled {
		t.Error("Expected config.Enabled = true")
	}
	if config.WindowSize != "1h" {
		t.Errorf("Expected config.WindowSize = '1h', got %v", config.WindowSize)
	}
	if config.Threshold != 10 {
		t.Errorf("Expected config.Threshold = 10, got %v", config.Threshold)
	}
	if len(config.Rules) == 0 {
		t.Error("Expected default rules to be present")
	}
}
