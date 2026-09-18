# Onboarding Documentation

**All new AI agents: start here, then the canonical guide.**

## Read order

1. **[Community first-run](./COMMUNITY_FIRST_RUN.md)** — Fail-closed `agent-onboard` sequence (detect → seat → prime → smoke). Start here for strangers / open-core.
1b. **[Edge / headless first-run](./EDGE_HEADLESS_FIRST_RUN.md)** — Vector B (appliances, DGX/Spark, local SLM hosts): `--headless`, edge signals, market probes.
2. **[AI Agent Onboarding Guide](./AI_AGENT_ONBOARDING.md)** — Studio-dense process guide (routines, convergence, policies). **Pack**, not community default narrative.
2a. **[Agent onboarding sequence (product)](../strategy/open-core/AGENT_ONBOARDING_SEQUENCE.md)** — Dual vector × SKUs.
2b. **[SKU onboarding surfaces](../strategy/open-core/SKU_ONBOARDING_SURFACES.md)** — What ships per `zqk` / admin / EE / organ roles.
3. **[Agent onboarding snapshot](./AGENT_ONBOARDING_SNAPSHOT.md)** — **Current** priority plan + active `convergence_session` pointers and commands to re-verify (refresh periodically; IDs are not magic).
4. **[Agent onboarding summaries digest](./AGENT_ONBOARDING_SUMMARIES_DIGEST.md)** — **Compressed history** of themes and plan/CVS evolution (March–April 2026) when dated snapshots lived alongside this README.
5. **[`archive/`](./archive/)** — Original **dated** `AGENT_ONBOARDING_SUMMARY_YYYY-MM-DD.md` files (audit trail; do not use as primary navigation).

**`doc_entry` IDs (snapshot + digest):** `DOC-1775197432882643000-89463915` (snapshot), `DOC-1775197433691153000-acbe23e3` (digest). `zqk object get <id>`.

**Process index:** `doc_entry` objects with `group=onboarding` — `zqk object list doc_entry --filter group=onboarding`. Archived summary paths under `docs/onboarding/archive/` match registered entries where applicable.

---

## Supplements

- **[Assessment & onboarding index](../architecture/ASSESSMENT_AND_ONBOARDING_INDEX.md)** — central doc map (alpha, metrics, onboarding); doc-graph backlog **`[REDACTED-ID]`** (**deferred**; see **`related_object_refs`** for **`GLS-1776207925199440000-aa236125`** + **`doc_entry`** ids); data-cell program on **`[REDACTED-ID]`** is the active persistence track for alpha
- **[Onboarding curriculum (YAML + seed job)](../../scripts/onboarding_roadmap/README.md)** — Priority plan, workstream, milestone, backlog items as **system objects**; *Reference pattern for advanced tutorials*; links [onboarding_roadmap_seed.yaml](../../scripts/scheduler_jobs/onboarding_roadmap_seed.yaml)
- **[Onboarding evaluation scenario](../process/testing/ONBOARDING_EVALUATION_SCENARIO.md)** — Isolated **`zqk-ts`** + **`ZQK_TS_TEST_ROOT`** runbook (P1 alpha gate); `make onboarding-eval-help`
- **[First-run object tutorial](./FIRST_RUN_OBJECT_TUTORIAL.md)** — `object template` → `create` → `get` → `update` (small `question` example; alpha journey B)
- [Efficient Data Processing](./EFFICIENT_DATA_PROCESSING.md) — Efficient data processing pipelines and optimization
- [Field State Tracking Principles](./FIELD_STATE_TRACKING_PRINCIPLES.md) — State-tracking fields (finite sets, conventions)
- **[Import Cycle Resolution](./IMPORT_CYCLE_RESOLUTION.md)** — **Required:** import cycles via architecture and abstraction layers
- [Refactoring & code quality](../process/refactoring/README.md) — Refactor status, [CODING_GUIDELINES](../process/refactoring/CODING_GUIDELINES.md), TODO triage
- [Agent onboarding assessment 2026-03-21](./AGENT_ONBOARDING_ASSESSMENT_2026-03-21.md) — One-off gap analysis (historical)

---

## Quick reference

- **Alpha readiness (repo root):** `make alpha-help` — bundle evidence, metrics capture, **`make onboarding-eval-help`**, pointers to [scripts/onboarding_roadmap/README.md](../../scripts/onboarding_roadmap/README.md) and [ONBOARDING_ROADMAP_AND_CERTIFICATION.md](../architecture/ONBOARDING_ROADMAP_AND_CERTIFICATION.md) (see [ALPHA_READINESS_SUMMARY.md](../process/testing/ALPHA_READINESS_SUMMARY.md) §8)
- **CLI:** `zqk` (stable; use **`zqk` / MCP** for process objects, not hand-edited YAML under `.zqk/process/`)
- **Project root:** Your clone root (directory containing `go.mod`)
- **Configuration:** `.zqk/config.yaml`
- **Strategic plan:** STRAT-PLAN-001 (3-year plan 2026–2028) — `zqk object get STRAT-PLAN-001`
- **MCP:** Exposes CLI commands via the CLI bridge
- **Cursor / workspace rules:** Often **stricter** than any single markdown file — see `.cursor/rules/`, **`docs/enforcement/AGENT_CONTEXT_REFRESH.md`** when present (**before implementing** reminders incl. **glossary**; **after logic is verified** → semantic density / §13). Runbook: **`scripts/README.md`**

---

## For new agents (session start)

1. Read **[COMMUNITY_FIRST_RUN.md](./COMMUNITY_FIRST_RUN.md)** and run `zqk system agent-onboard`.
2. Read **[AI_AGENT_ONBOARDING.md](./AI_AGENT_ONBOARDING.md)** when doing Studio process work.
3. Read **[AGENT_ONBOARDING_SNAPSHOT.md](./AGENT_ONBOARDING_SNAPSHOT.md)** and re-run its verification commands so IDs match **now**.
3. Before substantive code edits: **[PRE_CHANGE_CHECKLIST.md](../architecture/PRE_CHANGE_CHECKLIST.md)**; skim **`docs/enforcement/AGENT_CONTEXT_REFRESH.md`** for compliance status and reminders (**glossary** pointers + **after logic is verified**).

### Before merge or PR

4. **§13 post-verify** (semantic density): top callout in PRE_CHANGE_CHECKLIST and **[DRY_PATTERN_EXTRACTION.md](../best-practices/coding/DRY_PATTERN_EXTRACTION.md)** — dedupe scaffolding, rule of two / by the third.

```bash
zqk object list priority_plan --filter 'status=in_progress' --format table
zqk object list convergence_session --filter status=active --format table
zqk object get STRAT-PLAN-001
zqk system check --fast
```

---

*Last updated: 2026-04-03 — onboarding index tightened; dated summaries moved to `archive/` with digest.*
