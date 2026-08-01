# Bypass-Kind Storage: Avoiding CAS Overhead

**Status:** Design  
**Purpose:** For bypass (system-generated) kinds, avoid the overhead of content-addressable storage by using the same file to capture multiple similar events/objects, minimizing bits, I/O, size, churn, and concurrency contention.

---

## 1. Motivation

**Bypass kinds** (e.g. `audit_event`, `change_journal_entry`, `scheduler_job`, `audit_aggregation_metric`) are system-generated. They are not user-edited; they are created and updated by trusted paths (scheduler, retention, aggregation, CLI system commands). Per [QUARANTINE_AS_SAFETY_SWITCH.md](./QUARANTINE_AS_SAFETY_SWITCH.md), they must never have violations or integrity issues.

**Current storage:** These kinds use **CAS** (content-addressable storage): one hash-named file per object (`{hash}.yaml`), with an index mapping ID → hash. Every create/update writes a new file and orphans the old one (then async cleanup renames to `.tmp` and deletes). That implies:

- **High I/O** – New file per object/update; index update; orphan cleanup.
- **Size/churn** – Many small files; directory and index churn; duplicate content across similar objects (no sharing).
- **Concurrency** – Contention on the index, directory, and CAS write queue; many small writes amplify lock and filesystem overhead.

For system-generated, high-volume data we do **not** need content-addressable integrity per file (no user edits, no need to detect tampering per object). We can trade that for a more efficient storage shape.

---

## 2. Goal: Same File, Multiple Similar Events/Objects

For bypass kinds we should **avoid CAS overhead** by using **one file (or a small set of files) to capture multiple similar events/objects**. That yields:

| Benefit | How |
|--------|-----|
| **Fewer bits** | Same level of detail with shared structure (e.g. compact format, shared keys, or append-only records in one file). |
| **Less I/O** | Append or batch writes instead of one new hash-file per object; fewer index updates. |
| **Less size/churn** | Fewer files; no per-object hash filenames; no orphan cleanup for these kinds. |
| **Speed and concurrency** | Fewer files to create/delete; less contention on index and directory; append/batch reduces lock granularity. |

So: **aggregate storage** for bypass kinds – e.g. append-only log files, time-windowed files, or kind-specific compact files that hold many records – instead of one CAS file per object.

---

## 3. Audit Streams (Audit Events → One File, Compressed)

Instead of storing **audit events** as individual objects (one CAS file per event), we introduce **audit streams**: one file per stream, each containing **multiple events**, **compressed**.

- **Audit stream** = one file that holds many audit events (e.g. time-windowed or size-bounded), stored in a compressed form. Same level of detail as today’s events, but aggregated and compressed to minimize bits, I/O, and file churn.
- **Benefits:** Fewer files; less directory and index overhead; compression reduces size and read/write I/O; append or batch writes instead of one new file per event.
- **API / model:** List, get-by-ID, count, and time-window queries for “audit events” can be served from stream files (e.g. index: event ID → stream file + offset or record index; or scan stream files for range queries). The logical object remains “audit event”; the storage unit becomes “audit stream” (one file, many events, compressed).

Concrete format (compression algorithm, stream boundaries, index shape) is left to implementation. The design choice is: **audit events become audit streams** – multiple events per file, compressed – rather than one file per event.

---

## 4. High-Level Approach (Options)

Concrete format and migration are left to implementation. Possible shapes:

- **Audit streams (above):** For `audit_event`, one file per stream; multiple events per file; compressed. Primary example of aggregate storage for the highest-volume bypass kind.
- **Append-only log (per kind or per kind+window):** One file (or file per time window) to which we append serialized records (e.g. YAML docs, JSON lines, or a compact binary). ID and ordering are maintained by position or an inline index. List/count/range queries read from this file (or a small set) instead of many hash files.
- **Batched / chunked files:** Multiple objects per file (e.g. N objects per file, or one file per hour/day). Lookup by ID uses a small index (ID → file + offset or file + record index) instead of ID → hash and then hash-file read.
- **Compact format:** Schema-aware compact representation (shared keys, minimal per-record fields) in one or few files to minimize bits while preserving the same level of detail.

Existing patterns in the codebase (e.g. `high_volume_event_cache`, audit event buffers, change journal compaction) can inform the exact design. The principle is: **bypass kinds use aggregate/batch storage, not one CAS file per object.**

---

## 5. Related Prototyping and Existing Work

Recent prototyping and existing code that **focus on optimizing storage utilization for high-volume data** are directly applicable to audit streams and bypass-kind storage:

| Work | Relevance |
|------|------------|
| **[INTERNAL_OBJECTS_AS_DEDICATED_WALS.md](./INTERNAL_OBJECTS_AS_DEDICATED_WALS.md)** | Same goal: `audit_event` and `change_journal_entry` as **dedicated append-only WALs** (one or few files per kind/bucket), lean format (JSONL or compact), no per-record YAML/CAS/hash registry. Direct design precedent for “audit events → streams” and for bypass-kind aggregate storage. |
| **`pkg/storage/compressed_snapshot.go`** | **Dictionary compression** for batches of objects: `DictionaryBuilder`, `CompressObjects` / `ExpandObjects`, `.csnap` format (header + dictionary + compressed data). Reusable for **compressing many events in one audit stream file** – build a dictionary over a batch of events, store one compressed blob per stream or segment. |
| **`pkg/storage/change_journal_compaction.md`** | Design for **compressing change journal entries** in batches: dictionary over path strings and values, reuse `CompressObjects` from compressed_snapshot; ID range compression (e.g. CHA-1..CHA-960). Same pattern applies to **audit stream** segments: dictionary + compress a batch of events, optional ID range in metadata. |
| **`pkg/storage/audit_aggregation.go`** | **`compressEventIDs` / `ExpandIDRanges`**: consecutive audit event IDs → ranges (e.g. `AUD-1..AUD-960`). Already used when storing aggregated results. Applicable to **stream metadata or index** (which event IDs are in this stream file) to keep index small. |
| **`pkg/storage/high_volume_event_cache.go`** | Today: cache of event IDs, paths, `created_at` for fast time-window and count queries over existing per-file events. When storage moves to **streams**, the cache can index **stream file + segment or offset** instead of per-event paths; same query optimization, different backing store. |

**Summary:** The **internal-objects-as-WAL** design and the **compressed-snapshot / change-journal-compaction** prototyping (multiple records in one file, dictionary compression, ID ranges) are the same direction as audit streams and bypass-kind storage. Implementation of audit streams can reuse dictionary compression and WAL/append patterns from this work.

---

## 6. Invariant and Scope

- **System-generated objects never have violations** (see QUARANTINE_AS_SAFETY_SWITCH). They are never quarantined. So we do not need CAS’s per-file integrity guarantee for them; we rely on generation-path correctness and optional checksums at the aggregate level if needed.
- **Scope:** Bypass kinds as defined by blocking check config (`GetBypassKinds()`). Today: `audit_event`, `change_journal_entry`, `scheduler_job`, `audit_aggregation_metric`. New bypass kinds should use the same aggregate storage strategy.
- **User-facing behavior:** List, get, count, and time-window queries for these kinds must remain correct; only the on-disk layout and write path change.

---

## 7. Relationship to Current CAS Design

- **Non-bypass kinds** continue to use CAS (one hash-file per object, index, integrity by content-address). User-edited and process data benefit from content-addressable integrity.
- **Bypass kinds** migrate from CAS to aggregate storage (one or few files per kind/window, multiple objects per file). This is a **storage-tier optimization** for system-generated data, not a change to the quarantine or validation invariant.

---

## 8. References

- [QUARANTINE_AS_SAFETY_SWITCH.md](./QUARANTINE_AS_SAFETY_SWITCH.md) – Invariant: system-generated objects never have violations; never quarantined.
- [INTERNAL_OBJECTS_AS_DEDICATED_WALS.md](./INTERNAL_OBJECTS_AS_DEDICATED_WALS.md) – Dedicated WALs for audit_event / change_journal_entry; same direction as audit streams and bypass-kind storage.
- [content-addressable-storage-design.md](../process/architecture/content-addressable-storage-design.md) – CAS architecture (for non-bypass kinds).
- [content-addressable-storage-design-final.md](../process/architecture/content-addressable-storage-design-final.md) – CAS final design.
- `pkg/storage/blocking_check_config.go` – `GetBypassKinds()`.
- `pkg/storage/high_volume_event_cache.go` – High-volume event cache (time-window, count).
- `pkg/storage/compressed_snapshot.go` – Dictionary compression for batches; applicable to compressing stream segments.
- `pkg/storage/change_journal_compaction.md` – Compression design for batched entries; same pattern for audit streams.
