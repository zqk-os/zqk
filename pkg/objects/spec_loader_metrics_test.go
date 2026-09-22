package objects

import (
	"testing"
	"time"
)

func TestSpecLoaderMetrics(t *testing.T) {
	metrics := GetGlobalSpecLoaderMetrics()
	if metrics == nil {
		t.Fatal("expected non-nil SpecLoaderMetrics")
	}

	metrics.Reset()

	// Initial snapshot checks
	snap := metrics.GetSnapshot()
	if snap.TotalWaits != 0 || snap.TotalHolds != 0 || snap.TimeoutEvents != 0 {
		t.Fatalf("expected zeroed metrics after Reset, got %+v", snap)
	}
	if metrics.GetContentionRate() != 0 || metrics.GetTimeoutRate() != 0 {
		t.Fatalf("expected zero contention/timeout rates")
	}
	if metrics.GetMaxWaitTime() != 0 {
		t.Fatalf("expected 0 max wait time")
	}
	if metrics.GetObjectsPerSecond() != 0 {
		t.Fatalf("expected 0 objects per second")
	}

	// Set and get objects per second
	metrics.SetObjectsPerSecond(125.5)
	if rate := metrics.GetObjectsPerSecond(); rate < 125.49 || rate > 125.51 {
		t.Errorf("GetObjectsPerSecond: got %v, want 125.5", rate)
	}

	// Record lock waits
	metrics.RecordLockWait(10 * time.Millisecond)
	metrics.RecordLockWait(60 * time.Millisecond) // exceeds 50ms, triggers contention
	metrics.RecordLockWait(20 * time.Millisecond) // lower than 60ms to test max CAS

	// Record lock holds
	metrics.RecordLockHold(5 * time.Millisecond)
	metrics.RecordLockHold(15 * time.Millisecond)
	metrics.RecordLockHold(10 * time.Millisecond) // lower to test max CAS

	// Record timeouts and operations
	metrics.RecordTimeout("spec_parse")
	metrics.RecordOperation("spec_parse")
	metrics.RecordOperation("spec_parse")

	snap = metrics.GetSnapshot()
	if snap.TotalWaits != 3 {
		t.Errorf("expected 3 total waits, got %d", snap.TotalWaits)
	}
	if snap.TotalHolds != 3 {
		t.Errorf("expected 3 total holds, got %d", snap.TotalHolds)
	}
	if snap.ContentionEvents != 1 {
		t.Errorf("expected 1 contention event, got %d", snap.ContentionEvents)
	}
	if snap.TimeoutEvents != 1 {
		t.Errorf("expected 1 timeout event, got %d", snap.TimeoutEvents)
	}
	if snap.MaxWaitTime != 60*time.Millisecond {
		t.Errorf("expected MaxWaitTime 60ms, got %v", snap.MaxWaitTime)
	}
	if snap.MaxHoldTime != 15*time.Millisecond {
		t.Errorf("expected MaxHoldTime 15ms, got %v", snap.MaxHoldTime)
	}
	if snap.AverageWaitTime == 0 {
		t.Errorf("expected non-zero AverageWaitTime")
	}
	if snap.AverageHoldTime == 0 {
		t.Errorf("expected non-zero AverageHoldTime")
	}
	if snap.ContentionRate <= 0 {
		t.Errorf("expected positive ContentionRate, got %v", snap.ContentionRate)
	}
	if snap.TimeoutRate <= 0 {
		t.Errorf("expected positive TimeoutRate, got %v", snap.TimeoutRate)
	}
	if snap.OperationCounts["spec_parse"] != 2 {
		t.Errorf("expected 2 spec_parse ops, got %d", snap.OperationCounts["spec_parse"])
	}
	if snap.OperationCounts["spec_parse_timeout"] != 1 {
		t.Errorf("expected 1 spec_parse_timeout op, got %d", snap.OperationCounts["spec_parse_timeout"])
	}

	if metrics.GetContentionRate() != snap.ContentionRate {
		t.Errorf("GetContentionRate mismatch: %v vs %v", metrics.GetContentionRate(), snap.ContentionRate)
	}
	if metrics.GetTimeoutRate() != snap.TimeoutRate {
		t.Errorf("GetTimeoutRate mismatch: %v vs %v", metrics.GetTimeoutRate(), snap.TimeoutRate)
	}
	if metrics.GetMaxWaitTime() != snap.MaxWaitTime {
		t.Errorf("GetMaxWaitTime mismatch: %v vs %v", metrics.GetMaxWaitTime(), snap.MaxWaitTime)
	}

	metrics.Reset()
	if metrics.GetSnapshot().TotalWaits != 0 {
		t.Errorf("expected 0 total waits after Reset")
	}
}
