# ADR: Zero-Knowledge State Attestation & Succinct RSA Accumulator Membership Proofs v0.4.2

## Status
Accepted

## Date
2026-11-18

## Context
Distributed ZQK swarm nodes must periodically attest to their Knowledge Kernel and execution state without transmitting full state snapshots or revealing private payload contents to untrusted peers. Communicating full element sets across federated clusters scales linearly with state size and leaks sensitive metadata.

In Cycle 150, we introduce a cryptographic RSA Accumulator and Zero-Knowledge State Attestation subsystem in `pkg/community/zk_attestation.go` to support constant-size (`O(1)`) state commitments, deterministic prime representative mapping, succinct membership witnesses, non-membership proofs, and anti-replay state attestations.

## Decision
1. **Cryptographic Parameter Floor Invariants**:
   - `NewRSAAccumulator(cfg AccumulatorConfig)` enforces strict static floor invariants:
     - RSA modulus `N` must be non-nil, positive, odd, and at least `MinModulusBitsFloor` (512 bits minimum for fast deterministic testing; configurable upward via `MinModulusBits`).
     - Generator `g` must satisfy `1 < g < N` and `gcd(g, N) = 1`.
     - Prime representative bit width `PrimeBits` defaults to `DefaultPrimeBits` (128 bits) and is bounded within `[MinPrimeBitsFloor, MaxPrimeBitsCeiling]` (`[64, 256]`).
   - All invalid parameterizations fail closed with `ErrInvalidAccumulatorConfig`.
2. **Deterministic Prime Representative Mapping (`HashToPrime`)**:
   - Element strings are validated via `strutil.ErrorOnBlank` (rejecting empty or whitespace-only inputs with `ErrInvalidElement`).
   - Elements are deterministically mapped to odd probable primes using domain-separated SHA-256 (`zqk:rsa-accumulator:prime:v1:`) with length-prefixed encoding and a monotonic counter until `ProbablyPrime(20)` succeeds.
3. **Thread-Safe State & Digest**:
   - `RSAAccumulator` protects mutable accumulator state with `sync.RWMutex` and exposes defensive copies (`Modulus()`, `Generator()`, `Current()`) and a deterministic `StateDigest() string`.
4. **Proof & Attestation Contracts**:
   - Define `MembershipWitness`, `NonMembershipProof`, and `StateAttestation` alongside sentinel errors (`ErrInvalidAccumulatorConfig`, `ErrInvalidElement`, `ErrInvalidWitness`, `ErrReplayDetected`).

## Consequences
- Enables `O(1)` state commitment digests and succinct membership verification across federated nodes.
- Guarantees deterministic, cross-platform prime representative mapping without external state or random non-determinism.
- Establishes fail-closed cryptographic parameter validation before any witness generation or attestation exchange occurs.
