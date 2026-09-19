package storage

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestSyncValidationStrategy_ValidateMappings tests the sync validation strategy.
func TestSyncValidationStrategy_ValidateMappings(t *testing.T) {
	// Create temp directory for test files
	tempDir := t.TempDir()

	// Create some hash files
	existingHashes := []string{
		"abc123def456",
		"789xyz000111",
		"validhash001",
	}
	for _, hash := range existingHashes {
		hashFile := filepath.Join(tempDir, hash+".yaml")
		if err := fileutil.WriteFile(hashFile, []byte("test content"), paths.FilePerm644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	strategy := NewSyncValidationStrategy()

	tests := []struct {
		name             string
		mappings         map[string]string
		bucketKeys       map[string]string
		expectedValid    int
		expectedStale    int
		expectedValidIDs []string
	}{
		{
			name: "all valid mappings",
			mappings: map[string]string{
				"OBJ-001": "abc123def456",
				"OBJ-002": "789xyz000111",
			},
			bucketKeys:       nil,
			expectedValid:    2,
			expectedStale:    0,
			expectedValidIDs: []string{"OBJ-001", "OBJ-002"},
		},
		{
			name: "all stale mappings",
			mappings: map[string]string{
				"OBJ-001": "nonexistent1",
				"OBJ-002": "nonexistent2",
			},
			bucketKeys:       nil,
			expectedValid:    0,
			expectedStale:    2,
			expectedValidIDs: []string{},
		},
		{
			name: "mixed valid and stale",
			mappings: map[string]string{
				"OBJ-001": "abc123def456", // valid
				"OBJ-002": "nonexistent",  // stale
				"OBJ-003": "validhash001", // valid
			},
			bucketKeys:       nil,
			expectedValid:    2,
			expectedStale:    1,
			expectedValidIDs: []string{"OBJ-001", "OBJ-003"},
		},
		{
			name:             "empty mappings",
			mappings:         map[string]string{},
			bucketKeys:       nil,
			expectedValid:    0,
			expectedStale:    0,
			expectedValidIDs: []string{},
		},
		{
			name:             "nil mappings",
			mappings:         nil,
			bucketKeys:       nil,
			expectedValid:    0,
			expectedStale:    0,
			expectedValidIDs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validMappings, _, staleCount := strategy.ValidateMappings(tempDir, tt.mappings, tt.bucketKeys)

			if len(validMappings) != tt.expectedValid {
				t.Errorf("expected %d valid mappings, got %d", tt.expectedValid, len(validMappings))
			}

			if staleCount != tt.expectedStale {
				t.Errorf("expected %d stale entries, got %d", tt.expectedStale, staleCount)
			}

			// Verify expected IDs are present
			for _, id := range tt.expectedValidIDs {
				if _, ok := validMappings[id]; !ok {
					t.Errorf("expected ID %s to be in valid mappings", id)
				}
			}
		})
	}
}

// TestSyncValidationStrategy_WithBucketKeys tests validation with bucket keys.
func TestSyncValidationStrategy_WithBucketKeys(t *testing.T) {
	tempDir := t.TempDir()

	// Create bucket directory and files
	bucketDir := filepath.Join(tempDir, "2030-01")
	if err := fileutil.MkdirAll(bucketDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create bucket dir: %v", err)
	}

	// Create file in bucket
	bucketHash := "buckethash123"
	if err := fileutil.WriteFile(filepath.Join(bucketDir, bucketHash+".yaml"), []byte("test"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to create bucket file: %v", err)
	}

	// Create file in base dir
	baseHash := "basehash456"
	if err := fileutil.WriteFile(filepath.Join(tempDir, baseHash+".yaml"), []byte("test"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to create base file: %v", err)
	}

	strategy := NewSyncValidationStrategy()

	mappings := map[string]string{
		"OBJ-001": bucketHash, // in bucket
		"OBJ-002": baseHash,   // in base dir
		"OBJ-003": "stale",    // doesn't exist
	}
	bucketKeys := map[string]string{
		"OBJ-001": "2030-01",
		// OBJ-002 has no bucket key (base dir)
		// OBJ-003 has no bucket key
	}

	validMappings, validBucketKeys, staleCount := strategy.ValidateMappings(tempDir, mappings, bucketKeys)

	if len(validMappings) != 2 {
		t.Errorf("expected 2 valid mappings, got %d", len(validMappings))
	}

	if staleCount != 1 {
		t.Errorf("expected 1 stale entry, got %d", staleCount)
	}

	// Verify bucket key is preserved for bucket file
	if validBucketKeys["OBJ-001"] != "2030-01" {
		t.Errorf("expected bucket key '2030-01' for OBJ-001, got '%s'", validBucketKeys["OBJ-001"])
	}

	// Verify no bucket key for base dir file
	if _, ok := validBucketKeys["OBJ-002"]; ok {
		t.Errorf("expected no bucket key for OBJ-002")
	}
}

// TestSyncValidationStrategy_StartStop tests the no-op Start/Stop methods.
func TestSyncValidationStrategy_StartStop(t *testing.T) {
	strategy := NewSyncValidationStrategy()

	// Start should be no-op
	if err := strategy.Start("/some/path"); err != nil {
		t.Errorf("Start should not return error, got: %v", err)
	}

	// Stop should be no-op
	if err := strategy.Stop(); err != nil {
		t.Errorf("Stop should not return error, got: %v", err)
	}
}

// TestSyncValidationStrategy_Name tests the name method.
func TestSyncValidationStrategy_Name(t *testing.T) {
	strategy := NewSyncValidationStrategy()
	if strategy.Name() != "sync" {
		t.Errorf("expected name 'sync', got '%s'", strategy.Name())
	}
}

// TestValidationStrategyRegistry tests the strategy registry.
func TestValidationStrategyRegistry(t *testing.T) {
	// Create a fresh registry for testing (not using global)
	registry := &ValidationStrategyRegistry{
		strategies: make(map[string]ValidationStrategy),
		defaultStr: NewSyncValidationStrategy(),
	}

	// Test default strategy
	strategy := registry.GetStrategy("unknown_kind")
	if strategy.Name() != "sync" {
		t.Errorf("expected default sync strategy, got '%s'", strategy.Name())
	}

	// Register custom strategy for a kind
	customStrategy := &mockValidationStrategy{name: "custom"}
	registry.RegisterStrategy("audit_event", customStrategy)

	// Test registered strategy
	strategy = registry.GetStrategy("audit_event")
	if strategy.Name() != "custom" {
		t.Errorf("expected custom strategy, got '%s'", strategy.Name())
	}

	// Unregistered kind should still return default
	strategy = registry.GetStrategy("other_kind")
	if strategy.Name() != "sync" {
		t.Errorf("expected default sync strategy for unregistered kind, got '%s'", strategy.Name())
	}

	// Test SetDefaultStrategy
	newDefault := &mockValidationStrategy{name: "new_default"}
	registry.SetDefaultStrategy(newDefault)
	strategy = registry.GetStrategy("other_kind")
	if strategy.Name() != "new_default" {
		t.Errorf("expected new_default strategy, got '%s'", strategy.Name())
	}
}

// TestValidationMetrics tests the validation metrics tracking.
func TestValidationMetrics(t *testing.T) {
	metrics := &ValidationMetrics{}

	// Initial state
	if metrics.AverageValidationTimeNs() != 0 {
		t.Errorf("expected 0 average for no validations, got %d", metrics.AverageValidationTimeNs())
	}

	// Record some validations
	metrics.RecordValidation(1000, 10, 2)
	metrics.RecordValidation(2000, 20, 3)
	metrics.RecordValidation(3000, 30, 5)

	// Verify counts
	if metrics.TotalValidations.Load() != 3 {
		t.Errorf("expected 3 validations, got %d", metrics.TotalValidations.Load())
	}

	if metrics.TotalTimeNs.Load() != 6000 {
		t.Errorf("expected 6000ns total time, got %d", metrics.TotalTimeNs.Load())
	}

	if metrics.TotalEntriesScanned.Load() != 60 {
		t.Errorf("expected 60 entries scanned, got %d", metrics.TotalEntriesScanned.Load())
	}

	if metrics.TotalStaleRemoved.Load() != 10 {
		t.Errorf("expected 10 stale removed, got %d", metrics.TotalStaleRemoved.Load())
	}

	// Verify average
	if metrics.AverageValidationTimeNs() != 2000 {
		t.Errorf("expected average of 2000ns, got %d", metrics.AverageValidationTimeNs())
	}
}

// TestGetValidationMetrics tests the per-kind metrics retrieval.
func TestGetValidationMetrics(t *testing.T) {
	// Clear the global map for clean test
	validationMetricsMu.Lock()
	validationMetricsMap = make(map[string]*ValidationMetrics)
	validationMetricsMu.Unlock()

	// Get metrics for a new kind
	m1 := GetValidationMetrics("test_kind")
	if m1 == nil {
		t.Fatal("expected non-nil metrics")
	}

	// Record a validation
	m1.RecordValidation(1000, 10, 1)

	// Get metrics again - should be the same instance
	m2 := GetValidationMetrics("test_kind")
	if m2 != m1 {
		t.Error("expected same metrics instance")
	}

	// Verify the recorded data is there
	if m2.TotalValidations.Load() != 1 {
		t.Errorf("expected 1 validation, got %d", m2.TotalValidations.Load())
	}

	// Get metrics for different kind - should be different instance
	m3 := GetValidationMetrics("other_kind")
	if m3 == m1 {
		t.Error("expected different metrics instance for different kind")
	}
}

// TestValidationMetrics_Concurrent tests thread safety of metrics.
func TestValidationMetrics_Concurrent(t *testing.T) {
	metrics := &ValidationMetrics{}

	var wg sync.WaitGroup
	numGoroutines := 100
	recordsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("storage_test", "concurrent validation metrics record").StartSimple(func() {
			defer wg.Done()
			for j := 0; j < recordsPerGoroutine; j++ {
				metrics.RecordValidation(1000, 10, 1)
			}
		})
	}

	waitDone := make(chan struct{})
	go func() { defer close(waitDone); wg.Wait() }()
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		panic("timeout")
	}

	expectedValidations := int64(numGoroutines * recordsPerGoroutine)
	if metrics.TotalValidations.Load() != expectedValidations {
		t.Errorf("expected %d validations, got %d", expectedValidations, metrics.TotalValidations.Load())
	}
}

// TestBuildHashFilePath tests the helper function.
func TestBuildHashFilePath(t *testing.T) {
	proj := "/project"
	auditUnderProj := filepath.Join(proj, paths.ProcessAuditDir)
	metricsUnderProj := datacell.CellCASPrimaryDir(proj, "metrics")
	tests := []struct {
		kindDir   string
		hash      string
		bucketKey string
		expected  string
	}{
		{
			kindDir:   auditUnderProj,
			hash:      "abc123",
			bucketKey: "",
			expected:  filepath.Join(auditUnderProj, "abc123.yaml"),
		},
		{
			kindDir:   auditUnderProj,
			hash:      "abc123",
			bucketKey: "2030-01",
			expected:  filepath.Join(auditUnderProj, "2030-01", "abc123.yaml"),
		},
		{
			kindDir:   metricsUnderProj,
			hash:      "xyz789",
			bucketKey: "active",
			expected:  filepath.Join(metricsUnderProj, "active", "xyz789.yaml"),
		},
	}

	for _, tt := range tests {
		result := buildHashFilePath(tt.kindDir, tt.hash, tt.bucketKey)
		if result != tt.expected {
			t.Errorf("buildHashFilePath(%q, %q, %q) = %q, want %q",
				tt.kindDir, tt.hash, tt.bucketKey, result, tt.expected)
		}
	}
}

// mockValidationStrategy is a mock for testing the registry.
type mockValidationStrategy struct {
	name string
}

func (m *mockValidationStrategy) ValidateMappings(kindDir string, mappings map[string]string, bucketKeys map[string]string) (map[string]string, map[string]string, int) {
	return mappings, bucketKeys, 0
}

func (m *mockValidationStrategy) Start(_ string) error { return nil }
func (m *mockValidationStrategy) Stop() error          { return nil }
func (m *mockValidationStrategy) Name() string         { return m.name }

// =============================================================================
// AsyncCacheValidationStrategy Tests
// =============================================================================

// TestAsyncCacheValidationStrategy_ValidateMappings tests the async cache strategy validation.
func TestAsyncCacheValidationStrategy_ValidateMappings(t *testing.T) {
	tempDir := t.TempDir()

	// Create some hash files
	existingHashes := []string{
		"async_hash_001",
		"async_hash_002",
		"async_hash_003",
	}
	for _, hash := range existingHashes {
		hashFile := filepath.Join(tempDir, hash+".yaml")
		if err := fileutil.WriteFile(hashFile, []byte("test content"), paths.FilePerm644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	strategy := NewAsyncCacheValidationStrategy(nil)
	defer func() { _ = strategy.Stop() }()

	// Start the strategy (triggers initial scan)
	if err := strategy.Start(tempDir); err != nil {
		t.Fatalf("failed to start strategy: %v", err)
	}

	tests := []struct {
		name          string
		mappings      map[string]string
		expectedValid int
		expectedStale int
	}{
		{
			name: "all valid mappings",
			mappings: map[string]string{
				"OBJ-001": "async_hash_001",
				"OBJ-002": "async_hash_002",
			},
			expectedValid: 2,
			expectedStale: 0,
		},
		{
			name: "mixed valid and stale",
			mappings: map[string]string{
				"OBJ-001": "async_hash_001",
				"OBJ-002": "nonexistent",
				"OBJ-003": "async_hash_003",
			},
			expectedValid: 2,
			expectedStale: 1,
		},
		{
			name: "all stale mappings",
			mappings: map[string]string{
				"OBJ-001": "stale1",
				"OBJ-002": "stale2",
			},
			expectedValid: 0,
			expectedStale: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validMappings, _, staleCount := strategy.ValidateMappings(tempDir, tt.mappings, nil)

			if len(validMappings) != tt.expectedValid {
				t.Errorf("expected %d valid mappings, got %d", tt.expectedValid, len(validMappings))
			}

			if staleCount != tt.expectedStale {
				t.Errorf("expected %d stale entries, got %d", tt.expectedStale, staleCount)
			}
		})
	}
}

// TestAsyncCacheValidationStrategy_CacheMissFallsBackToStat ensures a hash written after the
// last scan is not dropped as stale (CLI parallel create ghost-success bug).
func TestAsyncCacheValidationStrategy_CacheMissFallsBackToStat(t *testing.T) {
	tempDir := t.TempDir()
	seedHash := "async_seed_hash"
	if err := fileutil.WriteFile(filepath.Join(tempDir, seedHash+".yaml"), []byte("seed"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	strategy := NewAsyncCacheValidationStrategy(nil)
	defer func() { _ = strategy.Stop() }()
	if err := strategy.Start(tempDir); err != nil {
		t.Fatalf("start: %v", err)
	}

	// New file after initial scan — not in cache yet.
	freshHash := "async_fresh_after_scan"
	if err := fileutil.WriteFile(filepath.Join(tempDir, freshHash+".yaml"), []byte("fresh"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	mappings := map[string]string{
		"OBJ-SEED":  seedHash,
		"OBJ-FRESH": freshHash,
		"OBJ-GONE":  "definitely_missing_hash",
	}
	valid, _, stale := strategy.ValidateMappings(tempDir, mappings, nil)
	if len(valid) != 2 {
		t.Fatalf("expected 2 valid (seed+fresh via Stat), got %d (%v) stale=%d", len(valid), valid, stale)
	}
	if _, ok := valid["OBJ-FRESH"]; !ok {
		t.Fatal("expected OBJ-FRESH kept after cache-miss Stat fallback")
	}
	if stale != 1 {
		t.Fatalf("expected 1 stale (missing hash), got %d", stale)
	}
}

// TestAsyncCacheValidationStrategy_WithBucketKeys tests async validation with bucket keys.
func TestAsyncCacheValidationStrategy_WithBucketKeys(t *testing.T) {
	tempDir := t.TempDir()

	// Create bucket directory and files
	bucketDir := filepath.Join(tempDir, "2030-02")
	if err := fileutil.MkdirAll(bucketDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create bucket dir: %v", err)
	}

	bucketHash := "bucket_async_hash"
	if err := fileutil.WriteFile(filepath.Join(bucketDir, bucketHash+".yaml"), []byte("test"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to create bucket file: %v", err)
	}

	baseHash := "base_async_hash"
	if err := fileutil.WriteFile(filepath.Join(tempDir, baseHash+".yaml"), []byte("test"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to create base file: %v", err)
	}

	strategy := NewAsyncCacheValidationStrategy(nil)
	defer func() { _ = strategy.Stop() }()

	if err := strategy.Start(tempDir); err != nil {
		t.Fatalf("failed to start strategy: %v", err)
	}

	mappings := map[string]string{
		"OBJ-001": bucketHash,
		"OBJ-002": baseHash,
		"OBJ-003": "stale",
	}
	bucketKeys := map[string]string{
		"OBJ-001": "2030-02",
	}

	validMappings, validBucketKeys, staleCount := strategy.ValidateMappings(tempDir, mappings, bucketKeys)

	if len(validMappings) != 2 {
		t.Errorf("expected 2 valid mappings, got %d", len(validMappings))
	}

	if staleCount != 1 {
		t.Errorf("expected 1 stale entry, got %d", staleCount)
	}

	if validBucketKeys["OBJ-001"] != "2030-02" {
		t.Errorf("expected bucket key '2030-02' for OBJ-001, got '%s'", validBucketKeys["OBJ-001"])
	}
}

// TestAsyncCacheValidationStrategy_FallbackToSync tests fallback behavior.
func TestAsyncCacheValidationStrategy_FallbackToSync(t *testing.T) {
	tempDir := t.TempDir()

	// Create a file for testing
	hash := "fallback_test_hash"
	if err := fileutil.WriteFile(filepath.Join(tempDir, hash+".yaml"), []byte("test"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create strategy but don't start it (cache will be empty)
	strategy := NewAsyncCacheValidationStrategy(nil)

	mappings := map[string]string{
		"OBJ-001": hash,
		"OBJ-002": "nonexistent",
	}

	// Should fallback to sync validation since cache is empty
	validMappings, _, staleCount := strategy.ValidateMappings(tempDir, mappings, nil)

	if len(validMappings) != 1 {
		t.Errorf("expected 1 valid mapping from sync fallback, got %d", len(validMappings))
	}

	if staleCount != 1 {
		t.Errorf("expected 1 stale entry from sync fallback, got %d", staleCount)
	}

	// Verify fallback counter incremented
	hits, _, fallbacks, _, _, _ := strategy.GetCacheStats()
	if fallbacks != 1 {
		t.Errorf("expected 1 fallback, got %d", fallbacks)
	}
	if hits != 0 {
		t.Errorf("expected 0 cache hits (fallback used sync), got %d", hits)
	}
}

// TestAsyncCacheValidationStrategy_StartStop tests lifecycle management.
func TestAsyncCacheValidationStrategy_StartStop(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files
	if err := fileutil.WriteFile(filepath.Join(tempDir, "test_hash.yaml"), []byte("test"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	strategy := NewAsyncCacheValidationStrategy(&AsyncCacheConfig{
		ScanInterval: 50 * time.Millisecond,
	})

	// Start should work
	if err := strategy.Start(tempDir); err != nil {
		t.Errorf("Start returned error: %v", err)
	}

	// Double start should be no-op
	if err := strategy.Start(tempDir); err != nil {
		t.Errorf("Double Start returned error: %v", err)
	}

	// Verify scanner is running
	if !strategy.scannerRunning.Load() {
		t.Error("expected scanner to be running after Start")
	}

	// Stop should work
	if err := strategy.Stop(); err != nil {
		t.Errorf("Stop returned error: %v", err)
	}

	// Verify scanner stopped
	if strategy.scannerRunning.Load() {
		t.Error("expected scanner to be stopped after Stop")
	}

	// Double stop should be no-op
	if err := strategy.Stop(); err != nil {
		t.Errorf("Double Stop returned error: %v", err)
	}
}

// TestAsyncCacheValidationStrategy_Name tests the name method.
func TestAsyncCacheValidationStrategy_Name(t *testing.T) {
	strategy := NewAsyncCacheValidationStrategy(nil)
	if strategy.Name() != "async-cache" {
		t.Errorf("expected name 'async-cache', got '%s'", strategy.Name())
	}
}

// TestAsyncCacheValidationStrategy_CacheStats tests cache statistics.
func TestAsyncCacheValidationStrategy_CacheStats(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files
	for i := 0; i < 5; i++ {
		hash := filepath.Join(tempDir, "hash_"+string(rune('a'+i))+".yaml")
		if err := fileutil.WriteFile(hash, []byte("test"), paths.FilePerm644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	strategy := NewAsyncCacheValidationStrategy(nil)
	defer func() { _ = strategy.Stop() }()

	if err := strategy.Start(tempDir); err != nil {
		t.Fatalf("failed to start strategy: %v", err)
	}

	// Do some validations
	mappings := map[string]string{
		"OBJ-1": "hash_a", // exists
		"OBJ-2": "hash_b", // exists
		"OBJ-3": "hash_x", // doesn't exist
	}

	_, _, _ = strategy.ValidateMappings(tempDir, mappings, nil)

	hits, misses, fallbacks, scans, swaps, _ := strategy.GetCacheStats()

	if hits != 2 {
		t.Errorf("expected 2 cache hits, got %d", hits)
	}

	if misses != 1 {
		t.Errorf("expected 1 cache miss, got %d", misses)
	}

	if fallbacks != 0 {
		t.Errorf("expected 0 fallbacks (cache was populated), got %d", fallbacks)
	}

	if scans < 1 {
		t.Errorf("expected at least 1 scan, got %d", scans)
	}

	if swaps < 1 {
		t.Errorf("expected at least 1 swap, got %d", swaps)
	}
}

// TestAsyncCacheValidationStrategy_DirectoryChange tests mtime-based change detection.
func TestAsyncCacheValidationStrategy_DirectoryChange(t *testing.T) {
	tempDir := t.TempDir()

	// Create initial file
	if err := fileutil.WriteFile(filepath.Join(tempDir, "initial_hash.yaml"), []byte("test"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	strategy := NewAsyncCacheValidationStrategy(&AsyncCacheConfig{
		ScanInterval: 50 * time.Millisecond, // Fast interval for testing
	})
	defer func() { _ = strategy.Stop() }()

	if err := strategy.Start(tempDir); err != nil {
		t.Fatalf("failed to start strategy: %v", err)
	}

	// Validate initial file exists
	mappings := map[string]string{"OBJ-1": "initial_hash"}
	validMappings, _, _ := strategy.ValidateMappings(tempDir, mappings, nil)
	if len(validMappings) != 1 {
		t.Errorf("expected 1 valid mapping, got %d", len(validMappings))
	}

	// Add new file
	newHash := "new_hash_after_start"
	if err := fileutil.WriteFile(filepath.Join(tempDir, newHash+".yaml"), []byte("test"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to create new test file: %v", err)
	}

	// Wait for scanner to detect change and rescan
	time.Sleep(200 * time.Millisecond)

	// Validate new file is now in cache
	mappings = map[string]string{"OBJ-2": newHash}
	validMappings, _, _ = strategy.ValidateMappings(tempDir, mappings, nil)
	if len(validMappings) != 1 {
		t.Errorf("expected new file to be in cache after rescan, got %d valid", len(validMappings))
	}
}

// TestAsyncCacheValidationStrategy_Concurrent tests thread safety.
func TestAsyncCacheValidationStrategy_Concurrent(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files
	for i := 0; i < 10; i++ {
		hash := filepath.Join(tempDir, "concurrent_hash_"+string(rune('0'+i))+".yaml")
		if err := fileutil.WriteFile(hash, []byte("test"), paths.FilePerm644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	strategy := NewAsyncCacheValidationStrategy(&AsyncCacheConfig{
		ScanInterval: 10 * time.Millisecond,
	})
	defer func() { _ = strategy.Stop() }()

	if err := strategy.Start(tempDir); err != nil {
		t.Fatalf("failed to start strategy: %v", err)
	}

	// Run many concurrent validations
	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("storage_test", "concurrent validation strategy test").StartSimple(func() {
			func(idx int) {
				defer wg.Done()

				for j := 0; j < 20; j++ {
					mappings := map[string]string{
						"OBJ-A": "concurrent_hash_" + string(rune('0'+j%10)),
						"OBJ-B": "nonexistent_" + string(rune('0'+j)),
					}
					_, _, _ = strategy.ValidateMappings(tempDir, mappings, nil)
				}
			}(i)
		})
	}

	waitDone := make(chan struct{})
	go func() { defer close(waitDone); wg.Wait() }()
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		panic("timeout")
	}

	// Just verify no panics occurred and stats are reasonable
	hits, misses, _, _, _, _ := strategy.GetCacheStats()
	if hits == 0 && misses == 0 {
		t.Error("expected some cache activity after concurrent validations")
	}
}

// TestAsyncCacheValidationStrategy_EmptyMappings tests empty input handling.
func TestAsyncCacheValidationStrategy_EmptyMappings(t *testing.T) {
	strategy := NewAsyncCacheValidationStrategy(nil)

	// Empty mappings should return immediately
	validMappings, validBucketKeys, staleCount := strategy.ValidateMappings("/any/path", nil, nil)

	if validMappings != nil {
		t.Errorf("expected nil valid mappings for nil input, got %v", validMappings)
	}
	if validBucketKeys != nil {
		t.Errorf("expected nil valid bucket keys for nil input, got %v", validBucketKeys)
	}
	if staleCount != 0 {
		t.Errorf("expected 0 stale count for nil input, got %d", staleCount)
	}

	// Empty map should also work
	emptyMap := map[string]string{}
	validMappings, _, emptyStaleCount := strategy.ValidateMappings("/any/path", emptyMap, nil)
	if len(validMappings) != 0 {
		t.Errorf("expected empty valid mappings for empty input, got %d", len(validMappings))
	}
	if emptyStaleCount != 0 {
		t.Errorf("expected 0 stale count for empty input, got %d", emptyStaleCount)
	}
}
