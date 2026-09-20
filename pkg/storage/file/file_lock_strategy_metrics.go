package file

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/metricsrecording"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
)

// FileLockStrategyMetrics tracks metrics for file lock strategy operations
// Provides detailed observability into lock usage patterns, resource contention, and system impact
type FileLockStrategyMetrics struct {
	// Strategy-specific metrics
	StrategyAcquisitions map[string]int64 // Per-strategy acquisition counts
	StrategyFailures     map[string]int64 // Per-strategy failure counts
	StrategyTimeouts     map[string]int64 // Per-strategy timeout counts

	// Resource type metrics (tracks what types of resources are being locked)
	ResourceTypeCounts map[string]int64 // e.g., "index", "cache", "object_file"

	// System object metrics (tracks impact on system objects)
	SystemObjectLocks atomic.Int64 // Number of locks on system objects (internal)
	PublicObjectLocks atomic.Int64 // Number of locks on public objects
	IndexFileLocks    atomic.Int64 // Number of locks on index files
	CacheFileLocks    atomic.Int64 // Number of locks on cache files

	// Lock duration metrics (tracks how long resources are held)
	TotalLockDuration   atomic.Int64 // Total time locks are held (nanoseconds)
	MaxLockDuration     atomic.Int64 // Maximum lock duration
	AverageLockDuration atomic.Int64 // Average lock duration (calculated)

	// Stale lock cleanup metrics
	StaleLocksCleaned atomic.Int64 // Number of stale locks cleaned up
	StaleCleanupTime  atomic.Int64 // Total time spent cleaning stale locks

	// Contention metrics by resource type
	ResourceContention map[string]int64 // Contention count per resource type

	strategyEventsTotal       atomic.Int64 // Lifetime count of all strategy lock events
	resourceTypesTrackedTotal atomic.Int64 // Lifetime count of unique resource types tracked

	mu sync.RWMutex
}

// GetFileLockStrategyTotalStats returns lifetime counters for strategy events and unique resource types tracked.
func (m *FileLockStrategyMetrics) GetFileLockStrategyTotalStats() (events, resourceTypes int64) {
	if m == nil {
		return 0, 0
	}
	return m.strategyEventsTotal.Load(), m.resourceTypesTrackedTotal.Load()
}

var globalStrategyMetrics = &FileLockStrategyMetrics{
	StrategyAcquisitions: make(map[string]int64),
	StrategyFailures:     make(map[string]int64),
	StrategyTimeouts:     make(map[string]int64),
	ResourceTypeCounts:   make(map[string]int64),
	ResourceContention:   make(map[string]int64),
}

// GetFileLockStrategyMetrics returns the global strategy metrics
func GetFileLockStrategyMetrics() *FileLockStrategyMetrics {
	return globalStrategyMetrics
}

// ResetFileLockStrategyMetrics resets all metrics (useful for testing)
func ResetFileLockStrategyMetrics() {
	globalStrategyMetrics = &FileLockStrategyMetrics{
		StrategyAcquisitions: make(map[string]int64),
		StrategyFailures:     make(map[string]int64),
		StrategyTimeouts:     make(map[string]int64),
		ResourceTypeCounts:   make(map[string]int64),
		ResourceContention:   make(map[string]int64),
	}
}

// RecordAcquisition records a successful lock acquisition with strategy and resource type
func (m *FileLockStrategyMetrics) RecordAcquisition(strategyName, resourceType string, lockDuration time.Duration) {
	if !metricsrecording.Enabled() {
		return
	}
	m.strategyEventsTotal.Add(1)
	_ = concurrency.RunInLockOrLog(&m.mu, locknames.LockNameFileLockStrategyMetricsRecordAcquisition, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		m.StrategyAcquisitions[strategyName]++
		if _, ok := m.ResourceTypeCounts[resourceType]; !ok {
			m.resourceTypesTrackedTotal.Add(1)
		}
		m.ResourceTypeCounts[resourceType]++
		return nil
	})

	m.TotalLockDuration.Add(int64(lockDuration))

	// Update max lock duration
	for {
		current := m.MaxLockDuration.Load()
		if int64(lockDuration) <= current {
			break
		}
		if m.MaxLockDuration.CompareAndSwap(current, int64(lockDuration)) {
			break
		}
	}

	// Track system vs public object locks
	switch resourceType {
	case "index":
		m.IndexFileLocks.Add(1)
	case "cache":
		m.CacheFileLocks.Add(1)
	case "system_object":
		m.SystemObjectLocks.Add(1)
	case "public_object":
		m.PublicObjectLocks.Add(1)
	}
}

// RecordFailure records a failed lock attempt
func (m *FileLockStrategyMetrics) RecordFailure(strategyName, resourceType string) {
	if !metricsrecording.Enabled() {
		return
	}
	_ = concurrency.RunInLockOrLog(&m.mu, locknames.LockNameFileLockStrategyRecordFailure, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		m.StrategyFailures[strategyName]++
		m.ResourceContention[resourceType]++
		return nil
	})
}

// RecordTimeout records a timeout failure
func (m *FileLockStrategyMetrics) RecordTimeout(strategyName, resourceType string) {
	if !metricsrecording.Enabled() {
		return
	}
	_ = concurrency.RunInLockOrLog(&m.mu, locknames.LockNameFileLockStrategyMetricsRecordTimeout, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		m.StrategyTimeouts[strategyName]++
		m.ResourceContention[resourceType]++
		return nil
	})
}

// RecordStaleCleanup records a stale lock cleanup operation
func (m *FileLockStrategyMetrics) RecordStaleCleanup(cleanupTime time.Duration) {
	if !metricsrecording.Enabled() {
		return
	}
	m.StaleLocksCleaned.Add(1)
	m.StaleCleanupTime.Add(int64(cleanupTime))
}

// GetSnapshot returns a thread-safe snapshot of metrics
func (m *FileLockStrategyMetrics) GetSnapshot() FileLockStrategyMetricsSnapshot {
	snapshot := FileLockStrategyMetricsSnapshot{
		StrategyAcquisitions: make(map[string]int64),
		StrategyFailures:     make(map[string]int64),
		StrategyTimeouts:     make(map[string]int64),
		ResourceTypeCounts:   make(map[string]int64),
		ResourceContention:   make(map[string]int64),
	}

	_ = concurrency.RunInRLockOrLog(&m.mu, locknames.LockNameFileLockStrategyMetricsGetSnapshot, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		for k, v := range m.StrategyAcquisitions {
			snapshot.StrategyAcquisitions[k] = v
		}
		for k, v := range m.StrategyFailures {
			snapshot.StrategyFailures[k] = v
		}
		for k, v := range m.StrategyTimeouts {
			snapshot.StrategyTimeouts[k] = v
		}
		for k, v := range m.ResourceTypeCounts {
			snapshot.ResourceTypeCounts[k] = v
		}
		for k, v := range m.ResourceContention {
			snapshot.ResourceContention[k] = v
		}
		return nil
	})

	// Load atomic values outside lock (they're already thread-safe)
	snapshot.SystemObjectLocks = m.SystemObjectLocks.Load()
	snapshot.PublicObjectLocks = m.PublicObjectLocks.Load()
	snapshot.IndexFileLocks = m.IndexFileLocks.Load()
	snapshot.CacheFileLocks = m.CacheFileLocks.Load()
	snapshot.TotalLockDuration = time.Duration(m.TotalLockDuration.Load())
	snapshot.MaxLockDuration = time.Duration(m.MaxLockDuration.Load())
	snapshot.StaleLocksCleaned = m.StaleLocksCleaned.Load()
	snapshot.StaleCleanupTime = time.Duration(m.StaleCleanupTime.Load())

	// Calculate average
	totalAcquisitions := int64(0)
	for _, count := range snapshot.StrategyAcquisitions {
		totalAcquisitions += count
	}
	if totalAcquisitions > 0 {
		snapshot.AverageLockDuration = snapshot.TotalLockDuration / time.Duration(totalAcquisitions)
	}

	return snapshot
}

// FileLockStrategyMetricsSnapshot is a thread-safe snapshot of strategy metrics
type FileLockStrategyMetricsSnapshot struct {
	StrategyAcquisitions map[string]int64
	StrategyFailures     map[string]int64
	StrategyTimeouts     map[string]int64
	ResourceTypeCounts   map[string]int64
	ResourceContention   map[string]int64

	SystemObjectLocks int64
	PublicObjectLocks int64
	IndexFileLocks    int64
	CacheFileLocks    int64

	TotalLockDuration   time.Duration
	MaxLockDuration     time.Duration
	AverageLockDuration time.Duration

	StaleLocksCleaned int64
	StaleCleanupTime  time.Duration
}

// GetTotalOperations returns total number of lock operations
func (s FileLockStrategyMetricsSnapshot) GetTotalOperations() int64 {
	total := int64(0)
	for _, count := range s.StrategyAcquisitions {
		total += count
	}
	for _, count := range s.StrategyFailures {
		total += count
	}
	for _, count := range s.StrategyTimeouts {
		total += count
	}
	return total
}

// GetContentionRate returns the overall contention rate
func (s FileLockStrategyMetricsSnapshot) GetContentionRate() float64 {
	totalOps := s.GetTotalOperations()
	if totalOps == 0 {
		return 0.0
	}

	totalContention := int64(0)
	for _, count := range s.ResourceContention {
		totalContention += count
	}

	return float64(totalContention) / float64(totalOps)
}

// GetResourceTypeBreakdown returns a breakdown of locks by resource type
func (s FileLockStrategyMetricsSnapshot) GetResourceTypeBreakdown() map[string]float64 {
	breakdown := make(map[string]float64)
	total := int64(0)

	for _, count := range s.ResourceTypeCounts {
		total += count
	}

	if total > 0 {
		for resourceType, count := range s.ResourceTypeCounts {
			breakdown[resourceType] = float64(count) / float64(total) * 100.0
		}
	}

	return breakdown
}
