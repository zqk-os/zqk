# Mesh Seat Worker & Supervisor Daemon (`pkg/seatworker`)

`pkg/seatworker` provides OS supervisor installation and lifecycle management for persistent autonomous agent worker processes (`zqk agent seat-worker`).

---

## 1. What is a Seat Worker?

A **Seat Worker** is a dedicated, headless autonomous process bound to a specific seated persona defined in `.zqk/agent-runtime/peer_seats.json` (or `peer_seats.json`).

While interactive agents operate within an IDE or single CLI session, a Seat Worker runs continuously in the background under the host OS supervisor:
- **macOS**: `launchd` User Agent (`~/Library/LaunchAgents/com.zqk.mesh.seat-worker.<seat>.plist`)
- **Linux**: `systemd` User Service (`~/.config/systemd/user/com.zqk.mesh.seat-worker.<seat>.service`)

The worker daemon periodically polls its seat's inbox (`poll_seconds`, default 30s), dequeues steers and task assignments (`ATK-*`), prepares kernel context, executes the cognitive reasoning and action loop, and returns status updates to the Knowledge Kernel.

---

## 2. When Should You Use Them?

1. **Continuous 24/7 Autonomous Loops**: When you want agents to autonomously make progress on backlog items, perform continuous verification, or monitor repository health without requiring an open IDE terminal.
2. **Specialized Agent Swarms**: When dedicating specific hardware or processes to specialized roles (e.g., dedicated TPM orchestrator, Code Craftsman, QA Verification, or Security Auditor).
3. **Sovereign Mesh Compute Clusters**: In multi-node federated mesh environments where remote machines advertise compute capacity and execute delegated workloads.

---

## 3. Installation & CLI Usage

Install seat worker daemons natively using the CLI:

```bash
# Install supervisor units for all agentapi/mcp seats in peer_seats.json
zqk agent install-seat-workers

# Install a specific seat worker with a 15-second polling interval
zqk agent install-seat-workers --seat craftsman --poll-seconds 15

# Enable autonomous execution of code/shell mutations
zqk agent install-seat-workers --seat craftsman --execute-non-comms

# Dry-run / test without registering with launchctl/systemctl
zqk agent install-seat-workers --write-only
```

To run a seat worker directly in the foreground for debugging:
```bash
zqk agent seat-worker --agent-id craftsman --persona-ref community-code-craftsman --poll-seconds 10
```

---

## 4. Architecture & Supervision Flow

```
  ┌─────────────────────────────────────────────────────────────┐
  │ Host Supervisor (launchd / systemd)                         │
  │ com.zqk.mesh.seat-worker.<seat_id>                          │
  └───────────────┬─────────────────────────────────────────────┘
                  │ Spawns & Keeps Alive
                  ▼
  ┌─────────────────────────────────────────────────────────────┐
  │ zqk agent seat-worker                                       │
  │   - Agent ID: <seat_id>                                     │
  │   - Persona:  <persona_ref>                                 │
  │   - Interval: <poll_seconds>                                │
  └───────┬───────────────────────────────▲─────────────────────┘
          │ Polls Inbox / Steer           │ Emits Status / ACKs
          ▼                               │
  ┌────────────────────────┐    ┌─────────┴─────────────┐
  │ Agent Feed (Inbox)     │    │ Knowledge Kernel      │
  │ .zqk/agent-runtime/    │    │ (Tasks, Criteria, CAS)│
  └────────────────────────┘    └───────────────────────┘
```

---

## 5. Limitations & Operational Considerations

1. **Root Kernel Binding**: Seat workers must be installed on the canonical project root containing the `.zqk/` kernel directory. Installing on isolated git worktrees is explicitly rejected (`cannot install seat workers on worktree root`).
2. **Execution Permissions (`--execute-non-comms`)**:
   - By default, seat workers only ingest correspondence and claim work.
   - When `--execute-non-comms` is enabled, the worker executes live bash commands, file modifications, and git operations. Ensure proper git credentials and branch protections are configured.
3. **Cognitive Timeout & Retries**:
   - `loadAttempts` (3 attempts): Caps retries per task to prevent infinite loops on failing tasks.
4. **Log Inspection**:
   Supervisor stdout and stderr streams are isolated per seat under:
   `.zqk/logs/mesh/seat-worker-<seat_id>.stdout.log`
   `.zqk/logs/mesh/seat-worker-<seat_id>.stderr.log`
