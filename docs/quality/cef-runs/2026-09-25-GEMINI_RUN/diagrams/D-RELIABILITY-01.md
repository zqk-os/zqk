# D-RELIABILITY-01: Storage Durability, Crash Consistency, and Concurrency Failure Modes

```yaml
diagram_id: D-RELIABILITY-01
type: flowchart
title: "Storage Durability, Crash Consistency, and Concurrency Failure Modes"
anchors:
  - path: pkg/storage/object_storage_file_transaction_commit_impl.go
    symbol: commitStageApplyPerOpMixed
    note: "Transaction commit loop with no-op rollback on intermediate failure"
  - path: pkg/storage/filecas/cas_publish_sync_darwin.go
    symbol: CasPublishSyncFileOS
    note: "Asynchronous fsync queue on Darwin with no CLI exit drain"
  - path: pkg/storage/object_write_behind_worker.go
    symbol: processOneOrMore
    note: "Write-behind worker checkpoint flush and retryable drop logic"
  - path: pkg/storage/ipc_writer.go
    symbol: WriteObject
    note: "Unbounded synchronous net/rpc call with discarded context"
  - path: pkg/scheduler/job_lock.go
    symbol: checkAndCleanStaleLock
    note: "File lock unlinking prior to flock causing split-brain inode locking"
claims:
  - "Darwin async fsync queue drops durability when short-lived CLI commands exit before background worker executes Sync()"
  - "Transaction rollback is a no-op that leaves partial mutations committed to disk/CAS when an intermediate operation fails"
  - "Stale lock unlinking allows concurrent processes to acquire exclusive locks on distinct inodes simultaneously"
  - "Advancing applied_seq on save-queue-full error permanently purges unpersisted mutations during compaction"
evidence_grade: E2
```

## Description

This diagram illuminates the four critical failure modes in the ZQK storage, concurrency, and durability subsystems:
1. **Darwin Fsync Drop:** Short-lived CLI commands enqueue file descriptors to `darwinSyncQueue` and immediately exit, killing the background goroutine before `f.Sync()` occurs.
2. **Transaction Atomicity Violation:** Multi-operation transactions apply mutations incrementally; when an operation fails, `Rollback()` performs a no-op (`tx.ops = nil`), abandoning prior mutations on disk.
3. **Flock Inode Race:** Stale lock cleanup deletes `lockPath` from the filesystem after `NewFileLock` opened the inode; subsequent processes open a new inode, breaking mutual exclusion.
4. **WAL Checkpoint Advancement on Queue Full:** Write-behind worker drops mutations when `saveQueue` is full and advances `applied_seq`, ensuring `CompactWAL` permanently purges the unwritten data.

```mermaid
flowchart TD
    subgraph CLI_Lifecycle["1. CLI Mutation & Darwin Durability"]
        CLI[CLI Command Invocations] -->|1. Write Temp File| TmpWrite[fileutil.CreateTemp + Write]
        TmpWrite -->|2. Enqueue FD| SyncQ["darwinSyncQueue (Channel 1024)"]
        TmpWrite -->|3. Hardlink CAS| Link["fileutil.Link(tmp, filePath)"]
        Link -->|4. Return Success| CLIOk[CLI Exits os.Exit 0]
        CLIOk -.->|KILL Process| DroppedFsync["❌ Background Worker Killed Before f.Sync() Runs<br/>(DURABILITY LOST ON CRASH)"]
    end

    subgraph Transaction_Lifecycle["2. Transaction Atomicity Violation"]
        TxOps["Transaction Ops [Op1, Op2, Op3]"] -->|Apply Op1| Storage1["tx.storage.Create(Op1) -> SUCCESS (Wrote to CAS)"]
        Storage1 -->|Apply Op2| Storage2["tx.storage.Create(Op2) -> FAILURE (Disk / Validation Error)"]
        Storage2 -->|Error Triggered| TxRollback["tx.Rollback() Called"]
        TxRollback -->|tx.ops = nil| NoOpRollback["❌ Rollback is NO-OP<br/>Op1 Remains Committed on Disk<br/>(PARTIAL STATE CORRUPTION)"]
    end

    subgraph Lock_Lifecycle["3. Distributed JobLock Inode Race"]
        ProcA["Process A: NewJobLock()"] -->|Open File Inode 101| FDA[FD held for Inode 101]
        ProcA -->|Acquire() -> age > 1h| Unlink["fileutil.Remove(lockPath)<br/>(Inode 101 Unlinked)"]
        ProcB["Process B: NewJobLock()"] -->|OpenFile O_CREATE| FDB["Created New Inode 102"]
        Unlink -->|flock(LOCK_EX)| LockA["Proc A locks Inode 101"]
        FDB -->|flock(LOCK_EX)| LockB["Proc B locks Inode 102"]
        LockA & LockB -->|MUTUAL EXCLUSION DESTROYED| SplitBrain["❌ BOTH PROCESSES RUN CONCURRENTLY<br/>(DUPLICATE AGGREGATION & RACES)"]
    end

    subgraph WAL_Lifecycle["4. Write-Behind Queue-Full Data Loss"]
        WBWorker[Write-Behind Worker] -->|apply()| HashReg["applyCreateFromBuffer() -> HashRegistry Save"]
        HashReg -->|Queue Full| QueueFullErr["ConstStreamSaveQueueIsFull Error"]
        QueueFullErr -->|isRetryableWriteBehindApplyDrop| DropOp["w.buf.RemoveFront(op)"]
        DropOp -->|flushCheckpoint| Checkpoint["WriteAppliedSeq(op.Seq)"]
        Checkpoint -->|applied_seq Advanced| WALCompact["CompactWAL() Purges seq <= applied_seq<br/>❌ MUTATION PERMANENTLY ERASED"]
    end
```
