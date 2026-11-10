# Zero-Duplication Parallel Matrix Coverage Consolidation Engine

## Status
- **Author**: PER-COMMUNITY-SYSTEM-ARCHITECT
- **Cycle**: Cycle 77
- **Status**: Approved / Draft

---

## 1. Executive Summary & Problem Mandate

In continuous integration and test verification, test execution time and resource utilization are primary constraints. Previously, the CI pipeline suffered from test duplication:
1. The 7 partitioned unit test suites in `ci.yml` executed all repository unit tests across parallel runner shards, producing individual coverage profiles (`coverage-core-kernel.out`, `coverage-storage.out`, etc.).
2. A separate standalone workflow `code-coverage.yml` re-executed all unit tests sequentially across `./pkg/...`, `./cmd/...`, and `./internal/...` purely to generate a monolithic `coverage.out` and package rollup report.
3. This duplication doubled test run duration, consumed excessive GitHub Actions runner minutes, introduced CI flake opportunities, and hid coverage discrepancies between the two workflows.

**Cycle 77 establishes the Parallel Matrix Coverage Consolidation Engine** (`pkg/coverage`):
1. **Multi-Shard Coverprofile Block Parser**: Parses Go coverage profile blocks (`path:startLine.startCol,endLine.endCol numStmt count`) across parallel matrix shards with sub-millisecond parsing throughput.
2. **Atomic Statement Deduplication & Mode Alignment**: Merges blocks across shard profiles, guaranteeing that shared packages and files are not double-counted in statement totals. Mode consistency (`atomic`, `count`, `set`) is verified fail-closed.
3. **Verified Package Coverage Rollup & CI Consolidation**: Integrates directly into the CI matrix lifecycle, eliminating redundant workflow runs and generating verified cryptographic coverage summaries (`package-coverage-rollup.json` + SHA-256 sidecar).

---

## 2. Architectural Topology & Invariants

```
   ┌─────────────────────── CI Matrix Shards (Parallel) ───────────────────────┐
   │  Shard 1: core-kernel           │  Shard 2: storage                       │
   │  -> coverage-core-kernel.out    │  -> coverage-storage.out                │
   │                                 │                                         │
   │  Shard 3: cli-commands          │  Shard 4: objects-and-specs             │
   │  -> coverage-cli-commands.out   │  -> coverage-objects-and-specs.out      │
   │                                 │                                         │
   │  Shard 5: agent-orchestration   │  Shard 6: validation-and-tpm            │
   │  -> coverage-agent.out          │  -> coverage-validation.out             │
   │                                 │                                         │
   │  Shard 7: ai-tools-scenarios    │                                         │
   │  -> coverage-ai-tools.out       │                                         │
   └─────────────────────────────────┬─────────────────────────────────────────┘
                                     │ (Upload Artifacts)
                                     ▼
   ┌───────────────────────────────────────────────────────────────────────────┐
   │            Parallel Matrix Coverage Consolidation Engine                  │
   │                       (`pkg/coverage.Merger`)                             │
   │                                                                           │
   │  1. Invariant Validation (Mode match, non-empty, path containment)        │
   │  2. Statement Deduplication: map[BlockKey]BlockEntry                      │
   │  3. Mode-Aware Count Aggregation (sum counts or bitwise OR)               │
   │  4. Canonical Serialization & Sorted Block Emission                       │
   └─────────────────────────────────┬─────────────────────────────────────────┘
                                     │
                                     ▼
   ┌───────────────────────────────────────────────────────────────────────────┐
   │                  Consolidated Coverage Deliverables                       │
   │  - `coverage.out` (Standard Go coverprofile format)                       │
   │  - `package-coverage-rollup.json` (Verified package statement telemetry)  │
   │  - `package-coverage-rollup.json.sha256` (Cryptographic integrity proof)  │
   └───────────────────────────────────────────────────────────────────────────┘
```

---

## 3. Three-Fold Verification Proof Specification

1. **Static Floor Invariants**:
   - Every profile block adheres strictly to the Go coverprofile syntax: `file:startLine.startCol,endLine.endCol numStmt count`.
   - Shard coverprofile metadata schemas and package mapping dictionaries maintain immutable structural consistency.
   - Initial header must declare a recognized coverage mode: `set`, `count`, or `atomic`.

2. **Operational Dynamic Proof**:
   - Ingests multiple coverage shard profiles concurrently.
   - Deduplicates identical statement blocks across shards without inflating total statement counts.
   - Accurately sums execution counts for `mode: atomic` and `mode: count`, and takes the logical OR for `mode: set`.
   - Generates verified package rollup JSON with valid SHA-256 sidecars.

3. **Negative Adversarial Boundaries**:
   - Rejects empty shard sets or non-existent profile paths.
   - Rejects malformed profile lines (non-integer line/column numbers, non-positive statement counts, negative counts).
   - Rejects mode mismatch across shards (e.g. attempting to merge `mode: set` with `mode: atomic`).
   - Rejects conflicting statement counts (`numStmt`) for the identical source file block.
   - Rejects path traversal attempts (`../`) or file paths resolving outside the configured module/project root.
