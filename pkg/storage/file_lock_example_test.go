package storage

import (
	"fmt"
	"time"
)

func ExampleFileLockMetrics() {
	// Reset metrics for clean example
	ResetFileLockMetrics()
	metrics := GetFileLockMetrics()

	// Simulate some lock operations
	metrics.RecordAcquisition(100 * time.Microsecond)
	metrics.RecordAcquisition(200 * time.Microsecond)
	metrics.RecordContention()
	metrics.RecordTimeout(50 * time.Millisecond)

	// Get snapshot
	snapshot := metrics.GetSnapshot()

	fmt.Printf("Total Acquisitions: %d\n", snapshot.TotalAcquisitions)
	fmt.Printf("Contention Rate: %.1f%%\n", snapshot.ContentionRate()*100)
	fmt.Printf("Avg Acquisition Time: %v\n", snapshot.AverageAcquisitionTime())
	fmt.Printf("Success Rate: %.1f%%\n", snapshot.SuccessRate()*100)

	// Output:
	// Total Acquisitions: 2
	// Contention Rate: 33.3%
	// Avg Acquisition Time: 150µs
	// Success Rate: 50.0%
}

func ExampleFileLockConfig() {
	// Create lock with early bailout threshold
	config := FileLockConfig{
		EarlyBailoutThreshold: 100 * time.Millisecond,
		EnableMetrics:         true,
	}

	// When using LockWithTimeout, if contention is detected and threshold is exceeded,
	// the function will return an error suggesting early bailout
	_ = config

	// This allows processes to detect system busyness and fail fast
	// rather than waiting for the full timeout
}
