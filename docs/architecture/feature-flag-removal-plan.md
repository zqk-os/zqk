# Feature Flag Removal Plan - I/O Queue Routing

**Date:** 2026-01-19  
**Status:** Ready for Implementation  
**Purpose:** Plan for removing `io_queue_routing` feature flag and legacy code

## Current Status

### System Health Assessment

**On-Demand Pattern Implementations:**
- ✅ **8 components** successfully converted to on-demand pattern
- ✅ **All tests passing** - comprehensive test coverage
- ✅ **Coordinator integration** - unified observability
- ✅ **Shutdown coordination** - graceful shutdown management

**Test Results:**
- ✅ IOQueue tests: All passing (8 tests)
- ✅ AsyncRouter tests: All passing (5 tests)
- ✅ Validation Async Workers tests: All passing (7 tests)
- ✅ Operation Executor tests: All passing
- ✅ Other on-demand components: All passing

**System Stability:**
- ✅ No known issues with on-demand patterns
- ✅ No deadlocks or race conditions detected
- ✅ Graceful shutdown working correctly
- ✅ Coordinator integration operational

## Feature Flag Analysis

### Current Usage

**Feature Flag:** `io_queue_routing`
- **Location:** `pkg/featureflags/feature_flags.go`
- **Default:** Disabled (`false`)
- **Purpose:** Routes file I/O operations through I/O queues

**Code Locations:**
1. `pkg/storage/object_storage_file.go`:
   - `readObjectFile()` - checks flag, routes to `readObjectFileViaQueue()` if enabled
   - `writeObjectFileWithPermAndData()` - checks flag, routes to `writeObjectFileViaQueue()` if enabled

**Legacy Code:**
- `readObjectFileViaQueue()` - I/O queue implementation
- `writeObjectFileViaQueue()` - I/O queue implementation
- Direct I/O code in `readObjectFile()` and `writeObjectFileWithPermAndData()`

## Removal Plan

### Step 1: Make I/O Queue Routing the Default

**Changes:**
1. Remove feature flag check from `readObjectFile()`
2. Remove feature flag check from `writeObjectFileWithPermAndData()`
3. Make `readObjectFileViaQueue()` the primary implementation
4. Make `writeObjectFileViaQueue()` the primary implementation
5. Remove `getFeatureFlags()` method (if only used for I/O queue routing)

**Files to Modify:**
- `pkg/storage/object_storage_file.go`

### Step 2: Remove Feature Flag

**Changes:**
1. Remove `FlagIOQueueRouting` constant from `pkg/featureflags/feature_flags.go`
2. Remove `io_queue_routing` flag from `initializeDefaults()`
3. Update any documentation referencing the flag

**Files to Modify:**
- `pkg/featureflags/feature_flags.go`

### Step 3: Clean Up Legacy Code

**Decision:** Keep or remove direct I/O code?

**Option A: Keep Direct I/O as Fallback**
- Pros: Safety net if I/O queue has issues
- Cons: Maintains two code paths, complexity

**Option B: Remove Direct I/O (Recommended)**
- Pros: Simpler codebase, single code path, forces I/O queue usage
- Cons: No fallback if I/O queue fails

**Recommendation:** **Option B** - Remove direct I/O code
- I/O queue is well-tested and stable
- On-demand pattern is proven
- Simpler codebase is easier to maintain
- If I/O queue fails, we should fix it, not fall back

### Step 4: Update Documentation

**Files to Update:**
- `docs/architecture/README.md` - Remove feature flag references
- `docs/architecture/README.md` - Update status
- Any other docs referencing the feature flag

## Implementation Steps

1. ✅ **Verify system health** - All tests passing, no known issues
2. ⏳ **Remove feature flag checks** - Make I/O queue routing default
3. ⏳ **Remove feature flag definition** - Clean up feature flags
4. ⏳ **Remove legacy direct I/O code** - Simplify codebase
5. ⏳ **Update documentation** - Remove feature flag references
6. ⏳ **Run full test suite** - Verify everything still works
7. ⏳ **Commit and push** - Finalize changes

## Risk Assessment

**Low Risk:**
- I/O queue is well-tested
- On-demand pattern is proven
- All tests passing
- No known issues

**Mitigation:**
- Run full test suite before removal
- Keep feature flag removal in separate commit for easy rollback
- Monitor system after deployment

## Timeline

**Estimated Time:** 30-60 minutes
- Code changes: 15-20 minutes
- Test verification: 10-15 minutes
- Documentation: 5-10 minutes
- Final testing: 10-15 minutes
