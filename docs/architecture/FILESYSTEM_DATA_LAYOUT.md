# Filesystem data layout (human-scannable “data cupboards”)

## Top-level entry budget

Any directory the system **fills with data** (generated files, object shards, execution artifacts, overlays—not hand-authored docs) should keep **at most 100 top-level entries** (files **or** immediate subdirectories). Beyond that, use **bucketing** (hash prefix, job id segment, date prefix, etc.) so a human can list the directory and reason about it.

**Why:** Thousands of siblings in one folder are hard to browse, diff, and clean up; bucketing keeps each level small.

**Related:** `pkg/storage/bucketing_strategy_defaults.go`; scheduler execution state (`JobStateRegistry`) — see [Scheduler execution state](#scheduler-execution-state-jobstateregistry) below.

---

## Scheduler execution state (`JobStateRegistry`)

**Location:** `{projectRoot}/.zqk/scheduler/state/` — per-execution YAML so multiple processes agree on job run status (`pkg/scheduler/job_state_registry.go`).

**Workload buckets (names + job ID prefixes):** **Do not duplicate in docs.** The single source of truth is **`pkg/scheduler/scheduler_state_bucket.go`** — `schedulerBucketRules` and `schedulerBucketCatchAll`. Prefix order matters (narrow rules before broad ones); changing buckets means editing that slice and tests in `scheduler_state_bucket_test.go`.

**Current nested layout:**

```text
.zqk/scheduler/state/<bucket>/<job_id_segment>/<execution>.yaml
```

Plus **`locks/`** at `state/locks/` for cross-process locking (not a workload bucket).

**Legacy** (still read; daemon and `zqk scheduler state --migrate` relocate best-effort):

- Nested **without** the `<bucket>/` segment (older per-job dirs directly under `state/`).
- **Flat** top-level `{job_id}-{execution_id}.yaml` files.

On-disk **`_README.yaml`** under `state/` lists buckets using a legend **generated from** `schedulerBucketRules` — if it disagrees with the Go table, fix the code, not the hint file.

---

## Scheduler CVS vs test-bundle logs (under `.zqk/logs/scheduler/`)

**Layout:** **`cvs/`** is the convergence-session namespace. **`cvs/test-bundles/`** holds shared test-bundle JSONL and bundle job artifacts ( **`events.jsonl`**, **`health.jsonl`**, `SCH-run-*.stdout`, `bundle-*.log` ). **`cvs/cvs_measurement_events.jsonl`** sits at the **`cvs/`** root (CVS tick handoff, not bundle-scoped). Legacy flat **`scheduler/test-bundles/`** is renamed to **`scheduler/cvs/test-bundles/`** on first access (`pkg/scheduler/job_log_paths.go`). **Implementation:** `paths.SchedulerCVSSubdir`, `JobLogsCVSDir`, `JobLogsTestBundlesDir`.

---

## `scheduler_job` storage roles (not stream-backed)

**Current model:** `scheduler_job` uses **CAS** under `docs/architecture/scheduler_jobs/` (with bucketing), plus **`runtime_delta_current`** overlays for hot fields listed in `docs/architecture/_internal/configs/runtime_delta_fields.yaml` and object specs. It is **not** stream-backed: `StreamStorageEnabledForKind("scheduler_job")` is **false** (see `pkg/storage/stream_config.go`, `docs/architecture/_internal/configs/high_volume_kinds.yaml`).

### `stream_current/scheduler_job/`

**Legacy / stale.** Stream-backed kinds use `.zqk/state/stream_current/<kind>/<id>.yaml` (`pkg/storage/stream_current_state.go`). Because `scheduler_job` is not stream-backed, **new code should not write here** for this kind. If you see files under `stream_current/scheduler_job/` from an older experiment, they are safe to remove locally; they are not part of the supported read path for `scheduler_job`.

### `runtime_delta_current/scheduler_job/`

**Intended overlay** for hot fields on CAS-backed reads (`WriteRuntimeDeltaCurrentState`, `runtime_delta_current_state.go`). Whether a given update **only** touches runtime-delta fields is decided by `updateIsRuntimeDeltaOnly` (`pkg/storage/runtime_delta_config.go`): **every** key in the update map must be an allowed runtime-delta field (plus `expected_updated_at` ignored). If any other key is present, the storage layer performs a **full CAS write**, which shows up as churn under `docs/architecture/scheduler_jobs/`.

**Write-behind:** `scheduler_job` updates **skip** the write-behind/WAL fast path (`object_storage_file_update.go`) so metadata is visible synchronously after daemon restarts. That does **not** disable runtime-delta handling on the synchronous CAS path; it only means those updates are not queued behind the WAL worker.

**Why git history can still show many scheduler_job file changes:** operational updates that include **non–runtime-delta** keys (or mixed maps), lifecycle/spec-driven fields, or tooling that rewrites full objects will still move CAS blobs. Reducing churn means keeping hot-path updates to **runtime-delta-only** key sets when possible.

---

## References

- `pkg/scheduler/scheduler_state_bucket.go` — scheduler `state/` workload buckets (`schedulerBucketRules`).
- `docs/architecture/PRE_CHANGE_CHECKLIST.md` — field storage roles and runtime delta (section 4); filesystem layout (section 17).
- `pkg/storage/stream_config.go` — `scheduler_job` excluded from stream storage.
- `pkg/storage/object_storage_file_update.go` — write-behind skip for `scheduler_job`, `runtimeDeltaOnly` / CAS branches.
