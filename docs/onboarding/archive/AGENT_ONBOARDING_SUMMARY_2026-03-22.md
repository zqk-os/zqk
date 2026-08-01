# Agent onboarding summary — 2026-03-22

**Generated:** 2026-03-22  
**Category:** onboarding · agent-summary · workflow  
**Purpose:** One-stop snapshot for a new agent: mental model, non-negotiables, where truth lives, how recent work and git activity line up, and what to do next.

**Doc entry:** `DOC-EXAMPLE` — discover via `zqk object get DOC-EXAMPLE` or `zqk object list doc_entry --filter group=onboarding`.

---

## 1. What this project is

**ZQK (Zen Quantum Kernel)** is a Go CLI (`zqk`) that behaves like an **operating layer** for AI–human teams: typed objects in CAS-backed storage, integrity/audit, scheduler-driven background work, and MCP mirroring the CLI. Strategic alignment is **STRAT-PLAN-001** (three phases through 2028). The normative idea is **object-first process data**: plans, backlog, policies, glossary, convergence sessions, etc. are **system objects**; **instance YAML under `docs/architecture/` is not edited by hand** — use **`zqk object …`** (or MCP tools that wrap the same).

---

## 2. Onboarding materials (what to read first)

| Resource | Role |
|----------|------|
| [docs/onboarding/README.md](../README.md) | Entry; links to the main guide and quick CLI discovery |
| [docs/onboarding/AI_AGENT_ONBOARDING.md](../AI_AGENT_ONBOARDING.md) | Canonical agent guide: routines, CLI/MCP, maintenance jobs, convergence, git, OHTV, policies, architecture discovery |
| [docs/architecture/README.md](../../process/enforcement/OBJECT_FIRST_ALIGNMENT.md) | “Start here” policy object, onboarding roadmap as objects |
| [docs/architecture/architecture/system-object-discovery-guide-v1.0.md](../../process/architecture/system-object-discovery-guide-v1.0.md) | How to **discover** truth via `zqk object list/get` |
| [docs/architecture/PRE_CHANGE_CHECKLIST.md](../../architecture/PRE_CHANGE_CHECKLIST.md) | Mandatory before code/CLI/process-impacting changes |
| [docs/architecture/README.md](../../process/architecture/README.md) | Large auto-index of ADRs, CLI normative path, MCP bridge, storage, validation |
| [docs/best-practices/coding/README.md](../../best-practices/coding/README.md) | Hub for error handling, concurrency, refactoring pointers |
| Prior snapshots | [AGENT_ONBOARDING_SUMMARY_2026-03-19.md](./AGENT_ONBOARDING_SUMMARY_2026-03-19.md); [AGENT_ONBOARDING_ASSESSMENT_2026-03-21.md](../AGENT_ONBOARDING_ASSESSMENT_2026-03-21.md) |

---

## 3. Workflows agents actually follow

1. **Direction:** Prefer live objects over stale markdown: `priority_plan` (especially **`status=in_progress`**), `backlog_item`, `convergence_session`, `glossary_term`, `policy`.
2. **Integrity:** All process/instance mutations through **`zqk`**; pre-commit/system checks keep hashes and caches coherent.
3. **Investigation:** **OHTV** (Observe → Hypothesize → Test → Verify); for reliability issues, **convergence evidence first** (persistence and violation counts).
4. **Code quality:** **Logger from profile** / **`cli.WriteOutput`** — not raw `fmt.Print*` to users (`AGENT_GUIDELINES.md`).
5. **Tests:** `go test` always with **`-timeout`**; long suites via **`go test ./...`** or **`zqk scheduler go test`** with logs to files.
6. **Git:** Feature/fix/chore branches; **no direct commits to `main`** without explicit authorization.
7. **Health:** After init, **`zqk system ensure-retention-jobs`** so maintenance/WAL/retention/aggregation jobs exist.

---

## 4. Architecture and system objects (compressed)

- **Storage:** CAS + streams + caches; scheduler and job state follow **filesystem layout** rules (bounded top-level entries — see `FILESYSTEM_DATA_LAYOUT.md` / checklist §16).
- **Specs:** Object kinds defined in `_internal` specs; **generated instance builders** in `pkg/specbuilder/bldr_instance_v1/` — **do not hand-edit**.
- **CLI bridge:** MCP exposes CLI commands, not a parallel API.
- **Convergence:** Remediation campaigns use **`convergence_session` (`CVS-*`)** with phases (C1–C6); **`zqk scheduler convergence measure`** supports the test-bundle story.

---

## 5. How prior summaries relate to now

- **2026-03-19 summary** emphasized PRI-218 and recommended **`system object-count-report` latency** work from the improvement report when that is the top item under the **current** plan.
- **2026-03-21 assessment** noted branch **`feature/pri-221-product-performance`**, doc wording drift, and recommended short cross-links (glossary ↔ convergence ↔ scheduler).

---

## 6. Live project state (snapshot at generation; re-verify with CLI)

| Signal | Value (example) |
|--------|------------------|
| **Git branch** | `feature/pri-221-product-performance` |
| **Execution-focused plan** | **PRI-221** — *Product & Performance* — **`status=in_progress`** |
| **Other active plans** | PRI-218, PRI-222, PRI-223 — `status=active` |
| **Active convergence session** | **`CVS-EXAMPLE`** — *Get test bundles green* — check **`zqk object get`** for current phase and fingerprints |

Run:

```bash
zqk object list priority_plan --filter 'status=in_progress' --format table
zqk object list convergence_session --filter status=active --format json
```

---

## 7. Recent version-control themes (approx. last days)

- **Scheduler / convergence:** Tombstones, convergence measure routing, measure scripts.
- **Process / convergence:** Draft **convergence_session** + backlog linkage; phase router/coordinator design.
- **Docs / ontology:** Semantic kernel concept; **doc_entry** and **glossary_term** updates.
- **Ongoing:** **`data updates`** — CLI-driven persistence of process objects and scheduler job records.

---

## 8. Recommended next action

**Primary:** Use the active **`convergence_session`** as the urgent feedback loop: **`zqk scheduler convergence measure`** and follow **`next_action` / activity log**; prefer **short** targeted **`go test`** or **`go test`** for listed fingerprints before wide **cmd/zqk/system** runs.

**Secondary:** **`zqk object list backlog_item --filter priority_plan_ref=PRI-221`** and work **P0 → P1 → …** per priority-plan rules.

---

*Last updated: 2026-03-22*
