# How-To: Agent Admin Membrane & Seating

Learn how the Agent Admin Membrane secures agent execution boundaries, governs tool permissions, and enforces work claim synchronization.

---

## 1. Agent Seating
ZQK assigns each cooperating agent an atomic seat identifier (e.g. `peer-agent-1`, `peer-agent-2`) bound to a persona specification (`PER-*`):

```bash
# Start an automated seat worker
zqk agent seat-worker --agent-id peer-agent-1 --persona-ref PER-ORCH-ALPHA --poll-seconds 30 --timeout 24h
```

---

## 2. Work Claim Protocol (Atomic Ownership)
To prevent duplicate execution and lock contention across the swarm:
1. When an agent selects a backlog item, it claims it in the kernel.
2. The claim records the agent's ID and timestamp.
3. Other agents respect the claim and pick alternative shovel-ready items from the lead Priority Plan.

---

## 3. Sandboxed Agent Execution
Subagent tasks are dispatched into isolated Git worktrees:
- Code mutations occur strictly inside the worktree branch.
- Only upon successful tool calls and zero gate failures is the worktree merged into the integration branch.
