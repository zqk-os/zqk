# High-Volume Storage: Deprecation of Legacy Format and Move to Streaming

**Status:** Policy / direction  
**Purpose:** Deprecate the old one-file-per-object (CAS) format for high-volume kinds and require efficient, compressed storage (stream storage for events, timeseries for numeric series). High-volume kinds keep their object specs; storage and indexing must use the new architecture to avoid unbounded resource use.

**References:** STREAM_STORAGE.md, HIGH_VOLUME_EVENT_INDEXES.md, OBJECT_COUNT_MANAGEMENT.md, OBJECT_COUNT_SELF_MAINTENANCE.md, MAINTENANCE_POLICY.md, `pkg/metrics/timeseries.go`, `pkg/storage/stream_config.go`, PRE_CHANGE_CHECKLIST.md.

---

## Strategic goal: reliable, performant maintenance at scale

**Critical system maintenance for high-volume objects must be reliable and performant** so the system is not bogged down by manual interventions. As **multi-agent workflows** unlock and produce more data, the architecture must:

- **Keep maintenance hands-off:** Retention, aggregation, and cleanup run on schedule and keep counts near target without manual "push toward 3k" or one-off scripts. See OBJECT_COUNT_SELF_MAINTENANCE.md.
- **Scale with load:** Stream storage and timeseries (append-only, delta-style, bounded index) allow Count, OldestIDs, and retention to stay fast as volume grows—instead of degrading (full scans, cache build timeouts, retention job timeouts).
- **Avoid magnifying past problems:** Unbounded file count, slow list/count, cache drift, and retention timeouts have already occurred. The legacy one-file-per-object format amplifies these under higher write volume. Moving high-volume kinds to stream/timeseries is the path to **upscaling capabilities without magnifying those problems**.

This doc and the migration path (§5) describe how to get high-volume maintenance to that reliable, performant place.

---

## 1. Deprecation of the old format

**Legacy format (deprecated for high-volume):** One YAML file per object under `docs/architecture/`, content-addressed by hash, with a CAS index. This format is acceptable for low-volume kinds (backlog, requirements, orgs, etc.) but is **deprecated for high-volume kinds** because it:

- Consumes excessive disk and inode count (one file per event/metric).
- Drives unbounded index and cache build time (ListIDs + read created_at per file).
- Does not compress or batch writes, increasing I/O and lock contention.

**High-volume kinds** (see §2) **must** use:

- **Event-like objects** (audit_event, change_journal_entry, *_metric): **Stream storage** — append-only segment files (e.g. JSONL per day), delta-style records where applicable, index contract via high-volume event cache (Count, OldestIDs, IDsOlderThan). See STREAM_STORAGE.md.
- **Numeric time series** (object_volume, health gauges): **Timeseries prototype** — chunked, base+delta encoded binary (varint/zigzag), one chunk per time window. See `pkg/metrics/timeseries.go`.

**Updates for stream-backed kinds** must **not** go through CAS (no hash compute, no content-addressed file per update). Instead:
- **Change journal** — each update is recorded as a change_journal_entry (object_ref, change_type=update, previous_state, diff_summary) for audit and rollback.
- **Stream-current overlay** — the latest state is written to `.zqk/state/stream_current/<kind>/<id>.yaml` (single overwrite per update, no hash). Read prefers this file when present; otherwise the initial stream record is used. Routine touches (e.g. session updated_at) are not treated as tamper-sensitive and do not trigger CAS re-validation.

Specs for high-volume kinds remain the source of schema and validation; only the **storage and indexing** path are required to use the new architecture.

---

## 2. Designated high-volume kinds

A kind is **high-volume** if it is retention-managed at scale and needs efficient Count, OldestIDs, and time-window operations without full scans. The canonical list for **storage policy** (stream required) and **retention config** is:

| Kind | Retention | Storage requirement |
|------|-----------|----------------------|
| audit_event | max_count, age, aggregation | Stream (segment per day, delta-friendly) |
| audit_aggregation_metric | max_count, age | Stream (delta-style fields) |
| base_metric, command_metric, file_lock_metric, code_quality_metric, scheduler_health_metric | max_count, age, protect_statuses | Stream (delta-style) |
| change_journal_entry | max_count, age | Stream (delta-only fields) |
| mcp_session | max_count, age, protect_statuses | Stream or CAS until stream supports status filter |
| scheduler_job | one_time job cleanup (scheduler_job_retention job) | Stream (high-volume at scale: go test, one_time jobs) |
| zqk_session | max_count, age (retention_tolerance) | Stream (session state; retention_tolerance.yaml) |
| verification_matrix | max_count, age, protect_statuses (draft/active) | Stream (create→segment; updates→`stream_current` overlay) |

**Declarative config:** `docs/architecture/_internal/configs/high_volume_kinds.yaml` lists these kinds for documentation and tooling. Code continues to use `streamStorageEnabledKinds` (stream_config.go) and `highVolumeKindsForCacheBuild` (high_volume_event_cache.go); a future refactor can read from the config for a single source of truth.

**Object specs:** High-volume kinds still have full specs in `docs/architecture/_internal/object_specs/` (e.g. audit_event.yaml, base_metric.yaml). No schema change is required; the **storage_policy** is "stream required" (or "timeseries" for numeric-only series).

---

## 3. Requirements for high-volume handling

- **Efficient handling:** Index contract (Count, OldestIDs, IDsOlderThan) must be satisfied without full scans. Use high-volume event cache populated from stream append path (or from CAS during migration).
- **Data compression:** Event streams use minimal per-record fields (delta-only for change_journal; delta-style for metrics). Numeric series use base+delta encoding (timeseries.go). No unbounded in-memory accumulation; streaming reads/writes and bounded buffers (see PRE_CHANGE_CHECKLIST, timeseries TODO).
- **No unnecessary resources:** Avoid one-file-per-object, unbounded goroutines, and full-cache clears on hot paths. Use append-only writes, bounded workers, and incremental cache updates.

---

## 4. Timeseries prototype implementation

The **timeseries prototype** (`pkg/metrics/timeseries.go`) provides chunked, base+delta encoded storage for numeric time series. It is **not yet wired** into production paths. Before using it in hot paths:

1. **PRE_CHANGE_CHECKLIST:** No new unbounded memory or goroutines; streaming reads/writes; no hidden global state. Reuse existing batching/concurrency (bounded workers, back-pressure) when integrated with schedulers or background jobs.
2. **DRY and observable:** Keep encoding/decoding consistent with other metrics code; add basic metrics/logs for chunk sizes, error rates, read/write durations.
3. **Use cases to wire:** (a) Object volume (object-count-report) — write samples to timeseries chunks under `.zqk/metrics/object_volume/`; (b) Health-check numeric series; (c) Any future scalar-per-timestamp metrics.

See the TODO in `pkg/metrics/timeseries.go` (PRI-218 / metrics-timeseries) and STREAM_STORAGE.md §6 for alignment between event streams (JSONL delta) and numeric timeseries (binary delta).

---

## 5. Migration path

1. **Stream storage:** Implemented; **default on** for high-volume kinds (opt-out via `ZQK_STREAM_STORAGE_ENABLED=false` or `0`). New creates for high-volume kinds use stream segments; List/Count/Delete and high-volume cache build include stream-backed IDs.
2. **List/Count/Delete for stream:** Ensure List, Count, and retention BulkDelete use the high-volume cache and stream location registry so stream-backed objects are counted and deletable (soft-delete: remove from registry/cache). See STREAM_STORAGE.md §4.
3. **Migrate legacy YAML to stream:** For kinds that have moved to stream but still have legacy YAML under `docs/architecture/<dir>`, use **`zqk system migrate (PRUNED)-legacy-to-stream --kind <kind>`** to Create each legacy object into the stream (so data is not abandoned). Use `--dry-run` to report what would be migrated; use `--remove-legacy` to delete legacy files after successful migration. Then optional cleanup: run the delete-unmanaged script if needed. See OBJECT_COUNT_MANAGEMENT.md § "Migrate legacy YAML to stream".
4. **Timeseries:** Implement writer/reader integration for object_volume and health monitors; chunk retention is enforced in `object_count_report.go` (`metricsChunkRetentionDays`, shared across object_volume / stream_volume / filesystem_snapshot metrics dirs).
5. **Deprecation window:** Stream is now default; timeseries is wired for object_volume and chunk retention. **CAS create path for high-volume kinds is deprecated**: new creates for those kinds go to stream (or timeseries where applicable). A future release may remove the CAS create code path for high-volume kinds entirely; the read path for legacy CAS data can remain for back-compat until migration is complete.

---

## 6. Production-readiness roadmap

See **DATA_STORAGE_PRODUCTION_ROADMAP.md** for: (1) high-volume fast path optimization, (2) minimizing bytes per event (timestamps), (3) wiring the timeseries prototype into production, (4) externalizing stream delta field lists.

---

## 7. References

- DATA_STORAGE_PRODUCTION_ROADMAP.md — Optimize-first and timestamp/timeseries follow-ups
- STREAM_STORAGE.md — Segment layout, delta pattern, index contract
- HIGH_VOLUME_EVENT_INDEXES.md — Index contract (Count, OldestIDs, IDsOlderThan)
- OBJECT_COUNT_MANAGEMENT.md — Retention and ~3k target
- PRE_CHANGE_CHECKLIST.md — Hot path, cache, concurrency
- `docs/architecture/_internal/configs/high_volume_kinds.yaml` — Declarative list
- `pkg/metrics/timeseries.go` — Timeseries prototype
- `pkg/storage/stream_config.go`, `stream_storage.go`, `high_volume_event_cache.go`
