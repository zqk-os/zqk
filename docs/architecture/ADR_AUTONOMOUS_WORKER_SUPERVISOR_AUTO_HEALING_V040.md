# ADR-AUTONOMOUS-WORKER-SUPERVISOR-AUTO-HEALING-V040: Autonomous Worker Health Supervisor & Process Auto-Healing Controller

## Status
Accepted

## Context
In decentralized, long-running multi-agent swarms, individual worker processes or subagent seats may crash, experience memory pressure, dead-lock on external I/O, or encounter silent network partition stalls. Without supervisor oversight, dead or hanging workers deplete swarm execution runway and stall continuous autonomous progression (CAP).

Manual daemon recycling by human operators contradicts zero-touch autonomous governance. The swarm runtime requires a self-healing process supervision engine that continuously verifies worker heartbeat vitality, calculates jittered exponential restart backoff intervals, and isolates unrecoverable crash-looping workers behind circuit breakers.

## Decision
1. **Worker Specification & Supervision Architecture**:
   - `WorkerProcessSpec`: Strongly-typed configuration encompassing worker identifier, execution binary, target seat, maximum crash limit, and heartbeat timeout threshold.
   - `WorkerSupervisor`: Dedicated supervisor controller monitoring active worker nodes via a concurrent registry.
2. **Exponential Backoff & Self-Healing Loop**:
   - When a worker fails to emit a heartbeat within its configured grace period, the supervisor transitions the worker to degraded status and initiates restart scheduling.
   - Restart backoff scales exponentially ($T_{backoff} = T_{base} \times 2^{retry}$) up to a hard ceiling ($T_{max}$), preventing CPU thrashing during transient infrastructure outages.
3. **Crash-Loop Detection & Circuit-Breaker Quarantine**:
   - If a worker crashes repeatedly within an active epoch exceeding `MaxCrashLimit`, the supervisor trips the circuit breaker and marks the worker as `Quarantined`.
   - Quarantined workers are removed from active seat dispatch, preventing cascading spawn storms and protecting system runway.
4. **Declarative Hygiene & Verification**:
   - String validations use declarative helpers (`strutil.RequireNonBlank` / `strutil.ErrorOnBlank`).
   - Verified across three-fold proof invariants: static process configuration, dynamic failure auto-recovery, and crash-loop circuit breaker quarantine.

## Consequences
- **Positive**:
  - Autonomous recovery of transient worker failures without human intervention.
  - Provable protection against runaway fork-bombs and crash storms via circuit breaker quarantine.
  - Zero external supervisor dependencies; implemented entirely in pure Go.
- **Negative**:
  - Requires workers to periodically emit heartbeats via IPC or kernel channels.
