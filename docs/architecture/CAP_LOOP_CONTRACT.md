# CAP loop contract

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

## Forbidden theater

- Manual `printf` / `EnsureCAPStage` to “unstick” CAP without journaled delivery
- Empty `cap_stage_receipt.json` without verified `artifact_ids`
- Treating open/`proposed` ATKs as dispatch delivery
- Ghost / `health_bridge` lines in `health.jsonl` to force review green
- Unsupervised overnight Cursor loops whose prompt is “advance CAP stages”

## Progress rule

A criterion (`CRIT-CAPH-*`) is done only with linked TEST evidence or an explicit `validation_method` check—not CAP stage churn.
