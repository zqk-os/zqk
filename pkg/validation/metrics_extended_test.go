package validation

import (
	"encoding/json"
	"testing"
	"time"
)

func TestValidationMetrics_Extended(t *testing.T) {
	m := NewValidationMetrics()

	// Initial metrics
	if m.GetObjectsPerSecond() != 0 {
		t.Errorf("expected 0 objects per second initially")
	}

	// Test increment and duration
	m.SetTotalObjects(10)
	m.SetWorkerCount(2)
	m.IncrementValidated()
	m.IncrementFailed()
	m.IncrementCacheHit()
	m.IncrementCacheMiss()
	m.IncrementRetry()
	m.UpdateQueueSize(5)
	m.RecordTierIssue(1)
	m.RecordTierIssue(2)
	m.RecordTierIssue(3)
	m.RecordTierIssue(4)

	m.RecordEnqueue(10 * time.Millisecond)
	m.RecordValidation(50 * time.Millisecond)
	m.RecordCollection(5 * time.Millisecond)

	// Hash registry tracking
	m.RecordLockWait(60 * time.Millisecond) // contention > 50ms
	m.RecordLockHold(20 * time.Millisecond)
	m.RecordLockTimeout()
	m.RecordHashRegistryLoad(15 * time.Millisecond)
	m.IncrementHashRegistryConcurrentAccess()
	m.RecordHashRegistryOperations(3)
	m.RecordHashRegistryBucketedProcessing(25 * time.Millisecond)
	m.RecordHashRegistryNonBucketedProcessing(10 * time.Millisecond)

	// Rates
	contentionRate := m.GetContentionRate()
	if contentionRate <= 0 {
		t.Errorf("expected positive contention rate, got %f", contentionRate)
	}

	timeoutRate := m.GetTimeoutRate()
	if timeoutRate <= 0 {
		t.Errorf("expected positive timeout rate, got %f", timeoutRate)
	}

	maxWait := m.GetMaxWaitTime()
	if maxWait <= 0 {
		t.Errorf("expected positive max wait time, got %v", maxWait)
	}

	// Finalize and String summary
	m.Finalize()
	summary := m.String()
	if len(summary) == 0 {
		t.Errorf("expected non-empty summary string")
	}

	// JSONSnapshot
	jsonBytes, err := m.JSONSnapshot()
	if err != nil {
		t.Fatalf("JSONSnapshot failed: %v", err)
	}
	var unmarshaled map[string]any
	if err := json.Unmarshal(jsonBytes, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal JSONSnapshot: %v", err)
	}
}
