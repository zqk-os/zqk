# WAL-Backed Bulk Transactions Design

**Goal:** Make bulk operations (create, update, delete) transactional at the WAL level: one logical bulk = one durable unit, with reduced WAL/buffer volume and clear replay semantics. Bulk update must support two modes: **patch** (merge; only supplied fields overwrite) and **post** (replace; payload can set unspecified fields to null/empty per spec).

**Current state:** All three bulks use a transaction today:

- **BulkDelete / BulkDeleteOptimized:** `BeginTransaction()` → N × `tx.Delete()` → `tx.Commit()`. On commit, `FileObjectTransaction.Commit()` calls `storage.Delete()` **per** op → N WAL records and N buffer entries.
- **BulkCreate:** `BeginTransaction()` → N × `tx.Create()` → `tx.Commit()`. On commit, each create is applied via `storage.Create()` → N WAL records and N buffer entries.
- **BulkUpdate:** `BeginTransaction()` → N × `tx.Update(id, updates)` → `tx.Commit()`. On commit, each update is applied via `storage.Update()` → N WAL records and N buffer entries.

So in all cases we get **N ops ⇒ N WAL records and N buffer entries**. There is no single "bulk" boundary in the WAL.

**Gaps:**

- No single durable bulk unit: crash mid-bulk leaves partial apply and no WAL-level rollback.
- High WAL/buffer volume and worker round-trips for large bulks.
- appliedSeq advances per op, not per bulk.
- Bulk update has no explicit **patch vs post** mode: today's `Update()` is merge-only (patch). We need **post** (full replace) for cases where the payload is the full document and omitted fields may be set to null/empty per spec.

---

## Update modes (patch vs post)

- **Patch (merge):** Only fields present in the payload are applied. Omitted fields are left unchanged. If a field *is* supplied, it overwrites (including with empty string or null when the spec allows). This is the current `Update()` behavior.
- **Post (replace):** The payload is treated as the full replacement. Fields not in the payload are set to null or empty according to the spec (or default). Use when the client sends the complete object and expects unspecified fields to be cleared.

**API surface:** Extend `BulkUpdateItem` to select mode:

- Add `Replace bool` to `BulkUpdateItem`: `false` = patch (default), `true` = post. This allows mixed patch/post in one bulk; default `false` preserves backward compatibility.

---

## Option A: Single WAL record per bulk (recommended)

One WAL record represents the entire bulk operation. Replay applies the whole bulk or skips it (no partial apply).

### 1. WAL record types and payloads

| op | Payload (JSON in DataB64) | Worker behavior |
|----|---------------------------|-----------------|
| `bulk_delete` | `[{"kind":"scheduler_job","id":"SCH-1"},...]` | For each entry call `applyDeleteFromBuffer(id, kind)`. |
| `bulk_create` | `[{"kind":"...","id":"...","...":...},...]` — full object per item | For each object call `applyCreateFromBuffer(id, kind, marshalledYAML)`. |
| `bulk_update` | `[{"id":"...","updates":{...},"replace":false},...]` — id + updates map + replace flag | For each item: if `replace` then apply as full replace (post); else merge into existing (patch). Call `applyUpdateFromBuffer` or new `applyUpdateFromBufferWithMode(id, kind, data, replace)`. |

For very large bulks, cap payload size and use multiple WAL records (chunking) to stay under line-size limits.

### 2. Bulk delete (unchanged from earlier design)

- **Record:** `op = "bulk_delete"`, payload = list of `{kind, id}`.
- **Buffer:** One `PendingOp` with `Op = "bulk_delete"`, `Data` = same JSON.
- **Worker:** Decode list, for each `(kind, id)` call `applyDeleteFromBuffer(ctx, id, kind, secCtx)`.

### 3. Bulk create

- **Record:** `op = "bulk_create"`, payload = list of full objects (each with `kind`, `id`, and all other fields). Serialize as JSON array; for YAML fidelity we can store per-object YAML in a subfield or use a single JSON representation that the worker marshals back to YAML per create.
- **Buffer:** One `PendingOp` with `Op = "bulk_create"`, `Data` = same payload.
- **Worker:** Decode list, for each object call existing create apply logic (e.g. marshal to YAML and `applyCreateFromBuffer(ctx, id, kind, yamlBytes, secCtx)`). Apply all before advancing appliedSeq.

### 4. Bulk update (with patch vs post)

- **Record:** `op = "bulk_update"`, payload = list of `{id, updates, replace}`. `updates` is the map of field changes; `replace` is boolean (patch vs post).
- **Buffer:** One `PendingOp` with `Op = "bulk_update"`, `Data` = same JSON.
- **Worker:** Decode list. For each item:
  - **Patch (`replace: false`):** Read current object, merge `updates` into it (existing behavior), write back → equivalent to current `applyUpdateFromBuffer` with merged body.
  - **Post (`replace: true`):** Treat `updates` as the full document (or payload = full object); write after applying spec defaults/null for omitted fields. Requires a new apply path that does replace semantics (e.g. `applyUpdateReplaceFromBuffer`) so unspecified fields are set per spec.

**BulkUpdateItem extension:**

```go
type BulkUpdateItem struct {
    ID      string
    Updates map[string]any
    Replace bool   // false = patch (default), true = post (full replace)
}
```

Single-object `Update()` can gain an optional parameter or context hint for replace mode when we add the apply path; bulk path passes `Replace` through the WAL payload.

### 5. Commit path (transaction-aware)

- **Delete-only transaction:** Build list of `(kind, id)` from `tx.ops`, append one `bulk_delete` WAL record, enqueue one buffer op, mark committed.
- **Create-only transaction:** Build list of full objects from `tx.ops`, append one `bulk_create` WAL record, enqueue one buffer op, mark committed.
- **Update-only transaction:** Build list of `{id, updates, replace}` from `tx.ops`. Today `tx.Update` only stores `id` and `updates`; we need to add `Replace` to the transaction op when we add the field to `BulkUpdateItem` and pass it through. Then append one `bulk_update` WAL record, enqueue one buffer op, mark committed.
- **Mixed transaction:** If the transaction contains more than one op type (create + delete, etc.), keep current behavior: apply each op individually via `storage.Create/Update/Delete` (N WAL records). Optionally in a later phase we could support multiple bulk records in sequence (e.g. one bulk_delete then one bulk_create) for the same commit.

### 6. Replay

- Replay callback sees `op` and payload. For `bulk_delete`, `bulk_create`, `bulk_update`, enqueue one buffer op with the same `Op` and `Data`. Worker applies the whole bulk when it processes that op.

### 7. Backward compatibility

- Existing WAL records remain `create`/`update`/`delete`. New bulk record types are additive. Default `Replace == false` for bulk update keeps patch behavior.

---

## Option B: Bulk boundary markers in WAL

- Append special records: `begin_bulk` (e.g. `bulk_id`, `count`) and `end_bulk` (`bulk_id`).
- Between them, append the usual N single-op records.
- Worker: on `begin_bulk`, set "in bulk" mode; on `end_bulk`, clear it and advance checkpoint. Replay must handle incomplete bulks (redo from `begin_bulk` if no `end_bulk`).

**Downside:** More complex replay and state. Option A is simpler and gives one record = one bulk.

---

## Implementation outline (Option A)

1. **WAL**
   - Use `DataB64` for all bulk payloads (JSON-encoded then base64). Add helpers: `AppendBulkDelete`, `AppendBulkCreate`, `AppendBulkUpdate`.
   - Chunking: if payload size exceeds a threshold (e.g. 1 MiB per line), split into multiple bulk records of the same op type (worker applies each chunk; commit still "logically" one bulk from caller's perspective, or we document that one tx commit can produce multiple WAL records when chunked).

2. **Buffer**
   - Reuse `PendingOp`: `Op` = `"bulk_delete"` | `"bulk_create"` | `"bulk_update"`, `Data` = payload bytes (JSON). No change to `Enqueue` signature; callers pass op and data.

3. **Worker**
   - In `apply()`, add cases for `bulk_delete`, `bulk_create`, `bulk_update`. Each decodes the list and loops over the existing apply helpers (or new replace-mode update helper). Apply all before advancing appliedSeq.

4. **Transaction commit (file backend)**
   - In `FileObjectTransaction.Commit()`: detect homogeneous transaction (all create, all update, or all delete). If homogeneous, build bulk payload, append one WAL record, enqueue one buffer op, mark committed. If mixed, keep current per-op apply.

5. **BulkUpdateItem and Update semantics**
   - Add `Replace bool` to `BulkUpdateItem`. When building bulk_update payload from `tx.ops`, include `replace` per item (from a new field on the transaction op, set when `tx.Update` is called with a replace flag; API for `tx.Update` can gain `UpdateWithMode(ctx, secCtx, id, updates, replace bool)` or we carry replace in context for the bulk). Single-object `Update()` remains patch-only unless we add an optional replace parameter; bulk path is the primary consumer of post mode.

6. **Post-mode apply (replace)**
   - Implement `applyUpdateReplaceFromBuffer` (or equivalent): read current object for kind/path, then replace with payload, applying spec defaults or null for omitted fields, then write. This may require spec/validation support to know which fields to set to null when omitted in a replace.

7. **Tests**
   - Unit: append each bulk record type, replay, assert worker applies all.
   - Integration: BulkDelete, BulkCreate, BulkUpdate (patch and post) with WAL enabled → one WAL record per bulk, worker applies correctly. Verify patch does not clear omitted fields and post does clear per spec.

---

## References

- `pkg/storage/object_wal.go` – WAL format and append/replay.
- `pkg/storage/object_write_buffer.go` – buffer enqueue.
- `pkg/storage/object_write_behind_worker.go` – worker apply.
- `pkg/storage/object_storage_file_transaction.go` – transaction commit.
- `pkg/storage/object_storage_file_bulk.go` – BulkCreate, BulkUpdate, BulkDelete (use transaction).
- `pkg/storage/object_storage_interface.go` – BulkUpdateItem.
- `pkg/storage/object_storage_file_update.go` – Update (merge semantics).
- `pkg/storage/bulk_delete_optimized.go` – BulkDeleteOptimized (uses transaction).
