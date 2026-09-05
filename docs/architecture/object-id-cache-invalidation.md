# Object ID Cache Invalidation Strategy

**Last Verified:** 2026-08-31


**Date:** 2025-12-25  
**Status:** ✅ Implemented

## Overview

The `ObjectIDCache` is used by the `check` command to quickly validate reference integrity. When objects are created or deleted, the cache must be invalidated or updated to maintain consistency.

**Excluded kinds:** High-volume kinds (`audit_event`, metrics, `scheduler_job`, `zqk_session`, `mcp_session`, `change_journal_entry`, etc.) are **not** included in the object-id-cache. They use the high-volume event cache for Count/OldestIDs/retention; putting them in the object-id-cache would bloat it and duplicate data. Build and `UpdateObjectIDCache` both skip these kinds (see `storage.IsHighVolumeKindForCache`).

**Security & Compliance**: All cache manipulation operations are automatically audited for security and compliance tracking.

## Cache Invalidation Methods

### 1. InvalidateObjectIDCache(id string)

Invalidates a single cache entry. Use when an object is deleted.

**Audit Event**: Automatically creates a `cache_invalidation` audit event (severity: low) if the entry existed.

```go
import "github.com/lanceman/zqk/cmd/zqk/system"

// After deleting an object
system.InvalidateObjectIDCache("BLI-123")
// Creates audit event: cache_invalidation for BLI-123
```

### 2. UpdateObjectIDCache(id, kind, filePath string) error

Updates or adds a cache entry. Use after creating or updating an object.

**Audit Event**: Automatically creates a `cache_update` audit event (severity: low) indicating whether it was a new entry or update.

```go
// After creating an object file
err := system.UpdateObjectIDCache("BLI-123", "backlog_item", "/path/to/BLI-123.yaml")
if err != nil {
    // Handle error (file doesn't exist)
}
// Creates audit event: cache_update for BLI-123
```

### 3. InvalidateObjectIDCacheKind(kind string)

Invalidates all entries for a specific kind. Use for bulk operations.

**Audit Event**: Automatically creates a `cache_bulk_invalidation` audit event (severity: medium) with count of entries removed.

```go
// After bulk deleting all objects of a kind
system.InvalidateObjectIDCacheKind("backlog_item")
// Creates audit event: cache_bulk_invalidation for backlog_item with count
```

## Usage in Create/Delete Commands

### Create Command Pattern

```go
func createObject(kind, id string, data map[string]any) error {
    // 1. Generate file path
    filePath := getObjectFilePath(kind, id)
    
    // 2. Write object to file
    if err := writeObjectFile(filePath, data); err != nil {
        return err
    }
    
    // 3. Update cache
    if err := system.UpdateObjectIDCache(id, kind, filePath); err != nil {
        // Log warning but don't fail creation
        log.Warn("Failed to update object ID cache", "error", err)
    }
    
    return nil
}
```

### Delete Command Pattern

```go
func deleteObject(id string) error {
    // 1. Find object (to get kind and path)
    obj, err := findObject(id)
    if err != nil {
        return err
    }
    
    // 2. Delete file
    if err := os.Remove(obj.FilePath); err != nil {
        return err
    }
    
    // 3. Invalidate cache
    system.InvalidateObjectIDCache(id)
    
    return nil
}
```

### Update Command Pattern

```go
func updateObject(id string, data map[string]any) error {
    // 1. Find object (to get kind and path)
    obj, err := findObject(id)
    if err != nil {
        return err
    }
    
    // 2. Update file
    if err := writeObjectFile(obj.FilePath, data); err != nil {
        return err
    }
    
    // 3. Update cache (mtime will be checked on next lookup, but update proactively)
    if err := system.UpdateObjectIDCache(id, obj.Kind, obj.FilePath); err != nil {
        // Log warning but don't fail update
        log.Warn("Failed to update object ID cache", "error", err)
    }
    
    return nil
}
```

## Audit Events

All cache manipulation operations automatically create audit events for security and compliance:

### Event Types

1. **`cache_invalidation`** (severity: low)
   - Created when: `InvalidateObjectIDCache()` is called and entry existed
   - Fields: `target_id`, `target_kind`, `target_path`, `operation`
   - Purpose: Track cache entries removed due to object deletion

2. **`cache_update`** (severity: low)
   - Created when: `UpdateObjectIDCache()` is called
   - Fields: `target_id`, `target_kind`, `target_path`, `operation` (indicates new vs update)
   - Purpose: Track cache entries added/updated for object creation/modification

3. **`cache_bulk_invalidation`** (severity: medium)
   - Created when: `InvalidateObjectIDCacheKind()` is called and entries existed
   - Fields: `target_kind`, `operation` (includes count of entries removed)
   - Purpose: Track bulk cache operations that affect multiple entries

### Audit Event Storage

- **Location**: `docs/process/audit/{YYYY-MM}/AUD-{sequence}.yaml`
- **Bucketing**: Monthly chronological bucketing (same as other audit events)
- **Best Effort**: Audit event creation failures don't block cache operations

### Security Considerations

- **Low Severity**: Individual cache operations are low severity (normal operations)
- **Medium Severity**: Bulk operations are medium severity (potentially disruptive)
- **No-Op Detection**: Only audits actual changes (no audit for non-existent entries)
- **Actor Tracking**: Records user/actor from environment (`USER` or `USERNAME`)

## Cache Behavior

### Automatic Invalidation

The cache also automatically invalidates entries on lookup if:
- File doesn't exist (deleted)
- File mtime has changed (modified)

This provides a safety net, but proactive invalidation/update is recommended for immediate consistency.

**Note**: Automatic invalidation on lookup does NOT create audit events (only explicit operations do).

### Global Cache Instance

The cache uses a global singleton pattern (similar to `IDValidator`):

```go
cache := system.GetGlobalObjectIDCache()
```

This ensures:
- Single cache instance across the process
- Cache persists across multiple commands in the same process
- Thread-safe access

### Cache Rebuilding

The cache is rebuilt from scratch when:
- `check all` command runs (if reference checking enabled)
- Cache is empty and first lookup occurs

For best performance, update/invalidate the cache proactively rather than relying on rebuilds.

## Daemon Coherence & Restart Gate

When the ZQK scheduler daemon is running, it retains the `ObjectIDCache` in memory. If you perform bulk mutations via the CLI (such as `bulk-delete` or checking out a different git branch), the CLI will update the cache on disk, but the long-lived daemon will still hold stale maps.

**Restart Gate:** After bulk CLI mutations, you must restart the daemon to re-sync its caches:
```bash
zqk scheduler stop
zqk scheduler start
```

## Thread Safety

All cache operations are thread-safe:
- `Get()`: Uses `RLock` (multiple concurrent readers)
- `Set()`, `Invalidate()`, `Update()`: Use `Lock` (exclusive writer)
- `InvalidateKind()`: Uses `Lock` (exclusive writer)

## Best Practices

1. **Always update cache after create**: Ensures immediate consistency
2. **Always invalidate cache after delete**: Prevents stale references
3. **Update cache after modify**: Keeps mtime current
4. **Use InvalidateKind for bulk operations**: More efficient than individual invalidations
5. **Don't fail operations if cache update fails**: Cache will auto-invalidate on next lookup

## Future Enhancements

1. **Persistent cache**: Save cache to disk, reload on startup
2. **Incremental updates**: Only rebuild changed kinds
3. **Cache metrics**: Track hit/miss rates
4. **Event-driven updates**: Listen to file system events for automatic updates

