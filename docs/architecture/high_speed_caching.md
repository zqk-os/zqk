# High-Speed Caching Architecture V2

## Overview
The previous implementation of Priority Plan PRI-17807959 (High-Speed Object Caching) caused a catastrophic `sync.RWMutex` deadlock during the `pkg/storage` unit tests. This occurred because caching was hooked too deeply into the filesystem YAML parser, which was susceptible to cyclic dependencies and reentrant locks.

To eliminate deadlocks and ensure absolute safety while maintaining high performance, Cache Architecture V2 shifts the caching boundary higher up the stack.

## Architecture

The cache is now positioned at the **`pkg/graph/provider` interface boundary**, rather than inside the low-level parsing logic.

### Deadlock-Free Mechanism
Instead of using complex `sync.RWMutex` locks at the file parsing level, we enforce thread-safety through:
1. **Immutable Maps / Copy-On-Write**: Once cached, an object representation is never mutated in place. If an object is updated, a new map/state is swapped in atomically.
2. **Channel-Based Invalidation**: Cache invalidation signals are processed sequentially via Go channels to a dedicated cache-manager goroutine. This avoids race conditions and eliminates the risk of reentrant locking across disparate systems.
3. **Strict Interface Boundaries**: Caching is isolated to the graph provider boundary. When a request asks for a graph node or object, the provider checks the high-level cache. If a cache miss occurs, the provider fetches the object via `pkg/storage` without holding any cache locks during the I/O or YAML parsing operations.

### Workflow
- **Read**: The graph provider accesses the atomic reference to the current cache map. Reads are completely lock-free and extremely fast via immutable data structures.
- **Write/Update**: 
  - Updates are passed as messages to the cache-manager channel.
  - The cache-manager constructs a new version of the cache (copying the immutable structures).
  - The manager swaps the atomic pointer to the new cache.
- **Eviction**: Handled within the cache-manager goroutine to ensure no concurrent modification panics or deadlocks.

## Migration
- **Remove** all `sync.RWMutex` usage in `pkg/storage` related to caching.
- **Introduce** `atomic.Value` or `atomic.Pointer` for the high-level cache maps in `pkg/graph/provider`.
- **Implement** the channel-based cache-manager.
