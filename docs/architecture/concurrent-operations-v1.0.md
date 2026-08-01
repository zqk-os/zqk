# Concurrent Operations System

**Version**: 1.0.0  
**Created**: 2026-01-01  
**Status**: Active  
**Purpose**: Handle concurrent create/update/delete operations with eventual consistency and user notifications

## Overview

The system must handle multiple concurrent operations (create, update, delete) on objects while ensuring:
- **Efficiency**: Non-blocking operations where possible
- **Accuracy**: No data corruption or lost updates
- **Eventual Consistency**: All operations eventually complete correctly
- **User Notifications**: Real-time progress and status updates

## Architecture

### Core Components

1. **Operation Queue**: Manages pending operations with priority
2. **Conflict Detector**: Identifies conflicting operations
3. **Conflict Resolver**: Resolves conflicts using defined strategies
4. **Operation Executor**: Executes operations with retry logic
5. **Notification System**: Sends progress updates to CLI users
6. **Consistency Checker**: Verifies eventual consistency

### Operation Lifecycle

```
Operation Request
    ↓
[Queue Operation] → Priority Assignment
    ↓
[Conflict Detection] → Check for conflicts
    ↓
[Conflict Resolution] → Apply resolution strategy
    ↓
[Execute Operation] → With retry logic
    ↓
[Cascade Updates] → Update dependent objects
    ↓
[Notify User] → Progress/status update
    ↓
[Verify Consistency] → Ensure eventual consistency
```

## Operation Types

### Create Operation
- **Conflict**: Object already exists
- **Resolution**: 
  - `REJECT`: Return error (default)
  - `UPDATE`: Treat as update operation
  - `SKIP`: Silently skip (idempotent)

### Update Operation
- **Conflict**: Object modified since read (version conflict)
- **Resolution**:
  - `RETRY`: Re-read and re-apply update (default)
  - `MERGE`: Merge changes intelligently
  - `REJECT`: Return error if conflict persists

### Delete Operation
- **Conflict**: Object deleted or modified
- **Resolution**:
  - `SKIP`: Silently skip if already deleted (idempotent)
  - `REJECT`: Return error if modified

### Cascade Update Operation
- **Triggered by**: Parent object deletion/update
- **Type**: Automatic, background operation
- **Conflict**: Dependent object modified during cascade
- **Resolution**: Retry with exponential backoff

## Conflict Detection

### Version-Based Conflicts
- Each object has `updated_at` timestamp
- Compare timestamps before write
- Detect if object changed since read

### Dependency Conflicts
- Object deleted while dependent is being updated
- Reference removed while cascade update in progress
- Circular dependency detection

### Concurrent Modification Conflicts
- Multiple operations on same object
- Same field updated by different operations
- Reference list modified concurrently

## Conflict Resolution Strategies

### 1. Last-Write-Wins (LWW)
- **Use case**: Non-critical fields, timestamps
- **Behavior**: Most recent update wins
- **Risk**: May lose updates

### 2. First-Write-Wins (FWW)
- **Use case**: Critical fields, required references
- **Behavior**: First update succeeds, others retry
- **Risk**: May cause retry storms

### 3. Merge Strategy
- **Use case**: Independent field updates
- **Behavior**: Merge non-conflicting changes
- **Risk**: Complex merge logic

### 4. Retry with Backoff
- **Use case**: Transient conflicts
- **Behavior**: Retry operation with exponential backoff
- **Risk**: May delay completion

### 5. Queue and Serialize
- **Use case**: Critical operations
- **Behavior**: Queue operations, execute serially
- **Risk**: May reduce concurrency

## Operation Queue

### Priority Levels

1. **CRITICAL**: Cascade updates, required reference fixes
2. **HIGH**: User-initiated operations
3. **NORMAL**: Background operations
4. **LOW**: Cleanup, optimization

### Queue Structure

```go
type Operation struct {
    ID          string
    Type        string // create, update, delete, cascade
    ObjectID    string
    ObjectKind  string
    Priority    int
    RetryCount  int
    MaxRetries  int
    Status      string // pending, running, completed, failed, retrying
    CreatedAt   time.Time
    StartedAt   *time.Time
    CompletedAt *time.Time
    Data        map[string]any
    Updates     map[string]any
    Error       error
}
```

## User Notifications

### Notification Types

1. **Progress Updates**: Operation progress percentage
2. **Status Changes**: Operation status (pending → running → completed)
3. **Conflicts Detected**: Warning about conflicts
4. **Conflicts Resolved**: Info about resolution
5. **Errors**: Operation failures
6. **Completion**: Operation completed successfully

### Notification Channels

1. **CLI Output**: Real-time progress bars, status messages
2. **Event Stream**: JSON-RPC notifications (for MCP)
3. **Log Files**: Structured logging
4. **Status Files**: Operation status persisted to disk

### Notification Format

```json
{
  "type": "operation_progress",
  "operation_id": "op-123",
  "operation_type": "update",
  "object_id": "GOAL-001",
  "status": "running",
  "progress": 75,
  "message": "Updating GOAL-001 and 3 dependent objects...",
  "timestamp": "2026-01-01T12:00:00Z"
}
```

## Eventual Consistency Guarantees

### Consistency Levels

1. **Immediate**: Operation completes before returning (synchronous)
2. **Eventual**: Operation completes asynchronously, consistency achieved eventually
3. **Causal**: Operations maintain causal ordering

### Consistency Checks

1. **Reference Integrity**: All references point to existing objects
2. **Cascade Completeness**: All cascade updates completed
3. **Version Consistency**: No version conflicts remain
4. **Dependency Completeness**: All dependencies satisfied

### Consistency Recovery

1. **Background Reconciliation**: Periodic checks for inconsistencies
2. **On-Demand Repair**: Repair inconsistencies when detected
3. **Validation Pass**: System check validates consistency

## Implementation Plan

### Phase 1: Operation Queue
- Implement priority queue for operations
- Add operation status tracking
- Basic conflict detection

### Phase 2: Conflict Resolution
- Implement resolution strategies
- Add retry logic with backoff
- Merge strategy for independent updates

### Phase 3: Notification System
- CLI progress reporting
- Event stream integration
- Status file persistence

### Phase 4: Consistency Guarantees
- Background reconciliation
- Consistency verification
- Automatic repair

### Phase 5: Performance Optimization
- Parallel operation execution
- Batch operations
- Cache optimization

## Example Scenarios

### Scenario 1: Concurrent Updates

**Operation A**: Update `GOAL-001.title = "New Title"`
**Operation B**: Update `GOAL-001.description = "New Description"`

**Resolution**: Merge strategy - both updates succeed (different fields)

### Scenario 2: Delete During Update

**Operation A**: Update `GOAL-001`
**Operation B**: Delete `GOAL-001`

**Resolution**: 
- If delete happens first: Update fails with "object not found"
- If update happens first: Delete succeeds, update is lost (acceptable for delete)

### Scenario 3: Cascade Update Conflict

**Operation A**: Delete `CRIT-001` (triggers cascade)
**Operation B**: Update `TEST-001.criteria_refs` (adds CRIT-001)

**Resolution**: 
- Cascade update removes CRIT-001 from TEST-001
- Operation B's update is applied after cascade
- Final state: TEST-001.criteria_refs without CRIT-001 (correct)

### Scenario 4: Version Conflict

**Operation A**: Read `GOAL-001` at T1
**Operation B**: Update `GOAL-001` at T2
**Operation A**: Update `GOAL-001` at T3 (based on T1 state)

**Resolution**: 
- Detect version conflict (updated_at mismatch)
- Retry Operation A with latest state
- Apply update to latest version

## Performance Considerations

1. **Parallel Execution**: Execute independent operations in parallel
2. **Batch Operations**: Group related operations for efficiency
3. **Lazy Cascade**: Defer cascade updates to background
4. **Cache Coherency**: Invalidate cache on updates
5. **Lock Granularity**: Minimize lock scope and duration

## Error Handling

1. **Transient Errors**: Retry with exponential backoff
2. **Permanent Errors**: Fail operation, notify user
3. **Partial Failures**: Complete successful operations, report failures
4. **Recovery**: Automatic retry of failed operations

