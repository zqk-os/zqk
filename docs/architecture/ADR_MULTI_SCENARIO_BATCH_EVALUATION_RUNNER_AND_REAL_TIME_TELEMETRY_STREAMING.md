# ADR: Multi-Scenario Batch Evaluation Runner & Real-Time Telemetry Streaming

## Context & Problem Statement
In evaluating LLM agents and multi-scenario workflows, sequential evaluation creates severe latency bottlenecks and fails to simulate concurrent operational load. Furthermore, real-time observability is necessary during execution to track token consumption against strict budgets, observe per-scenario progress, and intercept runaway executions before financial or computational limits are breached.

To deliver responsive, token-budgeted, concurrent evaluation, ZQK introduces the Multi-Scenario Batch Evaluation Runner with real-time Server-Sent Events (SSE) telemetry streaming.

## Decision Drivers
- **Concurrent Batch Execution**: Scenarios must execute concurrently under strict bounded concurrency controls to maximize throughput without exceeding host or provider rate limits.
- **Real-Time Telemetry Streaming**: Execution milestones, scenario lifecycle transitions, and live token usage metrics must be streamed via decoupled event channels (supporting SSE and CLI pubsub adapters).
- **Strict Token Budget Enforcement**: Token consumption must be atomically aggregated and evaluated against hard ceilings; exceeding budget invariants must halt further scenario dispatch fail-closed.
- **Isolated Failure Containment**: Failures in individual scenarios must not crash the batch runner or corrupt metrics of parallel evaluations.
- **Zero Raw Nanos-Hex Studio IDs**: Canonical documentation and specifications must maintain strict documentation cleanliness standards.

## Architectural Design

### 1. Concurrent Batch Engine (`pkg/eval/batch_runner.go`)
- The `BatchRunner` accepts a `BatchConfig` specifying:
  - `Scenarios`: Slice of scenario descriptors defining ID, model, input token profile, and execution parameters.
  - `Concurrency`: Worker pool cap enforcing max parallel executions.
  - `MaxTokens`: Upper bound on cumulative token usage across the batch.
  - `Timeout`: Wall-clock timeout for the complete batch run.
- Worker dispatch is managed via bounded concurrency channels and coordinated through context cancellation.

### 2. Thread-Safe Token Aggregation & Metrics (`BatchMetrics`)
- Uses atomic integers (`atomic.AddInt64`, `atomic.LoadInt64`) to aggregate:
  - Prompt tokens
  - Completion tokens
  - Cached tokens
  - Scenario pass/fail counters
- Guarantees zero race conditions across high-throughput parallel scenario evaluations.

### 3. Real-Time Telemetry Event Streaming (`StreamEvent`)
- Emits structured events over an unbuffered/buffered Go channel:
  - `EventBatchStarted`: Dispatched upon batch initialization.
  - `EventScenarioProgress`: Emitted during scenario execution with incremental token and payload data.
  - `EventScenarioCompleted`: Emitted upon successful scenario completion.
  - `EventScenarioFailed`: Emitted upon scenario execution error, containing error details without failing the remaining batch.
  - `EventBatchCompleted`: Emitted upon batch termination with final aggregated metrics.

### 4. Adversarial & Boundary Containment
- Statically validates configurations before execution:
  - Empty scenario slices return `ErrZeroScenarios`.
  - Non-positive concurrency returns `ErrInvalidConcurrency`.
  - Non-positive token budget returns `ErrInvalidTokenBudget`.
  - Missing scenario IDs return `ErrInvalidConfig`.
- Context timeouts or token budget breaches gracefully cancel pending scenarios while preserving outcomes from already-completed executions.

## Three-Fold Proof Matrix
- **Static Floor Invariant**: Complete structural and input parameter validation for scenario definitions, concurrency bounds, and token limits.
- **Operational Dynamic Proof**: Bounded concurrent execution across multi-scenario workloads, atomic token aggregation, and non-blocking event stream consumption.
- **Negative Adversarial Boundary**: Token budget exhaustion halting scenario dispatch, context cancellation containment, and isolated scenario failure resiliency.

## Traceability
- **Milestone**: Cycle 89 (Multi-Scenario Batch Evaluation Runner & Real-Time Telemetry Streaming)
- **Priority Plan**: Cycle 89 Execution Plan
- **Requirement**: Multi-Scenario Batch Evaluation Runner and Real-Time Telemetry Streaming
- **Criteria**:
  - Acceptance: Multi-Scenario Batch Evaluation Runner Functional Acceptance
  - Boundary: Multi-Scenario Batch Evaluation Runner Boundary & Error Handling
  - Documentation: Multi-Scenario Batch Evaluation Runner Documentation & Knowledge Base Entry
- **Test Suites**: `pkg/eval` (`TestCycle68_BatchRunner_.*`)
