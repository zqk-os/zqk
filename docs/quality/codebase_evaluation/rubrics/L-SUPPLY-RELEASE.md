# Rubric: L-SUPPLY-RELEASE

**Lens:** `L-SUPPLY-RELEASE` · **Density:** D-HIGH · **Axes:** SEC, OPS, RCV

## Criteria (exhaustive checklist)

1. LICENSE / NOTICE / copyright present and consistent.  
2. Dependency lockfiles committed where ecosystem expects them.  
3. Release artifacts: reproducible notes, checksums, provenance if claimed.  
4. CI presence for build/test on primary branches.  
5. Secret scanning / .gitignore for local state.  
6. SBOM or dependency review process — present or tooling gap.  
7. Install path works without private credentials **if** public distribution is claimed.

## Citations

| Work | Point |
|------|-------|
| NIST SSDF | Secure releases; provenance |
| SLSA framework (conceptual) | Supply-chain levels — cite only if claiming provenance |
| OpenSSF Scorecard concepts | Heuristic checks for OSS health |
| SPDX / license best practices | License clarity |

## Pros / cons

| Pros | Cons |
|------|------|
| Discrete yes/no checks — low hallucination | Scorecard cosplay without running tools — prefer actual commands |
| High launch relevance generally | Private monorepos may fail “public install” checks — mark N/A with reason |
