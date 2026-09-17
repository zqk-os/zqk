# Command orchestration (storage, session, scheduler check)

**Last Verified:** 2026-08-31


**Purpose:** Make it explicit which commands need storage, session, event coordination, and scheduler checks so root can orchestrate reliably and we avoid unnecessary blocking or hangs.

## Problem

Root's `PersistentPreRunE` used to decide what to initialize (storage pre-warm, session start, scheduler daemon check) with ad-hoc checks: `args[0] == "object"`, `args[0] == "system" && strings.HasPrefix(args[1], "generate-")`, etc. Adding a new command required editing root again, and the logic was scattered and easy to get wrong (e.g. `system generate-builders` hung because it still triggered storage/session and the CAS index queue).

## Approach: declarative requirements on commands

Commands declare what they need via **Cobra annotations**. Root resolves the **target command** from the current args (the command that will run), reads those annotations, and only runs the corresponding orchestration (storage, session, scheduler check).

- **Defined in:** `internal/cli/command_requirements.go`
- **Used in:** `cmd/zqk/root.go` `PersistentPreRunE`
- **Documentation:** This file

## Annotation keys and semantics

| Key | Values | Default | Effect when `false` |
|-----|--------|--------|----------------------|
| `zqk.requires_storage` | `true` / `false` | `true` only for commands under `object` or `internal` | Root does not pre-warm storage or create storage for session. |
| `zqk.requires_session` | `true` / `false` | `true` | Root does not start/reuse zqk_session (no `GetObjectStorageForCommand` for session). |
| `zqk.requires_scheduler_check` | `true` / `false` | `true` | Root skips scheduler daemon status check. |

Defaults are applied in `GetCommandRequirements`: storage is required when the resolved command is under the `object` or `internal` subtree; session and scheduler check are required otherwise. Annotations on the **target (leaf) command** override; parents can set annotations that apply to all children.

**Omission = use defaults.** If a command spec does not set these fields (or a hand-built command does not set the annotations), root uses the defaults above. **Existing specs need no retrofit**—only add `requires_session: false` (or the others) when you want to opt out.

## How to add or change a command

1. **New command that should not touch storage/session** (e.g. codegen, completion, auth):
   - When building the command, call `cli.RequireSession(cmd, false)` (and optionally `cli.RequireStorage(cmd, false)` if it must never create storage).
2. **New command that should not trigger scheduler check** (e.g. `scheduler start` / `stop` / `status`):
   - Call `cli.RequireSchedulerCheck(cmd, false)`.
3. **New command under `object`**:
   - No annotation needed; it gets `RequiresStorage = true` by default and will get storage and session.
4. **New command elsewhere that needs storage and session**:
   - No annotation needed; defaults are session and scheduler check true, storage false unless under `object` or `internal`. If the command needs storage (e.g. it calls `GetObjectStorageForCommand`), ensure root will create storage for it by either being under `object`/`internal` or by setting `RequiresSession = true` (session path creates storage on demand).

## Vital vs ignorable operations

**Vital operations** require a component (e.g. storage) to be running. Root only starts that component when the command declares it (via annotations / `GetCommandRequirements`). Vital ops (e.g. object CRUD) must not create the component on the fly; they run only when the command requested it and root set it in context. If a vital op needs storage and it is not available, the command should fail or block—the system must not silently progress as if the op succeeded.

**Ignorable operations** are best-effort (e.g. command audit event, optional metrics). Rule: **only use the component if it is already available; never create the component for an ignorable op.** If the component is not running, skip the op and continue. This avoids starting storage/CAS/orphan queues in PostRunE or other late paths and keeps process exit clean.

- **Check:** `cli.StorageAvailableForOptionalUse(cmd)` — true when storage was started by root and is in context.
- **Use:** For any optional use of storage (audit event, session touch, best-effort metrics), call this first; only proceed when true. Get the provider from `cli.GetStorageProvider(cmd.Context())`; never call `NewFileObjectStorage` or `GetObjectStorageForCommand` in those paths (they can create storage).
- **Session touch:** In PostRunE, touching the session (updated_at, title) is also ignorable: only when storage is available and session ID is in context; use storage from context, do not call `GetObjectStorageForCommand`.

## Helpers (internal/cli)

- `RequireSession(cmd, require bool)` – set `zqk.requires_session`
- `RequireStorage(cmd, require bool)` – set `zqk.requires_storage`
- `RequireSchedulerCheck(cmd, require bool)` – set `zqk.requires_scheduler_check`
- `GetCommandRequirements(root, args) CommandRequirements` – resolve target command from args and return requirements (used by root).
- `StorageAvailableForOptionalUse(cmd)` – true when storage is already in context (use for ignorable ops only).

## Commands that currently opt out

- **Session:** auth login, auth logout, scheduler status, completion, all `system generate-*` (generate-builders, generate-instance-builders, generate-config-builders, generate-command-builders, generate-lifecycle-builders, generate-profile-builders, generate-routing-builders, generate-trait-builders).
- **Scheduler check:** scheduler start, scheduler stop, scheduler status.

## Command specs (YAML) and codegen tie-in

Command specs in `.zqk/cli/specs/**/*_command.yaml` can declare orchestration so that **generated** builders and **runtime-built** commands get the right behavior without extra Go code.

- **Spec fields** (in `pkg/cli/command_spec.go` and `command_spec.schema.json`):
  - `requires_storage: false` – root will not pre-warm or create storage for this command.
  - `requires_session: false` – root will not start/reuse zqk_session.
  - `requires_scheduler_check: false` – root will skip scheduler daemon check.

- **Codegen** (`pkg/cli/command_builders/codegen.go`): When generating a command builder from a spec that has any of these set to `false`, the generated Go emits `cli.RequireStorage(cmd, false)` / `RequireSession` / `RequireSchedulerCheck` after `Build()`, so the built command has the correct annotations.

- **Runtime spec build** (`pkg/cli/command_spec_builder.go`): When a command is built from a loaded `CommandSpec` (e.g. via `CommandSpecBuilder.Build()` or `CRUDCommandSpecBuilder.Build()`), the builder applies the spec’s orchestration fields to the command’s annotations so root’s `GetCommandRequirements` sees them.

So for any command defined in YAML, set `requires_session: false` (or the others) in the spec; after codegen or when building from spec, the command will opt out of session (or storage / scheduler check) automatically. Hand-written commands can still call `cli.RequireSession(cmd, false)` etc. in code.

## References

- `internal/cli/command_requirements.go` – types, resolver, helpers
- `pkg/cli/command_spec.go` – `CommandSpec` orchestration fields
- `pkg/cli/command_spec_builder.go` – `applyOrchestrationFromSpec` (runtime build)
- `pkg/cli/command_builders/codegen.go` – emits `Require*` in generated builders
- `cmd/zqk/root.go` – `PersistentPreRunE` uses `cli.GetCommandRequirements(cmd, args)`
- `docs/architecture/GENERATE_BUILDERS_SAMPLE_ANALYSIS.md` – why generate-builders must not use storage/session

**Log routing:** Background/operational events (e.g. `orphan_cleanup_batch`, scheduler, generate-builders completion) are emitted with the system profile. The log router sends system (and MCP) profile logs to file destinations only, not to stdio, so they do not flood the terminal during `make build-all` or when the scheduler daemon is running. See `pkg/logging/router.go` (system/MCP skip stdio at all levels).
