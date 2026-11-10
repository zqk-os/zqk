# ADR: Automated Documentation Portal CI Invariant & Link Rot Sentinel

## Context
As the ZQK Core codebase expands with automated agents, modular packs, and subsystem guides, public documentation drift and broken hyperlinks (link rot) represent critical risks to developer adoption and ecosystem integrity. Previously, static documentation portal generation and link integrity checks were executed manually or during release packaging cuts, allowing broken relative links or orphaned anchors to linger in tree until deployment.

## Decision
1. **Continuous Documentation Portal CI Invariant**:
   - The static documentation portal generation script (`scripts/open-core/docs-portal/deploy-docs-portal.sh --dry-run`) is codified as a mandatory non-optional gate in the primary CI workflow (`.github/workflows/ci.yml`).
   - Every pull request and push to `main` must successfully compile the static portal without errors or regressions.
2. **Link Rot Sentinel & URI Normalization**:
   - The repository link validator (`scripts/open-core/check-doc-links.sh`) is integrated directly into CI static analysis.
   - The portal generator automatically normalizes local `file://` URI links into canonical GitHub repository references or relative HTML documentation targets, preventing environment leakage while maintaining link validity.
   - Internal link verification mandates zero broken links across all markdown documentation files in the repository.
3. **Fail-Closed Negative Boundary Testing**:
   - Automated test cases in `pkg/community/docs_portal_test.go` enforce that the link rot sentinel detects broken links and halts execution with a non-zero exit code.

## Status
Accepted and codified in Knowledge Kernel for Cycle 86 (Documentation Portal Automated CI Invariant & Link Rot Sentinel).
