# Cache Staleness Detection

**Date:** 2026-01-02  
**Status:** Implemented

## Overview

The object ID cache uses a two-tier staleness detection system to determine when the cache needs to be rebuilt:

1. **Directory mtime check** - Fast, coarse-grained check
2. **Individual entry validation** - Detailed, fine-grained check

## Tier 1: Directory mtime Check

### How It Works

The cache stores the `mtime` (modification time) of the `docs/process` directory in its metadata. When loading the cache, it compares:

```go
currentMTime := info.ModTime()  // Current docs/process mtime
cachedMTime := cacheData.Metadata.ProcessMTime  // Cached mtime
timeDiff := currentMTime.Sub(cachedMTime)
if timeDiff > 2*time.Second {
    // Cache is stale - rebuild
}
```

### Why 2-Second Tolerance?

The 2-second tolerance accounts for:
- Filesystem timestamp precision differences
- Minor directory metadata updates that don't indicate actual file changes
- Clock skew between cache save and load operations

### The Problem: Subdirectory mtime Updates

**Critical Bug (Fixed):** When files are written to subdirectories (e.g., `docs/architecture/criteria/CRIT-9002.yaml`), only the **subdirectory** mtime is updated, **not** the parent `docs/process` directory mtime.

This means:
- Creating `docs/architecture/criteria/CRIT-9002.yaml` → Only `docs/architecture/criteria` mtime changes
- Deleting `docs/architecture/criteria/CRIT-9002.yaml` → Only `docs/architecture/criteria` mtime changes
- The `docs/process` mtime remains unchanged
- Cache staleness check fails to detect changes

### The Fix

All file operations (Create, Update, Delete) now explicitly touch the `docs/process` directory:

```go
func (f *FileObjectStorage) touchProcessDirectory() error {
    processDir := filepath.Join(f.projectRoot, "docs", "process")
    now := time.Now()
    return os.Chtimes(processDir, now, now)
}
```

This ensures that:
- ✅ Create operations update `docs/process` mtime
- ✅ Update operations update `docs/process` mtime
- ✅ Delete operations update `docs/process` mtime
- ✅ Cache staleness detection works correctly

## Tier 2: Individual Entry Validation

### How It Works

When the cache is loaded (and passes the mtime check), it runs `ValidateAndCleanStale()` which:

1. Checks each cache entry to see if the file still exists
2. Compares the file's current mtime with the cached mtime
3. Removes entries where:
   - File doesn't exist (deleted)
   - File mtime doesn't match (modified outside of cache)

### When It Runs

- Automatically when cache is loaded (if mtime check passes)
- Can be triggered manually via `zqk system check --clean-cache`

### Performance

This validation is **expensive** (one `os.Stat()` per cached object), so it only runs:
- After the cache passes the mtime check (cache is likely valid)
- Not during bulk operations (check command) for performance

## Cache Rebuild Triggers

The cache is rebuilt when:

1. **Cache file doesn't exist** (first run)
2. **Project root changed** (different project)
3. **Process directory mtime changed** (by > 2 seconds) - **Now works correctly!**
4. **Cache file corrupted** (JSON parse error)
5. **Metadata missing** (invalid cache structure)
6. **Force rebuild requested** (`--refresh-cache` flag)
7. **Cache loaded but empty after cleanup** (all entries were stale)

## Operations That Touch Process Directory

All operations that modify files now touch the process directory:

- ✅ **Create** - `FileObjectStorage.Create()` calls `touchProcessDirectory()`
- ✅ **Update** - `FileObjectStorage.Update()` calls `touchProcessDirectory()`
- ✅ **Delete** - `FileObjectStorage.Delete()` calls `touchProcessDirectory()`
- ⚠️ **Archival** - Done via status updates (Update path), so covered

## Cache Invalidation vs. Staleness Detection

### Cache Invalidation

- **Immediate** - Removes specific entries from cache
- **Used for:** Individual object operations (create/update/delete)
- **Mechanism:** `executeCacheOperation()` → `CacheInvalidationContext`
- **Result:** Entry removed from in-memory cache

### Staleness Detection

- **Lazy** - Checks on cache load
- **Used for:** Determining if entire cache needs rebuild
- **Mechanism:** mtime comparison + `ValidateAndCleanStale()`
- **Result:** Cache rebuilt or entries cleaned

## Example Flow

### Scenario: Create Object via CLI

1. User runs: `zqk object create criteria --id CRIT-9002 ...`
2. `FileObjectStorage.Create()` writes `docs/architecture/criteria/CRIT-9002.yaml`
3. `touchProcessDirectory()` updates `docs/process` mtime
4. Cache invalidation removes CRIT-9002 from in-memory cache (if present)
5. Next `system check`:
   - Loads cache
   - Compares `docs/process` mtime → **Detects change** (was touched)
   - Rebuilds cache → **Includes CRIT-9002**

### Scenario: Delete Object via CLI

1. User runs: `zqk object delete CRIT-9002`
2. `FileObjectStorage.Delete()` removes `docs/architecture/criteria/CRIT-9002.yaml`
3. `touchProcessDirectory()` updates `docs/process` mtime
4. Cache invalidation removes CRIT-9002 from in-memory cache
5. Next `system check`:
   - Loads cache
   - Compares `docs/process` mtime → **Detects change** (was touched)
   - Rebuilds cache → **Excludes CRIT-9002**

## Testing

See `cmd/zqk/system/check_impl_cache_refresh_test.go`:
- `TestObjectIDCache_RefreshAfterCLICreation` - Verifies cache refresh after CLI operations
- `TestObjectIDCache_ReferenceValidationWithStaleCache` - Verifies reference validation with stale cache

## Related

- `pkg/storage/object_storage_file.go` - File operations with `touchProcessDirectory()`
- `cmd/zqk/system/check_impl.go` - Cache staleness detection logic
- `docs/architecture/README.md` - Cache invalidation details

