# Security Policy

## Reporting a vulnerability

Please report security issues privately to the maintainer via GitHub Security Advisories for this repository or by emailing [support@zqkos.com](mailto:support@zqkos.com). Do not open public issues for exploitable findings until a fix or coordinated disclosure plan exists.

## Vulnerability Triage and Response SLA Matrix

We commit to the following response SLAs based on CVSS severity classifications:

| Severity | CVSS v3.1 Range | Initial Triage Response | Remediation / Advisory Target |
| :--- | :--- | :--- | :--- |
| **Critical** | 9.0 – 10.0 | ≤ 24 hours | ≤ 72 hours (Hotfix release) |
| **High** | 7.0 – 8.9 | ≤ 48 hours | ≤ 7 calendar days |
| **Medium** | 4.0 – 6.9 | ≤ 5 business days | ≤ 30 calendar days (Next minor release) |
| **Low** | 0.1 – 3.9 | ≤ 10 business days | Next scheduled release cycle |

## Security Advisory Classifications

1. **Remote Code Execution (RCE) / Privilege Escalation:** Critical hotfix deployed immediately with patch releases and homebrew tap update.
2. **Unauthorized Kernel Access / Auth Bypass:** High priority resolution with CVE allocation via GitHub Security Advisories.
3. **Information Disclosure / PII Leakage:** Immediate patch ensuring local telemetry scrubbing and prompt notification.
4. **Denial of Service (DoS):** Medium/Low priority resolution with algorithmic or rate-limiting guards.

## Coordinated Disclosure Process

1. **Report:** Submit vulnerability report via GitHub Security Advisories or contact `support@zqkos.com`.
2. **Acknowledge:** The team acknowledges receipt within the specified SLA window above.
3. **Assessment:** Maintainers assess exploitability, draft reproduction scripts, and prepare confidential patch branches.
4. **Coordination:** Maintainers coordinate release dates, issue CVE IDs, and publish advisories alongside patched release binaries.

## Scope

- ZQK CLI, MCP daemon, scheduler, and privileged-writer local services
- Installer (`install.sh`) checksum verification and release artifacts
- Default network binds (loopback) and auth fail-closed behavior for local listeners

## Out of scope (typical)

- Misconfiguration of third-party IDE agents or vendor MCP clients
- Compromised host OS credentials outside ZQK’s keystore / process model

## Supply hygiene

- Release archives are verified against `checksums.txt` during `install.sh` (fail closed if verification cannot complete).
- Dependency and CI hygiene: see `.github/workflows/ci.yml` and `go.mod` / `toolchain` pins.
- Automated secret scanning: Gitleaks detection runs on all pull requests and branch pushes via `.github/workflows/secret-scan.yml` to prevent credential and private key leaks.
- Software Bill of Materials (SBOM): SPDX-formatted SBOM artifacts are generated in CI and releases via `.github/workflows/sbom.yml`.
- Automated Dependency Review: Dependabot checks for dependency security updates weekly via `.github/dependabot.yml`.

## Related docs

- `README.md` — Security basics (alpha)
- `docs/onboarding/COMMUNITY_FIRST_RUN.md` — first-run seating without degraded theater
- `docs/architecture/OPEN_CORE_PROPRIETARY_SPLIT.md` — open-core boundary
