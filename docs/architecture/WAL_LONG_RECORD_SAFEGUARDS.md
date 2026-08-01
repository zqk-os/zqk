# WAL Long Record: Causes and Safeguards

## Summary

The object WAL can grow a single line to hundreds of MB if either (1) an object ID is extremely long, or (2) a create/update writes a very large object payload (e.g. a multi‑MB field). Such lines break replay because the reader uses a 2 MiB max line size. This doc describes causes and the safeguards added to prevent recurrence.

## Root Causes

1. **Very long object ID**  
   IDs come from `op.id` on create/update/delete. If a bug or misuse produces an unbounded or corrupted ID (e.g. concatenation in a loop, wrong kind/pattern), that ID is written into the WAL and can make a single record line arbitrarily long (ID appears in the compact format and possibly in payloads).

2. **Very large object payload**  
   Create/update WAL records include full object YAML as base64 (`data_base64`). A single object with a huge field (e.g. a multi‑MB blob) produces one WAL line that can exceed the reader’s 2 MiB limit, so replay fails with “token too long” or similar.

## Likely Origins of a “Really Long” ID (not an internal race)

Internal ID generation (`ensureObjectID`, `generateID`, CAS timestamp+random) always produces **bounded** IDs (e.g. `PREFIX-NNN-...` or sequential). A very long ID is therefore almost certainly from **input**, not from a race or bug inside ID generation.

Plausible sources:

1. **Object YAML file on disk with a malformed `id`**
   - YAML allows multi-line literals. If a file has:
     - `id: |` (literal block) followed by a large paste, or
     - `id: >` (folded block) with many lines,
     then `yaml.Unmarshal` puts that entire string into `obj["id"]`. Any subsequent **Read**, **List**, or **Update** (including bulk update over listed objects) will then use that huge string as the object ID and write it into the WAL.
   - Can happen from: merge conflicts, editor paste into the wrong field, or a script that wrote `id` from the wrong variable (e.g. entire file content or a big blob).

2. **Bulk create or import from a YAML/JSON file**
   - If the input file has one item with `id` set to a multi-line or very long value (e.g. copy-paste error, or a script that set `id` to a description/body field), that object is created with a huge ID and written to the WAL.

3. **Scripts or external tools**
   - Any process that writes object YAML (e.g. under `docs/architecture/`) and sets `id` to something unbounded (e.g. concatenating many IDs, or dumping content into `id`) can produce a long ID. Once that file is read by List/Read or by bulk create/import, the long ID flows into Create/Update and then into the WAL.

**What to check if it happens again:** Look for object YAML files (or bulk input files) where `id` is a multi-line or very long value (e.g. `grep -r 'id: |' docs/architecture/` or inspect the object that corresponds to the WAL record’s kind and approximate creation time). The WAL safeguards (max ID length, max line size) will now reject such writes and report the kind/id so you can locate the source.

## Safeguards (Code)

- **Max object ID length**  
  `pkg/validation` defines `MaxObjectIDLength` (2048). It is enforced by: (1) **IDValidator.ValidateID** so create/update paths reject long IDs at validation time; (2) **base_object** spec `id.validation.max_length: 2048` so spec-driven validation rejects long IDs; (3) **WAL Append/AppendBatch** so any record with `len(rec.ID) > MaxObjectIDLength` is rejected. This prevents runaway or corrupted IDs from creating unbounded WAL lines.

- **Max WAL line size at write**  
  After marshalling a record (or batch) to a line, the code checks `len(line) <= maxWALLineSize` (2 MiB). If the line would exceed that, append returns an error instead of writing. That way we never write a line the reader cannot read, and large payloads are rejected at write time with a clear error (e.g. “object payload may be too large” or “reduce batch size or object payload size”).

## References

- WAL format and reader: `pkg/storage/object_wal.go` (maxWALLineSize, Append, AppendBatch), `wal_compact.go`.
- Where WAL records get ID and payload: `pkg/storage/object_storage_file_transaction.go` (prepareBatchWALRecords).
