# On-Demand Worker Pattern

**Last Verified:** 2026-08-31


**Version:** 1.0  
**Status:** Active  
**Date:** 2026-01-17  
**Purpose:** Document the "On-Demand Worker" pattern (wake-on-work with idle shutdown) for background processing

## Executive Summary

The **On-Demand Worker** pattern is a resource-efficient approach to background processing where workers:
1. **Wake on work**: Start automatically when work arrives
2. **Shut down when idle**: Stop after a configurable idle timeout
3. **Process in batches**: Group work items for efficiency
4. **Provide fallback**: Scheduled jobs can process queue if worker fails

This pattern is ideal for intermittent workloads where maintaining a continuously running worker would waste resources.

## Pattern Characteristics

### Core Behaviors

1. **Event-Driven Startup**
   - Worker starts automatically when first item is enqueued
   - No polling or scheduled checks required
   - Uses atomic flags to prevent duplicate workers

2. **Idle Shutdown**
   - Worker monitors queue and batch state
   - Shuts down after configurable idle timeout (e.g., 5 minutes)
   - Gracefully processes remaining batch before shutdown

3. **Batch Processing**
   - Groups work items into batches (size or time-based)
   - Processes when batch is full or timeout reached
   - Improves throughput and reduces overhead

4. **Fallback Mechanism**
   - Scheduled job can check queue and process if worker is idle
   - Ensures reliability even if worker crashes
   - Wakes worker to handle remaining items

### Implementation Requirements

✅ **MUST use goroutine labels** (`goroutinelabels.NewGoroutine`)
✅ **MUST use context for cancellation** (`WithContext`)
✅ **MUST use WaitGroup for lifecycle** (`WithWaitGroup`)
✅ **MUST log lifecycle events** (start, stop, idle shutdown)
✅ **MUST track metrics** (queue size, processing time, success/failure)
✅ **MUST integrate with coordinator** (emit events via coordinator for unified observability)
✅ **MUST create audit events** (for visibility into background operations - via coordinator when available)
✅ **MUST use atomic flags** for worker state tracking
✅ **MUST handle graceful shutdown** (process remaining batch)

## Current Implementation

### Example: CAS Orphan Cleanup Queue

**Location:** `pkg/storage/cas_orphan_cleanup_queue.go`

**Pattern Name:** `CASOrphanCleanupQueue`

**Key Features:**
- Batch size: 50 items
- Batch timeout: 500ms
- Idle timeout: 5 minutes
- Max retries: 3
- Queue capacity: 1000 items
- Coordinator integration: Events emitted via callback pattern (avoids import cycles)

**Lifecycle:**
```
EnqueueCleanup() → wakeWorkerIfNeeded() → startWorker() → processBatch() → idle shutdown
```

**Coordinator Integration:**
- Uses callback pattern (`OrphanCleanupEventCallback`) to avoid import cycles
- Emits worker lifecycle events (start, stop)
- Emits batch processing events (with success/failure counts)
- Events route through coordinator to logging, audit, metrics, and operational channels

**Fallback Command:** `zqk system orphan-cleanup-fallback` (PRUNED)

## Other Systems That Could Benefit

### 1. ID Generation Queue Manager ✅ **COMPLETED**
**Location:** `pkg/storage/id_generation/queue.go`

**Status:** ✅ **Implemented On-Demand Pattern**

**Key Features:**
- Wake-on-work: Worker starts when queues are created or need refill
- Idle shutdown: Shuts down after 5 minutes of inactivity
- Coordinator integration: Emits lifecycle events (start, stop)
- Atomic worker state tracking

**Implementation:**
- Uses `atomic.CompareAndSwapInt32` for worker state
- Worker checks queues every 1 second when active
- Shuts down when no queues need refill for 5 minutes
- Wakes automatically when new queues are created

**Coordinator Integration:**
- Uses callback pattern (`IDQueueEventCallback`) to avoid import cycles
- Emits worker lifecycle events (start, stop)
- Events route through coordinator to logging, audit, and metrics channels

**Files:**
- `pkg/storage/id_generation/queue.go` - On-demand worker implementation
- `cmd/zqk/system/id_queue_coordination.go` - Coordinator helper

---

### 2. CAS Index Write Queue ✅ **COMPLETED**
**Location:** `pkg/storage/cas_index_write_queue.go`

**Status:** ✅ **Implemented**

**Implementation:**
- **Wake-on-work**: Worker starts when index updates are enqueued
- **Idle shutdown**: Shuts down after 5 minutes of inactivity
- **Coordinator integration**: Emits lifecycle and batch events
- **Atomic worker state**: Thread-safe worker management

**Files:**
- `pkg/storage/cas_index_write_queue.go` - On-demand worker implementation
- `pkg/storage/cas_index_write_queue_on_demand_test.go` - Comprehensive tests

**Benefits:**
- ✅ Reduces resource usage during idle periods
- ✅ Consistent with orphan cleanup queue pattern
- ✅ Better resource utilization

---

### 3. Hash Registry Save Worker ✅ **COMPLETED**
**Location:** `pkg/storage/hash_registry.go`

**Status:** ✅ **Implemented**

**Implementation:**
- **Wake-on-work**: Worker starts when Save() is called
- **Idle shutdown**: Shuts down after 5 minutes of inactivity
- **Coordinator integration**: Emits lifecycle and batch events (already integrated)
- **Atomic worker state**: Thread-safe worker management

**Files:**
- `pkg/storage/hash_registry.go` - On-demand worker implementation
- `pkg/storage/hash_registry_on_demand_test.go` - Comprehensive tests

**Benefits:**
- ✅ Reduces resource usage during idle periods
- ✅ Consistent with other on-demand implementations
- ✅ Better resource utilization

---

### 4. Async Router Worker Pool ✅ **COMPLETED**
**Location:** `pkg/scheduler/transceiver/async_router.go`

**Status:** ✅ **Implemented On-Demand Pattern**

**Implementation:**
- **Wake-on-work**: Workers start when messages are enqueued
- **Idle shutdown**: Shuts down after 5 minutes of inactivity
- **Coordinator integration**: Emits lifecycle events via callback pattern
- **QueueShutdownHandler**: Implements graceful shutdown coordination
- **Callback-based shutdown**: Uses goroutine callbacks with timeout fallback

**Key Changes:**
- Removed fixed WorkerPool abstraction
- Workers process directly from routing queue
- Atomic worker count tracking
- Workers wake on `RouteAsync()` calls
- Shut down after idle timeout

**Files:**
- `pkg/scheduler/transceiver/async_router.go` - On-demand worker implementation
- `pkg/scheduler/transceiver/async_router_on_demand_test.go` - Comprehensive tests
- `cmd/zqk/system/async_router_coordination.go` - Coordinator integration

**Benefits:**
- ✅ Eliminates 10 idle worker goroutines during idle periods
- ✅ Reduces memory footprint (~100KB saved)
- ✅ Maintains responsiveness (workers wake quickly)
- ✅ Consistent with other on-demand implementations

---

### 5. Validation Async Workers
**Location:** `pkg/validation/async_validator.go`

**Current State:** Fixed worker pool

**Potential Benefit:**
- Scale workers based on validation queue depth
- Shut down during idle periods

**Migration Complexity:** High (complex state management)

---

### 6. Operation Executor Workers
**Location:** `pkg/storage/operation_executor.go`

**Current State:** Fixed worker pool

**Potential Benefit:**
- Scale workers based on operation queue depth
- Shut down during idle periods

**Migration Complexity:** Medium (similar to async router)

## Pattern Comparison

| Pattern | Use Case | Resource Usage | Complexity |
|---------|----------|----------------|------------|
| **On-Demand Worker** | Intermittent workloads | Low (only when needed) | Medium |
| **Continuous Worker** | Steady workloads | High (always running) | Low |
| **Fixed Worker Pool** | High-throughput, steady | High (always running) | Medium |
| **Dynamic Worker Pool** | Variable workloads | Medium (scales with load) | High |

## Best Practices

### When to Use On-Demand Worker

✅ **Use when:**
- Workload is intermittent (bursts of activity)
- Resource efficiency is important
- Work can be batched
- Fallback mechanism is feasible

❌ **Avoid when:**
- Workload is continuous and steady
- Latency requirements are strict (startup overhead)
- Work cannot be batched
- Complex state management required

### Implementation Checklist

- [ ] Use `goroutinelabels.NewGoroutine()` for all goroutines
- [ ] Use context for cancellation (`WithContext`)
- [ ] Use WaitGroup for lifecycle (`WithWaitGroup`)
- [ ] Log worker lifecycle events (start, stop, idle shutdown)
- [ ] Track metrics (queue size, processing time, success/failure)
- [ ] Integrate with coordinator (emit events via callback pattern)
- [ ] Create audit events for batch processing (via coordinator when available)
- [ ] Use atomic flags for worker state
- [ ] Implement graceful shutdown (process remaining batch)
- [ ] Provide fallback command for scheduled jobs
- [ ] Document idle timeout and batch configuration
- [ ] Wire up coordinator callback in CLI layer (to avoid import cycles)

## Related Patterns

- **Worker Pool Pattern**: For high-throughput, steady workloads
- **Event-Driven Architecture**: On-demand workers are event-driven
- **Batch Processing**: On-demand workers typically process in batches
- **Circuit Breaker**: Can be combined for resilience

## Coordinator Integration

The On-Demand Worker pattern **MUST** integrate with the coordinator for unified observability. This provides:

1. **Unified Event Routing**: Events flow through coordinator to logging, audit, metrics, and operational channels
2. **Process Coordination**: Operational events enable subscribers to coordinate dependent operations
3. **Observability**: All background operations are visible through coordinator channels

### Integration Pattern

Use the **callback pattern** to avoid import cycles:

```go
// In pkg/storage (worker implementation)
type OrphanCleanupEventCallback func(
    ctx context.Context,
    projectRoot string,
    storage ObjectStorageProvider,
    operationID string,
    operationType string,
    status string,
    batchSize int,
    successCount int,
    failureCount int,
    duration time.Duration,
    failedFiles []string,
)

// Set callback (called from CLI layer)
SetOrphanCleanupEventCallback(callback)

// Emit events via callback
callback := getOrphanCleanupEventCallback()
if callback != nil {
    callback(ctx, projectRoot, storage, ...)
}
```

```go
// In cmd/zqk/system (CLI layer - has access to coordination package)
import "github.com/lanceman/zqk/pkg/coordination"

// Wire up coordinator callback
storage.SetOrphanCleanupEventCallback(func(...) {
    coordinator := coordination.GetCoordinator()
    eventCtx := &coordination.EventContext{
        OperationID:   operationID,
        OperationType: operationType,
        Status:        status,
        // ... event data
        EmitLogging:     true,
        EmitAudit:       true,
        EmitMetrics:     true,
        EmitOperational: true,
    }
    coordinator.Emit(ctx, eventCtx)
})
```

### Benefits

- **No Import Cycles**: Storage package doesn't import coordination package
- **Unified Observability**: All events flow through coordinator
- **Flexible**: Can disable coordinator integration if needed (callback is nil)
- **Testable**: Easy to mock callback in tests

## References

- `pkg/storage/cas_orphan_cleanup_queue.go` - Reference implementation
- `pkg/storage/cas_index_write_queue.go` - Similar coordinator integration pattern
- `pkg/coordination/README.md` - Coordinator pattern documentation
- `docs/process/architecture/goroutine-entry-points-analysis.md` - Goroutine patterns
- `pkg/goroutinelabels/` - Goroutine labeling framework
