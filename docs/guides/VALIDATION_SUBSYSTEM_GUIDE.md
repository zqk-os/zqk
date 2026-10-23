# Validation Subsystem Architectural & Operational Guide

Comprehensive architectural manual, invariants specification, and operational guide for the **ZQK Validation Subsystem** (`pkg/validation`).

---

## 1. Overview & Architectural Compass

The Validation Subsystem (`pkg/validation`) serves as the foundational, fail-closed verification engine of the ZQK Knowledge Kernel. Unlike transient linting or superficial schema validators, `pkg/validation` enforces formal ontological, relational, and storage invariants across both authoritative Content-Addressed Storage (CAS) and the draft plane.

```
+-------------------------------------------------------------------------+
|                        ZQK Execution Engine                             |
+-------------------------------------------------------------------------+
                                    │
                                    ▼
+-------------------------------------------------------------------------+
|                  pkg/validation Multi-Layer Pipeline                    |
|                                                                         |
|  [ Layer 0 ]  CAS & Storage Integrity (SHA-256, Content Hash)          |
|  [ Layer 1 ]  Schema & Property Shapes (Types, Regex, Enums)           |
|  [ Layer 2 ]  Lifecycle State Machine (Role Transitions, Grandfather)   |
|  [ Layer 3 ]  Relational Graph Invariants (Air-Tight Locks, DAG Cycles) |
|  [ Layer 4 ]  Resource Hygiene & I/O (Worker Pools, Queue Budgets)      |
+-------------------------------------------------------------------------+
                                    │
                                    ▼
+-------------------------------------------------------------------------+
|               Knowledge Kernel State Cache & Remediation                |
|      (pkg/validation/state_cache.go & Automated Remediation Engine)     |
+-------------------------------------------------------------------------+
```

### Core Responsibilities
- **Multi-Layer Gatekeeping**: Prevent corrupted, malformed, or out-of-order state transitions from reaching disk.
- **Cross-Plane Boundary Enforcement**: Enforce strict isolation between draft-plane and CAS-resident objects to eliminate dual-plane split-brain anomalies.
- **Zero Swallowed Failures**: Ensure every structural error or invariant violation is captured in actionable diagnostic envelopes.
- **High-Throughput Asynchronous Processing**: Provide token-budgeted, concurrent validation via priority queues, bounded worker pools, and memory-constrained state caching.
- **Automated Remediation**: Identify auto-fixable drift (such as wall-clock overstatement) and apply idempotent correction recipes without manual operator intervention.

---

## 2. The 5-Layer Validation Pipeline

The subsystem organizes verification into five hierarchical invariant tiers. Higher layers presuppose the physical and structural integrity guaranteed by lower layers.

### Layer 0: CAS & Storage Integrity
- **SHA-256 Content Addressing**: Authoritative objects stored in `.zqk/objects/` are addressed by the cryptographic digest of their normalized payload.
- **Checksum Verification**: Validates file checksums to detect disk corruption, bit rot, or unauthorized out-of-band file modifications.
- **Empty-Overwrite & Poison File Guards**: Prevents in-memory cache flushes from overwriting populated disk files with empty or truncated state.

### Layer 1: Schema & Format Specification
- **SHACL-Inspired Property Shapes**: Enforces property presence, types (string, map, slice, integer, float, boolean), and custom semantic format constraints.
- **Canonical ID & Namespace Formatting**:
  - Validates typed prefixes (e.g., `GOL-` for goals, `REQ-` for requirements, `BLI-` for backlog items, `PER-` for personas).
  - Validates hierarchical namespace layers (`zqk`, `domain`, `integration`) with regex pattern:
    `^(zqk|domain|integration):[a-z0-9_]+(:[a-z0-9_]+)*$`
- **Temporal & Version Normalization**:
  - ISO-8601 / RFC-3339 strict timestamp validation (`created_at`, `updated_at`, `started_at`, `completed_at`).
  - Semantic version pattern normalization (`^\d+\.\d+\.\d+$`).

### Layer 2: Lifecycle State Machine Transitions
- **State Role Mapping**: Verifies transitions against the formal lifecycle state graph (`draft` ➔ `planned` ➔ `in_progress` ➔ `complete`).
- **Prerequisite & Precondition Gates**:
  - `shovel_ready`: Verifies all required decomposition artifacts and child bindings are present prior to entering active execution.
  - `active_ref`: Enforces that parent references point to currently active, non-archived entities.
- **Grandfathering Historical Records**:
  - Distinguishes between transitions into a terminal status (which strictly enforce machine-checkable closure evidence) versus grandfathered historical records already holding terminal status.

### Layer 3: Relational Graph & Semantic Boundary Invariants
- **Airtight Scope Locks**: Priority Plans entering `in_progress` lock their child backlog items to prevent stealth scope creep.
- **Acyclic Graph Verification (Cycle Detection)**: Depth-bounded graph traversal detects direct and transitive reference loops across dependency chains.
- **Link-Back Reciprocity**: Child-owned foreign keys must resolve to existing parent records with reciprocal schema traits.
- **Cross-Plane Split-Brain Prevention**: CAS-resident objects are strictly barred from referencing draft-plane entities across the membrane.

### Layer 4: I/O, File System, & Goroutine Resource Hygiene
- **Bounded Worker Pool (`ValidationPool`)**: Prevents goroutine explosions during large repository scans by constraining concurrency to host CPU limits.
- **Memory-Bounded Cache (`ValidationStateCache`)**: Caps cached validation states at `MaxValidationCacheEntries` (300,000 entries), evicting oldest entries to bound memory.
- **Graceful Shutdown & Drain**: Priority queues flush, wait for active workers, and persist validation cache to disk within strict cancellation budgets.

---

## 3. Asynchronous Validation Engine & Worker Pipeline

For large-scale continuous execution and daemon processes, `pkg/validation/async_validator*.go` provides an event-driven validation worker swarm.

```
                    Enqueue(task)
                          │
                          ▼
            +───────────────────────────+
            |      Priority Queue       |
            |   (Higher Priority First) |
            +───────────────────────────+
                          │
             Worker Pool Concurrency Limit
                          │
        ┌─────────────────┼─────────────────┐
        ▼                 ▼                 ▼
   [ Worker 1 ]      [ Worker 2 ]      [ Worker N ]
        │                 │                 │
        └─────────────────┼─────────────────┘
                          │
                          ▼
            +───────────────────────────+
            |   Validation Output Queue |
            |      (Batch Dequeue)      |
            +───────────────────────────+
                          │
                          ▼
               Output Writer / Handlers
```

### Worker Lifecycle
1. **On-Demand Worker Spawning**: Workers spawn dynamically up to the configured worker count when tasks are enqueued.
2. **Idle Worker Shutdown**: Workers shut down cleanly after their idle timeout expires, releasing goroutines and operating system threads.
3. **Stream Location Reads**: Supports both standard file paths and stream location paths (`<segmentPath>::<offset>`) for zero-copy stream processing.

---

## 4. Automated Remediation Engine & Autofix Recipes

The subsystem identifies auto-fixable issues (`issue.AutoFixable == true`) and applies deterministic remediation recipes:

### Wall-Clock Effort Clamping (`effort_wallclock.go`)
- **The Problem**: Agents or manual operators sometimes log estimated effort or exaggerated values into `actual_effort`, exceeding the real elapsed wall-clock time between `started_at` and `completed_at`.
- **The Remedy**: `ClampActualEffortToWallClock(obj)` automatically calculates the wall-clock span and clamps overstated effort down to the real elapsed span:
  ```go
  clamped, previous, next := ClampActualEffortToWallClock(obj)
  ```
- **Inverted Time Safeguard**: If `completed_at` is earlier than `started_at`, the span is inverted and clamped to `"0h"`.

### Cache Synchronization & Shrink Guards (`state_cache.go`)
- **Shrink Guard**: Prevents partial in-memory checks from replacing warm on-disk caches if entry count drops by more than 50% below warm size.
- **Stale Generation Guard**: Prevents long-running background tasks from overwriting newer cache updates written by CLI invocations.

---

## 5. Verification & Statement Coverage Invariant

To guarantee kernel reliability across all release targets, `pkg/validation` enforces a strict statement coverage floor:

> [!IMPORTANT]
> **Statement Coverage Invariant**: Statement coverage across `pkg/validation` must reach and maintain **>= 85.0%** across all test runs (`go test -short -cover ./pkg/validation`).

### Testing Protocol & Quality Gates
- **Isolated Test Environments**:
  Every test MUST execute in a completely isolated filesystem boundary using `t.TempDir()` and configure isolated test roots:
  ```go
  tempDir := t.TempDir()
  t.Setenv(zqkenv.ProjectRoot().Name(), tempDir)
  t.Setenv(zqkenv.TestRoot().Name(), tempDir)
  ```
- **AST Quality Invariant (Zero Swallowed Errors)**:
  Never discard error returns with blank identifiers (`_ = err`). Every error return must be explicitly checked with `require.NoError(t, err)`, `assert.Error(t, err)`, or idiomatic Go error handling.
- **Bounded Function Size**:
  To prevent unwieldy monolithic functions, helper and test functions must not exceed 100 lines. Break large test cases into modular assertion helpers.
- **No Process Bombs**:
  Test executions must target only `./pkg/validation` with an explicit `-timeout 30s` flag. Never run unbounded, multi-package suites in hot validation loops.

---

## 6. Operational CLI & API Quick Reference

### CLI Verification Commands
```bash
# Validate all objects across the repository
zqk system check

# Validate a specific object with verbose issue breakdown
zqk object inspect <object-id>

# Run validation subsystem tests with statement coverage
ZQK_ALLOW_FOREGROUND_GO_TEST=1 go test -short -cover ./pkg/validation

# Run targeted Cycle test suite
ZQK_ALLOW_FOREGROUND_GO_TEST=1 go test -v -timeout 30s ./pkg/validation -run 'TestCycle127'
```

### Programmatic Go APIs
```go
import "github.com/zqk-os/zqk/pkg/validation"

// 1. Synchronous validation of an object instance
validator := validation.NewGoValidator()
result, err := validator.Validate(ctx, objMap, kind, validation.DefaultValidationOptions())
if err != nil || !result.IsValid {
    // Inspect result.Errors and result.Warnings
}

// 2. Asynchronous validation worker engine
av := validation.NewAsyncValidator(ctx, projectRoot, 4, time.Minute)
defer av.Stop()
av.Enqueue("obj-123", "goals/goal.yaml", 1)

// 3. Automated remediation of wall-clock effort
clamped, prev, next := validation.ClampActualEffortToWallClock(objMap)
```

---

## 7. Open-Core Compliance Invariant

This guide adheres strictly to the **Open-Core Public Release Gateway Standards**. In compliance with `scripts/open-core/check-public-release-payload.sh`:
- All identifiers in this guide are generic placeholders or standard schema keys.
- No internal ephemeral nanosecond IDs or proprietary swarm addresses are present.
