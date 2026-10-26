# Test Package Concurrency Limits & Parallel Execution Throttling Subsystem Guide

## 1. Executive Architectural Summary

The **Test Package Concurrency Limits Subsystem** ([`pkg/testpackageconcurrency`](file:///Users/lanceettl/zqk-public-candidate/pkg/testpackageconcurrency/limits.go)) governs cross-process parallel test execution across the ZQK Knowledge Kernel execution plane under **POL-DEFAULT-b17f54c251eb3c4d**. During autonomous continuous loops, test verification swarms, and background batch runs, unconstrained parallel invocation of compute-heavy or disk-bound test packages (e.g. database transactions, filesystem CAS operations, or lock contention fixtures) risks hardware thrashing, thread starvation, and non-deterministic flakes.

This subsystem provides persistent, content-addressed, and runtime-mergeable concurrency constraints without requiring test harnesses to take circular dependencies on heavy execution frameworks. It serves as the single source of truth for:
- Standardized package path normalization across operating systems and workspace relative coordinates.
- On-disk policy loading and parsing from `.zqk/test-bundles/package_concurrency_limits.json`.
- Dynamic policy merging that combines persistent repo-level configuration with runtime job metadata and conservative scheduler defaults.
- Ceiling enforcement clamping concurrent test workers to safe host thresholds (maximum 32 concurrent workers per package).

---

## 2. Component Topology & Architecture

The subsystem coordinates four primary components across the scheduler and runtime execution boundary:

1. **HardwareResourceProber:** Probes available host CPU cores, memory limits, and container cgroups to evaluate safe baseline concurrency capacity.
2. **LimitCalculator:** Evaluates normalized package limits from on-disk declarations, job metadata annotations, and fallback rules.
3. **ThrottlingGovernor:** Reconciles multi-source limit maps via conservative minimums, ensuring local job manifests cannot unsafely exceed project-wide constraints.
4. **ConcurrencySemaphore:** Allocates per-package slot semaphores that throttle simultaneous test package runners, queuing excess test jobs with context-aware timeouts.

```mermaid
flowchart TD
    subgraph StorageLayer["Configuration & Storage Plane"]
        DiskPolicy["On-Disk Limits\n(.zqk/test-bundles/package_concurrency_limits.json)"]
        JobManifest["Job Metadata\n(Scheduler Raw Jobs)"]
    end

    subgraph CoreSubsystem["pkg/testpackageconcurrency Subsystem"]
        Prober["HardwareResourceProber\n(CPU / RAM / Container Cgroups)"]
        Reader["ReadLimitsMap\n(JSON Deserialization & Clamping)"]
        Merger["MergeLimitMaps\n(Conservative Per-Key Minimum)"]
        Normalizer["NormalizePackagePath\n(Trimming & Prefix Canonicalization)"]
    end

    subgraph ExecutionPlane["Scheduler & Concurrency Governance"]
        Limiter["PackageConcurrencyLimiter\n(CircuitBreaker Semaphore Map)"]
        WorkerPool["Parallel Test Runners\n(go test Subprocesses)"]
    end

    DiskPolicy -->|JSON Read| Reader
    Normalizer -->|Sanitized Keys| Reader
    JobManifest -->|Dynamic Limits| Merger
    Reader -->|Base Limits| Merger
    Prober -.->|Capacity Hints| Merger
    Merger -->|Merged Policy Map| Limiter
    Limiter -->|Acquire / Release| WorkerPool
```

---

## 3. Interface & Contract Specifications

The subsystem provides three core functions defined in [`limits.go`](file:///Users/lanceettl/zqk-public-candidate/pkg/testpackageconcurrency/limits.go):

```go
// NormalizePackagePath trims "./" and surrounding space for map keys and lookups.
func NormalizePackagePath(packagePath string) string

// ReadLimitsMap loads max concurrent run_wrapper jobs per package from disk.
// Missing file returns (nil, nil). Invalid JSON returns an error.
func ReadLimitsMap(projectRoot string) (map[string]int, error)

// MergeLimitMaps returns a new map with per-key minimum of a and b (when both set).
// Keys present in only one map are included. Used to combine on-disk policy with job metadata.
func MergeLimitMaps(a, b map[string]int) map[string]int
```

### On-Disk Schema Specification

Policy files are persisted at `paths.ProjectDataDir / paths.TestBundlesDir / LimitsFileName` (`.zqk/test-bundles/package_concurrency_limits.json`). The schema contract is defined by `limitsFile`:

```json
{
  "schema_version": 1,
  "updated_at": "2026-10-25T12:00:00Z",
  "max_concurrent_jobs_by_package": {
    "pkg/storage": 4,
    "pkg/mcp": 8,
    "pkg/scheduler": 2,
    "pkg/lifecycle": 6
  }
}
```

### Invariant Rules
- **Path Normalization:** All keys are stripped of leading `./` and surrounding whitespace through [`NormalizePackagePath`](file:///Users/lanceettl/zqk-public-candidate/pkg/testpackageconcurrency/limits.go#L27-L29). Empty keys are discarded.
- **Value Clamping:** Any configured concurrency limit greater than `32` is automatically clamped to `32` to protect host CPU and process tables from unbounded thread storms.
- **Non-Positive Discarding:** Any values `<= 0` are excluded from the active limit map.
- **Fail-Open on Missing File:** If the limits file does not exist, [`ReadLimitsMap`](file:///Users/lanceettl/zqk-public-candidate/pkg/testpackageconcurrency/limits.go#L33-L64) returns `(nil, nil)`, allowing the caller to apply default scheduler fallbacks without failing.

---

## 4. Concurrency Limit Resolution & Throttling Lifecycle

The parallel execution lifecycle coordinates disk configuration, runtime metadata, semaphore acquisition, test dispatch, and graceful release:

```mermaid
sequenceDiagram
    autonumber
    participant S as Scheduler Daemon
    participant TPC as pkg/testpackageconcurrency
    participant PCL as PackageConcurrencyLimiter
    participant TR as Test Runner Subprocess

    S->>TPC: ReadLimitsMap(projectRoot)
    TPC-->>S: return on-disk limits (e.g. storage: 4)
    S->>S: packageConcurrencyLimitsFromSchedulerJobRaw(jobs)
    S->>TPC: MergeLimitMaps(fileMap, jobMap)
    TPC-->>S: return merged conservative limits
    S->>PCL: MergeLimits(merged)

    Note over S,TR: Job Dispatch Cycle
    S->>PCL: ShouldLimit("pkg/storage")
    PCL-->>S: true (limit = 4)
    S->>PCL: Acquire(ctx, "pkg/storage")
    alt Slots Available (< 4)
        PCL-->>S: acquired immediately
    else Capacity Exhausted (= 4)
        PCL-->>S: wait on condition / timeout
    end
    S->>TR: Launch go test ./pkg/storage/...
    TR-->>S: Process exit (PASS / FAIL)
    S->>PCL: Release("pkg/storage")
```

---

## 5. Resource Adaptation & Boundary Matrix

The subsystem dynamically accommodates diverse host topologies, balancing CPU saturation against memory ceilings:

| Environment Profile | Available Cores | Memory Allocation | Default Package Limit | Hard Ceiling | Notes |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **CI Container (Constrained)** | 2 – 4 vCPUs | 4GB – 8GB | 2 | 4 | Conservative allocation to prevent OOM kills in cgroup-constrained pods. |
| **Developer Laptop (Interactive)**| 8 – 12 Cores | 16GB – 32GB | 4 | 8 | Prevents UI freezing while retaining responsive CLI turnaround times. |
| **Bare-Metal Build Node** | 32 – 64+ Cores | 64GB – 256GB | 8 | 32 | High throughput for large monorepo test passes while honoring package locks. |
| **Specialized Fixture Packages** | Any | Any | 1 | 1 | Strictly serialized (e.g. disk-exclusive CAS tests or port-binding services). |

### Conflict Resolution Strategy

When multiple limit definitions exist for the same package:
1. **Both on-disk and job metadata defined:** [`MergeLimitMaps`](file:///Users/lanceettl/zqk-public-candidate/pkg/testpackageconcurrency/limits.go#L68-L84) applies `min(fileVal, jobVal)`. The more restrictive limit always wins.
2. **Present in only one source:** The declared positive limit is preserved.
3. **Neither declared:** The scheduler applies its conservative fallback default (`getSchedulerDefaultPackageConcurrency()`).

---

## 6. Concurrency & Thread-Safety Guarantees

1. **Lock-Free Map Merging:** [`MergeLimitMaps`](file:///Users/lanceettl/zqk-public-candidate/pkg/testpackageconcurrency/limits.go#L68-L84) constructs and returns a newly allocated map, ensuring zero contention and safe immutability across concurrent readers.
2. **Atomic Semaphore Tracking:** Downstream limiters implement mutex-protected slot acquisition with context timeouts to prevent deadlocks when test bundles run long.
3. **Goroutine Labeling:** All asynchronous test and scheduler workers spawning concurrency checks are managed through `goroutinelabels.NewGoroutine`, ensuring transparent attribution in stack traces and panic telemetry.

---

## 7. Verification & Statement Coverage Invariant

Statement test coverage for [`pkg/testpackageconcurrency`](file:///Users/lanceettl/zqk-public-candidate/pkg/testpackageconcurrency/limits.go) is enforced at **>= 85.0%** with a verified achievement of **100.0%**:

```text
github.com/zqk-os/zqk/pkg/testpackageconcurrency/limits.go:27: NormalizePackagePath 100.0%
github.com/zqk-os/zqk/pkg/testpackageconcurrency/limits.go:33: ReadLimitsMap        100.0%
github.com/zqk-os/zqk/pkg/testpackageconcurrency/limits.go:68: MergeLimitMaps       100.0%
total:                                                         (statements)         100.0%
```

To execute the test suite:
```bash
ZQK_ALLOW_FOREGROUND_GO_TEST=1 go test -v -timeout 30s -cover ./pkg/testpackageconcurrency
```
