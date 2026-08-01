# Object Creation Requirements

**Version**: 1.0.0  
**Created**: 2026-01-18  
**Status**: Active  
**Purpose**: Explicitly define requirements for creating objects, especially regarding duplicate IDs and special cases

## Core Requirement: ID Uniqueness

### Requirement 1: Create Must Fail for Duplicate IDs

**Requirement**: `storage.Create()` must return `ErrObjectExists` when attempting to create an object with an ID that already exists.

**Rationale**: 
- Object IDs are unique identifiers within a kind
- Creating duplicates would cause ambiguity and data integrity issues
- Prevents accidental overwrites of existing objects

**Implementation**:
- For CAS-enabled kinds: Check CAS index via `cas.GetHashForID(id)`
- For non-CAS kinds: Check file existence via `os.Stat(filePath)`
- If object exists → return `ErrObjectExists` immediately

**Location**: `pkg/storage/object_storage_file.go:1018-1040` (`checkObjectExists`)

**Reference**: `docs/architecture/README.md`

## Special Cases

### Special Case 1: One-Time Scheduler Jobs

**Requirement**: One-time scheduler jobs (`execution_mode: "one_time"`) should be **updated** instead of creating new objects when the same ID is submitted again.

**Rationale**:
- One-time jobs are meant to be run multiple times with potentially different commands/args
- Creating new objects for each run would create hundreds of orphaned files
- The same logical job (identified by ID) should be reused across runs

**Behavior**:
1. Before calling `Create()`, check if a job with the same ID exists
2. If exists and `execution_mode == "one_time"`:
   - Call `Update()` instead of `Create()`
   - Update fields: `enabled`, `updated_at`, `updated_by`, `command`, `command_args`, `title`, `description`, `max_runtime_seconds`, `working_directory`, `environment_variables`
   - Generate audit event (via `storage.Update()`)
   - Create change journal entry (via `storage.Update()`)
3. If exists and `execution_mode != "one_time"`:
   - Return error: "scheduler job with ID X already exists (execution_mode: Y). Use a different ID or update the existing job"
4. If not exists:
   - Proceed with normal `Create()`

**Implementation Locations**:
- `cmd/zqk/scheduler/submit.go:216-287` - `submitJob()` function
- `cmd/zqk/system/auto_fix_scheduler_batch.go:182-226` - `createAutoFixJob()` function

**Note**: This is a **special case** for scheduler jobs only. All other object kinds must follow the general rule (Create fails for duplicate IDs).

### Special Case 1b: Reusable Scheduler Jobs - Normal Operation Updates

**Requirement**: Reusable scheduler jobs (`execution_mode: "reusable"`) **must be updated in place** during normal operation when the scheduler updates execution metadata.

**Rationale**:
- Reusable jobs run on a schedule (e.g., every 2 minutes) and need to track execution state
- Each run updates `last_run_at`, `next_run_at`, and `updated_at` timestamps
- These are **operational updates**, not new object creation attempts
- The same logical job (identified by ID) must be updated, not replaced

**Behavior During Normal Operation**:
1. Scheduler calls `storage.Update()` to update execution metadata after each run
2. For CAS-enabled kinds (like `scheduler_job`):
   - Content changes (new timestamps) → new hash calculated
   - New hash file created with updated content
   - CAS index updated to point to new hash
   - **Old hash file MUST be cleaned up immediately** after successful index update
3. Cleanup is handled by `OrphanCleanupCallback` in `CAS.Update()`
4. This prevents accumulation of orphaned hash files

**Expected Update Frequency**:
- Reusable jobs update on every execution (e.g., SCH-015 runs every 2 minutes)
- High-frequency updates are expected and normal
- Cleanup callback ensures no orphaned files accumulate

**Implementation Locations**:
- `pkg/scheduler/scheduler.go:1109-1147` - `updateJobInStorage()` function
- `pkg/storage/content_addressable_storage.go:339-425` - `CAS.Update()` with cleanup callback
- `pkg/storage/object_storage_file.go:326-334` - Cleanup callback registration

**Critical Requirement**: The cleanup callback **must** be called after every successful CAS update to prevent orphaned files. This is implemented and active for all CAS-enabled kinds.

**Note**: This is different from Special Case 1 (one_time jobs). One-time jobs are updated when **submitted again** by a user. Reusable jobs are updated **automatically by the scheduler** during normal operation.

### Special Case 2: CAS Create with Same ID, Same Content

**Behavior**: When `cas.Create()` is called with the same ID and identical content:
- Uses the same hash file (deduplication)
- Index mapping unchanged
- No error returned (silent deduplication)
- This is acceptable because content is identical

**Reference**: `docs/architecture/README.md:210-213`

### Special Case 3: CAS Create with Same ID, Different Content

**Behavior**: When `cas.Create()` is called with the same ID but different content:
- Creates new hash file with new content
- Updates index mapping to point to new hash
- Old hash file remains on disk (orphaned)
- **This should not happen** - `FileObjectStorage.Create()` prevents this by checking for duplicates first

**Note**: Direct calls to `cas.Create()` bypass duplicate checking. Always use `FileObjectStorage.Create()` which enforces ID uniqueness.

**Reference**: `docs/architecture/README.md:215-219`

## Operation Executor Behavior

**Location**: `pkg/storage/operation_executor.go:422-477`

**Behavior**: The operation executor checks for existing objects before creating:
```go
_, err := e.storage.Read(ctx, secCtx, op.ObjectID)
if err == nil || err != ErrObjectNotFound {
    // Object exists - conflict detected
    // Can use conflict resolver strategies: Skip, Reject, or Merge (treat as update)
}
```

**Conflict Resolution Strategies**:
- `StrategySkip`: Silently skip (idempotent)
- `StrategyReject`: Return error (default)
- `StrategyMerge`: Treat as update operation

**Note**: The conflict resolver is optional. If not provided, defaults to rejecting duplicates.

## Current Implementation Status

### ✅ Implemented

1. **FileObjectStorage.Create()** - Checks for duplicates (CAS and non-CAS)
2. **One-time scheduler jobs** - Update existing instead of creating duplicates
3. **Operation executor** - Checks for conflicts before creating
4. **CAS index checking** - Prevents duplicate IDs for CAS-enabled kinds

### ⚠️ Edge Cases

1. **Direct CAS.Create() calls** - Bypass duplicate checking
   - **Recommendation**: Always use `FileObjectStorage.Create()` which enforces uniqueness
   - **Risk**: Direct CAS calls could create duplicates if not careful

2. **Concurrent creates** - Race condition possible
   - **Current**: Each create checks independently
   - **Risk**: Two concurrent creates with same ID could both pass the check
   - **Mitigation**: CAS index updates are serialized via write queue

## Requirements Summary

| Scenario | Required Behavior | Current Status |
|----------|------------------|----------------|
| Create with existing ID (general) | Return `ErrObjectExists` | ✅ Implemented |
| Create with existing ID (one_time scheduler_job) | Update existing object | ✅ Implemented |
| Create with existing ID (reusable scheduler_job) | Return `ErrObjectExists` | ✅ Implemented |
| Update reusable job during normal operation | Update in place, cleanup old hash file | ✅ Implemented |
| Create with new ID | Proceed with creation | ✅ Implemented |
| CAS Create with same ID, same content | Silent deduplication | ✅ Implemented |
| CAS Create with same ID, different content | Should be prevented by `FileObjectStorage.Create()` | ✅ Prevented |
| CAS Update (content changes) | Create new hash, cleanup old hash | ✅ Implemented |

## Testing Requirements

### Test Cases

1. **TestCreate_DuplicateID_General**: Creating object with existing ID should fail
2. **TestCreate_DuplicateID_OneTimeJob**: Creating one-time job with existing ID should update
3. **TestCreate_DuplicateID_ReusableJob**: Creating reusable job with existing ID should fail
4. **TestUpdate_ReusableJob_NormalOperation**: Updating reusable job during normal operation should update in place and cleanup old hash file
5. **TestCreate_NewID**: Creating object with new ID should succeed
6. **TestCAS_Create_SameID_SameContent**: CAS deduplication works correctly
7. **TestCAS_Create_SameID_DifferentContent**: Prevented by FileObjectStorage.Create()
8. **TestCAS_Update_OrphanCleanup**: CAS update with content change should cleanup old hash file

### Existing Tests

- `pkg/storage/cas_duplicate_id_test.go` - Tests CAS duplicate ID behavior
- `pkg/storage/cas_cli_operations_test.go` - Tests duplicate ID prevention via CLI
- `pkg/storage/object_storage_overwrite_test.go` - Tests general duplicate prevention

## References

- `docs/architecture/README.md` - CAS duplicate ID analysis
- `pkg/storage/object_storage_file.go` - Main storage implementation
- `cmd/zqk/scheduler/submit.go` - Scheduler job submission (special case)
- `cmd/zqk/system/auto_fix_scheduler_batch.go` - Auto-fix job creation (special case)
