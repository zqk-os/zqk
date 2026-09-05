# Diagnostic Report Mining

**Last Verified:** 2026-08-31


**Purpose:** Extract and compare metrics from scheduler/CLI diagnostic dumps to validate reliability and efficiency fixes (e.g. HashRegistry caching, list/count concurrency, goroutine ceiling).

**Location of dumps:** `.zqk/diagnostics/` with subdirs:
- `scheduler_start/` — captures from the scheduler daemon (SIGUSR1 or on start)
- `cli/` — captures from CLI commands

Each capture produces (for a given timestamp, e.g. `20260225-063021`):
- `*_goroutines.txt` — full goroutine stacks (panic-style)
- `*_threads.txt` — header with **Goroutine count** and **OS thread count**, then stacks
- `*_processes.txt` — header with **PID** of capturing process, then `ps aux` (RSS in column 6)

---

## 1. Metrics report script

**Script:** `scripts/diagnostics/diagnostic_metrics_report.sh`

**Usage:**

```bash
# From repo root: list all scheduler_start captures (timestamp, goroutines, threads, RSS)
./scripts/diagnostics/diagnostic_metrics_report.sh scheduler_start

# Same for CLI captures
./scripts/diagnostics/diagnostic_metrics_report.sh cli

# All (scheduler + CLI)
./scripts/diagnostics/diagnostic_metrics_report.sh all

# Compare two timestamps (before vs after a fix or restart)
./scripts/diagnostics/diagnostic_metrics_report.sh scheduler_start --compare 061124 063021
# or
./scripts/diagnostics/diagnostic_metrics_report.sh --compare 061124 063021 scheduler_start
```

**Output:**
- CSV to stdout: `timestamp,goroutines,threads,rss_kb,capture_name`
- With `--compare`: a **Before vs After** block with deltas for goroutines, threads, and RSS (KB)

**Timestamps:** Use the time part of the filename, e.g. `061124` for `scheduler_start_20260225-061124_*`.

---

## 2. Interpreting the data

- **Goroutine count** — From `threads.txt` line `Goroutine count: N`. High growth under load (e.g. 1k+) can indicate uncached per-operation creation (e.g. HashRegistry before cache).
- **OS thread count** — From `threads.txt`; note: current capture writes `runtime.NumCPU()`, not true OS thread count.
- **RSS (KB)** — Resident set size of the zqk process from `processes.txt` (PID from header, then ps aux line for that PID, column 6). Use to track memory over time.

**Typical comparison:**
- **Before fix (e.g. 061124, jobs on, no HashRegistry cache):** High goroutines (e.g. 1362), elevated RSS.
- **After fix (e.g. 063021, jobs on, HashRegistry cached):** Lower goroutines (e.g. 137), similar or slightly higher RSS as work continues.

---

## 3. Goroutine stack summaries

For a deeper view of *where* goroutines are, use the existing script:

```bash
go run scripts/diagnostics/goroutine_thread_summary.go \
  .zqk/diagnostics/scheduler_start/scheduler_start_20260225-061124_goroutines.txt \
  .zqk/diagnostics/scheduler_start/scheduler_start_20260225-061124_threads.txt
```

Output: `goroutine_thread_summary.csv` in the same directory with top stack frame counts. Compare before/after to confirm reduction in `HashRegistry.startSaveWorker` (or similar) after caching.

---

## 4. References

- **RCA and baselines:** [SCHEDULER_STORAGE_RCA.md](./SCHEDULER_STORAGE_RCA.md)
- **Overload analysis:** [SCHEDULER_OVERLOAD_AND_TIMEOUT.md](./SCHEDULER_OVERLOAD_AND_TIMEOUT.md)
- **Capture implementation:** `pkg/diagnostics/capture.go`, `signal_handler.go`
