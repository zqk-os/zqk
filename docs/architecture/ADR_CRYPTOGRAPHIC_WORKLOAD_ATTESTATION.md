# ADR: Cryptographic Attestation & Workload Integrity Proofs

## Status
Accepted

## Date
2026-11-20

## Context
In distributed ZQK execution meshes, orchestrators dispatch workloads to remote worker nodes that may be untrusted, compromised, or subject to network replay and payload tampering. Without verifiable cryptographic proof of execution, an orchestrator cannot distinguish authentic workload completion from forged receipts, stale replayed responses, or altered execution outputs.

In Cycle 165, we introduce the Cryptographic Attestation & Workload Integrity Proofs subsystem in `pkg/community/workload_attestation.go` to provide deterministic, fail-closed verification of distributed workload execution through nonce-chained challenges, host identity key bindings, SHA-256 workload digesting, and automated node quarantine.

## Decision
1. **Monotonic Challenge Nonce Chaining & Host Identity Key Bindings**:
   - Each attestation challenge binds a target `nodeID`, a unique `challengeID`, a monotonically increasing sequence number (`NonceSeq`), and a cryptographically random nonce chained to the prior verified challenge digest for that host.
   - Worker nodes register an asymmetric host identity public key (e.g., Ed25519 / ECDSA) bound to their `nodeID` in the attestation verifier registry. Unregistered or revoked hosts fail closed before any receipt verification proceeds.
   - Challenge issuance and verification enforce strict monotonic sequence progression per host so out-of-order or stale challenge nonces cannot be reused.

2. **SHA-256 Workload Digesting & Cryptographic Receipt Signature Verification**:
   - Workload inputs, execution parameters, and outputs are canonicalized into a deterministic SHA-256 workload digest (`sha256` over domain-separated canonical fields: `challengeID`, `nodeID`, `nonceSeq`, `prevChallengeDigest`, and `workloadPayloadDigest`).
   - The executing host signs the canonical attestation receipt using its host identity private key.
   - The verification engine recomputes the expected SHA-256 workload digest, validates the chained nonce and sequence state, and verifies the host signature against the bound public key.

3. **Replay Attack Fencing, Tampered Digest Rejection & Malicious Node Quarantine**:
   - **Replay Attack Fencing**: Every consumed challenge nonce and sequence number is recorded in a thread-safe replay fence. Any duplicate submission of a previously verified `challengeID` or `NonceSeq <= lastVerifiedSeq` is rejected immediately with a replay-detected sentinel error.
   - **Tampered Digest Rejection**: Any mismatch between the recomputed SHA-256 workload digest and the signed receipt digest, or any invalid cryptographic signature, is rejected fail-closed.
   - **Malicious Node Quarantine**: Repeated or critical attestation failures (such as signature forgery, tampered workload digests, or replay attempts) transition the offending `nodeID` into a quarantined state in the verifier registry, blocking subsequent challenge issuance and workload dispatch until explicit operator or governance remediation.

## Consequences
- Establishes end-to-end cryptographic provenance for distributed workload execution without trusting transport-level assertions alone.
- Prevents replay attacks, cross-host receipt substitution, and silent payload tampering via monotonic nonce chaining and SHA-256 digest binding.
- Isolates compromised or Byzantine worker nodes automatically through deterministic quarantine enforcement.
