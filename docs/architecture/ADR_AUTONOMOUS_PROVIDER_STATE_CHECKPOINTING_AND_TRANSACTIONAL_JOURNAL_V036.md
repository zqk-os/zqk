# ADR-0050: Autonomous Provider State Checkpointing & Transactional Journal v0.3.6

## Status
Accepted

## Context
In distributed multi-agent operations within the ZQK Knowledge Kernel runtime, external tool providers, storage backends, and capability adapters frequently execute multi-step mutations (such as batch object updates, remote state synchronizations, or credential rotations). If a failure, process eviction, or network disruption occurs mid-operation, the runtime can be left in an inconsistent, partially-mutated state.

To guarantee atomicity and strict zero-idle self-healing, the ecosystem requires an Autonomous Provider State Checkpointing & Transactional Journal. The coordinator records uncommitted operations into an in-memory or persisted transactional journal with write-ahead logging (WAL), supporting two-phase commit (Prepare / Commit) and automated rollback handlers upon unexpected failure.

## Decision
1. **Transactional Journal Structure**:
   - Implement `TransactionCoordinator` managing provider transaction journals.
   - Define `JournalEntry` capturing transaction ID, phase (`prepared`, `committed`, `rolled_back`), provider identity, snapshot payloads, and compensation/rollback callbacks.
2. **Atomic Rollback Protocol**:
   - In the event of an unhandled panic, provider failure, or context cancellation during a multi-step operation, the coordinator automatically executes registered compensation routines in reverse order (LIFO).
3. **Managed Concurrency & Telemetry**:
   - Conform to `POL-CONCURRENCY` using `goroutinelabels` and safe synchronization primitives with zero naked goroutines.
   - Expose checkpoint metrics and transaction health telemetry for system-level audits.

## Consequences
- **Positive**:
  - Eliminates partial mutation corruptions across provider plugins.
  - Ensures atomic rollback and predictable state consistency during swarm execution.
- **Negative / Trade-offs**:
  - Memory overhead for staging uncommitted transaction snapshots; mitigated by bounded history windows and transaction cleanup timeouts.
