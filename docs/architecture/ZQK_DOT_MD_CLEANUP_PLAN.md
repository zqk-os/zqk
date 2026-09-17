# .zqk/ Markdown Cleanup: Analysis and Remedial Action Plan

**Last Verified:** 2026-08-31


**Status:** Draft  
**Created:** 2026-02-17  
**Purpose:** Align documentation with project document policy by cleaning up scattered `.md` files under `.zqk/`, merging/removing where appropriate, and relocating durable content into the indexed `docs/` tree with `doc_entry` findability.

---

## 1. Document Policy (Current State)

- **POL-DOC-001** (Documentation Structure, Location, and Preferences): All documentation must live in the **`docs/`** tree and be **registered with the system** (e.g. `zqk automation docman-sync` or `zqk docman register`). Creating detailed docs in code directories (or by extension in runtime directories like `.zqk/`) is **incorrect**.
- **AI_AGENT_YAML_EDIT_POLICY:** Markdown files should be registered via docman-sync; the canonical place for docs is under `docs/`.
- **Findability:** Documents under `docs/` are indexed via **`doc_entry`** objects (`.zqk/process/doc_entries/`), with `path`, `group`, `category`, `title`, and `summary` for search and discovery.
- **`.zqk/` role:** Runtime and working data (config, logs, scheduler state, autofix batch files, system-state snapshots). It is **not** the designated location for long-lived project documentation.

---

## 2. Current Situation: .zqk/ Markdown Inventory

**Total:** 122 `.md` files under `.zqk/` (including subdirectories).

### 2.1 Categories (Summary)

| Category | Approx. count | Description | Recommendation |
|----------|----------------|-------------|-----------------|
| **Ephemeral / status** | ~50+ | Date-stamped or “resolution/status/progress/final” notes (e.g. `system-check-resolution-*.md`, `*-2026-01-28.md`, `FINAL_STATUS.md`, `IMPROVEMENT_STATUS.md`) | **Remove** (or archive once in `docs/` as a single “historical status” artifact if desired). |
| **Test / retrofit** | ~20 | Test inventory, retrofit progress, coverage policy, skip policy, patterns, infrastructure summaries | **Merge** into 1–2 docs in `docs/testing/` or `docs/architecture/`; **remove** duplicates. |
| **System-check / autofix** | ~15 | Resolution plans, hang diagnosis, fix verification, autofix steps | **Merge** key patterns/lessons into existing `docs/archive/system_health/` or scheduler docs; **remove** one-off status files. |
| **Performance / concurrency** | ~10 | Unbounded concurrency fixes, goroutine/thread analysis, efficiency improvements | **Move** durable content (e.g. unbounded-concurrency pattern) to `docs/architecture/`; **remove** redundant summaries. |
| **MCP / job / scheduler** | ~10 | MCP troubleshooting, job failure analysis, scheduler history/cleanup, CAS index write queue | **Move** durable content to `docs/architecture/mcp/` or `.zqk/process/scheduler/`; **remove** one-off analysis. |
| **Policy-like** | 2–3 | e.g. `test-coverage-policy.md`, `skip-policy.md` | **Move** to `docs/testing/` or policy-backed doc in `docs/`; consider formal policy object if needed. |
| **Audits / analyses** | ~15 | Error wrapping, channel leak, code compliance, unused code, TODO, import cycle | **Merge** lessons into `docs/refactoring/` or `docs/enforcement/`; **remove** raw duplicates. |
| **CLI / specs** | 2 | `.zqk/cli/specs/COMMAND_SPEC_COVERAGE.md`, `COMMAND_ORGANIZATION.md`, `schemas/README.md` | **Move** to `docs/architecture/` or `.zqk/process/` and update references (e.g. ASYNC_RETROFIT_STATUS). |
| **System-state README** | 1 | `.zqk/system-state/README.md` | **Keep** as minimal “what’s in this directory” or **move** to `docs/` as “System state and snapshots” and replace with one-line pointer. |

(Remaining files are small clusters of design/implementation notes that can be merged into the above or removed.)

---

## 3. In-Repo References to .zqk/*.md (Must Update After Moves)

These files under `docs/` or `cmd/` reference `.zqk/` markdown and **must** be updated to point at the new locations (or removed if the target doc is deleted):

| Referencing file | Current reference | Action after cleanup |
|------------------|-------------------|----------------------|
| `docs/architecture/PRE_CHANGE_CHECKLIST.md` | `.zqk/unbounded-concurrency-fixes.md` | Point to new path in `docs/architecture/` (e.g. `unbounded-concurrency-fixes.md` or merged concurrency doc). |
| `docs/architecture/CLI_PERFORMANCE_AND_CONSISTENCY.md` | `.zqk/unbounded-concurrency-fixes.md` | Same as above. |
| `docs/architecture/ASYNC_RETROFIT_STATUS.md` | `.zqk/cli/specs/COMMAND_SPEC_COVERAGE.md` | Point to new path (e.g. `docs/architecture/COMMAND_SPEC_COVERAGE.md` or equivalent). |
| `docs/architecture/system-check-data-flow.md` | `.zqk/logs/system-check-profile-findings.md` | Point to doc in `docs/` if kept; else remove or say “see system-check run output”. |
| `docs/refactoring/REMAINING_REFACTOR_WORK.md` | `.zqk/error-wrapping-audit.md`, `.zqk/channel-leak-detection-audit.md`, “remove remaining .zqk references” | Point to new refactoring/audit docs; complete “remove .zqk references” as part of this cleanup. |
| `docs/refactoring/CODING_GUIDELINES.md` | `.zqk/error-wrapping-audit.md`, `.zqk/channel-leak-detection-audit.md` | Point to new locations under `docs/refactoring/` or `docs/enforcement/`. |
| `docs/refactoring/REFACTORING_STATUS.md` | `.zqk/performance-optimizations-complete.md` | Point to new doc or remove if obsolete. |
| `docs/architecture/ADR-MCP-SCALABILITY.md` | `.zqk/mcp/CHANGELOG.md` | If MCP changelog moves to `docs/`, point there; else keep as optional runtime path and document. |
| `docs/onboarding/STABLE_BINARY_MANAGEMENT.md` | `.zqk/mcp/CHANGELOG.md` | Same as above. |
| `docs/best-practices/coding/README.md` | `.zqk/error-wrapping-audit.md` | Point to new audit doc in `docs/`. |

Any **in-.zqk** references (e.g. `.zqk/test-coverage-policy.md` referencing `.zqk/job-update-path-fix.md`) become irrelevant once source files are moved or removed; update or drop when consolidating.

---

## 4. Recommended Remedial Action Plan

### Phase 1: One-time moves and merges (durable content only)

1. **Unbounded concurrency**
   - **Move** `.zqk/unbounded-concurrency-fixes.md` → `docs/architecture/unbounded-concurrency-fixes.md`.
   - **Update** `PRE_CHANGE_CHECKLIST.md` and `CLI_PERFORMANCE_AND_CONSISTENCY.md` to reference `docs/architecture/unbounded-concurrency-fixes.md`.
   - Run `zqk automation docman-sync --git-aware` (or register the new doc) so it gets a `doc_entry`.

2. **Test policy and skip policy**
   - **Move** `.zqk/test-coverage-policy.md` → `docs/testing/TEST_COVERAGE_POLICY.md`.
   - **Move** `.zqk/skip-policy.md` → `docs/testing/SKIP_POLICY.md`.
   - Optionally create/update policy objects (POL-DOC-001 style) if these are formal project policies.
   - Register with docman-sync.

3. **CLI / command specs**
   - **Move** `.zqk/cli/specs/COMMAND_SPEC_COVERAGE.md` → `docs/architecture/COMMAND_SPEC_COVERAGE.md` (or `docs/architecture/COMMAND_SPEC_COVERAGE.md`).
   - **Move** `.zqk/cli/specs/COMMAND_ORGANIZATION.md` to same area if still relevant.
   - Update `docs/architecture/ASYNC_RETROFIT_STATUS.md` to point to the new path(s).

4. **MCP troubleshooting**
   - **Merge** durable content from `.zqk/MCP_SERVER_TROUBLESHOOTING.md` (and related MCP_*.md) into `docs/architecture/mcp/mcp-troubleshooting-analysis-v1.0.md` or a dedicated MCP troubleshooting doc in `docs/architecture/mcp/`.
   - If `.zqk/mcp/CHANGELOG.md` exists and is useful, **move** to `docs/architecture/mcp/` or `docs/onboarding/` and update ADR/onboarding references.

5. **Refactoring / audits**
   - **Merge** key lessons from `.zqk/error-wrapping-audit.md`, `.zqk/channel-leak-detection-audit.md`, and (if still relevant) code-compliance/error-handling analyses into `docs/refactoring/` or `docs/enforcement/` (e.g. CODING_GUIDELINES.md, REMAINING_REFACTOR_WORK.md).
   - **Update** REMAINING_REFACTOR_WORK.md, CODING_GUIDELINES.md, and best-practices README to point to these docs (no `.zqk` paths).

6. **System-state**
   - **Option A:** Keep `.zqk/system-state/README.md` as a short “this directory is system-managed; see docs” and add one line pointing to a doc in `docs/` (e.g. `docs/architecture/SYSTEM_STATE_AND_SNAPSHOTS.md`).
   - **Option B:** **Move** content to `docs/architecture/SYSTEM_STATE_AND_SNAPSHOTS.md` and replace README with a one-line pointer. Prefer one place in `docs/` for findability.

### Phase 2: Remove or archive ephemeral .md under .zqk/

7. **Delete** (or archive in a single “historical status” doc if you want one snapshot):
   - All `system-check-resolution-*.md`, `system-check-*-2026-01-28.md`, `system-check-final-status*.md`, `system-check-*progress*.md`, etc.
   - Date-stamped one-off summaries (e.g. `*-2026-01-17.md`, `*-2026-01-28.md`).
   - `FINAL_STATUS.md`, `IMPROVEMENT_STATUS.md`, `IMMEDIATE_CLEANUP_SUMMARY.md`, and similar “status” docs.
   - Redundant test retrofit/infrastructure summaries (keep one merged doc in `docs/testing/` if needed; delete the rest from `.zqk/`).
   - Duplicate design notes (e.g. multiple CAS_INDEX_WRITE_QUEUE_*.md can be one doc in `docs/` or removed if superseded).

8. **Leave .zqk/ without durable .md**
   - After moves and merges, **no** long-lived project documentation should remain under `.zqk/`. Optional: keep a single `.zqk/README.md` that states “Runtime and working data only; see docs/ for documentation.”

### Phase 3: doc_entry and policy alignment

9. **Register all new/updated docs**
   - Run `zqk automation docman-sync --git-aware` so every new or moved doc under `docs/` gets a `doc_entry` (or register manually with `zqk docman register`).
   - Confirm `doc_entry` list includes the new paths and that `path` points under `docs/`.

10. **Optional: Document .zqk/ in policy**
    - Add a short note to POL-DOC-001 (or a “Document placement” section in AGENT_GUIDELINES or PRE_CHANGE_CHECKLIST): “Documentation does not live under `.zqk/`; use `docs/` and docman-sync for findability.”

---

## 5. Execution Checklist (High Level)

- [ ] Phase 1: Move/merge unbounded-concurrency, test policies, CLI specs, MCP troubleshooting, refactoring/audits, system-state (and update all in-repo references from section 3).
- [ ] Phase 2: Delete or archive ephemeral and duplicate .md under `.zqk/`; optionally add `.zqk/README.md` stating “runtime only.”
- [ ] Phase 3: Run docman-sync; verify doc_entries for new paths; optionally update POL-DOC-001 or PRE_CHANGE_CHECKLIST.
- [ ] Grep for remaining `.zqk/**/*.md` or `\.zqk/.*\.md` in `docs/` and fix any leftover references.

---

## 6. Summary

- **Problem:** 122 `.md` files under `.zqk/` violate POL-DOC-001 and reduce findability (not indexed via `doc_entry`).
- **Approach:** Move durable content into `docs/` (architecture, process/testing, process/refactoring, process/architecture/mcp, etc.), merge duplicates and status bloat, remove ephemeral files, update all in-repo references, then run docman-sync so the docs tree remains the single indexed, organized place for findability.

This plan should be executed in order (moves and reference updates first, then deletions, then doc_entry sync) to avoid broken links and to keep a clear audit trail.
