# Contributing to ZQK Community Edition

Welcome to the ZQK open-core project! We welcome contributions from human engineers and autonomous agents alike. This guide details how to set up your local development environment, conform to kernel invariants, run test suites, and submit pull requests.

---

## 1. Prerequisites & Environment Setup

- **Go**: Version 1.23 or higher
- **Make**: Standard GNU make
- **Git**: Version 2.30 or higher

### Initial Setup & First Run

```bash
# Clone and build
git clone https://github.com/zqk-os/zqk.git
cd zqk
make

# Verify version
./bin/zqk --version

# Onboard agent host and discover active work
./bin/zqk system agent-onboard --format json
./bin/zqk workflow whats-next --format json
```

For a guided first-run walkthrough, see [Community First-Run Guide](docs/onboarding/COMMUNITY_FIRST_RUN.md).

---

## 2. Core Development Invariants

1. **Cellular Membrane Boundary (Mode B)**:
   - All state machine mutations under `.zqk/process/` must be mediated through the CLI (`./bin/zqk object create|update|promote|demote|park`).
   - Never manually edit or write hash-named YAML files under `.zqk/process/`. Direct disk writes are blocked fail-closed.
2. **Environment Hygiene**:
   - Do **not** set or export `ZQK_PROJECT_ROOT` in your persistent shell profile. The CLI automatically resolves the nearest project root.
3. **Traceability & Evidence**:
   - Backlog items promoted to `complete` require git mutation evidence outside `.zqk/process/` and must reference the corresponding `BLI-*` ID in the commit message.

---

## 3. Branching & Commit Workflow

- **Integration Branches**: Autonomous workstreams operate on integration branches named `integration/<priority-plan-id>` (e.g. `integration/pri-community-docs-governance-001`).
- **Feature Branches**: Human contributors should create feature branches from `origin/main` named `feature/<topic>` or `fix/<topic>`.
- **Commit Format**: Follow Conventional Commits format with explicit backlog item and priority plan citations:

```text
feat(scheduler): align test concurrency limiter with dispatch budget

- Isolate global test concurrency limiter from package wait timeouts
- Record regression tests in global_test_concurrency_test.go

BLI-SCHEDULER-CONCURRENCY-001
PRI-SCHEDULER-REORG-001
```

---

## 4. Verification & Testing Gates

Before opening a pull request, ensure all validation gates and release checks pass cleanly:

```bash
# 1. Run unit and integration tests
go test ./...

# 2. Verify Knowledge Kernel integrity and schema compliance
./bin/zqk system check

# 3. Verify I/O hygiene and reap stale locks
./bin/zqk system resource-hygiene

# 4. Run the full public release gate suite
./scripts/open-core/test-public-release-gates.sh
```

---

## 5. Pull Request Lifecycle & Definition of Done

1. **Self-Contained Commits**: Group related changes logically; include test cases for all new functionality or bug fixes.
2. **Pre-Commit Compliance**: Pre-commit hooks will automatically verify the Test Matrix Definition of Done and ensure unbroken lineage from criteria to root objects.
3. **Review & Promotion**: Maintainers and TPMs will review the PR. Once merged, continuous autonomous workflows proceed with post-merge self-continuation.

---

## 6. Community & Governance

- Project governance, decision-making, and open-core boundaries are defined in [GOVERNANCE.md](GOVERNANCE.md).
- File bugs and enhancements with the GitHub issue templates under `.github/ISSUE_TEMPLATE/`.
- Security policies and vulnerability disclosures are governed by [SECURITY.md](SECURITY.md).
- Licensed under the Apache License 2.0 (`LICENSE` + `NOTICE`).
