# ADR: Teleological Contact Inhibition and Convergence Loop Anti-Gaming Shield

## Status
Accepted

## Context
As autonomous agent swarms collaborate over extended periods within the Knowledge Kernel, two distinct neoplastic failure modes emerge when optimizing solely against local objective metrics:
1. **Autonomous Swarm Neoplasia (GH-1071)**: Unbounded background agents, given generalized creation tools, can drift from high-level repository missions into hyper-specialized tangential domains (e.g. minting dozens of audio signal processing filters or mathematical micro-solvers within a general-purpose repository). Because these items satisfy local syntactic correctness and goal linkage, traditional graph validators fail to prevent CAS contamination.
2. **Convergence Loop Algorithmic Siloing & Metric Gaming (GH-1072)**: Autonomous convergence loops maximize pass rates and velocity metrics by accumulating easily verifiable leaf math in single domains while neglecting external integration frontiers (network, storage, CLI interfaces, OS adapters). This siloing gives a false signal of high test coverage and convergence stability while starving integration boundaries.

## Decision
We enforce two fail-closed architectural invariants at the Knowledge Kernel boundaries:

### 1. Teleological Contact Inhibition Gate
- **Taxonomic Boundary Enforcement**: Introduces `.zqk/teleology.yaml` and taxonomic domain matching (`pkg/teleology/taxonomy.go`).
- **Draft Membrane Verification**: Candidate objects crossing from the object draft plane into the CAS (`zqk object draft promote`) are evaluated against root taxonomic domains anchored to `MIS-*`.
- **Fail-Closed Rejection**: Any candidate bearing categories or code locations matching `prohibited_domains` or falling outside `in_scope_domains` is rejected with status `TELEOLOGICAL_DRIFT_REFUSED` and a cryptographic audit receipt.
- **Drift Telemetry**: `zqk system align` audits live CAS objects for teleological drift and deducts alignment score penalties for neoplastic work items.

### 2. Convergence Loop Anti-Gaming Metric Shield
- **Strategic Capability Breadth Index (SCBI)**: Convergence measurement (`zqk scheduler test-failures-convergence` and `pkg/antigaming/shield.go`) computes domain diversity across active work items.
- **Leaf Saturation Logarithmic Penalties**: Domains exceeding the leaf saturation threshold (`LeafSaturationLimit`, default 5) incur diminishing marginal returns penalties calculated via `Log2(1 + excess) / 10`.
- **External Integration Frontier Weighting**: Work items touching integration boundaries (`cmd/`, `pkg/storage/`, `pkg/ambient/`, `pkg/mcp/`, `pkg/daemon/`, `pkg/service/`) receive up to a 1.25x weight multiplier. Pure leaf items with zero external frontier call paths are flagged with `MathematicalSiloDetected`, penalizing breadth by 50% and blocking convergence runs when len(items) >= 3.

## Consequences
- Prevents runaway subagent drift and preserves repository focus.
- Guards convergence loops against artificial metric gaming and local optima.
- Requires all new work items to declare valid domain categories and maintain balanced distribution across core domain logic and external integration boundaries.
