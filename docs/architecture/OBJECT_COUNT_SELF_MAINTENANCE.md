# Object Count: Self-Maintaining Outcome (Mandatory Context)

**Last Verified:** 2026-08-31


**Canonical reference:** `docs/architecture/OBJECT_COUNT_SELF_MAINTENANCE.md`  
**Cursor rule:** `.cursor/rules/object-count-self-maintenance.mdc` — when working on object count, retention, audit cleanup, or caches, read this doc and achieve the outcome below.

---

## The outcome we are asking for

**The system must maintain its own object count.** No cryptic incantations. No "run this script then that command with --no-cache." No week-long drift to 250k+ files followed by manual recovery.

- **Target:** Total object count (and total disk YAML files under `docs/process/`) stays near **~3k** (or the configured retention targets in `retention_tolerance.yaml`) without manual bulk deletes or cache refresh commands.
- **Report must be trustworthy:** `zqk system object-count-report` (PRUNED) totals (total_disk, total_object, total_internal) should align. The user must not need `--no-cache` or `zqk system check` to get accurate numbers after retention or deletes.
- **Single maintenance path:** There must be one clear, documented way to "keep object count in range" (e.g. scheduled jobs that run aggregation + retention, and cache stays in sync with deletes). Recovery from runaway count should be a documented, repeatable flow—not a one-off script and guesswork.

---

## What has gone wrong (and keeps going wrong)

1. **Retention and aggregation did not keep up.** Counts grew to 250k+ files. Either jobs were not running, not enabled, or not tuned (batch size, frequency, timeout) to handle the volume. Outcome: the system did not maintain itself.
2. **Object ID cache and disk diverged.** After bulk deletes (script, retention, etc.), the cache was not invalidated or refreshed. The report showed `total_object > total_disk` until the user ran `zqk system object-count-report --no-cache` (PRUNED) or `zqk system check`. Outcome: the user had to recite a "cryptic incantation" to see accurate numbers.
3. **No single "fix it" story.** The user had to (a) discover the unmanaged-file script, (b) run it with `--execute`, (c) understand why totals were wrong, (d) run with `--no-cache` or refresh the cache. Outcome: the system did not maintain itself and recovery was fragmented.

---

## What "done" looks like

- **Scheduled jobs are sufficient:** Retention and aggregation jobs run often enough and with enough capacity (batch size, max_runtime, frequency) so that under normal load they keep counts near target. No manual "push toward 3k" except after a one-time migration or disaster recovery.
- **Cache stays in sync with deletes:** The object ID cache is invalidated on single delete (via context), on CLI bulk delete (via `BulkInvalidateObjectIDCache` after success), and the object-count-report auto-corrects when cache is stale: if `total_object > total_disk`, it runs `CleanStaleCacheEntries` and re-runs the report with storage counts so the written report is accurate without `--no-cache`.
- **One documented maintenance path:** See **Single maintenance path** below. No scattered incantations.

---

## Single maintenance path

- **To keep object count in range:** Ensure retention and aggregation scheduler jobs are enabled and run regularly (see `docs/process/_internal/configs/retention_tolerance.yaml` and OBJECT_COUNT_MANAGEMENT.md). The object-count-report is accurate without `--no-cache` because cache is invalidated on deletes and the report self-corrects when `total_object > total_disk`.
- **To recover from runaway count:** Run the documented one-time cleanup (e.g. `scripts/delete_unmanaged_audit_yaml.py --execute` when jobs are insufficient), then run `zqk system object-count-report` (PRUNED) (no `--no-cache` needed; report will use storage counts if cache was stale and will clean stale entries).

---

## How to use this when the outcome is still not achieved

**If you are an agent or developer** and the user says object count is high again, or the report is wrong, or "we've been asking for this for a long time":

1. **Read this document** and `docs/process/observability/OBJECT_COUNT_MANAGEMENT.md`.
2. **Do not** suggest only one-off commands (e.g. "run the delete script and then --no-cache") unless you also fix or document the underlying cause (jobs not keeping up, cache not invalidated on delete).
3. **Do** propose or implement the missing piece: e.g. cache invalidation on bulk delete, stronger retention job defaults, or a single "maintenance" command that runs aggregation + retention + cache refresh and optionally the report.

**If you are the user** and the system has drifted again or the agent didn't achieve the outcome:

- Point the agent at this file: **"Read docs/architecture/OBJECT_COUNT_SELF_MAINTENANCE.md and accomplish the outcome we've been asking for."**
- The doc exists so you have a single, citable place to remind agents (and humans) what "maintains itself properly" means and what has repeatedly gone wrong.

---

## Scale and multi-agent workflows

As **multi-agent workflows** produce more audit events, metrics, and change-journal data, high-volume object count and retention must remain **reliable and performant without manual intervention**. The legacy one-file-per-object (CAS) format does not scale: it drives unbounded file count, slow list/count, and retention timeouts. The path to a self-maintaining system at higher load is:

- **High-volume kinds** use **stream storage** (append-only segments, delta-style records) and, for numeric series, the **timeseries** prototype (chunked, base+delta). See **HIGH_VOLUME_STORAGE_DEPRECATION.md**.
- Retention and aggregation jobs then operate on an efficient index (Count, OldestIDs, IDsOlderThan) instead of full scans. Maintenance stays hands-off as data volume grows.

Getting high-volume maintenance to a reliable, performant place is a prerequisite for upscaling capabilities without magnifying the problems already encountered (drift to 250k+ files, cache/report divergence, fragmented recovery).

## Related

- **Config and per-kind targets:** `docs/process/_internal/configs/retention_tolerance.yaml`
- **High-volume storage policy (stream/timeseries):** `docs/architecture/HIGH_VOLUME_STORAGE_DEPRECATION.md`
- **Operational steps and one-time cleanup:** `docs/process/observability/OBJECT_COUNT_MANAGEMENT.md`
- **Unmanaged-file delete script (emergency only):** `scripts/delete_unmanaged_audit_yaml.py` — see OBJECT_COUNT_MANAGEMENT.md section "One-time direct delete when jobs aren't enough"
