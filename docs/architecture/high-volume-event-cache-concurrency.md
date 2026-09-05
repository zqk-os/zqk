# High-Volume Event Cache Concurrency Pattern

**Last Verified:** 2026-08-31


## Overview

The `HighVolumeEventCache` follows established concurrency patterns from `ObjectIDCache` and the `pkg/concurrency` package. This document describes the specific concurrency patterns used and why they were chosen.

## Established Patterns Followed

### 1. Timeout Wrappers for All Mutex Operations

**Pattern**: All mutex operations use `concurrency.WithLockTimeout` or `concurrency.WithRLockTimeout` with:
- Context with timeout (5 seconds for cache operations)
- Logger adapter for observability
- Operation name for debugging

**Example**:
```go
_ = concurrency.WithRLockTimeout(
    &c.mu,
    pkgctx.NewSystemContext(),
    nil,
    logging.NewLockLoggerAdapter(logger),
    "high_volume_cache_get",
    func() error {
        entry, ok = c.cache[id]
        exists = ok && entry != nil && entry.Exists
        return nil
    },
)
```

**Rationale**: Prevents deadlocks, provides observability, and ensures operations complete or timeout deterministically.

### 2. Minimize Lock Hold Time (Especially During I/O)

**Pattern**: Release locks before I/O operations. Use multiple lock acquisitions if needed:
1. RLock to read data
2. Release lock
3. Perform I/O
4. Lock to update metadata
5. Release lock
6. Perform I/O

**Example** (`SaveCache`):
```go
// Step 1: Read count with RLock (minimal lock hold)
_ = concurrency.WithRLockTimeout(...)
    entryCount = len(c.cache)

// Step 2: I/O operations WITHOUT lock
os.MkdirAll(cacheDir, 0755)

// Step 3: Prepare cache data with Lock (update metadata)
_ = concurrency.WithLockTimeout(...)
    // Copy cache map and metadata for serialization
    cacheData.Entries = make(map[string]*HighVolumeEventCacheEntry, len(c.cache))
    for k, v := range c.cache {
        cacheData.Entries[k] = v
    }

// Step 4: I/O operations WITHOUT lock (JSON marshaling and file write)
json.MarshalIndent(cacheData, "", "  ")
os.WriteFile(cachePath, data, 0644)
```

**Rationale**: File I/O can be slow and unpredictable. Holding locks during I/O blocks other goroutines unnecessarily and increases deadlock risk.

### 3. Read-Write Lock Separation

**Pattern**: Use `RWMutex` with appropriate lock type:
- `RLock()` for read-only operations (Get, Count, Query)
- `Lock()` for write operations (Set, Invalidate, BuildCache)

**Rationale**: Allows concurrent reads while serializing writes, improving performance for read-heavy workloads.

### 4. Copy Data Out Before I/O

**Pattern**: When preparing data for serialization, copy the cache map under lock, then release lock before marshaling/writing.

**Rationale**: Prevents holding lock during potentially slow JSON marshaling and file writes.

## Cache-Specific Patterns

### Time-Indexed Query Pattern

**Pattern**: Maintain a sorted slice (`byTime`) alongside the map for O(log n) time-window queries.

**Concurrency**: 
- Rebuild `byTime` under write lock whenever cache is modified
- Read `byTime` under read lock for queries
- Binary search is fast enough that lock hold time is minimal

**Example**:
```go
func (c *HighVolumeEventCache) QueryByTimeWindow(startTime, endTime time.Time, limit int) []string {
    var result []string
    _ = concurrency.WithRLockTimeout(
        &c.mu,
        pkgctx.NewSystemContext(),
        nil,
        logging.NewLockLoggerAdapter(logger),
        "high_volume_cache_query_time_window",
        func() error {
            // Binary search on byTime slice (fast, minimal lock hold)
            startIdx := sort.Search(len(c.byTime), ...)
            endIdx := sort.Search(len(c.byTime), ...)
            // Extract IDs (copy out before releasing lock)
            result = make([]string, 0, endIdx-startIdx)
            for i := startIdx; i < endIdx && ...; i++ {
                result = append(result, c.byTime[i].ID)
            }
            return nil
        },
    )
    return result
}
```

**Rationale**: Binary search is O(log n) and very fast, so lock hold time is minimal even for large caches.

### Incremental Update Pattern

**Pattern**: Update cache incrementally on create/delete rather than rebuilding.

**Concurrency**:
- `Set()`: Acquire write lock, update map, rebuild time index, release lock
- `Invalidate()`: Acquire write lock, delete from map, rebuild time index, release lock

**Rationale**: Incremental updates are faster than full rebuilds and maintain cache consistency.

### Build Cache Pattern

**Pattern**: Build cache in phases:
1. Clear cache under lock
2. Release lock
3. Query storage (I/O, no lock)
4. Acquire lock, add entries in batches
5. Release lock between batches
6. Final lock to rebuild index and save

**Rationale**: Allows context cancellation checks between batches and minimizes lock contention during long-running builds.

## Lock Ordering

The `HighVolumeEventCache` uses a single `sync.RWMutex` (`c.mu`), so there are no lock ordering concerns within the cache itself.

**When used with other locks**: Follow the global lock order defined in `docs/process/architecture/LOCK_ORDERING.md`. The cache lock should be acquired after any storage locks if both are needed.

## Performance Considerations

1. **Read-heavy workload**: RLock allows concurrent reads, improving throughput
2. **Time-indexed queries**: Binary search is fast, minimal lock hold time
3. **Incremental updates**: Avoid full rebuilds, maintain consistency
4. **Copy before I/O**: Never hold locks during file operations

## Testing

All cache operations are tested with concurrent access patterns:
- Concurrent reads (should not block each other)
- Concurrent read/write (should serialize writes)
- Concurrent writes (should serialize completely)

## Related Documentation

- `pkg/concurrency/README.md`: Standard concurrency patterns
- `docs/process/architecture/LOCK_ORDERING.md`: Global lock ordering
- `cmd/zqk/system/check_cache.go`: Reference implementation (`ObjectIDCache`)
