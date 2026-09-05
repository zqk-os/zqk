# Family: default_shovel

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Status:** Active  
**Hub:** [LIFECYCLE_SHOCKWAVE_MAP.md](../LIFECYCLE_SHOCKWAVE_MAP.md)

## Overview

Thin machines whose `active` is **`shovel_ready`** and which are **not** Gantt columns, lanes, membranes, or sessions: identity registries, north-star statements, tooling specs, workflow contracts, and similar.

Default planes: **A + B** only. No compiled shockwave. `paused → active` (when present) is halt → pickup and is **legal**. Do not apply the PRI check valve.

`active` still does **not** mean “enforced policy” or “CVS in flight.” It means “this object is the live accepted record of its kind” (vision accepted, namespace live, MCP spec published).

## Occupancy (typical 3–5 statuses)

| Value | Role |
|-------|------|
| `draft` / `planned` / `proposed` | `realign` |
| `active` | `shovel_ready` |
| `paused` / `error` | `halted` |
| `archived` / `complete` | `terminal` |

## Shockwave

Exam-only. Other kinds may require “`workflow` exists and `enabled`” (PRI already binds `PrecondWorkflowConstraintsIfSet`) — that is Plane A on the **referrer**, not a subscriber on workflow.

## Members

account, agent_architecture, agent_feed, auth_strategy, bucketing_strategy, decision, division, doc_entry, domain_registry, glossary_term, glossary_term_relation, important_date, keystore_entry, kind_synonym, mcp_built_in_tool, mcp_spec, mission, namespace, namespace_registry, organization, prompt_template, stakeholder_profile, strategic_context, tde_envelope, vision, vocabulary_scheme, workflow, workstream_transition.

## Kind specialty (Q5 only — cluster, not per-file)

| Cluster | Specialty |
|---------|-----------|
| North star (`vision`, `mission`, `strategic_context`) | Acceptance exam; not a Gantt seal. Linked via `related_object_refs` / ranking, not PRI `on_dependent_status`. |
| Registry (`namespace*`, `domain_registry`, `organization`, `division`, `account`) | Identity membrane for other objects’ create exams. |
| Vocabulary (`glossary_term`, `glossary_term_relation`, `vocabulary_scheme`, `kind_synonym`) | Navigation graph; counts in VOCABULARY_GRAPH.md. |
| Tooling (`mcp_spec`, `mcp_built_in_tool`, `prompt_template`, `auth_strategy`, `bucketing_strategy`, `keystore_entry`) | Publish/enable; `agent_feed` shovel-ready means the bus is addressable, not that a plan is sealed. |
| `workflow` | Referenced by PRI activation constraints; unset ref is vacuous. |
| `decision` / `doc_entry` / `important_date` / `stakeholder_profile` / `tde_envelope` / `workstream_transition` / `agent_architecture` | Thin records with shovel-ready live tokens. Promote `agent_architecture` `active` is terminal in inventory — treat that kind’s YAML as SSOT if it diverges. |

If a kind in this list grows a check valve or `on_dependent_status`, **move it** to the matching family and add a contract test. Do not grow this page into 28 mini-exams.
