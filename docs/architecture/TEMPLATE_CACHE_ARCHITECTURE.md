# Template Cache Architecture

**Version**: 1.0.0  
**Created**: 2026-01-08  
**Status**: Design  
**Purpose**: Design backend-agnostic template caching with file-based optimizations

## Overview

The template cache system uses a layered architecture that separates backend-agnostic caching from backend-specific optimizations. This allows the cache to work with any backend (file-based, graph-based, etc.) while providing optimizations for file-based backends.

## Architecture Layers

```
┌─────────────────────────────────────────────────────────────┐
│                   Template Cache (Backend-Agnostic)          │
│  - Simple in-memory cache                                    │
│  - Manual invalidation (ClearCache, InvalidateKind)         │
│  - Concurrent-safe (RWMutex)                                 │
│  - Works with any backend                                    │
└─────────────────────────────────────────────────────────────┘
                            ▲
                            │
            ┌───────────────┴───────────────┐
            │                               │
┌───────────────────────────┐   ┌──────────────────────────┐
│   Cache Populator         │   │   Direct Access          │
│   (Background Job)        │   │   (Manual Invalidation)  │
└───────────────────────────┘   └──────────────────────────┘
            ▲
            │
┌───────────────────────────┐
│   Mtime Cache Index       │
│   (File-Backend Only)     │
│   - Tracks file mtimes    │
│   - Bounded directory scan│
│   - Background refresh    │
└───────────────────────────┘
```

## Layer 1: Template Cache (Backend-Agnostic)

**Purpose**: Simple, fast, backend-agnostic template caching

**Characteristics**:
- In-memory cache (`map[string]*cachedTemplate`)
- Thread-safe with `sync.RWMutex`
- Manual invalidation only
- No backend-specific assumptions
- Fast reads (no filesystem operations)

**Interface**:
```go
type StreamingTemplateCache struct {
    cache map[string]*cachedTemplate
    mu    sync.RWMutex
}

func (stc *StreamingTemplateCache) GetTemplateWithTokens(kind string) (*TemplateWithTokens, error)
func (stc *StreamingTemplateCache) InvalidateKind(kind string)
func (stc *StreamingTemplateCache) ClearCache()
```

**Behavior**:
- Check cache → return if found
- Generate template if not cached
- Store in cache
- Manual invalidation required

## Layer 2: Mtime Cache Index (File-Backend Only)

**Purpose**: Track file modification times for automatic cache invalidation

**Characteristics**:
- Backend-specific optimization (file-based only)
- Bounded directory scanning (respects known limits)
- Background refresh (periodic or event-driven)
- Maps `kind -> mtime` for spec files
- Separate from template cache

**Interface**:
```go
type MtimeCacheIndex struct {
    index     map[string]time.Time // kind -> mtime
    specsDir  string
    mu        sync.RWMutex
    bounded   bool // Whether scanning is bounded
    maxDepth  int  // Maximum directory depth
}

func (mci *MtimeCacheIndex) GetMtime(kind string) (time.Time, bool)
func (mci *MtimeCacheIndex) Refresh() error
func (mci *MtimeCacheIndex) HasChanged(kind string) bool
```

**Bounded Scanning**:
- Only scans within known directory structure
- Respects `docs/architecture/_internal/object_specs/` boundary
- Does not traverse beyond spec directory
- Limits recursion depth if needed
- Can be disabled for unbounded backends

**Refresh Strategy**:
- Periodic refresh (e.g., every 5 seconds)
- Event-driven refresh (file system watcher)
- On-demand refresh (triggered by cache populator)

## Layer 3: Cache Populator (Background Job)

**Purpose**: Automatically refresh template cache based on mtime index

**Characteristics**:
- Runs as background goroutine
- Checks mtime index for changes
- Invalidates/regenerates templates when specs change
- Sequential processing (no object contention)
- Can be enabled/disabled per backend

**Interface**:
```go
type CachePopulator struct {
    templateCache *StreamingTemplateCache
    mtimeIndex    *MtimeCacheIndex  // nil for non-file backends
    interval      time.Duration
    stopChan      chan struct{}
    wg            sync.WaitGroup
}

func (cp *CachePopulator) Start()
func (cp *CachePopulator) Stop()
func (cp *CachePopulator) Refresh() error
```

**Refresh Logic**:
```go
func (cp *CachePopulator) refresh() error {
    if cp.mtimeIndex == nil {
        return nil // No mtime index for non-file backends
    }
    
    // Refresh mtime index
    if err := cp.mtimeIndex.Refresh(); err != nil {
        return err
    }
    
    // Get all kinds
    kinds, err := cp.templateCache.GetAllKinds()
    if err != nil {
        return err
    }
    
    // Check each kind for changes
    for _, kind := range kinds {
        if cp.mtimeIndex.HasChanged(kind) {
            // Invalidate cache entry
            cp.templateCache.InvalidateKind(kind)
            // Template will be regenerated on next access
        }
    }
    
    return nil
}
```

**Benefits**:
- **Sequential access**: Template cache reads are fast (no blocking)
- **No object contention**: Populator runs separately, doesn't block reads
- **Automatic refresh**: Cache stays up-to-date without manual intervention
- **Backend-agnostic**: Works without mtime index (just no auto-refresh)

## Usage Patterns

### File-Based Backend

```go
// Create template cache (backend-agnostic)
templateCache := NewStreamingTemplateCache(fieldRegistry)

// Create mtime index (file-backend specific)
mtimeIndex := NewMtimeCacheIndex(specsDir, BoundedScanOptions{
    MaxDepth: 1,
    SpecsDir: specsDir,
})

// Create cache populator
populator := NewCachePopulator(templateCache, mtimeIndex, 5*time.Second)
populator.Start()
defer populator.Stop()

// Use cache (fast, no filesystem operations)
template, err := templateCache.GetTemplateWithTokens("criteria")
```

### Graph-Based Backend

```go
// Create template cache (backend-agnostic)
templateCache := NewStreamingTemplateCache(fieldRegistry)

// No mtime index (graph backend)
// No cache populator (or populator with nil mtimeIndex)

// Use cache (manual invalidation)
template, err := templateCache.GetTemplateWithTokens("criteria")

// When specs change in graph, manually invalidate
templateCache.InvalidateKind("criteria")
```

### Hybrid Approach

```go
// Create template cache
templateCache := NewStreamingTemplateCache(fieldRegistry)

// Create mtime index for file-based specs
mtimeIndex := NewMtimeCacheIndex(fileSpecsDir, ...)

// Create cache populator (can work without mtime index)
populator := NewCachePopulator(templateCache, mtimeIndex, 5*time.Second)
populator.Start()

// Cache populator only refreshes file-based specs
// Graph-based specs use manual invalidation
```

## Bounded Directory Scanning

**Constraints**:
- Only scan `docs/architecture/_internal/object_specs/` directory
- Do not traverse subdirectories (specs are flat)
- Respect directory boundaries (don't escape spec directory)
- Limit recursion depth if subdirectories are added in future
- Skip non-YAML files
- Skip system files (`_placeholder.yaml`, etc.)

**Implementation**:
```go
type BoundedScanOptions struct {
    SpecsDir    string
    MaxDepth    int  // Default: 1 (no recursion)
    SkipPatterns []string  // e.g., ["_placeholder.yaml"]
}

func (mci *MtimeCacheIndex) Refresh() error {
    mci.mu.Lock()
    defer mci.mu.Unlock()
    
    // Scan only spec directory (bounded)
    entries, err := os.ReadDir(mci.specsDir)
    if err != nil {
        return err
    }
    
    for _, entry := range entries {
        if entry.IsDir() && mci.bounded && mci.maxDepth <= 1 {
            continue // Skip subdirectories in bounded mode
        }
        
        if !strings.HasSuffix(entry.Name(), ".yaml") {
            continue
        }
        
        // Check if should skip
        if shouldSkip(entry.Name(), mci.skipPatterns) {
            continue
        }
        
        // Get mtime
        info, err := entry.Info()
        if err != nil {
            continue
        }
        
        kind := extractKind(entry.Name())
        mci.index[kind] = info.ModTime()
    }
    
    return nil
}
```

## Benefits

1. **Backend-Agnostic Core**: Template cache works with any backend
2. **File-Backend Optimization**: Mtime index provides automatic refresh for file backends
3. **No Contention**: Sequential access, background refresh
4. **Bounded Scanning**: Respects directory limits, predictable performance
5. **Flexible**: Can enable/disable auto-refresh per backend
6. **Extensible**: Easy to add other backend-specific optimizations

## Future Enhancements

1. **Graph Backend Index**: Similar index for graph-based specs (version/timestamp based)
2. **Event-Driven Refresh**: File system watcher for instant cache updates
3. **Cache Metrics**: Track cache hit rate, refresh frequency, etc.
4. **Selective Refresh**: Only refresh changed kinds (not all)
5. **Cache Warming**: Pre-populate cache on startup for known kinds

## Migration Path

**Phase 1** (Current):
- Backend-agnostic template cache ✅
- Manual invalidation ✅

**Phase 2** (Next):
- Add mtime cache index (file-backend only)
- Add cache populator
- Enable automatic refresh for file backends

**Phase 3** (Future):
- Add graph-backend index
- Add event-driven refresh
- Add cache metrics
