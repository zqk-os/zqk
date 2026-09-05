package storage

import (
	"sync"
	"sync/atomic"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/logging"
)

// ValidationStrategy defines the interface for validating CAS index mappings before save.
// Implementations can be synchronous (validate each batch) or asynchronous (use cached file existence data).
//
// This abstraction allows queue workers to swap between validation strategies based on
// performance requirements. When StaleValidationTimeNs metrics show validation is slow,
// an async strategy with background file scanning can be used instead.
//
// See [REDACTED-ID] for the full design and phases.
type ValidationStrategy interface {
	// ValidateMappings filters out stale entries (files that don't exist) from the mappings.
	// Returns:
	//   - validMappings: mappings where the hash file exists
	//   - validBucketKeys: corresponding bucket keys for valid mappings
	//   - staleCount: number of entries removed
	//
	// kindDir is the base directory for this kind (e.g., docs/process/audit)
	// mappings is objectID -> hash
	// bucketKeys is objectID -> bucketKey (optional, may be nil)
	ValidateMappings(kindDir string, mappings map[string]string, bucketKeys map[string]string) (validMappings map[string]string, validBucketKeys map[string]string, staleCount int)

	// Start begins any background operations (e.g., file scanning for async strategies).
	// For sync strategies, this is a no-op.
	// kindDir is the base directory to monitor/scan.
	Start(kindDir string) error

	// Stop gracefully shuts down background operations.
	// For sync strategies, this is a no-op.
	Stop() error

	// Name returns the strategy name for logging/metrics.
	Name() string
}

// ValidationStrategyConfig configures validation strategy behavior.
type ValidationStrategyConfig struct {
	// AsyncThresholdNs is the average validation time (in nanoseconds) above which
	// the system should consider upgrading to an async strategy.
	// Default: 10_000_000 (10ms)
	AsyncThresholdNs int64

	// ScanIntervalMs is how often the async strategy rescans for file changes.
	// Default: 1000 (1 second)
	ScanIntervalMs int64

	// EnableAdaptiveUpgrade allows automatic strategy upgrade when metrics exceed threshold.
	// Default: false (manual upgrade only)
	EnableAdaptiveUpgrade bool
}

// DefaultValidationStrategyConfig returns sensible defaults.
func DefaultValidationStrategyConfig() *ValidationStrategyConfig {
	return &ValidationStrategyConfig{
		AsyncThresholdNs:      10_000_000, // 10ms
		ScanIntervalMs:        1000,       // 1 second
		EnableAdaptiveUpgrade: false,
	}
}

// ValidationStrategyRegistry manages validation strategies per kind.
// This allows different kinds to use different strategies based on their volume characteristics.
type ValidationStrategyRegistry struct {
	strategies map[string]ValidationStrategy // kind -> strategy
	mu         sync.RWMutex
	defaultStr ValidationStrategy
}

var (
	globalValidationStrategyRegistry     *ValidationStrategyRegistry
	globalValidationStrategyRegistryOnce sync.Once
)

// GetGlobalValidationStrategyRegistry returns the singleton registry.
func GetGlobalValidationStrategyRegistry() *ValidationStrategyRegistry {
	globalValidationStrategyRegistryOnce.Do(func() {
		globalValidationStrategyRegistry = &ValidationStrategyRegistry{
			strategies: make(map[string]ValidationStrategy),
			defaultStr: NewSyncValidationStrategy(), // Default to sync
		}
	})
	return globalValidationStrategyRegistry
}

// GetStrategy returns the validation strategy for a kind.
// Falls back to default strategy if no specific strategy is registered.
func (r *ValidationStrategyRegistry) GetStrategy(kind string) ValidationStrategy {
	var strategy ValidationStrategy
	var err_swallow_140 = concurrency.RunInRLock(&r.mu, func() error {
		if s, ok := r.strategies[kind]; ok {
			strategy = s
		} else {
			strategy = r.defaultStr
		}
		return nil
	})
	if err_swallow_140 != nil {

		// RegisterStrategy registers a validation strategy for a specific kind.
		logging.LogSwallowedError(err_swallow_140)
	}
	return strategy
}

func (r *ValidationStrategyRegistry) RegisterStrategy(kind string, strategy ValidationStrategy) {
	var err_swallow_141 = concurrency.RunInLock(&r.mu, func() error {
		r.strategies[kind] = strategy
		return nil
	})
	if err_swallow_141 !=

		// SetDefaultStrategy sets the default strategy for kinds without a specific registration.
		nil {
		logging.LogSwallowedError(err_swallow_141)
	}
}

func (r *ValidationStrategyRegistry) SetDefaultStrategy(strategy ValidationStrategy) {
	var err_swallow_142 = concurrency.RunInLock(&r.mu, func() error {
		r.defaultStr = strategy
		return nil
	})
	if err_swallow_142 !=

		// ValidationMetrics tracks validation performance for adaptive strategy selection.
		nil {
		logging.LogSwallowedError(err_swallow_142)
	}
}

type ValidationMetrics struct {
	TotalValidations    atomic.Int64 // Total number of validation calls
	TotalTimeNs         atomic.Int64 // Total time spent validating (nanoseconds)
	TotalStaleRemoved   atomic.Int64 // Total stale entries removed
	TotalEntriesScanned atomic.Int64 // Total entries scanned
}

// RecordValidation records metrics for a validation operation.
func (m *ValidationMetrics) RecordValidation(durationNs int64, entriesScanned, staleRemoved int) {
	m.TotalValidations.Add(1)
	m.TotalTimeNs.Add(durationNs)
	m.TotalEntriesScanned.Add(int64(entriesScanned))
	m.TotalStaleRemoved.Add(int64(staleRemoved))
}

// AverageValidationTimeNs returns the average validation time in nanoseconds.
func (m *ValidationMetrics) AverageValidationTimeNs() int64 {
	validations := m.TotalValidations.Load()
	if validations == 0 {
		return 0
	}
	return m.TotalTimeNs.Load() / validations
}

// validationMetricsMap tracks per-kind validation metrics.
var (
	validationMetricsMap = make(map[string]*ValidationMetrics)
	validationMetricsMu  sync.RWMutex
)

// KindValidationMetricsSnapshot is a point-in-time copy of per-kind CAS index validation counters.
type KindValidationMetricsSnapshot struct {
	Kind                string `json:"kind"`
	TotalValidations    int64  `json:"total_validations"`
	TotalTimeNs         int64  `json:"total_time_ns"`
	TotalStaleRemoved   int64  `json:"total_stale_removed"`
	TotalEntriesScanned int64  `json:"total_entries_scanned"`
	AverageTimeNs       int64  `json:"average_time_ns"`
}

// SnapshotAllKindValidationMetrics returns copies of all per-kind validation metrics (CAS index write path).
func SnapshotAllKindValidationMetrics() []KindValidationMetricsSnapshot {
	var out []KindValidationMetricsSnapshot
	var err_swallow_143 = concurrency.RunInRLock(&validationMetricsMu, func() error {
		out = make([]KindValidationMetricsSnapshot, 0, len(validationMetricsMap))
		for kind, m := range validationMetricsMap {
			out = append(out, KindValidationMetricsSnapshot{
				Kind:                kind,
				TotalValidations:    m.TotalValidations.Load(),
				TotalTimeNs:         m.TotalTimeNs.Load(),
				TotalStaleRemoved:   m.TotalStaleRemoved.Load(),
				TotalEntriesScanned: m.TotalEntriesScanned.Load(),
				AverageTimeNs:       m.AverageValidationTimeNs(),
			})
		}
		return nil
	})
	if err_swallow_143 !=

		// GetValidationMetrics returns or creates validation metrics for a kind.
		nil {
		logging.LogSwallowedError(err_swallow_143)
	}
	return out
}

func GetValidationMetrics(kind string) *ValidationMetrics {
	// Try read lock first (fast path)
	var m *ValidationMetrics
	var err_swallow_144 = concurrency.RunInRLock(&validationMetricsMu, func() error {
		m = validationMetricsMap[kind]
		return nil
	})
	if err_swallow_144 != nil {
		logging.LogSwallowedError(

			// Need write lock to create new metrics
			err_swallow_144)
	}
	if m != nil {
		return m
	}
	var err_swallow_145 = concurrency.RunInLock(&validationMetricsMu, func() error {

		if existing, ok := validationMetricsMap[kind]; ok {
			m = existing
			return nil
		}
		m = &ValidationMetrics{}
		validationMetricsMap[kind] = m
		return nil
	})
	if err_swallow_145 != nil {
		logging.LogSwallowedError(err_swallow_145)
	}
	return m
}
