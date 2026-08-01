# WAL and State Files: Naming and Cleanup

**Purpose:** Single reference for all write-ahead logs and key state files under `.zqk/`: what they are, how they are named, and how to keep them from growing without bound.

---

## 1. WAL files (`.zqk/wal/`)

**Convention:** All WAL files use the same pattern: **`<name>.wal`** for the log and **`<name>.wal.checkpoint`** for the checkpoint.

| File (on disk) | Checkpoint | Purpose |
|----------------|------------|---------|
| **object.wal** | object.wal.checkpoint | Object write-behind: create/update/delete records before background persist. Applied by write-behind worker; checkpoint = last applied seq. |
| **maintenance.wal** | maintenance.wal.checkpoint | Maintenance cycle requests (one JSON line per `cycle_requested`). Processed by maintenance runner (aggregate then retention); checkpoint = last processed seq. |
| **lifecycle_events.wal** | lifecycle_events.wal.checkpoint | Lifecycle events: status transitions and criterion_satisfied. Processed by lifecycle listener for transition rules; checkpoint = last processed seq. |

### Migration from legacy names

- **object_wal** / **object_wal.checkpoint** or **object_wal.wal** / **object_wal.wal.checkpoint** → **object.wal** / **object.wal.checkpoint**. Migration runs automatically on first open (NewObjectWAL, ReadAppliedSeq, CompactWAL, or ReplayWALChunk).
- **lifecycle_events** / **lifecycle_events.checkpoint** → **lifecycle_events.wal** / **lifecycle_events.wal.checkpoint**. Migration runs automatically when the lifecycle WAL is opened (NewLifecycleEventWAL).
- **maintenance.wal** has always used the canonical name; no migration.

### What lifecycle_events.wal is for

The **lifecycle event WAL** (`lifecycle_events.wal`) is used by the **lifecycle listener** to drive **transition criteria**: status changes and “criterion satisfied” events are appended here, then replayed in order so the listener can update an in-memory “satisfied set” and fire **auto transitions** (e.g. priority_plan active→complete when all backlog items for the plan are complete). See **`docs/architecture/LIFECYCLE_EVENT_LISTENER_AND_CRITERIA.md`**.

---

## 2. Routine compaction and checkpoint behavior

### Object WAL

- **Checkpoint:** Advanced by the write-behind worker as it applies entries.
- **Compaction:** `zqk system compact-wal` (PRUNED) removes entries with seq ≤ checkpoint. Run when WAL is large; safe to run while the system is running (see object_wal implementation).

### Maintenance WAL

- **Checkpoint:** Advanced by the maintenance runner **after each successful cycle** (so a single failing cycle does not prevent the checkpoint file from being created and the WAL from being advanced).
- **Compaction:** `zqk system compact-maintenance-wal` removes entries with seq ≤ checkpoint. **Run when the scheduler daemon is stopped** so no process has the file open for append. Run periodically (e.g. when the file has grown) so `maintenance.wal` does not grow indefinitely.

### Lifecycle events WAL

- **Checkpoint:** Advanced by the lifecycle listener after processing.
- **Compaction:** Not yet implemented; file can grow. A future improvement is to add compaction (same pattern: keep only seq > checkpoint) and run when the daemon is stopped or with care.

---

## 3. Stream state files (`.zqk/state/`)

| File pattern | Purpose |
|--------------|---------|
| **stream_registry_&lt;kind&gt;.jsonl** | Append-only id→loc for stream-backed kinds. Grows on every create; entries are not removed on delete (see stream_deleted). |
| **stream_deleted_&lt;kind&gt;.jsonl** | One soft-deleted ID per line. Grows on every stream-backed delete. |

**Compaction:** The **maintenance runner** runs `CompactStreamRegistryForKind` **after each successful retention cycle** for **all stream-backed kinds** (audit_event, change_journal_entry, scheduler_job, metrics, etc.), so registry files are trimmed automatically and do not grow unbounded. For immediate relief on a large registry, run **`zqk system compact-stream-state --kind (PRUNED) <kind>`** (e.g. `--kind audit_event`, `--kind change_journal_entry`). See **STREAM_STORAGE.md §8**.

---

## 4. Summary: don’t create a mess without a plan

- **Maintenance WAL:** Checkpoint is written after each successful cycle. The scheduler **compacts at daemon startup** (before opening the WAL), so applied entries are removed on each restart. You can also run `zqk system compact-maintenance-wal` when the daemon is stopped for an extra trim.
- **Stream state:** Maintenance compacts all stream-backed kinds after each retention cycle. For immediate relief, use `compact-stream-state --kind <kind>` (e.g. audit_event, change_journal_entry).
- **Object WAL:** Use `compact-wal` when the WAL is large; checkpoint is advanced by the write-behind worker.
- **Lifecycle events:** Documented here; compaction to be added later.

All of these have a defined cleanup path so we do not grow state indefinitely without a way to compact.
