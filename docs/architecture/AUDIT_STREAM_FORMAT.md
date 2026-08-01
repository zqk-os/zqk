# Audit Stream File Format

**Status:** Design / implementation  
**Purpose:** Define the on-disk format for audit streams (multiple events per file, append-only) so we can reduce per-event CAS overhead. See [BYPASS_KIND_STORAGE.md](./BYPASS_KIND_STORAGE.md) and [INTERNAL_OBJECTS_AS_DEDICATED_WALS.md](./INTERNAL_OBJECTS_AS_DEDICATED_WALS.md).

---

## Location

- **Directory:** Resolved via the stream path cache (built at scheduler pre-warm); default is `<project_root>/.zqk/streams/audit_event/` (grouped with other stream-backed kinds under `.zqk/streams/`). See `pkg/storage/stream_path_resolver.go` and STREAM_STORAGE.md §5.
- **Files:** One file per time window. Window is **daily** (UTC): `audit_stream_YYYY-MM-DD.jsonl`
- Example: `audit_stream_2026-03-01.jsonl` contains all events for that day (UTC).

**Migration from legacy path:** If you have existing data under `.zqk/audit_streams/`, move the directory so new path is used: `mv .zqk/audit_streams .zqk/streams/audit_event` (create `.zqk/streams` first if needed). New writes go to `.zqk/streams/audit_event/` only.

---

## Format: JSONL (newline-delimited JSON)

Each line is a single JSON object: one audit event. Fields match the logical audit_event object (id, event_type, operation, severity, created_at, created_by, target_kind, target_id, etc.) as produced by the audit event builder. No YAML; no envelope beyond the single line.

- **Encoding:** UTF-8
- **Line terminator:** `\n`
- **Ordering:** Append-only; order of lines is order of creation within the day.
- **Compression:** Not in v1. Optional later: whole file gzip or per-block compression (see BYPASS_KIND_STORAGE).

---

## Write path

- **Append:** Open file for append (create if not exists), write one JSON line, flush/sync. Use a single mutex or file lock per stream file to allow concurrent appends from multiple goroutines without corrupting the file.
- **Window rollover:** At midnight UTC, the next event goes to the new day’s file. No need to “close” the previous file; next day’s filename is different.

---

## Read path (future)

- **By time range:** Open the relevant day files, read line-by-line, parse JSON, filter by `created_at`.
- **By ID:** Either scan (v1) or maintain a small index (event_id → file + offset) later. Not required for the initial write-only slice.

---

## Stream-only write (current)

- **Enablement:** `ZQK_STREAM_STORAGE_ENABLED=true` turns on **stream-only** write for `audit_event` (and `change_journal_entry`). Create writes directly to the daily JSONL segment; no CAS, no dual-write.
- **Index:** High-volume event cache is updated on append (segmentPath::offset); retention uses the same Count/OldestIDs/IDsOlderThan contract. See [STREAM_STORAGE.md](./STREAM_STORAGE.md).

---

## References

- [BYPASS_KIND_STORAGE.md](./BYPASS_KIND_STORAGE.md) – Audit streams (multiple events per file, compressed)
- [INTERNAL_OBJECTS_AS_DEDICATED_WALS.md](./INTERNAL_OBJECTS_AS_DEDICATED_WALS.md) – Dedicated WALs for internal objects
- `pkg/storage/audit_events_helper.go` – CreateAuditEventWithBuilder (current create path)
- `pkg/storage/audit_stream.go` – Legacy dual-write append (optional)
- `pkg/storage/stream_storage.go` – Generic stream append (segment + offset); used when ZQK_STREAM_STORAGE_ENABLED=true
- [STREAM_STORAGE.md](./STREAM_STORAGE.md) – Stream infrastructure and change-journal delta pattern
