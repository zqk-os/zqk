# Concurrency Standardization Violations

**Last Verified:** 2026-08-31

**Version**: 1.0  
**Date**: 2026-01-05  
**Status**: Critical Analysis

## Executive Summary

This document identifies all violations of the concurrency standardization requirements:
1. **Direct mutex usage** without timeout wrappers
2. **Untracked goroutines** (not using GoroutineManager)
3. **Missing callback/notify pattern** for coordination
4. **Random waits** without callbacks
5. **Scattered locks** without standard patterns
6. **Deadlock risks** (nested locks, lock + I/O, no timeout)

## Critical Violations

### 1. Direct Mutex Usage Without Timeout (All Mutex Operations)

**Policy**: ALL mutex operations MUST use `WithLockTimeout()` or `WithRLockTimeout()`

**Violations**: ~1,494 mutex operations across 219 files

**Critical Files** (highest risk):
- `pkg/storage/object_storage_file.go` - 21 operations
- `pkg/validation/async_validator.go` - 25 operations
- `pkg/storage/cas_index_write_queue.go` - 21 operations
- `pkg/storage/io_queue.go` - 12 operations
- `pkg/storage/hash_registry.go` - 14 operations
- `pkg/storage/audit_event_buffer.go` - 15 operations
- `cmd/zqk/system/show_validation_progress_helpers.go` - 12 operations
- `pkg/scheduler/transceiver/async_router.go` - 12 operations

**Example Violation**:
```go
// WRONG: Direct lock (can hang indefinitely)
mu.Lock()
defer mu.Unlock()
// do work
```

**Required Fix**:
```go
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

### 2. Untracked Goroutines (Not Using GoroutineManager)

**Policy**: ALL goroutines MUST be tracked via GoroutineManager

**Violations**: ~100+ untracked goroutines

**Critical Areas**:
- Worker pools (validation, storage, scheduler)
- Background tasks (periodic compression, metrics, cleanup)
- Async operations (file I/O, network calls)

**Example Violation**:
```go
// WRONG: Untracked goroutine
go func() {
    doWork()
}()
```

**Required Fix**:
```go
// CORRECT: Tracked via GoroutineManager
id, ctx, err := goroutineManager.Start(GoroutineConfig{
    Name: "worker_1",
    Purpose: "processing tasks",
    Category: "worker",
}, func(ctx context.Context) error {
    return doWork(ctx)
})
```

### 3. Missing Callback/Notify Pattern

**Policy**: ALL async operations MUST use OperationCallback interface

**Violations**: ~50+ operations without callbacks

**Critical Areas**:
- Queue operations (enqueue, dequeue, drain)
- Worker operations (start, stop, process)
- Batch operations (processing, completion)

**Example Violation**:
```go
// WRONG: No callback/notify
func (q *Queue) Process() {
    // Process items
    // No notification when done
}
```

**Required Fix**:
```go
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

### 4. Random Waits Without Callbacks

**Policy**: ALL waits MUST use callback/notify pattern, not random waits

**Violations**: ~20+ instances of random waits

**Critical Areas**:
- WaitGroup waits (already fixed with timeout pattern)
- Channel waits without timeout
- Polling loops without notification

**Example Violation**:
```go
// WRONG: Random wait without callback
for {
    if condition {
        break
    }
    time.Sleep(100 * time.Millisecond) // Random wait
}
```

**Required Fix**:
```go
// CORRECT: Callback/notify pattern
done := make(chan struct{})
callback.OnStart(operationID, metadata)
go func() {
    for {
        if condition {
            callback.OnComplete(operationID, nil, duration)
            close(done)
            return
        }
        select {
        case <-ctx.Done():
            callback.OnCancel(operationID, ctx.Err().Error())
            return
        case <-time.After(100 * time.Millisecond):
            // Continue polling
        }
    }
}()
<-done
```

### 5. Scattered Locks Without Standard Pattern

**Policy**: ALL locks MUST follow standard pattern (timeout, ordering, no I/O)

**Violations**: All 1,494 mutex operations

**Issues**:
- No consistent timeout
- No lock ordering enforcement
- I/O operations while holding locks
- No shutdown checks before locks

### 6. Deadlock Risks

**Policy**: MUST prevent deadlocks through:
- Lock ordering (alphabetical)
- Timeout on all locks
- No blocking I/O while holding locks
- Shutdown checks before locks

**Identified Risks**:

#### Nested Locks (High Risk)
- Multiple components acquire multiple locks
- No consistent ordering
- Potential for circular dependencies

#### Lock + Blocking I/O (High Risk)
- File operations while holding locks
- Network calls while holding locks
- JSON/YAML marshaling while holding locks

#### No Timeout (Critical Risk)
- All 1,494 mutex operations can hang indefinitely
- No way to recover from deadlocks

## Standardization Requirements

### 1. ALL Mutex Operations

**MUST**:
- Use `concurrency.WithLockTimeout()` or `concurrency.WithRLockTimeout()`
- Have deterministic timeout
- Check shutdown before lock
- Release lock before any blocking I/O

### 2. ALL Goroutines

**MUST**:
- Be tracked via GoroutineManager
- Use GoroutineBuilder pattern
- Emit coordinator events for lifecycle
- Have deterministic timeout for waits

### 3. ALL Async Operations

**MUST**:
- Use OperationCallback interface
- Notify on start, progress, complete, error, cancel
- Emit coordinator events
- Have deterministic completion

### 4. ALL Coordination

**MUST**:
- Use callback/notify pattern (no random waits)
- Use channels with timeout
- Use coordinator events
- Be deterministic

## Migration Priority

### Phase 1: Critical Deadlock Risks (Immediate)
1. Replace all mutex operations with timeout wrappers in:
   - `pkg/storage/object_storage_file.go`
   - `pkg/validation/async_validator.go`
   - `pkg/storage/cas_index_write_queue.go`
   - `pkg/storage/io_queue.go`

### Phase 2: High Concurrency Components (Next Sprint)
1. Track all goroutines via GoroutineManager
2. Add callback/notify pattern to all queues
3. Add coordinator events to all operations

### Phase 3: Standardization (Backlog)
1. Migrate remaining components
2. Add enforcement (pre-commit, linter)
3. Document all patterns

## Enforcement

### Pre-commit Hook
- Check for direct `mu.Lock()` usage
- Check for untracked goroutines
- Check for missing callbacks

### Linter Rules
- Enforce timeout mutex wrapper
- Enforce GoroutineManager usage
- Enforce callback pattern

### Code Review Checklist
- [ ] All mutex operations use timeout wrapper
- [ ] All goroutines tracked via GoroutineManager
- [ ] All operations use callback/notify pattern
- [ ] All lifecycle events emit coordinator events
- [ ] No blocking I/O while holding locks
- [ ] Lock ordering is consistent (alphabetical)
- [ ] Shutdown checks before all locks
