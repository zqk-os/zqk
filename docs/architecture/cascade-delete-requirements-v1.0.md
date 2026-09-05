# Cascade Delete Requirements

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2026-01-18  
**Status**: Active  
**Purpose**: Explicitly define cascade delete behavior and requirements, especially for CAS objects

## Overview

Cascade delete is the process of handling dependent objects when an object is deleted. This document specifies the exact requirements and behavior for cascade delete operations, including special handling for Content-Addressable Storage (CAS) objects.

## Use case summary (stable behavior)

**When** a user (or CLI) deletes an object by ID with optional `--cascade`:

1. **Dependents first (always)**  
   We always check for dependents *before* deciding how to delete the target (CAS vs file-based). That keeps behavior the same for all object types and prevents CAS from bypassing cascade.

2. **cascade=false**  
   If any dependents exist → fail with a clear error: "N dependent object(s) exist (use cascade=true to delete them)". No storage type check is used to skip this.

3. **cascade=true**  
   For each dependent we resolve its kind, then call `Delete(..., true)` recursively. After all dependents are deleted, we delete the target using the appropriate path (CAS or file-based).

4. **Storage-type-specific delete**  
   Only after dependents are handled do we branch:
   - **CAS**: `cas.Delete(id)` — remove index entry and hash file.
   - **Non-CAS**: remove file and update hash registry.

5. **Leaf kinds**  
   Kinds that nothing references (e.g. `audit_aggregation_metric`, `change_journal_entry`) skip the dependent check for performance; they never participate in cascade as dependents.

**Stable invariants**: (1) Dependent check is independent of storage type. (2) CAS and non-CAS objects both support cascade delete and cascade block. (3) Deletion order is always: dependents first, then target.

## Core Requirements

### Requirement 1: Cascade Delete Must Work for All Object Types

**Requirement**: Cascade delete (`--cascade` flag) must work identically for both CAS and non-CAS objects.

**Implementation**: Dependent check runs first in `pkg/storage/object_storage_file_delete.go` (before CAS vs non-CAS branch). CAS objects no longer bypass cascade.

**Required Behavior**:
1. Check for dependents **before** determining storage type (CAS vs non-CAS)
2. If `cascade=false` and dependents exist → error
3. If `cascade=true` and dependents exist → recursively delete dependents first
4. Then delete the target object using appropriate storage method (CAS or file-based)

### Requirement 2: CAS Objects Must Support Cascade Delete

**Requirement**: CAS objects (e.g., `scheduler_job`, `audit_event`, `change_journal_entry`) must support cascade delete operations.

**Required Flow**:
```
1. Check for dependents (regardless of storage type)
2. If cascade=true and dependents exist:
   a. For each dependent:
      - Determine dependent's storage type (may be CAS or non-CAS)
      - Recursively call Delete(dependentID, cascade=true)
   b. After all dependents deleted, proceed to delete target
3. Delete target object:
   - If CAS: Use cas.Delete(objectID)
   - If non-CAS: Use file-based delete
```

### Requirement 3: Orphan Cleanup for CAS Updates

**Requirement**: When a CAS object is updated (content changes, hash changes), the old hash file must be cleaned up immediately after successful index update.

**Required Behavior**:
1. Write new hash file
2. Update CAS index (wait for completion)
3. If index update succeeds → delete old hash file via cleanup callback
4. If index update fails → rollback (delete new file, restore in-memory mapping)

**Note**: This cleanup callback mechanism applies to `Update()` operations only, not `Delete()` operations.

### Requirement 4: CAS Delete Must Remove Hash Files

**Requirement**: When a CAS object is deleted, both the index entry and the hash file must be removed.

**Required Behavior** (already implemented in `cas.Delete()`):
1. Get hash from index
2. Remove ID→hash mapping from index
3. Delete hash file
4. Return success

**Note**: No cleanup callback needed for deletes - direct file removal is appropriate.

## Cascade Delete Flow Specification

### Flow Diagram

```
Delete(id, cascade)
│
├─> Check for dependents (findDependents(id, kind))
│   │
│   ├─> If dependents exist AND cascade=false
│   │   └─> Return error: "cannot delete: N dependents exist (use cascade=true)"
│   │
│   └─> If dependents exist AND cascade=true
│       │
│       ├─> For each dependent:
│       │   ├─> Read dependent to get kind
│       │   ├─> Recursively call Delete(dependentID, cascade=true)
│       │   └─> Continue even if some fail (log warnings)
│       │
│       └─> After all dependents processed, continue to target deletion
│
├─> Determine storage type (CAS vs non-CAS)
│   │
│   ├─> If CAS object:
│   │   ├─> Get CAS instance for kind
│   │   ├─> Call cas.Delete(id)
│   │   │   ├─> Get hash from index
│   │   │   ├─> Remove from index
│   │   │   └─> Delete hash file
│   │   └─> Create audit event
│   │
│   └─> If non-CAS object:
│       ├─> Get file path
│       ├─> Create audit event
│       ├─> Delete file
│       └─> Update hash registry
│
└─> Return success
```

### Code Structure Requirements

**Implementation**: Dependent check is performed first; CAS vs non-CAS is decided only after dependents are handled. Reference implementation:

```go
func (f *FileObjectStorage) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
    // ... permission checks ...
    
    // STEP 1: Check for dependents FIRST (before storage type check)
    dependents, err := f.findDependents(id, kind)
    if err != nil {
        return fmt.Errorf("failed to check dependencies: %w", err)
    }
    
    if len(dependents) > 0 {
        if !cascade {
            return fmt.Errorf("cannot delete object %s: %d dependent object(s) exist (use cascade=true)", id, len(dependents))
        }
        
        // Cascade delete: delete dependents first
        for _, dependentID := range dependents {
            dependent, err := f.Read(ctx, secCtx, dependentID)
            if err != nil {
                continue // Skip if already deleted
            }
            
            dependentKind, _ := dependent["kind"].(string)
            if dependentKind == "" {
                continue
            }
            
            // Recursively cascade delete the dependent
            if err := f.Delete(ctx, secCtx, dependentID, true); err != nil {
                return fmt.Errorf("failed to cascade delete dependent %s: %w", dependentID, err)
            }
        }
    }
    
    // STEP 2: Now check storage type and delete target object
    if f.usesContentAddressableStorage(kind) {
        // CAS delete path
        cas, err := f.getContentAddressableStorage(kind)
        if err != nil {
            return fmt.Errorf("failed to get content-addressable storage: %w", err)
        }
        if err := cas.Delete(id); err != nil {
            return fmt.Errorf("failed to delete object in content-addressable storage: %w", err)
        }
        // ... audit event, hash registry ...
    } else {
        // Non-CAS delete path
        // ... file-based delete ...
    }
    
    return nil
}
```

## CAS Update vs Delete: Cleanup Behavior

### CAS Update (Content Changes)

**Scenario**: Object content changes, hash changes, new hash file created.

**Cleanup Mechanism**: Cleanup callback (orphan cleanup)
- Old hash file is orphaned (index no longer references it)
- Cleanup callback deletes old file after successful index update
- Transactional: if index update fails, new file is deleted and mapping restored

**When**: Only during `Update()` operations

### CAS Delete (Object Removal)

**Scenario**: Object is deleted entirely.

**Cleanup Mechanism**: Direct file deletion
- Hash file is deleted directly by `cas.Delete()`
- Index entry is removed
- No cleanup callback needed (file is intentionally deleted, not orphaned)

**When**: During `Delete()` operations

## Edge Cases and Error Handling

### Edge Case 1: Dependent is CAS, Parent is Non-CAS

**Requirement**: Must work correctly.

**Behavior**: 
- Dependent deletion uses CAS path (`cas.Delete()`)
- Parent deletion uses file-based path
- Both must complete successfully

### Edge Case 2: Circular Dependencies

**Requirement**: Must detect and prevent infinite loops.

**Behavior**:
- Track visited IDs during cascade delete
- If same ID encountered twice → error: "circular dependency detected"
- Abort cascade operation

### Edge Case 3: Partial Cascade Failure

**Requirement**: Must handle gracefully.

**Behavior**:
- If one dependent deletion fails, log warning and continue with others
- After all dependents processed, attempt target deletion
- Return error if target deletion fails
- Return success if target deletion succeeds (even if some dependents failed)

### Edge Case 4: Concurrent Cascade Deletes

**Requirement**: Must prevent race conditions.

**Behavior**:
- CAS index updates are serialized via write queue
- File deletions use OS-level atomic operations
- Dependent checks use consistent snapshots

## Testing Requirements

### Test Case 1: CAS Object with Dependents (cascade=false)

**Setup**: 
- Create CAS object `SCH-001` (scheduler_job)
- Create dependent object `BLI-001` that references `SCH-001`

**Action**: `Delete("SCH-001", cascade=false)`

**Expected**: Error - "cannot delete SCH-001: 1 dependent object(s) exist"

### Test Case 2: CAS Object with Dependents (cascade=true)

**Setup**: 
- Create CAS object `SCH-001`
- Create dependent `BLI-001` that references `SCH-001`

**Action**: `Delete("SCH-001", cascade=true)`

**Expected**: 
1. `BLI-001` deleted first
2. `SCH-001` deleted second
3. Both hash files removed
4. Both index entries removed

### Test Case 3: CAS Object Updated (Orphan Cleanup)

**Setup**: 
- Create CAS object `SCH-001` with hash `abc123...`
- Update object content (hash changes to `def456...`)

**Action**: `Update("SCH-001", newData)`

**Expected**:
1. New hash file `def456....yaml` created
2. Index updated to point to new hash
3. Old hash file `abc123....yaml` deleted via cleanup callback
4. No orphaned files remain

### Test Case 4: Multi-Level Cascade with Mixed Storage Types

**Setup**:
- CAS object `SCH-001` (scheduler_job)
- Non-CAS object `BLI-001` (backlog_item) references `SCH-001`
- CAS object `AUD-001` (audit_event) references `BLI-001`

**Action**: `Delete("SCH-001", cascade=true)`

**Expected**:
1. `AUD-001` deleted (CAS delete)
2. `BLI-001` deleted (file-based delete)
3. `SCH-001` deleted (CAS delete)
4. All files and index entries removed

## Implementation Checklist

- [x] Move dependent check before CAS check in `FileObjectStorage.Delete()` — implemented in `pkg/storage/object_storage_file_delete.go`
- [x] Ensure cascade delete works for CAS objects
- [x] Ensure cascade delete works for non-CAS objects
- [x] Ensure cascade delete works for mixed storage types (same flow; each object uses its own storage path)
- [ ] Add circular dependency detection (track visited IDs during cascade)
- [x] Partial failure handling (log and continue for dependent delete failures; fail target delete if it fails)
- [ ] Add comprehensive test cases (some CAS cascade tests may fail due to index/read visibility, not cascade order)
- [x] Document behavior in code comments and this doc
- [x] Use case summary above for stable, consistent behavior

## References

- `docs/process/architecture/cascade-analysis-v1.0.md` - Analysis of cascade scenarios
- `docs/process/architecture/cascade-rules-v1.0.md` - Cascade rules by object type
- `pkg/storage/object_storage_file.go` - Current implementation
- `pkg/storage/content_addressable_storage.go` - CAS implementation
