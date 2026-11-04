# Repository Test File Normalization & Hygiene Guide

## Executive Summary

During iterative coverage elevation cycles (e.g., Cycles 11–35), temporary test files were created with cycle-scoped nomenclature (such as `memgraph_coverage_elevation_cycle34_test.go`). While helpful for tracking localized coverage bursts during an active sprint, retaining cycle numbers in long-term repository filenames creates opaque indirection, hampers discoverability, and clutters the test taxonomy once coverage baselines are permanently secured.

This guide defines the canonical test file naming conventions and migration protocol for normalizing all cycle-scoped test files into permanent, descriptive test suites.

---

## Canonical Naming Standard

In accordance with Go test conventions and ZQK Community Kernel standards:

| Legacy / Ephemeral Pattern | Canonical Permanent Pattern | Example Migration |
| :--- | :--- | :--- |
| `<pkg>_coverage_elevation_cycle<N>_test.go` | `<pkg>_extended_test.go` | `pkg/graph/memgraph/memgraph_extended_test.go` |
| `<component>_coverage_elevation_cycle<N>_test.go` | `<component>_extended_test.go` | `pkg/graph/provider/provider_extended_test.go` |
| `<subsystem>_cycle<N>_crit<M>_test.go` | `<subsystem>_<feature>_test.go` | `pkg/storage/storage_extended_test.go` |

### Key Principles

1. **Permanence Over Sprints**: Test file names describe *what system or behavior is tested*, never *which sprint or calendar cycle* authored them.
2. **Zero Regressions**: Renaming must be performed via `git mv` to preserve Git blame and change history.
3. **Traceability Synchronization**: Any Knowledge Kernel test case objects (`test_case.path_or_id`) referencing renamed test files must be updated simultaneously via `./bin/zqk object update <tst-id> --field path_or_id="<new-path>"`.
4. **Race Detection & Zero Flakes**: All renamed suites must pass `go test -v -race` with zero unhandled panics and proper concurrency boundaries (`goroutinelabels.NewGoroutine`).

---

## Verification & Three-Fold Proof

The migration satisfies the canonical Three-Fold Verification Proof:

- **Static Floor Invariant (`TestCRIT_TestHygieneNormalization_StaticFloorInvariant`)**: Confirms that deprecated cycle-scoped files no longer exist on disk and that canonical replacements exist, exceed minimum size thresholds, and export valid test functions.
- **Operational Dynamic Proof (`TestCRIT_TestHygieneNormalization_OperationalDynamicProof`)**: Executes concurrent verification routines under `goroutinelabels` supervision to verify runtime hygiene.
- **Negative Adversarial Boundary (`TestCRIT_TestHygieneNormalization_NegativeAdversarialBoundary`)**: Rejects opaque naming regex patterns matching `.*_coverage_elevation_cycle\d+.*_test\.go$` while passing canonical naming schemes.
