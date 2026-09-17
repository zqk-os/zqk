# Graph edge ownership (acyclic typed refs)

**Last Verified:** 2026-08-31


**Status:** Design contract + runtime `edge_role` (2026-08-20). Typed A↔B mutual pairs are retired. SELF / untyped `related_object_refs` remain on `BLI-KERNEL-REF-GRAPH-ACYCLIC-001`. Do **not** mint onto `PRI-CEF-R9-MEASURE-001`.  
**TRACK:** `BLI-KERNEL-REF-GRAPH-ACYCLIC-001` / `REQ-KERNEL-REF-GRAPH-ACYCLIC-001`  
**Column:** `PRI-KERNEL-WORK-ENVELOPE-001` (`in_progress`)  
**Audit script:** `python3 scripts/audit-spec-ref-cycles.py`  
**Precedent:** `priority_plan` membership is child-owned (`backlog_item.priority_plan_ref`); `pri_no_backlog_item_refs` / `pri_no_backlog_related_refs`. Criteria↔backlog already: `backlog_item.criteria_refs` only (`BLI-CRITERIA-OWNERSHIP-001`).

## Verdict

A typed `*_ref` / `*_refs` field is a **directed graph edge**. Two kinds must not both store the same relationship. That is a cycle in the spec, a dual-write bug in instances, and a create-order hack in scenario apply (`requirement <-> criteria` was literally named in `pkg/scenario/create_order.go`).

`related_object_refs` on `base_object` is an **untyped dumping ground**. It inherits onto every kind and will recreate any cycle the typed fields just forbade (example: `PRI.related_object_refs` containing `BLI-*` while `BLI.priority_plan_ref` points at the plan). Do **not** delete it in this cut — CVS nesting and many live objects use it — but it must stop being a free-for-all.

## Ownership rule (one direction)

| Pattern | Who stores the edge | Reverse |
|---------|---------------------|---------|
| **Child → parent** (preferred for membership / “belongs to”) | Child field, e.g. `priority_plan_ref` | Query `filter field=parent-id`. Parent **must not** store children. |
| **Parent → child** (composition / “completion set”) | Parent field, e.g. `requirement.criteria_refs` | Reverse index. Child **must not** store the parent list. |
| **Self / tree** | One parent pointer **or** an ordered child list — not both | `parent_ref` XOR `child_refs`. DAG flags on remaining self-lists. |
| **Untyped associate** | Forbidden as a substitute for a typed edge | Per-kind prefix refuse until the field is lifted off `base_object`. |

Pick **one** column per pair. Kernel DNA for *membership* is child→parent (`pplan add`). Kernel DNA for *acceptance criteria* is parent→child (`*.criteria_refs`), already documented on `criteria.yaml` for backlog.

## Edge role (first-class traverser)

`checklist.criticality: composition | association` is field-vetting metadata. It is **not** the graph role. Runtime query, shockwave, and agents consult **`edge_role`** on the spec field (sibling of `semantic_type`):

| Role | Stored on | Walk |
|------|-----------|------|
| `membership` | child (`priority_plan_ref`, `criteria.goal_refs`) | Parent-start uses reverse index / `filter field=parent-id`. Child-start is outbound. |
| `composition` | parent (`*.criteria_refs`) | Parent-start is outbound. Child-start uses reverse index. |
| `associate` | untyped / secondary (`related_object_refs`) | Do not treat as occupancy or completion. |

Code: `objects.EdgeRole*` / `EdgeRoleForKindField` / `ShockwavePolicy.HopRole`. `GetRelated` / `GetPath` walk **both** stored outbound edges and reverse-index inbound hops, filtered by role name or field name. Dependency-ref events stamp `edge_role` + `field` on outbound hops.

Do **not** dual-write the pair to make both walks work. The reverse index is still `target → []dependents` on disk; role is resolved from the dependent's field when walking.

DNA annotated this cut: `backlog_item.priority_plan_ref` (membership), `*.criteria_refs` (composition), `base_object.related_object_refs` (associate), `criteria.goal_refs` (membership). Unannotated `*_ref` fields default to `associate` until DNA or spec `edge_role` is set.

## This cut: criteria composition

`criteria.yaml` already says parents link via `criteria_refs` and backlog reverse links are derived. The cycle was that **requirement** and **milestone** still had the reverse fields on criteria:

| Keep (SSOT) | Remove (reverse) |
|-------------|------------------|
| `requirement.criteria_refs` | `criteria.requirement_refs` |
| `milestone.criteria_refs` | `criteria.milestone_refs` |
| `backlog_item.criteria_refs` | (already gone) |
| `release.criteria_refs` / `test_case.criteria_refs` | no reverse today |

`criteria.goal_refs` stays: `goal` has no `criteria_refs` (one-way child→parent).

Compose overlays: `crit_no_requirement_refs`, `crit_no_milestone_refs` (`OpRefuseFieldPresent`), same shape as `pri_no_backlog_item_refs`. Scenario apply writes only the parent `criteria_refs` (no deferred reverse on criteria).

## `related_object_refs` — constrain, then lift

**Do not** keep “any ID, any kind” on `base_object` as the long-term graph. It cannot express target-kind constraints, so agents dump whatever is nearby and close cycles.

Target:

1. **Now:** per-kind prefix refuses where a typed edge already exists (`PRI` already refuses `BLI-`; `requirement.related_object_refs` refuses `CRIT-*`; `criteria.related_object_refs` refuses `REQ-*`).
2. **Next:** lift `related_object_refs` off `base_object` onto an opt-in mixin (`associable`) whose spec lists **allowed target kinds** (or allowed ID prefixes). Kinds that only need typed refs omit the mixin.
3. **CVS nesting** today appends children on the parent’s `related_object_refs`. Replace with a typed `child_session_refs` (parent→child composition) before deleting the generic field. TRACK on `BLI-KERNEL-REF-GRAPH-ACYCLIC-001`.

## Mutual typed pairs (audit 2026-08-20)

Re-run `python3 scripts/audit-spec-ref-cycles.py`. Snapshot of **MUTUAL** (two different kinds, both store the edge). **Owner** is the field that remains after this program; **Drop** is the reverse.

| Pair | Owner (keep) | Drop (forbid) | Notes |
|------|----------------|---------------|--------|
| `requirement.criteria_refs` ↔ `criteria.requirement_refs` | parent `criteria_refs` | `criteria.requirement_refs` | **This cut** |
| `milestone.criteria_refs` ↔ `criteria.milestone_refs` | parent `criteria_refs` | `criteria.milestone_refs` | **This cut** |
| `requirement.backlog_item_refs` ↔ `backlog_item.requirement_refs` | child `requirement_refs` | `requirement.backlog_item_refs` | **This cut** (same as PRI membership) |
| `goal.backlog_item_refs` ↔ `backlog_item.goal_refs` | child `goal_refs` | `goal.backlog_item_refs` | **This cut** |
| `milestone.backlog_item_refs` ↔ `backlog_item.milestone_refs` | child `milestone_refs` | `milestone.backlog_item_refs` | **This cut** |
| `goal.requirement_refs` ↔ `requirement.goal_refs` | child `goal_refs` | `goal.requirement_refs` | **This cut** |
| `goal.milestone_refs` ↔ `milestone.goal_refs` | child `goal_refs` | `goal.milestone_refs` | **This cut** (occupancy, not Gantt composition) |
| `milestone.requirement_refs` ↔ `requirement.milestone_refs` | child `milestone_refs` | `milestone.requirement_refs` | **This cut** |
| `requirement.test_case_refs` ↔ `test_case.requirement_refs` | child `requirement_refs` | `requirement.test_case_refs` | **This cut** |
| `requirement.technical_spec_refs` ↔ `technical_spec.requirement_refs` | child `requirement_refs` | `requirement.technical_spec_refs` | **This cut** |
| `requirement.workstream_refs` ↔ `workstream.requirement_refs` | child `workstream_refs` | `workstream.requirement_refs` | **This cut** |
| `milestone.workstream_refs` ↔ `workstream.milestone_refs` | child `workstream_refs` | `workstream.milestone_refs` | **This cut** |
| `agent_task.pipeline_ref` ↔ `pipeline.agent_task_refs` | child `pipeline_ref` | `pipeline.agent_task_refs` | **This cut** |
| `backlog_item.convergence_session_ref` ↔ `convergence_session.backlog_item_refs` | child `convergence_session_ref` | `convergence_session.backlog_item_refs` | **This cut** |
| `component.display_ref` ↔ `display.component_refs` | child `display_ref` | `display.component_refs` | **This cut** |
| `team.department_ref` ↔ `department.team_refs` | child `department_ref` | `department.team_refs` | **This cut** |
| `team.division_ref` ↔ `division.team_refs` | child `division_ref` | `division.team_refs` | **This cut** |
| `organizational_change.impact_analysis_refs` ↔ `impact_analysis.change_ref` | child `change_ref` | `organizational_change.impact_analysis_refs` | **This cut** |
| `partnership.organization_refs` ↔ `organization.partnership_refs` | parent `organization_refs` | `organization.partnership_refs` | **This cut** (partnership members are composition) |
| `persona.mission_refs` ↔ `mission.persona_refs` | child `mission_refs` | `mission.persona_refs` | **This cut** |

**SELF** pairs (same kind both ways, or list+parent): trees use child→parent membership — `component.parent_component_refs` / `division.parent_division_ref`; dropped reverse child lists. Remaining self-lists (`decision.decision_refs`, `glossary_term.alias_refs`, `question.related_question_refs`, `scenario.scenario_refs`, `workstream.workstream_refs`, `zqk_session.parent_session_ref`, `milestone.prerequisite_refs` / `blocked_by_refs`) keep DAG flags: refuse ID==self, refuse 2-cycles.

**One-way typed** (~111) are already DAGs at the spec layer. Leave them.

**Untyped** includes `base_object.related_object_refs` plus account/commit/lifecycle/metric stems that are not kernel kinds — those are not A↔B ontology cycles; they still must not duplicate a typed edge.

## Reading `grep '*_ref[s]?:'` on object_specs

That grep is the human inventory (~200 hits). Do **not** treat every match as a typed A↔B edge. Classify:

| Class | What to do |
|-------|------------|
| **False positive** | Spec metadata, not a kernel object edge. Examples: `qa_success.yaml` `lifecycle_ref: qa_success_lifecycle` (lifecycle file stem); `tde_envelope.capability_refs` (capability tokens like `fs:delete`, not CAP-* object ids). |
| **Removed this cut** | `criteria.requirement_refs`, `criteria.milestone_refs` — gone from the spec; parent `criteria_refs` is SSOT. |
| **MUTUAL** | The pairs in the table — all typed A↔B owners are picked. Do not add a new mutual pair. |
| **SELF** | Trees: parent pointer **or** child list. Keep DAG flags (no self-id, no 2-cycles). |
| **One-way typed** | Keep. Child→parent membership and parent→child composition as in the ownership rule. |
| **Untyped stem** | `related_object_refs`, `commit_refs`, `owner_ref`, `object_ref`, … — constrain prefixes or lift to a typed field; do not use as a second store for a typed pair. |

Machine check: `python3 scripts/audit-spec-ref-cycles.py` (fail if `mutual` grows).

## Agent / spec checklist

When adding a `*_ref` / `*_refs` field:

1. Name the **owner** kind, the **target** kind, and the **`edge_role`** (`membership` | `composition` | `associate`). Do not infer role from `checklist.criticality`.
2. Grep the target spec for the reverse field. If it exists, stop — extend this table, do not add the second edge.
3. Never put a typed relationship only in `related_object_refs`.
4. Re-run `python3 scripts/audit-spec-ref-cycles.py` and fail if `mutual` grows.

PRE_CHANGE_CHECKLIST §3a / this file. Compose `OpRefuseFieldPresent` / `OpRefuseRefPrefix` is the fail-closed twin of the spec delete (same as PRI membership).
