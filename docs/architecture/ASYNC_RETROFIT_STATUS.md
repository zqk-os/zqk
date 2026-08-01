# Async retrofit and CLI status

**Last updated:** 2026-02  
**Branch:** feature/pri-219  
**Purpose:** Single list of what’s done, what’s to do, and commands to upgrade or remove.

---

## What’s been done

### Async pattern and validation cache

- **Async pattern** (via **BindAsyncProgress** or **RunWithAsyncProgress**; operation type derived from command path where possible):
  - `system check`, `validate`, `init`, `status`, `sync`, `migrate`, `config-get`, `sync-cas-index`, `reminders`, `recover-cas`, `repair-yaml`, `repair-cas-corruption`, `generate-builders`, `generate-command-builders`, `generate-config-builders`, `generate-instance-builders`, `generate-lifecycle-builders`, `generate-profile-builders`, `generate-routing-builders`, `generate-trait-builders`, `generate-lifecycle-id-list`, `detect-spec-changes`, `feature-flags`, `check-baseline`, `check-async-baseline`, `scheduler-health-metrics test`, `whoami`, `discover-goals`, `align`, `seed-questions`, `fix-hash-mismatches`, `retention-tolerance`
  - `object create`, `update`, `delete`, `list`, `get`, `count`, `bulk-create`, `bulk-delete`, `bulk-get`, `bulk-update`, `template`, `related`, `path`, `neighbors`, `move`, `pplan-prev`, `pplan-next`, `pplan-current`
  - `internal create`, `internal update`, `internal delete`
  - `spec list`
  - `reports pcs`, `reports edd`, `reports blockers`, `reports quick`
  - `domain discover`, `domain register`
  - `organizational analyze-impact`, `propagate`, `record-change`, `sync`
  - `ontology import`
  - `scheduler start`, `stop`, `status`, `trigger`, `list`, `history`, `activity`, `submit`, `go test`, `test-failures list`, `test-failures rerun`, `test-failures analyze`, `health-check`
  - `utility validate-yaml`, `fix-registration`, `fix-hashes`
  - `callback notify`
  - `mcp list-tools`
- **Spec field `async: true`** in command spec schema and in YAML for object create/update/delete, internal create/update/delete, spec list. Glue code uses **BindAsyncProgress(cmd, runX)** so operation type is derived at runtime.
- **Helper:** **NewAsyncCommand(use, short, long, runE)** in `internal/cli` for hand-built commands that need only common flags and async.
- **Incremental validation cache:** shared per-project cache; on create/update/delete we invalidate and **EnqueueValidationForObject**; **emitIncrementalValidationComplete** so tests can block on a callback.
- **Context:** `CacheModeTestSync` for tests; `WithCommandOutputWriter` for capturing CLI output in tests.

### System check

- Async path only (no sync); output via `cmd.OutOrStdout` / `cmd.ErrOrStderr` and `logging.GetCommandOutputWriter(ctx)` (context override for tests).
- E2E test **TestSystemCheckAutoFix_E2E**: deterministic (coordinator reset, check cmd context set, `--workers 1`, **CopySpecsToTestRoot**), asserts JSON and on-disk CAS; 20s performance ceiling asserted.
- **TestObjectCreateDeleteFlow_UpdatesValidationCache**: create → callback → delete → list; temp dir cleanup best-effort.

### Test and tooling

- **Scoped regression:** `scripts/test-regression-scoped.sh` runs only tests covering updated code (integration, system async/output, context, logging, validation); `--no-e2e` to skip E2E.
- **CopySpecsToTestRoot** (pkg/testing) for CLI tests that need specs under the test root.
- Performance: **docs/architecture/system-check-performance-targets.md** has a “Verification” section noting the E2E test enforces the 20s ceiling.

### Docs

- **docs/architecture/async-cli-retrofit-and-validation-cache.md** — goals, pattern, incremental cache, implementation notes.
- **docs/architecture/system-check-performance-targets.md** — cold/warm targets and E2E verification.

---

## What’s yet to do

### Async retrofit (broader CLI)

- **Retrofit more commands** to `RunWithAsyncProgress` and consistent output (cmd Out/Err, or `logging.GetCommandOutputWriter` where applicable). Candidates (by group):
  - **scheduler** — start, stop, status, trigger, list, history, activity, submit, go test, test-failures (list, rerun, analyze), health-check (all done)
  - **reports** — pcs, edd, blockers, quick (done)
  - **spec** — list (done)
  - **domain** — discover, register (done)
  - **organizational** — analyze-impact, propagate, record-change, sync (done)
  - **ontology** — import (done)
  - **object** — create, update, delete, list, get, count, bulk-*, template, related, path, neighbors, move, pplan-* (all done)
  - **system** — check, validate, init, status, sync, migrate, config-get, sync-cas-index, reminders, recover-cas, repair-yaml, repair-cas-corruption, generate-*, detect-spec-changes, feature-flags, check-baseline, check-async-baseline, scheduler-health-metrics test, whoami, discover-goals, align, seed-questions, fix-hash-mismatches, retention-tolerance (all done)
  - **utility** — validate-yaml, fix-registration, fix-hashes (done)
  - **callback** — notify (done)
  - **mcp** — list-tools (done)
  - **Remaining (if any):** completion-builder, scenario-builder, or other one-off commands; verify with grep for `RunE:` / `cmd.RunE =` and add `BindAsyncProgress` where missing.
- **Retrofit checklist** (when doing a command):
  1. Prefer **`cli.BindAsyncProgress(cmd, runE)`** so operation type is derived from `cmd.CommandPath()` (e.g. `zqk spec list` → `spec_list`). Otherwise wrap with `RunWithAsyncProgress(cmd, args, "operation_type", runE)`.
  2. Use `cmd.OutOrStdout()` / `cmd.ErrOrStderr()` (and `logging.GetCommandOutputWriter(cmd.Context())` where result output goes through helpers).
  3. In tests: set `cmd.SetContext(runCtx)` on the subcommand if it’s reused; use `WithCommandOutputWriter` when capturing; reset coordinator if needed for isolation.

### Spec and command hygiene

- **Done:** Added CLI spec for `spec list` (see [COMMAND_SPEC_COVERAGE.md](./COMMAND_SPEC_COVERAGE.md)).
- **Done:** Removed stale specs for deprecated commands (code already removed):
  - `system/migrate_cas_command.yaml`
  - `system/migrate_audit_buckets_command.yaml`
  - `system/migrate_lifecycles_command.yaml`
- **Re-validate coverage:** Run `./scripts/validate_command_specs.sh` and update **[COMMAND_SPEC_COVERAGE.md](./COMMAND_SPEC_COVERAGE.md)** if the script or counts need updating.

### Optional follow-ups

- Wire scoped regression (or E2E) into CI.
- Consider grouping long-running or flaky tests in the scoped script with clear comments.

---

## Emerging patterns and abstractions

Patterns that make retrofitting more deterministic and builder-like:

### 1. **Derived operation type (done)**

- **Pattern:** Every retrofitted command passed a literal `operation_type` string (e.g. `"object_delete"`, `"spec_list"`), which is easy to typo or forget when adding subcommands.
- **Abstraction:** **`OperationTypeFromCommand(cmd)`** in `internal/cli/async_progress.go` derives the type from `cmd.CommandPath()` at runtime (e.g. `zqk spec list` → `spec_list`). **`BindAsyncProgress(cmd, runE)`** sets `cmd.RunE` to wrap `runE` with `RunWithAsyncProgress` using that derived type.
- **Use:** For hand-built commands (e.g. `spec list`, `internal create`), call `cli.BindAsyncProgress(cmd, runX)` instead of manually wrapping with a literal operation type. Example: `cmd/zqk/spec/list.go`.

### 2. **Builder-generated commands (not yet abstracted)**

- **Pattern:** Commands built from specs (e.g. `bldr_cli_cmd_v1.NewObjectDeleteCommandBuilder()`) return a `*cobra.Command`; the **call site** then overwrites `cmd.RunE` with `cli.RunWithAsyncProgress(cmd, args, "object_delete", runDelete)`. So async is applied manually in each command file.
- **Possible abstraction:** Add an **`async: true`** (or **`operation_type`**) field to the command spec schema. Codegen could then either:
  - Emit a builder that accepts a `runE` and sets `RunE` to `RunWithAsyncProgress(..., derivedType, runE)` (would require codegen to live in or depend on `internal/cli`), or
  - Emit a comment and a single line in the generated glue: `cli.BindAsyncProgress(cmd, runDelete)` so every async command follows the same pattern without a literal string.
- **Determinism:** Spec-driven async would make “is this command async?” and “what operation_type does it use?” answerable from the spec alone.

### 3. **Common flags + async (repeated)**

- **Pattern:** Retrofitted commands almost always call **`cli.AddCommonFlags(cmd)`** and then set **RunE** (async or not). Builders already support `WithCommonFlagsDefault(cli.AddCommonFlags)`; hand-built commands repeat the two steps.
- **Possible abstraction:** A small helper **`cli.NewAsyncCommand(use, short, long string, runE func(...) error) *cobra.Command`** that returns a command with common flags and `BindAsyncProgress` applied. Optional; only helps hand-built commands that don’t use the spec builders.

### 4. **Output routing (documented, not abstracted)**

- **Pattern:** Commands that write result output should use **`cmd.OutOrStdout()`** / **`cmd.ErrOrStderr()`** or **`logging.GetCommandOutputWriter(cmd.Context())`** so tests can inject a buffer via **`pkgctx.WithCommandOutputWriter`**.
- **Abstraction:** Already centralized in **`cli.WriteOutput`** and **`logging.GetCommandOutputWriter`**; the remaining work is to ensure every command that prints goes through them (no direct `os.Stdout` / `fmt.Fprint(cmd.OutOrStdout(), ...)` is fine).

### Summary

- **Done:** **`BindAsyncProgress(cmd, runE)`** is used for object create/update/delete, internal create/update/delete, system check, and spec list; operation type is derived from command path. Spec schema and YAML have **`async: true`** for those commands. **NewAsyncCommand(use, short, long, runE)** is available for hand-built commands that need only common flags and async.
- **Optional later:** Codegen could read **`async: true`** from spec and emit **BindAsyncProgress** in generated glue so new spec-driven commands get async by default.

---

## Commands to upgrade or remove

### Implemented but no CLI spec (upgrade)

| Command / group | Notes | Status |
|-----------------|--------|--------|
| **spec** | Only `spec list` | Done: added spec for `spec list` (see COMMAND_SPEC_COVERAGE.md). |

### Specs for removed commands (remove)

| Spec file | Status |
|-----------|--------|
| `system/migrate_cas_command.yaml` | Done: deleted (use `system migrate`). |
| `system/migrate_audit_buckets_command.yaml` | Done: deleted. |
| `system/migrate_lifecycles_command.yaml` | Done: deleted. |

### Commented / not registered (no change unless product decision)

- **root.go** has commented-out: `domain.NewBacklogCmd()`, `NewGoalCmd()`, `NewMilestoneCmd()`, `NewWorkflowCmd()`, `NewPriorityPlanCmd()`. These are not registered; leave as-is unless we want to re-enable or delete.

### Commands with specs and code (no change)

- All other groups (object, internal, system, scheduler, callback, docman, automation, reports, mcp, keystore, organizational, domain, ontology, utility, semantic) have both implementation and CLI specs (see [COMMAND_SPEC_COVERAGE.md](./COMMAND_SPEC_COVERAGE.md)); no removal suggested.

---

## Summary

- **Done:** Async pattern + validation cache for object, internal, and system check; E2E and integration tests deterministic; scoped regression script; output capture and performance assertions in place. **Broad async retrofit (2026-02):** All scheduler commands (submit, go test, test-failures, health-check), system commands (validate, init, status, sync, migrate, generate-*, repair-*, recover-cas, feature-flags, check-baseline, check-async-baseline, whoami, discover-goals, align, etc.), object read/bulk commands (list, get, count, bulk-*, template, related, path, neighbors, move, pplan-*), utility (validate-yaml, fix-registration, fix-hashes), callback notify, and mcp list-tools now use **BindAsyncProgress**.
- **To do:** Re-run `./scripts/validate_command_specs.sh` and update [COMMAND_SPEC_COVERAGE.md](./COMMAND_SPEC_COVERAGE.md) if needed; add progress callbacks for meaningful stage updates where long-running work occurs (e.g. go test, submit).
