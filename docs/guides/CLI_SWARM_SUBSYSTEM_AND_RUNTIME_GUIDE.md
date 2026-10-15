# CLI Swarm Subsystem & Multi-Agent Runtime Guide

## Overview

The ZQK Swarm Subsystem (`cmd/zqk/swarm`) provides the command-line interface and runtime coordination layer for executing autonomous, token-budgeted multi-agent swarms. It allows developers and automated Technical Program Managers (TPMs) to initialize swarm topologies, monitor swarm throughput across persistent agent tasks, and ingest declarative swarm package manifests (`swarm.yaml`) directly into the Knowledge Kernel.

---

## Architectural Taxonomy & Core Components

```
cmd/zqk/swarm/
├── swarm.go              # Root 'zqk swarm' command builder and subcommand aggregation
├── init.go               # 'swarm init' and 'agent swarm-init' pipeline setup & chat bootstrap
├── status.go             # 'swarm status' real-time task/instruction aggregation & throughput metrics
└── run.go                # 'swarm run' & 'zqk run' package manifest ingestion & execution engine
```

---

## 1. Swarm Initialization (`swarm init`)

The `swarm init` command configures an isolated multi-agent execution pipeline, binds coordinator identities, initializes chat mesh mailboxes, and ensures all MCP daemons and seat workers are responsive.

### Command Syntax

```bash
zqk swarm init [flags]
```

### Key Flags

| Flag | Type | Description |
|------|------|-------------|
| `--pipeline` | string | Target pipeline ID to initialize. |
| `--workflow` | string | Optional workflow ID associated with the pipeline. |
| `--plan-id` | string | Priority Plan ID bounding the active cycle of work. |
| `--agent-id` | string | Coordinator agent identifier (defaults to seat coordinator). |
| `--persona-ref` | string | Persona reference assigned to the coordinator seat. |
| `--from-stage` | string | Resume execution from a specific stage in the pipeline. |
| `--dry-run` | bool | Validates pipeline topology and worker readiness without persisting state. |
| `--allow-chat` | bool | Enables bidirectional agent chat mesh communication. |

---

## 2. Swarm Status & Observability (`swarm status`)

The `swarm status` command aggregates active and historical execution metrics from the Knowledge Kernel, reporting the lifecycle states of all `agent_task` and `agent_instruction` objects alongside the operational status of the CAP Orchestrator daemon.

### Command Syntax

```bash
zqk swarm status [flags]
```

### Core Output Metrics

- **Agent Tasks**: Throughput counts partitioned by status (`planned`, `in_progress`, `complete`, `failed`).
- **Agent Instructions**: Instruction delivery rates and acknowledgement status.
- **CAP Orchestrator**: Health, heartbeat timestamps, and queue lag for background scheduling.
- **Worker Seats**: Registered worker seats and active claim assignments.

---

## 3. Swarm Package Execution (`swarm run`)

The `swarm run` command (also accessible via top-level `zqk run`) validates, loads, and executes declarative swarm manifests (`swarm.yaml`).

### Command Syntax

```bash
zqk swarm run <path/to/swarm.yaml> [flags]
```

### Key Flags

| Flag | Type | Description |
|------|------|-------------|
| `--dry-run` | bool | Validates manifest syntax, schema version, and stage dependencies without execution. |
| `--stage-only` | bool | Ingests the package into `.zqk/process/` and CAS storage without triggering execution. |
| `--yes`, `-y` | bool | Automatically launches swarm execution without interactive console prompts. |
| `--entrypoint` | string | Overrides the default entrypoint pipeline defined in the package manifest. |

### Manifest Schema & Lifecycle

```yaml
schema_version: 1.0.0
name: autonomous-feature-delivery
description: Orchestrates multi-agent TDD implementation and verification
entrypoint: delivery-pipeline
pipelines:
  - id: delivery-pipeline
    name: Full Feature Delivery
    stages:
      - id: stage-craftsman
        name: Code Implementation
        agent: worker-craftsman
      - id: stage-qa
        name: Continuous Verification Gate
        agent: worker-qa
```

---

## 4. Operational Invariants & Security Boundaries

1. **CAS Isolation**: Package manifests and stage definitions are ingested into content-addressable storage (CAS) before execution lock, preventing mid-run tampering.
2. **Fail-Closed Gate Checks**: Swarm execution fails closed if required worker seats lack valid Ed25519 authentication credentials or if the project root is unresolvable.
3. **Traceability Guarantee**: Every spawned agent task is immutably linked to a parent Priority Plan and Backlog Item (`backlog_item.id`), preserving total provenance across all code mutations.
