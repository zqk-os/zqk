# Sample Analysis: Bulk Delete (pid 40505) and Scheduler (pid 20777)

**Last Verified:** 2026-08-31


Analysis of `main-sample.txt` (bulk delete) and `bulk-delete-sample.txt` (scheduler daemon), sampled at 1 ms.

---

## 1. Bulk Delete Process (main-sample.txt, pid 40505)

**Role:** `zqk object bulk delete --file ids-to-delete.yaml` (deleting hundreds of scheduler_job objects).

### Top of stack (collapsed) – where CPU time went

| Samples | Symbol | Interpretation |
|--------:|--------|----------------|
| 36,087 | `__psynch_cvwait` | Threads blocked in condition wait (idle / sync) |
| 3,362 | `__open` | **Many file opens** – open-heavy workload |
| 2,015 | `read` | Read syscalls |
| 2,010 | `kevent` | kqueue event loop (e.g. async I/O) |
| 1,975 | `__semwait_signal` | **usleep** – thread sleeping (polling/sleep loop) |
| 405 | `__getdirentries64` | **Directory scanning** – listing directory entries |

### Application bottleneck (call graph)

The only substantial **zqk** stack in the sample is:

```
runBulkDelete → BulkDelete → FileObjectTransaction.Commit → Delete
  → findDependents → ContentAddressableStorage.Read (YAML parse, etc.)
```

So during bulk delete, when the process is not waiting or sleeping, it is spending time in:

1. **findDependents** – For each delete, the code checks for dependents (non–leaf kinds). For `scheduler_job` this is unnecessary if it is treated as a leaf (no refs from process data).
2. **ContentAddressableStorage.Read** – Used inside dependent checks: read object by ID (and possibly directory scan when ID→path is not in the index).
3. **Directory I/O** – `fdopendir` / `readdir_r` / `__getdirentries64` (405 samples) from workers doing directory walks (e.g. listing kind dirs for dependents or CAS lookup).

### Root cause and recommendations

- **Cause:** Bulk delete uses the **transaction path** (`FileObjectTransaction`), and for **each** ID it calls `Delete()`, which:
  - Runs **findDependents** (even for kinds like `scheduler_job` that have no dependents in process data).
  - That can trigger **CAS.Read** and **directory scanning** when resolving IDs or scanning for references.
- **Recommendations:**
  1. **Treat `scheduler_job` as leaf in delete path**  
     Skip `findDependents` for `scheduler_job` (and any other “no dependents” kinds) so each delete does not pay for dependent checks and CAS/directory work.
  2. **Use BulkDeleteOptimized for known leaf kinds**  
     For bulk delete of only `scheduler_job` (or other leaf kinds), use the optimized path that skips dependency graph and does parallel `Delete()` without per-object dependent checks.
  3. **Avoid directory scan on hot path**  
     Ensure CAS resolution for these IDs uses the index where possible; fallback to `findCASFilePathByScanning` only when necessary so directory scans are not on the critical path of every delete.

---

## 2. Scheduler Daemon (bulk-delete-sample.txt, pid 20777)

**Role:** `zqk scheduler start --foreground` (long-running daemon).

### Observations

- Most threads are in **pthread_cond_wait** (idle).
- One thread in **kevent** (event loop).
- One thread in **read** (likely reading from a pipe or log).
- **Active work** seen in the sample: **RetentionToleranceHandler** (e.g. `enforceMaxCount`) during job execution:
  - `executeJob` → `RetentionToleranceHandler.Execute` → `enforceMaxCount`
  - Under that: **FileObjectStorage.Read** → **getObjectFilePath** → **findCASFilePathByScanning** (directory scan + **open** / **Read** of files).

So when the scheduler is doing retention-tolerance work, time goes into:

- **findCASFilePathByScanning** – Scanning the kind directory to find a file by ID when the path is not in the CAS index.
- Multiple **open** and **Read** calls (one per candidate or per object read).

### Recommendation for scheduler

- **CAS index:** Ensure scheduler_job (and any kind used by retention-tolerance) has its CAS index loaded or built when the daemon starts (or on first use), so **getObjectFilePath** resolves via index and **findCASFilePathByScanning** is not needed on the hot path. That will reduce **__open** and directory-scan cost during retention runs.

---

## 3. Summary

| Process | Main bottleneck | Fix |
|--------|------------------|-----|
| **Bulk delete** | Per-delete **findDependents** + **CAS.Read** + directory scans | Treat scheduler_job as leaf; use BulkDeleteOptimized for leaf-only bulk deletes; limit directory scan fallback. |
| **Scheduler** | **findCASFilePathByScanning** and **open**/Read during retention-tolerance | Ensure CAS index is used for scheduler_job so path resolution does not rely on directory scanning. |
