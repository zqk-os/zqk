# Agent Idle Detection & Resource Conservation Subsystem Guide (`pkg/agentidle`)

## 1. Architectural Overview & Design Philosophy

The `pkg/agentidle` subsystem delivers high-precision idle tracking and resource conservation telemetry for autonomous agents within the ZQK Knowledge Kernel. Governed by core resource conservation invariants (POL-DEFAULT-7c873b7213847d78), this package provides a lightweight, resilient, and thread-safe persistence membrane to monitor, accumulate, and persist agent idle durations.

In autonomous multi-agent swarms, unmonitored agent spinning, polling loops, and dormant task waits can consume significant CPU and token resources without driving progress. The Agent Idle Detection subsystem decouples idle accounting from heavy relational databases:
- **Zero-Latency In-Memory Accumulation:** Fast in-memory map mutations protected by fine-grained synchronization primitives.
- **Atomic Two-Phase Flush:** Writes are staged in temporary files (`<targetPath>.tmp`) and committed via POSIX-atomic file renames, eliminating partial-write race conditions and torn state anomalies.
- **Hermetic Process Security:** Enforces strict permission invariants (`0600` for storage files, `0755` for parent directories) via canonical kernel constants.
- **Transparent Legacy Migration:** Ingests legacy flat duration maps and auto-promotes them to structured `2.0.0` documents.
- **Convergence Session Integration:** Telemetry bridge feeding cumulative idle duration directly into Convergence Session (CVS) `after_state_snapshot` metrics.

---

## 2. Component Topology & Schema Architecture

The subsystem is organized around clean structural boundaries separating in-memory accounting from filesystem persistence and kernel telemetry:

```mermaid
flowchart TD
    subgraph Agent Runtime
        A1[Agent Worker 1] -->|Accumulate| FS[FileStore]
        A2[Agent Worker 2] -->|Accumulate| FS
        Obs[Observer / Waker] -->|GetIdleTime| FS
    end

    subgraph Memory & Synchronization
        FS -->|sync.Mutex| Cache[records map[string]time.Duration]
    end

    subgraph Persistence Membrane
        FS -->|save| Tmp[Write tmp file: 0600]
        Tmp -->|fileutil.Rename| Lite[idle.json: Schema 2.0.0]
    end

    subgraph Convergence Telemetry
        FS -->|GetAllRecords| CVS[RecordCVSSnapshot]
        CVS -->|zqk object update| Kernel[Kernel CVS after_state_snapshot]
    end
```

### Component Breakdown

1. **`Record`:**
   Represents a unique task binding with associated idle duration:
   ```go
   type Record struct {
       AgentID  string        `json:"agent_id"`
       TaskID   string        `json:"task_id"`
       IdleTime time.Duration `json:"idle_time"`
   }
   ```

2. **`StoreDocument` (Schema `2.0.0`):**
   The canonical disk envelope persisted as human-legible, indented JSON:
   ```go
   type StoreDocument struct {
       SchemaVersion string                   `json:"schema_version"`
       Records       map[string]time.Duration `json:"records"`
   }
   ```

3. **`FileStore`:**
   The state container managing thread-safe operations, key composition (`<agentID>:<taskID>`), disk loading, and atomic persistence.

4. **`RecordCVSSnapshot`:**
   The integration entrypoint that calculates the swarm's cumulative idle duration and updates the target Convergence Session object.

---

## 3. Idle Time Tracking Lifecycle

The accumulation and persistence lifecycle follows a strictly bounded, deterministic workflow:

```mermaid
sequenceDiagram
    participant Agent as Agent Execution Loop
    participant Store as FileStore
    participant Disk as Filesystem (OS)
    participant CVS as Convergence Session

    Agent->>Store: Accumulate(agentID, taskID, duration)
    activate Store
    Store->>Store: Lock mutex
    Store->>Store: records[key] += duration
    Store->>Store: Unlock mutex
    Store->>Store: save()
    Store->>Disk: Write <filePath>.tmp (0600)
    Store->>Disk: Rename <filePath>.tmp -> <filePath>
    Store-->>Agent: nil (success)
    deactivate Store

    Agent->>Store: GetIdleTime(agentID, taskID)
    Store-->>Agent: duration

    opt Session Finalization
        Agent->>Store: RecordCVSSnapshot(store, cvsID)
        Store->>Store: GetAllRecords() (defensive copy)
        Store->>Store: Sum total idle duration
        Store->>CVS: zqk object update --field baseline_idle --field captured_at
    end
```

### Key Lifecycle Stages

- **Initialization (`NewFileStore`):**
  - If the target file does not exist, initializes a clean, empty store in memory.
  - If the target file is empty (zero bytes), treats it as initialized and ready for writes.
  - If valid JSON exists, deserializes `StoreDocument` (`2.0.0`).
  - If deserialization into `StoreDocument` fails, attempts fallback deserialization into legacy `map[string]time.Duration`.
- **Accumulation (`Accumulate`):**
  - Synthesizes the unique record key `<agentID>:<taskID>`.
  - Mutates the in-memory duration map under mutex protection.
  - Invokes `save()` to commit the new state to durable storage.
- **Reading (`GetIdleTime` & `GetAllRecords`):**
  - `GetIdleTime`: Returns accumulated duration under read lock (returns `0` if non-existent).
  - `GetAllRecords`: Clones and returns a defensive copy of all records, ensuring callers cannot mutate internal store state.
- **Atomic Persistence (`save`):**
  - Verifies or creates parent directories with `paths.DirPerm755` (`0755`).
  - Serializes `StoreDocument` with `SchemaVersion: "2.0.0"`.
  - Writes to staging file `<filePath>.tmp` with `paths.FilePerm600` (`0600`).
  - Atomically renames `<filePath>.tmp` to `<filePath>`.
  - Automatically cleans up the temporary file if write or rename encounters an error.

---

## 4. Convergence Session Snapshots & Telemetry

When an execution wave or autonomous convergence loop completes, idle metrics are finalized and linked to the active Convergence Session (CVS).

```mermaid
stateDiagram-v2
    [*] --> IdleAccumulating: Agent enters idle wait state
    IdleAccumulating --> ActiveExecution: Agent receives wake signal
    ActiveExecution --> IdleAccumulating: Agent finishes batch step
    ActiveExecution --> CVSFinalization: Wave or plan completes
    CVSFinalization --> SnapshotCalculated: RecordCVSSnapshot invoked
    SnapshotCalculated --> TelemetryPersisted: zqk object update committed
    TelemetryPersisted --> [*]
```

### Telemetry Payload Contract

`RecordCVSSnapshot` reads all records via `GetAllRecords()`, computes the total accumulated idle time, and invokes the kernel CLI to update the CVS object:

```bash
zqk object update <cvs-id> \
  --field after_state_snapshot.baseline_idle=<duration_string> \
  --field after_state_snapshot.captured_at=<rfc3339_timestamp>
```

This telemetry enables downstream performance linters, scheduler backoff engines, and token budget analytics to distinguish between active computational time and quiescent wait cycles.

---

## 5. Concurrency, Process Isolation & Security Invariants

### 1. Fine-Grained Mutual Exclusion
All reads and writes on `FileStore` acquire an internal `sync.Mutex`. While `Accumulate` releases the lock before executing disk I/O, `save()` acquires the lock independently to ensure serialization consistency during temporary file writing and renaming.

### 2. Goroutine Hygiene (`goroutinelabels`)
All background workers, monitor loops, and concurrent stress tests interacting with `pkg/agentidle` MUST utilize `goroutinelabels.NewGoroutine`:
```go
goroutinelabels.NewGoroutine(
    "agentidle_worker",
    "concurrent idle accumulation and retrieval",
).StartSimple(func() {
    // Goroutine body
})
```
Naked `go` statements are strictly prohibited across the codebase by AST check-valves.

### 3. File & Directory Permission Invariants
The subsystem enforces strict POSIX filesystem permissions to ensure multi-tenant security and process isolation:
- Data files: `paths.FilePerm600` (`0600` - Owner read/write only).
- Parent directories: `paths.DirPerm755` (`0755` - Owner read/write/exec, group/other read/exec).

### 4. Map Isolation
`GetAllRecords` creates an independent copy of `fs.records`. External modifications to the returned map have zero impact on the `FileStore`'s internal state.

---

## 6. Verification Evidence & Three-Fold Proof

The `pkg/agentidle` subsystem satisfies the ZQK Three-Fold Proof verification standard:

### 1. Static Floor Invariant
- **Statement Coverage:** $\ge 85.0\%$ floor strictly enforced (achieved: **$96.9\%$**).
- **AST Hygiene:** Zero swallowed errors (`_ = ...` strictly eradicated). All error returns must be captured and evaluated.
- **Function Complexity:** Modular routines with all functions kept under 100 lines.

### 2. Operational Dynamic Proof
- **Accumulation & Monotonic Growth:** Verifies additive duration tracking across multiple distinct agents and tasks.
- **Atomic Restart Safety:** Verifies that a newly initialized `FileStore` on an existing store file accurately reloads all prior records.
- **Defensive Copy Immutability:** Verifies that mutating maps returned by `GetAllRecords` does not corrupt internal state.
- **High-Concurrency Stress:** Verified under multi-worker concurrent read/write stress without deadlocks or race anomalies.
- **Schema Migration:** Verified automatic ingestion and promotion of legacy raw maps to `2.0.0` documents.

### 3. Negative Adversarial Boundary
- **Filesystem Collisions:** Verifies fail-closed error handling when parent directory paths are blocked by regular files.
- **Write Failures:** Verifies clean error wrapping when temporary file write paths are blocked.
- **Rename Collisions:** Verifies error recovery when target paths collide with existing directory structures.
- **Payload Corruption:** Verifies explicit error returns on malformed JSON rather than silent truncation.

---

## 7. Operational Recipes & Code Examples

### Initializing a FileStore
```go
store, err := agentidle.NewFileStore("/path/to/.zqk/state/agent_idle.json")
if err != nil {
    return fmt.Errorf("failed to initialize idle store: %w", err)
}
```

### Accumulating Idle Duration
```go
// Add 500ms of idle wait time for worker-1 on task-alpha
err := store.Accumulate("worker-1", "task-alpha", 500*time.Millisecond)
if err != nil {
    return fmt.Errorf("failed to record idle time: %w", err)
}
```

### Retrieving Idle Time
```go
duration, err := store.GetIdleTime("worker-1", "task-alpha")
if err != nil {
    return err
}
fmt.Printf("Worker-1 idle time on task-alpha: %v\n", duration)
```

### Emitting Convergence Session Telemetry
```go
err := agentidle.RecordCVSSnapshot(store, cvsID)
if err != nil {
    return fmt.Errorf("failed to commit CVS idle snapshot: %w", err)
}
```

### Running Subsystem Unit Tests
```bash
ZQK_ALLOW_FOREGROUND_GO_TEST=1 go test -v -timeout 30s -cover ./pkg/agentidle
```
