# Stable log keys (POL-CODE-007) — migration status

**Last Verified:** 2026-08-31


Handlers and runtime code should log with **stable wire keys**: `prefix + "_suffix"` (often aligned to `JobType*` or a dedicated wire prefix like `cas_recovery_kind`), plus optional pooled fluent builders: **`logging.Fluent`** in `pkg/logging/fluent_builder.go` (scheduler wraps it as **`SLog`** in `pkg/scheduler/log_builder.go`; storage uses **`StorageLog`** in `pkg/storage/storage_log_builder.go`; domain wrappers in `pkg/scheduler/log_builder_domains.go`).

## Completed

- **`pkg/scheduler`** — scheduler daemon, lifecycle, job management/execution, trigger queue, dispatch pressure, maintenance runner, policy engine, CVS pipeline tick sync, convergence routing, job loader, notifications, change-journal aggregation pipeline, and **`pkg/scheduler/transceiver`** use `LogEvent*` constants from `scheduler_runtime_log_events.go` / `transceiver_log_events.go`. Handlers were migrated in earlier work (`*_log_events.go`, aggregation, run_wrapper, CAS recovery kind, etc.). Fluent logging uses **`SLog`** → **`logging.Fluent`** (`pkg/scheduler/log_builder.go`).
- **`pkg/storage`** — POL-CODE-007 wire keys for runtime logs (`storage_runtime_log_events*.go`); **`StorageLog`** wraps the same **`logging.Fluent`** builder (`storage_log_builder.go`) when attaching multiple structured fields. CAS recovery, WAL, journals, snapshots, listings, audit paths, etc., use `LogEvent*` constants rather than raw message strings.

## TODO (chip away toward alpha launch readiness)

Apply the same pattern outside completed areas:

| Area | Packages / paths (starting points) |
|------|-------------------------------------|
| CLI user-facing output | `cmd/zqk/`, `.zqk/cli/specs/`-driven commands — follow `AGENT_GUIDELINES.md`; pair with command spec codegen where applicable |
| Storage / CAS follow-ups | Optional: migrate remaining variadic `Logger.*(msg, logging.String…)` calls in `pkg/storage/` to **`StorageLog`** chains where it improves readability; **`EventLogger`** APIs (`LogInfo`, …) remain variadic unless extended separately |
| Validation / objects | `pkg/validation/`, object walks — align with FieldKey constants per project rules |
| MCP / integrations | Any package emitting structured logs for agent or external tools |
| Tests-only strings | Prefer leaving `t.Error` / assertion text human-readable unless a test asserts on log lines |

When adding keys:

1. Pick or add a **wire prefix** (job type, subsystem, or documented prefix like `scheduler_daemon`).
2. Define `LogEvent… = prefix + "_suffix"` in a `*_log_events.go` file in that package. Non-scheduler packages should not import **`pkg/scheduler`** for logging (import cycles): use **`logging.Logger` + constants**, and for multi-field lines use **`logging.Fluent`** or **`StorageLog`** (`pkg/storage`) rather than **`scheduler.SLog`**. For list/ops code that only has a **`logging.EventLogger`**, use **`eventLogger.Logger()`** to get the wrapped **`Logger`**, then **`StorageLog(…)`** (so you are not forced to keep variadic `LogInfo` / `LogDebug` with ad-hoc field slices).
3. Add a small prefix test (`strings.HasPrefix(evt, prefix)`) where practical.
4. Cross-reference **`docs/enforcement/AGENT_GUIDELINES.md`** and **`field-key-literal-scan.mdc`** / **`check-zqk-env-literals-repo.sh`** when touching field keys or `ZQK_*` env literals.
