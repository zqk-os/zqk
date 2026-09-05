package file

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/metricsrecording"
)

// FileLockMetrics tracks metrics for file lock operations
type FileLockMetrics struct {
	// Lock acquisition metrics
	TotalAcquisitions atomic.Int64 // Total number of successful lock acquisitions
	TotalFailures     atomic.Int64 // Total number of failed lock attempts
	TotalTimeouts     atomic.Int64 // Total number of timeout failures
	TotalContention   atomic.Int64 // Total number of times lock was already held (TryLock returned false)

	// Timing metrics (in nanoseconds)
	TotalAcquisitionTime atomic.Int64 // Total time spent acquiring locks (successful)
	TotalWaitTime        atomic.Int64 // Total time spent waiting for locks (timeouts + successful waits)
	MaxAcquisitionTime   atomic.Int64 // Maximum time to acquire a lock
	MaxWaitTime          atomic.Int64 // Maximum wait time (including timeouts)

	// Contention metrics
	CurrentHolders atomic.Int32 // Current number of processes holding locks (approximate)
	PeakContention atomic.Int64 // Maximum number of concurrent lock attempts

	mu sync.RWMutex //nolint:unused // Reserved for future thread-safety
}

var globalFileLockMetrics = &FileLockMetrics{}

// GetFileLockMetrics returns the global file lock metrics
func GetFileLockMetrics() *FileLockMetrics {
	return globalFileLockMetrics
}

// ResetFileLockMetrics resets all metrics (useful for testing)
func ResetFileLockMetrics() {
	globalFileLockMetrics = &FileLockMetrics{}
}

// RecordAcquisition records a successful lock acquisition
func (m *FileLockMetrics) RecordAcquisition(acquisitionTime time.Duration) {
	if !metricsrecording.Enabled() {
		return
	}
	m.TotalAcquisitions.Add(1)
	m.TotalAcquisitionTime.Add(int64(acquisitionTime))

	// Update max acquisition time
	for {
		current := m.MaxAcquisitionTime.Load()
		if int64(acquisitionTime) <= current {
			break
		}
		if m.MaxAcquisitionTime.CompareAndSwap(current, int64(acquisitionTime)) {
			break
		}
	}
}

// RecordFailure records a failed lock attempt
func (m *FileLockMetrics) RecordFailure() {
	if !metricsrecording.Enabled() {
		return
	}
	m.TotalFailures.Add(1)
}

// RecordTimeout records a timeout failure
func (m *FileLockMetrics) RecordTimeout(waitTime time.Duration) {
	if !metricsrecording.Enabled() {
		return
	}
	m.TotalTimeouts.Add(1)
	m.TotalWaitTime.Add(int64(waitTime))

	// Update max wait time
	for {
		current := m.MaxWaitTime.Load()
		if int64(waitTime) <= current {
			break
		}
		if m.MaxWaitTime.CompareAndSwap(current, int64(waitTime)) {
			break
		}
	}
}

// RecordContention records that a lock was already held (TryLock returned false)
func (m *FileLockMetrics) RecordContention() {
	if !metricsrecording.Enabled() {
		return
	}
	m.TotalContention.Add(1)
}

// RecordWaitTime records time spent waiting for a lock (before successful acquisition)
func (m *FileLockMetrics) RecordWaitTime(waitTime time.Duration) {
	if !metricsrecording.Enabled() {
		return
	}
	m.TotalWaitTime.Add(int64(waitTime))

	// Update max wait time
	for {
		current := m.MaxWaitTime.Load()
		if int64(waitTime) <= current {
			break
		}
		if m.MaxWaitTime.CompareAndSwap(current, int64(waitTime)) {
			break
		}
	}
}

// IncrementHolders increments the current holder count
func (m *FileLockMetrics) IncrementHolders() {
	if !metricsrecording.Enabled() {
		return
	}
	m.CurrentHolders.Add(1)
}

// DecrementHolders decrements the current holder count
func (m *FileLockMetrics) DecrementHolders() {
	if !metricsrecording.Enabled() {
		return
	}
	m.CurrentHolders.Add(-1)
}

// RecordContentionAttempt records a contention attempt and updates peak
func (m *FileLockMetrics) RecordContentionAttempt() {
	if !metricsrecording.Enabled() {
		return
	}
	// Increment current holders temporarily to track contention
	current := m.CurrentHolders.Add(1)
	defer m.CurrentHolders.Add(-1)

	// Update peak contention
	for {
		peak := m.PeakContention.Load()
		if int64(current) <= peak {
			break
		}
		if m.PeakContention.CompareAndSwap(peak, int64(current)) {
			break
		}
	}
}

// GetSnapshot returns a snapshot of current metrics
func (m *FileLockMetrics) GetSnapshot() FileLockMetricsSnapshot {
	return FileLockMetricsSnapshot{
		TotalAcquisitions:    m.TotalAcquisitions.Load(),
		TotalFailures:        m.TotalFailures.Load(),
		TotalTimeouts:        m.TotalTimeouts.Load(),
		TotalContention:      m.TotalContention.Load(),
		TotalAcquisitionTime: time.Duration(m.TotalAcquisitionTime.Load()),
		TotalWaitTime:        time.Duration(m.TotalWaitTime.Load()),
		MaxAcquisitionTime:   time.Duration(m.MaxAcquisitionTime.Load()),
		MaxWaitTime:          time.Duration(m.MaxWaitTime.Load()),
		CurrentHolders:       m.CurrentHolders.Load(),
		PeakContention:       m.PeakContention.Load(),
	}
}

// FileLockMetricsSnapshot is a thread-safe snapshot of metrics
type FileLockMetricsSnapshot struct {
	TotalAcquisitions    int64
	TotalFailures        int64
	TotalTimeouts        int64
	TotalContention      int64
	TotalAcquisitionTime time.Duration
	TotalWaitTime        time.Duration
	MaxAcquisitionTime   time.Duration
	MaxWaitTime          time.Duration
	CurrentHolders       int32
	PeakContention       int64
}

// GetTotalOperations returns the total number of operations for title generation
func (s FileLockMetricsSnapshot) GetTotalOperations() int64 {
	return s.TotalAcquisitions
}

// AverageAcquisitionTime returns the average time to acquire a lock
//
//nolint:gocritic // Value receiver snapshot; copying acceptable
func (s FileLockMetricsSnapshot) AverageAcquisitionTime() time.Duration {
	if s.TotalAcquisitions == 0 {
		return 0
	}
	return s.TotalAcquisitionTime / time.Duration(s.TotalAcquisitions)
}

// AverageWaitTime returns the average wait time (including timeouts)
//
//nolint:gocritic // Value receiver snapshot; copying acceptable
func (s FileLockMetricsSnapshot) AverageWaitTime() time.Duration {
	totalWaits := s.TotalTimeouts + s.TotalAcquisitions
	if totalWaits == 0 {
		return 0
	}
	return s.TotalWaitTime / time.Duration(totalWaits)
}

// ContentionRate returns the rate of contention (contention / total attempts)
//
//nolint:gocritic // Value receiver snapshot; copying acceptable
func (s FileLockMetricsSnapshot) ContentionRate() float64 {
	totalAttempts := s.TotalAcquisitions + s.TotalFailures + s.TotalContention
	if totalAttempts == 0 {
		return 0
	}
	return float64(s.TotalContention) / float64(totalAttempts)
}

// SuccessRate returns the rate of successful acquisitions
//
//nolint:gocritic // Value receiver snapshot; copying acceptable
func (s FileLockMetricsSnapshot) SuccessRate() float64 {
	// Include timeouts in the denominator: a timeout is a failed attempt, not a successful acquisition.
	totalAttempts := s.TotalAcquisitions + s.TotalFailures + s.TotalContention + s.TotalTimeouts
	if totalAttempts == 0 {
		return 0
	}
	return float64(s.TotalAcquisitions) / float64(totalAttempts)
}
