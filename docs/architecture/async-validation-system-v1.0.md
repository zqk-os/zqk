# Async Validation System v1.0

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Created:** 2026-01-01  
**Status:** Active  
**Purpose:** Optimize system check command with async validation, priority-based execution, and cached state

## Summary

The async validation system transforms the system check command from a synchronous, blocking operation into an eventually consistent, priority-based validation system that leverages caching and background processing.

## Problem Statement

The system check command is vital for maintaining system coherency and integrity, but:
- It grows complex and takes too long to run serially
- All validations run synchronously, blocking the user
- No caching means redundant validation work
- No prioritization means critical issues may be discovered late
- No progress feedback during long-running checks

## Solution Architecture

### Components

1. **Validation State Cache** (`pkg/validation/state_cache.go`)
   - Stores validation results with timestamps
   - Tracks file checksums to detect changes
   - Provides fast lookup for cached validation states
   - Supports invalidation by object ID, kind, or pattern

2. **Priority Queue** (`pkg/validation/priority_queue.go`)
   - Implements heap-based priority queue
   - Prioritizes by Tier (1 = highest, 4 = lowest)
   - Handles retry logic for failed validations
   - Thread-safe for concurrent access

3. **Async Validator** (`pkg/validation/async_validator.go`)
   - Manages worker pool for parallel validation
   - Enqueues validation tasks by priority
   - Provides progress reporting via channels
   - Handles file checksum validation for cache hits

4. **Check Integration** (`cmd/zqk/system/async_check.go`)
   - Bridges check command with async validator
   - Provides `--cache-only` mode for fast checks
   - Shows incremental progress during validation
   - Uses cached state when available

### Priority System

Validations are prioritized by Tier:
- **Priority 1 (Highest)**: Tier 1 (blocking) issues
- **Priority 2**: Tier 2 (warnings) issues
- **Priority 3**: Tier 3 (informational) issues
- **Priority 4 (Lowest)**: Tier 4 (recommendations)

Objects with previous Tier 1 issues are automatically prioritized.

### Validation Flow

```
1. Check Command Invoked
   ↓
2. Load Validation State Cache
   ↓
3. For each object:
   - Check cache for fresh validation state
   - If cache hit and file unchanged → use cached result
   - If cache miss or file changed → enqueue validation task
   ↓
4. Start Async Validator Workers (if not running)
   ↓
5. Workers process tasks by priority:
   - Tier 1 → Tier 2 → Tier 3 → Tier 4
   - Update cache with results
   - Send progress updates
   ↓
6. Display results from cache + live updates
```

### Cache Strategy

- **Cache Location**: `.zqk/validation_cache.json`
- **Max Age**: 1 hour (configurable)
- **Invalidation Triggers**:
  - File checksum mismatch (file changed)
  - Cache entry older than max age
  - Manual invalidation by object ID/kind/pattern
  - Object updates via CLI

### Progress Reporting

The async validator provides real-time progress updates:
- Total tasks queued
- Completed tasks
- Current tier being processed
- Current object being validated
- Status (queued, validating, completed, error)

## Usage

### Basic Check (Uses Cache + Async Validation)

```bash
zqk system check
```

This will:
1. Load cached validation states
2. Enqueue only objects that need validation
3. Show progress as validation completes
4. Display results from cache + live updates

### Cache-Only Check (Fast, No Validation)

```bash
zqk system check --cache-only
```

This will:
1. Load cached validation states
2. Display results from cache only
3. No new validation performed
4. Fast response time

### Force Full Validation

```bash
zqk system check --refresh-cache
```

This will:
1. Invalidate all cached states
2. Enqueue all objects for validation
3. Perform full validation run

## Configuration

### Worker Count

Default: 4 workers

To change:
```go
validator := validation.NewAsyncValidator(projectRoot, workers, maxCacheAge)
```

### Cache Max Age

Default: 1 hour

To change:
```go
validator := validation.NewAsyncValidator(projectRoot, workers, maxCacheAge)
```

## Background Validation

The async validator can run continuously in the background:

1. **Start Validator**:
   ```go
   validator := GetAsyncValidator(projectRoot)
   validator.Start()
   ```

2. **Enqueue Tasks**:
   ```go
   validator.Enqueue(objectID, objectKind, filePath, priority)
   ```

3. **Monitor Progress**:
   ```go
   progressChan := validator.GetProgress()
   for progress := range progressChan {
       // Handle progress updates
   }
   ```

4. **Stop Validator** (optional - can run continuously):
   ```go
   validator.Stop()
   ```

## Integration with Scheduler

The async validator can be integrated with the scheduler system for periodic background validation:

```yaml
# scheduler_job for background validation
id: SCH-XXX
kind: scheduler_job
job_type: run_wrapper
trigger_type: timer
schedule_expression: "0 */6 * * *"  # Every 6 hours
command: zqk
command_args:
  - system
  - check
  - --background
```

## Benefits

1. **Faster Response**: Cache hits return immediately
2. **Prioritized Execution**: Critical issues validated first
3. **Non-Blocking**: User sees progress, not frozen UI
4. **Eventually Consistent**: System converges to correct state
5. **Scalable**: Worker pool handles large object counts
6. **Efficient**: Only validates changed or stale objects

## Future Enhancements

1. **Scheduler Integration**: Automatic background validation jobs
2. **Event-Driven**: Invalidate cache on object updates
3. **Distributed Validation**: Multi-node validation for large systems
4. **Validation Metrics**: Track validation performance and trends
5. **Smart Caching**: Predictive cache warming for frequently accessed objects

## Related Documentation

- [System Check Architecture](./system-check-architecture-v1.0.md)
- [Object ID Cache](./object-id-cache-v1.0.md)
- [Scheduler System](../scheduler/recommended-jobs.md)

