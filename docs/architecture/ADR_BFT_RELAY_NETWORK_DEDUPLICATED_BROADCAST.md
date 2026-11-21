# ADR: Byzantine Fault Tolerant Relay Network and Deduplicated Peer Broadcast

## Status
Accepted

## Date
2026-11-20

## Context & Motivation
Federated ZQK swarm nodes communicate across untrusted network boundaries and multi-hop mesh bridges where intermediate peers may behave arbitrarily—duplicating packets to amplify bandwidth consumption, mutating payloads in transit, forging peer hop signatures, or replaying stale broadcast envelopes. Without strict envelope validation, hop-by-hop cryptographic verification, and bounded deduplication, a single Byzantine relay can cause broadcast storms, state divergence, or acceptance of tampered messages.

In Cycle 159, we introduce a Byzantine Fault Tolerant (BFT) Relay Network and Deduplicated Peer Broadcast subsystem in `pkg/community/bft_relay.go` to enforce deterministic relay envelope invariants, peer hop signature chains, bounded multi-hop deduplication, and immediate dropping of malicious or replayed packets.

## Decision & Architecture
1. **Relay Envelope Static Floor Invariants**:
   - Every relayed broadcast packet (`RelayEnvelope`) must carry a non-empty message identifier, origin peer identifier, monotonically ordered sequence number, content digest (`SHA-256` over canonical payload bytes), bounded hop count (`0 <= HopCount <= MaxHops`), and a non-empty ordered slice of cryptographic peer hop signatures.
   - Configuration and envelope constructors validate all identifiers via non-blank checks and enforce positive capacity/hop bounds, failing closed on empty identifiers, nil payloads, missing signatures, or out-of-range hop counts.

2. **Cryptographic Peer Signature & Content-Hash Verification**:
   - Each relay node signs the canonical digest of `(MessageID, OriginPeerID, SequenceNumber, ContentHash, HopIndex, RelayPeerID)` using asymmetric peer keys (e.g., Ed25519) before forwarding.
   - Receiving relays recompute the payload's `SHA-256` content hash and verify every signature along the hop chain against registered peer public keys. Any mismatch between the recomputed hash and the declared `ContentHash`, or any invalid/unregistered signature, causes immediate packet rejection.

3. **Bounded Multi-Hop Deduplication Cache**:
   - Each relay maintains a thread-safe, bounded deduplication cache keyed by `(OriginPeerID, MessageID)` (and tracking highest observed sequence numbers per origin) with deterministic eviction when capacity is reached.
   - When a valid packet arrives that has already been seen within the deduplication window, the relay suppresses re-broadcasting and increments deduplication telemetry without forwarding downstream, preventing amplification loops across cyclic mesh topologies.

4. **Replay & Forged-Packet Defense**:
   - Packets carrying sequence numbers at or below the per-origin watermark (or replayed `(OriginPeerID, MessageID, SequenceNumber)` tuples) are dropped immediately before forwarding.
   - Tampered payloads (altered content hash), forged or truncated hop signatures, exceeded `MaxHops` limits, and unknown relay signers are rejected fail-closed and recorded as dropped malicious packets without mutating relay forwarding queues.

## Verification & Three-Fold Proof Strategy
All invariants are verified in `pkg/community/bft_relay_test.go` following the Three-Fold Proof model:
1. **Static Floor Invariant**: Assert that blank message/peer IDs, empty signature chains, negative or out-of-bounds hop counts, and invalid relay configurations fail closed at validation boundaries.
2. **Operational Dynamic Proof**: Simulate multi-hop peer topologies with legitimate intermediate relay signatures, confirming reliable packet verification, hop-chain extension, downstream forwarding, and duplicate broadcast suppression across redundant paths.
3. **Negative Adversarial Boundary**: Inject packets with forged peer signatures, altered payload content hashes, exceeded hop limits, and replayed sequence numbers, asserting immediate packet dropping and zero downstream forwarding.

## Consequences
- Prevents broadcast amplification storms across cyclic mesh bridges via bounded, thread-safe deduplication.
- Guarantees end-to-end payload integrity and hop-by-hop provenance across untrusted intermediate relays.
- Establishes fail-closed adversarial boundaries that drop forged, mutated, and replayed packets before they enter relay forwarding queues.
