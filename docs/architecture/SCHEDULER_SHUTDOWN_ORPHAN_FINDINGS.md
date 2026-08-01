# Scheduler Shutdown Orphan Findings (2026-02-25)

**Context:** After `zqk scheduler stop`, `scheduler status` reports "Not running", but sampling showed orphaned zqk processes. Analysis of two sample dumps and the shutdown path.

---

## 1. Sample summary

| Sample file | PID  | Parent  | Launch → Sample | Interpretation |
|-------------|------|---------|-----------------|----------------|
| hang-after-shutdown-1.txt | 43847 | **launchd [1]** | ~53 min | **Scheduler daemon** (background start, adopted by launchd). |
| hang-after-shutdown-2.txt | 29636 | **bash [29592]** | ~5 min  | **Child process** – either job subprocess (daemon → sh → zqk) or user CLI (e.g. `zqk scheduler stop` or another command). |

Both processes show **no application-level frames** in the sample: all threads in kernel (e.g. `__psynch_cvwait`, `read`, `kevent`, `usleep`). So we can’t see which Go code was running; we only see that they were blocked/idle.

---

## 2. Daemon (43847) – parent launchd

- **Identity:** This is the scheduler daemon (started in background, parent shell exited so launchd became parent).
- **If “scheduler stop” was run:** Stop sends SIGTERM to the PID in the scheduler PID file. If 43847 was that PID, it should have received SIGTERM.
- **Intended flow:** Signal handler runs → `cancel()` (startCtx) → `sched.Start(startCtx)` returns (because `<-ctx.Done()` in `Start()` unblocks) → `Start()` calls `s.Stop()` → process group manager shutdown, storage shutdown, PID file removal, exit.

**Possible reasons it was still running at sample time:**

1. **Wrong PID** – PID file referred to a different (e.g. already dead) process, so 43847 never got SIGTERM.
2. **Daemon stuck in shutdown** – It received SIGTERM and entered `Stop()`, but some step in `Stop()` blocked (e.g. cron stop, triggered pool stop, process group manager, or storage shutdown), so the process never exited.
3. **Signal not delivered** – Less likely (e.g. process in bad state for delivery).

So either **context/signal handling** (who gets SIGTERM, or whether the main loop returns and calls `Stop()`), or **blocking during `Stop()`**, can leave the daemon running after “scheduler stop”.

---

## 3. Child (29636) – parent bash

- **Identity:** A zqk process whose parent is a shell (bash 29592). So it was either:
  - **A)** A **job subprocess** (run_wrapper job ran a script that exec’d `zqk`), or  
  - **B)** A **user CLI** (e.g. `zqk scheduler stop`, `zqk scheduler status`, or another command) run from that shell.

If **A:** When the daemon exits, job children are only guaranteed to be killed if the daemon runs `Stop()` and `ProcessGroupManager.Shutdown()` (which kills tracked process groups). If the daemon exits without running `Stop()` (e.g. crash, or main exits before `Stop()`), those children are orphaned.

If **B:** Then 29636 could be the **“zqk scheduler stop”** process itself, stuck (e.g. in `GetStorageProviderForCoordinator` or other post–SIGTERM work). The code uses a 5s timeout for storage there, but if something else in the stop path blocks, the stop command would remain running.

---

## 4. Context and shutdown path

**Daemon side (correct):**

- `Start()` blocks on `<-ctx.Done()` then calls `keepAliveCancel()` and **`s.Stop()`** before returning. So when the interrupt handler cancels `startCtx`, the main loop does run full shutdown, including process group manager and storage.

**ProcessGroupManager.Shutdown():**

- It immediately calls `pgm.shutdownCancel()`, then builds a “graceful” wait with  
  `context.WithTimeout(pgm.shutdownCtx, pgm.shutdownTimeout/2)`.
- **Bug:** `pgm.shutdownCtx` is already cancelled, so the derived context is cancelled immediately. The “graceful” wait therefore doesn’t wait; we proceed to force-kill right away. So we don’t actually do a graceful SIGTERM wait; we rely on the initial `KillFunc()` (which for run_wrapper is SIGKILL) and then the “remaining” SIGKILL. Functionally we still kill subprocesses, but the design is misleading and the graceful window is zero.

**Recommendation:** For graceful shutdown, create the timeout from `context.Background()` (or a new context) so the wait is real. Then send SIGTERM first, wait that duration, then send SIGKILL for any still-running process groups.

---

## 5. Recommendations

1. **Ensure daemon gets SIGTERM and runs Stop()**
   - Confirm PID file is written and read correctly so “scheduler stop” targets the real daemon.
   - Optionally: after sending SIGTERM, wait a few seconds and, if the process is still alive, send SIGKILL (with a clear log message).

2. **Avoid blocking in Stop()**
   - All steps in `s.Stop()` already use timeouts or bounded waits (cron 2s, triggered pool 5s, process group manager 3s). If any new step is added, keep it bounded so the daemon can’t hang in shutdown.

3. **ProcessGroupManager graceful wait**
   - Fix the “graceful” wait so it uses a non-cancelled parent context (e.g. `context.Background()`) for `WithTimeout`, so we actually wait before force-killing. Optionally send SIGTERM first and use the existing KillFunc for SIGKILL only after the wait.

4. **“scheduler stop” command**
   - Already uses a 5s timeout for storage when emitting the stop event. Ensure no other synchronous work in the stop command can block indefinitely.

5. **Observability**
   - On SIGTERM, log that the signal was received and that shutdown (including `Stop()`) has started. If possible, log when `Stop()` finishes so we can see from logs whether the daemon is stuck in shutdown.

---

## 6. References

- `cmd/zqk/scheduler/scheduler_core.go` – interrupt handler, `cancel()`, `sched.Start(startCtx)`.
- `pkg/scheduler/scheduler.go` – `Start()` (blocks on `<-ctx.Done()`, then `s.Stop()`), `Stop()` (cron, pool, process group manager, storage, PID file).
- `pkg/scheduler/process_group_manager.go` – `Shutdown()`, use of cancelled context for graceful wait.
- `pkg/scheduler/pid_file.go` – `StopSchedulerByPID()` (sends SIGTERM to PID from file).
