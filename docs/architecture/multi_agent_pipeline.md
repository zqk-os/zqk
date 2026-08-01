# Multi-agent Orchestration Pipeline Architecture

## Traceability
- **Priority Plan:** PRI-MULTI-AGENT-ORCHESTRATOR (Pending ID from CLI generation)
- **Backlog Item:** BLI-EXAMPLE
- **Status:** Grooming / Design Phase

## 1. Executive Summary
The Multi-agent Orchestration Pipeline provides a dynamic task-graph executor for the ZQK operating system. It enables complex workflows to be decomposed into discrete nodes that can be parallelized and executed seamlessly across a pool of specialized AI subagents, significantly reducing end-to-end latency and improving determinism.

## 2. Dynamic Task-Graph Executor

The core of the pipeline is a Directed Acyclic Graph (DAG) executor that evaluates dependencies at runtime.

### 2.1 Graph Formulation
- **Nodes (Tasks):** Each node represents an isolated unit of work (e.g., code generation, code review, test execution).
- **Edges (Dependencies):** Edges define strict input/output contracts. A node cannot begin execution until all inbound dependencies resolve successfully.
- **Dynamic Resolution:** The graph can mutate at runtime. If a subagent identifies a missing sub-task, it can inject new nodes into the DAG dynamically, and the executor will resolve the updated dependencies without restarting the pipeline.

### 2.2 Execution Semantics
- **Parallelization Engine:** The executor continuously evaluates the DAG for "ready" nodes (nodes with zero pending dependencies).
- **Subagent Pooling:** Ready nodes are dispatched to an available pool of subagents via MCP. Subagents are selected based on role definitions and capability matching (e.g., routing a "UI task" to a UI-specialized subagent).
- **Seamless Handoff:** Results from a node are stored in the Content Addressable Storage (CAS) and their hashes are passed to downstream nodes. This avoids passing massive context payloads, keeping context windows lean.

## 3. Subagent Coordination

To orchestrate multiple subagents concurrently without deadlock or context collision:

1. **State Isolation:** Subagents do not share memory or interactive shell sessions. All state mutations happen via standardized ZQK Process Objects.
2. **Convergence Sessions:** Each subagent operates within an isolated `convergence_session` that tracks its specific progress against the graph node.
3. **Tool Pod Mesh Integration:** Integrates with the Tool Pod Mesh (PRI-17807948) for automated retries. If a subagent fails a task, the node is retried with a Sentinel supervisor injecting correction feedback.

## 4. Operational Flow

1. **Elicitation:** The user or the top-level Agent triggers a high-level goal.
2. **Decomposition:** A Planner subagent breaks the goal into a DAG of actionable tasks.
3. **Dispatch:** The executor evaluates the DAG and dispatches independent tasks to parallel workers.
4. **Aggregation:** As branches of the DAG complete, reducer nodes summarize the outcomes.
5. **Finalization:** The root node is marked complete and the user is presented with the final artifact or prompt.

## 5. Next Steps
- Implement the DAG evaluation loop in `pkg/pipeline`.
- Define the Subagent MCP context-passing protocol.
- Extend `zqk` CLI to visualize the live orchestration graph (`zqk pipeline status`).
