# ADR: Zero-Duplication CI Matrix Coverage Aggregation & Telemetry

## Status
Accepted (Invariant Enforced)

## Context & Problem Statement
In earlier iterations of continuous integration pipelines, test suites were executed twice: once for fast partitioned unit testing, and a second redundant monolithic run solely to capture whole-repository coverage profiles. As noted during release gate audits, executing duplicate test passes simply to generate coverage reports is highly inefficient, inflates runner costs, and introduces pipeline latency.

To ensure rapid developer feedback loops and preserve resource hygiene, ZQK requires a zero-duplication coverage aggregation architecture.

## Decision & Architectural Architecture
We establish the following architectural invariants for test execution and coverage telemetry:

1. **Partitioned Concurrent Execution**:
   - The test matrix partitions Go packages across isolated suite shards (`core-kernel`, `storage`, `cli-commands`, `objects-and-specs`, `agent-and-orchestration`, `validation-and-tpm`, `scheduler`).
   - Each shard runs `go test` with `-coverprofile="coverage-${{ matrix.suite }}.out"` concurrently during primary verification. Zero separate coverage-only test passes are allowed.

2. **Artifact-Driven Telemetry Capture**:
   - Each shard uploads its individual coverprofile as a GitHub Actions artifact (`shard-coverage-${{ matrix.suite }}`).
   - Artifacts are verified against sha256 checksums and tracked in pipeline step telemetry.

3. **Deterministic Statement Profile Consolidation (`pkg/coverage`)**:
   - A single downstream consolidation job (`consolidate-coverage`) downloads the shard artifacts.
   - Using the zero-duplication merger engine (`pkg/coverage/merger.go` and `scripts/open-core/coverage-rollup`), statement blocks across all shards are normalized, deduplicated, and combined into `coverage.out` and `package-coverage-rollup.json` in sub-second execution time.
   - For statements covered by multiple test suites, execution counts are atomically aggregated (`Count += block.Count`).

4. **Negative Adversarial Verification & Boundary Invariants**:
   - Corrupted or syntactically invalid profile lines are rejected fail-closed.
   - Shard files with mismatched coverage modes (e.g. `set` vs `atomic`) are rejected.
   - Shards declaring conflicting statement counts for the identical source block are flagged and rejected.
   - Path traversals outside the repository root are strictly prohibited.

## Consequences & Verification
- **Pipeline Efficiency**: CI pipeline execution time is reduced by eliminating redundant passes; all coverage metrics derive strictly from primary test execution.
- **Traceability**: All statement coverage rollups are verified via `TestMatrixCoverage_StaticFloorInvariants`, `TestMatrixCoverage_OperationalDynamicProof`, and `TestMatrixCoverage_NegativeAdversarialBoundaries` in `pkg/coverage`.
