# Hash Registry Location for Bucketed Storage

**Last Verified:** 2026-08-31


**Version:** 1.0  
**Created:** 2025-12-29  
**Status:** Active  
**Category:** Architecture

## Summary

This document describes the correct pattern for hash registry location when dealing with bucketed storage, and safeguards to prevent registry location mismatches.

## Problem Statement

For bucketed objects (e.g., `audit_event`, `change_journal_entry`), files are stored in monthly subdirectories (e.g., `docs/process/audit/2025-12/`), but hash registries must be located in the **same directory as the files**, not in the parent kind directory.

Using the wrong registry location causes:
- Stale hash reads from parent-level registries
- Hash mismatches that persist even after fixes
- False-positive integrity violations

## Solution Pattern

### For Bucketed Objects

**Registry Location:** Use the **file's directory** (subdirectory), not the kind directory.

```go
// ✅ CORRECT: Use file's directory for bucketed objects
fileDir := filepath.Dir(filePath)
registry := storage.NewHashRegistry(kind, fileDir)

// ❌ WRONG: Using parent kind directory
kindDir := getKindDirectory(projectRoot, kind)
registry := storage.NewHashRegistry(kind, kindDir)  // Wrong for bucketed objects!
```

### For Non-Bucketed Objects

**Registry Location:** Use the **kind directory** (which matches the file's directory).

```go
// ✅ CORRECT: Use kind directory (same as file's directory for non-bucketed)
kindDir := getKindDirectory(projectRoot, kind)
registry := storage.NewHashRegistry(kind, kindDir)
```

## Safeguards

### 1. Runtime Validation

The `storage.ValidateRegistryLocation()` function validates that registry directories match file locations:

```go
// Validate registry location matches file location
isBucketed := fileDir != kindDir
if err := storage.ValidateRegistryLocation(fileDir, filePath, kind, isBucketed); err != nil {
    logger.Warn("Registry location validation failed", ...)
}
```

### 2. Test Coverage

Two tests ensure correct behavior:

- **`TestHashRegistryLocationForBucketedObjects`**: Verifies registries are in subdirectories for bucketed objects
- **`TestHashRegistryLocationForNonBucketedObjects`**: Verifies registries are in kind directories for non-bucketed objects

### 3. Helper Function

Use `storage.GetRegistryDirectoryForFile()` to ensure consistency:

```go
fileDir := storage.GetRegistryDirectoryForFile(filePath, isBucketed)
registry := storage.NewHashRegistry(kind, fileDir)
```

## Detection

### How to Identify the Issue

1. **Hash mismatches persist** even after running `--auto-fix --force`
2. **Registry file exists in parent directory** with different hash than file
3. **Registry file exists in subdirectory** with correct hash, but code reads from parent

### System Check

The `zqk system check` command now validates registry locations and logs warnings if mismatches are detected.

## Prevention Checklist

When working with hash registries:

- [ ] For bucketed objects, always use `filepath.Dir(filePath)` for registry directory
- [ ] For non-bucketed objects, use the kind directory (which matches file directory)
- [ ] Add `ValidateRegistryLocation()` calls in critical paths
- [ ] Write tests that verify registry location for both bucketed and non-bucketed objects
- [ ] Review code changes that create or access hash registries

## Graph Backend Considerations

**Important**: The graph backend does **not** have this issue because:

1. **Single Registry Per Kind**: `GraphHashRegistry` uses a single registry node per kind (e.g., `HashRegistry:backlog_item`), not per subdirectory
2. **No File Paths**: The graph backend doesn't use file paths - everything is stored in graph nodes
3. **No Bucketing Location Issue**: Since there's only one registry per kind, there's no risk of reading from the wrong location

The `ValidateRegistryLocation()` function is a no-op for graph backend (returns `nil` if `filePath` is empty).

## Related Documentation

- [Hash Registry Design](../architecture/hash-registry-design-v1.0.md)
- [Bucketing Configuration](../architecture/bucketing-config-v1.0.md)
- [System Check Implementation](../architecture/system-check-v1.0.md)

## Examples

### Correct Pattern (Bucketed Object)

```go
// File: docs/process/audit/2025-12/AUD-32.yaml
filePath := "docs/process/audit/2025-12/AUD-32.yaml"
fileDir := filepath.Dir(filePath)  // "docs/process/audit/2025-12"
registry := storage.NewHashRegistry("audit_event", fileDir)
// Registry file: docs/process/audit/2025-12/.audit_event.hashes ✅
```

### Incorrect Pattern (Bucketed Object)

```go
// File: docs/process/audit/2025-12/AUD-32.yaml
filePath := "docs/process/audit/2025-12/AUD-32.yaml"
kindDir := getKindDirectory(projectRoot, "audit_event")  // "docs/process/audit"
registry := storage.NewHashRegistry("audit_event", kindDir)
// Registry file: docs/process/audit/.audit_event.hashes ❌ WRONG!
```

## History

- **2025-12-29**: Initial version documenting the pattern and safeguards after fixing cache staleness issue

