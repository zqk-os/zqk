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

## 3. Comprehensive Pack & Kind Taxonomy Directory

ZQK organizes its object ontologies into 10 modular packs:

```
                               ┌─────────────────────────┐
                               │   KNOWLEDGE KERNEL      │
                               └────────────┬────────────┘
         ┌──────────────────────────────────┼──────────────────────────────────┐
         │                                  │                                  │
┌────────▼────────┐                ┌────────▼────────┐                ┌────────▼────────┐
│   WORK PACK     │                │ KNOWLEDGE PACK  │                │   AGENT PACK    │
│ goal, plan, req,│                │ glossary_term,  │                │ persona, task,  │
│ criteria, bli   │                │ doc_entry, lib  │                │ feed, mcp_spec  │
└─────────────────┘                └─────────────────┘                └─────────────────┘
```

### 3.1 Work & Strategy Pack (`packs/work/`)
The foundational spine for technical program management and execution:
- **`vision`** (`VIS-*`): Long-term strategic aspirations and market horizons.
- **`mission`** (`MSN-*`): Operational charter operationalizing a vision.
- **`goal`** (`GOAL-*`): Measurable milestone target supporting a mission.
- **`roadmap`** (`RDM-*`): Multi-quarter grouping of goals and releases.
- **`priority_plan`** (`PRI-*`): Tactical execution container scoping a focused set of requirements and backlog items.
- **`workstream`** (`WKS-*`): Functional domain stream grouping related work.
- **`requirement`** (`REQ-*`): Formal technical requirement specifying feature behavior.
- **`criteria`** (`CRIT-*`): Verifiable Definition of Done (DoD) acceptance condition.
- **`test_case`** (`TST-*`): Automated verification test validating criteria.
- **`backlog_item`** (`BLI-*`): Tactical, actionable unit of work claimed and executed by agents or engineers.
- **`epic`** (`EPC-*`): High-level feature epic aggregating multiple backlog items.
- **`important_date`** (`DAT-*`): Deadlines, freeze windows, or release dates.
- **`risk_blocker`** (`RSK-*`): Explicit impediment or dependency risk obstructing progress.
- **`technical_debt`** (`DEB-*`): Tracked architectural debt requiring remediation.

### 3.2 Semantic Vocabulary Pack (`packs/vocabulary/`)
Shared terminology, machine hints, and disambiguation:
- **`glossary_term`** (`GLS-*`): Context-scoped definition embedding human explanations, `agent_prompts` for LLMs, and `machine_hints` for tools.
- **`vocabulary_scheme`** (`VOC-*`): Scoped taxonomy or lens network (`inference`, `display`, `navigation`, `mixed`, `extension`).
- **`glossary_term_relation`** (`GTR-*`): Directed typed relationships between glossary terms.
- **`import_tracking`** (`IMP-*`): Provenance tracking for external ontology imports.

### 3.3 Library & Documentation Pack (`packs/library/`)
Documentation and composable architecture specifications:
- **`doc_entry`** (`DOC-*`): First-class CAS object representing a documentation file, tracked with SHA-256 `content_hash` and verified via `zqk docman verify`.
- **`library`** (`LIB-*`): Composable architectural pattern catalogs with cloneable configurations.
- **`technical_spec`** (`TSP-*`): Detailed technical specifications attached to libraries.

### 3.4 Autonomous Agent Pack (`packs/agent/`)
Agent coordination, seating, and capabilities:
- **`persona`** (`PER-*`): Registered agent profile (e.g. `PER-DEFAULT-OPERATOR`, `PER-DEFAULT-ARCHITECT`).
- **`agent_task`** (`TSK-*`): Subordinate execution task delegated to a subagent.
- **`agent_feed`** (`FED-*`): Append-only correspondence log for agent-to-agent and human-to-agent steering.
- **`agent_skill`** (`SKI-*`): Verified executable capability or workflow routine available to agents.
- **`agent_instruction`** (`INS-*`): Standing operating procedure or ambient directive.
- **`mcp_spec`** (`MCP-*`): Specification for Model Context Protocol servers.
- **`mcp_session`** (`MCS-*`): Active runtime session with an MCP host.

### 3.5 Decision & Rationalization Pack (`packs/decision/`)
Architectural decision records and inquiry tracking:
- **`decision`** (`DEC-*`): Authoritative architectural decision record (ADR).
- **`question`** (`QST-*`): Open design inquiry or clarification probe requiring consensus.
- **`impact_analysis`** (`IMP-*`): Formal risk and cost impact evaluation for architectural transitions.

### 3.6 Quality & Verification Pack (`packs/qa/`)
Continuous verification and done-gate enforcement:
- **`scenario`** (`SCN-*`): End-to-end integration scenario or system test suite.
- **`code_reference`** (`REF-*`): Semantic link connecting kernel objects to repository source lines (`file://...#L10-L20`).
- **`validation_rule`** (`VRL-*`): Declarative rule DSL expression evaluated against kernel objects.
- **`verification_matrix`** (`MTX-*`): Traceability matrix aggregating requirements, criteria, and test runs.
- **`maturation_report`** (`MAT-*`): Audit report on object and codebase maturity.

### 3.7 Organization & Stakeholders (`packs/org/`)
Organizational structure and governance ownership:
- **`organization`** (`ORG-*`), **`division`** (`DIV-*`), **`department`** (`DEP-*`), **`team`** (`TEM-*`): Structural hierarchy defining organizational boundaries.
- **`stakeholder_profile`** (`STK-*`): Person or role accountable for specific domains or decisions.

### 3.8 Telemetry & Metrics (`packs/metric/`)
Kernel telemetry, performance metrics, and daemon health:
- **`command_metric`** (`CMD-*`): Execution metrics (duration, exit code, resource utilization) per CLI command.
- **`status_history_metric_sampler`** (`SHM-*`): Time-series tracking of lifecycle state transitions across the graph.
- **`scheduler_health_metric`** (`SHL-*`): Operational health of background daemons and cron workers.

---

## 4. The Ontological Cascade: Verifiable Decomposition Spine (VDS)

In ZQK, every unit of executable work must form an unbroken chain from strategic intent to automated test verification:

```
[ vision: VIS-* ]
        │ defines
        ▼
[ mission: MSN-* ]
        │ operationalizes
        ▼
[ goal: GOAL-* ]
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
```

> [!IMPORTANT]
> **TPM Definition of Done Guardrail**:
> Pre-commit hooks (`zqk pre-commit`) and release gates fail-closed if any test case or criterion lacks a complete, unbroken lineage to root objects, or if any active requirement lacks mandatory documentation criteria (`POL-DOC-001`).

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
