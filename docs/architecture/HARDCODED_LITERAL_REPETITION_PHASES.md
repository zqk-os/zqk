# Hardcoded literal repetition — drift phases

**Last Verified:** 2026-08-31


**Rule:** Any **repeated** literal (same spelling in more than one place) is **drift-prone**: one site can change and the other lags. The goal is to drive repetition toward **one canonical definition** (constant, named `fs.FileMode`, helper, or generated barrel), then **re-scan** and continue.

This complements **`CONSTANTS_AND_DRY_INVENTORY_PLAN.md`** (existing A–E phases) with an **execution loop** that does not require picking ad hoc “slices.”

---

## Exempt (not drift targets)

- **JSON / struct tags** and other **schema-bound** spellings (changing them is a protocol change, not a DRY tweak).
- **Generated** `DO NOT EDIT` trees (change spec + codegen instead).
- **Third-party verbatim** strings where we intentionally mirror an external contract and wrapping would obscure that.

Everything else: if it **repeats**, treat it as a **candidate** for centralization.

---

## Phase R0 — Detect repetition

- **FS modes (legacy octal):** `os.MkdirAll` / `os.WriteFile` / `os.OpenFile` / `os.Chmod` with `0ddd` → use **`pkg/paths`** (`DirPerm700`, `DirPerm750`, `DirPerm755`, `FilePerm600`, `FilePerm644`). Mechanical migrate: **`python3 scripts/migrate_legacy_fs_octals_to_paths.py`** (repo root), then `goimports` on touched files (the script prints the `go run … goimports` command).
- **Broad baseline (noisy):** `./scripts/scan-hardcoded-go-literals.sh` — use for **trend**, not as a single “fix every line” gate.
- **Ontology / field / schema risk:** `zqk system analyze-drift-hotspots` (PRUNED) — prioritize **critical/high** before low-noise sweeps.
- **Quoted string volume triage (optional):** `python3 scripts/drift_quoted_literal_triage.py` (see `scripts/README.md`).

---

## Phase R1 — Eliminate (mechanical first)

1. Run the **highest-signal automated migrator** available for the bucket (e.g. FS octals script).
2. **`go build ./...`**
3. **Targeted tests** for touched packages (foreground only with **`-timeout`** per project rules; heavy packages → `zqk scheduler scan-tests --package …`).

---

## Phase R2 — Verify and re-baseline

- Re-run the same detector (script or scan). **Expect line count to drop** for that bucket; commit when green.
- If a change **unexpectedly** alters behavior (permissions, wire format, metrics labels), **stop** and fix forward — that is the only “human gate” implied by “larger deviation.”

---

## Phase R3 — Next bucket (ordered)

Repeat R0–R2 for the next category until mechanical wins slow down:

1. **FS modes** (`pkg/paths`) — first; fully scriptable.
2. **Brand env keys** — `pkg/zqkenv` + `scripts/migrate_zqk_env_literals` / `check-zqk-env-literals-repo.sh`.
3. **Field keys in object maps** — `objects.FieldKey*` + `scripts/fix_field_key_literals`.
4. **Logger profiles** — `pkg/context` profile constants (largely done per inventory plan; re-scan for regressions).
5. **Repeated error / metric / label strings** — package-level `const` blocks or existing metrics constants.
6. **Tests** — same rules as production when the same literal repeats across tests (helpers / shared test constants).

---

## References

- **`CONSTANTS_AND_DRY_INVENTORY_PLAN.md`** — canonical barrels and historical phases A–E.
- **`docs/architecture/GIT_DRIFT_SEARCH_PATTERNS.md`** — pattern 8 (`scan-hardcoded-go-literals.sh`).
- **`pkg/paths/constants.go`** — `DirPerm*` / `FilePerm*`.
