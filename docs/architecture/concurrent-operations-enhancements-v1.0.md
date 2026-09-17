# Concurrent Operations System - Enhancements

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2026-01-01  
**Status**: Design Complete

## Overview

Enhanced the concurrent operations system with:
- **Timeouts**: All I/O operations have configurable timeouts
- **Retries**: Exponential backoff retry logic following existing patterns
- **Caching**: Cache-aware operations to minimize I/O
- **Background Cache Invalidation**: Async invalidation with consistency reporting
- **Pending Invalidation Awareness**: Track and report pending invalidations

## Key Enhancements

### 1. Timeout Handling

All I/O operations use `context.WithTimeout` to prevent indefinite blocking:

```go
// Default I/O timeout: 30 seconds
ctx, cancel := context.WithTimeout(op.Context, e.ioTimeout)
defer cancel()
```

**Benefits**:
- Prevents hanging operations
- Allows graceful cancellation
- Enables retry on timeout

### 2. Retry Logic

Follows existing retry pattern from `pkg/storage/operation_helper.go`:

```go
type RetryConfig struct {
    MaxAttempts   int           // Default: 3
    InitialDelay  time.Duration // Default: 100ms
    MaxDelay      time.Duration // Default: 5s
    BackoffFactor float64       // Default: 2.0 (exponential)
}
```

**Retryable Errors**:
- Context/timeout errors (`context.DeadlineExceeded`, `context.Canceled`)
- Network/connection errors
- Version conflicts (optimistic locking)
- Temporary errors

**Non-Retryable Errors**:
- Permission denied
- Object not found (for create)
- Validation errors

### 3. Cache-Aware Operations

Operations automatically invalidate cache in background:

```go
// After successful create/update/delete
e.cacheManager.InvalidateAsync(ctx, []string{op.ObjectID}, reason)
```

**Benefits**:
- Non-blocking cache invalidation
- Operations complete faster
- Cache consistency maintained eventually

### 4. Background Cache Invalidation

Cache invalidations run in background goroutines:

```go
func (cm *CacheManager) InvalidateAsync(ctx context.Context, objectIDs []string, reason string) string {
    // Create pending invalidation
    inv := &PendingInvalidation{...}
    
    // Execute in background
    go cm.executeInvalidation(ctx, inv)
    
    return invID
}
```

**Features**:
- Non-blocking (operations don't wait)
- Tracks invalidation status
- Reports consistency status
- Cleans up old invalidations

### 5. Consistency Reporting

System reports best-guess at cache consistency:

```go
type ConsistencyStatus struct {
    IsConsistent        bool     // True if cache is consistent
    PendingCount        int      // Number of pending invalidations
    PendingInvalidations []string // IDs of pending invalidations
    Warnings           []string  // Warnings about potential issues
    LastChecked        time.Time // When status was last checked
}
```

**Consistency Checks**:
- Pending invalidations older than 30 seconds → inconsistent
- Failed invalidations → inconsistent
- All invalidations completed → consistent

### 6. Pending Invalidation Awareness

System tracks pending invalidations that might impact health:

```go
// Check for long-running invalidations
if time.Since(inv.StartedAt) > 30*time.Second {
    status.IsConsistent = false
    status.Warnings = append(status.Warnings, 
        fmt.Sprintf("Cache invalidation %s has been pending for %v", id, duration))
}
```

**Use Cases**:
- System health checks
- User notifications
- Debugging cache issues
- Performance monitoring

## Implementation Details

### Enhanced Operation Executor

`EnhancedOperationExecutor` extends `OperationExecutor` with:

1. **Timeout Configuration**: Configurable I/O timeout (default: 30s)
2. **Retry Configuration**: Configurable retry behavior
3. **Cache Manager**: Manages cache operations and consistency
4. **Enhanced Execution**: All operations use timeout + retry

### Cache Manager

`CacheManager` provides:

1. **Async Invalidation**: Background cache invalidation
2. **Status Tracking**: Tracks pending/completed/failed invalidations
3. **Consistency Reporting**: Reports cache consistency status
4. **Cleanup**: Removes old completed invalidations

### Integration Points

1. **Storage Operations**: Create/Update/Delete operations trigger cache invalidation
2. **Cache Handler**: Uses existing `cacheOperationHandler` from storage layer
3. **Context System**: Integrates with `CacheInvalidationContext` from `pkg/context`

## Usage Example

```go
// Create enhanced executor
executor := NewEnhancedOperationExecutor(
    storage,
    queue,
    4, // workers
    conflictResolver,
    30*time.Second, // I/O timeout
    DefaultRetryConfig(),
)

// Start executor
executor.Start()
defer executor.Stop()

// Enqueue operation
op := &Operation{
    Type:       OperationUpdate,
    ObjectID:   "GOAL-001",
    ObjectKind: "goal",
    Priority:   PriorityHigh,
    Updates:    map[string]any{"title": "New Title"},
    Context:    ctx,
    SecCtx:     secCtx,
}
queue.Enqueue(op)

// Check consistency status
status := executor.GetConsistencyStatus()
if !status.IsConsistent {
    fmt.Printf("Cache consistency warning: %d pending invalidations\n", status.PendingCount)
    for _, warning := range status.Warnings {
        fmt.Printf("  - %s\n", warning)
    }
}
```

## Performance Considerations

1. **Concurrent Execution**: Operations execute in parallel (worker pool)
2. **Non-Blocking I/O**: All I/O operations have timeouts
3. **Cache Minimization**: Cache invalidations are async, don't block operations
4. **Retry Efficiency**: Exponential backoff prevents retry storms
5. **Cleanup**: Old invalidations are cleaned up automatically

## Error Handling

1. **Timeout Errors**: Retried automatically (retryable)
2. **Version Conflicts**: Retried with latest state (retryable)
3. **Permanent Errors**: Fail immediately (non-retryable)
4. **Cache Errors**: Logged but don't fail operations (best effort)

## Next Steps

1. **Integration**: Integrate `EnhancedOperationExecutor` into existing codebase
2. **Testing**: Create comprehensive tests for timeout/retry scenarios
3. **Monitoring**: Add metrics for cache consistency and operation performance
4. **Documentation**: Add usage examples and best practices

