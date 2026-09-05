# High-Volume Event Indexes: Efficient Management and Bundled-Storage Alignment

**Last Verified:** 2026-08-31


**Purpose:** Define efficient index requirements for all high-volume event kinds so retention, count, and time-window operations are optimal. Align index design with the planned move from 1 event/YAML file to a lighter-weight, bundled format (multiple events per file/stream).

**References:** OBJECT_MAINTENANCE_REDESIGN.md, BYPASS_KIND_STORAGE.md, AUDIT_STREAM_FORMAT.md, INTERNAL_OBJECTS_AS_DEDICATED_WALS.md, high_volume_event_cache.go.

---

## 1. High-volume kinds in scope

All kinds that are retention-managed at scale and need **count**, **oldest N**, and **time-window** without full scans:

| Kind | Retention use | CAS today | Index need |
|------|----------------|-----------|------------|
| audit_event | max_count, age, aggregation cleanup | Yes | Count, OldestIDs, IDsOlderThan(cutoff) |
| audit_aggregation_metric | max_count, age | Yes | Count, OldestIDs, optional status filter |
| base_metric, command_metric, file_lock_metric, code_quality_metric, scheduler_health_metric | max_count, age, protect_statuses | Yes | Count, OldestIDs, status for prune |
| change_journal_entry | max_count, age | Yes | Count, OldestIDs, IDsOlderThan |
| mcp_session | max_count, age, protect_statuses | Yes | Count, OldestIDs, status for prune |

**Principle:** Every high-volume kind should have an **index (or index-like structure)** that supports at least:

- **Count()** – O(1) or O(index size), not O(disk scan).
- **OldestIDs(limit)** – Oldest N by created_at for retention max_count enforcement.
- **IDsOlderThan(cutoff, limit)** – For age-based retention and aggregation cleanup.

Optional for kinds with protect_statuses: filter by status so we only consider deletable candidates.

---

## 2. Index contract (storage-format agnostic)

The following contract allows retention and aggregation to stay efficient regardless of whether events are stored as **one file per event** (current CAS) or **bundled** (multiple events per file/stream).

### 2.1 Required operations

| Operation | Purpose |
|-----------|---------|
| **Count(kind)** | Total number of objects for the kind (for over-target checks and reporting). |
| **OldestIDs(kind, limit)** | Up to `limit` object IDs with smallest created_at (for max_count enforcement: delete oldest first). |
| **IDsOlderThan(kind, cutoff, limit)** | Up to `limit` IDs with created_at &lt; cutoff (for age-based retention and aggregation cleanup). |

Optional:

- **IDsWithStatus(kind, statuses)** or filter in OldestIDs/IDsOlderThan by status (for metrics with protect_statuses).

### 2.2 Who uses the index

- **Retention (max_count):** OldestIDs(kind, toDelete); then bulk delete those IDs.
- **Retention (age):** IDsOlderThan(kind, cutoff, batchSize); then bulk delete or archive.
- **Aggregation cleanup:** IDsOlderThan(audit_event, cutoff, batchSize); then bulk delete.
- **System check / retention-status:** Count(kind) for over-target visibility.

Today these are served by the **high-volume event cache** (for audit_event when populated) or by slow paths (List with SortBy created_at, or full scan). The goal is to have **efficient indexes for all high-volume kinds** so no kind falls back to full-scan or one-shot cache build under time pressure.

---

## 3. Today vs bundled storage

### 3.1 Current layout (1 event per file)

- **CAS index:** ID → hash (and optional bucket key). No created_at; no ordering.
- **High-volume cache:** Separate structure (in-memory + persisted JSON) built from CAS ListIDs + reading created_at from each file. Built at daemon start (background) or at start of aggregation job. Only **audit_event** is built from index today; other high-volume kinds (metrics, change_journal_entry, mcp_session) do not get cache build from index, so they fall back to List/sort or are not optimized.

To reach “efficient indexes for all high-volume events” **today**:

1. **Option A – Extend high-volume cache to all high-volume kinds:** Build cache from CAS index for audit_event, *_metric, change_journal_entry, mcp_session (same pattern: ListIDs + parallel read of created_at). Retention and count use cache when populated. Downside: still a separate structure and one-shot build per kind.
2. **Option B – Ordered index (CAS or sidecar):** Add created_at (and optionally status) to the CAS index or a sidecar index (id → created_at [, status]). Then Count = index size; OldestIDs = read first N from an ordered view. Single source of truth; no separate cache build. Requires index format change and migration.

### 3.2 Future: bundled format (multiple events per file)

Planned direction (BYPASS_KIND_STORAGE, AUDIT_STREAM_FORMAT, INTERNAL_OBJECTS_AS_DEDICATED_WALS): move from 1 YAML per event to **multiple events per file** (e.g. JSONL stream per day, or segment files). Benefits: fewer files, less index churn, compression, append-only writes.

**Index design for bundled storage:**

- **Per-event index:** (id → segment_id, offset [, created_at]). Updated on every append. Supports Get(id), Count (index size), and if created_at is in the index, OldestIDs and IDsOlderThan without reading event bodies.
- **Segment-level metadata:** Each segment file can have a small header or sidecar: min_created_at, max_created_at, event_count. Then “oldest N” can be implemented by: order segments by min_created_at, take IDs from oldest segments first (using per-event index or scanning only those segment files). Enables **delete by segment** (e.g. drop oldest day file) without scanning every event.
- **Same contract:** Count(), OldestIDs(limit), IDsOlderThan(cutoff, limit) are implemented over the index and segment metadata; retention and aggregation code do not care whether storage is 1-file-per-event or bundled.

So: **efficient indexes are the abstraction**. The storage format (current CAS vs future stream/bundle) sits behind the index. When we introduce bundled format, we implement the same index contract on top of segment metadata + per-id index.

---

## 4. Recommendations

### 4.1 Short term (current 1-file-per-event)

- **All high-volume kinds use the same index contract:** Count, OldestIDs, IDsOlderThan (and status filter where needed).
- **Extend high-volume cache build to all high-volume kinds** that use CAS (audit_event already; add *_metric, change_journal_entry, mcp_session) so daemon-start and in-job build populate the cache for every high-volume kind. Retention then uses cache for all of them instead of falling back to List/sort.
- **Keep daemon-start cache build** (already implemented) so the first retention/aggregation run often has the fast path for audit_event; extend to other kinds in the same build pass.

### 4.2 Medium term (ordered index)

- **Add created_at to the index** (CAS index extension or sidecar id → created_at [, status]). Enables Count and OldestIDs without a separate cache build and without reading every file. One source of truth; survives restarts; incremental updates on create/delete.
- **Retention and aggregation** call a single “high-volume index” API (Count, OldestIDs, IDsOlderThan) that is implemented by the ordered index when present, and by the cache when the ordered index is not yet available (backward compatibility).

### 4.3 Long term (bundled storage)

- **Bundled format** (stream/segment files) is introduced with an index that satisfies the same contract: Count, OldestIDs, IDsOlderThan, updated incrementally on append. Segment metadata (min/max created_at, count) allows segment-level retention (e.g. drop oldest day) and efficient “oldest N” from oldest segments.
- **Migration:** Existing 1-file-per-event data can be read through the current index/cache until migrated; new writes can go to the bundled format with the new index. No change to retention/aggregation logic beyond using the shared index API.

---

## 5. Summary

| Goal | Action |
|------|--------|
| All high-volume events have efficient indexes | Treat audit_event, *_metric, change_journal_entry, mcp_session uniformly: same index contract (Count, OldestIDs, IDsOlderThan). Extend cache build to all; later add ordered index (created_at) so index is the single source of truth. |
| Optimal management | Retention and aggregation use the index only; no full scans or List(sort) on the hot path. |
| Align with bundled format | Index contract is storage-format agnostic. When moving to multiple-events-per-file, implement the same contract over segment metadata + per-id index; retention continues to use Count/OldestIDs/IDsOlderThan. |

This keeps management efficient today and ensures the same efficiency after converting to the lighter-weight, bundled storage format.
