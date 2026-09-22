# Use the scheduler and maintenance jobs

The scheduler is the background organism: a daemon plus `scheduler_job` objects. It is useful for **every** project that wants the kernel to keep running after the shell closes. It is **not** a source-code linter. Lint, policy, and integrity timers are an optional **source-code pack** you turn on only when this checkout has code you intend to gate.

## Daily commands

```bash
./bin/zqk scheduler start
./bin/zqk scheduler status
./bin/zqk scheduler list
./bin/zqk scheduler history --job-id SCH-retention-tolerance
./bin/zqk scheduler trigger SCH-retention-tolerance
./bin/zqk scheduler issues
```

One-shot work (tests, a long command, anything you should not block a chat on):

```bash
./bin/zqk scheduler submit "./bin/zqk system check" --title "kernel check" --max-runtime 600
./bin/zqk scheduler scan-tests --package ./pkg/scheduler
```

CRUD, `object list`, and `whats-next` work **without** the daemon. Start it when you want timers, retention, and one-shots to keep running.

Ensure the kernel survival jobs exist (idempotent):

```bash
./bin/zqk system ensure-retention-jobs
```

`init --with-maintenance-jobs` is the same ensure step at first-run.

## Two layers (do not mix them)

| Layer | Purpose | Who needs it |
| :--- | :--- | :--- |
| **Kernel survival** | Keep CAS, caches, retention, and object validation healthy | Every project that runs the daemon |
| **Source-code pack** | Lint, logging/policy, kernel-integrity timers that feed git hooks | Checkouts that contain product source you gate before commit |
| **One-shots** | `submit` / `scan-tests` you ask for | Anyone with a long command or a test package |

A knowledge-only or orchestration project (backlog, Gantt, feed, no product tree) should run **kernel survival** and skip the source-code pack. Shipping those timers as required is how you get exit **126** on missing `scripts/pre-commit-*.sh`.

The list of jobs `ensure-retention-jobs` will mint lives in `.zqk/specs/configs/scheduler_maintenance_config.yaml` (`required_jobs`). That file is the contract. Templates live under `scripts/scheduler_jobs/`.

## Kernel survival (required on this SKU)

Read **title**, not the opaque id. `scheduler list` shows both. The id is a handle for `trigger` / `history`; the title is what the job does.

| Id (today) | Title | What it is for |
| :--- | :--- | :--- |
| `SCH-val` | Background Object Validations | Re-check objects after create/update/delete |
| `SCH-evag` | Scheduler Events Aggregation | Roll diagnostics into a metrics summary |
| `SCH-cache-prewarm` | (template title) | Warm kernel caches so first reads are not cold |
| `SCH-retention-tolerance` | Retention catch-all | Enforce retention policy across kinds |
| `SCH-audit-event-aggregation` | Audit aggregation | Compact audit events |
| `SCH-scheduler-job-retention` | Scheduler job retention | Reap completed one-shot `SCH-*` rows |
| `SCH-cleanup` | Cleanup | On-demand filesystem / lock hygiene |
| `SCH-maintenance-wal` | WAL maintenance | Storage write-ahead maintenance |
| `SCH-objcount-report` | Object-count report | Hourly count snapshot |
| `SCH-autofix-batch-cleanup` | Autofix batch cleanup | Drop processed autofix batches |
| `SCH-passive-test-sweeper` | Passive test sweeper | Idle test-bundle hygiene |
| `SCH-cap-orchestrator` | CAP orchestrator | Continuous agent loop tick (when CAP is in use) |

If a custom job's command or script is not present on your system, disable it rather than leaving an erroring timer:

```bash
./bin/zqk object update SCH-that-job --field "enabled=false"
```

## Source-Code Verification Jobs (Optional Pack)

These scheduled jobs are optional and provide automated test and lint checks. They cache output under `.zqk/pre-commit/` for fast evaluation during commit hooks:

| Id | What you think it is | What it actually is |
| :--- | :--- | :--- |
| `SCH-pre-commit-lint` | “Required maintenance” | Timer: `scripts/pre-commit-lint.sh` (golangci-lint / go vet) |
| `SCH-pre-commit-policy` | “Required maintenance” | Timer: logging + architecture policy scripts |
| `SCH-pre-commit-integrity` | “Required maintenance” | Timer: `system check` snapshot for the hook |

**Turn them on only if** (1) this project has source you gate, and (2) the matching scripts exist and are executable.

```bash
test -x scripts/pre-commit-lint.sh && \
  ./bin/zqk object create scheduler_job --file scripts/scheduler_jobs/pre_commit_lint.yaml --keep-file --force
test -x scripts/pre-commit-policy.sh && \
  ./bin/zqk object create scheduler_job --file scripts/scheduler_jobs/pre_commit_policy.yaml --keep-file --force
test -x scripts/pre-commit-integrity.sh && \
  ./bin/zqk object create scheduler_job --file scripts/scheduler_jobs/pre_commit_integrity.yaml --keep-file --force
./bin/zqk object update SCH-pre-commit-lint --field "enabled=true"
```

Force a hook-shaped run (writes the pre-commit aggregate the git hook reads):

```bash
./bin/zqk scheduler trigger SCH-pre-commit-lint --pre-commit
```

**Leave them off** for non-code orchestrations, knowledge-only kernels, and this Community SKU until you add those scripts. Missing scripts are not a configuration mystery — the job was never meant for that project shape.

If you need a one-off check without a timer:

```bash
./bin/zqk scheduler submit "./bin/zqk system check" --title "integrity once" --max-runtime 600
```

That is the orchestration-friendly substitute for `SCH-pre-commit-integrity`.

## Job Naming Conventions
 
 Scheduled jobs follow clear naming patterns:
 
 - **Format**: `SCH-<scope>-<noun>` using lowercase kebab-case (e.g., `SCH-retention-tolerance`, `SCH-cache-prewarm`, `SCH-audit-event-aggregation`).
 - **Scope**:
   - `kernel`: Core system maintenance, retention, and storage hygiene.
   - `source`: Codebase verification, automated testing, or linting.
 - **Title**: Human-readable summary displayed in `zqk scheduler list`.

- **Examples**: `SCH-retention-tolerance`, `SCH-cache-prewarm`, `SCH-audit-event-aggregation`.

## Job Layers and Categories

1. Does the command path live under `scripts/pre-commit-*.sh` or a language linter? → source pack.
2. Does it mention retention, WAL, cache, object validation, audit aggregation? → kernel survival.
3. Did you type `scheduler submit` / `scan-tests` this session? → one-shot; it will get its own `SCH-*` and should be reaped by job-retention.

`zqk scheduler issues` is the inbox for failed timers. Clear it after you have fixed or disabled the job, not to hide a missing script.

## Logs

Per-job output: `.zqk/logs/scheduler/jobs/<JOB-ID>/`. Daemon events: `.zqk/logs/scheduler/log-events.json`.

## See also

- Templates: `scripts/scheduler_jobs/README.md`
- Contract: `.zqk/specs/configs/scheduler_maintenance_config.yaml`
- First-run daemon: [Community first-run](../onboarding/COMMUNITY_FIRST_RUN.md)
