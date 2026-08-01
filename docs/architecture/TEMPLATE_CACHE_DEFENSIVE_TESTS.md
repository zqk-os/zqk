# Template Cache Defensive Tests

**Version**: 1.0.0  
**Created**: 2026-01-08  
**Status**: Active  
**Purpose**: Document comprehensive defensive tests that prevent cache sync issues and non-deterministic loops

## Overview

The template cache system includes 18 comprehensive defensive tests that anticipate and prevent:
- Race conditions
- Cache synchronization issues
- Non-deterministic loops
- Data races
- Resource leaks
- Invariant violations

## Test Categories

### 1. Race Condition Tests

#### `TestStreamingTemplateCache_ConcurrentGenerationRace`
**Purpose**: Prevent duplicate work when multiple goroutines request the same template

**Scenarios Tested**:
- 100 goroutines concurrently requesting the same template
- Verify double-checked locking prevents duplicate generation
- Verify all goroutines receive the same cached template pointer

**Defensive Measures**:
- Double-checked locking pattern in `GetTemplateWithTokens()`
- Atomic generation count tracking
- Pointer equality verification (same cached instance)

#### `TestStreamingTemplateCache_InvalidationDuringRead`
**Purpose**: Prevent panics or inconsistent state when cache is invalidated during reads

**Scenarios Tested**:
- 50 readers concurrently accessing cache
- 5 invalidators concurrently invalidating cache entries
- Verify no errors occur during concurrent invalidation/read

**Defensive Measures**:
- `RWMutex` allows concurrent reads, exclusive writes
- Invalidation doesn't block reads
- Cache remains usable after concurrent invalidation

#### `TestStreamingTemplateCache_ClearCacheDuringAccess`
**Purpose**: Prevent data races when cache is cleared during concurrent access

**Scenarios Tested**:
- 50 accessors concurrently reading cache
- 3 clearers concurrently clearing entire cache
- Verify no errors occur, cache remains usable

**Defensive Measures**:
- `ClearCache()` uses write lock (exclusive)
- Readers can complete before clear takes effect
- Cache rebuilt on next access (graceful degradation)

#### `TestStreamingTemplateCache_ConcurrentGetAndInvalidate`
**Purpose**: Prevent data races when getting and invalidating the same kind concurrently

**Scenarios Tested**:
- 100 iterations of concurrent get/invalidate operations
- Same kind accessed and invalidated simultaneously
- Verify no errors, cache remains consistent

**Defensive Measures**:
- `RWMutex` ensures thread-safe access
- Invalidation uses write lock (blocks concurrent gets briefly)
- Cache state remains consistent

#### `TestStreamingTemplateCache_MultipleKindsConcurrentAccess`
**Purpose**: Prevent contention when accessing different kinds concurrently

**Scenarios Tested**:
- 50 goroutines accessing 5 different kinds concurrently
- Verify all templates for each kind are cached (same pointer)
- Verify no cache misses occur

**Defensive Measures**:
- `RWMutex` allows concurrent reads of different cache entries
- Each kind cached independently
- No cross-kind contention

### 2. Cache Synchronization Tests

#### `TestStreamingTemplateCache_StaleCacheAfterFieldRegistryReload`
**Purpose**: Prevent stale templates after FieldRegistry reload

**Scenarios Tested**:
- Get template → reload FieldRegistry → clear cache → get template again
- Verify template is regenerated (different pointer)
- Verify regenerated template is valid

**Defensive Measures**:
- Manual cache invalidation required after FieldRegistry reload
- Cache doesn't auto-detect FieldRegistry changes (by design)
- Clear cache explicitly to avoid stale data

#### `TestStreamingTemplateCache_NonexistentKind`
**Purpose**: Prevent caching of errors or infinite retry loops

**Scenarios Tested**:
- Request non-existent kind (should error)
- Request again (should still error, not cached)
- Verify errors are not cached

**Defensive Measures**:
- Errors are not cached (only successful templates)
- Failed requests don't pollute cache
- No infinite retry loops

#### `TestStreamingTemplateCache_CacheStateAfterError`
**Purpose**: Verify cache state remains consistent after errors

**Scenarios Tested**:
- Request non-existent kind (error)
- Request valid kind (should work)
- Verify cache is usable after errors

**Defensive Measures**:
- Errors don't corrupt cache state
- Valid entries remain accessible after errors
- Cache state remains consistent

### 3. Non-Deterministic Loop Prevention Tests

#### `TestStreamingTemplateCache_RepeatedInvalidation`
**Purpose**: Prevent infinite loops from repeated invalidation

**Scenarios Tested**:
- Invalidate same kind 100 times
- Get template after invalidation
- Verify template regenerates correctly

**Defensive Measures**:
- Invalidation is idempotent (safe to call multiple times)
- No infinite loops from repeated invalidation
- Cache remains usable

#### `TestStreamingTemplateCache_NoInfiniteLoopOnInvalidation`
**Purpose**: Prevent infinite loops from invalidate-get cycles

**Scenarios Tested**:
- 1000 iterations of: get template → invalidate → get template
- Verify no infinite loops occur
- Verify cache remains usable

**Defensive Measures**:
- Invalidation doesn't trigger automatic regeneration
- Get after invalidation regenerates once (no loop)
- Bounded iteration count (1000) prevents true infinite loops

### 4. Idempotency Tests

#### `TestStreamingTemplateCache_ClearCacheIsIdempotent`
**Purpose**: Verify cache clear is idempotent

**Scenarios Tested**:
- Clear cache 100 times
- Verify cache remains usable
- Verify no errors occur

**Defensive Measures**:
- `ClearCache()` is idempotent (safe to call multiple times)
- Cache state remains valid after repeated clears
- No resource leaks

#### `TestStreamingTemplateCache_InvalidateNonexistentKind`
**Purpose**: Verify invalidating non-existent kinds is safe

**Scenarios Tested**:
- Invalidate non-existent kind (should not panic)
- Invalidate 100 times
- Verify cache remains usable

**Defensive Measures**:
- Invalidation of non-existent entries is safe (no-op)
- No errors or panics
- Valid entries remain accessible

### 5. Concurrent Operation Tests

#### `TestStreamingTemplateCache_GetAllKindsConcurrent`
**Purpose**: Prevent issues when GetAllKinds is called concurrently

**Scenarios Tested**:
- 20 goroutines concurrently calling GetAllKinds
- Verify all return same set of kinds
- Verify no errors occur

**Defensive Measures**:
- `GetAllKinds()` uses FieldRegistry (thread-safe)
- Concurrent calls don't interfere
- Results are consistent

#### `TestStreamingTemplateCache_PreloadAllKindsConcurrent`
**Purpose**: Prevent issues when PreloadAllKinds is called concurrently

**Scenarios Tested**:
- 5 goroutines concurrently calling PreloadAllKinds
- Verify cache is populated correctly
- Verify templates are cached

**Defensive Measures**:
- Preload uses cache (thread-safe via RWMutex)
- Concurrent preloads don't conflict
- Cache state remains consistent

### 6. Consistency Tests

#### `TestStreamingTemplateCache_InvariantConsistency`
**Purpose**: Verify cache maintains invariants under mixed operations

**Scenarios Tested**:
- 50 iterations of mixed operations: get, invalidate, clear
- Verify cache remains consistent
- Verify all kinds remain accessible

**Defensive Measures**:
- Cache maintains invariants under stress
- Mixed operations don't corrupt state
- All entries remain accessible

#### `TestStreamingTemplateCache_GetAllKindsAfterClear`
**Purpose**: Verify GetAllKinds works after cache clear

**Scenarios Tested**:
- Get all kinds → clear cache → get all kinds again
- Verify same set of kinds returned
- Verify no errors

**Defensive Measures**:
- `GetAllKinds()` doesn't depend on cache state
- Works correctly after cache clear
- Results remain consistent

#### `TestStreamingTemplateCache_PreloadAllKindsAfterClear`
**Purpose**: Verify PreloadAllKinds works after cache clear

**Scenarios Tested**:
- Preload → clear → preload again
- Verify cache is populated
- Verify templates are cached

**Defensive Measures**:
- Preload works correctly after cache clear
- Cache is repopulated correctly
- No stale state

#### `TestStreamingTemplateCache_CacheSizeBounded`
**Purpose**: Verify cache doesn't grow unbounded

**Scenarios Tested**:
- Preload all kinds
- Verify cache contains expected entries
- Verify cache size is bounded by number of kinds

**Defensive Measures**:
- Cache size bounded by number of kinds (not unbounded)
- No memory leaks from cached templates
- Cache remains manageable

## Test Execution

All tests run with:
- `-timeout 30s` to prevent hanging
- Multiple iterations (`-count=3`) to catch intermittent issues
- Concurrent operations to stress test thread safety

## Defensive Code Patterns

The cache implementation uses these defensive patterns:

1. **Double-Checked Locking**: Prevents duplicate work in concurrent scenarios
2. **RWMutex**: Allows concurrent reads, exclusive writes
3. **Idempotent Operations**: Invalidation and clearing are safe to repeat
4. **Error Handling**: Errors don't corrupt cache state
5. **Bounded Operations**: No unbounded loops or growth
6. **State Verification**: Tests verify cache state after operations

## Coverage Summary

- **Race Conditions**: 5 tests
- **Cache Synchronization**: 3 tests
- **Loop Prevention**: 2 tests
- **Idempotency**: 2 tests
- **Concurrent Operations**: 2 tests
- **Consistency**: 4 tests

**Total**: 18 comprehensive defensive tests

## Running Tests

```bash
# Run all cache tests
go test ./cmd/zqk/system -run "^TestStreamingTemplateCache"

# Run defensive tests only
go test ./cmd/zqk/system -run "^TestStreamingTemplateCache_(Concurrent|Invalidation|Clear|Stale|Nonexistent|Repeated|Multiple|GetAll|Preload|Cache|Invariant|NoInfinite|Size|Idempotent)"

# Run with race detector
go test -race ./cmd/zqk/system -run "^TestStreamingTemplateCache"

# Run multiple times to catch intermittent issues
go test -count=3 ./cmd/zqk/system -run "^TestStreamingTemplateCache"
```
