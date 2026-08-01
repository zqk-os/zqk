# Why `git commit` Can Be Slow (Sample Analysis)

When you sample `git` during a slow commit (e.g. `sample git 2685 1`), the call graph shows:

```
cmd_commit → run_commit_hook → run_processes_parallel → finish_command → wait_or_whine → __wait4
```

**Git is not slow by itself.** It is blocked in `__wait4` waiting for the **pre-commit hook** (a child process) to exit. So the delay comes from whatever the hook runs.

## What the pre-commit hook runs (in order)

| Step | What runs | Typical cost |
|------|-----------|--------------|
| 1 | `scripts/sync-git-hooks.sh` | Quick |
| 2 | Branch check (no direct commit to main) | Quick |
| 3 | `gofmt -w` on staged Go files | Quick |
| 4 | **`golangci-lint run --timeout=90s`** on changed packages | Can be tens of seconds |
| 5 | `scripts/check-architecture-compliance.sh` (PRE_COMMIT=1) | Moderate |
| 6 | `scripts/check-logging-compliance.sh` on staged files | Quick |
| 7 | If staged `.md`: **`zqk automation docman-sync --timeout 45s`** | Can be slow; non-blocking (warning only) |
| 8 | If staged process YAML: **`zqk system check all --format json --timeout 45s`** | Often the culprit: storage/CLI contention; hook uses 45s |
| 9 | If staged `docs/architecture/`: **`scripts/check-system-integrity.sh`** (PRE_COMMIT=1, 90s) | Moderate; runs `zqk system check` for Tier 1 |
| 10 | Brand check (log only) | Quick |

When the **scheduler daemon** is running, the same storage/lock contention that causes `zqk object create scheduler_job` to time out can also make `zqk system check all` (and thus the hook) block or run for a long time.

## Fast hook (background results)

The hook has been reworked to **read a single file** (`.zqk/pre-commit/results.json`) updated by background jobs. The hook no longer runs lint, system check, or docman-sync itself, so it stays fast. See **`docs/architecture/PRE_COMMIT_BACKGROUND_RESULTS.md`** for:

- Categories (lint, integrity, policy, docman) and single **block** indicator
- How to run linter per package and policy checks as scheduler jobs
- Example scripts: `scripts/pre-commit-lint.sh`, `scripts/pre-commit-integrity.sh`, `scripts/pre-commit-policy.sh`
- CLI: `zqk pre-commit aggregate`, `zqk pre-commit write-result`

If background jobs are not set up yet, the hook allows the commit and prints a one-line warning. You can still use `git commit --no-verify` to skip the hook when needed.

## Other mitigations

- **Code-only commits:** Don’t stage YAML under `docs/architecture/` or `.md`; the hook skips the heavy steps (system check, docman-sync) and stays fast.
- **Bypass hook once (not for policy bypass):** `git commit --no-verify` skips the hook; use only when you know the change is safe and you’ll run checks elsewhere (e.g. CI).
- **Shorter timeout for system check in hook:** The hook template uses a lower timeout (e.g. 45s) for `zqk system check all` so that if the CLI blocks (e.g. storage contention), the hook fails fast and you can retry or commit with `--no-verify` after fixing.

## Future: background checks (lifecycle / integrity)

To keep visibility without pre-commit slowness, some checks can be moved to **background jobs** (scheduler), with a single fast pre-commit step that **reads the results** of those jobs:

- **Candidate steps to move to background:** lifecycle checks, full system check (Tier 1–4), docman-sync, or heavy integrity checks. These would run on a timer or on save (e.g. via file watcher or IDE integration).
- **Pre-commit then:** one lightweight check that reads the latest result file or job status (e.g. from `.zqk/pre-commit/results.json` or scheduler job history). If Tier 1 issues were reported since last run, block the commit and print a short summary; otherwise pass.
- **Benefits:** Same visibility (issues still detected and visible), but commit is fast. Full checks continue to run in CI.

## References

- Pre-commit hook template: `tools/git-hooks/pre-commit`
- Installed hook: `.git/hooks/pre-commit` (synced from template via `scripts/sync-git-hooks.sh`)
