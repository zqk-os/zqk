# Goroutine Builder and Pool Factory

The `GoroutineBuilder` provides a fluent API for creating goroutines with consistent patterns and best practices. It ensures all goroutines have labels for profiling and handles common concerns like panic recovery, context cancellation, and cleanup.

The **Budget** and **Pool** types provide a **thread-pool factory** that limits the total number of goroutines across the process. Creating a pool reserves N slots from a global budget; one-off goroutines can reserve 1 via `WithBudget`. This is the single place that controls how many routines are running.

## Features

- **Automatic Label Setting**: All goroutines get labels for profiling visibility
- **Panic Recovery**: Built-in panic recovery with optional custom handlers
- **Context Support**: Automatic context cancellation checking
- **Wait Group Integration**: Easy integration with sync.WaitGroup
- **Cleanup Functions**: Automatic cleanup on goroutine exit
- **Error Handling**: Optional error handlers for functions that return errors
- **Global goroutine budget**: Optional `Budget` caps total goroutines; pools and one-off goroutines reserve slots so you can bound concurrency without sacrificing non-blocking behavior

## Usage Examples

### Simple Goroutine

```go
import "github.com/zqk-os/zqk/pkg/goroutinelabels"

// Fire-and-forget goroutine
goroutinelabels.NewGoroutine("worker_1", "processing tasks").
    StartSimple(func() {
        // do work
    })
```

### With Context

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

goroutinelabels.NewGoroutine("api_handler", "handling API request").
    WithContext(ctx).
    StartSimple(func() {
        // This won't run if context is cancelled
        processRequest()
    })
```

### With Error Handling

```go
goroutinelabels.NewGoroutine("data_processor", "processing data").
    WithErrorHandler(func(err error) {
        log.Error("processing failed", err)
    }).
    Start(func() error {
        return processData()
    })
```

### With Wait Group

```go
var wg sync.WaitGroup

for i := 0; i < 10; i++ {
    goroutinelabels.NewGoroutine(
        fmt.Sprintf("worker_%d", i),
        fmt.Sprintf("processing batch %d", i),
    ).
        WithWaitGroup(&wg).
        StartSimple(func() {
            processBatch(i)
        })
}

wg.Wait() // Wait for all goroutines to complete
```

### With Cleanup

```go
results := make(chan Result, 100)

goroutinelabels.NewGoroutine("collector", "collecting results").
    WithWaitGroup(&wg).
    WithCleanup(func() {
        close(results) // Ensure channel is closed
    }).
    StartSimple(func() {
        // collect results
    })
```

**Important**: Cleanup functions should be **context-agnostic**. They must complete even if the goroutine's context is cancelled. For operations that require lock acquisition or other blocking operations, use `context.Background()` to ensure they complete regardless of the parent context state.

### With Context Function

```go
goroutinelabels.NewGoroutine("periodic_task", "running periodic task").
    WithContext(ctx).
    StartWithContext(func(ctx context.Context) error {
        ticker := time.NewTicker(1 * time.Second)
        defer ticker.Stop()
        
        for {
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-ticker.C:
                doPeriodicWork()
            }
        }
    })
```

### With Result Channel

```go
results := make(chan any, 10)

for _, item := range items {
    goroutinelabels.NewGoroutine(
        fmt.Sprintf("processor_%s", item.ID),
        fmt.Sprintf("processing item %s", item.ID),
    ).
        StartWithResult(results)(func() (any, error) {
            return processItem(item)
        })
}

// Collect results
for i := 0; i < len(items); i++ {
    result := <-results
    // handle result
}
```

### With Panic Handler

```go
goroutinelabels.NewGoroutine("risky_operation", "performing risky operation").
    WithPanicHandler(func(r any) {
        log.Error("panic recovered", "panic", r)
        // Send alert, etc.
    }).
    StartSimple(func() {
        riskyOperation()
    })
```

## Migration from Direct `go func()`

### Before

```go
go func() {
    goroutinelabels.SetGoroutineLabel("worker", "processing")
    defer wg.Done()
    defer func() {
        if r := recover(); r != nil {
            log.Error("panic", r)
        }
    }()
    defer close(results)
    
    if ctx.Err() != nil {
        return
    }
    
    process()
}()
```

### After

```go
goroutinelabels.NewGoroutine("worker", "processing").
    WithContext(ctx).
    WithWaitGroup(&wg).
    WithPanicHandler(func(r any) {
        log.Error("panic", r)
    }).
    WithCleanup(func() {
        close(results)
    }).
    StartSimple(func() {
        process()
    })
```

## Context-Aware vs Context-Agnostic Operations

The builder distinguishes between two types of operations:

### Context-Aware Operations (The Work)

The actual work performed by the goroutine should respect context cancellation:

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

Cleanup functions, status updates, and lock acquisitions must complete **regardless** of context state. These operations should use `context.Background()` for any blocking operations:

```go
goroutinelabels.NewGoroutine("worker", "processing tasks").
    WithContext(ctx).
    WithPostCleanup(func() {
        // This cleanup MUST complete even if ctx is cancelled
        // Use context.Background() for any lock acquisitions or blocking operations
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

**Why this matters**: When a manager's `Shutdown()` method cancels its context, cleanup operations that use that same context for lock acquisition will fail, preventing proper status updates and resource cleanup. Using `context.Background()` ensures these critical operations always complete.

## Benefits

1. **Consistency**: All goroutines follow the same pattern
2. **Less Boilerplate**: No need to manually set labels, handle panics, etc.
3. **Type Safety**: Compile-time checking of function signatures
4. **Profiling**: All goroutines automatically get labels for better profiling
5. **Maintainability**: Changes to goroutine patterns can be made in one place
6. **Reliable Cleanup**: Context-agnostic cleanup ensures resources are always properly released
