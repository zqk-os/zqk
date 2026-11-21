# ADR: Planetary Merkle DAG State Synchronization and Differential Range Proofs

## Status
Accepted

## Date
2026-11-20

## Context
Federated Knowledge Kernel clusters operating across planetary regions must continuously converge distributed state trees despite network partitions, concurrent regional writes, and potentially Byzantine peers. Shipping full-state snapshots across wide-area links is bandwidth-prohibitive, while naive delta replication risks missing causal ancestors, silent state divergence, and accepting forged branches from compromised clusters.

In Cycle 158, we establish the Planetary Merkle DAG State Synchronization and Differential Range Proofs architecture to enforce fail-closed node and range-proof schema invariants, causal ancestor traversal, deterministic branch re-rooting, and immediate Byzantine branch quarantine.

## Decision
1. **Static Floor Invariants for DAG Nodes, Range Proofs, and Region Identities**:
   - Every Merkle DAG node must carry a valid, non-blank cryptographic node hash, causal parent references (empty only for the genesis root), and a valid, non-blank originating region identifier.
   - Differential range proofs must declare valid, non-blank range boundary hashes, a non-empty cryptographic proof path, and an expected Merkle root digest. Any node or range proof failing these static floor invariants is rejected fail-closed before traversal or state mutation.
2. **Causal Ancestor Traversal & Differential Range Proof Verification**:
   - During multi-region synchronization, the synchronizer traverses peer DAGs from the advertised remote head back to the lowest common ancestor shared with the local DAG, ensuring no causal ancestors are skipped or orphaned.
   - Peers exchange compact differential range proofs spanning the divergent interval rather than full subtrees. The receiver recomputes the Merkle root across the supplied range boundaries and sibling hashes, accepting the delta only when the computed digest matches the verified root.
3. **Deterministic Branch Re-Rooting**:
   - When concurrent regional updates create divergent valid branches, the synchronizer resolves divergence using a deterministic branch re-rooting protocol.
   - Canonical branch ordering is determined solely by deterministic structural metrics (such as causal depth/chain weight with lexicographical root hash tie-breaking), ensuring all honest clusters converge to the identical canonical Merkle root regardless of message arrival order or network topology.
4. **Byzantine Branch Quarantine & Corrupt Branch Isolation**:
   - Any branch presenting an invalid cryptographic range proof, a tampered Merkle root, or broken causal ancestry is immediately rejected and isolated in a Byzantine quarantine registry.
   - Quarantined branches and their unverified descendants are fenced from local state application, re-rooting, and outbound peer gossip, preventing Byzantine state poisoning across the planetary mesh.

## Consequences
- Enables bandwidth-efficient, causally complete state convergence across multi-region planetary clusters using differential range proofs.
- Guarantees deterministic, order-independent branch re-rooting across honest clusters.
- Protects local and federated state integrity by rejecting and quarantining tampered or Byzantine branches at the verification boundary.
