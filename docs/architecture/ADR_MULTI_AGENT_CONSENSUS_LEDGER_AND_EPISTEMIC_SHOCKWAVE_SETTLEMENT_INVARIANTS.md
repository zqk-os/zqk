# ADR: Multi-Agent Consensus Ledger & Epistemic Shockwave Settlement Invariants

## Context & Problem Statement
In distributed agent execution systems where multiple autonomous subagents, workers, and background daemons (scheduler, ambient, steward, overseer) operate concurrently across distinct processes and sandboxes, epistemic drift is an inherent hazard. Divergent agent views, out-of-order execution, and state modifications across local checkouts risk creating uncoordinated forks or corrupting execution state.

To guarantee deterministic, verifiable convergence across swarms, ZQK requires a multi-agent consensus ledger coupled with an epistemic shockwave settlement protocol.

## Decision Drivers
- **Anti-Idleness Protocol**: Swarm agents must reach agreement asynchronously without polling or idling.
- **Fail-Closed Verification**: Any proposed state mutation failing cryptographic hash integrity, epoch monotonicity, or quorum threshold must be deterministically rejected.
- **Epistemic Shockwave Synchronization**: When consensus is reached, a shockwave event must propagate through the agent reflex layer to align downstream subscribers and trigger atomic state transition.
- **Zero Raw Nanos-Hex Studio IDs**: All architectural documents and references adhere strictly to canonical namespace conventions and clean documentation policies.

## Architectural Design

### 1. Cryptographically Chained Consensus Ledger (`pkg/consensus`)
- Each `LedgerEntry` defines `Index`, `PreviousHash`, `Hash` (SHA-256 over canonical fields), `ProposedBy`, `Epoch`, `Timestamp`, and `Payload`.
- Genesis entry anchors the chain at `Index: 0` with `PreviousHash: ""` and verified payload.
- Subsequent entries require `PreviousHash == chain[len-1].Hash` and strictly monotonic epoch and index ordering.
- Entries transition through states: `proposed` -> `committed` (or `rejected`).

### 2. Quorum Settlement & Epistemic Shockwave Propagation
- The `SettlementEngine` aggregates peer voting attestations for proposed entries.
- Reaching consensus quorum (e.g. $\ge 2/3$ or required participant threshold) transitions the entry to `committed`.
- Once committed, an `EpistemicShockwave` event is emitted across the ambient reflex fabric (`pkg/ambient/consensus_reflex`), notifying subscribers and updating local state deterministically.

### 3. Boundary & Fork Resistance
- Conflicting proposals for the same height or epoch trigger fork detection.
- Invalid parent hashes or unverified signatures fail closed with explicit invariant violation errors.
- Network partitions or quorum shortfalls result in deterministic rejection without corrupting prior committed history.

## Three-Fold Proof Matrix
- **Static Floor Invariant**: Complete structural validation of `LedgerEntry` fields, positive monotonic index, valid timestamps, non-empty hashes, and canonical state enums.
- **Operational Dynamic Proof**: Valid proposal submission, quorum collection across simulated peer nodes, atomic ledger commit, and shockwave propagation across reflex handlers.
- **Negative Adversarial Boundary**: Rejection of forged parent hashes, replay attacks, stale epoch indices, sub-quorum proposals, and concurrent fork conflict arbitration.

## Traceability
- **Milestone**: Cycle 88 (Multi-Agent Consensus Ledger & Epistemic Shockwave Settlement Invariants)
- **Priority Plan**: Cycle 88 Execution Plan
- **Requirement**: Multi-Agent Consensus Ledger & Epistemic Shockwave Settlement Invariants
- **Criteria**:
  - Acceptance: Consensus ledger replication & state propagation
  - Boundary: Fork detection, partition tolerance & epistemic shockwave conflict resolution
  - Documentation: Architecture ADR and specification entry
- **Test Suites**: `pkg/consensus`, `pkg/ambient` (`TestConsensusReflex_.*`)
