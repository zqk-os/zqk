# ADR: Public-Facing Entity Integrity, Portal Link Rot Sentinel & Canonical Install Dispatcher

## Context
During public adoption readiness evaluation for release `v0.3.5`, deep inspection of public-facing endpoints and documentation revealed two critical integrity defects:
1. **Workstation Path & Scheme Leakage**: LLM prompt formatting instructions previously resulted in 371 `file:///Users/<developer>/...` URI links leaking into 45 markdown guides and generated HTML documents. The existing link-checker (`scripts/open-core/check-doc-links.sh`) inadvertently permitted `/Users/*` paths rather than failing closed.
2. **Symlink Failure on Raw GitHub Distribution**: The repository root `install.sh` was maintained as a symbolic link pointing to `scripts/install.sh`. When community operators executed `curl -fsSL https://zqk.dev/install.sh | bash` (redirected by Cloudflare to `https://raw.githubusercontent.com/zqk-os/zqk/main/install.sh`), GitHub's raw file server returned the literal ASCII string `"scripts/install.sh"` rather than following the symlink, causing client-side script execution to fail immediately with `No such file or directory`.

## Decision
1. **Canonical Regular File Distribution for Root Installers**:
   - The root `install.sh` is codified as a regular POSIX executable file (`0755`) identical byte-for-byte to `scripts/install.sh`, eliminating reliance on symlink resolution across raw HTTP content mirrors and public endpoints (`https://zqk.dev/install.sh`).
2. **Zero-Tolerance Fail-Closed Link Sentinel**:
   - `scripts/open-core/check-doc-links.sh` is upgraded to immediately flag any `file://` URI scheme or workstation path (`/Users/`, `/home/`, `/private/var/`) as a fatal validation error.
   - `scripts/open-core/docs-portal/generate_docs_portal.py` enforces automated normalization of workspace markers (`/zqk-public-candidate/`, `/zqk/`, `/zqke/`) to canonical relative documentation paths or GitHub repository blob references (`https://github.com/zqk-os/zqk/blob/main/...`).
3. **Three-Fold Proof Test Governance (`pkg/community/link_sentinel_test.go`)**:
   - **Static Floor Invariant (`TestLinkSentinel_StaticFloorInvariant`)**: Crawls all markdown files under `docs/` asserting 0 leaked developer usernames (`/Users/<developer>`), 0 `file://` link targets, and verifies `install.sh` is a non-symlink regular file matching `scripts/install.sh`.
   - **Operational Dynamic Proof (`TestLinkSentinel_OperationalDynamicProof`)**: Runs `check-doc-links.sh` against the entire codebase verifying 100% clean link resolution across all articles.
   - **Negative Adversarial Boundary (`TestLinkSentinel_NegativeAdversarialBoundary`)**: Injects adversarial documents containing leaked developer workstation paths and asserts fail-closed termination.
4. **Mandatory Documentation Portal Synchronous Deployment**:
   - Documentation portal assets are compiled and published directly to `zqk-os/zqk-docs` (`docs.zqk.dev`) with zero workstation leaks or unresolvable URI artifacts.

## Status
Accepted and verified for Cycle 98 (Public-Facing Entity Integrity & Portal Link Sentinel).
