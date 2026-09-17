# Integrity Resolution Plan

## Purpose

The integrity system exists to:

1. **Keep data clean and consistent** – Detect drift, duplicates, missing hashes, and policy/lifecycle violations so they can be corrected.
2. **Provide light assurances** – Ensure data is not being manipulated to mask other problems (e.g. hash integrity, CAS index consistency), without adding heavy process.

Integrity tooling must be **helpful, not cumbersome**. Resolution should be a clear, single-path experience where possible.

---

## Guidelines (resolution_strategy)

- **One primary path**: `zqk system check --auto-fix` should fix all auto-fixable integrity issues, including those that today require a separate `cleanup-duplicates` step. Users should not need to remember multiple commands or per-kind workflows for normal integrity resolution.
- **Stale CAS = auto-fix**: "Stale CAS version" (duplicate hash-based files for the same object ID) is resolved by the same cleanup logic as `cleanup-duplicates --hash-duplicates`. That logic runs automatically when `--auto-fix` is set and such issues are present, so the CLI does not advertise an auto-fixable issue without actually fixing it.
- **No misleading AutoFixable**: Every issue marked `AutoFixable: true` must be fixable in the same run when `--auto-fix` is used. If something requires a separate command, it is not marked auto-fixable; the message explains the manual step.
- **Resolution summary**: After a check run with `--auto-fix`, output briefly summarizes what was fixed (e.g. auto-fixed issue count, Stale CAS cleanup kinds and file count) so the user sees that the single command did the work.
- **Reminders/notifications**: Deferred. First we fix the actual disparities (one-command fix, Stale CAS in auto-fix, honest AutoFixable, resolution summary). Reminder text and notifications can be improved after this is in place.

---

## Gaps Addressed

| Gap | Resolution |
|-----|------------|
| Two-command flow (check + cleanup-duplicates) | Stale CAS cleanup runs automatically as part of `check --auto-fix` when results contain "Stale CAS version" issues. Programmatic entry point `RunHashDuplicatesCleanupForKinds` is used; no separate user invocation. |
| "Stale CAS version" marked AutoFixable but not fixed | When `--auto-fix` is set, a post-pass over results collects kinds with Stale CAS issues and runs hash-duplicates cleanup for those kinds. Issue remains AutoFixable and is now actually fixed in the same run. |
| No resolution summary | After auto-fix (and optional Stale CAS cleanup), a short "Resolution" block in the check output reports what was fixed (auto-fixed count, Stale CAS kinds and files quarantined). |
| Reminders only "view" not "fix" | Deferred; see above. |

---

## Implementation Summary

1. **Programmatic cleanup**
   - `RunHashDuplicatesCleanupForKinds(projectRoot, kinds, dryRun, logger)` in `cleanup_duplicates_helpers.go`: builds a `CleanupDuplicatesContext` with `HashDuplicates: true` (no cobra), runs `cleanupKind` for each kind, returns total deleted and errors (duplicates are always deleted; no quarantine).

2. **Stale CAS post-pass**
   - `RunStaleCASCleanupForResults(cmd, projectRoot, results, logger)`: scans `results` for issues whose message contains "Stale CAS version"; collects distinct kinds from those results; calls `RunHashDuplicatesCleanupForKinds` for those kinds; logs summary; returns kinds run and count (for resolution summary).

3. **Wiring**
   - **Sync path** (`check_impl.go`): After `checkKindsInParallel` and related handling, when `shouldAutoFix(cmd)`, call `RunStaleCASCleanupForResults(cmd, checkCtx.ProjectRoot, allResults, checkCtx.Logger)` before `outputResults`.
   - **Async path** (`show_validation_progress_completion.go`): When completing with results and `shouldAutoFix(vpc.Cmd)`, call `RunStaleCASCleanupForResults` before `outputResults`.

4. **Output**
   - When auto-fix was used and/or Stale CAS cleanup ran, add a "Resolution" section to the human-readable check output (in `output_table_helpers.go` or `check_impl_output.go`): e.g. "Auto-fixed: N issues. Stale CAS cleanup: kinds X, Y (M files deleted)." Data comes from run (e.g. return value from `RunStaleCASCleanupForResults` and existing auto-fixed counts).

5. **Docs**
   - Help text for `check --auto-fix` can note that it also runs Stale CAS cleanup (hash-duplicates) for affected kinds when such issues are found. No change to `cleanup-duplicates` CLI; it remains the standalone command for manual or scripted use.
   - Quarantine folder and analysis gap: see [QUARANTINE_AND_ANALYSIS.md](../../../docs/archive/system_health/QUARANTINE_AND_ANALYSIS.md) for what goes in `.zqk/system-health/quarantine/`, why we keep it, and current lack of analysis/cleanup tooling.

---

## Async check completion fallback

When the queue has been empty for 45+ seconds but not all tasks are accounted for (e.g. progress updates missing or cache lag), the run **force-completes** instead of declaring stuck: it stops the validator and proceeds with current results. This ensures `zqk system check --follow` (or async with wait) eventually finishes and reports what was validated, so users get a full report instead of an indefinite hang. Missing objects are logged; results include whatever was in cache. For a guaranteed full pass over all objects, use `zqk system check all --sync` (sync path processes every object in-process).

## Kind-from-ID as source of truth (wrong-kind violations)

To stop "wrong-kind" violations (e.g. REQ-026 validated as `audit_event`, MIL-033 as `mcp_session`, BAS-* as `criteria`), we use **object ID as the source of truth for kind** wherever the same ID can appear in multiple kind directories (e.g. stale or wrong CAS index).

- **Discovery / enqueue**: When the same ObjectID is seen from more than one kind directory, we prefer the kind that matches `inferKindFromID(objectID)` and only enqueue that task. If we only ever see the ID from a "wrong" directory, we still enqueue it once (fallback) so the object is not dropped.
- **Validation**: In the async validation callback, before running checks we set `effectiveKind := inferKindFromID(objectID)` and, when non-empty, use `effectiveKind` instead of the task’s `objectKind` for all validation (registration, instance, lifecycle, policy, integrity). So even when a task was enqueued from a wrong-kind directory, we validate against the correct spec/lifecycle. Auto-fix then adds defaults from the correct kind’s spec.
- **Persistence**: When creating or updating objects (e.g. scheduler_job, metrics), we use timestamps in UTC Z format (`2006-01-02T15:04:05Z`) and enum values that match the spec (e.g. `log_level`: `default`|`verbose`|`debug`, not `info`) so pre-write validation does not fail.

This gives a single rule: **for validation and auto-fix, kind is always inferred from the object ID when possible**, so we stop chasing wrong-kind lifecycle/instance errors and avoid auto-fix writing wrong-kind fields.

---

## Autofix batch pipeline and hash mismatches

When check runs with `--auto-fix` and scheduler batching (e.g. `--auto-fix-scheduler` and ≥10 fixable issues), fixable issues are written to `AUTOFIX-*.json` and processed by `zqk system auto-fix process-pending` (SCH-autofix-process-pending). The batch pipeline resolves **all** fixable issue types per object:

- **Instance validation** (pattern, required, type coercion, etc.) via `processInstanceValidationIssue` / spec fixer.
- **Integrity** (hash mismatch, missing hash, CAS index out of sync) via `processIssueForAutoFix` → `fixIntegrityIssue` (RemoveStaleIndexStrategy or fixCASIndexOutOfSync as appropriate).
- **Reference** (cache miss), **lifecycle**/policy (fix commands), etc.

Hash mismatch issues are included in batches when `AutoFixable: true` (see `IsIssueFixableForBatch`). Per-object fixes are applied during check (`autoFixIssues` → `batchFixHashMismatches` / `fixIntegrityIssue`) and again when process-pending runs; the second pass often skips because the fix was already applied. **One or two passes per object may be needed** until content and code stabilise (e.g. pattern normalisation changing content can create new hash mismatches that are fixed on the next run). After that, remaining Tier 2 / hash mismatches should be minimal.

---

## Out of Scope (for this pass)

- Reminders / notifications content (e.g. suggesting fix commands in reminder text).
- Root cause of why some tasks are unaccounted when queue is empty (validator/cache behavior); force-complete is a fallback.
- Progress display "what is being validated" (kind/phase); tracked separately.
- `--sync` removal; legacy flag remains for now.

---

## Testing

- Run `zqk system check --auto-fix` in a repo that has Stale CAS version issues; verify Stale CAS cleanup runs for the affected kinds and resolution summary appears.
- Run `zqk system check` (no auto-fix) with Stale CAS issues; verify no cleanup runs and message still tells user to run `cleanup-duplicates <kind> --hash-duplicates`.
- Unit test for `RunHashDuplicatesCleanupForKinds` with a temp dir and mock kinds; integration-style test for `RunStaleCASCleanupForResults` with synthetic results.
