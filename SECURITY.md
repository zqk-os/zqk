# Security policy

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability.

Use GitHub's private vulnerability reporting for the `zqk-os/zqk` repository,
or email [security@zqk.dev](mailto:security@zqk.dev). Include the affected version
or commit, reproduction steps, impact, and any suggested mitigation. Maintainers
will acknowledge a complete report within five business days and coordinate
disclosure after a fix is available.

## Supported versions

Until the first tagged public release, security fixes apply to the current
default branch only.

## Automated Verification & Dependency Security

- **Secret Scanning**: Automated secret scanning is enforced on all pull requests and branch updates via `.github/workflows/secret-scan.yml` and pre-commit checks to prevent credential leaks.
- **Dependency Management**: Automated dependency updates and security scanning are managed through **Dependabot** configured for Go modules (`gomod`) and GitHub Actions.
- **Software Bill of Materials (SBOM)**: Formal **SBOM** manifests are continuously generated and published for releases via `.github/workflows/sbom.yml`.
