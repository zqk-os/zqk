# Architectural Post-Mortem & Multi-Tier Concurrency Design

**Document ID:** ARC-POSTMORTEM-CONCURRENCY-BUDGET-001  
**Incident:** PID 33048 (`system check --details --verbose`) Unrecoverable Pipeline Deadlock  
**Target:** `zqk` Kernel Concurrency Architecture  
**Status:** PROPOSED ARCHITECTURAL BLUEPRINT  

---

## 1. Executive Incident Summary

During a full repository health validation (`system check --details --verbose`), PID 33048 entered an unrecoverable deadlock at 11:21:31 PDT immediately following the validation of policy `POL-DEBUG-001`. The process remained resident at 297.4MB RSS, stalled with zero CPU activity, until attached with LLDB at 11:23:25 PDT to generate core dump `to-investigate/33048.dump` (5.8GB).

### Root Cause
1. **Commit `b9979027`** introduced `defaultCLIGoroutineBudget = 96` into `cmd/zqk/app/max_os_threads.go`, installing a global process-wide budget of 96 goroutines on CLI startup.
2. The ZQK Knowledge Kernel currently defines **99 distinct object kinds**, and steady-state CLI baseline components (flusher, metrics, write-behind, validation pool) consume **~35 goroutines** before discovery begins.
3. In `cmd/zqk/system/async_check.go` (`discoverFromCache`), discovery iterated over all 99 kinds and scheduled a goroutine per kind using `cacheBuilder.WithBudget(DefaultBudget()).StartSimple(...)`.
4. After ~66 kinds, `DefaultBudget().Reserve(1)` saturated.
5. In `pkg/goroutinelabels/builder.go`, `StartSimple` **silently dropped execution** when `Reserve(1)` returned an error.
6. The discovery coordinator (`discover_from_cache_coordinator`) and result collector (`discover_from_cache_collector`) were subsequently invoked with `WithBudget(DefaultBudget())`. Both were **silently rejected and never spawned**.
7. Because the coordinator was dropped, `close(streamChan)` was never called. Downstream consumer `enqueueFilesAsDiscovered` (`async_check_helpers.go:1090`) blocked indefinitely on `for files := range filesStream`.
8. The entire pipeline deadlocked with zero logs or error messages.

---

## 2. Lessons Learned & Anti-Patterns to Avoid

```
+-------------------------------------------------------------------------------+
|                        ANTI-PATTERNS IDENTIFIED                               |
+-------------------------------------------------------------------------------+
|  1. Shared Budget for Workers & Closers   -> Saturation kills the closer      |
|  2. Silent Task Dropping on Capacity Fail -> Drops work, creating deadlocks   |
|  3. Global Limit < System Entity Count    -> Budget (96) < Kinds (99) + Base  |
|  4. Unbounded Per-Item Goroutine Spawns   -> for _, k := range kinds { go }   |
|  5. Homogeneous Worker Queues             -> Fast units stall behind heavy    |
+-------------------------------------------------------------------------------+
```

### Anti-Pattern 1: Applying Worker Budgets to Coordinators & Channel Closers
- **The Flaw:** Treating lifecycle control-plane routines (goroutines that call `workerWg.Wait()`, close channels, or signal completion) as standard worker tasks competing for the same worker budget.
- **The Result:** When data-plane workers saturate the budget, the very goroutine responsible for completing the phase and releasing resources cannot start, resulting in an unrecoverable deadlock.
- **Guardrail:** **The Unblockable Closer Guarantee.** Channel closers, waitgroup waiters, and cleanup routines must NEVER be throttled or blocked by data-plane worker limits.

### Anti-Pattern 2: Silent Dropping in Asynchronous Task Starters
- **The Flaw:** When `GoroutineBuilder.StartSimple(fn)` failed to reserve a slot, it returned without executing `fn` or logging any diagnostic error.
- **The Result:** The caller assumed the routine was running in the background. The missing work failed to signal dependent channels, freezing the pipeline without a traceback.
- **Guardrail:** If a budget reservation fails, the framework must either:
  1. Fall back to an emergency single-worker executor with high-priority warnings (as `NewPool` does).
  2. Reject explicitly with an error to the caller.
  3. Execute synchronously if the caller permits fallback.

### Anti-Pattern 3: Global Budget Setting Below Kernel Entity Scale
- **The Flaw:** Arbitrarily setting `defaultCLIGoroutineBudget = 96` without analyzing baseline system object counts. With 99 kinds in the schema and ~35 baseline daemon routines, 96 is mathematically guaranteed to fail.
- **Guardrail:** Global limits must derive from the OS thread envelope (`maxOSThreads()`, typically 512) and maintain a safety margin above known entity cardinality.

### Anti-Pattern 4: Unbounded Fan-Out Loops (`for range { go }`)
- **The Flaw:** Spawning 99 goroutines at once to filter in-memory cached entries.
- **The Result:** Instantaneous contention on budget locks and thread scheduler thrashing.
- **Guardrail:** Use fixed-size worker pools (`pkg/goroutinelabels.Pool`) with work channels, or process in-memory lookups in bounded batches (e.g. 16 workers).

---

## 3. Multi-Tier Concurrency Architecture

To achieve high efficiency, zero-deadlock guarantees, and prevent contention between fast tasks and heavy workloads, the ZQK kernel should adopt a **Three-Tier Concurrency Model**:

```
+-------------------------------------------------------------------------------+
|                       ZQK THREE-TIER CONCURRENCY MODEL                        |
+===============================================================================+
|  TIER 0: CONTROL-PLANE & CLEANUP (Unblockable / Dedicated Pool)               |
|  * Channel Closers, WaitGroup Waiters, Finalizers, Shutdown Hooks             |
|  * Capacity: Dedicated reservation (minimum 1:1 or 2:1 coordinator:cleanup)   |
|  * Guarantee: ZERO drop policy. Never contends with worker pools.             |
+-------------------------------------------------------------------------------+
|  TIER 1: FAST / INTERACTIVE WORK (High Throughput / Low Latency)             |
|  * In-memory cache lookups, status aggregation, metadata resolution           |
|  * Execution latency: < 5ms per unit                                          |
|  * Capacity: Non-blocking, elastic or high-capacity fast queue                |
+-------------------------------------------------------------------------------+
|  TIER 2: HEAVY / I/O WORK (Strictly Bounded Worker Pool)                     |
|  * File reads, AST parsing, policy validation, disk walks, Cgo syscalls       |
|  * Execution latency: 50ms - 5000ms per unit                                  |
|  * Capacity: Bounded pool (e.g., 16-64 workers) pulling from work queue       |
|  * Bounded concurrency protects OS file descriptors & pthread limits          |
+-------------------------------------------------------------------------------+
```

### Tier 0: Control-Plane & Cleanup (The Unblockable Lifeline)
- **Role:** Dedicated strictly to `close(ch)`, `wg.Wait()`, `defer done()`, and buffer flushing.
- **Allocation:** Sized dynamically at `2 * MaxCoordinators` (minimum 32 slots reserved exclusively for control-plane tasks).
- **Rule:** `GoroutineBuilder` must have a dedicated `.AsControlPlane()` or `.AsCleanup()` modifier that bypasses worker budget constraints and guarantees execution.

### Tier 1: Fast / Low-Latency Pool
- **Role:** Small, non-blocking units of work such as `ObjectIDCache` lookups and streaming progress updates.
- **Behavior:** Operates unhindered so progress meters, heartbeats, and coordination events never lag behind disk operations.

### Tier 2: Heavy / Long-Running Pool
- **Role:** Heavy compute or blocking syscall operations (such as `checkObjectWithCacheAndContent` or file parsing).
- **Behavior:** Work items are enqueued onto a work channel (`chan Task`). A fixed set of workers (e.g., 16 workers, matching `AsyncValidator`) pull items. Adding 10,000 tasks adds items to the queue, NOT goroutines to the Go runtime.

---

## 4. Concurrency Budget Sizing Formula

```
                +---------------------------------------+
                |    Max OS Threads Cap: 512 - 1024     |
                +-------------------+-------------------+
                                    |
          +-------------------------+-------------------------+
          |                                                   |
+---------v-------------------------+       +-----------------v-----------------+
|  Control & Cleanup Reserve (32)   |       |   General Worker Budget (480)     |
|  * 1:1 or 2:1 per active pipeline |       |   * Fast Pool: 128 slots          |
|  * Guaranteed, never throttled    |       |   * Heavy Pool: 64-128 workers    |
|  * Ensures channels always close  |       |   * Background DAEMON: 64 slots   |
+-----------------------------------+       +-----------------------------------+
```

### Mathematical Sizing
1. **OS Thread Floor:** `minMaxOSThreads = 64`, `defaultMaxOSThreads = 512`, `maxMaxOSThreads = 1024`.
2. **Process Goroutine Budget:**
   $$\text{CLIGoroutineBudget} = \max(512, \text{maxOSThreads}())$$
   *(Restores the intended 512 default, eliminating the arbitrary 96 limit).*
3. **Control-Plane Reserve:**
   $$\text{CleanupReserve} = \max(32, 2 \times \text{ActivePipelines})$$
4. **Data-Plane Worker Cap:**
   $$\text{WorkerCap} = \text{CLIGoroutineBudget} - \text{CleanupReserve} \quad (\approx 480 \text{ slots})$$

---

## 5. Immediate Code Remediation Plan

### Step 1: Fix `cmd/zqk/app/max_os_threads.go`
- Update `defaultCLIGoroutineBudget` from `96` to `512` (matching `defaultMaxOSThreads` and `defaultSchedulerGoroutineCap`).

### Step 2: Remove Worker Budget from Coordinators in `cmd/zqk/system/async_check.go`
- In `discoverFromCache`:
  - Remove `coordBud` from `coordBuilder` (`discover_from_cache_coordinator`).
  - Remove `aggBud` from `aggBuilder` (`discover_from_cache_collector`).
- In `discoverObjectsParallel`:
  - Remove `counterBud` from `counterBuilder` (`discovery_counter_updater`).
  - Remove `waitBud` from `waitBuilder` (`discover_wait_group`).
  - Remove `collectBud` from `collectBuilder` (`discovery_collect_results`).
- In `cmd/zqk/system/async_check_helpers.go`:
  - Remove `enqueueBud` from `enqueueBuilder` (`discovery_enqueue_stream`).

### Step 3: Implement Bounded Worker Pool for Kind Discovery
- In `discoverFromCache`, instead of spawning 99 goroutines in a loop, process kind cache scans using a bounded pool (e.g. 16 workers) or stream them synchronously/semi-synchronously, eliminating the 99-goroutine blast.

### Step 4: Add Fail-Safe to `GoroutineBuilder.StartSimple`
- In `pkg/goroutinelabels/builder.go`, if `budget.Reserve(1)` fails:
  - If `onBudgetExceeded` is nil, log a high-severity warning and invoke a fallback single-worker execution path instead of silently dropping the goroutine.

### Step 5: Unit Tests
- Add a test reproducing budget saturation with channel closers to guarantee that even when worker budgets are 100% full, coordinators and closers run and channels are closed cleanly without hangs.

---

## 6. Feedback & Decisions Needed

1. **Approval of Sizing:** Confirm alignment with bumping `defaultCLIGoroutineBudget` from `96` to `512`.
2. **Dedicated Control-Plane Option in `pkg/goroutinelabels`:** Confirm adding `.AsControlPlane()` / `.AsCleanup()` to `GoroutineBuilder` to formalize the Unblockable Closer pattern across the entire codebase.
3. **Next Execution Step:** Proceed with implementing the fixes in `cmd/zqk/app/max_os_threads.go`, `cmd/zqk/system/async_check.go`, and `pkg/goroutinelabels/builder.go`.
