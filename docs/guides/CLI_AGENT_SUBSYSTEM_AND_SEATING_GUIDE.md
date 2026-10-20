# CLI Agent Subsystem & Seating Guide

## Overview

The ZQK Agent Subsystem (`cmd/zqk/agent`) provides the primary orchestration, task delegation, execution lifecycle, and seating management commands for autonomous and paired agents operating within the Zen Quantum Kernel. 

Unlike transient LLM harness tools that lose context and lack persistent state, ZQK roots all agent coordination in the Knowledge Kernel. Agent instructions, task claims, orchestrator pipelines, worker seats, and execution traces are first-class, versioned graph objects persisted in Content Addressable Storage (CAS) under `.zqk/process/`.

The Agent Subsystem bridges human operators, coordinating Technical Program Managers (TPMs), and specialized worker agents (Software Craftsmen, Verification Engineers, Security Auditors) to achieve high-throughput, token-budgeted, and verifiable software engineering.

---

## Architectural Taxonomy & Core Components

```
cmd/zqk/agent/
├── agent.go                          # Root 'zqk agent' command builder and subcommand aggregation
├── orchestrate.go                    # 'agent orchestrate' pipeline coordination & subagent dispatch
├── orchestrate_run.go                # Multi-stage orchestrator execution engine & phase progression
├── orchestrate_mesh.go               # Agent mesh topology & bidirectional chat communication
├── orchestrate_steps.go              # Execution step generation & AST-driven instruction synthesis
├── orchestrate_reuse.go              # Pod caching & warm container/workspace reuse mechanics
├── seat_worker.go                    # 'agent seat-worker' daemon & slot assignment engine
├── seat_worker_engine.go             # Worker execution loop, step executor, and heartbeats
├── seat_worker_fill.go               # Dynamic seat slot allocation and capability matching
├── seat_worker_gate.go               # Seating precondition validation & credential checks
├── claim.go                          # 'agent claim' atomic task claiming & lease management
├── claim_gate.go                     # 'agent claim-gate' pre-claim invariant & conflict verification
├── release.go                        # 'agent release' atomic claim release & state rollback
├── recover.go                        # 'agent recover' stall recovery, worktree healing & task rescue
├── next.go                           # 'agent next' next actionable backlog item discovery
├── execute.go                        # 'agent execute' isolated step execution & artifact capture
├── prepare_context.go                # 'agent prepare-context' token-budgeted AST context preparation
├── prepared_context.go               # Context hydration, file symbol pruning & prompt assembly
├── synthesize_skill.go               # 'agent synthesize-skill' automated skill extraction from traces
├── sync_loop.go                      # 'agent sync-loop' reactive state synchronization loop
├── loop_guard.go                     # Anti-thrashing guard & debounce mechanism for sync loops
├── chat_responder.go                 # 'agent chat-responder' agent chat mailbox monitor
├── hook.go                           # 'agent hook' lifecycle hook execution & gate enforcement
├── continuation_hook.go              # Flywheel continuous autonomous loop progression hook
├── scoreboard.go                     # 'agent scoreboard' agent metrics, throughput & SLA reporting
└── validate.go                       # 'agent validate' agent configuration & environment validation
```

---

## 1. Agent Seating Subsystem & Worker Protocols

The Seating Subsystem governs agent identity, concurrency slots, capability matching, and safe multi-agent tenancy.

### 1.1 Seating Architecture

In ZQK, every active agent occupies a **Seat**. A seat pairs an cryptographic agent identity (Ed25519 keypair or system token) with specific capability tags, workspace isolation parameters, and assigned personas.

- **Coordinator Seat**: Operates at the Priority Plan / Epic level, decomposing requirements into atomic Backlog Items (BLIs), assigning tasks to worker seats, and driving the autonomous flywheel.
- **Worker Seat (`seat-worker`)**: Dedicated execution workers (e.g. `worker-craftsman`, `worker-qa`) that continuously monitor their assigned queue, claim ready tasks, execute TDD iterations in isolated git worktrees, and emit verifiable evidence.

### 1.2 The Seating Lifecycle & Daemon

The worker seat engine is launched via `zqk agent seat-worker`:

```bash
zqk agent seat-worker [flags]
```

Key configuration options include:
- `--seat-id`: Explicit unique seat identifier (e.g. `seat-craftsman-01`).
- `--capability`: Capability filters declared by the worker (e.g. `go-craftsman`, `qa-verification`).
- `--poll-interval`: Polling frequency for queue checks when idle.
- `--max-tasks`: Maximum number of tasks to process before graceful seat recycling.
- `--worktree-root`: Dedicated worktree directory for isolated branch checkouts.

When running, the seat worker:
1. Emits regular heartbeats into kernel storage to signal liveness.
2. Checks the `claim-gate` to guarantee no conflicting agent claims exist on target BLIs.
3. Automatically transitions the claimed task to `in_progress`.
4. Executes the instruction steps, runs continuous verification gates, and updates the task status to `complete` or `failed`.
5. Cleans up the worktree or marks the workspace for reuse.

---

## 2. Command Taxonomy & CLI Reference

### 2.1 Orchestration (`zqk agent orchestrate`)

Coordinates multi-agent execution pipelines for a Priority Plan or pipeline definition.

```bash
zqk agent orchestrate --plan-id <PRI-ID> [flags]
```

Supported flags:
- `--plan-id`: Target Priority Plan ID bounding the active cycle of work.
- `--pipeline`: Pipeline manifest identifier or file path.
- `--max-agents`: Concurrency threshold for parallel subagents.
- `--token-budget`: Maximum cumulative tokens across all subagent calls.
- `--dry-run`: Previews stage scheduling without spawning agent processes.

### 2.2 Atomic Claim & Gate (`zqk agent claim` & `claim-gate`)

Enforces the single-writer invariant across distributed agent swarms.

```bash
# Verify claim preconditions
zqk agent claim-gate --task-id <BLI-ID> --agent-id <AGENT-ID>

# Claim task atomically
zqk agent claim --task-id <BLI-ID> --agent-id <AGENT-ID> --lease-duration 30m

# Release claim
zqk agent release --task-id <BLI-ID> --agent-id <AGENT-ID> --reason "completed"
```

The claim gate enforces:
- Task is in an actionable state (`planned`, `blocked`, or expired lease).
- No unexpired lease is held by another active seat.
- Required dependencies and prerequisite criteria have passed.

### 2.3 Task Discovery & Progression (`zqk agent next`)

Discovers the next optimal task for an agent based on priority, dependencies, and capability tags.

```bash
zqk agent next --agent-id <AGENT-ID> --capability go-craftsman
```

### 2.4 Context Preparation (`zqk agent prepare-context`)

Extracts minimal, high-signal context from the Knowledge Kernel, Go AST, and documentation for prompt generation. This prevents context window flooding and maximizes LLM reasoning accuracy.

```bash
zqk agent prepare-context --task-id <BLI-ID> --max-tokens 4000 --format json
```

### 2.5 Reactive Sync Loop (`zqk agent sync-loop`)

Monitors kernel events, file changes, and agent task states, driving event-driven synchronization across distributed daemons.

```bash
zqk agent sync-loop --debounce-ms 250 --once
```

### 2.6 Skill Synthesis (`zqk agent synthesize-skill`)

Extracts successful problem-solving trajectories from completed agent tasks and formats them into reusable agent skill files (`SKILL.md`).

```bash
zqk agent synthesize-skill --task-id <TASK-ID> --output-dir .agent/skills/
```

### 2.7 Recovery & Healing (`zqk agent recover`)

Identifies orphaned tasks, abandoned leases, and corrupted worktrees, safely resetting or re-queuing them.

```bash
zqk agent recover --stale-threshold 1h --auto-rescue
```

---

## 3. Operational Invariants & Policies

1. **Anti-Idleness Protocol (POL-DEFAULT-ec4baffc4448367c)**:
   - Summary, test completion, or PR creation is a milestone transition, NOT a terminal stopping condition.
   - The coordinator agent must immediately query `zqk agent next` or `zqk workflow whats-next` and advance to the next task without going idle.
2. **Shift-Left Ontological Rigor**:
   - Every agent task must trace directly to an active `priority_plan` and `backlog_item`.
   - Work must never be performed without a backing kernel object and verified criteria (`criteria_refs`).
3. **Fail-Closed Lease Isolation**:
   - A worker seat whose heartbeat expires loses its claim lock immediately.
   - Any attempt to write artifacts without a valid lease fails closed with an authorization error.
4. **Token-Budgeted AST Context**:
   - Prompts must be assembled via `zqk agent prepare-context` using AST structural filtering rather than unbudgeted file dumps.

---

## 4. Verification and Done-Gate Matrix

| Criterion ID | Category | Description | Verification Method |
|--------------|----------|-------------|---------------------|
| `CRIT-1792429379773841000-8a2e2413` | Acceptance | Elevate `cmd/zqk/agent` test statement coverage to $\ge 85.0\%$ | `go test -short -cover ./cmd/zqk/agent/...` |
| `CRIT-1792429379773842000-ab3faebb` | Boundary | Validate edge cases, invalid flags, lease conflicts, worktree failures, and recovery paths | `cmd/zqk/agent/*_test.go` |
| `CRIT-1792429379773843000-2983fedc` | Compliance | Comprehensive Agent Subsystem & Seating Guide authored and verified | `docs/guides/CLI_AGENT_SUBSYSTEM_AND_SEATING_GUIDE.md` |
