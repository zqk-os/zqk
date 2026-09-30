package validation

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestValidationMetrics_BasicOperations tests basic metrics operations
func TestValidationMetrics_BasicOperations(t *testing.T) {
	t.Parallel()
	metrics := NewValidationMetrics()

	// Test initial state
	if metrics.StartTime.IsZero() {
		t.Error("NewValidationMetrics() StartTime should be set")
	}

	// Test RecordEnqueue
	metrics.RecordEnqueue(100 * time.Millisecond)
	if metrics.EnqueueDuration != "100ms" {
		t.Errorf("RecordEnqueue() expected '100ms', got %s", metrics.EnqueueDuration)
	}

	// Test RecordValidation
	metrics.RecordValidation(200 * time.Millisecond)
	if metrics.ValidationDuration != "200ms" {
		t.Errorf("RecordValidation() expected '200ms', got %s", metrics.ValidationDuration)
	}

	// Test RecordCollection
	metrics.RecordCollection(50 * time.Millisecond)
	if metrics.CollectionDuration != "50ms" {
		t.Errorf("RecordCollection() expected '50ms', got %s", metrics.CollectionDuration)
	}
}

// TestValidationMetrics_Counters tests counter increment functions
func TestValidationMetrics_Counters(t *testing.T) {
	t.Parallel()
	metrics := NewValidationMetrics()

	// Test IncrementValidated
	metrics.IncrementValidated()
	if metrics.ValidatedObjects != 1 {
		t.Errorf("IncrementValidated() expected 1, got %d", metrics.ValidatedObjects)
	}

	// Test IncrementFailed
	metrics.IncrementFailed()
	if metrics.FailedObjects != 1 {
		t.Errorf("IncrementFailed() expected 1, got %d", metrics.FailedObjects)
	}

	// Test IncrementCacheHit
	metrics.IncrementCacheHit()
	if metrics.CacheHits != 1 {
		t.Errorf("IncrementCacheHit() expected 1, got %d", metrics.CacheHits)
	}

	// Test IncrementCacheMiss
	metrics.IncrementCacheMiss()
	if metrics.CacheMisses != 1 {
		t.Errorf("IncrementCacheMiss() expected 1, got %d", metrics.CacheMisses)
	}

	// Test IncrementRetry
	metrics.IncrementRetry()
	if metrics.Retries != 1 {
		t.Errorf("IncrementRetry() expected 1, got %d", metrics.Retries)
	}
}

// TestValidationMetrics_Setters tests setter functions
func TestValidationMetrics_Setters(t *testing.T) {
	t.Parallel()
	metrics := NewValidationMetrics()

	// Test SetTotalObjects
	metrics.SetTotalObjects(100)
	if metrics.TotalObjects != 100 {
		t.Errorf("SetTotalObjects() expected 100, got %d", metrics.TotalObjects)
	}

	// Test SetWorkerCount
	metrics.SetWorkerCount(4)
	if metrics.WorkerCount != 4 {
		t.Errorf("SetWorkerCount() expected 4, got %d", metrics.WorkerCount)
	}
}

// TestValidationMetrics_UpdateQueueSize tests UpdateQueueSize function
func TestValidationMetrics_UpdateQueueSize(t *testing.T) {
	t.Parallel()
	metrics := NewValidationMetrics()

	// Test initial queue size
	metrics.UpdateQueueSize(10)
	if metrics.MaxQueueSize != 10 {
		t.Errorf("UpdateQueueSize() MaxQueueSize expected 10, got %d", metrics.MaxQueueSize)
	}
	if metrics.AverageQueueSize != 10.0 {
		t.Errorf("UpdateQueueSize() AverageQueueSize expected 10.0, got %f", metrics.AverageQueueSize)
	}

	// Test updating with larger size
	metrics.UpdateQueueSize(20)
	if metrics.MaxQueueSize != 20 {
		t.Errorf("UpdateQueueSize() MaxQueueSize expected 20, got %d", metrics.MaxQueueSize)
	}

	// Test updating with smaller size (should not change max)
	metrics.UpdateQueueSize(15)
	if metrics.MaxQueueSize != 20 {
		t.Errorf("UpdateQueueSize() MaxQueueSize should remain 20, got %d", metrics.MaxQueueSize)
	}
}

// TestValidationMetrics_RecordTierIssue tests RecordTierIssue function
func TestValidationMetrics_RecordTierIssue(t *testing.T) {
	t.Parallel()
	metrics := NewValidationMetrics()

	// Test tier 1
	metrics.RecordTierIssue(1)
	if metrics.Tier1Count != 1 {
		t.Errorf("RecordTierIssue(1) expected Tier1Count 1, got %d", metrics.Tier1Count)
	}

	// Test tier 2
	metrics.RecordTierIssue(2)
	if metrics.Tier2Count != 1 {
		t.Errorf("RecordTierIssue(2) expected Tier2Count 1, got %d", metrics.Tier2Count)
	}

	// Test tier 3
	metrics.RecordTierIssue(3)
	if metrics.Tier3Count != 1 {
		t.Errorf("RecordTierIssue(3) expected Tier3Count 1, got %d", metrics.Tier3Count)
	}

	// Test tier 4
	metrics.RecordTierIssue(4)
	if metrics.Tier4Count != 1 {
		t.Errorf("RecordTierIssue(4) expected Tier4Count 1, got %d", metrics.Tier4Count)
	}

	// Test invalid tier (should not increment any counter)
	initialTier1 := metrics.Tier1Count
	metrics.RecordTierIssue(5)
	if metrics.Tier1Count != initialTier1 {
		t.Error("RecordTierIssue(5) should not increment any tier counter")
	}
}

// TestValidationMetrics_Finalize tests Finalize function
func TestValidationMetrics_Finalize(t *testing.T) {
	t.Parallel()
	metrics := NewValidationMetrics()

	// Set up some metrics
	metrics.RecordValidation(100 * time.Millisecond)
	metrics.IncrementValidated()
	metrics.IncrementValidated()
	metrics.IncrementValidated()

	// Wait a bit to ensure duration
	time.Sleep(10 * time.Millisecond)

	// Finalize
	metrics.Finalize()

	// Check that EndTime is set
	if metrics.EndTime.IsZero() {
		t.Error("Finalize() EndTime should be set")
	}

	// Check that TotalDuration is set
	if metrics.TotalDuration == emptyValue {
		t.Error("Finalize() TotalDuration should be set")
	}

	// Check that ObjectsPerSecond is calculated
	if metrics.ObjectsPerSecond <= 0 {
		t.Error("Finalize() ObjectsPerSecond should be calculated")
	}

	// Check that AverageValidationTime is calculated
	if metrics.AverageValidationTime == emptyValue {
		t.Error("Finalize() AverageValidationTime should be calculated")
	}
}

// TestValidationMetrics_Save tests Save function
func TestValidationMetrics_Save(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	metricsFile := filepath.Join(tmpDir, "metrics.json")

	metrics := NewValidationMetrics()
	metrics.IncrementValidated()
	metrics.IncrementFailed()
	metrics.RecordTierIssue(1)

	// Finalize before saving to avoid deadlock (Save calls Finalize which needs write lock)
	metrics.Finalize()

	// Save metrics
	err := metrics.Save(metricsFile)
	if err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}

	// Verify file exists
	if _, err := fileutil.Stat(metricsFile); fileutil.IsNotExist(err) {
		t.Error("Save() metrics file should exist")
	}

	// Verify file is readable JSON
	data, err := fileutil.ReadFile(metricsFile)
	if err != nil {
		t.Fatalf("failed to read metrics file: %v", err)
	}
	if len(data) == 0 {
		t.Error("Save() metrics file should not be empty")
	}
}

// TestValidationMetrics_String tests String function
func TestValidationMetrics_String(t *testing.T) {
	t.Parallel()
	metrics := NewValidationMetrics()
	metrics.RecordEnqueue(100 * time.Millisecond)
	metrics.RecordValidation(200 * time.Millisecond)
	metrics.IncrementValidated()
	metrics.IncrementFailed()
	metrics.IncrementCacheHit()
	metrics.IncrementCacheMiss()
	metrics.SetWorkerCount(2)
	metrics.RecordTierIssue(1)
	metrics.RecordTierIssue(2)

	metrics.Finalize()

	str := metrics.String()
	if str == emptyValue {
		t.Error("String() should return non-empty string")
	}

	// Verify key metrics are in the string
	if len(str) < 50 {
		t.Error("String() should return detailed metrics string")
	}
}

// TestValidationMetrics_StringThroughputWithoutFinalize covers the check
// summary path that used to print "0.00 objects/sec" because String()
// trusted ObjectsPerSecond before Finalize() had run.
func TestValidationMetrics_StringThroughputWithoutFinalize(t *testing.T) {
	t.Parallel()
	metrics := NewValidationMetrics()
	metrics.StartTime = time.Now().Add(-2 * time.Second)
	for i := 0; i < 100; i++ {
		metrics.IncrementValidated()
	}
	str := metrics.String()
	if strings.Contains(str, "Performance: 0.00 objects/sec") {
		t.Fatalf("String() without Finalize still reports zero throughput:\n%s", str)
	}
	if strings.Contains(str, "Cache:") {
		t.Fatalf("zero-hit cache line must be omitted from the human summary:\n%s", str)
	}
}

func TestValidationMetrics_StringOmitsZeroHitCache(t *testing.T) {
	t.Parallel()
	metrics := NewValidationMetrics()
	metrics.IncrementValidated()
	for i := 0; i < 3; i++ {
		metrics.IncrementCacheMiss()
	}
	str := metrics.String()
	if strings.Contains(str, "Cache:") || strings.Contains(str, "hit rate") {
		t.Fatalf("all-miss cache line must be omitted:\n%s", str)
	}
}

func TestValidationMetrics_StringIncludesCacheWhenHits(t *testing.T) {
	t.Parallel()
	metrics := NewValidationMetrics()
	metrics.IncrementCacheHit()
	metrics.IncrementCacheMiss()
	str := metrics.String()
	if !strings.Contains(str, "Cache: 1 hits, 1 misses (50.0% hit rate)") {
		t.Fatalf("want cache line when hits>0, got:\n%s", str)
	}
}

// TestValidationMetrics_CacheHitRate tests cacheHitRate function
func TestValidationMetrics_CacheHitRate(t *testing.T) {
	t.Parallel()
	metrics := NewValidationMetrics()

	// Test with no hits or misses
	rate := metrics.cacheHitRate()
	if rate != 0 {
		t.Errorf("cacheHitRate() with no hits/misses expected 0, got %f", rate)
	}

	// Test with hits only
	metrics.IncrementCacheHit()
	metrics.IncrementCacheHit()
	rate = metrics.cacheHitRate()
	if rate != 100.0 {
		t.Errorf("cacheHitRate() with only hits expected 100.0, got %f", rate)
	}

	// Test with hits and misses
	metrics.IncrementCacheMiss()
	rate = metrics.cacheHitRate()
	expected := 66.66666666666666 // 2 hits / 3 total * 100
	if rate < expected-0.1 || rate > expected+0.1 {
		t.Errorf("cacheHitRate() expected ~%.1f, got %f", expected, rate)
	}
}
