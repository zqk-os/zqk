package provider

import (
	"time"
)

const (
	defaultSampleRate              = 1.0
	defaultBufferSize              = 1000
	defaultCollectionInterval      = 10 * time.Second
	defaultHealthCheckInterval     = 30 * time.Second
	defaultErrorRateThreshold      = 5.0
	defaultUnhealthyErrorThreshold = 10.0
	defaultRetryRateThreshold      = 20.0
	defaultPoolUtilization         = 90.0
	defaultMaxWaitTimeThreshold    = 1 * time.Second
	defaultConsecutiveFailures     = 3
	defaultSlowQueryThreshold      = 5 * time.Second
	defaultMaxMetricsHistory       = 100

	highPerfSampleRate         = 0.1
	highPerfBufferSize         = 5000
	highPerfCollectionInterval = 30 * time.Second

	verboseSampleRate          = 1.0
	verboseCollectionInterval  = 5 * time.Second
	verboseHealthCheckInterval = 10 * time.Second
)

// MetricsConfig controls metrics collection behavior
type MetricsConfig struct {
	// Enabled enables or disables metrics collection entirely
	Enabled bool

	// SampleRate controls what percentage of operations are sampled (0.0 to 1.0)
	// 1.0 = collect all metrics, 0.1 = collect 10% of metrics
	// Useful for high-throughput scenarios
	SampleRate float64

	// AsyncRecording enables non-blocking async metrics recording
	// When true, metrics are recorded in background goroutines
	AsyncRecording bool

	// BufferSize is the size of the async recording buffer
	// Only used when AsyncRecording is true
	BufferSize int

	// CollectionInterval is how often to flush/aggregate metrics
	// Only used when AsyncRecording is true
	CollectionInterval time.Duration

	// HealthCheckInterval is how often to run health diagnosis
	HealthCheckInterval time.Duration

	// HealthThresholds define when system is considered degraded/unhealthy
	HealthThresholds HealthThresholds

	// MaxMetricsHistory limits how many historical snapshots to keep
	MaxMetricsHistory int
}

// HealthThresholds define thresholds for health diagnosis
type HealthThresholds struct {
	// ErrorRateThreshold is the error rate (%) that triggers degraded state
	ErrorRateThreshold float64

	// UnhealthyErrorRateThreshold is the error rate (%) that triggers unhealthy state
	UnhealthyErrorRateThreshold float64

	// RetryRateThreshold is the retry rate (%) that triggers degraded state
	RetryRateThreshold float64

	// PoolUtilizationThreshold is the pool utilization (%) that triggers degraded state
	PoolUtilizationThreshold float64

	// MaxWaitTimeThreshold is the max wait time that triggers degraded state
	MaxWaitTimeThreshold time.Duration

	// ConsecutiveFailureThreshold is the number of consecutive failures for unhealthy
	ConsecutiveFailureThreshold int64

	// SlowQueryThreshold is the query duration that triggers a slow query alert
	SlowQueryThreshold time.Duration
}

// DefaultMetricsConfig returns a default metrics configuration
func DefaultMetricsConfig() MetricsConfig {
	return MetricsConfig{
		Enabled:             true,
		SampleRate:          defaultSampleRate, // Collect all metrics by default
		AsyncRecording:      true,              // Non-blocking by default
		BufferSize:          defaultBufferSize,
		CollectionInterval:  defaultCollectionInterval,
		HealthCheckInterval: defaultHealthCheckInterval,
		HealthThresholds: HealthThresholds{
			ErrorRateThreshold:          defaultErrorRateThreshold,      // 5% error rate = degraded
			UnhealthyErrorRateThreshold: defaultUnhealthyErrorThreshold, // 10% error rate = unhealthy
			RetryRateThreshold:          defaultRetryRateThreshold,      // 20% retry rate = degraded
			PoolUtilizationThreshold:    defaultPoolUtilization,         // 90% pool utilization = degraded
			MaxWaitTimeThreshold:        defaultMaxWaitTimeThreshold,
			ConsecutiveFailureThreshold: defaultConsecutiveFailures,
			SlowQueryThreshold:          defaultSlowQueryThreshold,
		},
		MaxMetricsHistory: defaultMaxMetricsHistory,
	}
}

// DisabledMetricsConfig returns a config with metrics disabled
func DisabledMetricsConfig() MetricsConfig {
	config := DefaultMetricsConfig()
	config.Enabled = false
	return config
}

// HighPerformanceMetricsConfig returns a config optimized for high throughput
func HighPerformanceMetricsConfig() MetricsConfig {
	config := DefaultMetricsConfig()
	config.SampleRate = highPerfSampleRate // Sample 10% of operations
	config.AsyncRecording = true
	config.BufferSize = highPerfBufferSize
	config.CollectionInterval = highPerfCollectionInterval
	return config
}

// VerboseMetricsConfig returns a config with maximum observability
func VerboseMetricsConfig() MetricsConfig {
	config := DefaultMetricsConfig()
	config.SampleRate = verboseSampleRate // Collect all metrics
	config.AsyncRecording = false         // Synchronous for immediate visibility
	config.CollectionInterval = verboseCollectionInterval
	config.HealthCheckInterval = verboseHealthCheckInterval
	return config
}
