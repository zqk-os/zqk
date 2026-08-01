# Memory and Goroutine Explosion Analysis

**Date**: 2026-02-19  
**Context**: Processes consuming 7-8GB RAM each during bulk operations, with high CPU usage and potential goroutine accumulation.

## Critical Issues Identified

### 1. **BulkResult.Results Accumulates All Objects in Memory** ⚠️ HIGH IMPACT

**Location**: `pkg/storage/object_storage_file_bulk.go`, `pkg/storage/object_storage_graph_bulk.go`

**Problem**:
- `BulkCreate` stores every successfully created object in `result.Results` (line 97: `result.Results = append(result.Results, obj)`)
- `BulkUpdate` reads and stores every updated object (line 179: `result.Results = append(result.Results, updated)`)
- `BulkGet` stores every retrieved object (line 221: `result.Results = append(result.Results, obj)`)

**Impact**:
- For bulk operations creating/updating thousands of objects, this accumulates all objects in memory
- Each object is a `map[string]any` with full YAML content
- With 1000+ objects, this can easily consume 7-8GB RAM
- Memory is held until the operation completes and results are formatted/output

**Evidence**:
- Processes showing 7-8GB RAM usage during bulk create/update operations
- `BulkResult.Results` is only used for output formatting (see `cmd/zqk/object/bulk_helpers.go`)

**Recommendation**:
- **Option A**: Don't store full objects in `Results` - only store IDs and metadata needed for output
- **Option B**: Stream results to output instead of accumulating in memory
- **Option C**: Add a flag to control whether full objects are returned (default: false for large operations)

### 2. **Nested Validation Goroutines** ⚠️ MEDIUM-HIGH IMPACT

**Location**: `pkg/validation/async_validator_worker.go`

**Problem**:
- Each validation worker spawns a validation goroutine (line 287)
- Each validation goroutine spawns an "inner" validation goroutine (line 403)
- This creates nested goroutines: worker → validation goroutine → inner validation goroutine

**Goroutine Count**:
- Workers: `maxWorkers` (default: 20)
- Validation goroutines: limited by semaphore `NumCPU * 2` (e.g., 16 on 8-core)
- Inner validation goroutines: one per validation goroutine (16)
- **Total**: 20 + 16 + 16 = 52 goroutines minimum
- If validation is slow or stuck, goroutines accumulate beyond this

**Impact**:
- Each goroutine consumes memory (stack: ~2KB, plus heap for closures/channels)
- OS thread creation (Go runtime maps goroutines to threads)
- If validation hangs, goroutines don't clean up, leading to accumulation

**Evidence**:
- Logs show validation workers timing out: "Worker still active after stop timeout"
- Async validator logs show workers stuck in "waiting" state

**Recommendation**:
- Remove the inner validation goroutine - run validation directly in the validation goroutine
- The inner goroutine was added to detect hangs, but timeouts already handle this
- Use context cancellation instead of nested goroutines for timeout detection

### 3. **Reference Validation File System Calls** ⚠️ MEDIUM IMPACT

**Location**: `pkg/storage/object_storage_file_validation.go`

**Problem**:
- `validateReferences` calls `os.Stat()` for each referenced object (lines 455, 580, 607)
- `getObjectFilePath()` is called for each reference (lines 453, 601)
- For objects with many references, this creates many file system calls

**Impact**:
- File system calls add latency and can block goroutines
- With many objects having many references, this multiplies I/O operations
- Each `os.Stat()` call may trigger directory reads

**Evidence**:
- High CPU usage during bulk operations (100-128% CPU per process)
- Processes spending time in `readdir_r`/`open` syscalls

**Recommendation**:
- Batch file existence checks where possible
- Use cache for recently checked references
- Consider deferring non-critical reference validation for bulk operations

### 4. **Validation State Cache Growth** ⚠️ MEDIUM IMPACT

**Location**: `pkg/validation/state_cache_shared.go`, `pkg/validation/async_validator_lifecycle.go`

**Problem**:
- Validation state cache accumulates state for all validated objects
- Cache is saved to disk but may grow large in memory
- Logs show cache save timeouts: "Timeout saving validation cache - cache may not be saved" with `state_count: 15373`

**Impact**:
- Large in-memory cache structures
- Cache save operations timing out, indicating large state counts
- Memory not released until cache is saved

**Evidence**:
- Log events show cache save timeouts with high state counts (7265, 12252, 15373)
- Cache save timeout increases with state count (1m2s, 1m32s, 1m50s)

**Recommendation**:
- Implement cache size limits with LRU eviction
- Save cache incrementally instead of all-at-once
- Consider periodic cache saves during long-running operations

### 5. **Bulk Operation Error Accumulation** ⚠️ LOW-MEDIUM IMPACT

**Location**: `pkg/storage/object_storage_file_bulk.go`

**Problem**:
- `result.Errors` accumulates error information for each failed operation
- Each error stores the full error object and message

**Impact**:
- Less critical than Results accumulation, but still adds memory
- Error messages may be large if they include object data

**Recommendation**:
- Store minimal error information (ID, index, message) instead of full error objects
- Consider limiting error detail for very large bulk operations

## Summary of Root Causes

1. **Primary**: `BulkResult.Results` storing all objects in memory (7-8GB for large operations)
2. **Secondary**: Nested validation goroutines creating unnecessary goroutine overhead
3. **Tertiary**: File system I/O from reference validation multiplying with many references
4. **Tertiary**: Validation state cache growing unbounded

## Immediate Actions

1. **Fix BulkResult.Results accumulation** (highest priority)
   - Modify bulk operations to not store full objects
   - Update output formatting to work with IDs/metadata only
   - Add option to return full objects only when explicitly requested

2. **Simplify validation goroutine structure**
   - Remove inner validation goroutine
   - Use context cancellation for timeout detection

3. **Optimize reference validation**
   - Batch file existence checks
   - Cache recent reference checks
   - Defer non-critical validation for bulk operations

4. **Limit validation cache growth**
   - Implement LRU eviction
   - Save cache incrementally

## Testing Recommendations

- Test bulk operations with 1000+ objects and monitor memory usage
- Test validation with many objects and monitor goroutine count
- Test reference validation with objects having many references
- Monitor cache size during long-running validation operations
