# Concurrency Standardization Plan

**Version**: 1.0  
**Date**: 2026-01-05  
**Status**: Critical - Addresses Deadlock and Goroutine Leak Risks

## Executive Summary

This plan addresses the critical issues of:
1. **Scattered mutex locks** without standard patterns
2. **Random waits** without callbacks/notifications
3. **Untracked goroutines** that can leak
4. **No deadlock prevention** mechanisms
5. **No standard callback/notify pattern** for coordination

**Goal**: Standardize all concurrency operations to use:
- **Standard callback/notify pattern** for all coordination
- **GoroutineManager** for all goroutine tracking
- **Timeout mutex wrappers** for all lock operations
- **Coordinator events** for all lifecycle events
- **Deterministic timeouts** for all waits

## Current State Analysis

### Mutex Usage Statistics
- **Total mutex operations**: ~1,494 across 219 files
- **Production code**: ~244 files with mutex usage
- **No standard pattern**: Locks scattered without consistent timeout/coordination

### Goroutine Tracking
- **Tracked via GoroutineManager**: ~5 components
- **Untracked goroutines**: ~100+ instances
- **No lifecycle guarantees**: Many goroutines can leak

### Deadlock Risks
- **Nested locks**: Multiple instances
- **Lock + blocking I/O**: Many instances
- **No timeout on locks**: All mutex operations can hang indefinitely
- **No lock ordering**: Potential for circular dependencies

## Standard Patterns to Implement

### 1. Standard Callback/Notify Pattern

**Problem**: Random waits, no coordination, no notifications

**Solution**: All coordination must use callback/notify pattern:

```go
// Standard callback interface for all async operations
type OperationCallback interface {
    OnStart(operationID string, metadata map[string]interface{})
    OnProgress(operationID string, progress int, total int, message string)
    OnComplete(operationID string, result interface{}, duration time.Duration)
    OnError(operationID string, err error)
    OnCancel(operationID string, reason string)
}

// Standard notify pattern - all operations notify via callback
type AsyncOperation struct {
    ID       string
    Callback OperationCallback
    ctx      context.Context
    cancel   context.CancelFunc
}

func (op *AsyncOperation) Execute(fn func() error) error {
    op.Callback.OnStart(op.ID, map[string]interface{}{
        "operation": "execute",
    })
    
    start := time.Now()
    err := fn()
    duration := time.Since(start)
    
    if err != nil {
        op.Callback.OnError(op.ID, err)
        return err
    }
    
    op.Callback.OnComplete(op.ID, nil, duration)
    return nil
}
```

### 2. GoroutineManager for All Goroutines

**Problem**: Untracked goroutines can leak

**Solution**: ALL goroutines must be tracked via GoroutineManager:

```go
// WRONG: Untracked goroutine
go func() {
    doWork()
}()

// CORRECT: Tracked via GoroutineManager
id, ctx, err := goroutineManager.Start(GoroutineConfig{
    Name: "worker_1",
    Purpose: "processing tasks",
    Category: "worker",
}, func(ctx context.Context) error {
    return doWork(ctx)
})
```

### 3. Timeout Mutex Wrapper for All Locks

**Problem**: Mutex locks can hang indefinitely

**Solution**: ALL mutex operations must use timeout wrapper:

```go
// WRONG: Direct lock (can hang)
mu.Lock()
defer mu.Unlock()
// do work

// CORRECT: Timeout wrapper
err := WithLockTimeout(
    &mu,
    ctx,
    metrics,
    logger,
    "operation_name",
    func() error {
        // do work
        return nil
    },
)
```

### 4. Coordinator Events for All Lifecycle

**Problem**: No visibility into goroutine/operation lifecycle

**Solution**: ALL lifecycle events must emit coordinator events:

```go
// WRONG: Direct logging
logger.Info("Operation started")

// CORRECT: Coordinator event
emitOperationStartEventViaCoordinator(
    ctx, projectRoot, storageProvider,
    operationID, operationType, profile)
```

## Implementation Plan

### Phase 1: Create Standard Interfaces

1. **Create `pkg/concurrency/operation_callback.go`**
   - Standard callback interface
   - Default implementations
   - Integration with coordinator

2. **Create `pkg/concurrency/timeout_mutex.go`**
   - Timeout wrapper for all mutex operations
   - Deadlock detection
   - Metrics integration

3. **Create `pkg/concurrency/goroutine_tracker.go`**
   - Wrapper around GoroutineManager
   - Ensures all goroutines are tracked
   - Automatic coordinator event emission

### Phase 2: Migrate Critical Components

**Priority 1 (Deadlock Risks)**:
1. `pkg/storage/object_storage_file.go` - 21 mutex operations
2. `pkg/validation/async_validator.go` - 25 mutex operations
3. `pkg/scheduler/transceiver/async_router.go` - 12 mutex operations
4. `pkg/storage/cas_index_write_queue.go` - 21 mutex operations
5. `pkg/storage/io_queue.go` - 12 mutex operations

**Priority 2 (High Concurrency)**:
1. All queue implementations
2. All cache implementations
3. All worker pool implementations

### Phase 3: Enforce Standards

1. **Pre-commit hook**: Check for direct mutex usage
2. **Linter rules**: Enforce timeout mutex wrapper
3. **Code review checklist**: Verify callback/notify pattern

## Standard Callback/Notify Pattern

### Interface Definition

```go
package concurrency

import (
    "context"
    "time"
)

// OperationCallback provides standard callback interface for all async operations
type OperationCallback interface {
    // OnStart is called when operation starts
    OnStart(operationID string, metadata map[string]interface{})
    
    // OnProgress is called for progress updates
    OnProgress(operationID string, progress int, total int, message string)
    
    // OnComplete is called when operation completes successfully
    OnComplete(operationID string, result interface{}, duration time.Duration)
    
    // OnError is called when operation fails
    OnError(operationID string, err error)
    
    // OnCancel is called when operation is cancelled
    OnCancel(operationID string, reason string)
}

// CoordinatorOperationCallback implements OperationCallback using coordinator
type CoordinatorOperationCallback struct {
    ctx            context.Context
    projectRoot    string
    storageProvider storage.ObjectStorageProvider
    profile        string
}

func (c *CoordinatorOperationCallback) OnStart(operationID string, metadata map[string]interface{}) {
    emitOperationStartEventViaCoordinator(
        c.ctx, c.projectRoot, c.storageProvider,
        operationID, getString(metadata, "operation_type", "unknown"), c.profile)
}

func (c *CoordinatorOperationCallback) OnComplete(operationID string, result interface{}, duration time.Duration) {
    emitOperationCompletionEventViaCoordinator(
        c.ctx, c.projectRoot, c.storageProvider,
        operationID, getString(metadata, "operation_type", "unknown"), duration, c.profile)
}

// ... (other methods)
```

### Usage Pattern

```go
// All async operations must use callback pattern
func (c *Component) ExecuteOperation(ctx context.Context, callback OperationCallback) error {
    operationID := generateOperationID()
    
    callback.OnStart(operationID, map[string]interface{}{
        "operation_type": "execute",
    })
    
    start := time.Now()
    err := c.doWork(ctx)
    duration := time.Since(start)
    
    if err != nil {
        callback.OnError(operationID, err)
        return err
    }
    
    callback.OnComplete(operationID, nil, duration)
    return nil
}
```

## Deadlock Prevention Strategy

### 1. Lock Ordering

**Rule**: Always acquire locks in consistent order (alphabetical by mutex name)

```go
// WRONG: Inconsistent order
mu1.Lock()
mu2.Lock()

// CORRECT: Consistent order (alphabetical)
mu1.Lock() // "mu1" < "mu2"
mu2.Lock()
```

### 2. Timeout on All Locks

**Rule**: ALL mutex operations must use timeout wrapper

```go
// Use timeout mutex wrapper for all locks
err := WithLockTimeout(&mu, ctx, metrics, logger, "operation", func() error {
    // Lock-protected code
    return nil
})
```

### 3. No Blocking I/O While Holding Locks

**Rule**: Release locks before any blocking I/O

```go
// WRONG: I/O while holding lock
mu.Lock()
file.Sync() // BLOCKING
mu.Unlock()

// CORRECT: Release lock before I/O
data := func() []byte {
    mu.Lock()
    defer mu.Unlock()
    return copyData()
}()
file.Write(data) // I/O without lock
```

### 4. Shutdown Checks Before Locks

**Rule**: Check shutdown before acquiring any lock

```go
// Check shutdown before lock
if atomic.LoadInt32(&shutdownFlag) == 1 {
    return ErrShutdown
}
mu.Lock()
defer mu.Unlock()
```

## Goroutine Tracking Standard

### Requirement

**ALL goroutines MUST be tracked via GoroutineManager**

```go
// WRONG: Untracked goroutine
go func() {
    doWork()
}()

// CORRECT: Tracked goroutine
id, ctx, err := goroutineManager.Start(GoroutineConfig{
    Name: "worker_1",
    Purpose: "processing tasks",
    Category: "worker",
}, func(ctx context.Context) error {
    return doWork(ctx)
})
```

### Integration with GoroutineBuilder

```go
// Use GoroutineBuilder + GoroutineManager together
goroutineManager.Start(GoroutineConfig{
    Name: "worker_1",
    Purpose: "processing tasks",
}, func(ctx context.Context) error {
    // Use builder for additional features
    goroutinelabels.NewGoroutine("worker_1", "processing tasks").
        WithContext(ctx).
        WithWaitGroup(&wg).
        StartWithContext(func(ctx context.Context) error {
            return doWork(ctx)
        })
    return nil
})
```

## Migration Checklist

For each component with mutex/goroutine usage:

- [ ] **Identify all mutex operations**
- [ ] **Replace direct locks with timeout wrappers**
- [ ] **Identify all goroutines**
- [ ] **Track all goroutines via GoroutineManager**
- [ ] **Add callback/notify pattern for coordination**
- [ ] **Add coordinator events for lifecycle**
- [ ] **Verify lock ordering (alphabetical)**
- [ ] **Remove blocking I/O from lock-protected code**
- [ ] **Add shutdown checks before locks**
- [ ] **Add deterministic timeouts for all waits**

## Files Requiring Migration

### Critical (Deadlock Risks)

1. `pkg/storage/object_storage_file.go` - 21 mutex operations
2. `pkg/validation/async_validator.go` - 25 mutex operations, multiple goroutines
3. `pkg/scheduler/transceiver/async_router.go` - 12 mutex operations
4. `pkg/storage/cas_index_write_queue.go` - 21 mutex operations
5. `pkg/storage/io_queue.go` - 12 mutex operations
6. `pkg/storage/hash_registry.go` - 14 mutex operations
7. `pkg/storage/audit_event_buffer.go` - 15 mutex operations
8. `cmd/zqk/system/show_validation_progress_helpers.go` - 12 mutex operations

### High Priority (High Concurrency)

1. All queue implementations
2. All cache implementations
3. All worker pool implementations

## Next Steps

1. **Create standard interfaces** (Phase 1)
2. **Migrate critical components** (Phase 2)
3. **Add enforcement** (Phase 3)
4. **Document patterns** (Ongoing)
