# Goroutine Architecture Policy

**Version**: 1.0  
**Last Updated**: 2026-01-05  
**Status**: Active

## Overview

This policy defines the acceptable patterns for goroutine usage across the codebase. All goroutines must be deterministic, observable, and follow consistent patterns for lifecycle management, error handling, and coordination.

## Core Principles

1. **Determinism**: All goroutines must have bounded execution time with deterministic timeouts
2. **Observability**: All goroutines must emit coordinator events for lifecycle tracking
3. **Consistency**: All goroutines must use the `goroutinelabels.NewGoroutine()` builder pattern
4. **Error Handling**: All goroutines must have error handlers and panic recovery
5. **Cancellation**: All goroutines must respect context cancellation

## Required Patterns

### 1. Goroutine Creation

**MUST** use `goroutinelabels.NewGoroutine()` builder pattern:

```go
goroutinelabels.NewGoroutine("worker_1", "processing tasks").
    WithContext(ctx).
    WithWaitGroup(&wg).
    WithErrorHandler(func(err error) {
        logger.Error("worker error", logging.Error(err))
    }).
    WithPanicHandler(func(r interface{}) {
        logger.Error("worker panic", logging.Any("panic", r))
    }).
    StartSimple(func() {
        // do work
    })
```

**MUST NOT** use direct `go func()` calls in production code.

### 2. WaitGroup Patterns

**MUST** use deterministic timeout pattern for all `WaitGroup.Wait()` calls:

```go
// CORRECT: Deterministic timeout pattern
waitDone := make(chan struct{})
goroutinelabels.NewGoroutine("wait_group", "waiting for wait group").
    WithContext(ctx).
    WithCleanup(func() {
        close(waitDone)
    }).
    StartSimple(func() {
        wg.Wait()
    })

select {
case <-waitDone:
    // All goroutines completed normally
case <-time.After(timeout):
    // Timeout - log warning but proceed
    logger.Warn("Wait timeout - some goroutines may still be running",
        logging.String("timeout", timeout.String()))
case <-ctx.Done():
    // Context cancelled
    logger.Debug("Wait cancelled", logging.Error(ctx.Err()))
}
```

**MUST NOT** call `wg.Wait()` directly without timeout:

```go
// WRONG: Non-deterministic wait
wg.Wait()
```

### 3. Coordinator Events

**MUST** emit coordinator events for all goroutine lifecycle events:

- **Start**: Emit start event when goroutine begins
- **Completion**: Emit completion event when goroutine finishes (success or error)
- **Cancellation**: Emit cancellation event when context is cancelled
- **Timeout**: Emit timeout event when operation times out

**MUST** use coordinator pattern for all events:

```go
// Emit start event
emitOperationStartEventViaCoordinator(
    ctx, projectRoot, storageProvider,
    operationID, operationType, profile)

// Emit completion event
emitOperationCompletionEventViaCoordinator(
    ctx, projectRoot, storageProvider,
    operationID, operationType, duration, profile)
```

**MUST NOT** use direct logging (`logger.Info()`, `fmt.Fprintf()`) for goroutine lifecycle events.

### 4. Error Handling

**MUST** provide error handlers for all goroutines:

```go
goroutinelabels.NewGoroutine("worker", "processing").
    WithErrorHandler(func(err error) {
        if err != nil && err != context.Canceled {
            logger.Error("worker error", logging.Error(err))
            // Emit error event via coordinator
            emitOperationErrorEventViaCoordinator(
                ctx, projectRoot, storageProvider,
                operationID, err, profile)
        }
    }).
    StartWithContext(func(ctx context.Context) error {
        return doWork(ctx)
    })
```

**MUST** provide panic handlers for all goroutines:

```go
goroutinelabels.NewGoroutine("worker", "processing").
    WithPanicHandler(func(r interface{}) {
        err := fmt.Errorf("panic: %v", r)
        logger.Error("worker panic", logging.Error(err))
        // Emit panic event via coordinator
        emitOperationPanicEventViaCoordinator(
            ctx, projectRoot, storageProvider,
            operationID, err, profile)
    }).
    StartSimple(func() {
        // do work
    })
```

### 5. Context Cancellation

**MUST** respect context cancellation in all goroutines:

```go
goroutinelabels.NewGoroutine("worker", "processing").
    WithContext(ctx).
    StartWithContext(func(ctx context.Context) error {
        for {
            select {
            case <-ctx.Done():
                return ctx.Err()
            case task := <-taskChan:
                if err := processTask(task); err != nil {
                    return err
                }
            }
        }
    })
```

**MUST** check context cancellation before starting work:

```go
// Check context cancellation before starting
select {
case <-ctx.Done():
    return ctx.Err()
default:
    // Continue with work
}
```

### 6. Timeout Patterns

**MUST** use deterministic timeouts for all blocking operations:

```go
// CORRECT: Deterministic timeout
select {
case result := <-resultChan:
    // Process result
case <-time.After(timeout):
    // Timeout - handle gracefully
    logger.Warn("Operation timeout", logging.String("timeout", timeout.String()))
case <-ctx.Done():
    // Context cancelled
    return ctx.Err()
}
```

**MUST NOT** use `time.Sleep()` for waiting (except retry backoff):

```go
// WRONG: Non-deterministic wait
time.Sleep(100 * time.Millisecond)

// CORRECT: Retry backoff (acceptable)
time.Sleep(backoffDelay) // Bounded exponential backoff
```

**MUST NOT** use polling with `time.Sleep()`:

```go
// WRONG: Polling with sleep
for {
    if condition {
        break
    }
    time.Sleep(100 * time.Millisecond)
}

// CORRECT: Use context-based ticker or channel signaling
ticker := time.NewTicker(100 * time.Millisecond)
defer ticker.Stop()
for {
    select {
    case <-ticker.C:
        if condition {
            return
        }
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

### 7. Channel Operations

**MUST** use timeouts for all channel operations:

```go
// CORRECT: Channel operation with timeout
select {
case msg := <-msgChan:
    // Process message
case <-time.After(timeout):
    // Timeout
    logger.Warn("Channel receive timeout")
case <-ctx.Done():
    // Context cancelled
    return ctx.Err()
}
```

**MUST** close channels deterministically:

```go
// CORRECT: Close channel after wait completes
waitDone := make(chan struct{})
goroutinelabels.NewGoroutine("collector", "collecting results").
    WithCleanup(func() {
        close(resultsChan)
    }).
    StartSimple(func() {
        wg.Wait()
    })

// Wait for collector to complete
select {
case <-waitDone:
    // Collector completed
case <-time.After(timeout):
    // Timeout
}
```

### 8. Semaphore Patterns

**MUST** use `WithPreCleanup()` for semaphore release:

```go
sem := make(chan struct{}, maxConcurrent)

for _, task := range tasks {
    sem <- struct{}{} // Acquire semaphore
    
    goroutinelabels.NewGoroutine("worker", "processing task").
        WithWaitGroup(&wg).
        WithPreCleanup(func() {
            <-sem // Release semaphore when done
        }).
        StartSimple(func() {
            processTask(task)
        })
}
```

## Acceptable Exceptions

### 1. Retry Backoff

`time.Sleep()` is acceptable for retry backoff with bounded delays:

```go
// ACCEPTABLE: Retry backoff
delay := initialDelay
for attempt := 0; attempt < maxAttempts; attempt++ {
    if err := operation(); err == nil {
        return nil
    }
    time.Sleep(delay)
    delay = time.Duration(float64(delay) * backoffFactor)
    if delay > maxDelay {
        delay = maxDelay
    }
}
```

### 2. File System Consistency Delays

Small deterministic delays for file system operations are acceptable:

```go
// ACCEPTABLE: File system consistency delay
time.Sleep(10 * time.Millisecond) // Small deterministic delay
```

### 3. Test Code

Test code may have different requirements, but should still follow deterministic patterns to prevent test hangs.

## Prohibited Patterns

### 1. Direct `go func()` Calls

**MUST NOT** use direct `go func()` calls in production code:

```go
// WRONG
go func() {
    // do work
}()

// CORRECT
goroutinelabels.NewGoroutine("worker", "processing").
    StartSimple(func() {
        // do work
    })
```

### 2. Direct `wg.Wait()` Calls

**MUST NOT** call `wg.Wait()` directly without timeout:

```go
// WRONG
wg.Wait()

// CORRECT
waitDone := make(chan struct{})
goroutinelabels.NewGoroutine("wait_group", "waiting").
    WithCleanup(func() {
        close(waitDone)
    }).
    StartSimple(func() {
        wg.Wait()
    })
select {
case <-waitDone:
case <-time.After(timeout):
}
```

### 3. Non-Deterministic Waits

**MUST NOT** use `time.Sleep()` for waiting (except retry backoff):

```go
// WRONG
time.Sleep(100 * time.Millisecond) // Waiting for something

// CORRECT
select {
case <-done:
case <-time.After(100 * time.Millisecond):
}
```

### 4. Missing Coordinator Events

**MUST NOT** use direct logging for goroutine lifecycle events:

```go
// WRONG
logger.Info("Operation started")
fmt.Fprintf(os.Stderr, "Operation started\n")

// CORRECT
emitOperationStartEventViaCoordinator(
    ctx, projectRoot, storageProvider,
    operationID, operationType, profile)
```

### 5. Missing Error Handlers

**MUST NOT** launch goroutines without error handlers:

```go
// WRONG
goroutinelabels.NewGoroutine("worker", "processing").
    StartSimple(func() {
        // No error handling
    })

// CORRECT
goroutinelabels.NewGoroutine("worker", "processing").
    WithErrorHandler(func(err error) {
        logger.Error("worker error", logging.Error(err))
    }).
    StartWithContext(func(ctx context.Context) error {
        return doWork(ctx)
    })
```

## Implementation Checklist

When creating or modifying goroutines, ensure:

- [ ] Uses `goroutinelabels.NewGoroutine()` builder
- [ ] Has deterministic timeout for all waits
- [ ] Emits coordinator events for lifecycle (start, completion, cancellation, timeout)
- [ ] Has error handler (`WithErrorHandler()`)
- [ ] Has panic handler (`WithPanicHandler()`)
- [ ] Respects context cancellation (`WithContext()`)
- [ ] Uses `WithWaitGroup()` if needed
- [ ] Uses `WithPreCleanup()` for semaphore release
- [ ] No direct `go func()` calls
- [ ] No direct `wg.Wait()` calls without timeout
- [ ] No `time.Sleep()` for waiting (except retry backoff)
- [ ] All channel operations have timeouts
- [ ] Channels are closed deterministically

## Migration Guide

To migrate existing code:

1. **Replace `go func()` with `NewGoroutine()` builder**
2. **Add deterministic timeouts to all `wg.Wait()` calls**
3. **Add coordinator events for lifecycle tracking**
4. **Add error and panic handlers**
5. **Replace polling `time.Sleep()` with context-based tickers**
6. **Add timeouts to all channel operations**

## Examples

See the following files for reference implementations:

- `cmd/zqk/system/async_check.go` - Discovery with deterministic timeouts
- `pkg/validation/async_validator.go` - Worker pool with timeouts
- `pkg/scheduler/transceiver/async_router.go` - Router with timeouts
- `cmd/zqk/system/discovery_coordination.go` - Coordinator event patterns

## Enforcement

- **Code Review**: All goroutine usage must be reviewed against this policy
- **Linting**: Consider adding linter rules to detect violations
- **Testing**: All goroutines must be tested with timeout scenarios
- **Documentation**: All exceptions must be documented with justification

## Related Documents

- `GOROUTINE_VIOLATIONS.md` - List of goroutines not using builder pattern
- `GOROUTINE_NON_DETERMINISTIC_PATTERNS.md` - List of non-deterministic patterns
- `pkg/goroutinelabels/README.md` - Builder pattern documentation
- `docs/architecture/architecture/concurrency-patterns-v1.0.md` - Concurrency patterns

## Version History

- **1.0** (2026-01-05): Initial policy document
