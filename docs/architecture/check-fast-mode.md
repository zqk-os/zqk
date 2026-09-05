# Check Command `--fast` / reduced refs (partial / non-authoritative)

**Last Verified:** 2026-08-31


**Status:** Constrained — must not mutate; must not be treated as kernel health.

## What it does

`--fast` **or** `--check-refs=false` **only skips reference integrity checks** (`goal_refs`, dangling IDs, etc.).
Other check surfaces (lifecycle/shape, integrity hashes, instance validation) still run.

It is **not**:

- the same logic as promote/transition (dry-run)
- a CAS-hash attestation (“bytes unchanged since last validated write”)
- an authoritative “kernel is healthy” verdict
- a signal `system status` may label as `healthy` (status uses `partial_ok` at best)

## Hard rules (capability gates — natural language is not enough)

1. **Reduced refs + `--auto-fix` or `--force` is refused** (fail closed). Covers both `--fast` and `--check-refs=false`. Incident lesson: scheduled `check all --auto-fix --fast` demoted terminal objects while the rollup printed pass.
2. A green reduced-surface result is labeled **PARTIAL CHECK** in table and JSON (`partial_check: true` + message). Do not equate it with full `system check` health.
3. Mutating / release / pre-commit / kernel-health gates must use a **full** check (refs on).
4. **Autofix must not demote `status` → `error`.** Persistence of that demotion is refused (no validation skip + break_glass). Tracked: **`REDACTED`**.

## Intended use

- Local smoke / iteration when you knowingly accept skipped refs
- Narrow re-check with `--ids-from-file` after fixing known failures

## Future (TRACK)

Replace `--fast` with **CAS-hash attestation**: if content hash is unchanged since the last validated write, reuse that verdict; if changed, the writer owed validation. Tracked under autofix/status demote discipline **`REDACTED`**.

## See also

- `cmd/zqk/system/check_fast_contract.go` — refuse + labeling
- `cmd/zqk/system/check_impl_autofix.go` / `spec_auto_fixer_helpers.go` — demote hard-off + refuse persist
- `scripts/scheduler_jobs/autofix_run_periodic.yaml` — must not pass `--fast` with `--auto-fix`
- `scripts/kernel-health-snapshot-check.sh` — full check only (refs on); refuses `partial_check`
