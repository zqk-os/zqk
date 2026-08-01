# Agent onboarding summary — 2026-04-02

**Generated:** 2026-04-02  
**Category:** onboarding · agent-summary · workflow  
**Purpose:** Consolidated review after reading the onboarding corpus: expectations, how pieces fit, **live** CLI snapshot at generation time, **progression** versus 2026-04-01, **CVS** state, and **anticipated next steps**.

**Related snapshots:** [AGENT_ONBOARDING_SUMMARY_2026-04-01](./AGENT_ONBOARDING_SUMMARY_2026-04-01.md), [AGENT_ONBOARDING_SUMMARY_2026-03-31](./AGENT_ONBOARDING_SUMMARY_2026-03-31.md).

**Doc entry:** `DOC-EXAMPLE` — `zqk object get DOC-EXAMPLE` or `zqk object list doc_entry --filter group=onboarding`. *(Created with `zqk object create doc_entry` — `zqk docman register` uses sequential `DOC-NNN` IDs and did not auto-create this path in this workspace; the long-form ID matches the index in [README.md](../README.md).)*

---

## 0. Progression since 2026-04-01

| Topic | 2026-04-01 snapshot | 2026-04-02 (this file) |
|--------|---------------------|-------------------------|
| **PRI** | `PRI-EXAMPLE` *Object maintenance redesign* (`in_progress`) | Unchanged: same plan; branch context `feature/pri-object-maintenance`. |
| **Active CVS** | `CVS-EXAMPLE` (maps.Copy / package-gate / golangci convergence) | **Different active session:** `CVS-EXAMPLE` — *Integration tests: fixture isolation (eliminate real-checkout storage roots)*, phase **`c5_verify`** (live snapshot), flow `testing-isolation`. Test-bundle fingerprints were all **pass** at last measure; session still completing per `desired_end_state`. |
| **Backlog (PRI)** | Six items referenced | Six items: delivery in progress; migrate storage/system tests *exploring*; spec origin plane *roadmap*; PHC items *exploring*; others *exploring*. |
| **System check** | (prior run) | `zqk system check --fast`: internal objects clean; medium reminders for tier-3 informational issues and retention drift toward targets. |

**Takeaway:** Onboarding norms (CLI-first, object-first, PRE_CHANGE, scheduler for broad tests, convergence discipline) are unchanged. **Active convergence work** has shifted to **test isolation**: integration tests must not use the live checkout as a storage root; use temp fixtures and documented patterns.

---

## 1. Corpus reviewed (reference)

| Document | Role |
|----------|------|
| [docs/onboarding/README.md](../README.md) | Index: main guide, dated summaries, supplements. |
| [AI_AGENT_ONBOARDING.md](../AI_AGENT_ONBOARDING.md) | Canonical guide: STRAT-PLAN-001, routines, maintenance jobs, **convergence_session**, CLI/MCP-only process mutations, policies, OHTV, discovery commands. |
| [AGENT_GUIDELINES.md](../../process/enforcement/AGENT_GUIDELINES.md) | Policies, logging, command surfaces. |
| Workspace rules | Process YAML via **zqk** only; long tests via scheduler / `test-runner.sh`; `go test` with `-timeout`; convergence session discipline. |

---

## 2. Thematic synthesis (unchanged core)

1. **Normative path:** Mutate process objects via **`zqk`** / MCP, not direct instance YAML under `docs/architecture/`.  
2. **Session start:** In-progress **priority plan**, active **`convergence_session`** (if any), **`zqk system check --fast`**, **STRAT-PLAN-001**.  
3. **Quality:** OHTV; instance builders for spec-backed creates; logging per AGENT_GUIDELINES.  
4. **Work ordering:** Finish current **priority plan** backlog thread before switching plans.  
5. **Tests:** Narrow **`go test -timeout`**; package / iteration boundaries via **`zqk scheduler go test`** and logs under **`.zqk/logs/scheduler/cvs/test-bundles/`**.

---

## 3. Live CVS snapshot (**CVS-EXAMPLE**) — re-verify

- **Title:** Integration tests: fixture isolation (eliminate real-checkout storage roots).  
- **Phase:** `c5_verify` (re-verify with `zqk object get` — phases advance with session work).  
- **Hypothesis:** Refactoring integration tests to use only `t.TempDir()` (or ephemeral roots) with copied/generated fixtures removes cross-contamination with the operator checkout.  
- **Desired end state:** No storage-backed test uses the live repo as `FileObjectStorage` root unless explicitly exempted and reviewed; documented pattern for new integration tests; optional CI guard for anti-patterns.  
- **next_action (live):** Test-bundle window is green; confirm scheduler `Executing=0` for relevant `pkg/storage` bundle jobs when idle; continue fixture isolation (temp roots only) per `iteration_process`; reconcile `docs/process` if still divergent after restore.  
- **Verify with:** `zqk object get CVS-EXAMPLE`, `zqk scheduler convergence measure` after bundle runs.

---

## 4. Commands to re-verify before acting

```bash
zqk object list priority_plan --filter 'status=in_progress' --format table
zqk object list backlog_item --filter 'priority_plan_ref=PRI-EXAMPLE' --format table
zqk object list convergence_session --filter status=active --format json
zqk object get CVS-EXAMPLE
zqk system check --fast
zqk scheduler test-failures health
zqk scheduler convergence measure
zqk system policy-interrupts pending (PRUNED)
```

---

## 5. Anticipated next steps

1. **Re-run §4** so priority, backlog, and CVS fields match the moment of execution.  
2. **Align with active CVS:** Drive integration tests toward **temp-only roots** and shared patterns; use targeted **`go test`** and **`zqk scheduler go test`** for touched packages.  
3. **Advance PRI-EXAMPLE** backlog items; keep process updates on the **CLI** path.  
4. **While CVS is active:** Honor **`desired_end_state`**, **`next_action`**, and **`iteration_process`** on the session object.  
5. **Before substantive code changes:** [PRE_CHANGE_CHECKLIST.md](../../architecture/PRE_CHANGE_CHECKLIST.md) + [AGENT_GUIDELINES.md](../../process/enforcement/AGENT_GUIDELINES.md).  
6. **Retention reminders:** If drift warnings persist, follow `zqk system retention-status` (PRUNED) and maintenance job guidance in onboarding.

---

*Last updated: 2026-04-02 (doc_entry registered; CVS phase aligned to live `c5_verify`)*
