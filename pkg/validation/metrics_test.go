package validation

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestValidationMetrics_BasicOperations tests basic metrics operations
func TestValidationMetrics_BasicOperations(t *testing.T) {
	t.Parallel()
	metrics := NewValidationMetrics()

	// Test initial state
	if metrics.StartTime.IsZero() {
		t.Error(ConstMagic61fed366)
	}

	// Test RecordEnqueue
	metrics.RecordEnqueue(100 * time.Millisecond)
	if metrics.EnqueueDuration != "100ms" {
		t.Errorf(ConstMagice6c00599, metrics.EnqueueDuration)
	}

	// Test RecordValidation
	metrics.RecordValidation(200 * time.Millisecond)
	if metrics.ValidationDuration != "200ms" {
		t.Errorf(ConstMagic8e0c9e2b, metrics.ValidationDuration)
	}

	// Test RecordCollection
	metrics.RecordCollection(50 * time.Millisecond)
	if metrics.CollectionDuration != "50ms" {
		t.Errorf(ConstMagic5a01883c, metrics.CollectionDuration)
	}
}

// TestValidationMetrics_Counters tests counter increment functions
func TestValidationMetrics_Counters(t *testing.T) {
	t.Parallel()
	metrics := NewValidationMetrics()

	// Test IncrementValidated
	metrics.IncrementValidated()
	if metrics.ValidatedObjects != 1 {
		t.Errorf(ConstMagic11d14318, metrics.ValidatedObjects)
	}

	// Test IncrementFailed
	metrics.IncrementFailed()
	if metrics.FailedObjects != 1 {
		t.Errorf(ConstMagic3135ddf9, metrics.FailedObjects)
	}

	// Test IncrementCacheHit
	metrics.IncrementCacheHit()
	if metrics.CacheHits != 1 {
		t.Errorf(ConstMagic9161c1b0, metrics.CacheHits)
	}

	// Test IncrementCacheMiss
	metrics.IncrementCacheMiss()
	if metrics.CacheMisses != 1 {
		t.Errorf(ConstMagic25689a02, metrics.CacheMisses)
	}

	// Test IncrementRetry
	metrics.IncrementRetry()
	if metrics.Retries != 1 {
		t.Errorf(ConstMagic6de8f510, metrics.Retries)
	}
}

// TestValidationMetrics_Setters tests setter functions
func TestValidationMetrics_Setters(t *testing.T) {
	t.Parallel()
	metrics := NewValidationMetrics()

	// Test SetTotalObjects
	metrics.SetTotalObjects(100)
	if metrics.TotalObjects != 100 {
		t.Errorf(ConstMagicbeac3774, metrics.TotalObjects)
	}

	// Test SetWorkerCount
	metrics.SetWorkerCount(4)
	if metrics.WorkerCount != 4 {
		t.Errorf(ConstMagicd02a57eb, metrics.WorkerCount)
	}
}

// TestValidationMetrics_UpdateQueueSize tests UpdateQueueSize function
func TestValidationMetrics_UpdateQueueSize(t *testing.T) {
	t.Parallel()
	metrics := NewValidationMetrics()

	// Test initial queue size
	metrics.UpdateQueueSize(10)
	if metrics.MaxQueueSize != 10 {
		t.Errorf(ConstMagice9506f06, metrics.MaxQueueSize)
	}
	if metrics.AverageQueueSize != 10.0 {
		t.Errorf(ConstMagicc4778574, metrics.AverageQueueSize)
	}

	// Test updating with larger size
	metrics.UpdateQueueSize(20)
	if metrics.MaxQueueSize != 20 {
		t.Errorf(ConstMagic342ac684, metrics.MaxQueueSize)
	}

	// Test updating with smaller size (should not change max)
	metrics.UpdateQueueSize(15)
	if metrics.MaxQueueSize != 20 {
		t.Errorf(ConstMagicb83e6716, metrics.MaxQueueSize)
	}
}

// TestValidationMetrics_RecordTierIssue tests RecordTierIssue function
func TestValidationMetrics_RecordTierIssue(t *testing.T) {
	t.Parallel()
	metrics := NewValidationMetrics()

	// Test tier 1
	metrics.RecordTierIssue(1)
	if metrics.Tier1Count != 1 {
		t.Errorf(ConstMagic0af62b51, metrics.Tier1Count)
	}

	// Test tier 2
	metrics.RecordTierIssue(2)
	if metrics.Tier2Count != 1 {
		t.Errorf(ConstMagic49a5f524, metrics.Tier2Count)
	}

	// Test tier 3
	metrics.RecordTierIssue(3)
	if metrics.Tier3Count != 1 {
		t.Errorf(ConstMagic42ce132a, metrics.Tier3Count)
	}

	// Test tier 4
	metrics.RecordTierIssue(4)
	if metrics.Tier4Count != 1 {
		t.Errorf(ConstMagic58b8e963, metrics.Tier4Count)
	}

	// Test invalid tier (should not increment any counter)
	initialTier1 := metrics.Tier1Count
	metrics.RecordTierIssue(5)
	if metrics.Tier1Count != initialTier1 {
		t.Error(ConstMagicd36f7a46)
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
		t.Error(ConstMagiccba14ce5)
	}

	// Check that TotalDuration is set
	if metrics.TotalDuration == emptyValue {
		t.Error(ConstMagic577a4616)
	}

	// Check that ObjectsPerSecond is calculated
	if metrics.ObjectsPerSecond <= 0 {
		t.Error(ConstMagicf220eadb)
	}

	// Check that AverageValidationTime is calculated
	if metrics.AverageValidationTime == emptyValue {
		t.Error(ConstMagiccf5860c9)
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
		t.Fatalf(ConstMagic6d285e66, err)
	}

	// Verify file exists
	if _, err := os.Stat(metricsFile); os.IsNotExist(err) {
		t.Error(ConstMagic4efead2a)
	}

	// Verify file is readable JSON
	data, err := os.ReadFile(metricsFile)
	if err != nil {
		t.Fatalf(ConstMagice72234bf, err)
	}
	if len(data) == 0 {
		t.Error(ConstMagic72c7eeb8)
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
		t.Error(ConstMagic66e2baaa)
	}

	// Verify key metrics are in the string
	if len(str) < 50 {
		t.Error(ConstMagicf993aadb)
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
		t.Errorf(ConstMagic4218184d, rate)
	}

	// Test with hits and misses
	metrics.IncrementCacheMiss()
	rate = metrics.cacheHitRate()
	expected := 66.66666666666666 // 2 hits / 3 total * 100
	if rate < expected-0.1 || rate > expected+0.1 {
		t.Errorf(ConstMagic62e41fb1, expected, rate)
	}
}
