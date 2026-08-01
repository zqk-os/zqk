# SCS facade metrics exchange contract

**Status:** Proposed pattern (architecture).

## Purpose

Define a stable way to emit source-control metrics from the facade layer while allowing operators to enable/disable metrics and configure output contract shape without changing command code.

## Decision

Use a two-layer design:

1. **Code-first internal event model** at the facade boundary (`ext/facade/scs`).
2. **Config-driven exchange contract** (system object) that maps internal events to external payload shape and sink format.

This keeps command paths simple while allowing future provider adapters (`git`, others) and transport formats to evolve.

## Scope

Applies to source-control operations exposed through the SCS facade, for example:

- current branch lookup
- branch existence checks
- checkout/create branch
- status/add/commit
- pull/push
- origin URL lookup

## Architecture pattern

### 1) Internal event contract (stable, minimal)

Facade operations emit a normalized event shape, regardless of provider:

- `component` (for example `scs`)
- `provider` (for example `git`)
- `operation` (for example `push`)
- `result` (`ok` or `error`)
- `duration_ms`
- optional attributes map (small, bounded keys)

This event model is internal and should remain small and capability-oriented.

### 2) Exchange contract object (configurable)

A system object controls how internal events are exported:

- global `enabled` toggle
- output `format` (`json` first; others can be added)
- field mapping profile (aliases, include/exclude, required fields)
- sink config (file/stream/http or project-standard sink)
- schema/version metadata for compatibility checks

At startup, load active contract config once, validate, cache, and use the cached config in the hot path.

### 3) Instrumentation seam

Implement metrics through an SCS client decorator:

- base client: provider adapter (`git` today)
- wrapper client: records timing/result and emits internal events
- command handlers depend only on `scs.Client`

This centralizes metrics and avoids scattering instrumentation across command code.

## Configuration as system object

Treat exchange config as a first-class object kind (for example `metrics_exchange_contract`) so it can be:

- versioned
- validated by lifecycle/status
- queried and audited like other system state

Process/object data updates must use `zqk` CLI, not direct YAML edits under `docs/architecture/`.

## Enable/disable behavior

- Disabled path must be near-zero overhead (fast no-op).
- Enabled path should emit bounded, non-blocking events.
- Failures in exporter path are best-effort and must not block source-control operations.

## Avro vs Protobuf vs JSON/YAML config

Use JSON/YAML for operator-facing configuration now, then add binary encoders if needed.

- **Protobuf:** efficient internal service boundaries, strict schema typing.
- **Avro:** strong schema evolution for analytics/data platform pipelines.
- **JSON/YAML config:** highest operator readability and easiest rollout.

Recommended sequence:

1. Internal normalized event model + config object.
2. JSON export profile.
3. Optional protobuf/avro encoders behind the same interface.

## Non-goals (for this pattern)

- Defining all sink implementations now.
- Committing to a single wire format for all future pipelines.
- Introducing per-command custom metric schemas.

## Related docs

- `docs/architecture/SEMANTIC_KERNEL_AND_CLI_FACADE.md`
- `docs/architecture/CLI_ASYNC_AND_PROGRESS.md`
- `docs/architecture/PRE_CHANGE_CHECKLIST.md`
