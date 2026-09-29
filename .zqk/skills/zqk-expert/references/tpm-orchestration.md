# TPM orchestration plane (Gantt matrix + branch collaboration)

Canonical operating geometry for program managers and primary orchestrators.
When this file disagrees with live `POL-*` / CLI validation, **trust the kernel**.

## Matrix geometry

```text
ROADMAP (product / system frame — title over the Gantt)
└── workstream_refs + timeline → chart viewport
    │
    │  columns ≈ priority_plans (feature / functional bundles)
    │  rows    ≈ workstreams (lane / flavor of work — long-lived)
    │
    ├── WS-backend ───── PRI-Sprint-A (BE) ─── PRI-Sprint-B …
    ├── WS-frontend ──── PRI-Sprint-A (FE) ─── …
    └── WS-docs-design ─ PRI-Sprint-A (docs/arch) …
```

| Plane | Role | Notes |
|-------|------|--------|
| **Roadmap** | Frame around the Gantt — product/system summary | Spec: `workstream_refs` + `timeline_*`; hosts the collective embodiment of plans, WS, BLIs, goals, reqs, criteria, tests, ATKs |
| **Workstream** | Horizontal lane — like/complementary activities | Reporting lanes; soft lifetime (can stay active indefinitely); e.g. backend+infra, frontend, docs-n-design |
| **Priority plan** | Column — feature/functional bundle | May span multiple workstreams via `workstream_refs[]`; does not have to |
| **Milestone** | Time anchors on the Gantt | Linked from workstreams / plans / BLIs |
| **BLI → ATK / swarm** | Cells under a locked plan | Execute the frozen set; do not expand mid-flight |

Plans reference workstreams whose flavor matches the work (frontend BLIs → frontend WS). Not mandatory for every object, but the intended design for TPM reporting and conflict separation.

## 3-plan runway (always)

Policy: **`POL-AGENT-THREE-PLAN-RUNWAY-001`**. Protocol: **`WFL-AGENT-THREE-PLAN-RUNWAY-001`**.

Keep **three** live `priority_plan` columns with real BLIs:

1. **One execution-facing lead** (`active` / `in_progress`) with open children **or** explicit ship-exit. Hand this column to a primary; hourglass-steer; do not orch it from chat.
2. **≥2 grooming columns** that TPM is shaping for the next lock. Workers do not poach these.

If the count drops below three, TPM work is **objectify from undelivered REQ/CRIT** onto an unlocked column — not persist `align-latest.json`, not remint `ORCHESTRATE_PLAN`, not stuffing exploring BLIs onto the sealed lead (`POL-AGENT-TPM-GROOM-AHEAD-001`).

## Scope lock (definitive start/stop)

Validation refuses draft-plane scope creep onto execution-facing plans (`active` / `in_progress`) — e.g. linking `exploring` BLIs (**Scope Creep Protection** / plan membership gates).

**TPM duty before lock:** fill the column (shovel-ready BLIs, milestones, workstream refs).  
**After lock:** primaries execute the sealed set to completion. Missing mid-sprint work → **new plan column** or strategic replan — never stuff the locked plan.

## Process administration only (2026-08-10)

## Process administration only (2026-08-10)

Policy: **`POL-AGENT-TPM-PROCESS-ADMIN-001`**.

TPM altitude is **process admin**, not whats-next execution:

1. `zqk system align` (+ persist `.zqk/state/ambient/align-latest.json`) so priorities stay tied to goals/mission/vision/strategic_plan.
2. Maintain `priority_plan.active_order` (lower wins on `whats-next`).
3. Groom shovel-ready columns; hourglass-steer peers to **self-serve** `whats-next --agent-id <seat>`.
4. **Do not** `agent orchestrate` peer-owned PRIs or absorb work via local Ollama sync-loops.

Peers execute the system-proposed plan and scale subagents (`WFL-SUBAGENT-DISPATCH`).


## Plan ownership (not ATK babysitting)

1. Strategic ambiguity / conflicting priorities → strategic planning meeting → `strategic_plan` / roadmap rewrite.
2. TPM decomposes to shovel-ready `priority_plan`s (+ workstream assignment).
3. Hand each plan to a **primary** → `zqk agent orchestrate <PRI>` / subagent swarm **to plan completion**.
4. Parallel plans only when workstreams keep lanes non-colliding.
5. Hourglass (`POL-AGENT-ORCH-HOURGLASS-001`, `WFL-TPM-AGY-MESH-001`) = handoff insurance — not a license to micro-mint ATKs as a conveyor.

**Primary altitude:** `.zqk/skills/zqk-orchestration/SKILL.md` — assigned plan + workstream lanes (including dedicated tech-debt vs project lanes). Primaries sequence *within* the sealed plan; they do not rewrite the roadmap Gantt.

## Branch collaboration (one plan → one integration branch)

Policy: **`POL-CODE-1784784370706305000-e3e7245a`** (Strict TPM Branch Collaboration). Mega-branch pattern: onboarding § branching mandate.

1. **TPM provisions** one collaboration / feature branch per plan (pre-PR trunk; e.g. `integration/pri-…`). Agents must not invent ad-hoc integration branches.
2. **Workers** operate in isolated git worktrees / agent branches **off that plan branch** — never race `main` or steal peers' tips.
3. **Merge up** into the plan branch when pedantic/build/policy gates pass.
4. **TPM opens the single PR** to `main` — not one PR per agent.
5. **Ship-exit (mandatory per column):** local `./scripts/pre-commit-policy.sh` + `./scripts/pre-commit-lint.sh` → targeted TDD (`go test -timeout ≤60s` probes + `zqk scheduler scan-tests --package …` with job id/log) → commit (agent identity) → push → PR → **automerge when criteria met** (`POL-CODE-1784784979738408000-233b96e3` local validation green; optional `[ZQK_AGENT_STAMP]` / `.github/workflows/auto-merge.yml`; else `gh pr merge --squash` after gates). Objectify claims when process objects ship.
6. **No idle:** hourglass on directed steers; while waiting, advance the next health/capability action. Peers self-serve `whats-next`; TPM process-admin only (`POL-AGENT-TPM-PROCESS-ADMIN-001`).

Value signal: swarm builds significant momentum without random blockers from poor process orchestration (branch fights, open-ended plan scope, TPM ATK babysitting).

## Kernel / docs pointers

- Specs: `.zqk/specs/objects/{roadmap,workstream,priority_plan,backlog_item}.yaml`
- Membership / immutability: `pkg/validation/priority_plan_membership.go`, `pkg/kernelcas/compose` (Scope Creep Protection)
- Mesh / hourglass: `.cursor/rules/tpm-agy-mesh-wake.mdc`, `WFL-TPM-AGY-MESH-001`
- Strategic meetings: `docs/architecture/STRATEGIC_PLANNING_MEETING_WIRING.md`
- Onboarding mega-branch: `docs/onboarding/AI_AGENT_ONBOARDING.md` (BRANCHING MANDATE)
