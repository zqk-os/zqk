# Concurrency Standardization - Executive Summary

**Last Verified:** 2026-08-31


**Date**: 2026-01-05  
**Status**: Critical - Immediate Action Required  
**Priority**: P0 - Addresses Deadlock and Goroutine Leak Risks

## Problem Statement

You're experiencing:
1. **Random goroutine issues** - Untracked goroutines leaking into heap/stack
2. **No standard callback/notify pattern** - Random waits scattered everywhere
3. **Scattered mutex locks** - 1,494 operations without timeout protection
4. **No deadlock prevention** - Locks can hang indefinitely
5. **Rogue processes/threads** - No tracking or lifecycle management

## Solution Overview

I've created a **comprehensive standardization plan** with:

### 1. Standard Callback/Notify Pattern ✅ CREATED
- **File**: `pkg/concurrency/operation_callback.go`
- **Interface**: `OperationCallback` for all async operations
- **Implementation**: `CoordinatorOperationCallback` integrates with coordinator
- **Usage**: All operations must notify via callbacks (start, progress, complete, error, cancel)

### 2. Timeout Mutex Wrapper ✅ CREATED
- **File**: `pkg/concurrency/timeout_mutex.go`
- **Functions**: `WithLockTimeout()`, `WithRLockTimeout()`
- **Note**: There's already `pkg/validation/timeout_mutex.go` - we should consolidate
- **Usage**: ALL mutex operations must use timeout wrapper

### 3. GoroutineManager Integration ✅ EXISTS
- **File**: `pkg/runtime/goroutine_manager.go` (already exists)
- **Requirement**: ALL goroutines must be tracked via GoroutineManager
- **Integration**: Works with GoroutineBuilder pattern

### 4. Documentation ✅ CREATED
- **Plan**: `CONCURRENCY_STANDARDIZATION_PLAN.md`
- **Violations**: `CONCURRENCY_STANDARDIZATION_VIOLATIONS.md`
- **Summary**: This document

## Critical Statistics

### Mutex Operations (All Need Timeout Wrapper)
- **Total**: ~1,494 operations across 219 files
- **Production**: ~333 operations in `pkg/storage` (45 files)
- **Scheduler**: ~100 operations in `pkg/scheduler` (17 files)
- **Validation**: ~123 operations in `pkg/validation` (14 files)

### Critical Files (Highest Risk)
1. `pkg/storage/object_storage_file.go` - 21 operations
2. `pkg/validation/async_validator.go` - 25 operations
3. `pkg/storage/cas_index_write_queue.go` - 21 operations
4. `pkg/storage/io_queue.go` - 12 operations
5. `pkg/storage/hash_registry.go` - 14 operations

### Untracked Goroutines
- **Estimated**: ~100+ untracked goroutines
- **Requirement**: ALL must use GoroutineManager

## Immediate Actions Required

### Phase 1: Critical Deadlock Prevention (This Week)

1. **Replace all mutex operations with timeout wrappers** in:
   - `pkg/storage/object_storage_file.go` (21 operations)
   - `pkg/validation/async_validator.go` (25 operations)
   - `pkg/storage/cas_index_write_queue.go` (21 operations)
   - `pkg/storage/io_queue.go` (12 operations)

2. **Track all goroutines** via GoroutineManager:
   - Worker pools
   - Background tasks
   - Async operations

3. **Add callback/notify pattern** to:
   - All queue operations
   - All worker operations
   - All batch operations

### Phase 2: Standardization (Next Sprint)

1. Migrate remaining components
2. Add enforcement (pre-commit, linter)
3. Document all patterns

## Standard Patterns

### 1. Mutex Operations (MUST Use Timeout)

```go
// WRONG: Direct lock (can hang indefinitely)
mu.Lock()
defer mu.Unlock()
// do work

// CORRECT: Timeout wrapper
err := concurrency.WithLockTimeout(
    &mu, ctx, logger, "operation_name",
    concurrency.DefaultLockTimeout(),
    func() error {
        // do work
        return nil
    },
)
```

### 2. Goroutines (MUST Be Tracked)

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

### 3. Async Operations (MUST Use Callback)

```go
// WRONG: No callback/notify
func (q *Queue) Process() {
    // Process items
    // No notification when done
}

// CORRECT: Callback pattern
func (q *Queue) Process(ctx context.Context, callback OperationCallback) error {
    operationID := generateOperationID()
    callback.OnStart(operationID, map[string]interface{}{
        "queue_size": q.Size(),
    })
    
    start := time.Now()
    err := q.doProcess(ctx)
    duration := time.Since(start)
    
    if err != nil {
        callback.OnError(operationID, err)
        return err
    }
    
    callback.OnComplete(operationID, nil, duration)
    return nil
}
```

## Deadlock Prevention Rules

1. **Lock Ordering**: Always acquire locks in alphabetical order
2. **Timeout on All Locks**: Use `WithLockTimeout()` wrapper
3. **No Blocking I/O While Holding Locks**: Release locks before I/O
4. **Shutdown Checks Before Locks**: Check shutdown before acquiring locks

## Next Steps

1. **Review** `CONCURRENCY_STANDARDIZATION_PLAN.md` for full details
2. **Review** `CONCURRENCY_STANDARDIZATION_VIOLATIONS.md` for all violations
3. **Start Phase 1** migration (critical deadlock risks)
4. **Consolidate** timeout mutex implementations (validation vs concurrency packages)

## Files Created

1. `CONCURRENCY_STANDARDIZATION_PLAN.md` - Full implementation plan
2. `CONCURRENCY_STANDARDIZATION_VIOLATIONS.md` - All violations listed
3. `pkg/concurrency/operation_callback.go` - Standard callback interface
4. `pkg/concurrency/timeout_mutex.go` - Timeout mutex wrapper
5. `CONCURRENCY_STANDARDIZATION_SUMMARY.md` - This document

## Questions to Resolve

1. **Consolidate timeout mutex**: Should we use `pkg/validation/timeout_mutex.go` or `pkg/concurrency/timeout_mutex.go`?
2. **GoroutineManager integration**: How do we ensure all goroutines use it?
3. **Enforcement**: Pre-commit hook or linter rules?
