# Agent onboarding summaries — digest (compressed history)

**Doc entry:** `DOC-1775197433691153000-acbe23e3` — `zqk object get DOC-1775197433691153000-acbe23e3` (or `zqk object list doc_entry --filter group=onboarding`).

**Purpose:** Preserve **direction and context** from the dated snapshot series (**2026-03-19 → 2026-04-04**) without maintaining a long list of peer files in `docs/onboarding/`. Full originals: [`archive/`](./archive/).

**How to use:** Read this for **themes and evolution**. For **what to do now**, use [`AGENT_ONBOARDING_SNAPSHOT.md`](./AGENT_ONBOARDING_SNAPSHOT.md) and live `zqk object …` commands.

---

## Stable norms (repeated across every snapshot)

- **CLI-first / object-first:** Mutate process data with **`zqk`** (or MCP wrapping it), not by editing instance YAML under `.zqk/process/`.
- **Session start:** In-progress **`priority_plan`**, active **`convergence_session`** (if any), **`zqk system check --fast`**, alignment to **STRAT-PLAN-001**.
- **Quality:** **PRE_CHANGE_CHECKLIST** before substantive edits; **AGENT_GUIDELINES** for output/logging; **OHTV** for investigations; convergence discipline (**evidence first** for autofix loops).
- **Tests:** Narrow **`go test -timeout …`**; broad or long runs via **scheduler** / **`test-runner.sh`** / **`zqk scheduler scan-tests`** with logs under **`.zqk/logs/`**.
- **Work ordering:** Finish the current **priority plan** thread before treating another plan as the main execution line.
- **Discovery:** Policies, backlog, and doc index are **objects** — prefer **`zqk object list` / `get`** over assuming markdown alone is current.

---

## Narrative arc (March–April 2026)

| Period | Plan / branch theme | Convergence / focus | Notes |
|--------|---------------------|---------------------|--------|
| **03-22 → 03-25** | Establish **dated snapshots** + `doc_entry` pattern; multi-plan era in older notes | Workflow + convergence commands documented | CLI-first, repo layout, git themes codified. |
| **03-27** | **PRI-221** (*Product & Performance*) vs other **active** plans | Policy: finish current plan before switching | “Live CLI next steps” pattern in onboarding. |
| **03-28 → 03-29** | Shift to **[REDACTED-ID]** — *Object maintenance redesign* (`in_progress`) | **[REDACTED-ID]** — code quality (DRY, pipelines, spec builders, constants, lint); **`c5_verify`** | Six-part **`desired_end_state`**; package-gate cadence; test bundles as boundary signal. |
| **03-31 → 04-01** | Same PRI; branch **`feature/pri-object-maintenance`** | **[REDACTED-ID]** — maps.Copy / golangci / **`maps.Copy`** tranches; test-bundle health as **exit gating** | Convergence deep in **C4/C5** with fingerprints and **`zqk scheduler convergence measure`**. |
| **04-02** | Unchanged PRI | **[REDACTED-ID]** — **integration test fixture isolation** (no live checkout as storage root); **`c5_verify`** | Active work shifted from maps.Copy session to **test isolation**; temp-dir / ephemeral roots as the pattern. |
| **04-03 → 04-04** | Same PRI | **[REDACTED-ID]** — vetting matrix / storage teardown traceability → **`completed`** (**`c6_exit`**) | **`status=active` convergence_session** often **empty** afterward; execution returns to **PRI backlog** (P0 maintenance delivery, P1 teardown + spec plane). |

**Takeaway:** The **umbrella plan** (*Object maintenance redesign*) stayed stable; **active CVS** rotated from broad code-quality convergence to **integration-test isolation** — both still under the same PRI and process norms.

---

## Pointers (unchanged)

- **Strategic plan:** `zqk object get STRAT-PLAN-001`
- **Policies index:** `docs/enforcement/POLICY_ENFORCEMENT_INDEX.md`, **`zqk object list policy`**
- **Object-first alignment:** `docs/enforcement/OBJECT_FIRST_ALIGNMENT.md`
- **Architecture index:** `docs/architecture/README.md`
- **Test bundle health:** `.zqk/logs/scheduler/cvs/test-bundles/health.jsonl`, **`zqk scheduler test-failures health`**, **`convergence`**
- **Quality / bundle matrix (CSV + native pipeline):** `docs/quality/README.md`, **`zqk system test-bundle-matrix` (PRUNED)**, `docs/architecture/data-pipeline-lifecycle.md`

---

*Digest introduced when dated summaries were moved to `archive/` (2026-04-03).*
