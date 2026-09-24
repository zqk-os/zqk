# Handoff schema (tool-agnostic)

**cef_version:** 0.1.0  

After Wave 4, package results for a **downstream process engineer / expert system** (ZQK swarm objectify, Jira importer, plain markdown program, etc.). CEF does **not** require ZQK.

---

## Package layout

```
cef-run-<timestamp>/
  run_scope.yaml
  scorecard.json
  findings.jsonl              # post-adversarial, integrator-accepted
  adversarial_resolutions.jsonl
  diagrams/**
  tooling_gaps.md
  run_log.md
  handoff_manifest.json
```

## `handoff_manifest.json`

```json
{
  "cef_version": "0.1.0",
  "mode": "truth_map",
  "repo_fingerprint": "git sha or archive hash",
  "accepted_finding_ids": ["F-..."],
  "waived": [{"finding_id": "F-...", "reason": "...", "by": "..."}],
  "suggested_program_shape": {
    "missions": [],
    "workstreams": [],
    "goals": [],
    "milestones": [],
    "priority_plans": [],
    "requirements": [],
    "criteria": [],
    "test_cases": [],
    "work_items": [],
    "agent_tasks": [],
    "agent_instructions": []
  },
  "notes": "Integrator narrative for process engineers"
}
```

## Mapping guidance (generic)

| Finding shape | Suggested work types |
|---------------|----------------------|
| Discrete defect with clear done-test | requirement + criterion + test_case + work_item |
| Architectural conflict | goal + workstream + milestone + tech_debt |
| Tooling gap | tooling_gap + work_item |
| Doc/mental model | work_item + agent_instruction (how to keep docs honest) |

**Rule:** Do not mint objects during CEF. Downstream systems choose IDs and lifecycles.

## Pros / cons

| Pros | Cons |
|------|------|
| Prevents analysis agents from flooding trackers | Requires a competent process engineer downstream |
| Portable across orgs | Mapping is guidance, not automation — v1 may add importers |

**Why:** You asked for objectify *after* analysis. Separating truth map from minting prevents the backlog sprawl that motivated this pivot.
