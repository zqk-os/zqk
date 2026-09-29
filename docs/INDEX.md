# ZQK Core Documentation Index

Welcome to the canonical documentation for **ZQK Core**, the Cellular Knowledge Operating System for autonomous AI agent swarms and human engineering teams.

---

## 🚀 Onboarding & First-Run

| Guide | Description |
| :--- | :--- |
| **[Community First-Run](./onboarding/COMMUNITY_FIRST_RUN.md)** | Complete first-run walkthrough: build, initialize, agent-onboard, MCP setup, and DAG discovery. |
| **[Quickstart & MCP](./onboarding/QUICKSTART.md)** | Fast onboarding cheatsheet and Model Context Protocol (MCP) server configuration. |
| **[AI Agent Onboarding](./onboarding/AI_AGENT_ONBOARDING.md)** | Mandatory directives, workspace seating, and continuous autonomous loop discipline for AI agents. |
| **[First-Run Object Tutorial](./onboarding/FIRST_RUN_OBJECT_TUTORIAL.md)** | Step-by-step tutorial: creating, querying, and updating kernel objects (`question`, `task`, `plan`). |
| **[Edge & Headless Mode](./onboarding/EDGE_HEADLESS_FIRST_RUN.md)** | Headless daemon startup, edge node runtime, and CLI-only autonomous operation. |
| **[Getting Started Pointer](./getting-started.md)** | High-level index and roadmap pointer for new contributors. |

---

## 🏛️ Core Architecture & Foundation

| Specification | Description |
| :--- | :--- |
| **[Architecture Overview](./architecture/README.md)** | Core system architecture, Knowledge Kernel data plane, and daemon topology. |
| **[Lifecycle State Machines](./architecture/LIFECYCLE_STATE_MACHINE.md)** | Visual finite state machines, check-valves, roles, and cryptographic quality gates. |
| **[Ambient Signal Action Rubric](./architecture/AMBIENT_SIGNAL_ACTION_RUBRIC.md)** | Anti-thrashing precedence hierarchy (P0–P5) and deterministic CLI action gates for agents. |
| **[Modular Pack Composition](./architecture/PACK_COMPOSITION_AND_EXTENSIBILITY.md)** | Pack manifests (`pack.yaml`), builder codegen (`bldr_cli_cmd_v1`), and composition root. |
| **[Cellular Membrane Mode B](./architecture/CELLULAR_MEMBRANE_MODE_B_CONFIGURATION.md)** | Privileged writer configuration, membrane isolation, and security guardrails. |
| **[CLI Command Taxonomy](./architecture/CLI_COMMAND_TAXONOMY_STANDARDS.md)** | Canonical taxonomy standards, verb-noun structures, and AST verification rules. |
| **[Tiered Storage Lifecycle](./architecture/TIERED_STORAGE_AND_ARCHIVAL_LIFECYCLE.md)** | Storage compaction, subtree flattening, and historical capsule archival. |
| **[Tray Cryptographic Security](./architecture/TRAY_COMMAND_CRYPTOGRAPHIC_SECURITY.md)** | HMAC envelopes, payload attestation, and secure indirect command execution. |
| **[Pack Composition Root](../PACK-COMPOSITION.md)** | Architecture, manifests, and CLI tool generation contracts. |

---

## 📜 Declarative Languages & Specifications

| Technical Specification | Description |
| :--- | :--- |
| **[ZPARQL Graph Traversal Grammar](./specs/SPEC-ZPARQL-GRAPH-TRAVERSAL-GRAMMAR.md)** | Declarative graph query language for pattern matching and relationship traversal. |
| **[ZPARQL Query Planner](./specs/SPEC-ZPARQL-QUERY-PLANNER.md)** | Index-accelerated query planner, cycle-safe traversal engine, and AST optimizer. |
| **[ZPARQL Streaming & Result Envelopes](./specs/SPEC-ZPARQL-RESULT-STREAMING.md)** | Portable result envelopes, reactive streaming, and backpressure protocols. |
| **[ZQL Declarative Mutation Grammar](./specs/SPEC-ZQL-DECLARATIVE-MUTATION-GRAMMAR.md)** | Declarative ACID mutation language grammar, variable resolution, and AST. |
| **[ZQL Preflight Validation](./specs/SPEC-ZQL-PREFLIGHT-VALIDATION.md)** | In-memory preflight validation, constraint checking, and diagnostic receipts. |
| **[ZQL ACID Transaction Execution](./specs/SPEC-ZQL-TRANSACTION-EXECUTION.md)** | Staged transaction isolation, atomic commits, rollback journals, and CAS writes. |
| **[Interactive Object Inspector Spec](./specs/SPEC-OBJECT-INSPECTOR-CONSOLE-001.md)** | Interactive console specification, TUI navigation, and live Policy Studio. |

---

## 📖 Reference Manuals & Guides

| Reference / Guide | Description |
| :--- | :--- |
| **[Manual & CLI Reference](./manual/README.md)** | Comprehensive CLI command flags, environment variables, and exit codes. |
| **[ZPARQL Query Language Manual](./manual/ZPARQL_QUERY_LANGUAGE.md)** | Developer manual and syntax guide for querying the Knowledge Kernel graph. |
| **[ZQL Declarative Mutations Manual](./manual/ZQL_MUTATIONS.md)** | Developer manual for crafting declarative atomic mutations and batch updates. |
| **[Object Inspector & Policy Studio](./manual/OBJECT_INSPECTOR_AND_POLICY_STUDIO.md)** | Reference manual for the interactive object inspector, keybindings, and policy testing. |
| **[Mission Control UI & Diagnostics Visual Guide](./manual/MISSION_CONTROL_UI_VISUAL_GUIDE.md)** | Complete visual walkthrough of all 8 TUI tabs, Web Studio timeline/DAG, and test dashboards with ASCII captures. |
| **[ZQL & ZPARQL Agent Guide](./guides/ZQL_ZPARQL_AGENT_GUIDE.md)** | Field guide for AI agents executing queries, mutating graph state, and handling errors. |

---

## 🚨 Operational Incident Runbooks

| Runbook | Trigger & Incident Triage |
| :--- | :--- |
| **[Incident Runbooks Overview](./runbooks/README.md)** | Directory of operational incident runbooks and triage procedures. |
| **[RB-CAS-001: CAS Corruption Recovery](./runbooks/RB-CAS-001-CAS-CORRUPTION-RECOVERY.md)** | Resolving Content-Addressed Storage hash mismatches and quarantine exhaustion. |
| **[RB-LCK-001: Lock Contention & Deadlocks](./runbooks/RB-LCK-001-LOCK-CONTENTION-DEADLOCKS.md)** | Diagnosing concurrency lock contention, stale lock cleanup, and deadlock breaking. |
| **[RB-SCH-001: Scheduler Daemon Triage](./runbooks/RB-SCH-001-SCHEDULER-DAEMON-TRIAGE.md)** | Recovering from scheduler daemon exit cascades, stuck recurring jobs, and restarts. |
| **[RB-WAL-001: WAL Compaction Failures](./runbooks/RB-WAL-001-WAL-COMPACTION-FAILURES.md)** | Triage and repair for Write-Ahead Log compaction corruption and disk saturation. |

---

## 🛠️ How-To & Tutorials

| Guide / Tutorial | Description |
| :--- | :--- |
| **[How-To Guides Overview](./howto/README.md)** | Practical, goal-oriented guides for common administrative and engineering tasks. |
| **[Inspect & Validate Objects](./howto/INSPECT_AND_VALIDATE_OBJECTS.md)** | Inspecting objects via dual TUI/JSON projections and running live policy dry-runs. |
| **[Scheduler & Maintenance Jobs](./howto/SCHEDULER_AND_MAINTENANCE.md)** | Configuring background daemons, cron intervals, and kernel self-healing jobs. |
| **[Tutorials Overview](./tutorials/README.md)** | Learning-oriented walkthroughs exploring kernel capabilities step-by-step. |
| **[Object Inspector Tutorial](./tutorials/INTERACTIVE_OBJECT_INSPECTION_TUTORIAL.md)** | Hands-on walkthrough: navigating the graph, authoring policies, and verifying constraints. |
| **[Interactive Demonstrations & Visual Showcase](./demos/README.md)** | Visual showcase with authentic terminal SVG recordings and command walk-throughs. |

---

## 🔧 Maintenance, Development & Evaluation

| Document | Description |
| :--- | :--- |
| **[Development Overview](./development/README.md)** | Contributor conventions, build instructions, and local testing workflows. |
| **[Policy Governance & Durability](./development/POLICY_GOVERNANCE_AND_DURABILITY.md)** | Durability tiers, AST linters, and cryptographically verified policy packs. |
| **[Quality & Verification Gates](./quality/README.md)** | Definition of Done (DoD), Verification Done-Gates (VDS), and chunk evaluation. |
| **[Evaluation & Benchmarks](./eval/README.md)** | Multi-axis evaluation benchmarks: maintainability, observability, reliability, and security. |
| **[Codebase Evaluation Framework (CEF)](./quality/codebase_evaluation/README.md)** | Multi-agent evaluation framework, 52 evaluation lens rubrics, and Diamond Scale grading. |
| **[System Explanation & Philosophy](./explanation/README.md)** | Architectural philosophy: why ZQK is designed as a Cellular Knowledge Operating System. |

---

## 🤖 Agent Operating Directives & Protocols

| Directive / Guide | Description |
| :--- | :--- |
| **[AI Agent Onboarding & Operating Protocol](./onboarding/AI_AGENT_ONBOARDING.md)** | Directives, MCP integration, persona seating, and continuous autonomous loop discipline. |
| **[Ambient Signal Action Rubric](./architecture/AMBIENT_SIGNAL_ACTION_RUBRIC.md)** | Anti-thrashing precedence hierarchy (P0–P5) and deterministic CLI action gates for agents. |
| **[First-Run Quality & Verification Gates](./quality/README.md)** | Definition of Done (DoD), Verification Done-Gates (VDS), and chunk evaluation. |
| **[ZQL & ZPARQL Agent Guide](./guides/ZQL_ZPARQL_AGENT_GUIDE.md)** | Field guide for AI agents executing queries, mutating graph state, and handling errors. |

---

## 📦 Kernel Subsystems & Package Architecture

| Subsystem / Package | Architectural Focus |
| :--- | :--- |
| **[Storage Subsystem](../pkg/storage/README.md)** | Unified storage abstraction, pluggable file/graph providers, and change journals. |
| **[Graph Backend](../pkg/graph/README.md)** | MemGraph P2P provider, connection pooling, and Cypher query execution. |
| **[MCP Server Architecture](../pkg/mcp/README.md)** | Model Context Protocol engine, tool dispatch, and client adapters. |
| **[Concurrency & Synchronization](../pkg/concurrency/README.md)** | Lock contention management, timeouts, and goroutine leak prevention. |
| **[Pipeline Execution Engine](../pkg/pipeline/README.md)** | Step-based mutation pipelines, stage checkpoints, and rollback handlers. |
| **[Telemetry & Diagnostics](../pkg/telemetry/README.md)** | Structured metrics, performance telemetry, and event streaming. |
| **[Bootstrap Subsystem](../internal/bootstrap/README.md)** | Embedded tarball packaging, unpack logic, and zero-friction initialization. |
| **[CLI Kernel Architecture](../internal/cli/README.md)** | Command routing, context profiles, and fail-closed CLI execution contracts. |

---

## ⚖️ Open Core Governance

| Document | Description |
| :--- | :--- |
| **[Contributing Guide](../CONTRIBUTING.md)** | Developer Certificate of Origin (DCO), pull request standards, and code review rules. |
| **[Project Governance](../GOVERNANCE.md)** | Open-core boundaries, RFC process, technical steering, and maintainer roles. |
| **[Security Policy](../SECURITY.md)** | Vulnerability disclosure policy, response SLAs, and security advisories. |
| **[Code of Conduct](../CODE_OF_CONDUCT.md)** | Community standards and Contributor Covenant guidelines. |
