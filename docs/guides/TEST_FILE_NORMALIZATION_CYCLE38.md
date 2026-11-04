# Repository Test File Normalization & Hygiene Migration (Cycle 38)

## 1. Executive Summary

During iterative coverage elevation cycles across the ZQK Knowledge Kernel (Cycles 11 through 35), test suites were authored into transiently named files matching `*_coverage_elevation_cycle<N>_test.go` and `*_cycle<N>_crit<M>_test.go`. While helpful during rapid sprint iterations for isolating new coverage assertions, retaining sprint cycle numbers in long-term repository filenames creates opaque indirection, hinders discoverability, and clutters the test taxonomy once test coverage baselines are permanently secured.

This architectural guide documents the structural migration, canonical naming conventions, Three-Fold Verification Proof, and automated tooling delivered in Cycle 38 under **REQ-1793779110030263000-646a7780** and **PRI-1793779152046546000-4aa29d9f**.

---

## 2. Canonical Naming Taxonomy

All test files are migrated from ephemeral sprint nomenclature to permanent domain-specific names according to their role in the Three-Fold Verification architecture:

| Ephemeral Sprint Pattern | Canonical Permanent Pattern | Example Legacy File | Canonical Replacement |
| :--- | :--- | :--- | :--- |
| `<pkg>_coverage_elevation_cycle<N>_test.go` | `<pkg>_<subsystem>_test.go` | `cmd/zqk/agent/agent_coverage_elevation_cycle137_test.go` | `agent_subcommands_and_guidance_test.go` |
| `<pkg>_coverage_elevation_cycle<N>_test.go` | `<pkg>_<feature>_test.go` | `cmd/zqk/query/query_coverage_elevation_cycle138_test.go` | `query_repl_and_execution_test.go` |
| `<pkg>_coverage_elevation_cycle<N>_test.go` | `<pkg>_<mechanism>_test.go` | `pkg/graph/memgraph/memgraph_coverage_elevation_cycle34_test.go` | `memgraph_extended_test.go` |

### Invariant Rules
1. **Zero Cycle Tags in Permanent Test Names**: Target filenames must never contain `_coverage_elevation_cycle` or `_cycle[0-9]+_`.
2. **AST-Derived Semantic Suffixes**: Filenames must reflect the Go `Test*` functions declared within each file.
3. **Collision Freedom**: Across 199 candidate test files in `cmd/` and `pkg/`, the collision check verifies 0 name collisions against the filesystem and peer targets.
4. **Preserved Domain Concepts**: Files representing domain graph concepts (such as `cap_cycle_test.go`, `ref_cycle_test.go`, and `spec_dependency_cycle_test.go`) are recognized as semantic domain tests and preserved.

---

## 3. Migration Tooling

Automated tooling is located in `scripts/open-core/test-normalization/`:

### 3.1 `normalize_cycle_test_names.sh`
- `--dry-run` (Default): Simulates all 199 renames, verifying source existence and target availability without touching files.
- `--check-collisions`: Formally verifies 0 collisions against filesystem entries and peer targets.
- `--validate`: Simulates renames in place and runs `go test -run=^$ ./cmd/... ./pkg/...` to assert compiler buildability, with automatic rollback trap on exit.
- `--apply`: Executes `git mv` across all candidate files.

### 3.2 Mapping Artifact
- Detailed file mapping catalog: [`docs/reports/REPO_HYGIENE_TEST_NORMALIZATION_MAP.md`](file:///Users/lanceettl/zqk-public-candidate/docs/reports/REPO_HYGIENE_TEST_NORMALIZATION_MAP.md).

---

## 4. Three-Fold Verification Proof

The test suite in [`pkg/community/test_hygiene_cycle38_test.go`](file:///Users/lanceettl/zqk-public-candidate/pkg/community/test_hygiene_cycle38_test.go) enforces:

1. **Static Floor Invariant (`TestTestNormalizationCycle38_StaticFloorInvariant`)**:
   - Asserts mapping artifact exists, defines >= 190 targets, and asserts zero destination filenames contain cycle elevation tags.
2. **Operational Dynamic Proof (`TestTestNormalizationCycle38_OperationalDynamicProof`)**:
   - Executes `--check-collisions` and `--dry-run`, asserting exit code 0 and clean completion.
3. **Negative Adversarial Boundary (`TestTestNormalizationCycle38_NegativeAdversarialBoundary`)**:
   - Verifies invalid flags fail closed with usage output, and confirms active in-flight files remain deferred.

---

## 5. Knowledge Kernel Traceability

- **Milestone**: `MIL-1793779146182730000-0aa5577d` (*Cycle 38: Repository Test File Normalization & Hygiene Migration*)
- **Requirement**: `REQ-1793779110030263000-646a7780`
- **Priority Plan**: `PRI-1793779152046546000-4aa29d9f`
- **Test Case**: `TST-1793779121489789001-e75aee8b`
- **Backlog Items**:
  - `BLI-1793779121489789500-d479b6dc` (Core Capability & Functional Delivery)
  - `BLI-1793779121489790000-3f6097d7` (Boundary Validation & Error Recovery)
  - `BLI-1793779121489790500-76fa9efb` (Architecture Specification & Conformance)
- **Technical Debt**: `TDE-1793777516472794000-2118fc77`
