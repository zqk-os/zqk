# Fail-Closed Upstream Synchronization Gates and Vendor-Neutral Anti-Idle Hooks

## Executive Summary

Autonomous agent progression in Continuous Autonomous Progression (CAP) loops depends on two critical architectural invariants:
1. **Upstream Base Synchronization (Gate 0b / POL-WORKFLOW-003):** Active integration and feature branches must never build, execute, or push from a stale git base behind `origin/main`. Operating on stale bases causes merge conflicts, breaks linear history, and triggers redundant, resource-intensive CI rebuilds.
2. **Vendor-Neutral Anti-Idle Continuation Loops (POL-AGENT-CAP-PROGRESSION):** Multi-turn agent swarms across heterogeneous environments (Antigravity CLI, Claude Code, Cursor, and generic hosts) require deterministic stop-hook and continuation interceptors that prevent premature idling and seamlessly ingest non-interrupting steering messages.

---

## 1. Upstream Base Synchronization Gate Architecture

### 1.1 The Stale Base Problem
When autonomous agents run multiple execution cycles in parallel or sequential batches, previous PR squash merges advance `origin/main`. If an agent begins Cycle $N+1$ from an integration branch or working tree that has not incorporated the latest squash commit:
- PR creation exhibits git merge conflicts.
- CI workflows fail or execute redundant runs against stale dependency baselines.
- Knowledge Kernel objects cross verification gates under diverged commit parentage.

### 1.2 Multi-Layered Enforcement Topology

Upstream base synchronization is enforced fail-closed across three distinct architectural planes:

```
┌────────────────────────────────────────────────────────────────────────┐
│                        Developer / Agent Turn                          │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│ Layer 1: Workflow Materialized View & Whats-Next Gland                 │
│ - Facade.CountBehindOriginMain() probes HEAD..origin/main              │
│ - EventStaleBaseBlocked preempts idle / execution turns                │
│ - Emits actionable rebase guiding step: git fetch && git rebase       │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│ Layer 2: Git Pre-Commit / Pre-Push Hooks (Gate 0b POL-WORKFLOW-003)    │
│ - Evaluates BEHIND_COUNT="$(git rev-list --count HEAD..origin/main)"   │
│ - Refuses push if BEHIND_COUNT > 0 unless ZQK_ALLOW_STALE_BASE=1       │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│ Layer 3: Kernel Validation Sentinel & Done-Gate Verification           │
│ - Validates non-merge commit hashes and mutation evidence              │
│ - Rejects promotion of backlog items with diverged commit roots        │
└────────────────────────────────────────────────────────────────────────┘
```

### 1.3 VCS Facade Methods
In `pkg/git/facade.go`, high-performance, context-bounded probes query commit offsets:
- `CountAheadUpstream() int`: Computes `rev-list --count @{upstream}..HEAD` to detect unpushed local changes.
- `CountBehindOriginMain() int`: Computes `rev-list --count HEAD..origin/main` to detect divergence from the primary branch.
- `CountBehindUpstream() int`: Computes `rev-list --count HEAD..@{upstream}` for tracked branch tracking.

---

## 2. Vendor-Neutral Anti-Idle Lifecycle Hooks

### 2.1 Continuation Hook Taxonomy
Different agent platforms intercept process stops and model completion through different JSON formats:

| Vendor | Hook Invocations | Return Schema | Stop Decision Behavior |
| :--- | :--- | :--- | :--- |
| **Antigravity CLI** | Stop hook / tool completion | `{"decision": "continue", "reason": "..."}` | Continues execution turn without prompting user |
| **Claude Code** | Stop interceptor / CLI subshell | `{"decision": "block", "reason": "..."}` | Blocks agent from exiting to terminal prompt |
| **Cursor** | Extension hook / background task | `{"decision": "continue", "message": "...", "reason": "..."}` | Re-prompts agent context with next directive |
| **Generic** | Stdin/stdout pipe | `{"decision": "continue", "reason": "..."}` | Standard POSIX stream interceptor |

### 2.2 Non-Interrupting Steering Feed Ingestion
Before emitting a continuation prompt, the hook queries the agent feed correspondence inbox:
1. Resolves active seat identity (`ZQK_AGENT_ID`, coordinator seat, or worker seat).
2. Drains pending unacked steering messages (`FeedEventTypeSteering`).
3. Formats concise steering summaries directly into the continuation prompt:
   ```
   CONTINUOUS AUTONOMOUS PROGRESSION (CAP LOOP): Autonomous progression trigger.
   Active plan in progress: Cycle 54 (PRI-xxx).
   Pending steering inbox (1 unacked message(s)):
   - [steering from human-tpm]: Focus on boundary verification and race safety.
   Query zqk workflow whats-next and advance without idling. Do not stop.
   ```
4. Provides continuous guidance without interrupting the human operator.

---

## 3. Layer 0 Backlog Item Integrity Blockers

To guarantee the **Hierarchical Chain of Integrity**, backlog items entering the CAS membrane must be anchored to strategic intent:
- Backlog items lacking both `goal_refs` and `requirement_refs` are elevated from Layer 1 advisory warnings to **Layer 0 fail-closed intake blockers** (`category: integrity`, `tier: 1`).
- `zqk system check` exits with code 2 whenever unanchored items are detected in non-exploring statuses.
- Draft-plane items (`status: exploring`) are permitted to be unlinked during initial brainstorming, but promotion to `planned` or `in_progress` is strictly gated.

---

## 4. Operational Invariants & Verification Checklist

- [x] `pkg/git/facade.go`: Context-safe `CountBehindOriginMain` and `CountBehindUpstream` implementation.
- [x] `pkg/interactionpolicy`: `EventStaleBaseBlocked` priority preemption in `ClassifyAmbience` and `catalog.go`.
- [x] `cmd/zqk/workflow`: `whatsNextOut` correctly surfaces stale base blockers and guiding rebase instructions.
- [x] `cmd/zqk/agent`: Multi-vendor support (`claude`, `cursor`, `antigravity`, `generic`) in `continuation_hook.go`.
- [x] Zero raw nanosecond identifiers in all public documentation.
