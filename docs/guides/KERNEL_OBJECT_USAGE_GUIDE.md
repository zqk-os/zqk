# Developer & Agent Guide: Knowledge Kernel Object Model & Usage Guide

## 1. Executive Summary & Architectural Motivation

In the **ZQK Knowledge Kernel**, state is never stored as loose files, untracked JSON snippets, or ephemeral agent memory caches. Every element of project truth—from multi-year corporate goals to tactical code acceptance criteria, glossary terms, documentation entries, and agent personas—is represented as a strongly typed, cryptographically verified **Kernel Object**.

By standardizing all state into a unified object model, ZQK provides:
1. **Universal Lineage & Traceability**: Every tactical code commit traces up to verifiable criteria, requirements, priority plans, and strategic goals without gaps.
2. **Content-Addressed Storage (CAS)**: Tamper-evident SHA-256 fingerprinting guarantees data integrity across local and distributed environments.
3. **Formal State Machine Governance**: Objects progress through deterministic, check-valved lifecycles (`originated` ➔ `planned` ➔ `in_progress` ➔ `validated` ➔ `complete`).
4. **Declarative Graph Integration**: All objects are nodes in the Knowledge Graph, queryable via Cypher-like ZPARQL and mutable via atomic ZQL transactions.

This guide provides developers, technical program managers (TPMs), and autonomous AI agents with the canonical reference manual and hands-on cookbook for working with kernel objects across all 10 core packs.

---

## 2. Universal Object Anatomy & Envelopes

Every kernel object serialized on disk adheres to a standard architectural envelope:

```yaml
schema_version: 2.0.0
id: BLI-AUTH-004
kind: backlog_item
namespace_id: zqk:kernel
title: "Implement Content-Addressed Storage Membrane Check-Valves"
status: in_progress
priority_tier: P1
priority_plan_ref: PRI-PUBLIC-LAUNCH-100
goal_refs:
  - GOAL-001
requirement_refs:
  - REQ-012
criteria_refs:
  - CRIT-049
test_case_refs:
  - TST-030-01
doc_entry_refs:
  - DOC-001
claimed_by: "PER-DEFAULT-OPERATOR"
created_at: "2026-09-29T14:00:00Z"
updated_at: "2026-09-30T01:15:00Z"
created_by: ACC-1785920548450214012-68b850c0
updated_by: ACC-1785920548450214012-68b850c0
storage_profile:
  cas_hash: "sha256:d8a9f4e2c8104598bfaaa107ac635e1070be99d5c10eba5793cac0baf38ba8a1"
  plane: "authoritative"
  etag: "rev-003-d8a9f"
```

### 2.1 Standard Envelope Fields:
| Field | Type | Description |
| :--- | :--- | :--- |
| `schema_version` | `string` | Semantic specification version (currently `2.0.0`). |
| `id` | `string` | Unique deterministic identifier prefixed by kind code (e.g. `BLI-*`, `REQ-*`, `GLS-*`, `DOC-*`). |
| `kind` | `string` | Authoritative object kind registered in the pack ontology. |
| `namespace_id` | `string` | Namespace boundary (`zqk:kernel` for core, `tenant:*` for federated domains). |
| `title` | `string` | Human-readable title summarizing the object. |
| `status` | `string` | Active state within the kind's lifecycle state machine. |
| `created_at` / `updated_at` | `timestamp` | ISO-8601 UTC timestamps. |
| `created_by` / `updated_by` | `string` | Account or persona ID responsible for mutations. |
| `storage_profile` | `map` | CAS hash, storage plane (`draft` vs `authoritative`), and revision etag. |

---

## 3. Comprehensive Ontological Domain & Kind Directory

ZQK organizes its object ontologies into **14 comprehensive domains**: the **Core Platform & Kernel Membrane**, plus **13 specialized domain packs**. Together, they form an interconnected graph of over 70 distinct object kinds governing every layer of the operating system:

```
                               ┌─────────────────────────┐
                               │   KNOWLEDGE KERNEL      │
                               │  Core Platform Membrane │
                               └────────────┬────────────┘
         ┌──────────────────────────────────┼──────────────────────────────────┐
         │                                  │                                  │
┌────────▼────────┐                ┌────────▼────────┐                ┌────────▼────────┐
│   WORK & TPM    │                │ KNOWLEDGE & DOC │                │ AGENT & SWARM   │
│ goal, milestone,│                │ glossary_term,  │                │ persona, task,  │
│ req, crit, bli  │                │ doc_entry, lib  │                │ feed, mcp_spec  │
└────────┬────────┘                └────────┬────────┘                └────────┬────────┘
         │                                  │                                  │
┌────────▼────────┐                ┌────────▼────────┐                ┌────────▼────────┐
│ RELEASE & EVO   │                │ QA & VERIFY     │                │ DECISION & ORG  │
│ release, evolve,│                │ scenario, test, │                │ decision, ADRs, │
│ workflow        │                │ qa_success, mtx │                │ teams, orgs     │
└─────────────────┘                └─────────────────┘                └─────────────────┘
```

### 3.1 Core Platform & Kernel Governance (`.zqk/specs/objects/kernel/` & `platform/`)
The underlying governance, security, and runtime primitives of the Knowledge Kernel:
- **`account`** (`ACC-*`): Human and agent identities, authentication keys, and authorization bindings.
- **`role`** (`ROL-*`): System authorities (`executive`, `owner`, `automation`, `steward`) governing command execution permissions.
- **`policy`** (`POL-*`): Project-wide declarative invariants, constraints, and validation standards (e.g. `POL-DOC-001`).
- **`rule`** (`RUL-*`): Operational boundary rules and governance policies.
- **`auto_fix_rule`** (`AFR-*`): Automated remediation recipes executed by `zqk system check --auto-remedy`.
- **`scheduler_job`** (`JOB-*`): Background cron jobs, maintenance workers, retention scanners, and callback routines.
- **`scheduler_handler_binding`** (`SHB-*`): Dynamic dispatch bindings routing jobs to internal handlers.
- **`namespace`** (`NSP-*`): Tenant and domain boundaries isolating objects (`zqk:kernel` vs `tenant:*`).
- **`namespace_registry`** (`NSR-*`): Global authoritative index of all active namespaces.
- **`remote_kernel`** (`RMK-*`): Remote peer kernel endpoints in the Federated Sovereign Mesh.
- **`domain_registry`** (`DOM-*`): Authoritative registry tracking registered domain ontologies.
- **`keystore_entry`** (`KEY-*`): Local cryptographic credential vault entries.
- **`audit_event`** (`AUD-*`): Immutable append-only log of state mutations and security events.
- **`audit_event_aggregation`** (`AEA-*`): Durable record of audit-event compaction passes.
- **`change_journal_entry`** (`CJE-*`): Write-Ahead Log (WAL) records enabling crash recovery and point-in-time replay.
- **`convergence_session`** (`CVS-*`): Multi-agent alignment sessions, consensus ballots, and convergence reports.
- **`zqk_session`** (`SES-*`): Active CLI invocations, interactive sessions, and daemon runtime contexts.
- **`shockwave_router`** (`SWR-*`): Micro-API Gateway router for semantic payloads and reactive events.
- **`integrity_manifest`** (`INT-*`): Snapshot of critical file and object hashes for drift detection.
- **`rollback_report`** (`RBR-*`): Summary receipts detailing rolled-back objects and state restorations.
- **`template`** (`TPL-*`): Prompt and artifact generation templates.
- **`watchdog_registration`** (`WDR-*`): Event-driven watchdog subscriptions and alerts.
- **`kind_synonym`** (`SYN-*`): Convenient aliases for object kinds (e.g. `bli` for `backlog_item`).
- **`brand`** (`BRD-*`): Brand and styling specifications for output generators.
- **`certificate`** (`CRT-*`): Cryptographic completion certificates issued upon milestone completion.
- **`resolver`** (`RES-*`): Reference scheme resolvers (`account:{id}`, `test://...`).
- **`bucketing_strategy`** (`BKT-*`): Storage bucketing strategies for tiered storage.
- **`base_object`**, **`auditable`**, **`object_spec`**, **`lifecycle`**, **`extensible_object`**, **`base_metric`**: Structural meta-types defining the schema system.

### 3.2 Work & Technical Program Management Pack (`packs/work/`)
The foundational spine for roadmap planning, requirement engineering, and task execution:
- **`vision`** (`VIS-*`): Long-term strategic aspirations and market horizons.
- **`mission`** (`MSN-*`): Operational charter operationalizing a vision.
- **`goal`** (`GOAL-*`): Measurable milestone target supporting a mission.
- **`milestone`** (`MIL-*`): Formal phase boundaries anchoring deliverables, releases, and done-gates.
- **`roadmap`** (`RDM-*`): Multi-quarter grouping of goals, milestones, and releases.
- **`priority_plan`** (`PRI-*`): Tactical execution container scoping an active set of requirements and backlog items.
- **`workstream`** (`WKS-*`): Functional domain stream grouping related work.
- **`workstream_transition`** (`WST-*`): Formal handoffs and boundary crossings between workstreams.
- **`work_unit`** (`WKU-*`): Granular execution slices within a backlog item.
- **`work_interval`** (`WKI-*`): Time-boxed iteration cycles and execution sprints.
- **`occupancy`** (`OCC-*`): Agent seating, concurrency leases, and active workspace locks.
- **`remaining_open`** (`REM-*`): Burn-down metrics tracking remaining open items.
- **`strategic_plan`** (`STP-*`): High-level organizational strategy plans.
- **`strategic_context`** (`STC-*`): Strategic framing, assumptions, and business drivers.
- **`requirement`** (`REQ-*`): Formal technical requirement specifying feature behavior (linked via `doc_entry_refs`).
- **`criteria`** (`CRIT-*`): Verifiable Definition of Done (DoD) acceptance condition.
- **`test_case`** (`TST-*`): Automated verification test validating criteria.
- **`backlog_item`** (`BLI-*`): Tactical unit of work claimed and executed by agents or engineers.
- **`epic`** (`EPC-*`): High-level feature epic aggregating multiple backlog items.
- **`important_date`** (`DAT-*`): Deadlines, freeze windows, or release dates.
- **`risk_blocker`** (`RSK-*`): Explicit impediments or dependency risks obstructing progress.
- **`technical_debt`** (`DEB-*`): Tracked architectural debt requiring remediation.

### 3.3 Release & Deployment Pack (`packs/release/`)
Formal release packaging, artifact manifests, and rollout governance:
- **`release`** (`REL-*`): Release envelopes containing semver versions, release candidates, target artifact manifests, cryptographic sign-offs, and rollout gates.

### 3.4 Evolution & Schema Migration Pack (`packs/evolution/`)
Continuous schema evolution and backward compatibility:
- **`evolution_management`** (`EVO-*`): Schema version migrations, field deprecation schedules, and database transformation plans.

### 3.5 Orchestrated Workflow Pack (`packs/workflow/`)
Multi-step automated workflows:
- **`workflow`** (`WKF-*`): Orchestrated multi-step graph workflows, automation chains, and phase transitions.

### 3.6 Semantic Vocabulary Pack (`packs/vocabulary/`)
Shared terminology, machine hints, and semantic disambiguation:
- **`glossary_term`** (`GLS-*`): Context-scoped definition embedding human explanations, `agent_prompts` for LLMs, and `machine_hints` for tools.
- **`vocabulary_scheme`** (`VOC-*`): Scoped taxonomy or lens network (`inference`, `display`, `navigation`, `mixed`, `extension`).
- **`glossary_term_relation`** (`GTR-*`): Directed typed relationships between glossary terms (`broader`, `narrower`, `related`, `governs`).
- **`import_tracking`** (`IMP-*`): Provenance tracking for external ontology imports (SKOS, OWL, Dublin Core).

### 3.7 Library & Documentation Pack (`packs/library/`)
Documentation and composable architecture specifications:
- **`doc_entry`** (`DOC-*`): First-class CAS object representing a documentation file, tracked with SHA-256 `content_hash` and verified via `zqk docman verify`.
- **`library`** (`LIB-*`): Composable architectural pattern catalogs with cloneable configurations and shared DNA propagation.
- **`technical_spec`** (`TSP-*`): Detailed technical specifications attached to libraries.

### 3.8 Autonomous Agent Pack (`packs/agent/`)
Agent coordination, workspace seating, and runtime capabilities:
- **`persona`** (`PER-*`): Registered agent profile (e.g. `PER-DEFAULT-OPERATOR`, `PER-DEFAULT-ARCHITECT`).
- **`agent_task`** (`TSK-*`): Subordinate execution task delegated to a subagent.
- **`agent_feed`** (`FED-*`): Append-only correspondence log for agent-to-agent and human-to-agent steering.
- **`agent_skill`** (`SKI-*`): Verified executable capability or workflow routine available to agents.
- **`agent_instruction`** (`INS-*`): Standing operating procedure or ambient directive.
- **`agent_architecture`** (`ARC-*`): Architectural topology and seating hierarchy for agent swarms.
- **`agent_onboarding_preparation`** (`AOP-*`): Machine-ready agent workspace configurations and seating directives.
- **`mcp_spec`** (`MCP-*`): Specification for Model Context Protocol servers.
- **`mcp_session`** (`MCS-*`): Active runtime session with an MCP host.
- **`mcp_built_in_tool`** (`MBT-*`): Built-in MCP tool manifests exposed to clients.
- **`prompt_template`** (`PRT-*`): Parameterized LLM prompt templates.
- **`provider_profile`** (`PRV-*`): LLM model and backend provider configuration.
- **`context_refresh_schedule`** (`CRS-*`): Periodic context window refresh triggers preventing context rot.

### 3.9 Decision & Rationalization Pack (`packs/decision/`)
Architectural decision records and design inquiries:
- **`decision`** (`DEC-*`): Authoritative architectural decision record (ADR).
- **`question`** (`QST-*`): Open design inquiry or clarification probe requiring consensus.
- **`impact_analysis`** (`IMP-*`): Formal risk and cost impact evaluation for architectural transitions.

### 3.10 Quality & Verification Pack (`packs/qa/`)
Continuous verification and done-gate enforcement:
- **`scenario`** (`SCN-*`): End-to-end integration scenario or system test suite.
- **`code_reference`** (`REF-*`): Semantic link connecting kernel objects to repository source lines (`file://...#L10-L20`).
- **`validation_rule`** (`VRL-*`): Declarative rule DSL expression evaluated against kernel objects.
- **`verification_matrix`** (`MTX-*`): Traceability matrix aggregating requirements, criteria, and test runs.
- **`qa_success`** (`QAS-*`): Cryptographically recorded passing test execution receipts in CAS.
- **`maturation_report`** (`MAT-*`): Audit report on object and codebase maturity.
- **`process_hygiene_rule`** (`PHR-*`): Process hygiene and drift detection rules.
- **`test_command_rule`** (`TCR-*`): Automated test execution recipes.
- **`test_audit_aggregation_metric`** (`TAM-*`): Aggregated test audit rollups.
- **`code_quality_metric`** (`CQM-*`): Static analysis and code quality scores.

### 3.11 Organization & Stakeholders Pack (`packs/org/`)
Organizational structure and governance ownership:
- **`organization`** (`ORG-*`), **`division`** (`DIV-*`), **`department`** (`DEP-*`), **`team`** (`TEM-*`): Structural hierarchy defining organizational boundaries.
- **`team_configuration`** (`TCF-*`): Execution configurations and authority delegates for teams.
- **`stakeholder_profile`** (`STK-*`): Person or role accountable for specific domains or decisions.
- **`corporate_initiative`** (`CRP-*`): Cross-cutting corporate initiatives.
- **`partnership`** (`PRN-*`): External ecosystem partner configurations.
- **`organizational_change`** (`OCH-*`): Organization change impact records.

### 3.12 Telemetry & Metrics Pack (`packs/metric/`)
Kernel telemetry, performance metrics, and daemon health:
- **`command_metric`** (`CMD-*`): Execution metrics (duration, exit code, resource utilization) per CLI command.
- **`sampler_profile`** (`SMP-*`): Telemetry sampler profiles and sample rates.
- **`status_history_metric_sampler`** (`SHM-*`): Time-series tracking of lifecycle state transitions across the graph.
- **`scheduler_health_metric`** (`SHL-*`): Operational health of background daemons and cron workers.
- **`file_lock_metric`** (`FLM-*`): Lock duration and contention metrics.
- **`scalar_metric_sampler`** (`SMS-*`), **`list_metric_sampler`** (`LMS-*`), **`ordered_list_metric_sampler`** (`OMS-*`): Generic metric samplers.
- **`kind_mapping_metric`** (`KMM-*`): Object-kind distribution metrics.
- **`metadata_package`** (`MDP-*`): Packaged telemetry metadata bundles.
- **`base_sampler`** (`BSM-*`): Base metric sampling configuration.
- **`audit_aggregation_metric`** (`AAM-*`): Telemetry aggregation metrics.

### 3.13 Pipeline Pack (`packs/pipeline/`)
Multi-stage build, deployment, and data pipelines:
- **`pipeline_definition`** (`PLD-*`): Declarative multi-stage pipeline definition.
- **`pipeline`** (`PIP-*`): Concrete instantiated pipeline instance.
- **`pipeline_execution`** (`PLX-*`): Execution run receipts with timing, stage outputs, and exit codes.
- **`pipeline_stage`** (`PLS-*`): Individual stage in a pipeline execution graph.

### 3.14 Interface & Display Pack (`packs/interface/`, `packs/display/`)
CLI commands, APIs, and UI console interfaces:
- **`command_spec`** (`CMS-*`): Canonical CLI command specifications.
- **`api_spec`** (`API-*`): Public and internal API endpoint specifications.
- **`display`** (`DSP-*`): TUI and GUI layout definitions.
- **`component`** (`CMP-*`): Reusable UI/TUI components.

---

## 4. The Ontological Cascade: Verifiable Decomposition Spine (VDS)

In ZQK, every unit of executable work must form an unbroken chain from strategic intent down to automated test verification and production release:

```
[ vision: VIS-* ]
        │ defines
        ▼
[ mission: MSN-* ]
        │ operationalizes
        ▼
[ goal: GOAL-* ]
        │ decomposes into
        ▼
[ milestone: MIL-* ]
        │ drives
        ▼
[ priority_plan: PRI-* ]
        │ groups
        ▼
[ workstream: WKS-* ]
        │ executes
        ▼
[ requirement: REQ-* ] ──(doc_entry_refs)──▶ [ doc_entry: DOC-* ] (POL-DOC-001)
        │ decomposed into
        ▼
[ criteria: CRIT-* ]
        │ verified by
        ▼
[ test_case: TST-* ]
        │ executed in
        ▼
[ backlog_item: BLI-* ]
        │ produces
        ▼
[ qa_success: QAS-* ]
        │ seals
        ▼
[ release: REL-* ]
```

> [!IMPORTANT]
> **TPM Definition of Done Guardrail**:
> Pre-commit hooks (`zqk pre-commit`) and release gates fail-closed if:
> 1. Any test case or criterion lacks an unbroken lineage up to root milestone and goal objects.
> 2. Any active requirement lacks mandatory documentation criteria (`POL-DOC-001`).
> 3. Any promotion to `release` lacks cryptographically verified `qa_success` receipts.

---

## 5. Everyday CLI Command Matrix & Recipes

### Recipe 1: Inspecting Specs & Generating Templates (`object template` / `new`)
Before minting an object, inspect its schema and default YAML template:
```bash
# View all available fields and validation constraints for a kind
zqk spec fields requirement

# Output a ready-to-fill YAML template
zqk object template requirement

# Scaffold a draft YAML template file to disk
zqk new backlog_item --out /tmp/bli_draft.yaml
```

### Recipe 2: Minting an Object (`object create`)
Create a new object directly through the CLI storage provider:
```bash
# Mint a new requirement with documentation references
zqk object create requirement \
  --title "Implement Unified Telemetry Stream" \
  --fields priority:p1,goal_refs:GOAL-001,doc_entry_refs:DOC-012

# Mint an actionable backlog item (BLI)
zqk object create backlog_item \
  --title "Add stream serialization handlers" \
  --fields priority_tier:P1,priority_plan_ref:PRI-PUBLIC-LAUNCH-100
```

### Recipe 3: Reading & Inspecting Objects (`object get` / `object inspect`)
Retrieve full or projected data for an object:
```bash
# Fetch complete object YAML
zqk object get BLI-AUTH-004

# Request JSON projection of specific keys (saves context window tokens)
zqk object get BLI-AUTH-004 --fields id,title,status,claimed_by --format json

# Launch full-screen interactive TUI object inspector
zqk inspect BLI-AUTH-004
```

### Recipe 4: Listing, Filtering & Grouping Objects (`object list`)
Filter across thousands of objects cleanly:
```bash
# List all active backlog items in progress
zqk object list backlog_item --filter "status=in_progress"

# Filter by multiple fields with projection and sorting
zqk object list requirement \
  --filter "priority=p1" \
  --fields id,title,status \
  --sort-by updated_at --sort-asc=false

# Group objects by status
zqk object list backlog_item --group-by status

# Count objects of a kind
zqk object list glossary_term --count
```

### Recipe 5: Establishing Graph References (`object ref add` / `remove`)
Wire relationships between objects atomically:
```bash
# Link a requirement to a backlog item
zqk object ref add BLI-001 REQ-001

# Link multiple criteria to a requirement
zqk object ref add REQ-001 CRIT-001 CRIT-002

# Link a doc_entry to a requirement (POL-DOC-001)
zqk object ref add REQ-001 DOC-014 --field doc_entry_refs

# Remove a relationship cleanly
zqk object ref remove BLI-001 REQ-001
```

### Recipe 6: Lifecycle State Progression (`object promote` / `demote` / `park`)
Never modify `status` directly via scalar edits. Use authoritative lifecycle state verbs:
```bash
# Promote an object to its next valid lifecycle state
zqk object promote BLI-001

# Promote multiple items concurrently
zqk object promote BLI-001,BLI-002,BLI-003

# Demote an item if verification uncovers defects
zqk object demote BLI-001

# Park an item along an intentional lifecycle exit
zqk object park BLI-001
```

### Recipe 7: Atomic Multi-Object Mutations (ZQL)
Execute ACID mutations across multiple objects in a single transaction:
```bash
zqk mutate "UPDATE backlog_item SET status = 'in_progress', claimed_by = 'PER-DEFAULT-OPERATOR' WHERE id = 'BLI-001'"
```

### Recipe 8: Complex Graph Traversal (ZPARQL)
Query the knowledge graph for multi-hop lineage:
```bash
zqk query "MATCH (p:priority_plan)-[:groups]->(r:requirement)-[:decomposed_into]->(c:criteria) WHERE p.id = 'PRI-PUBLIC-LAUNCH-100' RETURN r.title, c.title, c.status"
```

### Recipe 9: Schema & Policy Validation (`object validate` / `system check`)
Verify object compliance:
```bash
# Validate an individual object against its specification schema
zqk object validate BLI-001

# Run comprehensive 4-layer system health check
zqk system check all
```

---

## 6. Anti-Patterns & Fail-Closed Guardrails

1. **NEVER manually edit files under `.zqk/process/`**:
   - Direct edits break SHA-256 CAS hashes and corrupt the Content-Addressed Storage index, triggering Tier 0 CAS Corruption.
   - Always mutate state via `zqk object create`, `zqk object update`, `zqk object ref add`, or `zqk mutate`.
2. **NEVER bypass lifecycle check-valves**:
   - Setting `status: complete` directly bypasses DoD validation gates and test case verification. Always run `zqk object promote`.
3. **NEVER originate orphaned criteria or test cases**:
   - Every criteria must link to a parent requirement, and every test case must link to criteria. Unbound objects fail pre-commit gates.
4. **NEVER deliver code without linked documentation (`POL-DOC-001`)**:
   - Feature requirements must link to registered `doc_entry` objects before promotion to `complete`.
