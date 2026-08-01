# Native matrix CLI strategy (vetting & traceability)

## Problem

One-off Python scripts (`generate-codebase-vetting-matrix.py`, `verify-codebase-vetting-matrix.py`) are useful for automation but do not give users a **cohesive, discoverable** interface. The **verification matrix** pattern (CSV + profile YAML + optional `convergence_session` linkage) should be **first-class**: configurable, bulk-editable where safe, and aligned with CVS traceability—not a growing pile of ad-hoc tools.

## Principles

1. **Registry over paths** — Named matrices (`codebase_vetting`, `test_bundle`, …) resolve to `csv` + `profile` via **`docs/quality/matrix_registry.yaml`** (or override `--registry`). Users think in **aliases**, not raw paths.
2. **Profile is law** — Column names, gate columns, and `done_values` come from each matrix’s **profile YAML** (`vetting_matrix_profile.yaml`, `test_bundle_matrix_profile.yaml`, …). The CLI loads the profile to interpret and update rows; no hard-coded column sets in command handlers.
3. **CVS as session anchor** — A **`cvs_id`** column (name configurable per matrix via `session_ref_column`) ties rows to **`convergence_session`**. Optional: default CVS on the session object or CLI flag `--cvs-id` for bulk updates and activity logging.
4. **Fluid syntax (target)** — Evolve toward commands like:
   - `zqk matrix report [--name <alias>]`
   - `zqk matrix get --name <alias> --filter file_path=pkg/foo --field file_path --field fully_vetted`
   - `zqk matrix update --name <alias> --set fully_vetted=yes --match …` (bulk)
   - `zqk matrix update --cvs-id CVS-…` (stamp session + optional activity_log append)
5. **Python as implementation detail** — Regeneration of huge inventories (`git ls-files` merge) may stay in Python **until** ported; **report**, **get**, and **update** should be **native Go** for UX, logging (POL-CODE-007), and CI.

## Current implementation (this iteration)

| Piece | Status |
|-------|--------|
| `docs/quality/matrix_registry.yaml` | **Added** — aliases → csv/profile/session column |
| `zqk matrix report` | **Added** — summary counts + gate breakdown from profile + CSV |
| `zqk matrix list` | **Added** — registry aliases + paths + optional `value_map` key count |
| `zqk matrix validate` | **Added** — CSV + profile readable, row count, `session_ref_column` present when set; exit 1 if any matrix fails |
| `zqk matrix update` | **Added** — one row by `--file-path` or `--bundle-label`, or **bulk** by `--filter` (AND) + `--set`; optional `--limit`, **`--backup`** / **`--backup-to`**; gate values vs profile `done_values`; atomic CSV replace; `--dry-run`; optional **`--append-cvs-activity`** + **`--cvs-id`** (and/or row **`session_ref_column`**) for **`convergence_session.activity_log`** append |
| `zqk matrix get` | **Added** — `--filter`, `--glob` (file_path), `--go-only`, `--cvs-id`, `--limit`, `--field` (repeatable); json/yaml/table/csv |

**CI gate:** run **`zqk matrix validate`** in automation (non-zero exit if any matrix fails checks).

**Drift baseline:** registry alias **`drift_search_baseline`** (`DRIFT_SEARCH_BASELINE_MATRIX.csv`) ties pattern-8 grep volume to optional numeric **`target_bytes`** and gate **`at_target`**; sync from **`.zqk/logs/drift/search-baseline/latest`** via **`scripts/drift_search_baseline_matrix_sync.py`** (see **`docs/quality/DRIFT_SEARCH_BASELINE_RUBRIC.md`**).

### `verification_matrix` object kind (process data)

The **`verification_matrix`** kind (`VMX-*`) is the persisted counterpart to registry aliases: it can hold **`registry_alias`**, optional **`csv_path_override`** / **`profile_path_override`**, **`primary_convergence_session_ref`**, and links to **`linked_goal_refs`**, **`linked_milestone_refs`**, **`linked_roadmap_refs`**. Use **`matrix_role`** (e.g. `goal_progress`, `transition_gate`) to describe intent; **`gate_policy_notes`** / **`planning_notes`** capture gate rules and forward-looking context. Lifecycle is **`draft` → `active` → `archived`**. Drafts: **`zqk new object verification_matrix`**. Storage is **stream-backed** (append on create, **`stream_current`** overlay on update) with retention in **`retention_tolerance.yaml`** — see **`STREAM_STORAGE.md`**. A future **transition gate** evaluator would read these objects when blocking status changes (not implemented yet).

## `matrix get` (implemented)

- Resolves registry + CSV like `report`; profile path is recorded in output but full profile is not required to read rows.
- Filters: `--filter column=value` (repeatable, AND), `--glob` on `file_path` (`filepath.Match`), `--go-only`, `--cvs-id` (uses registry `session_ref_column`), `--limit`. Output projection: `--field` / `-C` (repeatable; order preserved; avoids clashing with global `--columns` for table widths).
- Output: `--format` json / yaml / table / **csv** (table uses YAML for human-readable structure; **csv** is RFC 4180 rows only—metadata fields are omitted, same as a plain CSV export of `header` + `rows`).

## Roadmap: `matrix update` (done vs future)

- **Implemented:** Single row via `--file-path` or `--bundle-label`; **bulk** via `--filter column=value` (repeatable, AND) + `--set`, optional `--limit` (first N matches in file order); profile gate validation; atomic write; `--dry-run`.
- **Bulk extras:** `--backup` (copy to `<csv>.bak` before replace) and `--backup-to <path>` (explicit path; overrides `--backup` default). `backup_path` in JSON result when written.
- **Dry-run preview:** Results include **`row_after`** (single-row) and **`row_after_samples`** (bulk, up to 5 full rows) so `--dry-run` shows the post-update state without writing.
- **value_map (optional):** Per-matrix in **`matrix_registry.yaml`**, maps lowercased tokens to canonical CSV values for **`zqk matrix update --set`** (e.g. `done: yes`). Omitted when using **`--matrix` / `--profile` overrides** (no registry entry).
- **CVS activity_log:** After successful update, optional **`--append-cvs-activity`**: appends one `activity_log` entry via storage (same merge rules as `object update`). Use **`--cvs-id`** (repeatable) and/or **`session_ref_column`** values from updated row(s). Incompatible with **`--dry-run`**. When **`--cvs-id`** is set, ids are read-checked before the CSV write; when ids come from the row column, the append runs after the write (fix the row or re-run if the session id is invalid).

## Relationship to `zqk system test-bundle-matrix` (PRUNED)

- **Test-bundle matrix** remains the **pipeline** for scheduler bundles + `health.jsonl` (generate + verify + optional convergence).
- **Codebase vetting matrix** is **file-level** gates (`fully_vetted`, …). Different profile shape; same **matrix** subsystem for `report` / future `get`/`update` where profiles align.

## Configuration on `convergence_session` (future)

- Optional fields (spec change + CLI): `matrix_registry_ref`, `primary_matrix_alias`, `matrix_default_filters` — so a session “knows” which matrix and slice it owns. Until then, **`cvs_id` on each row** + **`--cvs-id` flag** is sufficient.

## References

- `docs/quality/VETTING_RUBRIC.md` — column definitions; C1–C6 mapping
- `docs/quality/vetting_matrix_profile.yaml` — gate columns
- `pkg/storage/object_storage_file_update.go` — `convergence_session` `activity_log` append merge behavior
