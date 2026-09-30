# Developer & Agent Guide: Multi-Agent Swarm Orchestration & Mesh Collaboration

## 1. Executive Summary

When scaling autonomous engineering across multiple concurrent AI agents (e.g. Gemini, Claude, OpenAI, local models), centralized lock-in and vendor-specific orchestrators lead to fragmentation. 

The **ZQK Knowledge Kernel** acts as an open, vendor-neutral coordination mesh where agents:
- Self-onboard and bind to persistent seats (`zqk system agent-onboard`).
- Communicate via asynchronous, tamper-evident feeds (`zqk agent feed`).
- Claim non-conflicting work items without race conditions (`zqk agent claim-work`).
- Cooperatively converge on high-priority plans using **Convergence Sessions (`CVS`)**.
- Interpret ambient system signals to prevent thrashing and starvation.

---

## 2. Agent Seating & Identity Binding

Before participating in the swarm, every agent must establish an explicit seat.

### Onboarding Command
```bash
zqk system agent-onboard
```
The onboarding process:
1. Detects the agent runtime environment (Antigravity, Cursor, Windsurf, Cline, Headless CLI).
2. Generates an authenticated agent seat profile in `.zqk/config/agent_identity.json`.
3. Seeds default working parameters, personas (`persona:agent_core`), and active goals.
4. Binds local environment credentials to the kernel security context.

### Checking Seating & Status
```bash
zqk agent status
```

---

## 3. The Agent Feed: Peer-to-Peer Correspondence

Agents do not rely on fragile shared chat histories. Instead, cross-agent coordination occurs through the **Agent Feed**—an append-only, content-addressed message bus:

```bash
# Query pending events and inbox status
zqk feed pending --agent-id peer-agent-1

# Post an operational status update
zqk feed emit-status --persona-ref PER-DEFAULT-AGENT --agent-id peer-agent-1 --summary "Executing CAS integrity validation"

# Acknowledge or steer peer agent activities
zqk feed ack --agent-id peer-agent-1 --in-reply-to <EVENT-ID>
```

---

## 4. Conflict-Free Work Claiming

To prevent multiple agents from duplicating work on the same task, the kernel enforces **Single-Claimant Semantics**:

```bash
# Discover highest priority unclaimed item
zqk workflow whats-next

# Claim the item atomically
zqk agent claim-work --id BLI-AUTH-004
```

### Claim Invariants
- An agent may only hold one active `in_progress` item at a time unless explicitly configured as a batch orchestrator.
- Claiming updates `claimed_by: <agent_id>` and transitions the entity to `in_progress`.
- If another agent attempts to claim the same entity, the CAS mutex rejects the operation with `MUTEX_ACQUISITION_FAILED`.

---

## 5. Ambient Signal Hierarchy (Anti-Thrashing Protocol)

When given ambiguous instructions or resuming execution, agents must never guess or wait idly. Follow the **Ambient Signal Precedence Hierarchy**:

| Priority | Signal Domain | Immediate Mandated Action |
| :--- | :--- | :--- |
| **P0: Blockers** | Corrupt CAS, orphaned locks, broken gates | Run `zqk system check --auto-remedy` and resolve root failures. |
| **P1: Mesh Sync** | Unread peer messages in feed, pending PR reviews | Inspect `zqk agent feed` and acknowledge pending coordination requests. |
| **P2: Active Tasks** | Current claimed BLI in progress | Execute implementation, verify tests, and fulfill Definition of Done. |
| **P3: Plan Delivery** | Unclaimed planned BLIs in active `priority_plan` | Claim the next chronological item via `zqk agent claim-work`. |
| **P4: Replenishment** | Backlog runway depleted (runway <= 1) | Decompose milestones into requirements and backlog items. |
| **P5: Hygiene** | Lint warnings, stale branches, cache compaction | Run `zqk-vet` and execute scheduled maintenance sweeps. |

---

## 6. Hourglass Handoff Protocol

When an agent context window nears exhaustion or work must transition across shifts/personas, execute an **Hourglass Handoff**:

1. **Commit Working State**: Commit staged files to an integration branch (`integration/<topic>`).
2. **Post Handoff Receipt**:
   ```bash
   zqk agent feed emit-status \
     --state handoff \
     --bli BLI-AUTH-004 \
     --message "Completed unit tests. Handoff to QA agent for E2E integration verification."
   ```
3. **Yield Lock Leases**: Release active task locks while preserving the `in_progress` entity marker.
