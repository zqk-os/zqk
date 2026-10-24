# Agent Feed & Peer Mesh Subsystem Architecture Guide

## Overview

The `pkg/agentfeed` package delivers real-time peer-to-peer agent coordination, asynchronous correspondence, and cross-seat awareness for the ZQK microkernel. By coupling immutable JSONL event streams with POSIX process liveness tracking, deterministic protocol handshakes, and transport-agnostic wake adapters, the Agent Feed guarantees low-latency collaboration without relying on transient, centralized cloud brokers or unverified in-memory state.

```
+-----------------------------------------------------------------------------+
|                          ZQK Agent Feed Mesh Layer                          |
+-----------------------------------------------------------------------------+
                                       |
                   +-------------------+-------------------+
                   |                                       |
                   v                                       v
     +--------------------------+            +--------------------------+
     |   Agent Chat Channel     |            |    Peer Seats Registry   |
     | (.zqk/logs/ide-hooks/    |            | (.zqk/state/mesh/        |
     |  agent_chat_channel.jsonl|            |  peer_seats.json)        |
     +--------------------------+            +--------------------------+
                   |                                       |
                   +-------------------+-------------------+
                                       |
                                       v
                    +-------------------------------------+
                    |     Seat Worker & Protocol Loop     |
                    +-------------------------------------+
                     /                 |                 \
                    v                  v                  v
     +--------------------+  +--------------------+  +--------------------+
     | COMMS-CHECK Nonce  |  |  ORCHESTRATE_PLAN  |  | In-Flight Steering |
     | Deterministic Hand |  |  Directive & Duty  |  | Context Re-Prime   |
     +--------------------+  +--------------------+  +--------------------+
```

---

## Core Domain Models & Subsystem Taxonomy

### 1. Peer Seats & Liveness Tracking (`peer_seats.go`, `peer_seats_refresh.go`)

Every active agent in the mesh occupies a designated seat defined in `.zqk/state/mesh/peer_seats.json`. A seat models:
- **`PID`**: The local operating system process identifier executing the autonomous agent or harness.
- **`PersonaRef`**: The active role assigned from the Knowledge Kernel ontology (e.g. `PER-COMMUNITY-SOFTWARE-ENGINEER`).
- **`Duty`**: Operational role classification (`coordinator` vs. `worker`).
- **`Wake`**: Interruption transport membrane (`agentapi`, `mcp`, or `stamp`).

Liveness is continuously tracked via POSIX `signal 0` probes (`IsPIDAlive`). During swarm restarts or crash recovery, `RefreshPeerSeatsFromLivePIDs` sweeps dead PIDs and re-assigns active process handles to pending seat records in deterministic alphabetical order.

### 2. Event Ledger & Append Pipeline (`append.go`)

Inter-agent communication is committed to an append-only JSONL event ledger (`agent_chat_channel.jsonl`). Each event record guarantees:
- **`event_id`**: Universally unique event identifier (`AFE-<timestamp>-<hash>`).
- **`agent_id` / `persona_ref`**: Explicit actor provenance.
- **`session_id`**: Kernel convergence session binding.
- **`event_type`**: Semantic category (`chat`, `peer_ack`, `wake`, `steering`, `mesh_status`).
- **`in_reply_to`**: Causal linkage to parent events.

### 3. Peer Await & Asynchronous Wake Mesh (`await_peer_ack.go`, `wake_adapter.go`)

When an agent issues a task or handoff requiring peer confirmation:
1. An open await record is registered in `.zqk/state/mesh/peer_ack_awaits.json`.
2. When the destination seat commits a matching `peer_ack` stamped with `in_reply_to`, `CompletePeerAckAwaits` fires the registered `PeerWakeAdapter`.
3. Transport-agnostic wake adapters dispatch signals across:
   - **`agentapi`**: Native local CLI agent interrupt.
   - **`mcp`**: JSON-RPC `ActionRequired` event to IDE subscribers (Claude Desktop, Cursor, Zed).
   - **`stamp`**: Passive ledger entry for batch-polled agents.

---

## Verification & Three-Fold Proof Invariants

The `pkg/agentfeed` subsystem enforces the Three-Fold Proof across static, dynamic, and adversarial boundaries:

### 1. Static Floor Invariant
Statement coverage for `pkg/agentfeed` is strictly held above $\ge 85.0\%$ across all event mutations, seat assignments, and delivery modes. Zero discarded errors (`_ = ...`) and zero functions exceeding 80 lines are permitted by AST static analysis.

### 2. Operational Dynamic Proof
- **Deterministic Doorbell & COMMS Nonce Verification**: When a peer initiates a handshake (`COMMS-CHECK CC-<nonce>`), the seat worker automatically responds with formatted work tokens and inbox depth counters without starting unbounded AgentX executions.
- **Plan Super-Precedence**: When an `ORCHESTRATE_PLAN` directive arrives in an agent's inbox, stale unhandled work is superseded, clearing the inbox backlog and preventing priority plan starvation.
- **Atomic Heartbeat & Attempt Rate Limiting**: Worker heartbeats are persisted with millisecond UTC timestamps. Failed agent executions increment isolated per-event attempt counters, triggering circuit breakers before runaway looping occurs.

### 3. Negative Adversarial Boundary
- **Vendor Layout Leak Prevention (`vendor_path_policy.go`)**: Feed paths, lite configs, and project roots are continuously scanned against forbidden vendor markers (`.gemini/`, `.claude/`, `.windsurf/`, `.continue/`, `.aider/`). Any storage mutation targeting vendor directories is aborted with fail-closed errors.
- **Seat Identity Impersonation Protection (`peer_ack_auth.go`)**: A directed parent event addressed to a specific seat cannot be acknowledged or cleared by an alias or unauthorized peer seat (`ErrPeerAckSeatMismatch`).

---

## Command Reference & Operational Recipes

### Inspect Feed Health
```bash
zqk feed doctor
```
Verifies file existence, MCP subscriber connectivity, wake membrane script readability, and vendor path policy compliance.

### Refresh Seat Registry from Active OS Processes
```bash
zqk feed peer-seats refresh
```
Discovers running agent processes, prunes dead PIDs, and assigns new live process handles to idle seats.

### Acknowledge Peer Message with Provenance
```bash
zqk feed ack --event-id AFE-DEMO-001 --summary "Task accepted"
```
Records a verifiable `peer_ack`, resolves active await listeners, and completes the orchestration handoff loop.
