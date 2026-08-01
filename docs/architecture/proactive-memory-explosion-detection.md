# Proactive Memory Explosion Detection Strategy

**Date**: 2026-02-19  
**Context**: After fixing memory explosion from `EnqueueValidationForObject` creating new storage per object, we need systematic ways to catch similar issues proactively.

## What We Missed

### Previous Investigation Gaps

1. **Focused on visible symptoms, not root causes**
   - Fixed `BulkResult.Results` accumulation (visible in memory profiles)
   - Fixed nested goroutines (visible in goroutine counts)
   - **Missed**: `EnqueueValidationForObject` creating storage per object (not visible in initial profiles)

2. **Didn't trace call paths comprehensively**
   - Looked at bulk operations directly
   - Didn't trace "what happens when objects are created/updated?"
   - Didn't check background/async paths that trigger per-object

3. **No systematic pattern detection**
   - Relied on memory profiling after the fact
   - No static analysis for "expensive init in loops"
   - No runtime monitoring for repeated expensive operations

## Detection Strategies

### 1. Static Analysis Patterns

#### Pattern: Expensive Initialization in Loops/Goroutines

**What to detect:**
- Functions that create expensive resources (`NewFileObjectStorage`, `NewStorageFactory`, `LoadSpecWithInheritance`, etc.)
- Called inside loops, goroutines, or per-item callbacks
- Not using a cache or singleton pattern

**Detection approach:**
```go
// BAD PATTERN (detect this):
for _, obj := range objects {
    storage, _ := storage.NewFileObjectStorage(projectRoot) // ❌ Creates storage per object
    process(obj, storage)
}

// GOOD PATTERN (should use):
storageCache := storage.GetGlobalStorageProviderCache()
for _, obj := range objects {
    storage, _ := storageCache.GetOrCreate(ctx, projectRoot) // ✅ Uses cache
    process(obj, storage)
}
```

**Tools:**
- Custom Go linter rule: "expensive-init-in-loop"
- Grep patterns: `NewFileObjectStorage|NewStorageFactory` followed by `for|range|go func|StartSimple`
- Code review checklist item

#### Pattern: Per-Object Resource Creation

**What to detect:**
- Functions that accept `objectID` or `object` as parameter
- Create storage/validators/loaders inside the function
- Called from loops or async paths

**Detection approach:**
```bash
# Find functions that:
# 1. Take objectID/object as parameter
# 2. Create storage/validators inside
grep -r "func.*objectID.*string" cmd/zqk/system | xargs grep -l "NewFileObjectStorage\|NewStorageFactory"
```

### 2. Runtime Monitoring

#### Memory Profiling in Tests

**Strategy:**
- Add memory profiling to bulk operation tests
- Fail test if memory exceeds threshold (e.g., >100MB per 1000 objects)
- Track allocations per operation

**Implementation:**
```go
func TestBulkCreate_MemoryUsage(t *testing.T) {
    var m1, m2 runtime.MemStats
    runtime.GC()
    runtime.ReadMemStats(&m1)
    
    // Run bulk operation
    result := bulkCreate(1000, objects)
    
    runtime.GC()
    runtime.ReadMemStats(&m2)
    
    allocatedMB := float64(m2.Alloc-m1.Alloc) / 1024 / 1024
    if allocatedMB > 100 {
        t.Errorf("Memory usage too high: %.2f MB for 1000 objects (expected <100MB)", allocatedMB)
    }
}
```

#### Goroutine Counting

**Strategy:**
- Monitor goroutine count during operations
- Fail if goroutine count grows unbounded
- Track goroutine creation rate

**Implementation:**
```go
func TestAsyncOperation_GoroutineLeak(t *testing.T) {
    startGoroutines := runtime.NumGoroutine()
    
    // Run async operation
    runAsyncOperation(1000)
    
    // Wait for completion
    time.Sleep(5 * time.Second)
    runtime.GC()
    
    endGoroutines := runtime.NumGoroutine()
    if endGoroutines > startGoroutines+10 {
        t.Errorf("Goroutine leak: started with %d, ended with %d", startGoroutines, endGoroutines)
    }
}
```

#### Storage Creation Counting

**Strategy:**
- Add counter to `NewFileObjectStorage` and `NewStorageFactory`
- Log warning if called >N times per project root in short period
- Track in metrics/telemetry

**Implementation:**
```go
var (
    storageCreationCount = make(map[string]int)
    storageCreationMu    sync.RWMutex
)

func NewFileObjectStorage(projectRoot string) (*FileObjectStorage, error) {
    storageCreationMu.Lock()
    storageCreationCount[projectRoot]++
    count := storageCreationCount[projectRoot]
    storageCreationMu.Unlock()
    
    if count > 10 {
        logger := logging.GetLoggerFromProfile("system")
        logger.Warn("Multiple storage creations detected - possible memory leak",
            logging.String("project_root", projectRoot),
            logging.Int("count", count))
    }
    
    // ... rest of function
}
```

### 3. Code Review Checklist

**Add to PRE_CHANGE_CHECKLIST.md:**

```markdown
## Memory and Resource Management

- [ ] **No expensive initialization in loops**: Functions that create storage/validators/loaders are not called inside loops or per-object callbacks without caching
- [ ] **Storage reuse**: Storage providers are cached per project root (use `GetGlobalStorageProviderCache()` or similar)
- [ ] **Goroutine bounds**: Async operations use bounded worker pools, not one goroutine per item
- [ ] **Memory accumulation**: Operations on many items don't accumulate all results in memory (stream or use IDs only)
- [ ] **Resource cleanup**: Resources created in tests are properly cleaned up (use `t.Cleanup()`)
```

### 4. Test Patterns

#### Bulk Operation Memory Test

**Pattern:**
- Create test that runs bulk operation with large N
- Measure memory before/after
- Fail if memory usage exceeds threshold

**Location:** `pkg/storage/object_storage_file_bulk_test.go`

```go
func TestBulkCreate_MemoryBounded(t *testing.T) {
    // Test that bulk create doesn't accumulate all objects in memory
    // Should use <100MB for 1000 objects
}
```

#### Async Operation Resource Test

**Pattern:**
- Create test that triggers async operation for many objects
- Monitor storage creation count
- Fail if storage created more than once per project root

**Location:** `cmd/zqk/system/validation_cache_sync_test.go`

```go
func TestEnqueueValidation_ReusesStorage(t *testing.T) {
    // Test that EnqueueValidationForObject reuses cached storage
    // Should create storage once, not per object
}
```

### 5. Profiling Integration

#### Continuous Profiling

**Strategy:**
- Run memory/CPU profiles on CI for bulk operation tests
- Compare profiles across commits
- Alert on significant increases

**Tools:**
- `go test -memprofile` in CI
- Compare heap profiles between runs
- Set thresholds for allocation increases

#### Production Monitoring

**Strategy:**
- Add metrics for storage creation frequency
- Monitor memory usage during bulk operations
- Implement a `system watchdog` out-of-band monitor for memory thresholds, executing `scripts/run-watchdog-monitor.sh` to forcefully alert and restart the daemon if it exceeds 500MB, bypassing the daemon's internal event loop.

**Implementation:**
- Export metrics: `storage_creation_count{project_root}`
- Track memory usage: `process_memory_bytes`
- Out-of-band system watchdog script: `scripts/run-watchdog-monitor.sh` 

## Potential Latent Issues

### 1. `auto_fix_helpers_hash_fixing.go`

**Location:** `cmd/zqk/system/auto_fix_helpers_hash_fixing.go:28`

**Issue:** Creates `NewStorageFactory` if `storageProvider == nil`, could be called per object during auto-fix

**Risk:** MEDIUM - Only called if storageProvider is nil, but if called in loop, creates storage per object

**Fix:** Use cached storage provider

### 2. `check_async_baseline.go`

**Location:** `cmd/zqk/system/check_async_baseline.go:211`

**Issue:** Creates `NewFileObjectStorage` for timeout event emission, could be called repeatedly

**Risk:** LOW - Only called on timeout, but should still use cache

**Fix:** Use cached storage provider

### 3. `async_check_non_blocking.go`

**Location:** `cmd/zqk/system/async_check_non_blocking.go:44`

**Issue:** Creates `NewFileObjectStorage` for coordinator, could be called per async check

**Risk:** LOW - Only called once per command, but should use cache for consistency

**Fix:** Use cached storage provider

### 4. `list_helpers.go`

**Location:** `pkg/zqkcli/list_helpers.go:304,311`

**Issue:** Creates `NewFileObjectStorage` as fallback when graph unavailable, could be called per list operation

**Risk:** LOW - Fallback path, but should use cache

**Fix:** Use cached storage provider

### 5. `aggregate_audit.go`

**Location:** `cmd/zqk/system/aggregate_audit.go:189,195`

**Issue:** Creates `NewFileObjectStorage` as fallback, could be called per aggregation

**Risk:** LOW - Fallback path, but should use cache

**Fix:** Use cached storage provider

## Action Items

### Immediate (High Priority)

1. **Add static analysis check**
   - Create linter rule or grep pattern for "expensive init in loop"
   - Add to pre-commit hooks or CI

2. **Fix remaining storage creation issues**
   - Update `auto_fix_helpers_hash_fixing.go` to use cache
   - Update other fallback paths to use cache

3. **Add memory tests**
   - Create `TestBulkCreate_MemoryBounded`
   - Create `TestEnqueueValidation_ReusesStorage`

### Short-term (Medium Priority)

4. **Add runtime monitoring**
   - Add storage creation counter with warnings
   - Add goroutine leak detection in tests

5. **Update code review checklist**
   - Add memory/resource management section to PRE_CHANGE_CHECKLIST.md

6. **Document patterns**
   - Add "expensive init in loop" anti-pattern to docs
   - Document caching patterns for storage/validators

### Long-term (Low Priority)

7. **Continuous profiling**
   - Set up memory profiling in CI
   - Compare profiles across commits

8. **Production monitoring**
   - Add metrics for storage creation
   - Alert on memory spikes

## References

- `docs/architecture/memory-explosion-analysis.md` - Previous analysis
- `docs/architecture/PRE_CHANGE_CHECKLIST.md` - Code review checklist
- `pkg/storage/resource_cache.go` - Caching abstraction
- `docs/architecture/unbounded-concurrency-fixes.md` - Concurrency patterns
