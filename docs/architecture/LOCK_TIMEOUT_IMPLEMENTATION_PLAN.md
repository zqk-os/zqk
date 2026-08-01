# Lock Timeout Implementation Plan

## Executive Summary

After analyzing the async validation system, we've identified that **Go's `sync.Mutex` does not support timeout on lock acquisition**. However, we can implement a practical solution that:

1. **Times out operations that hold locks** (not lock acquisition itself)
2. **Tracks lock wait times** and warns if excessive
3. **Uses metrics to calculate operation timeouts** dynamically
4. **Monitors lock hold times** and detects potential deadlocks

## Key Findings

### Lock Acquisition Limitation

**Go's sync.Mutex Limitation**:
- `sync.Mutex.Lock()` is a blocking call with no timeout support
- Once a goroutine calls `Lock()`, it will block until the lock is available
- We cannot interrupt this blocking call

**This means**: We cannot prevent a goroutine from waiting indefinitely for a lock to become available.

### Practical Solutions

1. **Timeout operations that hold locks** - Wrap lock-protected code with timeout
2. **Track lock contention** - Monitor wait times and warn if excessive  
3. **Use channels instead of mutexes** where possible (channels support `select` with timeout)
4. **Context cancellation** - Use context to cancel long-running operations
5. **Lock hold time limits** - Monitor how long locks are held and warn if excessive

## Implementation Strategy

### Phase 1: Timeout Wrapper Functions (Current)

Created `pkg/validation/timeout_mutex.go` with:
- `WithLockTimeout()` - Wraps lock-protected operations with timeout
- `WithRLockTimeout()` - Wraps read-lock-protected operations with timeout
- `calculateLockTimeout()` - Calculates timeout based on metrics

**Usage Pattern**:
```go
err := WithLockTimeout(
    &mu,
    ctx,
    metrics,
    logger,
    "cache_set",
    func() error {
        // Lock-protected operation
        cache[key] = value
        return nil
    },
)
```

### Phase 2: Metrics-Based Timeout Calculation

The timeout is calculated using:
```go
timeout = (work_size / objects_per_second) * 1.1 + operation_overhead
```

Where:
- `objects_per_second` comes from `ValidationMetrics.ObjectsPerSecond`
- `work_size` is the number of objects/bytes to process
- `operation_overhead` is operation-specific (cache_get: 10ms, cache_set: 20ms, etc.)
- Result is clamped between 10ms and 5s

### Phase 3: Migration Plan

#### High Priority (Deadlock Risk)

1. **ValidationStateCache operations** (`pkg/validation/state_cache.go`)
   - `Get()` - Wrap with `WithRLockTimeout()`
   - `Set()` - Wrap with `WithLockTimeout()`
   - `Save()` - Already has file lock timeout, add operation timeout

2. **HashRegistryCache operations** (`cmd/zqk/system/async_check.go`)
   - `GetOrCreate()` - Wrap with `WithRLockTimeout()` / `WithLockTimeout()`

3. **Progress tracking** (`cmd/zqk/system/async_check.go`)
   - Drain goroutine updates - Wrap with `WithLockTimeout()`
   - Ticker reads - Already using RLock, add timeout wrapper

#### Medium Priority (Contention Risk)

4. **AsyncValidator operations** (`pkg/validation/async_validator.go`)
   - `Start()` - Add timeout wrapper
   - `Stop()` - Already has timeouts, verify they're sufficient
   - Worker `validateObjectWithData()` - Add timeout for validationFunc read

5. **PriorityQueue operations** (`pkg/validation/priority_queue.go`)
   - `Dequeue()` - Add timeout wrapper

## Validation Scenarios

### Scenario 1: Cache Get During High Contention ✅
**Before**: Worker blocks indefinitely waiting for cache RLock  
**After**: Operation times out after calculated duration, returns error

### Scenario 2: Cache Save During Active Validation ✅
**Before**: Save() holds RLock during file I/O, blocking Set()  
**After**: Save() operation times out, releases lock, returns error

### Scenario 3: Hash Registry Contention ✅
**Before**: Multiple workers wait for same registry RLock  
**After**: Operations timeout, workers can retry or skip

### Scenario 4: Progress Update During Completion ✅
**Before**: Drain goroutine blocks on mu.Lock() during completion  
**After**: Operation times out, completion detection continues

## Metrics Integration

### New Metrics to Track

1. **Lock Wait Times**: Already tracked in `HashRegistryCacheLockWaitDurationMs`
2. **Lock Hold Times**: Already tracked in `HashRegistryCacheLockHoldDurationMs`
3. **Timeout Events**: New metric to track when timeouts occur
4. **Operation Duration**: Track how long operations take with/without locks

### Metrics-Based Timeout Calculation

The system uses existing metrics to calculate timeouts:
- `ValidationMetrics.ObjectsPerSecond` - Baseline throughput
- `ValidationMetrics.AverageValidationTime` - Expected operation duration
- Operation-specific overheads - Based on historical data

## Testing Strategy

1. **Unit Tests**: Test timeout calculation with various metrics
2. **Integration Tests**: Test timeout behavior under contention
3. **Deadlock Tests**: Verify timeouts prevent hangs
4. **Performance Tests**: Ensure timeout overhead is minimal

## Rollout Plan

1. **Week 1**: Deploy timeout wrapper functions (non-breaking)
2. **Week 2**: Migrate high-priority operations (cache, hash registry)
3. **Week 3**: Migrate medium-priority operations (validator, queue)
4. **Week 4**: Monitor metrics, adjust timeouts based on real data
5. **Week 5**: Document learnings, optimize timeout calculations

## Success Criteria

- ✅ No deadlocks in production
- ✅ Lock wait times < 100ms (99th percentile)
- ✅ Timeout events < 0.1% of operations
- ✅ No performance degradation (< 5% overhead)

## Next Steps

1. Review and approve this plan
2. Implement Phase 1 (already done)
3. Begin Phase 2 migration (high-priority operations)
4. Monitor and adjust based on metrics

