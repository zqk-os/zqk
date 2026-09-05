# Multi-Binary CLI: Shared Runtime, Data Contract, and Integration Without Duplication

**Last Verified:** 2026-08-31


**Purpose:** Ensure that separate binaries (user-facing `zqk` and developer-focused e.g. `zqk-admin`) can coexist with **clean separation of concerns** while **avoiding duplication** of scheduler, metrics, aggregation, and storage logic, and while **sharing the same data** so processing and analysis remain unified.

**Status:** Design / reference. Not all patterns are implemented today; this document defines the target.

---

## 1. Principle: One Runtime, One Data Layout, Many Entrypoints

- **One project root** and **one data layout** (`.zqk/`, `docs/process/`) define the single source of truth. All binaries that operate on “this project” use the same root and the same paths.
- **One scheduler daemon** runs in one process (today: started by `zqk scheduler start`). It loads jobs from storage, runs handlers, and writes events. There is no “second scheduler” in the admin binary.
- **One storage and one WAL** back the object store and write-ahead log. Any binary that performs object or WAL writes uses the same `pkg/storage` and the same project root, so all data lands in the same place.
- **Shared libraries** implement storage, scheduler, metrics, aggregation, and path resolution. Both `zqk` and `zqk-admin` (or other binaries) **depend on the same packages**; there is no copy of “how to aggregate” or “how to record a command metric.”

**Separation** is achieved by **which commands each binary exposes** and **which operations each binary performs**, not by separate data stores or separate schedulers.

---

## 2. Shared Data Contract (Where Data Lives and Who Writes It)

### 2.1 Project Root and Paths

- **Resolution:** All binaries resolve project root the same way (e.g. `cli.ResolveProjectRoot(".")` or equivalent: current dir, `ZQK_PROJECT_ROOT`, `.zqk/state/session`, `.zqk/current_root`). Same algorithm → same root for the same workspace.
- **Layout:** `pkg/paths` defines all constants (`.zqk`, `cache/`, `metrics/`, `scheduler/`, `wal/`, `logs/`, etc.). Every component that needs a path uses `paths.ProjectPath(projectRoot, paths.ProjectDataDir, paths.MetricsDir, …)`. No hardcoded strings; one contract.

### 2.2 Distinct Data Domains (Same Store, Same Layout)

| Domain | Location / kind | Written by | Read by |
|--------|------------------|------------|---------|
| Object store (user + system objects) | `docs/process/` (by kind), CAS under `.zqk/` | zqk, zqk-admin (both via `pkg/storage`) | zqk, zqk-admin, scheduler handlers |
| WAL | `.zqk/wal/object.wal` | Any binary that uses `ObjectStorageProvider` with WAL enabled | Replay on init; compaction |
| Command metrics | `.zqk/metrics/command_metrics.json` | Any binary that uses `MetricsStore.RecordCommandExecution` with same schema | audit-report, dashboards |
| Scheduler config / state | `.zqk/scheduler/config.yaml`, `queue.json`, PID, etc. | zqk (scheduler daemon and CLI) | Scheduler daemon |
| Scheduler events | `.zqk/scheduler/diagnostics.jsonl` (JSONL) | Scheduler daemon (in zqk) | `zqk scheduler events aggregate`, dashboards |
| Audit events | Object store `audit_event` | zqk, zqk-admin (via storage Create) | SCH-002 aggregation handler |
| Change journal | Object store `change_journal_entry` | zqk (storage layer) | SCH-003 aggregation handler |
| Aggregated metrics | Object store `audit_aggregation_metric`, etc. | Scheduler handlers (in zqk process) | Reports, retention |

**Clean line of separation for data:**  
- **User-facing data:** backlog_item, policy, milestone, etc. — primarily created/updated by user flows in `zqk`.  
- **System/maintenance data:** audit_event, scheduler_job, metrics, WAL — written by either binary or by the scheduler, but **always through the same storage and path contract**.  
- **Developer-only artifacts:** generated builders, spec files under repo (e.g. `pkg/specbuilder/`, `.zqk/cli/specs/`) — written by `zqk-admin` (or `zqk` if we keep generate-* there). They live in the repo and optional caches, not in the object store’s “process” data.

So: **data domains are distinct by kind and purpose; the storage layout and access pattern are shared.** No duplicate stores.

---

## 3. Single Scheduler; Admin as a Consumer, Not a Second Daemon

### 3.1 One Scheduler Process

- The **scheduler daemon** runs inside the process that executed `zqk scheduler start` (today that is the main `zqk` binary). There is only one such process per project root.
- It **loads jobs** from the object store (`scheduler_job` objects) via `JobLoader.LoadJobs`. It does not care which binary created those jobs.
- It **runs handlers** (audit aggregation, retention, run_wrapper, etc.) in process. Handlers use `pkg/storage` and the same project root.

### 3.2 How the Admin Binary “Plugs In” to the Scheduler

- **Option A — Run as a job (run_wrapper):**  
  Create a `scheduler_job` with `job_type: run_wrapper`, `command: zqk-admin` (or full path), `command_args: ["system", "generate-builders", "--overwrite"]`, and a schedule or trigger. The **existing** scheduler daemon (zqk) runs that job by spawning `zqk-admin` as a subprocess. No new scheduler in zqk-admin; no duplication. zqk-admin is just another command the scheduler can run.

- **Option B — Trigger via queue:**  
  `zqk scheduler trigger --job-id SCH-xyz` (or a job that is “manual” or “immediate”) can start a job that runs `zqk-admin ...`. So a user or script can trigger admin tasks through the same scheduler and same queue.

- **Option C — No scheduler in zqk-admin:**  
  zqk-admin does **not** start or own the scheduler. It only needs to **read/write the same storage and same paths** so that when the main scheduler runs (in zqk), it sees a consistent world (e.g. updated specs or generated files). Optional: zqk-admin could **create** scheduler_job objects (e.g. “run generate-builders weekly”) that the main daemon then picks up.

So: **scheduler and background functionality stay in one place (zqk).** The separate admin binary plugs in by **being invoked by** that scheduler and by **writing data (objects, files) that the scheduler and its handlers already understand.**

---

## 4. Metrics and Aggregation: Same Interface, Same Path, Same Schema

### 4.1 Command Metrics

- **Contract:** `pkg/cli` defines `MetricsStore` and `CommandMetric` (invocation_count, success_count, failure_count, duration, etc.). `FileMetricsStore` writes to `projectRoot + .zqk/metrics/command_metrics.json` with a fixed schema.
- **Usage:** Any binary that wants to record command executions should:
  1. Resolve project root the same way as zqk.
  2. Use the same path: `filepath.Join(projectRoot, paths.ProjectDataDir, paths.MetricsDir, paths.CommandMetricsFile)`.
  3. Use the same interface (`MetricsStore.RecordCommandExecution`) and same struct so the file remains one consistent JSON document.

If zqk-admin uses `pkg/cli.NewFileMetricsStore(metricsPath)` and the same root, its runs appear in the same file. Optionally, add a field (e.g. `source: "zqk-admin"`) so reports can filter or label by binary. **No duplication:** one file, one schema, one library.

### 4.2 Audit and Aggregation

- **Write path:** Any code path that creates an `audit_event` (or `change_journal_entry`) does so via `ObjectStorageProvider.Create` with the same kind and schema. That can be from zqk or from zqk-admin if it ever needs to emit audit events (e.g. “spec_build_completed”). Same storage, same layout.
- **Aggregation:** Handlers like `AuditAggregationHandler` and `ChangeJournalAggregationHandler` live in **one place** (the scheduler in zqk). They **read** from the object store (audit_event, change_journal_entry) and **write** aggregated metrics. They do not care whether zqk or zqk-admin created the events. So: **processing and analysis are shared** — one aggregation pipeline for each domain (audit, change_journal). Data domains stay distinct (different kinds); the **operations** (time-window, group, aggregate, write metric) are the same pattern and live in one codebase.

### 4.3 Reaping the Benefit: Shared Processing, Distinct Data

- **Data domains:** audit_event, change_journal_entry, command_metric, scheduler_job, etc. are distinct. Each has its own schema and lifecycle.
- **Processing:** “Aggregate by time window, write a summary metric, optionally mark source records processed” is a **shared pattern**. Today it’s implemented per domain (audit vs change_journal) in the same scheduler handlers. If you add a new domain (e.g. “admin_operation_event”), you can either:
  - Reuse the same aggregation **pattern** (a new handler that reads that kind and writes a new metric kind), or
  - Extend the existing handler to support multiple kinds with the same time-window contract.
- So: **no duplication of “how to aggregate”** — you add handlers or kinds, not a second aggregation engine. And the admin binary does not run aggregation; it only writes events if needed. The scheduler (in zqk) does the rest.

---

## 5. Shared Libraries: What Both Binaries Use

To avoid duplication and keep integration clean, both zqk and zqk-admin should depend on the **same** packages for everything that touches data or scheduler:

| Package | Role | Used by zqk | Used by zqk-admin |
|--------|------|-------------|--------------------|
| `pkg/paths` | Project root and path constants | ✓ | ✓ |
| `pkg/storage` | ObjectStorageProvider, WAL, CAS | ✓ | ✓ (read/write objects, optional WAL) |
| `pkg/context` | SecurityContext, StorageContext | ✓ | ✓ |
| `pkg/cli` | MetricsStore, command builder, help | ✓ | ✓ (metrics, help) |
| `pkg/scheduler` | Job loader, handlers (if zqk-admin never runs daemon, it may only need job **definitions** or trigger client) | ✓ (daemon + handlers) | Optional (e.g. submit trigger or create job object) |
| `pkg/objects` / `pkg/specbuilder` | Kind mapping, specs, builders | ✓ | ✓ (generate-* commands) |
| `pkg/validation` | Validators, lifecycle | ✓ | ✓ if admin validates objects |

**Single implementation:** There is one `pkg/storage`, one `pkg/cli`, one `pkg/scheduler`. zqk-admin is a **different main package** (e.g. `cmd/zqk-admin`) that imports these packages and registers a **subset of commands** (generate-*, update-specs, detect-spec-changes, optionally repair-*). No copy of storage or aggregation logic.

---

## 6. Ensuring Clean Separation Without Duplication: Checklist

- **Data:** All binaries use the same project root and `pkg/paths`. No second `.zqk` or alternate layout.
- **Scheduler:** Only one daemon (zqk). zqk-admin is invoked by it via run_wrapper or trigger, or runs one-off from the CLI without starting a daemon.
- **Metrics:** Same `MetricsStore` interface and same path; same `CommandMetric` schema. Optionally tag by binary for reporting.
- **Aggregation:** Handlers live in the scheduler (zqk). Events written by any binary to the same store are aggregated by those handlers. No second aggregation pipeline.
- **Processing patterns:** New domains (new kinds or new event types) get new handlers or extended handlers in **one** codebase; they share the same time-window/grouping/merge patterns where applicable.
- **Code:** Shared logic lives in `pkg/`; `cmd/zqk` and `cmd/zqk-admin` only differ by which commands they register and which entrypoints they expose.

---

## 7. Summary

- **One runtime:** One project root, one data layout, one scheduler process, one storage and WAL.
- **Separation:** User-facing vs developer-facing is separation of **entrypoints and commands**, not of data or scheduler.
- **No duplication:** Both binaries use the same libraries and the same data contract; the admin binary plugs into scheduler and background functionality by **being run by** the scheduler and by **writing to the same store and metrics path**.
- **Shared processing:** Data domains (audit, change_journal, command_metric, etc.) are distinct; the operations that process and analyze them (aggregation, retention, reporting) are implemented once and consume data from the same store, regardless of which binary wrote it.

This gives you clean separation (different binaries for different users) without the cost of duplicated logic or fragmented data.
