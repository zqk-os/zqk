# Metrics dashboard views, performance boundaries, and historical snapshot lake

**Last Verified:** 2026-08-31


**Status:** Design (implementation follows CLI and storage work)  
**Companion:** [METRICS_AND_TOTAL_PICTURE_GUIDE.md](../reports/METRICS_AND_TOTAL_PICTURE_GUIDE.md) · [METRICS_TREASURE_MAP_AND_ALPHA_TIMELINE.md](./METRICS_TREASURE_MAP_AND_ALPHA_TIMELINE.md)  
**Historical artifacts:** [docs/reports/historical/README.md](../reports/historical/README.md)

## 1. Goals

- Make **load, quality, and process health** legible without turning the CLI into a real-time analytics engine.
- Separate **operational truth** (live files under `.zqk/`, scheduler CAS) from **published history** (immutable snapshots under `docs/reports/historical/`).
- Enable **trend and anomaly** reasoning by comparing same-schema reports across time.

## 2. Dashboard “views” (logical, not one UI)

Organize by **decision cadence** and **cost**:

| View | Audience | Primary inputs | Refresh |
|------|----------|----------------|---------|
| **Pulse** | Daily operator | `zqk scheduler activity`, `zqk system metrics --filter failures`, tail of `test-bundles/health.jsonl` | Minutes |
| **Inventory** | Platform / storage | `zqk system object-count-report` (PRUNED), `latest_object_count_snapshot.json`, retention | Hourly–daily |
| **Confidence** | Product / agents | `zqk reports pcs`, `edd`, `blockers` | Weekly or milestone |
| **Deep dive** | Incidents | Rotated `scheduler-events.json`, per-job logs, failing `normalized_cmd` from `command_metrics.json` | On demand |

**Performance rule:** Default CLI paths stay **read-mostly JSON/table**; anything that walks the full object store belongs in **scheduled jobs** or **background** (`scan-tests`, SCH-evag, object-count-report), not interactive loops.

## 3. What data we need (minimum set)

- **Command health:** `command_metrics.json` or `--format json` (failure/timeout/slow lists).
- **Scheduler:** `scheduler-metrics-summary.json`, activity output, optional `scheduler events health` when daemon + SCH-evag are current.
- **Object congruence:** object-count-report JSON + `reports_index.json` for history pointers.
- **Retention pressure:** `retention-status` JSON.
- **Quality signals:** PCS/EDD/blockers JSON.
- **Test convergence:** `health.jsonl` (or tail) for bundle pass/fail/fingerprint.
- **Codebase scale (optional):** `codebase-statistics.json`, `critical-gaps.json` for interpretation, not live ops.

## 4. Overlays and aggregates

**Overlays** (CAS/runtime overlays) are for **correctness and incremental indexes**, not for replacing scheduled aggregation. Use them when:

- A view needs **fast incremental updates** (e.g. index shards) with a clear merge contract.

For **dashboard-style aggregates** (sums, top-N, week-over-week deltas), prefer:

- **Deterministic CLI reports** writing JSON under `.zqk/logs/reports/` (already true for object-count-report).
- **Snapshot export** to `docs/reports/historical/<date>-…/` for immutability and git history.

If a future “aggregate overlay” is introduced, it should store **only derived fields** with **source report ids + content hashes** so comparisons remain explainable.

## 5. CLI information architecture (findability)

- **Namespace:** `zqk system metrics` (CLI health), `zqk system object-count-report` (PRUNED), `zqk system retention-status` (PRUNED), `zqk scheduler …`, `zqk reports …`.
- **Docs index:** `doc_entry` records for architecture reports; `docman` / `zqk object list doc_entry --filter group=architecture` for discovery.
- **Stable pointers:** Treasure map and “total picture” guide remain the **human index**; historical README lists **dated bundles**.

Avoid duplicating long command lists in multiple places—**link** to the guide and keep one canonical command block in the treasure map “refresh” section.

## 6. Historical snapshot lake (composite bundles)

Each capture is a **folder** plus optional **tar.gz**:

- **Folder:** `docs/reports/historical/<ISO-date>-<label>/`
- **Files:** JSON/txt artifacts from the commands in section 3, plus `composite-manifest.json` listing every file with **SHA-256** and size.
- **Archive:** `docs/reports/historical/<same-name>.tar.gz` for offline diff and backup.
- **Manifest checksum:** `composite-manifest.sha256` beside the manifest for quick integrity checks.

**Comparing runs:** Same `schema` in `composite-manifest.json`, same filenames → diff JSON with `jq`/JSON tools; flag **new keys**, **large deltas** in counts, or **missing files** as potential skew.

**Anomaly heuristics (manual or scripted):**

- PCS/EDD swing without corresponding backlog change → check git scope and report inputs.
- Object totals jump while **work item counts** flat → retention or import artifact.
- `normalized_cmd` error spike → correlate with scheduler failures and recent CLI changes.

## 7. Related process objects

- Alpha planning: `[REDACTED-ID]`, `[REDACTED-ID]` (see treasure map).
- Prompt/metrics productization index: [PRI-224_PROMPT_SYSTEMIZATION_OBJECTS.md](../process/enforcement/PRI-224_PROMPT_SYSTEMIZATION_OBJECTS.md) (includes dashboard-adjacent backlog items).

## 8. Follow-ups (not blocking snapshots)

- Optional `zqk system dashboard` subcommand that **reads manifest paths only** (no heavy scans).
- Automated weekly capture script invoking the guide’s sequence and writing a new historical folder.
- Explicit **schema version** bumps in PCS/EDD JSON when report definitions change.
