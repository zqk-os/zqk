# ZQK Architecture (Open Core)

ZQK Core is the canonical, un-bifurcated system kernel for sovereign human-agent software engineering. 

## Upstream Core & Studio Architecture

To prevent code bifurcation and eliminate the maintenance overhead of bidirectional syncs, **ZQK Studio consumes ZQK Core directly as an upstream dependency**.

- **Core (This Repository)**: Contains the foundational Knowledge Kernel, content-addressable storage (CAS), task DAG scheduler, ambient monitoring daemons, Model Context Protocol (MCP) server & adapters, and the full CLI taxonomy.
- **Studio (Downstream Consumer)**: Imports ZQK Core and layers enterprise/proprietary capabilities (such as multi-tenant orchestration, specialized enterprise compliance packs, and proprietary fine-tunes) on top of the Core foundation.
- **Hermetic Self-Sufficiency**: Core must never depend on Studio, reference studio-only internal scripts, or rely on downstream assumptions. All build, verification, installation, and daemon lifecycle workflows in Core are fully self-contained (`./scripts/install.sh`, `zqk scheduler restart`, `zqk mcp restart`).

## Getting Started

1. [`../onboarding/COMMUNITY_FIRST_RUN.md`](../onboarding/COMMUNITY_FIRST_RUN.md)
2. `./bin/zqk system agent-onboard --format json`
3. `./bin/zqk workflow whats-next --format json`

