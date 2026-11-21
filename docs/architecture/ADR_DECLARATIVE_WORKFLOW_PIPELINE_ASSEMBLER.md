# ADR-0153: Declarative Workflow Pipeline Self-Assembly and Execution Engine v0.4.3

## Status
Accepted (Milestone v0.4.3, Cycle 153)

## Date
2026-11-20

## Context
As autonomous agent swarms coordinate multi-step software engineering, verification, and knowledge synthesis tasks, hard-coded sequential execution loops fail to exploit independent parallelism and cannot adapt to dynamically declared dependency graphs. Without formal graph compilation, pipelines risk executing stages out of topological order, deadlocking on cyclic dependencies, referencing undeclared upstream stages, or exhausting token budgets under unbounded concurrent fan-out.

In Cycle 153, we introduce a declarative workflow pipeline self-assembly and execution engine (`WorkflowAssembler`) in `pkg/community/workflow_assembler.go` to parse stage specifications, validate schema and dependency invariants, compute deterministic topological execution layers, enforce token budget ceilings, and dispatch independent stages concurrently with fail-fast abort semantics.

## Decision

1. **Stage Schema & Dependency Floor Invariants (`pkg/community/workflow_assembler.go`)**:
   - Every declarative pipeline consists of a non-empty set of `WorkflowStage` definitions, each specifying a unique `ID`, an executable `Command` payload, a positive `Timeout` limit, an optional `TokenBudget`, and a list of upstream `DependsOn` stage identifiers.
   - Stage identifiers, command payloads, and dependency references are validated declaratively: blank stage IDs, duplicate stage IDs, blank command payloads, non-positive timeouts, negative token budgets, and self-dependencies are rejected at compilation time before any execution occurs.
   - Missing or undeclared `DependsOn` targets are rejected immediately during graph construction with descriptive compilation errors.

2. **Kahn's Deterministic Layered Topological Sort**:
   - The compiler builds a directed acyclic graph (DAG) with adjacency lists and in-degree counters for all declared stages.
   - Using Kahn's algorithm grouped by BFS wave/layer, all zero-in-degree stages in the current frontier are collected, sorted lexicographically by stage `ID` for deterministic ordering, and emitted as a single parallel `ExecutionLayer`.
   - Each layer's resolved stages decrement downstream in-degrees to form the subsequent frontier until all stages are scheduled.

3. **Cycle Detection & Missing-Dependency Rejection**:
   - If Kahn's layered traversal terminates with fewer scheduled stages than declared vertices (`scheduled < len(stages)`), a dependency cycle is present; compilation aborts immediately with a descriptive cyclic dependency error identifying the unresolvable subgraph.
   - Undeclared dependency references and self-loops are caught during adjacency construction and rejected prior to topological sorting.

4. **Token Budget Ceilings**:
   - Pipelines declare an aggregate `MaxTokenBudget` ceiling (alongside per-stage token estimates/budgets).
   - During compilation and dispatch, the assembler enforces that cumulative stage token allocations do not exceed the pipeline's configured ceiling, failing fast with a budget-exceeded error before dispatching over-budget workloads.

5. **Concurrent Layer Dispatch with Fail-Fast Abort**:
   - The execution engine iterates through the compiled `ExecutionLayer` slice sequentially (guaranteeing all upstream dependencies in layer `i` complete before layer `i+1` begins).
   - Within each layer, all independent stages are dispatched concurrently across autonomous worker goroutines bounded by a shared cancellable `context.Context` and per-stage `Timeout` enforcement.
   - On the first stage failure, timeout, or context cancellation within a layer, the layer context is cancelled immediately (fail-fast abort), halting sibling in-flight stages and preventing downstream layers from starting.

## Verification & Three-Fold Proof Strategy
All invariants are verified in `pkg/community/workflow_assembler_test.go` following the Three-Fold Proof model:

- **Static Floor Invariant (Workflow Stage Schema and Dependency Specifications)**:
  - Verifies that blank stage IDs, duplicate IDs, empty command payloads, non-positive timeout limits, negative token budgets, and malformed dependency entries are rejected deterministically at assembly time.
- **Operational Dynamic Proof (DAG Topological Ordering and Parallel Execution)**:
  - Verifies that multi-layer diamond and fan-out/fan-in DAGs compile into deterministic lexicographically ordered layers, execute independent stages concurrently within each layer without blocking, respect token budget ceilings, and preserve strict upstream-before-downstream ordering.
- **Negative Adversarial Boundary (Cyclic Dependency Detection, Missing Dependencies, and Fail-Fast Abort)**:
  - Verifies that 2-node cycles, multi-node cycles, self-dependencies, undeclared upstream stage references, over-budget pipelines, and mid-layer worker failures/timeouts abort cleanly with descriptive errors and cancel sibling/downstream execution.

## Consequences
- **Positive**:
  - Autonomous agent swarms can self-assemble complex execution pipelines declaratively without bespoke orchestration glue.
  - Deterministic layered topological sorting guarantees reproducible execution plans and safe parallel fan-out.
  - Fail-fast cancellation and upfront schema/cycle/budget validation eliminate wasted compute and partial inconsistent runs.
- **Negative**:
  - Barrier synchronization between topological layers means a slow straggler in layer `i` delays layer `i+1` even if some stages in layer `i+1` only depend on already-finished stages in layer `i`.
