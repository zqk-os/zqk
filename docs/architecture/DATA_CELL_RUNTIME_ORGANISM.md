> [!WARNING]
> **ARCHIVED DOCUMENT**: The primary commands referenced in this architectural document have been pruned from the `zqk` CLI.

# Data cell — runtime organism (protocol v1)

**Last Verified:** 2026-08-31


**Canonical model:** The full **data cell** abstraction (logical unit + operational envelope, multiple **storage profiles**, optional multi-kind grouping) is **[DATA_CELL_MODEL.md](./DATA_CELL_MODEL.md)**. **This document** is one **implemented slice**: operator-edited runtime files + `pkg/datacell` path API — not the whole cell platform. For **what is actually wired today** (membrane interfaces, read-path adapters, and what is still roadmap), see [DATA_CELL_MODEL.md](./DATA_CELL_MODEL.md) → *Formalization status* and *Membrane and coordinator — what exists today (v1)*.

**Status:** Implemented layout + Go API (`pkg/datacell`). **Spec / mapping closure:** `[REDACTED-ID]` (formal mapping recorded in [DATA_CELL_MODEL.md](./DATA_CELL_MODEL.md) § *[REDACTED-ID] — Formal mapping and sequencing*).  
**Isolated E2E (synthetic project, no `.zqk/process` migration):** [`pkg/datacell/stream_organism_e2e_test.go`](../../pkg/datacell/stream_organism_e2e_test.go) — materialized `spec_index.json`, optional `configs/high_volume_kinds.yaml` (same shape as repo), `datacellregistry` load, `FilterDescriptorsByStorageProfile`, profile contracts vs descriptors, stream-vs-spec alignment, `PostRetentionStreamStewardship` smoke, operational envelope + scheduler policy dry-run, runtime paths/manifest. [`cmd/zqk/system/data_cells_stream_organism_test.go`](../../cmd/zqk/system/data_cells_stream_organism_test.go) — in-process `zqk system data-cells` (PRUNED) (table + JSON, `--kind` including table banner/summary, `--json --json-envelope` with optional `.zqk/stream_summary/test_bundle_health.json`, unknown kind error, `--json-envelope` without `--json`) with `ZQK_TEST_ROOT`. Run: `go test ./pkg/datacell -run TestStreamOrganism -timeout 60s` and `go test ./cmd/zqk/system -run TestDataCells_StreamOrganism -timeout 60s`.  
**Related:** [DATA_CELL_MODEL.md](./DATA_CELL_MODEL.md), [SPEC_RUNTIME_AND_PATH_CACHE_VISION.md](./SPEC_RUNTIME_AND_PATH_CACHE_VISION.md) (keyed path cache and future cell hardening), [CLI_EXTERNAL_HOOK_PROTOCOL.md](./CLI_EXTERNAL_HOOK_PROTOCOL.md), [SPEC_ORIGIN_PLANE.md](./SPEC_ORIGIN_PLANE.md) (streams vs CAS). **Lite file** (runtime JSON such as feature flags): glossary `GLS-1776253895684744000-7799fa3e`.

## Performance

Microbenchmarks (foreground `go test`; use `-run '^$'` so only benchmarks run—especially in `./pkg/scheduler`, where a full package test run includes slow integration tests):

| Area | Command | Order of magnitude (representative) |
|------|---------|-------------------------------------|
| Operational envelope lookup | `go test ./pkg/datacell -run '^$' -bench 'BenchmarkStreamOrganism_OperationalEnvelopeForProfile' -benchmem -timeout 60s` | Single-digit ns/op |
| Envelope compact summary | same package, `BenchmarkStreamOrganism_OperationalEnvelope_CompactSummary` | Sub-microsecond to low µs/op |
| Spec index → descriptors (in-memory) | `BenchmarkStreamOrganism_DataCellDescriptorsFromSpecIndex` | Usually sub-µs/op (no disk) |
| Load descriptors (synthetic tree) | `BenchmarkStreamOrganism_LoadDataCellDescriptors` | Often ~100 µs/op (read + JSON + transform); grows with `spec_index.json` size |
| Scheduler policy dry-run (temp dir) | `go test ./pkg/scheduler -run '^$' -bench 'BenchmarkStreamOrganism_DryRunDataCellEnvelopePolicy' -benchmem -timeout 60s` | ~10 µs/op, a few KiB/op (allocates registry + engine + report each iteration) |
| Policy `Evaluate` only (same temp dir) | `BenchmarkStreamOrganism_PolicyEvaluate_DataCellEnvelope` | Lower than full dry-run; still touches scheduler state/policy paths under `.zqk/scheduler/` |

Treat numbers as relative signals on your machine; CI and laptops differ.

**Where perf issues tend to appear (not the envelope map):**

- **`zqk system data-cells` (PRUNED)** resolves descriptors via `datacellregistry.DescriptorReadModelForProject` when `objects.GetGlobalSpecLoader().SpecCacheRevision() != 0` (revision-correlated snapshot + per-root process cache; see `descriptor_read_model_cache.go`), and falls back to `LoadDataCellDescriptors` when the revision is zero (early init / tests). **`datacellregistry.InvalidateDescriptorReadModelCache`** runs after **`generate-spec-index`**, **`update-specs`** refresh, spec-writer field ops refresh, and spec-origination materialize — so the next `data-cells` read reloads from the new `spec_index.json`. If listing hundreds of kinds becomes hot, the lever is still **materialized spec index** churn and disk read size (see [SPEC_ORIGIN_PLANE.md](./SPEC_ORIGIN_PLANE.md)), not `OperationalEnvelopeForProfile`.
- **Full dry-run vs `Evaluate`:** compare `DryRunDataCellEnvelopePolicy` with `PolicyEvaluate_DataCellEnvelope` to see allocation/setup cost versus steady-state policy evaluation.
- **Repo-sized `spec_index.json`:** the tiny synthetic benchmark understates cost; use `go test ./pkg/datacell -run '^$' -bench 'BenchmarkStreamOrganism_LoadDataCellDescriptors' -benchmem` against a real checkout or run the stream-organism E2E tests under `scan-tests` for integration-level signal.

### Metrics feedback loop (feeds you already have)

The design stack already exposes **aggregate command timing** and **scheduler job** signals; use those first to drive optimization (trends, regressions, load growth), then **microbenchmarks** for pinpointing a change.

| Signal | Where | Use for data-cell / stream work |
|--------|--------|----------------------------------|
| **CLI command duration & frequency** | Root `Execute()` wires `pkg/cli.TimeoutHook` → `FileMetricsStore` → **`.zqk/metrics/command_metrics.json`** (`paths.CommandMetricsFile`). | **`zqk system metrics`** (`--command` filter, **`--filter slow`**, `--format json`) tracks whole-invocation wall time for `zqk system data-cells (PRUNED) …` as the spec plane grows—this is the natural feed for “load patterns” without new code. |
| **Reports** | Improvement / audit flows that read the same store. | Surfaces slow-command and churn-style hints alongside other commands; good for periodic review. |
| **Scheduler jobs** | **`pkg/scheduler.SchedulerMetricsCollector`** (job lifecycle, execution duration, failures). | When envelope-scoped work runs as real jobs, daemon metrics + **`.zqk/logs/scheduler/cvs/test-bundles/`** (`health.jsonl`, bundle logs) give run-level feedback; pair with **`zqk scheduler scan-tests --load-bundles data-cell-stream-organism,data-cell-stream-organism-pkg-storage`** for the curated data-cell slice ([DATA_CELL_MODEL.md](./DATA_CELL_MODEL.md) § *Program completion*). |
| **Spec authoring** | **`pkg/storage` `SpecMetricsCollector`** (field/spec change events). | Measures **edits** to specs, not cold **`spec_index.json` reads**—do not expect per-read latency there. Read latency for `data-cells` is reflected in **command metrics** until/unless you add finer stages. |

#### Stage timers (`data-cells`)

**Implemented** in `cmd/zqk/system/data_cells.go` + `data_cells_metrics.go`: **structured `Debug`** logs (`data-cells stage timing`) with human-readable durations and **`*_ns`** fields for scripts; when **`metricsrecording`** is enabled (normal `zqk` binary; off in default `go test` unless opted in), a line is **appended** to **`.zqk/metrics/data_cells_stages.jsonl`** (JSON object per run: `spec_load_ns`, `build_output_ns`, `stream_summary_ns`, `kind_filter`, `ts_rfc3339`). Truncate or rotate that file if it grows large.

| Stage | Span | What it answers |
|-------|------|-----------------|
| **`data_cells_spec_load`** | `datacellregistry.LoadDataCellDescriptors(root)` | Disk + JSON + descriptor build. |
| **`data_cells_build_output`** | After filter: JSON marshal/table string build, **excluding** stream-summary read time. | Row work + formatting. |
| **`data_cells_stream_summary_read`** | `loadTestBundleHealthSummary` (JSON envelope and/or table footer) | Single read per invocation; table path no longer loads the file twice. |

Whole-command duration remains in **`.zqk/metrics/command_metrics.json`**; JSONL stage samples complement **`zqk system metrics`** when you need a split.

## Background verification (scheduler bundles)

Saved bundle prefixes **`data-cell-stream-organism`** and **`data-cell-stream-organism-pkg-storage`** (under `.zqk/test-bundles/`, shards `data-cell-stream-organism-*.json` plus `data-cell-stream-organism-pkg-storage.json`) cover the isolated data-cell slice: `pkg/datacell` (`TestStreamOrganism_*`, `TestOperationalEnvelope*`, `TestEnvelopeTickKindAugmentSummaries_*`, membrane path tests), `pkg/objects` (spec index vs `high_volume_kinds` stream contract), `pkg/scheduler` (full `datacell_envelope_test.go`: policy dry-run, tick handler, metrics JSONL, registry), `pkg/storage` (stream stewardship — **pkg-storage** bundle), and `cmd/zqk/system` (`TestDataCells_StreamOrganism_*`, `TestDataCellsEnvelopePolicyDryRunJSON`, `TestBuildDataCellJSONRows_operationalEnvelope*`, …). The canonical test name lists are in the script; see [DATA_CELL_MODEL.md](./DATA_CELL_MODEL.md) → *Data cell test matrix*. Recreate or refresh with:

`./scripts/save-data-cell-stream-organism-bundle.sh`

Run in the background via the scheduler (requires daemon: `zqk scheduler start`):

`zqk scheduler scan-tests --load-bundles data-cell-stream-organism,data-cell-stream-organism-pkg-storage`

Same bundle set via the regression helper: `BUNDLES=data-cell-stream-organism,data-cell-stream-organism-pkg-storage ./scripts/run-regression-bundles.sh`.

The loader resolves the prefix to all numbered shards (parallel packages vs sequential `cmd/zqk/system` bundle). Logs: `.zqk/logs/scheduler/cvs/test-bundles/`. For ad-hoc package gates without a saved bundle: `zqk scheduler scan-tests --package ./pkg/datacell`, `./pkg/scheduler`, `./cmd/zqk/system` as needed.

## Purpose

Small, **project-local** artifacts the CLI edits without exposing whether persistence is single-file JSON, CAS, or stream-backed:

| Surface | Role | On-disk path (v1) |
|---------|------|-------------------|
| Feature flags | Boolean toggles for runtime behavior | `.zqk/config/feature_flags.json` |
| CLI hook profile | Built-in hooks, optional `tray_entry` | `.zqk/config/cli_hook_profile.json` |
| Tray manifest | Named shortcuts (`argv` lists) | `.zqk/tray.yaml` |
| Runtime manifest (optional) | Declares `protocol_version` for the layout | `.zqk/config/datacell_runtime.json` |
| **Agent chat channel (pilot)** | **Lite-file policy** (`schema_version`, `enabled`, optional `delivery_mode`, optional `note`, optional `feed_id` / `materialized_at` when materialized) + **append-only JSONL** for steward / IDE integration events | **`.zqk/config/agent_chat_channel.json`** + **`.zqk/logs/cursor-hooks/agent_chat_channel.jsonl`** (or `events_jsonl_path_override` from materialized `agent_feed`) — see `pkg/datacell`; **`zqk system materialize-agent-chat-channel --feed-id AGF- (PRUNED)…`** writes the lite file from CAS. `zqk system path-cache --show-paths` (PRUNED) / **`zqk system data-cells --json --json-envelope` (PRUNED)** print resolved paths. **CAS:** **`agent_feed`** under **`.zqk/process/agent_feeds/`** (`AGF-*`); **`zqk object template agent_feed`**. |

Together these are the **runtime organism**: one logical membrane around operator-edited config, with paths centralized in Go so **relocation** (aliases, path cache, or future manifest) does not require a string search across `pkg/`.

<a id="data-cell-narrative"></a>

### Narrative: `agent_feed`, delivery, orchestration, observability

This subsection exists because **onboarding and clarity** suffer when three different concerns are conflated: (1) **where policy lives**, (2) **what the system measured**, (3) **what should run next**. The data-cell program optimizes (1) and exposes signals for (2); **closed-loop “run the next convergence iteration for me”** is a **separate control plane** (scheduler, CVS ticks, pipelines, human workflow) described in [DATA_CELL_MODEL.md](./DATA_CELL_MODEL.md) (membrane/coordinator) and convergence architecture docs—not a gap in the lite file alone.

| Concern | What satisfies it (today) | What it is *not* |
|--------|---------------------------|------------------|
| **Delivery policy in the process plane** | **`agent_feed`** (`AGF-*` in CAS): structural binding + **`runtime_delta`** toggles (`enabled`, **`delivery_mode`**: off / log / clipboard / paste / …). Materialize to **`.zqk/config/agent_chat_channel.json`** with **`zqk system materialize-agent-chat-channel --feed-id (PRUNED) …`** so hooks do not fork policy from CAS. | Not a substitute for **when** to emit the next agent prompt or **why** to iterate; it configures **how** permitted delivery behaves once something else triggers it. |
| **Convergence state & bundle measurement** | **`convergence_session`** fields, **`zqk scheduler convergence measure`**, **`convergence_session_tick`**, **`health.jsonl`**, **`.zqk/logs/scheduler/cvs/cvs_measurement_events.jsonl`**. | Not the same as IDE chat delivery; ticks update CVS and logs—they do not post to Cursor by default. |
| **Orchestration coherence** | Roadmap: coordinator enqueue, explicit iteration policy; today: **compose** scheduler jobs + CVS + events + optional **`scripts/cvs/convergence-measure-paste-loop.sh`** for human-in-the-loop loops. | Expecting **`agent_feed`** alone to close the loop will disappoint; it was never the full orchestration story. |
| **Agent–system handoff (“blackboard”)** | **Glossary `GLS-1776410364614477000-e6d4c940`** — idempotent **stdout + durable state** (process objects: CVS, backlog, criteria, plans; toy: `math-volley-blackboard.sh`) per tick; **exit codes**; **no** chat-as-database. System **narrows** the corridor with gates/metrics; agent **chooses** inside the band. See [CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md](./CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md) § *Blackboard handoff pattern*. | Not the same as **hook paste** or **every-line-to-chat** loops; optional human status in chat, **machine truth** in objects + script stdout. |

**Observability** (meaningful, timely signals—onboarding, awareness, operations) spans **all** of the above and is intentionally **multi-surface**:

| Signal | Typical location | Helps answer |
|--------|------------------|--------------|
| Hook / IDE feed events | **`.zqk/logs/cursor-hooks/agent_chat_channel.jsonl`** (or override from materialized feed) | What did the hook record for delivery / probes? |
| CVS measurement events | **`.zqk/logs/scheduler/cvs/cvs_measurement_events.jsonl`** | When did measure apply vs no-new-watermark audit? |
| Bundle health timeline | **`.zqk/logs/scheduler/cvs/test-bundles/health.jsonl`** | What happened in test-bundle runs? |
| Scheduler | Job history, category logs, **`zqk scheduler activity` / `history`** | Did ticks/jobs run, succeed, defer? |
| Data-cells CLI | **`.zqk/metrics/command_metrics.json`**, **`data_cells_stages.jsonl`** (when metrics enabled) | How expensive is discovery as the spec plane grows? |
| Envelope tick | **`.zqk/metrics/data_cell_envelope_tick.jsonl`** (when enabled) | Operational envelope tick health |

**Mission alignment:** efficiency and reliability come from **clear boundaries** between policy materialization, measurement persistence, and orchestration; **observability** comes from knowing **which file or command** answers which question—without that, a flexible system feels opaque. This doc’s **runtime organism** slice owns the **left column** of the first table; see [DATA_CELL_MODEL.md](./DATA_CELL_MODEL.md) for the full cell platform and [CONVERGENCE_PREDICATES_AND_GATES.md](./CONVERGENCE_PREDICATES_AND_GATES.md) for bundle vs session gates.

## Protocol version

- **`pkg/datacell.ProtocolVersion`** is **`"1"`** for this layout (two JSON files under `.zqk/config/`, one YAML under `.zqk/`, optional manifest JSON).
- Optional **`datacell_runtime.json`** may set `"protocol_version"`; **`datacell.ReadRuntimeManifest`** / **`datacell.EffectiveProtocolVersion`** use that value when present, else the constant. Missing file is normal.

  Minimal stub (equivalent to omitting the file when it matches the constant):

  ```json
  { "protocol_version": "1" }
  ```

- Bump the constant when introducing a **breaking** on-disk change or a **manifest** file that redirects paths (future).

## API

Use **`datacell.FeatureFlagsPath`**, **`datacell.CLIHookProfilePath`**, **`datacell.TrayYAMLPath`**, **`datacell.RuntimeManifestPath`**, **`datacell.AgentChatChannelConfigPath`**, **`datacell.AgentChatChannelEventsJSONLPath`**, **`datacell.StewardEnqueueJSONLPath`**, **`datacell.StewardMetricsJSONLPath`** with `projectRoot` — do not duplicate `filepath.Join` for these files. **`datacell.AllRuntimePaths(projectRoot)`** returns them in a **`datacell.RuntimePaths`** struct (path-alias aware), for tooling and CLI output that lists the full layout (including **`zqk system path-cache --show-paths` (PRUNED)** and **`zqk system data-cells` (PRUNED)** footer fields).

## Path aliases (cache-aware relocation)

Resolution uses **`paths.ResolvePathFromCacheOrConstant`** with defaults that match the table above. When the path alias cache is built (pre-warm / `zqk system path-cache` (PRUNED) / brand settings), these keys override the relative path **per project**:

| Alias key (`paths` constant) | Default relative path |
|------------------------------|------------------------|
| **`PathAliasDatacellFeatureFlags`** (`datacell_feature_flags`) | `.zqk/config/feature_flags.json` |
| **`PathAliasDatacellCLIHookProfile`** (`datacell_cli_hook_profile`) | `.zqk/config/cli_hook_profile.json` |
| **`PathAliasDatacellTrayYAML`** (`datacell_tray_yaml`) | `.zqk/tray.yaml` |
| **`PathAliasDatacellRuntimeManifest`** (`datacell_runtime_manifest`) | `.zqk/config/datacell_runtime.json` |
| **`PathAliasDatacellAgentChatChannelConfig`** (`datacell_agent_chat_channel_config`) | `.zqk/config/agent_chat_channel.json` |
| **`PathAliasDatacellAgentChatChannelEvents`** (`datacell_agent_chat_channel_events`) | `.zqk/logs/cursor-hooks/agent_chat_channel.jsonl` |
| **`PathAliasDatacellStewardEnqueue`** (`datacell_steward_enqueue`) | `.zqk/logs/datacell/steward_enqueue.jsonl` |

Post-retention stream maintenance (`pkg/storage.PostRetentionStreamStewardship`) appends **`stream_steward_kind`** records (detail `c=<cycle>|k=<kind>|p=<phase>`) to this queue—one line per stream-backed kind / runtime-delta phase—drained by **`data_cell_envelope_tick`**; see [STREAM_KIND_STEWARDSHIP.md](./STREAM_KIND_STEWARDSHIP.md).

See **[PATH_ALIAS_RESOLUTION.md](./PATH_ALIAS_RESOLUTION.md)**. Defaults are registered in **`paths.DefaultPathAliases`**. Brand **`zqk-settings.yaml`** `paths.aliases` **merge** over those defaults (partial overrides do not drop unrelated aliases).

## Non-goals (this slice)

- Full **assembly pipeline** (storage, metrics, events, transceiver ports) — see backlog BLI description.
- Moving **tray** under `config/` without migration (compatibility would require dual-read or one-time migrate).
- Optional **`datacell_runtime.json`** stub is sufficient for protocol tagging; a richer top-level **`datacell_manifest.json`** is not required until multiple storage backends or admin-only layouts exist.

## Next steps (product backlog)

- Optional **manifest** file listing components and storage profile for admin CLI “editor” UX.
- **Adapters** that satisfy the same logical API over CAS or stream when those backends are chosen by system profile.
