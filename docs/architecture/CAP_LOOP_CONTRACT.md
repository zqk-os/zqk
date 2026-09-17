# CAP loop contract

**Last Verified:** 2026-08-31

**Status:** Binding for CAP honesty (`GOAL-CAPH-001` / `REQ-CAPH-001`–`004`).  
**Related:** [CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md](./CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md), [CONVERGENCE_PREDICATES_AND_GATES.md](./CONVERGENCE_PREDICATES_AND_GATES.md), [EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md](./EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md), traceability bundle `cap-loop-honesty-traceability-bundle`.

## What is the CAP loop?

**`SCH-cap-orchestrator`** is the CAP loop. It peeks `whats-next` / `cap_cycle`, runs the stage handler, and advances **only** via `maybeAdvanceCAPStage` after verified delivery evidence, writing `.zqk/state/cap_advance_journal.jsonl`.

Cursor `AGENT_LOOP_TICK_*` wakes are **not** CAP. They are optional human-agent wakes and must follow [`scripts/cap-agent-wake-contract.txt`](../../scripts/cap-agent-wake-contract.txt).

## Outcome contract: parent `convergence_session`

Ground truth for “did this tick achieve anything?” is the **bound parent `CVS-*`**, not the `cap_cycle` label.

- Pending state carries `cvs_id` and optional `focus_child_cvs_id`.
- Unbound parent → **hold** (no advance).
- Bundle-only `ready_for_session_completion` ≠ parent `ready_for_parent_completion` / `desired_end_state`.

## Nested CVS (trivial veneer)

| Command | Purpose |
|---------|---------|
| `zqk scheduler convergence nest-spawn --parent-session-id CVS-… --title … --hypothesis … --desired-end-state …` | Create child + append parent `related_object_refs` |
| `zqk scheduler convergence nest-link --parent-session-id … --child-session-id …` | Link existing child |
| `zqk scheduler convergence nest-status --parent-session-id … --format json` | BFS tree (`CollectCVSTreeBFS`, max depth 3) |

Do not hand-stitch `related_object_refs` or invent parallel nesting.

## Automatable facets (directive feeder — not CAP ping)

**Posture:** `DEC-REDACTED` / `GLS-1786415578038417000-8ae29e70` — CAP ping is compensatory scaffolding. The durable fix is the kernel **feeding** seats from continuous ambient **metrics**, not agents remembering to self-tick.

**Metrics vs verification (do not conflate):**

| Plane | What it is | Examples |
|-------|------------|----------|
| **Metrics (primary for feeder)** | `base_metric` + extensions; continuous collect/aggregate/summarize | `command_metric` timings/rates, `scheduler_health_metric`, `audit_aggregation_metric`, `file_lock_metric`, `zqk system metrics` / `--summary` |
| **Verification/evidence (secondary)** | CVS / test-bundle health | fingerprints, `health.jsonl`, `convergence measure` delta — **not** metrics (`GLS-1786416152350033000-cf1e5999`) |

**Glossary / REQ:** `GLS-1786416188034709000-292822ac` (metrics-plane feeder facets M1–M7), `REQ-REDACTED`.

| Facet | What to compute (metrics plane) | Where it must land |
|-------|----------------------------------|--------------------|
| M1 | `command_metric` / `system metrics` timing, failure, timeout outliers | TPM `agent_feed` digest |
| M2 | `scheduler_health_metric` + dispatch/concurrency pressure | Same digest |
| M3 | Other `base_metric` children rollups (audit/validation/cache/file-lock/…) | Same digest |
| M4 | Metrics `--summary` churn / slow / fail filters | Same digest |
| M5 | Align score + PRI `active_order` deltas | Same digest |
| M6 | Correspondence hourglass debt | Same digest |
| M7 | One ranked `next_admin_action` from M1–M6 | Same digest — **the obvious next thing** |

Optional appendix only: CVS `measure_compressed` (labeled verification, not metrics).

**Gap (today):** `cap_stage_metrics` / `executeMetricsStage` writes `.zqk/state/cap_metrics_latest.json` + object-count/`workflow convergence` — **not** a metrics-plane digest into `agent_feed`. Ambient/`whats-next` under-projects command/base metrics; seats still go idle.

**Implement slices:** `BLI-REDACTED` (metrics-plane→feed), `BLI-REDACTED` (idle/wait → metrics digest), `BLI-REDACTED` (unify ambience projector with metrics vocabulary).

Idle/`wait` CAP paths must still refresh M7 from metrics — never an empty CAP ping.

## Forbidden theater

- Manual `printf` / `EnsureCAPStage` to “unstick” CAP without journaled delivery
- Empty `cap_stage_receipt.json` without verified `artifact_ids`
- Treating open/`proposed` ATKs as dispatch delivery
- Ghost / `health_bridge` lines in `health.jsonl` to force review green
- Unsupervised overnight Cursor loops whose prompt is “advance CAP stages”
- Treating CAP ping / anti-idle wakes as the product control plane (use directive feeder instead)

## Stage prompt templates (posture every lap)

Each `CapStages` tick re-imprints posture via kernel `prompt_template` objects
(`pkg/agentprompt.BuildCapStageWakePrompt`).

| Stage | Template id |
|-------|-------------|
| *(all)* | `PROMPT-CAP-STAGE-PREAMBLE` |
| `cap_stage_planning` | `PROMPT-CAP-STAGE-PLANNING` |
| `cap_stage_design` | `PROMPT-CAP-STAGE-DESIGN` |
| `cap_stage_grooming` | `PROMPT-CAP-STAGE-GROOMING` |
| `cap_stage_orchestrating` | `PROMPT-CAP-STAGE-ORCHESTRATING` |
| `cap_stage_review` | `PROMPT-CAP-STAGE-REVIEW` |
| `cap_stage_metrics` | `PROMPT-CAP-STAGE-METRICS` |
| `cap_stage_self_improvement` | `PROMPT-CAP-STAGE-SELF-IMPROVEMENT` |
| `cap_stage_sentinel` | `PROMPT-CAP-STAGE-SENTINEL` (+ `BuildSentinelPrompt` / `PROMPT-1783091834904015000-066e0f7d`) |

TRACK: `BLI-REDACTED` / `REQ-CAP-STAGE-PROMPTS-001` / `CRIT-CAP-STAGE-PROMPTS-001`.

## Merge-up + branch yard (swarm cleanup)

Doctrine: `docs/architecture/BRANCHING_STRATEGY.md`, orchestration skill, `POL-CODE-1784784370706305000-e3e7245a`.

1. TPM provisions **one** plan trunk: `integration/pri-<plan_id lowercase>`.
2. ATK worktrees (`paths.AgentWorktreeDir` — off-project; `POL-AGENT-WORKTREE-ISOLATION-001`) merge **into that trunk** before ATK/BLI complete.
3. Then tear down worktree + delete `agent/ATK-*`. Orphans → `SCH-IDLE-WORKTREE-CLEANUP` or `./scripts/cleanup-agent-worktrees.sh`.
4. CAP preamble §8 + stage MERGE-UP lines restate this every lap.

TRACK: `BLI-ATK-MERGE-UP-HYGIENE-001` / `REQ-ATK-MERGE-UP-001` / `CRIT-ATK-MERGE-UP-001`.

## Progress rule

A criterion (`CRIT-CAPH-*`) is done only with linked TEST evidence or an explicit `validation_method` check—not CAP stage churn.

**VDS:** CAP consumes [Verifiable Decomposition Spine](./VERIFIABLE_DECOMPOSITION_SPINE.md) (`POL-WORKFLOW-VDS`). Stage advance without independently verified chunks (or a human waiver) is forbidden theater.
