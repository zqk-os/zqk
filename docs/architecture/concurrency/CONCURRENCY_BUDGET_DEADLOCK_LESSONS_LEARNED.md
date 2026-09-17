# Concurrency Budget Deadlock: Analysis, Lessons Learned & Multi-Tier Architecture

**Document ID:** ARC-CONCURRENCY-BUDGET-LESSONS-001  
**Incident Reference:** PID 33048 (`system check --details --verbose`) Unrecoverable Pipeline Hang  
**Status:** Canonical Kernel Concurrency Architecture & Standards  
**Last Updated:** 2026-09-13  

---

## 1. Incident Post-Mortem: What Happened in PID 33048

During an execution of `system check --details --verbose`, process PID 33048 hung permanently during the object discovery phase. Core dump inspection (`./to-investigate/33048.dump`, 5.8GB Mach-O arm64) revealed that 58 active goroutines remained resident with 0% CPU utilization.

### Diagnostic Trace from Core Dump

1. **Goroutine 1 (`main` / `TimeoutHook`):** Blocked in `select` waiting for `errChan` in `pkg/cli.(*TimeoutHook).executeCommandWithTimeout`.
2. **Goroutine 24 (`runCheckAsyncWithFollow`):** Blocked waiting on `checkDone` channel in `cmd/zqk/system/check_follower.go`.
3. **Goroutine 1228 (`discoverAndEnqueueObjectsImpl`):** Blocked in `select` waiting on `case <-enqueueDone:` in `cmd/zqk/system/async_check_helpers.go:920`.
4. **Goroutine 2176 (`enqueueFilesAsDiscovered`):** Blocked receiving from `filesStream` (`runtime.chanrecv2`) in `cmd/zqk/system/async_check_helpers.go:1090`.
5. **16 Validation Workers (`AsyncValidator.worker`):** Blocked in `select` waiting for new validation tasks.
6. **All discovery kind workers:** Terminated and dead (`Gdead`).
7. **Discovery Coordinator (`discover_from_cache_coordinator`):** **DID NOT EXIST in the dump.**

### Root Cause Cascade

```
+-----------------------------------------------------------------------------------------+
| Commit b9979027: Installed defaultCLIGoroutineBudget = 96 in cmd/zqk/app/max_os_threads |
+-----------------------------------------------------------------------------------------+
                                           |
                                           v
+-----------------------------------------------------------------------------------------+
| Baseline CLI components (flusher, metrics, validation pool, write-behind) consume ~35   |
| goroutines. Remaining budget slots: 96 - 35 = 61 slots.                                 |
+-----------------------------------------------------------------------------------------+
                                           |
                                           v
+-----------------------------------------------------------------------------------------+
| discoverFromCache iterates over all 99 object kinds in parallel.                        |
| Each kind requests 1 slot from DefaultBudget(). First ~61 succeed; kinds 62-99 FAIL.    |
+-----------------------------------------------------------------------------------------+
                                           |
                                           v
+-----------------------------------------------------------------------------------------+
| discover_from_cache_coordinator is invoked with WithBudget(DefaultBudget()).            |
| Budget is 100% saturated. Budget.Reserve(1) FAILS.                                      |
+-----------------------------------------------------------------------------------------+
                                           |
                                           v
+-----------------------------------------------------------------------------------------+
| StartSimple SILENTLY RETURNS without spawning the coordinator.                          |
| workerWg.Wait(), close(streamChan), and close(collectChan) NEVER EXECUTE.               |
+-----------------------------------------------------------------------------------------+
                                           |
                                           v
+-----------------------------------------------------------------------------------------+
| Downstream consumer enqueueFilesAsDiscovered (G 2176) reads available batches, then      |
| BLOCKS INDEFINITELY on filesStream (which is never closed).                             |
| Pipeline deadlocks permanently with zero errors, zero warnings, and zero CPU.           |
+-----------------------------------------------------------------------------------------+
```

---

## 2. Core Architectural Lessons Learned

### Lesson A: The "Unblockable Closer" Guarantee
**Coordinators, channel closers, and cleanup finalizers must never compete with data-plane workers for budget.**
If a goroutine is responsible for calling `workerWg.Wait()`, closing an output channel (`close(streamChan)`), or releasing a critical resource, it belongs to the **Control Plane**. Subjecting it to worker budget throttling guarantees that worker saturation will kill the cleanup mechanism, turning high load into a permanent deadlock.

### Lesson B: Never Drop Asynchronous Tasks Silently
`GoroutineBuilder.StartSimple` must never silently drop a task when `Reserve(1)` fails. If the caller did not configure an explicit `WithBudgetExceededHandler`, the system must treat the task as critical, invoke an emergency unreserved fallback path, and emit a high-priority diagnostic alert. Silent task drops make debugging nearly impossible because the dropped routine leaves no trace.

### Lesson C: Global Limits Must Exceed System Entity Cardinality
Setting a global limit of 96 when the kernel object registry contains 99 kinds and baseline daemons use 35 slots creates a mathematical certainty of failure. Process-wide budgets must align with the OS thread ceiling (`defaultMaxOSThreads = 512`, scaling with `maxOSThreads()`).

### Lesson D: Bounded Worker Pools Over Unbounded Fan-Out
Never use `for _, item := range collection { go worker(item) }` for bulk entities. Even when individual tasks are small (such as reading an in-memory `ObjectIDCache`), spawning 99 simultaneous goroutines creates lock contention on budget counters and thrashing in the Go scheduler. Always use bounded worker pools (e.g. 16 workers) pulling from a work channel.

---

## 3. ZQK Three-Tier Concurrency Architecture

To guarantee both high throughput and complete deadlock immunity, the ZQK kernel adheres to a **Three-Tier Concurrency Model**:

```
+=========================================================================================+
|                        ZQK THREE-TIER CONCURRENCY MODEL                                 |
+=========================================================================================+
|  TIER 0: CONTROL-PLANE & CLEANUP (Unblockable Lifeline)                                 |
|  * Target: Channel closers, waitgroup drainers, completion callbacks, buffer flushers   |
|  * Guarantee: ZERO DROP. Bypasses general worker budget constraints.                   |
|  * Modifier: .AsControlPlane() / .AsCleanup()                                           |
+-----------------------------------------------------------------------------------------+
|  TIER 1: FAST / INTERACTIVE WORK (High Throughput, Non-Blocking)                        |
|  * Target: In-memory cache lookups, status aggregation, progress heartbeats (< 5ms)     |
|  * Allocation: Unhindered execution; dedicated capacity.                                |
|  * Isolation: Protected from heavy disk I/O and AST parsing stalls.                     |
+-----------------------------------------------------------------------------------------+
|  TIER 2: HEAVY / LONG-RUNNING WORK (Strictly Bounded Worker Pool)                       |
|  * Target: File I/O, AST parsing, rule evaluation, disk scans (50ms - 5000ms)            |
|  * Pattern: Fixed-size worker pool (e.g., 16-64 workers) with buffered work channel.    |
|  * Boundary: Saturated queues buffer work items, NEVER spawning unbounded OS threads.   |
+=========================================================================================+
```

### Tier 0: Control-Plane & Cleanup (Unblockable Lifeline)
- Any goroutine created to close a channel, wait on a `sync.WaitGroup`, or finalize an async stage must be marked with `.AsControlPlane()` or `.AsCleanup()`.
- Control-plane tasks are exempt from data-plane worker budget limits.
- Sized dynamically at `2:1` or `1:1` relative to active pipeline coordinators.

### Tier 1: Fast / Low-Latency Pool
- Tasks that operate strictly on in-memory state (such as `ObjectIDCache.GetEntriesByKind`) must not be queued behind blocking disk or network operations.
- Progress heartbeats, terminal redraws, and MCP notification dispatches run in Tier 1.

### Tier 2: Heavy / Long-Running Work
- Tasks performing disk reads, YAML decoding, or AST parsing must use bounded pools (`goroutinelabels.Pool` or `AsyncValidator` pool).
- Submitting 10,000 objects to validate enqueues 10,000 tasks onto a work channel processed by 16 workers, keeping OS threads strictly bounded.

---

## 4. Concurrency Budget Sizing Standards

1. **OS Thread Ceiling (`maxOSThreads()`):** Default 512 (`minMaxOSThreads = 64`, `maxMaxOSThreads = 1024`).
2. **CLI Goroutine Budget (`defaultCLIGoroutineBudget`):** **512** (matches default OS threads and scheduler daemon cap).
3. **Dynamic Scaling:** `CLIGoroutineBudget = max(512, maxOSThreads())`.
4. **Safety Margin:** Budget must exceed baseline background goroutines (~35) + maximum concurrent pipeline workers (64) + entity cardinality (99 kinds) with at least 2.5x headroom.
