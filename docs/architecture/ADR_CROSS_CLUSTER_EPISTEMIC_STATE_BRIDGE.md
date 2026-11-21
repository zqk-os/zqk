# ADR: Cross-Cluster Epistemic State Bridge & Protocol Handshake

## Status
Accepted

## Date
2026-11-20

## Context
Federated ZQK clusters operating across sovereign network boundaries must exchange Knowledge Kernel state deltas without trusting unauthenticated intermediaries or exposing unencrypted state payloads. Ad-hoc peer connections risk downgrade attacks, out-of-order state application, replayed handshake transcripts, and rogue cluster impersonation.

In Cycle 158, we introduce the Cross-Cluster Epistemic State Bridge & Protocol Handshake subsystem in `pkg/community/epistemic_bridge.go` to enforce fail-closed mutual TLS (mTLS) session establishment, monotonic delta synchronization, and adversarial boundary fencing across federated clusters.

## Decision
1. **Mutual TLS Certificate Manifest & Cipher-Suite Floor Invariants**:
   - Every bridge endpoint configuration validates a structured TLS certificate manifest (`ClusterID`, non-empty cluster endpoint URI, leaf certificate, private key material, and trusted peer CA bundle) before any network socket or session state is initialized.
   - Transport security enforces a strict TLS 1.3 minimum protocol version and an explicit AEAD cipher-suite floor (e.g., `TLS_AES_256_GCM_SHA384`, `TLS_CHACHA20_POLY1305_SHA256`), rejecting legacy TLS versions, weak ciphers, or blank endpoints with a fail-closed validation error.
2. **Bidirectional Epistemic State Delta Streaming & Monotonic Sequence Verification**:
   - Once mutual handshake negotiation completes, peer clusters establish a bidirectional session that exchanges signed epistemic state deltas (`StateDelta`) tagged with source cluster identity, causal state digest, and a strictly increasing per-session sequence number.
   - Receiving bridges enforce monotonic sequence verification: deltas with sequence numbers less than or equal to the high-water mark are rejected, and out-of-order gaps are fenced rather than silently applied, preserving causal consistency across clusters.
3. **Adversarial Replay Fence, Spoofed Cluster Fencing & Bounded Clock-Skew Isolation**:
   - Handshake packets and session tokens carry unique cryptographic nonces and issuance timestamps. Replayed nonces or previously consumed handshake transcripts are tracked in a bounded replay fence and rejected immediately.
   - Peer certificate Subject/SAN identities must match the advertised `ClusterID` in the handshake payload; mismatched or rogue cluster advertisements are fenced and terminated at the protocol boundary.
   - Handshake and delta timestamps are validated against a bounded clock-skew window (e.g., `MaxClockSkew`), isolating expired session tokens and peers whose clocks drift beyond the tolerated threshold.

## Consequences
- Establishes a cryptographically authenticated, downgrade-resistant mTLS bridge for cross-cluster Knowledge Kernel synchronization.
- Guarantees causal, non-duplicative delta application via strict monotonic sequence tracking.
- Protects federated meshes against replay attacks, spoofed cluster advertisements, and stale-clock token reuse through fail-closed boundary checks.
