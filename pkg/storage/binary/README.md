# Positional Binary Storage (`pkg/storage/binary`)

`pkg/storage/binary` implements high-density binary serialization for ZQK kernel objects using the Field ID Registry (`specbuilder/registry`).

---

## 1. Architectural Purpose

In standard file-backed storage, kernel objects are persisted as human-readable YAML or JSON documents. While ideal for developer inspection and git diffability, text-based serialization introduces substantial overhead:
- Repeated field name strings (`status`, `kind`, `created_at`, `instructions_summary`) bloat disk and wire transfers.
- Parsing text requires lexical analysis and memory allocations for every key.

`BinaryStorageProvider` provides an alternative **Positional Binary Encoding**:

```
 ┌───────────────────────┬──────────────────────────┬────────────────────────┐
 │ Field ID (uint32, LE) │ Value Length (uint32, LE)│ Raw Value Bytes        │
 │ 4 bytes               │ 4 bytes                  │ N bytes                │
 └───────────────────────┴──────────────────────────┴────────────────────────┘
```

---

## 2. Integration with Field Registry

Field IDs are globally unique, stable integers managed by `pkg/specbuilder/registry/field_registry.go`.
When `WriteObject(w, data)` is called:
1. It iterates through the loaded field registry metadata.
2. For each field present in `data`, it emits the integer `FieldID` (4 bytes, little-endian).
3. It emits the byte length of the value (4 bytes, little-endian).
4. It streams the raw value bytes directly to the `io.Writer`.

---

## 3. Use Cases

1. **High-Volume Stream Delta Transfer**: Compressing stream-backed kinds (`audit_event`, `change_journal`) across IPC pipes and peer nodes.
2. **CAS Chunk Archival**: Packing immutable object snapshots into dense chunk storage with minimal metadata bloat.
3. **Wire Protocol**: Serving low-latency binary streams to remote agents without string-keyed JSON marshalling overhead.
