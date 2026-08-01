# Autonomous Capability Synthesis

## Objective
To unify Phase A (Cognitive Alignment & Policy Negotiation) and Phase B (Total Recall & Semantic Memory) into a cohesive operating framework for ZQK.

## Core Loop: The Synthesis Engine
The "Synthesis Engine" is an autonomous loop that runs inside a `convergence_session`. When a Goal or Priority Plan is dispatched, the Engine:
1. **Recalls Context (Phase B):** Queries the Graph-Vector Hybrid Store (`pkg/hivemind`) using the user's intent to find structurally and semantically related objects (previous patterns, analogous policies, relevant source code).
2. **Plans & Delegates:** The Orchestrator routes tasks to specialized sub-agents based on the retrieved context.
3. **Negotiates Constraints (Phase A):** If an agent proposes a change that violates a policy retrieved from the Hive Mind, the system invokes the `Truth Sentinel` and `Mesh Broker`. The agent must either negotiate an override or escalate to a Human-in-the-Loom (HITL).
4. **Commits (WAL):** Successful operations are committed to the Lifecycle WAL.

## Execution Flow
1. User Command: `zqk agent orchestrate "Optimize the scheduler"`
2. Semantic Retrieval: The Hive Mind looks up `"Optimize the scheduler"` and retrieves embeddings for `SCH-022`, `scheduler_job.yaml`, and historical bottleneck reports.
3. Policy Overlay: The system applies active policies.
4. Execution: Agents execute changes, which are immediately verified by the `Truth Sentinel`.
5. Feedback: Errors are fed back into the `convergence_session` loop for autonomous retry.

## Future Evolution
- **Sovereign Capacity:** If the local kernel lacks the capability (e.g., an advanced code auditor), it issues a `capacity_advertisement` to the Mesh and leases the compute from a neighboring ZQK instance via `zqk_session`.