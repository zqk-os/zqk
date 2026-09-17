# Async Validation Control Flow & Lock Analysis

**Last Verified:** 2026-08-31


## System Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                    runCheckAsync (Main Thread)                   │
│  - Discovers objects                                             │
│  - Enqueues tasks to AsyncValidator                              │
│  - Creates OutputQueue & OutputWriter                            │
│  - Sets up result callback for direct output writing            │
│  - Calls showValidationProgressWithMetrics                        │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│          showValidationProgressWithMetrics (Main Thread)         │
│                                                                   │
│  Mutex: mu (sync.RWMutex)                                        │
│    - Protects: completed, failed, completedObjectIDs,            │
│                failedObjectIDs                                   │
│    - Lock hold time: MINIMAL (released immediately after update) │
│                                                                   │
│  OutputQueue: FIFO queue for all output                          │
│    - Progress messages → stderr handler                           │
│    - Results → stdout/file handler                               │
│    - Non-blocking enqueue (drops if full)                         │
│                                                                   │
│  OutputWriter: Single goroutine for all I/O                      │
│    - Dequeues packets from OutputQueue (non-blocking)            │
│    - Routes to appropriate handlers (stderr/stdout/file)          │
│                                                                   │
│  Goroutines:                                                     │
│    1. Drain Goroutine (reads progressChan)                      │
│    2. OutputWriter (processes OutputQueue)                       │
│    3. Main Loop (ticker-based completion detection)             │
└─────────────────────────────────────────────────────────────────┘
                              │
                    ┌─────────┴─────────┐
                    ▼                   ▼
┌──────────────────────────┐  ┌──────────────────────────┐
│   Drain Goroutine        │  │   AsyncValidator         │
│                          │  │                          │
│  Reads: progressChan     │  │  Mutex: av.mu            │
│  Writes: mu (Lock)       │  │    - Protects: running,  │
│                          │  │      progressChan,       │
│  Operations:             │  │      validationFunc,     │
│  - mu.Lock()             │  │      resultCallback      │
│  - Update counters       │  │                          │
│  - mu.Unlock() IMMEDIATE │  │  Workers: N goroutines   │
│  - Log/metrics (no lock) │  │    - Dequeue tasks       │
│                          │  │    - Launch validation  │
│  Exit: When progressChan │  │      goroutines          │
│        closes            │  │                          │
└──────────────────────────┘  └──────────────────────────┘
                                        │
                                        ▼
                          ┌──────────────────────────┐
                          │  Validation Goroutines  │
                          │  (One per object)         │
                          │                          │
                          │  WaitGroup: validationWg │
                          │    - Separate from wg    │
                          │    - Stop() doesn't wait │
                          │                          │
                          │  Operations:             │
                          │  - av.mu.RLock()         │
                          │    (read callback)       │
                          │  - av.mu.RUnlock()       │
                          │  - Validate object       │
                          │  - stateCache.Set()      │
                          │    (c.mu.Lock())         │
                          │  - callback() (write)    │
                          │    (non-blocking queue)  │
                          │  - progressChan (non-block)│
                          └──────────────────────────┘
                                        │
                                        ▼
                          ┌──────────────────────────┐
                          │   ValidationStateCache   │
                          │                          │
                          │  Mutex: c.mu (RWMutex)   │
                          │    - Protects: cache map │
                          │                          │
                          │  Operations:             │
                          │  - Get() - RLock         │
                          │  - Set() - Lock          │
                          │  - GetAll() - RLock       │
                          │  - Save() - RLock + file  │
                          └──────────────────────────┘
                                        │
                                        ▼
                          ┌──────────────────────────┐
                          │   HashRegistryCache      │
                          │                          │
                          │  Mutex: mu (RWMutex)      │
                          │    - Protects: registry │
                          │      instances map       │
                          │                          │
                          │  OPTIMIZATION:           │
                          │  - Non-bucketed: RLock() │
                          │    → Get registry        │
                          │    → RUnlock()          │
                          │    → Validate (no lock) │
                          │    → Lock() (if update) │
                          │  - Bucketed: sync.Map    │
                          │    (no global lock)      │
                          │                          │
                          │  Operations:            │
                          │  - GetOrCreate()         │
                          │    - RLock() (non-bucket)│
                          │    - sync.Map (bucketed) │
                          └──────────────────────────┘
```

## Detailed Goroutine Flow

### 1. Main Thread (runCheckAsync → showValidationProgressWithMetrics)

```
runCheckAsync()
  │
  ├─> Discover objects
  │
  ├─> For each object:
  │     ├─> asyncValidator.Enqueue()
  │     │     ├─> Check cache (c.mu.RLock)
  │     │     ├─> If cache miss: Enqueue to priorityQueue
  │     │     └─> If cache hit: Send progress (progressChan, non-blocking)
  │     │
  │     └─> Record metrics
  │
  ├─> Create OutputQueue & OutputWriter
  │
  ├─> Set up result callback (writes output directly)
  │
  └─> showValidationProgressWithMetrics()
        │
        ├─> Start drain goroutine
        │     └─> Reads progressChan, updates mu-protected counters
        │
        └─> Main loop (ticker-based)
              ├─> Read mu-protected counters (RLock)
              ├─> Check completion conditions
              ├─> If complete:
              │     ├─> validator.Stop()
              │     ├─> Wait for drainDone
              │     └─> Collect results
              │
              └─> If timeout:
                    ├─> validator.Stop()
                    └─> Collect partial results
```

### 2. Drain Goroutine

```
Drain Goroutine
  │
  └─> Loop:
        ├─> select {
        │     ├─> <-ctxTimeout.Done(): Exit
        │     └─> progress, ok := <-progressChan:
        │           ├─> If !ok: Exit (channel closed)
        │           └─> If ok:
        │                 ├─> mu.Lock() ⚠️ NO TIMEOUT
        │                 ├─> Update completed/failed counters
        │                 ├─> Update completedObjectIDs map
        │                 ├─> mu.Unlock() IMMEDIATE ⚠️ CRITICAL
        │                 ├─> metrics.Increment*() (no lock)
        │                 └─> logger.Debug() (no lock)
        │
        └─> Continue loop
```

**Key Optimization**: Lock is released IMMEDIATELY after updating counters, before logging/metrics. This minimizes lock hold time and prevents blocking the ticker loop.

### 3. AsyncValidator Workers (N goroutines)

```
Worker Goroutine (N instances)
  │
  └─> Loop:
        ├─> Dequeue task from priorityQueue
        │     └─> priorityQueue.mu.Lock() ⚠️ NO TIMEOUT
        │
        ├─> Launch validation goroutine (per object)
        │     └─> av.validationWg.Add(1)
        │     └─> go func() {
        │           ├─> av.mu.RLock() ⚠️ NO TIMEOUT
        │           │     └─> Read resultCallback
        │           ├─> av.mu.RUnlock()
        │           │
        │           ├─> Validate object:
        │           │     ├─> c.mu.RLock() ⚠️ NO TIMEOUT (Get cached state)
        │           │     ├─> c.mu.RUnlock()
        │           │     │
        │           │     ├─> Hash Registry:
        │           │     │     ├─> If bucketed:
        │           │     │     │     └─> hashRegistryPool.GetOrCreate()
        │           │     │     │           └─> sync.Map (no global lock)
        │           │     │     │
        │           │     │     └─> If non-bucketed:
        │           │     │           ├─> hashRegistryCache.mu.RLock()
        │           │     │           ├─> Get registry
        │           │     │           ├─> hashRegistryCache.mu.RUnlock()
        │           │     │           ├─> Validate (no lock held)
        │           │     │           └─> Lock() only if update needed
        │           │     │
        │           │     ├─> c.mu.Lock() ⚠️ NO TIMEOUT (Set result)
        │           │     └─> c.mu.Unlock()
        │           │
        │           ├─> callback() (write output, non-blocking)
        │           │     └─> outputQueue.Enqueue() (drops if full)
        │           │
        │           └─> progressChan <- (non-blocking, drops if full)
        │         }()
        │
        └─> Continue loop (worker doesn't wait for validation goroutine)
```

**Key Changes**:
- Workers launch validation goroutines (one per object)
- Validation goroutines use separate `validationWg` (Stop() doesn't wait for them)
- All progress channel sends are non-blocking (drop if full)
- Output writes are non-blocking (drop if queue full)
- Hash registry lock is released before validation (non-bucketed)

### 4. AsyncValidator.Stop()

```
Stop()
  │
  ├─> av.mu.Lock() ⚠️ NO TIMEOUT
  │     ├─> Check if running
  │     ├─> Set running = false
  │     └─> av.cancel()
  ├─> av.mu.Unlock()
  │
  ├─> Wait for workers only (with timeout)
  │     └─> av.wg.Wait() (does NOT wait for validation goroutines)
  │
  ├─> av.mu.Lock() ⚠️ NO TIMEOUT
  │     └─> close(progressChan)
  ├─> av.mu.Unlock()
  │
  └─> Save cache (with timeout)
        └─> c.mu.RLock() ⚠️ NO TIMEOUT
              └─> Save to disk (file lock)
```

**Key Change**: `Stop()` only waits for worker goroutines (`wg`), not validation goroutines (`validationWg`). This prevents hanging if validation goroutines are stuck.

## Lock Acquisition Points (Current - NO TIMEOUTS)

### Critical Path Locks (High Contention Risk)

| Lock | Location | Operation | Current Timeout | Risk Level | Notes |
|------|----------|-----------|----------------|------------|-------|
| `mu` (RWMutex) | async_check.go:880 | Drain goroutine update | ❌ None | 🟡 MEDIUM | **OPTIMIZED**: Lock released immediately after counter update |
| `mu` (RWMutex) | async_check.go:978 | Ticker read counters | ❌ None | 🟢 LOW | Concurrent reads allowed, minimal contention |
| `av.mu` (RWMutex) | async_validator.go:131 | Start() | ❌ None | 🟢 LOW | One-time operation |
| `av.mu` (RWMutex) | async_validator.go:173 | Stop() | ❌ None | 🟡 MEDIUM | Critical shutdown path, but only waits for workers |
| `av.mu` (RWMutex) | async_validator.go:496,521 | Validation goroutine read callback | ❌ None | 🟢 LOW | Fast RLock, released immediately |
| `c.mu` (RWMutex) | state_cache.go:218 | Get() | ❌ None | 🔴 HIGH | Very frequent |
| `c.mu` (RWMutex) | state_cache.go:252 | Set() | ❌ None | 🔴 HIGH | Very frequent |
| `c.mu` (RWMutex) | state_cache.go:97 | Save() | ❌ None | 🔴 HIGH | File I/O while locked |
| `hashRegistryCache.mu` | async_check.go:513 | RLock() for non-bucketed | ❌ None | 🟡 MEDIUM | **OPTIMIZED**: Released before validation |
| `hashRegistryCache.mu` | async_check.go:526 | Lock() for updates | ❌ None | 🟡 MEDIUM | Only held during registry update |
| `hashRegistryPool` | async_check.go:485 | GetOrCreate() for bucketed | ✅ sync.Map | 🟢 LOW | **OPTIMIZED**: No global lock, fine-grained locking |
| `priorityQueue.mu` | priority_queue.go:76 | Dequeue() | ❌ None | 🟡 MEDIUM | Worker contention |
| `OutputQueue.mu` | output_queue.go:41 | Enqueue() | ❌ None | 🟢 LOW | Fast operation, drops if full |
| `OutputQueue.mu` | output_queue.go:255 | DequeueNonBlocking() | ✅ Non-blocking | 🟢 LOW | Single writer, no contention |

## Deadlock Scenarios

### Scenario 1: Drain Goroutine + Ticker Contention
**Thread 1 (Drain)**: `mu.Lock()` → Update counters → `mu.Unlock()` (immediate)  
**Thread 2 (Ticker)**: `mu.RLock()` → Read counters → `mu.RUnlock()`

**Risk**: Contention but no deadlock (RWMutex allows concurrent readers)  
**Status**: ✅ SAFE (lock hold time minimized)

### Scenario 2: Worker Blocked on Cache Lock
**Thread 1 (Validation Goroutine)**: `c.mu.RLock()` → Get() → `c.mu.RUnlock()`  
**Thread 2 (Stop)**: `av.mu.Lock()` → `c.mu.RLock()` → Save() → (file lock blocks)

**Risk**: If Save() blocks on file lock, worker's RLock() is delayed  
**Status**: ⚠️ POTENTIAL HANG (file lock has timeout, but cache lock doesn't)

### Scenario 3: Multiple Validation Goroutines on Same Hash Registry
**Thread 1 (Validation A)**: `hashRegistryCache.mu.RLock()` → Get registry → `RUnlock()` → Validate  
**Thread 2 (Validation B)**: `hashRegistryCache.mu.RLock()` → Get registry → `RUnlock()` → Validate

**Risk**: Low - lock is released before validation  
**Status**: ✅ SAFE (lock not held during validation)

### Scenario 4: Stop() Called During Active Validation
**Thread 1 (Stop)**: `av.mu.Lock()` → Set running=false → `av.mu.Unlock()` → Wait for workers  
**Thread 2 (Validation Goroutine)**: `av.mu.RLock()` → Read callback → `av.mu.RUnlock()` → (long validation)

**Risk**: Low - RLock doesn't block Stop()'s Lock(), and Stop() doesn't wait for validation goroutines  
**Status**: ✅ SAFE (validation goroutines use separate wait group)

### Scenario 5: Cache Save During High Contention
**Thread 1 (Stop)**: `c.mu.RLock()` → Save() → (file lock)  
**Thread 2 (Validation A)**: `c.mu.RLock()` → Get() → (allowed, RLock)  
**Thread 3 (Validation B)**: `c.mu.Lock()` → Set() → (waits for Save's RLock)

**Risk**: If Save() holds RLock for long (file I/O), Set() waits  
**Status**: ⚠️ POTENTIAL HANG (no timeout on Lock, but Save() has timeout)

### Scenario 6: Progress Channel Full
**Thread 1 (Validation Goroutine)**: `progressChan <-` (non-blocking, drops if full)  
**Thread 2 (Drain)**: `<-progressChan` (blocking read)

**Risk**: Low - sends are non-blocking, drain goroutine processes quickly  
**Status**: ✅ SAFE (non-blocking sends prevent goroutine blocking)

### Scenario 7: Output Queue Full
**Thread 1 (Validation Goroutine)**: `outputQueue.Enqueue()` (drops if full)  
**Thread 2 (OutputWriter)**: `outputQueue.DequeueNonBlocking()` (non-blocking)

**Risk**: Low - enqueue drops if full, writer processes continuously  
**Status**: ✅ SAFE (non-blocking, data loss acceptable for progress)

## Recent Optimizations

### 1. Per-Object Validation Goroutines
**Before**: 
- Workers validate objects sequentially
- Workers block on validation completion

**After**:
- Workers launch validation goroutines (one per object)
- Validation goroutines use separate wait group
- Stop() doesn't wait for validation goroutines
- True parallelism - all objects validate concurrently

**Impact**: 
- ✅ Maximum parallelism
- ✅ Stop() doesn't hang on stuck validation goroutines
- ✅ Better resource utilization

### 2. Non-Blocking Progress Channel
**Before**: 
- Progress channel sends could block if channel full
- Validation goroutines could hang waiting to send

**After**:
- All progress channel sends use `select` with `default`
- Drops progress updates if channel full (best-effort)
- Validation goroutines never block on progress

**Impact**:
- ✅ Validation goroutines never block on progress
- ✅ Progress updates are best-effort (acceptable loss)

### 3. Non-Blocking Output Queue
**Before**: 
- Output queue enqueue could block if queue full
- Validation goroutines could hang waiting to write

**After**:
- Output queue enqueue drops if full (with warning)
- Validation goroutines never block on output

**Impact**:
- ✅ Validation goroutines never block on output
- ✅ Output writes are best-effort (acceptable loss)

### 4. Minimized Lock Hold Time (Drain Goroutine)
**Before**: 
- Drain goroutine used `defer mu.Unlock()`
- Lock held through logging and metrics calls

**After**:
- Lock released immediately after counter update
- Logging and metrics done after lock release

**Impact**:
- ✅ Minimal lock contention with ticker loop
- ✅ Better parallelism

### 5. Hash Registry Lock Optimization (Non-Bucketed)
**Before**: 
- Lock held through entire validation + registry operations
- Serialized all workers for same kind

**After**:
- RLock() → Get registry → RUnlock()
- Validate (no lock held)
- Lock() only if update needed

**Impact**:
- ✅ Parallel validation for non-bucketed objects
- ✅ Lock only held during registry access/update

### 6. Bucketed Objects Use sync.Map
**Before**: All objects used hashRegistryCache with global lock

**After**: 
- Bucketed objects use `hashRegistryPool` (sync.Map)
- No global lock - workers processing different directories don't block
- Fine-grained locking within sync.Map

**Impact**:
- ✅ Eliminates lock contention for bucketed objects
- ✅ Better parallelism

## Lock Sequencing Analysis

### Lock Order (No Circular Dependencies)

1. **Validation Goroutine**:
   - `av.mu.RLock()` → read callback → `av.mu.RUnlock()`
   - `c.mu.RLock()` → Get() → `c.mu.RUnlock()`
   - `hashRegistryCache.mu.RLock()` → Get registry → `RUnlock()`
   - Validate (no locks)
   - `c.mu.Lock()` → Set() → `c.mu.Unlock()`
   - `hashRegistryCache.mu.Lock()` → Update registry → `Unlock()` (if needed)

2. **Drain Goroutine**:
   - `mu.Lock()` → Update counters → `mu.Unlock()` (immediate)

3. **Ticker Loop**:
   - `mu.RLock()` → Read counters → `mu.RUnlock()`

4. **Stop()**:
   - `av.mu.Lock()` → Set running=false → `av.mu.Unlock()`
   - `av.wg.Wait()` (wait for workers only)
   - `av.mu.Lock()` → Close progressChan → `av.mu.Unlock()`
   - `c.mu.RLock()` → Save() → `c.mu.RUnlock()`

**Analysis**: No circular dependencies detected. Lock ordering is consistent:
- `av.mu` → `c.mu` → `hashRegistryCache.mu` → `mu` (progress)
- All locks are released before acquiring next lock
- No nested lock acquisitions

## Current State Summary

### Implemented Optimizations
- ✅ Per-object validation goroutines (maximum parallelism)
- ✅ Separate wait groups (Stop() doesn't wait for validation goroutines)
- ✅ Non-blocking progress channel sends
- ✅ Non-blocking output queue writes
- ✅ Minimized lock hold time (drain goroutine)
- ✅ Hash registry lock released before validation (non-bucketed)
- ✅ Bucketed objects use sync.Map (no global lock)
- ✅ Output queue architecture (single writer pattern)
- ✅ RWMutex for progress tracking (allows concurrent reads)
- ✅ Stuck detection (2-minute timeout triggers stop)

### Remaining Risks
- ⚠️ No timeout on lock acquisition (Go limitation)
- ⚠️ Progress channel can fill up (drops updates, but acceptable)
- ⚠️ Output queue can fill up (drops data, but acceptable)

### Fixed Issues (2026-01-06)
- ✅ **HashRegistry.Save() nested lock**: Fixed RLock → Lock upgrade while holding RLock
- ✅ **HashRegistry.Save() blocking I/O**: Fixed file.Sync() while holding lock
- ✅ **checkObjectWithCacheAndContent redundant I/O**: Fixed redundant file read
- ✅ **Callback blocking**: Made callback async to prevent blocking validation goroutines

### Future Enhancements
- [ ] Timeout-based lock operations (wrap operations, not acquisitions)
- [ ] Dynamic timeout calculation from metrics
- [ ] Lock wait time monitoring and alerting
- [ ] Further reduce lock hold times where possible

## Validation Checklist

For each lock acquisition point:
- [x] Is lock hold time minimized? (Yes - immediate release in drain goroutine)
- [x] Are there circular dependencies? (No - lock order is consistent)
- [x] Are blocking operations non-blocking? (Yes - progress/output are non-blocking)
- [ ] Is there a timeout? (Not possible for acquisition, but can timeout operations)
- [ ] Is timeout calculated from metrics? (Future enhancement)
- [ ] Is there a fallback if metrics unavailable? (Future enhancement)
- [ ] Is timeout logged if exceeded? (Future enhancement)
- [x] Is error handled gracefully? (Yes - stuck detection exists)
- [x] Is deadlock scenario covered? (Yes - non-blocking operations prevent most deadlocks)
