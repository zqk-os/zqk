# File Lock Strategy and Transaction Coordinator Architecture

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2026-01-14  
**Status**: Active  
**Purpose**: Document the file lock strategy pattern and transaction coordinator for transactional file operations

## Overview

This architecture provides:
1. **Reusable File Lock Strategies** - Similar to bucketing strategies, provides a uniform interface for file locking with different cleanup behaviors
2. **Enhanced Metrics Collection** - Tracks lock usage by strategy, resource type, and system impact
3. **Transaction Coordinator** - Provides graph-database-like transactional safety for multi-file operations
4. **File System Adapter** - Acts as a micro-database interface for file-based storage

## File Lock Strategy Pattern

### Interface

```go
type FileLockStrategy interface {
    AcquireLock(lockPath string, timeout time.Duration) (LockHandle, error)
    CleanupStaleLocks(lockPath string) error
    Name() string
}
```

### Strategy Implementations

#### AutoCleanupStrategy
- **Purpose**: Automatically removes lock files after release
- **Use Case**: Index files, cache files (prevents git confusion)
- **Behavior**: 
  - Cleans up stale locks before acquiring
  - Removes lock file after release
  - Prevents lock file accumulation

#### PersistentLockStrategy
- **Purpose**: Keeps lock files for monitoring/debugging
- **Use Case**: Debugging lock contention, monitoring lock usage
- **Behavior**:
  - Cleans up stale locks
  - Updates modification time on release
  - Keeps lock files for analysis

### Benefits

1. **Reusability**: Same pattern used across index files, cache files, object files
2. **Testability**: Strategies can be mocked and tested independently
3. **Flexibility**: Easy to add new strategies (timeout-based, retry-based, etc.)
4. **Consistency**: Follows same pattern as bucketing strategies
5. **Maintainability**: Lock management logic centralized

## Enhanced Metrics Collection

### FileLockStrategyMetrics

Tracks detailed observability data:

- **Strategy-specific metrics**: Per-strategy acquisition/failure/timeout counts
- **Resource type metrics**: Breakdown by resource type (index, cache, object_file, system_object, public_object)
- **System object metrics**: Tracks impact on system vs public objects
- **Lock duration metrics**: Total, max, and average lock durations
- **Stale lock cleanup metrics**: Number cleaned and time spent
- **Contention metrics**: Contention count per resource type

### Usage

```go
metrics := GetFileLockStrategyMetrics()
snapshot := metrics.GetSnapshot()

// Get resource type breakdown
breakdown := snapshot.GetResourceTypeBreakdown()
// Returns: map[string]float64{"index": 45.2, "cache": 30.1, ...}

// Get contention rate
rate := snapshot.GetContentionRate()
// Returns: 0.05 (5% contention rate)
```

## Transaction Coordinator

### Architecture

The `FileTransactionCoordinator` provides transactional safety using:

1. **Two-Phase Locking Protocol**:
   - Phase 1: Acquire all locks (ordered to prevent deadlocks)
   - Phase 2: Apply operations atomically
   - Phase 3: Release locks

2. **Ordered Locking**: Locks files in sorted path order to prevent deadlocks

3. **Automatic Rollback**: Releases all locks on error

### FileSystemAdapter

Provides a micro-database interface:

```go
adapter := NewFileSystemAdapter(projectRoot, storage)

// Begin transaction
tx := adapter.BeginTransaction(ctx)

// Add operations
tx.Create(ctx, secCtx, obj1)
tx.Update(ctx, secCtx, "OBJ-123", updates)
tx.Delete(ctx, secCtx, "OBJ-456")

// Commit atomically (all or nothing)
err := tx.Commit(ctx, secCtx)
```

### Transaction Safety Guarantees

1. **Atomicity**: All operations in transaction succeed or all fail
2. **Isolation**: Locks prevent concurrent modifications
3. **Consistency**: Ordered locking prevents deadlocks
4. **Durability**: Operations are applied to disk before commit completes

## Integration Points

### CAS Index Files

The `saveMappings()` function in `content_addressable_storage.go` now uses:

```go
lockStrategy := NewAutoCleanupStrategy()
lockHandle, err := lockStrategy.AcquireLock(lockPath, 5*time.Second)
defer lockHandle.Release()
```

### Future Integration

- **Cache files**: Use `AutoCleanupStrategy` for validation cache
- **Object files**: Use transaction coordinator for multi-object updates
- **Index updates**: Use coordinator for atomic index + object updates

## Metrics Aggregation

### System Object Impact

Metrics provide visibility into:
- Number of locks on system objects (internal) vs public objects
- Lock duration by resource type
- Contention patterns by resource type
- Strategy effectiveness (acquisition rates, failure rates)

### Resource Lock Types

Tracked resource types:
- `index`: Index files (.index)
- `cache`: Cache files (.cache.json)
- `system_object`: Internal system objects (.zqk, _internal)
- `public_object`: Public process objects (.zqk/process)
- `file`: Generic file locks

## Comparison to Graph Database

The file system adapter provides graph-database-like guarantees:

| Feature | Graph DB | File System Adapter |
|---------|----------|---------------------|
| Transactions | ✅ Native | ✅ Two-phase locking |
| Atomicity | ✅ ACID | ✅ All-or-nothing |
| Isolation | ✅ Serializable | ✅ Lock-based |
| Consistency | ✅ Constraints | ✅ Ordered locking |
| Durability | ✅ WAL | ✅ fsync |

## Future Enhancements

1. **Write-Ahead Log (WAL)**: Add WAL for better durability guarantees
2. **Snapshot Isolation**: Implement read snapshots for better concurrency
3. **Deadlock Detection**: Add deadlock detection and resolution
4. **Lock Timeout Strategies**: Configurable timeout strategies per resource type
5. **Metrics Dashboard**: Visualize lock metrics and contention patterns

## Related Documentation

- [Shared Resource Locking](./shared-resource-locking.md) - General locking patterns
- [File Lock Metrics](./file-lock-metrics-and-observability.md) - Metrics collection
- [Content Addressable Storage Design](./content-addressable-storage-design.md) - CAS architecture
- [Bucketing Strategy Pattern](./CRUD_BUCKETING_ARCHIVING_REQUIREMENTS.md) - Similar strategy pattern
