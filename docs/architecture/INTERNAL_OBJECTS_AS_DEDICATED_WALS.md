# Internal Objects as Dedicated WALs

**Status:** Design proposal  
**Related:** object WAL (write-behind durability), change_journal_entry_config.md, KIND_BASED_STRATEGIES.md, CAS/hash registry

## Goal

Implement high-volume **internal objects** (e.g. `change_journal_entry`, `audit_event`) as **dedicated append-only WALs** instead of one YAML file per object. Keeps data lean and removes per-record YAML, content-addressable storage, hash registry, and integrity-check overhead. This is a **refined pattern for managing internal objects**: critical/vital data stays under tighter scrutiny and controls, with dedicated tight loops we can adjust by load and volume so object counts don’t grow out of control.

## Principles

- **Critical/vital under tighter controls:** Internal event logs and core metrics use dedicated storage (WAL or metric-specific), tunable retention/aggregation/compaction, and lean format. System performance is not driven by one-off or heavy job/scheduler wrapper operations.
- **Complicated job/scheduler wrapper operations** (customized and temporary configuration, run_wrapper jobs, ad-hoc workflows) stay out of the critical path and don’t impact the performance or growth of core internal data.
- **Metrics stay separate** but the same principles apply: dedicated storage, tight loops, configurable by load/volume. Metrics are not mixed into the same WAL as event logs; they have their own lifecycle and aggregation.

## Current overhead (internal objects as full objects)

Today each internal object is stored like a normal object:

- **One file per record:** e.g. `docs/architecture/change_journal/2006-01/CHA-001.yaml`, `CHA-002.yaml`, …
- **YAML:** Full object marshal (schema_version, kind, all fields, YAML formatting).
- **Content-addressable storage (CAS):** Hash of content, index by hash, optional dedup.
- **Hash registry:** Per-kind registry of filename → hash for integrity (system check, verification).
- **Integrity checking:** Load/spec validation, hash verification, reverse reference index updates.

For event-log style data (append-only, high volume, aggregated/compacted later), this is more than needed and adds write and read cost.

## Proposal: dedicated WAL per internal kind (or per kind+bucket)

- **One append-only log per kind** (or per kind + time bucket, e.g. per month): e.g. `.zqk/wal/change_journal_entry.wal` or `.zqk/wal/change_journal_entry/2006-01.wal`.
- **Format:** JSONL or compact binary. Each line = one record with only the fields required for that kind (e.g. `change_type`, `object_ref`, `diff_summary`, `previous_state`, `created_at`, `created_by`). No YAML, no schema_version envelope, no per-file path.
- **ID/seq:** Either implicit (sequence number in log) or a short id (e.g. CHA-001) stored in the line for reference; no separate file path per id.
- **No per-record hash registry:** The log is append-only. Optional: periodic segment checksum or full-file checksum for integrity if needed.
- **No per-record integrity check on write:** No LoadFields, no spec validation on append (or minimal: required fields only). Read path: scan or index for list/query; aggregation and compaction jobs read the log.
- **Lifecycle/aggregation/compaction unchanged:** Aggregation job still reads “entries” (from the log), marks ranges as aggregated, compaction still produces .cjournal or similar from log segments. Retention/cleanup truncates or archives old segments.

## Benefits

| Aspect | Current (one YAML per object) | Dedicated WAL |
|--------|-------------------------------|---------------|
| **Storage** | Many small files; directory and hash registry overhead | Single (or few) append-only file(s) per kind/bucket |
| **Write path** | Marshal YAML → CAS Create → hash registry update → optional reverse index | Append line to log (and optionally sync) |
| **Payload** | Full object envelope (kind, schema_version, all fields) | Lean record (only needed fields) |
| **Integrity** | Per-file hash, system check, spec validation | Optional segment/file checksum; no per-record hash |
| **List/query** | List directory + read each file (or CAS index) | Scan log or use a separate index (e.g. by time, by object_ref) |

Result: simpler write path, leaner data, less overhead for high-volume internal event data. **Object counts stay under control** because each internal kind has its own dedicated loop (retention, aggregation, compaction) that can be tuned by load and volume.

## Interaction API: same as other persisted objects (no YAML unless requested)

The **interaction API** for WAL-backed internal objects behaves **exactly like other persisted objects** from the caller’s perspective:

- **List, get, query:** Same CLI and programmatic surface (e.g. `zqk internal list change_journal_entry`, list filters, get by id/seq). Returned shape is the same logical “object” (id, kind, fields).
- **Default format is not YAML:** Responses use the same default format as the rest of the system (e.g. JSON for machine consumption, table for human). **YAML is only produced when explicitly requested** (e.g. `--format yaml`). On-disk storage is lean (JSONL or compact); YAML is an output format, not the storage format.
- So: persistence is WAL/lean; the API and output behave like other objects, with YAML only when the user asks for it.

## Scope: which kinds

Candidates for “internal as WAL”:

- **change_journal_entry** – append on object change and health_check; already time-bucketed; aggregation + compaction.
- **audit_event** – append on auditable operations; same pattern.

**Metrics (e.g. scheduler_health_metric, audit_aggregation_metric, base_metric)** stay **separate** from event-log WALs but the **same principles apply**: dedicated storage, tight loops, tunable by load/volume. They may use their own WAL-like or metric-specific storage rather than one-YAML-per-metric; the exact layout is separate from change_journal_entry/audit_event. Keep metrics under the same “critical/vital under tighter controls” idea without mixing them into the same WAL as event logs.

Other internal kinds that are lower volume or need random access by ID and full lifecycle can keep current object storage. **Event-log style, append-only, high-volume internal kinds** → dedicated WAL; **metrics** → separate dedicated path with same principles; **rest** → current storage.

## Format sketch (e.g. change_journal_entry)

Per-line JSON (or compact key-value) with only what’s needed:

```json
{"seq":1,"ts":"2026-02-26T12:00:00Z","ct":"health_check","ref":"health_monitor:scheduler_events","summary":"status=ok ...","prev":{"status":"ok","summary":"..."}}
```

- `seq`: monotonic in file (or global).
- `ts`, `ct`, `ref`, `summary`, `prev`: enough for aggregation, compaction, and list/query. No `id` file path; `seq` or `id` in line if needed for references.

## Migration and compatibility

- **New writes:** New code path “append to kind WAL” instead of `storage.Create(change_journal_entry, ...)`. Call sites (e.g. CreateChangeJournalEntryWithBuilder, CreateHealthCheckChangeJournalEntry) switch to a WAL append API.
- **Read path:** List/query and aggregation currently expect “list objects of kind change_journal_entry”. Either:
  - **Dual read:** Prefer reading from WAL when present; fall back to existing YAML objects for old data, or
  - **One-time migration:** Replay existing YAML objects into the WAL (or leave old data in place and only read new data from WAL), then run retention to drop old YAML.
- **ID generation:** Today change_journal_entry gets CHA-NNN from a per-bucket sequence. With a WAL, “id” can be optional (log position or seq) or generated the same way and stored in the line for compatibility with tools that expect CHA-*.

## Out of scope (for this doc)

- Changing the **object WAL** (write-behind for create/update/delete of normal objects) – that stays as-is.
- Moving **all** internal kinds to WAL – only event-log style, high-volume kinds are in scope.
- Exact on-disk format (JSONL vs binary) and indexing strategy – to be decided in implementation.

## Summary

- **Dedicated WALs for internal event-log objects** (change_journal_entry, audit_event): lean storage, no per-record YAML/CAS/hash registry; aggregation, compaction, and retention stay in dedicated tight loops tunable by load/volume so object counts don’t grow out of control.
- **Interaction API** matches other persisted objects; default output is not YAML; YAML only when `--format yaml` is requested.
- **Metrics** stay separate; same principles (dedicated storage, tight controls) apply to them without sharing the same WAL.
- **Complicated job/scheduler wrapper operations** remain customized/temporary and don’t drive or impact the performance of critical internal data.
- **Critical/vital path** (internal event logs, core metrics) stays under tighter scrutiny and controls.
