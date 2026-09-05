# High-Volume Event Cache Proposal

**Last Verified:** 2026-08-31


## Problem Statement

High-volume events (audit_event, metrics) are stored in `.zqk/` directories and currently require expensive real-time scans:

- **17,591 audit events** require directory scans + YAML parsing for queries
- `QueryOldAuditEventsByAge()` uses `List()` which scans all files
- `Count()` operations scan directories even with filters
- Aggregation jobs timeout (30 min) because cleanup phases are too slow

## Current Architecture

- **Object ID Cache**: Caches `docs/process/` objects (excludes `.zqk/` objects)
- **High-volume events**: No caching, real-time scans via `List()` and `Count()`
- **Performance**: O(n) scans where n = 17k+ events

## Proposed Solution: High-Volume Event Cache

### Design Principles

1. **Similar to Object ID Cache**: Reuse proven architecture pattern
2. **Time-window optimized**: Support efficient queries by `created_at`
3. **Incremental updates**: Update on create/delete, not full rebuilds
4. **Selective caching**: Only cache high-volume kinds (audit_event, metrics)

### Cache Structure

```go
type HighVolumeEventCacheEntry struct {
    ID        string    `json:"id"`
    Kind      string    `json:"kind"`
    CreatedAt time.Time `json:"created_at"`
    EventType string    `json:"event_type,omitempty"` // For audit_event
    FilePath  string    `json:"file_path"`
    MTime     time.Time `json:"mtime"`
    Exists    bool      `json:"exists"`
}

type HighVolumeEventCache struct {
    mu       sync.RWMutex
    cache    map[string]*HighVolumeEventCacheEntry // id -> entry
    // Time-indexed for efficient queries
    byTime   []*HighVolumeEventCacheEntry           // Sorted by CreatedAt
    metadata *HighVolumeEventCacheMetadata
}
```

### Key Features

1. **Time-indexed queries**: Maintain sorted slice by `created_at` for O(log n) queries
2. **Incremental updates**: Add on create, remove on delete/aggregation
3. **Fast Count()**: Count cache entries matching filters (no file scans)
4. **Fast List()**: Filter cache entries, then read only matching files
5. **Selective kinds**: Only cache `audit_event`, `*_metric` kinds

### Benefits

- **Performance**: O(log n) queries instead of O(n) scans
- **Timeout prevention**: Pre-computed data means faster aggregation jobs
- **Reduced I/O**: Only read files for matching entries, not all files
- **Consistency**: Same pattern as Object ID Cache (proven architecture)

### Implementation Approach

1. **Create `HighVolumeEventCache`** similar to `ObjectIDCache`
2. **Build incrementally**: Update on audit event create/delete
3. **Query optimization**: Use time-indexed slice for window queries
4. **Integration**: Update `QueryOldAuditEventsByAge()` to use cache
5. **Storage**: `.zqk/cache/high-volume-events-cache.json`

### Migration Strategy

1. **Phase 1**: Build cache on-demand (when aggregation job runs)
2. **Phase 2**: Maintain cache incrementally (update on create/delete)
3. **Phase 3**: Use cache for all queries (fallback to scan if cache stale)

### Performance Impact

**Before (current)**:
- `QueryOldAuditEventsByAge()`: ~5-30 seconds (scans 17k files)
- `Count(audit_event)`: ~2-5 seconds (directory scan)
- Aggregation job cleanup: Times out at 30 minutes

**After (with cache)**:
- `QueryOldAuditEventsByAge()`: ~10-100ms (binary search + filter)
- `Count(audit_event)`: ~1-5ms (count cache entries)
- Aggregation job cleanup: Completes in <5 minutes

### Considerations

- **Cache invalidation**: Update on create/delete/aggregation
- **Stale cache handling**: Fallback to scan if cache too old
- **Memory usage**: ~1KB per entry × 17k = ~17MB (acceptable)
- **Build time**: Initial build ~30 seconds (one-time cost)
