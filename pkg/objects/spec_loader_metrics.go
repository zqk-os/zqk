package objects

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
)

// SpecLoaderMetrics tracks metrics for spec loader lock operations
// Implements LockMetrics interface for use with WithRLockTimeout/WithLockTimeout
// All metrics are collected asynchronously using atomic operations for efficiency
type SpecLoaderMetrics struct {
	// Lock wait metrics (time to acquire lock)
	totalWaitTime atomic.Int64 // Total wait time in nanoseconds
	totalWaits    atomic.Int64 // Total number of wait events
	maxWaitTime   atomic.Int64 // Maximum wait time in nanoseconds
	waitTimeouts  atomic.Int64 // Number of wait timeouts

	// Lock hold metrics (time lock is held)
	totalHoldTime atomic.Int64 // Total hold time in nanoseconds
	totalHolds    atomic.Int64 // Total number of hold events
	maxHoldTime   atomic.Int64 // Maximum hold time in nanoseconds

	// Contention metrics
	contentionEvents atomic.Int64 // Number of times wait time exceeded threshold
	timeoutEvents    atomic.Int64 // Number of operation timeouts

	// Operation-specific metrics
	operationCounts map[string]*atomic.Int64 // Per-operation counts
	operationMu     sync.RWMutex             // Protects operationCounts map

	// Objects per second (for timeout calculation)
	objectsPerSecond atomic.Int64 // Stored as int64 (multiplied by 1000 for precision)
}

var globalSpecLoaderMetrics = &SpecLoaderMetrics{
	operationCounts: make(map[string]*atomic.Int64),
}

// GetGlobalSpecLoaderMetrics returns the global spec loader metrics instance
func GetGlobalSpecLoaderMetrics() *SpecLoaderMetrics {
	return globalSpecLoaderMetrics
}

// GetObjectsPerSecond returns the current processing rate (for timeout calculation)
// Implements LockMetrics interface
func (m *SpecLoaderMetrics) GetObjectsPerSecond() float64 {
	val := m.objectsPerSecond.Load()
	if val == 0 {
		return 0
	}
	// Stored as int64 * 1000 for precision, convert back
	return float64(val) / 1000.0
}

// GetContentionRate returns the current contention rate (0.0 to 1.0)
// Implements LockMetrics interface
func (m *SpecLoaderMetrics) GetContentionRate() float64 {
	snapshot := m.GetSnapshot()
	return snapshot.ContentionRate
}

// GetTimeoutRate returns the current timeout rate (0.0 to 1.0)
// Implements LockMetrics interface
func (m *SpecLoaderMetrics) GetTimeoutRate() float64 {
	snapshot := m.GetSnapshot()
	return snapshot.TimeoutRate
}

// GetMaxWaitTime returns the maximum wait time observed
// Implements LockMetrics interface
func (m *SpecLoaderMetrics) GetMaxWaitTime() time.Duration {
	snapshot := m.GetSnapshot()
	return snapshot.MaxWaitTime
}

// SetObjectsPerSecond sets the current processing rate
func (m *SpecLoaderMetrics) SetObjectsPerSecond(rate float64) {
	// Store as int64 * 1000 for precision
	m.objectsPerSecond.Store(int64(rate * 1000))
}

// RecordLockWait records lock wait time
// Implements LockMetrics interface
func (m *SpecLoaderMetrics) RecordLockWait(duration time.Duration) {
	nanos := int64(duration)
	m.totalWaitTime.Add(nanos)
	m.totalWaits.Add(1)

	// Update max wait time
	for {
		current := m.maxWaitTime.Load()
		if nanos <= current {
			break
		}
		if m.maxWaitTime.CompareAndSwap(current, nanos) {
			break
		}
	}

	// Track contention (wait time > 50ms indicates contention)
	if duration > 50*time.Millisecond {
		m.contentionEvents.Add(1)
	}
}

// RecordLockHold records lock hold time
// Implements LockMetrics interface
func (m *SpecLoaderMetrics) RecordLockHold(duration time.Duration) {
	nanos := int64(duration)
	m.totalHoldTime.Add(nanos)
	m.totalHolds.Add(1)

	// Update max hold time
	for {
		current := m.maxHoldTime.Load()
		if nanos <= current {
			break
		}
		if m.maxHoldTime.CompareAndSwap(current, nanos) {
			break
		}
	}
}

// RecordTimeout records an operation timeout event
func (m *SpecLoaderMetrics) RecordTimeout(operation string) {
	m.timeoutEvents.Add(1)
	m.recordOperation(operation + "_timeout")
}

// RecordOperation records an operation occurrence
func (m *SpecLoaderMetrics) RecordOperation(operation string) {
	m.recordOperation(operation)
}

// recordOperation records an operation count (thread-safe)
func (m *SpecLoaderMetrics) recordOperation(operation string) {
	var counter *atomic.Int64
	_ = concurrency.RunInRLock(&m.operationMu, func() error {
		counter = m.operationCounts[operation]
		return nil
	})

	if counter == nil {
		_ = concurrency.RunInLock(&m.operationMu, func() error {
			// Double-check after acquiring write lock
			counter = m.operationCounts[operation]
			if counter == nil {
				counter = &atomic.Int64{}
				m.operationCounts[operation] = counter
			}
			return nil
		})
	}

	counter.Add(1)
}

// GetSnapshot returns a snapshot of current metrics
type SpecLoaderMetricsSnapshot struct {
	TotalWaitTime    time.Duration
	TotalWaits       int64
	MaxWaitTime      time.Duration
	WaitTimeouts     int64
	TotalHoldTime    time.Duration
	TotalHolds       int64
	MaxHoldTime      time.Duration
	ContentionEvents int64
	TimeoutEvents    int64
	ObjectsPerSecond float64
	OperationCounts  map[string]int64
	AverageWaitTime  time.Duration
	AverageHoldTime  time.Duration
	ContentionRate   float64
	TimeoutRate      float64
}

// GetSnapshot returns a snapshot of current metrics
func (m *SpecLoaderMetrics) GetSnapshot() SpecLoaderMetricsSnapshot {
	totalWaits := m.totalWaits.Load()
	totalHolds := m.totalHolds.Load()
	totalTimeouts := m.timeoutEvents.Load()
	totalOperations := totalWaits + totalHolds

	snapshot := SpecLoaderMetricsSnapshot{
		TotalWaitTime:    time.Duration(m.totalWaitTime.Load()),
		TotalWaits:       totalWaits,
		MaxWaitTime:      time.Duration(m.maxWaitTime.Load()),
		WaitTimeouts:     m.waitTimeouts.Load(),
		TotalHoldTime:    time.Duration(m.totalHoldTime.Load()),
		TotalHolds:       totalHolds,
		MaxHoldTime:      time.Duration(m.maxHoldTime.Load()),
		ContentionEvents: m.contentionEvents.Load(),
		TimeoutEvents:    totalTimeouts,
		ObjectsPerSecond: m.GetObjectsPerSecond(),
		OperationCounts:  make(map[string]int64),
	}

	// Calculate averages
	if totalWaits > 0 {
		snapshot.AverageWaitTime = time.Duration(m.totalWaitTime.Load() / totalWaits)
	}
	if totalHolds > 0 {
		snapshot.AverageHoldTime = time.Duration(m.totalHoldTime.Load() / totalHolds)
	}

	// Calculate rates
	if totalOperations > 0 {
		snapshot.ContentionRate = float64(snapshot.ContentionEvents) / float64(totalOperations)
		snapshot.TimeoutRate = float64(totalTimeouts) / float64(totalOperations)
	}

	// Copy operation counts
	_ = concurrency.RunInRLock(&m.operationMu, func() error {
		for op, counter := range m.operationCounts {
			snapshot.OperationCounts[op] = counter.Load()
		}
		return nil
	})

	return snapshot
}

// Reset resets all metrics (useful for testing)
func (m *SpecLoaderMetrics) Reset() {
	m.totalWaitTime.Store(0)
	m.totalWaits.Store(0)
	m.maxWaitTime.Store(0)
	m.waitTimeouts.Store(0)
	m.totalHoldTime.Store(0)
	m.totalHolds.Store(0)
	m.maxHoldTime.Store(0)
	m.contentionEvents.Store(0)
	m.timeoutEvents.Store(0)
	m.objectsPerSecond.Store(0)

	_ = concurrency.RunInLock(&m.operationMu, func() error {
		m.operationCounts = make(map[string]*atomic.Int64)
		return nil
	})
}
