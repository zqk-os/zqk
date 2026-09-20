package file

import (
	"testing"
	"time"
)

func TestGenerateLockObservabilityReport(t *testing.T) {
	metrics := &FileLockMetrics{}
	hist := NewContentionHistogram()

	metrics.TotalAcquisitions.Store(100)
	metrics.TotalFailures.Store(5)
	metrics.TotalTimeouts.Store(2)
	metrics.TotalContention.Store(20)
	metrics.TotalWaitTime.Store(int64(250 * time.Millisecond))
	metrics.MaxWaitTime.Store(int64(150 * time.Millisecond))

	for i := 0; i < 90; i++ {
		hist.Record(800 * time.Microsecond)
	}
	for i := 0; i < 10; i++ {
		hist.Record(20 * time.Millisecond)
	}

	report := GenerateLockObservabilityReport(metrics, hist)

	if report.TotalAttempts != 105 {
		t.Fatalf("expected 105 total attempts, got %d", report.TotalAttempts)
	}

	if report.ContentionRate < 0.18 || report.ContentionRate > 0.20 {
		t.Fatalf("expected contention rate ~19%%, got %f", report.ContentionRate)
	}

	if report.AvgWaitTime <= 0 {
		t.Fatalf("expected positive avg wait time, got %v", report.AvgWaitTime)
	}

	if !report.Healthy {
		t.Fatalf("expected report to be healthy")
	}
}

// TestLockObservabilityReport_BLI_STORAGE_MEMBRANE_HEAL_001 verifies lock contention reporting per BLI-STORAGE-MEMBRANE-HEAL-001.
func TestLockObservabilityReport_BLI_STORAGE_MEMBRANE_HEAL_001(t *testing.T) {
	metrics := &FileLockMetrics{}
	hist := NewContentionHistogram()

	metrics.TotalAcquisitions.Store(50)
	metrics.TotalFailures.Store(0)
	metrics.TotalTimeouts.Store(0)
	metrics.TotalContention.Store(1)
	metrics.TotalWaitTime.Store(int64(5 * time.Millisecond))
	metrics.MaxWaitTime.Store(int64(5 * time.Millisecond))

	report := GenerateLockObservabilityReport(metrics, hist)
	if !report.Healthy {
		t.Fatalf("expected report to be healthy for BLI-STORAGE-MEMBRANE-HEAL-001")
	}
}
