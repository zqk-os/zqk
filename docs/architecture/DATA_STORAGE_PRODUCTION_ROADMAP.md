# Data Storage Production-Readiness Roadmap

**Last Verified:** 2026-08-31


**Purpose:** Track follow-ups so high-volume storage stays optimized and flexible as we bring the implementation to production-ready state. Avoid creating bigger messes by optimizing first, then compressing.

**References:** STREAM_STORAGE.md, HIGH_VOLUME_STORAGE_DEPRECATION.md, FAST_PATH_AND_CACHE_FALLBACK_CHECKLIST.md, `pkg/metrics/timeseries.go`.

---

## 1. High-volume fast path (optimize first) ✅

- **Goal:** Ensure list/count and retention use the high-volume event cache and stream index for all high-volume kinds, with correct fallback when cache is empty or stale (per FAST_PATH_AND_CACHE_FALLBACK_CHECKLIST.md).
- **Current:** List with `created_at` range + sort by `created_at` uses high-volume cache for **all** high-volume kinds when cache is populated; falls through to full path when cache returns 0 or too few. Count uses cache when populated; staleness check and fallback exist.
- **Done:** When high-volume cache is empty, Count() for stream-enabled kinds now adds `CountStreamSegmentLines(projectRoot, kind)` (line count in segment JSONL files, no JSON parse) so stream-only or hybrid kinds get accurate count without unbounded full scans. See `stream_storage.go` (CountStreamSegmentLines, getStreamSegmentDir) and `object_storage_file_list_query.go` (countFromCASIndex and fast-path early returns).

---

## 2. Minimize bytes per event (timestamps) ✅

- **Goal:** Reduce bytes written per stream record so we don’t grow segment files unnecessarily.
- **Issue:** Timestamps in JSONL are large (e.g. RFC3339 `created_at` ~24+ bytes per field; multiple timestamps per record add up).
- **Options when moving to production-ready streams:**
  - **Chunk-base + delta time:** Same idea as `pkg/metrics/timeseries.go` — one base timestamp per chunk or per segment, then store deltas (e.g. varint seconds or milliseconds from base). Decode on read.
  - **Epoch seconds/millis:** Store `created_at` as a number instead of ISO string; smaller and still sortable.
  - **Dictionary or schema-driven encoding:** For repeated values (e.g. kind, event_type), use short codes or a small dictionary in the segment header.
- **Do not forget:** As we mature the implementation, add compact timestamp (and optional value) encoding so segment files stay small and I/O stays bounded.
- **Done:** Stream records encode `created_at`, `window_start`, `window_end`, `last_seen` as Unix seconds (int) in JSONL; `ReadRecordAt` decodes back to RFC3339 for downstream. Backward compatible. See `stream_storage.go` (encodeStreamRecordTimestamps / decodeStreamRecordTimestamps).

---

## 3. Timeseries prototype → production ✅

- **Goal:** Wire the timeseries prototype into production paths so numeric series (object_volume, health gauges) use chunked, base+delta encoded storage instead of ad-hoc or unbounded formats.
- **Current:** `pkg/metrics/timeseries.go` implements `TimeSeriesWriter` / `TimeSeriesReader` with delta time (varint seconds from chunk base) and delta value (zigzag varint).
- **Done:** object-count-report calls `recordObjectVolumeMetrics` (cmd/zqk/system/object_count_report.go), writing one sample per kind to `.zqk/metrics/object_volume/`; `pruneObjectVolumeChunks` removes chunk files older than `metricsChunkRetentionDays`. Health gauges can be wired similarly when needed.

---

## 4. Externalized stream delta fields ✅

- **Goal:** Replace hardcoded `changeJournalDeltaFields` and `metricDeltaFields` in `pkg/storage/stream_storage.go` with config/spec so new kinds and field changes don’t require code edits.
- **Done:** Config at `.zqk/specs/configs/stream_delta_fields.yaml`; loader in `pkg/storage/stream_delta_config.go`. Keys: `change_journal_entry`, `metric`. Override path via `ZQK_STREAM_DELTA_FIELDS_CONFIG`. Built-in defaults when config missing or invalid. **Spec alignment:** Base list is config or built-in; any field with `storage_role: runtime_delta` from the kind's spec (ontology inheritance) is merged in via `getRuntimeDeltaFieldsFromSpec`, so new runtime_delta fields are included without editing the config.

---

## 5. WAL shrink: minimal records for stream-only kinds ✅

- **Goal:** Shrink WAL activity and bytes by writing minimal records (op, kind, id, seq) for stream-only kinds instead of full payload.
- **Done:** (1) `AppendToWALAndBuffer` and `AppendBatchToWALAndBuffer`: for kinds with `StreamStorageEnabledForKind`, WAL record omits `data_base64` (minimal record); full data is still enqueued to the buffer so the worker persists it. (2) Replay: when record has no payload and kind is stream-only, `applyCreateFromBuffer` / `applyUpdateFromBuffer` skip apply (best-effort: assume already persisted to stream) so replay advances. (3) Future: stream ReadByID or segment scan by id would allow re-applying from stream when payload missing; until then skip is best-effort.
- **Rollout done:** Change journal ID generation uses sequence file under `.zqk/state` when stream is enabled. Stream config includes all high-volume kinds from `high_volume_kinds.yaml` (including `mcp_session`).
