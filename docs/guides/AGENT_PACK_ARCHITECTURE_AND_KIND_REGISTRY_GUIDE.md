# Agent Pack Architecture, Composition Root & Object Kind Registry Subsystem Operational Architecture Guide (`pkg/agentpack`)

## Executive Summary

The `pkg/agentpack` package defines the composition root, object spec directory paths, lifecycle mappings, and registered kind enumerations for the **ZQK Knowledge Kernel Agent Subsystem**. In the open-core modular kernel architecture, business domains, swarm execution primitives, and persona models are organized into discrete packs that can be conditionally linked or omitted at compile-time.

This subsystem provides:
1. **Agent Pack Manifest Definition:** Declares canonical package metadata (`Name`, `SpecDir`, `LifecycleDir`) for the agent domain.
2. **Authoritative Kind Registration (`Kinds()`):** Enumerates all 13 core agent object kinds owned by the agent pack.
3. **Conditional Compilation Seam (`zqk_omit_agentpack`):** Provides a build-tag boundary allowing downstream pack consumers to strip agent-specific builders and lifecycles from minimal binaries.
4. **Builder Package Composition Anchors:** Imports the instance and lifecycle builder packages (`bldr_instance_v1`, `bldr_lifecycle_v1`, `bldr_v2`) to register concrete spec decoders during package initialization.

---

## Architectural Principles & Invariants

```
                      ┌────────────────────────────────────────┐
                      │          zqk Composition Root          │
                      └───────────────────┬────────────────────┘
                                          │
                                          ▼
                      ┌────────────────────────────────────────┐
                      │             pkg/agentpack              │
                      ├───────────────────┬────────────────────┤
                      │ Package Metadata  │ Kind Registry      │
                      │ Name, SpecDir     │ Kinds() (13 kinds) │
                      ├───────────────────┴────────────────────┤
                      │ Conditional Compilation Build Tag:     │
                      │ // +build !zqk_omit_agentpack          │
                      └───────────────────┬────────────────────┘
                                          │
                   ┌──────────────────────┼──────────────────────┐
                   ▼                      ▼                      ▼
         ┌───────────────────┐  ┌───────────────────┐  ┌───────────────────┐
         │ bldr_instance_v1  │  │ bldr_lifecycle_v1 │  │      bldr_v2      │
         │ (Spec Decoders)   │  │ (State Transitions)│ │ (Type Synthesizers)│
         └───────────────────┘  └───────────────────┘  └───────────────────┘
```

### 1. Single Source of Kind Ownership Invariant
Every object kind associated with autonomous agent execution, persona modeling, tool execution, and prompt generation must be declared in `agentpack.Kinds()`. Unregistered kinds are rejected during schema validation.

### 2. Hermetic Spec and Lifecycle Paths
All spec files owned by the agent pack reside exclusively under `packs/agent/specs`, and all state machine lifecycle definitions reside under `packs/agent/lifecycles`.

### 3. Modular Omission Invariant
The build tag `zqk_omit_agentpack` ensures that embedded distributions or stripped runtime targets can completely exclude agent swarm machinery without incurring binary bloat.

---

## Core Component Reference

### 1. Package Identifiers & Paths

- `agentpack.Name`: Canonical pack identifier (`"agent"`).
- `agentpack.SpecDir`: Directory containing declarative YAML schemas (`"packs/agent/specs"`).
- `agentpack.LifecycleDir`: Directory containing state machine definitions (`"packs/agent/lifecycles"`).
- `agentpack.OmitBuildTag`: Build tag used to decouple the pack from the composition root (`"zqk_omit_agentpack"`).

### 2. Owned Object Kinds (`Kinds()`)

The pack owns exactly 13 schema types:
1. `agent_architecture`: Cognitive design and topology descriptors.
2. `agent_feed`: Real-time asynchronous communication channels between agents and human operators.
3. `agent_instruction`: Versioned directive sets and operational constraints.
4. `agent_onboarding_preparation`: Context bootstrapping and seating configurations.
5. `agent_skill`: Reusable domain competencies and tool collections.
6. `agent_task`: Atomic units of work assigned to swarm workers.
7. `context_refresh_schedule`: Rules governing periodic context compaction and refresh.
8. `mcp_built_in_tool`: Standardized Model Context Protocol tool contracts.
9. `mcp_session`: Active protocol sessions connecting agents to MCP servers.
10. `mcp_spec`: Schema contracts for MCP tool discovery.
11. `persona`: Autonomous agent behavioral profiles and operational postures.
12. `prompt_template`: Parameterized prompt skeletons with AST variable injection.
13. `provider_profile`: LLM engine and API provider credentials and endpoint configurations.

---

## Operational Troubleshooting & Runbooks

### Issue 1: Missing Agent Spec During CLI Discovery
**Symptom:** `zqk object create persona` fails with `unknown schema kind: persona`.
**Resolution:**
Verify that the binary was not compiled with `-tags zqk_omit_agentpack`:
```bash
./bin/zqk system info
```
Ensure that `packs/agent/specs` exists and is populated in the source tree:
```bash
ls -la packs/agent/specs/
```

### Issue 2: Duplicate Kind Collision
**Symptom:** Validation engine reports duplicate kind registration during composition root initialization.
**Resolution:**
Ensure that newly introduced packs do not declare kinds already present in `agentpack.Kinds()`:
```bash
go test -v ./pkg/agentpack
```

---

## Verification & Conformance Invariants

| Proof Dimension | Target Invariant | Verification Mechanism |
| :--- | :--- | :--- |
| **Static Floor Invariant** | `pkg/agentpack` statement coverage = 100.0% | `go test -cover ./pkg/agentpack` |
| **Operational Dynamic Proof** | Kind enumeration completeness, uniqueness, and path invariants | `go test -v ./pkg/agentpack -run 'TestCycle248.*'` |
| **Negative Adversarial Boundary** | Guide conformance, zero prohibited literal nanosecond IDs | Hermetic adversarial injection test cases |
