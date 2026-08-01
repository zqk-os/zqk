# Check Command Performance & Caching Strategy v1.0

**Version:** 1.0.0  
**Created:** 2025-12-25  
**Status:** Design  
**Related:** BLI-626, BLI-022

## Current Performance Issues

### Observed Issues
- Check command is noticeably slow for 600+ objects
- No caching in SpecLoader (loads specs repeatedly)
- Validator creates new loaders for each object
- HashRegistry loaded per object (should be per kind)
- Semantic type validation has false positives (946 issues)

### Performance Bottlenecks

1. **Spec Loading** (No Cache)
   - `SpecLoader.LoadSpecWithInheritance()` called for each object
   - Re-parses YAML files repeatedly
   - Re-resolves inheritance chains repeatedly

2. **Lifecycle Loading** (Has Cache, But Not Reused)
   - `LifecycleLoader` has cache, but validator creates new loader per object
   - Cache is per-loader instance, not shared

3. **Hash Registry Loading** (Per Object Instead of Per Kind)
   - HashRegistry loaded for each object file
   - Should load once per kind and reuse

4. **Validator Creation** (New Instance Per Object)
   - `GoValidator` created per object check
   - Creates new `SpecLoader` and `LifecycleLoader` each time

## Caching Strategy

### 1. Spec Loader Caching

**Current**: No cache, loads spec for each object  
**Target**: In-memory cache with TTL

```go
type SpecLoader struct {
    specsDir        string
    dependencyGraph *SpecDependencyGraph
    cache          map[string]*CachedSpec  // kind -> cached spec
    cacheMu        sync.RWMutex
    cacheTTL       time.Duration
}

type CachedSpec struct {
    Spec      *Spec
    LoadedAt  time.Time
    ExpiresAt time.Time
}
```

**Cache Behavior**:
- Load spec once per kind, cache for duration of check command
- Cache key: object kind (e.g., "backlog_item")
- Cache invalidation: On file modification (check mtime)
- Cache TTL: 5 minutes (configurable)

**File vs Graph Backend**:
- **File backend**: Cache based on file mtime, invalidate if file changed
- **Graph backend**: Cache based on node version/timestamp, invalidate if node updated
- Same cache interface, different invalidation strategy

### 2. Lifecycle Loader Caching

**Current**: Has cache, but not shared across validators  
**Target**: Shared cache instance

```go
// Create single lifecycle loader and reuse
lifecycleLoader := objects.NewLifecycleLoader("")
validator := validation.NewGoValidatorWithLoaders(specLoader, lifecycleLoader)
```

**Cache Behavior**:
- LifecycleLoader already has cache (map[string]*Lifecycle)
- Reuse same loader instance across all object checks
- Cache persists for duration of check command

**File vs Graph Backend**:
- **File backend**: Cache based on file mtime
- **Graph backend**: Cache based on node version
- Same cache interface

### 3. Hash Registry Caching

**Current**: Loaded per object  
**Target**: Load once per kind, reuse

```go
// Cache hash registries per kind
type HashRegistryCache struct {
    registries map[string]HashRegistryProvider  // kind -> registry
    mu         sync.RWMutex
}

func (c *HashRegistryCache) GetRegistry(kind, dir string) (HashRegistryProvider, error) {
    c.mu.RLock()
    if reg, ok := c.registries[kind]; ok {
        c.mu.RUnlock()
        return reg, nil
    }
    c.mu.RUnlock()
    
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // Double-check after acquiring write lock
    if reg, ok := c.registries[kind]; ok {
        return reg, nil
    }
    
    // Create and load registry
    reg := storage.NewHashRegistry(kind, dir)
    if err := reg.Load(); err != nil {
        return nil, err
    }
    
    c.registries[kind] = reg
    return reg, nil
}
```

**Cache Behavior**:
- Load registry once per kind at start of check
- Reuse same registry instance for all objects of that kind
- Registry is in-memory map, lookups are O(1)

**File vs Graph Backend**:
- **File backend**: Load from `.{kind}.hashes` file once
- **Graph backend**: Load from `HashRegistry:{kind}` node once
- Same interface, different storage

### 4. Validator Reuse

**Current**: New validator per object  
**Target**: Single validator instance

```go
// Create validators once at start of check
specLoader := objects.NewSpecLoader("")
lifecycleLoader := objects.NewLifecycleLoader("")
validator := validation.GetGlobalRegistry().Get("go")
// Reuse same validator for all objects
```

**Cache Behavior**:
- Create validator once at start of check command
- Reuse same validator instance for all objects
- Validator uses cached spec/lifecycle loaders

## Expected Performance Improvements

### Before Optimization
- 606 objects checked
- ~2-3 seconds (estimated)
- Spec loaded 606 times (once per object)
- HashRegistry loaded 606 times (once per object)
- Validator created 606 times

### After Optimization
- 606 objects checked
- ~0.5-1 second (estimated 2-3x improvement)
- Spec loaded ~20 times (once per unique kind)
- HashRegistry loaded ~20 times (once per unique kind)
- Validator created 1 time

### Cache Hit Rates (Expected)
- Spec cache: ~97% hit rate (20 kinds / 606 objects)
- Lifecycle cache: ~97% hit rate
- HashRegistry cache: ~97% hit rate

## Cache Invalidation Strategy

### File Backend
- Check file mtime before using cached spec
- If file modified since cache load, reload
- HashRegistry: Reload if `.hashes` file modified

### Graph Backend
- Check node version/timestamp before using cached spec
- If node updated since cache load, reload
- HashRegistry: Reload if `HashRegistry:{kind}` node updated

### Manual Invalidation
- CLI flag: `--no-cache` to bypass cache
- CLI flag: `--refresh-cache` to force reload
- Cache TTL: 5 minutes default (configurable)

## Testing Strategy

### Performance Benchmarks
```bash
# Baseline (no cache)
time zqk check all --format json > /dev/null

# With cache
time zqk check all --format json > /dev/null

# Compare times
```

### Cache Behavior Tests
1. **File Backend**:
   - Run check, modify spec file, run again (should reload)
   - Run check twice (second should use cache)

2. **Graph Backend**:
   - Run check, update spec node, run again (should reload)
   - Run check twice (second should use cache)

3. **Backend Switching**:
   - Run check with file backend, switch to graph, run again
   - Cache should be backend-specific (file cache vs graph cache)

## Implementation Plan

1. Add caching to SpecLoader
2. Reuse validators/loaders in check command
3. Cache HashRegistry per kind
4. Add cache metrics (hit/miss rates)
5. Add cache configuration (TTL, size limits)
6. Document cache behavior for file vs graph backends

