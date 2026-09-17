package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/paths"
)

// TestInitializeAsyncValidationStrategies tests the initialization of async strategies.
func TestInitializeAsyncValidationStrategies(t *testing.T) {
	// Reset global state for clean test
	resetAsyncValidationState()

	tempDir := t.TempDir()

	// Create directories for high-volume kinds
	for _, kind := range HighVolumeKinds {
		kindDir := datacell.CellCASPrimaryDir(tempDir, kindDirName(kind))
		if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create kind directory for %s: %v", kind, err)
		}
	}

	// Initialize strategies
	count := InitializeAsyncValidationStrategies(tempDir)

	if count != len(HighVolumeKinds) {
		t.Errorf("expected %d strategies initialized, got %d", len(HighVolumeKinds), count)
	}

	// Verify strategies are registered
	registry := GetGlobalValidationStrategyRegistry()
	for _, kind := range HighVolumeKinds {
		strategy := registry.GetStrategy(kind)
		if strategy.Name() != "async-cache" {
			t.Errorf("expected async-cache strategy for %s, got %s", kind, strategy.Name())
		}
	}

	// Verify IsAsyncValidationEnabled returns true
	if !IsAsyncValidationEnabled() {
		t.Error("expected async validation to be enabled")
	}

	// Verify GetAsyncValidationKinds returns all kinds
	kinds := GetAsyncValidationKinds()
	if len(kinds) != len(HighVolumeKinds) {
		t.Errorf("expected %d kinds, got %d", len(HighVolumeKinds), len(kinds))
	}

	// Shutdown
	ShutdownAsyncValidationStrategies()

	// Verify IsAsyncValidationEnabled returns false after shutdown
	if IsAsyncValidationEnabled() {
		t.Error("expected async validation to be disabled after shutdown")
	}
}

// TestInitializeAsyncValidationStrategies_Idempotent tests that initialization is idempotent.
func TestInitializeAsyncValidationStrategies_Idempotent(t *testing.T) {
	// Reset global state for clean test
	resetAsyncValidationState()

	tempDir := t.TempDir()

	// Create directories for high-volume kinds
	for _, kind := range HighVolumeKinds {
		kindDir := datacell.CellCASPrimaryDir(tempDir, kindDirName(kind))
		if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create kind directory for %s: %v", kind, err)
		}
	}

	// Initialize twice with same project root
	count1 := InitializeAsyncValidationStrategies(tempDir)
	count2 := InitializeAsyncValidationStrategies(tempDir)

	if count1 == 0 {
		t.Error("expected non-zero count on first initialization")
	}

	if count2 != 0 {
		t.Errorf("expected 0 count on second initialization (idempotent), got %d", count2)
	}

	// Cleanup
	ShutdownAsyncValidationStrategies()
}

// TestAsyncStrategyStats tests that stats are collected correctly.
func TestAsyncStrategyStats(t *testing.T) {
	// Reset global state for clean test
	resetAsyncValidationState()

	tempDir := t.TempDir()

	// Create directories and some test files for audit
	auditDir := datacell.CellCASPrimaryDir(tempDir, "audit")
	if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create audit directory: %v", err)
	}

	// Create a test file
	testHash := "test_hash_for_stats"
	if err := fileutil.WriteFile(filepath.Join(auditDir, testHash+".yaml"), []byte("test"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create other kind directories
	for _, kind := range HighVolumeKinds {
		if kind == "audit_event" {
			continue // Already created
		}
		kindDir := datacell.CellCASPrimaryDir(tempDir, kindDirName(kind))
		if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create kind directory for %s: %v", kind, err)
		}
	}

	// Initialize
	InitializeAsyncValidationStrategies(tempDir)

	// Wait for initial scan to complete
	time.Sleep(100 * time.Millisecond)

	// Do a validation
	registry := GetGlobalValidationStrategyRegistry()
	strategy := registry.GetStrategy("audit_event")

	mappings := map[string]string{
		"OBJ-1": testHash,      // exists
		"OBJ-2": "nonexistent", // doesn't exist
	}

	validMappings, _, staleCount := strategy.ValidateMappings(auditDir, mappings, nil)

	if len(validMappings) != 1 {
		t.Errorf("expected 1 valid mapping, got %d", len(validMappings))
	}

	if staleCount != 1 {
		t.Errorf("expected 1 stale entry, got %d", staleCount)
	}

	// Check stats
	stats := GetAsyncStrategyStats()
	if _, ok := stats["audit_event"]; !ok {
		t.Error("expected stats for audit_event")
	}

	auditStats := stats["audit_event"]
	if auditStats.CacheHits+auditStats.CacheMisses == 0 && auditStats.FallbacksToSync == 0 {
		t.Error("expected some cache activity")
	}

	// Cleanup
	ShutdownAsyncValidationStrategies()
}

// TestKindDirName tests the kind to directory name mapping.
func TestKindDirName(t *testing.T) {
	tests := []struct {
		kind     string
		expected string
	}{
		{"audit_event", "audit"},
		{"mcp_session", objects.GetDirectoryFromKind(objects.KindMcpSession)},
		{"doc_entry", objects.GetDirectoryFromKind(objects.KindDocEntry)},
		{"backlog_item", objects.GetDirectoryFromKind(objects.KindBacklogItem)},
	}

	for _, tt := range tests {
		result := kindDirName(tt.kind)
		if result != tt.expected {
			t.Errorf("kindDirName(%q) = %q, want %q", tt.kind, result, tt.expected)
		}
		if tt.expected == "" {
			t.Errorf("GetDirectoryFromKind(%q) empty; kindDirName must stay aligned with CAS dirs", tt.kind)
		}
	}
}

// TestShutdownHandler tests the shutdown handler implementation.
func TestShutdownHandler(t *testing.T) {
	handler := getAsyncValidationShutdownHandler()

	// Test GetName
	if handler.GetName() != "async_validation_strategies" {
		t.Errorf("expected name 'async_validation_strategies', got '%s'", handler.GetName())
	}

	// Test GetPendingCount
	if handler.GetPendingCount() != 0 {
		t.Errorf("expected pending count 0, got %d", handler.GetPendingCount())
	}

	// Test IsDrained
	if !handler.IsDrained() {
		t.Error("expected IsDrained to return true")
	}

	// Test IsCritical
	if handler.IsCritical() {
		t.Error("expected IsCritical to return false")
	}
}

// resetAsyncValidationState resets the global async validation state for testing.
func resetAsyncValidationState() {
	ShutdownAsyncValidationStrategies()
	// Also reset the registry's strategies for clean test
	registry := GetGlobalValidationStrategyRegistry()
	for _, kind := range HighVolumeKinds {
		registry.RegisterStrategy(kind, NewSyncValidationStrategy())
	}
}
