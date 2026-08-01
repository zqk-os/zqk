# Metrics Configuration Guide

## Overview

The observability system is fully configurable, adjustable, and non-blocking. You can:
- Enable/disable metrics collection
- Adjust sampling rates for high-throughput scenarios
- Use async recording for non-blocking operation
- Configure health thresholds
- Update configuration at runtime

## Configuration Options

### Basic Configuration

```go
// Default configuration (recommended for most use cases)
config := DefaultMetricsConfig()
// - Enabled: true
// - SampleRate: 1.0 (collect all metrics)
// - AsyncRecording: true (non-blocking)
// - BufferSize: 1000

// Disable metrics entirely
config := DisabledMetricsConfig()

// High-performance configuration (for high-throughput scenarios)
config := HighPerformanceMetricsConfig()
// - SampleRate: 0.1 (sample 10% of operations)
// - BufferSize: 5000
// - CollectionInterval: 30s

// Verbose configuration (maximum observability)
config := VerboseMetricsConfig()
// - SampleRate: 1.0 (collect all)
// - AsyncRecording: false (synchronous for immediate visibility)
// - CollectionInterval: 5s
```

### Custom Configuration

```go
config := MetricsConfig{
    Enabled:            true,
    SampleRate:         0.5,              // Sample 50% of operations
    AsyncRecording:     true,            // Non-blocking
    BufferSize:         2000,            // Larger buffer for high throughput
    CollectionInterval: 15 * time.Second,
    HealthCheckInterval: 60 * time.Second,
    HealthThresholds: HealthThresholds{
        ErrorRateThreshold:           3.0,  // 3% error rate = degraded
        UnhealthyErrorRateThreshold:  7.0,  // 7% error rate = unhealthy
        RetryRateThreshold:           15.0, // 15% retry rate = degraded
        PoolUtilizationThreshold:     85.0, // 85% pool utilization = degraded
        MaxWaitTimeThreshold:         500 * time.Millisecond,
        ConsecutiveFailureThreshold:  5,
        SlowQueryThreshold:           3 * time.Second,
    },
    MaxMetricsHistory: 200,
}
```

## Usage Examples

### Creating a Pool with Custom Metrics Config

```go
// Create pool with custom metrics configuration
connConfig := ConnectionConfig{
    Host: "localhost",
    Port: 7687,
}

metricsConfig := HighPerformanceMetricsConfig()
pool := provider.NewBasePoolWithMetrics(connConfig, 20, metricsConfig)
```

### Updating Configuration at Runtime

```go
// Update metrics configuration without recreating pool
newConfig := MetricsConfig{
    Enabled:        true,
    SampleRate:     0.2, // Reduce to 20% sampling
    AsyncRecording: true,
}

pool.UpdateMetricsConfig(newConfig)
```

### Disabling Metrics

```go
// Disable metrics collection
pool.UpdateMetricsConfig(DisabledMetricsConfig())

// Or create pool without metrics
pool := provider.NewBasePoolWithMetrics(connConfig, 20, DisabledMetricsConfig())
```

## Non-Blocking Operation

### Async Recording

By default, metrics are recorded asynchronously in background goroutines:

```go
config := MetricsConfig{
    AsyncRecording: true,    // Non-blocking
    BufferSize:     1000,    // Buffer size for async queue
}

// Operations return immediately, metrics recorded in background
conn.CreateNode(ctx, node) // Returns immediately
```

### Buffer Overflow Handling

When the async buffer is full, metrics are dropped (non-blocking):

```go
// If buffer is full, metric is dropped (doesn't block)
// Increase BufferSize for high-throughput scenarios
config := MetricsConfig{
    BufferSize: 10000, // Larger buffer for high throughput
}
```

### Synchronous Recording

For immediate visibility (may block on high throughput):

```go
config := MetricsConfig{
    AsyncRecording: false, // Synchronous recording
}
```

## Sampling

### Sampling Rate

Control what percentage of operations are sampled:

```go
config := MetricsConfig{
    SampleRate: 0.1, // Sample 10% of operations
}

// Operations are randomly sampled
// Errors and transactions are always recorded (not sampled)
```

### When to Use Sampling

- **High-throughput scenarios**: Reduce overhead with 10-20% sampling
- **Development/debugging**: Use 100% sampling for full visibility
- **Production**: Balance between observability and performance

## Health Thresholds

### Configurable Thresholds

```go
thresholds := HealthThresholds{
    ErrorRateThreshold:           5.0,  // 5% error rate = degraded
    UnhealthyErrorRateThreshold:  10.0, // 10% error rate = unhealthy
    RetryRateThreshold:           20.0, // 20% retry rate = degraded
    PoolUtilizationThreshold:     90.0, // 90% pool utilization = degraded
    MaxWaitTimeThreshold:         1 * time.Second,
    ConsecutiveFailureThreshold:  3,
    SlowQueryThreshold:           5 * time.Second,
}

config := MetricsConfig{
    HealthThresholds: thresholds,
}
```

### Using Custom Thresholds

```go
// Get metrics snapshot
metrics := pool.GetMetricsCollector().GetMetrics()

// Diagnose with custom thresholds
indicator := DiagnoseHealthWithThresholds(metrics, customThresholds)
```

## Performance Considerations

### Overhead

- **Disabled**: Zero overhead
- **Sampled (10%)**: ~0.1% overhead
- **Full sampling, async**: ~0.5% overhead
- **Full sampling, sync**: ~2-5% overhead (on high throughput)

### Recommendations

1. **Development**: Use `VerboseMetricsConfig()` for full visibility
2. **Production (normal load)**: Use `DefaultMetricsConfig()`
3. **Production (high load)**: Use `HighPerformanceMetricsConfig()`
4. **Debugging issues**: Temporarily enable verbose config

## Best Practices

1. **Start with defaults**: Use `DefaultMetricsConfig()` initially
2. **Adjust based on load**: Switch to high-performance config if needed
3. **Monitor buffer usage**: If buffer fills frequently, increase `BufferSize`
4. **Tune thresholds**: Adjust health thresholds based on your system's characteristics
5. **Disable when not needed**: Use `DisabledMetricsConfig()` if metrics aren't needed

## Runtime Updates

Configuration can be updated at runtime without recreating the pool:

```go
// Start with default config
pool := provider.NewBasePool(connConfig, 20)

// Later, adjust for high load
pool.UpdateMetricsConfig(HighPerformanceMetricsConfig())

// Or disable temporarily
pool.UpdateMetricsConfig(DisabledMetricsConfig())

// Re-enable with custom config
pool.UpdateMetricsConfig(customConfig)
```

This allows dynamic adjustment based on system conditions.

