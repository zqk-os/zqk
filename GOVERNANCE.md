# Project Governance

The ZQK open-core project is an autonomous, deterministic, and self-hosted system kernel engineered for human-agent collaboration and sovereign software lifecycle execution. This document defines the project's governance structure, decision-making framework, open-core boundaries, and maintenance liaison expectations.

---

## 1. Core Principles

1. **Kernel Sovereignty**: All process data, state machines, and task cascades live in the local Knowledge Kernel (under `.zqk/process/` and content-addressable storage), never in vendor-proprietary memory stores.
2. **Determinism & Cryptographic Verification**: Work mutations, approvals, and releases must produce reproducible cryptographic evidence (`qa_success`, CAS hashes, and signed provenance).
3. **Cellular Membrane Boundaries**: Mutations to kernel state are mediated exclusively through the ZQK CLI and privileged intake boundaries. Raw disk tampering of process files is strictly forbidden.
4. **Autonomous Anti-Idleness**: Systems and autonomous TPM loops operate under continuous milestone progression without artificial halt states.

---

## 2. Decision-Making & Technical Steering

The project is stewarded by the **Technical Steering Committee (TSC)**, comprised of active project maintainers and autonomous TPM liaisons.

### Roles & Responsibilities

- **Primary Coordinating TPM**: Coordinates cross-workstream synchronization, milestone scheduling, and candidate release gating.
- **Peer Orchestrating TPMs**: Manage specific technical domains (CLI taxonomy, I/O hygiene, test convergence, documentation governance) and drive autonomous backlog convergence.
- **Maintainers & Code Craftsmen**: Own package architectures, code reviews, security boundaries, and patch integration.
- **Quality Assurance & Verification Sentinels**: Audit test matrices, Definition of Done criteria, and cryptographic evidence chains.

### Consensus & Escalation

1. **Lazy Consensus**: Routine improvements, documentation fixes, and criteria promotions operate on lazy consensus. Absence of objections within 24 hours constitutes acceptance.
2. **Technical Disputes**: In the event of competing architectural proposals, decisions are resolved by majority vote among the active maintainers and lead TPMs.
3. **Security & Break-Glass**: Urgent security patches and vulnerability mitigations follow the fast-track disclosure and remediation protocol documented in `SECURITY.md`.

---

## 3. Open-Core Boundary & Licensing

- **Canonical Open-Core Foundation**: ZQK Core is the canonical open-source system kernel. It provides the full offline-first runtime, content-addressable storage (CAS), task scheduler, ambient daemons, and Model Context Protocol (MCP) server.
- **Licensing**: ZQK Core is licensed under the **Apache License 2.0** (`LICENSE` + `NOTICE`). It includes the complete core CLI runtime, Knowledge Kernel, scheduler, ambient daemons, MCP bridge, and showcase suites.
- **Hermetic Architecture**: Core remains hermetic, self-contained, and free of proprietary dependencies or external assumptions. All build, verification, installation, and daemon lifecycle workflows function independently out of the box.
- **Release Gate Compliance**: The codebase is continuously audited by `./scripts/open-core/test-public-release-gates.sh` and `./scripts/open-core/check-public-release-payload.sh` to guarantee self-contained execution and zero proprietary leakage.

---

## 4. Maintenance Liaisons & Release Cadence

- **Candidate Verification**: Every release candidate must pass `./scripts/open-core/test-public-release-gates.sh` and achieve a clean, zero-warning report from `./bin/zqk system check`.
- **Release Versioning**: Follows Semantic Versioning (`vMAJOR.MINOR.PATCH`).
- **Triage Cadence**: Issues and incoming PRs are reviewed continuously by community liaisons. Contributors can inspect current backlog priorities at any time via `./bin/zqk workflow whats-next`.
