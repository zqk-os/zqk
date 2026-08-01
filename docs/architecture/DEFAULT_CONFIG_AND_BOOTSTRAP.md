# Default Configs and Bootstrap

**Status**: Active  
**Purpose**: Canonical list of config locations, what has intelligent defaults, what init/bootstrap provides, and what is optional.

## Config locations (under project root)

| Path | Purpose | In bootstrap archive? | Created by init? | Notes |
|------|---------|------------------------|------------------|--------|
| **docs/architecture/_internal/** | Object specs, lifecycles, configs (id_prefixes, kind_mappings, paths, retention, scanner, etc.) | ✅ Yes (entire tree) | ✅ Extracted on init | Template/schema data; required for validation and storage. May also contain the generated `spec_index.json` (field/trait index), which is treated as a first-class cache and bundled when present. |
| **.zqk/cli/specs/** | Command specs and schemas (YAML + JSON) | ✅ Yes (entire tree) | ✅ Extracted on init | Required for CLI command dispatch and schema validation. |
| **.zqk/config.yaml** | Project config (legacy path) | ❌ No | ✅ Yes (same content) | Init writes here for backward compat; config-get and some code try both paths. |
| **.zqk/config/config.yaml** | Project config (canonical) | ❌ No | ✅ Yes | Init creates this and .zqk/config.yaml. Context loader reads this; audit aggregation, validation timeouts, brand use it. |
| **.zqk/config/command_timeouts.yaml** | Per-command timeouts | ❌ No | ❌ No | Optional override; pattern-based timeouts. |
| **.zqk/mcp/config.yaml** | MCP server config | ❌ No | ❌ No | LoadMCPConfig(); defaults if missing. |
| **.zqk/mcp/config-*.yaml** | Agent-specific MCP configs | ❌ No | ❌ No | e.g. config-coder.yaml, config-pm.yaml. |
| **.zqk/mcp/specs/*.yaml** | MCP specs (e.g. onboarding_prompts) | ❌ No | ❌ No | Optional; prompts and tool specs. |
| **.zqk/metrics/profiles/** | Metrics sampler profiles | ❌ No | ❌ No | findMetricsProfilesDir(); optional. |
| **config/** (repo root) | Not a standard path in code | — | — | pkg/paths has no `config/` at repo root; likely you mean .zqk/config/. |
| **scripts/templates/*.yaml** | Scheduler job templates (pre-commit, etc.) | ❌ No | ❌ No | Used to create scheduler_job objects; not required for init. |
| **scripts/ontology-templates/base/*.yaml** | Base ontology templates (BLI-711) | ❌ No | ❌ No | Organizational and partnership base ontologies; copy to .zqk/ontologies/base/ or use with ontology/domain tooling. |

## Init / bootstrap behavior

- **Bootstrap archive** (embedded in binary): Contains `docs/architecture/_internal` and `.zqk/cli/specs`. Built by `scripts/build-bootstrap-archive.sh` (run on every build via `make bootstrap-archive`). Extracted to `projectRoot/docs/architecture/_internal` and `projectRoot/.zqk/cli/specs` on init. If a generated `docs/architecture/_internal/spec_index.json` is present at build time, it is included so field metadata and CLI completions/validation can read from the index without re-walking specs.
- **Init creates**:
  - `.zqk/` with subdirs `cache/`, `state/`
  - `docs/architecture/` and subdirs (including `_internal` before extract)
  - **.zqk/config.yaml** (minimal: project name, template, storage, logging, cache)
  - Does **not** create `.zqk/config/config.yaml`, `.zqk/mcp/*`, `.zqk/metrics/profiles`, or any script templates.

## Config paths (resolved)

- **Init** now writes project config to **both** `.zqk/config/config.yaml` (canonical) and `.zqk/config.yaml` (legacy) via **writeProjectConfigFiles**.
- **Context manager** reads `.zqk/config/config.yaml`, so a clean init is fully valid for context.
- **Other code** (config-get, GetErrorLogOutput, GetLogLevel, root init) tries both paths; both are populated on init.

## Scripts and init

- **No scripts are required for init or bootstrap to work.** Init is implemented entirely in Go (createProjectDataDir, createProcessDir, ExtractBootstrapFiles, createConfigFile). The bootstrap archive is built by a script during the **build** (make), not during init.
- **scripts/** are for:
  - Build: `build-bootstrap-archive.sh`, etc.
  - Test/CI: `test-runner.sh`, `run-long.sh`, `run-bg.sh`, etc.
  - Pre-commit / system check: `run-system-check-with-log.sh`, etc.
  - Scheduler job templates under `scripts/templates/` are optional; they are used to create scheduler_job objects (e.g. pre-commit lint) but are not needed for a minimal “init out of the box” run.

## Adding more defaults to bootstrap

To make “clean install” complete with intelligent defaults, consider:

1. **.zqk/config/config.yaml** – Init now creates this (and `.zqk/config.yaml`) with sensible defaults. For richer defaults (audit aggregation, validation timeouts), extend **buildProjectConfigContent** in `cmd/zqk/system/init.go` or bundle a template and extract when missing.
2. **.zqk/mcp/config.yaml** – Bundle a minimal default and extract to `.zqk/mcp/` if missing (or document that MCP uses in-memory defaults when absent).
3. **.zqk/mcp/specs/** and **.zqk/metrics/profiles/** – Same idea: optional defaults in the archive and extract-on-init if desired.
4. **scripts/templates/** – Remain optional; they are not required for init. They can stay in the repo for users who want to create scheduler jobs from templates.

## Test coverage

Full bootstrap is asserted in init tests (greenfield and legacy/existing-data):

- **TestInit_Greenfield** and **TestInit_Legacy** call **requireBootstrapPresent**, which verifies:
  - `docs/architecture/_internal/object_specs` has at least one `.yaml`
  - `docs/architecture/_internal` has id_prefixes or kind_mappings config (root or configs/)
  - `.zqk/config/config.yaml` and `.zqk/config.yaml` exist
  - `.zqk/cli/specs` exists and has content (skips with a note if binary has no embedded archive)
- **TestBootstrap_CRUD_Greenfield** (opt-in: `ZQK_ENABLE_BOOTSTRAP_CRUD_TESTS=1`): after greenfield init, discovers all object kinds via `object fields --list-kinds --format json` and runs CLI create→get→list→update→delete for **one object of every kind** using templates (`object template <kind>`). Skips kinds that are system-only or immutable (e.g. change_journal_entry). Also creates one internal kind_synonym and exercises profile load (`object list account --context ai-agent`). Ensures no code path expects a spec or config missing from the bootstrap. Use `-timeout 300s` or `go test ./...` when enabling.
- **TestBootstrap_SchedulerChecklist** (same opt-in): after init, starts an HTTP server that listens for the scheduler’s **completion callback** (webhook). Creates a one_time cache_prewarm scheduler_job with `callback_on_completion` set to that URL, starts the scheduler (with **project root = test temp dir only**), and blocks until the callback is received—no polling or arbitrary sleep. Then runs the same CRUD cycle for representative kinds, verifies count, and runs `system compact-wal --dry-run`. Ensures scheduler and maintenance paths work from ground-zero in an isolated scenario.

So greenfield and “project that already has some data” (legacy) both run full bootstrap and are validated.

## References

- Bootstrap archive: `scripts/build-bootstrap-archive.sh`, `internal/bootstrap/`, `cmd/zqk/system/bootstrap_extractor.go`
- Init: `cmd/zqk/system/init_impl.go`, `cmd/zqk/system/init.go`
- Init tests: `cmd/zqk/system/init_test.go` (requireBootstrapPresent, TestInit_Greenfield, TestInit_Legacy); bootstrap CRUD: `cmd/zqk/system/bootstrap_crud_test.go` (TestBootstrap_CRUD_Greenfield, opt-in with ZQK_ENABLE_BOOTSTRAP_CRUD_TESTS=1)
- Context loading: `internal/cli/context/context.go` (loadProjectConfig)
- Path constants: `pkg/paths/constants.go`
- BOOTSTRAP_REQUIREMENTS.md (docs/architecture/architecture) for historical required-files rationale
