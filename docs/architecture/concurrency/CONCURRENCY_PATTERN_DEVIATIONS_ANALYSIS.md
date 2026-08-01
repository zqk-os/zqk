# Concurrency Pattern Deviations Analysis

**Version**: 1.0  
**Date**: 2026-01-05  
**Status**: Comprehensive Analysis

## Executive Summary

This document provides a comprehensive analysis of all areas in the codebase that deviate from or break the established concurrency patterns documented in:
- `docs/architecture/architecture/concurrency-patterns-v1.0.md`
- `docs/architecture/README.md`

**Total Violations Identified**: ~100+ instances across production and test code

## Pattern Compliance Matrix

| Pattern | Required | Production Violations | Test Violations | Status |
|---------|----------|----------------------|-----------------|--------|
| Goroutine Builder | MUST | 6 | 40+ | ⚠️ Partial |
| WaitGroup Timeout | MUST | 8 | 50+ | ❌ Critical |
| Coordinator Events | MUST | 50+ | 30+ | ❌ Critical |
| Context Cancellation | MUST | ~10 | ~20 | ⚠️ Partial |
| WaitGroupManager | SHOULD | ~20 | ~10 | ⚠️ Partial |
| On-Demand Worker | Pattern | 0 | 0 | ✅ Compliant |
| Graceful Shutdown | MUST | ~5 | ~5 | ⚠️ Partial |
| Retry Backoff | Pattern | 0 | 0 | ✅ Compliant |

## Critical Violations by Pattern

### 1. Goroutine Builder Pattern Violations

**Policy**: MUST use `goroutinelabels.NewGoroutine()` for all goroutines

#### Production Code (6 violations)

**pkg/scheduler/handlers.go**
- **Line 143**: `go func() {` - Wait for tier 2 cache completion
  - **Issue**: Direct `go func()` instead of builder
  - **Impact**: No panic recovery, no labels, no error handling
  - **Fix**: Use `NewGoroutine().WithWaitGroup().WithContext().StartSimple()`
  - **Severity**: High

- **Line 201**: `go func() {` - Wait for tier 3 cache completion
  - **Issue**: Direct `go func()` instead of builder
  - **Impact**: No panic recovery, no labels, no error handling
  - **Fix**: Use `NewGoroutine().WithWaitGroup().WithContext().StartSimple()`
  - **Severity**: High

**pkg/storage/cas_orphan_cleanup_queue.go**
- **Line 525**: `go func() {` - Wait for worker to finish processing
  - **Issue**: Direct `go func()` instead of builder
  - **Impact**: No panic recovery, no labels
  - **Fix**: Use `NewGoroutine().WithWaitGroup().StartSimple()`
  - **Severity**: Medium

**pkg/storage/hash_registry_manager.go**
- **Line 71**: `go func(hr *HashRegistry) {` - Worker goroutine with wait group
  - **Issue**: Direct `go func()` instead of builder
  - **Impact**: No panic recovery, no labels, manual WaitGroup management
  - **Fix**: Use `NewGoroutine().WithWaitGroup().StartSimple()`
  - **Severity**: High

- **Line 80**: `go func() {` - Wait for drain operations to complete
  - **Issue**: Direct `go func()` instead of builder
  - **Impact**: No panic recovery, no labels
  - **Fix**: Use `NewGoroutine().WithWaitGroup().StartSimple()`
  - **Severity**: Medium

**pkg/storage/id_generation/queue.go**
- **Line 409**: `go func() {` - Wait for worker to finish
  - **Issue**: Direct `go func()` instead of builder
  - **Impact**: No panic recovery, no labels
  - **Fix**: Use `NewGoroutine().WithWaitGroup().StartSimple()`
  - **Severity**: Medium

#### Test Code (40+ violations)
- Many test files use direct `go func()` calls
- **Priority**: Lower, but should be addressed for consistency

### 2. WaitGroup Timeout Pattern Violations

**Policy**: MUST use deterministic timeout pattern for all `WaitGroup.Wait()` calls

#### Production Code (8 critical violations)

**cmd/zqk/system/snapshot_expand.go**
- **Line 299**: `wg.Wait()` - Direct wait without timeout in context cancellation handler
  - **Issue**: Blocks indefinitely if workers hang
  - **Pattern Violation**: Missing deterministic timeout
  - **Fix**: Use timeout pattern with channel + select
  - **Severity**: Critical

- **Line 307**: `wg.Wait()` - Direct wait without timeout after closing jobs channel
  - **Issue**: Blocks indefinitely if workers hang
  - **Pattern Violation**: Missing deterministic timeout
  - **Fix**: Use timeout pattern with channel + select
  - **Severity**: Critical

**cmd/zqk/system/hierarchical_fix_batch.go**
- **Line 197**: `wg.Wait()` - Direct wait without timeout
  - **Issue**: Blocks indefinitely if any goroutine hangs
  - **Pattern Violation**: Missing deterministic timeout
  - **Fix**: Use timeout pattern
  - **Severity**: Critical

**pkg/storage/cas_orphan_cleanup_queue.go**
- **Line 506**: `q.wg.Wait()` - Direct wait in `Shutdown()` method
  - **Issue**: Shutdown can hang indefinitely
  - **Pattern Violation**: Missing deterministic timeout
  - **Fix**: Use timeout pattern
  - **Severity**: Critical

- **Line 526**: `q.wg.Wait()` - Wait in goroutine without timeout in `Drain()`
  - **Issue**: Wait goroutine has no timeout (even though select has timeout)
  - **Pattern Violation**: Wait goroutine itself needs timeout
  - **Fix**: Add timeout to wait goroutine
  - **Severity**: High

**pkg/storage/waitgroup_manager.go**
- **Line 151**: `entry.wg.Wait()` - Direct wait without timeout
  - **Issue**: Blocks indefinitely if goroutines hang
  - **Pattern Violation**: WaitGroupManager should enforce timeout
  - **Fix**: Add timeout to `Wait()` method
  - **Severity**: Critical

**pkg/storage/hash_registry_manager.go**
- **Line 81**: `wg.Wait()` - Wait in goroutine, but no timeout on the wait itself
  - **Issue**: Wait goroutine has no timeout (even though select has timeout)
  - **Pattern Violation**: Wait goroutine itself needs timeout
  - **Fix**: Add timeout to wait goroutine
  - **Severity**: High

**pkg/storage/id_generation/queue.go**
- **Line 385**: `qm.wg.Wait()` - Direct wait in `Stop()` method
  - **Issue**: Stop can hang indefinitely
  - **Pattern Violation**: Missing deterministic timeout
  - **Fix**: Use timeout pattern
  - **Severity**: Critical

- **Line 410**: `qm.wg.Wait()` - Wait in goroutine without timeout in `Drain()`
  - **Issue**: Wait goroutine has no timeout (even though select has timeout)
  - **Pattern Violation**: Wait goroutine itself needs timeout
  - **Fix**: Add timeout to wait goroutine
  - **Severity**: High

**pkg/scheduler/handlers.go**
- **Line 144**: `wgTier2.Wait()` - Wait in goroutine, has timeout via context
  - **Issue**: Wait goroutine has no timeout (even though select has timeout)
  - **Pattern Violation**: Wait goroutine itself needs timeout
  - **Fix**: Add timeout to wait goroutine
  - **Severity**: High

- **Line 202**: `wgTier3.Wait()` - Wait in goroutine, has timeout via context
  - **Issue**: Wait goroutine has no timeout (even though select has timeout)
  - **Pattern Violation**: Wait goroutine itself needs timeout
  - **Fix**: Add timeout to wait goroutine
  - **Severity**: High

#### Test Code (50+ violations)
- Many test files use direct `wg.Wait()` without timeout
- **Priority**: Medium - should be addressed to prevent test hangs

### 3. Coordinator Event Pattern Violations

**Policy**: MUST emit coordinator events for all goroutine lifecycle events and operation status

#### Production Code (50+ violations)

See `COORDINATOR_PATTERN_VIOLATIONS.md` for complete list.

**Key Categories**:

1. **Direct fmt.Fprintf for Progress/Status** (~20 instances)
   - `cmd/zqk/system/async_check_helpers.go` - Progress updates
   - `cmd/zqk/system/show_validation_progress_helpers.go` - Progress bar, completion messages
   - `cmd/zqk/scheduler/submit.go` - Job submission output
   - `cmd/zqk/scheduler/scheduler.go` - Status messages
   - **Fix**: Replace with coordinator events

2. **Direct Logger Calls for Lifecycle Events** (~30 instances)
   - Operation start/complete events
   - Goroutine lifecycle events
   - Cache operation events
   - **Fix**: Replace with coordinator events

3. **Missing Coordinator Events for Goroutine Lifecycle** (~10 instances)
   - Worker start/stop events
   - Goroutine cancellation events
   - **Fix**: Add coordinator events

### 4. WaitGroupManager Pattern Violations

**Policy**: SHOULD use WaitGroupManager for multiple WaitGroups (per concurrency patterns doc)

#### Production Code (~20 violations)

**Components NOT using WaitGroupManager**:
- `cmd/zqk/system/snapshot_expand.go` - Uses direct `sync.WaitGroup`
- `cmd/zqk/system/hierarchical_fix_batch.go` - Uses direct `sync.WaitGroup`
- `cmd/zqk/system/cleanup_duplicates_helpers.go` - Uses direct `sync.WaitGroup`
- `cmd/zqk/system/git_analyze_helpers.go` - Uses direct `sync.WaitGroup`
- `pkg/validation/async_validator.go` - Uses direct `sync.WaitGroup` (multiple)
- `pkg/scheduler/transceiver/async_router.go` - Uses direct `sync.WaitGroup`
- Many more...

**Components using WaitGroupManager correctly**:
- `pkg/storage/operation_executor.go` - ✅ Migrated in Phase 1

**Impact**: Missing observability, harder to debug, no lifecycle tracking

### 5. Context Lifecycle Management Violations

**Policy**: MUST check `ctx.Done()` in all loops and long operations

#### Production Code (~10 violations)

**Areas missing context checks**:
- Some worker loops don't check context cancellation
- Some retry loops don't check context cancellation
- Some polling loops don't check context cancellation

**Examples**:
- `pkg/scheduler/transceiver/async_router.go` - Worker loop has context check ✅
- `pkg/storage/operation_executor.go` - Worker loop has context check ✅
- Some polling loops use `time.Sleep()` without context check

### 6. Graceful Shutdown Pattern Violations

**Policy**: MUST use atomic shutdown flags and proper cleanup

#### Production Code (~5 violations)

**Components missing proper shutdown**:
- Some components don't use atomic shutdown flags
- Some components don't wait for goroutines with timeout
- Some components don't cleanup resources properly

### 7. errgroup Usage (Pattern Deviation)

**Issue**: `errgroup.Group` is used in one location but is NOT a documented pattern

**Location**: `cmd/zqk/system/async_check.go` (line 451 - now removed)

**Status**: This was refactored to use `sync.WaitGroup` + `GoroutineBuilder` pattern

**Note**: `errgroup` is acceptable when used with `SetGoroutineLabel()`, but `sync.WaitGroup` + `GoroutineBuilder` is the preferred pattern per codebase conventions.

## Pattern-Specific Analysis

### WaitGroup Lifecycle Management Pattern

**Documented Pattern**: Use `WaitGroupManager` for centralized lifecycle management

**Compliance**: ~5% (1 component migrated, ~20 components still use direct WaitGroup)

**Violations**:
- Most components use direct `sync.WaitGroup` instead of `WaitGroupManager`
- Missing observability and tracking
- No lifecycle events

**Migration Priority**: Medium (SHOULD, not MUST)

### Goroutine Builder Pattern

**Documented Pattern**: MUST use `goroutinelabels.NewGoroutine()` for all goroutines

**Compliance**: ~95% (6 violations in production, 40+ in tests)

**Violations**: See section 1 above

**Migration Priority**: High (MUST)

### On-Demand Worker Pattern

**Documented Pattern**: Resource-efficient workers with wake-on-work and idle shutdown

**Compliance**: 100% ✅

**Status**: All on-demand workers follow the pattern correctly:
- `OperationExecutor` ✅
- `HashRegistry` ✅
- `IOQueueManager` ✅
- `CASIndexWriteQueue` ✅
- `IDGenerationQueueManager` ✅

### Explicit Async Operation Pattern

**Documented Pattern**: Make async operations explicit and testable

**Compliance**: ~50%

**Status**: Some components make async explicit, others have hidden async operations

### Async Processing with Synchronous Responses

**Documented Pattern**: Process asynchronously, respond synchronously

**Compliance**: 100% ✅

**Status**: MCP server and transceiver router follow pattern correctly

### Worker Pool Pattern

**Documented Pattern**: Bounded concurrency with queue management

**Compliance**: 100% ✅

**Status**: AsyncRouter and AsyncValidator follow pattern correctly

### Graceful Shutdown Pattern

**Documented Pattern**: Atomic shutdown flags, context cancellation, resource cleanup

**Compliance**: ~80%

**Violations**: Some components missing atomic flags or timeout waits

### Context Lifecycle Management Pattern

**Documented Pattern**: Context as first parameter, check `ctx.Done()` in loops

**Compliance**: ~90%

**Violations**: Some loops missing context checks

### Retry with Exponential Backoff Pattern

**Documented Pattern**: Configurable retry with exponential backoff

**Compliance**: 100% ✅

**Status**: All retry implementations follow pattern correctly

## Severity Classification

### Critical (Must Fix Immediately)
1. **WaitGroup.Wait() without timeout** (8 instances) - Can cause indefinite hangs
2. **Missing coordinator events for user-facing messages** (20+ instances) - Policy violation
3. **Direct go func() in production** (6 instances) - Policy violation

### High (Should Fix Soon)
1. **Wait goroutines without timeout** (5 instances) - Can cause hangs
2. **Missing coordinator events for lifecycle** (30+ instances) - Policy violation
3. **Not using WaitGroupManager** (20+ instances) - Missing observability

### Medium (Should Fix Eventually)
1. **Test code violations** (90+ instances) - Lower priority but should be addressed
2. **Missing context checks** (10 instances) - Can cause cancellation issues
3. **Shutdown pattern violations** (5 instances) - Can cause resource leaks

## Recommended Action Plan

### Phase 1: Critical Fixes (Immediate)
1. Fix all `wg.Wait()` without timeout (8 instances)
2. Replace direct `go func()` with builder (6 instances)
3. Add coordinator events for user-facing messages (20+ instances)

### Phase 2: High Priority (Next Sprint)
1. Add timeouts to wait goroutines (5 instances)
2. Add coordinator events for lifecycle (30+ instances)
3. Migrate to WaitGroupManager for key components (10 instances)

### Phase 3: Medium Priority (Backlog)
1. Fix test code violations (90+ instances)
2. Add missing context checks (10 instances)
3. Complete WaitGroupManager migration (remaining 10 instances)

## Metrics and Tracking

### Current State
- **Total Production Violations**: ~100
- **Total Test Violations**: ~150
- **Pattern Compliance**: ~70% overall
- **Critical Violations**: 34

### Target State
- **Production Violations**: 0
- **Test Violations**: 0 (or acceptable exceptions documented)
- **Pattern Compliance**: 100%
- **Critical Violations**: 0

## References

- [Concurrency Patterns v1.0](../docs/architecture/architecture/concurrency-patterns-v1.0.md)
- [Goroutine Architecture Policy](../docs/architecture/README.md)
- [GOROUTINE_VIOLATIONS.md](./GOROUTINE_VIOLATIONS.md)
- [GOROUTINE_NON_DETERMINISTIC_PATTERNS.md](./GOROUTINE_NON_DETERMINISTIC_PATTERNS.md)
- [COORDINATOR_PATTERN_VIOLATIONS.md](./COORDINATOR_PATTERN_VIOLATIONS.md)
