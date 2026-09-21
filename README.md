# ZQK OS (`zqk@zqkos.com`)

> **The Cellular Knowledge Operating System for Autonomous Agent Swarms**

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/zqk-os/zqk)](https://goreportcard.com/report/github.com/zqk-os/zqk)

ZQK (Zen Quantum Kernel) is an open-core, distributed **Cellular Knowledge Operating System (Cellular OS)** built in Go. It treats autonomous multi-agent systems as a biological computing substrate: where individual kernels operate like sovereign cells, transactional membranes isolate memory planes, and an active knowledge graph serves as the operational nervous system.

**Your AI agents are coding blind.** Dumping uncurated conversation logs into fragile vector swamps or letting agents edit repositories raw leads to catastrophic context decay. ZQK provides **cellular isolation**, **epistemic hygiene**, and **self-healing biological guardrails** so agent swarms govern themselves.

This repository hosts **ZQK Core**, the open-core community microkernel.

---

## 🧬 Biological Systems Architecture

Rather than treating multi-agent work as loose prompt scripts or terminal splits, ZQK introduces four foundational biological primitives:

1. **The Cell (Sovereign Local Node):** Each ZQK kernel is an authoritative domain expert over its local environment and private knowledge graph. No bloated, uncurated global vector lakes.
2. **The Membrane (Deterministic Boundaries):** A strict multi-plane state machine (`PlaneDraft` $\rightarrow$ `PlaneStaged` $\rightarrow$ `PlanePromoted`) ensuring that zero unverified agent mutations touch working code.
3. **The Nervous System (Active Operational Graph):** An active, real-time state bus driving task execution, lineage, and dependency trees—not a passive data dump.
4. **The Organism (Inter-Cellular Mesh):** Domain-expert kernels communicating over a typed P2P wire protocol to achieve compound objectives without central micromanagement.

---

## ⚡ Quickstart (5 Minutes)

👉 **[Community First-Run Guide (Human + Agent)](./docs/onboarding/COMMUNITY_FIRST_RUN.md)**

### 1. Build the Microkernel
```sh
make          # → ./bin/zqk
./bin/zqk --version
```

### 2. Seed a Sovereign Cell (Polyglot: Go, Python, TS, Rust)
```sh
mkdir my-project && cd my-project
/path/to/this-repo/bin/zqk system init --project-name my-project
/path/to/this-repo/bin/zqk quickstart
```

*Note: Do **not** `export ZQK_PROJECT_ROOT` in your shell profile. ZQK automatically discovers the nearest `.zqk/` cellular membrane in your working tree.*

### 3. Seat Your AI Agent (Cursor, Claude Code, Windsurf, Cline)
```sh
./bin/zqk system agent-onboard --format json
./bin/zqk system start-here
```

### 4. Connect via Model Context Protocol (MCP)
```sh
./bin/zqk mcp install
./bin/zqk mcp ensure --tcp 127.0.0.1:8443
# Cursor stdio: ./bin/zqk mcp cursor-adapter
```

---

## 🏛️ The 3-Tier Layering Model

| Layer | System Tier | Responsibilities |
| :--- | :--- | :--- |
| **Layer 3** | **Domain Workflows & Applications** | Software Engineering Profile, Security Ops Enclaves, Custom Enterprise Swarms |
| **Layer 2** | **The Kernel Standard Library (Core DNA)** | `Intent` (Objective), `WorkUnit` (BLI), `InvariantGate` (Verification), `LineageNode` (ADR) |
| **Layer 1** | **Microkernel Runtime Engine** | Graph State Bus, Multi-Plane Isolation (`Draft` vs `Promoted`), Deterministic Scheduler, IPC Protocol |

---

## ⚖️ Open-Core Boundary: Single Cell vs. Organism Mesh

ZQK follows the classical operating system boundary: **POSIX/Kernel primitives are 100% open-source; distributed multi-tenant mesh clustering and fleet governance are commercial.**

| Dimension | ZQK Core (Open-Source Community) | ZQK Enterprise Mesh (ZQK Studio) |
| :--- | :--- | :--- |
| **Scope** | Single Autonomous Cell (Local Kernel) | Multi-Node Mesh Orchestration & Fleet Governance |
| **State & Engine** | Go microkernel, local hybrid file + graph store, local validation | Cross-cell routing, multi-tenant directory, global trust topology |
| **Task Lifecycle** | Self-contained task loop, local self-healing, deterministic planes | Fleet-wide apoptotic quarantine, cross-cell task delegation |
| **Codegen & Specs** | Core DNA schemas, public object parser, manual/template bindings | Proprietary Spec-Driven Codegen Engine, automated AST synthesis |
| **Governance & Ops** | Single-node CLI dashboard, Git CAS integrity gates | Enterprise RBAC, cross-cell compliance audit trails, global telemetry |

---

## 🎁 Core Capabilities

- **Cellular Epistemic Hygiene:** Cryptographic provenance (`parent_hash`, `agent_id`, `signature`) attached to all mutations.
- **Autonomous Project Awareness:** Agents query the graph to discover objectives, constraints, and architecture without manual prompting.
- **Native Code Search:** High-performance in-process AST and trigram search via `./bin/zqk grep` (alias `zgrep`).
- **Local-First & Offline-Ready:** Pure Go with filesystem & embedded graph storage—zero external cloud dependency required.
- **Deterministic Traceability:** Every commit and diff links back to verified invariants and backlog work units.

---

## 📚 Documentation & Guides

- **[Community First-Run Guide](./docs/onboarding/COMMUNITY_FIRST_RUN.md)** — Recommended starting point for humans and agents.
- **[First-Run Object Tutorial](./docs/onboarding/FIRST_RUN_OBJECT_TUTORIAL.md)** — Creating and managing kernel objects.
- **[Quickstart & MCP Setup](./docs/onboarding/QUICKSTART.md)** — Connecting Cursor, Claude Code, and other LLMs.
- **[Architecture Guide](./docs/architecture/README.md)** — Deep dive into the cellular microkernel architecture.
- **[Contributing](./CONTRIBUTING.md)** — Development guidelines and PR policies.
- **[Security Policy](./SECURITY.md)** — Vulnerability reporting and fail-closed security guarantees.

---

## 📬 Connect & Community

- **Website:** [zqkos.com](https://zqkos.com)
- **Public Contact:** [zqk@zqkos.com](mailto:zqk@zqkos.com)
- **Schedule a Call:** [Book a ZQK Inquiry](https://calendar.app.google/VhhrKgXqrukr48Kg7)
- **GitHub:** [zqk-os/zqk](https://github.com/zqk-os/zqk)

---

## 📄 License

Open-Core Community Kernel: [Apache License 2.0](LICENSE) (see `NOTICE`). Enterprise modules and fleet mesh tooling are maintained separately under ZQK Studio (`github.com/zqk-os/zqke`).
