# Architecture Documentation

**Status**: Active  
**Last Updated**: AUTO-GENERATED - Do not edit manually

This directory contains architecture documentation for the zqk system, organized by topic and versioned for clarity.

**⚠️ This README is auto-generated. To update it, run:**
```bash
./scripts/generate-architecture-readme-index.sh
```

## Documentation Index

### Core Architecture

| Document | Version | Status | Description |
|----------|---------|--------|-------------|
| [Distributed Knowledge Kernel Architecture v1.0](./distributed-kernel-architecture-v1.0.md) | 1.0.0 | Design Complete | This document defines the distributed knowledge kernel architecture using PKI... |
| [Knowledge Kernel Separation Architecture v1.0](./knowledge-kernel-separation-v1.0.md) | 1.0.0 | Design Complete | This document defines the architecture boundaries and separation of concerns ... |
| [Managing Kernel State (Git-Efficient System Object Data)](./MANAGING_KERNEL_STATE.md) | 1.0.0 | Solution documented for future implementation | Architecture documentation |
| [Scheduler Coordination Kernel - Distributed Job Awareness](./scheduler-coordination-kernel.md) | 1.0.0 | Active | Architecture documentation |

### Graph Backend

| Document | Version | Status | Description |
|----------|---------|--------|-------------|
| [Commit and Timeout Semantics](./graph-backend/COMMIT_AND_TIMEOUT_SEMANTICS.md) | 1.0.0 | Active | Architecture documentation |
| [Content-Addressable Storage for Graph Backend](./content-addressable-storage-graph-backend.md) | 1.0.0 | Active | Architecture documentation |
| [Enabling the Graph Backend](./graph-backend/ENABLING_GRAPH_BACKEND.md) | 1.0.0 | Active | Architecture documentation |
| [Graph Backend Architecture Decisions](./graph-backend/ARCHITECTURE_DECISIONS.md) | 1.0.0 | Active | Architecture documentation |
| [GraphRAG Schema Design v1.0 - Three-Layer Architecture](./graphrag-schema-design-v1.0.md) | 1.0.0 | Design Complete | This document defines the GraphRAG schema architecture for the zqk Knowledge ... |
| [MemGraph Dependencies and Integration Requirements v1.0](./memgraph-research-v1.0.md) | 1.0.0 | Research Complete | This document provides research on MemGraph dependencies, integration require... |
| [Metrics Configuration Guide](./graph-backend/CONFIGURATION.md) | 1.0.0 | Active | The observability system is fully configurable, adjustable, and non-blocking.... |
| [Observability and Metrics](./graph-backend/OBSERVABILITY.md) | 1.0.0 | Active | The graph backend provides comprehensive observability for self-healing and c... |
| [Pluggable Graph Backend Interface Architecture v1.0](./pluggable-graph-backend-interface-v1.0.md) | 1.0.0 | Design Complete | This document defines the pluggable graph backend interface architecture for ... |
| [Pluggable Validator System Architecture](./pluggable-validators-v1.0.md) | 1.0.0 | Active | The validation system is designed to be pluggable, allowing users to specify ... |
| [Semantic Graph Traversal Engine](./semantic_graph_traversal.md) | 1.0.0 | Active | Architecture documentation |
| [Shared Implementation Patterns](./graph-backend/SHARED_IMPLEMENTATIONS.md) | 1.0.0 | Active | To keep code DRY across different graph backend providers, we've extracted co... |
| [The Cryptographic Transceiver: Zero-Trust Mesh Routing](./CRYPTOGRAPHIC_TRANSCEIVER.md) | 1.0.0 | Active | Architecture documentation |
| [Vocabulary schemes and glossary term relations](./VOCABULARY_GRAPH.md) | 1.0.0 | Active | Architecture documentation |

### Storage & Integrity

| Document | Version | Status | Description |
|----------|---------|--------|-------------|
| [Deferred Integrity Hash Updates](./deferred-hash-updates-v1.0.md) | 1.0.0 | Active | Integrity hashes are now computed and updated **only after all pending operat... |
| [Domain Registry and Namespace System Integration](./DOMAIN_REGISTRY_NAMESPACE_INTEGRATION.md) | 1.0.0 | Active | This document describes how the **Domain Registry** system and the **Namespac... |
| [Graph-Based Spec Storage v1.0](./graph-spec-storage-v1.0.md) | 1.0.0 | Design Complete | Object specifications can be stored in the graph backend as nodes, enabling d... |
| [Hash Registry Cache Optimization](./system/hash_registry_cache_optimization.md) | 1.0.0 | Active | Architecture documentation |
| [Hash Registry Design v1.0](./hash-registry-design-v1.0.md) | 1.0.0 | Design Complete | The Hash Registry provides consistent integrity verification across both file... |
| [Hash Registry Location for Bucketed Storage](./hash-registry-bucketed-storage-v1.0.md) | 1.0 | Active | Architecture documentation |
| [Hash Registry Sync Analysis and Prevention](./HASH_REGISTRY_SYNC_ANALYSIS.md) | 1.0.0 | Active | Architecture documentation |
| [Health Check Registry (Monitor/Beacon) Design](./HEALTH_CHECK_REGISTRY_DESIGN.md) | 1.0.0 | Design + minimal implementation | Architecture documentation |
| [MCP Agent Registry](./MCP_AGENT_REGISTRY.md) | 1.0.0 | Active | The agent registry allows you to **pre-register agents** with expected roles ... |

### Migration

| Document | Version | Status | Description |
|----------|---------|--------|-------------|
| [Cross-profile data cell migration — operator runbook](./DATA_CELL_CROSS_PROFILE_MIGRATION_RUNBOOK.md) | 1.0.0 | Active | Architecture documentation |
| [Legacy Codebase Migration Strategy v1.0](./legacy-codebase-migration-v1.0.md) | 1.0.0 | Active | This document defines a comprehensive strategy for safely migrating legacy co... |
| [Mandatory Coordinator Integration - Migration Complete](./coordinator-mandatory-migration-v1.0.md) | 1.0.0 | Active | All coordinator integration is now **mandatory** - legacy optional patterns h... |
| [Migration Binary Detection Strategy](./migration-binary-detection-strategy.md) | 1.0.0 | Design | This document defines how the zqk orchestrator detects and handles the option... |
| [Migration Binary Manifest Compatibility Constraints](./migration-manifest-compatibility.md) | 1.0.0 | Implemented | The migration binary manifest can specify compatibility constraints to ensure... |
| [Migration Builder Architecture](./MIGRATION_BUILDER_ARCHITECTURE.md) | 1.0.0 | Design | Migrations should follow the specbuilder pattern: |
| [Migration Snapshot Coherence](./MIGRATION_SNAPSHOT_COHERENCE.md) | 1.0.0 | Design | Architecture documentation |
| [Migration Spec Design](./MIGRATION_SPEC_DESIGN.md) | 1.0.0 | Design | Migrations should be spec-driven rather than individual commands. A migration... |
| [Migration Strategy: File-Based to Graph Backend v1.0](./migration-strategy-file-to-graph-v1.0.md) | 1.0.0 | Design Complete | This document defines the comprehensive migration strategy for transitioning ... |
| [Path-cache migration scan](./PATH_CACHE_MIGRATION_SCAN.md) | 1.0.0 | Active | Architecture documentation |
| [SpecBuilder Migration Guide](./specbuilder-migration-guide-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Stable log keys (POL-CODE-007) — migration status](./LOG_EVENT_KEYS_MIGRATION.md) | 1.0.0 | Active | Architecture documentation |

### CLI & Interface

| Document | Version | Status | Description |
|----------|---------|--------|-------------|
| [](./system/CLI_OPERATION_NOTIFIER.md) | 1.0.0 | Active | Architecture documentation |
| [AI Agent CLI-First Workflow](./AI_AGENT_CLI_FIRST_WORKFLOW.md) | 1.0.0 | Active | Architecture documentation |
| [Async CLI retrofit and incremental validation cache](./async-cli-retrofit-and-validation-cache.md) | 1.0.0 | Active | Architecture documentation |
| [Built-In MCP Tools vs. CLI Bridge Tools](./BUILT_IN_VS_CLI_TOOLS.md) | 1.0.0 | Active | Architecture documentation |
| [CLI Architecture & Semantic Structure](./CLI_ARCHITECTURE.md) | 1.0.0 | Active | Architecture documentation |
| [CLI Async and Progress: Never Hang Without Feedback](./CLI_ASYNC_AND_PROGRESS.md) | 1.0.0 | Active | Architecture documentation |
| [CLI Builder Architecture](./CLI_BUILDER_ARCHITECTURE.md) | 1.0.0 | Active | Architecture documentation |
| [CLI Command Structure Consistency Review](./cli-command-consistency-review.md) | 1.0.0 | Active | This document reviews the consistency and clarity of CLI commands that manipu... |
| [CLI Error Guard Options](./CLI_ERROR_GUARD_OPTIONS.md) | 1.0.0 | Active | Architecture documentation |
| [CLI Ontology Specification v1.0](./cli-ontology-v1.0.md) | 1.0.0 | Design Complete | This document establishes a formal CLI ontology specification that enables bo... |
| [CLI Session Reuse and Auth Integration v1.0](./CLI_SESSION_REUSE_AND_AUTH_V1.md) | 1.0 | Design | Architecture documentation |
| [CLI Standardization and Dry-Run Context v1.0](./cli-standardization-v1.0.md) | 1.0.0 | Active | The zqk CLI needs standardized command forms and a dry-run context system to ... |
| [CLI UX Overhaul Plan (PRI-EXAMPLE)](./cli_ux_overhaul.md) | 1.0.0 | Active | Architecture documentation |
| [CLI Version Handling Strategy](./cli-version-handling.md) | 1.0.0 | Implemented | This document describes how the zqk CLI handles different versions of command... |
| [CLI Wrapper Pattern](./CLI_WRAPPER_PATTERN.md) | 1.0.0 | Active | Architecture documentation |
| [CLI alpha launch — technical plan](./CLI_ALPHA_LAUNCH_PLAN.md) | 1.0.0 | Plan (executable) | Architecture documentation |
| [CLI as Normative Path for Object Operations v1.0](./cli-normative-path-v1.0.md) | 1.0.0 | Design Complete | All object operations (create, update, delete) **must** be performed through ... |
| [CLI context patterns (tests and handlers)](./CLI_CONTEXT_TEST_PATTERNS.md) | 1.0.0 | Active | Architecture documentation |
| [CLI external hook protocol (tray + cli-hooks)](./CLI_EXTERNAL_HOOK_PROTOCOL.md) | 1.0.0 | Active | Architecture documentation |
| [CLI membrane and system anatomy (cells, organelles, systems)](./CLI_MEMBRANE_AND_SYSTEM_ANATOMY.md) | 1.0.0 | Architecture (vocabulary + current vs target) | Architecture documentation |
| [CLI object create / update / delete — why the command can pause (bounded)](./CLI_OBJECT_MUTATION_LATENCY.md) | 1.0.0 | Operational reference | Architecture documentation |
| [CLI performance and consistency](./CLI_PERFORMANCE_AND_CONSISTENCY.md) | 1.1 | Active | Architecture documentation |
| [CLI vs Direct YAML Elicitation](./CLI_VS_DIRECT_YAML_ELICITATION.md) | 1.0.0 | Active | Architecture documentation |
| [CLI-First Enforcement Summary](./CLI_FIRST_ENFORCEMENT_SUMMARY.md) | 1.0.0 | Active | Architecture documentation |
| [CMDv2 and CLI Split: Rationale and Plan](./CMDV2_AND_CLI_SPLIT_RATIONALE.md) | 1.0.0 | Active | Architecture documentation |
| [MCP CLI Bridge Architecture v1.0](./mcp-cli-bridge-v1.0.md) | 1.0.0 | Active | This document describes the context-driven, security-aware CLI bridge that au... |
| [MCP Client Pattern for Programmatic Use](./MCP_CLIENT_PATTERN.md) | 1.0.0 | Active | The Model Context Protocol (MCP) is typically used by desktop clients (like C... |
| [MCP/CLI-First Elicitation Prompt](./MCP_CLI_FIRST_ELICITATION.md) | 1.0.0 | Active | Architecture documentation |
| [Native matrix CLI strategy (vetting & traceability)](./MATRIX_CLI_STRATEGY.md) | 1.0.0 | Active | Architecture documentation |
| [Semantic kernel and the CLI façade](./SEMANTIC_KERNEL_AND_CLI_FACADE.md) | 1.0.0 | Concept (architecture). Process traceability: `doc_entry` **DOC-EXAMPLE**; `glossary_term` **GLS-EXAMPLE** (via `zqk object create`). | Architecture documentation |
| [Stability for Multi-Agent Workflow and CLI Message Bus](./STABILITY_FOR_MULTI_AGENT_AND_CLI_MESSAGE_BUS.md) | 1.0.0 | Active | Architecture documentation |

### Documentation & Tooling

| Document | Version | Status | Description |
|----------|---------|--------|-------------|
| [Assessment, onboarding, and alpha launch — document index](./ASSESSMENT_AND_ONBOARDING_INDEX.md) | 1.0.0 | Active | Architecture documentation |
| [Concurrency Architecture Documentation](./concurrency/README.md) | 1.0.0 | Active | Architecture documentation |
| [Goroutine Manager Documentation Index](./GOROUTINE_MANAGER_INDEX.md) | 1.0.0 | Active | Architecture documentation |
| [High-Volume Event Indexes: Efficient Management and Bundled-Storage Alignment](./HIGH_VOLUME_EVENT_INDEXES.md) | 1.0.0 | Active | Architecture documentation |
| [Index-First, Low-CPU Scan Strategy](./INDEX_FIRST_LOW_CPU_SCAN_DESIGN.md) | 1.0.0 | Active | Architecture documentation |
| [README Index Auto-Generation System v1.0](./readme-index-generation-v1.0.md) | 1.0.0 | Active | The system automatically generates README index tables for all object directo... |
| [Reverse Reference Index Design](./REVERSE_REFERENCE_INDEX_DESIGN.md) | 1.0.0 | Active | Architecture documentation |

### Other

| Document | Version | Status | Description |
|----------|---------|--------|-------------|
| [](./SCENARIO_BUNDLES_AND_TRACEABILITY.md) | 1.0.0 | Active | Architecture documentation |
| [.zqk/ Markdown Cleanup: Analysis and Remedial Action Plan](./ZQK_DOT_MD_CLEANUP_PLAN.md) | 1.0.0 | Draft | Architecture documentation |
| [AI Agent Communication Channels](./ai-agent-communication-channels-v1.0.md) | 1.0.0 | Active | zqk supports multiple communication channels for AI agent coordination, enabl... |
| [AI Agent YAML Edit Policy](./AI_AGENT_YAML_EDIT_POLICY.md) | 1.0.0 | Active | Architecture documentation |
| [API Integration Pattern (Spec-Builder)](./API_INTEGRATION_PATTERN.md) | 1.0.0 | Active | As ZQK scales, we will integrate with numerous third-party APIs (e.g., Fal.ai... |
| [APISpec Builder Pattern Architecture](./apispec_builder.md) | 1.0.0 | Active | The APISpec Builder pattern provides a standardized, thread-safe approach to ... |
| [Accounts, Roles, and Permission Hierarchy v1.0](./mcp/accounts-roles-permissions-hierarchy-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Advanced Query Capabilities v1.0](./advanced-query-capabilities-v1.0.md) | 1.0.0 | Active | This document defines the advanced query capabilities for the zqk object stor... |
| [Agent Tool Selection Analysis: Direct File Write vs CLI](./AGENT_TOOL_SELECTION_ANALYSIS.md) | 1.0.0 | Active | Architecture documentation |
| [Agent orchestration — priority audible (2026)](./AGENT_ORCHESTRATION_AUDIBLE.md) | 1.0.0 | Active | Architecture documentation |
| [Aggregate-audit CPU profile findings](./AGGREGATE_AUDIT_CPU_PROFILES.md) | 1.0.0 | Active | Architecture documentation |
| [Alpha: Onboarding, Gating, and Progressive Access](./ALPHA_ONBOARDING_AND_GATING.md) | 1.0.0 | Active | Architecture documentation |
| [Ambience Engine & Anticipatory Logic](./ambience_engine.md) | 1.0.0 | Active | Architecture documentation |
| [Ambient Engine Architecture & Feature Gating](./AMBIENT_DAEMON_ARCHITECTURE.md) | 1.0.0 | Active | Architecture documentation |
| [Ambient Orchestration: Omnipresent Context for the Swarm](./AMBIENT_ORCHESTRATION_SWARM.md) | 1.0.0 | Active | Architecture documentation |
| [Architecture Blueprint: ZQK MCP Fal.ai Integration](./mcp_fal_integration.md) | 1.0.0 | Active | Architecture documentation |
| [Architecture Decision Record: Graph Database Engine (QLever vs Neo4j)](./decisions/ADR-QLEVER-NEO4J.md) | 1.0.0 | Active | Architecture documentation |
| [Architecture Decision Record: MCP Server Scalability](./ADR-MCP-SCALABILITY.md) | 1.0.0 | Active | Architecture documentation |
| [Architecture Decision Record: Matrix Swarm Pattern](./MATRIX_SWARM_PATTERN.md) | 1.0.0 | Active | Architecture documentation |
| [Architecture Decision Records (ADRs)](./ARCHITECTURE_DECISION_RECORDS.md) | 1.0.0 | Active | Architecture Decision Records (ADRs) document important architectural decisio... |
| [Architecture Enforcement Mechanisms](./ARCHITECTURE_ENFORCEMENT.md) | 1.0.0 | Active | While pre-commit hooks provide immediate feedback, architecture enforcement s... |
| [Architecture Governance Framework](./ARCHITECTURE_GOVERNANCE.md) | 1.0.0 | Active | This document defines the complete lifecycle and policy framework to ensure A... |
| [Architecture Lifecycle Reminders](./ARCHITECTURE_LIFECYCLE_REMINDERS.md) | 1.0.0 | Active | The lifecycle reminder system (see [BLI-092](../../backlog/BLI-092.yaml)) can... |
| [Architecture Patterns Library](./ARCHITECTURE_PATTERNS.md) | 1.0.0 | Active | This document serves as the authoritative source for architecture patterns in... |
| [Architecture Review Process](./ARCHITECTURE_REVIEW_PROCESS.md) | 1.0.0 | Active | This process ensures that all architecture changes follow established pattern... |
| [Async Router and Validation Workers - Improvement Analysis](./async-router-validation-improvements.md) | 1.0.0 | ✅ **Implemented | Architecture documentation |
| [Async Validation Architecture v1.0](./async-validation-architecture-v1.0.md) | 1.0.0 | Active | The async validation system transforms system check from a synchronous, block... |
| [Async Validation Control Flow & Lock Analysis](./ASYNC_VALIDATION_FLOW_DIAGRAM.md) | 1.0.0 | Active | Architecture documentation |
| [Async Validation Flow Analysis](./system/async_check_flow_analysis.md) | 1.0.0 | Active | Architecture documentation |
| [Async Validation System v1.0](./async-validation-system-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Async Validation Testing Guide v1.0](./async-validation-testing-v1.0.md) | 1.0.0 | Active | The async validation system includes comprehensive tests that use isolated te... |
| [Async retrofit and CLI status](./ASYNC_RETROFIT_STATUS.md) | 1.0.0 | Active | Architecture documentation |
| [Atomic Shutdown Flag Implementation](./mcp/ATOMIC_SHUTDOWN_IMPLEMENTATION.md) | 1.0.0 | Active | Architecture documentation |
| [Audit Event Aggregation](./AUDIT_EVENT_AGGREGATION.md) | 1.0.0 | Active | The audit event aggregation system compresses audit events into summary event... |
| [Audit Event Aggregation Strategy v1.0](./audit-event-aggregation-v1.0.md) | 1.0.0 | ✅ Implemented | Architecture documentation |
| [Audit Event Buffer Refactor: Singleton to Registry Pattern](./AUDIT_BUFFER_REFACTOR.md) | 1.0.0 | Active | To address cross-test resource contention and intermittent deadlocks identifi... |
| [Audit Event Bulk Operation Buffering v1.0](./audit-event-bulk-buffering-v1.0.md) | 1.0.0 | Design | Architecture documentation |
| [Audit Stream File Format](./AUDIT_STREAM_FORMAT.md) | 1.0.0 | Design / implementation | Architecture documentation |
| [Authority Resolution and Multi-User Collaboration v1.0](./authority-resolution-and-collaboration-v1.0.md) | 1.0.0 | Active | This document defines a comprehensive system for authority resolution, user c... |
| [Auto-Fix Pattern Recommendation](./AUTO_FIX_PATTERN.md) | 1.0.0 | Active | Architecture documentation |
| [Autonomous Capability Synthesis](./convergence/CAPABILITY_SYNTHESIS.md) | 1.0.0 | Active | Architecture documentation |
| [Autonomous Capability Synthesis: OrchestrationManager Design](./autonomous-capability-synthesis/DESIGN.md) | 1.0.0 | Active | The `OrchestrationManager` (OM) is the core actor responsible for transformin... |
| [Autonomy Inbox & TDE Envelopes](./tde_envelope_inbox.md) | 1.0.0 | Active | Architecture documentation |
| [Backlog Exploration Lane ("Boneyard")](./BACKLOG_EXPLORATION_LANE.md) | 1.0.0 | Active | Architecture documentation |
| [Backlog document references, auto-linking, and CLI creation veneers](./BACKLOG_REFS_AUTOLINK_AND_CREATE_VENEERS.md) | 1.0.0 | Roadmap / design memory (not a shipped feature checklist) | Architecture documentation |
| [Backlog items: Cache-first system check architecture](./system-check-cache-first-backlog.md) | 1.0.0 | Active | Architecture documentation |
| [Baseline Data Inventory — `.zqk` metrics, reports, and aggregations](./BASELINE_DATA_INVENTORY.md) | 1.0.0 | Active | Architecture documentation |
| [Baseline Testing Results](./baseline-results-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Bootstrap Requirements for File Backend](./BOOTSTRAP_REQUIREMENTS.md) | 1.0.0 | Active | The `zqk system init` command must bootstrap a complete, ready-to-use project... |
| [Bucketing Strategy Spec Design](./system/bucketing_strategy_spec_design.md) | 1.0.0 | Active | Architecture documentation |
| [Build System Documentation](./BUILD_SYSTEM.md) | 1.0.0 | Active | The zqk build system provides a centralized configuration for building multip... |
| [Builder vs Compressor Architecture](./BUILDER_VS_COMPRESSOR_ARCHITECTURE.md) | 1.0.0 | Active | The system has two distinct but complementary concepts: |
| [Bulletproof Shutdown Implementation](./mcp/BULLETPROOF_SHUTDOWN.md) | 1.0.0 | Active | Architecture documentation |
| [Bypass-Kind Storage: Avoiding CAS Overhead](./BYPASS_KIND_STORAGE.md) | 1.0.0 | Design | Architecture documentation |
| [CAS list vs get consistency: one source of truth](./CAS_LIST_GET_CONSISTENCY.md) | 1.0.0 | Active | Architecture documentation |
| [CRIT-DATACELL-001 — Strict nucleus boundary](./CRIT_DATACELL_001_STRICT_BOUNDARY.md) | 1.0.0 | Active | Architecture documentation |
| [CRUD Operations Must Consider Bucketing and Archiving Strategies](./CRUD_BUCKETING_ARCHIVING_REQUIREMENTS.md) | 1.0.0 | Active | Architecture documentation |
| [CRUD Operations Standardization Guide](./CRUD_OPERATIONS_STANDARDIZATION.md) | 1.0.0 | Active | Architecture documentation |
| [Cache Item Strategy Evolution Path](./system/CACHE_ITEM_STRATEGY_EVOLUTION.md) | 1.0.0 | Active | Architecture documentation |
| [Cache Item Strategy Hook Usage](./system/CACHE_ITEM_STRATEGY_USAGE.md) | 1.0.0 | Active | The cache item strategy hook provides a configurable mechanism for special ha... |
| [Cache Management Strategy](./CACHE_MANAGEMENT_STRATEGY.md) | 1.0.0 | Active | Architecture documentation |
| [Cache Pre-Warming Dependency Chain](./CACHE_PREWARM_DEPENDENCIES.md) | 1.0.0 | Active | Architecture documentation |
| [Cache Staleness Detection](./cache-staleness-detection.md) | 1.0.0 | Implemented | The object ID cache uses a two-tier staleness detection system to determine w... |
| [Capability Maturity Ontology v1.0](./capability-maturity-ontology-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Capability Object Architecture](./capability_object.md) | 1.0.0 | Active | Architecture documentation |
| [Cascade Delete Requirements](./cascade-delete-requirements-v1.0.md) | 1.0.0 | Active | Cascade delete is the process of handling dependent objects when an object is... |
| [Cascade Deletion and Update Analysis](./cascade-analysis-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Cascade Rules for Object References](./cascade-rules-v1.0.md) | 1.0.0 | Active | When objects are deleted or updated, dependent objects that reference them ne... |
| [Check Command Cache: Legacy vs Current Implementation](./check-cache-comparison.md) | 1.0.0 | Comparison Document | This document compares the legacy reference checking implementation with the ... |
| [Check Command Fast Mode](./check-fast-mode.md) | 1.0.0 | Implemented | The `zqk check` command now supports a `--fast` flag to skip reference integr... |
| [Check Command Fixes - Status](./check-fixes-status.md) | 1.0.0 | Partially Complete | Architecture documentation |
| [Check Command Improvements Summary](./check-improvements-summary.md) | 1.0.0 | Implemented | Architecture documentation |
| [Check Command Optimization - Final Results](./check-optimization-final.md) | 1.0.0 | ✅ Implemented and Working | Architecture documentation |
| [Check Command Optimization Results](./check-optimization-results.md) | 1.0.0 | Implemented | Architecture documentation |
| [Check Command Performance & Caching Strategy v1.0](./check-command-performance-v1.0.md) | 1.0.0 | Design | Architecture documentation |
| [Check Command Test Results](./check-test-results.md) | 1.0.0 | Active | Architecture documentation |
| [Check Command: Legacy zqk vs zqk](./check-command-legacy-vs-nexos.md) | 1.0.0 | Comparison Document | This document compares the legacy `zqk` CLI check command with the current `z... |
| [Check Violations Resolution Plan](./check-violations-resolution-plan.md) | 1.0.0 | In Progress | Architecture documentation |
| [Cleanup Maintenance: On-Demand Config-Driven Filesystem and CLI Tasks](./CLEANUP_MAINTENANCE_DESIGN.md) | 1.0.0 | Design | Architecture documentation |
| [Code Evaluation and Refactoring Policy](./CODE_EVALUATION_POLICY.md) | 1.0.0 | Active | This policy establishes a systematic approach to evaluating code for refactor... |
| [Command Organization and Hierarchy](./COMMAND_ORGANIZATION.md) | 1.0.0 | Active | Architecture documentation |
| [Command Path Brand Prefix Update](./mcp/COMMAND_PATH_BRAND_PREFIX.md) | 1.0.0 | Active | All hardcoded command path references have been updated to use the configurab... |
| [Command Result Builder](./mcp/COMMAND_RESULT_BUILDER.md) | 1.0.0 | Active | The `CommandResultBuilder` provides a fluent API for building standardized CL... |
| [Command Spec Coverage Report](./COMMAND_SPEC_COVERAGE.md) | 1.0.0 | Active | Architecture documentation |
| [Command Spec Pattern](./COMMAND_SPEC_PATTERN.md) | 1.0.0 | Active | The Command Spec pattern extends the CommandBuilder with a spec-driven approa... |
| [Command orchestration (storage, session, scheduler check)](./COMMAND_ORCHESTRATION.md) | 1.0.0 | Active | Architecture documentation |
| [Component Loader Pattern](./COMPONENT_LOADER_PATTERN.md) | 1.0.0 | Implemented | Architecture documentation |
| [Comprehensive Resolver Architecture Design](./RESOLVER_ARCHITECTURE_DESIGN.md) | 1.0.0 | Design | Architecture documentation |
| [Comprehensive Testing Status](./comprehensive-testing-status-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Compressed Snapshot Format Design](./COMPRESSED_SNAPSHOT_FORMAT.md) | 1.0.0 | Active | Snapshots should be stored in a compressed format that: |
| [Concurrency Analysis: System Check Command](./concurrency/CONCURRENCY_ANALYSIS_SYSTEM_CHECK.md) | 1.0.0 | Active | Architecture documentation |
| [Concurrency Cleanup Summary](./concurrency/CONCURRENCY_CLEANUP_SUMMARY.md) | 1.0.0 | Active | Architecture documentation |
| [Concurrency Fixes Summary](./concurrency/CONCURRENCY_FIXES_SUMMARY.md) | 1.0.0 | Active | Architecture documentation |
| [Concurrency Guardrail Addendum: High-Speed Object Caching (PRI-17807959)](./cache_tiger_team_strategy.md) | 1.0.0 | Active | This addendum details the impact of introducing an asynchronous Semantic Cach... |
| [Concurrency Pattern Deviations Analysis](./concurrency/CONCURRENCY_PATTERN_DEVIATIONS_ANALYSIS.md) | 1.0.0 | Active | Architecture documentation |
| [Concurrency Pattern Reconciliation Summary](./concurrency/CONCURRENCY_PATTERN_RECONCILIATION_SUMMARY.md) | 1.0.0 | Active | Architecture documentation |
| [Concurrency Patterns](./concurrency-patterns-v1.0.md) | 1.0.0 | Implemented | Architecture documentation |
| [Concurrency Standardization - Completion Summary](./concurrency/CONCURRENCY_STANDARDIZATION_COMPLETION_SUMMARY.md) | 1.0.0 | Active | Architecture documentation |
| [Concurrency Standardization - Executive Summary](./concurrency/CONCURRENCY_STANDARDIZATION_SUMMARY.md) | 1.0.0 | Active | Architecture documentation |
| [Concurrency Standardization Plan](./concurrency/CONCURRENCY_STANDARDIZATION_PLAN.md) | 1.0.0 | Active | Architecture documentation |
| [Concurrency Standardization Progress](./concurrency/CONCURRENCY_STANDARDIZATION_PROGRESS.md) | 1.0.0 | Active | Architecture documentation |
| [Concurrency Standardization Violations](./concurrency/CONCURRENCY_STANDARDIZATION_VIOLATIONS.md) | 1.0.0 | Active | Architecture documentation |
| [Concurrency and Deterministic Waits - Resolution Summary](./concurrency/CONCURRENCY_DETERMINISTIC_WAITS_SUMMARY.md) | 1.0.0 | Active | Architecture documentation |
| [Concurrent Operations System](./concurrent-operations-v1.0.md) | 1.0.0 | Active | The system must handle multiple concurrent operations (create, update, delete... |
| [Concurrent Operations System - Enhancements](./concurrent-operations-enhancements-v1.0.md) | 1.0.0 | Active | Enhanced the concurrent operations system with: |
| [Concurrent Operations System - Implementation Summary](./concurrent-operations-summary-v1.0.md) | 1.0.0 | Active | A comprehensive system for handling concurrent create/update/delete operation... |
| [Configurable Brand Prefix for Tool Names](./mcp/BRAND_PREFIX.md) | 1.0.0 | Active | All MCP tool names now use a configurable brand prefix derived from the execu... |
| [Constants and DRY Inventory Plan](./CONSTANTS_AND_DRY_INVENTORY_PLAN.md) | 1.0.0 | Active | Architecture documentation |
| [Content-Addressable Storage Design (Final)](./content-addressable-storage-design-final.md) | 1.0.0 | Active | Architecture documentation |
| [Content-Addressable Storage Design (Git-Style)](./content-addressable-storage-design.md) | 1.0.0 | Active | Architecture documentation |
| [Content-Addressable Storage Design v2 (Compatible)](./content-addressable-storage-design-v2.md) | 1.0.0 | Active | Architecture documentation |
| [Content-Addressable Storage Phase 1 Status](./CONTENT_ADDRESSABLE_STORAGE_PHASE1_STATUS.md) | 1.0.0 | Active | Architecture documentation |
| [Content-Addressable Storage Search Gap](./CONTENT_ADDRESSABLE_STORAGE_SEARCH_GAP.md) | 1.0.0 | Active | Architecture documentation |
| [Content-Addressable Storage: Duplicate ID Impact Analysis](./CAS_DUPLICATE_ID_IMPACT_ANALYSIS.md) | 1.0.0 | ✅ Should work correctly | Architecture documentation |
| [Context Architecture: Single Context Principle](./cli-context/architecture-v1.0.md) | 1.0.0 | Active | The context system follows a **Single Context Principle**: everything should ... |
| [Context Building Patterns](./cli-context/patterns-v1.0.md) | 1.0.0 | Active | The context system supports multiple patterns for building contexts: |
| [Context-Aware vs Context-Agnostic Operations](./context-aware-vs-context-agnostic-operations.md) | 1.0.0 | Active | This document describes the critical distinction between **context-aware** an... |
| [Convergence orchestration, outcome aggregation, and nested CVS](./CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md) | 1.0.0 | MVP — **`pkg/convergerollup`**, **`rollup_status_core`** in **`zqk scheduler convergence measure`**, JSONL **`rollup_status_core.jsonl`**; **`scripts/cvs_outcome_rollup.py`** (full matrix/drift), **`cvs_convergence_orchestrate.sh`** (persist + rollup). | Architecture documentation |
| [Convergence phase router, tombstones, and coordinator-backed routing](./CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md) | 1.0.0 | Design with **implemented** pieces in-tree (phase router, session routing context, suggested CVS fields from `zqk scheduler convergence measure`); coordinator-backed async evaluation remains **partial / optional** per profile notes. Traceability via `doc_entry` + `requirement` + `criteria` in process data. | Architecture documentation |
| [Convergence predicates and gates](./CONVERGENCE_PREDICATES_AND_GATES.md) | 1.0.0 | Active | Architecture documentation |
| [Coordinator Integration Gaps](./coordinator-integration-gaps.md) | 1.1 | ✅ **Integrated | Architecture documentation |
| [Coordinator Integration Test Coverage](./coordinator-integration-test-coverage.md) | 1.0 | ✅ **ALL TESTS PASSING | Architecture documentation |
| [Coordinator Pattern Violations](./concurrency/COORDINATOR_PATTERN_VIOLATIONS.md) | 1.0.0 | Active | Architecture documentation |
| [Critical Next Steps for Test and Scheduler Stability](./STABILITY_NEXT_STEPS.md) | 1.0.0 | Active | Architecture documentation |
| [Cross-Process File Locking Pattern](./shared-resource-locking.md) | 1.0.0 | Active | When a resource is shared across processes (e.g., files, queues, registries),... |
| [Data Management Optimization - Progress Report](./data-management-progress.md) | 1.0.0 | In Progress | Architecture documentation |
| [Data Management Optimization Plan](./data-management-optimization-plan.md) | 1.0.0 | In Progress | Architecture documentation |
| [Data Storage Production-Readiness Roadmap](./DATA_STORAGE_PRODUCTION_ROADMAP.md) | 1.0.0 | Active | Architecture documentation |
| [Data cell model (logical unit + storage profiles)](./DATA_CELL_MODEL.md) | 1.0.0 | Architecture — **intent and vocabulary** (product direction). **Spec requirement + construction pipeline** are **decided** in [ADR-DATA-CELL-SPEC-PIPELINE-v1.0.md](../process/decisions/ADR-DATA-CELL-SPEC-PIPELINE-v1.0.md). **Reality check:** partial **membrane** read paths and **operational envelope** wiring exist in code (see table below); remaining gaps are **full** stewardship/coordinator execution, a **unified cell CRUD** product API, **migration** tooling, and **admin** surfaces — not “nothing implemented yet.” | Architecture documentation |
| [Data cell — runtime organism (protocol v1)](./DATA_CELL_RUNTIME_ORGANISM.md) | 1.0.0 | Implemented layout + Go API (`pkg/datacell`). **Spec / mapping closure:** `BLI-EXAMPLE` (formal mapping recorded in [DATA_CELL_MODEL.md](./DATA_CELL_MODEL.md) § *BLI-EXAMPLE — Formal mapping and sequencing*). | Architecture documentation |
| [Agent correspondence feed standard](./AGENT_CORRESPONDENCE_FEED_STANDARD.md) | 0.1.0 | Initiative: single ZQK-owned `agent_feed`, MCP wake/health, human steering CLI → enterprise messaging bridges. Plan `PRI-EXAMPLE`. | Architecture documentation |
| [Data origination from specs — pipeline vision (target)](./DATA_ORIGINATION_PIPELINE_VISION.md) | 1.0.0 | Stage names and order are **defined** in code (`pkg/specorigination` + `pipeline_kind` **`spec.origination`**). **Implementations** that call `zqk` subprocesses or storage APIs per stage are still **incremental**; this section is the contract to converge on. | Architecture documentation |
| [Data pipeline pilot selection](./data-pipeline-pilot-selection.md) | 0.1 | Active | Architecture documentation |
| [Data stream summary pilot (test-bundle health)](./DATA_STREAM_SUMMARY_PILOT.md) | 1.0.0 | Pilot implementation for REQ-DATASTREAM-001 (queryable summary per logical stream). | Architecture documentation |
| [Data-Driven Decision Framework](./DATA_DRIVEN_DECISION_FRAMEWORK.md) | 1.0.0 | Active | Architecture documentation |
| [Deadlock Analysis Checklist](./DEADLOCK_ANALYSIS_CHECKLIST.md) | 1.0.0 | Active | Architecture documentation |
| [Deadlock Analysis Methodology](./DEADLOCK_ANALYSIS_METHODOLOGY.md) | 1.0.0 | Active | This document outlines the systematic methodology for analyzing potential dea... |
| [Decision Lifecycle: System Awareness and Recurring Considerations](./DECISION_LIFECYCLE.md) | 1.0.0 | Active | The decision lifecycle establishes appropriate levels of system awareness and... |
| [Declarative product launch as a matrix-shaped pipeline](./DECLARATIVE_LAUNCH_AND_MATRIX_LOOP.md) | 1.0.0 | Design note (pre-final URLs) | Architecture documentation |
| [Default Configs and Bootstrap](./DEFAULT_CONFIG_AND_BOOTSTRAP.md) | 1.0.0 | Active | Architecture documentation |
| [Design Improvements & Intuitive Features](./DESIGN_IMPROVEMENTS.md) | 1.0.0 | Complete | This document tracks design improvements, fluidity enhancements, and intuitiv... |
| [Design Patterns Library](./DESIGN_PATTERNS.md) | 1.0.0 | Active | This document catalogs the design patterns used throughout zqk to solve commo... |
| [Design Specification: Magic Onboarding (`zqk join`)](./magic-onboarding-design.md) | 1.0.0 | Active | Architecture documentation |
| [Design Specification: The Federated Economy](./federated-economy-design.md) | 1.0.0 | Active | Architecture documentation |
| [Diagnostic Report Mining](./DIAGNOSTIC_REPORT_MINING.md) | 1.0.0 | Active | Architecture documentation |
| [Digital Asset Object Schema](./digital_asset_schema.md) | 1.0.0 | Active | Architecture documentation |
| [Dispatch Loop: Outline and MCP Use](./DISPATCH_LOOP_AND_MCP.md) | 1.0.0 | Active | Architecture documentation |
| [Document Query System v1.0](./document-query-system-v1.0.md) | 1.0.0 | Active | The document query system enables users to discover and search documentation ... |
| [Dynamic Agent Registration Strategy v1.0](./mcp/dynamic-agent-registration-strategy-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Error Response Builder](./mcp/ERROR_RESPONSE_BUILDER.md) | 1.0.0 | Active | The `ErrorResponseBuilder` provides a standardized, fluent API for creating J... |
| [Event pipelines, convergence, and integration choices](./EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md) | 1.0.0 | Vocabulary and direction only — no committed schema, object kind, or scheduler hook yet. The aim is to give the idea **bones**: **declarative configurations** that act like **custom automated test cases** over process reality (objects, CAS paths, log streams, external snapshots), producing **named pass/fail or scored indicators** that **measure**, **rollup**, and operator prompts can treat as **first-class signals** alongside test-bundle health. | Architecture documentation |
| [Execution Convergence Controller: Architecture Design](./convergence/DESIGN.md) | 1.0.0 | Active | The `ConvergenceController` is an autonomous actor that ensures the ZQK syste... |
| [Expertise in architecture and code quality (the non-functional side of the house)](./EXPERTISE_ARCHITECTURE_AND_CODE_QUALITY.md) | 1.0.0 | Assessment (living document) | Architecture documentation |
| [Factory Instance Builder Refactor](./FACTORY_INSTANCE_BUILDER_REFACTOR.md) | 1.0.0 | Active | Update object creation factories and streaming elicitation logic to use insta... |
| [Fast Path and Cache Fallback Checklist](./FAST_PATH_AND_CACHE_FALLBACK_CHECKLIST.md) | 1.0.0 | Active | Architecture documentation |
| [Feature Flag Removal Plan - I/O Queue Routing](./feature-flag-removal-plan.md) | 1.0.0 | Ready for Implementation | Architecture documentation |
| [Feature Flags Documentation](./feature-flags-documentation-v1.0.md) | 1.0.0 | Active | Feature flags allow toggling experimental and advanced features on/off withou... |
| [Feature Flags System](./feature-flags-v1.0.md) | 1.0.0 | Active | Feature flags allow toggling experimental features on/off without code change... |
| [Feature Flags Usage Guide](./feature-flags-usage-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Federated Economy Governance](./federation/FEDERATED_ECONOMY_GOVERNANCE.md) | 1.0.0 | Active | This document outlines the governance and security model for the Sovereign Me... |
| [Field Discovery System v1.0](./field-discovery-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Field Versioning System](./FIELD_VERSIONING_SYSTEM.md) | 1.0.0 | Active | The field versioning system tracks the lifecycle and changes of fields in obj... |
| [File Lock Metrics and Observability](./file-lock-metrics-and-observability.md) | 1.0.0 | Active | The `FileLock` implementation includes comprehensive metrics collection to un... |
| [File Lock Strategy Integration Summary](./FILE_LOCK_STRATEGY_INTEGRATION.md) | 1.0.0 | Active | Architecture documentation |
| [File Lock Strategy and Transaction Coordinator Architecture](./FILE_LOCK_STRATEGY_AND_TRANSACTION_COORDINATOR.md) | 1.0.0 | Active | This architecture provides: |
| [Filesystem data layout (human-scannable “data cupboards”)](./FILESYSTEM_DATA_LAYOUT.md) | 1.0.0 | Active | Architecture documentation |
| [Filter Operators Specification](./FILTER_OPERATORS_SPEC.md) | 1.0.0 | Active | This document defines the filter operators available for querying objects in ... |
| [Full-Text Search v1.0](./full-text-search-v1.0.md) | 1.0.0 | Active | This document defines the full-text search implementation for the zqk object ... |
| [Gantt SVG interaction contract (minimal)](./GANTT_SVG_INTERACTION_CONTRACT.md) | 1.0.0 | Contract and tests are in-tree; full renderer integration remains aligned with `docs/marketing/strategic-pivot/SVG_GANTT_WORK_PRESERVATION.md` (graph-backend pivot). | Architecture documentation |
| [Gantt export (PNG, PDF)](./GANTT_EXPORT_DEFERRED.md) | 1.0.0 | Active | Architecture documentation |
| [Git Workflow Enforcement via Pre-commit Hook](./git-workflow-enforcement-v1.0.md) | 1.0.0 | Active | This document describes the pre-commit hook implementation that enforces the ... |
| [Git and GitHub Osmosis Interceptor Architecture](./GIT_GH_OSMOSIS_INTERCEPTOR.md) | 1.0.0 | Active | Architecture documentation |
| [Git-native drift search patterns](./GIT_DRIFT_SEARCH_PATTERNS.md) | 1.0.0 | Active | Architecture documentation |
| [Go-To-Market (GTM): Pricing & Revenue Engine](./GTM_PRICING_STRATEGY.md) | 1.0.0 | Active | Architecture documentation |
| [Goroutine Architecture Policy](./GOROUTINE_ARCHITECTURE_POLICY.md) | 1.0.0 | Active | This policy defines the acceptable patterns for goroutine usage across the co... |
| [Goroutine Entry Points Analysis](./goroutine-entry-points-analysis.md) | 1.0 | Active | Architecture documentation |
| [Goroutine Leak Fixes Summary](./concurrency/GOROUTINE_FIXES_SUMMARY.md) | 1.0.0 | ✅ High and Medium Priority Items Completed | Architecture documentation |
| [Goroutine Leak Prevention Checklist](./mcp-goroutine-leak-prevention-checklist.md) | 1.0.0 | Active | Architecture documentation |
| [Goroutine Manager Artifact Registration](./GOROUTINE_MANAGER_ARTIFACT_REGISTRATION.md) | 1.0.0 | accepted | Architecture documentation |
| [Goroutine Manager Artifact Registration Summary](./goroutine-manager-artifact-registration-summary.md) | 1.0.0 | Pending creation (YAML syntax needs adjustment) | Architecture documentation |
| [Goroutine Manager Artifacts - Registration Complete](./GOROUTINE_MANAGER_ARTIFACTS_REGISTERED.md) | 1.0.0 | exploring | Architecture documentation |
| [Goroutine Manager Capabilities](./GOROUTINE_MANAGER_CAPABILITIES.md) | 1.0 | Active | Architecture documentation |
| [Goroutine Manager Implementation Summary](./GOROUTINE_MANAGER_IMPLEMENTATION_SUMMARY.md) | 1.0.0 | ✅ Complete | Architecture documentation |
| [Goroutine Manager Integration Artifacts](./goroutine-manager-integration-artifacts.md) | 1.0 | ✅ PASS | Architecture documentation |
| [Goroutine Manager Registration - Complete](./GOROUTINE_MANAGER_REGISTRATION_COMPLETE.md) | 1.0.0 | Ready for registration via `docman-sync` | Architecture documentation |
| [Goroutine Manager Registration Status](./GOROUTINE_MANAGER_REGISTRATION_STATUS.md) | 1.0.0 | Tracked via Git | Architecture documentation |
| [Goroutine Manager System Registration Guide](./GOROUTINE_MANAGER_SYSTEM_REGISTRATION.md) | 1.0.0 | Active | All artifacts have been created and are ready for system registration. This d... |
| [Goroutine Manager Verification Checklist](./goroutine-manager-verification-checklist.md) | 1.0.0 | Active | Architecture documentation |
| [Goroutine Manager Verification Report](./GOROUTINE_MANAGER_VERIFICATION_REPORT.md) | 1.0 | ✅ **VERIFIED AND READY FOR INTEGRATION | Architecture documentation |
| [Goroutine Naming Implementation](./concurrency/GOROUTINE_NAMING_IMPLEMENTATION.md) | 1.0.0 | Active | Architecture documentation |
| [Goroutines Not Using Builder Pattern](./concurrency/GOROUTINE_VIOLATIONS.md) | 1.0.0 | Active | Architecture documentation |
| [Greenfield team operating plan — seed for a zqk-class system](./GREENFIELD_TEAM_OPERATING_PLAN.md) | 1.0.0 | Strategy / seed document | Architecture documentation |
| [Guided Spec Management v1.0](./guided-spec-management-v1.0.md) | 1.0.0 | Active | Spec management should be **guided and interactive**, leveraging semantic lin... |
| [Hardcoded literal repetition — drift phases](./HARDCODED_LITERAL_REPETITION_PHASES.md) | 1.0.0 | Active | Architecture documentation |
| [Help Builder Documentation](./HELP_BUILDER.md) | 1.0.0 | Active | Architecture documentation |
| [High-Speed Caching Architecture V2](./high_speed_caching.md) | 1.0.0 | Active | The previous implementation of Priority Plan PRI-17807959 (High-Speed Object ... |
| [High-Volume Event Cache Concurrency Pattern](./high-volume-event-cache-concurrency.md) | 1.0.0 | Active | The `HighVolumeEventCache` follows established concurrency patterns from `Obj... |
| [High-Volume Event Cache Proposal](./high-volume-event-cache-proposal.md) | 1.0.0 | Active | Architecture documentation |
| [High-Volume Storage: Deprecation of Legacy Format and Move to Streaming](./HIGH_VOLUME_STORAGE_DEPRECATION.md) | 1.0.0 | Policy / direction | Architecture documentation |
| [Hive Mind Memory Architecture](./hive-mind-memory/DESIGN.md) | 1.0.0 | Active | The 'Hive Mind Memory' component provides a unified storage layer for ZQK, me... |
| [I/O Queue Architecture](./io-queue-architecture.md) | 1.0 | Design | Architecture documentation |
| [IA Audit Report](./ia_audit_report.md) | 1.0.0 | Active | Architecture documentation |
| [Import Tracking Architecture](./types/import_tracking.md) | 1.0.0 | Active | Architecture documentation |
| [Infrastructure Demarcation Strategy v1.0](./infrastructure-demarcation-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Init Command Scenarios](./INIT_SCENARIOS.md) | 1.0.0 | Active | The `zqk system init` command must support three distinct initialization scen... |
| [Initialization Seed Questions v1.0](./initialization-seed-questions-v1.0.md) | 1.0.0 | Active | This document defines a comprehensive seed question system that must be answe... |
| [Instance Builder Architecture](./INSTANCE_BUILDER_ARCHITECTURE.md) | 1.0.0 | Active | Instance builders extend the spec builder pattern to programmatically create ... |
| [Instance Validation System Architecture](./instance-validation-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Integrated Lifecycle and Policy Enforcement Architecture](./integrated-lifecycle-policy-enforcement-v1.0.md) | 1.0.0 | Active | This document describes the architecture for integrated lifecycle and policy ... |
| [Interactive Creation Architecture Analysis](./INTERACTIVE_CREATION_ARCHITECTURE_ANALYSIS.md) | 1.0.0 | Active | Architecture documentation |
| [Interactive Object Creation Architecture](./INTERACTIVE_CREATION_ARCHITECTURE.md) | 1.0.0 | Active | Architecture documentation |
| [Interactive Object Creation Tool Proposal](./INTERACTIVE_OBJECT_CREATION_TOOL.md) | 1.0.0 | Active | Architecture documentation |
| [Internal Objects as Dedicated WALs](./INTERNAL_OBJECTS_AS_DEDICATED_WALS.md) | 1.0.0 | Design proposal | Architecture documentation |
| [Lessons Learned: Avoiding Recursive Degradation](./LESSONS_LEARNED.md) | 1.0.0 | Active | Architecture documentation |
| [Lifecycle Definitions as Internal Objects: Design](./LIFECYCLE_AS_OBJECTS_DESIGN.md) | 1.0.0 | Preferred Implementation | Architecture documentation |
| [Lifecycle Definitions: Architecture and Management](./LIFECYCLE_DEFINITIONS_EXPLAINED.md) | 1.0.0 | Documentation | Architecture documentation |
| [Lifecycle Event Listener and Transition Criteria](./LIFECYCLE_EVENT_LISTENER_AND_CRITERIA.md) | 1.0.0 | Design + implementation | Architecture documentation |
| [Lifecycle Rollback: Status Snapshot and Audit-Chain Reconstruction](./LIFECYCLE_ROLLBACK_SNAPSHOT_AND_AUDIT_CHAIN.md) | 1.0.0 | Implemented | Architecture documentation |
| [Lint Error Prevention Strategy](./LINT_ERROR_PREVENTION.md) | 1.0.0 | Active | Architecture documentation |
| [Lint and code-quality rule inventory](./LINT_RULE_INVENTORY.md) | 1.0.0 | Active | Architecture documentation |
| [List Operations v1.0 - Common Query Patterns](./list-operations-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Lock Ordering](./LOCK_ORDERING.md) | 1.0.0 | Active | Architecture documentation |
| [Lock Timeout Implementation Plan](./LOCK_TIMEOUT_IMPLEMENTATION_PLAN.md) | 1.0.0 | Active | Architecture documentation |
| [Log Naming Conventions and Patterns](./LOG_NAMING_CONVENTIONS.md) | 1.0.0 | Active | Architecture documentation |
| [Logging Framework Enhancements for Complex Output Patterns](./LOGGING_FRAMEWORK_ENHANCEMENTS.md) | 1.0.0 | Active | Architecture documentation |
| [Logging System Architecture](./logging-system-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Logging System Audit Report](./logging-audit-report.md) | 1.0.0 | Active | Architecture documentation |
| [MCP Abstraction Layer Architecture](./MCP_ABSTRACTION_LAYER.md) | 1.0.0 | Active | The MCP (Model Context Protocol) abstraction layer provides a clean separatio... |
| [MCP Account Identification and Role Enforcement](./MCP_ACCOUNT_IDENTIFICATION.md) | 1.0.0 | Active | The MCP server now proactively prompts agents to provide their `account_id` w... |
| [MCP Binary Stability Decision Framework](./mcp-binary-stability-decision-framework.md) | 1.0.0 | Active | This document provides a decision framework for determining when the stable M... |
| [MCP Binary Stability and Compatibility Architecture v1.0](./mcp-binary-stability-v1.0.md) | 1.0.0 | Active | This document defines the architecture for managing MCP server binary stabili... |
| [MCP Built-In Tools](./MCP_BUILT_IN_TOOLS.md) | 1.0.0 | Active | Built-in tools are MCP tools that are **hardcoded directly in the MCP server*... |
| [MCP Built-In Tools Architecture](./MCP_BUILT_IN_TOOLS_ARCHITECTURE.md) | 1.0.0 | Active | Architecture documentation |
| [MCP Built-In Tools Validation](./MCP_BUILT_IN_TOOLS_VALIDATION.md) | 1.0.0 | Active | Architecture documentation |
| [MCP Built-In Tools Validation Results](./MCP_BUILT_IN_TOOLS_VALIDATION_RESULTS.md) | 1.0.0 | ✅ **VALIDATION PASSED | Architecture documentation |
| [MCP Dynamic Lookup Implementation v1.0](./mcp/mcp-dynamic-lookup-implementation-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [MCP Error Code System](./mcp/ERROR_CODES.md) | 1.0.0 | Active | The MCP server uses a comprehensive, HTTP-like error code system that categor... |
| [MCP Event Format as Output Format](./MCP_EVENT_FORMAT.md) | 1.0.0 | Active | The MCP event subscription system can be conceptualized as another output for... |
| [MCP Event Subscription Quick Reference](./MCP_EVENT_SUBSCRIPTION_QUICK_REF.md) | 1.0.0 | Active | Architecture documentation |
| [MCP Event Subscription System](./MCP_EVENT_SUBSCRIPTION.md) | 1.0.0 | Active | The MCP Event Subscription System provides a pub/sub mechanism for MCP client... |
| [MCP Format Restrictions](./MCP_FORMAT_RESTRICTIONS.md) | 1.0.0 | Active | The MCP server supports restricting clients to specific output formats. This ... |
| [MCP Goroutine Lifecycle Management](./mcp-goroutine-lifecycle-v1.0.md) | 1.0 | Active | This document defines best practices for managing goroutine lifecycles in the... |
| [MCP Integration Testing Guide](./MCP_INTEGRATION_TESTING.md) | 1.0.0 | Active | This guide provides instructions for testing the MCP event subscription syste... |
| [MCP Metrics Instrumentation](./mcp/METRICS_INSTRUMENTATION.md) | 1.0.0 | Active | Comprehensive metrics instrumentation has been added to all MCP protocol oper... |
| [MCP Multi-Agent Orchestration Foundation](./MCP_MULTI_AGENT_ORCHESTRATION.md) | 1.0.0 | Active | The MCP server architecture provides a foundational infrastructure for multi-... |
| [MCP Object-Level Access Control via Semantic Tags](./MCP_OBJECT_ACCESS_CONTROL.md) | 1.0.0 | Active | The MCP server now enforces object-level access restrictions based on semanti... |
| [MCP Output Routing Requirements](./MCP_OUTPUT_ROUTING.md) | 1.0.0 | Active | Architecture documentation |
| [MCP Package Utilities Reference](./mcp/UTILS_REFERENCE.md) | 1.0.0 | Active | Parser and extractor functions have been consolidated into standardized utili... |
| [MCP Privilege Validation Tests v1.0](./mcp-privilege-tests-v1.0.md) | 1.0.0 | Active | This document describes the comprehensive test suite that validates privilege... |
| [MCP Protocol Interface](./mcp/MCP_INTERFACE.md) | 1.0.0 | Active | This package provides explicit, type-safe interfaces for the MCP (Model Conte... |
| [MCP Protocol Metrics](./mcp/MCP_METRICS.md) | 1.0.0 | Active | Comprehensive metrics instrumentation for all MCP protocol operations, provid... |
| [MCP Role Elicitation](./MCP_ROLE_ELICITATION.md) | 1.0.0 | Active | When an agent connects to the MCP server without specifying roles or permissi... |
| [MCP Role Elicitation Test Results](./MCP_ROLE_ELICITATION_TEST_RESULTS.md) | 1.0.0 | ✅ **ALL TESTS PASSING | Architecture documentation |
| [MCP Role Elicitation Validation](./MCP_ROLE_ELICITATION_VALIDATION.md) | 1.0.0 | ✅ **IMPLEMENTATION COMPLETE | Architecture documentation |
| [MCP Role Enforcement](./MCP_ROLE_ENFORCEMENT.md) | 1.0.0 | Active | Role enforcement ensures that AI agents can only use roles that are allowed o... |
| [MCP Roles and Permissions](./MCP_ROLES_AND_PERMISSIONS.md) | 1.0.0 | Active | Architecture documentation |
| [MCP Security Testing](./MCP_SECURITY_TESTING.md) | 1.0.0 | Active | Comprehensive security testing suite for the MCP permission system, including... |
| [MCP Server Async Handling for Variable-Length Operations](./MCP_ASYNC_HANDLING.md) | 1.0.0 | Active | The MCP server now supports **async processing with synchronous responses** t... |
| [MCP Server Functionality Overview](./MCP_SERVER_FUNCTIONALITY.md) | 1.0.0 | Active | The MCP (Model Context Protocol) server provides comprehensive access to the ... |
| [MCP Server Refactoring - Fully Capable Implementation](./MCP_SERVER_REFACTOR.md) | 1.0.0 | Active | The MCP server has been fully refactored to use a clean, layered architecture... |
| [MCP Server Scalability Considerations](./MCP_SCALABILITY.md) | 1.0.0 | Active | Architecture documentation |
| [MCP Server Security Enforcement](./MCP_SECURITY_ENFORCEMENT.md) | 1.0.0 | Active | The MCP server enforces security at multiple levels: |
| [MCP Server Troubleshooting Analysis v1.0](./mcp/mcp-troubleshooting-analysis-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [MCP Spec Builder Pattern](./mcp/MCP_SPEC_BUILDER.md) | 1.0.0 | Active | The MCP Spec Builder Pattern externalizes MCP server configuration (prompts, ... |
| [MCP Spec-Based Access Control](./MCP_SPEC_ACCESS_CONTROL.md) | 1.0.0 | Active | The MCP server now uses **spec-based access control** instead of object-level... |
| [MCP Trace Analysis Report v1.0](./mcp/mcp-trace-analysis-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [MCP Workflow Specialization](./MCP_WORKFLOW_SPECIALIZATION.md) | 1.0.0 | Active | MCP workflows are a specialization of the general `workflow` object that appl... |
| [MCP Workflow Tools Validation](./MCP_WORKFLOW_TOOLS_VALIDATION.md) | 1.0.0 | ✅ **VALIDATION PASSED | Architecture documentation |
| [Maintenance WAL and Runner](./MAINTENANCE_WAL_AND_RUNNER.md) | 1.0.0 | Implemented | Architecture documentation |
| [Measurement outcome taxonomy (normative)](./MEASUREMENT_OUTCOME_TAXONOMY.md) | 1.0.0 | Adopted; **`primary_measurement_outcome`** / **`primary_measurement_outcome_detail`** / **`measurement_outcome_schema_version`** are emitted on **`rollup_v1`** and **`rollup_status_core`** (see § below). Tracking: **`BLI-EXAMPLE`** (complete). | Architecture documentation |
| [Memory Fixes Compliance Review](./memory-fixes-compliance-review.md) | 1.0.0 | Active | Architecture documentation |
| [Memory and Goroutine Explosion Analysis](./memory-explosion-analysis.md) | 1.0.0 | Active | Architecture documentation |
| [Message Queue Implementation for Backpressure Relief](./mcp/MESSAGE_QUEUE_IMPLEMENTATION.md) | 1.0.0 | Active | A message queue system has been implemented between the MCP server and client... |
| [Metric Tier-1 Auto-Fix Design](./METRIC_TIER1_AUTOFIX_DESIGN.md) | 1.0.0 | Active | Architecture documentation |
| [Metrics Pipeline System](./METRICS_PIPELINE_SYSTEM.md) | 1.0.0 | Active | The metrics pipeline system provides a trait-based, extensible framework for ... |
| [Metrics Sampler System](./METRICS_SAMPLER_SYSTEM.md) | 1.0.0 | Active | The metrics sampler system provides in-memory batching for high-frequency, lo... |
| [Metrics dashboard views, performance boundaries, and historical snapshot lake](./METRICS_DASHBOARD_AND_HISTORICAL_LAKE.md) | 1.0.0 | Design (implementation follows CLI and storage work) | Architecture documentation |
| [Metrics treasure map, timeline, and alpha launch signals](./METRICS_TREASURE_MAP_AND_ALPHA_TIMELINE.md) | 1.0.0 | Observability reference + planning aid | Architecture documentation |
| [Model Context Protocol (MCP): Control Plane vs Data Plane](./mcp_control_plane.md) | 1.0.0 | Active | Architecture documentation |
| [Modular and Scalable Architecture v1.0](./modular-scalable-architecture-v1.0.md) | 1.0.0 | Active | This document defines a modular, scalable architecture for zqk that supports ... |
| [Multi-Agent Orchestration Architecture](./multi_agent_orchestration.md) | 1.0.0 | Active | This document defines the architecture and schemas for multi-agent pipeline r... |
| [Multi-Agent Permission Issue Analysis v1.0](./mcp/multi-agent-permission-issue-analysis-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Multi-Binary CLI: Shared Runtime, Data Contract, and Integration Without Duplication](./MULTI_BINARY_SHARED_RUNTIME_AND_DATA.md) | 1.0.0 | Design / reference. Not all patterns are implemented today; this document defines the target. | Architecture documentation |
| [Multi-Instance Policy Reconciliation and Techscape Alignment v1.0](./multi-instance-policy-reconciliation-v1.0.md) | 1.0.0 | Active | This document defines a comprehensive strategy for reconciling policies, goal... |
| [Multi-Layered Ontology and Domain Integration v1.0](./multi-layered-ontology-and-domain-integration-v1.0.md) | 1.0.0 | Active | This document defines a multi-layered ontology system that distinguishes betw... |
| [Multi-agent Orchestration Pipeline Architecture](./multi_agent_pipeline.md) | 1.0.0 | Grooming / Design Phase | Architecture documentation |
| [Multi-binary ecosystem](./MULTI_BINARY_ECOSYSTEM.md) | 1.0.0 | Active | Architecture documentation |
| [Namespace System Specification v1.0](./namespace-system-specification-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Namespace-Based Modular Architecture](./NAMESPACE_MODULAR_ARCHITECTURE.md) | 1.0.0 | Active | Organize by **namespace** where each namespace (`cli`, `metrics`, `storage`, ... |
| [Non-Deterministic Goroutine Patterns](./concurrency/GOROUTINE_NON_DETERMINISTIC_PATTERNS.md) | 1.0.0 | Active | Architecture documentation |
| [Non-Functional Requirements: External Data & MCP Security Boundaries](./EXTERNAL_DATA_AND_MCP_SECURITY.md) | 1.0.0 | Active | Architecture documentation |
| [Object Count: Self-Maintaining Outcome (Mandatory Context)](./OBJECT_COUNT_SELF_MAINTENANCE.md) | 1.0.0 | Active | Architecture documentation |
| [Object Creation Requirements](./object-creation-requirements-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Object Directory Location Validation v1.0](./object-directory-location-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Object ID Cache Invalidation Strategy](./object-id-cache-invalidation.md) | 1.0.0 | ✅ Implemented | The `ObjectIDCache` is used by the `check` command to quickly validate refere... |
| [Object ID Cache: Test Coverage Evaluation](./object-id-cache-test-coverage.md) | 1.0.0 | Active | Architecture documentation |
| [Object Maintenance: Why It Underperforms and How to Redesign It](./OBJECT_MAINTENANCE_REDESIGN.md) | 1.0.0 | Active | Architecture documentation |
| [Object Management API Gap Analysis](./object-management-api-gap-analysis.md) | 1.0.0 | Active | Architecture documentation |
| [Object Parsing Optimization v1.0 - "Parse Once" Strategy](./object-parsing-optimization-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Object Storage Bucketing Strategy v1.0](./object-storage-bucketing-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Object Storage Provider Interface v1.0](./object-storage-provider-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Object Template System](./OBJECT_TEMPLATE_SYSTEM.md) | 1.0.0 | Active | The object template system supports creation of objects from spec-driven YAML... |
| [Object Type Generation v1.0 - "Croptop" Approach](./object-type-generation-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Object WAL, file descriptors, and related anti-patterns](./OBJECT_WAL_FD_LIFECYCLE.md) | 1.0.0 | Active | Architecture documentation |
| [Object operations performance (create / update / delete)](./OBJECT_OPERATIONS_PERFORMANCE.md) | 1.0.0 | Active | Architecture documentation |
| [Observability receptacles and coordination](./OBSERVABILITY_RECEPTACLES_AND_COORDINATION.md) | 1.0.0 | Active | Architecture documentation |
| [Observer Agent Architecture v1.0](./observer-agent-architecture-v1.0.md) | 1.0.0 | Active | This document defines the comprehensive architecture for the Observer Agent, ... |
| [Observer Agent Onboarding Guide (BLI-812)](./observer-agent-onboarding.md) | 1.0.0 | Active | Architecture documentation |
| [Observer Agent Privileges Update v1.0](./mcp/observer-agent-privileges-update-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Observer Agent Workflow Clarification v1.0](./mcp/observer-agent-workflow-clarification-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [On-Demand Pattern - Consistency Analysis](./on-demand-pattern-consistency-analysis.md) | 1.0.0 | Analysis | Architecture documentation |
| [On-Demand Pattern - Remaining Opportunities](./on-demand-pattern-remaining-opportunities.md) | 1.0.0 | Analysis | Architecture documentation |
| [On-Demand Pattern Implementation - Completion Summary](./on-demand-pattern-completion-summary.md) | 1.0.0 | ✅ **Implementation Complete | Architecture documentation |
| [On-Demand Worker Pattern](./on-demand-worker-pattern.md) | 1.0 | ✅ **Implemented On-Demand Pattern | Architecture documentation |
| [Onboarding Roadmap and Certification](./ONBOARDING_ROADMAP_AND_CERTIFICATION.md) | 1.0.0 | Active | - **Onboarding as objects**: The onboarding curriculum is a first-class works... |
| [Ontology Versioning Layer: Architecture Design](./ontology/VERSIONING_DESIGN.md) | 1.0.0 | Active | The Ontology Versioning Layer introduces context-awareness to the ZQK Knowled... |
| [Ontology Versioning Manager](./ontology_versioning_manager.md) | 1.0.0 | Active | Architecture documentation |
| [Osmosis Interceptor Architecture](./osmosis_interceptor.md) | 1.0.0 | Active | Architecture documentation |
| [Output Queue Integration Guide](./OUTPUT_QUEUE_INTEGRATION.md) | 1.0.0 | Active | The new output queue system replaces the current mutex-heavy progress trackin... |
| [PRI-211 Work Verification](./PRI-211-WORK-VERIFICATION.md) | 1.0.0 | Active | Architecture documentation |
| [Path alias resolution (any path, prefix scheme)](./PATH_ALIAS_RESOLUTION.md) | 1.0.0 | Implemented | Architecture documentation |
| [Per-kind stream stewardship](./STREAM_KIND_STEWARDSHIP.md) | 1.0.0 | Implemented (enqueue + execution contract) | Architecture documentation |
| [Performance Bottleneck Audit: Uncached / Per-Operation Heavy Resources](./PERFORMANCE_BOTTLENECK_AUDIT.md) | 1.0.0 | List/count use a bounded semaphore (`listCountMaxConcurrent` increased to 16); list workers use a fixed worker pool; context cancellation and closer patterns are in place. No uncached “per call” heavy resource; contention is bounded. | Architecture documentation |
| [Performance Investigation and Recommendations](./PERFORMANCE_INVESTIGATION_AND_RECOMMENDATIONS.md) | 1.0.0 | Active | Architecture documentation |
| [Persistency Layer Gap Analysis](./persistency-layer-gap-analysis.md) | 1.0.0 | No gaps identified | Architecture documentation |
| [Policy Lifecycle: System Awareness and Recurring Considerations](./POLICY_LIFECYCLE.md) | 1.0.0 | Active | The policy lifecycle establishes appropriate levels of system awareness and r... |
| [Policy vs Context Refresh Schedule](./POLICY_AND_CONTEXT_REFRESH.md) | 1.0.0 | Active | Architecture documentation |
| [Pre-Change Checklist](./PRE_CHANGE_CHECKLIST.md) | 1.0.0 | Active | Architecture documentation |
| [Pre-Commit Background Results](./PRE_COMMIT_BACKGROUND_RESULTS.md) | 1.0.0 | Active | Architecture documentation |
| [Pre-Commit Checks: Persistent Jobs + Trigger-Only Callback](./PRE_COMMIT_INTEGRITY_TRIGGER_DESIGN.md) | 1.0.0 | Active | Architecture documentation |
| [Pre-Stage Formatting](./PRE_STAGE_FORMATTING.md) | 1.0.0 | Active | Architecture documentation |
| [Priority roadmap (current focus)](./PRIORITY_ROADMAP_CURRENT.md) | 1.0.0 | Active | Architecture documentation |
| [Proactive Memory Explosion Detection Strategy](./proactive-memory-explosion-detection.md) | 1.0.0 | Active | Architecture documentation |
| [Profile Storage Proposal](./PROFILE_STORAGE_PROPOSAL.md) | 1.0.0 | Active | Architecture documentation |
| [Profile Systems Comparison](./PROFILE_SYSTEMS_COMPARISON.md) | 1.0.0 | Active | zqk has two profile systems serving different purposes: |
| [Project Discovery and Strategic Alignment v1.0](./project-discovery-and-strategic-alignment-v1.0.md) | 1.0.0 | Active | This document defines a comprehensive strategy for discovering, configuring, ... |
| [Project Initialization Guide](./PROJECT_INITIALIZATION.md) | 1.0.0 | Active | New projects must be initialized with a core set of policies that establish s... |
| [Project Policy System](./PROJECT_POLICY_SYSTEM.md) | 1.0.0 | Active | The Project Policy System provides a generalized `policy` object that serves ... |
| [Project Root Resolution: Locations Updated to ResolveProjectRoot](./PROJECT_ROOT_RESOLUTION_LOCATIONS.md) | 1.0.0 | Active | Architecture documentation |
| [Project root orientation](./PROJECT_ROOT_ORIENTATION.md) | 1.0.0 | Active | Architecture documentation |
| [Project root: one per instance, persistent "use", and scheduler alignment](./PROJECT_ROOT_USE_AND_SCHEDULER_ALIGNMENT.md) | 1.0.0 | Active | Architecture documentation |
| [Quarantine as Safety Switch](./QUARANTINE_AS_SAFETY_SWITCH.md) | 1.0.0 | Design | Architecture documentation |
| [Queue Shutdown Management](./queue-shutdown-management.md) | 1.0 | Design | Architecture documentation |
| [Queue Shutdown Testing & Implementation Summary](./shutdown-testing-summary.md) | 1.0.0 | Unit tests complete, integration tests pending | Architecture documentation |
| [Queue Shutdown Usage Guide](./shutdown-usage-guide.md) | 1.0 | Active | The `QueueShutdownCoordinator` provides graceful shutdown management for all ... |
| [Quick Create: One-Click System Objects from Text or Files](./QUICK_CREATE_OBJECTS.md) | 1.0.0 | Active | Architecture documentation |
| [Reliability and Efficiency Improvement Opportunities](./reliability-efficiency-improvements.md) | 1.1 | ✅ **Implemented | Architecture documentation |
| [Requirements Traceability System v1.0](./requirements-traceability-system-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Resource Cache Abstraction](./storage/RESOURCE_CACHE_ABSTRACTION.md) | 1.0.0 | Implemented | Architecture documentation |
| [Resource MIME Type Adapters](./mcp/RESOURCE_MIME_ADAPTERS.md) | 1.0.0 | Active | The resource MIME type adapter system allows the MCP server to extract metada... |
| [Resource URI Scheme Configuration](./mcp/RESOURCE_URI_SCHEMES.md) | 1.0.0 | Active | Resource URI scheme rules determine which URI scheme (e.g., `file://`, `docs:... |
| [Retention max_count Performance](./RETENTION_MAX_COUNT_PERFORMANCE.md) | 1.0.0 | Active | Architecture documentation |
| [Role Discovery and Progressive Configuration v1.0](./role-discovery-and-progressive-configuration-v1.0.md) | 1.0.0 | Active | This document defines a comprehensive system for discovering roles and privil... |
| [Role Guidance System](./mcp/ROLE_GUIDANCE_SYSTEM.md) | 1.0.0 | Active | The role guidance system externalizes role-specific responsibilities, duties,... |
| [Role Prompt Templates System](./mcp/ROLE_PROMPT_TEMPLATES.md) | 1.0.0 | Active | The role prompt templates system allows all MCP prompts to be externalized an... |
| [Runtime Goroutine Manager](./runtime-goroutine-manager-v1.0.md) | 1.0 | Active | The GoroutineManager provides OS-level tracking and management of all gorouti... |
| [SCS facade metrics exchange contract](./SCS_FACADE_METRICS_EXCHANGE_CONTRACT.md) | 1.0.0 | Proposed pattern (architecture). | Architecture documentation |
| [SHACL-Based Validation System Evaluation](./shacl-validation-evaluation-v1.0.md) | 1.0.0 | Active | SHACL (Shapes Constraint Language) is a W3C standard for validating RDF data ... |
| [SHACL-Based Validation System Evaluation](./shacl-evaluation-v1.0.md) | 1.0.0 | Active | This document evaluates SHACL (Shapes Constraint Language) as a potential val... |
| [Sample Analysis: Bulk Delete (pid 40505) and Scheduler (pid 20777)](./SAMPLE_ANALYSIS_BULK_DELETE_AND_SCHEDULER.md) | 1.0.0 | Active | Architecture documentation |
| [Scheduler Architecture](./SCHEDULER_ARCHITECTURE.md) | 1.0.0 | Active Documentation | The scheduler system is a robust, event-driven job execution engine that supp... |
| [Scheduler Concurrency & Access Control - Executive Summary](./scheduler-concurrency-summary.md) | 1.0.0 | Active | Architecture documentation |
| [Scheduler Concurrency and Access Control Analysis](./scheduler-concurrency-analysis.md) | 1.0.0 | Active | Architecture documentation |
| [Scheduler Degraded-Mode Guardrails](./SCHEDULER_DEGRADED_MODE_GUARDRAILS.md) | 1.0.0 | Active | Architecture documentation |
| [Scheduler Execution Boundaries & Robustness](./SCHEDULER_EXECUTION_BOUNDARIES.md) | 1.0.0 | Architecture Documentation | The scheduler already provides comprehensive execution boundaries and retry m... |
| [Scheduler Helpers Thread-Safety Analysis](./scheduler-helpers-thread-safety.md) | 1.0.0 | Active | The scheduler helper functions (`scheduler_helpers.go`) provide a unified int... |
| [Scheduler Integration Summary](./scheduler-integration-summary-v1.0.md) | 1.0.0 | Active | Integrated the concurrent operations system with `scheduler_job` objects to m... |
| [Scheduler Integration for Background Operations](./scheduler-integration-v1.0.md) | 1.0.0 | Active | Instead of spawning goroutines directly, background operations are managed th... |
| [Scheduler Maintenance: Semantic Type and Policy Alignment](./SCHEDULER_MAINTENANCE_SEMANTIC_TYPE_AND_POLICY.md) | 1.0.0 | Design | Architecture documentation |
| [Scheduler Notification System v1.0](./scheduler-notifications-v1.0.md) | 1.0.0 | Active | The scheduler notification system provides attention-grabbing, user-facing no... |
| [Scheduler Shutdown Orphan Findings (2026-02-25)](./SCHEDULER_SHUTDOWN_ORPHAN_FINDINGS.md) | 1.0.0 | Active | Architecture documentation |
| [Scheduler Startup Sequence and Job Execution Flow](./SCHEDULER_STARTUP_AND_EXECUTION_FLOW.md) | 1.0.0 | Active Documentation | This document describes the **critical startup sequence** and **job execution... |
| [Scheduler Transceiver Architecture](./SCHEDULER_TRANSCEIVER_ARCHITECTURE.md) | 1.0.0 | Proposed Architecture | A protocol-agnostic transceiver/router that handles all scheduler job communi... |
| [Scheduler and Storage Concurrency Audit](./SCHEDULER_CONCURRENCY_AUDIT.md) | 1.0.0 | Active | Architecture documentation |
| [Scheduler and Storage: Root Cause Analysis and Performance Baselines](./SCHEDULER_STORAGE_RCA.md) | 1.0.0 | Active | Architecture documentation |
| [Scheduler daemon: memory bloat and hang analysis](./SCHEDULER_MEMORY_AND_HANG_ANALYSIS.md) | 1.0.0 | Active | Architecture documentation |
| [Scheduler diagnostics findings and fixes](./SCHEDULER_DIAGNOSTICS_FINDINGS.md) | 1.0.0 | Active | Architecture documentation |
| [Scheduler job long ID and lock file](./SCHEDULER_JOB_LONG_ID_AND_LOCK.md) | 1.0.0 | Resolved | Architecture documentation |
| [Scheduler overload and timeout](./SCHEDULER_OVERLOAD_AND_TIMEOUT.md) | 1.0.0 | Implemented and DRY. **All tiers** use a single pattern in `handlers_cache_prewarm.go`: `tierTimeoutFromJob(ctx, defaultTimeout, buffer)` for job-derived timeouts; `runSequentialTier` for Tier 1 (single task); `runParallelTier` for Tier 2 and Tier 3 (and any future Tier 4+). Each parallel tier creates its context *before* starting tasks and passes it to every task so the wait goroutine never blocks forever. Adding Tier 4 is a single `runParallelTier(ctx, job.ID, 4, "Tier 4", defaultTimeout, buffer, []tierTask{...})` call. | Architecture documentation |
| [Scheduler status and health-check alignment](./SCHEDULER_STATUS_AND_HEALTH_CHECK_ALIGNMENT.md) | 1.0.0 | Follow-up to investigate | Architecture documentation |
| [Semantic Bridge System Architecture](./semantic-bridge/DESIGN.md) | 1.0.0 | Active | Architecture documentation |
| [Semantic Translation Engine](./semantic_translation_engine.md) | 1.0.0 | Active | Architecture documentation |
| [Semantic Types and Formal Ontology Integration](./semantic-types-ontology-v1.0.md) | 1.0.0 | Active | This document describes the formal ontology integration for semantic type val... |
| [Server Resilience Tests](./mcp/SERVER_RESILIENCE_TESTS.md) | 1.0.0 | Active | Comprehensive unit tests that stress-test the MCP server to ensure it's "inde... |
| [Service Management](./SERVICE_MANAGEMENT.md) | 1.0.0 | Active | The zqk CLI includes service management capabilities to start, stop, and moni... |
| [Shutdown Hang Analysis (Process Sample)](./SHUTDOWN_HANG_ANALYSIS.md) | 1.0.0 | Active | Architecture documentation |
| [Signal and Context Pattern](./SIGNAL_AND_CONTEXT.md) | 1.0.0 | Active | Architecture documentation |
| [Simplified Output Architecture](./SIMPLIFIED_OUTPUT_ARCHITECTURE.md) | 1.0.0 | Active | Architecture documentation |
| [Snapshot References and Template-Driven Growth](./SNAPSHOT_REFERENCE_AND_TEMPLATE_GROWTH.md) | 1.0.0 | Design | Architecture documentation |
| [Snapshot Sequence Format Enhancement](./SNAPSHOT_SEQUENCE_FORMAT_ENHANCEMENT.md) | 1.0.0 | Active | The compressed snapshot format currently uses dictionary compression with fie... |
| [Snapshot to Auto-Fixer Workflow Design](./SNAPSHOT_TO_AUTOFIXER_WORKFLOW.md) | 1.0.0 | Active | The end-goal is to validate snapshot data transfer and restoration of check v... |
| [Source Control Steward Spec](./SOURCE_CONTROL_STEWARD_SPEC.md) | 1.0.0 | Active | Architecture documentation |
| [Spec Loading Order and Trait Validation](./spec-loading-order-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Spec Persistence Gap Analysis](./spec-persistence-gap-analysis.md) | 1.0.0 | Active | Architecture documentation |
| [Spec origin plane: canonical model, derived indexes, and traceability](./SPEC_ORIGIN_PLANE.md) | 1.0.0 | Architecture (normative direction + inventory of what exists today) | Architecture documentation |
| [Spec plane, streams, and data cells — shared mental model](./SPEC_STREAM_CELL_PEDAGOGY.md) | 1.0.0 | Architecture (pedagogy + alignment with implementation) | Architecture documentation |
| [Spec-Based Auto-Fixer Plan](./SPEC_BASED_AUTO_FIXER_PLAN.md) | 1.0.0 | Active | Extend the existing auto-fix functionality (currently only handles hash misma... |
| [Spec-Driven Builder Pattern](./SPEC_DRIVEN_BUILDER_PATTERN.md) | 1.0.0 | Active | The **Spec-Driven Builder Pattern** combines declarative YAML specifications ... |
| [SpecBuilder Architecture](./SPECBUILDER_ARCHITECTURE.md) | 1.0.0 | Active | The SpecBuilder package (`pkg/specbuilder`) provides a reusable foundation fo... |
| [SpecBuilder Safety Plan](./SPECBUILDER_SAFETY_PLAN.md) | 1.0.0 | Active | This document outlines safety measures and migration strategies for introduci... |
| [Standardized data pipeline lifecycle](./data-pipeline-lifecycle.md) | 0.2 | Active (canonical) | Architecture documentation |
| [Storage Backend Detection and Configuration v1.0](./storage-backend-detection-v1.0.md) | 1.0.0 | Implemented | The CLI is modularized such that only **one backend is active at a time** (fi... |
| [Storage Layer Async Architecture Improvements](./STORAGE_ASYNC_ARCHITECTURE_IMPROVEMENTS.md) | 1.0.0 | Active | Architecture documentation |
| [Storage Orchestration Integration Guide](./storage-orchestration-integration-v1.0.md) | 1.0.0 | Active | The storage orchestrator coordinates operations across multiple storage backe... |
| [Storage Orchestration for Multi-Backend Support](./storage-orchestration-v1.0.md) | 1.0.0 | Active | The storage orchestrator coordinates operations across multiple storage backe... |
| [Storage public API: neutral aliases (BLI-177484)](./STORAGE_PUBLIC_API_NEUTRAL_ALIASES.md) | 1.0.0 | Active | Architecture documentation |
| [Strategic Alignment Analysis and Semantic Bridge v1.0](./strategic-alignment-analysis-and-semantic-bridge-v1.0.md) | 1.0.0 | Active | This document analyzes how recent architectural considerations align with zqk... |
| [Strategic Options: Post-Beta Execution (Phase 14)](./STRATEGIC_OPTIONS_PHASE_14.md) | 1.0.0 | Active | Architecture documentation |
| [Stream Storage: Append-Only Segments for High-Volume Kinds](./STREAM_STORAGE.md) | 1.0.0 | Implemented | Architecture documentation |
| [Structural vs Runtime-Delta and Stream Storage: Implementation Investigation](./STRUCTURAL_VS_RUNTIME_DELTA_INVESTIGATION.md) | 1.0.0 | Active | Architecture documentation |
| [Subordinate Namespaces Extension](./SUBORDINATE_NAMESPACES.md) | 1.0.0 | Active | Extend the existing `namespace_id` system to support **subordinate namespaces... |
| [Subordinate Namespaces Implementation](./SUBORDINATE_NAMESPACES_IMPLEMENTATION.md) | 1.0.0 | Active | Extended the existing `namespace_id` system to support **subordinate namespac... |
| [System Check Monitoring and Awareness](./SYSTEM_CHECK_MONITORING.md) | 1.0.0 | Active | System check violations must be proactively monitored and project resources m... |
| [System Check: Architecture vs Implementation Alignment](./system-check-architecture-alignment.md) | 1.0.0 | Active | Architecture documentation |
| [System Health Assessment - On-Demand Pattern Implementation](./system-health-assessment.md) | 1.0.0 | Unrelated to on-demand pattern work | Architecture documentation |
| [System Object Discovery Guide v1.0](./system-object-discovery-guide-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [System Object Leverage Strategy v1.0](./system-object-leverage-strategy-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [System Sync Command Architecture](./system-sync-command-v1.0.md) | 1.0.0 | Active | The `zqk system sync` command provides a simple wrapper around Git operations... |
| [System check CPU profiling and "Warming CAS indexes" slowness](./system-check-cpu-profile-notes.md) | 1.0.0 | Active | Architecture documentation |
| [System check data flow and lock sequencing](./system-check-data-flow.md) | 1.0 | Active | Architecture documentation |
| [System check performance targets](./system-check-performance-targets.md) | 1.0.0 | Active | Architecture documentation |
| [System check pipeline architecture and specs](./system-check-pipeline.md) | 1.0.1 | Active | Architecture documentation |
| [System check: cache-first, async population, quick CLI](./system-check-cache-first-and-async.md) | 1.0 | Target architecture | Architecture documentation |
| [System-Managed Resources Container](./SYSTEM_MANAGED_RESOURCES.md) | 1.0.0 | Active | System-managed resources are files and directories that are: |
| [Template Cache Architecture](./TEMPLATE_CACHE_ARCHITECTURE.md) | 1.0.0 | Active | The template cache system uses a layered architecture that separates backend-... |
| [Template Cache Defensive Tests](./TEMPLATE_CACHE_DEFENSIVE_TESTS.md) | 1.0.0 | Active | The template cache system includes 18 comprehensive defensive tests that anti... |
| [Test Separation: Unit vs Integration](./scheduler-test-separation-v1.0.md) | 1.0.0 | Active | Architecture documentation |
| [Test Separation: Unit vs Integration](./storage/TEST_SEPARATION.md) | 1.0.0 | Active | Architecture documentation |
| [Test Service Management](./TEST_SERVICE_MANAGEMENT.md) | 1.0.0 | Active | The test service management system allows tests to automatically spin up requ... |
| [Test baseline scope (storage, cache, reporting, scheduler, spec)](./TEST_BASELINE_SCOPE.md) | 1.0.0 | Active | Architecture documentation |
| [Testing Plan for Storage Orchestration and Async Validation](./testing-plan-v1.0.md) | 1.0.0 | Active | This document outlines the testing strategy for: |
| [Testing Summary - Storage Orchestration and Async Validation](./testing-summary-v1.0.md) | 1.0.0 | Active | Comprehensive testing infrastructure has been created for: |
| [The Creative Media Division: Mesh Governance](./creative_media_division.md) | 1.0.0 | Active | Architecture documentation |
| [The External Mesh Gateway: Asynchronous Agentic Architecture](./EXTERNAL_MESH_GATEWAY.md) | 1.0.0 | Active | Architecture documentation |
| [The Zen Quantum Cortex: Swarm Strategy for Massive Semantic Graphs](./zen-quantum-cortex-swarm-strategy.md) | 1.0.0 | Active | Architecture documentation |
| [Tool Pod Mesh: Automated Retries and Sentinel Rollbacks](./sentinel_mesh.md) | 1.0.0 | Active | This architectural spec defines the automated retry and rollback mechanism wi... |
| [TraitHarness: Unified List-Manipulating Behavior for All Data](./TRAIT_HARNESS.md) | 1.0.0 | Implemented (harness in pkg/cli; healthchk list wired as first consumer) | Architecture documentation |
| [Transceiver Async Architecture](./TRANSCEIVER_ASYNC_ARCHITECTURE.md) | 1.0.0 | Implemented | The transceiver router uses async execution with worker pools to prevent bloc... |
| [Transceiver Router Implementation Plan](./TRANSCEIVER_IMPLEMENTATION_PLAN.md) | 1.0.0 | Implementation Plan | This document outlines the implementation plan for the transceiver router arc... |
| [Translation Manifest Architecture](./translation_manifest.md) | 1.0.0 | Active | Architecture documentation |
| [UPDATE Loop Design](./UPDATE_LOOP_DESIGN.md) | 1.0.0 | Active | The UPDATE loop is similar to the CREATE loop but has key differences: |
| [Unbounded Concurrency Fixes (Thread Explosion)](./unbounded-concurrency-fixes.md) | 1.0.0 | Active | Architecture documentation |
| [Unified Profile System](./UNIFIED_PROFILE_SYSTEM.md) | 1.0.0 | Active | The Unified Profile System provides a common schema core for all profile type... |
| [Validation Flow and Call Order v1.0](./validation-flow-v1.0.md) | 1.0.0 | Active | This document maps out the call order for object updates, change journal entr... |
| [Validation Run Issues Investigation](./VALIDATION_RUN_ISSUES_INVESTIGATION.md) | 1.0.0 | Active | Architecture documentation |
| [Vectorization Engine Architecture Blueprint](./VECTORIZATION_ENGINE_ARCHITECTURE.md) | 1.0.0 | Active | Architecture documentation |
| [Verification outcome authority (criteria and convergence_session)](./VERIFICATION_OUTCOME_AUTHORITY.md) | 1.0.0 | Active | Architecture documentation |
| [Vision: path-keyed cache, migration-safe FS, spec-as-objects, “classloader” loading](./SPEC_RUNTIME_AND_PATH_CACHE_VISION.md) | 1.0.0 | Direction note (not an implementation plan) | Architecture documentation |
| [Visual Plan Object Schema](./visual_plan_schema.md) | 1.0.0 | Active | Architecture documentation |
| [WAL Long Record: Causes and Safeguards](./WAL_LONG_RECORD_SAFEGUARDS.md) | 1.0.0 | Active | Architecture documentation |
| [WAL and State Files: Naming and Cleanup](./WAL_AND_STATE_FILES.md) | 1.0.0 | Active | Architecture documentation |
| [WAL-Backed Bulk Transactions Design](./WAL_BULK_TRANSACTIONS_DESIGN.md) | 1.0.0 | Active | Architecture documentation |
| [Why `git commit` Can Be Slow (Sample Analysis)](./GIT_COMMIT_HOOK_SLOW.md) | 1.0.0 | Active | Architecture documentation |
| [Workflow Constraints vs MCP Config](./WORKFLOW_CONSTRAINTS_VS_MCP_CONFIG.md) | 1.0.0 | Active | Workflow constraints provide a more flexible, granular, and business-logic-fo... |
| [Workstream Transition and Agent Onboarding Strategy v1.0](./workstream-transition-and-agent-onboarding-strategy-v1.0.md) | 1.0.0 | Active | This document defines policies and strategies for workstream transitions, mul... |
| [Wrapper inner-context guard (pattern)](./WRAPPER_INNER_CONTEXT_GUARD_PATTERN.md) | 1.0.0 | Architecture (normative for CLI wrapper types) | Architecture documentation |
| [Write Wrapper Tool Proposal](./WRITE_WRAPPER_TOOL_PROPOSAL.md) | 1.0.0 | Active | Architecture documentation |
| [ZQK Semantic Bridge Phase 3: Reconciliation & Inference](./SEMANTIC_BRIDGE_PHASE_3_DESIGN.md) | 1.0.0 | Active | This document outlines the technical design, complexity breakdown, and implem... |
| [ZQK Tool Pod Mesh Architecture](./TOOL_POD_MESH.md) | 1.0.0 | Active | To maintain the core ZQK binary as a pristine, zero-dependency, lightweight o... |
| [`zqk new` — standard object origination pipeline](./NEW_COMMAND_OBJECT_ORIGINATION.md) | 1.0.0 | Implemented (CLI). Process traceability: requirement + doc_entry + criteria in process data (via `zqk object create`). | Architecture documentation |
| [generate-builders Process Sample Analysis (PID 7937)](./GENERATE_BUILDERS_SAMPLE_ANALYSIS.md) | 1.0.0 | Active | Architecture documentation |
| [zqk_session Stream Behavior](./ZQK_SESSION_STREAM_BEHAVIOR.md) | 1.0.0 | Active | Architecture documentation |

**Graph Backend Details:**
- [Commit and Timeout Semantics](./graph-backend/COMMIT_AND_TIMEOUT_SEMANTICS.md) - Architecture documentation
- [Enabling the Graph Backend](./graph-backend/ENABLING_GRAPH_BACKEND.md) - Architecture documentation
- [Graph Backend Architecture Decisions](./graph-backend/ARCHITECTURE_DECISIONS.md) - Architecture documentation
- [Metrics Configuration Guide](./graph-backend/CONFIGURATION.md) - The observability system is fully configurable, adjustable, and non-blocking. You can:
- [Observability and Metrics](./graph-backend/OBSERVABILITY.md) - The graph backend provides comprehensive observability for self-healing and continuous improvement. All metrics are collected automatically and can be exported to various backends.
- [Shared Implementation Patterns](./graph-backend/SHARED_IMPLEMENTATIONS.md) - To keep code DRY across different graph backend providers, we've extracted common logic into shared base implementations.

## Organization Principles

1. **Versioning**: All architecture docs use semantic versioning (v1.0, v2.0, etc.)
2. **Categorization**: Docs organized by topic (Core, Graph, Storage, Migration, CLI)
3. **Subdirectories**: Related docs grouped (e.g., `graph-backend/` for implementation details)
4. **Status Tracking**: Each doc has a status (Design Complete, Active, Research Complete, etc.)
5. **Cross-References**: Related documents linked for easy navigation

## Related Documentation

- [System Ontology](../ontology/system-ontology-v1.0.md) - Object type definitions
- [Object Specs](../_internal/object_specs/README.md) - Object specifications
- [Process README](../README.md) - Process directory overview

---

*This README is auto-generated. Architecture documents are discovered dynamically from the file system.*
