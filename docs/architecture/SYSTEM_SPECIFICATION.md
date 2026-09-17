# ZQK (Zen Quantum Kernel) Master Specification & Schema Blueprint

**Last Verified:** 2026-08-31

> **Target Audience:** Technical Program Manager (TPM) Agents, Lead Architects, and Implementation Agents.
> **Purpose:** This document is the ultimate source of truth for ZQK. It provides the hierarchical product narrative and the strict YAML schemas, internal data structures, and lifecycle state machines for EVERY system object. It is designed to be parsed by an LLM without hallucination.

## PART 1: The Hierarchical Product Narrative
### 1.1 The Foundation (Mission & Vision)
**Mission (`MIS-1775446507801844000-42ba7fc8`)**: **ZQK (Zen Quantum Kernel)** exists to give AI and human collaborators a **shared operating layer**: a distributed **knowledge kernel**, first-class **objects** (backlog, requirements, policies, metrics), and a **stable CLI/MCP surface** so work is **effective, efficient, accurate, safe, reliable, and observable**.

We ship the primitives that make **agent-driven software engineering** practical: traceability from vision → goals → plans → backlog, **human-in-the-loop** governance hooks, **observable** automation (scheduler, audit, health), and a path toward **GraphRAG-style** structured memory—without sacrificing day-to-day velocity on the kernel itself.

**The Problem**: Software delivery with many AI agents and humans fragments across chat threads, ad hoc scripts, and siloed docs. Context is lost, governance is inconsistent, and outcomes are hard to prove or replay. Teams need a **single, queryable system of record** for goals, work, policies, and evidence—not another chat wrapper.

**Vision**: Success looks like hybrid teams where every important decision and deliverable is **represented as objects**, automation runs **through** the platform (not beside it), and newcomers—human or agent—can **query** mission, vision, plans, and health the same way operators do.

### 1.2 Strategic Goals & Linked Requirements
#### Goal: System Security and Compliance (`GOAL-001`)
**Metric:** None | **Target:** 100
Maintain system security, compliance, and auditability through proper access controls
and comprehensive audit logging for privileged operations.

**Fulfilling Requirements:**
- **Stream path cache highest priority; everything uses it** (`[REDACTED-ID]`): The stream path cache must be built first (Tier 0) at scheduler cache pre-warm so that any component containing a path can resolve it at runtime. All path resolution must use the resolver (GetStreamSegmentDir) so data can be moved with minimal risk.

Implements and is traceable to architectural policy on stream path resolution from cache (POL-ARCH-*). Criteria define verification (code review: no hardcoded paths; pre-warm order).
- **Swarm-Scale Stream Sharding (Zen Quantum Cortex)** (`[REDACTED-ID]`): To support the 100B+ semantic triple threshold and Swarm Orchestration (Neuron/Muscle cells), the stream storage backend must support lock-free horizontal partitioning (sharding) of high-volume event streams (like audit_event and change_journal). Workers must be able to ingest and compact shards concurrently without central lock contention.
- **Cryptographic SHA-256 Guarantee for Test Case Verification** (`[REDACTED-ID]`): When a test case passes during orchestration, compute a SHA-256 hash of the codebase/verified artifacts to establish complete spec-driven traceability.
- **Durable Spine Driver (Kafka)** (`REQ-406`): Replace stdout stub with actual Kafka/File-backed durability.
- **Merkle-Proofed Skill Bundling** (`REQ-407`): Implement cryptographic hashing of skill packages (SKILL.md, scripts, assets) to ensure integrity.
- **Skill Verification Protocol** (`REQ-408`): Define the standards for a "Verified" skill, including traceability to REQs/TESTs and author signatures.
- **Compile-time substitution framework** (`REQ-990`): Provide a secure, reliable, dynamic compile-time substitution library so that commands, statements, blocks of text, numbers, and patterns can be designated as compile-time dynamic. Leverage the spec-builder pattern to produce pre-compile-time objects that understand how to walk the code during a pre-build/generate phase and apply one or more substitutions once (with force-override for edge cases). Use cases: brand updates, logging level updates, user-facing command strings. The library should understand which files were modified and contain compile-time substitution markers; use stash/shelf for backup; attempt substitution and quick clean/compile for affected files; if compile and lint succeed, mark as applied in a cache. Optional: watch notifier with in-memory index; on mtime change invalidate entry, restore original, and optionally re-apply.

#### Goal: Prompt systemization and reuse (`[REDACTED-ID]`)
**Metric:** None | **Target:** None
Reusable, right-sized prompts with lifecycle, policy adherence, and audit support so the system provides skeleton and rigidity while enabling creative outcomes; prompt spec and examples delivered and incorporated.

**Fulfilling Requirements:**
- **Prompt spec and examples delivered with lifecycle, policy, and audit considerations** (`[REDACTED-ID]`): Deliver prompt_template spec (or formalized proposal), canonical prompt examples incorporated into the system, and documented considerations for prompt lifecycles, policy adherence, and auditing.

#### Goal: ZQK AI UX CLI Ergonomics (`[REDACTED-ID]`)
**Metric:** None | **Target:** None
Improve CLI ergonomics and fix pain points for AI agents and users.

#### Goal: End-to-End Orchestration Verification (`[REDACTED-ID]`)
**Metric:** None | **Target:** None
A goal to verify that the Hierarchical Chain of Integrity engine correctly forces spec compliance, and that subagents receive the appropriately hydrated prompt context.

**Fulfilling Requirements:**
- **Artifact Generation Requirement** (`[REDACTED-ID]`): The execution MUST yield an artifact named verification_success.md

#### Goal: Ambient Orchestration (`[REDACTED-ID]`)
**Metric:** None | **Target:** None
The overarching goal to implement and orchestrate the ambient sensory network and swarm cells.

#### Goal: Native Swarm Resilience & Scaffolding for Local LLMs (`[REDACTED-ID]`)
**Metric:** swarm_resilience_coverage | **Target:** 100%
Enhance the Native Swarm execution engine with strict validation loops, explicit token budgeting, and tool fallback mechanisms to prevent local models from silently failing or hallucinating.

**Fulfilling Requirements:**
- **Swarm Tool Normalization and Missing-Tool Feedback Loop** (`[REDACTED-ID]`): The Swarm Engine must normalize hallucinated tool names (e.g., stripping spaces to underscores) and provide explicit error feedback (e.g., "Tool 'X' not found. Did you mean Y?") when a local LLM calls a non-existent tool. This prevents the LLM from silently failing or repeating the mistake.
- **Explicit Token Budgeting and Context Limits for Native Swarm** (`[REDACTED-ID]`): The Swarm Engine must enforce token budgeting on all dynamically built prompts and explicitly configure the Ollama client's context window size (e.g., `num_ctx`) to prevent large payloads from truncating the LLM's memory or instruction set.

#### Goal: Public Launch Readiness and Interface Parity (`GOAL-LAUNCH-READINESS`)
**Metric:** system_check_failures | **Target:** 0 failures
Establish total interface consistency and unify the CRUD filesystem and Graph backend data paths before public release.

#### Goal: Open-Source Core Distribution Readiness (`GOAL-OSS-CORE`)
**Metric:** Percentage of implemented BLIs promoted to complete or archived with QA evidence | **Target:** 100% of implemented BLIs closed out
Ensure all implemented features are properly tracked and promoted through the lifecycle ahead of initial open-source release.

#### Goal: Ambient Orchestration (`GOAL-SYM-001`)
**Metric:** orchestration_efficiency | **Target:** 1 pass
Dissolving the boundary between human intent and machine execution via the Universal Autonomy Inbox.

**Fulfilling Requirements:**
- **Partner Demo Slide-Deck Outline** (`[REDACTED-ID]`): Draft the strategic narrative, split-screen flow, and messaging anchors for the impending partner briefings.
- **Planner Validation Gates & Notification Protocol** (`[REDACTED-ID]`): Strategic objects (priority_plan, workstream, milestone) must utilize the event bus and webhook callbacks to automatically transition their statuses when child conditions are met, matching the builder/doer agent task protocol.

#### Goal: Neurological Governance (`GOAL-SYM-002`)
**Metric:** governance_compliance | **Target:** 100%
Baking policy and governance into the transport layer using TDE envelopes.

#### Goal: Hive Mind Memory (`GOAL-SYM-003`)
**Metric:** sync_latency_ms | **Target:** 500ms
Shared distributed context across federated kernels with sub-500ms synchronization.

#### Goal: Anticipatory Workspace (`GOAL-SYM-004`)
**Metric:** workspace_prep_time_ms | **Target:** 500ms
The OS prepares the workspace (code, tests, docs) before the user commands it.

**Fulfilling Requirements:**
- **Strict TDE Validation** (`REQ-SYM-020`): All autonomous agent actions originating from a remote kernel or exceeding predefined local resource thresholds MUST be governed by a verified Trust-Domain Envelope (TDE).
- **Skill Integrity Proofs** (`REQ-SYM-021`): Agent skills transferred across the Sovereign Mesh MUST be packaged with a Merkle proof enabling the receiving node to independently verify the artifact integrity and author signature.
- **Real-time Workspace Observation** (`REQ-SYM-030`): The kernel MUST passively monitor the active workspace for high-signal filesystem and terminal events without degrading host performance.
- **Predictive Event Heuristics** (`REQ-SYM-031`): The kernel MUST map ambient workspace events to predefined intent heuristics to autonomously stage preemptive convergence sessions.

#### Goal: Skill Breeding (`GOAL-SYM-005`)
**Metric:** breeding_success_ratio | **Target:** 1.0 ratio
Autonomous agents combining and evolving new capabilities without human intervention.

#### Goal: Native Source Control Steward (`GOAL-SYM-006`)
**Metric:** steward_coverage | **Target:** 100%
Implement the GitHub Repo/Project Steward agent to automate PR life cycles, documentation, and repository orchestration.

**Fulfilling Requirements:**
- **Rigor in PR Audits** (`REQ-SYM-006-01`): Validate PRs against test bundles, enforce CI checks, respect semver. Must never force-push to main.
- **Efficiency via GraphQL API** (`REQ-SYM-006-02`): Must utilize GitHub GraphQL API for fetching delta payloads and operate within decoupled MCP boundaries.
- **Robustness under Swarm Load** (`REQ-SYM-006-03`): Must handle network timeouts, rate limits, and concurrent swarm pushes with intelligent queue management.
- **Observability and Traceability** (`REQ-SYM-006-04`): All actions (reviews, labels, merges) must leave a trace in the Knowledge Kernel, linking back to convergence_sessions.
- **Security and Supply Chain Audits** (`REQ-SYM-006-05`): Must hold scoped, short-lived tokens and perform supply-chain checks for leaked secrets in diffs.
- **Maintainability and Auto-Documentation** (`REQ-SYM-006-06`): Must auto-update CHANGELOG.md and RELEASE_NOTES.md, summarizing merged backlog items.

### 1.3 Active Work (Priority Plans & Backlog)
#### Priority Plan: Autonomous Agentic Marketing & Video Engine (`PRI-003`)
A priority plan for building a new ZQK CLI module that orchestrates a swarm of specialized AI agents. This module will autonomously analyze market trends, write compelling scripts, generate high-fidelity visuals, auto-clip assets, and dynamically edit the final video cut using seamless branding templates. Crucially, the video processing (FFmpeg stitching) MUST be offloaded to a separate, serverless 'Tool Pod Mesh' binary to align with ZQK's distributed pkg/mesh architecture. The boundary uses asynchronous job handoffs with state polling/webhooks, passing pointers via shared blob storage (e.g., S3) rather than raw payload data, and enforcing strict fault tolerance.

#### Priority Plan: Developer Experience: CLI Fluency & Resilience (`[REDACTED-ID]`)
Address critical CLI friction points identified during the Alpha launch stabilization phase. Focus on fault tolerance, non-blocking asynchronous behaviors, and seamless standalone execution.

#### Priority Plan: Phase 4: Sovereign Intelligence & Commercial Launch (`[REDACTED-ID]`)
Transition ZQK from an engineering prototype to a commercial product by packaging the Federated Sovereign Mesh—a secure, policy-bound ecosystem where multiple autonomous kernels collaborate across the enterprise.

#### Priority Plan: Phase 5.1: The Telemetry Mesh (High-Velocity Observability) (`[REDACTED-ID]`)
The Sovereign Mesh relies on rapid, cross-kernel event correlation. Standard CAS objects are too heavy for high-velocity metrics and logs. The Telemetry Mesh implements bypass-kernel JSONL streams, allowing real-time observability, prompt analytics, and performance tracing across the global neural network without suffocating the core GraphRAG database.

#### Priority Plan: Phase A: Cognitive Alignment (`[REDACTED-ID]`)
Implement agent-based policy negotiation and human-in-the-loom feedback loops.

#### Priority Plan: Ontology Expansion for Autonomous Kernel (`[REDACTED-ID]`)
Expand ZQK system ontology to support semantic-operational mapping and autonomous capability synthesis.

#### Priority Plan: Mission Control: Observability & Narrative Synthesis (`[REDACTED-ID]`)
Build the OmniTicker UI/UX and SemanticImpactNarrator to surface autonomous agent work in real-time.

#### Priority Plan: Phase 1: Enterprise Beachhead (`[REDACTED-ID]`)
Secure pilot partnerships with Fortune 500 companies to validate the Symbiotic Mesh at scale and drive early revenue.

#### Priority Plan: Phase 2: The Consumer Veneer (`[REDACTED-ID]`)
Abstract the CLI entirely for mainstream users. Build conversational and GUI layers so that non-technical users can interact with the Knowledge Kernel seamlessly.

#### Priority Plan: The God Demo (GTM Catalyst) (`[REDACTED-ID]`)
Automate a flawless, visually striking demonstration of ZQK executing a strategic plan. Utilize AppleScript for precise video and frame-by-frame screen capture to generate annotated marketing assets.

#### Priority Plan: Implement APISpec Builder Pattern (`[REDACTED-ID]`)
Implement the new APISpec Builder pattern to handle thread-safe API clients, telemetry, and resiliency.

#### Priority Plan: Tool Pod Mesh: Automated Retries and Sentinel Rollbacks (`[REDACTED-ID]`)
Fully integrate the newly built Sentinel Quality Gates with an automated retries and rollback mechanism within the Orchestrator.

#### Priority Plan: Deterministic Curriculum & Ambient Coach (`[REDACTED-ID]`)
Implement the zqk learn CLI command and Ambient Coach intercepts

#### Priority Plan: Implement Missing Requirement Verifications (`[REDACTED-ID]`)
Implement the draft and placeholder test cases to verify all requirements.

#### Priority Plan: Implement Missing Requirement Verifications (`[REDACTED-ID]`)
Implement the 132 missing test cases identified during the matrix generation

#### Priority Plan: Implement 3rd-Party Agent Init & Config Sync (`[REDACTED-ID]`)
Frictionless way to ensure that the kernel slurps up the relevant info buried in 3rd-party agent folders (gemini, agy, cursor, claude, etc.) to manage and maintain synchronicity without manual disambiguation, avoiding split-braining the agent.

#### Priority Plan: Release Pipeline Automation (`[REDACTED-ID]`)
Establish end-to-end autonomous merge and release pipeline. Blocked on GitHub account upgrade from free tier. Contains CI, branch protection, auto-merge, and release tagging backlog items.

#### Priority Plan: Drift-Control: Session CVS-BACKLOG-ALIGN Timeout (`[REDACTED-ID]`)
Convergence session exceeded 1 hour timeout. Automated re-alignment required.

#### Priority Plan: Drift-Control: Session [REDACTED-ID] Timeout (`[REDACTED-ID]`)
Convergence session exceeded 1 hour timeout. Automated re-alignment required.

#### Priority Plan: Zero-Hallucination Integrity Initiative (`[REDACTED-ID]`)
The immediate swarm priority: Build and verify the context onion prompts, native validation gates, and retroactive graph purification to establish an unbreakable Chain of Integrity.

#### Priority Plan: Tech Debt Remediation Plan (`[REDACTED-ID]`)
Overnight audit findings resolving POL-CODE-007 violations, ignored errors, unoptimized cypher queries, and missing test coverage.

#### Priority Plan: Phase 18: Full-Stack Autonomic Orchestration (`[REDACTED-ID]`)
Building upon Phase 17's Semantic CLI Routing and Phase 16's Autonomous Capability Synthesis, Phase 18 transitions the Sovereign Mesh into zero-touch autonomic operations. The orchestrator's CAP loop will be fully automated across distributed kernels. ZQK will proactively scan the environment, identify missing capabilities from the marketplace, synthesize solutions, validate them through the automated compliance engine, and deploy them natively—materializing continuous, unsupervised Sovereign Intelligence at scale.

#### Priority Plan: Implement Doer/Checker Paired-Agent Workflow & Verification Strategies (`[REDACTED-ID]`)
Expand the agent_task schema to embed task_steps and reusable verification strategies (metric_threshold, state_negation, command_exit_code, ast_semantic_match). Wire the state machine to leverage pkg/pipeline to enforce these constraints objectively before allowing tasks to advance to implemented.

#### Priority Plan: Information Architect Cleanup Plan (`PRI-IA-CLEANUP`)
Relocate non-system objects and random cruft out of .zqk/process/ to docs/ and clean up agent_skills.

#### Priority Plan: Phase 3 - Ambient Orchestration (`PRI-SYM-003`)
Anticipatory Logic moves ZQK from 'Do what I say' to 'Prepare for what I'm about to do.' It leverages ambient triggers to drive developer velocity.

#### Priority Plan: TDE Remediation Wave 1 (`PRI-TDE-WAVE-1`)
Mass cleanup of orphaned technical debt objects.

#### Priority Plan: TDE Remediation Wave 6 (`PRI-TDE-WAVE-6`)
Mass cleanup of orphaned technical debt objects.

#### Priority Plan: Phase 3 Strategy - Enterprise Scale & Distributed Kernel (`PRI-phase3`)
Planning out the future trajectory of ZQK, focusing on enterprise scale and distributed kernel networking.

#### Priority Plan: Phase 4 Strategy - Agent Orchestration & Autonomy (`PRI-phase4`)
Maximize multi-agent parallelization, driving efficiency and quality across the system.

#### Priority Plan: Phase 6: Enterprise Governance & Convergence (`PRI-phase6`)
Addressing prompt drift, observability blind spots, and multi-agent swarm thrashing through governance, telemetry, and structured convergence sessions.

## PART 2: Strict Object Schemas (The Source of Truth)
Every entity in ZQK is defined here. Note that all objects recursively inherit fields from their `Extends` base class (most commonly `base_object`).

### Schema: `base_object`
**Extends:** `auditable`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `actual_effort` | `string` | Actual effort expended (e.g., \"5.2 days\", \"32 hours\"). / Optional |
| `answer_due_by` | `string` | ISO-8601 date for when a question needs an answer. / Optional |
| `artifacts` | `list[string]` | Files/components touched by this object. / Optional |
| `completeness_validation` | `list[string]` | A programmatic Universal Verification DSL definition dictating exactly how this object's completeness is objectively verified. / Optional |
| `context` | `text` | Problem statement / motivation. / Optional |
| `deadline` | `string` | ISO-8601 date or datetime when the object must be completed. / Optional |
| `dependencies` | `list[string]` | Related object IDs this entity depends on. / Optional |
| `estimated_effort` | `string` | Estimated effort required (e.g., \"4-6 weeks\", \"2 days\", \"8 hours\", \"5 story points\"). / Optional |
| `id` | `string` | Stable identifier used across the graph (\"PREFIX-####\"). / Required |
| `kind` | `enum` | Declares ontology type (decision, requirement, etc.). / Required |
| `namespace_id` | `string` | Namespace identifier for this object (e.g., \"zqk:kernel\", \"domain:organizational\") / Optional |
| `priority_tier` | `enum['P0', 'P1', 'P2', 'P3']` | Priority tier tracking (P0/P1/P2/P3) for prioritization. / Optional |
| `questions` | `list[string]` | Prompt items that captured this object's data. / Optional |
| `related_object_refs` | `list[string]` | **GRAPH EDGE** / Optional links to other objects by stable ID (any kind). Use when typed *\_refs fields are not enough
(e.g. secondary priority plans, historical milestones, parallel tracks). Distinct from dependencies
(structural depends-on for impact analysis). Empty list when unused. / Optional |
| `schema_version` | `string` | SemVer indicating which object schema this instance conforms to. / Required |
| `source_type` | `enum['internal', 'external', 'imported']` | Indicates if the object originated internally, was imported, or provided by a user. / Optional |
| `spec_adherence` | `object` | Records the validation status/results for the current schema version. / Optional |
| `stakeholders` | `list[string]` | Personas/roles impacted. / Optional |
| `status` | `enum` | Lifecycle stage (\"proposed\", \"approved\", \"in_progress\", \"implemented\", \"archived\"). / Required |
| `status_history` | `list[string]` | List of StatusHistoryEntry records for auditing and lifecycle analysis. / Optional |
| `target_date` | `string` | ISO-8601 date (alternative to deadline) for target completion. / Optional |
| `title` | `string` | Human-readable name/summary. / Required |

### Schema: `account`
**Extends:** `base_object`
**Lifecycle:** `account_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `display_name` | `string` | Friendly name for UI/docs. / Optional |
| `email` | `string` | Contact/authentication email. / Optional |
| `id` | `string` | Stable identifier for the account. Supports both \"account:username\" format and \"ACC-####\" format. / Required |
| `profile_metadata` | `object` | Additional preferences (default profile, context cadence). / Optional |
| `roles` | `list[string]` | Roles assigned to this account. / Optional |
| `tokens` | `list[string]` | References to active auth tokens/credentials (local keystore, JWT fingerprint, etc.). / Optional |
| `username` | `string` | Login/display name. / Required |

### Schema: `agent_architecture`
**Extends:** `base_object`
**Lifecycle:** `agent_architecture_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `agent_type` | `enum['observer', 'test_agent', 'coder_agent', 'devops_agent', 'documentation_agent', 'optimization_agent']` | Type of agent this architecture defines. / Required |
| `components` | `list[string]` | List of components in this agent's architecture. / Optional |
| `id` | `string` | Stable identifier used across the graph (\"AGENT-ARCH-####\"). / Required |
| `integration_points` | `list[string]` | List of integration points (tendrils) for this agent. / Optional |
| `role_ref` | `string` | **GRAPH EDGE** / Reference to the role definition for this agent. / Optional |

### Schema: `agent_feed`
**Extends:** `base_object`
**Lifecycle:** `agent_feed_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `config_path_override` | `string` | When set, steward uses this path instead of the default datacell alias for config. / Optional |
| `contract_schema_version` | `string` | Schema version string for events appended for this feed (e.g. `1`). / Required |
| `delivery_mode` | `enum['off', 'log', 'clipboard', 'paste', 'notify']` | Coarse delivery mode for this feed binding. / Required |
| `enabled` | `bool` | When false, producers should not emit to the feed or paste into IDE. / Required |
| `events_jsonl_path_override` | `string` | When set, writers append here instead of the default datacell events path. / Optional |
| `note` | `text` | Free-form operator notes for this feed binding. / Optional |
| `probe_command_substrings` | `list[string]` | e.g. zqk, bin/zqk — narrows probe to CLI-shaped invocations. / Optional |
| `probe_tool_allowlist` | `list[string]` | When non-empty, only these tool_name values (e.g. Shell, run_terminal_cmd) may append probe JSONL; case-insensitive AND with probe_command_substrings when both set. / Optional |

### Schema: `agent_instruction`
**Extends:** `base_object`
**Lifecycle:** `agent_instruction_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `execution_log` | `list[string]` | Log of execution steps and results. / Optional |
| `instruction` | `text` | The actual prompt or proposal instruction text for the agent to execute. / Required |
| `source_session` | `string` | Convergence session that generated this instruction. / Optional |
| `status` | `enum['proposed', 'approved', 'rejected', 'in_progress', 'completed', 'error']` | Lifecycle status. / Required |
| `target_persona` | `string` | Optional persona reference intended to execute this instruction. / Optional |

### Schema: `agent_onboarding_preparation`
**Extends:** `base_object`
**Lifecycle:** `agent_onboarding_preparation_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `agent_type` | `enum['observer', 'test_agent', 'coder_agent', 'devops_agent', 'documentation_agent', 'optimization_agent']` | Type of agent being prepared for onboarding. / Required |
| `id` | `string` | Stable identifier used across the graph (\"AGENT-PREP-####\"). / Required |
| `preparation_status` | `enum['not_started', 'in_progress', 'ready', 'complete', 'blocked']` | Current status of preparation work. / Required |
| `preparation_tasks` | `list[string]` | Backlog items that represent preparation tasks. / Optional |
| `readiness_criteria` | `list[string]` | List of criteria that must be met before agent can onboard. / Optional |
| `target_date` | `date` | Target date for agent onboarding. / Optional |
| `target_workstream_ref` | `string` | **GRAPH EDGE** / Workstream where this agent will be activated. / Required |

### Schema: `agent_skill`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `instructions` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `instructions_summary` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `provider` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |

### Schema: `agent_task`
**Extends:** `base_object`
**Lifecycle:** `agent_task_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `assignee_persona_ref` | `string` | **GRAPH EDGE** / Reference to the persona assigned to execute this task. / Required |
| `inputs` | `list[string]` | References to objects or specs the agent requires to complete work. / Optional |
| `model_tier` | `string` | The model tier required to execute this task (e.g. tier_1_complex, tier_2_simple). / Optional |
| `outputs` | `list[string]` | References to objects or artifacts the agent produces. / Optional |
| `pipeline_ref` | `string` | **GRAPH EDGE** / Reference to the parent pipeline. / Optional |
| `policy_refs` | `list[string]` | **GRAPH EDGE** / References to policies that govern this task. / Optional |
| `requirement_refs` | `list[string]` | **GRAPH EDGE** / References to requirements driving this task. / Optional |
| `validation_criteria_refs` | `list[string]` | **GRAPH EDGE** / References to criteria objects that must pass for completion. / Optional |

### Schema: `application_record`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `cover_letter_content` | `text` | The generated cover letter. / Optional |
| `job_listing_id` | `string` | The ID of the target job listing. / Optional |

### Schema: `assessment_rating`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `persona_ref` | `string` | **GRAPH EDGE** / [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |
| `score` | `float` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |
| `skill_ref` | `string` | **GRAPH EDGE** / [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `status` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |

### Schema: `audit_aggregation_metric`
**Extends:** `base_metric`
**Lifecycle:** `audit_aggregation_metric_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `aggregated_event_ids` | `array` | Array of audit event IDs or ID ranges that were aggregated (for traceability). / Optional |
| `aggregation_window_end` | `string` | ISO 8601 timestamp of the end of the aggregation window. / Required |
| `aggregation_window_start` | `string` | ISO 8601 timestamp of the start of the aggregation window. / Required |
| `compression_ratio` | `number` | Compression ratio (events aggregated / metric size ratio). / Optional |
| `error_event_count` | `integer` | Total number of error-status events in the aggregation window. / Optional |
| `error_rate` | `number` | Ratio of error-status events to total events in the aggregation window. / Optional |
| `event_count` | `integer` | Total number of audit events aggregated into this metric. / Required |
| `event_type_counts` | `object` | Count of events by event_type (e.g., hash_regeneration: 5, integrity_recovery: 2). / Required |
| `metric_type` | `enum['system']` | Type of metric - always \"system\" for audit aggregations. / Required |
| `object_kind_counts` | `object` | Count of events by object_kind (e.g., backlog_item: 10, goal: 3). / Optional |
| `operation_counts` | `object` | Count of events by operation type (e.g., Regenerated hash: 5, Fixed integrity: 2). / Optional |
| `status_counts` | `object` | Count of events by audit_event status (e.g., completed: 120, failed: 4, error: 2). / Optional |

### Schema: `audit_event`
**Extends:** `base_object`
**Lifecycle:** `audit_event_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `aggregated_count` | `integer` | Total number of individual events aggregated into this summary event. / Optional |
| `aggregated_events` | `array` | Array of event summaries with counts for each event type/kind combination aggregated. / Optional |
| `aggregation_window` | `string` | Time range covered by aggregated events (ISO 8601 interval format, e.g., \"2025-12-25T14:00:00Z to 2025-12-25T15:00:00Z\"). / Optional |
| `event_type` | `enum['hash_mismatch_fix', 'hash_regeneration', 'integrity_recovery', 'object_deletion', 'object_quarantine', 'object_unquarantine', 'object_creation', 'object_update', 'object_move', 'permission_change', 'role_assignment', 'security_alert', 'system_config_change', 'cache_invalidation', 'cache_update', 'cache_bulk_invalidation', 'cache_operation', 'cache_availability', 'cache_cleanup', 'aggregated_summary', 'command_execution', 'code_quality_bypass', 'scheduler_job_started', 'scheduler_job_completed', 'scheduler_job_failed', 'listing_index_batch_start', 'listing_index_batch_complete', 'listing_index_batch_error', 'hash_registry_batch_start', 'hash_registry_batch_complete', 'hash_registry_batch_error', 'validation_start', 'validation_complete', 'validation_error', 'async_router_start', 'async_router_complete', 'async_router_error', 'operation_executor_start', 'operation_executor_complete', 'operation_executor_error', 'change_journal_created', 'change_journal_error', 'audit_buffer_flush_start', 'audit_buffer_flush_complete', 'audit_buffer_flush_error', 'orphan_cleanup_start', 'orphan_cleanup_complete', 'orphan_cleanup_error', 'service_start', 'service_stop', 'service_error', 'migration_start', 'migration_complete', 'migration_error', 'scenario_builder_start', 'scenario_builder_progress', 'scenario_builder_complete', 'scenario_builder_error', 'prompt_run_start', 'prompt_run_complete', 'prompt_run_error']` | Type of audit event (hash_mismatch_fix, hash_regeneration, integrity_recovery, etc.). / Required |
| `id` | `string` | Stable identifier for audit event (\"AUD-####\"). / Required |
| `metadata` | `object` | Additional structured metadata about the event (command flags, context, environment, etc.). / Optional |
| `new_value` | `string` | New value after the operation (e.g., new hash, new status). / Optional |
| `occurrence_count` | `integer` | Number of times this event occurred (for bulk operations). Defaults to 1 if not present. / Optional |
| `occurrence_timestamps` | `list[string]` | List of timestamps when this event occurred (for bulk operations). Append-only list similar to status_history. / Optional |
| `operation` | `string` | Human-readable description of the operation performed (e.g., \"Regenerated hash for BLI-626\", \"Recovered integrity for priority_plan\"). / Required |
| `original_value` | `string` | Original value before the operation (e.g., original hash, original status). / Optional |
| `preserved_samples` | `array` | Array of sample individual events preserved for forensics (if configured to preserve samples). / Optional |
| `reason` | `text` | Optional explanation of why the operation was performed (user-provided or system-generated). / Optional |
| `recovery_method` | `enum['auto-fix', 'manual', 'force', 'interactive', 'scheduled']` | Method used to perform the operation (auto-fix, manual, force, interactive, scheduled). / Optional |
| `severity` | `enum['low', 'medium', 'high', 'critical']` | Severity level of the event (low, medium, high, critical) for security analysis. / Optional |
| `status` | `enum` | Status of the audit event operation; allowed values and transitions from audit_event_lifecycle.yaml. / Required |
| `target_id` | `string` | ID of the specific object that was affected (e.g., \"BLI-626\", \"PRI-208\"). / Optional |
| `target_kind` | `string` | Object kind that was affected (e.g., \"backlog_item\", \"priority_plan\", \"requirement\"). / Optional |
| `target_path` | `string` | File path that was affected (e.g., \".zqk/process/backlog/BLI-626.yaml\"). / Optional |
| `title` | `string` | Optional human-readable title for the audit event (operation field is primary identifier). / Optional |

### Schema: `auditable`
**Extends:** `null`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `archived_at` | `datetime` | Timestamp when object was archived (if applicable). / Optional |
| `archived_by` | `string` | Who archived the object. / Optional |
| `change_log` | `list[string]` | Append-only record of meaningful changes. / Optional |
| `created_at` | `datetime` | Timestamp when the object was first materialized. / Required |
| `created_by` | `string` | Identifies the user/profile that created the object. / Required |
| `origin_project` | `string` | Human-readable project/workstream name where the object began. / Optional |
| `origin_system` | `string` | Where the object was originally authored (repo, external system). / Optional |
| `updated_at` | `datetime` | Last time any field changed. / Required |
| `updated_by` | `string` | User/profile responsible for last change. / Required |

### Schema: `auth_strategy`
**Extends:** `base_object`
**Lifecycle:** `auth_strategy_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `configuration` | `object` | Strategy-specific configuration (e.g., OAuth provider URL, keystore path) / Optional |
| `description` | `string` | Human-readable description of this authentication strategy / Optional |
| `enabled` | `boolean` | Whether this authentication strategy is enabled / Required |
| `priority` | `integer` | Priority order for this strategy (lower numbers checked first) / Required |
| `strategy_type` | `string` | Type of authentication strategy (keystore, oauth, username_password, personal_access_token, api_key) / Required |

### Schema: `auto_fix_rule`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `applies_to_kind` | `string` | Object kind this rule applies to (e.g. backlog_item, requirement). Required. / Required |
| `condition_category` | `string` | Issue category to match (e.g. instance_validation, reference, integrity). Empty = any. / Optional |
| `condition_message_contains` | `string` | Substring that issue message must contain. Empty = any. / Optional |
| `condition_rule` | `string` | Validation rule to match (e.g. lifecycle, required). Empty = any. / Optional |
| `condition_tier` | `integer` | Issue tier to match (1-4). 0 = any tier. / Optional |
| `enabled` | `boolean` | When false, rule is ignored. Default true. / Optional |
| `fix_command_template` | `string` | Fix command template. Placeholders: {object_id}, {kind}, {field}, {message}, {rule}, {tier}, {category}.
Example: "zqk object update {object_id} --field {field}=<VALUE>"
Or with query hint: "zqk object update {object_id} --field milestone_refs+=<MILESTONE_ID:category=feature>" / Required |
| `priority` | `integer` | Lower value = higher priority when multiple rules match. Default 100. / Optional |
| `title` | `string` | Human-readable label for this rule (e.g. "Backlog item lifecycle milestone_refs"). / Optional |

### Schema: `backlog_item`
**Extends:** `base_object`
**Lifecycle:** `backlog_item_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `acceptance_criteria` | `list[string]` | Verifiable conditions that must be met for the backlog item to be considered complete. Can contain either free-form text strings or criteria object references (CRIT-####) for traceability. / Optional |
| `benefits` | `list[string]` | List of benefits this feature provides (user value, technical value, etc.). / Optional |
| `category` | `string` | Category classification (e.g., \"Developer Experience\", \"Product\", \"Reliability\"). / Optional |
| `commit_hashes` | `list[string]` | **GRAPH EDGE** / Version control system commit identifiers that implement or relate to this backlog item. / Optional |
| `completed_at` | `string` | ISO-8601 datetime when the backlog item was completed. / Optional |
| `components` | `list[string]` | List of components or sub-features that make up this item. / Optional |
| `considerations` | `list[string]` | Implementation considerations, risks, or constraints. / Optional |
| `context` | `text` | When/why the idea surfaced (meeting, user feedback, technical debt, etc.). / Optional |
| `convergence_session_profile` | `string` | Short label for how the linked CVS is used (e.g. vetting_matrix, health, nested_orchestration). / Optional |
| `convergence_session_ref` | `string` | **GRAPH EDGE** / Optional link to the convergence session that owns or measures work for this backlog item. / Optional |
| `date_captured` | `date` | ISO-8601 date when the idea was first captured. / Optional |
| `description` | `text` | Detailed description of the feature or enhancement. / Optional |
| `document_refs` | `list[string]` | **GRAPH EDGE** / References to documents (paths or doc IDs) related to this backlog item. / Optional |
| `goal_refs` | `list[string]` | **GRAPH EDGE** / References to goals this backlog item supports. / Optional |
| `milestone_refs` | `list[string]` | **GRAPH EDGE** / References to milestones that implement this backlog item. Milestones are the primary link from
execution back to strategic context (mission, vision, goals, strategic plan); planned and
in_progress statuses require at least one milestone reference so work stays traceable. / Optional |
| `model_tier` | `string` | The model tier required to execute tasks generated from this backlog item (e.g. tier_1_complex, tier_2_simple). / Optional |
| `notes` | `text` | Additional notes, decisions, or context about this backlog item. / Optional |
| `persona_refs` | `list[string]` | **GRAPH EDGE** / Personas assigned or relevant to this backlog item. / Optional |
| `priority` | `enum['critical', 'high', 'medium', 'low']` | Priority level (critical, high, medium, low) aligned with priority tiers (P0=critical, P1=high, P2=medium, P3=low). / Optional |
| `priority_plan_ref` | `string` | **GRAPH EDGE** / Reference to the priority plan this backlog item belongs to (1:1 relationship, enforced). Required for items beyond exploring/validated status per DEC-priority-plan-ref-requirement. / Optional |
| `related_features` | `list[string]` | References to related backlog items or features (by ID or title). / Optional |
| `requirement_refs` | `list[string]` | **GRAPH EDGE** / References to requirements this backlog item fulfills. / Optional |
| `status` | `enum['exploring', 'validated', 'roadmap', 'deferred', 'planned', 'in_progress', 'complete', 'archived', 'rejected', 'error']` | Lifecycle status (exploring, validated, planned, in_progress, complete, archived, rejected). Note: 'workstream' status replaced with 'planned' per DEC-backlog-item-priority-plan-1-1-relationship. Statuses planned and in_progress require priority_plan_ref and milestone linkage so items cannot be worked under those statuses until traceability to the strategic hierarchy is in place. / Required |
| `workstream_refs` | `list[string]` | **GRAPH EDGE** / Workstreams this backlog item belongs to or contributes to. / Optional |

### Schema: `base_metric`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `batch_size` | `integer` | Batch size used when creating this metric from sampled events (only present if sampled=true). / Optional |
| `collection_count` | `integer` | Total number of times this metric has been collected. / Required |
| `context` | `object` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `field_name` | `string` | Field name that was aggregated to create this metric (e.g., \"duration_seconds\", \"status_history\"). / Optional |
| `first_seen` | `string` | ISO 8601 timestamp of first metric collection. / Required |
| `last_seen` | `string` | ISO 8601 timestamp of most recent metric collection. / Required |
| `metric_type` | `enum['command', 'performance', 'system', 'application', 'custom']` | Type of metric (command, performance, system, application, custom). / Required |
| `metric_type_specific` | `string` | Specific metric type from metrics pipeline (e.g., \"scalar_metric\", \"list_metric\", \"status_history_metric\"). / Optional |
| `object_count` | `integer` | Number of objects that were aggregated to create this metric. / Optional |
| `object_kind` | `string` | Object kind that was aggregated to create this metric (e.g., \"audit_event\", \"backlog_item\"). / Optional |
| `sampled` | `boolean` | Boolean indicating if this metric was created from sampled/batched events (true) or direct aggregation (false). / Optional |
| `source` | `string` | Source or origin of the metric (e.g., \"cli\", \"api\", \"scheduler\", \"event\"). / Optional |
| `tags` | `array` | Optional tags for categorizing and filtering metrics. / Optional |
| `window_end` | `string` | ISO 8601 timestamp of the end of the aggregation window. / Optional |
| `window_start` | `string` | ISO 8601 timestamp of the start of the aggregation window. / Optional |

### Schema: `base_sampler`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `batch_size` | `integer` | Number of events to collect before creating a metric object. Must be between 1 and max_batch_size. / Required |
| `description` | `string` | Human-readable description of this sampler configuration and its purpose. / Optional |
| `enabled` | `boolean` | Whether sampling is enabled for this configuration. If false, events are processed immediately without batching. / Required |
| `field_name` | `string` | Optional field name this sampler applies to. If null, applies to all fields of the object kind. / Optional |
| `flush_interval` | `string` | Maximum time to wait before flushing a partial batch (e.g., \"5m\", \"10m\", \"1h\"). Prevents data loss and memory buildup. / Required |
| `group_by_object_id` | `boolean` | If true, batches are per-object (objectID -> batch). If false, batches are global (all objects combined). / Required |
| `max_batch_size` | `integer` | Maximum allowed batch size (safety limit). Prevents excessive memory usage. / Required |
| `metric_type` | `string` | Metric type this sampler handles (e.g., \"system\", \"scalar_metric\", \"list_metric\", \"ordered_list_metric\", \"status_history_metric\"). / Required |
| `object_kind` | `string` | Object kind this sampler configuration applies to (e.g., \"audit_event\", \"change_journal_entry\"). / Required |

### Schema: `brand`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `brand_name` | `string` | Primary brand name identifier. The canonical name for this brand (e.g., 'zqk', 'WorkstreamOS'). Used as the primary reference for branding substitutions and system identification. / Required |
| `brand_variant` | `string` | Brand variant identifier. Distinguishes different variants of the same brand (e.g., 'primary', 'secondary', 'legacy', 'deprecated'). Enables multiple brand configurations for the same brand name with different scopes or contexts. / Optional |
| `dna_fields` | `array` | List of field names that are part of the DNA (propagated from source to clones). Fields listed here are inherited from the source configuration during propagation. Fields not listed are not part of the DNA and can be freely set on clones. Only applicable to DNA source configurations (dna_source_ref is null/empty). / Optional |
| `dna_source_ref` | `string` | **GRAPH EDGE** / Reference to the source configuration (the DNA). If null/empty, this configuration IS the DNA. If set, this is a clone that references the source DNA. Format: object ID (e.g., 'BRD-001') or object reference (e.g., 'brand:BRD-001'). / Optional |
| `dna_version` | `string` | Version/timestamp of the DNA being used by this clone. Set to source configuration's updated_at when clone is created or DNA is propagated. Used to detect when DNA has been updated and clone needs propagation. Format: ISO-8601 datetime (e.g., '2026-01-10T08:00:00Z'). / Optional |
| `metadata` | `object` | Brand metadata. Contains metadata for tagging and filtering: tags (array of meta tags), description (brand description), legal_status (legal status indicator), effective_date (when brand becomes effective), deprecated_date (when brand is deprecated). Enables meta tag-based filtering and pointer resolution for brand substitutions. / Optional |
| `overrides` | `object` | Field overrides that differ from the DNA. Contains field names and values that override the inherited DNA values. Only fields listed here override DNA; all other DNA fields are inherited. Overrides take precedence over DNA values during clone resolution. Only applicable to clone configurations (dna_source_ref is set). / Optional |
| `propagation_mode` | `enum['immediate', 'manual', 'scheduled', 'versioned']` | How updates to the DNA propagate to this clone. Options: immediate (changes propagate immediately when source is updated), manual (changes require explicit propagation command), scheduled (changes propagate on schedule, e.g., daily sync), versioned (clones track DNA version and can choose when to update). Only applicable to clone configurations (dna_source_ref is set). / Optional |
| `scope` | `object` | Brand scope definition. Defines where this brand applies: systems (list of system identifiers), components (list of component names), environments (list of environment names), paths (glob patterns for file paths), metadata_tags (list of meta tags for filtering). Enables multi-system branding with different brands for different contexts. / Optional |
| `substitution_patterns` | `array` | Substitution pattern references. Array of references (IDs or paths) to substitution configuration objects that define how to replace the current brand with this brand. Can reference substitution objects by ID or file path. Enables systematic branding changes across code, documentation, and configurations. / Required |

### Schema: `brand_asset`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `asset_type` | `enum['logo', 'font', 'color_palette', 'tagline', 'media', 'document', 'template']` | Type of the brand asset. / Required |
| `asset_url_or_path` | `string` | URI or file path to the asset. / Required |
| `brand_ref` | `string` | **GRAPH EDGE** / Reference to the parent brand. / Optional |
| `usage_guidelines` | `string` | Guidelines on how to use this asset. / Optional |

### Schema: `bucketing_strategy`
**Extends:** `auditable`
**Lifecycle:** `bucketing_strategy_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `applies_to` | `array` | List of object kinds this strategy applies to (e.g., [\"audit_event\", \"change_journal_entry\"]). / Optional |
| `archive_strategy` | `object` | Archive strategy configuration for long-term storage. Supports simple single-tier or complex multi-tier progression (warm → cold → iced). / Optional |
| `enabled` | `boolean` | Whether this strategy is currently enabled and active. / Optional |
| `field` | `string` | Object field name to extract bucket key from (e.g., \"created_at\", \"status\", \"size\"). / Optional |
| `format` | `string` | Go time format string for chronological strategies. Common formats: \"2006-01\" (monthly), \"2006-01-02\" (daily), \"2006-01-02T15\" (hourly), \"2006-01-02T15:04\" (half_hourly), \"2006-01-02T15:04:05\" (qtr_hourly), \"2006-01-02T15:04:05.9\" (tenths). Or use predefined granularity: \"monthly\", \"weekly\", \"daily\", \"hourly\", \"half_hourly\", \"qtr_hourly\", \"tenths\". / Optional |
| `retention_tolerance` | `object` | Configurable tolerance that triggers archive and cleanup when retention_tolerance job runs.
archive_after (duration, e.g. "24h", "30d") - objects older than this are marked status=archived.
cleanup_after (duration) - objects older than this are deleted (only if status not in protect_statuses).
max_count (integer, 0=no limit) - oldest objects not in protect_statuses are deleted until count <= max_count.
protect_statuses (array of strings) - active-like statuses that must not be removed; strategy does not delete objects in these statuses. If count cannot be reduced (all objects protected), job returns a blocking error requiring human intervention.
Aligns with .zqk/specs/configs/retention_tolerance.yaml; strategy-level value overrides config for applies_to kinds. / Optional |
| `strategy_name` | `string` | Human-readable name for this strategy instance (e.g., \"monthly\", \"status_based\"). / Optional |
| `strategy_type` | `enum` | Type of bucketing strategy (chronological, state, size, composite, first_letter). first_letter buckets by first character of a string field (e.g. title); used for glossary_term (A -> a, P -> p, etc.). / Optional |

### Schema: `capability`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `signature` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |

### Schema: `capacity_advertisement`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `availability_window` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `provider_kernel_ref` | `string` | **GRAPH EDGE** / [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `quantity` | `float` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `resource_id` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `resource_type` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `terms_ref` | `string` | **GRAPH EDGE** / [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `units` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |

### Schema: `certificate`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `credential_type` | `string` | Type of credential (signed_payload, encrypted_payload). Alpha uses self-signed CA; later may use instance-specific CA. / Optional |
| `expires_at` | `string` | ISO-8601 datetime when the certificate expires (null = no expiry for alpha) / Optional |
| `holder_ref` | `string` | **GRAPH EDGE** / Account that completed the scope and holds this certificate (e.g. account:developer) / Required |
| `issued_at` | `string` | ISO-8601 datetime when the certificate was issued / Required |
| `issuer_id` | `string` | Identifier of the issuer (CA id or instance id) so verification can use the correct public key. Instance-specific for alpha. / Required |
| `payload` | `string` | Opaque signed or encrypted payload (e.g. JWS or public-key encrypted blob). Contains attested claims; verification uses issuer public key. / Required |
| `scope` | `string` | Scope this certificate attests to (e.g. onboarding, software_tester_onboarding). May be standard across instances or instance-specific. / Required |

### Schema: `change_journal_entry`
**Extends:** `base_object`
**Lifecycle:** `change_journal_entry_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `change_type` | `enum['create', 'update', 'delete', 'import', 'health_check']` | Nature of the change (create, update, delete, import, health_check). / Required |
| `changed_paths` | `array` | Flattened dotted paths of changed fields (e.g. meta.tags, status) for analysis and compaction. / Optional |
| `diff_summary` | `text` | Summary of what changed (fields, old/new). / Optional |
| `object_ref` | `string` | **GRAPH EDGE** / Reference to the object that changed. / Required |
| `previous_state` | `object` | Snapshot of object state before change (for rollback). / Optional |

### Schema: `code_quality_metric`
**Extends:** `base_metric`
**Lifecycle:** `code_quality_metric_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `compliance_percentage` | `number` | Compliance percentage (0-100) for adherence metrics / Optional |
| `issue_count` | `integer` | Number of issues detected (for violation metrics) / Optional |
| `issue_type` | `string` | Type of issue detected (e.g., \"gofmt\", \"unused\", \"unparam\", \"gocyclo\", \"gocritic\") / Optional |
| `measurement_period` | `enum['daily', 'weekly', 'monthly']` | Measurement period for the metric (daily, weekly, monthly) / Required |
| `metric_category` | `enum['adherence', 'compliance', 'trend', 'violation', 'resolution']` | Category of code quality metric (adherence, compliance, trend, violation, resolution) / Required |
| `policy_ref` | `string` | **GRAPH EDGE** / Reference to policy being measured (POL-#### format, e.g., \"POL-CODE-009\") / Required |
| `resolution_count` | `integer` | Number of technical debt items resolved (for resolution metrics) / Optional |
| `tier` | `enum['tier_1', 'tier_2', 'tier_3']` | Issue tier for violation metrics (tier_1, tier_2, tier_3) / Optional |
| `trend_direction` | `enum['improving', 'stable', 'degrading']` | Trend direction for trend metrics (improving, stable, degrading) / Optional |

### Schema: `code_reference`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `author` | `string` | Author of the version control commit. / Optional |
| `author_email` | `string` | Email of the commit author. / Optional |
| `backlog_item_refs` | `list[string]` | **GRAPH EDGE** / Backlog items this code reference implements. / Optional |
| `change_type` | `string` | Type of change to the file (added, modified, deleted, renamed, copied). / Optional |
| `commit_date` | `string` | Date of the version control commit (ISO 8601 format). / Optional |
| `commit_hash` | `string` | Version control system commit identifier that introduced this code reference. / Optional |
| `file_path` | `string` | Path to the source file (relative to repo root or absolute). / Required |
| `function_name` | `string` | Name of the function/method being referenced. / Optional |
| `goal_refs` | `list[string]` | **GRAPH EDGE** / Goals this code reference supports. / Optional |
| `line_end` | `number` | Ending line number of the code reference (1-indexed, inclusive). / Optional |
| `line_start` | `number` | Starting line number of the code reference (1-indexed). / Required |
| `lines_added` | `number` | Number of lines added in this file change. / Optional |
| `lines_removed` | `number` | Number of lines removed in this file change. / Optional |
| `milestone_refs` | `list[string]` | **GRAPH EDGE** / Milestones this code reference supports. / Optional |
| `requirement_refs` | `list[string]` | **GRAPH EDGE** / Requirements this code reference implements. / Optional |
| `test_case_refs` | `list[string]` | **GRAPH EDGE** / Test cases that test this code reference. / Optional |
| `workstream_refs` | `list[string]` | **GRAPH EDGE** / Workstreams this code reference belongs to. / Optional |

### Schema: `command_metric`
**Extends:** `base_metric`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `avg_duration_seconds` | `number` | Average execution duration in seconds. / Required |
| `baseline_duration_seconds` | `number` | Baseline execution duration in seconds (used for timeout calculation). / Required |
| `command` | `string` | The original command string as executed. / Required |
| `error_rate` | `number` | Error rate as a percentage (0-100). / Required |
| `failure_count` | `integer` | Number of failed command executions. / Required |
| `fastest_duration_seconds` | `number` | Fastest execution duration in seconds. / Required |
| `first_seen` | `string` | ISO 8601 timestamp of first command execution (inherited from base_metric). / Required |
| `invocation_count` | `integer` | Total number of times this command has been executed. / Required |
| `last_seen` | `string` | ISO 8601 timestamp of most recent command execution (inherited from base_metric). / Required |
| `normalized_cmd` | `string` | Normalized command string with variable data replaced (e.g., file paths, IDs). / Required |
| `slowest_duration_seconds` | `number` | Slowest execution duration in seconds. / Required |
| `success_count` | `integer` | Number of successful command executions. / Required |
| `timeout_count` | `integer` | Number of times this command timed out. / Required |
| `timeout_rate` | `number` | Timeout rate as a percentage (0-100). / Required |

### Schema: `command_spec`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `flags` | `list[string]` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `group_id` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `long` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `short` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |
| `use` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |

### Schema: `commercial_sequence`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `conversion_goal` | `string` | The ultimate goal of the sequence. / Required |
| `narrative_refs` | `list[string]` | **GRAPH EDGE** / Narratives employed in this sequence. / Optional |
| `sequence_type` | `enum['sales_funnel', 'email_campaign', 'onboarding', 'advertisement']` | The type of sequence. / Required |
| `stages` | `list[string]` | The steps or stages in the sequence. / Required |

### Schema: `component`
**Extends:** `extensible_object`
**Lifecycle:** `component_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `child_component_refs` | `list[string]` | **GRAPH EDGE** / References to child component instances (varies by display_type) / Optional |
| `component_type` | `string` | The type of component (maps to component_types.yaml) / Required |
| `constraint_contexts` | `list[string]` | List of constraint contexts where this component participates / Optional |
| `dashboard_relationships` | `list[string]` | Dashboard-specific component relationships (widgets, panels, etc.) / Optional |
| `display_ref` | `string` | **GRAPH EDGE** / Reference to the display this component belongs to / Optional |
| `gantt_relationships` | `list[string]` | Gantt-specific component relationships (contains, references, etc.) / Optional |
| `kanban_relationships` | `list[string]` | Kanban-specific component relationships (columns, lanes, etc.) / Optional |
| `object_ref` | `string` | **GRAPH EDGE** / Reference to the underlying object (if component maps to object) / Optional |
| `parent_component_refs` | `list[string]` | **GRAPH EDGE** / References to parent component instances (varies by display_type) / Optional |
| `semantic_group_refs` | `list[string]` | **GRAPH EDGE** / References to semantic groups this component belongs to / Optional |

### Schema: `compression_policy`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `algorithm` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `omit_schema_defaults` | `bool` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `shared_ontology_refs` | `list[string]` | **GRAPH EDGE** / [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `threshold_bytes` | `integer` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |

### Schema: `compute_advertisement`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `capability_type` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `endpoint` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `metadata` | `map` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `provider_kernel_ref` | `string` | **GRAPH EDGE** / [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |

### Schema: `context_refresh_schedule`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `cadence` | `string` | Desired refresh interval (cron or ISO duration). / Required |
| `last_refresh` | `string` | Last time context was refreshed. / Optional |
| `next_refresh` | `string` | Next scheduled refresh time. / Optional |
| `target` | `string` | What the schedule applies to (\"profile:{id}\", \"project:{id}\", etc.). / Required |

### Schema: `convergence_session`
**Extends:** `base_object`
**Lifecycle:** `convergence_session_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `activity_log` | `list[string]` | Ordered list of activity entries. Each entry is a map with at least: timestamp (RFC3339), phase (string),
action (string), optional before_snapshot/after_snapshot (objects), delta_assessment, outcome_character,
notes. Enables replay and cross-session continuity. / Optional |
| `after_state_snapshot` | `object` | Latest measured state (e.g. failing fingerprints, health_watermark, test counts). Flexible object map. / Optional |
| `automation_hooks` | `string` | Optional hints for tooling (e.g. event names, job categories) when this session updates. / Optional |
| `backlog_item_refs` | `list[string]` | **GRAPH EDGE** / Backlog items this convergence session supports or is filed under. / Optional |
| `before_state_snapshot` | `object` | Baseline state at session start or iteration boundary (fingerprints, metrics, watermark). / Optional |
| `current_phase` | `enum['c1_scope', 'c2_triage', 'c3_order', 'c4_act', 'c5_verify', 'c6_exit']` | Current convergence phase (aligned with glossary C1–C6). / Required |
| `debrief_notes` | `text` | Post-close operator analysis: what worked, what to avoid, scheduler/queue tips, and how to run the
next convergence_session more efficiently. Filled at finalize; future sessions may reference this
object or copy excerpts into iteration_process / next_action on a new CVS. / Optional |
| `delta_assessment` | `enum['trending_toward', 'trending_away', 'neutral', 'unknown']` | Whether the latest measurement moves toward the desired end state. / Optional |
| `desired_end_state` | `text` | Target outcome in plain language (e.g. all scoped fingerprints pass in health). / Optional |
| `flow_variant` | `string` | Optional named branch flow (e.g. storage-heavy, scheduler-only) for specificity beyond the base C1–C6 path. / Optional |
| `glossary_term_ref` | `string` | **GRAPH EDGE** / Reference to glossary term (typically GLS-* for convergence lifecycle). / Optional |
| `hypothesis` | `text` | Preliminary claim to validate (what we believe is wrong or what fix will work). / Optional |
| `iteration_process` | `text` | Proposed iteration or loop body (human-readable); may reference PRE_CHANGE_CHECKLIST or bundle commands. / Optional |
| `last_measurement_at` | `string` | ISO-8601 timestamp of the last measurement or verify step. / Optional |
| `next_action` | `text` | One concrete next step for the current resource (or handoff). / Optional |
| `outcome_character` | `enum['pending', 'expected', 'surprising', 'catastrophic']` | Whether the latest result matched expectations, was surprising, or catastrophic relative to hypothesis. / Required |
| `predictions` | `object` | Structured predictions before iterations: e.g. expected_duration, time_estimate, expected_signal,
hypothesis_confidence. Flexible object map; tooling may standardize keys. / Optional |
| `requirement_refs` | `list[string]` | **GRAPH EDGE** / Requirements this session advances or validates. / Optional |
| `start_condition` | `text` | Starting trigger or acceptance predicate (what makes this session applicable). / Optional |
| `status` | `enum['draft', 'active', 'paused', 'completed', 'abandoned', 'escalated', 'error']` | Session lifecycle status. / Required |
| `thresholds` | `object` | Adjustable thresholds (max iterations, time budget, flake tolerance, min pass rate). Flexible object map. / Optional |

### Schema: `corporate_initiative`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `aggregated_db` | `object` | Aggregated Dependencies & Blockers result across all projects. / Optional |
| `aggregated_edd` | `number` | Aggregated Effort Distribution Discrepancy across all projects. / Optional |
| `aggregated_pcs` | `number` | Aggregated Project Confidence Score (0-100) across all projects in this initiative. / Optional |
| `aggregation_status` | `enum['success', 'partial', 'failed']` | Status of last aggregation attempt (success/partial/failed). / Optional |
| `cross_project_dependencies` | `list[string]` | Cross-project dependency relationships (e.g., \"project:A depends on project:B\"). / Optional |
| `last_aggregated_at` | `string` | ISO-8601 timestamp when metrics were last aggregated. / Optional |
| `owner_ref` | `string` | **GRAPH EDGE** / Reference to account/role representing the initiative owner. / Optional |
| `project_refs` | `list[string]` | **GRAPH EDGE** / References to Workstream OS instances/projects that are part of this initiative. / Optional |
| `resource_allocation` | `object` | Resource allocation across projects (team assignments, budget, etc.). / Optional |

### Schema: `criteria`
**Extends:** `base_object`
**Lifecycle:** `criteria_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `backlog_item_refs` | `list[string]` | **GRAPH EDGE** / Backlog items this criterion validates. / Optional |
| `category` | `enum['functional', 'non-functional', 'acceptance', 'test', 'performance', 'security', 'compliance']` | Type of criterion (functional, non-functional, acceptance, test, performance, security, compliance). See criteria-categories-v1.0.md for definitions. / Required |
| `goal_refs` | `list[string]` | **GRAPH EDGE** / Goals this criterion validates. / Optional |
| `milestone_refs` | `list[string]` | **GRAPH EDGE** / Milestones this criterion validates. / Optional |
| `priority` | `enum['critical', 'high', 'medium', 'low']` | Priority level (critical, high, medium, low) for validation ordering. / Optional |
| `requirement_refs` | `list[string]` | **GRAPH EDGE** / Requirements this criterion validates. / Optional |
| `status` | `enum['not_started', 'in_progress', 'validated', 'complete', 'blocked', 'rejected']` | Validation status of the criterion. / Required |
| `validation_method` | `enum['manual_check', 'automated_test', 'metric_threshold', 'code_review', 'external_approval']` | How the criterion is validated (manual_check, automated_test, metric_threshold, code_review, external_approval). / Optional |
| `validation_threshold` | `string` | Target value for metric-based validation (e.g., \"80%\", \"100ms\", \"0 errors\"). / Optional |

### Schema: `decision`
**Extends:** `base_object`
**Lifecycle:** `decision_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `decision_refs` | `list[string]` | **GRAPH EDGE** / References to related or dependent decisions. / Optional |
| `goal_refs` | `list[string]` | **GRAPH EDGE** / Goals impacted by this decision. / Optional |
| `id` | `string` | Stable identifier for decision objects (DEC-### or ADR-### format). ADR-### format is used for Architecture Decision Records. / Required |
| `impact` | `text` | Summarize downstream effects/outcomes of the decision. / Optional |
| `impact_level` | `enum['primary', 'secondary', 'observed']` | Strength of goal linkage (\"primary\", \"secondary\", \"observed\"). / Optional |
| `milestone_refs` | `list[string]` | **GRAPH EDGE** / Milestones impacted by this decision. / Optional |
| `requirement_refs` | `list[string]` | **GRAPH EDGE** / Requirements impacted by this decision. / Optional |
| `revisit` | `string` | When to revisit/reassess the decision. / Optional |
| `workstream_refs` | `list[string]` | **GRAPH EDGE** / Workstreams impacted by this decision. / Optional |

### Schema: `department`
**Extends:** `extensible_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `department_name` | `string` | The name of the department / Required |
| `division_ref` | `reference` | **GRAPH EDGE** / Reference to the division this department belongs to (optional if department is directly under organization) / Optional |
| `kernel_goals_refs` | `list[string]` | **GRAPH EDGE** / References to kernel goals that align with this department / Optional |
| `team_refs` | `list[string]` | **GRAPH EDGE** / References to teams that belong to this department / Optional |

### Schema: `digital_asset`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `lineage` | `string` | Asset lineage. / Required |
| `metadata` | `string` | Asset metadata. / Required |
| `quality_metrics` | `string` | Quality metrics. / Required |
| `storage` | `string` | Asset storage details. / Required |

### Schema: `display`
**Extends:** `extensible_object`
**Lifecycle:** `display_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `component_refs` | `list[string]` | **GRAPH EDGE** / References to component instances used in this display / Optional |
| `constraint_contexts` | `list[string]` | List of constraint contexts where this display participates / Optional |
| `display_type` | `string` | The type of display (maps to display_types.yaml) / Required |
| `layout_config` | `object` | Display-specific layout configuration (varies by display_type) / Optional |

### Schema: `division`
**Extends:** `extensible_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `child_division_refs` | `list[string]` | **GRAPH EDGE** / References to child divisions within this division / Optional |
| `division_name` | `string` | The name of the division / Required |
| `kernel_goals_refs` | `list[string]` | **GRAPH EDGE** / References to kernel goals that align with this division / Optional |
| `parent_division_ref` | `reference` | **GRAPH EDGE** / Reference to the parent division (null for top-level divisions) / Optional |
| `team_refs` | `list[string]` | **GRAPH EDGE** / References to teams that belong to this division / Optional |

### Schema: `doc_entry`
**Extends:** `base_object`
**Lifecycle:** `doc_entry_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `category` | `string` | Fine-grained category for document classification (e.g., \"validation\", \"graph-backend\", \"semantic-types\"). / Optional |
| `content_searchable` | `boolean` | Flag indicating if document content should be indexed for full-text search. / Optional |
| `goal_refs` | `list[string]` | **GRAPH EDGE** / Goals this document relates to. / Optional |
| `group` | `enum['project_goals', 'project_specific', 'tooling', 'process', 'onboarding', 'design', 'architecture', 'other']` | Category/group the document belongs to (project_goals, project_specific, tooling, architecture, etc.). / Optional |
| `milestone_refs` | `list[string]` | **GRAPH EDGE** / Milestones this document relates to. / Optional |
| `path` | `string` | File system path or URL to the document. / Required |
| `requirement_refs` | `list[string]` | **GRAPH EDGE** / Requirements this document relates to. / Optional |
| `summary` | `string` | Brief description of the document's content and purpose. / Required |
| `workstream_refs` | `list[string]` | **GRAPH EDGE** / Workstreams this document relates to. / Optional |

### Schema: `domain_registry`
**Extends:** `base_object`
**Lifecycle:** `domain_registry_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `domains` | `list[string]` | List of registered domain ontologies with their metadata. / Required |
| `id` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |

### Schema: `economic_policy`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `cost_per_query` | `integer` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `max_credit_limit` | `integer` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `settlement_currency` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `storage_rent_per_mb` | `integer` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |

### Schema: `evolution_management`
**Extends:** `base_object`
**Lifecycle:** `evolution_management_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `adaptive_adjustments` | `list[string]` | List of adaptive adjustments made based on chaos indicators. / Optional |
| `chaos_indicators` | `list[string]` | List of chaos indicators being monitored (graph growth rate, agent onboarding velocity, etc.). / Optional |
| `period` | `string` | Time period covered by this evolution management (e.g., \"2026-01-01 to 2026-03-31\"). / Required |
| `strategy` | `enum['adaptive', 'conservative', 'aggressive', 'balanced']` | Strategy for managing evolution (adaptive, conservative, aggressive, balanced). / Required |

### Schema: `extensible_object`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `domain` | `string` | Identifies the external domain this object belongs to / Required |
| `lifecycle_ref` | `string` | **GRAPH EDGE** / Explicit reference to lifecycle definition file (optional - defaults to naming convention) / Optional |
| `spec_context_broker` | `string` | Identifier for the context broker that provides domain context / Required |
| `spec_interpreter` | `string` | Identifier for the spec interpreter that handles this domain / Required |

### Schema: `field_registry`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `active_fields` | `list[string]` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |

### Schema: `file_lock_metric`
**Extends:** `base_metric`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `avg_acquisition_time_ms` | `number` | Average time to acquire a lock in milliseconds. / Required |
| `avg_wait_time_ms` | `number` | Average wait time in milliseconds (including timeouts). / Required |
| `contention_rate` | `number` | Contention rate as a percentage (0-100). / Required |
| `max_acquisition_time_ms` | `number` | Maximum time to acquire a lock in milliseconds. / Required |
| `max_wait_time_ms` | `number` | Maximum wait time in milliseconds (including timeouts). / Required |
| `measurement_window_end` | `string` | ISO 8601 timestamp of measurement window end. / Required |
| `measurement_window_start` | `string` | ISO 8601 timestamp of measurement window start. / Required |
| `peak_contention` | `integer` | Maximum number of concurrent lock attempts observed. / Required |
| `success_rate` | `number` | Success rate as a percentage (0-100). / Required |
| `total_acquisitions` | `integer` | Total number of successful lock acquisitions. / Required |
| `total_contention` | `integer` | Total number of times lock was already held (TryLock returned false). / Required |
| `total_failures` | `integer` | Total number of failed lock attempts (errors). / Required |
| `total_timeouts` | `integer` | Total number of timeout failures. / Required |

### Schema: `fission_event`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `child_node_id` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `parent_node_id` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `reason` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `snapshot_id` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `status` | `enum` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |

### Schema: `glossary_term`
**Extends:** `base_object`
**Lifecycle:** `glossary_term_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `agent_prompts` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |
| `alias_refs` | `list[string]` | **GRAPH EDGE** / [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `category` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |
| `context_scope` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |
| `definition` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |
| `machine_hints` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |
| `semantic_tags` | `list[string]` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |

### Schema: `glossary_term_relation`
**Extends:** `base_object`
**Lifecycle:** `glossary_term_relation_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `notes` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `predicate_ref` | `string` | **GRAPH EDGE** / [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |
| `scheme_ref` | `string` | **GRAPH EDGE** / [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |
| `sort_order` | `integer` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `source_term_ref` | `string` | **GRAPH EDGE** / [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |
| `target_term_ref` | `string` | **GRAPH EDGE** / [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |

### Schema: `goal`
**Extends:** `base_object`
**Lifecycle:** `goal_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `achieved_at` | `string` | ISO-8601 datetime when the goal was achieved. / Optional |
| `authority` | `string` | Role/person responsible for goal achievement. / Optional |
| `backlog_item_refs` | `list[string]` | **GRAPH EDGE** / Backlog items that contribute to achieving this goal. / Optional |
| `commit_hashes` | `list[string]` | **GRAPH EDGE** / Version control system commit identifiers that implement or relate to this goal. / Optional |
| `current_value` | `string` | Current measured value for the metric. / Optional |
| `deadline` | `string` | ISO-8601 date or datetime when the goal must be achieved. / Optional |
| `metric` | `string` | Human-readable metric description (may mirror template). / Optional |
| `metric_template_id` | `string` | Reference to metric template governing measurement. / Optional |
| `milestone_refs` | `list[string]` | **GRAPH EDGE** / Milestones that contribute to achieving this goal. / Optional |
| `requirement_refs` | `list[string]` | **GRAPH EDGE** / Requirements this goal fulfills or relates to. / Optional |
| `target` | `string` | Success threshold for the metric. / Optional |
| `workstream_refs` | `list[string]` | **GRAPH EDGE** / Workstreams that contribute to achieving this goal. / Optional |

### Schema: `impact_analysis`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `affected_objects` | `map` | Map of affected kernel objects organized by type (workstreams, goals, backlog_items, milestones, etc.) / Optional |
| `change_ref` | `string` | **GRAPH EDGE** / Reference to the change object that triggered this impact analysis / Optional |
| `change_type` | `string` | Type of change that triggered this analysis (e.g., division_restructure, schema_change, organizational_change) / Optional |
| `impact_categories` | `list[string]` | List of impact categories with severity, affected count, and descriptions / Optional |
| `recommended_actions` | `list[string]` | List of recommended actions to address the impacts, including action type, priority, and affected objects / Optional |

### Schema: `import_tracking`
**Extends:** `base_object`
**Lifecycle:** `import_tracking_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `id` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `imported_at` | `string` | ISO-8601 timestamp when the import was recorded / Optional |
| `source_file` | `string` | Path or identifier of the source file that was imported / Optional |
| `source_format` | `string` | Detected or specified format (e.g. turtle, rdf_owl, jsonld) / Optional |
| `status` | `string` | Import status (ready, translated, failed) / Optional |

### Schema: `important_date`
**Extends:** `base_object`
**Lifecycle:** `important_date_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `date` | `string` | ISO-8601 date (e.g. 2026-03-31). / Optional |
| `date_type` | `string` | Type of date (e.g. deadline, milestone, market_window). / Optional |
| `dependencies` | `list[string]` | Free-form dependency descriptions (e.g. MIL-010 must complete by 2026-03-15). / Optional |
| `goal_refs` | `list[string]` | **GRAPH EDGE** / Goals related to this date. / Optional |
| `impact_scope` | `string` | Scope of impact (e.g. project_wide, team, release). / Optional |
| `importance` | `string` | Importance level (e.g. critical, high, medium). / Optional |
| `milestone_refs` | `list[string]` | **GRAPH EDGE** / Milestones related to this date. / Optional |
| `stakeholder_notifications` | `list[string]` | List of {stakeholder, notification_days_before} for reminders. / Optional |

### Schema: `inference_heuristic`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `condition_indicator_type` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `proposed_description` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |
| `proposed_priority` | `enum['P0', 'P1', 'P2', 'P3']` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |
| `proposed_title` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |
| `target_maturity_level` | `integer` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |

### Schema: `infrastructure_adapter`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `assigned_specialization` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `credentials_ref` | `string` | **GRAPH EDGE** / [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `endpoint` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `proxy_mode` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `throughput_capacity` | `float` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `units` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `utility_type` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |

### Schema: `integrity_manifest`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `entries` | `list[string]` | Hash entries (target reference + hash + timestamp). / Optional |
| `hash_algorithm` | `string` | Algorithm used (sha256, blake3, etc.). / Optional |
| `scope` | `list[string]` | Files/objects included in the manifest (paths, object IDs). / Required |

### Schema: `job_listing`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `company_name` | `string` | The name of the company offering the job. / Optional |
| `curation_status` | `enum` | Status of the job listing (e.g., discovered, reviewed, interested, applied, rejected). / Required |
| `description` | `text` | The full description of the job. / Optional |
| `job_title` | `string` | The title of the job listing. / Optional |
| `job_url` | `string` | The URL to the original job posting. / Optional |
| `match_score` | `integer` | Automated match score against user profile. / Optional |

### Schema: `job_search_profile`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `enabled` | `boolean` | Whether this search profile is actively running. / Optional |
| `excluded_companies` | `list[string]` | Companies to exclude from search results. / Optional |
| `keywords` | `list[string]` | Keywords to search for in job titles or descriptions. / Optional |
| `locations` | `list[string]` | Target locations for the job search (e.g., 'Remote', 'San Francisco, CA'). / Optional |
| `match_threshold` | `integer` | Minimum match score (0-100) required to automatically save a job listing. / Optional |
| `minimum_salary` | `integer` | Minimum acceptable salary. / Optional |

### Schema: `keystore_entry`
**Extends:** `base_object`
**Lifecycle:** `keystore_entry_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `account_id` | `string` | Account ID this key belongs to (e.g., \"account:developer\") / Required |
| `credential_hash` | `string` | Hashed credential (bcrypt for passwords, SHA256 for tokens). Never store raw credentials. Only system can read this field. / Required |
| `description` | `string` | Human-readable description of this key (e.g., \"MCP authentication key\", \"API access token\") / Optional |
| `expires_at` | `string` | ISO-8601 timestamp when this key expires (null = never expires) / Optional |
| `key_type` | `string` | Type of credential (password, oauth_token, personal_access_token, api_key) / Required |
| `last_used_at` | `string` | ISO-8601 timestamp of last successful authentication using this key / Optional |
| `revoked` | `boolean` | Whether this key has been revoked (revoked keys cannot be used for authentication) / Required |
| `revoked_at` | `string` | ISO-8601 timestamp when this key was revoked / Optional |
| `salt` | `string` | Salt for password hashing (if not using bcrypt which has built-in salt). Only system can read this field. / Optional |

### Schema: `kind_mapping_metric`
**Extends:** `base_metric`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `backend_config_merges` | `integer` | Total number of backend config merges performed. / Required |
| `backend_switches` | `integer` | Total number of backend type switches (file to graph, etc.). / Required |
| `cache_hits` | `integer` | Total number of cache hits (lookups that found cached mappings). / Required |
| `cache_misses` | `integer` | Total number of cache misses (lookups that required computation). / Required |
| `config_load_duration_ms` | `number` | Total time spent loading configs in milliseconds. / Required |
| `config_load_failures` | `integer` | Total number of failed config loads. / Required |
| `config_loads` | `integer` | Total number of successful config loads. / Required |
| `directories_scanned` | `integer` | Total number of directories scanned during initialization. / Required |
| `directory_lookups` | `integer` | Total number of GetDirectoryFromKind lookup operations. / Required |
| `discovery_errors` | `integer` | Total number of errors encountered during discovery/initialization. / Required |
| `inference_rule_hits` | `integer` | Total number of times inference rules were used to determine mappings. / Required |
| `initialization_duration_ms` | `number` | Total time spent initializing in milliseconds. / Required |
| `initializations` | `integer` | Total number of Initialize() operations performed. / Required |
| `kind_lookups` | `integer` | Total number of GetKindFromDirectory lookup operations. / Required |
| `lookup_errors` | `integer` | Total number of errors encountered during lookup operations. / Required |
| `mappings_discovered` | `integer` | Total number of kind-to-directory mappings discovered. / Required |
| `max_initialization_time_ms` | `number` | Maximum initialization time in milliseconds (worst-case performance). / Required |
| `measurement_window_end` | `string` | ISO 8601 timestamp of measurement window end. / Required |
| `measurement_window_start` | `string` | ISO 8601 timestamp of measurement window start. / Required |
| `specs_scanned` | `integer` | Total number of spec files scanned during initialization. / Required |

### Schema: `kind_synonym`
**Extends:** `null`
**Lifecycle:** `kind_synonym_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `convention` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `description` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `id` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `priority` | `integer` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `synonym` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `target_kind` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |

### Schema: `library`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `composable_specs` | `array` | References to composable specifications. Array of references (IDs or ontology names) to specifications that comprise this library. Specs can comprise specs that comprise specs, enabling hierarchical composition. Each spec layer provides more granular components. / Optional |
| `connector_patterns` | `array` | Connector patterns defined in this library. Array of connector pattern identifiers that describe how components connect and compose. Connectors form the basis for building out semantic libraries that describe architectural patterns. / Optional |
| `dna_fields` | `array` | List of field names that are part of the DNA (propagated from source to clones). Fields listed here are inherited from the source configuration during propagation. Fields not listed are not part of the DNA and can be freely set on clones. Only applicable to DNA source configurations (dna_source_ref is null/empty). / Optional |
| `dna_source_ref` | `string` | **GRAPH EDGE** / Reference to the source configuration (the DNA). If null/empty, this configuration IS the DNA. If set, this is a clone that references the source DNA. Format: object ID (e.g., 'LIB-001') or object reference (e.g., 'library:LIB-001'). / Optional |
| `dna_version` | `string` | Version/timestamp of the DNA being used by this clone. Set to source configuration's updated_at when clone is created or DNA is propagated. Used to detect when DNA has been updated and clone needs propagation. Format: ISO-8601 datetime (e.g., '2026-01-10T08:00:00Z'). / Optional |
| `library_name` | `string` | Primary library name identifier. The canonical name for this library (e.g., 'connector_library', 'semantic_patterns', 'architecture_components'). Used as the primary reference for library composition and semantic compression. / Required |
| `library_type` | `string` | Library type/category. Distinguishes different types of libraries (e.g., 'architectural_patterns', 'connectors', 'semantic_components', 'composable_specs'). Enables type-based filtering and composition rules. / Optional |
| `metadata` | `object` | Library metadata. Contains metadata for tagging and filtering: tags (array of meta tags), description (library description), version (library version), status (library status indicator), effective_date (when library becomes effective), deprecated_date (when library is deprecated). Enables meta tag-based filtering and pointer resolution for library composition. / Optional |
| `overrides` | `object` | Field overrides that differ from the DNA. Contains field names and values that override the inherited DNA values. Only fields listed here override DNA; all other DNA fields are inherited. Overrides take precedence over DNA values during clone resolution. Only applicable to clone configurations (dna_source_ref is set). / Optional |
| `propagation_mode` | `enum['immediate', 'manual', 'scheduled', 'versioned']` | How updates to the DNA propagate to this clone. Options: immediate (changes propagate immediately when source is updated), manual (changes require explicit propagation command), scheduled (changes propagate on schedule, e.g., daily sync), versioned (clones track DNA version and can choose when to update). Only applicable to clone configurations (dna_source_ref is set). / Optional |
| `scope` | `object` | Library scope definition. Defines where this library applies: systems (list of system identifiers), components (list of component names), environments (list of environment names), paths (glob patterns for file paths), metadata_tags (list of meta tags for filtering). Enables multi-system library usage with different libraries for different contexts. / Optional |
| `semantic_structure` | `object` | Semantic structure definition. Defines the semantic stack structure for this library, enabling significant data compression by understanding how components relate semantically. Contains semantic token mappings, hierarchy definitions, and compression strategies. / Optional |

### Schema: `lifecycle`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `extends` | `string` | Parent lifecycle to extend (e.g., "base_lifecycle"). Child lifecycle inherits parent statuses and transitions, with child overrides. / Optional |
| `id` | `string` | Stable identifier for lifecycle objects (LIFECYCLE-{ABBR}-### format). / Required |
| `object_type` | `string` | The object kind this lifecycle defines states for (e.g., "backlog_item", "goal", "milestone") / Required |
| `percent_complete` | `object` | Configuration for calculating percent complete. Supports methods: "status_defaults", "milestone_based", or custom calculation. / Optional |
| `source_type` | `enum['built-in', 'internal']` | "built-in" for immutable defaults (generated from builders), "internal" for overridable objects / Optional |
| `status_mapping` | `object` | Maps internal (base) statuses to external (this lifecycle) statuses. Used when this lifecycle extends another. / Optional |
| `statuses` | `list[string]` | List of valid statuses for this object type. Each status defines value, display, initial/terminal/archive/system flags, and preconditions. / Required |
| `transitions` | `list[string]` | Valid state transitions. Each transition defines from/to statuses, description, manual/auto flags, and preconditions. / Optional |

### Schema: `list_metric_sampler`
**Extends:** `base_sampler`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `batch_size` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `flush_interval` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `group_by_object_id` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `max_batch_size` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `metric_type` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |

### Schema: `maturation_report`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `component_id` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |
| `fitness_score` | `float` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |
| `graduation_status` | `enum['maturation', 'ready', 'promoted']` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |
| `observation_duration` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |

### Schema: `mcp_session`
**Extends:** `base_object`
**Lifecycle:** `mcp_session_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `account_id` | `string` | Account ID once authenticated (e.g., \"account:cursor-vscode\") / Optional |
| `authentication_status` | `string` | Current authentication status (pending_authentication, authenticated, expired, revoked) / Required |
| `client_id` | `string` | Unique identifier for the MCP client connection / Required |
| `client_name` | `string` | Human-readable name of the MCP client (e.g., \"cursor-vscode\") / Optional |
| `id` | `string` | Stable identifier for an MCP session (supports client sessions and agent/test sessions) / Required |
| `last_activity` | `datetime` | ISO-8601 timestamp of last MCP activity / Optional |
| `permissions` | `array` | Permissions granted to this session after authentication / Optional |
| `roles` | `array` | Roles assigned to this session after authentication / Optional |
| `status` | `enum['in_progress', 'disconnected', 'archived', 'error']` | Session lifecycle stage (in_progress while connected; disconnected on EOF/timeout/shutdown; eligible for retention when not in_progress) / Required |
| `title` | `string` | Optional human-readable label for the session / Optional |

### Schema: `metadata_package`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `collected_at` | `string` | ISO-8601 timestamp when this metadata package was collected. / Required |
| `metrics` | `object{metric keys and values.}` | Collected metrics data (codebase health, test coverage, etc.). / Optional |
| `scope` | `enum['workstream', 'milestone', 'project']` | The scope of this metadata package (workstream, milestone, or project). / Required |
| `scope_id` | `string` | ID of the workstream or milestone this package is for (empty for project scope). / Optional |

### Schema: `metrics_exchange_contract`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `enabled` | `boolean` | Global on/off switch for facade client metrics exchange. / Required |
| `field_mapping` | `object` | Optional key-map and include/exclude profile for external payload shape. / Optional |
| `format` | `enum['json', 'yaml', 'protobuf', 'avro']` | Outbound payload format for metrics exchange events. / Required |
| `operation_scope` | `list[string]` | Optional operation allow-list (for example push, commit). / Optional |
| `provider_scope` | `list[string]` | Optional provider allow-list (for example git). / Optional |
| `sample_rate` | `number` | Fraction of eligible events to emit (0.0 to 1.0). / Required |
| `sink_kind` | `enum['file', 'stream', 'http']` | Sink adapter type for contract events. / Required |
| `sink_target` | `string` | Target descriptor for chosen sink kind. / Optional |

### Schema: `metrics_feedback`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `analysis` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `metric_refs` | `list[string]` | **GRAPH EDGE** / [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `suggested_actions` | `list[string]` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `target_report` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |

### Schema: `milestone`
**Extends:** `base_object`
**Lifecycle:** `milestone_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `backlog_item_refs` | `list[string]` | **GRAPH EDGE** / Backlog items that contribute to this milestone. / Optional |
| `blocked_by_refs` | `list[string]` | **GRAPH EDGE** / Milestones or work items blocking this milestone. / Optional |
| `commit_hashes` | `list[string]` | **GRAPH EDGE** / Version control system commit identifiers that implement or relate to this milestone. / Optional |
| `completed_at` | `string` | ISO-8601 datetime when the milestone was completed. / Optional |
| `completion_criteria` | `list[string]` | Verifiable conditions that must be met for milestone completion. / Optional |
| `criteria_refs` | `list[string]` | **GRAPH EDGE** / Criteria this milestone must satisfy for completion. / Optional |
| `goal_refs` | `list[string]` | **GRAPH EDGE** / Goals this milestone contributes to. / Optional |
| `prerequisite_refs` | `list[string]` | **GRAPH EDGE** / Milestones that must be completed before this one can start. / Optional |
| `requirement_refs` | `list[string]` | **GRAPH EDGE** / Requirements this milestone fulfills. / Optional |
| `stage_type` | `enum['tier', 'stage', 'prerequisite', 'validation', 'release_gate']` | Classify the milestone (tier, stage, prerequisite, validation, release gate, etc.). / Optional |
| `status` | `enum['not_started', 'in_progress', 'blocked', 'complete', 'deferred']` | Milestone status (\"not_started\", \"in_progress\", \"blocked\", \"complete\", \"deferred\"). / Required |
| `status_history` | `list[string]` | Chronological record of status changes and notes. / Optional |
| `workstream_refs` | `list[string]` | **GRAPH EDGE** / Workstreams this milestone belongs to or impacts. / Optional |

### Schema: `mission`
**Extends:** `base_object`
**Lifecycle:** `mission_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `goal_refs` | `list[string]` | **GRAPH EDGE** / Goals that support this mission. / Optional |
| `mission_statement` | `text` | Core statement of why this effort exists. / Required |
| `persona_refs` | `list[string]` | **GRAPH EDGE** / Personas this mission serves. / Optional |
| `problem_statement` | `text` | Defines the pain points motivating the mission. / Optional |
| `vision` | `text` | Desired future state / success narrative. / Optional |
| `workstream_refs` | `list[string]` | **GRAPH EDGE** / Workstreams that support this mission. / Optional |

### Schema: `namespace`
**Extends:** `base_object`
**Lifecycle:** `namespace_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `applicability` | `object` | Defines when, where, and how the namespace can be used. / Required |
| `domain` | `string` | Domain name for domain and integration layers (e.g., \"organizational\", \"financial\"). / Optional |
| `integration` | `object` | Defines how namespaces integrate with each other while maintaining boundaries. / Required |
| `isolation` | `object` | Defines mechanisms to keep namespaces separate and prevent conflicts. / Required |
| `layer` | `enum['kernel', 'domain', 'integration']` | Namespace layer (kernel, domain, or integration). / Required |
| `namespace_id` | `string` | Unique namespace identifier (e.g., \"zqk:kernel\", \"domain:organizational\"). / Required |
| `origin` | `object` | Describes where the namespace comes from and who has authority over it. / Required |

### Schema: `namespace_registry`
**Extends:** `base_object`
**Lifecycle:** `namespace_registry_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `id` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `namespaces` | `list[string]` | List of registered namespaces with their metadata. / Required |

### Schema: `narrative`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `brand_asset_refs` | `list[string]` | **GRAPH EDGE** / Assets used in this narrative. / Optional |
| `key_messages` | `list[string]` | Key messages conveyed by the narrative. / Optional |
| `story_arc` | `text` | The structure or arc of the story. / Optional |
| `target_audience` | `string` | Target audience for this narrative. / Required |
| `tone` | `string` | The tone of the narrative. / Optional |

### Schema: `object_spec`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `completeness_validation` | `list[string]` | The canonical Validation DSL steps that must pass for any object of this kind to be considered fully complete. / Optional |
| `file_path` | `string` | Absolute or project-relative path to the YAML spec file when materialized as an object row. / Optional |
| `ontology` | `string` | The object kind (ontology) defined by this spec file (e.g. backlog_item, lifecycle). / Optional |
| `source_type` | `enum['built-in', 'internal', 'public']` | Provenance bucket (built-in, internal, public) when listing from disk. / Optional |

### Schema: `ordered_list_metric_sampler`
**Extends:** `base_sampler`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `batch_size` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `flush_interval` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `group_by_object_id` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `max_batch_size` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `metric_type` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |

### Schema: `organization`
**Extends:** `extensible_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `division_refs` | `list[string]` | **GRAPH EDGE** / References to divisions that belong to this organization / Optional |
| `kernel_goals_refs` | `list[string]` | **GRAPH EDGE** / References to kernel goals that align with this organization / Optional |
| `organization_name` | `string` | The official name of the organization / Required |
| `partnership_refs` | `list[string]` | **GRAPH EDGE** / References to partnerships this organization participates in / Optional |

### Schema: `organizational_change`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `affected_objects` | `map` | Map of affected organizational objects by type (divisions, teams, organizations, etc.) / Optional |
| `change_date` | `datetime` | Date/time when the organizational change occurred or is scheduled to occur / Optional |
| `change_description` | `text` | Human-readable description of the organizational change (e.g., "Split Engineering division into Infrastructure and Product divisions") / Optional |
| `change_type` | `string` | Type of organizational change (e.g., division_restructure, team_reassignment, organization_merge, team_move) / Required |
| `impact_analysis_refs` | `list[string]` | **GRAPH EDGE** / References to impact analysis objects generated for this organizational change / Optional |

### Schema: `partnership`
**Extends:** `extensible_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `end_date` | `date` | The date when the partnership ended (null for active partnerships) / Optional |
| `organization_refs` | `list[string]` | **GRAPH EDGE** / References to organizations participating in this partnership (minimum 2) / Required |
| `partnership_name` | `string` | The name or description of the partnership / Required |
| `partnership_type` | `enum['collaboration', 'joint_venture', 'strategic_alliance', 'supplier', 'customer', 'other']` | The type of partnership (collaboration, joint_venture, strategic_alliance, etc.) / Optional |
| `start_date` | `date` | The date when the partnership began / Optional |

### Schema: `persona`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `goal_refs` | `list[string]` | **GRAPH EDGE** / Goals this persona supports. / Optional |
| `mission_refs` | `list[string]` | **GRAPH EDGE** / Missions this persona supports. / Optional |
| `name` | `string` | Persona name/label. / Required |
| `needs` | `list[string]` | Bullet list of needs/pain points. / Optional |
| `role` | `string` | Persona's job/context. / Required |

### Schema: `pipeline`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `agent_task_refs` | `list[string]` | **GRAPH EDGE** / List of agent_task IDs associated with this pipeline. / Optional |
| `goal_refs` | `list[string]` | **GRAPH EDGE** / References to system or business goals this pipeline serves. / Optional |
| `policy_refs` | `list[string]` | **GRAPH EDGE** / References to policies that govern this pipeline. / Optional |
| `requirement_refs` | `list[string]` | **GRAPH EDGE** / References to requirements driving this pipeline. / Optional |
| `stages` | `list[string]` | Ordered stages or DAG nodes of execution. / Optional |
| `trigger` | `map` | Describes what triggers the pipeline. / Required |

### Schema: `policy`
**Extends:** `base_object`
**Lifecycle:** `policy_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `applicability` | `object{optional fields like \"workstreams\", \"object_types\", \"file_patterns\", \"exclusions\".}` | Scope of policy applicability (workstreams, object types, file patterns, etc.). / Optional |
| `body` | `text` | The actual policy content, standards, and expectations. / Required |
| `category` | `string` | Policy category (architecture, code_quality, documentation, testing, security, workflow, etc.). / Required |
| `effective_date` | `string` | When this policy version became effective. / Optional |
| `enforcement` | `object{fields like \"automated\", \"reminder_enabled\", \"review_required\", \"severity\".}` | Enforcement mechanism configuration (automated checks, reminders, review requirements). / Optional |
| `examples` | `list[string]` | Code examples, patterns, or anti-patterns illustrating the policy. / Optional |
| `goal_refs` | `list[string]` | **GRAPH EDGE** / Goals this policy supports. / Optional |
| `id` | `string` | Stable identifier for policy objects (POL-CATEGORY-### format). / Required |
| `milestone_refs` | `list[string]` | **GRAPH EDGE** / Milestones this policy relates to. / Optional |
| `policy_type` | `enum['standard', 'requirement', 'guideline', 'best_practice', 'anti_pattern']` | Type of policy (standard=mandatory, requirement=must follow, guideline=should follow, best_practice=recommended, anti_pattern=what to avoid). / Required |
| `related_patterns` | `list[string]` | References to architecture patterns, ADRs, or other policies related to this policy. / Optional |
| `review_date` | `string` | Next scheduled review date for this policy. / Optional |
| `validation_overlays` | `list[string]` | Universal Verification DSL definitions that are automatically injected as validation overlays into objects governed by this policy. / Optional |
| `version` | `string` | Policy version (SemVer) for tracking policy evolution. / Optional |
| `workstream_refs` | `list[string]` | **GRAPH EDGE** / Workstreams this policy applies to (if specific). / Optional |

### Schema: `priority_plan`
**Extends:** `base_object`
**Lifecycle:** `priority_plan_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `active_order` | `integer` | Explicit ordering for active priority plans. Lower numbers take precedence when multiple plans are active. If not set, defaults to lowest priority (treated as highest number). / Optional |
| `description` | `text` | Description of this priority plan. / Optional |
| `id` | `string` | Stable identifier for the priority plan. Supports formats like \"PRIO-20251212-week1\", \"PRIO-4\", \"PRI-001\", etc. / Required |
| `next_plan_id` | `string` | Reference to the next priority plan for historical tracking. / Optional |
| `note` | `text` | Optional notes about the priority plan. / Optional |
| `persona_refs` | `list[string]` | **GRAPH EDGE** / List of persona IDs associated with this priority plan. / Optional |
| `plan_date` | `string` | ISO-8601 date when this priority plan was created. / Optional |
| `plan_version` | `string` | Version identifier for this priority plan (e.g., \"v1.0\", \"2025-01-21\"). / Optional |
| `previous_plan_id` | `string` | Reference to the previous priority plan for historical tracking. / Optional |
| `rationale` | `text` | Rationale for priority assignments in this plan. / Optional |
| `release_ref` | `string` | **GRAPH EDGE** / Link to release object created when plan completes. / Optional |
| `source_file` | `string` | Path to source markdown file used to create this priority plan. / Optional |
| `source_format` | `string` | Indicates the source format used to create this priority plan. / Optional |
| `team_configuration_ref` | `reference` | **GRAPH EDGE** / References a team configuration (cellular archetype) for the priority plan pod execution. / Optional |
| `workflow_ref` | `string` | **GRAPH EDGE** / Reference to workflow object that defines constraints and rules for this priority plan. Workflow constraints are enforced when plan transitions to active status. / Optional |
| `workstream_ref` | `string` | **GRAPH EDGE** / Link to associated workstream (singular reference, legacy field). / Optional |
| `workstream_refs` | `list[string]` | **GRAPH EDGE** / List of workstream IDs associated with this priority plan. / Optional |

### Schema: `probe_spec`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `command` | `string` | The exact command or pipeline to execute. / Required |
| `description` | `text` | What this probe accomplishes and when to use it. / Required |
| `format` | `string` | Format of the output (e.g. json, text, table). / Optional |

### Schema: `process_hygiene_rule`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `description` | `string` | Human-readable explanation of the check. / Optional |
| `enabled` | `boolean` | When false, rule is skipped. / Optional |
| `match_equals` | `string` | Match when field equals this literal. / Optional |
| `match_field` | `string` | Object field name to read (e.g. title, id). / Required |
| `match_prefix` | `string` | Match when field value has this prefix (exclusive with suffix/equals/regex). / Optional |
| `match_regex` | `string` | Match when field matches this regex (Go RE2 syntax). / Optional |
| `match_suffix` | `string` | Match when field value has this suffix. / Optional |
| `rule_id` | `string` | Stable identifier for findings (matches process hygiene rule id in YAML bundles). / Required |
| `sort_order` | `integer` | Ordering when multiple storage rules apply (lower first). / Optional |
| `title` | `string` | Short label for logs and UI. / Optional |

### Schema: `prompt_template`
**Extends:** `base_object`
**Lifecycle:** `prompt_template_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `category` | `string` | Category for grouping prompt templates (establish_deliverable, tactical, strategic, reporting, etc.). / Optional |
| `doc_entry_refs` | `list[string]` | **GRAPH EDGE** / Doc entries that contain or expand this template (human-readable, reporting). / Optional |
| `expected_outcome_kinds` | `list[string]` | Object kinds the prompt is intended to produce or touch (e.g. milestone, goal, requirement, criteria, priority_plan, backlog_item, risk_blocker, doc_entry). / Optional |
| `goal_refs` | `list[string]` | **GRAPH EDGE** / Goals this prompt template supports. / Optional |
| `mandatory_constraints` | `list[string]` | Constraint identifiers or short statements that must be reflected in the prompt
(e.g. "use CLI only for process data", "backlog items start with initial status",
"no direct YAML edits under .zqk/process/"). Ensures system guardrails are present. / Optional |
| `originator_questionnaire_refs` | `list[string]` | **GRAPH EDGE** / References to question or criteria objects that the originator should answer to right-size the prompt (scope, timeline, risks, stakeholders, etc.). / Optional |
| `prompt_archetype` | `enum['tactical', 'structured', 'strategic', 'hybrid']` | Archetype that drives expected length, specificity, and rigidity.
- tactical: Short, simple, explicit instructions; clear success criteria; minimal ambiguity.
- structured: Medium length; defined outcomes (e.g. object kinds to create); lifecycle and linkage rules explicit.
- strategic: Broad, exploratory; subjective; influenced by market/seasonal/financial/political factors; looser expectations.
- hybrid: Combines structured skeleton (milestones, risks, timelines) with open-ended creative scope. / Required |
| `prompt_body` | `text` | The main prompt text or template with placeholders (e.g. {{deliverable}}, {{intent}}, {{constraints}}). Used when archetype and specificity are known. / Optional |
| `risk_blocker_refs` | `list[string]` | **GRAPH EDGE** / Risks or blockers mitigated by using this template (or risks from not using it). / Optional |
| `specificity_level` | `enum['high', 'medium', 'low']` | How prescriptive the prompt should be (high = explicit constraints and steps;
medium = outcome-focused with guardrails; low = open-ended with minimal guardrails). / Optional |

### Schema: `provider_profile`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `base_url` | `string` | The URL prefix for the LLM API. / Required |
| `capabilities` | `list[string]` | List of supported features (e.g. code_generation, validation, routing, json_mode). / Optional |
| `concurrency_limit` | `string` | Max concurrent execution threads allowed. / Optional |
| `context_window_limit` | `string` | Max tokens or chars allowed in the context window. / Required |
| `endpoint_type` | `string` | The type of the API endpoint. / Required |
| `id` | `string` | Stable identifier. / Required |
| `initialization_sequence` | `list[string]` | Step-by-step startup process for the provider. / Optional |
| `injection_rules` | `list[string]` | Provider-specific constraints or instructions. / Optional |
| `max_tool_schema_bytes` | `string` | Hard limit for JSON schema byte size to prevent SWA blowouts. / Required |
| `model_id` | `string` | The specific model identifier (e.g. qwen3.6.Q4_K_M.gguf). / Required |
| `model_tier` | `string` | Operational tier (e.g. tier_1_complex, tier_2_simple, tier_3_light). / Optional |

### Schema: `question`
**Extends:** `base_object`
**Lifecycle:** `question_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `answer` | `text` | The answer to the question, once resolved. / Optional |
| `answer_ref` | `string` | **GRAPH EDGE** / Reference to the object that answers this question (e.g., decision ID, backlog item ID, document path). / Optional |
| `blocking_goal_refs` | `list[string]` | **GRAPH EDGE** / Goals that are blocked by this unanswered question. / Optional |
| `blocking_milestone_refs` | `list[string]` | **GRAPH EDGE** / Milestones that are blocked by this unanswered question. / Optional |
| `question_text` | `text` | The actual question being asked. Should be clear and specific. / Required |
| `related_question_refs` | `list[string]` | **GRAPH EDGE** / References to related questions (e.g., follow-up questions, related unknowns). / Optional |

### Schema: `release`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `backlog_item_refs` | `list[string]` | **GRAPH EDGE** / Backlog items included in this release. / Optional |
| `criteria_refs` | `list[string]` | **GRAPH EDGE** / Release criteria that must be met for this release. / Optional |
| `milestone_refs` | `list[string]` | **GRAPH EDGE** / Milestones included in this release. / Optional |
| `release_date` | `string` | ISO-8601 date when this release is scheduled or was released. / Optional |
| `release_type` | `enum['major', 'minor', 'patch', 'hotfix', 'beta', 'alpha']` | Type of release (e.g., \"major\", \"minor\", \"patch\", \"hotfix\", \"beta\", \"alpha\"). / Optional |
| `requirement_refs` | `list[string]` | **GRAPH EDGE** / Requirements fulfilled in this release. / Optional |
| `version` | `string` | Version identifier for this release (e.g., \"v1.0.0\", \"2025.12.06\"). / Required |

### Schema: `remote_kernel`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `capabilities` | `list[string]` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `endpoint` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `infrastructure_adapter_refs` | `list[string]` | **GRAPH EDGE** / [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `last_heartbeat` | `timestamp` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `public_key` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `shared_namespaces` | `list[string]` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `trust_level` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |

### Schema: `requirement`
**Extends:** `base_object`
**Lifecycle:** `requirement_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `acceptance_criteria` | `list[string]` | Legacy free-form text acceptance criteria (deprecated - use criteria_refs instead). / Optional |
| `actual_effort` | `string` | Actual effort spent on completing this requirement. / Optional |
| `backlog_item_refs` | `list[string]` | **GRAPH EDGE** / Backlog items that implement this requirement. / Optional |
| `completed_at` | `string` | ISO-8601 datetime when the requirement was completed. / Optional |
| `criteria_refs` | `list[string]` | **GRAPH EDGE** / Criteria objects that define requirement completion. Requirements MUST reference at least one criteria object (CRIT-####) instead of using free-form acceptance_criteria strings. / Required |
| `description` | `text` | Detailed description of the requirement, its context, and expected outcomes. / Optional |
| `estimated_effort` | `string` | Estimated effort for completing this requirement. / Optional |
| `goal_refs` | `list[string]` | **GRAPH EDGE** / Goals fulfilled by this requirement. / Required |
| `milestone_refs` | `list[string]` | **GRAPH EDGE** / Milestones that include this requirement. / Optional |
| `priority` | `enum['p0', 'p1', 'p2', 'p3']` | Relative urgency (p0..p3 or similar). / Optional |
| `technical_spec_refs` | `list[string]` | **GRAPH EDGE** / Technical specifications detailing the implementation approach for this requirement. / Optional |
| `test_case_refs` | `list[string]` | **GRAPH EDGE** / Test cases that validate this requirement. / Optional |
| `workstream_refs` | `list[string]` | **GRAPH EDGE** / Workstreams implementing this requirement. / Optional |

### Schema: `resolver`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `reference_format` | `string` | Pattern/URI describing valid references (e.g., \"account:{id}\" or \"test://suite/name\"). / Required |
| `scheme` | `string` | Identifier for the reference scheme handled (e.g., \"account\", \"workstream\", \"test\"). / Required |
| `user_hint` | `text` | Optional user-defined hint/config (path template, host, etc.). / Optional |

### Schema: `risk_blocker`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `affected_items` | `list[string]` | Work items (milestones, goals, backlog items) affected by this risk/blocker. / Optional |
| `backlog_item_refs` | `list[string]` | **GRAPH EDGE** / Backlog items affected by this risk/blocker. / Optional |
| `detected_by` | `string` | How this risk/blocker was detected (system, manual, D&B detection, etc.). / Optional |
| `goal_refs` | `list[string]` | **GRAPH EDGE** / Goals affected by this risk/blocker. / Optional |
| `impact` | `text` | Description of the impact if this risk materializes or blocker persists. / Optional |
| `milestone_refs` | `list[string]` | **GRAPH EDGE** / Milestones affected by this risk/blocker. / Optional |
| `mitigation_plan` | `text` | Plan to mitigate the risk or resolve the blocker. / Optional |
| `probability` | `enum['high', 'medium', 'low']` | Probability of risk materializing (for risks only). Not applicable to blockers (they're already materialized). / Optional |
| `related_risks` | `list[string]` | Related risk/blocker objects (e.g., cascading risks, related blockers). / Optional |
| `resolution_status` | `enum['open', 'in_progress', 'resolved', 'mitigated', 'accepted']` | Current resolution status (open, in_progress, resolved, mitigated, accepted). / Optional |
| `resolved_at` | `string` | When the risk was mitigated or blocker was resolved. / Optional |
| `resolved_by` | `string` | Actor who resolved the risk/blocker. / Optional |
| `risk_score` | `number` | Calculated risk score (0-100) based on severity and probability. Higher score = higher priority. / Optional |
| `risk_type` | `enum['risk', 'blocker']` | Distinguish between risk (potential future issue) and blocker (current impediment). / Optional |
| `severity` | `enum['critical', 'high', 'medium', 'low']` | Severity level (critical, high, medium, low) for prioritization and impact analysis. / Optional |

### Schema: `roadmap`
**Extends:** `base_object`
**Lifecycle:** `roadmap_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `goal_refs` | `list[string]` | **GRAPH EDGE** / Goals this roadmap supports. / Optional |
| `milestone_refs` | `list[string]` | **GRAPH EDGE** / Milestones included in this roadmap. / Optional |
| `timeline_end` | `string` | End date/time for the roadmap (ISO-8601). / Optional |
| `timeline_start` | `string` | Start date/time for the roadmap (ISO-8601). / Optional |
| `workstream_refs` | `list[string]` | **GRAPH EDGE** / Workstreams this roadmap applies to. / Optional |

### Schema: `role`
**Extends:** `base_object`
**Lifecycle:** `role_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `description` | `text` | Explains responsibilities/influence. / Required |
| `influence_level` | `enum['executive', 'owner', 'collective', 'observer', 'automation']` | Relative authority level (\"executive\", \"owner\", \"collective\", \"observer\", \"automation\", etc.). / Optional |
| `permissions` | `list[string]` | List of specific permissions granted to this role. / Optional |
| `role_id` | `string` | Unique identifier for the role. / Required |

### Schema: `rollback_report`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `affected_objects` | `list[string]` | References to objects/documents that were rolled back/removed. / Optional |
| `git_reference` | `string` | Git hash/tag involved in the rollback. / Required |
| `summary` | `text` | Human-readable explanation of the rollback (why, what next). / Optional |

### Schema: `rule`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `body` | `text` | The actual rule instructions/policy. / Required |
| `goal_refs` | `list[string]` | **GRAPH EDGE** / Goals this rule supports. / Optional |
| `scope` | `string` | Domain the rule applies to (command_execution, documentation, etc.). / Required |
| `type` | `enum['hard_limit', 'required', 'recommendation', 'informational']` | Severity (\"hard_limit\", \"required\", \"recommendation\", \"informational\"). / Optional |
| `workstream_refs` | `list[string]` | **GRAPH EDGE** / Workstreams this rule applies to. / Optional |

### Schema: `sampler_profile`
**Extends:** `base_sampler`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `applies_to` | `list[string]` | List of object kinds this profile applies to. If empty, profile is a template that must be explicitly applied. / Optional |
| `is_default` | `boolean` | If true, this profile is used as the default when no specific profile is requested. Only one profile should be marked as default per metric type. / Required |
| `profile_name` | `string` | Unique name for this sampler profile (e.g., \"high_frequency\", \"low_latency\", \"audit_events\"). / Required |

### Schema: `scalar_metric_sampler`
**Extends:** `base_sampler`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `batch_size` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `flush_interval` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `group_by_object_id` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `max_batch_size` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `metric_type` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |

### Schema: `scenario`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `category` | `string` | Category classification (e.g., \"performance\", \"validation\", \"integration\", \"stress\"). / Optional |
| `change_policy` | `string` | Policy for handling objects that change between snapshot initiation and clone completion. Options: \"reject\" (skip changed objects), \"include\" (use current state), \"reconstruct\" (reconstruct from change journal). / Optional |
| `cleanup_config` | `object` | Cleanup configuration (what to preserve, what to clean, retention policies, etc.). / Optional |
| `data_generation_config` | `object` | Configuration for automated test data generation (kinds, counts, diversity, etc.). / Optional |
| `documentation_refs` | `list[string]` | **GRAPH EDGE** / References to documentation files (doc_entry IDs) related to this scenario. / Optional |
| `edge_cases` | `list[string]` | Enumerate edge cases covered. / Optional |
| `environment_config` | `object` | Environment configuration (environment variables, resource limits, timeouts, etc.). / Optional |
| `fixtures` | `list[string]` | Paths to fixture data/assets. / Optional |
| `goal_refs` | `list[string]` | **GRAPH EDGE** / Goals this scenario tests. / Optional |
| `hash_mappings` | `object` | Hash mapping from original object hash to new scenario object hash (original_hash -> new_hash). Used for integrity verification and reference updates. / Optional |
| `infrastructure_config` | `object` | Configuration for infrastructure setup (which configs, specs, lifecycles to copy, or use defaults). / Optional |
| `last_run_info` | `object` | Information about the last scenario execution (timestamp, duration, status, metrics, etc.). / Optional |
| `objective` | `text` | What the scenario aims to test. / Required |
| `performance_targets` | `object` | Performance targets and thresholds (e.g., max execution time, min cache hit rate, etc.). / Optional |
| `scenario_refs` | `list[string]` | **GRAPH EDGE** / Other scenarios this scenario depends on or extends. / Optional |
| `scheduler_config` | `object` | Scheduler configuration for the scenario (enabled, project_type, job overrides, etc.). / Optional |
| `snapshot_hashes` | `object` | Hash of each source object at snapshot time (object_id -> hash). Used for integrity verification and change detection. / Optional |
| `snapshot_timestamp` | `string` | Timestamp (UTC) at which the system state was captured for this scenario. Used for change detection and historical state reconstruction. / Optional |
| `source_object_ids` | `list[string]` | Object IDs from the main project that were copied to create this scenario (for traceability and recreation). / Optional |
| `tags` | `list[string]` | Tags for categorizing and filtering scenarios (e.g., \"performance\", \"validation\", \"integration\"). / Optional |
| `test_execution_config` | `object` | Test execution configuration (commands to run, assertions, performance targets, etc.). / Optional |
| `validation_rules` | `object` | Validation rules that must pass for scenario to be considered successful (e.g., no blocking errors, max warning count, etc.). / Optional |
| `workstream_refs` | `list[string]` | **GRAPH EDGE** / Workstreams this scenario exercises. / Optional |

### Schema: `scheduler_handler_binding`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `enabled` | `boolean` | Whether this binding is active. / Optional |
| `handler_key` | `enum['cache_prewarm', 'cache_invalidation', 'retention_tolerance', 'maintenance', 'scheduler_job_retention', 'autofix_batch_cleanup', 'cleanup', 'lifecycle_check', 'integrity_check', 'object_validation', 'audit_event_aggregation', 'change_journal_aggregation', 'aggregation_metrics_cleanup', 'generic_metrics_cleanup', 'metrics_collection', 'scheduler_events_aggregation', 'cascade_update', 'operation_execution', 'run_wrapper', 'callback_listener', 'test_io', 'context_refresh', 'convergence_session_tick', 'data_cell_envelope_tick']` | Stable handler key used by prewarm overlay resolution. / Required |
| `job_type` | `enum['context_refresh', 'manifest_snapshot', 'test_runner', 'aggregation', 'lifecycle_check', 'audit_event_aggregation', 'change_journal_aggregation', 'integrity_check', 'cache_prewarm', 'cache_invalidation', 'cascade_update', 'operation_execution', 'run_wrapper', 'callback_listener', 'metrics_collection', 'retention_tolerance', 'scheduler_job_retention', 'autofix_batch_cleanup', 'aggregation_metrics_cleanup', 'generic_metrics_cleanup', 'maintenance', 'object_validation', 'scheduler_events_aggregation', 'cleanup', 'convergence_session_tick', 'data_cell_envelope_tick']` | Scheduler job_type to bind (must be a known scheduler job_type value). / Required |
| `notes` | `text` | Optional rationale/context for the binding. / Optional |
| `priority` | `integer` | Lower value is applied first when multiple bindings target one job_type. / Optional |

### Schema: `scheduler_health_metric`
**Extends:** `base_metric`
**Lifecycle:** `scheduler_health_metric_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `cron_restarts` | `integer` | Total number of times the cron scheduler was automatically restarted. / Required |
| `goroutine_count` | `integer` | Number of goroutines in the scheduler process at sample time (helps diagnose thread explosion). / Optional |
| `health_check_duration_ms` | `number` | Duration of the health check in milliseconds. / Required |
| `health_checks` | `integer` | Total number of health check runs performed. / Required |
| `heap_alloc_bytes` | `integer` | Heap memory allocated by the scheduler process at sample time. / Optional |
| `measurement_window_end` | `string` | ISO 8601 timestamp of measurement window end. / Required |
| `measurement_window_start` | `string` | ISO 8601 timestamp of measurement window start. / Required |
| `missed_triggers` | `integer` | Total number of missed job triggers detected (jobs that should have run but didn't). / Required |
| `recovered_jobs` | `integer` | Total number of jobs automatically recovered after missed triggers. / Required |
| `runtime_thread_count` | `integer` | OS thread count of the scheduler process at sample time (Linux only; 0 elsewhere). Helps correlate with goroutine_count for thread exhaustion. / Optional |
| `sys_memory_bytes` | `integer` | Total OS memory obtained by the scheduler process at sample time. / Optional |

### Schema: `scheduler_job`
**Extends:** `base_object`
**Lifecycle:** `scheduler_job_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `action_ref` | `string` | **GRAPH EDGE** / Reference to the handler/resolver action. / Optional |
| `allow_parallel_execution` | `boolean` | When true, multiple instances of this job may run concurrently (e.g. catch-up triggers for missed runs). When false, scheduler uses job-type default (e.g. cache_prewarm, audit_event_aggregation allow concurrent; run_wrapper does not). / Optional |
| `auth_config` | `object` | Authentication configuration (for callback_listener job_type). Type-specific settings (JWT secret, x509 CA cert, API key, OAuth2 client ID/secret). / Optional |
| `auth_type` | `enum['none', 'jwt', 'x509', 'api_key', 'oauth2']` | Authentication type for callback listener (for callback_listener job_type). / Optional |
| `callback_on_completion` | `string` | Callback URL or command to execute on successful job completion (for run_wrapper job_type). / Optional |
| `callback_on_error` | `string` | Callback URL or command to execute on job failure (for run_wrapper job_type). / Optional |
| `callback_on_status` | `string` | Callback URL or command to execute for incremental status updates (for run_wrapper job_type). / Optional |
| `callback_type` | `enum['webhook', 'command', 'event']` | Type of callback mechanism (webhook=HTTP POST, command=execute command, event=emit system event). / Optional |
| `category` | `string` | Job category for grouping and organization (e.g., \\\"maintenance\\\", \\\"monitoring\\\", \\\"cleanup\\\", \\\"cache\\\"). / Optional |
| `command` | `string` | Command to execute (for run_wrapper job_type). Full path or command name. / Optional |
| `command_args` | `array` | Command arguments (for run_wrapper job_type). Array of strings. / Optional |
| `enabled` | `boolean` | Whether the job is currently enabled. / Optional |
| `environment_variables` | `object` | Environment variables to set for command execution (for run_wrapper job_type). Key-value pairs. / Optional |
| `execution_mode` | `enum['reusable', 'one_time']` | Whether the job can be executed multiple times (reusable) or only once (one_time). / Optional |
| `idle_shutdown_seconds` | `integer` | Idle timeout in seconds before listener server shuts down (for callback_listener job_type). 0 = never shutdown. / Optional |
| `job_type` | `enum['context_refresh', 'manifest_snapshot', 'test_runner', 'aggregation', 'lifecycle_check', 'audit_event_aggregation', 'change_journal_aggregation', 'integrity_check', 'cache_prewarm', 'cache_invalidation', 'cascade_update', 'operation_execution', 'run_wrapper', 'callback_listener', 'metrics_collection', 'retention_tolerance', 'scheduler_job_retention', 'autofix_batch_cleanup', 'aggregation_metrics_cleanup', 'generic_metrics_cleanup', 'maintenance', 'object_validation', 'scheduler_events_aggregation', 'cleanup', 'convergence_session_tick', 'data_cell_envelope_tick', 'auto_transition', 'cap_orchestrator']` | Kind of job (context_refresh, manifest_snapshot, test_runner, etc.). / Required |
| `last_run_at` | `string` | ISO-8601 timestamp of last execution. / Optional |
| `listener_path` | `string` | Base path for callback listener endpoints (for callback_listener job_type). Default is /callbacks. / Optional |
| `listener_port` | `integer` | Port number for callback listener server (for callback_listener job_type). / Optional |
| `log_level` | `enum['default', 'verbose', 'debug']` | Logging verbosity level for this job:
- default: Standard logging (Info/Warn/Error only)
- verbose: Include command output (stdout/stderr) in logs
- debug: Full debug logging including all execution details / Optional |
| `max_runtime_seconds` | `integer` | Maximum execution time in seconds before job is killed (prevents hangs). / Optional |
| `next_run_at` | `string` | ISO-8601 timestamp of next scheduled execution. / Optional |
| `priority` | `enum['normal', 'high', 'critical']` | Scheduling priority. Higher priority jobs are submitted before lower when the scheduler
loads and runs immediate/triggered work (e.g. cache_prewarm, cache_invalidation, retention).
Use critical for jobs that must run first so caches and critical work complete before others. / Optional |
| `retry_count` | `integer` | Number of retry attempts on failure (for run_wrapper job_type). 0 = no retries. / Optional |
| `retry_delay_seconds` | `integer` | Delay in seconds between retry attempts (for run_wrapper job_type). / Optional |
| `route_handlers` | `object` | Route-to-handler mappings for callback listener (for callback_listener job_type). Key-value pairs map route to handler_type. / Optional |
| `schedule_expression` | `string` | Cron/ISO schedule or cadence definition (required for timer trigger_type, optional otherwise). / Optional |
| `transactional` | `boolean` | If true, all storage operations within the job are wrapped in a transaction. On success, all operations commit atomically. On failure, all operations rollback. / Optional |
| `trigger_type` | `enum['timer', 'manual', 'immediate', 'workflow', 'event', 'lifecycle']` | How the job is triggered (timer=cron schedule, manual=user-triggered, immediate=run once now, workflow=workflow event, event=system event). / Required |
| `working_directory` | `string` | Working directory for command execution (for run_wrapper job_type). / Optional |

### Schema: `stakeholder_profile`
**Extends:** `base_object`
**Lifecycle:** `stakeholder_profile_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `alignment_metrics` | `list[string]` | Metrics and targets (e.g. metric, target) for stakeholder satisfaction. / Optional |
| `communication_preferences` | `object` | Frequency, format, and channels (e.g. frequency, format, channels). / Optional |
| `expectations` | `list[string]` | List of stakeholder expectations (e.g. quarterly reports, budget adherence). / Optional |
| `priorities` | `list[string]` | Prioritized concerns with weights (priority, concern, weight). / Optional |
| `stakeholder_type` | `string` | Type of stakeholder (e.g. decision_maker, contributor, reviewer). / Optional |

### Schema: `status_history_metric_sampler`
**Extends:** `base_sampler`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `batch_size` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `flush_interval` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `group_by_object_id` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `max_batch_size` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `metric_type` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |

### Schema: `strategic_context`
**Extends:** `base_object`
**Lifecycle:** `strategic_context_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `content` | `string` | Multiline description of the strategic context. / Optional |
| `context_type` | `string` | Type of context (e.g. market_condition, technical_constraint, business_constraint). / Optional |
| `goal_refs` | `list[string]` | **GRAPH EDGE** / Goals this context affects (alias affects_goals in design). / Optional |
| `important_dates` | `list[string]` | List of {date, event, impact} entries. / Optional |
| `policy_refs` | `list[string]` | **GRAPH EDGE** / Policies this context affects. / Optional |
| `stakeholder_refs` | `list[string]` | **GRAPH EDGE** / References to stakeholders (e.g. STK-001, account:executive-team). / Optional |

### Schema: `strategic_plan`
**Extends:** `base_object`
**Lifecycle:** `strategic_plan_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `goal_refs` | `list[string]` | **GRAPH EDGE** / References to all goals in this strategic plan. / Optional |
| `id` | `string` | Stable identifier used across the graph (\"STRAT-PLAN-####\"). / Required |
| `phases` | `list[string]` | List of phases in this strategic plan, each with workstreams, goals, and agent onboarding. / Required |
| `planning_horizon` | `string` | Time period covered by this strategic plan (e.g., \"2026-01-01 to 2028-12-31\"). / Required |
| `workstream_refs` | `list[string]` | **GRAPH EDGE** / References to all workstreams in this strategic plan. / Optional |

### Schema: `tde_envelope`
**Extends:** `none`
**Lifecycle:** `None`

### Schema: `team`
**Extends:** `extensible_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `department_ref` | `reference` | **GRAPH EDGE** / Reference to the department this team belongs to (optional if team belongs directly to division) / Optional |
| `division_ref` | `reference` | **GRAPH EDGE** / Reference to the division this team belongs to (optional if team belongs to department) / Optional |
| `member_refs` | `list[string]` | **GRAPH EDGE** / References to accounts that are members of this team / Optional |
| `team_name` | `string` | The name of the team / Required |
| `workstream_refs` | `list[string]` | **GRAPH EDGE** / References to zqk kernel workstreams that this team is working on / Optional |

### Schema: `team_configuration`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `cell_type` | `string` | The biological cell archetype (e.g., neuron, muscle, heart, lungs). / Required |
| `focus_area` | `string` | The strategic focus of the team (e.g., marketing, strategic, architecture). / Required |
| `id` | `string` | Stable identifier. / Required |
| `persona_allocations` | `list[string]` | List of personas that comprise this team and their counts. / Optional |

### Schema: `technical_debt`
**Extends:** `base_object`
**Lifecycle:** `technical_debt_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `backlog_ref` | `string` | **GRAPH EDGE** / Reference to backlog item tracking the resolution work (BLI-#### format) / Optional |
| `complexity_score` | `integer` | Cyclomatic complexity score (for complexity-type debt) / Optional |
| `debt_type` | `enum['complexity', 'performance', 'maintainability', 'infrastructure', 'tooling', 'process', 'documentation', 'testing', 'security', 'observability']` | Type of technical debt (complexity, performance, maintainability, infrastructure, tooling, process, documentation, testing, security, observability) / Required |
| `description` | `string` | Detailed description of the technical debt issue / Required |
| `file_path` | `string` | File path where the technical debt exists (e.g., \"cmd/zqk/object/list.go\") / Optional |
| `function_name` | `string` | Function name where the technical debt exists (e.g., \"runList\") / Optional |
| `impact_assessment` | `enum['low', 'medium', 'high', 'critical']` | Impact assessment of the technical debt (low, medium, high, critical) / Required |
| `linter_rule` | `string` | Linter rule that identified this technical debt (e.g., \"gocyclo\", \"gocritic:hugeParam\") / Optional |
| `mitigation_plan` | `string` | Plan for mitigating or resolving the technical debt / Optional |
| `policy_ref` | `string` | **GRAPH EDGE** / Reference to policy that identifies this as technical debt (POL-#### format) / Optional |
| `resolution_notes` | `string` | Notes about how the technical debt was resolved / Optional |
| `tags` | `array` | Optional tags for categorizing technical debt (e.g., \"continuous_improvement\", \"refactoring\", \"performance\") / Optional |
| `target_resolution_date` | `string` | Target date for resolving the technical debt (ISO 8601 format) / Required |

### Schema: `technical_spec`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `component` | `string` | Target component or system module this spec covers. / Optional |
| `description` | `text` | Architectural explanation and context of the implementation approach. / Required |
| `requirement_refs` | `list[string]` | **GRAPH EDGE** / Requirements this technical specification addresses. / Required |

### Schema: `template`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `category` | `enum['branching', 'commit', 'structure', 'bootstrap']` | Template category for organization. / Optional |
| `flow_order` | `number` | Position within initialization/generation pipeline. / Optional |
| `outputs` | `list[string]` | Files/artifacts produced by the template. / Required |
| `questions` | `list[string]` | Question set driving the template prompts. / Optional |

### Schema: `test_audit_aggregation_metric`
**Extends:** `base_metric`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `aggregated_event_ids` | `array` | Array of audit event IDs or ID ranges that were aggregated (for traceability). / Optional |
| `aggregation_window_end` | `string` | ISO 8601 timestamp of the end of the aggregation window. / Required |
| `aggregation_window_start` | `string` | ISO 8601 timestamp of the start of the aggregation window. / Required |
| `compression_ratio` | `number` | Compression ratio (events aggregated / metric size ratio). / Optional |
| `error_event_count` | `integer` | Total number of error-status events in the aggregation window. / Optional |
| `error_rate` | `number` | Ratio of error-status events to total events in the aggregation window. / Optional |
| `event_count` | `integer` | Total number of audit events aggregated into this metric. / Required |
| `event_type_counts` | `object` | Count of events by event_type (e.g., hash_regeneration: 5, integrity_recovery: 2). / Required |
| `metric_type` | `enum['system']` | Type of metric - always \"system\" for audit aggregations. / Required |
| `object_kind_counts` | `object` | Count of events by object_kind (e.g., backlog_item: 10, goal: 3). / Optional |
| `operation_counts` | `object` | Count of events by operation type (e.g., Regenerated hash: 5, Fixed integrity: 2). / Optional |
| `status_counts` | `object` | Count of events by audit_event status (e.g., completed: 120, failed: 4, error: 2). / Optional |

### Schema: `test_case`
**Extends:** `base_object`
**Lifecycle:** `test_case_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `backlog_item_refs` | `list[string]` | **GRAPH EDGE** / Backlog items this test case validates. / Optional |
| `category` | `string` | Test category classification (e.g., \"Unit\", \"Integration\", \"E2E\", \"Performance\"). / Optional |
| `criteria_refs` | `list[string]` | **GRAPH EDGE** / Criteria this test case validates. / Optional |
| `description` | `text` | Detailed description of what the test case validates. / Optional |
| `milestone_refs` | `list[string]` | **GRAPH EDGE** / Milestones this test case validates. / Optional |
| `path_or_id` | `string` | File path, test identifier, or command to run the test. / Optional |
| `priority` | `enum['critical', 'high', 'medium', 'low']` | Priority level (critical, high, medium, low) for test execution ordering. / Optional |
| `requirement_refs` | `list[string]` | **GRAPH EDGE** / Requirements this test case validates. / Optional |
| `scope` | `enum['unit', 'integration', 'e2e', 'manual', 'scenario']` | Test category (unit, integration, e2e, manual, scenario). / Optional |
| `verification_suites` | `list[string]` | List of antagonistic test suites to run before cryptographic signing. / Optional |
| `workstream_refs` | `list[string]` | **GRAPH EDGE** / Workstreams this test case belongs to. / Optional |

### Schema: `test_command_rule`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `conditions` | `list[string]` | List of conditions; all must match for this rule to match. Each item has field (command/args/shell_script), operator (eq/contains/in), value (string or array). / Required |
| `description` | `text` | Optional description of what this rule matches. / Optional |
| `name` | `string` | Short name for this rule (for display/admin). / Optional |
| `priority` | `integer` | Evaluation order; lower value = higher priority (e.g. 0 before 10). / Optional |

### Schema: `validation_rule`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `parameters` | `object` | Configuration parameters mapping values for the rule_type. / Optional |
| `rule_type` | `enum['field_presence', 'active_reference', 'alignment', 'expression']` | The generic validator template (field_presence, active_reference, alignment, expression). / Required |
| `sequence_level` | `integer` | Evaluation tier (1 = local completeness, 2 = dereference/active checks, 3 = path integrity). / Required |
| `target_kind` | `string` | The object kind this rule restricts (e.g. backlog_item, milestone). / Required |
| `target_status` | `string` | The target lifecycle status (e.g. active, complete, in_progress) this rule restricts. / Required |
| `transition_from` | `string` | Optional source status limiting this rule to a specific transition path. / Optional |

### Schema: `verification_matrix`
**Extends:** `base_object`
**Lifecycle:** `verification_matrix_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `csv_path_override` | `string` | Optional repo-relative path overriding registry CSV for this object instance. / Optional |
| `gate_policy_notes` | `text` | Human- and machine-readable notes for gate rules (which rows must be yes/na, criteria links).
May reference CRIT- ids or backlog items. / Optional |
| `gated_object_kind` | `string` | When matrix_role is transition_gate, the object kind whose transitions are gated
(e.g. roadmap, milestone). Empty when not used as a gate. / Optional |
| `gated_transition_to` | `string` | Target status value that requires matrix completion (e.g. complete). / Optional |
| `linked_goal_refs` | `list[string]` | **GRAPH EDGE** / Goals whose achievement or progress this matrix tracks or gates. / Optional |
| `linked_milestone_refs` | `list[string]` | **GRAPH EDGE** / Milestones tied to this verification surface (e.g. all complete before roadmap closes). / Optional |
| `linked_roadmap_refs` | `list[string]` | **GRAPH EDGE** / Roadmaps whose completion may depend on matrix fullness. / Optional |
| `matrix_role` | `enum['custom', 'codebase_vetting', 'test_bundle', 'goal_progress', 'transition_gate', 'agent_coordination']` | Kind of matrix: codebase file vetting, test bundles, goal progress rollup, transition_gate
(block object status until rows complete), or agent_coordination (cross-agent punch list). / Required |
| `planning_notes` | `text` | Forward-looking context: unknowns, complexity, cross-cutting constraints, agent specialization
slices. Complements gate_policy_notes with qualitative planning. / Optional |
| `primary_convergence_session_ref` | `string` | **GRAPH EDGE** / Primary CVS id (CVS-…) for traceability when updating matrix rows or notes. / Optional |
| `profile_path_override` | `string` | Optional repo-relative path overriding registry profile. / Optional |
| `registry_alias` | `string` | Key in matrix_registry.yaml when csv/profile overrides are not used. / Optional |
| `status` | `enum['draft', 'active', 'archived']` | Lifecycle status (draft, active, archived). / Required |

### Schema: `vision`
**Extends:** `base_object`
**Lifecycle:** `vision_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `goal_refs` | `list[string]` | **GRAPH EDGE** / Goals that support this vision. / Optional |
| `mission_refs` | `list[string]` | **GRAPH EDGE** / Mission statements this vision aligns with. / Optional |
| `narrative` | `text` | Core vision narrative describing the desired future state. / Required |
| `pillars` | `list[string]` | Key pillars or principles that support the vision (e.g., \"Trustworthy automation\", \"Composable drivers\"). / Optional |
| `workstream_refs` | `list[string]` | **GRAPH EDGE** / Workstreams that support this vision. / Optional |

### Schema: `visual_plan`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `marketing_vision` | `string` | The marketing vision. / Required |
| `project_context` | `string` | Project context. / Required |
| `scenes` | `list[string]` | Scenes in the plan. / Required |
| `shots` | `list[string]` | Shots in the plan. / Required |

### Schema: `vitality_report`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `drift_frequency` | `float` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `last_calculation_at` | `timestamp` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `project_confidence_score` | `integer` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |
| `success_rate` | `float` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |

### Schema: `vocabulary_scheme`
**Extends:** `base_object`
**Lifecycle:** `vocabulary_scheme_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `context_scope` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |
| `machine_hints` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |
| `purpose` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Required |
| `summary` | `string` | [Warning: Undocumented Semantic Meaning - Refer to Implementation Context] / Optional |

### Schema: `watchdog_registration`
**Extends:** `base_object`
**Lifecycle:** `None`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `condition_query` | `string` | The specific criteria to trigger the notification (e.g., 'status == error'). / Required |
| `frequency` | `string` | How frequently this rule should be evaluated (e.g., '1m', '5m', or 'event-driven'). / Required |
| `heartbeat_enabled` | `bool` | If true, the Watchdog will periodically send an all-clear payload to assure the pool the monitoring loop is alive even if no infractions occur. / Optional |
| `note` | `text` | Human-readable context about why this registration exists. / Optional |
| `notify_target_ref` | `string` | **GRAPH EDGE** / The graph ID of the recipient (e.g., an agent_feed, team, or persona) that will receive the wake-up payload. / Required |
| `target_kind` | `string` | The object kind being monitored (e.g., agent_task, convergence_session). / Required |

### Schema: `workflow`
**Extends:** `base_object`
**Lifecycle:** `workflow_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `applicable_accounts` | `list[string]` | List of account IDs that can use this workflow. Empty list means available to all accounts. / Optional |
| `applicable_roles` | `list[string]` | List of role IDs that can use this workflow. Empty list means available to all roles. / Optional |
| `blocking_check_config_ref` | `string` | **GRAPH EDGE** / Reference to a blocking check configuration that applies to this workflow (overrides default). / Optional |
| `category` | `enum['development', 'operations', 'planning', 'review', 'testing', 'deployment', 'maintenance', 'onboarding', 'documentation', 'mcp', 'other']` | Workflow category for organization (development, operations, planning, review, etc.). / Optional |
| `constraints` | `object` | Defines role-based constraints for object creation and manipulation. Structure: {role_constraints: {role_id: {allowed_operations: [create, update, delete], allowed_kinds: [kind1, kind2], blocked_kinds: [kind3]}}, object_constraints: {kind: {required_roles: [role_id], blocked_roles: [role_id]}}}. / Optional |
| `description` | `text` | Detailed description of the workflow, its purpose, and when to use it. / Required |
| `enabled` | `boolean` | Whether this workflow is currently enabled and available for use. / Optional |
| `lifecycle_refs` | `list[string]` | **GRAPH EDGE** / References to lifecycle definitions that this workflow uses (e.g., backlog_item lifecycle, requirement lifecycle). / Optional |
| `mcp_config` | `object` | MCP-specific configuration for workflows with category 'mcp'. Controls which CLI commands/tools are exposed via MCP server. Structure: {exposed_tools: [list of command patterns], blocked_tools: [list of command patterns], tool_constraints: {tool_name: {required_roles: [roles], blocked_roles: [roles]}}}. Only applies when category is 'mcp'. / Optional |
| `metadata` | `object` | Additional metadata for workflow configuration (e.g., automation triggers, integration settings). / Optional |
| `policy_refs` | `list[string]` | **GRAPH EDGE** / References to policy objects that apply to this workflow. / Optional |
| `stages` | `list[string]` | Ordered list of workflow stages, each defining what happens at that stage. Each stage object contains: name, description, order, lifecycle_states (map of object kind to expected status), required_objects (list of object kinds that should exist), optional_objects (list of object kinds that may exist), policies (list of policy refs that apply), templates (list of template refs to use). / Optional |
| `template_refs` | `list[string]` | **GRAPH EDGE** / References to template objects used in this workflow. / Optional |

### Schema: `workstream`
**Extends:** `base_object`
**Lifecycle:** `workstream_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `application` | `string` | Optional application/component tag. / Optional |
| `blockers` | `list[string]` | Known blockers preventing progress on this workstream. / Optional |
| `category` | `enum['application', 'system', 'feature', 'component', 'ops', 'tooling']` | Scope classification (application/system/feature/component/ops/tooling/etc.). / Optional |
| `description` | `text` | Detailed description of the workstream's purpose and scope. / Optional |
| `entry_point` | `string` | Canonical doc or script describing the workstream. / Required |
| `metadata` | `object` | Additional flags (has_deferred_items, clarifications, etc.). / Optional |
| `milestone_refs` | `list[string]` | **GRAPH EDGE** / References to milestones that are part of this workstream. / Optional |
| `order` | `integer` | Ordering among active workstreams. / Optional |
| `owner_display` | `string` | Cached human-readable owner name. / Optional |
| `owner_ref` | `reference` | **GRAPH EDGE** / Reference to account/role representing the owner. / Required |
| `prerequisites` | `list[string]` | Other workstreams, milestones, or conditions that must be complete before this workstream can start. / Optional |
| `related_docs` | `list[string]` | Supporting docs/information. / Optional |
| `requirement_refs` | `list[string]` | **GRAPH EDGE** / References to requirements that this workstream implements. / Optional |
| `stage_type` | `string` | Stage or phase classification for this workstream (e.g., \"planning\", \"execution\", \"validation\"). / Optional |
| `workflow_ref` | `string` | **GRAPH EDGE** / Reference to workflow object that defines constraints and rules for this workstream. If not set, inherits workflow from associated priority plan. Workflow constraints enforce role-based object creation/manipulation restrictions. / Optional |
| `workstream_refs` | `list[string]` | **GRAPH EDGE** / References to related or dependent workstreams. / Optional |

### Schema: `workstream_transition`
**Extends:** `base_object`
**Lifecycle:** `workstream_transition_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `agent_onboarding` | `list[string]` | List of agents that must be onboarded for this transition. / Optional |
| `from_workstream_ref` | `string` | **GRAPH EDGE** / Reference to the workstream transitioning from. / Required |
| `readiness_criteria` | `list[string]` | List of criteria that must be met before transition can occur. / Optional |
| `to_workstream_ref` | `string` | **GRAPH EDGE** / Reference to the workstream transitioning to. / Required |
| `transition_date` | `date` | Target date for this transition. / Optional |
| `trigger` | `enum['milestone_completion', 'agent_readiness', 'dependency_resolution', 'strategic_pivot']` | Type of trigger that initiates this transition. / Required |
| `trigger_milestone_ref` | `string` | **GRAPH EDGE** / Reference to milestone that triggers transition (if trigger is milestone_completion). / Optional |

### Schema: `zqk_session`
**Extends:** `base_object`
**Lifecycle:** `zqk_session_lifecycle.yaml`

| Field Name | Strict Type | Constraints / Purpose |
|---|---|---|
| `account_id` | `string` | Account that owns this session (e.g. account:system or logged-in user) / Required |
| `id` | `string` | Stable identifier for the session (e.g. ZQK-001) / Required |
| `title` | `string` | Optional human-readable label for the session / Optional |

## PART 3: Lifecycle State Machines
Objects transition through strict states defined below.

### Lifecycle: `account_lifecycle.yaml`
**Statuses:**
- `active`: Active
- `inactive`: Inactive
- `suspended`: Suspended

**Transitions:**
- `active` -> `inactive` (Manual): [No explicit description provided]
- `active` -> `suspended` (Manual): [No explicit description provided]
- `inactive` -> `active` (Manual): [No explicit description provided]
- `suspended` -> `active` (Manual): [No explicit description provided]

### Lifecycle: `agent_architecture_lifecycle.yaml`
**Statuses:**
- `design`: Design
- `implementation`: Implementation
- `active`: Active (Terminal)
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `design` -> `implementation` (Manual): Move from design to implementation
- `implementation` -> `active` (Manual): Activate architecture
- `active` -> `archived` (Manual): Archive architecture
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `agent_feed_lifecycle.yaml`
**Statuses:**
- `active`: Active
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `active` -> `archived` (Manual): Disable feed binding.
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `agent_instruction_lifecycle.yaml`
**Statuses:**
- `proposed`: Proposed
- `approved`: Approved
- `rejected`: Rejected (Terminal)
- `in_progress`: In Progress
- `completed`: Completed (Terminal)
- `error`: Error

**Transitions:**
- `proposed` -> `approved` (Manual): Human operator approves the instruction
- `proposed` -> `rejected` (Manual): Human operator rejects the instruction
- `approved` -> `in_progress` (Auto): Agent begins executing the instruction
- `in_progress` -> `completed` (Auto): Agent completes the instruction successfully
- `in_progress` -> `error` (Auto): Agent encounters an error during execution
- `error` -> `in_progress` (Manual): Retry the instruction after fixing the error
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `agent_onboarding_preparation_lifecycle.yaml`
**Statuses:**
- `pending`: Pending
- `in_progress`: In Progress
- `active`: Active
- `complete`: Complete (Terminal)
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `pending` -> `in_progress` (Manual): Start preparation work
- `in_progress` -> `active` (Manual): Mark preparation as active
- `active` -> `complete` (Manual): Complete preparation
- `complete` -> `archived` (Manual): Archive preparation
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `agent_task_lifecycle.yaml`
**Statuses:**
- `proposed`: Proposed
- `approved`: Approved
- `in_progress`: In Progress
- `pending_verification`: Pending Verification
- `implemented`: Implemented (Terminal)
- `completed`: Completed (Terminal)
- `error`: Error
- `archived`: Archived (Terminal)

**Transitions:**
- `proposed` -> `approved` (Manual): Legacy approval
- `approved` -> `in_progress` (Manual): Begin work on task
- `proposed` -> `in_progress` (Manual): Begin work on task
- `in_progress` -> `pending_verification` (Manual): Task is complete, awaiting verification
- `in_progress` -> `implemented` (Manual): Legacy completion
- `pending_verification` -> `completed` (Auto): Task verified and complete
- `pending_verification` -> `implemented` (Auto): Task verified and complete (legacy/alternative)
- `pending_verification` -> `error` (Auto): Task verification failed
- `error` -> `in_progress` (Manual): Wake agent to fix task
- `implemented` -> `in_progress` (Manual): Reset implemented task back to work
- `implemented` -> `archived` (Manual): Archive legacy task
- `completed` -> `archived` (Manual): Archive completed task
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `audit_aggregation_metric_lifecycle.yaml`
**Statuses:**
- `completed`: Completed (Terminal)
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `completed` -> `archived` (Manual): Aggregation metric is archived for long-term storage
- `*` -> `error` (Auto): Aggregation metric encountered a system error

### Lifecycle: `audit_event_aggregation_lifecycle.yaml`
**Statuses:**
- `pending`: Pending Aggregation
- `aggregating`: Aggregating
- `aggregated`: Aggregated
- `archived`: Archived
- `deleted`: Deleted

**Transitions:**
- `pending` -> `aggregating` (Auto): Start aggregating audit events into metrics [Preconditions: aggregation job scheduled, events older than aggregation window]
- `aggregating` -> `aggregated` (Auto): Aggregation complete, metrics created [Preconditions: aggregation job completed successfully, metrics created from events]
- `aggregating` -> `pending` (Auto): Aggregation failed, retry [Preconditions: aggregation job failed, retry count not exceeded]
- `aggregated` -> `archived` (Auto): Archive aggregated events [Preconditions: events aggregated, archive retention period met]
- `archived` -> `deleted` (Auto): Delete archived events after final retention [Preconditions: events archived, final retention period expired]

### Lifecycle: `audit_event_lifecycle.yaml`
**Statuses:**
- `pending`: Pending
- `completed`: Completed (Terminal)
- `failed`: Failed (Terminal)
- `reverted`: Reverted (Terminal)
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `pending` -> `completed` (Manual): Audit event operation completed successfully
- `pending` -> `failed` (Manual): Audit event operation failed
- `completed` -> `reverted` (Manual): Completed audit event is reverted
- `*` -> `archived` (Manual): Audit event is archived (retention policy)
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `auth_strategy_lifecycle.yaml`
**Statuses:**
- `active`: Active
- `disabled`: Disabled
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `active` -> `disabled` (Manual): Disable the strategy (temporarily disable authentication)
- `disabled` -> `active` (Manual): Re-enable a disabled strategy
- `*` -> `archived` (Manual): Archive the strategy permanently (deprecate it)
- `*` -> `error` (Auto): Strategy encountered a system error or validation failure

### Lifecycle: `backlog_item_lifecycle.yaml`
**Statuses:**
- `exploring`: Exploring
- `validated`: Validated
- `roadmap`: Roadmap
- `deferred`: Deferred
- `planned`: Planned
- `in_progress`: In Progress
- `complete`: Complete (Terminal)
- `archived`: Archived (Terminal)
- `rejected`: Rejected (Terminal)
- `error`: Error

**Transitions:**
- `exploring` -> `validated` (Manual): Idea reviewed and approved for deeper planning [Preconditions: Problem statement and acceptance considerations defined]
- `exploring` -> `roadmap` (Manual): Move item to roadmap for future planning
- `exploring` -> `deferred` (Manual): Defer item for later consideration
- `roadmap` -> `validated` (Manual): Move item from roadmap to validated
- `roadmap` -> `planned` (Manual): Move item from roadmap to planned [Preconditions: CRI-SHOVEL-READY, priority_plan_ref is set]
- `deferred` -> `exploring` (Manual): Move deferred item back to exploring
- `deferred` -> `validated` (Manual): Move deferred item to validated
- `exploring` -> `rejected` (Manual): Idea closed after review
- `exploring` -> `planned` (Manual): Fast-track plan assignment when validation happens during promotion [Preconditions: CRI-SHOVEL-READY, priority_plan_ref is set]
- `roadmap` -> `planned` (Manual): Move from roadmap to planned [Preconditions: CRI-SHOVEL-READY, priority_plan_ref is set]
- `validated` -> `planned` (Manual): Item assigned to priority plan [Preconditions: CRI-SHOVEL-READY, Priority assigned (high/medium/low), Owner identified, priority_plan_ref is set (required per DEC-priority-plan-ref-requirement)]
- `error` -> `planned` (Manual): Recover from validation error status [Preconditions: CRI-SHOVEL-READY]
- `planned` -> `in_progress` (Manual): Active development begins [Preconditions: CRI-SHOVEL-READY, At least one active milestone_ref linked, priority_plan_ref is set, milestone_refs must link back to goal_refs]
- `in_progress` -> `complete` (Auto): Auto-complete when validation and acceptance criteria are met [Preconditions: commit_refs is not empty]
- `in_progress` -> `planned` (Manual): Work paused and returned to plan [Preconditions: priority_plan_ref is set]
- `complete` -> `archived` (Manual): Manual archival after completion review
- `*` -> `archived` (Manual): Manual archival from any status
- `*` -> `error` (Manual): System-assigned when lifecycle incoherency detected

### Lifecycle: `base_object_lifecycle.yaml`
**Statuses:**
- `proposed`: Proposed
- `approved`: Approved
- `in_progress`: In Progress
- `implemented`: Implemented (Terminal)
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `proposed` -> `approved` (Manual): Approve the proposed object
- `approved` -> `in_progress` (Manual): Begin work on the approved object
- `in_progress` -> `implemented` (Manual): Mark object as implemented
- `*` -> `archived` (Manual): Archive object from any status
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `bucketing_strategy_lifecycle.yaml`
**Statuses:**
- `active`: Active
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `active` -> `archived` (Manual): Archive the strategy (deprecate it)
- `*` -> `error` (Auto): Strategy encountered a system error or validation failure

### Lifecycle: `change_journal_entry_lifecycle.yaml`
**Statuses:**
- `pending`: Pending
- `completed`: Completed
- `aggregated`: Aggregated (Terminal)
- `failed`: Failed (Terminal)
- `reverted`: Reverted (Terminal)
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `pending` -> `completed` (Manual): Change journal entry operation completed successfully
- `pending` -> `failed` (Manual): Change journal entry operation failed
- `completed` -> `reverted` (Manual): Completed change journal entry is reverted
- `completed` -> `aggregated` (Auto): Entry aggregated into metric (change_journal_aggregation job)
- `*` -> `archived` (Manual): Change journal entry is archived (retention policy)
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `code_quality_metric_lifecycle.yaml`
**Statuses:**
- `collected`: Collected
- `analyzed`: Analyzed
- `reported`: Reported
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `collected` -> `analyzed` (Auto): Analyze collected code quality metric
- `analyzed` -> `reported` (Manual): Include metric in quality report
- `reported` -> `archived` (Auto): Archive metric after reporting
- `*` -> `archived` (Manual): Archive metric directly
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `component_lifecycle.yaml`
**Statuses:**
- `created`: Created
- `validated`: Validated
- `placed`: Placed
- `rendered`: Rendered
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `created` -> `validated` (Manual): Component passes validation checks
- `validated` -> `placed` (Manual): Component positioned in layout
- `placed` -> `rendered` (Auto): Component successfully rendered to SVG
- `*` -> `error` (Auto): Component encountered an error
- `*` -> `archived` (Manual): Component is no longer needed and can be archived

### Lifecycle: `convergence_session_lifecycle.yaml`
**Statuses:**
- `draft`: Draft
- `active`: Active
- `paused`: Paused
- `completed`: Completed (Terminal)
- `abandoned`: Abandoned (Terminal)
- `escalated`: Escalated (Terminal)
- `error`: Error

**Transitions:**
- `draft` -> `active` (Manual): Start convergence work
- `draft` -> `abandoned` (Manual): Discard draft
- `active` -> `paused` (Manual): Pause for handoff or dependency
- `active` -> `completed` (Manual): Mark converged or done
- `active` -> `abandoned` (Manual): Stop without success
- `active` -> `escalated` (Manual): Escalate to design or other owner
- `paused` -> `active` (Manual): Resume
- `paused` -> `abandoned` (Manual): Abandon while paused
- `*` -> `error` (Auto): System error

### Lifecycle: `criteria_lifecycle.yaml`
**Statuses:**
- `not_started`: Not Started
- `in_progress`: In Progress
- `validated`: Validated
- `complete`: Complete (Terminal)
- `blocked`: Blocked
- `rejected`: Rejected (Terminal)

**Transitions:**
- `not_started` -> `in_progress` (Manual): Manual transition when work on criterion begins
- `in_progress` -> `validated` (Manual): Manual transition after validation check
- `in_progress` -> `complete` (Auto): Auto-transition when automated test passes
- `in_progress` -> `complete` (Auto): Auto-transition when metric threshold is met
- `validated` -> `complete` (Manual): Manual confirmation after validation
- `in_progress` -> `blocked` (Auto): Auto-transition when dependent criteria are blocked
- `blocked` -> `in_progress` (Auto): Auto-transition when all dependent criteria are complete
- `*` -> `rejected` (Manual): Manual rejection from any status

### Lifecycle: `decision_lifecycle.yaml`
**Statuses:**
- `draft`: Draft
- `under_review`: Under Review
- `active`: Active
- `approved`: Approved (Terminal)
- `rejected`: Rejected (Terminal)
- `archived`: Archived (Terminal)

**Transitions:**
- `draft` -> `under_review` (Manual): Submit draft for review
- `under_review` -> `active` (Manual): Activate decision (move to active)
- `under_review` -> `approved` (Manual): Approve decision
- `active` -> `approved` (Manual): Approve from active
- `under_review` -> `rejected` (Manual): Reject decision
- `active` -> `rejected` (Manual): Reject from active
- `*` -> `archived` (Manual): Manual archival from any status

### Lifecycle: `display_lifecycle.yaml`
**Statuses:**
- `created`: Created
- `validated`: Validated
- `configured`: Configured
- `rendered`: Rendered
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `created` -> `validated` (Manual): Display passes validation checks
- `validated` -> `configured` (Manual): Display layout and components are configured
- `configured` -> `rendered` (Auto): Display successfully rendered to output
- `*` -> `error` (Auto): Display encountered an error
- `*` -> `archived` (Manual): Display is no longer needed and can be archived

### Lifecycle: `doc_entry_lifecycle.yaml`
**Statuses:**
- `draft`: Draft
- `review`: Review
- `published`: Published (Terminal)
- `active`: Active (Terminal)
- `archived`: Archived (Terminal)
- `deprecated`: Deprecated (Terminal)
- `error`: Error

**Transitions:**
- `draft` -> `review` (Manual): Document entry is submitted for review
- `review` -> `published` (Manual): Document entry is published
- `published` -> `active` (Manual): Document entry is activated (becomes searchable/visible)
- `*` -> `archived` (Manual): Document entry is archived (retention policy)
- `*` -> `deprecated` (Manual): Document entry is deprecated (replaced or superseded)
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `domain_registry_lifecycle.yaml`
**Statuses:**
- `active`: Active
- `deprecated`: Deprecated
- `archived`: Archived (Terminal)

**Transitions:**
- `active` -> `deprecated` (Manual): Deprecate domain registry
- `*` -> `archived` (Manual): Archive domain registry

### Lifecycle: `evolution_management_lifecycle.yaml`
**Statuses:**
- `planning`: Planning
- `active`: Active
- `complete`: Complete (Terminal)
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `planning` -> `active` (Manual): Activate evolution management
- `active` -> `complete` (Manual): Complete evolution
- `complete` -> `archived` (Manual): Archive evolution
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `glossary_term_lifecycle.yaml`
**Statuses:**
- `active`: Active
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `active` -> `archived` (Manual): Term is deprecated
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `glossary_term_relation_lifecycle.yaml`
**Statuses:**
- `active`: Active
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `active` -> `archived` (Manual): Archive relation
- `*` -> `error` (Auto): System error

### Lifecycle: `goal_lifecycle.yaml`
**Statuses:**
- `planned`: Planned
- `active`: Active
- `blocked`: Blocked
- `complete`: Complete (Terminal)
- `archived`: Archived (Terminal)

**Transitions:**
- `planned` -> `active` (Auto): Auto-transition when any linked workstream becomes active [Preconditions: At least one milestone_ref linked]
- `active` -> `complete` (Auto): Auto-transition when metric target met OR all workstreams complete
- `active` -> `blocked` (Auto): Auto-transition when blockers detected
- `blocked` -> `active` (Auto): Auto-transition when blockers resolved
- `*` -> `archived` (Manual): Manual archival from any status
- `*` -> `planned` (Manual): Manual recovery to planned state

### Lifecycle: `import_tracking_lifecycle.yaml`
**Statuses:**
- `ready`: Ready
- `translated`: Translated
- `failed`: Failed (Terminal)
- `archived`: Archived (Terminal)
- `error`: Error (Terminal)

**Transitions:**
- `ready` -> `translated` (Auto): Translation completed
- `ready` -> `failed` (Auto): Translation or import failed
- `*` -> `archived` (Manual): Archive import record
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `important_date_lifecycle.yaml`
**Statuses:**
- `draft`: Draft
- `active`: Active
- `passed`: Passed (Terminal)
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `draft` -> `active` (Manual): Activate the important date
- `active` -> `passed` (Manual): Mark date as passed
- `active` -> `archived` (Manual): Archive the date
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `keystore_entry_lifecycle.yaml`
**Statuses:**
- `active`: Active
- `revoked`: Revoked
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `active` -> `revoked` (Manual): Revoke the key (marks as revoked, cannot be used)
- `revoked` -> `active` (Manual): Re-activate a revoked key
- `*` -> `archived` (Manual): Archive the key permanently
- `*` -> `error` (Auto): Key encountered a system error

### Lifecycle: `kind_synonym_lifecycle.yaml`
**Statuses:**
- `active`: Active
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `active` -> `archived` (Manual): Synonym is deprecated by system
- `*` -> `error` (Auto): Synonym encountered a system error

### Lifecycle: `mcp_built_in_tool_lifecycle.yaml`
**Statuses:**
- `draft`: Draft
- `proposed`: Proposed
- `under_review`: Under Review
- `active`: Active
- `deprecated`: Deprecated
- `archived`: Archived (Terminal)

**Transitions:**
- `draft` -> `proposed` (Manual): Propose tool for implementation [Preconditions: tool name defined, use case documented, benefits identified]
- `proposed` -> `under_review` (Manual): Submit for review and approval [Preconditions: implementation plan documented, usage metrics analyzed, conversion criteria met]
- `under_review` -> `active` (Manual): Approve and activate tool [Preconditions: code implemented, tests written, documentation updated]
- `active` -> `under_review` (Manual): Tool updated - return to review
- `active` -> `deprecated` (Manual): Deprecate tool (replaced or no longer needed)
- `active` -> `under_review` (Manual): Quarterly review reminder (POL-MCP-001)
- `*` -> `archived` (Manual): Manual archival

### Lifecycle: `mcp_session_lifecycle.yaml`
**Statuses:**
- `in_progress`: In Progress
- `disconnected`: Disconnected (Terminal)
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `in_progress` -> `disconnected` (Auto): Client disconnected (EOF, idle timeout, or explicit shutdown)
- `in_progress` -> `archived` (Manual): Archive session (manual or retention)
- `disconnected` -> `archived` (Manual): Archive disconnected session
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `milestone_lifecycle.yaml`
**Statuses:**
- `not_started`: Not Started
- `in_progress`: In Progress
- `blocked`: Blocked
- `complete`: Complete (Terminal)
- `deferred`: Deferred (Terminal)
- `archived`: Archived (Terminal)

**Transitions:**
- `not_started` -> `in_progress` (Auto): Auto-transition when any linked requirement becomes active [Preconditions: At least one active criteria_ref linked, At least one active goal_ref linked, criteria_refs must link back to goal_refs]
- `in_progress` -> `complete` (Auto): Auto-transition when all completion criteria are checked
- `in_progress` -> `blocked` (Auto): Auto-transition when any prerequisite milestone is blocked or deferred
- `blocked` -> `in_progress` (Auto): Auto-transition when all prerequisites are complete
- `*` -> `deferred` (Manual): Manual deferral from any status
- `*` -> `archived` (Manual): Manual archival from any status
- `complete` -> `in_progress` (Manual): Manual reopening when new work is linked to a recently closed milestone

### Lifecycle: `mission_lifecycle.yaml`
**Statuses:**
- `draft`: Draft
- `active`: Active
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `draft` -> `active` (Manual): Mission is published and active
- `active` -> `archived` (Manual): Mission is archived (no longer active)
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `namespace_lifecycle.yaml`
**Statuses:**
- `active`: Active
- `deprecated`: Deprecated
- `archived`: Archived (Terminal)

**Transitions:**
- `active` -> `deprecated` (Manual): Deprecate namespace
- `*` -> `archived` (Manual): Archive namespace

### Lifecycle: `namespace_registry_lifecycle.yaml`
**Statuses:**
- `active`: Active
- `deprecated`: Deprecated
- `archived`: Archived (Terminal)

**Transitions:**
- `active` -> `deprecated` (Manual): Deprecate namespace registry
- `*` -> `archived` (Manual): Archive namespace registry

### Lifecycle: `policy_lifecycle.yaml`
**Statuses:**
- `draft`: Draft
- `under_review`: Under Review
- `active`: Active
- `deprecated`: Deprecated
- `superseded`: Superseded
- `archived`: Archived (Terminal)

**Transitions:**
- `draft` -> `under_review` (Manual): Submit policy for review [Preconditions: category field populated, policy_type field populated, body field populated]
- `under_review` -> `active` (Manual): Activate policy (approved and effective)
- `active` -> `under_review` (Manual): Policy updated - return to review
- `active` -> `deprecated` (Manual): Deprecate policy (no longer applicable but kept for reference)
- `active` -> `superseded` (Manual): Policy superseded by new policy [Preconditions: related_patterns contains reference to superseding policy]
- `under_review` -> `active` (Auto): Policy effective date reached - activate if in review
- `*` -> `archived` (Manual): Manual archival

### Lifecycle: `priority_plan_lifecycle.yaml`

SSOT is `.zqk/specs/lifecycles/priority_plan_lifecycle.yaml` plus the design exam in `docs/architecture/LIFECYCLE_STATE_MACHINE_RUBRIC.md`. Do not treat this dump as authority if it drifts.

**Statuses (roles):** `grooming` (grooming, origin), `active` (shovel_ready), `in_progress` (execution_locked, check valve), `paused`/`blocked` (halted), `complete`/`cancelled`/`archived` (terminal). Legacy `planning`/`prioritizing` coerce to `grooming` via `status_mapping`.

**Check valve:** `in_progress` exits are pause / blocked / complete / cancelled only. Halt resume is `paused|blocked → in_progress`, **not** `→ active`.

**Happy path:** `grooming → active → in_progress → complete`. Shockwave lock: first linked `backlog_item` → `in_progress`. Child realign (`exploring`/`validated`) may auto-return sealed/halted plans to `grooming`.

### Lifecycle: `prompt_template_lifecycle.yaml`
**Statuses:**
- `draft`: Draft
- `active`: Active
- `deprecated`: Deprecated (Terminal)
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `draft` -> `active` (Manual): Template reviewed and approved for general use
- `active` -> `deprecated` (Manual): Template superseded by a newer version but kept for reference
- `deprecated` -> `archived` (Manual): Template fully retired from active and fallback use
- `*` -> `archived` (Manual): Manual archival from any status
- `*` -> `error` (Manual): System-assigned when lifecycle incoherency or illegal transition is detected

### Lifecycle: `qa_success_lifecycle.yaml`
**Statuses:**
- `success`: Success (Terminal)
- `failure`: Failure (Terminal)
- `archived`: Archived (Terminal)

*(No transitions defined)*

### Lifecycle: `question_lifecycle.yaml`
**Statuses:**
- `open`: Open
- `answered`: Answered
- `resolved`: Resolved (Terminal)
- `deferred`: Deferred (Terminal)
- `error`: Error

**Transitions:**
- `open` -> `answered` (Manual): Answer provided to the question [Preconditions: answer or answer_ref is set]
- `answered` -> `resolved` (Manual): Answer accepted and question closed [Preconditions: answer or answer_ref is set]
- `open` -> `resolved` (Manual): Question resolved directly (answer provided and accepted in one step) [Preconditions: answer or answer_ref is set]
- `open` -> `deferred` (Manual): Question postponed for later consideration
- `answered` -> `open` (Manual): Answer rejected or insufficient, question reopened
- `answered` -> `deferred` (Manual): Answer provided but question deferred for later resolution
- `deferred` -> `open` (Manual): Deferred question reopened
- `*` -> `error` (Manual): System-assigned when lifecycle incoherency detected

### Lifecycle: `requirement_lifecycle.yaml`
**Statuses:**
- `planned`: Planned
- `active`: Active
- `complete`: Complete (Terminal)
- `deferred`: Deferred (Terminal)
- `rejected`: Rejected (Terminal)

**Transitions:**
- `planned` -> `active` (Auto): Auto-transition when linked milestone transitions to in_progress [Preconditions: At least one active milestone_ref linked, At least one active test_case_ref linked]
- `active` -> `complete` (Auto): Auto-transition when all test cases passing OR linked milestone complete
- `active` -> `deferred` (Manual): Manual deferral
- `*` -> `rejected` (Manual): Manual rejection from any status

### Lifecycle: `roadmap_lifecycle.yaml`
**Statuses:**
- `draft`: Draft
- `active`: Active
- `published`: Published
- `complete`: Complete (Terminal)
- `archived`: Archived (Terminal)

**Transitions:**
- `draft` -> `published` (Manual): Manual publication from draft [Preconditions: At least one workstream_ref or milestone_ref linked (required for roadmap structure)]
- `published` -> `active` (Manual): Manual activation from published
- `active` -> `complete` (Auto): Auto-transition when all linked milestones are complete
- `*` -> `archived` (Manual): Manual archival from any status

### Lifecycle: `role_lifecycle.yaml`
**Statuses:**
- `draft`: Draft
- `active`: Active (Terminal)
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `draft` -> `active` (Manual): Activate role
- `active` -> `archived` (Manual): Archive role
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `scheduler_health_metric_lifecycle.yaml`
**Statuses:**
- `active`: Active
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `active` -> `archived` (Auto): Archive metric after retention period
- `*` -> `error` (Auto): Metric encountered a system error

### Lifecycle: `scheduler_job_lifecycle.yaml`
**Statuses:**
- `active`: Active
- `disabled`: Disabled
- `archived`: Archived (Terminal)
- `pending`: Pending
- `error`: Error

**Transitions:**
- `pending` -> `active` (Auto): Run or activate the pending job
- `active` -> `pending` (Auto): Put job in pending state
- `active` -> `disabled` (Manual): Disable the job temporarily
- `disabled` -> `active` (Manual): Re-enable the job
- `*` -> `archived` (Manual): Archive the job permanently
- `*` -> `error` (Auto): Job encountered an execution error

### Lifecycle: `stakeholder_profile_lifecycle.yaml`
**Statuses:**
- `draft`: Draft
- `active`: Active
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `draft` -> `active` (Manual): Activate the stakeholder profile
- `active` -> `archived` (Manual): Archive the profile
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `strategic_context_lifecycle.yaml`
**Statuses:**
- `draft`: Draft
- `active`: Active
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `draft` -> `active` (Manual): Activate the strategic context
- `active` -> `archived` (Manual): Archive the context
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `strategic_plan_lifecycle.yaml`
**Statuses:**
- `planning`: Planning
- `active`: Active
- `complete`: Complete (Terminal)
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `planning` -> `active` (Manual): Activate strategic plan
- `active` -> `complete` (Manual): Complete strategic plan
- `complete` -> `archived` (Manual): Archive strategic plan
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `tde_envelope_lifecycle.yaml`
**Statuses:**
- `pending`: Pending
- `approved`: Approved
- `rejected`: Rejected (Terminal)
- `error`: Error

**Transitions:**
- `pending` -> `approved` (Manual): Approved for execution
- `pending` -> `rejected` (Manual): Rejected
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `technical_debt_lifecycle.yaml`
**Statuses:**
- `identified`: Identified
- `planned`: Planned
- `in_progress`: In Progress
- `verifying`: Verifying
- `resolved`: Resolved (Terminal)
- `deferred`: Deferred
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `identified` -> `planned` (Manual): Plan resolution for identified technical debt [Preconditions: mitigation_plan provided, target_resolution_date set]
- `planned` -> `in_progress` (Manual): Begin work on planned technical debt [Preconditions: backlog_ref provided (if work tracked in backlog)]
- `in_progress` -> `verifying` (Manual): Mark resolution work as complete and ready for verification
- `verifying` -> `resolved` (Manual): Verify that technical debt has been resolved [Preconditions: resolution_notes provided]
- `identified` -> `deferred` (Manual): Defer resolution to a later date
- `planned` -> `deferred` (Manual): Defer resolution to a later date
- `deferred` -> `planned` (Manual): Resume work on deferred technical debt
- `*` -> `archived` (Manual): Archive technical debt from any status
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `test_case_lifecycle.yaml`
**Statuses:**
- `draft`: Draft
- `active`: Active
- `metrics_captured`: Metrics Captured
- `complete`: Complete (Terminal)
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `draft` -> `active` (Manual): Test case is ready for tracking [Preconditions: path_or_id field populated, scope field set (unit, integration, e2e, etc.)]
- `active` -> `metrics_captured` (Manual): Test metrics have been captured [Preconditions: test_metrics field populated with minimum required fields, At minimum: test_count, lines_of_code, and coverage_percentage]
- `metrics_captured` -> `active` (Manual): Return to active if metrics need updating
- `metrics_captured` -> `complete` (Manual): Test suite is complete and metrics are stable [Preconditions: test_metrics.health_score >= 70 (recommended, not enforced)]
- `active` -> `complete` (Manual): Complete without metrics (allowed but not recommended)
- `complete` -> `archived` (Manual): Manual archival after completion review
- `*` -> `archived` (Manual): Manual archival from any status
- `*` -> `draft` (Manual): Manual recovery to draft state
- `*` -> `error` (Manual): System-assigned when lifecycle incoherency detected

### Lifecycle: `verification_matrix_lifecycle.yaml`
**Statuses:**
- `draft`: Draft
- `active`: Active
- `archived`: Archived (Terminal)

**Transitions:**
- `draft` -> `active` (Manual): Publish matrix for use in reports and gates
- `draft` -> `archived` (Manual): Abandon draft
- `active` -> `archived` (Manual): Retire matrix

### Lifecycle: `vision_lifecycle.yaml`
**Statuses:**
- `draft`: Draft
- `active`: Active
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `draft` -> `active` (Manual): Vision is published and active
- `active` -> `archived` (Manual): Vision is archived (no longer active)
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `vocabulary_scheme_lifecycle.yaml`
**Statuses:**
- `active`: Active
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `active` -> `archived` (Manual): Archive scheme
- `*` -> `error` (Auto): System error

### Lifecycle: `workflow_lifecycle.yaml`
**Statuses:**
- `draft`: Draft
- `active`: Active (Terminal)
- `deprecated`: Deprecated
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `draft` -> `active` (Manual): Activate workflow
- `active` -> `deprecated` (Manual): Deprecate workflow (keep for reference but mark as deprecated)
- `deprecated` -> `archived` (Manual): Archive deprecated workflow
- `active` -> `archived` (Manual): Archive active workflow
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `workstream_lifecycle.yaml`
**Statuses:**
- `planned`: Planned
- `active`: Active
- `paused`: Paused
- `complete`: Complete (Terminal)
- `archived`: Archived (Terminal)

**Transitions:**
- `planned` -> `active` (Auto): Auto-transition when all prerequisite milestones are complete
- `active` -> `complete` (Auto): Auto-transition when all linked milestones are complete
- `active` -> `paused` (Auto): Auto-transition when any linked milestone is blocked for > threshold duration
- `paused` -> `active` (Manual): Manual resume from paused
- `*` -> `archived` (Manual): Manual archival from any status

### Lifecycle: `workstream_transition_lifecycle.yaml`
**Statuses:**
- `planned`: Planned
- `active`: Active
- `complete`: Complete (Terminal)
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `planned` -> `active` (Manual): Activate transition
- `active` -> `complete` (Manual): Complete transition
- `complete` -> `archived` (Manual): Archive transition
- `*` -> `error` (Auto): System error occurred

### Lifecycle: `zqk_session_lifecycle.yaml`
**Statuses:**
- `active`: Active
- `completed`: Completed (Terminal)
- `archived`: Archived (Terminal)
- `error`: Error

**Transitions:**
- `active` -> `completed` (Auto): Session completed normally
- `active` -> `archived` (Manual): Archive session (manual or retention)
- `completed` -> `archived` (Manual): Archive completed session
- `*` -> `error` (Auto): System error occurred

## PART 4: Architecture & Governance
### 4.1 Strict Go Core Mandates
- **Zero Swallowed Errors:** All Go code must explicitly handle errors. Use of `_` for error returns is forbidden.
- **Strict Dependency Injection:** Core services must be instantiated via `OrchestratorRegistry` or factories.
- **Managed Concurrency:** Every goroutine must receive a `context.Context` and be managed by `concurrency.InterruptChecker`.

