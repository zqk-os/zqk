# Object Maintenance: Why It Underperforms and How to Redesign It

**Last Verified:** 2026-08-31


**Purpose:** Explain what has made object maintenance (retention, aggregation, counts toward target) perform poorly, what was overlooked, and what a ground-up redesign would change. Use for planning and for avoiding the same traps in future work.

**References:** PERFORMANCE_INVESTIGATION_AND_RECOMMENDATIONS.md, RETENTION_TARGET_VISIBILITY_GAP.md, OBJECT_COUNT_MANAGEMENT.md, OBJECT_OPERATIONS_PERFORMANCE.md, handlers_retention_tolerance.go, high_volume_event_cache.go.

---

## 1. What has contributed to poor performance

### 1.1 Storage and index design don’t support “delete oldest” natively

- **CAS index is ID → hash only.** There is no `created_at` (or any ordering) in the index. So “keep at most N and delete oldest first” cannot be answered from the index alone.
- **To get “oldest” you must either:** (a) read object bodies to read `created_at`, or (b) maintain a separate structure (e.g. the high-volume event cache) that has creation order.
- **Current design:** Retention uses (b). That structure is built in a **one-off job-phase** (start of audit_event_aggregation) with a **shared timeout**. If the build doesn’t finish (e.g. 260k+ events, 5-minute cap), the cache stays empty and retention falls back to **List(sort by created_at, limit 2000)** in a loop. That List path **loads and sorts objects** (or walks all IDs and reads each file for created_at), so each batch is expensive and you get only ~30–2000 deletes per batch. So **the fast path depends on a cache that often never gets built** in real conditions.

**Design flaw:** Retention’s efficiency was made to depend on a separate, time-limited “build a full cache” step instead of making “count” and “oldest N” **first-class in the storage model** (e.g. index that supports ordering or a layout that makes “oldest” cheap).

### 1.2 The fast path is gated by a single, fragile build

- **Cache build runs inside** the audit_event_aggregation job, with a timeout (e.g. 5 minutes or job deadline − 1 min). With 260k+ events, build does: ListIDs from CAS index, then **64 workers** each reading `created_at` from object files, then sort. That can exceed the timeout, so the cache is never populated.
- **No fallback:** If the build fails or times out, retention has **no** “medium” path (e.g. “use CAS index order + sample” or “incremental cache”). It goes straight to the **slow path** (batched List with SortBy), which does not scale.
- **Cache is process-scoped and one-shot.** It’s not built incrementally (e.g. “add new events as they’re created”), so every daemon restart or first run needs a full build again. No “warm cache over time.”

**Overlooked:** Decoupling cache build from the aggregation job (e.g. dedicated cache-build at daemon start with its own long timeout, or an incremental index that grows on write) and defining a **medium path** when the full cache isn’t ready.

### 1.3 List(sort by created_at) is O(read everything) on file-backed storage

- **List with SortBy** on file-backed CAS has no index on `created_at`. So the implementation either: (i) loads all objects (or a huge subset) and sorts in memory, or (ii) walks IDs and reads each file to get created_at, then sorts. Both are O(n) reads.
- **Retention’s batched List** does List(limit=2000, sort=created_at asc). To get the “oldest 2000” the backend may still have to consider a large set (or the whole set) to sort. So each batch can be very slow and time out, yielding only a small number of deletes per batch.
- **Result:** When the cache isn’t populated, delete rate is tiny (e.g. ~30–2000 per batch) and total time to go from 270k → 600 is enormous.

**Overlooked:** Making “oldest N by created_at” a **storage-level operation** (e.g. index that stores created_at or a dedicated structure), so retention never has to “list everything and sort” on the hot path.

### 1.4 Per-operation I/O and index updates

- **Every Create/Delete** triggers FlushKind (and often full index load → merge one mapping → save). So each delete in a retention run pays full index update cost. With 50k deletes, that’s 50k index updates unless batched.
- **Batch index updates** (SetMappings, RemoveMapping in bulk) exist but are not used everywhere. So retention’s bulk delete can still drive O(deleted) index writes.
- **Hash registry** for large kinds (e.g. audit_event with 260k entries) does full marshal + write + sync per save; under load that can take minutes and cause timeouts/rollbacks.

**Overlooked:** Designing for **bulk delete as a first-class path**: delete N objects by ID list with **batched** index updates and **deferred or asynchronous** flush, so one retention run doesn’t serialize 50k individual index writes.

### 1.5 Coordination and contention

- **Multiple retention-tolerance runners** (daemon maintenance, CLI, cleanup script) were allowed to run in parallel. They contended on storage, locks, and index, so **running more made things worse**. That was only later fixed with a project-level singleton lock.
- **Aggregation and retention share the same process and same global state.** Cache build, aggregation, and retention all compete for the same job deadline and the same disk. There’s no first-class “maintenance readiness” (e.g. “cache built,” “index compacted”) that retention checks before choosing a path.

**Overlooked:** Treating “only one retention run per project” as a **design requirement from the start**, and modeling “maintenance readiness” so retention can choose fast vs medium vs slow path explicitly.

### 1.6 Observability and bootstrap

- **“Over target” was invisible** in the main feedback loops (system check, health, reminders). So the system could be far over target and never surface it unless someone ran `retention-status` or internal count manually (RETENTION_TARGET_VISIBILITY_GAP).
- **Retention/aggregation jobs** were not guaranteed to exist or to have the correct `job_type`. Wrong job_type (e.g. context_refresh instead of retention_tolerance) meant jobs “ran” but never enforced limits. No single bootstrap step that “ensures the right jobs exist and run.”

**Overlooked:** Building **retention status into the main feedback loop** from day one, and **establishing** the advancing job (correct type, schedule) as part of init or a single “ensure” command so that “running” implies “actually advancing toward target.”

---

## 2. What a ground-up redesign would change

### 2.1 Make “count” and “oldest N” cheap in the storage layer

- **Option A – Index includes ordering:** For high-volume kinds (e.g. audit_event, metrics), the primary index (or a secondary structure) stores **id, created_at** (and maybe status). Then:
  - **Count** = index size (or a maintained counter).
  - **Oldest N** = read the first N entries from an ordered structure (e.g. B-tree or sorted index), no full scan, no reading every object body.
- **Option B – Layout and append log:** Store events in an append-only log (or time-bucketed segments) so “oldest” is “first segment” or “first N entries in log.” Retention deletes by segment or by range of IDs from the log, not by “list all then sort.”
- **Principle:** Retention should **never** need to “list all objects and sort by created_at” on the hot path. The storage layer should expose **Count** and **OldestIDs(limit)** (or equivalent) as primitive, efficient operations.

### 2.2 Decouple “cache” (or ordered index) build from aggregation

- **Don’t tie cache build to the aggregation job’s timeout.** Either:
  - **Dedicated build at daemon start:** A single “maintenance prep” or “cache prewarm” phase with a **long, dedicated** timeout (e.g. 15–30 minutes) that only builds the structure retention needs; aggregation and retention run **after** that, using the structure.
  - **Incremental structure:** Maintain an “oldest” structure incrementally: on each Create, append (id, created_at) to a compact structure (e.g. a small index or log); retention reads from it. No one-off “build from 260k files” step.
- **Principle:** The thing retention needs (ordered view of ids by creation) should either be **always available** (from the primary index) or **built once with enough time** or **grown incrementally**. It should not depend on “run aggregation job and hope the cache build finishes in 5 minutes.”

### 2.3 Bulk delete and batch index updates as the default

- **Bulk delete by ID list:** One retention run should call a single (or few) “DeleteIDs(ids []string)” style API that:
  - Removes files (or marks segments for deletion).
  - Updates the index in **batches** (e.g. one load, remove N mappings, one save), not one load/save per ID.
- **Deferred or async flush:** So that retention doesn’t block on FlushKind per delete; index and disk can catch up in the background or in a single flush at the end of the batch.
- **Principle:** Retention is a **bulk operation**. The storage layer should optimize for “delete these 50k IDs” with minimal per-ID syscalls and index writes.

### 2.4 One retention run per project, and explicit path selection

- **Singleton retention** is a **design requirement**, not an afterthought. Only one process (daemon or CLI) runs retention at a time; others skip or exit with a clear message.
- **Path selection:** Before running retention, check: “Do we have an ordered structure (index or cache) for this kind?” If yes → **fast path** (delete by oldest IDs from that structure). If no → **medium path** if available (e.g. CAS index + sample, or bounded List), or **slow path** with clear logging (“using slow path: no ordered index”). So we never “hope” the cache is there; we **decide** which path and log it.

### 2.5 Retention status and targets in the main feedback loop

- **Check / health / reminders** should include “object count vs retention target” (e.g. “over target” or “at target”). So “are we advancing?” is visible without running a separate retention-status command.
- **Bootstrap:** Init or a single “ensure” step guarantees that at least one retention_tolerance job and one audit_event_aggregation job (or equivalent) **exist**, have the **correct job_type**, and run on a schedule. “System is running” should imply “retention jobs exist and are configured correctly.”

### 2.6 Catch-up as a first-class scenario

- **Design for “270k → 600”** from the start: large backlog, single run (or few runs) should be able to delete in large batches (e.g. 50k–90k per run) without timeouts. That implies:
  - Fast “oldest N” from index or cache.
  - Bulk delete and batch index updates.
  - No dependency on a cache build that can’t finish in the same run.
- **Operationally:** Optional “maintenance burst” mode (e.g. longer timeout, or dedicated catch-up job) so that after a long outage or initial load, one or a few runs can bring counts down without requiring days of 15-minute cycles.

---

## 3. What was overlooked (summary)

| Area | What was overlooked |
|------|---------------------|
| **Storage model** | Making “count” and “oldest N by created_at” cheap in the index or layout so retention doesn’t depend on a separate cache build. |
| **Cache / ordered structure** | Decoupling its build from the aggregation job; incremental build or dedicated long-timeout build so it’s ready before retention runs. |
| **List(sort by created_at)** | It’s O(n) on file-backed storage; assuming “we can just list and sort” for retention led to the slow path. Should be a storage primitive, not “list everything then sort.” |
| **Bulk delete** | Designing for “delete 50k by ID” with batched index updates and deferred flush; avoiding per-delete FlushKind and per-ID index save. |
| **Coordination** | “Only one retention run per project” and “maintenance readiness” as explicit design requirements from the start. |
| **Feedback loop** | Surfacing “over target” and “not advancing” in check, health, and reminders; ensuring jobs exist with correct type and schedule. |
| **Catch-up** | Treating “massive backlog → target” as a normal scenario and designing fast path and timeouts so a single run can make large progress. |

---

## 4. Practical next steps (without a full rewrite)

If a full redesign isn’t feasible yet, the highest-leverage improvements are:

1. **Ordered index or dedicated “oldest” structure:** Add created_at (or a proxy) to the CAS index (or a sidecar index) for audit_event (and optionally other high-volume kinds) so Count and “oldest N IDs” don’t require reading every object. Then retention’s fast path uses this index instead of the separate high-volume cache.
2. **Build that structure at daemon start:** Run a dedicated “build ordered index / cache” step at scheduler (or CLI) start with a long timeout (e.g. 15–30 min), before any aggregation or retention. So retention’s first run already has the fast path available.
3. **Bulk delete API and batch index updates:** Implement (or use) DeleteIDs(ids) with batched RemoveMapping and a single FlushKind at the end of the batch. Use it in retention instead of “delete one by one.”
4. **Retention status in check/health:** Add a short “retention” section to system check or health output (over target / at target, and suggested action). So the main feedback loop always shows whether we’re advancing.

These preserve the current architecture but move the bottlenecks (no ordered index, cache build inside aggregation, per-delete index updates, invisible status) toward the redesigned behavior.

### 4.1 Implementation status

| Item | Status | Notes |
|------|--------|-------|
| 1. Ordered index | Implemented | CAS index stores optional created_at (ID -> RFC3339); Create passes it for high-volume kinds; IDIndex.OldestIDs(limit); retention uses index first, then cache. |
| 2. Build at daemon start | Implemented | scheduler_core.go: background goroutine calls EnsureHighVolumeEventCacheReady with 20 min timeout; non-blocking. |
| 3. Bulk delete / batch index | In place | retention_tolerance uses BulkDeleteOptimized when cache populated; CleanupAggregatedEvents uses BulkDeleteOptimized. |
| 4. Retention status in check/health | In place | GetRetentionDriftReminder in system check (reason_code retention_drift); zqk system retention-status for details (PRUNED). |

See **HIGH_VOLUME_EVENT_INDEXES.md** for index requirements for all high-volume kinds and alignment with the planned bundled (multi-event) storage format.

### 4.2 Optional / not first-class yet

- **Maintenance burst** (§2.6): no dedicated “burst mode” job type or template yet. Operators use **`zqk system retention-tolerance` (PRUNED)** with **`--max-batches -1`** (and appropriate **`--batch-size` / workers**) and scheduler **`max_runtime_seconds`** on maintenance jobs for large catch-up; see CLI examples on the retention-tolerance command.

---

## 5. System objects (outline of work)

The following system objects were created to track this work (via `zqk object create`). Backlog items use **`priority_plan_ref=[REDACTED-ID]`** (*Object maintenance redesign*).

| Type | ID / scope | Purpose |
|------|------------|---------|
| **Priority plan** | [REDACTED-ID] | Object maintenance redesign (planning). |
| **Goal** | [REDACTED-ID] | Fast, predictable object maintenance and catch-up. |
| **Criteria** | [REDACTED-ID], [REDACTED-ID], [REDACTED-ID], [REDACTED-ID] | Ordered index; bulk delete; manual reset; retention status in feedback loop. |
| **Requirements** | [REDACTED-ID] (ordered index), [REDACTED-ID] (decouple cache), [REDACTED-ID] (bulk delete), [REDACTED-ID] (manual reset), [REDACTED-ID] (feedback loop). | Link goal + criteria to backlog. |
| **Backlog items** | `zqk object list backlog_item --filter priority_plan_ref=[REDACTED-ID]` | Storage primitives; decouple cache; bulk delete; **manual reset (shut down, bulk delete, sacrifice data)**; retention status in feedback loop. |

**Manual reset:** When catch-up is not feasible, the supported path is to shut everything down, perform a manual delete of high-volume data (e.g. audit_event, metrics), and restart; data sacrifice is an accepted outcome. Backlog item and requirement capture documenting and supporting this path.
