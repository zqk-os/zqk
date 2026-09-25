# RB-WAL-001: Write-Ahead Log (WAL) Compaction Failures

## Metadata
- **Severity**: P1 / High
- **Component**: `pkg/storage/wal`, Write-Behind Pipeline
- **Owner**: Reliability & Durability Engineering

## Symptoms & Alerts
- Error message: `wal compaction failed: <reason>` or `requires non-empty projectRoot`
- Metric: WAL file size (`.zqk/wal/object.wal`) monotonically increasing beyond 50MB.
- Metric: Replay duration on startup exceeding 5 seconds.
- Storage disk saturation warnings on `.zqk/wal/`.

## Root Cause Analysis
1. Stale `.checkpoint` lock or orphaned temporary compaction file (`object.wal.compacting`).
2. High-frequency write burst preventing compaction lock acquisition.
3. Unclosed file descriptors holding OS-level lock on `object.wal`.

## Step-by-Step Remediation Procedure

### Step 1: Check Current WAL Status
Inspect the size and active lines of the WAL:
```bash
ls -lh .zqk/wal/
wc -l .zqk/wal/object.wal
```

### Step 2: Trigger Manual Compaction
Attempt manual compaction via scheduler job or CLI:
```bash
./bin/zqk scheduler trigger SCH-maintenance-wal
```
Check job execution history:
```bash
./bin/zqk scheduler history --job-id SCH-maintenance-wal --limit 5
```

### Step 3: Handle Orphaned Compaction Files
If compaction reports file conflict or deadlock:
1. Temporarily pause scheduler jobs:
   ```bash
   ./bin/zqk scheduler stop
   ```
2. Check for stale temporary compaction artifacts:
   ```bash
   ls -la .zqk/wal/*.compacting .zqk/wal/*.tmp 2>/dev/null
   ```
3. Remove stale compaction remnants if the process is dead:
   ```bash
   rm -f .zqk/wal/*.compacting .zqk/wal/*.tmp
   ```
4. Restart the scheduler:
   ```bash
   ./bin/zqk scheduler start
   ```

### Step 4: Verification Gate
Re-run compaction verification:
```bash
./bin/zqk scheduler trigger SCH-maintenance-wal
./bin/zqk scheduler status
```
Verify `object.wal` has compacted to snapshot baseline.
