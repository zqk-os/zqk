# ZQK Core Architecture

ZQK Core is the foundational system kernel for sovereign human-agent software engineering. It provides an offline-first, mathematically auditable operating system designed to orchestrate autonomous AI agents alongside human developers.

---

## Architectural Foundations

The architecture is built on four core pillars:

1. **Knowledge Kernel & Graph System of Record**:
   - Process states, roadmaps, workstreams, requirements, acceptance criteria, and backlog items are represented as first-class, semantically typed system objects.
   - Objects maintain explicit cryptographic integrity and lineage tracking via Content-Addressable Storage (CAS).

2. **The Cellular Membrane**:
   - Enforces isolation between autonomous agent operations and persistent system state.
   - Mutations pass through the Semantic Intake Reasoner (`zqk intake` / `zqk object create`) to guarantee schema validation and prevent unauthorized state drift.

3. **Deterministic Scheduler & Task DAG**:
   - High-performance, concurrent scheduler daemon (`zqk scheduler`) manages background loops, maintenance tasks, and automated test-bundle sweeps.
   - Fail-closed execution semantics ensure tasks cannot silently fail or corrupt system state.

4. **Native Agent Mesh & MCP Integration**:
   - Implements native Model Context Protocol (MCP) server endpoints (`zqk mcp`) for seamless bidirectional pairing with IDE agents (Cursor, Claude Code, Cline, Windsurf).
   - Agent seating, role specialization, and workspace directive synchronization (`zqk system agent-onboard`) bridge local workspaces and kernel memory.

---

## Architectural Specifications

- **[Cellular Membrane Configuration](./CELLULAR_MEMBRANE_MODE_B_CONFIGURATION.md)**: Privileged Writer barrier and UNIX domain socket intake topology.
- **[CLI Command Taxonomy & Standards](./CLI_COMMAND_TAXONOMY_STANDARDS.md)**: Unified CLI taxonomy, argument validation, and automated spec verification.
- **[Tiered Storage & Capsule Archival](./TIERED_STORAGE_AND_ARCHIVAL_LIFECYCLE.md)**: 4-tier lifecycle (Hot → Warm → Cold → Abyss) and immutable Semantic Capsules.
- **[Tray Cryptographic Security](./TRAY_COMMAND_CRYPTOGRAPHIC_SECURITY.md)**: P-256 ECDSA mutual handshake and HMAC-SHA256 authentication for privileged operations.

---

## Next Steps

- **[First-Run Guide](../onboarding/COMMUNITY_FIRST_RUN.md)**: Get started with installation and local configuration.
- **[First-Run Object Tutorial](../onboarding/FIRST_RUN_OBJECT_TUTORIAL.md)**: Walk through creating, inspecting, and updating kernel objects.

