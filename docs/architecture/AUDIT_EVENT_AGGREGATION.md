> [!WARNING]
> **ARCHIVED DOCUMENT**: The primary commands referenced in this architectural document have been pruned from the `zqk` CLI.

# Audit Event Aggregation

**Last Verified:** 2026-08-31


## Overview

The audit event aggregation system compresses audit events into summary events for efficient storage and analysis while preserving key information for compliance and reporting. This system uses a **Hybrid Approach** that buffers low-severity events in memory and flushes aggregated summaries periodically.

## Architecture

### Components

1. **AuditEventBuffer** (`pkg/storage/audit_event_buffer.go`) ✅ **Implemented**
   - In-memory buffer for low-severity events
   - Automatic periodic flushing (configurable window)
   - Threshold-based immediate flushing
   - Thread-safe with mutex protection
   - Creates `aggregated_summary` audit events

2. **AuditAggregationService** (`pkg/storage/audit_aggregation.go`)
   - Post-processing aggregation for historical events
   - Works with both file and graph backends
   - Compresses event IDs into ranges for space efficiency
   - Creates `audit_aggregation_metric` objects

3. **Configuration** (`pkg/storage/audit_aggregation_config.go`) ✅ **Implemented**
   - Loads from `.zqk/config.yaml`
   - Configurable rules, thresholds, window sizes
   - Sensible defaults

4. **CLI Commands** (`cmd/zqk/system/audit_buffer.go`) ✅ **Implemented**
   - `zqk system audit-buffer stats` (PRUNED) - View buffer statistics
   - `zqk system audit-buffer flush` (PRUNED) - Flush buffer immediately
   - `zqk system audit-buffer enable (PRUNED)/disable` - Enable/disable buffer

## Hybrid Approach (In-Memory Buffer)

### Event Flow

1. **Event Creation**: When `createCacheAuditEvent()` is called
   - High-severity events → Written immediately (bypass buffer)
   - Low-severity events → Added to buffer if matching aggregation rules

2. **Buffering**: Events are grouped by:
   - `event_type` + `target_kind` (configurable via `group_by`)
   - Same severity level
   - Within time window

3. **Flushing**: Aggregated summaries are created when:
   - Time window expires (periodic flush)
   - Threshold exceeded (immediate flush for that group)
   - Manual flush via CLI
   - Process shutdown

4. **Aggregated Event**: Creates `audit_event` with:
   - `event_type: aggregated_summary`
   - `aggregated_count`: Total events aggregated
   - `aggregation_window`: Time range covered
   - `aggregated_events`: Breakdown by event type/kind
   - `preserved_samples`: Sample events (if configured)

### Post-Processing Aggregation

The `AuditAggregationService` provides post-processing aggregation for historical events:

1. **pending** → Events waiting to be aggregated
2. **aggregating** → Currently being processed
3. **aggregated** → Successfully aggregated, ready for cleanup
4. **archived** → Events archived after aggregation
5. **deleted** → Events deleted after archival period

## Graph Backend Support

The graph backend fully supports audit event aggregation:

### Query Capabilities

- **Time Range Queries**: Uses `List` with `$gte` and `$lte` operators on `created_at`
- **Status Filtering**: Filters by `status = "completed"`
- **Bulk Operations**: Uses `BulkUpdate` and `BulkDelete` for efficient batch processing
- **Transactions**: All operations are atomic via transactions

### Example Cypher Query (for reference)

```cypher
MATCH (n:AuditEvent:Entity)
WHERE n.created_at >= $windowStart 
  AND n.created_at <= $windowEnd 
  AND n.status = 'completed'
RETURN n
ORDER BY n.created_at
```

### Performance Considerations

- **Indexing**: Graph backend should index `created_at` and `status` for efficient queries
- **Batch Size**: Large aggregations are processed in batches
- **Transactions**: Bulk operations use transactions for atomicity

## ID Range Compression

Event IDs are compressed into ranges to save space:

- **Individual IDs**: `["AUD-1", "AUD-5", "AUD-10"]`
- **Ranges**: `["AUD-100..AUD-199"]` (represents AUD-100 through AUD-199)
- **Mixed**: `["AUD-1", "AUD-100..AUD-199", "AUD-250"]`

Ranges are used when 3+ consecutive IDs are found. This can significantly reduce storage for large aggregations.

## Usage

```go
// Create aggregation service
service := storage.NewAuditAggregationService(storageProvider)

// Aggregate events in a time window
windowStart := time.Now().Add(-24 * time.Hour)
windowEnd := time.Now()
result, err := service.AggregateAuditEvents(ctx, secCtx, storageCtx, windowStart, windowEnd)

// Cleanup aggregated events (archive or delete)
updatedCount, err := service.CleanupAggregatedEvents(ctx, secCtx, result.EventsProcessed, true)
```

## Configuration

### ✅ In-Memory Buffer Configuration

Configure via `.zqk/config.yaml`:

```yaml
audit:
  aggregation:
    enabled: true
    window_size: "1h"  # Flush window (1h, 24h, 7d, 1w)
    threshold: 10      # Default threshold
    rules:
      - event_types: ["cache_invalidation", "cache_update"]
        severities: ["low"]
        group_by: ["event_type", "target_kind"]
        window: "1h"
        threshold: 10
        preserve_samples: 5
```

**CLI Management**:
```bash
# View buffer statistics
zqk system audit-buffer stats (PRUNED)

# Flush buffer immediately
zqk system audit-buffer flush (PRUNED)

# Enable/disable
zqk system audit-buffer enable (PRUNED)
zqk system audit-buffer disable (PRUNED)
```

### Post-Processing Aggregation

Configure via scheduler job (for historical aggregation):

```yaml
kind: scheduler_job
job_type: audit_event_aggregation
schedule_expression: "0 2 * * *"  # Daily at 2 AM
enabled: true
```

## Benefits

1. **Storage Efficiency**: Compresses thousands of events into single aggregated_summary events
2. **Query Performance**: Faster queries on aggregated events vs individual events
3. **Compliance**: Preserves key information (counts, types, time windows, samples)
4. **Traceability**: Aggregated events include time ranges and event type breakdowns
5. **Backend Agnostic**: Works identically with file and graph backends
6. **Zero Latency for High-Severity**: Critical events written immediately
7. **Configurable**: Rules, thresholds, and windows configurable per project
8. **Automatic**: No external schedulers or scripts required

## Implementation Status

✅ **Completed (BLI-633)**:
- In-memory aggregation buffer
- Configuration loading from `.zqk/config.yaml`
- CLI commands for buffer management
- Integration with audit event creation
- Comprehensive test coverage
- Documentation

**Related Documentation**:
- [Audit Event Aggregation Strategy v1.0](./audit-event-aggregation-v1.0.md) - Detailed design document
- [Object Storage Bucketing](./object-storage-bucketing-v1.0.md) - Storage organization

