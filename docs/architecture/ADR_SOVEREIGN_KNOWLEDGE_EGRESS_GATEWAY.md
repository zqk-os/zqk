# ADR: Sovereign Knowledge Boundary and Cryptographic Egress Gateway

## Status
Accepted

## Date
2026-11-20

## Context
Distributed knowledge networks and multi-cluster agent swarms routinely exchange state, telemetry, and artifacts across trust boundaries. Without an explicit sovereign boundary and policy-enforced egress layer, cross-cluster transfers risk unintended data exfiltration, leakage of private credentials, and unauthorized cross-domain propagation of restricted knowledge objects.

To establish a verifiable security perimeter at cross-cluster boundaries, we introduce the Sovereign Knowledge Boundary and Cryptographic Egress Gateway (`pkg/community/egress_gateway.go`). The gateway enforces sovereign domain isolation, automated pre-transit credential redaction, cryptographic egress clearance token verification, and fail-closed interception with security telemetry alerting.

## Decision
1. **Sovereignty Classification Tags & Domain Policy Schema**:
   - Every outbound payload and domain policy must declare explicit sovereignty metadata: a non-empty classification label (such as `public`, `internal`, `confidential`, or `restricted`), well-formed transit rules, and a valid destination domain identifier.
   - Domain policies define allowed source-to-destination domain pairs, permitted classification ceilings, and required redaction profiles.
   - Schema validation enforces strict static floor invariants: empty, whitespace-only, or malformed classification labels, transit rules, or destination domain identifiers are rejected prior to transit evaluation.

2. **Pre-Transit Credential Redaction**:
   - Before any authorized payload leaves a sovereign domain, the egress gateway executes deterministic pattern-based and structural credential scrubbing.
   - Private credentials—including bearer tokens, API keys, private key blocks, and secret assignments—are automatically redacted and replaced with canonical redaction markers so secrets never traverse cross-cluster links.

3. **Cryptographic Egress Clearance Tokens**:
   - Authorized cross-domain transfers require a valid cryptographic egress clearance token bound to the payload digest, source domain, destination domain, classification label, and expiration window.
   - Clearance tokens are signed and verified using keyed HMAC-SHA256 authentication with constant-time signature comparison, ensuring tamper resistance and non-repudiable policy authorization.

4. **Fail-Closed Restricted Egress Interception & Intrusion Alerting**:
   - The gateway operates strictly fail-closed: any outbound payload flagged as `restricted` (or exceeding the destination domain's classification ceiling), lacking a valid clearance token, carrying an expired or tampered signature, or targeting an unapproved domain is immediately intercepted and blocked.
   - Every blocked transfer emits a structured security telemetry intrusion alert capturing the source domain, destination domain, classification label, rejection reason, and timestamp for auditability.

## Verification & Three-Fold Proof Strategy
Verification is anchored in `pkg/community/egress_gateway_test.go` using the Three-Fold Proof pattern:
1. **Static Floor Invariant**: Validates that sovereignty policy schemas, classification labels, transit rules, and destination domain identifiers are strictly non-empty and well-formed, rejecting malformed configurations.
2. **Operational Dynamic Proof**: Demonstrates end-to-end pre-transit credential redaction on outbound payloads and successful HMAC-SHA256 egress clearance token signing and verification for authorized cross-domain transfers.
3. **Negative Adversarial Boundary**: Confirms fail-closed interception of `restricted` payloads, missing clearance tokens, and forged or expired tokens, verifying that transfers are blocked and security telemetry intrusion alerts are emitted.

## Consequences
- Establishes a deterministic, fail-closed cryptographic boundary preventing unauthorized knowledge exfiltration across sovereign domains.
- Eliminates accidental credential leakage in cross-cluster payloads via mandatory pre-transit redaction.
- Provides auditable security telemetry on all policy violations and attempted boundary crossings.
