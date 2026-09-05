# Lifecycle shockwave kind index

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Status:** Active  
**Hub:** [LIFECYCLE_SHOCKWAVE_MAP.md](../LIFECYCLE_SHOCKWAVE_MAP.md)  
**Inventory date:** 2026-08-30 — 65 files in `docs/process/_internal/lifecycles/*_lifecycle.yaml`.

## Overview

One row per kernel lifecycle. Inherit the [family](./README.md); do not clone [kind_priority_plan.md](./kind_priority_plan.md).

| Shockwave column | Meaning |
|------------------|---------|
| `compiled_target` | Plane C matcher + E/F occupancy actually mutate this kind |
| `trigger` | Status save is a compiled catalyst for a target (BLI → PRI) |
| `exam_only` | Plane A may consult status; kinds without `status_reactive` no-op on the event |

`ods` = count of YAML `on_dependent_status` blocks. Compiled targets: `priority_plan`, `milestone`, `goal`, `criteria`.

SSOT for legal edges remains the lifecycle YAML + `pkg/objects/lifecycle_contract_test.go`.

## gantt_column

Family page: [family_gantt_column.md](./family_gantt_column.md)

| Kind | Spec extends | n | `active` role | Shockwave | ods | Specialty |
|------|--------------|---|---------------|-----------|-----|-----------|
| `priority_plan` | `work_interval` | 8 | `shovel_ready` | compiled_target | 7 | [kind_priority_plan.md](./kind_priority_plan.md) |

## gantt_lane

Family page: [family_gantt_lane.md](./family_gantt_lane.md)

| Kind | Spec extends | n | `active` role | Shockwave | ods | Specialty |
|------|--------------|---|---------------|-----------|-----|-----------|
| `evolution_management` | `base_object` | 5 | `shovel_ready` | exam_only | 0 | has `grooming` |
| `goal` | `work_interval` | 6 | `shovel_ready` | compiled_target | 1 | `active` is executing occupancy (no lock status); BLI `in_progress` activates `proposed` |
| `requirement` | `work_interval` | 6 | `shovel_ready` | exam_only | 0 | — |
| `roadmap` | `work_interval` | 5 | `shovel_ready` | exam_only | 0 | — |
| `strategic_plan` | `work_interval` | 5 | `shovel_ready` | exam_only | 0 | has `grooming` |
| `workstream` | `work_interval` | 5 | `shovel_ready` | exam_only | 0 | `paused → active` legal |

## work_unit

Family page: [family_work_unit.md](./family_work_unit.md)

| Kind | Spec extends | n | `active` role | Shockwave | ods | Specialty |
|------|--------------|---|---------------|-----------|-----|-----------|
| `agent_instruction` | `base_object` | 7 | `—` | exam_only | 0 | — |
| `agent_onboarding_preparation` | `base_object` | 6 | `shovel_ready` | exam_only | 0 | — |
| `agent_skill` | `base_object` | 6 | `—` | exam_only | 0 | — |
| `agent_task` | `work_unit` | 7 | `—` | exam_only | 0 | hourglass / feed, not Plane C |
| `backlog_item` | `work_unit` | 9 | `—` | trigger | 0 | catalyst stubs (`priority_plan_ref`, `milestone_refs`, `goal_refs`, `criteria_refs`) |
| `base_object` | `auditable` | 6 | `—` | exam_only | 0 | template machine; not a product instance |
| `milestone` | `work_unit` | 6 | `—` | compiled_target | 1 | BLI `in_progress` locks `not_started` → `in_progress` |
| `persona` | `base_object` | 6 | `—` | exam_only | 0 | CAP identity |
| `risk_blocker` | `base_object` | 6 | `—` | exam_only | 0 | — |
| `technical_debt` | `work_unit` | 8 | `—` | exam_only | 0 | — |
| `test_case` | `work_interval` | 6 | `shovel_ready` | exam_only | 0 | work_interval clocks |

## membrane

Family page: [family_membrane.md](./family_membrane.md)

| Kind | Spec extends | n | `active` role | Shockwave | ods | Specialty |
|------|--------------|---|---------------|-----------|-----|-----------|
| `policy` | `base_object` | 6 | `enforced` | exam_only | 0 | enforced `active` |
| `role` | `base_object` | 4 | `enforced` | exam_only | 0 | enforced `active` |

## execution_session

Family page: [family_execution_session.md](./family_execution_session.md)

| Kind | Spec extends | n | `active` role | Shockwave | ods | Specialty |
|------|--------------|---|---------------|-----------|-----|-----------|
| `convergence_session` | `work_interval` | 8 | `execution_locked` | exam_only | 0 | `active` = locked; phases C1–C6 |
| `mcp_session` | `base_object` | 4 | `—` | exam_only | 0 | MCP connection |
| `scheduler_health_metric` | `base_metric` | 3 | `execution_locked` | exam_only | 0 | observation window |
| `scheduler_job` | `base_object` | 5 | `execution_locked` | exam_only | 0 | runtime delta occupancy |
| `zqk_session` | `base_object` | 4 | `execution_locked` | exam_only | 0 | seat lifetime |

## predicate

Family page: [family_predicate.md](./family_predicate.md)

| Kind | Spec extends | n | `active` role | Shockwave | ods | Specialty |
|------|--------------|---|---------------|-----------|-----|-----------|
| `criteria` | `base_object` | 7 | `—` | compiled_target | 1 | Parent BLI `in_progress` locks shovel-ready CRITs; Plane J designed |
| `question` | `base_object` | 5 | `—` | exam_only | 0 | — |
| `verification_matrix` | `base_object` | 3 | `shovel_ready` | exam_only | 0 | — |

## record

Family page: [family_record.md](./family_record.md)

| Kind | Spec extends | n | `active` role | Shockwave | ods | Specialty |
|------|--------------|---|---------------|-----------|-----|-----------|
| `audit_aggregation_metric` | `base_metric` | 3 | `—` | exam_only | 0 | — |
| `audit_event_aggregation` | `base_object` | 5 | `—` | exam_only | 0 | — |
| `audit_event` | `base_object` | 6 | `—` | exam_only | 0 | — |
| `change_journal_entry` | `base_object` | 7 | `—` | exam_only | 0 | — |
| `code_quality_metric` | `base_metric` | 5 | `—` | exam_only | 0 | — |
| `component` | `extensible_object` | 6 | `—` | exam_only | 0 | — |
| `display` | `extensible_object` | 6 | `—` | exam_only | 0 | — |
| `import_tracking` | `base_object` | 5 | `—` | exam_only | 0 | — |
| `qa_success` | `null` (stub) | 3 | `—` | exam_only | 0 | signed QAS token; spec exists, `fields: {}` |

## default_shovel

Family page: [family_default_shovel.md](./family_default_shovel.md)

| Kind | Spec extends | n | `active` role | Shockwave | ods | Specialty |
|------|--------------|---|---------------|-----------|-----|-----------|
| `account` | `base_object` | 3 | `shovel_ready` | exam_only | 0 | — |
| `agent_architecture` | `base_object` | 5 | `terminal` | exam_only | 0 | — |
| `agent_feed` | `base_object` | 3 | `shovel_ready` | exam_only | 0 | — |
| `auth_strategy` | `base_object` | 4 | `shovel_ready` | exam_only | 0 | — |
| `bucketing_strategy` | `auditable` | 3 | `shovel_ready` | exam_only | 0 | — |
| `decision` | `base_object` | 6 | `shovel_ready` | exam_only | 0 | — |
| `division` | `extensible_object` | 3 | `shovel_ready` | exam_only | 0 | — |
| `doc_entry` | `base_object` | 7 | `shovel_ready` | exam_only | 0 | — |
| `domain_registry` | `base_object` | 3 | `shovel_ready` | exam_only | 0 | — |
| `glossary_term` | `base_object` | 3 | `shovel_ready` | exam_only | 0 | — |
| `glossary_term_relation` | `base_object` | 3 | `shovel_ready` | exam_only | 0 | — |
| `important_date` | `base_object` | 5 | `shovel_ready` | exam_only | 0 | — |
| `keystore_entry` | `base_object` | 4 | `shovel_ready` | exam_only | 0 | — |
| `kind_synonym` | `null` | 3 | `shovel_ready` | exam_only | 0 | — |
| `mcp_built_in_tool` | `base_object` | 6 | `shovel_ready` | exam_only | 0 | — |
| `mcp_spec` | `base_object` | 5 | `shovel_ready` | exam_only | 0 | — |
| `mission` | `base_object` | 4 | `shovel_ready` | exam_only | 0 | — |
| `namespace` | `base_object` | 3 | `shovel_ready` | exam_only | 0 | — |
| `namespace_registry` | `base_object` | 3 | `shovel_ready` | exam_only | 0 | — |
| `organization` | `extensible_object` | 3 | `shovel_ready` | exam_only | 0 | — |
| `prompt_template` | `base_object` | 5 | `shovel_ready` | exam_only | 0 | — |
| `stakeholder_profile` | `base_object` | 4 | `shovel_ready` | exam_only | 0 | — |
| `strategic_context` | `base_object` | 4 | `shovel_ready` | exam_only | 0 | — |
| `tde_envelope` | `base_object` | 4 | `—` | exam_only | 0 | — |
| `vision` | `base_object` | 4 | `shovel_ready` | exam_only | 0 | — |
| `vocabulary_scheme` | `base_object` | 3 | `shovel_ready` | exam_only | 0 | — |
| `workflow` | `base_object` | 5 | `shovel_ready` | exam_only | 0 | — |
| `workstream_transition` | `base_object` | 5 | `shovel_ready` | exam_only | 0 | — |

## How to add or move a kind

1. Assign a family from occupancy of `active` / `in_progress` (hub §3), not from spec `extends` alone.
2. Add the row here. Put Q5 specialty on the family page (one bullet).
3. Write `kind_<ontology>.md` only when remaining edges need a hop table.
4. If you add `on_dependent_status`, update `strictAutoTriggerKinds` or the completion-rollup allowlist and add a contract test.

