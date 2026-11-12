# ADR: Agent Node Graceful Drain & Auto-Recovery Controller v0.3.7

## Status
Accepted

## Date
2026-11-12

## Context
In distributed multi-agent swarm environments, nodes periodically require rolling upgrades, configuration changes, capacity rebalancing, or temporary quarantine due to telemetry anomalies or resource exhaustion.
Abruptly terminating agent worker nodes risks corrupting active Knowledge Kernel transactions, interrupting in-flight LLM token streams, and stranding claimed Backlog Items (`BLI-*`).

In Cycle 135, we introduce a deterministic Agent Node Graceful Drain and Auto-Recovery Controller (`NodeDrainController`) within `pkg/community/node_drain.go`.

The controller must:
1. Model explicit deterministic node states: `NodeStatusActive`, `NodeStatusCordoned`, `NodeStatusDraining`, `NodeStatusDrained`, and `NodeStatusRecovered`.
2. Provide a cordon primitive that prevents new task dispatches to the target node while permitting existing active tasks to proceed.
3. Manage a graceful drain barrier with timeout enforcement, tracking in-flight task completions until the node reaches the fully drained state.
4. Provide an auto-recovery reconciliation mechanism that verifies node health invariants before smoothly transitioning a drained or cordoned node back to active service.
5. Adhere strictly to the AST hygiene mandate, utilizing declarative validation (`strutil.ErrorOnBlank`) and structured concurrency (`goroutinelabels`).

## Decision
1. Implement `NodeDrainController` and associated state types in `pkg/community/node_drain.go`.
2. Enforce declarative input validation (`NodeID`, `Reason`) using `strutil.ErrorOnBlank`.
3. Provide comprehensive test suite `pkg/community/node_drain_test.go` verifying the Three-Fold Proof:
   - Static Floor Invariant: Valid initial state transitions, default timeout configurations, state immutability safeguards.
   - Operational Dynamic Proof: Concurrent task execution, graceful draining with zero dropped tasks, task completion signaling, and auto-recovery back to active status.
   - Negative Adversarial Boundary: Rejection of dispatch to cordoned/draining nodes, timeout expiry handling for stuck tasks, invalid state transition rejections, and blank ID validations.
4. Register the architectural decision into the Knowledge Kernel CAS via `zqk new decision --cas` and `zqk docman register`.

## Consequences
- Guarantees zero dropped work and safe rolling upgrades across agent clusters.
- Provides predictable lifecycle hooks for orchestration engines and swarm fleet operators.
- Aligns node management with declarative, functional Go patterns and Knowledge Kernel policies.
