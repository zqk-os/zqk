# Content-Addressable Storage Design (Git-Style)

**Last Verified:** 2026-08-31


## Problem
Current hash handling has race conditions because:
1. We write files with ID-based names (`AAM-068.yaml`)
2. We calculate hash after writing (reading back from disk)
3. We store hash in separate registry file
4. Two files must stay in sync (object + hash registry)

## Solution: Git-Style Content-Addressable Storage

### Architecture

**File Storage:**
- Objects stored as: `{hash}.yaml` (content-addressable)
- Example: `abc123def456...yaml` instead of `AAM-068.yaml`

**ID Index:**
- Maintain `.{kind}.index` file mapping ID -> hash
- Format: JSON `{"AAM-068": "abc123...", "AAM-069": "def456...", ...}`
- Example: `.audit_aggregation_metric.index`

**Benefits:**
1. **Hash IS the filename** - no mismatch possible
2. **No separate hash registry** - hash is in the filename
3. **Content-addressable** - same content = same hash = same file (deduplication)
4. **Human-readable lookups** - ID index enables search by ID
5. **Git's reliability** - proven approach

### Operations

**Create:**
1. Marshal object to YAML
2. Calculate hash from marshaled data
3. Write to `{hash}.yaml`
4. Update `.{kind}.index`: `id -> hash`
5. Trust `file.Sync()` succeeded (no read-back verification)

**Read:**
1. Look up hash in `.{kind}.index` for given ID
2. Read `{hash}.yaml`
3. Verify hash matches filename (optional integrity check)

**Update:**
1. Read existing `{old_hash}.yaml`
2. Apply updates
3. Marshal to YAML
4. Calculate new hash
5. Write to `{new_hash}.yaml`
6. Update `.{kind}.index`: `id -> new_hash`
7. Delete `{old_hash}.yaml` (if no other references)

**Delete:**
1. Look up hash in `.{kind}.index`
2. Delete `{hash}.yaml`
3. Remove entry from `.{kind}.index`

### Migration Strategy

**Phase 1: Dual-Write (Backward Compatible)**
- Write both `{id}.yaml` (old) and `{hash}.yaml` (new)
- Update both hash registry and index
- Read from old format (ID-based)

**Phase 2: Dual-Read**
- Read from both formats (prefer new, fallback to old)
- Migrate objects on read

**Phase 3: New-Only**
- Only write new format
- Remove old format support

### Implementation Notes

**Index File Format:**
```json
{
  "version": "1.0",
  "kind": "audit_aggregation_metric",
  "mappings": {
    "AAM-068": "abc123def456...",
    "AAM-069": "def456ghi789..."
  }
}
```

**Hash Calculation:**
- Calculate from marshaled YAML data (before writing)
- Use SHA-256 (same as current)
- Hash is the filename (no separate storage needed)

**Concurrency:**
- Index file needs locking for updates
- Use file locking (flock) like current hash registry
- Atomic updates: write to temp, then rename

**Backward Compatibility:**
- Keep reading old format during migration
- CLI commands work with both formats
- System check validates both formats

### Performance Considerations

**Index Lookups:**
- Load index into memory on startup (per kind)
- Update in-memory copy, flush periodically
- Fast ID -> hash lookups

**File System:**
- Many files in single directory (hash-based names)
- Consider subdirectories: `{hash[:2]}/{hash[2:]}.yaml` (like Git)
- Example: `ab/c123def456...yaml`

### Benefits Summary

1. **No hash mismatches** - hash IS the filename
2. **No race conditions** - calculate hash before writing
3. **Content deduplication** - same content = same file
4. **Integrity by design** - filename verifies content
5. **Git's proven approach** - battle-tested reliability
6. **Human-readable search** - ID index enables lookups

