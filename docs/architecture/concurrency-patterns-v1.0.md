# Concurrency Patterns

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Status:** Active  
**Date:** 2026-01-XX  
**Purpose:** Catalog standardized concurrency and async operation patterns used throughout the ZQK codebase

## Executive Summary

This document catalogs the standardized concurrency and async operation patterns used throughout the ZQK codebase. These patterns provide consistency, testability, and observability for concurrent operations. All patterns follow established best practices for goroutine lifecycle management, resource cleanup, and graceful shutdown.

## Pattern Index

1. [WaitGroup Lifecycle Management Pattern](#waitgroup-lifecycle-management-pattern)
2. [Goroutine Builder Pattern](#goroutine-builder-pattern)
3. [On-Demand Worker Pattern](#on-demand-worker-pattern)
4. [Explicit Async Operation Pattern](#explicit-async-operation-pattern)
5. [Async Processing with Synchronous Responses](#async-processing-with-synchronous-responses)
6. [Worker Pool Pattern](#worker-pool-pattern)
7. [Graceful Shutdown Pattern](#graceful-shutdown-pattern)
8. [Context Lifecycle Management Pattern](#context-lifecycle-management-pattern)
9. [Retry with Exponential Backoff Pattern](#retry-with-exponential-backoff-pattern)
10. [Write-Behind Cache Update Buffer Pattern](#write-behind-cache-update-buffer-pattern)
11. [Wall-clock timeout multiplexing (blocking fn)](#wall-clock-timeout-multiplexing-blocking-fn)
12. [Cache sidecar idle coordination](#cache-sidecar-idle-coordination-object-id--reverse-reference-background)

## Related Documentation

- [On-Demand Worker Pattern](./on-demand-worker-pattern.md) - Detailed documentation
- [MCP Async Handling](./MCP_ASYNC_HANDLING.md) - Async processing pattern
- [Storage Async Architecture Improvements](../../architecture/STORAGE_ASYNC_ARCHITECTURE_IMPROVEMENTS.md) - Storage layer async patterns
- [Goroutine Leak Prevention Checklist](./mcp-goroutine-leak-prevention-checklist.md) - Implementation checklist
- [Concurrency package README](../../../pkg/concurrency/README.md) — `WaitCondContext`, `WithRLockTimeout`, and other primitives referenced from system cache code

---

## WaitGroup Lifecycle Management Pattern

**Version:** 1.0.0  
**Status:** Implemented (Phase 1)  
**Purpose:** Centralize WaitGroup lifecycle management with observability and tracking

### Problem Statement

Direct `sync.WaitGroup` usage leads to:
- **Inconsistent patterns** across codebase (some use `goroutinelabels.WithWaitGroup()`, some manually call `Add()`/`Done()`)
- **Hidden async operations** (caller doesn't know about WaitGroups)
- **Difficult to debug** (no tracking of WaitGroup lifecycle)
- **Easy to introduce bugs** (double `Done()`, missing `Add()`, negative counter panics)

### Solution

**WaitGroupManager** provides centralized lifecycle management with:
- Unique ID-based tracking
- Optional observer interface for lifecycle events
- Metadata tracking (operation type, creation time, last access)
- Panic safety (panics if WaitGroup doesn't exist - catches bugs early)
- Backward compatible (returns `*sync.WaitGroup` for existing code)

### Implementation

**Location**: `pkg/storage/waitgroup_manager.go`

**Interface**:
```go
type WaitGroupManager struct {
    wgs      map[string]*waitGroupEntry
    mu       sync.RWMutex
    observer WaitGroupObserver
}

// Methods
CreateGroup(id, operation string) *sync.WaitGroup
GetGroup(id string) *sync.WaitGroup
Add(id string, delta int)
Done(id string)
Wait(id string)
DeleteGroup(id string)
```

**Observer Interface** (optional):
```go
type WaitGroupObserver interface {
    OnGroupCreated(id, operation string)
    OnGroupAdd(id string, delta int)
    OnGroupDone(id string)
    OnGroupWait(id string)
    OnGroupCompleted(id string, duration time.Duration)
}
```

### Usage Example

```go
// Create manager
wgManager := storage.NewWaitGroupManager()

// Create a WaitGroup with tracking
wg := wgManager.CreateGroup("operation_id", "operation_type")

// Use WaitGroup (compatible with goroutinelabels)
goroutinelabels.NewGoroutine("worker", "description").
    WithWaitGroup(wg).
    StartSimple(func() {
        // Work here
    })

// Wait for completion (with observability)
wgManager.Wait("operation_id")
```

### Implementation Requirements

✅ **MUST** create WaitGroupManager in component constructor  
✅ **MUST** create WaitGroups through manager (not direct `sync.WaitGroup`)  
✅ **MUST** use unique IDs for each WaitGroup  
✅ **MUST** use descriptive operation types  
✅ **SHOULD** add observer for tracking in production  
✅ **SHOULD** delete groups after Wait() completes (cleanup)

### Benefits

- **Clear Intent**: WaitGroups are explicitly created and tracked
- **Observable**: Can track all WaitGroup lifecycle events
- **Debuggable**: Can see which WaitGroups exist, when created, last accessed
- **Testable**: Can inject observer for testing
- **Consistent**: Single pattern across codebase

### Migration Strategy

1. Create `WaitGroupManager` in component constructor
2. Create WaitGroup through manager: `wg := manager.CreateGroup(id, operation)`
3. Use WaitGroup normally (compatible with `goroutinelabels`)
4. Replace `wg.Wait()` with `manager.Wait(id)`
5. Optionally add observer for tracking

### Examples in Codebase

- `OperationExecutor` (`pkg/storage/operation_executor.go`) - Migrated in Phase 1

---

## Goroutine Builder Pattern

**Version:** 1.0.0  
**Status:** Implemented  
**Purpose:** Provide consistent, safe goroutine creation with built-in best practices

### Problem Statement

Direct `go func()` usage leads to:
- Inconsistent panic recovery
- Missing goroutine labels (hard to profile)
- Manual WaitGroup management (error-prone)
- No context cancellation checking
- Inconsistent cleanup patterns

### Solution

**GoroutineBuilder** provides a fluent API for creating goroutines with:
- Automatic label setting for profiling
- Built-in panic recovery
- Context cancellation support
- WaitGroup integration
- Cleanup functions
- Error handling

### Implementation

**Location**: `pkg/goroutinelabels/builder.go`

**Usage**:
```go
// Simple goroutine
goroutinelabels.NewGoroutine("worker_1", "processing tasks").
    StartSimple(func() {
        // do work
    })

// With context and error handling
goroutinelabels.NewGoroutine("api_handler", "handling API request").
    WithContext(ctx).
    WithErrorHandler(func(err error) {
        log.Error("goroutine error", err)
    }).
    Start(func() error {
        return processRequest()
    })

// With wait group
var wg sync.WaitGroup
goroutinelabels.NewGoroutine("background_worker", "background processing").
    WithWaitGroup(&wg).
    WithCleanup(func() {
        close(results)
    }).
    StartSimple(func() {
        // do work
    })
wg.Wait()
```

### Wall-clock timeout multiplexing (blocking fn)

**When to use:** The callee `fn` may block for a long time and does **not** take a `context.Context` (so `StartWithContext` cannot interrupt it). The parent still needs a **hard wall-clock bound** on how long it waits.

**Pattern:** Run `fn` inside `NewGoroutine(...).StartSimple` (optionally `WithBudget(DefaultBudget())`), send the result on a **buffered** channel of size 1; the parent `select`s on that channel vs `time.After` / `Timer`. This is **not** a substitute for making `fn` cancellable: if the timeout fires, the labeled goroutine may still run until `fn` returns (same as a raw `go func`). Prefer context-aware `fn` when adding new APIs.

**Example (scheduler control):** `runSchedulerControlWithTimeout` in `cmd/zqk/scheduler/scheduler_core.go` — wraps `scheduler stop` / `scheduler status` work with `schedulerControlTimeout`.

```go
done := make(chan error, 1)
builder := goroutinelabels.NewGoroutine("scheduler_control_operation", fmt.Sprintf("run %s with timeout", op))
if bud := goroutinelabels.DefaultBudget(); bud != nil {
    builder = builder.WithBudget(bud)
}
builder.StartSimple(func() { done <- fn() })
select {
case err := <-done:
    return err
case <-time.After(timeout):
    return fmt.Errorf("%s timed out after %s", op, timeout)
}
```

### Implementation Requirements

✅ **MUST** use `goroutinelabels.NewGoroutine()` for all goroutines  
✅ **MUST** provide descriptive name and purpose  
✅ **MUST** use `WithContext()` if goroutine should respect cancellation  
✅ **MUST** use `WithWaitGroup()` if goroutine should be tracked  
✅ **SHOULD** use `WithPanicHandler()` for custom panic handling  
✅ **SHOULD** use `WithCleanup()` for resource cleanup

### Benefits

- **Consistency**: All goroutines follow the same pattern
- **Less Boilerplate**: No need to manually set labels, handle panics, etc.
- **Type Safety**: Compile-time checking of function signatures
- **Profiling**: All goroutines automatically get labels
- **Maintainability**: Changes to goroutine patterns can be made in one place

### Examples in Codebase

- Used in 101+ files across the codebase
- `OperationExecutor` workers
- `HashRegistry` save workers
- `IOQueueManager` workers
- All on-demand worker implementations
- `runSchedulerControlWithTimeout` (`cmd/zqk/scheduler/scheduler_core.go`) — labeled goroutine + channel + `select` for wall-clock timeout (see variant above)

---

## On-Demand Worker Pattern

**Version:** 1.0.0  
**Status:** Implemented  
**Purpose:** Resource-efficient background processing with wake-on-work and idle shutdown

### Problem Statement

Traditional worker pools:
- Always consume resources (even when idle)
- Fixed worker count (can't scale down)
- No automatic cleanup

### Solution

**On-Demand Workers**:
- Start workers only when work is available
- Workers shut down after idle timeout
- Dynamic scaling based on workload

### Implementation Pattern

```go
type Component struct {
    activeWorkers int32  // Atomic counter
    workerMu      sync.Mutex
    ctx           context.Context
    cancel        context.CancelFunc
    wg            sync.WaitGroup // Or WaitGroupManager
}

func (c *Component) wakeWorkerIfNeeded() {
    // Check if we need a worker
    if atomic.LoadInt32(&c.activeWorkers) >= c.maxWorkers {
        return
    }

    // Try to start worker (atomic check-and-set)
    c.workerMu.Lock()
    defer c.workerMu.Unlock()

    // Double-check after lock
    if atomic.LoadInt32(&c.activeWorkers) >= c.maxWorkers {
        return
    }

    // Check if there's work
    if c.queue.Peek() == nil {
        return
    }

    // Start worker
    atomic.AddInt32(&c.activeWorkers, 1)
    goroutinelabels.NewGoroutine("worker", "description").
        WithWaitGroup(&c.wg).
        StartSimple(func() {
            c.worker()
        })
}

func (c *Component) worker() {
    defer atomic.AddInt32(&c.activeWorkers, -1)

    idleStartTime := time.Now()
    for {
        select {
        case <-c.ctx.Done():
            return
        default:
            work := c.queue.Dequeue()
            if work == nil {
                // Check idle timeout
                if time.Since(idleStartTime) >= idleTimeout {
                    return // Shut down
                }
                time.Sleep(checkInterval)
                continue
            }

            // Reset idle timer
            idleStartTime = time.Now()

            // Process work
            c.processWork(work)
        }
    }
}
```

### Implementation Requirements

✅ **MUST** use atomic flags for worker state tracking  
✅ **MUST** use `goroutinelabels.NewGoroutine()`  
✅ **MUST** use context for cancellation  
✅ **MUST** use WaitGroup for lifecycle  
✅ **MUST** log lifecycle events  
✅ **MUST** track metrics  
✅ **MUST** integrate with coordinator  
✅ **MUST** handle graceful shutdown

### Key Characteristics

1. **Wake-on-Work**: Workers start when work arrives
2. **Idle Shutdown**: Workers shut down after idle timeout
3. **Bounded Concurrency**: Maximum worker count enforced
4. **Resource Efficient**: No workers when idle

### Examples in Codebase

- `OperationExecutor` (`pkg/storage/operation_executor.go`)
- `HashRegistry` (`pkg/storage/hash_registry.go`)
- `IOQueueManager` (`pkg/storage/io_queue.go`)
- `CASIndexWriteQueue` (`pkg/storage/cas_index_write_queue.go`)
- `IDGenerationQueueManager` (`pkg/storage/id_generation/queue.go`)

### Benefits

- **Resource Efficient**: No idle workers consuming resources
- **Self-Regulating**: Automatically scales to workload
- **Simple**: No complex worker pool management
- **Graceful Shutdown**: Workers complete current work before shutting down

### Related Documentation

- [On-Demand Worker Pattern](./on-demand-worker-pattern.md) - Detailed documentation

---

## Explicit Async Operation Pattern

**Version:** 1.0.0  
**Status:** Proposed (Phase 2)  
**Purpose:** Make async operations explicit and testable

### Problem Statement

Storage operations trigger hidden async operations:
- `storage.Create()` → audit event creation (may be async)
- `storage.Create()` → metrics collection (may be async)
- `storage.Create()` → cache invalidation (may be async)

Caller doesn't know about these, can't coordinate lifecycle, can't test easily.

### Solution

**Explicit Async Operations**:

```go
// StorageConfig makes async explicit
type StorageConfig struct {
    AsyncOperations bool
    AsyncTimeout    time.Duration
}

// Create with explicit async handling
func (f *FileObjectStorage) Create(ctx context.Context, secCtx *SecurityContext, obj map[string]any) error {
    // Synchronous core operation
    if err := f.createSync(ctx, secCtx, obj); err != nil {
        return err
    }

    // Explicit async operations (if enabled)
    if f.config.AsyncOperations {
        f.asyncManager.ScheduleAuditEvent(ctx, secCtx, "create", obj)
        f.asyncManager.ScheduleMetrics(ctx, "create", obj)
    }

    return nil
}
```

### Implementation Requirements

✅ **MUST** separate sync and async operations  
✅ **MUST** make async operations optional via config  
✅ **MUST** use AsyncOperationManager for async work  
✅ **SHOULD** provide test config to disable async  
✅ **SHOULD** track async operations via WaitGroupManager

### Key Characteristics

1. **Explicit**: Caller knows what's async
2. **Optional**: Can disable async for testing
3. **Observable**: Can track async operations
4. **Testable**: Can test sync and async paths separately

### Benefits

- **Clear Intent**: Code shows what's sync vs async
- **Testable**: Can disable async for unit tests
- **Observable**: Can track async operations
- **Predictable**: Caller knows what's async

### Related Documentation

- [Storage Async Architecture Improvements](../../architecture/STORAGE_ASYNC_ARCHITECTURE_IMPROVEMENTS.md)

---

## Async Processing with Synchronous Responses

**Version:** 1.0.0  
**Status:** Implemented  
**Purpose:** Process operations asynchronously while maintaining synchronous response ordering

### Problem Statement

Long-running operations block the server, but protocol requires synchronous responses in order.

### Solution

**Async Processing with Synchronous Responses**:

```
Request → AsyncHandler → Goroutine → Wait for Result → Response
         ↓
    Concurrency Limit
    Timeout Protection
    Cancellation Support
```

**Key Principle**: Process asynchronously, respond synchronously

### Implementation

**Location**: `pkg/mcp/async_handler.go`

```go
type AsyncHandler struct {
    handler       Handler
    maxConcurrent int
    timeout       time.Duration
    activeOps     int
}

func (a *AsyncHandler) Handle(ctx context.Context, method string, params json.RawMessage) (interface{}, error) {
    // Check capacity
    if a.activeOps >= a.maxConcurrent {
        return nil, ErrAtCapacity
    }

    // Execute asynchronously
    resultChan := make(chan interface{}, 1)
    errChan := make(chan error, 1)

    go func() {
        // Process in goroutine
        result, err := a.handler.Handle(ctx, method, params)
        if err != nil {
            errChan <- err
            return
        }
        resultChan <- result
    }()

    // Wait for result (maintains ordering)
    select {
    case result := <-resultChan:
        return result, nil
    case err := <-errChan:
        return nil, err
    case <-time.After(a.timeout):
        return nil, ErrTimeout
    }
}
```

### Implementation Requirements

✅ **MUST** check capacity before starting operation  
✅ **MUST** use goroutines for async execution  
✅ **MUST** wait for result before responding  
✅ **MUST** enforce timeout  
✅ **MUST** handle context cancellation

### Key Characteristics

1. **Non-Blocking**: Operations run in goroutines
2. **Ordered Responses**: Server waits for completion before responding
3. **Concurrency Limits**: Maximum concurrent operations
4. **Timeout Protection**: Operations timeout after max duration

### Examples in Codebase

- MCP Server (`pkg/mcp/async_handler.go`)
- Transceiver Router (`pkg/scheduler/transceiver/async_router.go`)

### Benefits

- **Non-Blocking**: Long operations don't block server
- **Protocol Compliant**: Responses sent in order
- **Resource Bounded**: Concurrency limits prevent overload
- **Timeout Protection**: Operations can't run indefinitely

### Related Documentation

- [MCP Async Handling](./MCP_ASYNC_HANDLING.md)

---

## Worker Pool Pattern

**Version:** 1.0.0  
**Status:** Implemented  
**Purpose:** Manage a pool of workers for concurrent task processing

### Problem Statement

Unbounded goroutines can overwhelm the system. Need bounded concurrency with queue management.

### Solution

**Worker Pool** with:
- Fixed or configurable worker count
- Task queue for buffering
- Graceful shutdown
- Panic recovery

### Implementation Pattern

```go
type WorkerPool struct {
    workers    int
    taskQueue  chan Task
    wg         sync.WaitGroup
    ctx        context.Context
    cancel     context.CancelFunc
}

func (p *WorkerPool) Start() {
    for i := 0; i < p.workers; i++ {
        p.wg.Add(1)
        goroutinelabels.NewGoroutine(fmt.Sprintf("worker_%d", i), "processing tasks").
            WithWaitGroup(&p.wg).
            StartSimple(func() {
                p.worker(i)
            })
    }
}

func (p *WorkerPool) worker(id int) {
    defer p.wg.Done()
    for {
        select {
        case <-p.ctx.Done():
            return
        case task := <-p.taskQueue:
            p.processTask(task)
        }
    }
}

func (p *WorkerPool) Submit(task Task) error {
    select {
    case p.taskQueue <- task:
        return nil
    case <-p.ctx.Done():
        return ErrShutdown
    }
}
```

### Implementation Requirements

✅ **MUST** use bounded worker count  
✅ **MUST** use buffered task queue  
✅ **MUST** use context for cancellation  
✅ **MUST** use WaitGroup for lifecycle  
✅ **MUST** handle graceful shutdown  
✅ **SHOULD** use goroutinelabels for workers

### Key Characteristics

1. **Bounded Concurrency**: Fixed worker count
2. **Queue Buffering**: Tasks queued when all workers busy
3. **Graceful Shutdown**: Workers complete current task before stopping
4. **Panic Recovery**: Workers recover from panics

### Examples in Codebase

- Transceiver AsyncRouter (`pkg/scheduler/transceiver/async_router.go`)
- Validation AsyncValidator (`pkg/validation/async_validator.go`)

### Benefits

- **Bounded Resources**: Fixed worker count prevents overload
- **Queue Management**: Tasks buffered when workers busy
- **Graceful Shutdown**: Workers complete current work
- **Panic Safety**: Workers recover from panics

---

## Graceful Shutdown Pattern

**Version:** 1.0.0  
**Status:** Implemented  
**Purpose:** Ensure clean shutdown with no resource leaks

### Problem Statement

Components need to:
- Shut down cleanly without leaving goroutines running
- Clean up resources (files, connections, timers)
- Handle shutdown from multiple sources (signals, errors, timeouts)
- Prevent deadlocks during shutdown

### Solution

**Atomic Shutdown Pattern** with:
- Atomic shutdown flag (lock-free, thread-safe)
- Context cancellation for notification
- Shutdown hooks for component cleanup
- Process group management for child processes

### Implementation Pattern

```go
type Component struct {
    shutdownFlag  int32              // Atomic: 0 = running, 1 = shutting down
    shutdownCtx   context.Context
    shutdownCancel context.CancelFunc
    shutdownMu    sync.Mutex
    shutdownHooks []ShutdownHook
}

func (c *Component) shutdownSequence(reason string) {
    // Use CompareAndSwapInt32 to ensure shutdown only runs once
    if !atomic.CompareAndSwapInt32(&c.shutdownFlag, 0, 1) {
        return // Already shutting down
    }

    // Cancel context FIRST to notify all components
    c.shutdownCancel()

    // Notify all registered hooks
    for _, hook := range c.shutdownHooks {
        hook(c.shutdownCtx)
    }

    // Cleanup resources
    c.cleanup()
}

func (c *Component) isShuttingDown() bool {
    return atomic.LoadInt32(&c.shutdownFlag) == 1
}
```

### Guard Locations

**CRITICAL**: Check shutdown at:
1. **Function Entry**: Before ANY work
2. **Before Lock Acquisition**: Check before every mutex lock
3. **After Lock Acquisition**: Check again (defensive)
4. **Loop Iterations**: Check on every loop iteration
5. **Before Goroutine Spawn**: Check before spawning any goroutine
6. **Goroutine Entry**: Check at the start of every goroutine

### Implementation Requirements

✅ **MUST** use atomic shutdown flag (CompareAndSwapInt32)  
✅ **MUST** cancel context FIRST (before cleanup)  
✅ **MUST** check shutdown at all guard locations  
✅ **MUST** register shutdown hooks for cleanup  
✅ **MUST** wait for goroutines to complete (with timeout)  
✅ **MUST** cleanup all resources (tickers, channels, files)

### Bounded wait with completion channel

When shutdown must wait for a blocking operation (e.g. pool.Stop() or wg.Wait()) but must not block indefinitely, use a **labeled goroutine + completion channel + select timeout**:

1. Create a completion channel (e.g. `done := make(chan struct{})`).
2. Start the blocking work in a **goroutinelabels**-labeled goroutine; use `WithCleanup(func() { close(done) })` so the channel closes when the goroutine exits.
3. `select` on `<-done` vs `<-time.After(timeout)`; on timeout, log and continue shutdown (the goroutine may still be running in the background).

This gives observability (labeled goroutine), budget tracking when available, and a bounded shutdown. **References:** `pkg/scheduler/transceiver/async_router.go` (Stop), `pkg/scheduler/scheduler.go` (triggered pool stop), [queue-shutdown-management.md](./queue-shutdown-management.md) (expedited shutdown).

### Examples in Codebase

- MCP Server (`pkg/mcp/server.go`)
- ProcessGroupManager (`pkg/mcp/process_group_manager.go`)
- ShutdownHookManager (`pkg/mcp/shutdown_hooks.go`)

### Benefits

- **No Leaks**: All goroutines and resources cleaned up
- **Thread-Safe**: Atomic flags prevent race conditions
- **Deadlock-Free**: Context cancellation before locks
- **Observable**: Shutdown hooks provide visibility

### Related Documentation

- [Bulletproof Shutdown Implementation](../best-practices/atomic-shutdown-flags.md)
- [Goroutine Leak Prevention Checklist](./mcp-goroutine-leak-prevention-checklist.md)

---

## Context Lifecycle Management Pattern

**Version:** 1.0.0  
**Status:** Implemented  
**Purpose:** Proper context creation, propagation, and cancellation

### Problem Statement

Contexts need to:
- Be created with proper parent relationships
- Propagate through call chains
- Be cancelled at the right time
- Have timeouts for long operations

### Solution

**Context Lifecycle Management**:
- Create contexts with `context.WithCancel()` or `context.WithTimeout()`
- Store cancel functions for cleanup
- Propagate contexts through all function calls
- Check `ctx.Done()` in all loops and long operations

### Implementation Pattern

```go
type Component struct {
    ctx    context.Context
    cancel context.CancelFunc
}

func NewComponent() *Component {
    ctx, cancel := context.WithCancel(context.Background())
    return &Component{
        ctx:    ctx,
        cancel: cancel,
    }
}

func (c *Component) StartWorker() {
    goroutinelabels.NewGoroutine("worker", "processing").
        WithContext(c.ctx).
        StartWithContext(func(ctx context.Context) error {
            for {
                select {
                case <-ctx.Done():
                    return ctx.Err()
                case work := <-c.queue:
                    c.processWork(ctx, work)
                }
            }
        })
}

func (c *Component) Shutdown() {
    c.cancel() // Cancel context to stop all workers
}
```

### Implementation Requirements

✅ **MUST** accept `context.Context` as first parameter  
✅ **MUST** check `ctx.Done()` in all loops  
✅ **MUST** store cancel functions for cleanup  
✅ **MUST** use `context.WithTimeout()` for operations with deadlines  
✅ **MUST** propagate context through call chains  
✅ **SHOULD** use `context.WithValue()` sparingly (only for request-scoped data)

### Key Characteristics

1. **Context as First Parameter**: All functions that might block accept context
2. **Cancellation Checking**: All loops check `ctx.Done()`
3. **Timeout Enforcement**: Long operations use `context.WithTimeout()`
4. **Proper Propagation**: Context flows through call chains

### Examples in Codebase

- All async operations use context
- MCP server operations
- Storage operations
- Validation operations

### Benefits

- **Cancellable**: Operations can be cancelled
- **Timeout Protection**: Operations can't run indefinitely
- **Resource Cleanup**: Context cancellation triggers cleanup
- **Composable**: Contexts can be nested and combined

---

## Retry with Exponential Backoff Pattern

**Version:** 1.0.0  
**Status:** Implemented  
**Purpose:** Handle transient failures with configurable retry logic and exponential backoff

### Problem Statement

Operations can fail due to transient errors:
- Network timeouts
- Temporary resource unavailability
- Version conflicts (optimistic locking)
- Context deadline exceeded

Need a consistent pattern for retrying these operations.

### Solution

**Retry with Exponential Backoff**:
- Configurable max attempts
- Exponential backoff between retries
- Context-aware (respects cancellation)
- Retryable error detection

### Implementation Pattern

```go
type RetryConfig struct {
    MaxAttempts   int           // Default: 3
    InitialDelay  time.Duration // Default: 100ms
    MaxDelay      time.Duration // Default: 5s
    BackoffFactor float64       // Default: 2.0 (exponential)
}

func ExecuteWithRetry(ctx context.Context, config *RetryConfig, fn func() error) error {
    var lastErr error
    delay := config.InitialDelay

    for attempt := 0; attempt < config.MaxAttempts; attempt++ {
        // Check context cancellation
        if ctx.Err() != nil {
            return ctx.Err()
        }

        // Execute the operation
        err := fn()
        if err == nil {
            return nil
        }

        lastErr = err

        // Check if error is retryable
        if !isRetryableError(err) {
            return err
        }

        // Last attempt, don't wait
        if attempt == config.MaxAttempts-1 {
            break
        }

        // Wait before retry with exponential backoff
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(delay):
            // Continue to next attempt
        }

        // Exponential backoff
        delay = time.Duration(float64(delay) * config.BackoffFactor)
        if delay > config.MaxDelay {
            delay = config.MaxDelay
        }
    }

    return fmt.Errorf("operation failed after %d attempts: %w", config.MaxAttempts, lastErr)
}
```

### Retryable Errors

**Retryable**:
- Context/timeout errors (`context.DeadlineExceeded`, `context.Canceled`)
- Network/connection errors
- Version conflicts (optimistic locking)
- Temporary errors

**Non-Retryable**:
- Permission denied
- Object not found (for create operations)
- Validation errors
- Permanent errors

### Implementation Requirements

✅ **MUST** check context cancellation before each attempt  
✅ **MUST** check if error is retryable before retrying  
✅ **MUST** use exponential backoff  
✅ **MUST** respect max delay  
✅ **MUST** return error after max attempts exhausted  
✅ **SHOULD** log retry attempts for observability

### Key Characteristics

1. **Exponential Backoff**: Delay increases exponentially between retries
2. **Context Aware**: Respects context cancellation
3. **Retryable Detection**: Only retries on retryable errors
4. **Configurable**: Max attempts, delays, backoff factor all configurable

### Examples in Codebase

- `OperationExecutor` (`pkg/storage/operation_executor.go`)
- `ExecuteSimpleRetry` (`pkg/storage/operation_helper.go`)
- Graph Provider Retry (`pkg/graph/provider/retry.go`)

### Benefits

- **Resilient**: Handles transient failures automatically
- **Efficient**: Exponential backoff prevents overwhelming system
- **Cancellable**: Respects context cancellation
- **Configurable**: Can tune for different operation types

---

## Write-Behind Cache Update Buffer Pattern

**Version:** 1.0.0  
**Status:** Implemented  
**Purpose:** Reduce contention on concurrent cache updates by batching writes instead of blocking on frequent `sync.Map.Store()` calls

### Problem Statement

When many goroutines update a cache concurrently (e.g., `sync.Map.Store()`), they contend for internal locks, causing:
- **Thread blocking**: Goroutines block on condition variables (`pthread_cond_wait`)
- **Reduced throughput**: Frequent lock acquisition/release overhead
- **Hangs under load**: High contention can cause processes to hang (e.g., `system generate-builders` with 14k+ objects)
- **Poor scalability**: Performance degrades linearly with concurrent updates

### Solution

**Write-Behind Cache Update Buffer**:
- Queue cache updates instead of applying them immediately
- Background worker batches updates and applies them in bursts
- Reduces contention by grouping updates together
- Blocks on enqueue (provides back-pressure) rather than on cache operations

### Implementation

**Location**: `pkg/objects/spec_cache_update_buffer.go`

**Core Components**:
```go
type SpecCacheUpdateBuffer struct {
    updates   chan *CacheUpdate  // Buffered channel for queued updates
    batchSize int                 // Number of updates per batch
    interval  time.Duration       // Time-based flush interval
    // ... worker state
}

type CacheUpdate struct {
    Type      CacheUpdateType  // Type of cache update (spec, path, ontology, file_content)
    Key       string
    Value     any
    Shard     *specShard       // Target shard for the update
    Timestamp time.Time
}
```

**Usage Pattern**:
```go
// Create buffer with configuration
buffer := NewSpecCacheUpdateBuffer(
    10000,              // Buffer size (handles bursts)
    100,                // Batch size (updates per batch)
    10*time.Millisecond // Flush interval (time-based flush)
)

// Start background worker
ctx := pkgctx.NewSystemContext()
buffer.Start(ctx)
defer buffer.Stop()

// Queue updates instead of direct Store()
update := &CacheUpdate{
    Type:      CacheUpdateSpec,
    Key:       ontology,
    Value:     cached,
    Shard:     shard,
    Timestamp: time.Now(),
}
buffer.Enqueue(update) // Non-blocking: falls back to direct Store() if buffer full
```

**Worker Implementation**:
```go
func (b *SpecCacheUpdateBuffer) worker(ctx context.Context) {
    ticker := time.NewTicker(b.interval)
    defer ticker.Stop()

    batch := make([]*CacheUpdate, 0, b.batchSize)

    for {
        select {
        case <-ctx.Done():
            b.applyBatch(batch) // Flush remaining
            return
        case update := <-b.updates:
            batch = append(batch, update)
            if len(batch) >= b.batchSize {
                b.applyBatch(batch) // Size-based flush
                batch = batch[:0]
            }
        case <-ticker.C:
            if len(batch) > 0 {
                b.applyBatch(batch) // Time-based flush
                batch = batch[:0]
            }
        }
    }
}

func (b *SpecCacheUpdateBuffer) applyBatch(batch []*CacheUpdate) {
    // Group updates by shard to reduce contention
    shardUpdates := make(map[*specShard][]*CacheUpdate)
    for _, update := range batch {
        shardUpdates[update.Shard] = append(shardUpdates[update.Shard], update)
    }

    // Apply updates per shard (reduces sync.Map contention)
    for shard, updates := range shardUpdates {
        for _, update := range updates {
            b.applyUpdateToShard(shard, update)
        }
    }
}
```

### Implementation Requirements

✅ **MUST** use buffered channel for queued updates  
✅ **MUST** block on `Enqueue()` (not on cache operations)  
✅ **MUST** batch updates by size and time  
✅ **MUST** group updates by shard before applying  
✅ **MUST** use `goroutinelabels.NewGoroutine()` for worker  
✅ **MUST** use context for cancellation  
✅ **MUST** flush remaining batch on shutdown  
✅ **SHOULD** use large buffer size (10k+) to handle bursts  
✅ **SHOULD** use reasonable batch size (100-1000) for efficiency  
✅ **SHOULD** use short flush interval (10-50ms) for responsiveness

### Key Characteristics

1. **Non-Blocking Enqueue**: Updates are queued quickly (non-blocking channel send)
2. **Batched Application**: Updates applied in groups (reduces contention)
3. **Shard Grouping**: Updates grouped by shard before applying (further reduces contention)
4. **Dual Flush Triggers**: Size-based (batch full) and time-based (interval)
5. **Fallback on Full Buffer**: Falls back to direct Store() if buffer is full (prevents goroutine blocking)

### Benefits

- **Reduced Contention**: Batched updates reduce lock acquisition frequency
- **Better Scalability**: Performance improves with concurrent load
- **Prevents Hangs**: Eliminates thread blocking on condition variables
- **Resource Efficient**: Background worker processes batches asynchronously
- **Predictable**: Back-pressure prevents unbounded memory growth

### When to Use

✅ **Use when:**
- Many goroutines update cache concurrently
- Cache operations (`sync.Map.Store()`) show contention
- Process hangs under high concurrent load
- Cache updates can be delayed slightly (write-behind)

❌ **Avoid when:**
- Cache reads must see updates immediately (use direct `Store()`)
- Updates are infrequent (overhead not justified)
- Latency requirements are strict (< 10ms)

### Configuration Guidelines

**Buffer Size**: 
- Small workloads (< 100 concurrent): 1,000-5,000
- Medium workloads (100-1,000 concurrent): 5,000-10,000
- Large workloads (> 1,000 concurrent): 10,000-50,000

**Batch Size**:
- Small batches (low latency): 10-50
- Medium batches (balanced): 100-500
- Large batches (high throughput): 500-1,000

**Flush Interval**:
- Low latency: 5-10ms
- Balanced: 10-50ms
- High throughput: 50-100ms

### Examples in Codebase

- **SpecLoader** (`pkg/objects/spec_loader.go`):
  - Reduces contention during `system generate-builders` (14k+ objects)
  - Prevents hangs from thread blocking on `sync.Map.Store()`
  - Batches spec cache, path cache, ontology cache, and file content cache updates

### Testing

Comprehensive test suite in `pkg/objects/spec_cache_update_buffer_test.go`:
- `TestSpecCacheUpdateBuffer_Batching`: Verifies batching behavior
- `TestSpecCacheUpdateBuffer_TimeBasedFlush`: Verifies time-based flushing
- `TestSpecCacheUpdateBuffer_HighContention`: Simulates 5,000+ concurrent updates
- `TestSpecCacheUpdateBuffer_ShardGrouping`: Verifies shard grouping
- `TestSpecCacheUpdateBuffer_ConcurrentEnqueue`: Tests thread-safety
- `TestSpecCacheUpdateBuffer_IntegrationWithSpecLoader`: Integration test

### Related Patterns

- **On-Demand Worker Pattern**: Background worker processes batches
- **Batch Processing**: Updates are processed in batches
- **Write-Behind Pattern**: Updates queued and applied asynchronously
- **Sharding Pattern**: Updates grouped by shard to reduce contention

### Migration Example

**Before** (Direct `Store()` - High Contention):
```go
// Many goroutines calling this concurrently causes contention
shard.cache.Store(ontology, cached)
shard.pathCache.Store(specPath, cached)
```

**After** (Write-Behind Buffer - Reduced Contention):
```go
// Queue updates instead of blocking on Store()
sl.queueCacheUpdate(shard, CacheUpdateSpec, ontology, cached)
sl.queueCacheUpdate(pathShard, CacheUpdatePath, specPath, cached)
```

### Performance Impact

**Before**: 
- Thread blocking on `pthread_cond_wait` under high load
- Process hangs (e.g., `system generate-builders` exceeds 10-minute timeout)
- Linear performance degradation with concurrent updates

**After**:
- No thread blocking (updates queued quickly)
- Process completes successfully under high load
- Batched updates reduce contention by 10-100x

---

## Cache sidecar idle coordination (object ID / reverse reference background)

**Version:** 1.0.0  
**Status:** Implemented  
**Purpose:** Deterministic join and **observable** transitions when optional background work for the object ID cache and async reverse-reference index finishes for a given `projectRoot`.

### Traceability (code)

| Piece | Location |
|-------|-----------|
| In-flight counters, `sync.Cond`, `WaitProjectCacheBackgroundWork`, `RegisterCacheSidecarsIdleCallback` | `cmd/zqk/system/cache_background_wait.go` |
| Coordinator emit `cache_sidecars` / `cache_event: cache_sidecars_idle` | `cmd/zqk/system/cache_coordination.go` (`emitCacheSidecarsIdleViaCoordinator`) |
| Cond + `context` wait primitive | `pkg/concurrency/wait_cond_context.go` (`WaitCondContext`) |
| Background triggers (object ID / reverse ref) | `cmd/zqk/system/check_cache.go` (`triggerBackgroundObjectIDCacheBuild`, `triggerBackgroundReverseReferenceIndexBuild`) |

### Three layers (do not conflate)

1. **Barrier** — `WaitProjectCacheBackgroundWork` blocks until sidecar counters for that root reach zero. Use for teardown and any logic that must not run until background work has finished.
2. **In-process hooks** — `RegisterCacheSidecarsIdleCallback` runs **synchronously** after the idle transition (outside the sidecar mutex). For short chaining only; it does **not** replace the barrier.
3. **Coordinator / subscribers** — `emitCacheSidecarsIdleViaCoordinator` is **async** (goroutine + `Emit`). For logging, metrics, and subscribers that react off the event bus. Do not use it as a synchronous barrier.

### Avoiding feedback loops and cycles

Idle notifications are **edge-triggered** (transition into all-zero for that root). Orchestration built on top must stay **acyclic** (e.g. stage A idle → schedule stage B), not a closed loop that can recurse without a new external stimulus.

**Do not:**

- Call `TriggerBackgroundObjectIDCacheBuild`, `TriggerBackgroundObjectIDCacheForceRebuild`, or any path that increments the same root’s sidecar counters **synchronously** from inside `RegisterCacheSidecarsIdleCallback` for **that** `projectRoot`. That can re-enter the tracker while idle handlers still run, confuse waiters, and stack recursive idle → work → idle behavior.
- Handle `cache_sidecars_idle` (or equivalent operational channel) with code that **synchronously** waits on `WaitProjectCacheBackgroundWork` for the same root and then immediately schedules work that completes and invokes the same handler again in the same call chain without an **async boundary** (queue, separate goroutine, job with explicit generation/one-shot token).

**Do:**

- If follow-up work must start another background build for the same root, **post** that work to another goroutine or work queue, and use a **generation**, **one-shot**, or **state machine** so the same idle edge cannot reschedule itself indefinitely.
- Treat coordinator-driven orchestration as **at-least-once / async**: subscribers must tolerate ordering and duplication relative to other events.

### Related primitives

- [Wall-clock timeout multiplexing (blocking fn)](#wall-clock-timeout-multiplexing-blocking-fn) — different problem (cap wait on blocking I/O).
- `pkg/concurrency/README.md` — § *sync.Cond with context* (`WaitCondContext`).

---

## Pattern Selection Guide

### When to Use Each Pattern

| Pattern | Use When | Example |
|---------|----------|---------|
| **WaitGroup Lifecycle Management** | Need to track multiple WaitGroups, want observability | OperationExecutor, multiple async operations |
| **Goroutine Builder** | Creating any goroutine | All goroutine creation |
| **On-Demand Worker** | Workload is variable, want resource efficiency | HashRegistry, CASIndexWriteQueue |
| **Explicit Async Operation** | Need to make async operations visible and testable | Storage operations with audit/metrics |
| **Async Processing with Sync Responses** | Protocol requires ordered responses, but operations are long | MCP server, API handlers |
| **Worker Pool** | Fixed concurrency needs, bounded resource usage | Transceiver router, validation |
| **Graceful Shutdown** | Component needs clean shutdown | All long-lived components |
| **Context Lifecycle Management** | Operations need cancellation or timeout | All async operations |
| **Retry with Exponential Backoff** | Operations may fail transiently | I/O operations, network calls |
| **Write-Behind Cache Update Buffer** | Many concurrent cache updates causing contention | Spec loading, cache population under high load |
| **Cache sidecar idle coordination** | Tests or components must join background object-id / reverse-ref work; optional observability without polling | `WaitProjectCacheBackgroundWork` in teardown; coordinator `cache_sidecars_idle` for metrics/subscribers |

### Pattern Combinations

- **On-Demand Worker + WaitGroupManager**: On-demand workers with lifecycle tracking
- **Worker Pool + WaitGroupManager**: Worker pool with observability
- **Explicit Async + WaitGroupManager**: Async operations with coordination
- **Goroutine Builder + Context Lifecycle**: Safe goroutines with cancellation
- **Graceful Shutdown + Context Lifecycle**: Clean shutdown with cancellation
- **Retry + Context Lifecycle**: Resilient operations with timeout protection
- **Retry + WaitGroupManager**: Retry operations with lifecycle tracking
- **Write-Behind Cache Update Buffer + On-Demand Worker**: Batched cache updates with resource-efficient worker
- **Cache sidecar idle + WaitCondContext**: Deterministic barrier on cond; coordinator emit for async observers only (no cyclic sync handoff)

---

## Best Practices

### 1. Always Use GoroutineBuilder for Goroutines

**Bad**:
```go
go func() {
    // No tracking, no panic recovery
    doWork()
}()
```

**Good**:
```go
goroutinelabels.NewGoroutine("worker", "description").
    WithContext(ctx).
    WithWaitGroup(&wg).
    StartSimple(func() {
        doWork()
    })
```

### 2. Always Use WaitGroupManager for Multiple WaitGroups

**Bad**:
```go
var wg1, wg2, wg3 sync.WaitGroup
wg1.Add(1)
wg2.Add(1)
// Hard to track, easy to make mistakes
```

**Good**:
```go
manager := NewWaitGroupManager()
wg1 := manager.CreateGroup("op1", "create")
wg2 := manager.CreateGroup("op2", "update")
// Centralized tracking, observable
```

### 3. Always Check Context in Loops

**Bad**:
```go
for {
    work := queue.Dequeue()
    process(work)
}
```

**Good**:
```go
for {
    select {
    case <-ctx.Done():
        return ctx.Err()
    case work := <-queue:
        process(work)
    }
}
```

### 4. Always Use Atomic Shutdown Flags

**Bad**:
```go
var shutdown bool
mu.Lock()
shutdown = true
mu.Unlock()
```

**Good**:
```go
var shutdownFlag int32
if !atomic.CompareAndSwapInt32(&shutdownFlag, 0, 1) {
    return // Already shutting down
}
```

### 5. Always Clean Up Resources

**Bad**:
```go
ticker := time.NewTicker(interval)
go func() {
    for range ticker.C {
        // No cleanup!
    }
}()
```

**Good**:
```go
ticker := time.NewTicker(interval)
defer ticker.Stop()
goroutinelabels.NewGoroutine("periodic", "periodic task").
    WithContext(ctx).
    StartWithContext(func(ctx context.Context) error {
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-ticker.C:
                doWork()
            }
        }
    })
```

---

## Migration Checklist

When migrating to these patterns:

- [ ] Identify all `sync.WaitGroup` usage
- [ ] Create `WaitGroupManager` in component
- [ ] Replace direct WaitGroup with manager
- [ ] Replace all `go func()` with `goroutinelabels.NewGoroutine()`
- [ ] Add context parameters to all async functions
- [ ] Add shutdown flags to long-lived components
- [ ] Add observer for tracking (optional)
- [ ] Update tests to use manager
- [ ] Document pattern usage in code comments

---

## References

- **On-Demand Worker Pattern**: [on-demand-worker-pattern.md](./on-demand-worker-pattern.md)
- **MCP Async Handling**: [MCP_ASYNC_HANDLING.md](./MCP_ASYNC_HANDLING.md)
- **Storage Async Architecture**: [STORAGE_ASYNC_ARCHITECTURE_IMPROVEMENTS.md](../../architecture/STORAGE_ASYNC_ARCHITECTURE_IMPROVEMENTS.md)
- **Transceiver Async Architecture**: [TRANSCEIVER_ASYNC_ARCHITECTURE.md](./TRANSCEIVER_ASYNC_ARCHITECTURE.md)
- **Goroutine Builder**: [pkg/goroutinelabels/README.md](../../pkg/goroutinelabels/README.md)
- **Goroutine Leak Prevention**: [mcp-goroutine-leak-prevention-checklist.md](./mcp-goroutine-leak-prevention-checklist.md)
- **Bulletproof Shutdown**: [pkg/mcp/BULLETPROOF_SHUTDOWN.md](../../pkg/mcp/BULLETPROOF_SHUTDOWN.md)
