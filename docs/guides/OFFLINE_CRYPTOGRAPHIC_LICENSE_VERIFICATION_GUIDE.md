# Offline Cryptographic License Verification, Ed25519 Signatures & Feature Entitlements Architecture Guide

## Overview

The `pkg/license` package implements cryptographically secure, offline license verification for enterprise feature gating across ZQK clusters, relays, and autonomous swarms. It ensures that features (such as relay routing, multi-cluster mesh federation, and high-concurrency worker pools) are authenticated without network egress, telephone-home dependencies, or centralized verification latency.

## Architectural Placement & Security Model

```
┌─────────────────────────────────────────────────────────────┐
│                       ZQK License JWT                       │
│  - Algorithm: EdDSA (Ed25519)                               │
│  - Claims: Features ([]string), RegisteredClaims (Expiry)   │
└──────────────────────────────┬──────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────┐
│                 pkg/license.Validator                       │
│  - Hardcoded / Embedded Ed25519 Public Key                  │
│  - Offline Signature Verification                           │
│  - Algorithm Downgrade Attack Prevention (Strict EdDSA)     │
│  - Expiration Invalidation (ErrExpiredToken)                │
└──────────────────────────────┬──────────────────────────────┘
                               │
            ┌──────────────────┴──────────────────┐
            ▼                                     ▼
┌─────────────────────────────┐     ┌─────────────────────────────┐
│        HasFeature()         │     │      RequireFeature()       │
│  - Non-blocking check       │     │  - Fail-closed enforcement  │
│  - Feature availability     │     │  - ErrMissingFeature return │
└─────────────────────────────┘     └─────────────────────────────┘
```

## Core Architectural Invariants

1. **Strict Algorithm Enforcement (Anti-Downgrade):**
   The verification pipeline explicitly checks `token.Method.(*jwt.SigningMethodEd25519)` and configures `jwt.WithValidMethods([]string{"EdDSA"})`. Symmetric key confusion attacks (e.g., signing with HMAC using the public key as the secret) are completely blocked.

2. **Offline Hermetic Operation:**
   Verification requires zero network connectivity. The Ed25519 public key is supplied during validator construction, ensuring that disconnected air-gapped environments function seamlessly.

3. **Deterministic Error Taxonomy:**
   - `ErrInvalidToken`: Returned when signature verification fails, tokens are malformed, or cryptographic assertions are violated.
   - `ErrExpiredToken`: Returned when the `exp` claim indicates expiration.
   - `ErrMissingFeature`: Returned by `RequireFeature` when the license is structurally valid but lacks the requested capability tag.

4. **Fail-Closed Entitlement Checks:**
   Any parsing failure, key mismatch, or expired timestamp immediately aborts feature access, maintaining a strict fail-closed posture across all enterprise surfaces.

## Verification & Testing Standards

- **Coverage Floor:** Statement coverage must be strictly maintained at >= 85.0% (target 100.0%).
- **Boundary Validation:** Test suites must explicitly exercise algorithm confusion/downgrade attacks, expired tokens, key mismatches, and missing claims.
- **Negative Testing:** All exported error sentinels must be verified for message consistency and `errors.Is` wrapping compatibility.
