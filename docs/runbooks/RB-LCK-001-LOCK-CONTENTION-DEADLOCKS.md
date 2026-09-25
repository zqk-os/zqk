# RB-LCK-001: Concurrency Lock Contention and Deadlock Resolution

## Metadata
- **Severity**: P1 / High
- **Component**: `pkg/concurrency`, `pkg/storage/locknames`
- **Owner**: Runtime & Concurrency Stewards

## Symptoms & Alerts
- Log warning: `lock held longer than threshold: lock=<LOCK_NAME> duration=>100ms`
- Operations stalling or timing out with `context deadline exceeded` during storage transactions.
- Goroutine dump showing multiple threads waiting on `sync.(*Mutex).Lock` in `concurrency.RunInLockWithLogger`.

## Root Cause Analysis
1. Inverted lock acquisition order between object-level locks and store-wide index locks.
2. Long-running I/O or external network calls executed while holding an in-memory mutex.
3. Panic occurring inside a critical section where mutex unlock was not deferred properly.

## Step-by-Step Remediation Procedure

### Step 1: Capture Goroutine Stack Trace
Dump active goroutines to locate blocking locks:
```bash
./bin/zqk diagnostics dump --include-goroutines
```
Inspect generated diagnostics under `.zqk/diagnostics/` for mutex contention.

### Step 2: Identify Contended Lock Names
Filter the recent daemon logs for slow lock acquisitions:
```bash
grep "lock held longer than threshold" .zqk/state/logs/scheduler.log | tail -n 20
```
Note the specific `LockName*` reported.

### Step 3: Clear Stale Lockfiles (if filesystem-based)
If locks are backed by file locks (`flock`) under `.zqk/locks/`:
1. Check for stale lock holder PIDs:
   ```bash
   lsof +D .zqk/locks/
   ```
2. If the owning process has terminated ungracefully:
   ```bash
   rm -f .zqk/locks/*.lock
   ```

### Step 4: Verification Gate
Run concurrent read/write validation:
```bash
go test -race -v -run TestAdversarialConcurrency ./pkg/storage
```
Verify zero deadlocks and zero data race warnings.
