# Observability and Metrics

**Last Verified:** 2026-08-31


## Overview

The graph backend provides comprehensive observability for self-healing and continuous improvement. All metrics are collected automatically and can be exported to various backends.

## Metrics Collection

### Automatic Collection

Metrics are collected automatically for:
- **Operations**: All CRUD operations (CreateNode, UpdateNode, etc.)
- **Retries**: Retry attempts and outcomes
- **Connections**: Acquisition, release, errors
- **Transactions**: Start, commit, rollback, errors
- **Pool**: Wait times, size changes, utilization
- **Queries**: Execution time, row counts, errors
- **Health**: Health check results and latency

### Metrics Interface

```go
type MetricsCollector interface {
    RecordOperation(operation string, duration time.Duration, err error)
    RecordRetry(operation string, attempt int, err error)
    RecordConnectionAcquired(duration time.Duration)
    RecordConnectionReleased()
    RecordTransactionStarted()
    RecordTransactionCommitted(duration time.Duration)
    RecordTransactionRolledBack(duration time.Duration, reason string)
    RecordQuery(operation string, duration time.Duration, rowsAffected int, err error)
    RecordHealthCheck(duration time.Duration, healthy bool)
    GetMetrics() MetricsSnapshot
}
```

### Default Implementation

`DefaultMetricsCollector` provides in-memory metrics collection:
- Thread-safe
- Low overhead
- Suitable for most use cases
- Can be replaced with custom implementations

### Custom Implementations

You can implement `MetricsCollector` to send metrics to:
- **Prometheus**: Export Prometheus metrics
- **StatsD**: Send to StatsD/DataDog
- **OpenTelemetry**: Export traces and metrics
- **Custom backends**: Any monitoring system

## Health Indicators

### Automatic Health Diagnosis

The system automatically analyzes metrics to determine health:

```go
indicator := DiagnoseHealth(metrics)
// Returns:
// - Overall status (healthy/degraded/unhealthy)
// - Component health (pool, connections, queries, transactions)
// - Key metrics (error rate, latency, retry rate, pool utilization)
// - Self-healing recommendations
```

### Health States

- **Healthy**: System operating normally
- **Degraded**: Performance issues detected, but functional
- **Unhealthy**: Critical issues requiring attention
- **Unknown**: Insufficient data

### Self-Healing Recommendations

The system generates actionable recommendations:

- "High error rate detected - check database connectivity"
- "Pool near capacity - consider increasing MaxConns"
- "High retry rate - optimize queries or increase pool size"
- "Slow query detected - consider optimizing or adding indexes"
- "Consecutive health check failures - database may be unavailable"

## Usage Examples

### Basic Usage (Automatic)

```go
pool, _ := provider.CreatePool(ctx, config)
// Metrics are automatically collected

// Get metrics snapshot
metrics := pool.GetMetricsCollector().GetMetrics()
fmt.Printf("Total operations: %d\n", metrics.TotalOperations)
fmt.Printf("Error rate: %.2f%%\n", metrics.Operations["CreateNode"].ErrorRate)
```

### Custom Metrics Backend

```go
// Implement MetricsCollector
type PrometheusCollector struct {
    // ... Prometheus client
}

func (p *PrometheusCollector) RecordOperation(op string, duration time.Duration, err error) {
    // Send to Prometheus
    prometheus.RecordOperation(op, duration, err)
}

// Use custom collector
pool.SetMetricsCollector(&PrometheusCollector{...})
```

### Health Monitoring

```go
// Get health indicator
metrics := pool.GetMetricsCollector().GetMetrics()
indicator := DiagnoseHealth(metrics)

if indicator.Status == HealthStateUnhealthy {
    // Trigger alerting or self-healing
    for _, rec := range indicator.Recommendations {
        log.Warn("Recommendation:", rec)
    }
}
```

### Periodic Reporting

```go
// Report metrics every 5 minutes
ticker := time.NewTicker(5 * time.Minute)
go func() {
    for range ticker.C {
        metrics := pool.GetMetricsCollector().GetMetrics()
        indicator := DiagnoseHealth(metrics)
        
        // Send to monitoring system
        reportMetrics(metrics, indicator)
        
        // Auto-heal based on recommendations
        if indicator.Status == HealthStateDegraded {
            autoHeal(indicator.Recommendations)
        }
    }
}()
```

## Metrics Structure

### Operation Metrics

```go
type OperationMetrics struct {
    Operation      string
    Count          int64
    SuccessCount   int64
    FailureCount   int64
    TotalDuration  time.Duration
    AvgDuration    time.Duration
    ErrorRate      float64
    LastError      error
    LastErrorTime  time.Time
}
```

### Transaction Metrics

```go
type TransactionMetrics struct {
    TotalStarted     int64
    TotalCommitted   int64
    TotalRolledBack  int64
    AvgCommitTime    time.Duration
    AvgRollbackTime  time.Duration
    TransactionErrors int64
    OpenTransactions  int
}
```

### Pool Metrics

```go
type PoolMetrics struct {
    TotalWaits       int64
    AvgWaitTime      time.Duration
    CurrentActive    int
    CurrentIdle      int
    UtilizationRate float64
}
```

## Self-Healing Integration

### Example: Auto-Scale Pool

```go
func autoScalePool(pool *BasePool, metrics MetricsSnapshot) {
    if metrics.Pool.UtilizationRate > 90 {
        // Increase pool size
        newSize := pool.maxSize * 2
        pool.Resize(newSize)
        log.Info("Auto-scaled pool to", newSize)
    }
}
```

### Example: Circuit Breaker

```go
func checkCircuitBreaker(metrics MetricsSnapshot) bool {
    if metrics.Health.ConsecutiveFailures > 5 {
        // Open circuit breaker
        return false
    }
    return true
}
```

### Example: Query Optimization Alerts

```go
func checkSlowQueries(metrics MetricsSnapshot) {
    for _, query := range metrics.Queries {
        if query.AvgDuration > 5*time.Second {
            alert("Slow query detected", query.Query)
        }
    }
}
```

## Best Practices

1. **Monitor Key Metrics**: Error rates, latency, pool utilization
2. **Set Thresholds**: Define what "healthy" means for your system
3. **Automate Responses**: Use recommendations for self-healing
4. **Track Trends**: Monitor metrics over time for patterns
5. **Alert on Degradation**: Set up alerts for degraded/unhealthy states
6. **Regular Reporting**: Export metrics periodically for analysis

## Integration with zqk

The observability system integrates with zqk's self-improvement capabilities:

- **Metrics Collection**: Automatic collection of all operations
- **Health Diagnosis**: Automatic health assessment
- **Recommendations**: Actionable suggestions for improvement
- **Self-Healing**: Can trigger automatic remediation
- **Reporting**: Export metrics for analysis and learning

This enables the system to:
- Detect issues early
- Self-heal common problems
- Learn from patterns
- Continuously improve performance

