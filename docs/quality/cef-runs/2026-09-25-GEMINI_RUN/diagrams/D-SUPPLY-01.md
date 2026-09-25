---
diagram_id: D-SUPPLY-01
type: flowchart
title: "ZQK Build, Release, and Installation Supply Chain Pipeline"
anchors:
  - path: .goreleaser.yaml
    symbol: builds
    note: "GoReleaser build configuration and ldflags"
  - path: install.sh
    symbol: install_source
    note: "Source build installation fallback"
  - path: tools/git-hooks/pre-commit
    symbol: pre-commit
    note: "Git pre-commit verification entrypoint"
  - path: .github/workflows/ci.yml
    symbol: lint-and-format
    note: "CI verification and lint pipeline"
claims:
  - "GoReleaser injects version ldflags into package main instead of cmd/zqk/app"
  - "install.sh invokes make zqk which lacks a corresponding target in Makefile"
  - "Secret scanning script is unreferenced by pre-commit hooks and CI workflows"
  - "Release pipeline produces unsigned archives without cosign or SLSA provenance"
evidence_grade: E2
---

# ZQK Supply Chain & Release Verification Architecture

```mermaid
flowchart TD
    subgraph LocalDev ["Local Workstation / Developer Plane"]
        GitCommit["git commit"] --> PreCommit["tools/git-hooks/pre-commit"]
        PreCommit --> GateBranch["0. Branch Protection Gate"]
        PreCommit --> GateCAS["1. CAS Membrane Gate"]
        PreCommit --> GateKernel["2. Kernel Integrity Gate"]
        PreCommit --> GateTDD["3. Test Matrix DoD Gate"]
        SecretScanScript["scripts/scan-secrets.sh"] -.->|NOT WIRED (F-SUPPLY-RELEASE-003)| PreCommit
    end

    subgraph CI ["GitHub Actions CI (.github/workflows/)"]
        PushPR["Push / Pull Request (main)"] --> CI_Lint["Lint & Format (ci.yml)"]
        CI_Lint --> Vet["go vet & golangci-lint"]
        CI_Lint --> PayloadGate["check-public-release-payload.sh"]
        CI_Lint --> SecretScanStep["Secret Scanning Gate (MISSING)"]
        PushPR --> CI_Unit["Partitioned Unit Tests"]
        PushPR --> CI_Coverage["Coverage Report (code-coverage.yml)"]
        PushPR --> CI_SBOM["Anchore SPDX SBOM (sbom.yml)"]
        CI_Unit --> CI_Build["Binary Build (make all)"]
        CI_Build --> CI_Integration["Release Gates & Integration Tests"]
    end

    subgraph ReleaseFlow ["Release Pipeline (.github/workflows/release.yml)"]
        TagPush["Tag Push (v*)"] --> GoReleaser["goreleaser/goreleaser-action (~> v2)"]
        GoReleaser --> BadLdflags["ldflags: -X main.version (MISMATCH: F-SUPPLY-RELEASE-001)"]
        BadLdflags --> BuiltArchive["Tar.gz Archives & checksums.txt"]
        BuiltArchive -.->|Unsigned: No Cosign / No SLSA (F-SUPPLY-RELEASE-009)| GitHubRelease["GitHub Releases (Public/Private)"]
    end

    subgraph ConsumerInstall ["Consumer Installation (install.sh)"]
        InstallRun["curl install.sh | sh"] --> Detect["Detect OS / Arch"]
        Detect --> MethodChoice{"Install Method"}
        MethodChoice -->|binary| DL["Download Archive & checksums.txt"]
        DL --> ChecksumVerify["Verify SHA-256 Checksum"]
        ChecksumVerify --> Extract["Extract & Install to /usr/local/bin"]
        MethodChoice -->|goinstall| GoInstall["go install github.com/zqk-os/zqk/cmd/zqk@latest"]
        MethodChoice -->|source| Clone["git clone --depth=1"]
        Clone --> MakeCmd["make -C src zqk (TARGET MISSING: F-SUPPLY-RELEASE-002)"]
    end

    GitHubRelease --> DL
```
