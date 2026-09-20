# Refactoring Plan: handlers.go

**File:** `pkg/scheduler/handlers.go`  
**Original Size:** 1,548 lines  
**Final Size:** 67 lines (95% reduction)  
**Target:** Split into focused files (~300-500 lines each)  
**Status:** ✅ ALL PHASES COMPLETE (Phases 1-4)

## Analysis

### Handler Distribution
The file contains multiple job handler implementations:

1. **CachePrewarmHandler** (~400 lines)
   - Main Execute method
   - prewarmSpecCache
   - prewarmLifecycleCache
   - prewarmFieldRegistry
   - prewarmSystemFieldsRegistry
   - prewarmHashRegistries
   - prewarmObjectIDCache

2. **LifecycleCheckHandler** (~15 lines)
   - Simple handler for lifecycle checks

3. **AuditAggregationHandler** (~250 lines)
   - Execute method
   - preExecutionHealthCheck

4. **ChangeJournalAggregationHandler** (~225 lines)
   - Execute method
   - preExecutionHealthCheck

5. **AggregationMetricsCleanupHandler** (~130 lines)
   - Execute method

6. **GenericMetricsCleanupHandler** (~185 lines)
   - Execute method

7. **AutofixBatchCleanupHandler** (~195 lines)
   - Execute method

8. **NoOpHandler** (~18 lines)
   - Placeholder handler

9. **Shared Helper** (~70 lines)
   - preExecutionHealthCheck (standalone function)

### Proposed File Structure

#### 1. `handlers_core.go` (~50 lines)
**Purpose:** Core interface and simple handlers

**Contents:**
- `JobHandler` interface
- `NoOpHandler` struct and methods
- `LifecycleCheckHandler` struct and methods

**Dependencies:** Core scheduler types

---

#### 2. `handlers_cache_prewarm.go` (~400 lines)
**Purpose:** Cache pre-warming handler

**Contents:**
- `CachePrewarmHandler` struct
- `NewCachePrewarmHandler` constructor
- `Execute` method
- All prewarm helper methods (prewarmSpecCache, prewarmLifecycleCache, etc.)

**Dependencies:** Core, objects, storage

---

#### 3. `handlers_aggregation.go` (~500 lines)
**Purpose:** Aggregation handlers (audit, change journal, metrics cleanup)

**Contents:**
- `AuditAggregationHandler` struct and methods
- `ChangeJournalAggregationHandler` struct and methods
- `AggregationMetricsCleanupHandler` struct and methods
- `GenericMetricsCleanupHandler` struct and methods
- `preExecutionHealthCheck` shared helper function

**Dependencies:** Core, storage

---

#### 4. `handlers_cleanup.go` (~200 lines)
**Purpose:** Cleanup handlers

**Contents:**
- `AutofixBatchCleanupHandler` struct and methods

**Dependencies:** Core, storage

---

## Migration Strategy

### Phase 1: Extract Cache Prewarm Handler (Low Risk) - ✅ COMPLETE
1. ✅ Create `handlers_cache_prewarm.go` (397 lines)
2. ✅ Move CachePrewarmHandler and all its methods:
   - CachePrewarmHandler struct
   - NewCachePrewarmHandler constructor
   - Execute method
   - prewarmSpecCache, prewarmLifecycleCache, prewarmFieldRegistry
   - prewarmSystemFieldsRegistry, prewarmHashRegistries, prewarmObjectIDCache
3. ✅ Test - Build successful

### Phase 2: Extract Aggregation Handlers (Low Risk) - ✅ COMPLETE
1. ✅ Create `handlers_aggregation.go` (892 lines)
2. ✅ Move AuditAggregationHandler, ChangeJournalAggregationHandler
3. ✅ Move AggregationMetricsCleanupHandler, GenericMetricsCleanupHandler
4. ✅ Move preExecutionHealthCheck helper function
5. ✅ Test - Build successful

### Phase 3: Extract Cleanup Handler (Low Risk) - ✅ COMPLETE
1. ✅ Create `handlers_cleanup.go` (228 lines)
2. ✅ Move AutofixBatchCleanupHandler with Execute method
3. ✅ Test - Build successful

### Phase 4: Finalize Core (Low Risk)
1. Keep JobHandler interface, NoOpHandler, LifecycleCheckHandler in core
2. Final testing

## Testing Strategy

1. **Unit Tests:** Each extracted file should have corresponding test file
2. **Integration Tests:** Full handler operations
3. **Regression Tests:** Run full test suite after each phase

## Risk Mitigation

- **Incremental:** One handler group at a time
- **Test After Each Phase:** Don't proceed until tests pass
- **Review Dependencies:** Ensure imports are correct
