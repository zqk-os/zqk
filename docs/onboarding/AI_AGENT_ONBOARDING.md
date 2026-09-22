# AI Agent Onboarding & Operating Protocol

Welcome, agent. This document outlines the operational directives, architectural constraints, and self-discovery protocols for operating autonomously within a ZQK repository.

---

## Core Directives for Autonomous Agents

1. **Kernel Primacy & State Integrity**:
   - Process states, task ownership, roadmaps, and requirements exist exclusively in the Knowledge Kernel.
   - Do not store state, loops, or notes in local vendor-specific scratch directories (such as `.gemini/` or `.cursor/`). If information is not committed to the kernel graph or registered via the CLI, it does not exist.

2. **Session Initialization & Self-Discovery**:
   - On initial contact or when starting a session, run `./bin/zqk system agent-onboard --format json` to detect your agent host and register seating.
   - Proactively discover current mission, active priority plans, and backlog items by executing `./bin/zqk workflow whats-next --format json`. Do not stop or wait for manual human instructions if active plan work is queued.

3. **Intake & Mutation Boundaries**:
   - Process instances live strictly under `.zqk/process/` and content-addressable storage (CAS).
   - Never write process YAML files directly or bypass the CLI intake layer (`zqk intake` / `zqk object create`). All object creation must satisfy schema validation rules.
   - Project configuration is maintained in `config/zqk.yaml` / `config/zqk-local.yaml`.

4. **Lifecycle & Daemons**:
   - Background tasks and test executions must be dispatched through the native scheduler (`zqk scheduler`).
   - Re-prime directives and verify daemon status using standard commands: `./scripts/install.sh && ./bin/zqk mcp restart`.

