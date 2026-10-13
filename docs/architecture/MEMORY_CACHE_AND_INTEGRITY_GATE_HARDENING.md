# Memory Cache and Integrity Gate Hardening

## Executive Summary
This document specifies the architectural implementation of the in-memory write-through cache with mtime invalidation in the `JobStateRegistry` and the elevation of graph reference integrity to a Layer 0 fail-closed validation blocker. These enhancements eliminate disk I/O contention under high-throughput agent polling while establishing strict mathematical boundaries preventing unanchored backlog items from entering the persistent Knowledge Kernel.

## 1. Context & Motivation
Under autonomous execution swarms, multiple concurrent agent processes query scheduler job state and graph relationships hundreds of times per second. Without coordinated caching, disk I/O contention on state YAML files creates latency spikes and file descriptor exhaustion. Concurrently, graph integrity requires that atomic effort units (`backlog_item`) remain anchored to strategic compasses (`goal`) and feature specifications (`requirement`). Unanchored items violate the Hierarchical Chain of Integrity and must fail closed at the earliest validation boundary.

## 2. In-Memory Write-Through Cache Architecture
The `JobStateRegistry` provides a unified execution state management surface combining thread-safe in-memory caching with synchronous durability:

```
+--------------------------------------------------------------------+
|                         JobStateRegistry                           |
|                                                                    |
|  +---------------------------+     +----------------------------+  |
|  |  jobExecutionStateCache   |     |       knownIdleJobs        |  |
|  |  map[string]*ExecState    |     |    map[string]struct{}     |  |
|  +---------------------------+     +----------------------------+  |
|                                                                    |
|  +--------------------------------------------------------------+  |
|  |             fileStateCache map[string]cachedFileState        |  |
|  |             (mtime + size invalidation detector)             |  |
|  +--------------------------------------------------------------+  |
+--------------------------------------------------------------------+
                                |
                   Synchronous Write-Through
                                |
                                v
+--------------------------------------------------------------------+
|             Atomic Filesystem Persistence (.zqk/scheduler/)        |
+--------------------------------------------------------------------+
```

### 2.1 Cache Invariants
1. **Synchronous Write-Through:** Every state transition (`RegisterExecution`, `UpdateState`, `DeferExecution`, `CompleteExecution`) updates the persistent filesystem atomically prior to updating the in-memory registry cache.
2. **Known-Idle Optimization:** Jobs with no active or deferred executions are indexed in `knownIdleJobs`, bypassing redundant directory scans on frequent poll passes.
3. **MTime & Size Invalidation:** The secondary `fileStateCache` tracks modification timestamps and file sizes. Out-of-process modifications to state files automatically invalidate cached entries and trigger re-reading.
4. **Explicit Eviction & Invalidation:** The `InvalidateJobCache(jobID string)` API enables immediate coordinated eviction of in-memory and file-level cached states across worker lifecycles.
5. **Fault Recovery:** Corrupted or unreadable state files on disk do not corrupt in-memory states; the cache recovers gracefully upon subsequent writes or status transitions.

## 3. Layer 0 Graph Reference Gate Hardening
The Hierarchical Chain of Integrity mandates that every non-draft `backlog_item` must declare at least one parent reference (`goal_refs` or `requirement_refs`).

### 3.1 Verification Topology & Elevation
| Verification Plane | Mechanism | Severity / Layer | Action on Violation |
| :--- | :--- | :--- | :--- |
| **Pre-Commit / Vet** | `CheckReferenceIntegrity` (`pkg/vet`) | `SeverityError` | Fails commit; blocks push |
| **System Check** | `check_instance_validation_helpers` | Layer 0 (Exit Code 2) | Halts CI; blocks promotion |
| **Compose CAS** | `bli_hierarchical_chain` overlay rule | Tier 1 Blocker | Rejects CAS object write |

### 3.2 Exit Code Semantics
- **Exit Code 0:** Clean and compliant.
- **Exit Code 1:** Tier 1 functional issues.
- **Exit Code 2:** Layer 0 CAS and Structural Integrity Blockers (missing parent references, hash mismatches, parse errors).
- **Exit Code 3:** Non-blocking warnings.

## 4. Verification and Conformance Matrix
- **JobStateRegistry Unit Suite:** `TestJobStateRegistry_WriteThroughCacheAndMTimeInvalidation`, `TestJobStateRegistry_InvalidateJobCache`, `TestJobStateRegistry_StorageFaultRecoveryAndCacheEvictionRace`.
- **Reference Integrity Vet Suite:** `TestCheckReferenceIntegrity` verifying `SeverityError` fail-closed enforcement.
- **Layer 0 System Check Suite:** `TestUnanchoredBacklogItemLayer0IntegrityBlocker` verifying Layer 0 categorization and exit code 2 mapping.
