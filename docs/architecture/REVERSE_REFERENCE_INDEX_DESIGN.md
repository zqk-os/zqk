# Reverse Reference Index Design

**Last Verified:** 2026-08-31


**Problem:** `findDependents` currently scans ALL objects and reads/unmarshals each one to check for references. This is O(n) per delete operation, violating the caching policy in `CLI_PERFORMANCE_AND_CONSISTENCY.md`.

**Current Implementation:**
- `findDependents` scans all object directories
- For CAS kinds: Gets all IDs from index, then reads and unmarshals EVERY object
- For non-CAS kinds: Scans files and reads/unmarshals each one
- Comment in code: "This is a simple implementation - for better performance, we could use an index"

**Design Policy:**
- `CLI_PERFORMANCE_AND_CONSISTENCY.md`: "Everything that can be cached, should be cached"
- "Caches must be incrementally maintained (update/invalidate on create/update/delete)"
- "Full cache population only at scheduler init or when explicitly requested"

**Solution: Reverse Reference Index**

A reverse reference index maps `referencedID → []dependentIDs`, allowing O(1) lookup of dependents instead of O(n) scan.

## Implementation Plan

### 1. Cache Structure

**File:** `.zqk/cache/reverse-reference-index.json`

```json
{
  "metadata": {
    "version": "1.0",
    "build_time": "2026-02-17T...",
    "project_root": "/path/to/project",
    "entry_count": 1234
  },
  "index": {
    "BLI-001": ["REQ-001", "REQ-002"],
    "REQ-001": ["CRIT-001"],
    "SCH-run-bundle-1": []
  }
}
```

### 2. Incremental Maintenance

**On Create:**
- Extract reference fields from new object
- For each referenced ID, add the new object's ID to the index entry
- Update cache file (async flush)

**On Update:**
- Extract reference fields from old and new object
- Remove old references: For each old referenced ID, remove this object's ID from index entry
- Add new references: For each new referenced ID, add this object's ID to index entry
- Update cache file (async flush)

**On Delete:**
- Extract reference fields from deleted object
- For each referenced ID, remove this object's ID from the index entry
- Remove this object's ID from the index entirely (no dependents can reference a deleted object)
- Update cache file (async flush)

### 3. Usage in `findDependents`

**Before (current):**
```go
func (f *FileObjectStorage) findDependents(ctx context.Context, id, _ string) ([]string, error) {
    // Scan ALL directories and read ALL objects
    // O(n) where n = total objects
}
```

**After (optimized):**
```go
func (f *FileObjectStorage) findDependents(ctx context.Context, id, _ string) ([]string, error) {
    // Load reverse reference index
    index := f.getReverseReferenceIndex()
    
    // O(1) lookup
    dependents, exists := index[id]
    if !exists {
        return []string{}, nil
    }
    
    // Filter to only existing objects (best-effort)
    return filterExistingDependents(ctx, dependents), nil
}
```

### 4. Cache Loading Strategy

**Lazy Load:**
- Load index on first `findDependents` call
- If cache file doesn't exist or is invalid, trigger background rebuild
- Return empty dependents during rebuild (safe: cascade delete will still work, just slower)

**Background Rebuild:**
- Triggered by scheduler `cache_prewarm` job (Tier 3)
- Or explicitly via `zqk system check --refresh-cache`
- Scans all objects once to build index

### 5. Performance Impact

**Before:**
- Per delete: O(n) scan + O(n) reads + O(n) YAML unmarshal
- For 20k objects: ~20k file operations per delete

**After:**
- Per delete: O(1) index lookup + O(k) filter (where k = dependents, typically 0-10)
- For 20k objects: ~1 map lookup per delete

**Expected improvement:** 100-1000x faster for bulk deletes

## Implementation Notes

1. **Thread Safety:** Use RWMutex for concurrent reads, exclusive lock for updates
2. **Persistence:** Async flush pattern (like ObjectIDCache) - update in-memory, flush to disk in background
3. **Cache Invalidation:** On catastrophic failure or explicit refresh, rebuild from scratch
4. **Leaf Node Optimization:** For leaf kinds (e.g., `scheduler_job`), skip `findDependents` entirely (already implemented)
5. **Backward Compatibility:** If index is missing, fall back to scan (with warning log)

## References

- `docs/architecture/CLI_PERFORMANCE_AND_CONSISTENCY.md` - Caching policy
- `pkg/storage/object_storage_file_delete.go` - Current `findDependents` implementation
- `cmd/zqk/system/check_cache.go` - ObjectIDCache pattern to follow
- `docs/architecture/PRE_CHANGE_CHECKLIST.md` - Incremental cache maintenance requirement
