# Developer & Agent Guides

Practical, battle-tested execution guides for human software engineers and autonomous AI agents working within the **ZQK Knowledge Kernel**.

---

## Overview

While the [Reference Manual](../manual/README.md) details comprehensive subsystem APIs and [Specifications](../specs/README.md) formalize architectural grammar, these **Guides** provide step-by-step, actionable recipes for common engineering disciplines, graph operations, and multi-agent coordination patterns.

Every guide is structured to serve both **human pair programmers** using the interactive CLI/TUI and **autonomous AI agents** operating via headless interfaces or Model Context Protocol (MCP) tools.

---

## Operational Guide Directory

### 1. Knowledge Graph & Data Plane Operations

| Guide | Focus Area | Target Audience | Primary CLI & MCP Commands |
| :--- | :--- | :--- | :--- |
| **[Knowledge Kernel Object Model & Usage Guide](./KERNEL_OBJECT_USAGE_GUIDE.md)** | Canonical taxonomy across all 14 ontological domains and packs (70+ object kinds), standard envelopes, VDS cascade, and copy-pasteable CLI recipes. | All Engineers & Agents | `zqk object create`, `zqk object list`, `zqk object ref add`, `zqk object promote` |
| **[Knowledge Management, Vocabularies & Semantic Recall](./KNOWLEDGE_MANAGEMENT_AND_SEMANTIC_RECALL_GUIDE.md)** | Operational glossaries, lens taxonomies, composable libraries, `docman` CAS verification, and on-demand recall protocols. | AI Agents, Architects, SREs | `zqk docman register`, `zqk docman verify`, `zqk object list glossary_term` |
| **[Declarative Graph Queries (ZPARQL) & Atomic Mutations (ZQL)](./ZQL_ZPARQL_AGENT_GUIDE.md)** | Multi-hop graph traversals, Cypher-like pattern queries, and atomic multi-object ACID transactions. | AI Agents, Backend Devs | `zqk query`, `zqk mutate`, `query_zparql`, `mutate_zql` |
| **[Object Lifecycle, State Machines & CAS Storage](./OBJECT_LIFECYCLE_AND_CAS_STORAGE_GUIDE.md)** | Schema contracts, Content-Addressed Storage (CAS) membrane, two-phase draft/authoritative isolation, and DoD criteria binding. | All Engineers & Agents | `zqk object create`, `zqk object inspect`, `zqk object promote` |
| **[Objects & Specs Subsystem Operational & Architectural Guide](./OBJECTS_AND_SPECS_SUBSYSTEM_GUIDE.md)** | Ontological schema contracts, spec roots, meta-schema compilation, trait normalization, and shockwave propagation. | Kernel Architects & Agents | `pkg/objects` |
| **[CLI Object Subsystem Operational Guide](./CLI_OBJECT_SUBSYSTEM_GUIDE.md)** | Subsystem command taxonomy, CAS storage membrane, two-phase draft plane isolation, and relational query pipelines. | All Engineers & Agents | `zqk object create`, `zqk object get`, `zqk object list`, `zqk object update`, `zqk object delete`, `zqk object promote`, `zqk object demote`, `zqk object ref` |
| **[Storage Subsystem Operational & Architectural Guide](./STORAGE_SUBSYSTEM_GUIDE.md)** | Foundational persistence engine, Content Addressable Storage (CAS), write-behind audit event buffering, transaction safety, and TSDB engine. | Storage Engineers & Agents | `pkg/storage` |
| **[CLI Organizational Subsystem Architectural Guide](./CLI_ORGANIZATIONAL_SUBSYSTEM_GUIDE.md)** | Subsystem command taxonomy, domain modeling, impact analysis cascade, propagation lifecycle, and declarative structure sync. | All Engineers & Agents | `zqk organizational record-change`, `zqk organizational analyze-impact`, `zqk organizational propagate`, `zqk organizational sync` |
| **[CLI Application Subsystem Architectural & Operational Guide](./CLI_APP_SUBSYSTEM_GUIDE.md)** | Root application bootstrap, command tree composition, execution context propagation, runtime diagnostics, and authentication middleware. | All Engineers & Agents | `zqk`, `cmd/zqk/app` |
| **[CLI Code Search Subsystem Operational & Architectural Guide](./CLI_GREP_SUBSYSTEM_GUIDE.md)** | Subsystem command taxonomy, pure-Go in-process search, trigram indexing, Go AST structural queries, and token-budgeted AI agent output. | All Engineers & Agents | `zqk grep`, `zgrep` |

---

### 2. Governance, Security & Policy DSL

| Guide | Focus Area | Target Audience | Primary CLI & MCP Commands |
| :--- | :--- | :--- | :--- |
| **[Validation Subsystem Architectural & Operational Guide](./VALIDATION_SUBSYSTEM_GUIDE.md)** | Multi-layer validation architecture (Layers 0–4), AST invariant checks, async priority queue execution, automated remediation engine, and >=85.0% statement coverage invariant. | All Engineers & Agents | `pkg/validation`, `zqk system check`, `zqk object inspect` |
| **[Custom Validation Policy Creation & Rule DSL](./POLICY_CREATION_AND_VALIDATION_DSL_GUIDE.md)** | Composing declarative Validation Rule DSL expressions, live population dry-run audits, grandfathering, and check-valve registration. | Tech Leads, Governance Stewards | `zqk object inspect --policy-studio`, `zqk system check-policy` |

---

### 3. Autonomous Execution & Continuous Loops

| Guide | Focus Area | Target Audience | Primary CLI & MCP Commands |
| :--- | :--- | :--- | :--- |
| **[Single-Command Execution Loop (`zqk do`)](./SINGLE_COMMAND_EXECUTION_LOOP_GUIDE.md)** | 5-phase execution engine (Intent ➔ Preflight ➔ Locks ➔ Dispatch ➔ Verification), fail-closed invariant guards, and anti-thrashing discipline. | Autonomous AI Agents | `zqk do <intent>`, `zqk workflow whats-next` |
| **[CLI Application Subsystem Architectural & Operational Guide](./CLI_APP_SUBSYSTEM_GUIDE.md)** | Subsystem lifecycle, execution topology, session persistence, runtime diagnostics, profiling, shell completions, and failure isolation. | All Engineers & Agents | `zqk`, `cmd/zqk/app`, `zqk completion` |
| **[CLI Workflow Subsystem Architectural & Operational Guide](./CLI_WORKFLOW_SUBSYSTEM_GUIDE.md)** | Subsystem command taxonomy, composite ambient routing, deterministic next-action resolution, gate preconditions, and VDS verification. | All Engineers & Agents | `zqk workflow whats-next`, `zqk workflow next`, `zqk workflow check`, `zqk workflow add` |
| **[Scheduler Subsystem Operational & Architectural Guide](./SCHEDULER_SUBSYSTEM_GUIDE.md)** | Embedded job queue, CAP loop orchestrator, transient retry membranes, atomic job state registry, retention cleanup, and watchdog monitoring. | Kernel Engineers & Agents | `pkg/scheduler`, `zqk scheduler` |

---

### 4. Multi-Agent Swarm Collaboration

| Guide | Focus Area | Target Audience | Primary CLI & MCP Commands |
| :--- | :--- | :--- | :--- |
| **[Multi-Agent Swarm Orchestration & Mesh Collaboration](./AGENT_ORCHESTRATION_AND_SWARM_COLLABORATION_GUIDE.md)** | Agent seating, append-only agent feeds, conflict-free single-claimant work leases, and ambient signal interpretation rubrics. | Swarm Engineers, Multi-Agent Setups | `zqk system agent-onboard`, `zqk agent feed`, `zqk agent claim-work` |
| **[CLI MCP-Simple Subsystem Architectural Guide](./CLI_MCP_SIMPLE_SUBSYSTEM_GUIDE.md)** | Minimal footprint Model Context Protocol (MCP) server, stdio & mTLS transports, diagnostic tools, and isolated client execution. | AI Agents, Swarm Integrators | `zqk mcp-simple`, `zqk mcp-simple -mtls-addr` |

---

### 5. Diagnostics, Self-Healing & Telemetry

| Guide | Focus Area | Target Audience | Primary CLI & MCP Commands |
| :--- | :--- | :--- | :--- |
| **[Diagnostics, Auto-Remedies & Self-Healing](./DIAGNOSTICS_SELF_HEALING_AND_REMEDY_GUIDE.md)** | 4-layer health checks, automated resolution of stale locks and orphaned temp buffers, and CAS quarantine recovery. | SREs, Operators, CI/CD | `zqk system check --auto-remedy`, `zqk system doctor` |
| **[CLI System Subsystem & Health Operations Guide](./CLI_SYSTEM_SUBSYSTEM_GUIDE.md)** | Multi-layer validation, automated remediation engine, strategic alignment scoring, and service lifecycle management. | SREs, Operators, Swarm Engineers | `zqk system check`, `zqk system align`, `zqk system status` |

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
