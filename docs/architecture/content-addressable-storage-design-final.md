# Content-Addressable Storage Design (Final)

## Decision: Option C - Index-Only (Pure Content-Addressable)

**Rationale**: No backward compatibility concerns (no public variants), so we can implement the purest, most efficient approach from the start.

## Architecture

### File Structure

**Content Files:**
- `{hash}.yaml` - Full object data (content-addressable)
- Hash IS the filename (integrity guaranteed)

**Index File:**
- `.{kind}.index` - Maps `ID -> hash` for lookups
- JSON format: `{"AAM-068": "abc123...", ...}`

**No ID-based files** - Pure content-addressable storage

### Operations

**Create:**
1. Marshal object to YAML
2. Calculate hash from data (before writing)
3. Write `{hash}.yaml`
4. Update `.{kind}.index`: `id -> hash`
5. Trust `file.Sync()` (Git's approach)

**Read:**
1. Look up hash in `.{kind}.index` for given ID
2. Read `{hash}.yaml`
3. Validate hash matches filename
4. Validate YAML structure (corruption detection)
5. Return data

**Update:**
1. Look up old hash in index
2. Marshal updated object
3. Calculate new hash
4. Write `{new_hash}.yaml`
5. Update index: `id -> new_hash`
6. Delete `{old_hash}.yaml` (if no other references)

**Delete:**
1. Look up hash in index
2. Delete `{hash}.yaml`
3. Remove entry from index

**List:**
1. Read `.{kind}.index` to get all ID -> hash mappings
2. For each ID, read `{hash}.yaml`
3. Apply filters/sorting as needed

### Modified Utilities

**`getObjectFilePath()`:**
- Look up hash in index
- Return `{hash}.yaml` path

**`List()`:**
- Read index file
- Iterate through ID -> hash mappings
- Read hash files as needed

**Benefits:**
- ✅ True content-addressable storage
- ✅ No duplicate files
- ✅ Hash integrity guaranteed (hash IS filename)
- ✅ Content deduplication (same content = same file)
- ✅ Most efficient storage
- ✅ Git's proven approach

### Implementation Strategy

**Phase 1: Pilot with `audit_aggregation_metric`**
- Most problematic (hash mismatches)
- System-created (easier to migrate)
- Isolated (doesn't break other things)
- High volume (good test)

**Phase 2: Expand to system-created kinds**
- `audit_event`
- `change_journal_entry`
- `base_metric`, `command_metric`, etc.

**Phase 3: All kinds**
- User-created kinds
- Full migration

### Index File Format

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

### Performance Considerations

**Index Loading:**
- Load index into memory on startup (per kind)
- Update in-memory copy, flush periodically
- Fast ID -> hash lookups

**File System:**
- Many files in single directory (hash-based names)
- Consider subdirectories: `{hash[:2]}/{hash[2:]}.yaml` (like Git)
- Example: `ab/c123def456...yaml`

**Listing:**
- Read index once (fast)
- Lazy-load hash files as needed
- Much faster than directory scanning

