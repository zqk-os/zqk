# Pre-Commit Background Results

**Last Verified:** 2026-08-31


The pre-commit hook stays **fast** by only reading a single file that is updated in the background by scheduler jobs. One **block** indicator controls whether the commit is allowed. **All pre-commit state lives under one directory:** `.zqk/pre-commit/`.

## File layout

| Path | Purpose |
|------|--------|
| `.zqk/pre-commit/<category>.json` | Per-category result written by background jobs (e.g. `lint.json`, `integrity.json`, `policy.json`) |
| `.zqk/pre-commit/results.json` | Single aggregated file read by the hook; has `block` and `categories` |
| `.zqk/pre-commit/lint-output.txt` | Full linter output from the last lint run (only present when lint failed); view with `zqk pre-commit lint-report`. Removed when lint passes so we don't leave stale output. |
| `.zqk/pre-commit/policy-output.txt` | Full policy-check output (logging + architecture) from the last policy run (only present when policy failed); view with `zqk pre-commit policy-report`. Removed when policy passes so we don't leave stale output. |
| `.zqk/pre-commit/integrity-output.txt` | Human-readable integrity summary and last 80 lines of check output (only present when integrity failed); view with `zqk pre-commit integrity-report`. Removed when integrity passes. |
| `.zqk/pre-commit/integrity-check.log` | Full stdout/stderr of the last `zqk system check all` run when integrity failed; use for deeper insight (Tier 1 details, object-level issues). Removed when integrity passes. |

## Aggregated result schema

The hook reads `.zqk/pre-commit/results.json`:

```json
{
  "updated_at": "2026-02-19T12:00:00Z",
  "block": false,
  "categories": {
    "lint": { "ok": true, "summary": "", "details": [], "blocking": true, "updated_at": "..." },
    "integrity": { "ok": true, "summary": "", "blocking": true, "updated_at": "..." },
    "policy": { "ok": false, "summary": "POL-CODE-007: 2 violations", "details": ["pkg/foo.go:10"], "blocking": true, "updated_at": "..." },
    "docman": { "ok": true, "summary": "", "blocking": false, "updated_at": "..." }
  }
}
```

- **block**: If `true`, the hook exits 1 and prints a short summary. Set when any **blocking** category has `ok: false`.
- **categories**: Each key is a category name. `blocking: true` means “when `ok` is false, block the commit”.

## Categories

| Category | Typical source | Hook-blocking | Notes |
|----------|-----------------|----------------|-------|
| **lint** | Linter (e.g. golangci-lint) per package or whole tree | **yes** | Fast enough when run in background; only category that blocks commit. |
| **integrity** | System check (Tier 1 / hash registry / lifecycle) | no | Heavy; runs in background/CI. Fix before merge; see `zqk pre-commit status`. |
| **policy** | Logging compliance (POL-CODE-007), architecture compliance | no | Heavy; runs in background/CI. Fix before merge; see `zqk pre-commit policy-report`. |
| **docman** | Documentation registration (docman-sync) | optional (often no) | — |

**Reduce pre-commit latency (P2):** Only **lint** is hook-blocking. Integrity and policy run in background and write category files; failures are visible via `zqk pre-commit status` and reports. CI or a full check before merge should still enforce integrity and policy.

All policy and rule checks should derive their information in the background and write a category file; the hook never runs them directly.

**Target model (lint, policy, integrity):** Lint, policy, and integrity should behave the same way: one persistent scheduler job per category; on a timer the job runs the check only (system operation); when triggered from pre-commit, a callback runs that writes the pre-commit category and runs aggregation. See **PRE_COMMIT_INTEGRITY_TRIGGER_DESIGN.md** for the unified design.

## Flow

1. **Background jobs** (scheduler run_wrapper or cron) run checks and write category files:
   - Lint job(s): run linter, then `zqk pre-commit write-result --category=lint --ok=<exit==0> --summary="..." --blocking=true` (only lint blocks the hook)
   - Integrity job: run `zqk system check`, then write-result for `integrity` with `--blocking=false`
   - Policy job(s): run check-logging-compliance, check-architecture-compliance, etc., then write-result for `policy` with `--blocking=false`
2. **Aggregator**: After category jobs (or on a timer), run `zqk pre-commit aggregate` to merge category files in `.zqk/pre-commit/*.json` (excluding `results.json`) into `.zqk/pre-commit/results.json` and set `block`.
3. **Hook**: On commit, the hook reads `.zqk/pre-commit/results.json`; if `block` is true, it exits 1 and prints a summary; otherwise it allows the commit.

## Linter per package

**Local `scripts/pre-commit-lint.sh`:** By default it **`go build`**s and runs **golangci-lint** on packages that contain in-scope `*.go` files. Extra gates (full CLI rebuild, full-tree env AST, cobra, VDS `--all`) are skipped unless those paths are in scope; use **`PRECOMMIT_FULL=1`** for **`./...`** (see **`scripts/README.md`** → **`pre-commit-lint.sh`**).

To keep lint fast and accurate, run **one job per package** (or one job that lints all packages and writes a single `lint.json`):

- **Option A (one job per package):** Scheduler jobs like `SCH-lint-pkg-storage`, `SCH-lint-pkg-scheduler`, … each run `golangci-lint run ./pkg/storage/...`, then write a **shared** lint result. Use a lock or “last writer wins” for `lint.json`, or have a single “lint aggregator” job that runs after all package lint jobs and merges their outputs into one `lint.json`.
- **Option B (single job):** One job runs `golangci-lint run ./...` (or a list of packages) and writes `lint.json` once. Simpler; can be slower if the tree is large.

Recommendation: start with Option B; move to per-package jobs if needed for speed or parallelism.

## CLI commands

- **`zqk pre-commit status`**  
  Shows whether the results file exists, when it was updated, and whether the hook would block. Prints **required action** steps if the file is missing or if checks failed. Run this when the hook warns or blocks to see what to do.

- **`zqk pre-commit aggregate`**  
  Reads category files in `.zqk/pre-commit/*.json` (excluding `results.json`), sets `block` if any blocking category has `ok: false`, writes `.zqk/pre-commit/results.json`.

- **`zqk pre-commit write-result --category=<name> --ok=<bool> [--summary=...] [--blocking=true] [--details=...]`**  
  Writes one category file. Use from scripts or run_wrapper callbacks after a check.

- **`zqk pre-commit clear`**  
  Clears **all** files under `.zqk/pre-commit/` (category JSON, results.json, lint-output.txt, policy-output.txt, integrity-output.txt) and writes a clean `results.json` with `block: false`. Run after resolving blockers so the hook allows commits; the next background script run will repopulate categories. Prevents accumulation of stale output files.

- **`zqk pre-commit lint-report`**  
  Prints the contents of `.zqk/pre-commit/lint-output.txt` (written by `scripts/pre-commit-lint.sh`). **Use this to view lint issues and create backlog items** to resolve them. If the file is missing, run the lint script first.

- **`zqk pre-commit policy-report`**  
  Prints the contents of `.zqk/pre-commit/policy-output.txt` (written by `scripts/pre-commit-policy.sh`). **Use this when the policy category fails** to see the exact violations (logging compliance POL-CODE-007, architecture compliance). If the file is missing, run the policy script first.

## Background scripts (run once or via scheduler)

| Script | Category | Writes category | Also writes full output to |
|--------|----------|-----------------|----------------------------|
| `scripts/pre-commit-lint.sh` | lint | `lint.json` | `.zqk/pre-commit/lint-output.txt` (view: `zqk pre-commit lint-report`) |
| `scripts/pre-commit-policy.sh` | policy | `policy.json` | `.zqk/pre-commit/policy-output.txt` (view: `zqk pre-commit policy-report`) |
| `scripts/pre-commit-integrity.sh` | integrity | `integrity.json` | `.zqk/pre-commit/integrity-output.txt` (view: `zqk pre-commit integrity-report`) |
| `scripts/pre-commit-integrity.sh` | integrity | `integrity.json` | `.zqk/pre-commit/integrity-output.txt` (view: `zqk pre-commit integrity-report`) |

Run any of these (or all three) then `zqk pre-commit aggregate` is invoked at the end of each script. After fixing blockers, run `zqk pre-commit clear` to reset results and allow commits until the next run.

## Pre-commit background jobs (scheduler)

The hook reads `.zqk/pre-commit/results.json`. That file is updated when (1) you run the scripts manually, or (2) the scheduler jobs below run (each script now calls `zqk pre-commit write-result-from-last` at the end, so **timer runs** populate the folder; the callback still runs when you trigger with `--pre-commit`). If the pre-commit folder is empty, create the jobs and run the scripts once or wait for the first timer run.

**Jobs to schedule:** The three pre-commit timer jobs are part of the **maintenance bundle**. They are created automatically when you run `zqk system ensure-retention-jobs` or when the scheduler daemon starts (ensure runs once at startup). No manual create is needed unless you removed them.

If they are missing (e.g. before the bundle included them), create once from templates (run from repo root):
```bash
zqk object create scheduler_job --file scripts/templates/pre-commit-lint-job.yaml
zqk object create scheduler_job --file scripts/templates/pre-commit-policy-job.yaml
zqk object create scheduler_job --file scripts/templates/pre-commit-integrity-job.yaml
```
Or run `zqk system ensure-retention-jobs` to create any missing maintenance jobs (including pre-commit).

1. **Start the scheduler** (if not already running):  
   `zqk scheduler start`

2. **What each job does**

   | Job ID | Template | Schedule | Purpose |
   |--------|----------|----------|---------|
   | SCH-pre-commit-lint | `pre-commit-lint-job.yaml` | Every 10 min | Runs lint script → writes `lint.json` + aggregate |
   | SCH-pre-commit-policy | `pre-commit-policy-job.yaml` | Every 15 min | Runs policy script → writes `policy.json` + aggregate |
   | SCH-pre-commit-integrity | `pre-commit-integrity-job.yaml` | Every 2 h | Runs integrity script → writes `integrity.json` + aggregate |

   Each script runs `zqk pre-commit aggregate` at the end, so the last run updates `.zqk/pre-commit/results.json` for the hook.

4. **Optional:** Run the scripts once manually so results exist before the first timer fire:
   ```bash
   ./scripts/pre-commit-lint.sh
   ./scripts/pre-commit-policy.sh
   ./scripts/pre-commit-integrity.sh
   ```

## Viewing lint results (for backlog items)

When lint fails, the full linter output is written to **`.zqk/pre-commit/lint-output.txt`** by `scripts/pre-commit-lint.sh`. To view it:

- **`zqk pre-commit lint-report`** — prints that file so you can see file:line, rule, and message for each issue and create backlog items to fix them.
- **`zqk pre-commit integrity-report`** — prints that file so you can see Tier 1 issues, hash mismatches, lifecycle violations, and other integrity problems to fix them.
- Or open `.zqk/pre-commit/lint-output.txt` directly after running the lint script.

## Viewing policy check output (when policy fails)

When the policy category fails, the **exact violations** are written to **`.zqk/pre-commit/policy-output.txt`** by `scripts/pre-commit-policy.sh`. That file contains the combined stdout/stderr of the logging compliance and architecture compliance scripts (with section headers). To view it:

- **`zqk pre-commit policy-report`** — prints that file so you can see which files/lines failed POL-CODE-007 or architecture checks and fix them.
- Or open `.zqk/pre-commit/policy-output.txt` directly. If the file is missing, run `./scripts/pre-commit-policy.sh` first.

## Viewing integrity check output (when integrity fails)

When the integrity category fails, **two files** provide insight:

- **`.zqk/pre-commit/integrity-output.txt`** — summary, per-object issue lines (from JSON when available), and the last 80 lines of the check run. View with **`zqk pre-commit integrity-report`**.
- **`.zqk/pre-commit/integrity-check.log`** — full stdout/stderr of `zqk system check all` for that run; use for deeper insight when the summary is insufficient.

If either file is missing, run `./scripts/pre-commit-integrity.sh` first.

## Example: script that runs linter and writes lint category

```sh
#!/bin/sh
# scripts/pre-commit-lint.sh - run from scheduler run_wrapper or cron
set -e
REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"
if command -v golangci-lint >/dev/null 2>&1; then
	golangci-lint run --timeout=120s ./... 2>/dev/null && OK=true || OK=false
else
	go vet ./... 2>/dev/null && OK=true || OK=false
fi
SUMMARY="lint passed"
[ "$OK" = "false" ] && SUMMARY="lint failed"
"$REPO_ROOT/zqk" pre-commit write-result --category=lint --ok="$OK" --summary="$SUMMARY" --blocking=true
"$REPO_ROOT/zqk" pre-commit aggregate
```

## Example: scheduler jobs

1. **Timer job (e.g. every 5 min):** Run a script that (a) runs lint, integrity, policy checks in sequence or parallel, (b) writes each category with `zqk pre-commit write-result`, (c) runs `zqk pre-commit aggregate`.
2. **Per-package lint jobs:** One run_wrapper per package; each runs linter for that package and writes to a temp or shared location; a follow-up job runs the aggregator that merges per-package results into one `lint.json` and then runs `zqk pre-commit aggregate`.

## Hook behavior

- **Sync hooks** and **branch check** (no main) and **gofmt** on staged Go files always run (quick).
- **Results file present and `block: true`:** Hook exits 1 and prints which categories failed.
- **Results file present and `block: false`:** Commit allowed.
- **Results file missing:** Commit allowed; hook prints a one-line warning that background results are not set up.

## Quality and accuracy

- **Freshness:** Run the aggregator (and underlying checks) on a schedule or after save so results are recent when the user commits.
- **Stale results:** If the user commits before the next run, they may see an older result. Optional: allow a short “max age” for the results file and warn or block if older than N minutes.
- **CI:** Keep running full checks (lint, system check, policy) in CI; the hook is a fast local gate, not a replacement for CI.

## Other suggestions (speed, quality, accuracy)

- **Linter per package:** Run one scheduler job per package (e.g. `./pkg/storage`, `./cmd/zqk/scheduler`); each writes a partial result; a final job merges into one `lint.json` and runs aggregate. Keeps lint parallel and avoids one huge run.
- **Staleness:** Optionally block or warn if `updated_at` in the results file is older than N minutes (e.g. 10), so developers don’t commit on stale “pass” state.
- **Max age in hook:** Have the hook read `updated_at` and, if older than a threshold, allow commit but print “Pre-commit results are N minutes old; consider running background jobs.”
- **Policy checks from single source:** Run all policy/rule scripts (logging, architecture, etc.) in one job and write one `policy.json`; or split into subcategories (e.g. `policy_logging`, `policy_arch`) and let aggregate combine them.
- **CI alignment:** Keep CI running the same checks (lint, system check, policy) so the background results and CI stay aligned; the hook is a fast local gate, not a replacement for CI.
- **Optional jq:** The hook uses `jq` to read `block` and category summaries. If `jq` is missing, the hook allows the commit (no false blocks). Document “install jq for full pre-commit block check” in setup docs.

## Test bundles after commit (regression)

**Intent:** Areas of the code that were updated in a commit should automatically have their tests re-run to check for regression. The **post-commit hook** runs **scan-tests** for only the packages that had changed `.go` files.

**External contract (membrane):** Git hooks and scripts must not reach into `pkg/` or undocumented JSON. Use **`zqk system cli-hooks` (PRUNED)** and optional **`zqk tray`** as described in **[CLI_EXTERNAL_HOOK_PROTOCOL.md](./CLI_EXTERNAL_HOOK_PROTOCOL.md)**. The repo’s reference runner is **`scripts/hooks/zqk-post-commit-regression.sh`** (invoked from `tools/git-hooks/post-commit`).

1. **One-time setup:** Create test bundles and start the scheduler:
   ```bash
   zqk scheduler scan-tests --setup-bundles --overwrite
   zqk scheduler start
   ```
2. **After each commit:** The **post-commit hook** (`tools/git-hooks/post-commit`) runs `scripts/run-tests-for-changed-packages.sh` in the background, which runs `zqk scheduler scan-tests --load-bundles <changed>` for any package that had changed `.go` files. Output is appended to **`.zqk/logs/post-commit-tests.log`**. Use this log to see what the hook did (e.g. "No changed .go files", "Scheduling test bundles for changed packages (regression check): …", or errors). **Lifecycle:** No automatic rotation or retention; the file is append-only and grows until cleared. It is removed by `scripts/clear-all-logs.sh` along with other `.zqk/logs/*.log` files. Optionally truncate it manually (`: > .zqk/logs/post-commit-tests.log`) or run clear-all-logs when doing a full log reset.
3. **Manual run:** `./scripts/run-tests-for-changed-packages.sh` (uses HEAD) or `./scripts/run-tests-for-changed-packages.sh HEAD~1` for the previous commit.

Bundle names match package paths with `/` replaced by `-` (e.g. `pkg/precommit` → `pkg-precommit`). List bundles: `zqk scheduler scan-tests --list-bundles`.

**Test bundle status and failures:** Use `zqk scheduler activity` to see recent job runs (including test bundles). Use `zqk scheduler test-failures list` to see failing tests from recent runs; `zqk scheduler test-failures rerun` to re-run only those. Bundle log files are under `.zqk/logs/tests/` and in job metadata.

**Why test bundle jobs aren’t running (empty queue):** Test bundle jobs only get **onto the trigger queue** when something enqueues them. That happens when you run:

- **Post-commit:** The hook runs `run-tests-for-changed-packages.sh` → `zqk scheduler scan-tests --load-bundles <changed>` (only for packages that changed in the last commit).
- **Manual:** `zqk scheduler scan-tests --load-bundles X,Y`, `--package ./pkg/foo`, or `--all`.

There is **no automatic** “re-queue failed bundles” — no cron or hook looks at activity/test-failures and enqueues those bundles. So if the queue is empty, no test bundles run until you run one of the above (or `zqk scheduler test-failures rerun`, which creates one-off jobs and enqueues them). To get failed tests to run again: run `zqk scheduler test-failures rerun` (re-runs only failing tests) or `zqk scheduler scan-tests --load-bundles <bundle-names>` for the bundles you want.

## Reporting and trends (intent)

Pre-commit results, test bundle outcomes, and existing reporting utilities can be aggregated in the background to produce **trend data** that the pre-commit handler and quality dashboards can act on:

- **Pre-commit:** `.zqk/pre-commit/results.json` and category files (lint, policy, integrity) already provide a snapshot; these can be sampled over time (e.g. by a timer job that appends a row to a small history file or metric) to see pass/fail trends.
- **Test bundles:** Scheduler activity and test-failures list (and audit events for job outcomes) are the source of truth for test runs; a background job could aggregate pass/fail counts per bundle or package and write summary metrics.
- **Existing reporting:** `zqk system health-data` (PRUNED) (check summary, scheduler metrics, quarantine), `zqk reports pcs`, `zqk reports edd`, and scheduler activity/history are all candidates for periodic capture and aggregation so dashboards can show trends (e.g. lint pass rate over the last N runs, test failure rate by package).

**Explicit intention:** Avoid leaving "mystery" data—any background aggregation or report should be documented (where it is written, how to view it, and what it is for). Pre-commit output locations (lint-output.txt, policy-output.txt) and test bundle logs follow that pattern.

## Orphaned or hung zqk after timeout

If you see a **zqk process left running** (e.g. via Activity Monitor or `sample` on macOS) with parent **bash** or **sh**, it was likely started by:

- A **scheduler run_wrapper job** (e.g. pre-commit-integrity running `zqk system check all`, or a script that invokes zqk) that hit `max_runtime_seconds` and was killed—the run_wrapper kills the **process group** (shell + children), so normally the child zqk is terminated; in rare cases (e.g. process group boundary, race) a child can be left behind.
- A **manual run** of a script that starts zqk (e.g. `./scripts/pre-commit-integrity.sh` or `zqk system check all`) where the terminal was closed or the runner timed out without killing the child process group.

A sample/report of such a process typically shows threads **waiting** (e.g. `pthread_cond_wait`, `read`, `kevent`) rather than busy-looping—i.e. the process is idle and was left blocked when its parent or job was terminated.

**Clean up:**

- **Single process:** `kill <pid>` (use the pid from the sample or `ps aux | grep zqk`). If it does not exit, `kill -9 <pid>`.
- **All zqk processes (use with care):** `pkill -f zqk` or `pkill zqk`. Do **not** run this if you intend to keep the scheduler daemon running; stop the scheduler first (`zqk scheduler stop`) or kill only the specific pid.

**Avoid leaving orphans when running scripts manually:** Run the script under a process group and a timeout so the whole group is killed on timeout (e.g. `timeout 1200 ./scripts/pre-commit-integrity.sh` on Linux; on macOS use a wrapper or ensure you Ctrl+C in the same terminal so the shell sends signals to the foreground process group).

## Scheduler not firing / too many scheduler jobs

If **no new jobs are firing** (e.g. timer jobs like pre-commit or **scheduler_job_retention** never run), the usual cause is **too many scheduler_job objects**.

**What happens:** The daemon reloads all scheduler jobs every 30s (`loadAndScheduleJobs`). It does a single **unlimited** `List(scheduler_job)` — so if you have thousands of one_time jobs (e.g. from scan-tests or run_wrapper), that list is very slow. Reload then blocks or takes minutes; cron may not get updated or timer callbacks don’t run. The **scheduler_job_retention** job (which deletes old one_time jobs) is itself a timer job, so it never runs → the count never drops → the next reload is even slower.

**Fix: reduce job count first, then ensure retention runs.**

1. **Stop the daemon** so only the CLI touches the store:
   ```bash
   zqk scheduler stop
   ```
2. **List and bulk-delete done one_time jobs** (same idea as “Refreshing test bundle jobs” below; use a long timeout if the list is slow):
   ```bash
   zqk object list scheduler_job --filter execution_mode=one_time --filter enabled=false --format json --timeout 120s 2>/dev/null | jq -r '.objects[].id' > /tmp/sch-onetime-ids.txt
   # Optional: also status=disabled one_time
   zqk object list scheduler_job --filter execution_mode=one_time --filter status=disabled --format json --timeout 120s 2>/dev/null | jq -r '.objects[].id' >> /tmp/sch-onetime-ids.txt
   sort -u /tmp/sch-onetime-ids.txt -o /tmp/sch-onetime-ids.txt
   while read id; do echo "- $id"; done < /tmp/sch-onetime-ids.txt > /tmp/sch-onetime-ids.yaml
   zqk object bulk delete --file /tmp/sch-onetime-ids.yaml --timeout 120s
   ```
3. **Start the daemon** again:
   ```bash
   zqk scheduler start
   ```
4. **Ensure a scheduler_job_retention job exists** and is scheduled (e.g. daily). If missing, create one via CLI from a template (see `.zqk/process/scheduler/SCHEDULER_JOB_POLICY_AND_LIFECYCLE.md` and `docs/observability/OBJECT_COUNT_MANAGEMENT.md`). Then timer jobs (including retention) will fire on schedule.

After cleanup, check job count: `zqk object count scheduler_job`.

## Cleaning up one-off validation jobs (use bulk delete)

Object change notifications **used to** create a one-off scheduler_job per change (ID pattern `SCH-<timestamp>-<kind>-<id>`). That caused recursive creation when the changed object was a scheduler_job and jammed the system. The fix is a **single reusable job** (SCH-val, job_type=object_validation) triggered by event. Triggers are **batched** (by size and time) so one job run can process many changes; see `cmd/zqk/validation_trigger_batch.go` and `pkg/scheduler/handlers_object_validation.go`.

To **remove existing one-off validation jobs** (category=validation, job_type=run_wrapper, ID like `SCH-<digits>-*`):

1. **Stop the scheduler** so list/delete are not competing with daemon load.
2. **List IDs** (use long timeout; list can be slow with many jobs):
   ```bash
   zqk object list scheduler_job --filter category=validation --filter job_type=run_wrapper --format json --timeout 120s 2>/dev/null \
     | jq -r '.objects[]? | select(.id | test("^SCH-[0-9]+-")) | .id' > /tmp/sch-val-onetime-ids.txt
   ```
3. **Build YAML and bulk delete:**
   ```bash
   if [ -s /tmp/sch-val-onetime-ids.txt ]; then
     while read id; do echo "- $id"; done < /tmp/sch-val-onetime-ids.txt > /tmp/sch-val-onetime-ids.yaml
     zqk object bulk delete --file /tmp/sch-val-onetime-ids.yaml --timeout 120s
   fi
   ```

## Refreshing test bundle jobs (use bulk delete)

When you need to **delete many test-bundle scheduler jobs** (e.g. all `SCH-run-*` jobs) before re-running the scanner so new jobs get updated command args (e.g. `-timeout`), **use bulk delete** instead of looping over `zqk object delete`—looping is very slow for hundreds of jobs.

1. **List IDs** (testing category, SCH-run- prefix):
   ```bash
   zqk object list scheduler_job --filter category=testing --format json | jq -r '.objects[].id' | grep '^SCH-run-' > /tmp/sch-run-ids.txt
   ```
2. **Build a YAML file** (bulk delete expects a YAML array of IDs):
   ```bash
   while read id; do echo "- $id"; done < /tmp/sch-run-ids.txt > /tmp/sch-run-ids.yaml
   ```
3. **Bulk delete:**
   ```bash
   zqk object bulk delete --file /tmp/sch-run-ids.yaml
   ```
4. **Re-run the scanner** to create fresh jobs (with current args, e.g. `-timeout`):
   ```bash
   zqk scheduler scan-tests --all
   ```
   Or use `--setup-bundles` then `--load-bundles` if you use saved bundles.

5. **Verify** (after layout or multi-binary changes especially): `zqk scheduler test-failures health` / `zqk scheduler convergence measure` should show green or actionable **`suggested_rerun_commands`**; check **`zqk scheduler activity`** and **`.zqk/scheduler/issues.json`** for stuck failures. See **`docs/onboarding/AI_AGENT_ONBOARDING.md`** (test-bundle stream and scheduler reliability).

## References

- Pre-commit hook: `tools/git-hooks/pre-commit`
- Post-commit (test bundles): `tools/git-hooks/post-commit`
- Why the hook was slow: `docs/architecture/GIT_COMMIT_HOOK_SLOW.md`
- Paths: `pkg/paths` (`PreCommitDir`, `PreCommitResultsFile` → `results.json` under pre-commit dir), `pkg/precommit` (aggregate, write-result, read)
