# Universal Verification DSL

## Overview
As ZQK scales from task-level orchestration to full project governance, the verification mechanism must be abstracted from `agent_task` and applied universally. Whether transitioning a `backlog_item` to `implemented`, a `goal` to `achieved`, or an `agent_task` to `completed`, the system requires a standard Domain Specific Language (DSL) to enforce objective constraints.

This DSL allows a TPM or System Steward to explicitly define the gauntlet of checks an object must pass before its state can transition. 

## The DSL Schema
The DSL will be implemented as a YAML-based structural array that can be attached to any kernel object (via a `verification_gates` field) or bound directly to a lifecycle transition rule in `pkg/lifecycle`.

```yaml
verification_gates:
  - id: "V-001"
    name: "Architectural Integrity Check"
    target_transition: "in_progress -> implemented"
    strategy: "ast_semantic_match"
    config:
      target_paths: ["pkg/", "cmd/"]
      forbidden_patterns: ["fmt.Println", "os.Exit"]
      required_abstractions: ["logging.Fluent", "errfmt"]
    blocking: true

  - id: "V-002"
    name: "Targeted Build & Test Validation"
    target_transition: "in_progress -> implemented"
    strategy: "command_exit_code"
    config:
      command: "./scripts/agent-validate-changes.sh"
      timeout_seconds: 120
    blocking: true

  - id: "V-003"
    name: "Peer Review Coverage"
    target_transition: "proposed -> approved"
    strategy: "query_metric"
    config:
      query: "MATCH (o:object)-[:HAS_REVIEW]->(r:review) WHERE o.id = $this.id AND r.status = 'approved' RETURN count(r)"
      operator: ">="
      threshold: 1
    blocking: true
```

## Core Execution Engine
The `pkg/pipeline` will be upgraded to act as the central ingestion engine for this DSL. 

Instead of hardcoding logic inside `sync_loop.go`, the lifecycle engine will hook into the pipeline on every requested state transition:

1. **State Change Requested**: An agent or user attempts to update an object's status.
2. **DSL Ingestion**: The lifecycle engine intercepts the request, reads the object's `verification_gates` (and any global gates for that object `kind`), and compiles a dynamic `pkg/pipeline`.
3. **Execution**: The pipeline executes the `strategy` for each gate (e.g., parsing AST, running shell commands, querying the Neo4j graph).
4. **Enforcement**: If any `blocking: true` gate fails, the transaction rolls back, the state transition is rejected, and an `audit_event` is logged with the exact DSL failure output.

## Supported Strategy Plugins
To ensure the system remains extensible and "blind" to the implementation details, the DSL will support pluggable strategies:

- `command_exit_code`: Runs a sandboxed shell command and verifies a `0` exit code.
- `ast_semantic_match`: Uses Go AST parsing to enforce coding standards.
- `query_metric`: Executes a Cypher query against the graph to enforce relational constraints.
- `state_negation`: Ensures a specific boolean or state does *not* exist before proceeding.
- `cognitive_adversary`: Summons a specific Persona (e.g., the Antagonistic Checker) via MCP to perform a rigorous LLM-based audit, strictly gated by a rigid JSON schema response.

## Next Steps for Implementation
1. **Define Schema**: Introduce the `verification_gate` struct in `pkg/objects`.
2. **Refactor Lifecycle**: Intercept `sp.Update()` inside the storage provider or lifecycle engine to auto-trigger DSL ingestion on status changes.
3. **Migrate Hardcoded Hooks**: Remove the hardcoded `sync_loop` checks we just built and migrate them into this standardized DSL structure.
4. **Test Scenarios**: Write comprehensive `test-scenario-init` scripts to prove the DSL blocks rogue agents across all object kinds.
