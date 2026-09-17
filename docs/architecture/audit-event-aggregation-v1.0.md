> [!WARNING]
> **ARCHIVED DOCUMENT**: The primary commands referenced in this architectural document have been pruned from the `zqk` CLI.

# Audit Event Aggregation Strategy v1.0

**Last Verified:** 2026-08-31


**Date:** 2025-12-25  
**Status:** ✅ Implemented  
**Related:** BLI-633, REQ-032  
**Version:** 1.0.0

## Problem Statement

High-volume audit events (e.g., cache operations, frequent integrity checks) can generate thousands of events per day, leading to:
- Storage bloat
- Performance degradation when querying audit logs
- Difficulty identifying patterns in large volumes of low-severity events
- Compliance reporting overhead

## Solution: Aggregation Strategy

Aggregate low-severity, high-volume events into summary events while preserving individual events for high-severity operations.

## Aggregation Rules

### 1. Event Eligibility

**Aggregatable Events**:
- Severity: `low` only
- Event types: `cache_invalidation`, `cache_update` (configurable)
- Volume threshold: > N events per time window (default: 10 per hour)
- Pattern: Same event type, same target kind (if applicable)

**Non-Aggregatable Events** (always preserved individually):
- Severity: `medium`, `high`, `critical`
- Event types: `hash_mismatch_fix`, `security_alert`, `permission_change`
- Events with `target_id` (specific object references)
- Events with custom `metadata` requiring individual tracking

### 2. Aggregation Windows

**Time Windows**:
- **Hourly**: Default for cache operations (e.g., "100 cache updates in the last hour")
- **Daily**: For lower-frequency but still high-volume events
- **Custom**: Configurable per event type

**Grouping Keys**:
- `event_type` + `target_kind` (if applicable)
- `event_type` + `severity`
- `event_type` + `recovery_method` (if applicable)

### 3. Aggregation Event Structure

```yaml
id: AUD-AGG-{timestamp}-{hash}
kind: audit_event
schema_version: "2.0.0"
event_type: aggregated_summary
operation: "Aggregated 100 cache_update events for backlog_item"
target_kind: backlog_item
severity: low
status: completed
aggregation_window: "2025-12-25T14:00:00Z to 2025-12-25T15:00:00Z"
aggregated_count: 100
aggregated_events:
  - event_type: cache_update
    count: 100
    first_occurrence: "2025-12-25T14:05:23Z"
    last_occurrence: "2025-12-25T14:58:47Z"
    target_kind: backlog_item
metadata:
  aggregation_strategy: "hourly_by_kind"
  original_event_types: ["cache_update"]
  aggregation_threshold: 10
```

## Implementation Strategy

### ✅ Implemented: Hybrid Approach

**Status**: Fully implemented in `pkg/storage/audit_event_buffer.go`

**Approach**: 
- Write high-severity events immediately (no aggregation)
- Buffer low-severity events in memory
- Flush aggregated summaries periodically
- Optionally keep sample individual events for forensics

**Components**:
1. **AuditEventBuffer** (`pkg/storage/audit_event_buffer.go`)
   - In-memory buffer for aggregatable events
   - Thread-safe with mutex protection
   - Automatic periodic flushing
   - Threshold-based immediate flushing

2. **AggregationGroup**
   - Groups events by `event_type` + `target_kind` (configurable)
   - Tracks count, first/last seen timestamps
   - Preserves sample events for forensics

3. **Configuration** (`pkg/storage/audit_aggregation_config.go`)
   - Loads from `.zqk/config.yaml`
   - Supports configurable rules, thresholds, window sizes
   - Falls back to sensible defaults

**Flush Triggers**:
1. Time-based: Every hour (or configurable window via `window_size`)
2. Count-based: When buffer exceeds threshold (per rule)
3. Shutdown: On process exit (via `Shutdown()`)
4. Manual: Via `zqk system audit-buffer flush` (PRUNED) command

**Integration**:
- Automatically integrated into `createCacheAuditEvent()` function
- High-severity events bypass buffer and write immediately
- Low-severity events are buffered and aggregated

## Configuration

### ✅ Implemented: Aggregation Rules Configuration

Configuration is loaded from `.zqk/config.yaml`:

```yaml
# .zqk/config.yaml
audit:
  aggregation:
    enabled: true
    window_size: "1h"  # hourly aggregation window
    threshold: 10      # default threshold (can be overridden per rule)
    rules:
      - event_types: ["cache_invalidation", "cache_update"]
        severities: ["low"]
        group_by: ["event_type", "target_kind"]
        window: "1h"
        threshold: 10
        preserve_samples: 5  # Keep N individual events for forensics
      - event_types: ["cache_bulk_invalidation"]
        severities: ["low", "medium"]
        group_by: ["event_type", "target_kind"]
        window: "1h"
        threshold: 5
        preserve_samples: 3
```

**Configuration Loading**:
- Loaded automatically when buffer is initialized
- Falls back to defaults if config file doesn't exist
- Validates window size format (supports: `1h`, `24h`, `7d`, `1w`)
- Defaults applied for missing fields

**CLI Management**:
```bash
# View buffer statistics
zqk system audit-buffer stats (PRUNED)

# Flush buffer immediately
zqk system audit-buffer flush (PRUNED)

# Enable/disable buffer
zqk system audit-buffer enable (PRUNED)
zqk system audit-buffer disable (PRUNED)
```

### Default Rules

If no rules are specified in config, the following defaults are used:

```yaml
rules:
  - event_types: ["cache_invalidation", "cache_update"]
    severities: ["low"]
    group_by: ["event_type", "target_kind"]
    window: "1h"
    threshold: 10
    preserve_samples: 5
  - event_types: ["cache_bulk_invalidation"]
    severities: ["low", "medium"]
    group_by: ["event_type", "target_kind"]
    window: "1h"
    threshold: 5
    preserve_samples: 3
```

**Note**: Per-event-type configuration is handled via rules. Events not matching any rule are written immediately (not aggregated).

## Aggregation Event Types

### ✅ Implemented: Event Type `aggregated_summary`

**Status**: Already exists in `audit_event.yaml` spec

The `aggregated_summary` event type is used for aggregated events:

```yaml
event_type: aggregated_summary
aggregation_window: "2025-12-25T14:00:00Z to 2025-12-25T15:00:00Z"
aggregated_count: 100
aggregated_events:
  - event_type: cache_update
    count: 100
    first_occurrence: "2025-12-25T14:05:23Z"
    last_occurrence: "2025-12-25T14:58:47Z"
    target_kind: backlog_item
preserved_samples: []  # Optional: sample events for forensics
```

**Fields** (from `audit_event.yaml` spec):
- `aggregation_window`: Time range covered (ISO 8601 interval format)
- `aggregated_count`: Total number of events aggregated
- `aggregated_events`: Array of event summaries with counts and time ranges
- `preserved_samples`: Array of sample individual events (if `preserve_samples > 0`)

## Storage Strategy

### ✅ Implemented: Aggregated Event Storage

**Location**: Same monthly bucketing as individual events
- `.zqk/process/audit/{YYYY-MM}/AUD-{sequence}.yaml`

**Naming**: 
- Uses standard audit event ID format: `AUD-{sequence}`
- Sequence numbers are assigned per month directory
- Hash registry tracks file integrity

**Note**: Aggregated events are stored as regular `audit_event` objects with `event_type: aggregated_summary`. They follow the same bucketing and storage patterns as individual events.

### Sample Preservation

If `preserve_samples: N` is configured:
- Keep N individual events from the aggregation window
- Store in same directory with normal naming
- Link from aggregated event via `sample_event_ids`

## Query and Reporting

### ✅ Implemented: Querying Aggregated Events

**Individual Events**: Query normally (high-severity events are written immediately)
**Aggregated Events**: Query by `event_type: aggregated_summary`

**Example Queries**:
```bash
# Find all aggregated cache operations
zqk object list audit_event --filter "event_type=aggregated_summary"

# Find aggregated events for specific target kind
zqk object list audit_event --filter "event_type=aggregated_summary" --filter "target_kind=backlog_item"

# Find individual high-severity events (not aggregated)
zqk object list audit_event --filter "severity=high,medium,critical"

# View buffer statistics
zqk system audit-buffer stats (PRUNED)
```

### Reporting

**Summary Reports**:
- Total events (individual + aggregated counts)
- Aggregation efficiency (events aggregated / total events)
- Patterns in aggregated events (peak times, common operations)

## Migration Strategy

### ✅ Implemented: Backward Compatible

**Existing Events**:
- No changes to existing audit events
- Aggregation only applies to new events created after implementation
- Historical events remain unchanged

**Rollback**:
- Aggregation can be disabled via config: `audit.aggregation.enabled: false`
- Or via CLI: `zqk system audit-buffer disable` (PRUNED)
- Individual events can be extracted from aggregated summaries (if `preserved_samples` configured)
- Aggregated events are immutable (like all audit events) - cannot be "exploded" back

**Post-Processing Aggregation**:
- The existing `AuditAggregationService` (`pkg/storage/audit_aggregation.go`) provides post-processing aggregation
- Use `zqk system aggregate-audit` (PRUNED) command to aggregate historical events into metrics
- This creates `audit_aggregation_metric` objects (different from `aggregated_summary` events)

## Performance Considerations

### ✅ Implemented: Performance Optimizations

**Memory Usage**:
- Buffer size limited by window size and threshold
- Typical: < 1MB for hourly windows with 1000s of events
- Flush on threshold to prevent unbounded growth
- Thread-safe with mutex protection

**I/O Impact**:
- Reduced write operations (1 aggregated event vs N individual events)
- Reduced storage (compressed representation with ID ranges)
- Faster queries (fewer files to scan)
- High-severity events still written immediately (no latency impact)

**Latency**:
- Aggregation adds minimal latency (< 1ms per event)
- Flush operations async (don't block event creation)
- Buffer operations are non-blocking
- High-severity events bypass buffer entirely (zero latency)

## Security and Compliance

### Audit Trail Integrity

- Aggregated events are immutable (like individual events)
- Sample preservation ensures forensic capability
- Aggregation metadata includes all grouping keys

### Compliance Reporting

- Aggregated events satisfy compliance requirements (counts, time ranges)
- Individual events available for detailed forensics
- Can generate "exploded" reports showing estimated individual events

## Future Enhancements

1. **Adaptive Aggregation**: Adjust thresholds based on volume patterns
2. **Multi-Level Aggregation**: Hourly → Daily → Weekly summaries
3. **Event Correlation**: Group related events (e.g., cache update + integrity check)
4. **Anomaly Detection**: Flag unusual patterns in aggregated data
5. **Compression**: Further compress aggregated event storage

