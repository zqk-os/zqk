# AI Agent Onboarding (ZQK Core)

You are operating on **ZQK Core**, the canonical open-core system kernel that ZQK Studio pulls in directly as an upstream dependency.

## Core Directives for Autonomous Agents

1. **Core-First Hermeticity**: Keep Core strictly self-contained. Never add dependencies on downstream Studio scripts, paths, or proprietary concepts. All workflows must function standalone in this repository.
2. **First-Run Protocol**:
   - Run `./bin/zqk system agent-onboard --format json` to detect your agent host and seat your identity.
   - Run `./bin/zqk workflow whats-next --format json` to self-discover active priority plans and backlog items from the Knowledge Kernel.
3. **Storage & Data Discipline**:
   - Kernel process data lives strictly under `.zqk/process/` and content-addressable storage (CAS).
   - Never write process instances directly to `.zqk/specs/objects/` or bypass the CLI intake layer (`zqk intake` / `zqk object create`).
   - Project YAML single source of truth is `config/zqk.yaml` / `config/zqk-local.yaml`.
4. **Lifecycle & Daemons**:
   - Promote binaries and restart daemons using Core tooling: `./scripts/install.sh && ./bin/zqk mcp restart`.

