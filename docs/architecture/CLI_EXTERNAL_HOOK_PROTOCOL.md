# CLI external hook protocol (tray + cli-hooks)

**Last Verified:** 2026-08-31


**Protocol revision:** `zqk.cli_hooks.v1` (see `clihooks.ProtocolVersion` in code; `list` output includes `protocol_version`).

**Purpose:** Define a **stable boundary** between:

- **Inside:** Go packages (`pkg/tray`, `pkg/clihooks`, scheduler, storage, …).
- **Outside:** Git hooks, CI scripts, custom wrappers, third-party tooling.

External integrations must **not** depend on internal package layouts, undocumented JSON files, or private binaries. They should use only the **published `zqk` CLI** (or `zqk-admin`, same tree) — the same “membrane” idea as the user vs developer command split in `AGENT_GUIDELINES.md`.

---

## 1. Allowed surface (normative)

| Capability | Command(s) | Notes |
|------------|------------|--------|
| Hook profile | `zqk system cli-hooks list (PRUNED)|get|enable|disable|set-tray|is-enabled|print-tray-entry` | Persisted: `.zqk/config/cli_hook_profile.json`. Schema: `.zqk/cli/specs/schemas/cli_hook_profile.schema.json`. |
| Tray manifest | `zqk tray list|show|explain|run` | Defaults + `.zqk/tray.yaml`. Schema: `.zqk/cli/specs/schemas/tray_config.schema.json`. |
| Scheduler / tests | `zqk scheduler …` | As documented elsewhere; hooks may enqueue work, not replace scheduler invariants. |

**Environment:** `ZQK_CMD` — optional override for the binary name or path (default `zqk`). Used by repo scripts so CI can point at a specific build **without** editing hook logic.

**Wall-clock timeouts:** `ZQK_HOOKS_TIMEOUT_SEC` — optional seconds (default **5**) for each `cli-hooks` probe in `scripts/hooks/zqk-post-commit-regression.sh` (`is-enabled`, `print-tray-entry`). Prevents git hooks from hanging indefinitely if the CLI blocks in kernel I/O; when the timeout fires, the script treats the hook as inactive (same as `is-enabled` failure) and exits without scheduling work.

**Exit codes (`is-enabled`):** `0` if the hook is enabled, `1` if disabled or unknown id (unknown id returns an error; treat as fail-closed in scripts only if you check stderr).

**Machine output (`print-tray-entry`):** Writes **only** the tray entry name (one line, UTF-8) to stdout — for `entry=$(zqk system cli-hooks print-tray-entry HOOK_ID (PRUNED))`. Empty `tray_entry` yields an empty line. Do not parse JSON from internal files unless you also use the documented schema and accept breakage on schema bumps.

---

## 2. Hook IDs (stable names)

Defined in `pkg/clihooks` (e.g. `HookPostCommitScanTests = "post_commit_scan_tests"`). New IDs are **additive**; renaming or removing IDs requires a **protocol bump** (e.g. `zqk.cli_hooks.v2`) and a changelog.

---

## 3. Delegation pattern (tray vs default script)

For each hook, the profile may set:

- **`tray_entry` empty:** External runner uses the **default** behavior documented for that hook (e.g. `scripts/run-tests-for-changed-packages.sh` for post-commit regression).
- **`tray_entry` non-empty:** External runner may delegate to `zqk tray run <name>` instead of the default script. The tray entry must exist (`zqk tray list`). Semantics are **user-defined** (tray argv is validated; the hook does not interpret Go packages).

This keeps **one** indirection: hooks → **CLI** → tray or script — not hooks → pkg.

---

## 4. Anti-patterns (do not)

- Importing `github.com/lanceman/zqk/pkg/...` from non-Go external code (impossible) or from ad-hoc scripts that fork `go run` — use `zqk`.
- Reading `.zqk/config/*.json` without the published schema and version discipline.
- Assuming paths under `cmd/`, `internal/`, or `pkg/` from shell.
- Bypassing `zqk` for process objects under `.zqk/process/` — use `zqk object` per `process-data-cli-only.mdc`.

---

## 5. Repo reference implementation

- **`scripts/hooks/zqk-post-commit-regression.sh`** — Implements this protocol for `post_commit_scan_tests` using only `zqk system cli-hooks` (PRUNED) and `zqk tray` + the default script.
- **`tools/git-hooks/post-commit`** — Invokes that script in the background; does not embed zqk logic.

Custom sites can copy the same pattern: gate on `is-enabled`, branch on `print-tray-entry`, call `tray run` or a local script.

---

## 6. Future formal specification

When hook IDs, tray shapes, and JSON versions stabilize, this document can evolve into a versioned spec (OpenAPI-style description of commands + JSON schemas) **without** changing the principle: **external contract = CLI + schemas + protocol version**, not Go symbols.

---

## See also

- `docs/architecture/PRE_COMMIT_BACKGROUND_RESULTS.md` — post-commit test bundles
- `docs/enforcement/AGENT_GUIDELINES.md` — command surfaces
- `pkg/tray`, `pkg/clihooks` — implementation (not for external import from shell)
