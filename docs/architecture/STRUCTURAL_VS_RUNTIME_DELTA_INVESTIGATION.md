# Structural vs Runtime-Delta and Stream Storage: Implementation Investigation

**Last Verified:** 2026-08-31


**Purpose:** Verify that high-volume stream storage and the structural vs runtime_delta (change journal) model are fully implemented and identify gaps or inefficiencies.

**References:** PRE_CHANGE_CHECKLIST.md §4, DATA_STORAGE_PRODUCTION_ROADMAP.md, stream_delta_config.go, object_storage_file_update.go, change_journal.go.

---

## 1. Evidence of What Was Implemented

### 1.1 Stream storage (lighter-weight than per-object YAML)

- **Create path:** When `StreamStorageEnabledForKind(kind)` is true, `writeObjectToStorage` → `writeObjectToStream`: appends to segment JSONL (e.g. `.zqk/streams/audit_event/audit_stream_YYYY-MM-DD.jsonl`, `.zqk/streams/<kind>/`), updates stream location registry and high-volume cache. No CAS hash files, no hash registry. (**object_storage_file_create.go**)
- **Stream record shape:** `recordForStream()` uses `getStreamDeltaFieldsForKind()` so only configured/runtime_delta fields are persisted per record; timestamps encoded as Unix seconds. (**stream_storage.go**, **stream_delta_config.go**)
- **Read path:** For stream-backed objects, `getObjectFilePath` returns `segmentPath::offset` from registry; `Read` uses `ReadRecordAt(segmentPath, offset)`. (**object_storage_file_read.go**, **object_storage_file.go**)
- **Config/spec:** `stream_delta_fields.yaml` and `getRuntimeDeltaFieldsFromSpec()` merge spec `storage_role: runtime_delta` into the field list for change_journal_entry and metric kinds. (**stream_delta_config.go**)

### 1.2 Structural vs runtime_delta in specs

- **Spec annotations:** `auditable.yaml`, `base_object.yaml`, `base_metric.yaml` define `storage_role: runtime_delta` on fields such as `updated_at`, `updated_by`, `status`, and metric runtime fields. (**.zqk/specs/objects/**)
- **Checklist:** PRE_CHANGE_CHECKLIST.md §4 states: structural = CAS/YAML updates; runtime_delta = deltas (change journal / stream) instead of full CAS rewrites; for kinds with `storage.runtime_deltas.enabled: true`, route only runtime_delta through delta mechanism and avoid CAS when no structural change.

### 1.3 Change journal (audit trail and rollback)

- **Creation:** `createChangeJournalEntry()` is invoked from the **file-based** Update path (and from `applyUpdateFromBuffer`) with previous state and updates; it creates a change_journal_entry for rollback and auditing. (**change_journal.go**, **object_storage_file_update.go**)
- **Reconstruction:** `ReconstructStateAtTimestamp()` walks change journal entries after a snapshot time and applies reverse changes to obtain object state at that time. (**change_journal_reconstruction.go**)
- **Usage:** Snapshot/rollback and audit trail; not used for “current” read (see gaps).

---

## 2. Gaps and Inefficiencies

### 2.1 Update always does full CAS rewrite (no “delta-only” path)

- **Update (2026):** For kinds in `runtime_delta_kinds.yaml`, when **every** key in `updates` is listed as `checklist.storage_role: runtime_delta` on the resolved object spec, `Update` writes `.zqk/state/runtime_delta_current/...` and **skips** `cas.Update` (see **object_storage_file_update.go** `runtimeDeltaOnly`). The historical gap below applies when that condition is **not** met or for kinds not in `runtime_delta_kinds.yaml`.

- **Earlier characterization:** For any Update on a CAS-backed kind, the code merges `updates` into `existing`, validates, then marshals the full object and calls `cas.Update(id, data)`. There was **no** branch that:
  - Classifies the keys in `updates` as structural vs runtime_delta (e.g. via spec or a shared helper).
  - If **only** runtime_delta fields changed: write only a change journal entry (or append a delta record), and **skip** `cas.Update()` and hash registry update.
  - If **any** structural field changed: perform full CAS update (current behavior).
- **Impact:** Every update (including status-only or timestamp-only) triggers a full CAS write (new hash file, index update, old file delete, hash registry update). High-churn objects (e.g. scheduler_job, metrics) pay full CAS cost on every update even when only runtime_delta fields change.
- **Location:** **object_storage_file_update.go** – main Update body (e.g. before the “Check if this kind uses content-addressable storage” block around line 239).

### 2.2 No kind-level “runtime_deltas.enabled” and no read path that merges deltas

- **Checklist** refers to “kinds with `storage.runtime_deltas.enabled: true`”. There is **no** such flag in object specs or in code; no loader or branch checks it.
- **Intended model** (from guided-spec-management and checklist): “runtime_delta = stored via deltas instead of full CAS rewrites.” That implies a read path that merges a “base” (structural) representation with the latest deltas. **Current Read** returns either the full object from CAS or the full record from stream; it does **not** merge “base object + latest change journal deltas” for current state. So the “store only deltas for runtime_delta” model is only partially present: we have change journal for history and rollback, but not for “current state = base + deltas.”

### 2.3 CAS Update path does not create a change journal entry

- **Observation:** `createChangeJournalEntry()` is called in the file-write Update paths (e.g. around lines 535 and 619) and in `applyUpdateFromBuffer` (729). The **main CAS-only Update path** (lines 239–301) does **not** call `createChangeJournalEntry()`.
- **Impact:** Updates that go through the CAS path (e.g. most CAS-backed kinds) do not get a change journal entry; rollback and audit trail are incomplete for those updates.
- **Fix:** Call `createChangeJournalEntry(..., previousStateForJournal, updates, secCtx)` in the CAS Update path before returning (with appropriate file path for journal metadata).

### 2.4 Stream-backed objects: Update creates a CAS file instead of updating in stream

- **Current behavior:** For kinds that use stream storage (e.g. scheduler_job, metrics), Create uses `writeObjectToStream()` and does **not** add the object to the CAS index. On the first **Update**, the code still calls `cas.Update(id, data)`. CAS finds the object “not in index” and treats it as a new object: it writes a **new hash-named file** and adds the mapping to the CAS index.
- **Impact:** The original record remains in the stream segment (orphaned); the “current” version lives in a CAS file. So we get mixed storage (stream + CAS) for the same logical object and no single “update in place” in the stream (e.g. append-only delta record that Read could merge).
- **Design choice:** Either (1) define an explicit “stream update” path (e.g. append an update delta to the stream and have Read merge base + deltas), or (2) document that the first Update of a stream-backed object “promotes” it to CAS and accept the hybrid. Right now this is implicit and can surprise operators.

### 2.5 getStreamDeltaFieldsForKind only used for stream record shape, not for Update classification

- **Current use:** `getStreamDeltaFieldsForKind()` (and spec `storage_role: runtime_delta`) is used in `recordForStream()` to decide which fields to write into stream segments. It is **not** used in the Update path to decide “only these fields changed → delta-only, skip CAS.”
- **Gap:** The same notion of “runtime_delta” could drive both (a) stream record shape and (b) Update’s structural-vs-delta decision. Today (b) is missing.

---

## 3. Recommendations (in order of impact)

1. **Add “only runtime_delta changed” fast path in Update (CAS-backed, non–stream-only kinds)**  
   - Before calling `cas.Update()`:
     - Get the set of runtime_delta field names for the kind (reuse or mirror `getStreamDeltaFieldsForKind` / `getRuntimeDeltaFieldsFromSpec`; consider a shared helper that returns “structural vs runtime_delta” sets).
     - If every key in `updates` is in the runtime_delta set (and no structural field changed):
       - Create change journal entry (and audit event) with the updates and previous state.
       - Skip `cas.Update()`, hash registry update, and index update; invalidate list cache and run notifications.
     - Otherwise, keep current full CAS update.
   - Ensures we avoid CAS rewrite when only high-churn fields change, and aligns with PRE_CHANGE_CHECKLIST §4.

2. **Create change journal entry on the CAS Update path**  
   - In the block that performs `cas.Update()` (and before return), call `createChangeJournalEntry(f.projectRoot, id, kind, filePath, OpUpdate, previousStateForJournal, updates, secCtx)` so every update is auditable and reversible regardless of path.

3. **Define stream-backed Update behavior explicitly**  
   - Option A: For stream-backed kinds, implement “update = append delta record to stream” and a Read path that resolves “current” by merging the base record (at registered offset) with later delta records.  
   - Option B: Document that the first Update of a stream-backed object promotes it to CAS (current behavior) and ensure registry/index are updated so List/Read see a single source of truth (e.g. remove or hide the stream record once promoted).  
   - Choose one and implement or document it consistently.

4. **Optional: kind-level `storage.runtime_deltas.enabled`**  
   - If we want to gate the “delta-only” Update behavior by kind, add a spec or config flag (e.g. `storage.runtime_deltas.enabled: true`) and use it so that only kinds that opt in use the “only runtime_delta → skip CAS” path. This avoids surprising behavior for kinds that do not yet support it.

5. **Reuse runtime_delta notion for Update**  
   - Use the same source of truth (spec `storage_role` + config) for both stream record shape and Update’s “structural vs runtime_delta” check, to keep behavior and config aligned.

### Implemented in this pass

- **Change journal on CAS Update path:** The main CAS Update branch now calls `createChangeJournalEntry()` so every update (including CAS path) is recorded for audit and rollback. See **object_storage_file_update.go**.

---

## 4. Summary Table

| Area                         | Implemented | Gap / Note |
|-----------------------------|------------|------------|
| Stream Create (append-only) | Yes        | —          |
| Stream record = delta fields| Yes        | recordForStream + getStreamDeltaFieldsForKind |
| Spec storage_role           | Yes        | auditable, base_object, base_metric |
| Change journal on Update    | Partial    | Only file path and applyUpdateFromBuffer; CAS path does not create entry |
| Update: only runtime_delta   | Partial    | **CAS kinds in `runtime_delta_kinds.yaml`:** `updateIsRuntimeDeltaOnly` + `WriteRuntimeDeltaCurrentState` skips `cas.Update` when updates are only spec-annotated runtime_delta fields (**object_storage_file_update.go**). Otherwise full CAS. |
| Read = base + deltas        | Partial    | **scheduler_job (and other CAS + overlay):** read/list merges `runtime_delta_current` overlay onto structural CAS payload (**applyRuntimeDeltaOverlay**). Stream-backed kinds: stream record, not this overlay. |
| runtime_deltas.enabled      | No         | Kind-level flag still absent; use `runtime_delta_kinds.yaml` + per-field `storage_role` |
| Stream-backed Update        | Implicit   | First Update creates CAS file; stream record orphaned |
