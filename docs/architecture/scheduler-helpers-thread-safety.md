# Scheduler Helpers Thread-Safety Analysis

**Last Verified:** 2026-08-31


## Overview

The scheduler helper functions (`scheduler_helpers.go`) provide a unified interface for checking scheduler status across multiple commands. This document analyzes the thread-safety guarantees of these functions.

## Thread-Safety Guarantees

### ✅ All Helper Functions Are Thread-Safe

All helper functions are safe to call concurrently from multiple goroutines:

1. **`getSchedulerStatus()`** - Core status check function
2. **`requireSchedulerRunning()`** - Ensures scheduler is running
3. **`requireSchedulerNotRunning()`** - Ensures scheduler is NOT running
4. **`getSchedulerInstance()`** - Gets scheduler instance if in this process

## Shared State Analysis

### 1. Global Scheduler Registry

**State**: `globalSchedulerRegistry` (package-level variable in `pkg/scheduler`)

**Protection**: `globalSchedulerMu` (RWMutex)
- `GetGlobalScheduler()` uses `RLock()` for read access ✅
- `RegisterGlobalScheduler()` uses `Lock()` for write access ✅

**Thread-Safety**: ✅ **Protected**

### 2. Scheduler Instance State

**State**: `Scheduler.running`, `Scheduler.jobs` (instance fields)

**Protection**: 
- `s.runningMu` (RWMutex) protects `running` field ✅
- `s.jobsMu` (RWMutex) protects `jobs` map ✅
- `sched.IsRunning()` uses `RLock()` for read access ✅

**Thread-Safety**: ✅ **Protected**

### 3. PID File

**State**: `.zqk/scheduler/scheduler.pid` (file on disk)

**Protection**: 
- File reads are atomic (OS-level) ✅
- No shared mutable state in memory ✅
- Each call reads independently ✅
- `readPIDFile()` doesn't modify the file ✅

**Thread-Safety**: ✅ **Safe** (read-only operations)

### 4. SchedulerStatus Return Value

**State**: `SchedulerStatus` struct returned by helpers

**Protection**:
- Each call returns a **new instance** (value semantics) ✅
- No shared mutable state ✅
- Immutable after creation ✅

**Thread-Safety**: ✅ **Safe** (no shared state)

## Concurrent Access Patterns

### Pattern 1: Multiple Commands Checking Status

```go
// Goroutine 1: status command
go func() {
    status, _ := getSchedulerStatus(ctx1)
    // Uses status...
}()

// Goroutine 2: trigger command
go func() {
    status, _ := getSchedulerStatus(ctx2)
    // Uses status...
}()
```

**Result**: ✅ **Safe** - Each gets its own status instance, all read operations are protected.

### Pattern 2: Concurrent Start Attempts

```go
// Process 1
go func() {
    status, err := requireSchedulerNotRunning(ctx1)
    // If OK, starts scheduler...
}()

// Process 2 (different process)
go func() {
    status, err := requireSchedulerNotRunning(ctx2)
    // If OK, starts scheduler...
}()
```

**Result**: ⚠️ **Race condition possible** - Two processes could both pass the check and try to start. However:
- PID file check in `scheduler.Start()` provides additional protection
- Only one process will successfully write PID file
- Second process will fail with "already running" error

**Mitigation**: The PID file check in `scheduler.Start()` (line 191) provides a second check that prevents duplicate starts even if both processes pass the initial check.

### Pattern 3: Status Check While Scheduler Starting/Stopping

```go
// Goroutine 1: Starting scheduler
go func() {
    sched.Start(ctx)
}()

// Goroutine 2: Checking status
go func() {
    status, _ := getSchedulerStatus(ctx)
    // May see intermediate state
}()
```

**Result**: ✅ **Safe** - Status check uses read locks, won't block scheduler operations. May see transient intermediate state, but won't cause crashes or data corruption.

## Potential Issues and Mitigations

### Issue 1: Stale Status Cache (Not Currently Implemented)

**Problem**: If we add caching, concurrent reads could see stale data.

**Mitigation**: 
- No caching currently implemented
- Each call reads fresh state
- If caching is added, use `sync.RWMutex` with cache invalidation

### Issue 2: PID File Race Condition

**Problem**: Two processes could read PID file simultaneously, both see "not running", both try to start.

**Mitigation**:
- PID file check in `scheduler.Start()` (line 191) provides second check
- File-based locking could be added if needed (using `flock()`)
- Current implementation is sufficient for typical use cases

### Issue 3: Scheduler Instance Mutation

**Problem**: Caller receives `*Scheduler` pointer from `getSchedulerInstance()`, could mutate it unsafely.

**Mitigation**:
- Scheduler's internal state is protected by its own mutexes
- Caller must use scheduler's public API (which is thread-safe)
- Documented in function comments

## Recommendations

### ✅ Current Implementation is Thread-Safe

The current implementation provides adequate thread-safety for the use cases:

1. **Read Operations**: All status checks use read locks or read-only file operations ✅
2. **Return Values**: Each call returns a new instance (no shared mutable state) ✅
3. **Scheduler State**: Protected by scheduler's own mutexes ✅

### 🔄 Future Enhancements (If Needed)

1. **Caching**: If performance becomes an issue, add thread-safe caching with TTL
2. **File Locking**: For stronger cross-process coordination, use `flock()` on PID file
3. **Atomic Operations**: For counters or flags, consider `sync/atomic` package

## Test Coverage

Thread-safety tests in `scheduler_helpers_test.go`:

- `TestGetSchedulerStatus_ThreadSafety` - Concurrent status checks
- `TestGetSchedulerStatus_WithPIDFile` - Concurrent checks with PID file
- `TestRequireSchedulerRunning_Concurrent` - Concurrent requirement checks
- `TestGetSchedulerInstance_Concurrent` - Concurrent instance retrieval

## Conclusion

**The scheduler helper functions are thread-safe for concurrent read operations.** The shared state (global scheduler registry, scheduler instance state, PID file) is properly protected by mutexes or uses read-only operations. Each function returns a new `SchedulerStatus` instance, eliminating shared mutable state in the helpers themselves.

The only potential race condition (concurrent start attempts) is mitigated by the PID file check in `scheduler.Start()`, which provides a second validation point.

