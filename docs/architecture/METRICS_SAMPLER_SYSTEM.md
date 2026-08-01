# Metrics Sampler System

**Version**: 1.0.0  
**Status**: Implemented  
**Date**: 2026-01-05  
**Related**: POL-OBS-001, Metrics Pipeline System

## Overview

The metrics sampler system provides in-memory batching for high-frequency, low-value events (like audit events). Instead of creating a metric object for every event, events are batched in memory and aggregated when the batch is full or a timeout is reached.

## Problem Statement

High-frequency events (e.g., audit events, change journal entries) can create thousands of metric objects per hour, overwhelming the scheduler and storage system. These events are often not useful individually but provide value when aggregated.

## Solution

The sampler system:
1. **Batches events in memory** - Collects events before creating metric objects
2. **Configurable batch sizes** - Default 50-100 events per batch (configurable)
3. **Per-object batching** - Can batch by object ID (default) or globally
4. **Automatic flushing** - Flushes partial batches after a timeout (default 5 minutes)
5. **Opt-in/opt-out** - Configurable per object kind

## Architecture

### Components

1. **`Sampler`**: Manages in-memory batching for a specific metric type
   - Buffers events in memory
   - Flushes when batch is full or timeout reached
   - Creates `base_metric` objects with aggregated data

2. **`SamplerRegistry`**: Manages multiple samplers
   - Registers samplers for different object kinds/metric types
   - Provides lookup and management
   - Handles initialization and cleanup

3. **`SamplerConfig`**: Configuration for a sampler
   - `enabled`: Enable/disable sampling
   - `batch_size`: Number of events per batch (default: 50)
   - `max_batch_size`: Maximum batch size (safety limit, default: 1000)
   - `flush_interval`: Time to wait before flushing partial batch (default: 5m)
   - `group_by_object_id`: Batch per object ID (true) or globally (false)

## Configuration

### Configuration File

Located at: `docs/architecture/_internal/configs/metrics_sampler_config.yaml`

```yaml
defaults:
    enabled: true
    batch_size: 50
    max_batch_size: 1000
    flush_interval: "5m"
    group_by_object_id: true

samplers:
    - object_kind: audit_event
      metric_type: system
      enabled: true
      batch_size: 100
      flush_interval: "5m"
      group_by_object_id: true

opt_out:
    - base_metric  # Metrics themselves should not be sampled
    - audit_aggregation_metric

opt_in: []  # Empty = all enabled by default
```

### Opt-In vs Opt-Out

- **Opt-out**: List of object kinds that should never use sampling
- **Opt-in**: List of object kinds that require explicit opt-in (if empty, all are enabled by default)

## Usage

### Automatic Sampling

When an event is created, the metrics pipeline checks if sampling is enabled:

```go
pipeline := metrics.NewMetricPipeline(storageProvider)

// Event is automatically sampled if sampler is configured
event := map[string]interface{}{
    "kind": "audit_event",
    "event_type": "object_created",
    // ... other fields
}

flushed, err := pipeline.Sample(event)
// Returns true if batch was flushed (metric created)
```

### Manual Configuration

```go
config := &metrics.SamplerConfig{
    Enabled:         true,
    BatchSize:       100,
    MaxBatchSize:    1000,
    FlushInterval:   5 * time.Minute,
    GroupByObjectID: true,
    MetricType:      "system",
    ObjectKind:      "audit_event",
}

registry := pipeline.GetSamplerRegistry()
registry.RegisterSampler(config)
```

### Loading Configuration from File

```go
config, err := metrics.LoadSamplerConfig(configPath)
if err != nil {
    return err
}

registry := pipeline.GetSamplerRegistry()
if err := metrics.ApplySamplerConfig(registry, config, projectRoot); err != nil {
    return err
}
```

## Benefits

1. **Reduced Overhead**: Instead of 1000 metric objects, create 10-20 (with batch_size=50-100)
2. **Better Aggregation**: Events are aggregated before storage
3. **Configurable**: Per-object-kind configuration
4. **Automatic**: No code changes needed, just configuration
5. **Safe**: Automatic flushing prevents data loss

## Example: Audit Events

**Before (without sampling)**:
- 1000 audit events → 1000 `base_metric` objects
- High scheduler load
- High storage overhead

**After (with sampling, batch_size=100)**:
- 1000 audit events → 10 `base_metric` objects
- 90% reduction in metric objects
- Significantly reduced scheduler load
- Better aggregation (event_type_counts, severity_counts)

## Integration Points

### With Audit System

When audit events are created, they can be automatically sampled:

```go
// In audit event creation
event := createAuditEvent(...)
pipeline.Sample(event)  // Automatically batched if sampler configured
```

### With Scheduler

Scheduler can periodically flush all samplers:

```yaml
id: SCH-019
kind: scheduler_job
job_type: metrics_flush
schedule_expression: "*/15 * * * *"  # Every 15 minutes
command: |
  zqk metrics flush-samplers
```

### With System Shutdown

On system shutdown, all samplers should be flushed:

```go
registry := pipeline.GetSamplerRegistry()
registry.StopAll()  // Flushes all pending batches
```

## Performance Considerations

1. **Memory Usage**: Each batch holds events in memory until flushed
   - Typical: 50-100 events per batch
   - With 1000 active batches: ~50,000-100,000 events in memory
   - Mitigation: Automatic flushing on timeout

2. **Batch Size**: Larger batches = fewer metric objects but more memory
   - Recommended: 50-200 for high-frequency events
   - Maximum: 1000 (safety limit)

3. **Flush Interval**: Shorter intervals = less memory but more frequent writes
   - Recommended: 5-10 minutes
   - Minimum: 1 minute (to avoid excessive writes)

## Related Documentation

- [Metrics Pipeline System](./METRICS_PIPELINE_SYSTEM.md)
- [POL-OBS-001](../policies/POL-OBS-001.yaml) - Metrics Definition and Capture Policy

