package provider

import (
	"context"
	"time"
)

// HealthStatus represents the health of a connection or pool
type HealthStatus struct {
	Status    HealthState
	Latency   time.Duration
	Error     error
	Timestamp time.Time
	Details   map[string]any
}

// HealthState represents the current health state
type HealthState string

const (
	HealthStateHealthy   HealthState = "healthy"
	HealthStateDegraded  HealthState = "degraded"
	HealthStateUnhealthy HealthState = "unhealthy"
	HealthStateUnknown   HealthState = "unknown"
)

// HealthChecker provides health check capabilities
type HealthChecker interface {
	// CheckHealth performs a health check
	CheckHealth(ctx context.Context) HealthStatus

	// GetHealthHistory returns recent health check history
	GetHealthHistory(limit int) []HealthStatus
}

// HealthIndicator provides detailed health information for self-healing
type HealthIndicator struct {
	// Overall status
	Status HealthState

	// Component health
	PoolHealth        HealthStatus
	ConnectionHealth  HealthStatus
	QueryHealth       HealthStatus
	TransactionHealth HealthStatus

	// Metrics-based indicators
	ErrorRate       float64
	AvgLatency      time.Duration
	RetryRate       float64
	PoolUtilization float64

	// Recommendations for self-healing
	Recommendations []string

	// Timestamp
	Timestamp time.Time
}

// DiagnoseHealth analyzes metrics and returns health indicators
//
//nolint:gocritic // MetricsSnapshot is small enough here; value keeps caller data immutable
func DiagnoseHealth(metrics MetricsSnapshot) HealthIndicator {
	return DiagnoseHealthWithThresholds(&metrics, HealthThresholds{})
}

// DiagnoseHealthWithThresholds analyzes metrics with custom thresholds
func DiagnoseHealthWithThresholds(metrics *MetricsSnapshot, thresholds HealthThresholds) HealthIndicator {
	indicator := HealthIndicator{
		Timestamp: time.Now(),
		Status:    HealthStateUnknown,
	}

	// Use default thresholds if none provided
	if thresholds.ErrorRateThreshold == 0 {
		thresholds = DefaultMetricsConfig().HealthThresholds
	}

	// Calculate overall error rate
	var totalOps, totalErrors int64
	for _, op := range metrics.Operations {
		totalOps += op.Count
		totalErrors += op.FailureCount
	}
	if totalOps > 0 {
		indicator.ErrorRate = float64(totalErrors) / float64(totalOps) * 100
	}

	// Calculate average latency
	var totalDuration time.Duration
	for _, op := range metrics.Operations {
		totalDuration += op.TotalDuration
	}
	if totalOps > 0 {
		indicator.AvgLatency = totalDuration / time.Duration(totalOps)
	}

	// Calculate retry rate
	if metrics.TotalOperations > 0 {
		indicator.RetryRate = float64(metrics.TotalRetries) / float64(metrics.TotalOperations) * 100
	}

	// Pool utilization
	indicator.PoolUtilization = metrics.Pool.UtilizationRate

	// Determine overall status using configurable thresholds
	if indicator.ErrorRate >= thresholds.UnhealthyErrorRateThreshold ||
		metrics.Health.ConsecutiveFailures >= thresholds.ConsecutiveFailureThreshold {
		indicator.Status = HealthStateUnhealthy
	} else if indicator.ErrorRate >= thresholds.ErrorRateThreshold ||
		indicator.RetryRate >= thresholds.RetryRateThreshold ||
		metrics.Pool.UtilizationRate >= thresholds.PoolUtilizationThreshold ||
		metrics.Pool.MaxWaitTime >= thresholds.MaxWaitTimeThreshold {
		indicator.Status = HealthStateDegraded
	} else {
		indicator.Status = HealthStateHealthy
	}

	// Generate recommendations
	indicator.Recommendations = generateRecommendationsWithThresholds(metrics, indicator, thresholds)

	return indicator
}

// generateRecommendations creates self-healing recommendations based on metrics
//
//nolint:unused,gocritic // Helper retained; value semantics acceptable for readability
func generateRecommendations(metrics *MetricsSnapshot, indicator HealthIndicator) []string {
	return generateRecommendationsWithThresholds(metrics, indicator, DefaultMetricsConfig().HealthThresholds)
}

// generateRecommendationsWithThresholds creates recommendations using custom thresholds
//
//nolint:gocritic // HealthIndicator passed by value for readability; size acceptable
func generateRecommendationsWithThresholds(metrics *MetricsSnapshot, indicator HealthIndicator, thresholds HealthThresholds) []string {
	var recommendations []string

	// High error rate
	if indicator.ErrorRate >= thresholds.UnhealthyErrorRateThreshold {
		recommendations = append(recommendations, "High error rate detected - check database connectivity and query patterns")
	} else if indicator.ErrorRate >= thresholds.ErrorRateThreshold {
		recommendations = append(recommendations, "Elevated error rate - monitor database connectivity")
	}

	// High retry rate
	if indicator.RetryRate >= thresholds.RetryRateThreshold {
		recommendations = append(recommendations, "High retry rate - consider increasing connection pool size or optimizing queries")
	}

	// Pool exhaustion
	if metrics.Pool.UtilizationRate >= thresholds.PoolUtilizationThreshold {
		recommendations = append(recommendations, "Pool near capacity - consider increasing MaxConns or optimizing connection usage")
	}

	// High wait times
	if metrics.Pool.MaxWaitTime >= thresholds.MaxWaitTimeThreshold {
		recommendations = append(recommendations, "High connection wait times - increase pool size")
	}

	// Transaction issues
	if metrics.Transactions.TransactionErrors > 0 {
		errorRate := float64(metrics.Transactions.TransactionErrors) / float64(metrics.Transactions.TotalStarted) * 100
		if errorRate > 5 {
			recommendations = append(recommendations, "High transaction error rate - check transaction timeout settings")
		}
	}

	// Health check failures
	if metrics.Health.ConsecutiveFailures >= thresholds.ConsecutiveFailureThreshold {
		recommendations = append(recommendations, "Consecutive health check failures - database may be unavailable")
	}

	// Slow queries
	for _, query := range metrics.Queries {
		if query.AvgDuration >= thresholds.SlowQueryThreshold {
			recommendations = append(recommendations,
				"Slow query detected: "+query.Query+" - consider optimizing or adding indexes")
		}
	}

	return recommendations
}
