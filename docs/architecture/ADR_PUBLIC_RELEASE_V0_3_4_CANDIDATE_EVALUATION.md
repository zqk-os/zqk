# ADR: Public Release v0.3.4 Candidate Readiness Evaluation & Pre-Flight Certification

## Status
Accepted (Release Candidate v0.3.4 Certified)

## Context & Motivation
Following extensive architectural improvements and issue remediation across Cycles 75 through 82, this evaluation assesses system maturity and release readiness for public release **v0.3.4**. Multiple high-impact community-reported GitHub issues (#1014, #1018, #1019) and developer ergonomics concerns have been systematically resolved and verified on `main`. Preparing a certified release candidate enables immediate adoption of these stability enhancements by external developers and community agents.

## Evaluation of System Advancements & Burndown

### 1. Closed GitHub Issues & Stability Enhancements
- **GH-1014 (Concurrent CAS Atomic Writes)**: Multi-agent burst writes to the Knowledge Kernel CAS previously suffered race conditions during high-volume swarms. Resolved via lockless CAS staging and verified atomic object commits.
- **GH-1018 (`zqk system check` Large Object Graph Timeout)**: Fixed 10s default check timeout that triggered spurious context cancellations across 3,000+ object graphs. Resolved via streaming graph validation and batch pipelining in `cmd/zqk/system/async_check.go`. Full validation now completes in under 3.0s across all active and archived objects.
- **GH-1019 (`object update --unset-field` Precondition Deadlock)**: Child status progression previously caused updates on parent entities with obsolete or stray non-immutable fields to fail closed. Resolved by introducing the `--unset-field` CLI flag, allowing explicit removal of deprecated attributes without manual CAS manipulation.
- **PR #1025 & #1033 (CI Stability & Flake Elimination)**: Eradicated intermittent runner timeouts caused by redundant whole-tree coverage passes.

### 2. Architectural Invariants Enforced
- **Zero-Vendor Hardcoding & IDE Decoupling (PR #1039, Cycle 81)**: Decoupled all host assistant and IDE specific logic into pluggable adapter packages (`pkg/adapters/ide/`). Zero vendor-specific hacks, string literals, or paths remain in the core kernel.
- **Zero-Duplication CI Matrix Coverage Aggregation (PR #1049, Cycle 82)**: Replaced duplicate whole-repository test runs with single-pass matrix shard coverage telemetry combined deterministically via `pkg/coverage/merger.go`.
- **Dynamic Object Resolution in Automation Scripts (PR #1045, Cycle 80)**: Eliminated hardcoded kernel object IDs across shell scripts and gate hooks. Scripts dynamically query the CLI (`zqk object get <id> --format json`) ensuring resilience across schema and object lifecycles.
- **Process I/O & File Descriptor Hygiene**: Validated multi-daemon long-running memory and descriptor usage. The ambient daemon, scheduler, steward, privileged writer, and IDE adapter consistently operate under the 100 FD ceiling (averaging 13–43 open FDs under live load).

## Release Gate Verification Matrix
All pre-flight verification gates codified in `pkg/releasegate/` satisfy the Three-Fold Verification standard:

| Verification Stage | Gate Description | Target Verification | Result |
| :--- | :--- | :--- | :--- |
| **Static Floor Invariants** | Cross-platform multi-architecture compilation (`darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`) & zero ghost graph references. | `pkg/releasegate` multiplatform compilation test suite | **PASS** |
| **Operational Dynamic Proof** | End-to-end payload packaging, tarball/zip archive construction, executable bit retention, license embedding, and checksum integrity. | `scripts/open-core/check-public-release-payload.sh` & `TestVerifyPublicReleasePayload` | **PASS** |
| **Negative Adversarial Boundaries** | Fail-closed rejection of process instance data, studio kernel IDs, sensitive files (.DS_Store, private keys), directory traversals, and insecure symlinks. | `TestPublicPayloadCheck_BoundaryAndErrorHandling` | **PASS** |

## Resolved Technical Debt & Issue Backlog
The following technical debt areas in the Knowledge Kernel CAS are verified and promoted to resolved:
- **GH-1018**: Remediate `zqk system check` timeout on deep object graphs.
- **GH-1019**: Implement `--unset-field` on `zqk object update` to clear stray properties.
- **GH-1014**: Eliminate write contention in concurrent agent CAS transactions.
- **Graph Streamlining**: Streamline graph validation pipeline to sub-3s latency.

## Distribution Manifest
The candidate release bundle for `v0.3.4` comprises:
1. `zqk_v0.3.4_darwin_arm64.tar.gz` (macOS Apple Silicon)
2. `zqk_v0.3.4_darwin_amd64.tar.gz` (macOS Intel)
3. `zqk_v0.3.4_linux_arm64.tar.gz` (Linux ARM64)
4. `zqk_v0.3.4_linux_amd64.tar.gz` (Linux AMD64)
5. `checksums.txt` (SHA-256 verification hashes)
6. Release notes and public docs deployed via `scripts/open-core/docs-portal/deploy-docs-portal.sh`.
