# Materialized View Reconciler Determinism & CI Flake Eradication

## Abstract
This architectural specification details the determinism, concurrency safety, and test lifecycle guarantees for the `whatsnext` Materialized View projection reconciler in the ZQK Knowledge Kernel.

## Background & Problem Statement
The `whatsnext` subsystem maintains a low-latency materialized projection (`.zqk/state/whats_next_lite.json`) allowing sub-millisecond evaluation of priority plans, active orders, and task routing. When projections grow older than `DefaultStalenessTolerance`, `GetOrRecoverPayload` executes one of two paths:
1. **Quiescent Graph Watermark Touch**: If the lifecycle WAL (`lifecycle_events.wal`) has received no new mutations since `payload.MaterializedAt`, the graph is verified to be quiescent. The watermark is updated in-place without triggering an expensive full CAS storage scan.
2. **Active Graph Reconciler Circuit Breaker**: If mutations have occurred since `MaterializedAt`, the payload is returned with `Stale=true` and `Recovering=true`, and an asynchronous background rebuild (`TriggerAsyncRebuild`) is dispatched.

Under CI environments and concurrent test execution, two systemic test races were observed:
1. **Unseeded WAL Quiescence Race**: When inducing staleness in test environments, failure to write an active WAL mutation newer than `MaterializedAt` allowed the quiescent watermark touch to trigger instead of the active reconciler circuit breaker.
2. **TempDir Cleanup / Goroutine Teardown Race**: In error-recovery tests (`TestGetOrRecoverPayload_CorruptedAndPermissionError`), `TriggerAsyncRebuild` dispatched asynchronous background goroutines that wrote to `.zqk/state/` while Go's test framework was concurrently executing `os.RemoveAll(tempDir)`. This resulted in `unlinkat: directory not empty` test failures.
3. **Runner Scheduling Latency Spikes**: Under concurrent test execution where multiple tests perform disk I/O, individual read iterations occasionally experience scheduling or GC pauses (>15ms), failing overly strict per-iteration cutoffs even when the average read latency remained well below 5ms.

## Architectural Invariants & Remedies

### 1. Deterministic WAL Mutation Seeding
Tests verifying the asynchronous recovery circuit breaker must explicitly seed a lifecycle WAL mutation newer than `MaterializedAt`:
```go
walPath := filepath.Join(tempDir, paths.ProjectDataDir, paths.WalDir, "lifecycle_events.wal")
_ = fileutil.EnsureDir(filepath.Dir(walPath))
_ = fileutil.WriteFile(walPath, []byte("event: test_mutation\n"), paths.FilePerm644)
```
Conversely, tests asserting quiescent behavior must verify that WAL mod times older than `MaterializedAt` refresh watermarks cleanly without entering recovery.

### 2. Supervised Reconciler Teardown
Any test invoking `GetOrRecoverPayload` or `TriggerAsyncRebuild` in a temporary test directory must guarantee that background goroutines have finished before `t.TempDir()` cleanup runs:
- Call `waitForReconcile(t, timeout)` after triggering background rebuilds.
- Register `t.Cleanup` handlers that restore file permissions and await background reconciler completion.
- Reset the atomic gate `isReconciling` upon test completion.

### 3. Load-Resilient Latency Assertions
When verifying hot path performance over multi-iteration loops (e.g. 50 reads):
- Assert that the **average read latency** satisfies the performance invariant (`avg < 10ms` under concurrency, targeting sub-millisecond median).
- Allow individual iteration jitter up to 75ms on heavily loaded CI runners, bounding allowable outliers to ensure single-thread scheduling anomalies do not fail test suites.

## Verification & Traceability
- **Requirement**: `REQ-1794024331551639000-74bea9e1` (Materialized View Reconciler Determinism & CI Flake Eradication)
- **Test Case**: `TST-1794024333601816001-34b7c634` (Test Suite: Materialized View Reconciler Determinism & CI Flake Eradication)
- **Criteria**:
  - `CRIT-1794024333601816000-7a9a8257` (Functional Acceptance)
  - `CRIT-1794024333601817000-0dd242a6` (Boundary & Error Handling)
  - `CRIT-1794024333601818000-53c6e712` (Documentation & Knowledge Base Entry)
