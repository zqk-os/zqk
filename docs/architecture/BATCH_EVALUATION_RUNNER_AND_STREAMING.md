# Multi-Scenario Batch Evaluation Runner & Real-Time Telemetry Streaming

## Overview

The Multi-Scenario Batch Evaluation Runner provides high-throughput concurrent evaluation execution across heterogeneous model endpoints and scenario definitions. Built for zero data-race concurrency and strict token budgeting, the runner bounds memory and thread growth via dedicated worker pools while publishing incremental progress events over server-sent event (SSE) channels.

## Architecture & Data Flow

```
                           +----------------------+
                           |     BatchConfig      |
                           |  (Scenarios, Pool,   |
                           |    Token Budget)     |
                           +----------+-----------+
                                      |
                                      v
                           +----------------------+
                           |     BatchRunner      |
                           +----------+-----------+
                                      |
               +----------------------+----------------------+
               |                      |                      |
               v                      v                      v
        +--------------+       +--------------+       +--------------+
        | Worker 1     |       | Worker 2     |       | Worker N     |
        | (Scenario A) |       | (Scenario B) |       | (Scenario K) |
        +-------+------+       +-------+------+       +-------+------+
                |                      |                      |
                +----------------------+----------------------+
                                       |
                     +-----------------+-----------------+
                     |                                   |
                     v                                   v
          +----------------------+            +----------------------+
          |   Token Aggregator   |            |   SSE Event Stream   |
          | (Atomic Telemetry)   |            |   (Real-time State)  |
          +----------------------+            +----------------------+
```

## Key Components

### 1. BatchRunner
The central coordinator that validates static configuration constraints, manages bounded worker pools, and aggregates scenario outcomes into a cohesive `BatchResult`.

### 2. Token Budgeting & Circuit Breaking
Every scenario execution reports prompt and completion tokens into an atomic accumulator. If the cumulative token consumption reaches or exceeds `MaxTokens`, the runner halts in-flight work and returns `ErrTokenBudgetExceeded`, preventing runaway cost and quota starvation.

### 3. Real-Time Telemetry Event Stream
Scenario lifecycles emit typed `StreamEvent` notifications over buffered channels:
- `batch_started`: Dispatched upon worker initialization.
- `scenario_progress`: Dispatched as each scenario starts execution.
- `scenario_completed`: Dispatched when an individual scenario passes evaluation.
- `scenario_failed`: Dispatched when a scenario encounters evaluation or model errors without halting peer workers.
- `batch_completed`: Emitted upon drain of all scenarios or budget exhaustion.

## Verification & Test Proofs

- **Static Floor Invariant**: Bounded concurrency must be positive; scenarios list non-empty; token budgets non-zero.
- **Operational Dynamic Proof**: Bounded concurrent execution verified under heavy parallel load; SSE stream event ordering guaranteed.
- **Negative Adversarial Boundary**: Pre-cancelled contexts and token budget exhaustion gracefully halt workers without deadlocks or resource leaks.
