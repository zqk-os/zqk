package storage

import (
	"fmt"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// Tests in this file use ResetFileLockMetrics / GetFileLockMetrics (process-global singleton).
// Do not use here: parallel tests in this package would race on the same counters.

func TestFileLockMetrics_Basic(t *testing.T) {
	ResetFileLockMetrics()
	metrics := GetFileLockMetrics()

	// Record some operations
	metrics.RecordAcquisition(100 * time.Microsecond)
	metrics.RecordAcquisition(200 * time.Microsecond)
	metrics.RecordFailure()
	metrics.RecordContention()
	metrics.RecordTimeout(50 * time.Millisecond)

	snapshot := metrics.GetSnapshot()

	if snapshot.TotalAcquisitions != 2 {
		t.Errorf("Expected 2 acquisitions, got %d", snapshot.TotalAcquisitions)
	}
	if snapshot.TotalFailures != 1 {
		t.Errorf("Expected 1 failure, got %d", snapshot.TotalFailures)
	}
	if snapshot.TotalContention != 1 {
		t.Errorf("Expected 1 contention, got %d", snapshot.TotalContention)
	}
	if snapshot.TotalTimeouts != 1 {
		t.Errorf("Expected 1 timeout, got %d", snapshot.TotalTimeouts)
	}

	avgAcq := snapshot.AverageAcquisitionTime()
	expectedAvg := 150 * time.Microsecond // (100 + 200) / 2
	if avgAcq != expectedAvg {
		t.Errorf("Expected average acquisition time %v, got %v", expectedAvg, avgAcq)
	}
}

func TestFileLockMetrics_ContentionRate(t *testing.T) {
	ResetFileLockMetrics()
	metrics := GetFileLockMetrics()

	// 10 acquisitions, 5 contentions, 2 failures
	for i := 0; i < 10; i++ {
		metrics.RecordAcquisition(100 * time.Microsecond)
	}
	for i := 0; i < 5; i++ {
		metrics.RecordContention()
	}
	for i := 0; i < 2; i++ {
		metrics.RecordFailure()
	}

	snapshot := metrics.GetSnapshot()
	contentionRate := snapshot.ContentionRate()
	expectedRate := 5.0 / 17.0 // 5 contentions / 17 total attempts

	if contentionRate != expectedRate {
		t.Errorf("Expected contention rate %f, got %f", expectedRate, contentionRate)
	}
}

func TestFileLockMetrics_SuccessRate(t *testing.T) {
	ResetFileLockMetrics()
	metrics := GetFileLockMetrics()

	// 8 acquisitions, 2 failures
	for i := 0; i < 8; i++ {
		metrics.RecordAcquisition(100 * time.Microsecond)
	}
	for i := 0; i < 2; i++ {
		metrics.RecordFailure()
	}

	snapshot := metrics.GetSnapshot()
	successRate := snapshot.SuccessRate()
	expectedRate := 8.0 / 10.0 // 8 successes / 10 total attempts

	if successRate != expectedRate {
		t.Errorf("Expected success rate %f, got %f", expectedRate, successRate)
	}
}

func TestFileLockMetrics_MaxTimes(t *testing.T) {
	ResetFileLockMetrics()
	metrics := GetFileLockMetrics()

	// Record acquisitions with different times
	metrics.RecordAcquisition(100 * time.Microsecond)
	metrics.RecordAcquisition(500 * time.Microsecond)
	metrics.RecordAcquisition(200 * time.Microsecond)

	snapshot := metrics.GetSnapshot()
	if snapshot.MaxAcquisitionTime != 500*time.Microsecond {
		t.Errorf("Expected max acquisition time 500μs, got %v", snapshot.MaxAcquisitionTime)
	}

	// Record timeouts with different wait times
	metrics.RecordTimeout(10 * time.Millisecond)
	metrics.RecordTimeout(50 * time.Millisecond)
	metrics.RecordTimeout(20 * time.Millisecond)

	snapshot = metrics.GetSnapshot()
	if snapshot.MaxWaitTime != 50*time.Millisecond {
		t.Errorf("Expected max wait time 50ms, got %v", snapshot.MaxWaitTime)
	}
}

func TestFileLockMetrics_ConcurrentUpdates(t *testing.T) {
	ResetFileLockMetrics()
	metrics := GetFileLockMetrics()

	// Concurrent updates
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		i := i
		goroutinelabels.StartTestGoroutine(fmt.Sprintf("test_metrics_record_%d", i), fmt.Sprintf("recording metrics %d", i), func() {
			metrics.RecordAcquisition(100 * time.Microsecond)
			done <- true
		})
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	snapshot := metrics.GetSnapshot()
	if snapshot.TotalAcquisitions != 10 {
		t.Errorf("Expected 10 acquisitions, got %d", snapshot.TotalAcquisitions)
	}
}
