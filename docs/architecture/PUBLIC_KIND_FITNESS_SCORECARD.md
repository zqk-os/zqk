# Public-Kind Fitness Scorecard (7-Lens Rubric)

**Last Verified:** 2026-08-31


**Audit Scope:** All registered `visibility:public` object kinds in `zqk:kernel`.  
**Governing Standard:** [`docs/architecture/KERNEL_OBJECT_KIND_EVALUATION_RUBRIC.md`](./KERNEL_OBJECT_KIND_EVALUATION_RUBRIC.md)  
**Plan:** `PRI-REDACTED` / `BLI-REDACTED`  
**Auditor Seat:** `antigravity-1` (`PER-ORCH-ALPHA`)  

---

## 1. Executive Summary & Inventory

| Kind (Singular / Plural) | Instance Count (CAS) | Primary Lens Finding | Explicit Disposition | Follow-Up Tracking |
| :--- | :--- | :--- | :--- | :--- |
| `backlog_item` (`backlog`) | 879 | High utilization; strict lifecycle gates enforced | `keep_enforce` | `PRI-CEF-R2-S7-POLISH` |
| `criteria` (`criteria`) | 992 | Core verification gates; validated/complete gates holding | `keep_enforce` | `PRI-CEF-R2-S7-POLISH` |
| `doc_entry` (`doc_entries`) | 2053 | High-volume documentation index | `keep_enforce` | `WS-CEF-DOCS-UX` |
| `glossary_term` (`glossary_terms`) | 1562 | Core domain terminology dictionary | `keep_enforce` | `GLS-1786687875188966000-0ed1e9a6` |
| `requirement` (`requirements`) | 207 | Functional/non-functional commitments | `keep_enforce` | `WS-CEF-ARCHITECTURE` |
| `milestone` (`milestones`) | 128 | Roadmap and cadence checkpoints | `keep_enforce` | `ROAD-CEF-R2-GRADE-UPLIFT` |
| `policy` (`policies`) | 125 | Guardrails & governance enforcement rules | `keep_enforce` | `POL-CODE-007` family |
| `risk_blocker` (`risk_blockers`) | 93 | Operational/architectural risk tracking | `keep_enforce` | `WS-CEF-STABILITY` |
| `goal` (`goals`) | 83 | High-level project objectives | `keep_enforce` | `GOAL-OSS-CORE` |
| `persona` (`personas`) | 55 | Agent & operator seat definitions | `keep_enforce` | `PER-ORCH-ALPHA` |
| `priority_plan` (`priority_plans`) | 50 | TPM Gantt matrix active execution columns | `keep_enforce` | `PRI-REDACTED` |
| `test_case` (`tests`) | 55 | Traceable test definitions | `keep_enforce` | `WS-CEF-TESTING` |
| `agent_task` (`agent_tasks`) | 38 | In-flight execution tasks for agents | `keep_enforce` | `WS-CEF-ARCHITECTURE` |
| `agent_skill` (`agent_skills`) | 38 | Autonomous agent capabilities | `keep_enforce` | `docs/onboarding/SYSTEM_OBJECTS_GUIDE.md` |
| `prompt_template` (`prompt_templates`) | 39 | Structured prompt contracts | `keep_enforce` | `WS-CEF-DOCS-UX` |
| `workstream` (`workstreams`) | 37 | Long-lived functional tracks | `keep_enforce` | `ROAD-CEF-R2-GRADE-UPLIFT` |
| `account` (`accounts`) | 51 | Actor & agent IDs; contains fixture IDs | `quarantine_fixtures` | `BLI-REDACTED` |
| `glossary_term_relation` | 30 | Cross-term graph edges | `keep_enforce` | `GLS-1786687875188966000-0ed1e9a6` |
| `role` (`roles`) | 17 | Organizational & runtime roles | `keep_enforce` | `WS-CEF-ARCHITECTURE` |
| `decision` (`decisions`) | 15 | Architectural decision records (ADRs) | `keep_enforce` | `WS-CEF-ARCHITECTURE` |
| `convergence_session` | 14 | Session synchronization markers | `keep_enforce` | `DEC-REDACTED` |
| `technical_debt` (`technical_debts`) | 11 | Explicit debt items | `keep_enforce` | `WS-CEF-ARCHITECTURE` |
| `roadmap` (`roadmaps`) | 11 | Multi-quarter strategic roadmaps | `keep_enforce` | `ROAD-CEF-R2-GRADE-UPLIFT` |
| `vocabulary_scheme` | 9 | Ontology classification schemes | `keep_enforce` | `.zqk/process/vocabulary_schemes` |
| `rule` (`rules`) | 9 | Operational system rules | `keep_enforce` | `WS-CEF-STABILITY` |
| `domain_registry` | 7 | Domain authority registrations | `keep_enforce` | `pkg/domain` |
| `workflow` (`workflows`) | 6 | Reusable pipeline blueprints | `keep_enforce` | `pkg/pipeline` |
| `strategic_context` | 5 | Mission/strategic boundary objects | `keep_enforce` | `GOAL-OSS-CORE` |
| `question` (`questions`) | 12 | Community first-run onboarding tutorial kind | `keep_enforce` | `FIRST_RUN_OBJECT_TUTORIAL.md` |
| `command_spec` | 0 | Dual-source CLI DNA (AST vs Spec) | `remediate` | `PRI-REDACTED` |
| `agent_onboarding_preparation` | 0 | Low utilization on CAS (draft plane only) | `quarantine_fixtures` | `BLI-REDACTED` |
| `strategic_plan` | 1 | Strategic executive direction | `keep_enforce` | `ROAD-CEF-R2-GRADE-UPLIFT` |
| `probe_spec` | 1 | Inquiry probe specification | `keep_enforce` | `pkg/probe` |
| `library` (`libraries`) | 1 | Shared module references | `keep_enforce` | `pkg/storage` |
| `scenario` (`scenarios`) | 1 | Verification scenario models | `keep_enforce` | `ONBOARDING_EVALUATION_SCENARIO.md` |
| `mcp_spec` (`mcp_specs`) | 3 | MCP tool schemas | `keep_enforce` | `pkg/mcp` |

---

## 2. Detailed Per-Kind 7-Lens Scorecards

### Strategic & Core Ontology Kinds

```yaml
kind: goal
visibility: public
instance_count_kernel: 83
lenses:
  L1_lifecycle: pass — active/achieved/deprecated lifecycle transitions holding
  L2_utilization: pass — high utilization; linked by roadmaps, milestones, and BLIs
  L3_test_pollution: pass — clean production goal IDs
  L4_logic_teeth: pass — validated refs to roadmaps and requirements
  L5_required_fields: pass — title, description, and milestone_refs validated
  L6_duplicative_ids: pass — unique CAS blobs, zero report duplicates
  L7_status_vocabulary: pass — status matches commitment semantics (active, completed)
disposition: keep_enforce
follow_ups: [GOAL-OSS-CORE]
evidence: [.zqk/process/goals/]
```

```yaml
kind: requirement
visibility: public
instance_count_kernel: 207
lenses:
  L1_lifecycle: pass — draft -> active -> validated -> complete transitions enforced
  L2_utilization: pass — primary contract for all functional & architectural features
  L3_test_pollution: pass — test_case_refs cleanly linked
  L4_logic_teeth: pass — validation rules and criteria enforcement verified
  L5_required_fields: pass — title, description, criteria_refs present
  L6_duplicative_ids: pass — zero collision
  L7_status_vocabulary: pass — active requirements represent live commitments
disposition: keep_enforce
follow_ups: [WS-CEF-ARCHITECTURE]
evidence: [.zqk/process/requirements/]
```

```yaml
kind: priority_plan
visibility: public
instance_count_kernel: 50
lenses:
  L1_lifecycle: pass — active_order sequence strictly validated
  L2_utilization: pass — active execution vehicle for multi-agent swarm orchestration
  L3_test_pollution: pass — test scenarios isolate priority plan lifecycles
  L4_logic_teeth: pass — blocking dependencies prevent premature lane advancement
  L5_required_fields: pass — title, active_order, status present
  L6_duplicative_ids: pass — unique ID validation verified
  L7_status_vocabulary: pass — planned -> in_progress -> complete -> archived
disposition: keep_enforce
follow_ups: [PRI-REDACTED]
evidence: [.zqk/process/priority_plans/]
```

```yaml
kind: backlog_item
visibility: public
instance_count_kernel: 879
lenses:
  L1_lifecycle: pass — strict gate: in_progress -> complete requires validated CRITs and commit_refs
  L2_utilization: pass — central work tracking object across all engineering tracks
  L3_test_pollution: pass — fixture items isolated to test harness
  L4_logic_teeth: pass — commit_refs and actual_effort fail-closed on promotion
  L5_required_fields: pass — all required fields verified on write
  L6_duplicative_ids: pass — single CAS hash per BLI ID
  L7_status_vocabulary: pass — planned -> in_progress -> complete -> archived
disposition: keep_enforce
follow_ups: [PRI-CEF-R2-S7-POLISH]
evidence: [.zqk/process/backlog/]
```

```yaml
kind: criteria
visibility: public
instance_count_kernel: 992
lenses:
  L1_lifecycle: pass — awaiting_verification -> in_progress -> validated -> complete
  L2_utilization: pass — immutable acceptance contracts for BLIs and REQs
  L3_test_pollution: pass — verification evidence required before validation
  L4_logic_teeth: pass — promotion blocked without non-empty evidence string
  L5_required_fields: pass — title, category, validation_method verified
  L6_duplicative_ids: pass — validated unique
  L7_status_vocabulary: pass — verification status vocabulary strictly typed
disposition: keep_enforce
follow_ups: [WS-CEF-TESTING]
evidence: [.zqk/process/criteria/]
```

```yaml
kind: agent_task
visibility: public
instance_count_kernel: 38
lenses:
  L1_lifecycle: pass — proposed -> in_progress -> pending_verification -> implemented
  L2_utilization: pass — atomic seat assignments for peer/subagent swarms
  L3_test_pollution: pass — task_steps verification strategies required
  L4_logic_teeth: pass — implemented status requires commit_hash and completed task_steps
  L5_required_fields: pass — title, claimed_by, integration_branch verified
  L6_duplicative_ids: pass — single assignment per ATK ID
  L7_status_vocabulary: pass — implemented signifies ready for merge and peer ack
disposition: keep_enforce
follow_ups: [WS-CEF-ARCHITECTURE]
evidence: [.zqk/process/agent_tasks/]
```

---

## 3. High-Risk Findings & Fixture Remediation

### A. Account Fixture Quarantine (Crevice-Sweep Cross-Link)
- **Finding**: Several legacy test accounts (e.g. `ACC-TEST-*`) remain stored in `.zqk/process/accounts/`.
- **Cross-Link**: Explicitly linked to Crevice-Sweep [`BLI-REDACTED`](file:///tmp/zqk-worktrees/pri-fitness/.zqk/process/backlog/02b260ae84fcf71f9f8a76e19b6bc1e7a44f0bec0b3c7d939b1d297964b1ba9b.yaml) to archive or relocate fixture accounts outside the `zqk:kernel` production namespace (`CRIT-REDACTED`).

### B. Command Spec Dual-Source
- **Finding**: `command_spec` currently has 0 instances on CAS because CLI commands are declared directly in Go code (`cmd/zqk/`).
- **Disposition**: `remediate` — tracked under [`PRI-REDACTED`](file:///tmp/zqk-worktrees/pri-fitness/.zqk/process/priority_plans/1747311079e8d890f36226b35a5daae07c4fb823d2ab9725e7502ae06eeebad8.yaml) to generate `command_spec` directly from CLI AST to preserve single-source-of-truth.

### C. Draft Plane Honesty
- **Finding**: Draft objects with `status=draft` must remain on the `.zqk/object_drafts/` draft plane and never be directly committed to CAS.
- **Disposition**: `keep_enforce` — enforced by [`BLI-REDACTED`](file:///tmp/zqk-worktrees/pri-fitness/.zqk/process/backlog/1a8394fe716ad516e4179c1f66ff32aa68a0320821d50b1a0297f91b280e9339.yaml) and `zqk system check` Layer-0 integrity rules.
