package storage

// This file demonstrates how to use the metrics framework to create new metrics collectors
// It's a reference implementation showing the pattern

/*
Example: Creating a new metrics collector using the framework

1. Define your metrics struct with atomic counters:
```go
type MyMetrics struct {
    Operations int64
    Failures   int64
    TotalTime  int64
    MaxTime    int64
}

var globalMyMetrics = &MyMetrics{}

func GetMyMetrics() *MyMetrics {
    return globalMyMetrics
}

func ResetMyMetrics() {
    globalMyMetrics = &MyMetrics{}
}

func (m *MyMetrics) RecordOperation(duration time.Duration, err error) {
    atomic.AddInt64(&m.Operations, 1)
    if err != nil {
        atomic.AddInt64(&m.Failures, 1)
        return
    }
    durationNs := int64(duration)
    atomic.AddInt64(&m.TotalTime, durationNs)
    // Update max time...
}

func (m *MyMetrics) GetSnapshot() MyMetricsSnapshot {
    return MyMetricsSnapshot{
        Operations: atomic.LoadInt64(&m.Operations),
        Failures:   atomic.LoadInt64(&m.Failures),
        // ...
    }
}
```

2. Define your snapshot type implementing MetricsSnapshot:
```go
type MyMetricsSnapshot struct {
    Operations int64
    Failures   int64
    AvgTime    time.Duration
    MaxTime    time.Duration
}

func (s MyMetricsSnapshot) GetTotalOperations() int64 {
    return s.Operations
}
```

3. Create a collector using BaseMetricsCollector:
```go
func NewMyMetricsCollector(storage ObjectStorageProvider) MetricsCollector {
    config := MetricBuilderConfig{
        Kind:        "my_metric",
        TitlePrefix: "My Metrics",
        MetricType:  "performance",
        Source:      "my_system",
        Tags:        []string{"my", "metrics", "performance"},
        IDPrefix:    "MYM",
        GetSnapshot: func() MetricsSnapshot {
            return GetMyMetrics().GetSnapshot()
        },
        ResetMetrics: func() {
            ResetMyMetrics()
        },
        BuildMetricObject: func(builder any, snapshot MetricsSnapshot, windowStart, windowEnd time.Time) error {
            mySnapshot := snapshot.(MyMetricsSnapshot)
            // Use type assertion to set metric-specific fields
            // For now, you'll need to add your builder type to createMetricBuilder()
            // and setCommonMetricFields() in metrics_framework.go
            // Or use SetField() for generic fields
            return nil
        },
    }
    return NewBaseMetricsCollector(storage, config)
}
```

4. Create async collector (optional, for high-volume scenarios):
```go
func NewMyMetricsAsyncCollector(storage ObjectStorageProvider) *AsyncMetricsCollector {
    collector := NewMyMetricsCollector(storage)
    return NewAsyncMetricsCollector(collector, 100) // Buffer 100 batches
}
```

5. Use in your code:
```go
// Record metrics (atomic, zero overhead)
metrics := GetMyMetrics()
metrics.RecordOperation(duration, err)

// Periodically collect (creates metric object)
collector := NewMyMetricsCollector(storage)
metricID, err := collector.CollectAndReset(ctx, secCtx, windowStart, windowEnd)

// Or use async collector (non-blocking)
asyncCollector := NewMyMetricsAsyncCollector(storage)
asyncCollector.CollectMetricsAsync(ctx, secCtx, windowStart, windowEnd, callback)
```

Note: The framework handles:
- ID generation (CAS-aware)
- Common field setting (title, timestamps, metadata)
- Builder creation and instance building
- Async batching and background processing
- Error handling

You only need to:
- Define your metrics struct with atomic counters
- Implement GetSnapshot() and GetTotalOperations()
- Provide BuildMetricObject() function to set metric-specific fields
*/
