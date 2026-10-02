# Scheduler job templates

Templates for creating **scheduler_job** objects via the CLI (process-data-cli-only: do not edit instance YAML under `.zqk/process/`).

Operator guide (kernel survival vs optional source-code pack, how to use the daemon): `docs/howto/SCHEDULER_AND_MAINTENANCE.md`.

## autofix_batch_cleanup_hourly.yaml

Hourly job that cleans up `.zqk/autofix/`:

- Removes processed batch files (FIXED_*, PROCESSED_*).
- Deletes unprocessed `AUTOFIX-*.json` files older than **AUTOFIX_BATCH_MAX_AGE_HOURS** (default 1 hour). Set to `0` to delete all unprocessed on each run.

**Create (first time):**

```bash
zqk object create scheduler_job --file scripts/scheduler_jobs/autofix_batch_cleanup_hourly.yaml --keep-file
```

**If job already exists (e.g. same ID):**

```bash
zqk object create scheduler_job --file scripts/scheduler_jobs/autofix_batch_cleanup_hourly.yaml --keep-file --force
```

**Trigger once (manual run):**

```bash
zqk scheduler trigger SCH-autofix-batch-cleanup
```

**Check it’s loaded and scheduled:**

```bash
zqk scheduler list
zqk scheduler status
```

**Check run history:**

```bash
zqk scheduler history --job-id SCH-autofix-batch-cleanup
zqk scheduler activity --job-id SCH-autofix-batch-cleanup
```

Policy and lifecycle: `docs/scheduler/SCHEDULER_JOB_POLICY_AND_LIFECYCLE.md`.

## autofix_process_pending.yaml

Timer **`run_wrapper`** for **`zqk system auto-fix-process-pending`** (id **`SCH-autofix-process-pending`**).  
Now includes glossary maintenance flags:

- `--sync-glossary` (dry-run glossary candidate scan after batch processing)
- `--sync-glossary-apply` (create missing terms, capped by `AUTOFIX_GLOSSARY_MAX_CREATE`)

Default cap in YAML: `AUTOFIX_GLOSSARY_MAX_CREATE=25`.

**Create/update from YAML:**

```bash
zqk object create scheduler_job --file scripts/scheduler_jobs/autofix_process_pending.yaml --keep-file --force
```

## retention_tolerance_catchall.yaml

Catch-all retention job: runs retention tolerance for all kinds in `retention_tolerance.yaml`. Used by `zqk system ensure-retention-jobs` when no job with `job_type: retention_tolerance` exists.

**Ensure jobs exist (creates or fixes job_type):**

```bash
zqk system ensure-retention-jobs
```

**Create manually (if needed):**

```bash
zqk object create scheduler_job --file scripts/scheduler_jobs/retention_tolerance_catchall.yaml
```

## SCH-016: Cleanup Old Command Metrics (bulk delete)

SCH-016 must use **bulk delete** instead of a per-ID loop to avoid re-creating storage bloat. Apply the patch (portable macOS/Linux) via CLI:

```bash
zqk object update SCH-016 --file scripts/sch016-command-patch.yaml
```

To re-enable the job after patching, promote it along its lifecycle: `zqk object promote SCH-016` (or `zqk scheduler resume SCH-016`; legacy alternative: `zqk object update SCH-016 --field "status=active"`).

See `scripts/sch016-command-patch.yaml` and rule `object-bulk-delete-not-loop.mdc`.

## audit_event_aggregation_default.yaml

Default audit event aggregation job (e.g. SCH-002). Used by `zqk system ensure-retention-jobs` when no job with `job_type: audit_event_aggregation` exists.

**Ensure jobs exist:**

```bash
zqk system ensure-retention-jobs
```

**Create manually:**

```bash
zqk object create scheduler_job --file scripts/scheduler_jobs/audit_event_aggregation_default.yaml
```


## convergence_orchestrate.yaml

Timer **`run_wrapper`** for **`./scripts/cvs_convergence_orchestrate.sh`** (persist session + Python rollup **`--apply`**, with **`--no-fail-on-gates`** for scheduled runs). **`schedule_expression`** on the job object is the source of truth for cadence (the committed YAML may ship an example such as **`*/15 * * * *`**). Set **`CONVERGENCE_SESSION_ID`** before enabling. **Disabled by default** (`enabled: false`). Id **`SCH-convergence-orchestrate`**.

**Create:**

```bash
zqk object create scheduler_job --file scripts/scheduler_jobs/convergence_orchestrate.yaml --keep-file
```

**If the job id already exists:** `zqk object create scheduler_job --file scripts/scheduler_jobs/convergence_orchestrate.yaml --keep-file --force`

**Manual trigger (after enabling):**

```bash
zqk scheduler trigger SCH-convergence-orchestrate
```

Update **`CONVERGENCE_SESSION_ID`** in **`environment_variables`** when your active **`CVS-*`** changes. Tests: **`go test ./cmd/zqk/scheduler -run ConvergenceOrchestrate -timeout 60s`**.

See **`docs/architecture/CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md`** (Appendix E).

## Persistent Daemons vs. Scheduled Jobs

### semantic_bridge.yaml & truth_sentinel.yaml

`scripts/scheduler_jobs/semantic_bridge.yaml` and `scripts/scheduler_jobs/truth_sentinel.yaml` describe **continuous background daemons** rather than recurring timer jobs:

- **Semantic Bridge (`SCH-semantic-bridge`)**: Continuously monitors the lifecycle WAL for completed requirements and specifications, updating vector index embeddings.
- **QA Truth Sentinel (`SCH-truth-sentinel`)**: Continuously monitors the lifecycle WAL for `in_progress` and `complete` transitions to run the AST structural auditor and QA gates.

> [!WARNING]
> **Anti-Orphan Discipline**: Running continuous daemons (`max_runtime_seconds: 0`) under timer scheduler loops without supervisor process management causes them to detach under PID 1 when parent tasks terminate, causing hidden resource leaks.
>
> **Recommended Lifecycle**:
> - **Supervised Daemons**: Manage via hostservice / OS supervision (`zqk system start`, LaunchAgent on macOS, systemd user service on Linux).
> - **One-Shot Evaluation**: For QA audits without long-running daemons, run `zqk system truth-sentinel --once --id <ID>`.
> - Both templates ship **disabled by default** (`enabled: false`) to prevent accidental orphaned detachments.
