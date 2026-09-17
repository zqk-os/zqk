# E2E Test Infra: CAP Strategy Iteration 2

**Last Verified:** 2026-08-31


## Test Philosophy
- Opaque-box, requirement-driven. No dependency on implementation design.
- Methodology: Category-Partition + BVA + Pairwise + Workload Testing.

## Feature Inventory
| # | Feature | Source (requirement) | Tier 1 | Tier 2 | Tier 3 |
|---|---------|---------------------|:------:|:------:|:------:|
| 1 | Swarm Queue & Dispatch | R1 | 5 | 5 | ✓ |
| 2 | Failed Task Feedback Loop | R2 | 5 | 5 | ✓ |
| 3 | Concurrent Spec Generation | R3 | 5 | 5 | ✓ |
| 4 | Autonomy Metrics Dashboard | R4 | 5 | 5 | ✓ |

## Test Architecture
- Test runner: `go test ./pkg/swarm/...` and `go test ./pkg/specbuilder/...` and CLI execution tests.
- Test case format: Unit and Integration tests.

## Real-World Application Scenarios (Tier 4)
| # | Scenario | Features Exercised | Complexity |
|---|----------|--------------------|------------|
| 1 | E2E Swarm Execution with Failures & Concurrency | F1, F2, F3, F4 | High |

## Coverage Thresholds
- Tier 1: ≥5 per feature
- Tier 2: ≥5 per feature
- Tier 3: pairwise coverage of major feature interactions
- Tier 4: ≥5 realistic application scenarios
