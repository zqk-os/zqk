# Storage Backend Detection and Configuration v1.0

**Version:** 1.0.0  
**Created:** 2025-12-29  
**Status:** Implemented  
**Related:** Graph Backend Architecture Requirements

## Overview

The CLI is modularized such that only **one backend is active at a time** (file-based OR graph-based, never both). The system automatically detects which backend is available and configured, then creates the appropriate storage implementation.

## Architecture

### Storage Factory Pattern

The `StorageFactory` (`pkg/storage/storage_factory.go`) provides automatic backend detection and configuration:

```go
factory, err := storage.NewStorageFactory(ctx, projectRoot)
storageProvider := factory.GetStorage()
backendType := factory.GetBackendType() // "file" or "graph"
```

### Detection Logic

1. **Check Graph Backend Availability**
   - Checks `<CLI_ENV_PREFIX>_GRAPH_ENABLED` environment variable (default: `ZQK_GRAPH_ENABLED`)
   - If enabled, attempts to connect to graph database
   - If connection succeeds → use graph backend
   - If connection fails → fall back to file backend (with warning)

2. **Fallback to File Backend**
   - If graph backend not enabled → use file backend
   - If graph connection fails → use file backend
   - File backend is always available (no external dependencies)

### Backend Types

#### File Backend (`BackendTypeFile`)
- **Storage**: `FileObjectStorage`
- **Location**: `docs/architecture/` directory structure
- **Cache**: Object ID cache built from file system scan
- **Reference Validation**: Uses object ID cache for O(1) lookups
- **Use Case**: Default, no external dependencies

#### Graph Backend (`BackendTypeGraph`)
- **Storage**: `PoolAwareGraphStorage` (wraps `GraphObjectStorage`)
- **Location**: Graph database (MemGraph, Neo4j, etc.)
- **Cache**: Not needed (graph queries are already fast)
- **Reference Validation**: Direct graph queries via `GetNode()`
- **Use Case**: When `<CLI_ENV_PREFIX>_GRAPH_ENABLED=true` (default: `ZQK_GRAPH_ENABLED=true`) and graph database is available

## Integration Points

### CLI Processor

The `Processor` (`internal/cli/processor.go`) uses the storage factory:

```go
// Auto-detects backend based on capabilities
storageFactory, err := storage.NewStorageFactory(cmd.Context(), projectRoot)
storageProvider := storageFactory.GetStorage()
```

### Check Command

The `check` command (`cmd/zqk/system/check_impl.go`) adapts based on backend:

```go
// Detect backend type
storageFactory, err := storage.NewStorageFactory(ctx, projectRoot)

if storageFactory.IsFileBackend("backlog_item") {
    // Build object ID cache from file system
    objectIDCache.BuildCache(projectRoot, forceRebuild)
} else {
    // Graph backend: skip cache (direct queries are fast)
    logger.Debug("Skipping object ID cache (graph backend uses direct queries)")
}
```

## Cache Behavior

### File Backend
- **Object ID Cache**: Built from file system scan
- **Purpose**: Optimize reference validation (O(1) lookups instead of O(n) scans)
- **Invalidation**: Based on file mtime changes
- **Location**: `.zqk/cache/object-id-cache.json`

### Graph Backend
- **Object ID Cache**: Not used
- **Reason**: Graph queries are already fast (O(1) node lookups)
- **Reference Validation**: Direct graph queries via `conn.GetNode()`
- **Performance**: No cache needed, queries are efficient

## Configuration

### Environment Variables

```bash
# Enable graph backend (default: false)
export ZQK_GRAPH_ENABLED=true

# Graph connection settings (optional, defaults shown)
export ZQK_GRAPH_HOST=localhost
export ZQK_GRAPH_PORT=7687
export ZQK_GRAPH_USERNAME=""
export ZQK_GRAPH_PASSWORD=""
export ZQK_GRAPH_DATABASE=""
export ZQK_GRAPH_POOL_SIZE=10
```

### Detection Flow

```
┌─────────────────────────────────────┐
│  NewStorageFactory(ctx, projectRoot) │
└──────────────┬──────────────────────┘
               │
               ▼
    ┌──────────────────────┐
    │ Graph Backend Enabled?│
    │ (ZQK_GRAPH_ENABLED)   │
    └──────┬───────────────┘
           │
    ┌──────┴──────┐
    │             │
   Yes           No
    │             │
    ▼             ▼
┌─────────┐  ┌──────────────┐
│ Connect │  │ Use File     │
│ to Graph│  │ Backend      │
└────┬────┘  └──────────────┘
     │
┌────┴────┐
│ Success?│
└────┬────┘
     │
┌────┴────┐
│         │
Yes       No
│         │
▼         ▼
┌─────────┐  ┌──────────────┐
│ Use     │  │ Fallback to  │
│ Graph   │  │ File Backend │
│ Backend │  │ (with warning)│
└─────────┘  └──────────────┘
```

## Future Considerations

### Kind Discovery

Currently, `discoverObjectKinds()` is file-system specific. For graph backend, we may need:

- Graph-based kind discovery (query graph schema)
- Or: Load kinds from spec directory (specs are still file-based)

### Cache for Graph Backend

If needed in the future, a graph-based cache could:
- Cache object IDs from graph queries
- Invalidate based on graph node timestamps
- Use similar interface to file-based cache

## Benefits

1. **Single Backend Active**: Only one backend is used at a time, simplifying logic
2. **Automatic Detection**: No manual configuration needed
3. **Graceful Fallback**: Falls back to file backend if graph unavailable
4. **Backend-Specific Optimizations**: Each backend uses appropriate strategies (cache for file, direct queries for graph)
5. **Unified Interface**: Both backends implement `ObjectStorageProvider`, transparent to consumers

## Testing

To test backend detection:

```bash
# Test file backend (default)
zqk system check

# Test graph backend
export ZQK_GRAPH_ENABLED=true
zqk system check  # Should detect graph backend and skip cache
```

