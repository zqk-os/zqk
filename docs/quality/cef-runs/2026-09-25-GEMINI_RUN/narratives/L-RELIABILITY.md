# L-RELIABILITY Narrative: Reliability, Robustness & Error Recovery Evaluation

**Run Package:** `2026-09-25-GEMINI_RUN`  
**Lens:** `L-RELIABILITY`  
**Density Class:** `D-MED` (Top-N Budget: 10; Emitted: 9)  
**Primary Axes:** `REL` (Reliability), `ROB` (Robustness), `RCV` (Recoverability)  
**Evidence Baseline:** Git SHA `392fb153b506c0ecee2047a3ddb05a327a161f4e` (8,452 tracked files)

---

## 1. Executive Assessment

The ZQK platform features sophisticated reliability concepts: an append-only Write-Ahead Log (`ObjectWAL`), content-addressable storage (`filecas`), a write-behind buffer (`ObjectWriteBuffer`), a Privileged Writer IPC membrane daemon, distributed POSIX advisory file locks (`JobLock`), and bounded worker pools (`goroutinelabels.Pool`). 

However, deep evaluation across storage mutation hot paths, error recovery routines, and inter-process boundaries reveals several severe architectural and mechanical failure modes that undermine the codebase's reliability guarantees:

1. **Transaction Atomicity Violation (`F-REL-TX-PARTIAL-APPLY-ATOMICTY-VIOLATION`, Critical):** `FileObjectTransaction` claims ACID transaction semantics, but its execution loop (`commitStageApplyPerOpMixed`) writes mutations sequentially directly to live CAS storage. When an intermediate operation fails (e.g. disk quota exhaustion, schema validation failure, membrane refusal), `tx.Rollback(ctx)` is invoked. However, `Rollback` is an explicit no-op that merely clears the in-memory `tx.ops` slice under the comment *"no-op for file backend since we haven't applied changes yet"*. The preceding mutations remain permanently committed to disk, breaking atomicity and leaving repository state corrupt.
2. **Darwin Fsync Drop on CLI Exit (`F-REL-DARWIN-CAS-SYNC-DROP-ON-EXIT`, High):** To mitigate macOS `F_FULLFSYNC` latency, `CasPublishSyncFileOS` delegates file synchronization to an unmonitored background goroutine consuming an in-memory channel (`darwinSyncQueue`). The runtime provides no drain or flush hook upon process termination. When short-lived CLI commands (such as `zqk object create`) return success and call `os.Exit(0)`, the Go runtime immediately terminates the process before `dupFD.Sync()` runs, breaking crash consistency and durability on macOS.
3. **Privileged Writer IPC Unbounded Hang (`F-REL-IPC-WRITER-UNBOUNDED-HANG`, High):** `IPCWriter` communicates with the helper daemon over a Unix domain socket using standard library `net/rpc`. Although `WriteObject`, `DeleteObject`, and `RenameObject` accept a `ctx context.Context` parameter, the context is entirely discarded. `w.client.Call` is invoked synchronously without timeouts, deadlines, or cancellation propagation. If the daemon deadlocks or stalls on disk I/O, all CLI and agent mutation commands hang indefinitely.
4. **WAL Silent Data Loss on Queue Full (`F-REL-WAL-DROP-AND-ADVANCE-DATA-LOSS`, Critical):** When the write-behind worker encounters an error matching `isRetryableWriteBehindApplyDrop` (indicating `HashRegistry` queue capacity exhaustion), it drops the mutation from memory and flushes the checkpoint to disk via `WriteAppliedSeq(op.Seq)`. The comment asserts that the WAL will retain the op for replay, but advancing `applied_seq` guarantees that subsequent replays will skip the sequence. When `CompactWAL` executes, it purges all entries where `seq <= applied_seq`, causing permanent, irrecoverable data loss.
5. **Scheduler Stale Lock Inode Unlinking Race (`F-REL-SCHEDULER-STALE-LOCK-MUTEX-BREACH`, High):** `JobLock` implements distributed mutual exclusion across scheduler processes using POSIX `flock`. In `NewJobLockWithConfig`, `NewFileLock` opens `lockPath` and holds the file descriptor. In `Acquire()`, before acquiring `flock`, `checkAndCleanStaleLock()` checks `ModTime()`. If `age > threshold`, it unlinks `lockPath` from disk. When `fileLock.LockWithTimeout()` then executes, it acquires `flock` on an unlinked inode. Concurrently, a second process calling `NewJobLock` creates and locks a brand new inode at `lockPath`. Both processes now hold exclusive locks on separate inodes, destroying mutual exclusion and allowing concurrent execution of singleton tasks (e.g. audit aggregation and retention tolerance).
6. **Corrupted WAL Purge During Compaction (`F-REL-WAL-COMPACT-CORRUPT-LINE-PURGE`, Moderate):** When `parseWALLine` encounters an unparseable or torn line, it emits a warning and skips the line. If subsequent lines succeed, `applied_seq` advances. During `CompactWAL`, corrupted lines are omitted from the compacted output file, and `fileutil.Rename` permanently overwrites the original WAL without creating a quarantine copy (`.corrupt` or `.quarantine`), eliminating forensic recovery.
7. **Worker Pool Panic Drain Starvation (`F-REL-GOROUTINE-POOL-PANIC-DRAIN`, Moderate):** `goroutinelabels.Pool` worker goroutines execute tasks without an inner per-task `defer recover()` block. If an individual task panics, the panic unwinds the entire worker goroutine rather than just the task. Because the pool does not respawn deceased workers, each unhandled panic permanently reduces active pool capacity until all workers perish and the pool completely halts.
8. **RelayServer Cleanup Goroutine Leak (`F-REL-RELAY-CLEANUP-GOROUTINE-LEAK`, Moderate):** `RelayServer` spawns `cleanupLoop()` in an unmonitored goroutine executing an infinite ticker with no cancellation context or stop channel, leaking a goroutine on every initialization. Furthermore, `BufferPayload` appends messages to an in-memory map without capacity boundaries, risking out-of-memory crashes under disconnected client load.
9. **Uncached Object File Reader Partial Read (`F-REL-READ-OBJECT-PARTIAL-READ-CORRUPTION`, Low):** `readObjectFileNoCache` reads YAML files directly from disk by allocating `data = make([]byte, stat.Size())` and calling `file.Read(data)` followed by `data = data[:n]`. Because `io.Reader.Read` can return short chunks, partial reads truncate YAML documents, causing false syntax errors or field loss instead of using `io.ReadFull`.

---

## 2. Findings Summary Table

| Finding ID | Title | Severity | Grade | Axes | Density |
|------------|-------|----------|-------|------|---------|
| `F-REL-TX-PARTIAL-APPLY-ATOMICTY-VIOLATION` | Transaction commit performs sequential unrolled mutations with no-op rollback | critical | E2 | REL, ROB, RCV | D-MED |
| `F-REL-WAL-DROP-AND-ADVANCE-DATA-LOSS` | Write-behind worker drops mutations and advances checkpoints on save-queue full errors | critical | E2 | REL, ROB, RCV | D-MED |
| `F-REL-DARWIN-CAS-SYNC-DROP-ON-EXIT` | Darwin async fsync queue drops durability on immediate CLI command exit | high | E2 | REL, ROB, RCV | D-MED |
| `F-REL-IPC-WRITER-UNBOUNDED-HANG` | Privileged Writer IPC client discards context and executes unbounded synchronous RPC | high | E2 | REL, ROB | D-MED |
| `F-REL-SCHEDULER-STALE-LOCK-MUTEX-BREACH` | JobLock stale lock cleanup unlinks lock file breaking flock mutual exclusion | high | E2 | REL, ROB | D-MED |
| `F-REL-WAL-COMPACT-CORRUPT-LINE-PURGE` | WAL compaction silently purges corrupted log lines without quarantine | moderate | E2 | REL, RCV, ROB | D-MED |
| `F-REL-GOROUTINE-POOL-PANIC-DRAIN` | Worker pool task panic terminates worker goroutines causing pool starvation | moderate | E2 | REL, ROB | D-MED |
| `F-REL-RELAY-CLEANUP-GOROUTINE-LEAK` | RelayServer cleanupLoop lacks shutdown lifecycle leaking background goroutines | moderate | E2 | REL, ROB | D-MED |
| `F-REL-READ-OBJECT-PARTIAL-READ-CORRUPTION` | Uncached object file reader truncates YAML data on partial stream read | low | E2 | REL, ROB | D-MED |

---

## 3. Thematic Deep Dives

### 3.1 Transaction Atomicity and Partial Failure Vulnerability
In `pkg/storage/object_storage_file_transaction_commit_impl.go`, `commitStageApplyPerOpMixed` iterates over transaction operations:
```go
for _, op := range tx.ops {
    switch op.opType {
    case OpCreate:
        if err := tx.storage.Create(ctx, secCtx, op.obj); err != nil {
            var _err_83887600 = tx.Rollback(ctx)
            return nil, errfmt.Errorf(ConstStreamFailedToCreateObjectStrErr, op.id, err)
        }
    case OpUpdate:
        if err := tx.storage.Update(ctx, secCtx, op.id, op.updates); err != nil {
            var _err_83887851 = tx.Rollback(ctx)
            return nil, errfmt.Errorf(ConstStreamFailedToUpdateObjectStrErr, op.id, err)
        }
    ...
```
When `tx.Rollback(ctx)` is invoked on failure, line 303 reveals:
```go
// Rollback rolls back the transaction (no-op for file backend since we haven't applied changes yet)
func (tx *FileObjectTransaction) Rollback(ctx context.Context) error {
    if tx.committed {
        return errfmt.Errorf(ConstStreamTransactionAlreadyCommitted)
    }
    tx.rolledBack = true
    tx.ops = nil // Clear operations
    return nil
}
```
This is a direct violation of atomicity (the "A" in ACID). Because the earlier operations were already applied directly to the underlying file and CAS storage engines, clearing `tx.ops` leaves preceding changes permanently committed. Downstream callers receive an error and assume the transaction aborted cleanly, while the filesystem contains an inconsistent partial state.

### 3.2 Platform-Specific Durability Divergence on macOS (Darwin)
To work around macOS `F_FULLFSYNC` disk stall issues (where fsync can block for seconds or minutes under Spotlight/Time Machine indexing), `cas_publish_sync_darwin.go` creates a background queue:
```go
func CasPublishSyncFileOS(f *fileutil.File) error {
    darwinSyncQueueOnce.Do(initDarwinSyncQueue)
    dupFD, err := fileutil.Open(f.Name())
    if err != nil {
        return err
    }
    return queueOrSync(darwinSyncQueue, dupFD)
}
```
In `content_addressable_storage_file.go`, `WriteObject` writes a temporary file, calls `CasPublishSyncFile(tmp)`, and hardlinks the file into its final CAS destination. In a long-running daemon, this background worker eventually calls `dupFD.Sync()`. However, for CLI commands (`zqk object create`, `zqk object update`, etc.), the CLI process exits immediately after the command returns. The Go runtime terminates the entire process and its goroutines without draining `darwinSyncQueue`. As a result, writes acknowledged as successful to the user on macOS do not possess hardware durability.

### 3.3 Checkpoint Advancement and WAL Silent Data Loss
In `object_write_behind_worker.go`, the write-behind worker maintains an in-memory buffer and a durable checkpoint (`WriteAppliedSeq`). When processing entries, if an error matches `isRetryableWriteBehindApplyDrop`:
```go
if isRetryableWriteBehindApplyDrop(err) {
    w.buf.RemoveFront(op)
    pendingSeq = op.Seq
    flushCheckpoint(pendingSeq)
    continue
}
```
`isRetryableWriteBehindApplyDrop` checks for `ConstStreamSaveQueueIsFull`. Because `pendingSeq` is flushed to disk via `WriteAppliedSeq`, the system records that this sequence has been durably applied. When `CompactWAL` executes, it discards all records with `seq <= applied_seq`. The dropped mutation is never replayed on restart or background sweep and is permanently deleted during compaction.

### 3.4 POSIX Flock Inode Races in JobLock
In `pkg/scheduler/job_lock.go`, `JobLock` implements cross-process mutual exclusion. Prior to acquiring `flock`, `checkAndCleanStaleLock()` checks the file's modification time:
```go
age := time.Since(info.ModTime())
if age > threshold {
    if err := fileutil.Remove(jl.lockPath); err != nil ...
}
```
However, `jl.fileLock` was already opened in `NewJobLockWithConfig` prior to this check. When `jl.fileLock.LockWithTimeout()` executes, it acquires `flock` on an inode that has already been unlinked from the directory tree. A competing process calls `NewJobLock`, opens a newly created inode at `jl.lockPath`, and acquires `flock` on that new inode. Both processes proceed concurrently, violating the singleton invariant required for audit aggregation and retention tolerance jobs.

---

## 4. Remediation Roadmap

1. **Phase 1: Transaction Staging / Compensation Journal (P0):**
   - Refactor `commitStageApplyPerOpMixed` to maintain an in-memory undo log of applied mutations.
   - If an operation fails, execute inverse operations (deleting created objects, restoring original states for updated objects) before returning.
   - Alternatively, write transaction payloads to a staged temporary directory and atomically link/rename upon final commit.
2. **Phase 2: CLI Exit Fsync Drain on Darwin (P0):**
   - Provide an exported `DrainDarwinSyncQueue(ctx context.Context) error` function in `pkg/storage/filecas`.
   - Wire this drain hook into `cli.Processor.Close()` or `main.go` exit paths to guarantee all queued file descriptors are synced before process termination.
3. **Phase 3: Context & Deadline Propagation in Privileged Writer (P1):**
   - Replace unbounded `w.client.Call` in `IPCWriter` with `w.client.Go` paired with `select { case <-call.Done: ... case <-ctx.Done(): ... }`.
   - Enforce a 5-second dial timeout in `NewIPCWriter`.
   - Configure read/write timeouts on `ipc_server.go` listeners.
4. **Phase 4: Halt Checkpoint Advancement on Queue Full (P1):**
   - Remove `isRetryableWriteBehindApplyDrop` from the drop-and-advance path. When `saveQueue` is full, apply exponential backoff and retain the operation at the head of the buffer without advancing `applied_seq`.
5. **Phase 5: Fix Stale Lock Flock Semantics (P1):**
   - Eliminate file unlinking (`fileutil.Remove`) from `checkAndCleanStaleLock`.
   - Test lock availability exclusively via non-blocking `flock(LOCK_EX | LOCK_NB)`. If `flock` succeeds, the lock is acquired safely without inode churn.

---

## 5. Diamond Scale Impact & Scorecard Implications

- **Reliability (`REL`): Grade 2 (Rough / Industrial).** Confidence: 0.85. Driven by critical transaction atomicity failures, silent WAL data loss on queue full, and Darwin CLI fsync dropping.
- **Robustness (`ROB`): Grade 2 (Rough / Industrial).** Confidence: 0.80. Driven by unbounded IPC hangs, stale lock inode race conditions, and worker pool panic starvation.
- **Recoverability (`RCV`): Grade 3 (Commercial).** Confidence: 0.75. WAL provides replay capability and torn-tail sanitization, but corrupt lines are purged during compaction without quarantine.
