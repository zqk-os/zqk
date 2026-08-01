# Goroutine Entry Points Analysis

**Version:** 1.0  
**Status:** Active  
**Date:** 2026-01-13  
**Purpose:** Identify all goroutine entry points, establish requirements, and create reusable components

## Executive Summary

This document identifies all goroutine entry points in the codebase, categorizes them by pattern, establishes requirements for each category, and provides reusable components to ensure consistency and prevent leaks.

## Entry Point Categories

### Category 1: Periodic Background Tasks ⏰

**Pattern:** Long-running goroutines with tickers for periodic work

**Examples Found:**
- `pkg/mcp/client_metrics.go` - Periodic compression (24h)
- `pkg/metrics/sampler.go` - Flush ticker loop
- `pkg/storage/audit_event_buffer.go` - Periodic flush
- `pkg/scheduler/scheduler.go` - Keep-alive heartbeat, job recovery
- `pkg/scheduler/job_trigger_queue.go` - Queue checking (2s)
- `pkg/scheduler/handlers_callback_listener.go` - Callback checking (30s)
- `pkg/scheduler/notification_context.go` - Suppression cleanup (1s)

**Requirements:**
- ✅ MUST use `GoroutineManager.Start()` with context
- ✅ MUST check `ctx.Done()` in loop
- ✅ MUST register ticker as Resource with CleanupFunc
- ✅ MUST have meaningful name, purpose, category
- ✅ MUST emit lifecycle events through coordinator
- ✅ MUST stop on shutdown

**Reusable Component:** `runtime.StartPeriodicTask()`

---

### Category 2: Worker Pools 👷

**Pattern:** Multiple goroutines processing from a queue/channel

**Examples Found:**
- `pkg/validation/async_validator.go` - Validation workers
- `pkg/storage/bulk_delete_optimized.go` - Delete workers
- `pkg/context/pipeline.go` - Async listener workers
- `pkg/git/commit.go` - Commit analysis workers

**Requirements:**
- ✅ MUST use `GoroutineManager.Start()` for each worker
- ✅ MUST use `sync.WaitGroup` for coordination
- ✅ MUST check `ctx.Done()` in worker loop
- ✅ MUST have worker ID in name (e.g., "validation_worker_1")
- ✅ MUST track worker pool as a group
- ✅ MUST stop all workers on shutdown

**Reusable Component:** `runtime.StartWorkerPool()`

---

### Category 3: Async Event Emission 📡

**Pattern:** Fire-and-forget goroutines for non-blocking event emission

**Examples Found:**
- `pkg/coordination/coordinator.go` - Route to channels (4 goroutines per event)
- `pkg/coordination/progress_helper.go` - Progress events
- `pkg/migration/coordination.go` - Migration events
- `cmd/zqk/create_command_audit_coordination.go` - Audit events
- `pkg/mcp/server.go` - RecordEvent

**Requirements:**
- ✅ MUST use `GoroutineManager.Start()` for long-running
- ✅ OR use `runtime.EmitEventAsync()` helper for fire-and-forget
- ✅ MUST be non-blocking
- ✅ MUST handle errors gracefully (log, don't fail)
- ✅ MUST have timeout protection

**Reusable Component:** `runtime.EmitEventAsync()`

---

### Category 4: One-Off Async Operations 🚀

**Pattern:** Single goroutine for async operation that completes

**Examples Found:**
- `pkg/mcp/server.go` - Read message goroutine (per loop iteration)
- `pkg/mcp/async_handler.go` - Handler execution
- `pkg/mcp/server_handlers.go` - Welcome message
- `pkg/cli/timeout_hook.go` - Command execution
- `pkg/scheduler/scheduler.go` - Job execution

**Requirements:**
- ✅ MUST use `GoroutineManager.Start()` if long-running
- ✅ MUST check `ctx.Done()` if blocking
- ✅ MUST have timeout protection
- ✅ MUST recover from panics
- ✅ MUST emit completion events

**Reusable Component:** `runtime.ExecuteAsync()`

---

### Category 5: Test Goroutines 🧪

**Pattern:** Goroutines in test code for concurrency testing

**Examples Found:**
- All `*_test.go` files with `go func()`
- Test helpers for concurrent operations

**Requirements:**
- ✅ SHOULD use `GoroutineManager` in integration tests
- ✅ MUST use `sync.WaitGroup` for coordination
- ✅ MUST clean up in test teardown
- ✅ CAN use direct `go func()` for simple unit tests

**Reusable Component:** `runtime.TestGoroutine()` (test helper)

---

### Category 6: Background Services 🏗️

**Pattern:** Long-running services that manage their own lifecycle

**Examples Found:**
- `pkg/scheduler/scheduler.go` - Scheduler daemon
- `pkg/validation/async_validator.go` - Async validator service
- `pkg/mcp/server.go` - MCP server

**Requirements:**
- ✅ MUST use `GoroutineManager` for all internal goroutines
- ✅ MUST implement `Start()` and `Stop()` methods
- ✅ MUST integrate with system shutdown
- ✅ MUST track all spawned goroutines
- ✅ MUST provide health monitoring

**Reusable Component:** `runtime.Service` interface

---

## Entry Point Inventory

### High Priority (Must Migrate)

| Location | Type | Current Pattern | Status |
|----------|------|----------------|--------|
| `pkg/mcp/client_metrics.go:772` | Periodic | Direct `go func()` + ticker | ✅ Fixed - Uses StartWithContext with proper context |
| `pkg/metrics/sampler.go:119` | Periodic | Direct `go func()` + ticker | ✅ Fixed (2026-01-26) |
| `pkg/storage/audit_event_buffer.go:154` | Periodic | Direct `go func()` + ticker | ✅ Fixed (2026-01-26) |
| `pkg/scheduler/scheduler.go:578` | One-off | Direct `go func()` | ✅ Fixed (2026-01-26) |
| `pkg/validation/async_validator.go:174` | Worker Pool | Direct `go func()` | ✅ Fixed (2026-01-26) |
| `pkg/coordination/coordinator.go:106` | Async Event | Direct `go func()` | ✅ Fixed (2026-01-26) |

### Medium Priority (Should Migrate)

| Location | Type | Current Pattern | Status |
|----------|------|----------------|--------|
| `pkg/mcp/server.go:621` | One-off | Direct `go func()` | ✅ Verified - No goroutine at this line |
| `pkg/mcp/async_handler.go:82` | One-off | Direct `go func()` | ✅ Fixed (2026-01-26) |
| `pkg/context/pipeline.go:125` | Worker Pool | Direct `go func()` | ✅ Fixed (2026-01-26) |
| `pkg/storage/bulk_delete_optimized.go:251` | Worker Pool | Direct `go func()` | ✅ Fixed (2026-01-26) |

### Low Priority (Test Code)

| Location | Type | Current Pattern | Status |
|----------|------|----------------|--------|
| All `*_test.go` files | Test | Direct `go func()` | ✅ Acceptable |

---

## Requirements Matrix

### Requirement: R-GOROUTINE-001 - Lifecycle Tracking

**All goroutines MUST be tracked through GoroutineManager**

**Criteria:**
- ✅ Goroutine ID assigned
- ✅ Start time recorded
- ✅ Status tracked (starting → running → stopping → stopped)
- ✅ Stop time recorded
- ✅ Duration calculated

**Traceability:**
- Architecture: `runtime-goroutine-manager-v1.0.md`
- Implementation: `pkg/runtime/goroutine_manager.go`
- Checklist: `mcp-goroutine-leak-prevention-checklist.md`

---

### Requirement: R-GOROUTINE-002 - Context Cancellation

**All goroutines MUST respect context cancellation**

**Criteria:**
- ✅ Accept `context.Context` as first parameter
- ✅ Check `ctx.Done()` in all loops
- ✅ Exit gracefully on cancellation
- ✅ Clean up resources on exit

**Traceability:**
- Architecture: `mcp-goroutine-lifecycle-v1.0.md`
- Pattern: Context-first function signature

---

### Requirement: R-GOROUTINE-003 - Resource Management

**All resources MUST be tracked and cleaned up**

**Criteria:**
- ✅ Register resources with GoroutineManager
- ✅ Provide CleanupFunc for each resource
- ✅ Cleanup called on stop
- ✅ No resource leaks

**Traceability:**
- Architecture: `runtime-goroutine-manager-v1.0.md`
- Implementation: `Resource` type in `goroutine_manager.go`

---

### Requirement: R-GOROUTINE-004 - Observability

**All goroutines MUST emit lifecycle events**

**Criteria:**
- ✅ Start event emitted
- ✅ Stop event emitted
- ✅ Error event emitted (if applicable)
- ✅ Events routed through coordinator
- ✅ Metadata includes goroutine info

**Traceability:**
- Architecture: `GOROUTINE_MANAGER_CAPABILITIES.md`
- Implementation: `emitGoroutineEvent()` in `goroutine_manager.go`

---

### Requirement: R-GOROUTINE-005 - Graceful Shutdown

**All goroutines MUST stop on system shutdown**

**Criteria:**
- ✅ Context cancelled on shutdown
- ✅ Resources cleaned up
- ✅ Timeout protection
- ✅ Leak detection

**Traceability:**
- Architecture: `runtime-goroutine-manager-v1.0.md`
- Implementation: `Shutdown()` method

---

## Reusable Components

### Component 1: `runtime.StartPeriodicTask()`

**Purpose:** Start a periodic background task with full tracking

**Signature:**
```go
func StartPeriodicTask(
    manager *GoroutineManager,
    name string,
    interval time.Duration,
    fn func(ctx context.Context) error,
) (string, context.Context, error)
```

**Usage:**
```go
id, ctx, err := runtime.StartPeriodicTask(
    manager,
    "metrics_compression",
    24*time.Hour,
    func(ctx context.Context) error {
        return compressMetrics(ctx)
    },
)
```

---

### Component 2: `runtime.StartWorkerPool()`

**Purpose:** Start a worker pool with full tracking

**Signature:**
```go
func StartWorkerPool(
    manager *GoroutineManager,
    name string,
    workerCount int,
    fn func(ctx context.Context, workerID int) error,
) ([]string, error)
```

**Usage:**
```go
workerIDs, err := runtime.StartWorkerPool(
    manager,
    "validation_worker",
    4,
    func(ctx context.Context, id int) error {
        return processValidationQueue(ctx, id)
    },
)
```

---

### Component 3: `runtime.EmitEventAsync()`

**Purpose:** Emit event asynchronously (fire-and-forget)

**Signature:**
```go
func EmitEventAsync(
    manager *GoroutineManager,
    coordinator coordination.EventCoordinator,
    eventCtx *coordination.EventContext,
) error
```

**Usage:**
```go
runtime.EmitEventAsync(manager, coordinator, eventCtx)
```

---

### Component 4: `runtime.ExecuteAsync()`

**Purpose:** Execute operation asynchronously with tracking

**Signature:**
```go
func ExecuteAsync(
    manager *GoroutineManager,
    name string,
    fn func(ctx context.Context) error,
) (string, context.Context, <-chan error)
```

**Usage:**
```go
id, ctx, errChan := runtime.ExecuteAsync(
    manager,
    "async_operation",
    func(ctx context.Context) error {
        return doWork(ctx)
    },
)
```

---

### Component 5: `runtime.Service` Interface

**Purpose:** Standard interface for background services

**Signature:**
```go
type Service interface {
    Start(ctx context.Context) error
    Stop() error
    GetGoroutineManager() *GoroutineManager
}
```

**Usage:**
```go
type MyService struct {
    manager *GoroutineManager
}

func (s *MyService) Start(ctx context.Context) error {
    // Start all goroutines via manager
}

func (s *MyService) Stop() error {
    return s.manager.Shutdown()
}
```

---

## Migration Plan

### Phase 1: High Priority (Week 1)

1. Migrate periodic tasks:
   - [ ] `pkg/mcp/client_metrics.go` - Compression
   - [ ] `pkg/metrics/sampler.go` - Flush loop
   - [ ] `pkg/storage/audit_event_buffer.go` - Periodic flush

2. Create reusable components:
   - [ ] `runtime.StartPeriodicTask()`
   - [ ] `runtime.StartWorkerPool()`
   - [ ] `runtime.EmitEventAsync()`

### Phase 2: Medium Priority (Week 2)

1. Migrate worker pools:
   - [ ] `pkg/validation/async_validator.go`
   - [ ] `pkg/storage/bulk_delete_optimized.go`
   - [ ] `pkg/context/pipeline.go`

2. Migrate one-off operations:
   - [ ] `pkg/mcp/server.go` - Read goroutine
   - [ ] `pkg/mcp/async_handler.go` - Handler execution

### Phase 3: Services (Week 3)

1. Migrate background services:
   - [ ] `pkg/scheduler/scheduler.go`
   - [ ] `pkg/validation/async_validator.go` (service layer)

2. Implement `runtime.Service` interface

### Phase 4: Observability (Week 4)

1. Add monitoring:
   - [ ] Dashboard for goroutine stats
   - [ ] Alerts for leaks
   - [ ] Metrics aggregation

---

## Traceability Matrix

| Requirement | Component | Architecture Doc | Implementation | Test |
|-------------|-----------|-----------------|----------------|------|
| R-GOROUTINE-001 | GoroutineManager | runtime-goroutine-manager-v1.0.md | goroutine_manager.go | goroutine_manager_test.go |
| R-GOROUTINE-002 | Context pattern | mcp-goroutine-lifecycle-v1.0.md | All components | leak_detection_test.go |
| R-GOROUTINE-003 | Resource tracking | runtime-goroutine-manager-v1.0.md | Resource type | resource_cleanup_test.go |
| R-GOROUTINE-004 | Event emission | GOROUTINE_MANAGER_CAPABILITIES.md | emitGoroutineEvent() | integration_test.go |
| R-GOROUTINE-005 | Shutdown | runtime-goroutine-manager-v1.0.md | Shutdown() method | shutdown_test.go |

---

## Compliance Checklist

For each new goroutine, verify:

- [ ] Uses `GoroutineManager.Start()` or reusable component
- [ ] Accepts `context.Context` as first parameter
- [ ] Checks `ctx.Done()` in all loops
- [ ] Registers resources with CleanupFunc
- [ ] Emits lifecycle events
- [ ] Has meaningful name, purpose, category
- [ ] Integrates with shutdown sequence
- [ ] Has tests for lifecycle and cleanup

---

## Next Steps

1. **Create Reusable Components** (Priority 1)
   - Implement `runtime.StartPeriodicTask()`
   - Implement `runtime.StartWorkerPool()`
   - Implement `runtime.EmitEventAsync()`
   - Implement `runtime.ExecuteAsync()`

2. **Migrate High Priority Entry Points** (Priority 2)
   - Start with periodic tasks (highest leak risk)
   - Then worker pools
   - Then one-off operations

3. **Add Monitoring** (Priority 3)
   - Dashboard for goroutine stats
   - Alerts for leaks
   - Metrics aggregation

4. **Documentation** (Ongoing)
   - Update architecture docs
   - Create migration guides
   - Add examples
