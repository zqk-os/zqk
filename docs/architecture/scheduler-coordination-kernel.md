# Scheduler Coordination Kernel - Distributed Job Awareness

## Vision

A **knowledge kernel** that provides centralized awareness of all job execution across all scheduler processes, enabling:
- **Total system awareness**: Know what jobs are running, queued, deferred across all processes
- **Policy-driven execution**: Policies determine skip, defer, scheduled-defer, execute
- **Evolvable structure**: Policies and standards can evolve without code changes
- **Synchronized coordination**: All processes share the same view of system state

## Architecture

### Core Components

```
┌─────────────────────────────────────────────────────────────┐
│              Job Coordination Kernel                         │
│  (Shared State: File-based or Graph-based)                  │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Job State Registry                                 │   │
│  │  - Tracks all job executions across all processes   │   │
│  │  - State: pending, in_progress, completed, failed   │   │
│  │  - Process ID, start time, expected duration        │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                               │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Policy Engine                                       │   │
│  │  - Evaluates execution policies                      │   │
│  │  - Decisions: execute, skip, defer, scheduled-defer │   │
│  │  - Considers: job state, system load, dependencies  │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                               │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Coordination Channel                                │   │
│  │  - Event stream: job_started, job_completed, etc.  │   │
│  │  - All processes subscribe to same channel         │   │
│  │  - File-based or graph-based event log              │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                               │
└─────────────────────────────────────────────────────────────┘
         ▲                    ▲                    ▲
         │                    │                    │
    ┌────┴────┐         ┌────┴────┐         ┌────┴────┐
    │Process 1│         │Process 2│         │Process N│
    │Scheduler│         │Scheduler│         │Scheduler│
    └─────────┘         └─────────┘         └─────────┘
```

### 1. Job State Registry

**Purpose**: Centralized tracking of all job executions across all processes

**Storage**: 
- File-based: `.zqk/scheduler/state/{jobID}.state`
- Graph-based: `scheduler_job_execution` nodes with relationships

**State Model**:
```yaml
job_execution_state:
  job_id: SCH-001
  execution_id: exec-{timestamp}-{pid}
  state: in_progress | completed | failed | deferred | skipped
  process_id: 12345
  started_at: 2026-01-15T10:00:00Z
  expected_duration_seconds: 300
  actual_duration_seconds: null
  policy_decision: execute | skip | defer | scheduled-defer
  policy_reason: "System load too high"
  dependencies: [SCH-002, SCH-003]
  retry_count: 0
  max_retries: 3
```

**Operations**:
- `RegisterExecution(jobID, executionID, processID)` - Register job start
- `GetExecutionState(jobID)` - Get current state
- `ListInProgress()` - List all in-progress jobs
- `CompleteExecution(executionID, result)` - Mark job complete
- `DeferExecution(jobID, reason, deferUntil)` - Defer job execution

### 2. Policy Engine

**Purpose**: Evaluate execution policies to determine job behavior

**Policy Types**:
1. **Execution Policy**: Should job execute now?
   - Check: Is job already running? (skip)
   - Check: Are dependencies satisfied? (defer)
   - Check: System load acceptable? (defer if high)
   - Check: Time window restrictions? (scheduled-defer)

2. **Concurrency Policy**: Can job run concurrently?
   - Check: `concurrent_allowed` flag
   - Check: Number of concurrent instances
   - Check: Resource limits

3. **Deferral Policy**: When should deferred job run?
   - Immediate retry (after dependency completes)
   - Scheduled deferral (run at specific time)
   - Exponential backoff (retry with increasing delay)

**Policy Configuration**:
```yaml
execution_policy:
  job_id: SCH-001
  rules:
    - type: dependency_check
      dependencies: [SCH-002]
      action: defer
    - type: concurrency_check
      max_concurrent: 1
      action: skip_if_running
    - type: system_load_check
      max_load: 0.8
      action: defer_if_high
  default_action: execute
```

**Policy Evaluation Flow**:
```
1. Check if job already in progress → skip
2. Check dependencies → defer if not satisfied
3. Check concurrency limits → skip if exceeded
4. Check system load → defer if too high
5. Check time windows → scheduled-defer if outside window
6. Default → execute
```

### 3. Coordination Channel

**Purpose**: Event stream for job coordination across processes

**Event Types**:
- `job_execution_requested` - Job scheduled to run
- `job_execution_started` - Job started executing
- `job_execution_completed` - Job completed successfully
- `job_execution_failed` - Job failed
- `job_execution_deferred` - Job deferred
- `job_execution_skipped` - Job skipped
- `job_execution_cancelled` - Job cancelled

**Event Structure**:
```yaml
event:
  type: job_execution_started
  timestamp: 2026-01-15T10:00:00Z
  job_id: SCH-001
  execution_id: exec-12345-67890
  process_id: 12345
  policy_decision: execute
  metadata:
    expected_duration: 300
    dependencies: []
```

**Channel Implementation**:
- **File-based**: Append-only event log (`.zqk/scheduler/events/coordination-bus.jsonl`)
- **Graph-based**: Event nodes with timestamps
- **Processes**: Tail/watch the event log for new events

**Event Processing**:
```go
type EventProcessor struct {
    stateRegistry *JobStateRegistry
    policyEngine  *PolicyEngine
    eventChannel  chan Event
}

func (ep *EventProcessor) ProcessEvent(event Event) {
    switch event.Type {
    case "job_execution_requested":
        decision := ep.policyEngine.Evaluate(event.JobID)
        if decision.Action == "execute" {
            ep.stateRegistry.RegisterExecution(...)
            ep.PublishEvent("job_execution_started", ...)
        } else {
            ep.stateRegistry.DeferExecution(...)
            ep.PublishEvent("job_execution_deferred", ...)
        }
    case "job_execution_completed":
        ep.stateRegistry.CompleteExecution(...)
        ep.PublishEvent("job_execution_completed", ...)
        // Check if any deferred jobs can now run
        ep.CheckDeferredJobs(event.JobID)
    }
}
```

## Implementation Design

### Phase 1: Job State Registry (Foundation)

**File**: `pkg/scheduler/job_state_registry.go`

```go
type JobStateRegistry struct {
    projectRoot string
    stateDir    string
    mu          sync.RWMutex
}

type JobExecutionState struct {
    JobID              string
    ExecutionID        string
    State              string // in_progress, completed, failed, deferred, skipped
    ProcessID          int
    StartedAt          time.Time
    ExpectedDuration   time.Duration
    ActualDuration     *time.Duration
    PolicyDecision     string
    PolicyReason       string
    Dependencies       []string
    RetryCount         int
    MaxRetries         int
    DeferUntil         *time.Time
}

func (r *JobStateRegistry) RegisterExecution(jobID, executionID string, processID int) error
func (r *JobStateRegistry) GetExecutionState(jobID string) (*JobExecutionState, error)
func (r *JobStateRegistry) ListInProgress() ([]*JobExecutionState, error)
func (r *JobStateRegistry) CompleteExecution(executionID string, result string) error
func (r *JobStateRegistry) DeferExecution(jobID string, reason string, deferUntil *time.Time) error
```

**Storage**:
- File: `.zqk/scheduler/state/{jobID}-{executionID}.yaml`
- Lock: Use file locking for atomic updates
- Cleanup: Remove completed executions after retention period

### Phase 2: Policy Engine

**File**: `pkg/scheduler/policy_engine.go`

```go
type PolicyEngine struct {
    stateRegistry *JobStateRegistry
    policies      map[string]*ExecutionPolicy
}

type ExecutionPolicy struct {
    JobID        string
    Rules        []PolicyRule
    DefaultAction string
}

type PolicyRule struct {
    Type        string // dependency_check, concurrency_check, system_load_check, time_window_check
    Condition   map[string]any
    Action      string // execute, skip, defer, scheduled-defer
    Reason      string
}

type ExecutionDecision struct {
    Action      string // execute, skip, defer, scheduled-defer
    Reason      string
    DeferUntil  *time.Time
    RetryAfter  *time.Duration
}

func (pe *PolicyEngine) Evaluate(jobID string, job *ScheduledJob) (*ExecutionDecision, error)
func (pe *PolicyEngine) LoadPolicy(jobID string) (*ExecutionPolicy, error)
func (pe *PolicyEngine) SavePolicy(policy *ExecutionPolicy) error
```

**Policy Storage**:
- File: `.zqk/scheduler/policies/{jobID}.yaml`
- Graph: `execution_policy` objects linked to `scheduler_job`

### Phase 3: Coordination Channel

**File**: `pkg/scheduler/coordination_channel.go`

```go
type CoordinationChannel struct {
    projectRoot string
    eventLog    string
    watchers    []chan Event
    mu          sync.RWMutex
}

type Event struct {
    Type        string
    Timestamp   time.Time
    JobID       string
    ExecutionID string
    ProcessID   int
    PolicyDecision string
    Metadata    map[string]any
}

func (cc *CoordinationChannel) PublishEvent(event Event) error
func (cc *CoordinationChannel) Subscribe() <-chan Event
func (cc *CoordinationChannel) WatchEvents(ctx context.Context) error
```

**Event Log**:
- File: `.zqk/scheduler/events/coordination-bus.jsonl` (append-only)
- Format: JSON Lines (one event per line)
- Rotation: Daily or size-based
- Index: For fast lookups by jobID, timestamp

### Phase 4: Integration with Scheduler

**Modified**: `pkg/scheduler/scheduler.go`

```go
type Scheduler struct {
    // ... existing fields ...
    stateRegistry      *JobStateRegistry
    policyEngine       *PolicyEngine
    coordinationChannel *CoordinationChannel
}

func (s *Scheduler) executeJob(ctx context.Context, job *ScheduledJob, handler JobHandler) {
    // 1. Request execution (publish event)
    event := Event{
        Type: "job_execution_requested",
        JobID: job.ID,
        ProcessID: os.Getpid(),
    }
    s.coordinationChannel.PublishEvent(event)
    
    // 2. Evaluate policy
    decision, err := s.policyEngine.Evaluate(job.ID, job)
    if err != nil {
        s.logger.Error("Policy evaluation failed", ...)
        return
    }
    
    // 3. Execute decision
    switch decision.Action {
    case "skip":
        s.logger.Info("Job skipped", logging.String("reason", decision.Reason))
        s.coordinationChannel.PublishEvent(Event{
            Type: "job_execution_skipped",
            JobID: job.ID,
            PolicyReason: decision.Reason,
        })
        return
        
    case "defer":
        deferUntil := decision.DeferUntil
        if deferUntil == nil {
            deferUntil = time.Now().Add(*decision.RetryAfter)
        }
        s.stateRegistry.DeferExecution(job.ID, decision.Reason, deferUntil)
        s.coordinationChannel.PublishEvent(Event{
            Type: "job_execution_deferred",
            JobID: job.ID,
            PolicyReason: decision.Reason,
            DeferUntil: deferUntil,
        })
        return
        
    case "scheduled-defer":
        s.stateRegistry.DeferExecution(job.ID, decision.Reason, decision.DeferUntil)
        s.coordinationChannel.PublishEvent(Event{
            Type: "job_execution_scheduled_deferred",
            JobID: job.ID,
            PolicyReason: decision.Reason,
            DeferUntil: decision.DeferUntil,
        })
        return
        
    case "execute":
        // Continue with execution
    }
    
    // 4. Register execution
    executionID := fmt.Sprintf("exec-%d-%d", time.Now().Unix(), os.Getpid())
    s.stateRegistry.RegisterExecution(job.ID, executionID, os.Getpid())
    
    // 5. Publish start event
    s.coordinationChannel.PublishEvent(Event{
        Type: "job_execution_started",
        JobID: job.ID,
        ExecutionID: executionID,
        ProcessID: os.Getpid(),
    })
    
    // 6. Execute job
    err = handler.Execute(ctx, job)
    
    // 7. Complete execution
    if err != nil {
        s.stateRegistry.CompleteExecution(executionID, "failed")
        s.coordinationChannel.PublishEvent(Event{
            Type: "job_execution_failed",
            JobID: job.ID,
            ExecutionID: executionID,
            Error: err.Error(),
        })
    } else {
        s.stateRegistry.CompleteExecution(executionID, "completed")
        s.coordinationChannel.PublishEvent(Event{
            Type: "job_execution_completed",
            JobID: job.ID,
            ExecutionID: executionID,
        })
    }
}
```

## Benefits

1. **Total System Awareness**: All processes see the same job state
2. **Policy-Driven**: Execution decisions based on configurable policies
3. **Evolvable**: Policies can change without code changes
4. **Synchronized**: Event stream keeps all processes in sync
5. **Flexible**: Supports skip, defer, scheduled-defer, execute
6. **Knowledge Kernel**: Centralized coordination with distributed execution

## Migration Path

### Step 1: Job State Registry
- Implement file-based state tracking
- Add atomic operations with file locking
- Test with single process

### Step 2: Policy Engine
- Implement basic policies (dependency, concurrency)
- Load policies from files
- Test policy evaluation

### Step 3: Coordination Channel
- Implement event log
- Add event publishing
- Add event subscription/watching

### Step 4: Integration
- Integrate with scheduler
- Replace ConflictManager with JobStateRegistry
- Test with multiple processes

### Step 5: Advanced Policies
- Add system load checking
- Add time window restrictions
- Add retry policies

## Questions to Answer

1. **Storage Backend**: File-based or graph-based?
   - File-based: Simpler, works with current architecture
   - Graph-based: More powerful, better for complex relationships

2. **Event Channel**: Real-time or eventual consistency?
   - Real-time: More complex, requires polling/watching
   - Eventual: Simpler, check state before execution

3. **Policy Storage**: Separate files or in scheduler_job spec?
   - Separate files: More flexible, easier to evolve
   - In spec: Tighter coupling, but simpler

4. **Performance**: How to handle high-frequency events?
   - Batching: Batch events for efficiency
   - Indexing: Index event log for fast lookups
   - Rotation: Rotate event log to prevent growth

