# ADR: CAS Graph Compaction and Generational Storage Reclamation Engine

## Status
Accepted

## Date
2026-11-20

## Context
As Content-Addressable Storage (CAS) object graphs grow across long-lived swarm executions, generational segments accumulate superseded revisions, tombstoned references, and fragmented hash buckets. Unbounded generational growth degrades lookup latency and inflates disk footprint, while naive in-place compaction risks corrupting or prematurely reclaiming objects referenced by concurrent in-flight writers.

In Cycle 157, we introduce the CAS Graph Compaction and Generational Storage Reclamation Engine (`pkg/community/cas_compactor.go`) to reclaim generational disk footprint, re-index SHA-256 CAS hash buckets, and guarantee zero-loss consistency and concurrency safety across compacted storage.

## Decision
1. **Monotonic Generation Sequence & Page Allocation Bounds Invariants**:
   - Compaction descriptors and storage segments enforce strict static structural invariants before any reclamation pass begins:
     - Generation sequence numbers must be strictly monotonic (`generation_n > generation_{n-1}`), preventing out-of-order or replay compaction of historical segments.
     - Page allocations must be non-empty (`page_count > 0`, `page_size > 0`) and bounded within configured segment capacity limits.
     - Invariant storage parameters (alignment, bucket fanout, and retention thresholds) are validated fail-closed prior to opening or compacting a generation.

2. **Zero-Loss Generational Storage Reclamation & SHA-256 Hash Bucket Re-Indexing**:
   - Compaction sweeps reachable live objects from active generation roots into a compacted target generation, reclaiming dead/unreferenced generational storage footprint.
   - Every live object is re-indexed into updated SHA-256 CAS hash buckets, preserving deterministic content-addressable lookup and full cryptographic object retrievability after compaction completes.
   - Post-compaction verification confirms zero-loss consistency: every live SHA-256 digest present before compaction resolves identically from the re-indexed target generation before superseded segments are unlinked.

3. **Uncommitted Transaction Write Shield (Compaction Race Prevention)**:
   - An explicit uncommitted transaction write shield tracks active writer leases and in-flight snapshot mutations.
   - Objects and generation pages touched by uncommitted transactions are pinned in the write shield set and excluded from reclamation until the owning transaction commits or aborts.
   - Concurrent active writers cannot have their pending payloads corrupted, evicted, or compacted away during an in-flight snapshot compaction cycle.

## Consequences
- Reclaims generational CAS disk footprint without sacrificing cryptographic integrity or object retrievability.
- Eliminates compaction-vs-writer race conditions via the uncommitted transaction write shield.
- Establishes fail-closed static bounds on generation monotonicity and page allocations before any storage mutation occurs.
