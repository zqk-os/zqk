# ZQK OS (`zqk@zqkos.com`)

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/zqk-os/zqk)](https://goreportcard.com/report/github.com/zqk-os/zqk)

**The Cellular Knowledge Operating System for autonomous agent swarms.** This repository is **ZQK Core**, the open-core community microkernel. Full first-run: [Community First-Run Guide](./docs/onboarding/COMMUNITY_FIRST_RUN.md).

## Quickstart (5 Minutes)

### 1. Install or Build

**Via Homebrew (macOS & Linux):**
```sh
brew tap zqk-os/zqk
brew install zqk
```

**Or Build from Source:**
```sh
make          # → ./bin/zqk
./bin/zqk --version
```

### 2. Seed a Sovereign Cell (Polyglot: Go, Python, TS, Rust)
```sh
mkdir my-project && cd my-project
zqk init
zqk quickstart
```

Do **not** `export ZQK_PROJECT_ROOT` in your shell profile. ZQK discovers the nearest `.zqk/` membrane from the working tree.

### 3. Seat Your AI Agent (Cursor, Claude Code, Windsurf, Cline)
```sh
zqk system agent-onboard --format json
zqk system start-here
```

### 4. Connect via Model Context Protocol (MCP)
```sh
zqk mcp install
zqk mcp ensure --tcp 127.0.0.1:8443
# Cursor stdio: zqk mcp cursor-adapter
```

### 5. Run Swarms & Live Telemetry
```sh
# Run a portable swarm package (local path or remote git URL)
zqk run ./examples/swarms/code-eval/
# Or run from remote git: zqk run https://github.com/zqk-os/swarm-starter-kit

# Launch interactive full-screen terminal mission control
zqk ui

# Stream real-time mutations with the visual ANSI seismograph
zqk state stream --dashboard
```

---

> *The Substrate for Living Software: Sovereign Cells. Verified Truth. Autonomous Organisms.*

ZQK (Zen Quantum Kernel) is an open-core, distributed **Cellular Knowledge Operating System (Cellular OS)** engineered in high-performance Go. It fundamentally rejects the status quo of gluing autonomous agents together with fragile Python scripts and chaotic vector swamps.

Instead, ZQK treats multi-agent systems as a biological computing substrate built upon a rigorous **microkernel architecture**: where individual repositories and services function as **autonomous holons**—sovereign, self-governing wholes that integrate seamlessly into a collaborative distributed organism.

**Your AI agents are coding blind.** Dumping uncurated conversation logs into vector lakes produces context decay and runaway hallucination blast radiuses. ZQK provides **cellular memory isolation**, **epistemic hygiene**, **ambient feedback sensing**, and **biological self-healing guardrails** so agent swarms govern themselves.

## Biological Systems & Holonic Architecture

Rather than treating multi-agent work as loose prompt scripts or terminal splits, ZQK introduces four foundational biological primitives:

1. **The Cell / Holon (Sovereign Node):** Each ZQK kernel is an autonomous holon—simultaneously a complete, self-governing whole and an organic participant in the wider mesh. It maintains authoritative stewardship over its local codebase, tests, and private knowledge graph without relying on bloated global vector lakes.
2. **The Membrane (Deterministic Boundaries):** A strict multi-plane state machine (`PlaneDraft` → `PlaneStaged` → `PlanePromoted`) ensuring that zero unverified agent mutations ever pollute working code or canonical state.
3. **The Nervous System (Active Operational Graph):** An active, real-time synaptic state bus driving task execution, causal provenance lineage, and dependency trees—not a passive secondary data lake.
4. **The Organism (Inter-Cellular Mesh):** Domain-expert kernels communicating over a typed P2P wire protocol to achieve compound objectives without central micromanagement.

## Autonomous Memory & Imparted Human Intent

Knowledge in ZQK is never static documentation. It is an active, sensing memory layer:

- **Programmed Human Intent:** Human goals, non-negotiable rules, architectural invariants, mission, and vision are encoded directly into kernel schema objects (`Intent`, `Policy`, `InvariantGate`). Agents never invent objectives in isolation; their actions are strictly bounded by human-imparted intent.
- **Ambient Feedback Sensing:** Integrated background daemons monitor filesystem changes, test execution suites, and process drift. The kernel senses when reality diverges from intent and dynamically triggers corrective cycles.
- **Temporal Agility (Snapshot, Rollback & Cherry-Pick):** Treat knowledge like Git commits. Create atomic cryptographic snapshots, fork or branch memory planes for parallel experimentation, rollback failed excursions, and cherry-pick verified knowledge across cells.
- **Laser Context Curation & Token Efficiency:** Rather than exhausting LLM context windows on uncurated chat history, ZQK slices precise graph subtrees based on the specific WorkUnit an agent claims—**reducing token bloat by up to 80%** while maximizing reasoning accuracy.

## The 3-Tier Layering Model

| Layer | System Tier | Responsibilities |
| :--- | :--- | :--- |
| **Layer 3** | **Domain Workflows & Applications** | Software Engineering Profile, Security Ops Enclaves, Custom Enterprise Swarms |
| **Layer 2** | **The Kernel Standard Library (Core DNA)** | `Intent` (Objective), `WorkUnit` (BLI), `InvariantGate` (Verification), `LineageNode` (ADR) |
| **Layer 1** | **Microkernel Runtime Engine** | Graph State Bus, Multi-Plane Isolation (`Draft` vs `Promoted`), Deterministic Scheduler, IPC Protocol |

## Open-Core Boundary: Single Cell vs. Organism Mesh

ZQK follows the classical operating system boundary: **POSIX/Kernel primitives are 100% open-source; distributed multi-tenant mesh clustering and fleet governance are commercial.**

| Dimension | ZQK Core (Open-Source Community) | Enterprise Fleet Mesh |
| :--- | :--- | :--- |
| **Scope** | Single Autonomous Cell (Local Kernel) | Multi-Node Mesh Orchestration & Fleet Governance |
| **State & Engine** | Go microkernel, local hybrid file + graph store, local validation | Cross-cell routing, multi-tenant directory, global trust topology |
| **Task Lifecycle** | Self-contained task loop, local self-healing, deterministic planes | Fleet-wide apoptotic quarantine, cross-cell task delegation |
| **Codegen & Specs** | Core DNA schemas, public object parser, manual/template bindings | Fleet Spec-Driven Codegen Engine, automated AST synthesis |
| **Governance & Ops** | Single-node CLI dashboard, Git CAS integrity gates | Enterprise RBAC, cross-cell compliance audit trails, global telemetry |

## Core Capabilities

- **Cellular Epistemic Hygiene:** Cryptographic provenance (`parent_hash`, `agent_id`, `signature`) attached to all mutations.
- **Autonomous Project Awareness:** Agents query the graph to discover objectives, constraints, and architecture without manual prompting.
- **Native Code Search:** High-performance in-process AST and trigram search via `./bin/zqk grep` (alias `zgrep`).
- **Local-First & Offline-Ready:** Pure Go with filesystem & embedded graph storage—zero external cloud dependency required.
- **Deterministic Traceability:** Every commit and diff links back to verified invariants and backlog work units.

## Documentation & Guides

- **[Community First-Run Guide](./docs/onboarding/COMMUNITY_FIRST_RUN.md)** — Recommended starting point for humans and agents.
- **[First-Run Object Tutorial](./docs/onboarding/FIRST_RUN_OBJECT_TUTORIAL.md)** — Creating and managing kernel objects.
- **[Quickstart & MCP Setup](./docs/onboarding/QUICKSTART.md)** — Connecting Cursor, Claude Code, and other LLMs.
- **[Architecture Guide](./docs/architecture/README.md)** — Deep dive into the cellular microkernel architecture.
- **[Contributing](./CONTRIBUTING.md)** — Development guidelines and PR policies.
- **[Security Policy](./SECURITY.md)** — Vulnerability reporting and fail-closed security guarantees.

## Connect & Community

- **Website:** [zqkos.com](https://zqkos.com)
- **Public Contact:** [zqk@zqkos.com](mailto:zqk@zqkos.com)
- **Schedule a Call:** [Book a ZQK Inquiry](https://calendar.app.google/VhhrKgXqrukr48Kg7)
- **GitHub:** [zqk-os/zqk](https://github.com/zqk-os/zqk)

## License

Open-Core Community Kernel: [Apache License 2.0](LICENSE) (see `NOTICE`). Enterprise modules and multi-node fleet mesh capabilities are licensed separately.
