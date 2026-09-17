# Object Directory Location Validation v1.0

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Created:** 2026-01-01  
**Status:** Active  
**Category:** Architecture

## Summary

This document describes the correct directory structure for object files and validation mechanisms to prevent files from being placed in incorrect directories.

## Problem Statement

Object files must be stored in directories that match their kind mapping. For example:
- `scheduler_job` objects must be in `.zqk/process/scheduler_jobs/`
- `backlog_item` objects must be in `.zqk/process/backlog/`
- `audit_event` objects must be in `.zqk/process/audit/YYYY-MM/` (bucketed)

Placing files in incorrect directories causes:
- Objects not being discovered by `List()` operations
- Objects not being found by `Read()` operations
- Inconsistent system behavior
- Objects appearing "missing" even though files exist

## Solution

### Directory Mapping

The system uses `objects.GetDirectoryFromKind(kind)` to determine the correct directory for each object kind. This mapping is defined in `pkg/objects/kind_mappings.go`.

**Common Mappings:**
- `scheduler_job` → `scheduler_jobs/`
- `backlog_item` → `backlog/`
- `audit_event` → `audit/` (with monthly bucketing)
- `account` → `accounts/`
- `role` → `roles/`

### Validation

The `storage.ValidateObjectFileLocation()` function validates that files are in the correct directory:

```go
if err := storage.ValidateObjectFileLocation(filePath, kind); err != nil {
    // File is in wrong directory
    return err
}
```

### System Check Integration

The `zqk system check` command automatically validates directory locations and reports Tier 2 warnings for misplaced files.

## Safeguards

### 1. Runtime Validation

- **When**: During `zqk system check`
- **Action**: Reports Tier 2 warnings for misplaced files
- **Location**: `pkg/storage/directory_location_validator.go`

### 2. CLI Object Creation

- **When**: Creating objects via `zqk object create`
- **Action**: Automatically places files in correct directory
- **Prevention**: Always use CLI commands instead of direct file creation

### 3. Find Misplaced Files Utility

The `storage.FindMisplacedObjectFiles()` function can scan for all misplaced files:

```go
misplaced, err := storage.FindMisplacedObjectFiles(projectRoot)
// Returns map[kind][]filePaths
```

## Best Practices

1. **Always use CLI commands** for object creation:
   ```bash
   zqk object create SCH-001 --file job.yaml
   ```

2. **Check directory mapping** before manually placing files:
   ```bash
   # Check what directory a kind uses
   zqk object list <kind> --format json | jq '.meta.directory'
   ```

3. **Run system check regularly** to catch misplaced files:
   ```bash
   zqk system check
   ```

4. **Use FindMisplacedObjectFiles** for bulk validation:
   ```go
   misplaced, _ := storage.FindMisplacedObjectFiles(projectRoot)
   for kind, files := range misplaced {
       fmt.Printf("Kind %s has %d misplaced files\n", kind, len(files))
   }
   ```

## Common Mistakes

### ❌ Wrong: Nested Directory Structure
```
.zqk/process/scheduler/jobs/SCH-001.yaml  # WRONG
```

### ✅ Correct: Flat Directory Structure
```
.zqk/process/scheduler_jobs/SCH-001.yaml  # CORRECT
```

### ❌ Wrong: Using Plural Form Incorrectly
```
.zqk/process/scheduler_job/SCH-001.yaml  # WRONG (singular)
```

### ✅ Correct: Using Correct Plural Form
```
.zqk/process/scheduler_jobs/SCH-001.yaml  # CORRECT (plural)
```

## Migration

If files are found in incorrect directories:

1. **Identify misplaced files**:
   ```bash
   zqk system check | grep "wrong directory"
   ```

2. **Move files to correct location**:
   ```bash
   # Example: Move scheduler jobs
   mv .zqk/process/scheduler/jobs/*.yaml .zqk/process/scheduler_jobs/
   ```

3. **Refresh cache**:
   ```bash
   zqk system check --refresh-cache
   ```

4. **Verify**:
   ```bash
   zqk object list scheduler_job
   ```

## Related

- `pkg/objects/kind_mappings.go` - Directory mapping definitions
- `pkg/storage/directory_location_validator.go` - Validation implementation
- `cmd/zqk/system/check_impl.go` - System check integration

