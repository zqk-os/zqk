# `zqk new` — standard object origination pipeline

**Status:** Implemented (CLI). Process traceability: requirement + doc_entry + criteria in process data (via `zqk object create`).

## Problem

Creating spec-backed objects and scenario bundles today requires knowing field shapes, valid enums, and the right `zqk object create` / `zqk-scenario bundle apply` flows. That friction slows humans and agents.

## Pattern

1. **Materialize** — `zqk new` writes an **editable draft** (YAML) derived from the same sources as runtime validation (object specs via `cliexamples`, scenario bundle schema for bundles).
2. **Edit** — human or agent fills placeholders in an editor or answer file.
3. **Persist** — `zqk object create <kind>` (or `zqk internal create <kind>` for internal kinds). After materializing to the default path, **`--file` is optional**: the CLI records **`.zqk/drafts/last-draft.yaml`** pointing at the last written draft; `object create` / `internal create` load that file when `--file`, `--data`, and stdin are unset. Override with **`--file`** when you want an explicit path. Scenario bundles: `zqk-scenario bundle apply -f <draft> -R .`

Drafts are **not** stored under `docs/architecture/` instance paths until the user runs create/apply. Output: **`--output -`** (or **`-o -`**) sends YAML to the command output writer (stdout in normal CLI); when **`--output`** is omitted, the default path is **`.zqk/drafts/<name>-<timestamp>.yaml`** under the project root (see command help). Successful **`object create` / `internal create`** from that path clears the pointer so the next `new` sets a fresh default.

## Commands

| Command | Output |
|--------|--------|
| `zqk new object <kind>` | YAML template for one object kind (required + common fields). |
| `zqk new internal <kind>` | Same pipeline; internal kinds use specs under `docs/architecture/_internal/object_specs/`. |
| `zqk new object-spec <ontology>` | Draft **kind definition** YAML for `object_specs/` (inherits top-level defaults from `--extends`, with comments). Not an object instance. |
| `zqk new bundle [--name <name>]` | Minimal `scenario_bundle` with `api_version` / `kind` / `metadata`; extend `objects:` as needed. |

**Implementation:** Root long/short text and subcommand structure (`--output` / `-o`, bundle `--name` / `--description`, object-spec `--extends`) are defined in `.zqk/cli/specs/new/` (`root_command.yaml` for the `new` group, plus per-subcommand specs). Run `zqk system generate-command-builders --overwrite` after changing those specs; generated Go lives in `pkg/cli/bldr_cli_cmd_v1/new_*_command_builder.go`. RunE wiring lives in `cmd/zqk/new/new.go`.

## Non-goals (v1)

- Interactive TUI or embedded editor (future).
- Validating drafts before create (use `zqk object create --dry-run` where supported).
- Writing process instance YAML directly — **create/update** remains the only persistence path for process objects.

## Tests

- `go test ./cmd/zqk/new/... -timeout 120s` — covers `new object` (several kinds, unknown kind error, default `.zqk/drafts/` output with `ZQK_PROJECT_ROOT`, `last-draft.yaml` pointer), `new internal object_spec`, unknown internal kind error, `new object-spec` (stdout, inheritance comments + `storage_profile`), `new bundle` (stdout via `-o -`), bundle scaffold contract (`api_version` / `metadata` / `objects:`), and default bundle path + `last-draft.yaml` with `scope: bundle`.
- `go test ./internal/cli/... -timeout 120s` — last-draft pointer read/write/clear and kind mismatch (`last_draft_pointer_test.go`).

## References

- `pkg/cliexamples` — example object generation from specs.
- `pkg/scenario` — bundle decode/apply.
- `docs/architecture/PRE_CHANGE_CHECKLIST.md` §1 — process data via CLI only.
