# Content-Addressable Storage Design v2 (Compatible)

## Problem with Original Design

The original design (hash-only filenames) breaks existing utilities:
- `getObjectFilePath()` expects `{id}.yaml` files
- `List()` scans for `*.yaml` files by ID pattern
- Object utilities expect ID-based filenames
- References work by ID, not hash

## Revised Design: Hybrid Approach

### File Structure

**Two types of files:**

1. **ID pointer files**: `{id}.yaml` (for compatibility)
   - Contains minimal metadata + hash reference
   - OR: Symlink to `{hash}.yaml` (simpler)
   - OR: Just contains hash, actual data in hash file

2. **Content files**: `{hash}.yaml` (content-addressable)
   - Actual object data
   - Hash is filename (integrity guaranteed)

### Option A: ID Files as Hash References (Recommended)

```
AAM-068.yaml          → Contains: hash: abc123...
abc123...yaml         → Contains: full object data
.{kind}.index         → Maps: AAM-068 -> abc123...
```

**ID file format:**
```yaml
id: AAM-068
kind: audit_aggregation_metric
hash: abc123def456...
# Optional: minimal metadata for fast lookups
```

**Benefits:**
- Existing utilities see `{id}.yaml` files (compatible)
- Content stored once (deduplication)
- Hash integrity guaranteed
- Fast ID lookups (just read small pointer file)

**Read flow:**
1. Read `AAM-068.yaml` → get hash
2. Read `{hash}.yaml` → get full data
3. Merge if needed (or just use hash file)

### Option B: Symlinks (Simpler but Fragile)

```
AAM-068.yaml          → Symlink to: abc123...yaml
abc123...yaml         → Contains: full object data
.{kind}.index         → Maps: AAM-068 -> abc123...
```

**Benefits:**
- Very simple
- Existing utilities work transparently
- File system handles the link

**Drawbacks:**
- Symlinks can break (not portable)
- Some tools don't follow symlinks
- Git handling varies

### Option C: Index-Only Lookups (Most Efficient)

```
abc123...yaml         → Contains: full object data
.{kind}.index         → Maps: AAM-068 -> abc123...
```

**Modified utilities:**
- `getObjectFilePath()` → looks up hash in index, returns hash file path
- `List()` → reads index, then reads hash files
- All utilities use index for ID -> hash mapping

**Benefits:**
- No duplicate files
- True content-addressable storage
- Most efficient

**Drawbacks:**
- Requires updating all utilities
- More complex migration

## Recommended: Option A (Hash References)

### Implementation

**Create:**
1. Marshal object to YAML
2. Calculate hash from data
3. Write `{hash}.yaml` with full data
4. Write `{id}.yaml` with hash reference:
   ```yaml
   id: AAM-068
   kind: audit_aggregation_metric
   content_hash: abc123...
   ```
5. Update `.{kind}.index`: `id -> hash`

**Read:**
1. Read `{id}.yaml` → get hash
2. Read `{hash}.yaml` → get full data
3. Return full data

**Update:**
1. Read `{id}.yaml` → get old hash
2. Marshal updated object
3. Calculate new hash
4. Write `{new_hash}.yaml`
5. Update `{id}.yaml` with new hash
6. Delete `{old_hash}.yaml` (if no other references)

**List:**
- Scan for `{id}.yaml` files (existing logic works!)
- For each, read hash reference
- Optionally: read full data from hash file

### Migration Path

**Phase 1: Dual-Write**
- Write both `{id}.yaml` (full data) and `{hash}.yaml`
- Update index
- Read from `{id}.yaml` (backward compatible)

**Phase 2: Convert to References**
- Migrate `{id}.yaml` files to hash references
- Keep `{hash}.yaml` files
- Read: `{id}.yaml` → hash → `{hash}.yaml`

**Phase 3: Optimize**
- Remove old full-data `{id}.yaml` files
- Only keep reference files

## Alternative: Start with One Kind

**Pilot with `audit_aggregation_metric`:**
- Most problematic (hash mismatches)
- High volume (good test)
- System-created (easier migration)
- Isolated (doesn't break other things)

**If successful:**
- Expand to other system-created kinds
- Then user-created kinds
- Finally all kinds

## Decision Needed

Which approach do you prefer?

1. **Option A**: ID files as hash references (recommended)
2. **Option B**: Symlinks (simpler but fragile)
3. **Option C**: Index-only (most efficient, requires utility updates)
4. **Pilot**: Start with one kind, see how it works

