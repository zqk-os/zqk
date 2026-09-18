# Concurrency Package

This package provides standardized concurrency primitives and patterns to ensure consistent, safe, and observable concurrent operations across the codebase.

## Overview

The `pkg/concurrency` package addresses critical concurrency concerns:
- **Deadlock prevention** via timeout wrappers for all mutex operations
- **Standard callback/notify pattern** for operation lifecycle tracking
- **Smart defaults** for worker counts and concurrency parameters
- **Observability** via metrics and logging integration

## Core Components

### 1. Run with max wait (RunWithMaxWait) — Cap wait on fire-and-forget

**Use when**: You must run a function (e.g. writing to stdout/pipe) but must not block the current goroutine indefinitely if that operation blocks (e.g. broken pipe, full buffer). The function runs in a new goroutine; the caller returns when the function completes or when `maxWait` elapses, whichever comes first.

**Note**: The function is not cancelled when the timeout elapses; it may still complete in the background. This pattern only caps how long the *caller* waits.

```go
concurrency.RunWithMaxWait(func() {
    fmt.Fprintf(w, "Scheduler started (PID: %d).\n", pid)
}, 2*time.Second)
```

See `run_with_max_wait.go` for full documentation.

### 2. Transactional Critical Sections (RunInLock / RunInRLock) — Preferred for shared state

**Problem**: Lock helpers that run the callback in a *different* goroutine (e.g. for timeout detection) leave the lock held by the caller while the callback runs without the lock, causing data races when the callback touches shared state.

**Solution**: Use **RunInLock** / **RunInRLock** for any critical section that reads or modifies shared state. The callback runs in the **same goroutine** that holds the lock, so the "transaction" (lock → modify → unlock) is atomic and no other goroutine can see half-updated state.

- **Single, predictable pattern**: one place owns the lock; no scattering of raw `Lock()`/`Unlock()`.
- **Deadlock avoidance**: establish a global lock order when multiple locks are needed (e.g. always A before B); prefer holding one lock at a time and copying data out when possible. See **Lock ordering** in [docs/architecture/LOCK_ORDERING.md](../../docs/architecture/LOCK_ORDERING.md).

```go
// Preferred: same-goroutine critical section
err := concurrency.RunInLock(&buf.mu, func() error {
    buf.items[key] = value
    return nil
})

// Read-only
err := concurrency.RunInRLock(&buf.mu, func() error {
    v = buf.items[key]
    return nil
})
```

**With optional logging** (when you want to see long-held locks):

```go
err := concurrency.RunInLockWithLogger(&buf.mu, "audit_buffer_flush_copy", logging.GetLockLoggerFromProfile("system"), func() error {
    // copy shared state
    return nil
})
```

**When to use RunInLock / RunInRLock**:
- ✅ Any read or write to shared state protected by a mutex
- ✅ When you need a clear "transaction": read/copy under lock, then release before I/O or callbacks

**When to use WithLockTimeout / WithLockCtxLogger instead**:
- Only when you need timeout detection and your callback **does not** touch the same shared state (e.g. the callback only does logging). **Note**: WithLockTimeout runs the callback in a separate goroutine; the lock is held by the caller, so the callback does *not* run with the lock. Do not use it for critical sections that modify shared state.

### 2b. sync.Cond with context (`WaitCondContext`)

**Problem**: You cannot `select` on `sync.Cond`. Spawning a goroutine only to call `cond.Wait` so the parent can `select` on `ctx.Done()` is easy to get wrong (stray goroutines, misleading docs about locks) and ignores the same-goroutine lock rule above.

**Solution**: Wait in the **calling** goroutine with `mu` held, in a `for !ready() { cond.Wait() }` loop. Register [`context.AfterFunc`](https://pkg.go.dev/context#AfterFunc) so that when `ctx` is cancelled, a one-shot callback acquires `mu`, calls `cond.Broadcast()`, and releases `mu`. The waiter then wakes, observes `ctx.Err()`, and returns without an extra waiter goroutine.

**Use** [`WaitCondContext`](wait_cond_context.go). Example call site: `cmd/zqk/system.WaitProjectCacheBackgroundWork`.

**Architecture / anti-cycles:** `docs/architecture/concurrency-patterns-v1.0.md` § *Cache sidecar idle coordination* documents wait vs callback vs coordinator layers and rules to avoid feedback loops when reacting to idle edges.

---

### 3. Timeout Mutex Wrappers (WithLock* / WithRLock*)

**Problem**: Direct mutex operations can deadlock indefinitely with no visibility.

**Solution**: Standardized convenience overloads provide deterministic timeouts and observability. **Important**: These run the callback in a separate goroutine for timeout monitoring; use **RunInLock** for critical sections that modify shared state.

**Implementation note (WithLockTimeout / WithRLockTimeout)**: The inner operation runs via `goroutinelabels.GoroutineBuilder` (established pattern; see `docs/architecture/concurrency-patterns-v1.0.md`). The builder is used with `WithCleanup` so that a completion channel is always signaled when the inner goroutine exits (normal return, panic, or context cancel). That ensures the caller releases the lock promptly instead of blocking until the operation timeout. This enhancement keeps goroutine usage consistent with project patterns while fixing the case where a panic or early exit would otherwise leave the caller holding the lock until the timeout fired.

#### Standard Pattern (when timeout is required and callback does not touch shared state)

**For most production code** - use `WithLockCtxLogger` / `WithRLockCtxLogger`:

```go
import (
    "github.com/lanceman/zqk/pkg/concurrency"
    "github.com/lanceman/zqk/pkg/logging"
)

// Standard pattern: context + logger (no metrics boilerplate)
err := concurrency.WithLockCtxLogger(
    &myStruct.mu,
    ctx,
    "operation_name",
    logging.GetLockLoggerFromProfile("system"),
    func() error {
        // Critical section - guaranteed to complete or timeout
        myStruct.data = newValue
        return nil
    },
)
if err != nil {
    // Handle timeout (deadlock prevented)
    return fmt.Errorf("operation timed out: %w", err)
}
```

**For read locks**:

```go
err := concurrency.WithRLockCtxLogger(
    &myStruct.mu,
    ctx,
    "read_operation",
    logging.GetLockLoggerFromProfile("system"),
    func() error {
        // Read-only critical section
        value := myStruct.data
        return nil
    },
)
```

#### Import Cycle Avoidance Pattern

**For packages that cannot import `pkg/logging`** (e.g., `pkg/context`, `pkg/logging` itself) - use `WithLockCtx` / `WithRLockCtx`:

```go
import "github.com/lanceman/zqk/pkg/concurrency"

// No logger to avoid import cycles
err := concurrency.WithLockCtx(
    &myStruct.mu,
    ctx,
    "operation_name",
    func() error {
        myStruct.data = newValue
        return nil
    },
)
```

#### Available Overloads

The package provides multiple convenience overloads to eliminate boilerplate:

- **`WithLockCtxLogger` / `WithRLockCtxLogger`** ⭐ **Primary pattern** - context + logger (most production code)
- **`WithLockCtx` / `WithRLockCtx`** - context only (import cycle avoidance)
- **`WithLockLogger` / `WithRLockLogger`** - logger only, uses `context.Background()`
- **`WithLock` / `WithRLock`** - minimal, uses `context.Background()` and no logger
- **`WithLockTimeout` / `WithRLockTimeout`** - full control (legacy/internal use only)

**Note**: The verbose `WithLockTimeout` with all parameters is still available for special cases, but the convenience overloads should be preferred for cleaner, more readable code.

**When to Use**:
- ✅ High-contention operations (caches, shared state)
- ✅ Operations that might hold locks during I/O
- ✅ Operations in critical paths (validation, scheduling)

**When NOT to Use**:
- ❌ Simple getters/setters (just field assignments, < 1ms)
- ❌ Operations that already release locks before I/O
- ❌ Operations guaranteed to be very fast

### 4. Operation Callback Interface

**Problem**: No standard pattern for tracking long-running operation lifecycles.

**Solution**: `OperationCallback` interface provides consistent lifecycle tracking.

```go
type OperationCallback interface {
    OnStart(operationID string, metadata map[string]any)
    OnProgress(operationID string, progress int, total int, message string)
    OnComplete(operationID string, result any, duration time.Duration)
    OnError(operationID string, err error)
    OnCancel(operationID string, reason string)
}
```

**Implementations**:
- `NoOpOperationCallback`: No-op implementation for testing/fallback
- `CoordinatorOperationCallback`: Coordinator-integrated implementation (in `pkg/coordination`)

**Usage**:
```go
import (
    "github.com/lanceman/zqk/pkg/concurrency"
    "github.com/lanceman/zqk/pkg/coordination"
)

// Create callback (coordinator-integrated)
callback := coordination.NewCoordinatorOperationCallback(
    ctx,
    projectRoot,
    storageProvider,
    "my_operation",
    "human", // profile
)

// Use in operation
callback.OnStart(operationID, map[string]any{
    "total_items": 100,
})

// ... do work ...

callback.OnComplete(operationID, result, duration)
```

**Note**: `CoordinatorOperationCallback` is in `pkg/coordination` to avoid import cycles. The interface is defined here in `pkg/concurrency`.

### 5. Goroutine Ceiling (Circuit Breaker)

**Problem**: Unbounded goroutine growth (e.g. many cron jobs or list workers) can make the process unresponsive; we want to block *new* work until existing work catches up.

**Solution**: Use `WaitUnderGoroutineCeiling` at "gate" points (e.g. before submitting to the triggered-job pool, or before acquiring a list slot). When `runtime.NumGoroutine()` is already at or above the ceiling (default 2000), the call blocks until the count drops or the context is cancelled. This prevents adding more work when the process is already overloaded.

```go
import "github.com/lanceman/zqk/pkg/concurrency"

// Before submitting work that would add goroutines
if err := concurrency.WaitUnderGoroutineCeiling(ctx, concurrency.DefaultGoroutineCeiling, 200*time.Millisecond); err != nil {
    return err // e.g. context cancelled
}
// ... submit to channel, acquire slot, etc.
```

**Constants**:
- `DefaultGoroutineCeiling` (2000): block new work when at or above this count.
- `GoroutineCountWarningThreshold` (1500): health monitor logs a warning when count exceeds this so operators see "things slowing down" before the ceiling blocks work.

**When to use**: At any point where you're about to add work that creates or triggers many goroutines (job submission, starting a list/count operation). Use together with bounded pools so each subsystem has a cap *and* a global ceiling acts as a safety net.

### 6. Smart Defaults Configuration

**Problem**: Hardcoded worker counts don't adapt to system resources.

**Solution**: CPU-aware defaults with global configuration.

```go
import "github.com/lanceman/zqk/pkg/concurrency"

// Get smart defaults (CPU-aware)
cfg := concurrency.GetGlobalConcurrencyConfig()
validatorWorkers := cfg.ValidatorMaxWorkers  // NumCPU * 2, clamped [4, 32]
routerWorkers := cfg.AsyncRouterMaxWorkers    // NumCPU, clamped [2, 16]

// Override defaults (for CLI, tests, etc.)
concurrency.SetGlobalConcurrencyConfig(&concurrency.ConcurrencyConfig{
    ValidatorMaxWorkers: 16,
    AsyncRouterMaxWorkers: 8,
})
```

**Defaults**:
- `ValidatorMaxWorkers`: `runtime.NumCPU() * 2`, clamped to [4, 32]
- `AsyncRouterMaxWorkers`: `runtime.NumCPU()`, clamped to [2, 16]

## Architecture Decisions

### Import Cycle Resolution

**Problem**: `pkg/concurrency` needed `pkg/storage` for coordinator callbacks, but `pkg/storage` also needed `pkg/concurrency` for timeout wrappers.

**Solution**: 
- Keep `OperationCallback` interface in `pkg/concurrency` (no dependencies)
- Move `CoordinatorOperationCallback` to `pkg/coordination` (can import both)
- Result: Clean dependency graph with no cycles

```
pkg/concurrency → (no storage/coordination deps)
pkg/coordination → pkg/concurrency + pkg/storage
pkg/storage → pkg/concurrency (no cycles!)
```

### Timeout Calculation

Timeouts are calculated based on:
1. Operation type (validation, scheduling, etc.)
2. Optional metrics (if available, uses historical data)
3. Default fallback (5 seconds for most operations)

This ensures timeouts are appropriate for the operation while preventing indefinite hangs.

### LockMetrics Interface

Optional metrics interface for lock observability:

```go
type LockMetrics interface {
    GetObjectsPerSecond() float64
    RecordLockWait(duration time.Duration)
    RecordLockHold(duration time.Duration)
}
```

Implemented by `pkg/validation/ValidationMetrics` for validation operations.

## Testing

### RecordingOperationCallback

For tests that don't need coordinator/storage dependencies:

```go
import "github.com/lanceman/zqk/pkg/concurrency"

recorder := concurrency.NewRecordingOperationCallback()

// Use in operation
operation.SetCallback(recorder)

// ... execute operation ...

// Verify callbacks
recorder.AssertStartEventCount(t, 1)
recorder.AssertCompleteEventCount(t, 1)
events := recorder.GetStartEvents()
// ... verify event data ...
```

This avoids import cycles in tests while providing full verification capabilities.

## Integration Examples

### Async Validator

```go
// Uses global config for worker count
cfg := concurrency.GetGlobalConcurrencyConfig()
validator := validation.NewAsyncValidator(projectRoot, cfg.ValidatorMaxWorkers, timeout)
```

### Async Router

```go
// Uses global config for worker count
cfg := concurrency.GetGlobalConcurrencyConfig()
router := transceiver.NewAsyncRouter(router, cfg.AsyncRouterMaxWorkers, queueSize, logger)
```

### System Check Command

```go
// High-level operation callback for lifecycle tracking
callback := coordination.NewCoordinatorOperationCallback(
    ctx, projectRoot, storageProvider, "system_check", profile,
)

callback.OnStart(operationID, map[string]any{
    "total_tasks": totalTasks,
})

// ... validation work ...

callback.OnComplete(operationID, result, duration)
```

## Best Practices

1. **Always use timeout wrappers** for high-contention or I/O-holding operations
2. **Prefer convenience overloads** - use `WithLockCtxLogger` / `WithRLockCtxLogger` for most production code
3. **Avoid verbose patterns** - don't use `WithLockTimeout` with all parameters unless absolutely necessary
4. **Use `WithLockCtx` / `WithRLockCtx`** when logger would cause import cycles (e.g., in `pkg/context` or `pkg/logging`)
5. **Use operation callbacks** for long-running operations that need lifecycle tracking
6. **Leverage smart defaults** - only override when necessary
7. **Follow import cycle patterns** - keep interfaces in `pkg/concurrency`, implementations in `pkg/coordination`
8. **Use RecordingOperationCallback** in tests to avoid coordinator dependencies

## Standardization Status

**✅ Complete**: All packages now use standardized concurrency patterns:
- **Storage**: 76 occurrences standardized
- **Validation**: ~60 occurrences standardized
- **Objects**: 4 occurrences standardized
- **Logging**: ~23 occurrences standardized (uses `WithLockCtx`/`WithRLockCtx` for import cycle avoidance)
- **Graph**: ~39 occurrences standardized
- **Scheduler**: 83 occurrences standardized
- **MCP**: 102 occurrences standardized
- **Runtime, Coordination, CLI, Config, and others**: ~77 occurrences standardized

**Total**: ~464 occurrences across ~103 files now use the standardized convenience overloads, eliminating boilerplate and improving code readability.

## Related Documentation

- [docs/architecture/LOCK_ORDERING.md](../../docs/architecture/LOCK_ORDERING.md): Global lock order when multiple locks are needed; enforce in code review (POL-ARCH-004).
- `docs/architecture/GOROUTINE_ARCHITECTURE_POLICY.md`: Goroutine patterns
- `CONCURRENCY_STANDARDIZATION_PLAN.md`: Standardization plan
- `CONCURRENCY_STANDARDIZATION_PROGRESS.md`: Progress tracking
- `pkg/coordination/operation_callback.go`: Coordinator-based callback implementation
