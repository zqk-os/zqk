# Pre-Commit Checks: Persistent Jobs + Trigger-Only Callback

Lint, policy, and integrity checks **all use the same pattern**: one persistent scheduler job per category; timer runs = system operation only; when triggered from pre-commit, a callback runs that writes the pre-commit category and runs aggregation so the hook can block.

## Goal (applies to lint, policy, and integrity)

- **System operation (critical):** One persistent job per category runs on a schedule. It evaluates only what’s needed (e.g. files modified since last run where applicable), runs auto-fix where applicable, and reports failures until the underlying inputs change. No pre-commit wiring on timer runs.
- **User enforcement (pre-commit):** When the user runs “pre-commit” for that category (e.g. before commit or on demand), we trigger that same job and **only then** run a callback that writes the pre-commit category and runs aggregation so the hook can block if there are failures.

So for each category: one job, one check; timer runs = system health only; trigger-from-pre-commit runs = same check + callback that enforces the result in the hook.

## Categories and behavior

| Category   | Persistent job        | Timer run              | Pre-commit trigger                          |
|-----------|------------------------|------------------------|---------------------------------------------|
| **lint**  | SCH-pre-commit-lint    | Lint (e.g. incremental)| Trigger job + callback → write lint.json + aggregate |
| **policy**| SCH-pre-commit-policy  | Policy check           | Trigger job + callback → write policy.json + aggregate |
| **integrity** | SCH-012 or dedicated | Integrity check (incremental + auto-fix) | Trigger job + callback → write integrity.json + aggregate |

Lint is the only category that is hook-**blocking**; integrity and policy are non-blocking but still write their category and aggregate so the hook can show status.

## Current State

- **Lint / policy / integrity:** Each has a run_wrapper job that runs a script (e.g. `pre-commit-lint.sh`, `pre-commit-policy.sh`, `pre-commit-integrity.sh`). The script runs the check and then write-result + aggregate on every run (timer or manual).
- **SCH-012** (integrity_check): handler runs `zqk system check` directly; no pre-commit output.
- Scheduler already supports:
  - **TriggerJobWithCallback(ctx, jobID, opCallback)** – callback runs in the same process that executes the job (daemon).
  - **CallbackOnCompletion** on the job – invoked after every run when set. We want it **only** when the run was triggered from pre-commit.

## Design: Trigger-Only Callback (same for lint, policy, integrity)

### 1. One persistent job per category

- **Lint:** One job (e.g. SCH-pre-commit-lint) runs the linter (incremental where possible). Timer = no callback. Trigger with pre-commit origin = run callback.
- **Policy:** One job (e.g. SCH-pre-commit-policy) runs policy checks. Timer = no callback. Trigger with pre-commit origin = run callback.
- **Integrity:** One job (e.g. SCH-012 or SCH-pre-commit-integrity) with `job_type: integrity_check` or run_wrapper. Timer = incremental check + auto-fix, no callback. Trigger with pre-commit origin = run callback.

### 2. Callback only when pre-commit triggers

- **Option A – Run origin on trigger:**  
  Trigger API accepts an optional “origin” (e.g. `--pre-commit` or `origin=pre_commit`). Stored in run context. After execution, we invoke **CallbackOnCompletion only when run origin is pre_commit**. Timer runs do not set origin, so callback is never invoked on schedule. Each job definition can include `callback_on_completion` (e.g. `zqk pre-commit write-result --category=<lint|policy|integrity> ... && zqk pre-commit aggregate`), but that callback is only run when the run was triggered with pre-commit origin.
- **Option B – One-shot callback at trigger time:**  
  Trigger API accepts an optional “on-completion command” for this run only. Daemon runs it when this run completes. Timer runs never have a one-shot callback.

Either way: **callback is registered or only activated when pre-commit triggers the job.** Same rule for lint, policy, and integrity.

### 3. Incremental / since-last-run where applicable

- **Integrity:** Handler or check supports “only evaluate files modified since last run”; same failure reported until file mtime/content changes.
- **Lint / policy:** Similarly, prefer incremental scope (e.g. only changed files or packages) on timer runs so system operation stays fast; pre-commit trigger can run full or incremental as needed.

### 4. Pre-commit flow (all three categories)

- **Hook:** Unchanged: reads `.zqk/pre-commit/results.json`; blocks if `block` is true (lint is the blocking category).
- **“Run &lt;category&gt; for pre-commit”:** Trigger the persistent job for that category with “pre-commit” origin (Option A) or one-shot completion command (Option B). Callback writes that category’s JSON and runs aggregate. Optionally wait for completion so results.json is updated before the user commits again.
- **Timer:** Scheduler runs each job on its schedule; no callback; no pre-commit category write on timer alone.

## Implementation Hooks

- **Scheduler:** Add run origin (e.g. `trigger_origin`) to trigger path and execution context. When invoking callbacks, only invoke **CallbackOnCompletion** when `trigger_origin == "pre_commit"` (Option A). Or support one-shot completion command per trigger (Option B). Apply to all job types used for lint, policy, and integrity.
- **Handlers / scripts:** Lint and policy jobs (run_wrapper or dedicated handlers) run the check; integrity_check handler adds incremental/since-last-run behavior. Each can write result to a known path or pass ok/summary so the completion callback can call `write-result` correctly.
- **CLI:** `zqk scheduler trigger <job-id> --pre-commit` (and optionally `--wait`) for any of the three jobs. Same semantics for lint, policy, and integrity.
- **Job definitions:** Each of the three jobs may set `callback_on_completion` to a command that runs from project root, e.g. `zqk pre-commit write-result --category=<category> --ok=... --summary=... [--blocking=...] && zqk pre-commit aggregate`. Invoked only when the run was pre-commit-triggered.

## Summary

| Run type    | Who runs it   | Callback (write category + aggregate) |
|-------------|---------------|----------------------------------------|
| Timer       | Scheduler     | No – system operation only            |
| Pre-commit  | Trigger+flag  | Yes – only when triggered with pre-commit |

**Lint, policy, and integrity all behave the same way:** one persistent job per category; callback registered or only activated when pre-commit triggers it; same job used for both system operation and user enforcement.
