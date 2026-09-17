# Async Validation Flow Analysis

**Last Verified:** 2026-08-31


## Flow Diagram

```
Worker Goroutine
  ↓
Enqueue Task (AUD-271)
  ↓
validationFunc()
  ↓
[LOCK: hashRegistryCache.mu] ← Potential contention point
  ↓
  Determine cacheKey (audit_event:/path/to/2026-01)
  ↓
  if isBucketed:
    delete(cache, cacheKey)  ← Fixed: direct deletion
    registry = NewHashRegistry()
    registry.Load() ← [LOCK: HashRegistry.mu] + File I/O (SLOW)
  ↓
[UNLOCK: hashRegistryCache.mu]
  ↓
checkObjectWithCacheAndContent()
  ↓
  [May call registry.GetHash/SetHash] ← [LOCK: HashRegistry.mu]
  ↓
  autoFixIssues()
    ↓
    [May call registry.SetHash/Save] ← [LOCK: HashRegistry.mu] + File I/O (SLOW)
  ↓
Return ValidationState
```

## Potential Deadlock Scenarios

### Scenario 1: Long-Held Cache Lock with File I/O
**Risk Level: HIGH**

**Sequence:**
1. Worker 1: Locks `hashRegistryCache.mu`
2. Worker 1: Calls `registry.Load()` (file I/O, may be slow)
3. Worker 2: Tries to lock `hashRegistryCache.mu` (BLOCKED)
4. Worker 3: Tries to lock `hashRegistryCache.mu` (BLOCKED)
5. Worker 4: Tries to lock `hashRegistryCache.mu` (BLOCKED)

**Impact:** All workers blocked on cache lock while one does file I/O

**Mitigation:** Minimize lock hold time, or use read locks where possible

### Scenario 2: Multiple Workers Processing Same Bucketed Directory
**Risk Level: MEDIUM**

**Sequence:**
1. Worker 1: Processing AUD-271 (audit_event/2026-01/)
2. Worker 2: Processing AUD-272 (audit_event/2026-01/) ← Same directory!
3. Worker 3: Processing AUD-273 (audit_event/2026-01/) ← Same directory!

All three workers:
- Lock `hashRegistryCache.mu`
- Delete same cache key
- Create new registry for same directory
- Load from same file (potential file contention)

**Impact:** Serialized processing of objects in same directory, potential file I/O contention

### Scenario 3: Registry Save While Load in Progress
**Risk Level: LOW** (separate locks, but file contention)

**Sequence:**
1. Worker 1: `registry.Load()` (locks HashRegistry.mu, reads file)
2. Worker 2: `registry.Save()` (locks HashRegistry.mu, writes file)
3. Both operations on same file → potential file system contention

**Impact:** File I/O contention, but no deadlock (separate registry instances)

### Scenario 4: Future Nested Lock Scenario
**Risk Level: MEDIUM** (if code evolves)

If we ever need to:
1. Lock `hashRegistryCache.mu`
2. Then lock another mutex (e.g., validation state cache)
3. And another code path does the reverse

**Impact:** Classic deadlock

**Mitigation:** Always acquire locks in consistent order

## Metrics to Add

### 1. Hash Registry Cache Lock Contention
- **Metric**: `hash_registry_cache_lock_wait_duration_ms`
- **When**: Measure time waiting for `hashRegistryCache.mu.Lock()`
- **Why**: Detect if workers are blocking each other

### 2. Hash Registry Load Duration
- **Metric**: `hash_registry_load_duration_ms`
- **When**: Measure `registry.Load()` duration
- **Why**: Detect slow file I/O that holds cache lock

### 3. Cache Lock Hold Duration
- **Metric**: `hash_registry_cache_lock_hold_duration_ms`
- **When**: Measure time between Lock() and Unlock()
- **Why**: Detect long-held locks causing contention

### 4. Concurrent Cache Access
- **Metric**: `hash_registry_cache_concurrent_access_count`
- **When**: Count workers accessing same cache key simultaneously
- **Why**: Detect hot spots (multiple workers processing same directory)

### 5. Registry Operations Per Object
- **Metric**: `hash_registry_operations_per_validation`
- **When**: Count GetHash/SetHash/Save calls per validation
- **Why**: Detect excessive registry operations

### 6. Bucketed vs Non-Bucketed Processing Time
- **Metric**: `validation_duration_by_object_type_ms`
- **When**: Track validation time separately for bucketed/non-bucketed
- **Why**: Identify if bucketed objects are slower (due to cache lock + file I/O)

