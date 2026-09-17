# Scheduler Concurrency and Access Control Analysis

**Last Verified:** 2026-08-31


## Current State

### 1. Process Isolation
**Status**: ✅ **Partially Protected**

- **PID File Prevention**: We prevent multiple scheduler instances via PID file check
- **Process-Local State**: Each scheduler instance maintains its own `jobs` map and `ConflictManager`
- **Shared Storage**: All scheduler instances read from the same storage (file-based or graph)
- **No Cross-Process Coordination**: If two schedulers somehow run simultaneously, they would both execute the same jobs

### 2. Concurrent Run Prevention
**Status**: ⚠️ **Process-Local Only**

**Within Single Process**:
- `ConflictManager` prevents overlapping runs: for **`run_wrapper`**, only the same **`scheduler_job` id**; for other types (e.g. `audit_event_aggregation`), the same **`job_type`** across different job ids where still non-concurrent.
- Uses `ConcurrentAllowed` from `isConcurrentAllowed()` (e.g. testing `run_wrapper` bundles).
- Default: non-concurrent where the same-type rule applies; `run_wrapper` differs as above.

**Across Processes**:
- ❌ **NO protection** - If two scheduler instances run, they will both execute jobs
- ❌ **NO distributed locking** - No file-based or storage-based locks
- ❌ **NO coordination** - Each scheduler independently loads and executes jobs

### 3. Access Control
**Status**: ⚠️ **Defaults to System Context**

**Permission Checks**:
- `manage:scheduler` - Required to start/stop scheduler daemon
- `read:scheduler_job` - Required to load jobs from storage
- `execute:scheduler_job` - Required to execute jobs
- `write:scheduler_job` - Required to create/update jobs
- `delete:scheduler_job` - Required to delete jobs

**Default Security Context**:
- Scheduler defaults to `NewSystemSecurityContext()` which likely has admin privileges
- CLI commands set security context, but scheduler daemon uses system context
- **No role-based restrictions** - If you can start the scheduler, you can execute all jobs

### 4. Job Loading
**Status**: ✅ **Permission-Protected**

- Jobs are loaded from storage using scheduler's security context
- Requires `read:scheduler_job` permission
- All enabled jobs are loaded and scheduled
- Jobs are watched for changes (via `watchJobChanges()`)

## Critical Gaps

### Gap 1: No Cross-Process Job Execution Locking
**Risk**: HIGH
- If PID file check is bypassed or fails, multiple schedulers could run simultaneously
- Both would execute the same jobs, causing:
  - Duplicate audit events
  - Duplicate aggregations
  - Resource contention
  - Data corruption (if jobs modify shared state)

**Example Scenario**:
```bash
# Terminal 1
zqk scheduler start &

# Terminal 2 (before PID file is written)
zqk scheduler start &  # Could start if there's a race condition
```

### Gap 2: System Security Context Too Permissive
**Risk**: MEDIUM
- Scheduler daemon runs with system/admin privileges
- No way to restrict which jobs can be executed
- No role-based job filtering
- MCP profile might have different permissions, but scheduler doesn't respect them

### Gap 3: No Job-Level Access Control
**Risk**: MEDIUM
- Jobs are executed with scheduler's security context (system/admin)
- No way to specify different security contexts per job
- No way to restrict job execution based on user/role
- All jobs run with full system privileges

### Gap 4: Concurrent Execution Control is Hardcoded
**Risk**: LOW
- `isConcurrentAllowed()` is hardcoded per job type
- No way to configure per-job concurrency settings
- `ConcurrentAllowed` field exists but is set automatically, not from job spec

## Recommendations

### Priority 1: Distributed Job Execution Locking

**Solution**: Implement file-based locking for job execution

```go
// pkg/scheduler/job_lock.go
type JobLock struct {
    lockFile string
    file     *os.File
}

func (jl *JobLock) Acquire(jobID string, timeout time.Duration) error {
    // Create lock file: .zqk/scheduler/locks/{jobID}.lock
    // Use flock() for exclusive lock
    // Set timeout to prevent indefinite blocking
}

func (jl *JobLock) Release() error {
    // Release flock and remove lock file
}
```

**Implementation**:
1. Before executing a job, acquire a lock file
2. Lock file path: `.zqk/scheduler/locks/{jobID}.lock`
3. Use `flock()` for cross-process coordination
4. Lock is released when job completes or times out
5. Stale locks are cleaned up (check mtime, max age)

**Benefits**:
- Prevents duplicate execution across processes
- Works with file-based storage
- Can be extended to graph storage later
- Simple and reliable

### Priority 2: Role-Based Scheduler Access

**Solution**: Make scheduler respect security context from CLI/MCP

**Implementation**:
1. CLI/MCP sets security context when starting scheduler
2. Scheduler stores and uses this context for all operations
3. Jobs are filtered based on security context permissions
4. Job execution uses job-specific security context if available

**Benefits**:
- MCP profile can have restricted permissions
- Different users can have different job access
- Aligns with existing permission system

### Priority 3: Job-Level Security Context

**Solution**: Allow jobs to specify their own security context

**Implementation**:
1. Add `security_context` field to `scheduler_job` spec
2. If specified, use job's security context for execution
3. If not specified, use scheduler's security context
4. Validate permissions before scheduling

**Benefits**:
- Fine-grained access control
- Jobs can run with minimal required permissions
- Principle of least privilege

### Priority 4: Configurable Concurrency

**Solution**: Allow jobs to specify concurrency settings

**Implementation**:
1. Add `concurrent_allowed` field to `scheduler_job` spec
2. Override hardcoded `isConcurrentAllowed()` with job setting
3. Default to non-concurrent for safety

**Benefits**:
- Flexible per-job configuration
- Override defaults when needed
- Better control over resource usage

## Migration Path

### Phase 1: Immediate (Critical)
1. ✅ Implement distributed job execution locking
2. ✅ Add tests for cross-process job execution prevention
3. ✅ Document the locking mechanism

### Phase 2: Short-term (High Priority)
1. Make scheduler respect CLI/MCP security context
2. Add role-based job filtering
3. Update scheduler startup to require explicit security context

### Phase 3: Medium-term (Medium Priority)
1. Add job-level security context support
2. Add configurable concurrency per job
3. Add job execution audit trail with security context

## Testing Requirements

1. **Cross-Process Locking Tests**:
   - Test that two scheduler instances cannot execute the same job simultaneously
   - Test lock timeout and cleanup
   - Test stale lock detection

2. **Access Control Tests**:
   - Test that scheduler respects security context
   - Test that jobs are filtered based on permissions
   - Test that job execution uses correct security context

3. **Concurrency Tests**:
   - Test that non-concurrent jobs are prevented from running simultaneously
   - Test that concurrent jobs can run simultaneously
   - Test cross-process concurrency prevention

## Questions to Answer

1. **Should MCP profile be the only one with scheduler access?**
   - Current: No restriction
   - Recommendation: Make it configurable, but default to system context for CLI, MCP context for MCP

2. **Should we allow multiple schedulers for different job categories?**
   - Current: Single scheduler instance
   - Recommendation: Keep single instance, but allow job-level isolation

3. **How do we handle job execution failures and lock cleanup?**
   - Current: Locks are process-local
   - Recommendation: Use file-based locks with timeout and cleanup

