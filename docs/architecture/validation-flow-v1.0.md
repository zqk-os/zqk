# Validation Flow and Call Order v1.0

**Status**: Active  
**Version**: 1.0.0  
**Last Updated**: 2025-12-29  
**Category**: Validation  
**Group**: Architecture

## Overview

This document maps out the call order for object updates, change journal entry creation, reference validation, and cache operations to identify potential cycles and impossible validation scenarios. It provides a comprehensive reference for understanding when validation occurs and how to avoid cyclic or impossible validation situations.

## Table of Contents

1. [Object Update Flow](#object-update-flow)
2. [Reference Validation Flow](#reference-validation-flow)
3. [Cache Building Flow](#cache-building-flow)
4. [Potential Cycles and Issues](#potential-cycles-and-issues)
5. [Solutions and Fixes](#solutions-and-fixes)
6. [Call Order Summary](#call-order-summary)
7. [Validation Timing Matrix](#validation-timing-matrix)

## Object Update Flow

### 1. CLI Command Execution

**Location**: `cmd/zqk/root.go`

**Flow**:
```
PersistentPreRunE:
  1. Initialize CommandExecutionTracker
  2. Extract command, flags, context
  3. Set actor information
  4. Store tracker in command context

Command Execution:
  - User runs: `zqk object update AUD-32 --field 'updated_at="..."'`
  - Command handler processes update

PersistentPostRunE:
  1. Finalize CommandExecutionTracker
  2. Create audit_event (AUD-32) for command execution
  3. Update hash registry for audit event
```

### 2. Object Storage Update

**Location**: `pkg/storage/object_storage_file.go::Update`

**Flow**:
```
FileObjectStorage.Update():
  1. Read existing object from file
  2. Capture previousStateForJournal (BEFORE merging updates)
  3. Merge updates into existing object
  4. Write updated object to file
  5. Update hash registry for updated object
  6. Execute cache operation (update cache entry)
  7. RecordObjectStateChange() - notify tracker
  8. createChangeJournalEntry() - creates CHA-XXX
```

**Critical Point**: `previousStateForJournal` is captured BEFORE merging, ensuring we have the true previous state.

### 3. Change Journal Entry Creation

**Location**: `pkg/storage/change_journal.go`

**Flow**:
```
createChangeJournalEntry():
  1. Generate journal ID (CHA-XXX) using sequence number
  2. Build object_ref: "audit_event:AUD-32" (format: "kind:id")
  3. Create journal entry map
  4. Include previous_state (for rollback)
  
  VALIDATION PHASE (BEFORE WRITING):
  5. validator.Validate() with ValidateSemanticTypes: false
     - Spec validation (fields, types, required)
     - Lifecycle validation
     - Reference validation is SKIPPED (deferred to system check)
  
  6. If validation fails: Log warning but continue (best effort)
  
  WRITE PHASE:
  7. Write journal entry to file
  8. Update hash registry for journal entry
```

**Critical Point**: Reference validation is deferred to `system check` to avoid cycles where referenced objects (e.g., audit events) may not exist yet.

### 4. Audit Event Creation

**Location**: `cmd/zqk/root.go::createCommandAuditEvent`

**Flow**:
```
createCommandAuditEvent():
  1. Create audit_event object (AUD-32)
  2. Write audit event to file
  3. Update hash registry for audit event
```

**Timing Issue**: Audit events are created in `PersistentPostRunE`, which runs AFTER the command completes. This means:
- If a change journal entry references an audit event created in the same command, the audit event may not exist yet during validation.
- **Solution**: Reference validation is deferred to `system check` where all objects exist.

## Reference Validation Flow

### During Change Journal Entry Creation

**Current Behavior**: Reference validation is **skipped** during change journal entry creation.

**Rationale**:
- Referenced objects (especially audit events) may not exist yet during creation
- Change journal entries are "best effort" - should be created even if validation fails
- Reference validation is deferred to `system check` command where all objects exist

### During System Check

**Location**: `cmd/zqk/system/check_impl.go::checkReferencesWithCache`

**Flow**:
```
checkObjectWithCacheAndContent():
  1. checkRegistration()
  2. checkIntegrityWithRegistryAndContent()
  3. checkInstanceValidationWithValidator()
  4. checkLifecycleWithLoader()
  5. checkPolicy()
  6. checkReferencesWithCache() [if --check-refs and not --fast]
     - Extract reference fields
     - Parse "kind:id" format:
       * Split on ":"
       * kind = "audit_event"
       * actualRefID = "AUD-32"
     - Special handling for change_journal_entry.object_ref:
       * Skip validation for audit_event references (may not exist during creation)
     - Look up in objectIDCache using actualRefID
     - Validate existence and kind matches
```

**Reference Format Parsing**:
- `"account:ACC-001"` → kind="account", id="ACC-001"
- `"audit_event:AUD-32"` → kind="audit_event", id="AUD-32"
- `"BLI-123"` → infer kind from ID pattern

## Cache Building Flow

**Location**: `cmd/zqk/system/check_impl.go::BuildCache`

**Flow**:
```
BuildCache(projectRoot, forceRebuild):
  1. If !forceRebuild:
     - Try LoadCache() from disk
     - If loaded: ValidateAndCleanStale()
     - If valid: return (use cached)
  
  2. If forceRebuild or cache invalid:
     - Clear cache
     - discoverObjectKinds() - find all kind directories
     - For each kind:
       - Get kind directory path
       - YAMLScanner.Scan() - recursively scan directory
       - For each YAML file found:
         * Extract object ID from filename or file content
         * Read kind from file
         * Create ObjectIDCacheEntry
         * Add to cache
     - SaveCache() to disk
```

**Critical Point**: Cache is built by scanning file system. If a file exists but isn't scanned (e.g., in a subdirectory not traversed), it won't be in the cache.

**Cache Freshness**:
- Cache is rebuilt when:
  - `--refresh-cache` flag is used
  - Cache file doesn't exist
  - Cache is detected as stale (process directory mtime changed)
- Cache is updated incrementally when objects are created/updated via `UpdateObjectIDCache()`

## Potential Cycles and Issues

### 1. Audit Event Self-Reference Cycle

**Scenario**: 
- Command execution creates audit event AUD-32
- Audit event update triggers change journal entry CHA-XXX
- Change journal entry references "audit_event:AUD-32"
- Validation checks if AUD-32 exists

**Timing Issue**:
- If `createChangeJournalEntry()` is called during the same command that creates AUD-32, AUD-32 may not exist yet.
- This happens when updating an audit event that was just created.

**Solution**:
- Reference validation is skipped during change journal entry creation
- Validation is deferred to `system check` where all objects exist
- Special handling in `checkReferencesWithCache()` skips audit event references in change journal entries

### 2. Cache Staleness

**Scenario**:
- Object AUD-32 exists in file system
- Cache is built and saved to disk
- Object AUD-32 is created/modified AFTER cache is built
- Cache doesn't include AUD-32
- Reference validation fails because AUD-32 not in cache

**Current Behavior**:
- Individual object checks use file-based lookup (works)
- Full system checks use cache-based lookup (fails if cache stale)
- Cache is rebuilt when stale or when `--refresh-cache` is used

**Solution**:
- Cache is updated incrementally when objects are created/updated
- Cache staleness is detected via process directory mtime
- Use `--refresh-cache` to force rebuild

### 3. Change Journal Entry Validation Timing

**Scenario**:
- Object update creates change journal entry
- Change journal entry references the updated object
- Validation happens BEFORE change journal entry is written
- Referenced object exists (it was just updated)

**Current Behavior**:
- Validation happens in `createChangeJournalEntry()` BEFORE writing
- Reference validation is skipped (deferred to system check)
- Referenced object should exist (it was updated in step 2)
- If validation fails, entry is still written (best effort)

### 4. Hash Registry Update Timing

**Scenario**:
- Object is updated
- Hash registry is updated
- Change journal entry is created
- Change journal entry hash registry is updated
- System check runs and validates hashes

**Current Behavior**:
- Hash registry is updated AFTER file write
- This ensures hash is computed from final file content
- Prevents hash mismatches from partial writes

## Solutions and Fixes

### 1. Defer Change Journal Entry Validation ✅ IMPLEMENTED

**Problem**: Validation happens before writing, but referenced object may not exist yet (e.g., audit events).

**Solution**: 
- Skip reference validation during change journal entry creation
- Defer validation to `system check` command
- Change journal entries are "best effort" - they should be created even if validation fails

**Implementation**:
```go
// In createChangeJournalEntry()
validationOptions := &validation.ValidationOptions{
    CurrentState:          "completed",
    ValidateLifecycle:     true,
    ValidateSemanticTypes: false, // Skip reference validation - defer to system check
}
```

### 2. Ensure Cache Includes All Objects ✅ IMPLEMENTED

**Problem**: Cache may not include objects created after cache was built.

**Solution**:
- Always rebuild cache when `--refresh-cache` is used
- Detect cache staleness via process directory mtime
- Update cache incrementally when objects are created/updated

**Implementation**:
- Cache operations update cache incrementally via `UpdateObjectIDCache()`
- Cache is saved after incremental updates
- `BuildCache()` accepts `forceRebuild` parameter

### 3. Handle Self-Referencing Objects ✅ IMPLEMENTED

**Problem**: Objects that reference themselves (e.g., audit events tracking their own updates) create validation cycles.

**Solution**:
- Skip reference validation for audit event references in change journal entries
- Add special handling in `checkReferencesWithCache()`

**Implementation**:
```go
// In checkReferencesWithCache()
if kind == "change_journal_entry" && fieldName == "object_ref" {
    if refStr, ok := refValue.(string); ok && strings.HasPrefix(refStr, "audit_event:") {
        // Skip validation for audit event references in change journal entries
        // These are created during command execution and validation is deferred to system check
        continue
    }
}
```

## Call Order Summary

### Normal Object Update (Non-Audit Event)

```
1. CLI Command: `zqk object update OBJ-123 --field '...'`
2. FileObjectStorage.Update():
   a. Read existing object
   b. Capture previousStateForJournal
   c. Merge updates
   d. Write file
   e. Update hash registry
   f. Update cache
   g. RecordObjectStateChange()
   h. createChangeJournalEntry():
      - Validate (spec, lifecycle - references skipped)
      - Write CHA-XXX
      - Update hash registry
3. PersistentPostRunE:
   - Create audit event (if command execution)
```

### Audit Event Update (Self-Reference)

```
1. CLI Command: `zqk object update AUD-32 --field '...'`
2. FileObjectStorage.Update():
   a. Read AUD-32
   b. Capture previousStateForJournal
   c. Merge updates
   d. Write AUD-32
   e. Update hash registry
   f. Update cache
   g. RecordObjectStateChange()
   h. createChangeJournalEntry():
      - object_ref: "audit_event:AUD-32"
      - Validate: Spec, lifecycle (references skipped)
      - Write CHA-XXX
3. PersistentPostRunE:
   - Create new audit event for this command (AUD-33)
```

### System Check Flow

```
1. BuildCache() or LoadCache()
2. For each object:
   a. checkRegistration()
   b. checkIntegrity()
   c. checkInstanceValidation()
   d. checkLifecycle()
   e. checkPolicy()
   f. checkReferencesWithCache():
      - Parse "kind:id" references
      - Special handling for audit events in change journal entries
      - Look up in cache
      - Validate existence
```

## Validation Timing Matrix

| Operation | When Validation Happens | What is Validated | Can Reference Exist? | Status |
|-----------|------------------------|-------------------|---------------------|--------|
| Object Update | During update (before write) | Object spec, lifecycle | Yes (object exists) | ✅ Working |
| Change Journal Creation | Before writing entry | Spec, lifecycle (references skipped) | Maybe (audit events may not exist yet) | ✅ Fixed |
| System Check | After all objects exist | All checks including references | Yes (all objects exist) | ✅ Working |

## Key Principles

1. **Defer Reference Validation**: Skip reference validation during change journal entry creation, defer to system check
2. **Cache Freshness**: Ensure cache is updated incrementally when objects are created/updated
3. **Self-Reference Handling**: Add special handling for audit events in change journal entries
4. **Validation Order**: Ensure validation happens at the right time - after objects exist but before they're used
5. **Best Effort**: Change journal entries are "best effort" - created even if validation fails

## Related Documentation

- [Instance Validation v1.0](./instance-validation-v1.0.md) - Instance validation architecture
- [Object Storage Provider v1.0](./object-storage-provider-v1.0.md) - Storage layer architecture
- [Hash Registry Design v1.0](./hash-registry-design-v1.0.md) - Hash registry implementation
- [Object ID Cache Invalidation](./object-id-cache-invalidation.md) - Cache invalidation strategy

## Version History

- **v1.0.0** (2025-12-29): Initial documentation of validation flow and call order

