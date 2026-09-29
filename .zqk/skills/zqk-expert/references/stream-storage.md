# Stream Storage: Append-Only Segments for High-Volume Kinds

**Status:** Implemented  
**Purpose:** Avoid CAS overhead for high-volume kinds by writing to append-only segment files (multiple records per file). Uses the **change journal pattern** where only deltas are stored for `change_journal_entry`. The **legacy one-file-per-object (CAS) format is deprecated** for high-volume kinds; see [HIGH_VOLUME_STORAGE_DEPRECATION.md](./HIGH_VOLUME_STORAGE_DEPRECATION.md). References: BYPASS_KIND_STORAGE.md, AUDIT_STREAM_FORMAT.md, INTERNAL_OBJECTS_AS_DEDICATED_WALS.md, HIGH_VOLUME_EVENT_INDEXES.md.

**Vocabulary:** End-to-end policy-driven maintenance of these streams (compaction, retention, overlays) is **object stream stewardship** (synonym: *stream maintainer pattern*). Canonical `glossary_term`: **GLS-1774059548288204000-31cf7bc6** (`zqk object get GLS-1774059548288204000-31cf7bc6`). Template for new environments: `scripts/templates/glossary_object_stream_stewardship.yaml`. Requirement articulation: **REQ-STREAM-001**.

**Implementation hook:** After retention, the scheduler maintenance runner invokes **`storage.PostRetentionStreamStewardship`** (`pkg/storage/stream_stewardship.go`) — registry compaction, orphaned segment GC, runtime-delta backfill and overlay GC. Steward wake is **per stream-backed kind** (see [STREAM_KIND_STEWARDSHIP.md](./STREAM_KIND_STEWARDSHIP.md)). Same entry point is safe to call from tests on an empty tree.

---

## 1. Enablement

- **Env:** `ZQK_STREAM_STORAGE_ENABLED=true` (or `1`) enables stream storage for supported kinds.
- **Kinds:** `audit_event`, `change_journal_entry`, all high-volume metrics (`audit_aggregation_metric`, `base_metric`, `command_metric`, `file_lock_metric`, `code_quality_metric`, `scheduler_health_metric`), `mcp_session`, `zqk_session`, `verification_matrix`. See `high_volume_kinds.yaml` and `stream_config.go`.
- When enabled, **Create** for those kinds writes to stream segments instead of CAS (no hash file, no hash registry, no CAS index update). **Retention** continues to use the same index contract (Count, OldestIDs, IDsOlderThan) via the high-volume event cache, which is updated on stream append.

---

## 2. Segment layout

| Kind                  | Directory / file pattern |
|-----------------------|---------------------------|
| audit_event           | `.zqk/streams/audit_event/audit_stream_YYYY-MM-DD.jsonl` |
| change_journal_entry  | `.zqk/streams/change_journal_entry/change_journal_YYYY-MM-DD.jsonl` |
| *_metric              | `.zqk/streams/<kind>/<kind>_YYYY-MM-DD.jsonl` (e.g. audit_aggregation_metric_YYYY-MM-DD.jsonl) |
| verification_matrix   | `.zqk/streams/verification_matrix/<yyyy-mm-dd>_stream.json` (same per-kind directory pattern; updates use `stream_current` — see `stream_current_state.go`) |

- **Window:** Daily (UTC). One file per day; next day rolls to a new file.
- **Format:** JSONL (one JSON object per line, UTF-8, `\n` terminator). Append-only; mutex per kind for concurrent appends.

---

## 3. Change journal pattern (delta-only)

**change_journal_entry:** Only **delta** fields are written (no full-state bloat):

- **Stored:** `id`, `object_ref`, `change_type`, `diff_summary`, `changed_paths`, `created_at`, `created_by`, `title`, and optionally `previous_state` (for rollback).
- **Omitted from stream:** Full object snapshot and other fields not needed for aggregation or reconstruction.

**Metrics (*_metric):** Minimal **delta-style** fields: `id`, `kind`, `status`, `created_at`, `created_by`, plus kind-specific (e.g. `window_start`, `window_end`, `aggregated_entry_count` for audit_aggregation_metric). Retention uses the same index contract (Count, OldestIDs, status for protect_statuses); segment files stay small.

See INTERNAL_OBJECTS_AS_DEDICATED_WALS.md and change_journal_compaction.md for the same delta/compaction philosophy.

---

## 4. Index and retention

- **Per-record location:** On append, the write path registers `id → segmentPath::offset` in the **stream location registry** (in-process) and adds an entry to the **high-volume event cache** (id, kind, created_at, file_path = `segmentPath::offset`).
- **Same contract:** Count, OldestIDs, and IDsOlderThan are satisfied by the high-volume cache; retention and aggregation logic are unchanged.
- **Delete:** Stream-backed objects are **soft-deleted**: removed from the stream registry and from the cache; the segment file is not rewritten (record remains on disk but is no longer visible to List/Get/retention).

---

## 5. Implementation

- **Append:** `pkg/storage/stream_storage.go` – `AppendToStream(projectRoot, kind, id, obj, createdAt)` → (segmentPath, offset). `recordForStream` reduces `change_journal_entry` to delta fields and metrics to minimal delta-style fields.
- **Config:** `pkg/storage/stream_config.go` – `StreamStorageEnabledForKind(kind)`.
- **Path resolution (no hardcoding):** All paths use the **path alias cache** and scheme prefixes (`prefix:`, `abs:`, `web:`). See [PATH_ALIAS_RESOLUTION.md](./PATH_ALIAS_RESOLUTION.md). Segment dirs resolve as `prefix:streams/<kind>`. Call sites use `paths.ResolvePath` or `GetStreamSegmentDir`. The cache is built during pre-warm **Tier 0**. Architectural policy and requirement: see process objects (path resolution from cache). Do not hardcode paths; use the resolver.
- **Create path:** `object_storage_file_create.go` – when stream enabled for kind, `writeObjectToStream` appends, sets stream location, and updates high-volume cache; no CAS or hash registry.
- **Object path resolution:** `getObjectFilePath` returns `segmentPath::offset` when the object is in the stream registry.
- **Delete:** `object_storage_file_delete.go` – if path is `segmentPath::offset`, soft delete (remove from registry and cache only).

---

## 6. Alignment with timeseries prototype (delta storage)

The codebase has a **timeseries prototype** for numeric, append-only data that stores only **deltas** (base + delta encoding, chunked files):

- **`pkg/metrics/timeseries.go`**: `TimeSeriesWriter` / `TimeSeriesReader` – chunked files per time window, **delta time** (varint seconds from chunk base) and **delta value** (zigzag varint). Used for **object_volume** (object-count-report) and health-check monitors. See TODO(PRI-218 / metrics-timeseries) for integration checklist.

**Alignment:**

| Use case | Storage shape | Delta pattern |
|----------|----------------|---------------|
| **Event log** (audit_event, change_journal_entry) | Stream segments (JSONL per day) | change_journal: only delta fields (object_ref, change_type, diff_summary, changed_paths, …). audit_event: full event per line (could add dictionary compression later). |
| **Numeric time series** (object_volume, gauges) | Chunked binary (timeseries.go) | Base + delta time and delta value (varint/zigzag). |

Both patterns avoid full-state bloat: **event streams** store minimal per-record fields (delta-only for change_journal); **time series** store deltas from a chunk base. For metrics *objects* (audit_aggregation_metric, *_metric), stream storage uses the same segment-per-day layout and minimal per-record fields (id, kind, status, created_at, and only what retention/aggregation need) so storage stays optimized.

---

## 7. Stream-only write policy (no YAML for stream-backed kinds)

When stream storage is **enabled** for a kind, that kind must **not** be written to `.zqk/process/` YAML (CAS). All writes must go through the stream path so that:

- No new files are created under `.zqk/process/audit/`, `.zqk/process/scheduler_jobs/`, etc., for those kinds.
- Object count and retention use the high-volume cache and stream registry only; disk YAML count for those dirs does not grow.

**Required behavior:**

1. **Create / update / aggregated flush:** Use `fileStorage.Create`, `fileStorage.Update`, or `writeObjectToStorage` so the code path chooses stream when `StreamStorageEnabledForKind(kind)` is true. Do **not** use `WriteSystemObjectAndRegisterHash` for stream-backed kinds when stream is enabled—that would create YAML under .zqk/process. See `audit_events.go` (WriteSystemObjectAndRegisterHash comment) and `audit_event_buffer_flush.go` (aggregated event flush).
2. **Audit event buffer flush:** When stream is enabled for `audit_event`, the buffer **requires** `fileStorage` and uses only `writeObjectToStorage` (stream). If `fileStorage` is nil, flush returns an error instead of falling back to CAS/YAML.
3. **Write-behind replay:** WAL replay for stream-backed kinds uses minimal records (no payload); apply skips when data is empty and kind is stream-enabled. Replay with payload still goes through `writeObjectToStorage` → stream.

**Stream-enabled kinds** (see `stream_config.go`): `audit_event`, `change_journal_entry`, `audit_aggregation_metric`, `base_metric`, `command_metric`, `file_lock_metric`, `code_quality_metric`, `scheduler_health_metric`, `mcp_session`, `scheduler_job`, `zqk_session`.

---

## 8. Stream registry and deleted-ID state files (growth and cleanup)

**Location:** `.zqk/state/`  
**Files (per kind):**

- **`stream_registry_<kind>.jsonl`** — Append-only. Each line: `{"id":"...","loc":"segmentPath::offset"}`. Every created stream-backed object gets one line; lines are **never removed** on delete.
- **`stream_deleted_<kind>.jsonl`** — Append-only. One object ID per line. Every soft-deleted stream-backed object gets one line; lines are **never removed**.

**Why both:** On delete we only remove the ID from the **in-memory** registry; we do not rewrite the registry file. So the registry file still contains id→loc for deleted events. The deleted file is the source of truth for “this ID was deleted”: when loading the snapshot we build `locations` from the registry and `deleted` from the deleted file, and treat an ID as live only if it’s in `locations` and not in `deleted`. So both files are required for correct List/Get/Count.

**Current behavior:** Both files **grow indefinitely**. There is no retention, compaction, or truncation. For high-volume kinds (e.g. `audit_event`) with aggressive retention, the deleted file can reach hundreds of thousands of lines (e.g. 400k+). Impact:

- **Disk:** State directory size grows without bound.
- **Load cost:** `loadStreamRegistrySnapshot` reads both files in full and builds two maps. Cache is invalidated after every batch of deletes, so large deleted files mean repeated full reads and large in-memory sets.

**Cleanup plan (to implement):** Add a **compaction** path (scheduler job or maintenance command) that:

1. Reads the full registry and full deleted set for the kind.
2. Writes a **new registry file** containing only lines for IDs that are **not** in the deleted set (i.e. live IDs only).
3. Replaces the old registry with the new one (atomic rename).
4. **Truncates or rewrites the deleted file to empty** (or to a minimal sentinel), since all those IDs are no longer in the registry and no longer need to be tracked.

**Routine compaction:** Run `zqk system compact-stream-state --kind audit_event` (or other stream-backed kind) when the deleted count is high. See **WAL_AND_STATE_FILES.md** for all WAL and state file naming and cleanup.

---

## 9. References

- **Glossary (logical data paths):** **Data stream summary** — `GLS-1774504419525806000-78a9c2e4` (`zqk object get GLS-1774504419525806000-78a9c2e4`); **Data cell** (coordinator membrane around stream/CAS nucleus) — `GLS-1774504405009664000-c1c04629`. Requirements: **REQ-DATASTREAM-001**, **REQ-DATACELL-001**; backlog: **[REDACTED-ID]**.
- [DATA_STORAGE_PRODUCTION_ROADMAP.md](./DATA_STORAGE_PRODUCTION_ROADMAP.md) — Optimize first, then timestamp compression and timeseries wiring
- [BYPASS_KIND_STORAGE.md](./BYPASS_KIND_STORAGE.md) – Motivation and aggregate storage
- [AUDIT_STREAM_FORMAT.md](./AUDIT_STREAM_FORMAT.md) – Audit stream file format
- [INTERNAL_OBJECTS_AS_DEDICATED_WALS.md](./INTERNAL_OBJECTS_AS_DEDICATED_WALS.md) – WAL pattern for internal kinds
- [HIGH_VOLUME_EVENT_INDEXES.md](./HIGH_VOLUME_EVENT_INDEXES.md) – Index contract (Count, OldestIDs, IDsOlderThan)
- `pkg/metrics/timeseries.go` – Timeseries prototype (base+delta, chunked)
- `pkg/storage/stream_storage.go`, `stream_config.go`, `high_volume_event_cache.go`
- §7 above: stream-only write policy (no YAML for stream-backed kinds)
- [WAL_AND_STATE_FILES.md](./WAL_AND_STATE_FILES.md) — WAL and state file naming, lifecycle_events purpose, routine compaction
