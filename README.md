# ZQK OS

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/zqk-os/zqk)](https://goreportcard.com/report/github.com/zqk-os/zqk)
[![Documentation](https://img.shields.io/badge/docs-docs.zqk.dev-blue)](https://docs.zqk.dev)

**The Cellular Knowledge Operating System for autonomous agent swarms.** This repository is **ZQK Core**, the open-core community microkernel. Canonical portal: [Community First-Run Guide](docs/onboarding/COMMUNITY_FIRST_RUN.md).

## Quickstart

### Installation (macOS & Linux)

```sh
# Single-command install (defaults to interactive choice: Go Fast or Walk Through)
curl -sSL https://raw.githubusercontent.com/zqk-os/zqk/main/scripts/install.sh | sh
```

Or install via Homebrew:
```sh
brew tap zqk-os/tap && brew install zqk
```

---

### Choose Your Path: Go Fast vs. Walk Through

ZQK offers two paths for Polyglot projects (Go, Python, TypeScript, Rust):

#### Option 1: Go Fast
*For developers who want immediate token savings with zero learning curve.*

Installs `zqk` and links `zgrep` (`zqk grep`) into your PATH. Drop it directly into Cursor, Claude Code, Cline, or Aider to stop context-window blowouts:

```sh
# Token-budgeted search via zqk grep / zgrep (default 500 tokens, sub-15ms)
zgrep "HandleRequest" --max-tokens 500 -f json

# Go AST structural search
zgrep --ast --kind struct MemoryStore
```

Add this to `.cursorrules`, `CLAUDE.md`, or `.clinerules`:
```markdown
- NEVER run raw recursive grep or find.
- ALWAYS use `zgrep <query> --max-tokens 500 -f json`.
- For Go syntax: `zgrep --ast --kind struct|func <name>`.
```

#### Option 2: Walk Through
*For developers who want to understand core principles and see the Knowledge Kernel in action.*

Conducts an interactive walkthrough to capture intent, objectify goals, and experience the 5-layer cascade:

```sh
# Launch the interactive walkthrough
zqk system start-here
```

- **5-Layer Cascade:** Vision ➔ Goals ➔ Priority Plans ➔ Requirements ➔ Backlog Items.
- **Fail-Closed Done Gates:** Automated AST verification and test execution before state transitions.
- **Visual Web Studio:** Real-time Gantt timeline and DAG visualization:
  ```sh
  zqk ui -w
  ```
- **Ambient MCP Server:** Expose local kernel tools to Cursor or Claude:
  ```sh
  zqk mcp proxy --tcp 127.0.0.1:7777
  ```

---

> *The Substrate for Living Software: Sovereign Cells. Verified Truth. Autonomous Organisms.*

ZQK (Zen Quantum Kernel) is an open-core, distributed **Cellular Knowledge Operating System (Cellular OS)** engineered in high-performance Go. It fundamentally rejects the status quo of gluing autonomous agents together with fragile Python scripts and chaotic vector swamps.

Instead, ZQK treats multi-agent systems as a biological computing substrate built upon a rigorous **microkernel architecture**: where individual repositories and services function as **autonomous holons**—sovereign, self-governing wholes that integrate seamlessly into a collaborative distributed organism.

**Your AI agents are coding blind.** Dumping uncurated conversation logs into vector lakes produces context decay and runaway hallucination blast radiuses. ZQK provides **cellular memory isolation**, **epistemic hygiene**, **ambient feedback sensing**, and **biological self-healing guardrails** so agent swarms govern themselves.

## Biological Systems & Holonic Architecture

Rather than treating multi-agent work as loose prompt scripts or terminal splits, ZQK introduces four foundational biological primitives:

1. **The Cell / Holon (Sovereign Node):** Each ZQK kernel is an autonomous holon—simultaneously a complete, self-governing whole and an organic participant in the wider mesh. It maintains authoritative stewardship over its local codebase, tests, and private knowledge graph without relying on bloated global vector lakes.
2. **The Membrane (Deterministic Boundaries):** A strict multi-plane state machine (`Draft` → `Promoted` (via CAS membrane qualification)) ensuring that zero unverified agent mutations ever pollute working code or canonical state.
3. **The Nervous System (Active Operational Graph):** An active, real-time synaptic state bus driving task execution, causal provenance lineage, and dependency trees—not a passive secondary data lake.
4. **The Organism (Inter-Cellular Mesh):** Domain-expert kernels communicating over a typed P2P wire protocol to achieve compound objectives without central micromanagement.

## Autonomous Memory & Imparted Human Intent

Knowledge in ZQK is never static documentation. It is an active, sensing memory layer:

- **Programmed Human Intent:** Human goals, non-negotiable rules, architectural invariants, mission, and vision are encoded directly into kernel schema objects (`Goal`, `Policy`, `InvariantGate`). Agents never invent objectives in isolation; their actions are strictly bounded by human-imparted intent.
- **Ambient Feedback Sensing:** Integrated background daemons monitor filesystem changes, test execution suites, and process drift. The kernel senses when reality diverges from intent and dynamically triggers corrective cycles.
- **Temporal Agility (Snapshot, Rollback & Cherry-Pick):** Treat knowledge like Git commits. Create atomic cryptographic snapshots, fork or branch memory planes for parallel experimentation, rollback failed excursions, and cherry-pick verified knowledge across cells.
- **Laser Context Curation & Token Efficiency:** Rather than exhausting LLM context windows on uncurated chat history, ZQK slices precise graph subtrees based on the specific WorkUnit an agent claims—**significantly reducing token consumption** while maximizing reasoning accuracy.

## The 3-Tier Layering Model

| Layer | System Tier | Responsibilities |
| :--- | :--- | :--- |
| **Layer 3** | **Domain Workflows & Applications** | Software Engineering Profile, Security Ops Enclaves, Custom Enterprise Swarms |
| **Layer 2** | **The Kernel Standard Library (Core DNA)** | `Goal` (Objective), `BacklogItem` (BLI / WorkUnit), `InvariantGate` (Verification), `Decision` (ADR) |
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

- **[Community First-Run Guide](docs/onboarding/COMMUNITY_FIRST_RUN.md)** ([Web Portal](https://docs.zqk.dev/docs/onboarding/COMMUNITY_FIRST_RUN.html)) — Recommended starting point for humans and agents.
- **[First-Run Object Tutorial](docs/onboarding/FIRST_RUN_OBJECT_TUTORIAL.md)** ([Web Portal](https://docs.zqk.dev/docs/onboarding/FIRST_RUN_OBJECT_TUTORIAL.html)) — Creating and managing kernel objects.
- **[Quickstart & MCP Setup](docs/onboarding/QUICKSTART.md)** ([Web Portal](https://docs.zqk.dev/docs/onboarding/QUICKSTART.html)) — Connecting Cursor, Claude Code, and other LLMs.
- **[Architecture Guide](docs/architecture/README.md)** ([Web Portal](https://docs.zqk.dev/docs/architecture/README.html)) — Deep dive into the cellular microkernel architecture.
- **[Contributing](CONTRIBUTING.md)** — Development guidelines and PR policies.
- **[Security Policy](SECURITY.md)** — Vulnerability reporting and fail-closed security guarantees.

## Connect & Community

- **Website:** [zqk.dev](https://zqk.dev)
- **Documentation Portal:** [docs.zqk.dev](https://docs.zqk.dev)
- **General Inquiries:** [inquiry@zqkos.com](mailto:inquiry@zqkos.com)
- **Technical Support:** [support@zqkos.com](mailto:support@zqkos.com)
- **Schedule a Call:** [Book a ZQK Inquiry](https://calendar.app.google/VhhrKgXqrukr48Kg7)
- **GitHub:** [zqk-os/zqk](https://github.com/zqk-os/zqk)

## License

Open-Core Community Kernel: [Apache License 2.0](LICENSE) (see `NOTICE`). Enterprise modules and multi-node fleet mesh capabilities are licensed separately.
