# Cleanup Maintenance: On-Demand Config-Driven Filesystem and CLI Tasks

**Last Verified:** 2026-08-31


**Status:** Design  
**Purpose:** Extend the maintenance job pattern with an **on-demand cleanup** capability: a single, config-driven flow for delete, truncate, archive, move, group/archive, lightweight analysis (no metric duplication), and delegation to any build tool (make, ant, gradle, npm, etc.). Goals: **quick**, **intuitive**, and **flexible** for filesystem and CLI tasks.

**Related:** [MAINTENANCE_WAL_AND_RUNNER.md](./MAINTENANCE_WAL_AND_RUNNER.md) (maintenance config, WAL trigger pattern), [scheduler_maintenance_config.yaml](../process/_internal/configs/scheduler_maintenance_config.yaml) (required jobs).

---

## Problem

Teams need to:

- **Delete or truncate** files (e.g. old logs, temp files) on a schedule or on demand.
- **Archive** files (compress, move to archive dir, optionally by date/size).
- **Analyze** logs or outputs for trends/patterns without duplicating data already collected by metrics (scheduler_health_metric, audit aggregation, etc.).
- **Move, group, and archive** (e.g. group by date, move to `.zqk/archive/YYYY-MM-DD/`).
- Use **make** (or **ant**, **gradle**, **npm**) under the hood for complex or project-specific cleanup, while keeping simple cases **config-only** (no Makefile required).

Today, each of these is either a one-off script, a `run_wrapper` job with ad-hoc command, or scattered logic. There is no single **cleanup configuration** that is both expressive and quick to author (filesystem + CLI in one place, build-tool agnostic).

---

## Design

### 1. Cleanup as a maintenance-style job

- **Job type:** `cleanup` (new scheduler job type).
- **Trigger:** On-demand (manual or immediate) or timer. Optionally driven by a **cleanup request WAL** (like maintenance `cycle_requested`) so any component can request a cleanup run without knowing the job ID.
- **Config:** One cleanup config per job (or one project-level config). Config is YAML: list of **steps** executed in order. Each step has a **type** and type-specific **params**.

Cleanup jobs fit the same **maintenance config** pattern: optional entry in `scheduler_maintenance_config.yaml` (e.g. `SCH-cleanup`), template under `scripts/scheduler_jobs/`, ensured at daemon start or via `zqk system ensure-retention-jobs`. So adding a cleanup job is “add to config + add template + go.”

### 2. Cleanup config schema (conceptual)

**Location:** Project-configurable. Defaults: per-job `cleanup_config_path` (e.g. `.zqk/cleanup/cleanup.yaml`) or a single project file (e.g. `.zqk/cleanup/config.yaml`). Override via job field or env.

**Top level:**

```yaml
# Optional: default working directory (project root if omitted)
working_directory: "."   # or absolute path

steps:
  - type: delete_files
    path: ".zqk/logs/**/*.log"
    older_than: 7d
    # OR: keep_last_n: 100 (per file or per glob group)
  - type: truncate_files
    path: ".zqk/logs/log-events-human.log"
    keep_last_lines: 10000
  - type: archive
    path: ".zqk/logs/scheduler/**"
    archive_dir: ".zqk/archive/logs"
    older_than: 14d
    format: tar.gz
  - type: move
    path: ".zqk/tmp/*.json"
    dest: ".zqk/archive/tmp/"
    older_than: 1d
  - type: group_archive
    path: ".zqk/logs/scheduler/*/events.json"
    group_by: date   # or directory, prefix
    archive_dir: ".zqk/archive/scheduler-events"
    older_than: 7d
  - type: analyze
    path: ".zqk/logs/scheduler/**/*.events.json"
    summary: true
    # Contract: no new time-series or metric objects that duplicate scheduler_health_metric / audit.
    # Output: append to a single summary log or report file (e.g. .zqk/cleanup/analysis.log).
  - type: build_target
    tool: make
    file: Makefile
    target: cleanup
    # OR tool: npm, script: cleanup
    # OR tool: gradle, task: cleanup
    # OR tool: ant, target: cleanup
  - type: cli
    command: "./scripts/custom-cleanup.sh"
    args: ["--dry-run"]
```

**Step types (summary):**

| Type             | Purpose                                      | Key params                                                                 |
|------------------|----------------------------------------------|----------------------------------------------------------------------------|
| `delete_files`   | Remove files (optional age/count filter)     | `path` (glob), `older_than`, `keep_last_n`                                |
| `truncate_files` | Truncate file to last N lines (or bytes)     | `path`, `keep_last_lines` or `keep_last_bytes`                           |
| `archive`        | Compress and move to archive dir             | `path`, `archive_dir`, `older_than`, `format` (tar.gz, zip)                |
| `move`           | Move files to destination                    | `path`, `dest`, optional `older_than`                                     |
| `group_archive`  | Group by date/dir/prefix, then archive       | `path`, `group_by`, `archive_dir`, `older_than`                            |
| `analyze`        | Lightweight analysis (no metric duplication) | `path`, `summary`; output to designated log/report only                 |
| `build_target`   | Run a build tool target                      | `tool` (make, ant, gradle, npm), `file`/`target`/`script`/`task`          |
| `cli`            | Run arbitrary command                        | `command`, `args`                                                         |

**Rationale for “quick and intuitive”:**

- **Simple cases:** Only YAML. No Makefile or script required for delete/truncate/archive/move.
- **Complex cases:** One step type `build_target` or `cli` delegates to make/ant/gradle/npm or a script; the project stays in control.
- **Single config:** All filesystem and CLI cleanup in one place; order of steps is explicit.

### 3. Build-tool flexibility

- **Default:** `make` is the natural default for many projects (e.g. `make -f scripts/cleanup.mk cleanup`). Config can point to a project Makefile and target.
- **Other tools:** `build_target` step supports `tool: ant` (target + buildfile), `tool: gradle` (task), `tool: npm` (script). Handler resolves the binary (PATH or project-local), sets working directory, runs one target/script/task.
- **Escape hatch:** `cli` step runs any command + args; frontend or custom tooling can be invoked there.

Implementation can start with **make** and **cli** only; **ant** / **gradle** / **npm** can be added when needed so the contract stays small and quick to ship.

### 4. On-demand trigger and optional cleanup WAL

- **Trigger today:** `zqk scheduler trigger <cleanup-job-id>`. Job can be `trigger_type: immediate` (run once after load) or **manual** (only when triggered).
- **Optional cleanup WAL (future):** Like maintenance WAL, a `.zqk/wal/cleanup.wal` with events e.g. `cleanup_requested`. A single runner goroutine processes the queue; one scheduler job (or CLI) appends `cleanup_requested` and returns. This allows “run cleanup” without coupling callers to a specific job ID. Not required for v1; the trigger-based flow is enough for on-demand.

### 5. Analyze step: no metric duplication

- **Contract:** The `analyze` step must **not** create or update object kinds that already represent the same data (e.g. scheduler_health_metric, audit_aggregation_metric, or other metric/audit objects). It may:
  - Read logs and append **summaries** to a dedicated cleanup report/log (e.g. `.zqk/cleanup/analysis.log` or a single report file).
  - Produce one-off summaries (counts, simple trends) that are not stored as new time-series or metrics.
- **Implementation:** Handler for `analyze` steps should have a small, well-defined output surface (e.g. one file per run or one append-only log) and no writes to metric/audit object storage. This keeps cleanup “analysis” complementary to, not duplicating, existing metrics.

### 6. Integration with maintenance config

- **Optional required job:** In `scheduler_maintenance_config.yaml`, an entry like:
  - `id: SCH-cleanup`, `job_type: cleanup`, `template_file: scripts/scheduler_jobs/cleanup_on_demand.yaml`
- **Template:** Minimal scheduler_job YAML: `trigger_type: immediate` or `manual`, `cleanup_config_path: .zqk/cleanup/config.yaml` (or job-specific path). No schedule if purely on-demand.
- **Handler:** New `CleanupHandler` in the scheduler: loads cleanup config from `cleanup_config_path`, iterates steps, executes each (built-in for delete/truncate/archive/move/group_archive/analyze, subprocess for build_target/cli). Same timeout and logging as other handlers (per-job log dir, events).

### 7. Single track (implemented)

- **One input:** Config path defaults to `.zqk/cleanup/config.yaml` relative to project root. No job field for path in v0; the handler always uses this path so the flow is: job runs → load that file → execute steps in order.
- **Step order:** Steps run sequentially; first failure stops the run and returns an error.
- **Implemented step types:** `cli`, `build_target` (make only), `delete_files`, `truncate_files`. Others (archive, move, group_archive, analyze) can be added on the same track by extending the handler.

### 8. Implementation order (suggested)

1. **Config schema and loader:** Define cleanup config struct and load from path (job field or default).
2. **Handler and job type:** Add `job_type: cleanup`, `CleanupHandler`, register in `HandlerFactory`. Job fields: at least `cleanup_config_path` (optional; default path if unset).
3. **Step executors:** Implement in order: `delete_files`, `truncate_files`, `cli`, `build_target` (make + cli cover most cases). Then `archive`, `move`, `group_archive`, `analyze` with the no-metric-duplication contract.
4. **Maintenance config and template:** Add SCH-cleanup to config (optional), add `scripts/scheduler_jobs/cleanup_on_demand.yaml` template.
5. **Spec and instance builder:** Add `cleanup` to scheduler_job `job_type` enum; add `cleanup_config_path` field if stored on the job object.
6. **Optional:** Cleanup request WAL + runner for event-driven “run cleanup” without job ID.

---

## References

- [MAINTENANCE_WAL_AND_RUNNER.md](./MAINTENANCE_WAL_AND_RUNNER.md) — maintenance config, WAL trigger, ensure pattern.
- [scheduler_maintenance_config.yaml](../process/_internal/configs/scheduler_maintenance_config.yaml) — required jobs list.
- [PRE_CHANGE_CHECKLIST.md](./PRE_CHANGE_CHECKLIST.md) — process-data-cli-only (no direct edits to instance YAML under .zqk/process/); cleanup config is project/config, not instance data.
