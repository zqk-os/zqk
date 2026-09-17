# Concurrency Architecture & Patterns

**Last Verified:** 2026-09-09
**Status:** Active

## Overview
This document serves as the canonical reference for concurrency patterns, synchronization requirements, and the coordinator pattern across the codebase. All concurrent operations MUST adhere to these rules to ensure determinism and system stability.

## Core Principles

1. **Determinism**: All goroutines and synchronization primitives must have bounded execution times.
2. **Observability**: Operations must emit events through the EventCoordinator. Direct logging for lifecycle events is prohibited.
3. **Consistency**: Use standardized builder patterns for creation and timeout wrappers for locks.
4. **Safety**: Error handling, panic recovery, and context cancellation are mandatory.

## Required Patterns

### 1. Synchronization and Deterministic Waits

**WaitGroups:**
Never call `wg.Wait()` directly without a timeout. This creates deadlock risks.
```go
// CORRECT: Deterministic wait
waitDone := make(chan struct{})
goroutinelabels.NewGoroutine("waiter", "wait for workers").
    WithCleanup(func() { close(waitDone) }).
    StartSimple(func() { wg.Wait() })

select {
case <-waitDone:
    // Success
case <-time.After(timeout):
    // Timeout
case <-ctx.Done():
    // Cancelled
}
```

**Mutexes:**
Avoid direct `mu.Lock()`. Use the timeout wrapper to prevent indefinite hangs.
- Always acquire locks in a consistent order (alphabetical by name).
- Release locks before performing blocking I/O.
- Check shutdown flags before acquiring a lock.

### 2. Coordinator Pattern

Instead of random ad-hoc channels or callbacks, use the centralized Coordinator for event emission.
- **Lifecycle Events**: Emit start, progress, complete, error, and timeout events.
- **Example**: Use `emitOperationStartEventViaCoordinator` instead of `logger.Info`.

### 3. Goroutine Construction

All manual goroutines MUST be created using the `goroutinelabels.NewGoroutine()` builder or the `GoroutineManager`. Direct `go func()` calls are banned in production code.

```go
goroutinelabels.NewGoroutine("worker", "process items").
    WithContext(ctx).
    WithErrorHandler(func(err error) { /* handle */ }).
    WithPanicHandler(func(r interface{}) { /* recover */ }).
    StartWithContext(func(ctx context.Context) error {
        // Safe execution
        return nil
    })
```

### 4. Channel and Semaphore Operations

- **Channel Reads/Writes**: Must always include a timeout or context check.
- **Semaphores**: Use buffered channels and release them deterministically using `WithPreCleanup` or `defer`.

### 5. Allowed Exceptions
- Bounded exponential backoff (`time.Sleep()` is allowed only in retry logic).
- Minor deterministic file system consistency delays (e.g., `time.Sleep(10 * time.Millisecond)`).

## Migration & Compliance
Legacy patterns (untracked goroutines, direct mutex locking, unbound waitgroups) are actively being removed. New code MUST follow this document natively.
