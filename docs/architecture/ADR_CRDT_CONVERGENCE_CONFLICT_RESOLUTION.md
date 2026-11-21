# ADR: Adaptive Conflict Resolution & Deterministic CRDT State Convergence Engine

## Status
Accepted

## Date
2026-11-21

## Context & Motivation
In distributed multi-agent swarm deployments, autonomous nodes operate across asynchronous network links where partitions, message reordering, and concurrent mutations are routine. Relying on synchronous distributed locks or single-leader consensus for every state update introduces latency bottlenecks and availability failures during cluster splits.

To guarantee eventual consistency without central coordination, cross-cluster state synchronization requires Conflict-Free Replicated Data Types (CRDTs) whose merge semantics are:
1. **Commutative:** `Merge(A, B) == Merge(B, A)` regardless of gossip delivery order.
2. **Associative:** `Merge(Merge(A, B), C) == Merge(A, Merge(B, C))` across arbitrary multi-hop replication topologies.
3. **Idempotent:** `Merge(A, A) == A` so duplicate anti-entropy delta deliveries are safe no-ops.

Without deterministic conflict resolution and strict boundary validation, concurrent writes with identical timestamps or malformed anti-entropy payloads can cause permanent state divergence across replicas.

## Decision & Architecture
We implement a deterministic CRDT convergence engine in `pkg/community/crdt_convergence.go` built around thread-safe composite documents, Last-Write-Wins (LWW) registers, LWW-Element-Sets, and validated incremental deltas.

### 1. CRDT Document & LWW Register Schema
- **`LWWRegister`**: Stores a scalar `Value` (`string`), monotonic `Timestamp` (`int64`), and `WriterID` (`string`). Constructed via `NewLWWRegister`, which enforces non-blank `WriterID` and strictly positive `Timestamp`.
- **`ElementRecord` & `LWWElementSet`**: Maintains separate `addSet` and `removeSet` maps keyed by element identifier, recording `(Timestamp, WriterID)` per operation.
- **`CRDTDocument`**: Thread-safe (`sync.RWMutex`) composite container holding a `id` (`DocumentID`), named `LWWRegister` map, an `LWWElementSet`, and a high-water `updatedAt` timestamp.
- **`CRDTDelta`**: Lightweight incremental payload (`DocumentID`, `Registers`, `Added`, `Removed`, `Timestamp`, `SenderID`) for low-bandwidth anti-entropy gossip.

### 2. Deterministic Tie-Breaking
All conflict resolution is total and deterministic across replicas:
- **Register Merge (`LWWRegister.Merge`)**: A candidate register wins if `other.Timestamp > r.Timestamp`, or if timestamps are equal and `other.WriterID > r.WriterID` (lexicographical tie-break).
- **Element Set Merge & Membership (`LWWElementSet.Merge`, `Contains`)**: Both `addSet` and `removeSet` independently retain the record with the highest `(Timestamp, WriterID)` pair. An element is active (`Contains == true`) when present in `addSet` and either absent from `removeSet` or `addRec.Timestamp > remRec.Timestamp`, with an add-wins tie-break on `addRec.Timestamp == remRec.Timestamp && addRec.WriterID >= remRec.WriterID`.

### 3. Commutative Delta Merge & State Convergence
- **`CRDTDocument.Merge(other *CRDTDocument)`**: Snapshots the remote document under `RLock` before acquiring the local write lock to avoid lock-ordering deadlocks, merges all registers and element sets commutatively, and advances `updatedAt` to the maximum observed timestamp.
- **`CRDTDocument.ApplyDelta(delta *CRDTDelta)`**: Applies validated incremental register and set updates atomically under the document write lock, enabling bandwidth-efficient anti-entropy convergence.

### 4. Malformed Delta & Boundary Rejection
All entry points enforce fail-closed validation before mutating state:
- **Blank Identifiers (`ErrBlankID`)**: Enforced via `strutil.ErrorOnBlank` for document IDs, register keys, element names, `WriterID`, and `SenderID`.
- **Non-Positive Timestamps (`ErrInvalidTimestamp`)**: Any timestamp `<= 0` on registers, elements, or deltas (including nested `Registers`, `Added`, and `Removed` entries in `CRDTDelta.Validate`) is rejected.
- **Nil & Mismatched Payloads (`ErrNilDelta`, `ErrCorruptPayload`)**: `ApplyDelta(nil)` returns `ErrNilDelta`; applying a delta or merging a document whose `DocumentID` differs from the target document returns `ErrCorruptPayload`.

## Verification & Three-Fold Proof Strategy
Verification is implemented in `pkg/community/crdt_convergence_test.go` following the Three-Fold Proof pattern:
1. **Static Floor Invariant (`TestCRDT_StaticFloorInvariant`)**: Verifies `NewCRDTDocument` initialization, `SetRegister`/`GetRegister` round-trip fidelity, and `AddElement`/`HasElement` baseline behavior.
2. **Operational Dynamic Proof (`TestCRDT_OperationalDynamicProof`)**: Simulates divergent replicas (`docA`, `docB`) with concurrent register writes, element additions, and tombstone removals; verifies `Merge(A, B)` and `Merge(B, A)` converge to identical register values and sorted element sets, and validates incremental `ApplyDelta` synchronization.
3. **Negative Adversarial Boundary (`TestCRDT_NegativeAdversarialBoundary`)**: Verifies rejection of blank document IDs and register keys (`ErrBlankID`), negative timestamps (`ErrInvalidTimestamp`), nil deltas (`ErrNilDelta`), mismatched delta document IDs (`ErrCorruptPayload`), and foreign document merges (`ErrCorruptPayload`).

## Consequences
- **Deterministic Convergence**: All replicas converge to identical state regardless of message ordering, duplication, or partial network partitions.
- **Deadlock-Free Merging**: Snapshot-then-merge locking in `CRDTDocument.Merge` prevents ABBA lock inversion during concurrent bidirectional anti-entropy sync.
- **Fail-Closed Hygiene**: Corrupt deltas, blank identifiers, and non-positive timestamps are rejected prior to state mutation, preserving cluster state integrity.
