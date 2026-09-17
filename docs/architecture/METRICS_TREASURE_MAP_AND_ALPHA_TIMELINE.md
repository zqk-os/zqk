# Metrics treasure map, timeline, and alpha launch signals

**Last Verified:** 2026-08-31


**Status:** Observability reference + planning aid  
**Tags:** `metrics`, `alpha`, `edd`, `pcs`, `scheduler`, `audit`, `quality`, `timeline`  
**Links:** [ASSESSMENT_AND_ONBOARDING_INDEX.md](./ASSESSMENT_AND_ONBOARDING_INDEX.md) · [CLI_ALPHA_LAUNCH_PLAN.md](./CLI_ALPHA_LAUNCH_PLAN.md) · `[REDACTED-ID]` · `[REDACTED-ID]` · `[REDACTED-ID]` (data cell program umbrella — **full program gate** before alpha; see [DATA_CELL_MODEL.md](./DATA_CELL_MODEL.md)) · `[REDACTED-ID]` (doc graph — **deferred**) · `GLS-1776207925199440000-aa236125` (glossary — *documentation graph*) · `DOC-1776207931662056000-409a1147` (`doc_entry` — CLI alpha plan) · `DOC-1776208254618247000-0db09abe` (`doc_entry` — [BACKLOG_REFS_AUTOLINK…](../process/architecture/BACKLOG_REFS_AUTOLINK_AND_CREATE_VENEERS.md))  
**Companion:** [METRICS_AND_TOTAL_PICTURE_GUIDE.md](../reports/METRICS_AND_TOTAL_PICTURE_GUIDE.md) (operational “where to look”) · [historical snapshots](../reports/historical/README.md) · [METRICS_DASHBOARD_AND_HISTORICAL_LAKE.md](./METRICS_DASHBOARD_AND_HISTORICAL_LAKE.md) (views + composite bundles)

This document **formalizes** where insight lives, what committed **baselines** say, how to read **load and failure** patterns, and how that feeds **alpha launch** planning. Live numbers change per machine — re-run the listed commands on your project root.

---

## 1. Treasure map — all major metric sources

| Domain | Location / command | What it tells you |
|--------|--------------------|-------------------|
| **CLI command health** | `.zqk/metrics/command_metrics.json` | Per normalized command: `invocation_count`, `success_count`, `failure_count`, `timeout_count`, durations, `error_rate`, `timeout_rate`. |
| **CLI command view** | `zqk system metrics` | Table; `--filter failures` \| `timeouts` \| `slow`; `--command "…"` for one command. |
| **Command metric objects** | `zqk object list command_metric` | Persisted snapshots (if emitted); filter `created_at` for windows. |
| **File lock contention** | `zqk system metrics file-lock view` | Current process buffer. |
| **File lock history** | `zqk object list file_lock_metric` | After `zqk system metrics file-lock flush`; filter measurement window. |
| **Scheduler raw events** | `.zqk/scheduler/scheduler-events.json` (+ rotations) | Job started/completed/failed, durations — **size-rolled**, not day-rolled. |
| **Scheduler summary** | `.zqk/scheduler/scheduler-metrics-summary.json` | Per-job aggregates; from **SCH-evag** (~15m) or trigger. |
| **Scheduler health** | `zqk scheduler events health` | Failures + slow jobs (>5m). Run `zqk scheduler trigger SCH-evag` if summary stale. |
| **Scheduler activity** | `zqk scheduler activity` | Queue, executing, completed, failed, missed triggers. |
| **Test bundle outcomes** | `.zqk/logs/scheduler/cvs/test-bundles/events.jsonl` | Per-bundle run, exit codes, suggested reruns. |
| **Test bundle health timeline** | `.zqk/logs/scheduler/cvs/test-bundles/health.jsonl` | Rolling pass/fail/fingerprint lines — **convergence fuel**. |
| **Object / stream volume TS** | `.zqk/metrics/object_volume/`, `stream_volume/` | Chunks from object-count-report; pruned per `metricsChunkRetentionDays` in `object_count_report.go`. |
| **Object-count / congruence** | `zqk system object-count-report` (PRUNED) | Disk vs logical counts, reports under `.zqk/logs/reports/`. |
| **Retention pressure** | `zqk system retention-status` (PRUNED) | Kinds over targets (e.g. `audit_event`, `file_lock_metric`). |
| **Pre-commit gate state** | `.zqk/pre-commit/results.json` | Lint / integrity / policy / docman — see [PRE_COMMIT_BACKGROUND_RESULTS.md](./PRE_COMMIT_BACKGROUND_RESULTS.md). |
| **PCS (project confidence)** | `zqk reports pcs` | Score + status; also sample JSON in repo (below). |
| **EDD (effort distribution discrepancy)** | `zqk reports edd` | Effort vs plan signal; git-enhanced when available. |
| **D&B (dependencies & blockers)** | `zqk reports blockers` | Blockers / dependencies narrative + JSON. |
| **Codebase / coverage snapshots** | `docs/reports/codebase-summary.json`, `critical-gaps.json` | Scale and gap hints (not live). |
| **Vetting matrix** | `docs/quality/CODEBASE_VETTING_MATRIX.csv` | Package-level human+agent vetting progress. |
| **Audit trail** | `.zqk/process/audit/` (CAS, high volume) | Query via storage/list patterns — not for git. |
| **MCP tool inventory** | `pkg/mcp/server_handlers_list.go` (help text) | Lists PCS, EDD, blockers tools for agents. |

---

## 2. Committed baseline snapshots (repo — not live)

These are **point-in-time** artifacts under `docs/reports/metrics/` (2025-12-29):

| Metric | Value (baseline file) | Interpretation |
|--------|-------------------------|----------------|
| **PCS** | ~98.62, status `excellent` | Strong project-confidence score at capture time. |
| **EDD** | 0, status `good_accuracy` | No effort distribution discrepancy flagged at baseline. |
| **Blockers** | Empty lists | No automated blockers at baseline. |

**Codebase summary** (`docs/reports/codebase-summary.json`, generated **2026-01-27**): ~1903 Go files, ~23% test coverage (line metric), ~862 markdown files tracked in that scan — use as **scale** context, not a gate.

**Critical gaps** (`docs/reports/critical-gaps.json`): includes low-coverage packages (e.g. MCP, specbuilder trees) — alpha should **not** promise full coverage; scope CLI + core paths first.

---

## 3. ASCII — conceptual timeline (engineering narrative)

Scale: `|` = month boundary, `*` = intensity of merge/convergence activity (qualitative).

```
2025          2026
 10   11   12 | 01   02   03   04   05   06
  |----|----|--|----|----|----|----|----|----|
  *  **    ***|**********|****  **    **    ..  ← doc+spec+scheduler hardening
              |    ^PCS/EDD baseline (Dec 29)
                    ^codebase summary (Jan)
                          ^object maintenance PRI waves / storage fixes
                               ^kindsynonyms + assessment docs (Apr)
                                    ^~~~ alpha prep window (target)
```

**Recent concrete signals (from development narrative, not auto-mined):**

- **Test bundles:** Failures concentrated in **`pkg/storage`** compile/vet (`errfmt` mismatches) — **fixed** so scheduler bundles can go green again.  
- **CLI synonym collision:** `roadmap` vs `priority_plan` — **fixed** in `pkg/kindsynonyms`.  
- **Process:** Alpha PRI + CVS created; architecture docs landed for assessment and greenfield seed.

---

## 4. ASCII — “usage / load” pattern (generic)

When `command_metrics.json` is healthy, **high invocations** on these usually indicate “project gravity”:

```
Relative CLI load (typical zqk dev day — illustrative)

object list      ████████████████████
object get       ███████████████
scheduler *      ██████████
system check     ████████
system metrics   ████
reports pcs/edd  ██
```

**Failing commands** (find in your tree):  
`zqk system metrics --filter failures` → then drill into `normalized_cmd` and correlate with **scheduler** failures in `events.jsonl`.

---

## 5. What’s working vs isn’t (from signals + architecture)

| Working | Isn’t / risk |
|---------|----------------|
| Rich **metrics plumbing** (CLI tracker, file metrics, scheduler JSONL). | **No single dashboard** — you must compose (see guide). |
| **PCS/EDD/Blockers** commands for agent-facing narrative. | Baselines in git are **stale**; re-run for decisions. |
| **Test-bundle health** as convergence evidence. | **Stale summaries** if stream sidecar not refreshed (see SPEC_STREAM_CELL_PEDAGOGY). |
| **Pre-commit** aggregation under `.zqk/pre-commit/`. | Depends on background jobs actually running. |
| **Large scale** codebase with clear architecture docs. | **Coverage** low in generated/specbuilder/MCP areas — alpha docs must set expectations. |

---

## 6. Ideas → alpha launch (actionable refs)

| Idea | Alpha tie-in |
|------|----------------|
| **Publish a “metrics checklist”** for operators (`system metrics`, `scheduler activity`, `health.jsonl` last line time). | First-run doc + troubleshooting (`[REDACTED-ID]`). |
| **Weekly PCS/EDD capture** to `docs/reports/metrics/` (automated or manual) | Trend line for confidence vs launch. |
| **Gate alpha on** 2 consecutive weeks **green** targeted bundles for `./cmd/zqk/app`, `./cmd/zqk/object`, `./internal/cli`. | Matches CLI_ALPHA_LAUNCH plan. |
| **Surface top N failing `normalized_cmd`** in release notes “known issues”. | Honest alpha. |
| **Vetting matrix** progress for `cmd/zqk/system` | Parallel track — don’t block alpha on full matrix. |
| **Retention dashboard** snippet in docs | Avoid surprise data loss perception. |

---

## 7. Tentative alpha target date (metrics-informed, realistic)

**Facts used:**

- Alpha PRI **`[REDACTED-ID]`** is **in_progress** as of creation; workstreams are **documentation, bootstrap, CLI UX, security copy, tests**.  
- Historical **PCS/EDD** baseline was **strong** (Dec 2025 snapshot) — but **stale** for a launch decision.  
- **Scale** (~1.9k Go files, heavy packages) implies **parallel** doc + CLI + smoke-test work, not a single weekend.  
- Industry-shaped heuristic: **6–10 weeks** from a clear plan + green critical paths for a **small** team to reach a **credible** CLI alpha.

**Proposed target window**

| Label | Date | Note |
|-------|------|------|
| **Earliest credible** | **2026-05-26** (Mon) | If bundle health stays green, init/docs land in ~4 weeks, and P0 alpha items complete. |
| **Recommended target** | **2026-06-09** (Mon) | **Default:** ~8 weeks from mid-April 2026; allows one slip week + buffer for onboarding polish. |
| **No-later-than (soft)** | **2026-06-30** | If convergence or storage/scheduler churn returns, hold scope; don’t ship broken init. |

**Already defined items** (must stay in scope for this window to be valid):

- **`CLI_ALPHA_LAUNCH_PLAN.md`** acceptance criteria.  
- **`GREENFIELD_TEAM_OPERATING_PLAN.md`** / **`EXPERTISE_ARCHITECTURE_AND_CODE_QUALITY.md`** quality bars.  
- Process objects: **`[REDACTED-ID]`**, **`[REDACTED-ID]`**.  
- Ongoing **test-bundle** discipline (`scan-tests` targeted, not only long foreground runs).

**Revise the date when:**

- `zqk system metrics --filter failures` shows **zero** P0 failures for core flows **and**  
- `health.jsonl` watermark is **fresh** for **14 days** and  
- README **golden path** is executed by a **clean** machine successfully.

---

## 8. Commands to refresh this report’s numbers

### Operator quick health checklist (alpha)

Before treating the tree as stable or moving a launch date, run these in order (same commands as the table in §6):

1. **Failures pulse:** `zqk system metrics --filter failures` — expect **no P0** for the core flows you care about (see [`CLI_ALPHA_LAUNCH_PLAN.md`](./CLI_ALPHA_LAUNCH_PLAN.md)).
2. **Scheduler pulse:** `zqk scheduler activity` — jobs completing; daemon healthy.
3. **Event stream (optional):** `zqk scheduler trigger SCH-evag && zqk scheduler events health` — quick sanity on scheduler events.
4. **Test-bundle watermark:** `tail -20 .zqk/logs/scheduler/cvs/test-bundles/health.jsonl` — if the file is **missing**, run targeted bundles first (`zqk scheduler scan-tests --package ./…`); see [`PRE_COMMIT_BACKGROUND_RESULTS.md`](./PRE_COMMIT_BACKGROUND_RESULTS.md) (refreshing test bundle jobs). `SCH-cvs-pipeline-tick` may skip until `health.jsonl` exists when `HEALTH_FILE_MISSING=skip` is set on the job.
5. **Narrative (trend vs launch):** `zqk reports pcs`, `edd`, `blockers` (JSON) — retain dated snapshots under `docs/reports/metrics/` when comparing week over week.

```bash
# Project confidence & EDD & blockers (AI-friendly JSON)
zqk reports pcs --format json
zqk reports edd --format json
zqk reports blockers --format json

# CLI + scheduler pulse
zqk system metrics --filter failures
zqk scheduler activity
zqk scheduler trigger SCH-evag && zqk scheduler events health

# Tail of bundle health (last lines)
tail -20 .zqk/logs/scheduler/cvs/test-bundles/health.jsonl
```

---

*Revision: update quarterly or when alpha scope changes.*
