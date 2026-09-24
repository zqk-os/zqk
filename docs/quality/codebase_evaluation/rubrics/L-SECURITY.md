# Rubric: L-SECURITY

**Lens:** `L-SECURITY` · **Density:** D-MED · **Axes:** SEC, ROB

## Criteria

1. Trust boundaries (authn/authz, multi-tenant, network exposure).  
2. Input validation / output encoding.  
3. Secrets handling (not in repo; rotation; memory).  
4. Dependency/supply risk (lockfiles, known vuln process).  
5. Dangerous defaults (open bind, debug left on, permissive CORS).  
6. Agentic/tool surfaces if present: prompt/tool injection, confused deputy, forgeable acknowledgements.

## Citations

| Work | Point |
|------|-------|
| OWASP ASVS | Verification requirements by level |
| OWASP Top 10 | Common web risks (when applicable) |
| NIST SSDF (SP 800-218) | Secure software development practices |
| NIST SP 800-53 (select controls) | Audit/auth concepts when systems are sensitive |

## Pros / cons

| Pros | Cons |
|------|------|
| Industry-standard vocabulary | ASVS Level overreach on small CLIs — state assumed level |
| Includes agentic abuse (modern) | Easy to speculate threats without anchors — require E2 paths |

## Severity guidance

- Secrets in tree → critical  
- Missing auth on exposed network admin → critical/high  
- Theoretical threat without entry point → info/low or retract
