# Hash Registry Cache Optimization

**Last Verified:** 2026-08-31


## Current Problem

**Contention Point:** All workers processing objects in the same bucketed directory contend for the same global cache lock.

**Example:**
- Worker 1: Processing `AUD-271` in `audit_event/2026-01/`
- Worker 2: Processing `AUD-272` in `audit_event/2026-01/` ← Same cache key!
- Worker 3: Processing `AUD-273` in `audit_event/2026-01/` ← Same cache key!

All three workers:
1. Lock `hashRegistryCache.mu` (global lock)
2. Delete cache key `audit_event:/path/to/2026-01`
3. Create new registry instance
4. Load from disk (file I/O while holding lock)
5. Unlock

**Impact:** Workers processing the same month are serialized, even though they could process different months in parallel.

## Proposed Solutions

### Option 1: Sharded Locks (Recommended)
**Approach:** Partition the cache by directory hash, so different directories use different locks.

**Benefits:**
- Workers processing different directories don't block each other
- Simple to implement
- Maintains current cache structure

**Implementation:**
```go
type shardedHashRegistryCache struct {
    shards []*hashRegistryCacheShard
    shardCount int
}

type hashRegistryCacheShard struct {
    mu    sync.RWMutex
    cache map[string]storage.HashRegistryProvider
}

func (c *shardedHashRegistryCache) getShard(key string) *hashRegistryCacheShard {
    hash := fnv.New32a()
    hash.Write([]byte(key))
    return c.shards[hash.Sum32()%uint32(c.shardCount)]
}
```

**Lock Contention:** Reduced by factor of `shardCount` (e.g., 16 shards = 16x less contention)

### Option 2: Per-Directory Registry Pool
**Approach:** Use a sync.Map keyed by directory path, eliminating the need for a global lock.

**Benefits:**
- No global lock contention
- Each directory has its own registry instance
- Thread-safe via sync.Map

**Implementation:**
```go
type directoryRegistryPool struct {
    registries sync.Map // map[string]*HashRegistry (keyed by directory path)
}

func (p *directoryRegistryPool) GetOrCreate(kind, dir string) storage.HashRegistryProvider {
    key := fmt.Sprintf("%s:%s", kind, dir)
    if reg, ok := p.registries.Load(key); ok {
        return reg.(storage.HashRegistryProvider)
    }
    
    // Create new registry
    reg := storage.NewHashRegistry(kind, dir)
    reg.Load() // Load from disk
    
    // Store in pool (may overwrite if another goroutine created it, but that's OK)
    p.registries.Store(key, reg)
    return reg
}
```

**Lock Contention:** Eliminated (sync.Map uses fine-grained locking internally)

### Option 3: Read-Write Lock with Directory-Level Caching
**Approach:** Use RWMutex so multiple readers can access different directories concurrently.

**Benefits:**
- Multiple workers can read from different directories simultaneously
- Only writes (cache updates) require exclusive lock
- Simple change from current code

**Implementation:**
```go
type hashRegistryCacheType struct {
    mu    sync.RWMutex  // Changed from sync.Mutex
    cache map[string]storage.HashRegistryProvider
}

// For reads (getting registry):
hashRegistryCache.mu.RLock()
registry, exists := hashRegistryCache.cache[cacheKey]
hashRegistryCache.mu.RUnlock()

// For writes (updating cache):
hashRegistryCache.mu.Lock()
hashRegistryCache.cache[cacheKey] = registry
hashRegistryCache.mu.Unlock()
```

**Lock Contention:** Reduced for read-heavy workloads (multiple concurrent reads allowed)

### Option 4: Registry Factory (No Caching for Bucketed Objects)
**Approach:** Don't cache bucketed objects at all - create registries on-demand without cache coordination.

**Benefits:**
- No cache lock needed for bucketed objects
- Each worker creates its own registry instance
- HashRegistry itself is thread-safe (has its own mutex)

**Implementation:**
```go
// For bucketed objects, skip cache entirely
if isBucketed {
    // No cache lock needed - create registry directly
    registry = storage.NewHashRegistry(objectKind, fileDir)
    registry.Load() // HashRegistry.Load() has its own mutex
} else {
    // Non-bucketed: use cache (as before)
    hashRegistryCache.mu.Lock()
    // ... existing cache logic ...
    hashRegistryCache.mu.Unlock()
}
```

**Lock Contention:** Eliminated for bucketed objects (they don't use cache anyway)

## Recommendation

**Best Approach: Option 2 (Per-Directory Registry Pool with sync.Map)**

**Rationale:**
1. **Eliminates global lock contention** - Different directories don't block each other
2. **Maintains correctness** - Each directory still gets its own registry instance
3. **Simple implementation** - sync.Map handles thread-safety
4. **Performance** - Fine-grained locking in sync.Map is more efficient than global lock
5. **Compatible with current design** - Registry instances are still per-directory

**Additional Optimization:**
- For bucketed objects, we can still reload from disk on each access (to catch newly created objects)
- But we can use the pool to avoid creating multiple registry instances for the same directory
- The registry's internal mutex handles concurrent access to the same registry instance

## Implementation Plan

1. Replace `hashRegistryCacheType` with `directoryRegistryPool` using `sync.Map`
2. Remove global `hashRegistryCache.mu` lock for bucketed objects
3. Keep per-directory registry instances (correctness)
4. Add metrics to track pool hits/misses
5. Update both `async_check.go` and `check_impl.go` to use new pool

## Metrics to Add

- `hash_registry_pool_hits` - Registry found in pool
- `hash_registry_pool_misses` - Registry created (not in pool)
- `hash_registry_pool_size` - Number of registries in pool
- `hash_registry_directory_contention` - Workers accessing same directory (should be low with pool)

