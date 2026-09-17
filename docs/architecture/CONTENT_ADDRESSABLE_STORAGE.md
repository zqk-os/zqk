# Content-Addressable Storage (CAS)

**Last Verified:** 2026-09-09

## Overview

The ZQK project implements a Git-style Content-Addressable Storage (CAS) system for process objects to resolve race conditions and hash mismatches. By ensuring that the hash of the content IS the filename, the system guarantees integrity and avoids the need for a separate out-of-band hash registry. 

This canonical spec consolidates previous design iterations into the final Index-Only approach (pure content-addressable storage).

## Architecture

The storage model consists of two primary components:

1. **Content Files:**
   - Objects are stored as `{hash}.yaml` (e.g., `abc123def456...yaml`).
   - The file contains the full object data.
   - Hash is the filename, guaranteeing integrity.

2. **Index File:**
   - Maintained per kind: `.{kind}.index`
   - Maps logical IDs to content hashes for lookups.
   - JSON format: `{"AAM-068": "abc123...", "AAM-069": "def456..."}`

**No ID-based files** exist for CAS-enabled kinds in the main storage directory. This provides true content deduplication: if two objects have identical content, they map to the same underlying file.

## Write Path (Membrane)

Operations creating or modifying objects follow a strict pipeline to maintain the CAS guarantees. Per **POL-CODE-002**, mutations should rely on CLI-only operations and system builders rather than direct file edits.

**Create/Update Flow:**
1. Marshal object to YAML.
2. Calculate hash from the marshaled data before writing.
3. Write the content to `{hash}.yaml`.
4. Update the in-memory and on-disk `.{kind}.index`: `id -> new_hash`.
5. For updates, delete `{old_hash}.yaml` if there are no other references to it.
6. Trust `file.Sync()` per the durability policy.

**Object Draft Plane & Membrane:**
Lifecycle-preliminary objects (e.g., drafts) are persisted under `.zqk/object_drafts/<kind>/<shard>/<id>.yaml`. These are mutable, ID-keyed, and bypass the CAS index until they are promoted. Once an object leaves the preliminary state, it is materialized via a CAS write and removed from the draft plane.

**Pending Visibility Layer:**
To ensure cross-process read-your-writes consistency without O(N) directory scans, a successful mutation publishes the `id -> hash` mapping to a pending visibility layer before returning. Readers check this pending cache before falling back to the durable on-disk index.

## Read Path

**List and Get Operations:**
- `getObjectFilePath()`: Looks up the hash in the index and returns the `{hash}.yaml` path.
- `List()`: Reads the `.{kind}.index` to get all ID -> hash mappings. It then lazy-loads hash files as needed. Disparity detection triggers a background refresh if the on-disk file count differs from the index count.

**Search Limitation:**
Currently, the search operation only inspects top-level string fields. Nested object fields (e.g., `metadata.description`) are not recursively searched. This is a known gap and requires flattening or recursive traversal in future iterations.

## Hash Scheme

- **Algorithm:** SHA-256.
- **Input:** Marshaled YAML data (calculated before writing).
- The calculated hash dictates the filename, eliminating any separate storage requirements for hash validation.

## Duplicate Detection

By design, CAS inherently handles content deduplication:
- **Same ID, Same Content:** Resolves to the same hash file. The index mapping is unchanged. No error is returned (silent deduplication).
- **Same ID, Different Content:** A new hash file is created. The index is updated to point to the new hash. The old file is left orphaned until garbage collected.
- **Duplicate ID Prevention:** `FileObjectStorage.Create` and `FileObjectStorage.Exists` explicitly query the CAS index (not `os.Stat` on ID-based paths) to detect and prevent unintentional ID collisions.

## Quarantine

Quarantine acts as a safety switch and audit mechanism for objects with validation or integrity issues, adhering to system checks under **POL-CODE-004**.

- **Isolation:** Objects failing validation are moved to a quarantine folder (e.g., `.zqk/system-health/quarantine/`). This isolates them from lifecycle logic and reduces re-validation overhead.
- **Audit Trail:** The quarantine manifest records the original violations, the remedy applied (e.g., an auto-fix command), the outcome, and related metrics.
- **Targeted Auto-Fix:** Auto-fix options can run strictly against quarantined items rather than re-validating the entire tree.
- **Invariant:** System-generated objects (produced via schedulers or system commands) are assumed valid by construction and are **never** quarantined.
