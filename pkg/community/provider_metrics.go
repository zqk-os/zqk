package community

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// ErrProviderQuarantined is returned when an invocation is attempted on a quarantined provider.
	ErrProviderQuarantined = errors.New("provider is currently quarantined due to anomalous error rate")
)

// ProviderHealthState represents the health status of a provider.
type ProviderHealthState string

const (
	// HealthStateHealthy indicates normal operational status.
	HealthStateHealthy ProviderHealthState = "healthy"
	// HealthStateDegraded indicates elevated latency or warnings.
	HealthStateDegraded ProviderHealthState = "degraded"
	// HealthStateQuarantined indicates the provider is isolated due to high error rate.
	HealthStateQuarantined ProviderHealthState = "quarantined"
)

// ProviderExecutionMetric captures a single execution data point.
type ProviderExecutionMetric struct {
	ProviderID string
	Duration   time.Duration
	Err        error
	Timestamp  time.Time
}

// ProviderMetricsSummary represents aggregated telemetry for a provider.
type ProviderMetricsSummary struct {
	ProviderID      string
	TotalCalls      int64
	SuccessfulCalls int64
	FailedCalls     int64
	AverageLatency  time.Duration
	ErrorRate       float64
	HealthState     ProviderHealthState
}

// ProviderMetricsCollectorConfig holds configuration thresholds for metric collection.
type ProviderMetricsCollectorConfig struct {
	MaxErrorRateThreshold float64       // e.g. 0.5 (50% failure rate)
	MinSamplesForAnomaly  int64         // minimum sample count before quarantine can trigger
	LatencyWarningCutoff  time.Duration // latency threshold marking state as degraded
}

// DefaultMetricsCollectorConfig provides safe production defaults.
func DefaultMetricsCollectorConfig() ProviderMetricsCollectorConfig {
	return ProviderMetricsCollectorConfig{
		MaxErrorRateThreshold: 0.50,
		MinSamplesForAnomaly:  5,
		LatencyWarningCutoff:  500 * time.Millisecond,
	}
}

// ProviderMetricsCollector tracks real-time execution statistics and isolates anomalous providers.
type ProviderMetricsCollector struct {
	mu          sync.RWMutex
	config      ProviderMetricsCollectorConfig
	summaries   map[string]*ProviderMetricsSummary
	totalDelays map[string]time.Duration
}

// NewProviderMetricsCollector creates an initialized metrics collector.
func NewProviderMetricsCollector(config ProviderMetricsCollectorConfig) *ProviderMetricsCollector {
	return &ProviderMetricsCollector{
		config:      config,
		summaries:   make(map[string]*ProviderMetricsSummary),
		totalDelays: make(map[string]time.Duration),
	}
}

// RecordExecution registers an invocation data point and evaluates anomaly boundaries.
func (mc *ProviderMetricsCollector) RecordExecution(metric ProviderExecutionMetric) ProviderHealthState {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	summary, ok := mc.summaries[metric.ProviderID]
	if !ok {
		summary = &ProviderMetricsSummary{
			ProviderID:  metric.ProviderID,
			HealthState: HealthStateHealthy,
		}
		mc.summaries[metric.ProviderID] = summary
	}

	summary.TotalCalls++
	if metric.Err != nil {
		summary.FailedCalls++
	} else {
		summary.SuccessfulCalls++
	}

	mc.totalDelays[metric.ProviderID] += metric.Duration
	summary.AverageLatency = time.Duration(int64(mc.totalDelays[metric.ProviderID]) / summary.TotalCalls)
	summary.ErrorRate = float64(summary.FailedCalls) / float64(summary.TotalCalls)

	// Anomaly evaluation: check error rate against threshold if sample floor is met
	if summary.HealthState != HealthStateQuarantined {
		if summary.TotalCalls >= mc.config.MinSamplesForAnomaly && summary.ErrorRate >= mc.config.MaxErrorRateThreshold {
			summary.HealthState = HealthStateQuarantined
		} else if summary.AverageLatency > mc.config.LatencyWarningCutoff {
			summary.HealthState = HealthStateDegraded
		} else {
			summary.HealthState = HealthStateHealthy
		}
	}

	return summary.HealthState
}

// CheckInvocationAllowed verifies whether a provider is eligible to execute.
func (mc *ProviderMetricsCollector) CheckInvocationAllowed(ctx context.Context, providerID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	mc.mu.RLock()
	defer mc.mu.RUnlock()

	summary, ok := mc.summaries[providerID]
	if !ok {
		return nil // Provider is fresh and clean
	}

	if summary.HealthState == HealthStateQuarantined {
		return ErrProviderQuarantined
	}

	return nil
}

// ResetQuarantine explicitly lifts a quarantine state and resets error counters for a provider.
func (mc *ProviderMetricsCollector) ResetQuarantine(providerID string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	summary, ok := mc.summaries[providerID]
	if !ok {
		return
	}

	summary.HealthState = HealthStateHealthy
	summary.TotalCalls = 0
	summary.SuccessfulCalls = 0
	summary.FailedCalls = 0
	summary.ErrorRate = 0
	summary.AverageLatency = 0
	mc.totalDelays[providerID] = 0
}

// GetSummary retrieves a copy of aggregated metrics for a specific provider.
func (mc *ProviderMetricsCollector) GetSummary(providerID string) (ProviderMetricsSummary, bool) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	summary, ok := mc.summaries[providerID]
	if !ok {
		return ProviderMetricsSummary{}, false
	}
	return *summary, true
}
