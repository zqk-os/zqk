# ZQK Knowledge Kernel Configurations Reference

<!-- tags: reference, configuration, configs, storage, scheduler, ambient, timeouts, gates, retention -->

This document illuminates all active configuration files across the ZQK Knowledge Kernel (`.zqk/specs/configs/` and `config/`). It details their architectural purpose, schema, consuming Go subsystems, and operational guidance.

---

## Quick Reference Index

| Configuration File | Path | Architectural Purpose |
| :--- | :--- | :--- |
| **`runtime_delta_fields.yaml`** | `.zqk/specs/configs/` | Declares volatile runtime fields persisted as delta overlays |
| **`runtime_delta_kinds.yaml`** | `.zqk/specs/configs/` | Lists kinds subject to runtime delta updates (`scheduler_job`) |
| **`stream_delta_fields.yaml`** | `.zqk/specs/configs/` | Pruned field projections persisted into `.zqk/streams/` WALs |
| **`high_volume_kinds.yaml`** | `.zqk/specs/configs/` | Canonical storage routing (stream vs CAS) and retention tiers |
| **`namespaces_config.yaml`** | `.zqk/specs/configs/` | Object kind namespace mappings, hierarchy, and inference rules |
| **`pipeline_outcome_keys.yaml`** | `.zqk/specs/configs/` | Declarative registry generating Go `OutcomeKey` constants |
| **`retention_tolerance.yaml`** | `.zqk/specs/configs/` | Retention windows, max object counts, and protected statuses |
| **`scanner_config.yaml`** | `.zqk/specs/configs/` | File and directory exclusion patterns for filesystem scanners |
| **`scheduler_logs_config.yaml`** | `.zqk/specs/configs/` | Rolling log line retention for background scheduler jobs |
| **`scheduler_maintenance_config.yaml`** | `.zqk/specs/configs/` | Required survival maintenance jobs and pause exemptions |
| **`ambient.yaml`** | `config/` | Filesystem watcher ignore lists preventing OS descriptor exhaustion |
| **`command_timeouts.yaml`** | `config/` | Substring-matched execution timeouts and idle watchdogs |
| **`gates.yaml`** | `config/` | Single Source of Truth for `zqk-vet` release and hygiene gates |
| **`zqk.yaml`** | `config/` | Project settings, brand names, cache TTL, and validation budgets |

---

## 1. Storage & Runtime Optimization Configurations

### `runtime_delta_kinds.yaml` & `runtime_delta_fields.yaml`
- **Location:** `.zqk/specs/configs/`
- **Consuming Code:** `pkg/storage/runtime_delta.go`
- **Why It Exists:** In standard Content-Addressable Storage (CAS), any field change recalculates the SHA-256 hash and writes a brand new YAML blob. For objects like `scheduler_job`—which update timestamps (`last_run_at`, `next_run_at`, `updated_at`) every few seconds—writing full CAS files creates severe disk churn and index bloat.
- **Mechanism:** `runtime_delta_kinds.yaml` lists kinds managed by runtime deltas. `runtime_delta_fields.yaml` lists the specific fields (`status`, `updated_at`, `updated_by`, `last_run_at`, `next_run_at`). These fields are stored in a fast in-memory/delta overlay without invalidating the immutable underlying CAS entity.

### `stream_delta_fields.yaml`
- **Location:** `.zqk/specs/configs/`
- **Consuming Code:** `pkg/storage/stream_storage.go` (`recordForStream`)
- **Why It Exists:** Streaming WAL events (`.zqk/streams/<kind>/<date>_stream.json`) are append-only. Persisting full objects (with redundant documentation and empty optional attributes) wastes disk space and I/O bandwidth.
- **Mechanism:** Defines the exact subset of attributes serialized into stream lines for `change_journal_entry`, `metric`, and `command_metric`.

### `high_volume_kinds.yaml`
- **Location:** `.zqk/specs/configs/`
- **Consuming Code:** `pkg/storage/stream_config.go`
- **Why It Exists:** Single Source of Truth for storage routing:
  - `storage: stream`: Created directly in append-only day segments (`audit_event`, `command_metric`, `change_journal_entry`, `zqk_session`, `agent_instruction`).
  - `storage: cas`: High-volume but full-CRUD CAS objects (`scheduler_job`, `verification_matrix`).

---

## 2. Governance, Taxonomy & Pipeline Configurations

### `namespaces_config.yaml`
- **Location:** `.zqk/specs/configs/`
- **Consuming Code:** `pkg/validation/namespaces_config.go`
- **Why It Exists:** ZQK objects belong to formal namespaces (`zqk:kernel`, `domain:organizational`, `zqk:kernel:cli`, `zqk:kernel:metrics`). This file configures:
  1. Default namespace (`zqk:kernel`).
  2. Namespace layers (`zqk`, `domain`, `integration`).
  3. Validation regex pattern (`^(zqk|domain|integration):[a-z0-9_]+(:[a-z0-9_]+)*$`).
  4. Kind-to-namespace mappings (e.g. `backlog_item` -> `zqk:kernel`, `team` -> `domain:organizational`).
  5. Inference rules (e.g. `integration_*` -> `integration:{domain}`).

### `pipeline_outcome_keys.yaml`
- **Location:** `.zqk/specs/configs/`
- **Consuming Code:** `cmd/zqk/system/generate_pipeline_outcome_keys.go` -> `pkg/pipeline/outcome_keys.go`
- **Why It Exists:** Multi-stage execution pipelines pass context and telemetry via a shared `pipeline.Context.Outcome` map. Using raw strings introduces typos and silent bugs. This YAML registry generates typed constants (`OutcomeKeyFinalizeJobID`, `OutcomeKeyValidationError`, `OutcomeKeyCommitDone`) in Go.

### `retention_tolerance.yaml`
- **Location:** `.zqk/specs/configs/`
- **Consuming Code:** `pkg/scheduler/handlers_retention_tolerance.go`
- **Why It Exists:** Governs the `retention_tolerance` background maintenance job (`SCH-retention-tolerance`). Configures:
  - `archive_after`: Duration after which active objects transition to `archived`.
  - `cleanup_after`: Duration after which archived objects are deleted.
  - `max_count`: Upper bound on objects retained per kind (oldest purged first).
  - `protect_statuses`: Statuses that must never be purged (e.g. `["pending", "active"]`).

### `scanner_config.yaml`
- **Location:** `.zqk/specs/configs/`
- **Consuming Code:** `pkg/docman/discoverer.go`, `pkg/specbuilder/bldr_config_v1/`
- **Why It Exists:** Centralized directory and file exclusion rules for repository scanners (such as documentation discoverers, YAML scanners, and AST analyzers). Prevents traversal of vendor/build trees (`.git`, `node_modules`, `__pycache__`, `.venv`, `.zqk`) without hardcoding exclusion patterns across Go packages.

---

## 3. Daemon, Scheduler & Watcher Configurations

### `config/ambient.yaml`
- **Location:** `config/`
- **Consuming Code:** `pkg/ambient/watcher.go`
- **Why It Exists:** The ambient monitoring daemon observes repository changes to provide proactive assistance and health signals. If it watches the entire repo (including build outputs, vendor directories, and go packages), it exhausts OS file descriptors (`kqueue` on macOS, `inotify` on Linux).
- **Mechanism:** Defines `ignored_dirs` (`pkg`, `cmd`, `internal`, `bin`, `packs`, `dist`, `ext`, `docs`) so the watcher only monitors process and metadata directories.

### `config/command_timeouts.yaml`
- **Location:** `config/`
- **Consuming Code:** `pkg/cli/timeout_hook_timeout_calculation.go`
- **Why It Exists:** Matches CLI invocations against command substrings to set execution timeouts and idle watchdog intervals. Long-running operations (`system check`, `workflow whats-next`, `object draft promote`, `agent orchestrate`) get generous time and exemption from the 2-minute non-ZQK parent process cap (`child_max_timeout_exempt`).
- **Ambient Signal Recommendation:**
  > [!TIP]
  > When `command_metric` records excessive timeouts (`timeout_count > 3` or `timeout_rate > 20%`) for a specific command, the Ambient Daemon analyzes the baseline duration and emits a proactive recommendation:
  > ```text
  > ⚡ Recommendation: Command 'object export' timed out 4 times. Add an override rule in config/command_timeouts.yaml:
  >   - pattern: "object export"
  >     timeout: 15m
  >     child_max_timeout_exempt: true
  > ```

### `scheduler_maintenance_config.yaml`
- **Location:** `.zqk/specs/configs/`
- **Consuming Code:** `pkg/scheduler/maintenance_config.go`
- **Why It Exists:** Declares the minimum required survival maintenance jobs that must be registered in `.zqk/process/scheduler_job/`:
  - `SCH-val` (object validation)
  - `SCH-evag` (events aggregation)
  - `SCH-cache-prewarm` (warm CAS caches)
  - `SCH-retention-tolerance` (retention enforcement)
  - `SCH-maintenance-wal` (WAL compaction)
  Also declares `jobs_paused_schedule_exempt_job_ids`, allowing critical maintenance jobs to continue running even when user jobs are globally paused (`jobs_paused: true`).

### `scheduler_logs_config.yaml`
- **Location:** `.zqk/specs/configs/`
- **Consuming Code:** `pkg/scheduler/scheduler_job_logger.go`
- **Why It Exists:** Caps per-job log files (`.zqk/logs/scheduler/<job-id>/<job-id>.log`) to the last N lines (default: 500), preventing infinite disk growth.

---

## 4. Release Gates & Project Settings

### `config/gates.yaml`
- **Location:** `config/`
- **Consuming Code:** `pkg/vet/` (`zqk-vet`)
- **Why It Exists:** Single Source of Truth for codebase quality and release integrity:
  - `hygiene`: Path checks, permission enforcement, duplicate statement sequence analysis, raw goroutine bans, and CLI builder audits.
  - `tree_police`: Forbidden paths (`cmd/build`, `pkg/billing`), forbidden files, and allowed scripts.
  - `payload`: Validates module paths, blocks credential markers, and checks for hardcoded workstation paths.

### `config/zqk.yaml`
- **Location:** `config/`
- **Consuming Code:** `pkg/config/project_config.go`
- **Why It Exists:** Primary project configuration:
  - `brand`: Executable name (`zqk`) and product name.
  - `storage`: Mode (`file`), data directory (`.zqk/process`), and streaming enablement.
  - `logging`: Default profile (`human`) and level (`info`).
  - `cache`: In-memory cache enablement and TTL (3600s).
  - `validation`: Fail-fast timeout budgets per object (default: 5s, stuck timeout: 30s).
