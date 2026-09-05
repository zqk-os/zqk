# Command Spec Coverage Report

**Last Verified:** 2026-08-31


**Validation status:** Run `zqk system validate-command-specs --format json` for
current results. Pair it with **`./scripts/check-cli-spec-consistency.sh`**
(generated builders + internal DNA).

**Canonical source:** `.zqk/cli/specs/` is the sole authoring source. Generated
builders are projections. `command_spec` (`CSPEC-*`) process objects are not a
second authoring tree; the former `docs/process/command_specs/` directory has been
removed. Create DNA with `zqk new command-spec`. See `REDACTED`.

**Baseline measured 2026-08-14:**
- Loaded Cobra paths: 1,261 (includes dynamic kind commands excluded from the static gate)
- File DNA specs: 95
- Historical static commands without file DNA: 195
- Historical specs not wired at the matching command path: 8
- Exact parity: false
- New drift beyond the committed baseline: zero

The baseline is debt inventory, not a coverage claim. The native gate fails for
new drift while allowing historical entries to be removed. Do not describe this
state as 100% coverage.

## Historical command-group inventory

The per-group counts below describe the former shell gate and are retained only
as migration context. They are not current measurements. The native validator's
structured result is authoritative.

### Root-level commands (1/1)
Top-level commands on the root command (not under a group) also have YAML specs and generated builders where applicable. **`zqk version`** — `.zqk/cli/specs/root/version_command.yaml` — shares `RunVersion` with **`utility version`** (`utility/version_command.yaml`). Other root commands (e.g. **`use`**, completion) have their own specs under **`project/`** or are hand-wired; `validate_command_specs.sh` enumerates **group** subcommands only.

### System Commands (74/74)
All system commands have specs (including cli-hooks, generate-field-keys, spec-origination, prepare-onboarding, update-mutation-metrics, maintenance-request-cycle, migrate-audit-stream, generate-pipeline-outcome-keys, validate-agent-rules).

### Object Commands (17/17)
Fixed subcommands (create, get, list, bulk, …) have specs. Dynamic `<kind>` subcommands (account, backlog_item, …) are generated from the spec index and are not validated per kind.

### Utility Commands (6/6)
All utility commands have specs.

### Automation Commands (2/2)
All automation commands have specs.

### Callback Commands (1/1)
All callback commands have specs.

### Docman Commands (1/1)
All docman commands have specs.

### Internal Commands (9/9)
Fixed subcommands (list, get, create, …, `fields`) have specs; `fields` carries root vs per-kind behavior at runtime. Dynamic kind subcommands (lifecycle, object_spec, …) are not validated per kind.

### Keystore Commands (3/3)
All keystore commands have specs.

### Feed Commands
Parent **`feed_command.yaml`**: steer, emit-status, ack, pending, doctor, bridge-ingest, serve, watch, **wake**, **proof-of-life**.

### Ambient Commands
Parent **`ambient_command.yaml`**: status, ingest, automerge.

### MCP Commands
DNA under **`.zqk/cli/specs/`**: parent **`mcp_command.yaml`**, leaves **`mcp/ensure`**, **`mcp/supervise`**, and **`mcp/svc/{serve,daemon,proxy,install,list_tools}`** (svc path keeps builder stems unique vs `feed serve`).

### Organizational Commands (4/4)
All organizational commands have specs.

### Reports Commands (4/4)
All reports commands have specs.

### Matrix Commands (5/5)
`matrix report`, `matrix list`, `matrix validate`, `matrix get`, and `matrix update` have specs (parent `matrix` group is hand-wired; subcommands are spec-driven).

### Scheduler Commands (22/22)
All scheduler commands have specs (including **print-cursor-paste-applescript**, events parent, record-cvs-orchestrate-run, convergence — measure, overseer, promotion-readiness, record-overseer-run — and **skip-window** with set/clear/status under `.zqk/cli/specs/scheduler/skip_window/`).

### Semantic Commands (2/2)
All semantic commands have specs.

### Spec Commands (1/1)
spec/list has a spec.

### Bundle Commands (zqk-scenario, 2/2)
The **zqk-scenario** binary (cmdv2) builds `bundle` and `bundle apply` from **generated** builders (`pkg/cli/bldr_cli_cmd_v1/bundle_*_command_builder.go`), produced by `zqk system generate-command-builders` from `.zqk/cli/specs/bundle/`. RunE for `bundle apply` is wired in `cmdv2/zqk-scenario/bundle_commands.go`. Validation script checks only the main **zqk** binary.

## Removed Deprecated Commands

The following deprecated commands have been **removed from the codebase** (specs were deleted to match):

1. **migrate-cas** — Removed (use `system migrate` with migration spec instead)
2. **migrate-audit-buckets** — Removed (use `system migrate` with migration spec instead)
3. **migrate-lifecycles** — Removed (use `system migrate lifecycle-files-to-objects.yaml` instead)

**Spec files deleted**: `system/migrate_cas_command.yaml`, `system/migrate_audit_buckets_command.yaml`, `system/migrate_lifecycles_command.yaml`

## Native validation gate

Run from project root:

```bash
zqk system validate-command-specs --format json
```

The command exits non-zero when it detects drift beyond
`.zqk/cli/command_spec_coverage_baseline.json`. Deliberately replacing the debt
inventory requires `--write-baseline`; routine command work must never use that
flag to hide a regression.

## Keeping help and specs in sync

- **Create new DNA:** `zqk new command-spec <command-path> --short "…" --description "…"` writes directly beneath the `command_specs` path registered in `docs/process/_internal/configs/paths_config.yaml` (normally `.zqk/cli/specs`). Generic `zqk new object command_spec` authoring is rejected so it cannot create a competing CAS source.
- **Source of truth:** Command help (short, long, examples, flags) comes from the YAML specs under **`.zqk/cli/specs/`** (e.g. `project/use_command.yaml` for `zqk use`).
- **Main zqk binary:** The primary CLI (`cmd/zqk`) builds commands from **generated** builders in `pkg/cli/bldr_cli_cmd_v1/`. Those builders are generated from the YAML specs. To update the help menu after editing a spec, run:
  ```bash
  zqk-admin system generate-command-builders --specs-dir .zqk/cli/specs --overwrite
  ```
  Then rebuild the binary so the new help is in the executable.
- **cmdv2 binaries:** The separate binaries (zqk-v2, zqk-scenario, zqkdev, zqk-svc in `cmdv2/`) do **not** include every command. For example, **`use`** exists only on the main **zqk** binary. If you only see different or missing help for a command, confirm you are running the same binary that owns that command (main `zqk` vs a cmdv2 binary).
- **Bootstrap:** Specs under `.zqk/cli/specs` are packed into the bootstrap archive by `make bootstrap-archive` so `zqk system init` can install them. After changing specs, run `make bootstrap-archive` before the next build if you want the embedded archive to include the updates.

## Spec Quality

Specs are the **first-class registration record** for each CLI surface:
`zqk system validate-command-specs` fails closed when the live binary introduces
a subcommand without matching YAML under `.zqk/cli/specs/` or leaves a new spec
unwired. That keeps generated builders, bootstrap archives, and documentation
aligned with what ships. Run it whenever the CLI changes; a **snug membrane**
(see **CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md**, *Membrane tightness*) means zero
new drift while the explicit historical baseline is worked down to zero.

Specs should include:
- `$schema` references for JSON Schema validation
- Proper flags, args, and help examples
- Common flags and query flags marked appropriately
- Dry-run support indicated where applicable

### Internal commands: registered surface vs YAML (DNA)

Fixed `internal` subcommands under `zqk internal …` must match their YAML under **`.zqk/cli/specs/internal/`** (explicit flags plus `list_harness_flags`, `count_harness_flags`, or `fields_harness_flags` / `fields_include_list_kinds` where the runtime uses `AddListFlags`, `AddCountFlags`, or `AddFieldsFlags`). **`TestInternalCommandSurfaceMatchesYAMLSpecs`** in **`pkg/zqkcli/command_spec_dna_test.go`** compares sorted local Cobra flags to **`cli.ExpectedLocalFlagNamesFromCommandSpec`**.

After editing specs, run **`zqk system generate-command-builders --overwrite`** and commit **`pkg/cli/bldr_cli_cmd_v1/`**. From repo root, **`./scripts/check-cli-spec-consistency.sh`** fails if generated builders are dirty or internal commands drift from the specs.
