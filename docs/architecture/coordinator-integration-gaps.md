# Coordinator Integration Gaps

**Last Verified:** 2026-08-31


**Version:** 1.1  
**Status:** Active (Phase 1 complete)  
**Date:** 2026-01-28  
**Purpose:** Identify areas of the system that should use coordinator pattern but don't currently

## Executive Summary

This document identifies operations and background workers that should integrate with the coordinator pattern for unified observability but currently don't. The coordinator provides unified event routing to logging, audit, metrics, and operational channels.

## High-Priority Gaps

### 1. Hash Registry Save Operations ✅ **COMPLETED**

**Location:** `pkg/storage/hash_registry.go`

**Status:** ✅ **Integrated**
- Added callback pattern (`HashRegistryEventCallback`)
- Emits events for batch processing (start, complete, error)
- Includes batch size, hash count, duration, and error information
- Coordinator helper: `emitHashRegistryEventViaCoordinator`
- Wired up in `check_impl.go`
- All HashRegistry instances configured with project root and storage

**Files:**
- `pkg/storage/hash_registry.go` - Callback pattern and event emission
- `cmd/zqk/system/hash_registry_coordination.go` - Coordinator helper
- `cmd/zqk/system/check_impl.go` - Wiring

---

### 2. Audit Event Buffer Flush Operations ✅ **COMPLETED**

**Location:** `pkg/storage/audit_event_buffer.go`

**Status:** ✅ **Integrated**
- Added callback pattern (`AuditBufferFlushEventCallback`)
- Emits events for flush operations (start, complete, error)
- Includes aggregation statistics (event count, group key, event type, target kind, aggregation window)
- Coordinator helper: `emitAuditBufferFlushEventViaCoordinator`
- Wired up in `check_impl.go`
- Events emitted for both threshold-triggered and periodic flushes

**Files:**
- `pkg/storage/audit_event_buffer.go` - Callback pattern and event emission
- `cmd/zqk/system/audit_buffer_coordination.go` - Coordinator helper
- `cmd/zqk/system/check_impl.go` - Wiring

---

### 3. Change Journal Entry Creation ✅ **COMPLETED**

**Location:** `pkg/storage/change_journal_helper.go`

**Status:** ✅ **Integrated**
- Added callback pattern (`ChangeJournalEventCallback`)
- Emits events for change journal entry creation (complete, error)
- Includes change type, object reference, kind, object ID, and duration
- Coordinator helper: `emitChangeJournalEventViaCoordinator`
- Wired up in `check_impl.go`
- Operational events enabled for rollback coordination

**Files:**
- `pkg/storage/change_journal_helper.go` - Callback pattern and event emission
- `cmd/zqk/system/change_journal_coordination.go` - Coordinator helper
- `cmd/zqk/system/check_impl.go` - Wiring

---

### 3a. CAS Index / I/O Queue State Change ✅ **COMPLETED**

**Location:** `pkg/storage/cas_index_write_queue_types.go`, `pkg/storage/io_queue.go`

**Status:** ✅ **Integrated**
- Added `CASIndexStateChangeEventCallback` for CAS index queue (SetProjectRoot, SetStorage).
- Added `IOQueueStateChangeEventCallback` for I/O queue manager (SetProjectRoot, SetStorage).
- Coordinator helpers: `emitCASIndexStateChangeEventViaCoordinator`, `emitIOQueueStateChangeEventViaCoordinator`.
- Wired up in `check_impl.go`.

**Files:**
- `pkg/storage/cas_index_write_queue_types.go`, `pkg/storage/io_queue.go` - Callback pattern and emit sites
- `cmd/zqk/system/cas_index_write_queue_coordination.go`, `cmd/zqk/system/io_queue_coordination.go` - Coordinator helpers
- `cmd/zqk/system/check_impl.go` - Wiring

---

### Phase 1 status (PRI-211)

**High-priority coordinator integration (Phase 1) is complete.** All targeted gaps (1, 2, 3, 3a) are integrated: Hash Registry, Audit Buffer Flush, Change Journal Entry Creation, and CAS Index / I/O Queue state change. Remaining items in this document are either deferred (Object CRUD – architectural constraint) or low priority (File Parsing, CAS Readdir). Phase 1 work aligns with PRI-211 (System Maturity and Integration – Phase 1: Data Management).

**Finish current plan before moving on.** Do not start another priority plan until PRI-211 is complete. Remaining PRI-211 scope (e.g. BLI-956 coordinator events – close as complete once verified; BLI-962, BLI-957, and other planned items under PRI-211) should be completed or explicitly deferred before beginning PRI-213 or other plans.

---

### 4. Object CRUD Operations ⚠️ **ARCHITECTURAL CONSTRAINT**

**Location:** `pkg/storage/object_storage_file.go`, `pkg/storage/audit_events.go`

**Current State:**
- Object create/update/delete operations call `createCreateAuditEvent()` / `createUpdateAuditEvent()` directly
- These call `CreateAuditEventWithBuilder` directly
- Highest frequency events in the system

**Impact:**
- All object lifecycle operations would have coordinated event emission
- Enables operational event coordination for dependent operations
- Better observability for object lifecycle events

**Complexity:** **High - Import Cycle Challenge**
- **IMPORT CYCLE CONSTRAINT**: `pkg/storage` cannot import `pkg/coordination` (cycle: storage -> coordination -> metrics -> storage)
- Current pattern: `StorageAuditRouter` (in coordination) calls `storage.CreateAuditEventWithBuilder` (reverse dependency)

**Options:**
1. **Keep current approach** (Recommended): Object CRUD events use `CreateAuditEventWithBuilder` directly (already routed through CAS/metrics)
2. **Event callback/hook pattern**: Storage layer accepts optional coordinator callback (dependency injection)
3. **Wrapper layer**: Create coordination wrappers in `cmd/zqk/system` that wrap storage operations (architectural change)

**Recommendation:** ⚠️ **Defer** - Architectural constraint makes this complex. Current approach already provides CAS routing and metrics tracking. Consider dependency injection pattern if operational coordination becomes critical.

---

## Medium-Priority Gaps

### 5. File Parsing Workers

**Location:** `pkg/storage/object_storage_file.go`

**Current State:**
- Background goroutines parse object files during List/Count operations
- No event emission for parsing operations
- Low-level operations

**Impact:**
- Could provide visibility into file parsing performance
- Could help diagnose parsing errors or performance issues

**Complexity:** Low
- These are low-level operations
- May not need full coordinator integration (could just log)

**Recommendation:** ⚠️ **Low priority** - These are low-level operations. Consider simple logging instead of full coordinator integration.

---

### 6. CAS Readdir Timeout Worker

**Location:** `pkg/storage/content_addressable_storage.go`

**Current State:**
- Background goroutine for readdir operations with timeout
- No event emission

**Impact:**
- Could provide visibility into CAS directory operations
- Could help diagnose performance issues

**Complexity:** Low
- Low-level operation
- May not need full coordinator integration

**Recommendation:** ⚠️ **Low priority** - Low-level operation. Consider simple logging instead.

---

## Integration Priority Matrix

| Operation | Priority | Complexity | Impact | Recommendation |
|-----------|----------|------------|--------|----------------|
| Hash Registry Save | High | Low | High | ✅ **Integrate** |
| Audit Buffer Flush | Medium | Medium | Medium | ✅ **Integrate** |
| Change Journal Creation | Medium | Medium | Medium | ✅ **Integrate** |
| Object CRUD Operations | High | High | High | ⚠️ **Defer** (Architectural) |
| File Parsing Workers | Low | Low | Low | ⚠️ **Low Priority** |
| CAS Readdir Timeout | Low | Low | Low | ⚠️ **Low Priority** |

## Integration Patterns

**Consistency and uniformity:** All storage→coordinator event emission MUST follow the canonical pattern below. Do not deviate (e.g. direct coordinator calls from `pkg/storage` or ad-hoc callback signatures).

### Canonical pattern (uniform implementation)

Use this exact structure for every new storage component that must emit coordinator events.

**1. In `pkg/storage` (component file, e.g. `cas_index_write_queue_types.go`):**

- Define a **callback type** named `XxxEventCallback` (e.g. `CASIndexBatchEventCallback`, `CASIndexStateChangeEventCallback`) with a single `func(...)` signature. Parameters typically include: `ctx context.Context`, `projectRoot string`, `storageProvider ObjectStorageProvider`, plus operation-specific args (kind, status, counts, duration, err, etc.).
- Declare **package-level state**: `globalXxxEventCallback XxxEventCallback`, `globalXxxCallbackMu sync.RWMutex`.
- Provide **setter**: `SetXxxEventCallback(callback XxxEventCallback)` — uses `concurrency.WithLockCtxLogger` on the mutex, stores callback. Comment: "This should be called by the CLI layer to wire up coordinator integration. Thread-safe."
- Provide **getter**: `getXxxEventCallback() XxxEventCallback` — uses `concurrency.WithRLockCtxLogger`, returns current callback.
- At **emit sites**: `callback := getXxxEventCallback(); if callback != nil { ... callback(ctx, projectRoot, storage, ...) }`. Prefer invoking in a goroutine (e.g. `goroutinelabels.NewGoroutine(...).StartSimple(...)`) when the operation is non-blocking so storage never blocks on coordinator.

**2. In `cmd/zqk/system` (coordination helper file, e.g. `cas_index_write_queue_coordination.go`):**

- Implement **emitter**: `emitXxxViaCoordinator(ctx, projectRoot, storageProvider, ...)` that:
  - Returns early if `projectRoot == "" || projectRoot == "."` (best-effort).
  - Sets up context (e.g. `createContextWithLoggingProfile(ctx, "system")`).
  - Builds routers: `auditRouter := coordination.NewStorageAuditRouter(projectRoot, storageProvider)`, optional metrics pipeline if storageProvider != nil.
  - Creates coordinator: `coordination.NewCoordinator(coordination.CoordinatorConfig{...})`.
  - Builds `coordination.EventData` (LoggingFields, AuditMetadata, MetricsData as needed).
  - Builds `coordination.EventContext` via `coordination.NewEventContext(operationID, operationType, status).WithEventData(...).WithContext(ctx).WithChannels(...)` (and WithError/WithDuration if applicable).
  - Emits: `goroutinelabels.NewGoroutine("xxx_event_emit", ...).StartSimple(func() { _ = coordinator.Emit(ctx, eventCtx) })` (async, best-effort).

**3. Wiring (one of):**

- **Preferred:** In `cmd/zqk/system/check_impl.go` init: `storage.SetXxxEventCallback(emitXxxViaCoordinator)` with a comment: "Register Xxx event callback for coordinator integration. This allows Xxx to emit events via coordinator without import cycles."
- **Alternative:** In the coordination helper file’s `init()` (e.g. `xxx_coordination.go`): same setter call and comment. Use when the subsystem is initialized from that file. See "Documented deviations and exceptions" below.

**Naming convention:**

- Callback type: `XxxEventCallback` (e.g. `CASIndexStateChangeEventCallback`).
- Setter: `SetXxxEventCallback`.
- Getter: `getXxxEventCallback` (unexported).
- Emitter: `emitXxxViaCoordinator` (unexported in cmd, called only via callback).

**Existing implementations to match:** `CASIndexBatchEventCallback`, `CASIndexStateChangeEventCallback`, `IOQueueStateChangeEventCallback`, `OrphanCleanupEventCallback`, `HashRegistryEventCallback`, `AuditBufferFlushEventCallback`, `ChangeJournalEventCallback`. Any new callback MUST follow the same structure and naming.

### Documented deviations and exceptions

Consistency and uniformity are key. The following are the **only** allowed variations from the canonical pattern above. Any other deviation must be documented here and justified before implementation.

**1. Wiring location**

- **Canonical:** Wire in `cmd/zqk/system/check_impl.go` init.
- **Allowed:** Wiring may instead be done in the coordination helper file’s `init()` (e.g. `queue_shutdown_coordination.go`, `id_queue_coordination.go`, `operation_executor_coordination.go`, `async_router_coordination.go`, `validation_lifecycle_coordination.go`). Use this when the callback is tied to a subsystem that is initialized from that file.
- **Rule:** One place per callback; either `check_impl.go` or the coordination file, not both. Prefer `check_impl.go` for storage callbacks so all storage→coordinator wiring is visible in one place; use the coordination file’s init when the subsystem is self-contained there.

**2. Callback in a storage subpackage**

- **Canonical:** Callback type and global state live in `pkg/storage` (e.g. in the component file).
- **Allowed:** Callback may live in a storage subpackage (e.g. `pkg/storage/id_generation/queue.go` for `IDQueueEventCallback`) when the component is implemented in that subpackage. The same canonical structure applies: `XxxEventCallback` type, global + mutex, `SetXxxEventCallback` / `getXxxEventCallback`, emit at call sites (optionally in a goroutine).
- **Rule:** Naming and structure must match the canonical pattern; only the package path may differ.

**3. No other deviations**

- Do not call `pkg/coordination` from `pkg/storage` (or storage subpackages). Do not introduce ad-hoc callback signatures or omit the getter/setter/mutex pattern. If a genuine exception is needed, add it to this section with a short justification before implementing.

---

### Pattern 1: Callback Pattern (Storage Layer)

**Use for:** Operations in `pkg/storage` that can't import `pkg/coordination`

**Example:** `CASIndexBatchEventCallback`, `OrphanCleanupEventCallback`

**Implementation:** Follow the **Canonical pattern** above (step 1).

**Benefits:**
- Avoids import cycles
- Flexible (can disable if coordinator not available)
- Testable (easy to mock)

### Pattern 2: Direct Coordinator (CLI Layer)

**Use for:** Emitter implementations in `cmd/zqk/system` that bridge storage callbacks to coordinator

**Example:** `emitCASIndexBatchEventViaCoordinator`, `emitOrphanCleanupEventViaCoordinator`

**Implementation:** Follow the **Canonical pattern** above (step 2). Do not call coordinator from `pkg/storage`; only from `cmd` via the callback.

**Benefits:**
- Keeps coordination dependency out of storage
- Full control over event routing in one place
- Uniform structure across all storage event types

## Implementation Roadmap

### Phase 1: High-Impact, Low-Complexity ✅ **COMPLETED**
1. ✅ **Hash Registry Save Operations** - Follow CAS index write queue pattern
2. ✅ **Audit Event Buffer Flush** - Add coordinator integration for flush operations
3. ✅ **Change Journal Entry Creation** - Add operational event coordination

### Phase 2: Medium-Priority (Future)
4. ⚠️ **Object CRUD Operations** - Evaluate dependency injection pattern (architectural constraint)

### Phase 3: Low-Priority (Backlog)
5. ⚠️ **File Parsing Workers** - Consider simple logging instead
6. ⚠️ **CAS Readdir Timeout** - Consider simple logging instead

## References

- `pkg/storage/cas_index_write_queue.go` - Reference implementation for callback pattern
- `pkg/storage/cas_orphan_cleanup_queue.go` - Reference implementation for on-demand worker with coordinator
- `cmd/zqk/system/cas_index_write_queue_coordination.go` - Reference implementation for coordinator helper
- `pkg/coordination/INTEGRATION_OPPORTUNITIES.md` - Previous analysis
- `docs/architecture/on-demand-worker-pattern.md` - On-demand worker pattern documentation
