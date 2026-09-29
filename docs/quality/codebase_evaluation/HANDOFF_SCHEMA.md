# Handoff Schema & Downstream Integration Contract

> **Purpose:** Formal specifications for packaging evaluation results into an authoritative, tool-agnostic handoff manifest ready for downstream remediation, technical program management, and automated backlog minting.

| Specification Metadata | Value |
| :--- | :--- |
| **Framework Version** | CEF v0.1.0 |
| **Governance Tier** | Authoritative Core (Handoff Boundary) |
| **Target Roles** | Lead Integrators, Process Engineers, Swarm Orchestrators |
| **Target Systems** | ZQK Knowledge Kernel, Issue Trackers, CI/CD Gate Orchestrators |

---

## 1. Handoff Package Directory Structure

Upon completion of Wave 4, the lead integrator packages evaluation artifacts into a versioned, self-contained directory:

```text
cef-run-<timestamp>/
  run_scope.yaml                  # Frozen scope configuration
  scorecard.json                  # Multi-axis Diamond Scale scores
  findings.jsonl                  # Validated, post-adversarial findings
  adversarial_resolutions.jsonl   # Auditor verification log
  diagrams/**                     # Anchored architectural diagrams
  tooling_gaps.md                 # Discovered scanner and tooling deficiencies
  run_log.md                      # Operational execution journal
  handoff_manifest.json           # Canonical integration manifest
```

---

## 2. Handoff Manifest Schema (`handoff_manifest.json`)

The `handoff_manifest.json` file serves as the canonical contract between evaluation analysis and downstream remediation execution:

```json
{
  "cef_version": "0.1.0",
  "mode": "truth_map",
  "repo_fingerprint": "7e51cb7a61d15dfb8efb36873bcdd74bf93f2f51",
  "accepted_finding_ids": [
    "F-ARCH-001",
    "F-SEC-002",
    "F-CONC-003"
  ],
  "waived": [
    {
      "finding_id": "F-PERF-004",
      "reason": "Bounded by memory limits in container runtime.",
      "by": "lead-architect"
    }
  ],
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
  "notes": "Consolidated evaluation synthesis for engineering remediation."
}
```

---

## 3. Work Item Mapping Guidance

Downstream process engineers and orchestration pipelines map findings to work structures using the following taxonomy:

| Finding Shape | Suggested Downstream Artifact Types |
| :--- | :--- |
| Discrete defect with clear automated verification gate | `requirement` + `criterion` + `test_case` + `work_item` |
| Systemic architectural conflict or boundary violation | `goal` + `workstream` + `milestone` + `tech_debt` |
| Missing automated linter, scanner, or analyzer | `tooling_gap` + `work_item` |
| Documentation inaccuracy or mental model conflict | `work_item` + `agent_instruction` |

> [!IMPORTANT]
> **Minting Isolation Invariant:** Evaluation agents must never mint tracker objects or backlog items directly during analysis. Decoupling the objective truth map from backlog creation prevents issue tracker sprawl and maintains evaluation objectivity.

---

## 4. Architectural Analysis & Tradeoffs

| Advantage | Operational Consideration |
| :--- | :--- |
| Prevents evaluation swarms from polluting production issue trackers with speculative tasks. | Requires a downstream process engineer or automation to ingest and objectify the handoff package. |
| Tool-agnostic architecture integrates cleanly with ZQK, GitHub Issues, Jira, or plain Markdown plans. | Mapping taxonomy provides guidance rather than rigid enforcement, accommodating varied organizational workflows. |
