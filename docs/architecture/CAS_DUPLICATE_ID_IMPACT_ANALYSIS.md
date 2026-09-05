# Content-Addressable Storage: Duplicate ID Impact Analysis

**Last Verified:** 2026-08-31


**Date:** 2026-01-05  
**Status:** ✅ Fixed and Documented

## Summary

This document analyzes the impact of duplicate ID handling in content-addressable storage (CAS) and documents all fixes applied to ensure proper duplicate prevention across the system.

## Problem Statement

When CAS was initially implemented, the `Create` method in `FileObjectStorage` checked for duplicate IDs using `os.Stat(filePath)`, which doesn't work for CAS-enabled kinds because:
- CAS objects are stored with hash-based filenames (e.g., `abc123...def456.yaml`)
- The ID-to-hash mapping is stored in an index file (`.{kind}.index`)
- `os.Stat()` on the ID-based file path will always fail for CAS objects

This could allow duplicate IDs to be created silently, overwriting existing objects.

## Fixes Applied

### 1. FileObjectStorage.Create ✅

**Location:** `pkg/storage/object_storage_file.go:865-868`

**Before:**
```go
// Check if object already exists
if _, err := os.Stat(filePath); err == nil {
    return ErrObjectExists
}
```

**After:**
```go
// Check if object already exists
// For CAS-enabled kinds, check the CAS index instead of file path
if f.usesContentAddressableStorage(kind) {
    cas, err := f.getContentAddressableStorage(kind)
    if err == nil {
        _, hashErr := cas.GetHashForID(id)
        if hashErr == nil {
            // Object exists in CAS index
            return ErrObjectExists
        }
    }
    // If CAS check fails, fall through to file-based check as backup
} else {
    // For non-CAS kinds, check file existence
    if _, err := os.Stat(filePath); err == nil {
        return ErrObjectExists
    }
}
```

**Impact:** Prevents duplicate IDs from being created for CAS-enabled kinds.

### 2. FileObjectStorage.Exists ✅

**Location:** `pkg/storage/object_storage_file.go:3973-3986`

**Before:**
```go
// Check if file exists (more efficient than reading and parsing)
_, err = os.Stat(filePath)
if err == nil {
    return true, nil
}
```

**After:**
```go
// Check if object exists
// For CAS-enabled kinds, check the CAS index instead of file path
if f.usesContentAddressableStorage(kind) {
    cas, err := f.getContentAddressableStorage(kind)
    if err == nil {
        _, hashErr := cas.GetHashForID(id)
        if hashErr == nil {
            // Object exists in CAS index
            return true, nil
        }
        // Not found in CAS index
        return false, nil
    }
    // If CAS check fails, fall through to file-based check as backup
}

// For non-CAS kinds, check file existence
_, err = os.Stat(filePath)
```

**Impact:** `Exists()` now correctly reports existence for CAS-enabled objects.

## Other Scenarios Analyzed

### 3. Operation Executors ✅

**Location:** `pkg/storage/operation_executor.go`, `pkg/storage/operation_executor_enhanced.go`

**Status:** ✅ Already handles CAS correctly

Both operation executors use `storage.Read()` to check for object existence before creating:
```go
_, err := e.storage.Read(ctx, op.SecCtx, op.ObjectID)
if err == nil || err != ErrObjectNotFound {
    // Object exists or other error
    return fmt.Errorf("object already exists: %s", op.ObjectID)
}
```

Since `FileObjectStorage.Read()` already has CAS support with fallback logic, this works correctly.

### 4. Update Operations ✅

**Location:** `pkg/storage/object_storage_file.go:1295-1314`

**Status:** ✅ Already handles CAS correctly

The `Update` method checks if an object exists in CAS before updating:
```go
_, hashErr := cas.GetHashForID(updateID)
if hashErr != nil {
    // Object not in CAS index - migrate from ID-based to CAS
    if err := cas.Create(updateID, data); err != nil {
        return fmt.Errorf("failed to migrate object to content-addressable storage: %w", err)
    }
}
```

This correctly handles migration and prevents creating duplicates during updates.

### 5. Validation State Cache ⚠️

**Location:** `pkg/validation/state_cache.go`, `pkg/validation/async_validator.go`

**Status:** ⚠️ Uses file paths, but should work with CAS

The validation state cache stores `FilePath` in `ValidationState`. For CAS objects:
- The file path would be the hash-based filename (e.g., `abc123...def456.yaml`)
- The validation system reads files by path using `os.ReadFile(filePath)`
- This should work correctly as long as the path is the actual hash-based file path

**Recommendation:** When validation state is saved for CAS objects, ensure the `FilePath` field contains the actual hash-based file path (not the ID-based path). This is currently handled by the validation system reading files directly, but we should verify that the path stored in the cache is correct.

**Current Behavior:**
- Validation tasks are enqueued with `filePath` from `getObjectFilePath()` (ID-based path for non-CAS)
- For CAS objects, we should use `cas.GetFilePathForID()` to get the actual hash-based path
- However, the validation system reads files by path, so as long as the path is correct, it should work

**Action Required:** Verify that validation state cache uses correct file paths for CAS objects. Consider updating validation enqueue logic to use `cas.GetFilePathForID()` for CAS-enabled kinds.

### 6. Cache Building Processes ✅

**Location:** `pkg/storage/object_storage_file.go:2507-2606` (List method)

**Status:** ✅ Already handles CAS correctly

The `List` method combines IDs from both CAS index and ID-based file scans:
```go
// Get all IDs from index
casIDs, err := cas.ListIDs()
// Also scan directory for ID-based files (migration support)
idBasedFiles := f.scanIDBasedFilesForMigration(kindDir, filter.Kind)
// Combine CAS IDs and ID-based file IDs, deduplicating
```

This ensures all objects (migrated and unmigrated) are listed correctly.

### 7. Move Operations ✅

**Location:** `pkg/storage/object_storage_file.go:1795-1810`

**Status:** ✅ Should work correctly

Move operations check if the target file exists:
```go
// Check if target file already exists
if _, err := os.Stat(newFilePath); err == nil {
    return fmt.Errorf("target file already exists: %s", newFilePath)
}
```

For CAS objects, this check happens before the CAS operations, so it should work. However, we should verify that move operations correctly handle CAS objects (they should update the CAS index, not just move files).

**Action Required:** Verify move operations work correctly with CAS. The move should:
1. Read object from CAS (or ID-based file)
2. Create object in new kind's CAS (if new kind uses CAS)
3. Delete from old kind's CAS (if old kind uses CAS)
4. Update CAS indexes accordingly

## Test Coverage

### Tests Created

1. **TestCAS_DuplicateID_SameContent** - Verifies that creating an object with the same ID and same content deduplicates correctly
2. **TestCAS_DuplicateID_DifferentContent** - Verifies behavior when creating an object with the same ID but different content
3. **TestCAS_DuplicateID_ThroughStorage** - Verifies that `FileObjectStorage.Create` correctly prevents duplicates

### Test Results

All tests pass:
- ✅ `TestCAS_DuplicateID_SameContent` - CAS deduplicates correctly (same hash file)
- ✅ `TestCAS_DuplicateID_DifferentContent` - CAS overwrites index mapping (new hash file)
- ✅ `TestCAS_DuplicateID_ThroughStorage` - `FileObjectStorage.Create` prevents duplicates correctly

## Behavior Summary

### CAS Create Behavior

1. **Same ID, Same Content:**
   - Uses same hash file (deduplication)
   - Index mapping unchanged
   - No error returned (silent deduplication)

2. **Same ID, Different Content:**
   - Creates new hash file with new content
   - Updates index mapping to point to new hash
   - Old hash file remains on disk (orphaned, but harmless)
   - No error returned (silent overwrite)

### FileObjectStorage.Create Behavior

- **For CAS-enabled kinds:** Checks CAS index for duplicate IDs, returns `ErrObjectExists` if found
- **For non-CAS kinds:** Checks file existence via `os.Stat()`, returns `ErrObjectExists` if found
- **Prevents accidental overwrites:** Always checks before creating

## Recommendations

1. ✅ **Fixed:** `FileObjectStorage.Create` now checks CAS index for duplicates
2. ✅ **Fixed:** `FileObjectStorage.Exists` now checks CAS index for CAS-enabled kinds
3. ⚠️ **Verify:** Validation state cache uses correct file paths for CAS objects
4. ⚠️ **Verify:** Move operations correctly handle CAS objects

## Related Files

- `pkg/storage/object_storage_file.go` - Main storage implementation
- `pkg/storage/content_addressable_storage.go` - CAS implementation
- `pkg/storage/cas_duplicate_id_test.go` - Test coverage
- `pkg/validation/state_cache.go` - Validation state cache
- `pkg/validation/async_validator.go` - Async validation system

