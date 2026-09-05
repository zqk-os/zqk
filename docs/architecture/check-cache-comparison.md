# Check Command Cache: Legacy vs Current Implementation

**Last Verified:** 2026-08-31


**Date:** 2025-12-25  
**Status:** Comparison Document

## Overview

This document compares the legacy reference checking implementation with the current cached implementation.

## Legacy Implementation (`checkReferences`)

### How It Worked

```go
func checkReferences(ctx *context.Context, obj *parser.ParsedObject, kind string) []Issue {
    // For each reference in the object:
    //   1. Call findObjectByID(projectRoot, refID)
    //   2. findObjectByID scans ALL object directories sequentially
    //   3. For each kind directory:
    //      - Create new YAMLScanner
    //      - Scan all files in directory
    //      - Check each file's ObjectID
    //   4. Return file path and kind if found
}
```

### Characteristics

1. **No Caching**: Every reference check performs a full file system scan
2. **Sequential Scanning**: Scans all object directories for each reference
3. **O(n*m) Complexity**: 
   - n = number of references to check
   - m = total number of objects across all kinds
   - Worst case: n * m file system operations
4. **Repeated Work**: Same directories scanned multiple times for different references
5. **No Persistence**: Cache rebuilt from scratch every command run
6. **Single-threaded**: All scans happen sequentially

### Performance Impact

- **For 616 objects with ~1000 references**: 
  - Each reference check scans all 616 objects
  - Total operations: ~1000 * 616 = ~616,000 file system operations
  - Time: ~13-14 seconds just for reference checking

### Code Location

- `checkReferences()`: Lines 1404-1492
- `findObjectByID()`: Lines 2326-2346

## Current Implementation (`checkReferencesWithCache`)

### How It Works

```go
func checkReferencesWithCache(ctx *context.Context, obj *parser.ParsedObject, kind string, objectIDCache *ObjectIDCache) []Issue {
    // 1. Extract references from object
    // 2. For each reference:
    //    - Lookup in ObjectIDCache (O(1) map lookup)
    //    - No file system operations
    //    - Instant validation
}
```

### Characteristics

1. **Pre-built Cache**: Cache built once at start of command
2. **Persistent Storage**: Cache saved to `.zqk/cache/object-id-cache.json`
3. **O(1) Lookups**: Map-based lookups, no file system operations
4. **Parallel Building**: 10 worker goroutines build cache in parallel
5. **Smart Invalidation**: Only rebuilds when process directory mtime changes
6. **Reused Across Runs**: Cache persists between command executions
7. **Thread-safe**: Uses `sync.RWMutex` for concurrent access

### Performance Impact

- **Cache Building**: ~1-2 seconds (one-time, parallelized)
- **Reference Checking**: O(1) per reference
- **For 616 objects with ~1000 references**:
  - Cache built once: ~1-2 seconds
  - Each reference check: O(1) map lookup
  - Total operations: ~1000 map lookups (instant)
  - Time: ~1-2 seconds total (including cache build)

### Code Location

- `checkReferencesWithCache()`: Lines 1112-1199
- `ObjectIDCache.BuildCache()`: Lines 296-389
- `ObjectIDCache.LoadCache()`: Lines 103-175
- `ObjectIDCache.SaveCache()`: Lines 177-220

## Key Differences

| Aspect | Legacy | Current |
|--------|--------|---------|
| **Caching** | None | In-memory + persistent disk |
| **Lookup Complexity** | O(n*m) | O(1) |
| **File System Operations** | ~616,000 per run | ~616 (once, parallelized) |
| **Persistence** | None | Saved to `.zqk/cache/` |
| **Parallelization** | None | 10 workers for cache building |
| **Invalidation** | N/A | Process directory mtime check |
| **Reference Check Time** | ~13-14 seconds | ~0.001 seconds (after cache) |
| **Total Check Time** | ~36 seconds | ~20 seconds |
| **Reuse Across Runs** | No | Yes (if cache valid) |

## Performance Comparison

### Legacy Approach
```
Reference Check Flow:
  For each reference (1000 refs):
    For each kind (11 kinds):
      Scan all files in kind directory (avg 56 files)
      Check ObjectID match
  Total: 1000 * 11 * 56 = 616,000 operations
  Time: ~13-14 seconds
```

### Current Approach
```
Cache Build (once):
  Parallel scan all directories (10 workers)
  Build in-memory map: 616 entries
  Save to disk: .zqk/cache/object-id-cache.json
  Time: ~1-2 seconds

Reference Check Flow:
  For each reference (1000 refs):
    Map lookup by ID (O(1))
  Total: 1000 map lookups
  Time: ~0.001 seconds
```

## Cache Structure

### In-Memory Cache
```go
type ObjectIDCache struct {
    mu       sync.RWMutex
    cache    map[string]*ObjectIDCacheEntry  // id -> entry
    metadata *ObjectIDCacheMetadata
    cacheDir string
}

type ObjectIDCacheEntry struct {
    ID       string
    Kind     string
    FilePath string
    MTime    time.Time
    Exists   bool
}
```

### Persistent Cache (JSON)
```json
{
  "metadata": {
    "build_time": "2025-12-25T00:12:20-08:00",
    "project_root": "/Users/lanceettl/ai-projects/zqk",
    "process_dir": "/Users/lanceettl/ai-projects/zqk/docs/process",
    "process_mtime": "2025-12-24T22:40:13.228589728-08:00"
  },
  "entries": {
    "BLI-010": {
      "id": "BLI-010",
      "kind": "backlog_item",
      "file_path": ".../BLI-010.yaml",
      "mtime": "2025-12-24T22:21:33.214139254-08:00",
      "exists": true
    },
    ...
  }
}
```

## Cache Validation

### When Cache is Rebuilt

1. **Cache file doesn't exist** (first run)
2. **Project root changed** (different project)
3. **Process directory mtime changed** (files added/removed/modified)
4. **Cache file corrupted** (JSON parse error)
5. **Metadata missing** (invalid cache structure)

### When Cache is Reused

1. **Cache file exists** ✓
2. **Project root matches** ✓
3. **Process directory mtime unchanged** (within 2 second tolerance) ✓
4. **Cache has entries** ✓

## Migration Path

The legacy `checkReferences()` function is still available for:
- Non-cached code paths (e.g., `checkObject()` without cache)
- Fallback if cache fails
- Single object checks where cache overhead isn't worth it

The cached `checkReferencesWithCache()` is used by:
- `checkObjectWithCache()` (main check path)
- `checkAll()` command (bulk operations)

## Benefits of Current Implementation

1. **42% faster**: ~36s → ~20s for full check
2. **Persistent**: Cache survives between command runs
3. **Scalable**: Performance doesn't degrade with more objects
4. **Parallel**: Utilizes multiple CPU cores
5. **Smart**: Only rebuilds when necessary
6. **Thread-safe**: Safe for concurrent access

## Future Improvements

1. **Incremental Updates**: Update cache when individual objects change (not just rebuild)
2. **Cache Warming**: Pre-build cache in background
3. **Distributed Cache**: Share cache across multiple processes/machines
4. **Graph Backend Support**: Cache from graph database instead of file system

