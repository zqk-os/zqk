# Architectural Decision Record: Automated CI Coverage Aggregation & Duplication Elimination Gate

## Status
Accepted

## Context
The ZQK Continuous Integration pipeline previously ran unit tests in two distinct workflows:
1. `unit-tests`: A matrix of 7 partitioned parallel shards covering the entire repository (`core-kernel`, `storage`, `cli-commands`, `objects-and-specs`, `agent-and-orchestration`, `validation-and-tpm`, `scheduler`).
2. `extended-unit-tests`: A redundant non-matrix job that re-executed over 80% of all package suites previously executed in the matrix shards.

This duplication introduced several critical systemic issues:
- **Redundant Compute & Execution Lag:** Running tests twice on the same commit consumed excessive GitHub Actions runner minutes and prolonged PR feedback cycles by over 13 minutes per run.
- **Coverage Disparity:** Matrix shards and extended unit tests generated separate coverage outputs (`coverage-<suite>.out` vs `coverage-extended.out`), creating confusion over which gate governed release certification.
- **Flake Amplification:** Duplicative execution of identical test suites doubled the probability of transient timing flakes on CI runners without providing any additional static or dynamic coverage proof.

## Decision
1. **Single Source of Truth for Test Execution:**
   - The 7 partitioned shards in the `unit-tests` matrix are the exclusive and canonical execution harness for all unit and package verification suites.
   - The separate `extended-unit-tests` job is completely deprecated and removed from the CI workflow.

2. **Shard-Based Coverage Rollup:**
   - Each matrix shard produces an isolated coverage profile (`coverage-${{ matrix.suite }}.out`).
   - The `consolidate-coverage` job aggregates all shard profiles using `scripts/open-core/coverage-rollup`.
   - Merged coverage is compiled into a single `coverage.out` artifact, generating cryptographic verification telemetry (`package-coverage-rollup.json` and `.sha256` sidecar).

3. **Gated Dependency Topology:**
   - The `build` step depends directly on `[lint-and-format, unit-tests, consolidate-coverage]`, guaranteeing that binaries are only compiled and staged after all test shards and coverage consolidation gates pass.

## Consequences
- **Positive:**
  - 100% elimination of redundant test executions across CI workflows.
  - Reduction of PR CI turnaround times by ~13-15 minutes per run.
  - Unified coverage accounting across all packages and tools.
- **Negative / Risks:**
  - Shard package mappings must remain strictly exhaustive across `pkg/` and `cmd/` so no newly added packages are orphaned. Enforced by repository hygiene policies.
