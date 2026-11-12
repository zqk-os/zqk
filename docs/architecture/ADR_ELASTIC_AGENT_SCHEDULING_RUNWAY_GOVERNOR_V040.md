# ADR-ELASTIC-AGENT-SCHEDULING-RUNWAY-GOVERNOR-V040: Elastic Agent Scheduling & Adaptive Runway Capacity Governor

## Status
Accepted

## Context
High-concurrency multi-agent swarms face variable execution loads. During heavy bursts (e.g. concurrent PR reviews, full integration passes, code refactorings), static worker pools either experience deep queuing latency or, if unconstrained, saturate system compute and exhaust LLM API token runway budgets.

Without an adaptive capacity governor, runaway swarms risk abrupt termination from API provider rate limits or disk/memory exhaustion. The swarm runtime requires an elastic scheduling and admission control governor that dynamically sizes active worker pools against backlog pressure while rigorously respecting token runway consumption budgets and shedding excess admission load when thresholds are breached.

## Decision
1. **Runway Capacity Policy & Specification**:
   - `RunwayCapacityPolicy`: Strongly-typed configuration defining minimum/maximum worker pool size, target queue length per worker, token consumption rate limits, and runway budget ceilings.
   - `RunwayGovernor`: Thread-safe controller monitoring queue depth and token budget in real time.
2. **Elastic Scaling & Dynamic Pool Sizing**:
   - Evaluates queue pressure: When `QueueDepth / ActiveWorkers > TargetTasksPerWorker` and token budget is positive, the governor increments worker capacity up to `MaxWorkers`.
   - Scale-down dampening: When queue depth drops, the governor scales down surplus workers after a stabilization cooldown window.
3. **Admission Control & Load Shedding**:
   - Token exhaustion boundary: If available token budget drops to zero or below the minimum reserve threshold, new task admissions are rejected with explicit backpressure errors (`ErrRunwayExhausted`).
   - Pool saturation boundary: When active workers reach `MaxWorkers` and queue exceeds capacity, tasks are rejected with `ErrAdmissionShed`.
4. **Declarative Hygiene & Verification**:
   - All string and ID validations adhere strictly to declarative helpers (`strutil.RequireNonBlank` / `strutil.ErrorOnBlank`).
   - Verified across three-fold proof invariants: static policy boundaries, operational elastic pool scaling, and runway exhaustion admission shedding.

## Consequences
- **Positive**:
  - Predictable execution costs and guaranteed token budget protection.
  - Elastic scale-out during peak demand and graceful scale-in during idle periods.
  - Zero-touch self-regulation without manual operator capacity tuning.
- **Negative**:
  - Tasks may be shed when token budget is completely exhausted, requiring retry orchestration.
