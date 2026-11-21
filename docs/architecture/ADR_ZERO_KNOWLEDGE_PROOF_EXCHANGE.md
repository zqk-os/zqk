# ADR: Zero-Knowledge Verification & Cross-Tenant Proof Exchange

## Status
Accepted

## Date
2026-11-20

## Context
Federated ZQK deployments require tenant boundaries to verify compliance, artifact integrity, and state invariants across untrusted or semi-trusted domains without disclosing internal payload secrets, proprietary AST structures, or raw Knowledge Kernel objects.

While `pkg/community/zk_attestation.go` provides single-cluster RSA accumulator commitments and membership witnesses, cross-tenant federation requires an explicit zero-knowledge proof exchange layer in `pkg/community/zk_proof_exchange.go`. This subsystem must guarantee mathematical non-disclosure of underlying secrets, deterministic succinct proof generation over AST artifacts, constant-time cross-tenant verification, and fail-closed defense against proof tampering and cross-domain replay attacks.

## Decision

### 1. Commitment Schema & Non-Disclosure Invariants
- **Statement & Commitment Envelope**:
  - Cross-tenant proofs are encapsulated in a structured proof exchange envelope binding a public `Statement` (verifier domain, issuer tenant identifier, artifact digest, epoch, and nonce) to a cryptographic `Commitment` and succinct `Proof`.
  - Commitments are derived deterministically via domain-separated SHA-256 hashing over length-prefixed canonical encodings of the secret witness, blinding factor, and public statement context.
- **Non-Disclosure Invariants**:
  - Secret witnesses, raw AST payloads, and blinding factors are never serialized into the exchangeable proof envelope.
  - Structural validation rejects any envelope containing blank identifiers, zeroed commitments, or unblinded payload fragments before cryptographic verification begins.

### 2. Succinct Proof Generation & Constant-Time Verification
- **Deterministic Proof Generation over AST Artifacts**:
  - Provers generate succinct proofs deterministically from canonical AST artifact digests and private witness commitments, enabling reproducible verification without transmitting the underlying AST nodes.
  - Proof size remains constant (`O(1)`) relative to the size of the attested AST or kernel subgraph.
- **Constant-Time Cross-Tenant Verification**:
  - Cross-tenant verifiers evaluate proof commitments and challenge responses using constant-time byte comparisons (`crypto/subtle.ConstantTimeCompare`) to eliminate timing side channels across tenant boundaries.
  - Verifiers operate solely on public statement metadata, tenant public parameters, and the succinct proof envelope, requiring zero shared secret state with the proving tenant.

### 3. Adversarial Replay & Cross-Domain Tamper Defense
- **Domain Separation & Anti-Replay Binding**:
  - Every proof is cryptographically bound to its `IssuerTenant`, `TargetDomain`, `Epoch`, and single-use `Nonce` via explicit domain-separation tags.
  - Verifiers maintain bounded anti-replay state tracking seen nonces per `(IssuerTenant, TargetDomain, Epoch)` tuple, rejecting duplicate submissions fail-closed.
- **Fail-Closed Tamper Rejection**:
  - Any mutation to the proof bytes, statement fields, target domain, or commitment digest invalidates the Fiat-Shamir challenge binding and is rejected immediately.
  - Cross-domain replay attempts (re-submitting a valid proof from Tenant A/Domain X to Tenant B/Domain Y) fail domain-binding verification prior to acceptance.

## Consequences
- **Privacy-Preserving Federation**: Tenants can exchange verifiable proofs of AST and state compliance without exposing proprietary payloads or internal kernel data.
- **Side-Channel Resistance**: Constant-time comparison in the verification path prevents timing oracles from leaking commitment or witness properties across tenant boundaries.
- **Replay & Tamper Resilience**: Strict domain separation, nonce tracking, and fail-closed validation ensure counterfeit, falsified, or replayed proofs are deterministically rejected.
