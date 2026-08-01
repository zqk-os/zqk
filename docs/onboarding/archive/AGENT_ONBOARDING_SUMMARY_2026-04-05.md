# Agent onboarding summary — 2026-04-05

**Generated:** 2026-04-05  
**Category:** onboarding · agent-summary · workflow  
**Purpose:** Corpus review of onboarding docs plus **live** CLI snapshot after re-verification commands. Registers this file as a **`doc_entry`** for the onboarding index.

**Doc entry:** `DOC-EXAMPLE` — `zqk object get DOC-EXAMPLE` or `zqk object list doc_entry --filter group=onboarding`.

**Related:** [AGENT_ONBOARDING_SUMMARY_2026-04-04](./AGENT_ONBOARDING_SUMMARY_2026-04-04.md), [../AGENT_ONBOARDING_SNAPSHOT.md](../AGENT_ONBOARDING_SNAPSHOT.md), [../AGENT_ONBOARDING_SUMMARIES_DIGEST.md](../AGENT_ONBOARDING_SUMMARIES_DIGEST.md).

---

## 0. Corpus review (what the guides say)

| Layer | Document | Role |
|--------|-----------|------|
| **Index** | [docs/onboarding/README.md](../README.md) | Read order: canonical guide → **snapshot** (live pointers) → **digest** (compressed history) → **archive** (dated audit trail). |
| **Canonical** | [AI_AGENT_ONBOARDING.md](../AI_AGENT_ONBOARDING.md) | Project context (STRAT-PLAN-001, MCP), essential routines (timestamps, command-timings, `zqk query`/`yaml` vs jq/yq), TDD/criteria, **maintenance bundle** (`ensure-retention-jobs`), **convergence_session** workflow, **CLI/MCP-only** process mutations, git/PR flow, policies, architecture discovery, OHTV, lessons learned, and “next steps” via live `zqk object …` queries. |
| **Live pointers** | [AGENT_ONBOARDING_SNAPSHOT.md](../AGENT_ONBOARDING_SNAPSHOT.md) | Short re-verify block; **must be refreshed** when plan/CVS changes — static markdown goes stale (see §1 vs snapshot **as-of 2026-04-04**). |
| **History** | [AGENT_ONBOARDING_SUMMARIES_DIGEST.md](../AGENT_ONBOARDING_SUMMARIES_DIGEST.md) | Themes March–April 2026; stable norms (CLI-first, PRE_CHANGE, scheduler tests, plan ordering). |
| **Supplements** | EFFICIENT_DATA_PROCESSING, FIELD_STATE_TRACKING, **IMPORT_CYCLE_RESOLUTION** (required), refactoring links, one-off **ASSESSMENT 2026-03-21** | Deep dives; import-cycle doc is mandatory for structural changes. |

**Assessment:** The stack is coherent: one canonical guide, one snapshot for “now,” digest for narrative, archive for audit. **Gap:** `AGENT_ONBOARDING_SNAPSHOT.md` still lists example IDs from 2026-04-04; this summary records **2026-04-05** live state below — refresh the snapshot file when an operator wants the markdown to match the workspace again.

---

## 1. Live snapshot (this workspace, 2026-04-05)

Commands run: `priority_plan` (`in_progress`), `convergence_session` (`active`), backlog for PRI, `system check --fast`, `policy-interrupts pending`.

| Topic | State |
|--------|--------|
| **Priority plan (`in_progress`)** | `PRI-EXAMPLE` — *Object maintenance redesign* |
| **Active convergence** | `CVS-EXAMPLE-f1e2pkgvet` — *Package vetting and test-bundle pipeline reliability* — **`current_phase: c4_act`**; `next_action` cites failing bundle fingerprint(s) and suggested `go test` reruns. |
| **Backlog (same PRI)** | P0 **delivery** item **`BLI-EXAMPLE`** is **`complete`** (shift vs 2026-04-04 summary). **`BLI-EXAMPLE`** (*teardown migration*) is **`in_progress`**. Traceability/matrix and other items per `zqk object list backlog_item --filter priority_plan_ref=…`. |
| **System check** | Tiers clean; **medium** `retention_drift` reminder (internal count vs target) unchanged in kind from prior notes. |
| **Policy interrupts** | None pending. |

---

## 2. Commands to re-verify before acting

```bash
zqk object list priority_plan --filter 'status=in_progress' --format table
zqk object list backlog_item --filter 'priority_plan_ref=PRI-EXAMPLE' --format table
zqk object list convergence_session --filter status=active --format json
zqk object get CVS-EXAMPLE-f1e2pkgvet
zqk object get STRAT-PLAN-001
zqk system check --fast
zqk system policy-interrupts pending (PRUNED)
zqk scheduler test-failures health
zqk scheduler convergence measure
```

---

## 3. Proposed next steps

1. **Refresh** [AGENT_ONBOARDING_SNAPSHOT.md](../AGENT_ONBOARDING_SNAPSHOT.md) so **as-of** date, **active CVS** id/phase, and backlog examples match §1 (optional but reduces confusion vs 2026-04-04 text).  
2. **Drive active CVS** toward `desired_end_state` (green bundle window / teardown patterns per session text); use `zqk scheduler convergence measure` and suggested reruns.  
3. **Continue PRI work** through **`in_progress`** backlog (teardown migration and planned P1 items); keep mutations on **CLI/MCP** only.  
4. If **`retention_drift`** matters operationally: `zqk system retention-status` (PRUNED), `zqk system ensure-retention-jobs`, and maintenance runbooks referenced in the main onboarding guide.  
5. **Digest:** No change required unless new multi-week themes emerge; append a row to the digest table only when the narrative arc materially shifts.

---

*Audit-trail summary; primary navigation remains [AI_AGENT_ONBOARDING.md](../AI_AGENT_ONBOARDING.md) and [AGENT_ONBOARDING_SNAPSHOT.md](../AGENT_ONBOARDING_SNAPSHOT.md).*
