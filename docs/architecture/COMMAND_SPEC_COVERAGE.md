# Command Spec Coverage Report

**Validation Status**: Run `./scripts/validate_command_specs.sh` for current results. Pair with **`./scripts/check-cli-spec-consistency.sh`** (generated builders + internal DNA).

- **Total commands**: 147 (per `./scripts/validate_command_specs.sh`; object and internal use fixed subcommand whitelists; dynamic kind subcommands are not required to have per-kind specs)
- **Total specs**: 147
- **Coverage**: 100%

## Command Groups Coverage

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

### MCP Commands (2/2)
All MCP commands have specs.

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

## Validation Script

Run from project root:

```bash
./scripts/validate_command_specs.sh
```

The script exits with an error if any command lacks a spec. Update this document after adding or removing commands/specs so the numbers and "Missing specs" lists stay accurate.

## Keeping help and specs in sync

- **Source of truth:** Command help (short, long, examples, flags) comes from the YAML specs under **`.zqk/cli/specs/`** (e.g. `project/use_command.yaml` for `zqk use`).
- **Main zqk binary:** The primary CLI (`cmd/zqk`) builds commands from **generated** builders in `pkg/cli/bldr_cli_cmd_v1/`. Those builders are generated from the YAML specs. To update the help menu after editing a spec, run:
  ```bash
  zqk system generate-command-builders --specs-dir .zqk/cli/specs --overwrite
  ```
  Then rebuild the binary so the new help is in the executable.
- **cmdv2 binaries:** The separate binaries (zqk-v2, zqk-scenario, zqkdev, zqk-svc in `cmdv2/`) do **not** include every command. For example, **`use`** exists only on the main **zqk** binary. If you only see different or missing help for a command, confirm you are running the same binary that owns that command (main `zqk` vs a cmdv2 binary).
- **Bootstrap:** Specs under `.zqk/cli/specs` are packed into the bootstrap archive by `make bootstrap-archive` so `zqk system init` can install them. After changing specs, run `make bootstrap-archive` before the next build if you want the embedded archive to include the updates.

## Spec Quality

Specs are the **first-class registration record** for each CLI surface: `./scripts/validate_command_specs.sh` fails closed when the live binary exposes a subcommand without a matching YAML file under `.zqk/cli/specs/`. That keeps generated builders, bootstrap archives, and documentation aligned with what ships—analogous to other “measure then persist” gates, but enforced at spec-ingest time. Run it in CI whenever the CLI changes; a **snug membrane** (see **CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md**, *Membrane tightness*) means zero tolerated drift between `zqk <group> --help` and the repo’s specs.

Specs should include:
- `$schema` references for JSON Schema validation
- Proper flags, args, and help examples
- Common flags and query flags marked appropriately
- Dry-run support indicated where applicable

### Internal commands: registered surface vs YAML (DNA)

Fixed `internal` subcommands under `zqk internal …` must match their YAML under **`.zqk/cli/specs/internal/`** (explicit flags plus `list_harness_flags`, `count_harness_flags`, or `fields_harness_flags` / `fields_include_list_kinds` where the runtime uses `AddListFlags`, `AddCountFlags`, or `AddFieldsFlags`). **`TestInternalCommandSurfaceMatchesYAMLSpecs`** in **`pkg/zqkcli/command_spec_dna_test.go`** compares sorted local Cobra flags to **`cli.ExpectedLocalFlagNamesFromCommandSpec`**.

After editing specs, run **`zqk system generate-command-builders --overwrite`** and commit **`pkg/cli/bldr_cli_cmd_v1/`**. From repo root, **`./scripts/check-cli-spec-consistency.sh`** fails if generated builders are dirty or internal commands drift from the specs.
