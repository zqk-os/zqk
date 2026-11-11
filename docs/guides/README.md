# Developer & Agent Guides

Practical, battle-tested execution guides for human software engineers and autonomous AI agents working within the **ZQK Knowledge Kernel**.

---

## Overview

While the [Reference Manual](../manual/README.md) details comprehensive subsystem APIs and [Specifications](../specs/README.md) formalize architectural grammar, these **Guides** provide step-by-step, actionable recipes for common engineering disciplines, graph operations, and multi-agent coordination patterns.

Every guide is structured to serve both **human pair programmers** using the interactive CLI/TUI and **autonomous AI agents** operating via headless interfaces or Model Context Protocol (MCP) tools.

---

## 1.1 Object Model & Core Contracts

Core Knowledge Kernel object taxonomy, envelope anatomy, and Content Addressable Storage (CAS) lifecycle.

| Guide | Focus Area | Primary Commands |
| :--- | :--- | :--- |
| **[Knowledge Kernel Object Model & Usage Guide](./KERNEL_OBJECT_USAGE_GUIDE.md)** | Canonical taxonomy across 14 ontological domains and packs (70+ object kinds), standard envelopes, and VDS cascade. | `zqk object create`, `zqk object list`, `zqk object promote` |
| **[CLI Kind Alias Parity Guide](./CLI_KIND_ALIAS_PARITY.md)** | Specification of canonical kind shorthand aliases (tst, tc, crit, req, pri, bli, mil, goal) across CLI entrypoints. | `zqk new`, `zqk object new` |
| **[Object Scaffolding Hygiene Guide](./OBJECT_SCAFFOLDING_HYGIENE.md)** | Reference field initialization invariants across CLI templates, omission of optional references, and strict validation. | `zqk object template`, `zqk new` |
| **[Kernel Subsystem Operational Guide](./KERNEL_SUBSYSTEM_GUIDE.md)** | Cellular microkernel architecture, KnowledgeKernel lifecycle, relational graph membrane, and extension registry. | `pkg/kernel` |
| **[Object Lifecycle, State Machines & CAS Storage](./OBJECT_LIFECYCLE_AND_CAS_STORAGE_GUIDE.md)** | Schema contracts, Content-Addressed Storage membrane, two-phase draft/authoritative isolation, and DoD criteria binding. | `zqk object create`, `zqk object promote` |

---

## 1.2 Ontology, Semantic Inference & Translations

Ontological schema projection, graph translation, and semantic recall across domains.

| Guide | Focus Area | Primary Commands |
| :--- | :--- | :--- |
| **[Knowledge Management & Semantic Recall](./KNOWLEDGE_MANAGEMENT_AND_SEMANTIC_RECALL_GUIDE.md)** | Operational glossaries, lens taxonomies, composable libraries, `docman` CAS verification, and recall protocols. | `zqk docman register`, `zqk object list glossary_term` |
| **[Objects & Specs Subsystem Guide](./OBJECTS_AND_SPECS_SUBSYSTEM_GUIDE.md)** | Ontological schema contracts, spec roots, meta-schema compilation, and trait normalization. | `pkg/objects` |
| **[System Ontology & Object Classification](./ONTOLOGY_SUBSYSTEM_GUIDE.md)** | Semantic schema projection, 14 planning kinds, typed edge directional occupancy, and data plane contextualizer. | `pkg/ontology` |
| **[Semantic Graph & AST Inference Engine](./SEMANTIC_GRAPH_AND_INFERENCE_ENGINE_GUIDE.md)** | Ontological translation, semantic maturity assessment (Levels 0-4), dynamic rule inference, and drift reconciliation. | `pkg/semantic` |
| **[Schema Translation & Serialization](./SCHEMA_TRANSLATION_AND_FORMAT_SERIALIZATION_GUIDE.md)** | Multi-format deserialization and AST mapping (RDF/OWL, Turtle, JSON-LD, Cypher, JSON Schema, OpenAPI). | `pkg/translation` |

---

## 1.3 High-Performance Storage, Buffer Pools & WAL

Foundational persistence engine, append-only logs, and real-time state accumulation.

| Guide | Focus Area | Primary Commands |
| :--- | :--- | :--- |
| **[Storage Subsystem Operational Guide](./STORAGE_SUBSYSTEM_GUIDE.md)** | Foundational persistence engine, Content Addressable Storage (CAS), write-behind audit event buffering, and TSDB engine. | `pkg/storage` |
| **[Real-Time State Accumulator Guide](./ACCUMULATOR_SUBSYSTEM_GUIDE.md)** | Fast-path state accumulator (<5ms reads), dual-format deserialization, quiescent WAL detection, and recovery. | `pkg/accumulator` |
| **[Write-Ahead Log Utilities Subsystem Guide](./WAL_UTILITY_SUBSYSTEM_GUIDE.md)** | Append-only line-delimited JSON log streams, dual-coordinate ReplayCursor checkpoints, and quarantine handling. | `pkg/walutil` |
| **[High-Performance Buffer Pool Guide](./BUFFER_POOL_SUBSYSTEM_GUIDE.md)** | Byte buffer recycling, retention ceilings, zero-allocation serialization, and chunked streaming readers. | `pkg/bufferpool` |
| **[Lifecycle Subsystem Operational Guide](./LIFECYCLE_SUBSYSTEM_GUIDE.md)** | State machine transitions, event WAL persistence, reactive listener rules, and shockwave cascades. | `pkg/lifecycle` |

---

## 1.4 CLI Object Operations & Query Pipelines

Terminal commands for object manipulation, graph traversal, and transaction rollback.

| Guide | Focus Area | Primary Commands |
| :--- | :--- | :--- |
| **[Declarative Graph Queries (ZPARQL) & Mutations (ZQL)](./ZQL_ZPARQL_AGENT_GUIDE.md)** | Multi-hop graph traversals, Cypher-like pattern queries, and atomic multi-object ACID transactions. | `zqk query`, `zqk mutate` |
| **[CLI Object Subsystem Operational Guide](./CLI_OBJECT_SUBSYSTEM_GUIDE.md)** | Subsystem command taxonomy, CAS storage membrane, two-phase draft plane isolation, and relational query pipelines. | `zqk object create`, `zqk object get`, `zqk object list` |
| **[CLI Command Taxonomy & Internal Operations](./CLI_COMMAND_TAXONOMY_AND_OPERATIONS_GUIDE.md)** | Internal command taxonomy, administrative role gates, bulk mutation engine, and write-behind flushes. | `zqk internal`, `pkg/zqkcli` |
| **[Lifecycle Rollback & Snapshot Management](./ROLLBACK_TRANSACTION_AND_SNAPSHOT_MANAGEMENT_GUIDE.md)** | Transactional snapshot capture, WAL-backed rollback points, and multi-entity graph state reversion. | `zqk rollback`, `pkg/rollback` |
| **[CLI Query Subsystem Guide](./CLI_QUERY_SUBSYSTEM_GUIDE.md)** | Declarative ZPARQL pattern queries, interactive REPL console, and graph edge path visualization. | `zqk query`, `zqk query -i` |

---

## 2.1 Static AST Linting & Policy Rules

Code vetting, static analysis, and declarative invariant enforcement.

| Guide | Focus Area | Primary Commands |
| :--- | :--- | :--- |
| **[Validation Subsystem Guide](./VALIDATION_SUBSYSTEM_GUIDE.md)** | Multi-layer validation architecture (Layers 0–4), AST invariant checks, and automated remediation engine. | `zqk system check`, `zqk object inspect` |
| **[Static AST Linting & Code Vetting Engine](./STATIC_AST_LINTING_AND_CODE_VETTING_GUIDE.md)** | Declarative GatesConfig, AST check suites (command specs, CLI builders, concurrency, duplication, paths/perms). | `zqk-vet`, `pkg/vet` |
| **[Architectural Drift Hotspots & Field Key Fixer](./ARCHITECTURAL_DRIFT_HOTSPOTS_AND_FIXER_GUIDE.md)** | AST interval inspection, constant barrels, data catalog boundaries, and automated field key literal rewriter. | `pkg/drifthotspots` |
| **[System Check & Validator Registration](./SYSTEM_CHECK_AND_VALIDATION_SURFACE_GUIDE.md)** | ValidatorRegistry, bounded worker pool execution, spec-informed violation resolution, and auto-remedy strategies. | `zqk system check`, `zqk system telemetry` |
| **[Custom Validation Policy Creation & Rule DSL](./POLICY_CREATION_AND_VALIDATION_DSL_GUIDE.md)** | Declarative Validation Rule DSL expressions, live population dry-run audits, and check-valve registration. | `zqk object inspect --policy-studio` |

---

## 2.2 Test Verification Harness & Continuous Evidence

Requirements-to-test traceability, verification matrices, and delivery gates.

| Guide | Focus Area | Primary Commands |
| :--- | :--- | :--- |
| **[CLI Test Subsystem Guide](./CLI_TEST_SUBSYSTEM_GUIDE.md)** | 3-Fold Proof verification, test execution pipeline, AST contamination detector, and TUI test dashboard. | `zqk test run`, `zqk test dashboard` |
| **[Test Runner & Parallel Safety](./TEST_RUNNER_AND_PARALLEL_SAFETY_GUIDE.md)** | ParallelSafetyGovernor, RaceConditionGate, ContaminationSentinel, and process group sandboxing. | `pkg/testrunner`, `zqk test run` |
| **[Polyglot Test Discovery & Engine Registry](./POLYGLOT_TEST_DISCOVERY_AND_ENGINE_GUIDE.md)** | Static AST/source parsing across Go, Python, TypeScript, and Rust, DiscoveryEngine, and SHA-256 incremental cache. | `pkg/testdiscovery`, `zqk test discover` |
| **[Requirements Harness & Predicate Evidence](./REQUIREMENTS_HARNESS_AND_PREDICATE_EVIDENCE_GUIDE.md)** | POL-WORKFLOW-VDS evidence engine, scope:Name predicate grammar, and verification lifecycle. | `pkg/reqharness` |
| **[Verifiable Delivery Standard (VDS) & Evidence Engine](./VERIFICATION_DATA_SPEC_AND_EVIDENCE_GUIDE.md)** | Verifiable Decomposition Spine, five rigid delivery stages, chunk evaluation engine, and predicate assertions. | `zqk workflow vds`, `pkg/vds` |

---

## 2.3 Hermetic Environments & Simulation

Hermetic isolation, scenario replay, and leak detection.

| Guide | Focus Area | Primary Commands |
| :--- | :--- | :--- |
| **[Hermetic Test Environment & Mutation Guard](./HERMETIC_TEST_ENVIRONMENT_AND_MUTATION_GUARD_GUIDE.md)** | Isolated test root materialization, repository write protection, and state modtime snapshotting. | `pkg/testenvroot` |
| **[Scenario Testing & Behavioral Simulation](./SCENARIO_TESTING_AND_BEHAVIORAL_SIMULATION_GUIDE.md)** | Scenario bundle application, doc convergence, traceability matrix resolution, and draft bundle authoring. | `pkg/scenario` |
| **[Universal Testkit & Leak Detection](./UNIVERSAL_TESTKIT_AND_LEAK_DETECTION_GUIDE.md)** | Hermetic project isolation, goroutine and subprocess leak profiling, and managed subprocess groups. | `pkg/testkit` |
| **[Test Services Lifecycle & Ephemeral Fixtures](./TEST_SERVICES_LIFECYCLE_AND_FIXTURES_GUIDE.md)** | ServiceManager, HealthCheckEngine, PortPublisherSentinel, and container lifecycle governance. | `pkg/testservices` |
| **[Object Closure Gate & Verification Guide](./CLOSURE_EVIDENCE_GATE_AND_VERIFICATION_GUIDE.md)** | Machine-checkable closure evidence, bundle log disk verification, and scheduler health fingerprinting. | `pkg/closureevidence` |

---

## 2.4 Agent Rules Governance & Predicate DSL

Declarative agent instruction governance and AST predicate evaluation.

| Guide | Focus Area | Primary Commands |
| :--- | :--- | :--- |
| **[Agent Execution Rules & Manifest Governance](./AGENT_RULES_SUBSYSTEM_GUIDE.md)** | Rule definitions (*.mdc), manifest registry, candidate resolution hierarchy, and pre-commit enforcement. | `zqk system validate-agent-rules` |
| **[Predicate DSL & AST Evaluation Engine](./PREDICATE_SUBSYSTEM_GUIDE.md)** | Formal EBNF grammar, tokenization, AST parser construction, truth table evaluation, and built-in predicates. | `pkg/predicate` |
| **[Conditional Evaluation & Fluent Predicate Chains](./CONDITIONAL_EVALUATION_AND_FLUENT_CHAIN_GUIDE.md)** | Fluent conditional chains, short-circuit evaluation, and Try/Catch monadic containers. | `pkg/when` |
| **[Continuous Verification & Evidence Classification](./CONTINUOUS_VERIFICATION_AND_EVIDENCE_CLASSIFICATION_GUIDE.md)** | Content-addressed verification matrix, evidence classification, and diamond quality scoring. | `pkg/verification` |

---

## 3.1 Autonomous Loops & Workflow Orchestration

Continuous autonomy pipeline, single-command execution, and DAG workflows.

| Guide | Focus Area | Primary Commands |
| :--- | :--- | :--- |
| **[Single-Command Execution Loop (`zqk do`)](./SINGLE_COMMAND_EXECUTION_LOOP_GUIDE.md)** | End-to-end autonomous CAP loop execution, three-phase delivery, and atomic branch verification. | `zqk do <bli-id>`, `zqk do` |
| **[Workflow Subsystem & Pipeline Orchestration](./WORKFLOW_SUBSYSTEM_AND_PIPELINE_ORCHESTRATION_GUIDE.md)** | Workflow state machines, step dispatching, loop continuation hooks, and reactive wake conditions. | `zqk workflow whats-next`, `zqk workflow run` |
| **[Pipeline Subsystem Operational Guide](./PIPELINE_SUBSYSTEM_GUIDE.md)** | Composable execution stages, DAG executor, monotonic process mutation pipeline, and worktree isolation. | `pkg/pipeline` |
| **[Shockwave Propagation & Pipeline Dispatch](./SHOCKWAVE_PROPAGATION_AND_PIPELINE_DISPATCH_GUIDE.md)** | Decoupled pub/sub event pipeline, dynamic batch accumulation, and gateway dispatching. | `pkg/shockwave` |
| **[Session Paging & Virtual Context](./SESSION_PAGING_AND_VIRTUAL_CONTEXT_GUIDE.md)** | Local session paging, bounded context buffers, and zero-leak transient epoch isolation. | `pkg/sessionpaging` |

---

## 3.2 Scheduler Engine, Daemons & Lifecycle Supervision

Background scheduler daemon, cron tasks, and host service supervision.

| Guide | Focus Area | Primary Commands |
| :--- | :--- | :--- |
| **[Scheduler Subsystem Operational Guide](./SCHEDULER_SUBSYSTEM_GUIDE.md)** | Chronological event loop, recurrent task dispatch, lease expiration, and failure backoff. | `zqk scheduler status`, `zqk scheduler run` |
| **[Autonomous Daemon Overseer & Supervision](./AUTONOMOUS_DAEMON_OVERSEER_AND_SUPERVISION_GUIDE.md)** | Launchd/systemd host service units, crash recovery, process group lease management, and heartbeat sentinels. | `zqk service start`, `zqk daemon list` |
| **[Maintenance Jobs & Health Scans](./MAINTENANCE_JOBS_AND_HEALTH_SCANS_GUIDE.md)** | Background CAS compaction, orphan draft sweep, index re-indexing, and automated resource hygiene. | `zqk job list`, `zqk job trigger` |
| **[Process Supervision & Engine Registry](./PROCESS_SUPERVISION_AND_ENGINE_REGISTRY_GUIDE.md)** | Failure diagnostic extraction, contrastive retry prompt formatting, and canonical exemplar registry. | `pkg/supervision` |

---

## 4.1 Swarm Topology, Seating & Toolpods

Autonomous swarm architecture, agent personas, and capability toolpods.

| Guide | Focus Area | Primary Commands |
| :--- | :--- | :--- |
| **[Swarm Subsystem Architecture & Orchestration](./SWARM_SUBSYSTEM_ARCHITECTURE_AND_ORCHESTRATION_GUIDE.md)** | Multi-agent swarm topology, concurrent worker pooling, task claiming protocols, and AST verification gates. | `zqk swarm status`, `zqk swarm run` |
| **[Agent Subsystem & Seating Architecture](./AGENT_SUBSYSTEM_AND_SEATING_GUIDE.md)** | 8 seated personas, capability boundaries, token-budgeted prompt generation, and seating manifests. | `zqk agent status`, `zqk agent claim` |
| **[Model Federation & Inference Harvester](./MODEL_FEDERATION_AND_INFERENCE_HARVESTER_GUIDE.md)** | Zero-egress local inference discovery (Ollama/vLLM), model federation, and provider fallback gradients. | `zqk provider list`, `pkg/llm` |
| **[Agent Rules & Persona Seating Architecture](./AGENT_RULES_AND_PERSONA_SEATING_GUIDE.md)** | Declarative seating manifests, role token budgets, capability access masks, and runtime invariants. | `pkg/agentrules` |

---

## 4.2 Multi-Agent Context Binding & P2P Mesh

Inter-agent communication, mesh networking, and context handoffs.

| Guide | Focus Area | Primary Commands |
| :--- | :--- | :--- |
| **[Mesh Networking, State Sync & Toolpods](./MESH_NETWORKING_STATE_SYNC_AND_TOOLPOD_GUIDE.md)** | P2P agent mesh networking, distributed state sync, routing, and toolpod container management. | `zqk mesh status`, `zqk mesh sync` |
| **[Agent Feed & Correspondence](./AGENT_FEED_AND_CORRESPONDENCE_GUIDE.md)** | Real-time agent event stream, peer steering, status broadcasting, and human interaction inbox. | `zqk feed status`, `zqk feed steer` |
| **[MCP Server Architecture & Integration](./MCP_SERVER_ARCHITECTURE_AND_INTEGRATION_GUIDE.md)** | Model Context Protocol server lifecycle, stdio/mTLS transports, and IDE client integration shims. | `zqk mcp serve`, `zqk mcp install` |
| **[Session Lifecycle & Worker Context](./SESSION_LIFECYCLE_AND_WORKER_CONTEXT_GUIDE.md)** | Session token validation, ephemeral worktrees, context hydration, and clean teardown guarantees. | `pkg/session` |

---

## 5.1 Telemetry Pipelines & Metric Aggregators

Kernel metrics, telemetry streams, and observability dashboards.

| Guide | Focus Area | Primary Commands |
| :--- | :--- | :--- |
| **[Metrics Subsystem & Telemetry Pipeline](./METRICS_SUBSYSTEM_AND_TELEMETRY_PIPELINE_GUIDE.md)** | In-memory time series counters, gauges, histograms, and thread-safe atomic aggregators. | `zqk system telemetry`, `pkg/metrics` |
| **[Mission Control UI & Visual Web Studio](./MISSION_CONTROL_UI_VISUAL_GUIDE.md)** | Full-screen terminal dashboard (8 tabs) and browser Web Studio (Gantt roadmap and DAG visualizer). | `zqk ui`, `zqk ui -w` |
| **[Logging & Structured Event Dispatch](./LOGGING_AND_EVENT_DISPATCH_OPERATIONS_GUIDE.md)** | Fluent structured logging, MCP stream isolation, rolling log rotation, and event multiplexing. | `pkg/logging` |
| **[Workstream Observability & Metric Sets](./WORKSTREAM_OBSERVABILITY_AND_METRIC_SNAPSHOT_GUIDE.md)** | Point-in-time metric snapshots, tagged metric sets, multi-type heuristic bounds, and threshold alerts. | `pkg/wsobs` |

---

## 5.2 Self-Healing, Drift Reconciliation & System Diagnostics

Autonomous remediation, stagnation sentinels, and diagnostic captures.

| Guide | Focus Area | Primary Commands |
| :--- | :--- | :--- |
| **[System Auto-Remedy & Anomaly Healing](./SYSTEM_AUTO_REMEDY_AND_ANOMALY_HEALING_GUIDE.md)** | Automated remediation engine, stale lock/temp pruning, precondition seeding, and boundary healing. | `zqk system check --auto-fix` |
| **[Diagnostic Capture & Stagnation Detection](./DIAGNOSTIC_CAPTURE_AND_STAGNATION_DETECTION_GUIDE.md)** | Loop stagnation detection, failure signatures, SIGUSR1/SIGUSR2 signal profiling, and pprof servers. | `pkg/diagnostics` |
| **[Diagnostic Reports & System Vitality](./DIAGNOSTIC_REPORTS_AND_SYSTEM_VITALITY_GUIDE.md)** | Lifecycle maturation scores, system vitality health metrics, and QA test telemetry summaries. | `pkg/reports`, `zqk reports` |
| **[Supply Chain Integrity & SBOM Generation](./SUPPLY_CHAIN_INTEGRITY_AND_SBOM_GUIDE.md)** | OpenVEX attestation validation, SHA-256 manifest verification, secret pattern scanning, and release gates. | `scripts/open-core`, `pkg/supply` |

---

## Recommended Learning & Onboarding Paths

```
Human Engineer Onboarding Path:
[Community First-Run] ──► [Object Lifecycle Guide] ──► [Mission Control Visual Guide] ──► [Policy Studio Guide]
      (docs/onboarding)            (docs/guides)                 (docs/manual)                 (docs/guides)

Autonomous AI Agent Onboarding Path:
[Agent Boot Protocol] ──► [Agent Onboarding Guide] ──► [ZQL & ZPARQL Guide] ──► [Single-Command Execution Loop]
     (AGENTS.md)                  (docs/onboarding)             (docs/guides)                  (docs/guides)
```

---

## Related Documentation

- **[Reference Manual](../manual/README.md)**: Deep subsystem manuals for Mission Control, Web Studio, and language references.
- **[Interactive Tutorials](../tutorials/README.md)**: Hands-on walkthroughs for object inspection, console navigation, and TUI features.
- **[Architecture Specifications](../specs/README.md)**: Formal EBNF grammars, AST schemas, and transaction execution specifications.
- **[Operational Incident Runbooks](../runbooks/README.md)**: Tactical runbooks for CAS corruption, lock contention, and WAL recovery.
- **[Documentation Index](../INDEX.md)**: Complete hierarchical site map of all ZQK Core documentation.
