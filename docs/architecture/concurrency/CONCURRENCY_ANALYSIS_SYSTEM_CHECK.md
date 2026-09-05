# Concurrency Analysis: System Check Command

**Last Verified:** 2026-08-31

## Executive Summary

This document provides a deep concurrency analysis of the `zqk system check` command, identifying potential deadlocks, blocking operations, and context utilization issues.

## Key Findings

### 1. Context Propagation ✅ GOOD
- System check uses command context (`cmd.Context()`) throughout
- Context is properly propagated to async workers
- Timeout contexts are used for operations that could block

### 2. Lock Usage Analysis

#### Hash Registry Cache Lock (`hashRegistryCache.mu`)
**Location**: `cmd/zqk/system/async_check_helpers.go`

**Risk Level**: MEDIUM
- Lock is held during `registry.Load()` which performs file I/O
- Multiple workers processing same bucketed directory will serialize
- **Mitigation**: Lock is released before long operations, but cache lookup itself is protected

**Recommendation**: 
- Consider using read locks for cache lookups
- Use separate locks for cache vs registry operations

#### Hash Registry Lock (`HashRegistry.mu`)
**Location**: `pkg/storage/hash_registry.go`

**Risk Level**: LOW
- Lock is held during file I/O operations (Load/Save)
- Multiple workers may contend for same registry
- **Mitigation**: Operations are relatively fast, and registry operations are batched

### 3. Potential Blocking Points

#### A. Object ID Cache Building
**Location**: `cmd/zqk/system/check_impl.go:1032`

**Risk**: Cache building can be slow for large projects
- **Mitigation**: Uses `--fast` flag to skip reference checking
- **Context**: Operation respects context cancellation

#### B. Async Validation Worker Pool
**Location**: `cmd/zqk/system/async_check.go`

**Risk**: Workers may block on:
1. Hash registry cache lock contention
2. File I/O operations
3. Validation operations

**Mitigation**:
- Workers check context cancellation regularly
- Timeout contexts are used for operations
- Worker pool size is configurable

#### C. Hash Registry Cache Lookup
**Location**: `cmd/zqk/system/async_check_helpers.go`

**Risk**: Cache lock contention when multiple workers process same directory
- **Current**: Lock is held during entire lookup + registry creation
- **Impact**: Serializes processing of objects in same bucketed directory

### 4. Context Utilization Issues

#### ✅ GOOD: Context Propagation
- Command context is passed to all async operations
- Timeout contexts are created for potentially long operations
- Context cancellation is checked in worker loops

#### ⚠️ WARNING: Background Context Usage
**Location**: `cmd/zqk/system/status_helpers.go:143`
```go
ctx, cancel := stdctx.WithTimeout(stdctx.Background(), 10*time.Second)
```
- Uses `context.Background()` instead of command context
- **Impact**: Cannot be cancelled from parent command
- **Recommendation**: Accept context parameter if called from command

### 5. Deadlock Scenarios

#### Scenario 1: Hash Registry Cache + Registry Lock
**Risk**: LOW
- Cache lock is released before registry operations
- Registry has its own lock, separate from cache

#### Scenario 2: Multiple Workers + Same Directory
**Risk**: MEDIUM
- Multiple workers processing same bucketed directory
- All acquire cache lock, then create/load registry
- **Impact**: Serialized processing (not a deadlock, but performance issue)

#### Scenario 3: Context Cancellation During Lock Hold
**Risk**: LOW
- Workers check context before acquiring locks
- Locks are held for short durations
- **Mitigation**: Lock operations are fast, context checks are frequent

## Recommendations

### High Priority
1. **Add timeout to hash registry cache operations**
   - Use `WithLockTimeout` wrapper for cache operations
   - Prevents indefinite blocking on cache lock

2. **Use read locks for cache lookups**
   - Cache reads don't need exclusive lock
   - Allows concurrent lookups

3. **Add context parameter to `getSystemHealthData`**
   - Replace `context.Background()` with command context
   - Enables proper cancellation

### Medium Priority
1. **Monitor lock contention**
   - Add metrics for lock wait times
   - Alert on excessive contention

2. **Optimize hash registry cache**
   - Consider per-directory locks instead of global cache lock
   - Reduces contention for different directories

3. **Add deadlock detection**
   - Use timeout wrappers for all lock operations
   - Log warnings on timeout

### Low Priority
1. **Profile lock hold times**
   - Identify locks held during I/O
   - Optimize to minimize hold time

2. **Consider lock-free alternatives**
   - Use `sync.Map` for cache (already considered)
   - Atomic operations where possible

## Testing Recommendations

1. **Stress Test**: Run system check with many objects in same directory
2. **Timeout Test**: Verify system check completes within timeout
3. **Cancellation Test**: Verify context cancellation works during check
4. **Concurrent Test**: Run multiple system checks simultaneously

## Conclusion

The system check command has good context propagation and timeout handling. The main risk is lock contention in the hash registry cache when multiple workers process the same directory. This is a performance issue rather than a deadlock risk, but should be optimized.

**Overall Risk Assessment**: LOW-MEDIUM
- No critical deadlock scenarios identified
- Performance optimizations recommended
- Context propagation is good
