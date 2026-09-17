# Change Journal Compaction and Compression

## Current state

- Each update creates a **change journal entry** with `diff_summary`, `previous_state`, and (as of this feature) **`changed_paths`**: a flattened list of dotted paths (e.g. `["meta.tags", "meta.version", "status"]`).
- Entries are stored per month under `.zqk/process/change_journal/YYYY-MM/`.
- **Aggregation** (e.g. `ChangeJournalAggregationService`) rolls up entries into metrics and can **archive or delete** processed entries; it does not today rewrite entry content.

## Why compress during compaction?

- **`changed_paths`** repeats the same path strings across many entries (e.g. `status`, `meta.tags`, `updated_at`). Over a window of thousands of entries, a small dictionary can map those strings to short codes and cut payload size.
- **Entry IDs** (CHA-001, CHA-002, …) can be **range-compressed** like audit event IDs: e.g. `CHA-1..CHA-960` instead of 960 strings (see `audit_aggregation.compressEventIDs` and `ExpandIDRanges`).
- **Optional**: storing the **updates payload** (or a compact diff) per entry would allow “what changed at path X” queries and rollback of single paths; that payload also repeats field names and values and is a good candidate for dictionary encoding.

## Compression patterns we already have

1. **Audit aggregation** (`pkg/storage/audit_aggregation.go`):
   - **`compressEventIDs`**: consecutive IDs → ranges (`AUD-1..AUD-960`). Expand with `ExpandIDRanges`.
   - Use the same idea for change journal entry IDs when recording “which entries were compacted” or when merging aggregation results.

2. **Compressed snapshots** (`pkg/storage/compressed_snapshot.go`):
   - **Dictionary**: `SnapshotDictionary` maps field names and common values to integer IDs; built from a batch of objects with `DictionaryBuilder.AnalyzeObject` / `BuildDictionary`.
   - **Encode**: `CompressObjects` / `compressObject` replace field names with `f{id}` and repeated values with `v{id}` (and optional pattern refs `p{id}:value`).
   - **Decode**: `ExpandObjects` / `expandObject` reverse the mapping using the same dictionary.
   - **Format**: Header + dictionary + compressed data; checksum; optional `.csnap` file.

## Compaction design (future)

1. **Input**: A time window of change journal entries (e.g. a day or a month), either from the aggregation query or a dedicated compaction job.

2. **Flattened format per entry**: Treat each entry as a small “object” for compression:
   - `object_ref`, `change_type`, `created_at`, `created_by`
   - `changed_paths`: already a list of strings (ideal for dictionary: paths like `meta.tags`, `status` repeat a lot)
   - Optionally `updates_payload`: the actual `map[string]any` updates (also highly repetitive in field names and often in values).

3. **Build dictionary** over the batch:
   - Run `DictionaryBuilder.AnalyzeObject` over each “entry object” (or over a representative subset). Path strings and common values (e.g. `active`, `completed`, kind names) get high frequency and low IDs.
   - `BuildDictionary()` produces the same structure as compressed snapshots (field names + values + optional patterns).

4. **Compress the batch**:
   - Convert each entry to a map (e.g. `object_ref`, `change_type`, `changed_paths`, …).
   - `CompressObjects(entriesAsMaps, dict)` → same machinery as `compressed_snapshot.go`. Paths and repeated values become short codes.

5. **Store compacted artifact**:
   - One compressed blob per window (e.g. `change_journal/2026-02/compacted-2026-02-01.csnap` or a dedicated “compacted_journal” kind).
   - Header + dictionary + compressed entries. Optionally record entry ID range (`CHA-001..CHA-100`) for that window.

6. **After compaction**:
   - Archive or delete the original entries for that window (same as today’s aggregation cleanup).
   - Queries that need “entries in window W” can read the compacted blob and expand (or query by ID range if we only need metadata).

## Why the stream file (e.g. `2026-03-14_stream.json`) gets huge

- **Stream storage** for `change_journal_entry` is append-only: one JSONL line per entry in `.zqk/streams/change_journal_entry/YYYY-MM-DD_stream.json`. "Delete" is **soft**: IDs are added to `stream_deleted_change_journal_entry.jsonl`; the segment file is **not** rewritten, so its size only grows.
- **Maintenance** (change_journal_aggregation job, SCH-003) aggregates entries in a time window (e.g. 15m) into metrics and deletes those entries (soft-delete). It also had a step that only deleted entries with `status=aggregated` and `created_at` older than retention. With a **short aggregation window** (e.g. 15m), entries older than 15m were never in the window, so they were never aggregated and never qualified for that cleanup — they accumulated and the stream grew unbounded.
- **Fix (retention by age):** The job now also runs **retention by age**: it queries entries with `created_at` older than `RETENTION_DURATION` (any status) and deletes them (soft-delete), in batches per run. So logical growth is bounded; List/Count exclude deleted IDs. The **physical segment file** still does not shrink (we do not rewrite it); only a future **segment compaction** (rewrite file excluding deleted IDs) would reduce on-disk size.

## Summary

- **Flattened `changed_paths`** is already a good fit for dictionary compression because the same path strings repeat across many entries.
- We can reuse **compressed snapshot** (dictionary + compress/expand) for a batch of entries during compaction, and **audit-style ID range compression** when we store or merge references to entry IDs.
- Implementing compaction would add a step (either inside aggregation or a separate job) that: builds a dictionary over a window of entries, compresses them with `CompressObjects`, writes a `.csnap`-like artifact, then archives/deletes the originals.
- **Segment compaction** (rewriting `YYYY-MM-DD_stream.json` to drop lines whose IDs are in `stream_deleted_*`) would reduce on-disk size and is future work.
