# Scheduler Concurrency & Access Control - Executive Summary

**Last Verified:** 2026-08-31


## Current State Analysis

### ✅ What We Have

1. **Single Scheduler Instance Prevention**
   - PID file prevents multiple scheduler daemons
   - Process-local state (`jobs` map, `ConflictManager`)
   - Each scheduler loads jobs from shared storage

2. **Process-Local Concurrency Control**
   - `ConflictManager`: for most job types, prevents the same `job_type` from running concurrently; **`run_wrapper` is per `scheduler_job` id only** (different job ids may overlap). `ConcurrentAllowed` still applies for testing bundles and similar.
   - Default: non-concurrent for safety where the same-type rule applies

3. **Permission Checks**
   - `manage:scheduler` - start/stop daemon
   - `read:scheduler_job` - load jobs
   - `execute:scheduler_job` - execute jobs
   - `write:scheduler_job` - create/update jobs
   - `delete:scheduler_job` - delete jobs

### ❌ Critical Gaps

1. **No Cross-Process Job Execution Locking**
   - If two schedulers somehow run (race condition, PID file bypass), both execute same jobs
   - No file-based or distributed locking
   - Risk: Duplicate executions, data corruption, resource contention

2. **System Security Context Too Permissive**
   - Scheduler defaults to `NewSystemSecurityContext()` = admin role
   - All jobs run with full system privileges
   - No role-based job filtering
   - MCP profile permissions are ignored

3. **No Job-Level Access Control**
   - Can't specify different security contexts per job
   - Can't restrict which users/roles can execute specific jobs
   - All jobs inherit scheduler's system context

4. **Hardcoded Concurrency Rules**
   - `isConcurrentAllowed()` is hardcoded per job type
   - Can't configure per-job concurrency settings
   - `ConcurrentAllowed` field exists but is auto-set, not from job spec

## Answers to Your Questions

### Q1: Does each process have its own view of scheduled jobs?
**A**: Yes, but they all read from the same storage. Each scheduler instance:
- Independently loads all enabled `scheduler_job` objects from storage
- Maintains its own `jobs` map (process-local)
- Schedules jobs independently
- **Problem**: If two schedulers run, they both see and execute the same jobs

### Q2: Do we prevent concurrent runs?
**A**: Partially - only within a single process:
- ✅ Within one scheduler: `ConflictManager` prevents overlapping runs of the same kind (for `run_wrapper`, same **job id**; for types like `audit_event_aggregation`, same **job type** across ids)
- ❌ Across processes: NO protection - both schedulers would execute the same jobs simultaneously

### Q3: Do we have the job system disabled unless there's a role?
**A**: No - it's the opposite:
- Scheduler defaults to `NewSystemSecurityContext()` = admin role with all permissions
- No role-based restrictions
- If you can start the scheduler, you can execute all jobs

### Q4: Maybe the MCP profile is the only one with access?
**A**: No - scheduler doesn't use MCP profile:
- CLI sets security context, but scheduler daemon uses system context
- MCP profile permissions are not respected
- All jobs run with system/admin privileges

### Q5: How do we best ensure flexibility but avoid creating a parallel nightmare?
**A**: Implement distributed job execution locking + role-based access:

**Solution 1: Distributed Job Execution Locking (CRITICAL)**
- File-based locks: `.zqk/scheduler/locks/{jobID}.lock`
- Use `flock()` for cross-process coordination
- Lock acquired before execution, released after completion
- Prevents duplicate execution across processes

**Solution 2: Role-Based Scheduler Access**
- Make scheduler respect security context from CLI/MCP
- Filter jobs based on security context permissions
- Allow different users to have different job access

**Solution 3: Job-Level Security Context**
- Add `security_context` field to `scheduler_job` spec
- Jobs can specify their own security context
- Principle of least privilege per job

## Recommended Implementation Priority

### Phase 1: Immediate (Critical) - Prevent Parallel Nightmare
1. ✅ Implement distributed job execution locking
2. ✅ Add tests for cross-process prevention
3. ✅ Document locking mechanism

### Phase 2: Short-term (High Priority) - Access Control
1. Make scheduler respect CLI/MCP security context
2. Add role-based job filtering
3. Update scheduler to require explicit security context

### Phase 3: Medium-term (Nice to Have) - Flexibility
1. Add job-level security context support
2. Add configurable concurrency per job
3. Add job execution audit trail

## Quick Win: Distributed Locking Implementation

```go
// pkg/scheduler/job_lock.go
type JobLock struct {
    lockFile string
    file     *os.File
}

func AcquireJobLock(projectRoot, jobID string, timeout time.Duration) (*JobLock, error) {
    lockDir := filepath.Join(projectRoot, ".zqk", "scheduler", "locks")
    os.MkdirAll(lockDir, 0755)
    
    lockFile := filepath.Join(lockDir, jobID+".lock")
    file, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0644)
    if err != nil {
        return nil, err
    }
    
    // Acquire exclusive lock with timeout
    err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
    if err != nil {
        file.Close()
        return nil, fmt.Errorf("job %s is already running", jobID)
    }
    
    return &JobLock{lockFile: lockFile, file: file}, nil
}

func (jl *JobLock) Release() error {
    syscall.Flock(int(jl.file.Fd()), syscall.LOCK_UN)
    jl.file.Close()
    os.Remove(jl.lockFile)
    return nil
}
```

**Usage in `executeJob()`**:
```go
// Before executing job
lock, err := AcquireJobLock(s.projectRoot, job.ID, 30*time.Second)
if err != nil {
    s.logger.Warn("Job already running in another process", ...)
    return
}
defer lock.Release()

// Execute job...
```

This prevents the parallel nightmare while maintaining flexibility.

