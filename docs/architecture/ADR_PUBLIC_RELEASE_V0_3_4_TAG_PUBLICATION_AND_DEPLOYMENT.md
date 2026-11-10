# ADR: Public Release v0.3.4 Tag Publication & Documentation Portal Deployment

## Status
Accepted (Publication & Deployment Protocol Certified)

## Context & Motivation
With release candidate pre-flight verification gates certified in Cycle 83 and merged to `main`, this architectural decision establishes the canonical release tag publication, binary packaging, and public documentation deployment protocol for **v0.3.4**.

## Decision & Execution Protocol

### 1. Release Tag Specification & Provenance
- Target Version Tag: `v0.3.4`
- Tag commit must derive directly from fast-forwarded `main` containing certified pre-flight release gates.
- Signed release tag metadata references release notes at `docs/releases/v0.3.4.md`.

### 2. Multiplatform Binary Distribution Manifest
Cross-platform archive bundles built and validated against checksums:
- `zqk_v0.3.4_darwin_arm64.tar.gz` (SHA-256 verified)
- `zqk_v0.3.4_darwin_amd64.tar.gz` (SHA-256 verified)
- `zqk_v0.3.4_linux_arm64.tar.gz` (SHA-256 verified)
- `zqk_v0.3.4_linux_amd64.tar.gz` (SHA-256 verified)
- `checksums.txt`

### 3. Documentation Portal Compilation & Live Deployment
- Every release cut mandates automated compilation of the public documentation portal and deployment to `docs.zqk.dev`.
- Deployment pipeline executed via `scripts/open-core/docs-portal/deploy-docs-portal.sh`.
- Build artifacts validated for static link integrity, responsive console search, and zero dead links.

### 4. Zero Studio-Leak & Boundary Invariants
- Production documentation and release archives strictly prohibit internal studio nanos-hex identifiers.
- Public release payload gate (`scripts/open-core/check-public-release-payload.sh`) enforces zero process instance leaks, zero private keys, and directory traversal protections.

## Verification
- Pre-release gate verification passing 100% in `pkg/releasegate`.
- `scripts/open-core/docs-portal/deploy-docs-portal.sh --dry-run` verifies site generator and asset compilation.
- Binary executable permissions and tar/zip compression verified via `TestVerifyPublicReleasePayload`.
