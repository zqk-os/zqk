# Metrics Pipeline System

**Version**: 1.0.0  
**Status**: Implemented  
**Date**: 2026-01-05  
**Related**: POL-OBS-001, Field Versioning, Trait System

## Overview

The metrics pipeline system provides a trait-based, extensible framework for collecting and aggregating metrics from any object or field. By adding metric traits to fields, they automatically participate in the metrics collection and aggregation pipeline.

## Core Concepts

### Trait-Based Metrics

Metrics are enabled through traits:

1. **`base_metric_enabled_group`**: Base trait group that enables metrics collection
   - Includes: `readable`, `listable`, `filterable`, `snapable`
   - Can be applied at object or field level

2. **Metric Type Traits**: Specialized traits for different metric types
   - `scalar_metric`: Single numeric values (count, duration, size)
   - `list_metric`: Unordered collections (tags, categories)
   - `ordered_list_metric`: Ordered sequences (event sequences, time-series)
   - `status_history_metric`: Status/state transition histories

### Extensibility

The system is designed to be extensible:

- **Custom Aggregators**: Register custom aggregators via `RegisterAggregator()`
- **Custom Metric Types**: Define new metric type traits and aggregators
- **Default Aggregators**: Sensible defaults for each metric type
- **Pipeline Architecture**: Pluggable aggregator system

## Metric Types

### Scalar Metric

For single numeric values:

```yaml
duration_seconds:
    type: float
    traits:
        - readable
        - base_metric_enabled_group
        - scalar_metric
```

**Aggregations**: `sum`, `avg`, `min`, `max`, `count`

### List Metric

For unordered collections:

```yaml
tags:
    type: list
    traits:
        - readable
        - base_metric_enabled_group
        - list_metric
```

**Aggregations**: `count`, `unique_count`, `frequency`, `distribution`, `top_values`

### Ordered List Metric

For ordered sequences:

```yaml
event_sequence:
    type: list
    traits:
        - readable
        - base_metric_enabled_group
        - ordered_list_metric
```

**Aggregations**: `sequence_count`, `avg_sequence_length`, `transitions`, `common_sequences`

### Status History Metric

For status/state transitions:

```yaml
status_history:
    type: list
    traits:
        - readable
        - base_metric_enabled_group
        - status_history_metric
```

**Aggregations**: `state_distribution`, `state_durations`, `transitions`, `common_transitions`, `avg_history_length`

## Usage

### Automatic Discovery

The pipeline automatically discovers fields with metric traits:

```go
pipeline := metrics.NewMetricPipeline(storageProvider)
metricFields, err := pipeline.DiscoverMetricFields("backlog_item")
// Returns all fields with metric traits
```

### Manual Collection

Collect metrics for a specific field:

```go
config := &metrics.AggregationConfig{
    ObjectKind:  "backlog_item",
    FieldName:   "duration_seconds",
    MetricType:  "scalar_metric",
    WindowStart: time.Now().Add(-24 * time.Hour),
    WindowEnd:   time.Now(),
    Aggregations: []string{"sum", "avg", "min", "max"},
}

result, err := pipeline.CollectMetrics(ctx, config)
// Creates base_metric object with aggregated data
```

### Custom Aggregators

Register custom aggregators:

```go
pipeline.RegisterAggregator(&MyCustomAggregator{})
```

## Architecture

### Components

1. **`MetricPipeline`**: Main orchestrator
   - Discovers metric-enabled fields
   - Routes to appropriate aggregators
   - Creates base_metric objects

2. **`MetricAggregator`**: Interface for aggregators
   - `Aggregate()`: Performs aggregation
   - `GetMetricType()`: Returns metric type

3. **Default Aggregators**:
   - `ScalarMetricAggregator`: For scalar metrics
   - `ListMetricAggregator`: For list metrics
   - `OrderedListMetricAggregator`: For ordered lists
   - `StatusHistoryMetricAggregator`: For status histories

### Flow

1. **Discovery**: Pipeline discovers fields with metric traits
2. **Collection**: Aggregator queries objects and collects field values
3. **Aggregation**: Aggregator performs aggregations (sum, avg, frequency, etc.)
4. **Storage**: Creates `base_metric` object with aggregated data
5. **Async**: All operations are async per POL-OBS-001

## Integration

### With Scheduler

Scheduler jobs can trigger metric collection:

```yaml
id: SCH-018
kind: scheduler_job
job_type: metrics_collection
schedule_expression: "0 */6 * * *" # Every 6 hours
command: |
  zqk metrics collect \
    --kind backlog_item \
    --field duration_seconds \
    --window 6h
```

### With Spec Writer

Spec writer automatically tracks metrics for field operations (already implemented).

### With Snapshot System

Snapshot system can collect metrics during snapshot capture.

## Benefits

1. **Declarative**: Just add traits, metrics are collected automatically
2. **Extensible**: Easy to add new metric types and aggregators
3. **Consistent**: All metrics use `base_metric` as entry-point
4. **Async**: Non-blocking, per POL-OBS-001
5. **Flexible**: Supports custom aggregations and grouping

## Related Documentation

- [POL-OBS-001](../policies/POL-OBS-001.yaml) - Metrics Definition and Capture Policy
- [Field Versioning System](./FIELD_VERSIONING_SYSTEM.md)
- [Trait System](../testing/DYNAMIC_TRAIT_LOADING.md)

