# Agent onboarding summary — 2026-03-25

**Generated:** 2026-03-25  
**Category:** onboarding · agent-summary · workflow  
**Purpose:** Condensed mental model after reviewing onboarding docs: mission, strategy, **CLI-first** and **object-first** norms, repository layout signals, recent git themes, and a **live** pointer for what to do next (re-verify with `zqk object …`).

**Doc entry:** `DOC-1774468489822509000-607a4940` — discover via `zqk object get DOC-1774468489822509000-607a4940` or `zqk object list doc_entry --filter group=onboarding`.

**Related snapshot:** [AGENT_ONBOARDING_SUMMARY_2026-03-22](./AGENT_ONBOARDING_SUMMARY_2026-03-22.md).

---

## 1. Mission, vision, and strategy (from process docs)

- **Product:** **ZQK (Zen Quantum Kernel)** — a production Go CLI (`zqk`) acting as an **operating layer** for AI–human teams: typed objects, CAS-backed storage, integrity and audit, scheduler-driven background work, and **MCP mirroring the CLI** (CLI bridge pattern).
- **Mission (MIS-001, ZQK framing):** Address fragmentation of intelligence by providing a **distributed knowledge kernel** so collaboration is effective, safe, reliable, and observable — enabling **Agent-Driven Software Engineering (ADSE)** vs. traditional CASE.
- **Vision (VIS-001):** A shared command center where intent, dependencies, and goals are unified — graph-capable kernel, governance, observability, and provenance as themes.
- **Strategic plan:** **STRAT-PLAN-001** (2026–2028), three phases — **Foundation (2026):** ontology, semantic bridge, system commands; **Advanced (2027):** org modeling, domains; **Scale (2028):** enterprise and multi-instance concerns.

Canonical onboarding entry: [docs/onboarding/AI_AGENT_ONBOARDING.md](../AI_AGENT_ONBOARDING.md). Extended mission/vision context: [docs/analysis/PROJECT_CONTEXT_SUMMARY.md](../../process/analysis/PROJECT_CONTEXT_SUMMARY.md).

---

## 2. CLI-first (normative path)

- **All meaningful object and process mutations** go through **`zqk`** (or MCP tools that invoke the same commands). Editing instance YAML under `.zqk/process/` by hand is **out of band** — it bypasses integrity hashes, caches, audit, and validation.
- **MCP** exposes CLI commands, not a parallel API — keeps a single behavior surface.
- Prefer **`zqk query`** over ad hoc `jq`/`yq` for JSON path post-processing; use CLI **`time` / `timestamp`** for time semantics; wrap non-CLI shell work with **`command-timings`** when required by onboarding.
- **Maintenance:** after init, ensure scheduler maintenance jobs exist: `zqk system ensure-retention-jobs`.
- **Investigation workflow:** **OHTV** (Observe → Hypothesize → Test → Verify); for autofix/convergence complaints, see **convergence evidence first** (persistence and whether violations actually drop).

---

## 3. Object-first (source of truth)

- **Policies, backlog, priority plans, goals, milestones, convergence sessions, glossary terms, doc entries,** etc. are **first-class objects**. Markdown in `docs/` is explanatory or indexed views; **live truth** is discovered with **`zqk object list` / `zqk object get`** and filters.
- **Alignment doc:** [docs/enforcement/OBJECT_FIRST_ALIGNMENT.md](../../process/enforcement/OBJECT_FIRST_ALIGNMENT.md) — includes pointers to “Start Here” tutorial policies and onboarding roadmap as objects (`PRIO-onboarding` backlog filter).
- **Traceability:** backlog items link to milestones, criteria, and requirements where the model supports it; use objects for prioritization, not informal notes alone.

---

## 4. Project structure (high level)

| Area | Role |
|------|------|
| `cmd/zqk/` | CLI commands and wiring |
| `pkg/storage/` | CAS, streams, caches, persistence paths |
| `pkg/validation/` | Object and reference validation |
| Scheduler / jobs | Background work, test bundles, maintenance |
| `pkg/specbuilder/` | Specs and **generated** instance builders (do not hand-edit generated `*_instance_builder.go`) |
| `.zqk/process/` | Object **instances** (change via CLI only) |
| `docs/architecture/`, `docs/onboarding/` | Human-readable architecture and onboarding |

Before code changes: [docs/architecture/PRE_CHANGE_CHECKLIST.md](../../architecture/PRE_CHANGE_CHECKLIST.md). Before user-facing CLI output: [docs/enforcement/AGENT_GUIDELINES.md](../../process/enforcement/AGENT_GUIDELINES.md).

---

## 5. Git history — themes (recent ~2 weeks, representative)

**Branch observed:** `feature/pri-221-product-performance`.

- **Durability / storage:** Flush-after-write fixes for internal and bulk create/update/delete paths; classification of runtime-delta updates by effective changed fields; stabilization of object comprehensive tests (refs, bulk IDs, templates).
- **Observability:** `feat(system): add update mutation metrics report command` and plumbing (`mutation class` for update decisions).
- **Model / spec:** Optional `related_object_refs` on `base_object` for cross-kind links.
- **Process data:** Frequent **`data updates`** commits — CLI-driven persistence of process objects (expected in this repo).
- **Earlier in window:** Convergence session debrief fields, scheduler finalize debrief, CLI timeout adjustments for long maintenance commands, requirement traceability updates (e.g. toward PRI-221).

*Interpretation:* Execution is focused on **reliability of writes and scheduler-backed workflows**, **metrics around mutations**, and keeping **process objects** aligned via CLI — consistent with **Product & Performance** and convergence-adjacent work.

---

## 6. Live state snapshot (re-verify before acting)

Commands:

```bash
zqk object list priority_plan --filter 'status=in_progress' --format table
zqk object list priority_plan --filter 'status=active' --sort-by active_order --format table
zqk object list backlog_item --filter 'priority_plan_ref=<PLAN_ID>' --format json
zqk object list convergence_session --filter status=active --format json
zqk system check --fast
```

**At file generation time:**

- **`priority_plan` in progress:** **PRI-221** — *Product & Performance (1 week)*.
- **Other `active` plans (by `active_order`):** PRI-218, PRI-222, PRI-223 (themes: Development & Quality, Documentation & Observability, Phase 1 core backlog).
- **Active `convergence_session`:** none listed (empty filter result) — **backlog and PRI-221** drive work unless a new session is opened.
- **PRI-221 backlog — incomplete items include multiple P0 `in_progress` themes:** retention-tolerance error investigation, scheduler `scan-tests` reliability, and system-check timeout standardization; plus P1/P2 items (convergence lifecycle E2E, spec loader refactor, missing system commands, etc.).

**Priority rule from workspace policy:** Finish the **current** priority plan’s active work (**PRI-221** while it remains `in_progress`) before treating other active plans as the main execution thread.

---

## 7. Recommended next priority (opinion)

**Stay on PRI-221** until its remaining items are **complete** or explicitly re-planned: the three **P0 / in_progress** items (retention-tolerance errors, **`scan-tests`** error/timeout rates, **system check** timeout behavior) are the highest-leverage alignment with recent git themes (storage durability, scheduler, observability). Pick one P0 thread, close it with tests and measurable verification, then rotate.

If **convergence** returns: treat **`convergence_session`** objects and `zqk scheduler convergence measure` as the urgent loop again; when no CVS is active, rely on **`zqk object list backlog_item`** for PRI-221 ordering (P0 before P1).

---

*Last updated: 2026-03-25*
