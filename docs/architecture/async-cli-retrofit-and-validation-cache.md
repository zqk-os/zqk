# Async CLI retrofit and incremental validation cache

**Status:** Active  
**Scope:** Object and internal commands first; then broader CLI.  
**Reference:** `cmd/pattern-cli` (standalone example), `pkg/coordination` (existing coordinator).

## Goals

1. **Async CLI pattern** — Every command: ack within ~1s, then progress at key stages (validation, execute), then complete or error. Notifications go through the coordinator to CLI user (brief) and logs (detailed).
2. **Staged validation** — Object validation has two logical stages: **core/spec** (required fields, types) and **lifecycle** (status-dependent rules, e.g. `planned` requires `milestone_refs`, `priority_plan_ref`). Both run before create/update; progress events distinguish the stages.
3. **Incremental validation cache** — On create/update/delete we already invalidate the validation (violation) cache. We also **enqueue** the affected object(s) for async validation so the cache is repopulated incrementally instead of only on full system check.

## Pattern (align with pattern-cli)

- **Submit** → handler receives, emits **ack/start** (via coordinator).
- **Validate** → core/spec first, then lifecycle; each stage can emit progress.
- **Result** → valid: proceed to execute (create/update/delete), emit progress; invalid: emit error + remedial/help, stop.
- **Complete** → emit completion or error through coordinator.

All notifications are routed through the existing **EventCoordinator** (`pkg/coordination`). Subscribers (CLI notifier, logging, audit, metrics) receive events concurrently. The **ProgressHelper** is used to emit start, progress, completion, and error.

## Validation stages

- **Stage: spec** — Required fields, types, patterns (GoValidator field validation). Lifecycle not yet checked.
- **Stage: lifecycle** — Status and transition rules; status-dependent required refs (e.g. backlog_item `planned` → `milestone_refs`, `priority_plan_ref`). If refs are missing, validation fails with clear message; creating missing refs is a separate flow (e.g. user creates milestone/plan first, or a future “fix” flow).

Progress callbacks (optional) are invoked at stage boundaries so the CLI can emit “Validating schema…”, “Validating lifecycle…”.

## Incremental validation cache

- **On create/update:** After cache handler runs (UpdateObjectIDCache, InvalidateListCacheForKind, InvalidateValidationCacheForCacheContext), we call **EnqueueValidationForObject(projectRoot, objectID, kind, filePath)**. This runs full validation (spec, lifecycle, refs) in a background goroutine via a minimal AsyncValidationContext (Cmd=nil) and writes the result to ValidationStateCache. Best-effort: if the process exits before the goroutine completes, the next `zqk system check` repopulates the cache.
- **On delete:** We only invalidate (no enqueue); the object is gone.
- **Best effort:** If the process exits before the background validation completes, the cache may stay empty until the next `zqk system check` or next create/update that triggers validation.

## Implementation notes

- **Async wrapper** — Object (and internal) commands use a wrapper that: gets coordinator, creates ProgressHelper, emits start, calls the existing RunE (with optional opts that carry the helper for validation progress), then emits complete or error. New commands get the same treatment via the same wrapper.
- **Validation progress** — `pkg/context` gains an optional `ValidationProgress` callback. Storage `validateObject` passes it into `ValidationOptions.ProgressCallback` when present. GoValidator calls it at spec and lifecycle boundaries.
- **Existing components** — Coordinator, ProgressHelper, CLI notifier adapter, cache handler, and InvalidateValidationCacheForCacheContext stay; we add EnqueueValidationForObject and the wrapper, and we add ProgressCallback to the validator options.

## Retrofit checklist

When retrofitting a command to the async pattern:

1. Wrap its `RunE` with `RunWithAsyncProgress(cmd, args, "operation_type", runE)`.
2. Use `cmd.OutOrStdout()` / `cmd.ErrOrStderr()` (and `logging.GetCommandOutputWriter(cmd.Context())` where result output goes through helpers).
3. In tests: set `cmd.SetContext(runCtx)` on the subcommand if it’s reused; use `WithCommandOutputWriter` when capturing output; reset the coordinator if needed for isolation.

**Status and candidate groups:** See **docs/architecture/ASYNC_RETROFIT_STATUS.md** for what’s done, what’s left (scheduler, reports, spec, domain, organizational, utility, system), and spec-hygiene notes.

## References

- `cmd/pattern-cli/` — Standalone async pattern and output coordinator.
- `pkg/coordination/` — EventCoordinator, ProgressHelper, EventContext.
- `docs/architecture/ASYNC_RETROFIT_STATUS.md` — Done/to-do and retrofit candidate list.
- `docs/architecture/CLI_PERFORMANCE_AND_CONSISTENCY.md` — Performance and consistency.
- `docs/architecture/PRE_CHANGE_CHECKLIST.md` — Pre-change checklist.
