# Metrics Recording Policy, Sampling Invariants & Kind Registration Subsystem Operational Architecture Guide (`pkg/metricsrecording`)

## Executive Summary

The `pkg/metricsrecording` package serves as the single source of truth governing telemetry emission, storage mutation counters, pipeline sampling, and metric object generation across the **ZQK Knowledge Kernel**. In high-throughput CI environments and test suites, unrestricted metric object creation can lead to severe I/O bottlenecks and file lock contention. Conversely, production daemon processes require continuous, high-fidelity metrics recording.

This subsystem provides:
1. **Production vs. Test Execution Partitioning:** Production binaries always record metrics; test harnesses default to non-recording unless explicitly opted in.
2. **Atomic Reference-Counted Opt-In (`EnterAllowRecording` / `LeaveAllowRecording`):** Allows fine-grained test cases to enable metric collection safely and clean up via `t.Cleanup`.
3. **Hot-Path Suppression Switch (`EnterHotPathNoPersist` / `LeaveHotPathNoPersist`):** Critical circuit breaker designed to suppress metric object persistence during resource-heavy validation scans (e.g. `zqk system check`).
4. **Selective Kind Filtering (`DenyKind`, `AllowKind`, `ClearDeniedKinds`, `EnabledForKind`):** Thread-safe predicate mechanism to selectively suppress telemetry for specific object kinds during testing.
5. **Atomic Collection & Reset (`CollectAndReset`):** Transactional pattern to gather accumulated telemetry and invoke resetting callbacks in a single operation.

---

## Architecture & Gating Logic

```
                        ┌─────────────────────────────────┐
                        │        Invocation Site          │
                        │ (Storage Counter / Sample / etc)│
                        └────────────────┬────────────────┘
                                         │
                                         ▼
                        ┌─────────────────────────────────┐
                        │      metricsrecording.Enabled() │
                        └────────────────┬────────────────┘
                                         │
                 ┌───────────────────────┴───────────────────────┐
                 │                                               │
                 ▼                                               ▼
     [hotPathNoPersistRef > 0]                       [hotPathNoPersistRef == 0]
                 │                                               │
                 ▼                                               ▼
            RETURN FALSE                               [Is Production Binary?]
                                                      (testing.Testing() == false)
                                                                 │
                                                   ┌─────────────┴─────────────┐
                                                   │                           │
                                                   ▼                           ▼
                                              RETURN TRUE               [In Test Binary]
                                                                               │
                                                               ┌───────────────┴───────────────┐
                                                               │                               │
                                                               ▼                               ▼
                                                   [Env / Config Opt-In]           [allowRecordingRef > 0]
                                                   (ZQK_TEST_METRICS_RECORDING)                │
                                                               │                               │
                                                               └───────────────┬───────────────┘
                                                                               ▼
                                                                     RETURN TRUE / FALSE
```

---

## Architectural Principles & Invariants

### 1. Zero Negative Refcount Invariant
The internal reference-counting helper (`decRef`) atomically guarantees that counters never underflow below zero. Calling `LeaveAllowRecording()` or `LeaveHotPathNoPersist()` when refcounts are already zero is safely ignored.

### 2. Hot-Path Precedence Invariant
`EnterHotPathNoPersist` strictly supersedes all other opt-in mechanisms (including `EnterAllowRecording` and `ZQK_TEST_METRICS_RECORDING=1`). When active, `Enabled()` unconditionally returns `false` to guarantee that critical path operations like `system check` never block on metric persistence.

### 3. Thread-Safe Kind Filtering
Kind denial and allowance operations utilize synchronized map storage (`sync.Map`), ensuring concurrent queries from multi-threaded test runners never cause race conditions.

---

## Core API Reference

- `Enabled() bool`: Returns whether metrics recording is active in the current scope.
- `EnabledForKind(kind string) bool`: Evaluates `Enabled()` and verifies that `kind` is not currently denied.
- `EnterAllowRecording()` / `LeaveAllowRecording()`: Pairs refcounted test scope activation with cleanup.
- `EnterHotPathNoPersist()` / `LeaveHotPathNoPersist()`: Circuit breaker for batch scans.
- `DenyKind(kind string)` / `AllowKind(kind string)` / `ClearDeniedKinds()`: Fine-grained kind filters.
- `CollectAndReset(collect func() (string, error), reset func()) (string, error)`: Conditional metric extraction.

---

## Operational Runbooks

### Issue 1: High Lock Contention During Bulk Validation
**Symptom:** Operations like `zqk system check` or batch CAS sweeps experience timeout or latency spikes due to `coordinator_metrics_router` goroutines.
**Resolution:**
Ensure that batch scan entry points invoke `EnterHotPathNoPersist()` before starting iteration, and `LeaveHotPathNoPersist()` in a defer statement.

### Issue 2: Metrics Not Recorded in Test Harnesses
**Symptom:** Integration tests asserting metric object creation fail because no objects are minted.
**Resolution:**
Opt into metrics recording within the test setup:
```go
metricsrecording.EnterAllowRecording()
t.Cleanup(metricsrecording.LeaveAllowRecording)
```
Or run the test suite with `ZQK_TEST_METRICS_RECORDING=1`.
