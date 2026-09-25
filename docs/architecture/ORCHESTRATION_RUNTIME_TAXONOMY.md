# Orchestration, Swarm, and Agent Execution Runtime Taxonomy

This document clarifies the architectural boundaries, ownership responsibilities, and routing paths across orchestration packages in the ZQK codebase, resolving finding `F-ARCH-007`.

## Core Orchestration Responsibilities

| Subsystem / Package | Canonical Responsibility | Key Entrypoints & Types | In-Process vs Subprocess |
| :--- | :--- | :--- | :--- |
| **`pkg/primaryorch`** | Host/Project Primary Orchestrator resolution, waking, and IPC binding dispatch. | `primaryorch.WakePrimary`, `primaryorch.LoadBinding`, `primaryorch.Binding` | In-process file/IPC signal |
| **`pkg/orchestration`** | In-process agent session management, intent synthesis, subagent runners, and isolated worktree lifecycle. | `orchestration.Manager`, `orchestration.SubagentRunner`, `orchestration.WorktreePool` | In-process Go routines & worktrees |
| **`pkg/swarm`** | Distributed multi-agent task execution, swarm worker pooling, task claiming, and persona dispatch. | `swarm.Dispatcher`, `swarm.Executor`, `swarm.WorkerPool` | In-process concurrency with task boundaries |
| **`pkg/scheduler`** | Periodic cron execution, background maintenance jobs, and convergence session monitoring. | `scheduler.Scheduler`, `scheduler.StartConvergenceEngine` | In-process scheduler daemon |

## Legacy & Deprecated Packages

- **`pkg/agentorch`**: Deprecated skeletal stub retained strictly for legacy compile compatibility. Callers must migrate to `pkg/orchestration.Manager` or `pkg/swarm.Dispatcher`.
- **`pkg/agentclaim` / `pkg/agentdelivery` / `pkg/agentfeed`**: Domain-specific message routing wrappers designed to bridge agent feed communication to the kernel storage layer.
