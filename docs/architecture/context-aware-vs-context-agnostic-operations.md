# Context-Aware vs Context-Agnostic Operations

**Version**: 1.0  
**Last Updated**: 2026-01-24  
**Status**: Active

## Overview

This document describes the critical distinction between **context-aware** and **context-agnostic** operations in goroutine management, particularly in manager patterns where shutdown sequences cancel contexts.

## The Problem

When a manager's `Shutdown()` method cancels its context (`gm.shutdownCtx`), any operations that use that same context for lock acquisition or other blocking operations will fail. This prevents:

- Status updates from completing
- Resource cleanup from executing
- Proper goroutine lifecycle tracking

## The Solution: Two Types of Operations

### Context-Aware Operations (The Work)

**Purpose**: The actual work performed by the goroutine that should respect cancellation.

**Characteristics**:
- Should check `ctx.Done()` in loops
- Should exit gracefully when context is cancelled
- Should propagate cancellation to child operations

**Example**:
```go
goroutinelabels.NewGoroutine("worker", "processing tasks").
    WithContext(ctx).
    StartWithContext(func(ctx context.Context) error {
        for {
            select {
            case <-ctx.Done():
                return ctx.Err() // Respect cancellation
            case task := <-taskChan:
                processTask(task)
            }
        }
    })
```

### Context-Agnostic Operations (Cleanup & Status Updates)

**Purpose**: Operations that must complete regardless of context state.

**Characteristics**:
- Must complete even if parent context is cancelled
- Use `context.Background()` for lock acquisition
- Used for cleanup, status updates, resource release

**Example**:
```go
goroutinelabels.NewGoroutine("worker", "processing tasks").
    WithContext(ctx).
    WithPostCleanup(func() {
        // This cleanup MUST complete even if ctx is cancelled
        logger := logging.GetLockLoggerFromProfile("system")
        _ = concurrency.WithLockTimeout(
            &manager.mu,
            context.Background(), // ✅ Context-agnostic
            nil,
            logger,
            "update_status",
            func() error {
                manager.status = "stopped"
                return nil
            },
        )
    }).
    StartWithContext(func(ctx context.Context) error {
        // Context-aware work
        return doWork(ctx)
    })
```

## When to Use Each

### Use Context-Aware Operations For:
- ✅ The main work loop
- ✅ I/O operations that should be cancellable
- ✅ Long-running computations
- ✅ Network requests
- ✅ Any operation that should respect shutdown signals

### Use Context-Agnostic Operations For:
- ✅ Cleanup functions (`WithCleanup`, `WithPreCleanup`, `WithPostCleanup`)
- ✅ Status updates in manager patterns
- ✅ Lock acquisition in cleanup paths
- ✅ Resource release (closing channels, stopping tickers)
- ✅ Final state updates
- ✅ Event emission for lifecycle tracking

## Real-World Example: GoroutineManager

The `GoroutineManager` pattern demonstrates this distinction:

```go
// Context-aware: The actual work
goroutinelabels.NewGoroutine(config.Name, config.Purpose).
    WithContext(ctx). // Context for cancellation
    WithPostCleanup(func() {
        // Context-agnostic: Status update must complete
        _ = concurrency.WithRLockTimeout(
            &gm.mu,
            context.Background(), // ✅ Not gm.shutdownCtx
            nil, logger, "postcleanup_check",
            func() error {
                // Update status - must work even after shutdown
                tracked.Status = StatusStopped
                return nil
            },
        )
    }).
    StartWithContext(func(ctx context.Context) error {
        // Context-aware work
        return fn(ctx)
    })
```

**Why this matters**: When `Shutdown()` cancels `gm.shutdownCtx`, the `PostCleanup` function still needs to update the goroutine's status. Using `context.Background()` ensures the lock acquisition succeeds, allowing proper cleanup.

## Implementation Guidelines

### For Manager Patterns

1. **Work Context**: Use the manager's context (or a derived context) for the actual work
2. **Cleanup Context**: Use `context.Background()` for all cleanup operations
3. **Lock Acquisition**: Always use `context.Background()` in cleanup paths

### For Builder Pattern

The `GoroutineBuilder` cleanup functions (`WithCleanup`, `WithPreCleanup`, `WithPostCleanup`) are inherently context-agnostic. They receive no context parameter and should use `context.Background()` for any blocking operations.

### Anti-Patterns

**❌ DON'T**: Use the work context for cleanup operations
```go
WithPostCleanup(func() {
    _ = concurrency.WithLockTimeout(
        &manager.mu,
        ctx, // ❌ Will fail if ctx is cancelled
        nil, logger, "update",
        func() error { ... },
    )
})
```

**✅ DO**: Use `context.Background()` for cleanup operations
```go
WithPostCleanup(func() {
    _ = concurrency.WithLockTimeout(
        &manager.mu,
        context.Background(), // ✅ Always works
        nil, logger, "update",
        func() error { ... },
    )
})
```

## Benefits

1. **Reliable Cleanup**: Resources are always properly released
2. **Accurate Status**: Status updates complete even during shutdown
3. **No Resource Leaks**: Cleanup operations can't be blocked by context cancellation
4. **Deterministic Behavior**: Shutdown sequences complete predictably

## Related Patterns

- [Goroutine Architecture Policy](./GOROUTINE_ARCHITECTURE_POLICY.md)
- [Context Lifecycle Management](./concurrency-patterns-v1.0.md#context-lifecycle-management-pattern)
- [Bulletproof Shutdown Implementation](../best-practices/atomic-shutdown-flags.md)
