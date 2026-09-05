# Native Agent Execution Loop

**Last Verified:** 2026-08-31


**Status**: Active
**Created**: 2026-06-25
**Purpose**: Document the architecture for ZQK's native agent execution loop, removing the dependency on external CLI-IDE bridges.

## 1. Problem Statement

Historically, `zqk agent orchestrate` assembled highly contextual prompts (using `observer` AST and `builder` policies) and dispatched them via `agentdelivery.FileDeliverer` into `.zqk/inbox/coder-agent/`. This approach relies on an external IDE bridge (e.g., Antigravity) to passively watch the inbox and execute the prompt.

This creates a severe disconnect:
- **Context Loss**: Antigravity uses its own tool schemas and context pipelines, often ignoring ZQK-specific constraints (e.g., Neo4j graph context or Mesh capabilities).
- **Control Loss**: ZQK cannot maintain a tight execution loop (CAP loop) or guarantee deterministic compliance with internal project policies.
- **Dependency**: The ZQK swarm cannot scale horizontally without relying on an external IDE client for every worker.

## 2. Architectural Solution

We must build a **Native Agent Execution Loop** (AgentX) inside ZQK, fully utilizing `pkg/llm` and `pkg/swarm`.

### 2.1 Native LLM Delivery
Replace the `FileDeliverer` with a native queue or directly execute it using a new `LLMDeliverer`. The system will invoke `llm.Client.GenerateStructuredCompletion()` directly.

### 2.2 `zqk agent execute` Command
A new CLI command `cmd/zqk/agent/execute.go` will act as the native swarm worker.
- It consumes a generated prompt (or queue item ID).
- It feeds the prompt and tool schemas to the native Gemini LLM.
- It acts as the dispatcher, executing `llm.ToolCall` requests natively via `swarm.MCPExecutor`.
- It loops until the task is marked complete, then updates the `backlog_item` status.

## 3. Expert & Antagonist Perspectives

**Expert (Proponent):**
- Enables horizontal scaling of the swarm (deploying `zqk agent execute` on any VM without an IDE).
- Ensures 100% adherence to ZQK's graph constraints, RBAC, and telemetry standards.

**Antagonist (Critique):**
- **Risk:** Replicating an IDE's tool execution capabilities (file editing, bash execution) inside ZQK is complex and requires robust sandboxing.
- **Mitigation:** We are offloading tool execution to the existing `zqk-mcp` server via `pkg/swarm/mcp_client.go`, which is already isolated and proven. We do not need to rebuild the tools, only the orchestration loop that calls them.

## 4. Implementation Plan
1. Implement `cmd/zqk/agent/execute.go`.
2. Connect `agent execute` to the `llm` and `swarm` packages.
3. Deprecate `FileDeliverer` in `cmd/zqk/agent/orchestrate.go` and transition to the new native execution paths.
